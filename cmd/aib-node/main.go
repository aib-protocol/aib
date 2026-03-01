package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/aib-protocol/aib/pkg/api"
	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// NodeConfig 节点配置
type NodeConfig struct {
	DataDir   string
	APIPort   int
	P2PPort   int
	Validator bool
	LogLevel  string
	Bootstrap string
	NodeID    string
}

// Node AIB节点
type Node struct {
	config     *NodeConfig
	logger     *log.Logger
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// 核心组件
	chain      *SimpleChain
	utxoStore  *utxoPkg.UTXOStore
	consensus  *utxoPkg.ConsensusState
	mempool    *utxoPkg.Mempool
	apiServer  *api.Server
}

// SimpleChain 简化的区块链
type SimpleChain struct {
	blocks    []*utxoPkg.Block
	utxoStore *utxoPkg.UTXOStore
	mu        sync.RWMutex
}

func NewSimpleChain(utxoStore *utxoPkg.UTXOStore) *SimpleChain {
	return &SimpleChain{
		blocks:    make([]*utxoPkg.Block, 0),
		utxoStore: utxoStore,
	}
}

func (c *SimpleChain) AddBlock(block *utxoPkg.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocks = append(c.blocks, block)
	return nil
}

func (c *SimpleChain) GetHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return uint64(len(c.blocks))
}

func (c *SimpleChain) GetLatestBlock() *utxoPkg.Block {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.blocks) == 0 {
		return nil
	}
	return c.blocks[len(c.blocks)-1]
}

func (c *SimpleChain) GetBlockHash(height uint64) [32]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if height >= uint64(len(c.blocks)) {
		return [32]byte{}
	}
	return c.blocks[height].Hash
}

// chainAdapter 适配器，将 SimpleChain 转换为 api.ChainReader
type chainAdapter struct {
	chain *SimpleChain
}

func (a *chainAdapter) GetHeight() uint64 {
	return a.chain.GetHeight()
}

func (a *chainAdapter) GetLatestBlock() api.Block {
	block := a.chain.GetLatestBlock()
	if block == nil {
		return nil
	}
	return &blockAdapter{block: block}
}

// blockAdapter 适配器，将 utxoPkg.Block 转换为 api.Block
type blockAdapter struct {
	block *utxoPkg.Block
}

func (a *blockAdapter) GetHeader() api.BlockHeader {
	return &headerAdapter{header: &a.block.Header}
}

func (a *blockAdapter) GetHash() [32]byte {
	return a.block.Hash
}

func (a *blockAdapter) GetTransactions() int {
	return len(a.block.Transactions)
}

// headerAdapter 适配器，将 utxoPkg.BlockHeader 转换为 api.BlockHeader
type headerAdapter struct {
	header *utxoPkg.BlockHeader
}

func (a *headerAdapter) GetHeight() uint64 {
	return a.header.Height
}

func (a *headerAdapter) GetTimestamp() uint64 {
	return a.header.Timestamp
}

func (a *headerAdapter) GetPrevBlockHash() [32]byte {
	return a.header.PrevBlockHash
}

func (a *headerAdapter) GetProposer() [32]byte {
	return a.header.Proposer
}

// NewNode 创建新节点
func NewNode(config *NodeConfig) *Node {
	return &Node{
		config:     config,
		logger:     log.New(os.Stdout, "[AIB] ", log.LstdFlags),
		shutdownCh: make(chan struct{}),
	}
}

// Start 启动节点
func (n *Node) Start() error {
	n.logger.Println("╔════════════════════════════════════════╗")
	n.logger.Println("║       AIB 2.0 Testnet Node          ║")
	n.logger.Println("╚════════════════════════════════════════╝")
	n.logger.Printf("DataDir: %s", n.config.DataDir)
	n.logger.Printf("API Port: %d", n.config.APIPort)
	n.logger.Printf("Validator Mode: %v", n.config.Validator)

	// 创建数据目录
	if err := os.MkdirAll(n.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// 1. 初始化UTXO存储
	n.logger.Println("[1/4] Initializing UTXO store...")
	n.utxoStore = utxoPkg.NewUTXOStore()
	n.logger.Println("    ✓ UTXO store initialized")

	// 2. 初始化区块链
	n.logger.Println("[2/4] Initializing blockchain...")
	n.chain = NewSimpleChain(n.utxoStore)
	n.logger.Println("    ✓ Blockchain initialized")

	// 3. 初始化共识引擎
	n.logger.Println("[3/4] Initializing consensus (60s block, 314 epoch)...")
	posConfig := utxoPkg.DefaultPoSConfig()
	posConfig.EpochLength = 314
	n.consensus = utxoPkg.NewConsensusState(posConfig)
	n.logger.Println("    ✓ Consensus engine initialized")

	// 4. 初始化交易内存池
	n.logger.Println("[4/4] Initializing transaction mempool...")
	n.mempool = utxoPkg.NewMempool(10000, 100)
	n.logger.Println("    ✓ Mempool initialized")

	// 5. 创建创世区块
	n.logger.Println("\n[Setup] Creating genesis block...")
	var proposer [32]byte
	copy(proposer[:], []byte("genesis"))
	genesis := utxoPkg.NewBlock(nil, [32]byte{}, 0, proposer)
	n.chain.AddBlock(genesis)
	n.logger.Printf("    ✓ Genesis block: height=%d, hash=%x", genesis.Header.Height, genesis.Hash[:8])

	// 6. 启动 API 服务器
	n.logger.Printf("\n[API] Starting API server on port %d...", n.config.APIPort)
	n.apiServer = api.NewServer(n.config.APIPort)

	// 设置区块链引用
	n.apiServer.SetChain(&chainAdapter{chain: n.chain})

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		if err := n.apiServer.Start(); err != nil {
			n.logger.Printf("API server error: %v", err)
		}
	}()

	// 7. 如果是验证者，启动出块循环
	if n.config.Validator {
		n.logger.Println("\n[Validator] Starting block production (60s interval)...")
		n.wg.Add(1)
		go n.runBlockProduction()
	}

	n.logger.Println("\n╔════════════════════════════════════════╗")
	n.logger.Println("║   AIB Node started successfully!      ║")
	n.logger.Println("╚════════════════════════════════════════╝")
	return nil
}

// runBlockProduction 运行出块循环
func (n *Node) runBlockProduction() {
	defer n.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// 立即产出第一个区块
	n.produceBlock()

	for {
		select {
		case <-ticker.C:
			n.produceBlock()
		case <-n.shutdownCh:
			n.logger.Println("Block production stopping...")
			return
		}
	}
}

// produceBlock 产出新区块
func (n *Node) produceBlock() {
	height := n.chain.GetHeight()

	// 获取当前验证者
	validators := n.consensus.GetActiveValidators()

	// 从内存池获取交易
	txs := n.mempool.GetTransactionsForBlock(1000000)

	// 创建新区块
	prevHash := n.chain.GetBlockHash(height)
	var proposer [32]byte
	if len(validators) > 0 {
		proposer = validators[0].Address
	} else {
		copy(proposer[:], []byte("default"))
	}

	newBlock := utxoPkg.NewBlock(txs, prevHash, height+1, proposer)

	// 添加到链
	if err := n.chain.AddBlock(newBlock); err != nil {
		n.logger.Printf("[Block %d] Failed to add block: %v", height+1, err)
		return
	}

	// 从内存池移除已确认的交易
	txHashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txHashes[i] = tx.Hash()
	}
	n.mempool.RemoveConfirmed(txHashes)

	n.logger.Printf("[Block %d] ✅ New block - txs=%d, hash=%x",
		height+1, len(txs), newBlock.Hash[:8])
}

// Stop 停止节点
func (n *Node) Stop() {
	n.logger.Println("\nShutting down AIB Node...")

	// 关闭API服务器
	if n.apiServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		n.apiServer.Stop(ctx)
		cancel()
	}

	// 发送关闭信号
	close(n.shutdownCh)

	// 等待所有goroutine退出
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		n.logger.Println("All components shut down gracefully")
	case <-time.After(10 * time.Second):
		n.logger.Println("Shutdown timeout, forcing exit")
	}

	n.logger.Println("AIB Node stopped")
}

// parseFlags 解析命令行参数
func parseFlags() *NodeConfig {
	config := &NodeConfig{}

	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".aib")

	flag.StringVar(&config.DataDir, "data-dir", defaultDataDir, "Data directory")
	flag.IntVar(&config.APIPort, "api-port", 8080, "API port")
	flag.IntVar(&config.P2PPort, "p2p-port", 30303, "P2P port")
	flag.BoolVar(&config.Validator, "validator", false, "Enable validator mode")
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level")
	flag.StringVar(&config.Bootstrap, "bootstrap", "", "Bootstrap node")
	flag.StringVar(&config.NodeID, "node-id", "", "Node ID")

	flag.Parse()
	return config
}

func main() {
	config := parseFlags()

	node := NewNode(config)

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动节点
	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	// 等待退出信号
	sig := <-sigCh
	fmt.Printf("\nReceived signal: %v\n", sig)

	node.Stop()
}
