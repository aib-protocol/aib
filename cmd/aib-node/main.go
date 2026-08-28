package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/aib-protocol/aib/pkg/api"
	"github.com/aib-protocol/aib/pkg/p2p"
	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// ======================================================================
// Network Configuration (Testnet / Mainnet)
// ======================================================================

// NetworkConfig holds chain-specific parameters for testnet or mainnet.
type NetworkConfig struct {
	ChainID        string
	GenesisTime    uint64
	GenesisMsg     string
	GenesisReward  uint64
	BootstrapNodes []string
	DefaultP2PPort int
	BlockVersion   uint32 // 1=V1, 2=V2 PoAIW
}

var TestnetConfig = NetworkConfig{
	ChainID: "aib-testnet-3",
	GenesisTime: 1755916800, // 2026-08-23T00:00:00Z
	GenesisMsg: "AIB Testnet v3 Genesis | Reuters 2026-08-18: Trump tariff pause | Consensus: SHA256d PoW era blocks 1..1000 @ 31.415 AIB/block, then pure-stake VRF PoS with deterministic proposer selection",
	GenesisReward: 0,
	BootstrapNodes: []string{"212.56.43.128:51413"},
	DefaultP2PPort: 51413,
	BlockVersion:   3,
}

var MainnetConfig = NetworkConfig{
	ChainID:        "aib-mainnet-1",
	GenesisTime:    1773532800, // 2026-03-14T00:00:00Z (Pi Day)
	GenesisMsg:     "AIB 2.0 Genesis - Pi Day 2026 - Decentralized AI Inference Network",
	GenesisReward:  uint64(5000000000), // 50 AIB in satoshi
	BootstrapNodes: []string{"www.aib.one:31415"},
	DefaultP2PPort: 31415,
	BlockVersion:   2,
}

// Legacy constants for backward compatibility
const (
	DefaultP2PPort   = 31415
	DefaultBootstrap = "www.aib.one:31415"
)

// NodeConfig nodeconfig
type NodeConfig struct {
	DataDir   string
	APIPort   int
	APIBind   string
	P2PPort   int
	Validator bool
	LogLevel  string
	Bootstrap string
	NodeID    string
	Nickname  string
	BlockTime int    // Block time (seconds)
	Network   string // "testnet" or "mainnet"
}

// Node is a production-grade AIB node implementation
type Node struct {
	config     *NodeConfig
	logger     *log.Logger
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// networkconfig
	networkCfg *NetworkConfig

	// Core components - persistent storage
	chainState *utxoPkg.ChainState
	utxoStore  *utxoPkg.PersistentUTXOStore
	consensus  *utxoPkg.ConsensusState
	mempool    *utxoPkg.Mempool
	apiServer  *api.Server

	// PoAIW components
	reputationMgr *utxoPkg.ReputationManager
	miningStats   *MiningStats
	epochFees     *epochFeeAccumulator

	// P2P network
	peerManager *p2p.ChainPeerManager
	blockSyncer *p2p.ChainBlockSyncer
	genesisHash string

	// Key pair
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	address    [32]byte

	// State tracking
	isRunning bool
	startTime time.Time
}

// ======================================================================
// Adapters (unchanged)
// ======================================================================

type chainAdapter struct {
	chainState *utxoPkg.ChainState
}

func (a *chainAdapter) GetHeight() uint64 {
	return a.chainState.GetBestBlockHeight()
}

func (a *chainAdapter) GetLatestBlock() api.Block {
	block, err := a.chainState.GetBestBlock()
	if err != nil {
		return nil
	}
	return &blockAdapter{block: block}
}

func (a *chainAdapter) GetBestBlockHeight() (uint64, error) {
	return a.chainState.GetBestBlockHeight(), nil
}

func (a *chainAdapter) GetBlockByHeight(height uint64) (*utxoPkg.Block, error) {
	return a.chainState.GetBlockByHeight(height)
}

func (a *chainAdapter) GetBlockByHash(hash [32]byte) (api.Block, error) {
	block, err := a.chainState.GetBlockByHash(hash)
	if err != nil {
		return nil, err
	}
	return &blockAdapter{block: block}, nil
}

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

// ======================================================================
// Shared Genesis Block
// ======================================================================

// createStandardGenesisBlock creates the canonical genesis block.
// Every node MUST produce the exact same genesis block hash.
func createStandardGenesisBlock(cfg *NetworkConfig) *utxoPkg.Block {
	// Fixed proposer: "genesis" padded to 32 bytes
	var proposer [32]byte
	copy(proposer[:], []byte("genesis"))

	// Create coinbase with fixed data
	coinbaseTx := utxoPkg.CreateCoinbaseTransaction(proposer, cfg.GenesisReward, []byte(cfg.GenesisMsg))

	// Create genesis block with fixed parameters
	genesis := utxoPkg.NewBlock([]*utxoPkg.Transaction{coinbaseTx}, [32]byte{}, 0, proposer)
	genesis.Header.Timestamp = cfg.GenesisTime
	genesis.Header.Version = cfg.BlockVersion
	genesis.Hash = genesis.CalculateHash()
	if cfg.BlockVersion >= 3 {
		genesis.Header.Bits = utxoPkg.PoWGenesisBits
		for nonce := uint64(0); ; nonce++ {
			genesis.Header.Nonce = nonce
			if utxoPkg.CheckProofOfWork(&genesis.Header) {
				break
			}
		}
		genesis.Hash = genesis.CalculateHash()
	}
	return genesis
}

// ======================================================================
// Node Creation & Startup
// ======================================================================

func NewNode(config *NodeConfig) *Node {
	netCfg := resolveNetworkConfig(config.Network)

	// Apply network defaults if not overridden by CLI flags
	if config.P2PPort == 0 {
		config.P2PPort = netCfg.DefaultP2PPort
	}
	if config.Bootstrap == "" {
		config.Bootstrap = netCfg.BootstrapNodes[0]
	}

	return &Node{
		config:     config,
		networkCfg: netCfg,
		logger:     log.New(os.Stdout, "[AIB] ", log.LstdFlags),
		shutdownCh: make(chan struct{}),
		startTime:  time.Now(),
	}
}

func (n *Node) Start() error {
	n.logger.Println("╔════════════════════════════════════════╗")
	n.logger.Println("║     AIB 2.0 Production Node           ║")
	n.logger.Println("╚════════════════════════════════════════╝")
	n.logger.Printf("Network: %s | ChainID: %s", n.config.Network, n.networkCfg.ChainID)
	n.logger.Printf("DataDir: %s", n.config.DataDir)
	n.logger.Printf("API Port: %d | P2P Port: %d", n.config.APIPort, n.config.P2PPort)
	n.logger.Printf("Validator Mode: %v | Block Time: %ds", n.config.Validator, n.config.BlockTime)

	// Create data directory
	if err := os.MkdirAll(n.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// 1. Initialize persistent UTXO store
	n.logger.Println("[1/7] Initializing persistent UTXO store...")
	utxoDBPath := filepath.Join(n.config.DataDir, "utxo.db")
	utxoStore, err := utxoPkg.NewPersistentUTXOStore(utxoDBPath)
	if err != nil {
		return fmt.Errorf("failed to create UTXO store: %w", err)
	}
	n.utxoStore = utxoStore
	n.logger.Printf("    ✓ UTXO store opened: %s", utxoDBPath)

	// 2. Initialize persistent chain state
	n.logger.Println("[2/7] Initializing persistent chain state...")
	chainDBPath := filepath.Join(n.config.DataDir, "chain.db")
	chainState, err := utxoPkg.NewChainState(chainDBPath)
	if err != nil {
		n.utxoStore.Close()
		return fmt.Errorf("failed to create chain state: %w", err)
	}
	chainState.SetUTXOStore(n.utxoStore)
	n.chainState = chainState
	n.logger.Printf("    ✓ Chain state opened: %s", chainDBPath)

	// 3. Initialize consensus engine
	n.logger.Println("[3/7] Initializing PoS consensus engine...")
	posConfig := utxoPkg.DefaultPoSConfig()
	posConfig.EpochLength = 314
	n.consensus = utxoPkg.NewConsensusState(posConfig)
	// Rebuild the PoS validator set on demand when proposer validation finds
	// no active validators (nodes syncing into the PoS era).
	n.consensus.SetEmptyValidatorSetHook(func() {
		n.buildValidatorSetFromPoWHistory()
	})
	chainState.SetConsensus(n.consensus)
	n.logger.Println("    ✓ Consensus engine initialized")

	// 3b. initialize ReputationManager (PoAIW)
	n.reputationMgr = utxoPkg.NewReputationManager()
	n.miningStats = NewMiningStats()
	n.epochFees = &epochFeeAccumulator{}
	n.logger.Println("    ✓ Reputation manager initialized (PoAIW)")

	// 4. Generate or load node keys
	n.logger.Println("[4/7] Loading node keys...")
	if err := n.initializeKeys(); err != nil {
		return fmt.Errorf("failed to initialize keys: %w", err)
	}
	nodeID := hex.EncodeToString(n.publicKey[:16])
	n.logger.Printf("    ✓ Node ID: %s", nodeID)
	n.logger.Printf("    ✓ Address: %x", n.address[:8])

	// Register node as validator
	// Restore consensus height from persisted chain (restart-safe)
	if h := n.chainState.GetBestBlockHeight(); h > 0 {
		n.consensus.RestoreHeight(h)
		n.logger.Printf("[Chain] Restored consensus height to %d from DB", h)
	}

		// Validator membership comes ONLY from on-chain PoW history
	// (buildValidatorSetFromPoWHistory). Self-registration would let any node
	// grant itself sortition weight with zero contribution — a consensus
	// vulnerability. -validator mode just enables block PRODUCTION for
	// validators already in the set.

	// 5. Initialize transaction mempool
	n.logger.Println("[5/7] Initializing transaction mempool...")
	n.mempool = utxoPkg.NewMempool(10000, 100)
	chainState.SetMempool(n.mempool)
	n.logger.Println("    ✓ Mempool initialized")

	// Transaction gossip: broadcast locally-submitted txs, accept gossiped ones.
	if n.peerManager != nil {
		n.peerManager.SetTxCallback(func(tx *utxoPkg.Transaction) {
			if err := n.mempool.AddTransaction(tx, n.utxoStore); err == nil {
				n.logger.Printf("[TX] gossiped tx %x accepted to mempool", tx.Hash())
			}
		})
	}

	// 6. Initialize chain state (shared genesis)
	n.logger.Println("[6/7] Loading chain state...")
	if err := n.initializeChain(); err != nil {
		return fmt.Errorf("failed to initialize chain: %w", err)
	}

	// 7. start P2P network
	n.logger.Println("[7/7] Starting P2P network...")
	if err := n.startP2P(nodeID); err != nil {
		n.logger.Printf("    ⚠ P2P network failed: %v (running in standalone mode)", err)
	}

	// Start API server
	n.logger.Printf("\n[API] Starting API server on port %d...", n.config.APIPort)
	// SECURITY: bind the API to loopback by default. The public aib.one
	// endpoints are served by a local reverse proxy on the same host; remote
	// curl/agents should talk to that proxy, never to the raw node API which
	// accepts private keys for signing. Override with -api-bind if you really
	// need external exposure (not recommended).
	apiBind := n.config.APIBind
	if apiBind == "" {
		apiBind = "127.0.0.1"
	}
	n.apiServer = api.NewServerBind(apiBind, n.config.APIPort)
	n.apiServer.SetChain(&chainAdapter{chainState: n.chainState})
	n.apiServer.SetChainID(n.networkCfg.ChainID)
	n.apiServer.SetMiningStats(n.miningStats.Snapshot)
	n.apiServer.SetPeersProvider(func() []api.PeerEntry {
		if n.peerManager == nil {
			return nil
		}
		entries := make([]api.PeerEntry, 0)
		for _, ci := range n.peerManager.GetChainPeers() {
			entries = append(entries, api.PeerEntry{
				ID:        ci.NodeID,
				Address:   ci.Address,
				LastSeen:  time.Unix(ci.LastSeen, 0),
				Connected: true,
			})
		}
		return entries
	})
n.apiServer.SetWalletInfo(func() map[string]interface{} {
		bal := uint64(0)
		utxoCount := 0
		wAddr := n.walletAddress()
		if n.utxoStore != nil {
			bal = n.utxoStore.GetBalance(wAddr)
			utxoCount = len(n.utxoStore.GetAllUTXOs(wAddr))
		}
		return map[string]interface{}{
			"address":      hex.EncodeToString(wAddr[:]),
			"balance_aib":  float64(bal) / 1e8,
			"balance_raw":  bal,
			"utxo_count":   utxoCount,
			"mining":       n.config.Validator,
			"height":       n.chainState.GetBestBlockHeight(),
		}
	})
	n.apiServer.SetUTXOStore(n.utxoStore)
	n.apiServer.SetMempool(n.mempool)
	// API-submitted transactions are gossiped to peers (MsgTx).
	n.apiServer.SetTxBroadcaster(n.peerManager.BroadcastTx)

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		if err := n.apiServer.Start(); err != nil {
			n.logger.Printf("API server error: %v", err)
		}
	}()

	// If validator, start block production loop
	if n.config.Validator {
		n.logger.Printf("\n[Validator] Starting block production (%ds interval)...", n.config.BlockTime)
		n.wg.Add(1)
		go n.runBlockProduction()
	}

	n.isRunning = true
	peerCount := 0
	if n.peerManager != nil {
		peerCount = n.peerManager.GetPeerCount()
	}
	n.logger.Println("\n╔════════════════════════════════════════╗")
	n.logger.Println("║   AIB Node started successfully!       ║")
	n.logger.Printf("║   Network: %-28s║", n.networkCfg.ChainID)
	n.logger.Printf("║   Genesis: %s...            ║", n.genesisHash[:16])
	n.logger.Printf("║   Block Version: V%d (PoAIW)            ║", n.networkCfg.BlockVersion)
	n.logger.Printf("║   Peers: %d | Height: %d                ║", peerCount, n.chainState.GetBestBlockHeight())
	n.logger.Println("╚════════════════════════════════════════╝")

	// Start seed server heartbeat registration
	n.wg.Add(1)
	go n.runSeedHeartbeat()

	return nil
}

// ======================================================================
// Seed Server Heartbeat
// ======================================================================

func (n *Node) runSeedHeartbeat() {
	defer n.wg.Done()

	seedURL := "https://www.aib.one/v1/heartbeat"
	nodeID := hex.EncodeToString(n.publicKey[:16])

	// Get this machine's external IP
	externalIP := n.getExternalIP()

	sendHeartbeat := func() {
		height, _ := n.chainState.GetBestBlockHeight(), uint64(0)
		blockHash := "" // latest block hash
		if block, err := n.chainState.GetBlockByHeight(height); err == nil {
			blockHash = hex.EncodeToString(block.Hash[:])
		}

		// Build signed message: nodeID + height + blockHash + genesisHash + timestamp
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d:%s:%s:%d", nodeID, height, blockHash, n.genesisHash, timestamp)

		// Sign with private key
		signature := ed25519.Sign(n.privateKey, []byte(message))

		payload := map[string]interface{}{
			"id":           nodeID,
			"address":      externalIP,
			"port":         n.config.P2PPort,
			"p2p_port":     n.config.P2PPort,
			"version":      "2.0.0-mvp",
			"block_height": height,
			"block_hash":   blockHash,
			"network":      n.config.Network,
			"genesis_hash": n.genesisHash,
			"timestamp":    timestamp,
			"signature":    hex.EncodeToString(signature),
			"public_key":   hex.EncodeToString(n.publicKey),
		}

		body, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(seedURL, "application/json", bytes.NewReader(body))
		if err != nil {
			n.logger.Printf("[Heartbeat] Failed to send: %v", err)
			return
		}
		resp.Body.Close()
		n.logger.Printf("[Heartbeat] Registered with seed (height=%d, hash=%s, ip=%s)", height, blockHash[:16], externalIP)
	}

	// Send immediately on first run
	sendHeartbeat()

	// Send every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sendHeartbeat()
		case <-n.shutdownCh:
			return
		}
	}
}

func (n *Node) getExternalIP() string {
	// Try to get local IP by dialing an external address
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}

// ======================================================================
// Key Management
// ======================================================================

func (n *Node) initializeKeys() error {
	keyPath := filepath.Join(n.config.DataDir, "node_key.pem")

	if _, err := os.Stat(keyPath); err == nil {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}
		if len(data) < ed25519.PrivateKeySize {
			return fmt.Errorf("invalid key file size")
		}
		n.privateKey = ed25519.PrivateKey(data[:ed25519.PrivateKeySize])
		n.publicKey = n.privateKey.Public().(ed25519.PublicKey)
		// Header.Proposer carries the PUBLIC KEY (signature verification
		// requires it); the wallet-side address is SHA256(publicKey).
		copy(n.address[:], n.publicKey)
		return nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	n.privateKey = priv
	n.publicKey = pub
	copy(n.address[:], pub)

	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}
	return nil
}

// ======================================================================
// Chain Initialization (Shared Genesis)
// ======================================================================

func (n *Node) initializeChain() error {
	// Compute the canonical genesis hash
	genesis := createStandardGenesisBlock(n.networkCfg)
	n.genesisHash = hex.EncodeToString(genesis.Hash[:])

	height := n.chainState.GetBestBlockHeight()

	if height > 0 {
		// Chain exists - verify genesis hash
		genesisBlock, err := n.chainState.GetBlockByHeight(0)
		if err != nil {
			return fmt.Errorf("failed to get genesis block: %w", err)
		}

		localGenesisHash := hex.EncodeToString(genesisBlock.Hash[:])
		if localGenesisHash != n.genesisHash {
			return fmt.Errorf(
				"GENESIS HASH MISMATCH! Local: %s, Expected: %s. "+
					"Delete %s and restart to resync.",
				localGenesisHash[:16], n.genesisHash[:16], n.config.DataDir)
		}

		bestBlock, err := n.chainState.GetBestBlock()
		if err != nil {
			return fmt.Errorf("failed to get best block: %w", err)
		}
		n.logger.Printf("    ✓ Chain loaded: height=%d, hash=%x",
			height, bestBlock.Hash[:8])
		n.logger.Printf("    ✓ Genesis verified: %s", n.genesisHash[:16])

		validators := n.consensus.GetActiveValidators()
		n.logger.Printf("    ✓ Active validators: %d", len(validators))
		utxoCount, addrCount, _ := n.utxoStore.GetStats()
		n.logger.Printf("    ✓ UTXO stats: %d UTXOs, %d addresses", utxoCount, addrCount)
	} else {
		// Create genesis block
		n.logger.Println("    Creating standard genesis block...")
		if err := n.chainState.AddBlock(genesis); err != nil {
			return fmt.Errorf("failed to add genesis block: %w", err)
		}
		if err := n.utxoStore.SetChainHead(0); err != nil {
			return fmt.Errorf("failed to set chain head: %w", err)
		}
		n.logger.Printf("    ✓ Genesis block: hash=%s", n.genesisHash[:16])
		n.logger.Printf("    ✓ ChainID: %s | GenesisTime: %d", n.networkCfg.ChainID, n.networkCfg.GenesisTime)
	}

	return nil
}

// ======================================================================
// P2P Network
// ======================================================================

func (n *Node) startP2P(nodeID string) error {
	// Bootstrap is already resolved in NewNode from network config
	bootstrapNodes := []string{n.config.Bootstrap}

	nickname := n.config.Nickname
	if nickname == "" {
		nickname = fmt.Sprintf("node-%s", nodeID[:8])
	}

	pm := p2p.NewChainPeerManager(p2p.ChainPeerConfig{
		NodeID:      nodeID,
		Nickname:    nickname,
		ListenPort:  n.config.P2PPort,
		GenesisHash: n.genesisHash,
		ChainID:     n.networkCfg.ChainID,
		Bootstrap:   bootstrapNodes,
		MaxPeers:    25,
		Logger:      n.logger,
	})

	// Set block handlers
	pm.SetHandlers(
		n.handleNewBlock,
		n.handleGetBlocks,
		func() uint64 { return n.chainState.GetBestBlockHeight() },
	)

	if err := pm.StartChainP2P(); err != nil {
		return fmt.Errorf("start P2P: %w", err)
	}
	n.peerManager = pm
	pm.StartAutoSync(15 * time.Second)

	// Start block syncer
	syncer := p2p.NewChainBlockSyncer(pm, n.logger)
	syncer.SetHandlers(
		func() uint64 { return n.chainState.GetBestBlockHeight() },
		n.handleNewBlock,
	)
	syncer.StartSync()
	n.blockSyncer = syncer

	n.logger.Printf("    ✓ P2P listening on port %d", n.config.P2PPort)
	n.logger.Printf("    ✓ Bootstrap: %v", bootstrapNodes)
	n.logger.Printf("    ✓ Genesis hash: %s", n.genesisHash[:16])

	return nil
}

// handleNewBlock processes a block received from a peer.
func (n *Node) handleNewBlock(data p2p.BlockData) error {
	// Learn the proposer pubkey carried by this block for future relays
	n.learnProposerPubKey(data.Height, data.Proposer)
	// (v3) No validator learning from received blocks — PoW era has no
	// validator set; PoS set is built deterministically from PoW history.
	// Deserialize block
	block, err := utxoPkg.DeserializeBlock(data.RawBlock)
	if err != nil {
		return fmt.Errorf("deserialize block %d: %w", data.Height, err)
	}

	// Verify hash matches
	blockHash := hex.EncodeToString(block.Hash[:])
	if blockHash != data.Hash {
		return fmt.Errorf("block %d hash mismatch: got %s, expected %s",
			data.Height, blockHash[:16], data.Hash[:16])
	}

	// Check if we already have this block
	localHeight := n.chainState.GetBestBlockHeight()
	if data.Height <= localHeight {
		return nil // already have it
	}

	// Verify block connects to our chain
	if data.Height != localHeight+1 {
		return fmt.Errorf("block %d does not connect (local height %d)", data.Height, localHeight)
	}

	bestHash := n.chainState.GetBestBlockHash()
	if block.Header.PrevBlockHash != bestHash {
		return fmt.Errorf("block %d prev hash mismatch", data.Height)
	}

	// Add block to chain (validation happens inside AddBlock)
	if err := n.chainState.AddBlock(block); err != nil {
		return fmt.Errorf("add block %d: %w", data.Height, err)
	}
	n.utxoStore.SetChainHead(data.Height)

	// For V2 blocks, update reputation manager from block data
	if block.Header.Version >= 2 && n.reputationMgr != nil {
		n.applyBlockReputationUpdates(block)
	}

	// Update P2P manager's best height
	if n.peerManager != nil {
		n.peerManager.UpdateBestHeight(data.Height)
	}

	versionTag := "V1"
	if block.Header.Version >= 2 {
		versionTag = "V2"
	}
	n.logger.Printf("[P2P] Received %s block %d, hash=%s", versionTag, data.Height, data.Hash[:16])
	return nil
}

// applyBlockReputationUpdates processes reputation data embedded in V2 blocks.
func (n *Node) applyBlockReputationUpdates(block *utxoPkg.Block) {
	// V2 blocks carry reputation updates in the block's extra data.
	// For now, we update the proposer's reputation score positively
	// for having successfully produced a valid block.
	var content utxoPkg.ScoreContent
	content.TargetPubKey = block.Header.Proposer
	content.Score = 7.0 // good block production
	content.Reason = "block_produced"
	content.Timestamp = block.Header.Timestamp

	// Self-report: system awards reputation for valid block production
	score := &utxoPkg.ReputationScore{
		Content: content,
		Signer:  block.Header.Proposer, // signed by proposer
	}
	// Ignore error - reputation update is best-effort
	_ = n.reputationMgr.SubmitScore(score)
}

// handleGetBlocks returns blocks for a peer sync request.
func (n *Node) handleGetBlocks(from, to uint64) ([]p2p.BlockData, error) {
	localHeight := n.chainState.GetBestBlockHeight()
	if to > localHeight {
		to = localHeight
	}
	if from > to {
		return nil, nil
	}

	// Limit batch size
	maxBatch := uint64(500)
	if to-from+1 > maxBatch {
		to = from + maxBatch - 1
	}

	blocks := make([]p2p.BlockData, 0, to-from+1)
	for h := from; h <= to; h++ {
		block, err := n.chainState.GetBlockByHeight(h)
		if err != nil {
			return blocks, fmt.Errorf("get block %d: %w", h, err)
		}

		rawBlock := block.SerializeBlock()
		blocks = append(blocks, p2p.BlockData{
			Height:        block.Header.Height,
			Hash:          hex.EncodeToString(block.Hash[:]),
			PrevBlockHash: hex.EncodeToString(block.Header.PrevBlockHash[:]),
			MerkleRoot:    hex.EncodeToString(block.Header.MerkleRoot[:]),
			Timestamp:     block.Header.Timestamp,
			// Full ed25519 pubkey + signature so receivers can verify.
			// Header.Proposer is the address (pubkey-derived), but signature
			// verification needs the raw pubkey; for self-produced blocks we
			// know it. For relayed blocks the pubkey must come with the block
			// — see proposerPubKeyForHeight cache.
			Proposer:      hex.EncodeToString(block.Header.Proposer[:]),
			Signature:     hex.EncodeToString(block.Header.Signature),
			SignedHash:    func() string { sh := computeSignedHash(block); return hex.EncodeToString(sh[:]) }(),
			TxCount:       len(block.Transactions),
			RawBlock:      rawBlock,
		})
	}

	return blocks, nil
}

// ======================================================================
// Block Production
// ======================================================================

func (n *Node) runBlockProduction() {
	defer n.wg.Done()

	blockTime := time.Duration(n.config.BlockTime) * time.Second
	ticker := time.NewTicker(blockTime)
	defer ticker.Stop()

	// Wait for sync to catch up before producing
	if n.blockSyncer != nil {
		n.logger.Println("[Validator] Waiting for sync before producing blocks...")
		waitStart := time.Now()
		for {
			select {
			case <-n.shutdownCh:
				return
			case <-time.After(5 * time.Second):
			}

			state := n.blockSyncer.GetState()
			if state == p2p.SyncStateCaughtUp || time.Since(waitStart) > 60*time.Second {
				break
			}
		}
		n.logger.Println("[Validator] Sync complete, starting block production")
	}

	_ = ticker
	n.produceBlock()

	for {
		select {
		case <-ticker.C:
			n.produceBlock()
		case <-n.shutdownCh:
			n.logger.Println("Block production stopping...")
			return
		case <-time.After(200 * time.Millisecond):
			// PoW era: mine continuously; produceBlock returns fast when
			// someone else wins or we're not the expected producer.
			h := n.chainState.GetBestBlockHeight()
			if n.networkCfg.BlockVersion >= 3 && h < utxoPkg.PoWEraBlocks {
				n.produceBlock()
			}
		}
	}
}

func (n *Node) produceBlock() {
	height := n.chainState.GetBestBlockHeight()

	txs := n.mempool.GetTransactionsForBlock(1000000)
	if txs == nil {
		txs = []*utxoPkg.Transaction{}
	}

	prevHash := n.chainState.GetBestBlockHash()
	proposer := n.address // public key (block header/signature requirement)
	// Coinbase pays to the WALLET address (SHA256 of the public key) — this
	// is what /v1/wallet/send and /v1/balance operate on.
	walletAddr := n.walletAddress()
	height1 := height + 1

	// ---- Consensus V3: PoW era (blocks 1..10000) ----
	if n.networkCfg.BlockVersion >= 3 && height1 <= utxoPkg.PoWEraBlocks {
		coinbaseTx := utxoPkg.CreateCoinbaseTransaction(walletAddr, utxoPkg.PoWBlockReward, []byte(fmt.Sprintf("pow-v3-h%d", height1)))
		blockTxs := append([]*utxoPkg.Transaction{coinbaseTx}, txs...)
		newBlock := utxoPkg.NewBlock(blockTxs, prevHash, height1, proposer)
		newBlock.Header.Version = 3
		newBlock.Header.Bits = n.nextPoWBits(height)
		newBlock.Header.Timestamp = uint64(time.Now().Unix())
		// ProposerKey participates in the PoW hash — set it BEFORE mining
		// (SignBlock would fill it later and invalidate the nonce).
		copy(newBlock.Header.ProposerKey[:], n.publicKey)

		for nonce := uint64(0); ; nonce++ {
			select {
			case <-n.shutdownCh:
				return
			default:
			}
			newBlock.Header.Nonce = nonce
			if utxoPkg.CheckProofOfWork(&newBlock.Header) {
				break
			}
			if nonce%100000 == 0 {
				// check for a newer tip (another miner won the race)
				if n.chainState.GetBestBlockHeight() >= height1 {
					return
				}
			}
		}
		newBlock.Hash = newBlock.CalculateHash()
		if err := newBlock.SignBlock(n.privateKey); err != nil {
			n.logger.Printf("[PoW %d] sign failed: %v", height1, err)
			return
		}
		n.acceptAndBroadcast(newBlock, txs)
		return
	}

	// ---- PoS era (height > 10000): V2 pure-stake VRF path, validator set from PoW history ----
	if n.networkCfg.BlockVersion >= 3 && height1 == utxoPkg.PoWEraBlocks+1 {
		n.buildValidatorSetFromPoWHistory()
	}

	var newBlock *utxoPkg.Block

	if n.networkCfg.BlockVersion >= 2 {
		// V2+ (fee-burn trial): PURE-STAKE VRF sortition (RFC-002 Route C, 2026-08-22 decision)
		// No reputation weighting — α=1.0, β=0.
		seed := prevHash[:]
		proof, err := n.consensus.SelectProposerVRFDeterministic(seed)
		if err != nil {
			// Fallback: single-node genesis phase — self proposes
			n.logger.Printf("[Block %d] VRF selection fallback: %v", height+1, err)
			proof = nil
		}
		// Winner is a WALLET address (validator set); proposer var holds the
		// public key. Compare against our wallet address.
		if proof != nil && proof.Winner != walletAddr {
			// Not our slot — someone else won the sortition
			n.miningStats.recordMiss(height+1, proof.Winner)
			return
		}
		if proof != nil {
			n.miningStats.recordWin(height+1, proof.Stakes)
		}

		// Epoch fee-burn settlement: at each epoch boundary, split accumulated
		// fees — stakers get up to φ·S/year, the excess is burned (pkg/economy).
		n.settleEpochFees(height + 1)

		// Coinbase: bootstrap-window low reward (RFC-003): 1 AIB/block during
		// the first 10,000 blocks, then fee-share only (zero inflation path).
		// After the PoW era (PoWEraBlocks) the fixed supply of 31415 AIB is
		// fully mined; PoS blocks carry no coinbase (fee-only).
		coinbaseAmount := uint64(1 * 1e8)
		if height+1 > utxoPkg.PoWEraBlocks {
			coinbaseAmount = 0
		}
		var coinbaseTx *utxoPkg.Transaction
		if coinbaseAmount > 0 {
			coinbaseTx = utxoPkg.CreateCoinbaseTransaction(walletAddr, coinbaseAmount, []byte(fmt.Sprintf("vrf-coinbase-h%d", height+1)))
		}
		var blockTxs []*utxoPkg.Transaction
		if coinbaseTx != nil {
			blockTxs = append([]*utxoPkg.Transaction{coinbaseTx}, txs...)
		} else {
			blockTxs = txs
		}

		// PoS block: Header.Proposer must be the WALLET address (the staked
		// identity) — matches validator set comparison.
		newBlock = utxoPkg.NewBlock(blockTxs, prevHash, height+1, walletAddr)
		newBlock.Header.Version = 3
	} else {
		// V1: Legacy block production
		coinbaseTx := utxoPkg.CreateCoinbaseTransaction(proposer, 50*1e8, []byte("block reward"))
		blockTxs := append([]*utxoPkg.Transaction{coinbaseTx}, txs...)
		newBlock = utxoPkg.NewBlock(blockTxs, prevHash, height+1, proposer)
	}

	if err := newBlock.SignBlock(n.privateKey); err != nil {
		n.logger.Printf("[Block %d] Failed to sign: %v", height+1, err)
		return
	}

	n.acceptAndBroadcast(newBlock, txs)
	return
}

// acceptAndBroadcast adds a locally-produced block to the chain and gossips it.
func (n *Node) acceptAndBroadcast(newBlock *utxoPkg.Block, txs []*utxoPkg.Transaction) {
	height := newBlock.Header.Height
	if err := n.chainState.AddBlock(newBlock); err != nil {
		n.logger.Printf("[Block %d] Failed to add: %v", height, err)
		return
	}

	txHashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txHashes[i] = tx.Hash()
	}
	n.mempool.RemoveConfirmed(txHashes)

	n.utxoStore.SetChainHead(height)

	versionTag := "V1"
	if newBlock.Header.Version >= 2 {
		versionTag = "V2-PoAIW"
	}
	n.logger.Printf("[Block %d] ✅ %s txs=%d hash=%x proposer=%x",
		height, versionTag, len(newBlock.Transactions), newBlock.Hash[:8], newBlock.Header.Proposer[:8])

	// Broadcast to P2P network
	if n.peerManager != nil {
		n.peerManager.UpdateBestHeight(height)
		rawBlock := newBlock.SerializeBlock()
		n.peerManager.BroadcastNewBlock(p2p.BlockData{
			Height:        newBlock.Header.Height,
			Hash:          hex.EncodeToString(newBlock.Hash[:]),
			PrevBlockHash: hex.EncodeToString(newBlock.Header.PrevBlockHash[:]),
			MerkleRoot:    hex.EncodeToString(newBlock.Header.MerkleRoot[:]),
			Timestamp:     newBlock.Header.Timestamp,
			Proposer:      hex.EncodeToString(n.publicKey), // full ed25519 pubkey for verification
			Signature:     hex.EncodeToString(newBlock.Header.Signature),
			SignedHash:    hex.EncodeToString(newBlock.SignedHash[:]),
			TxCount:       len(newBlock.Transactions),
			RawBlock:      rawBlock,
		})
	}
}

// ======================================================================
// Shutdown
// ======================================================================

func (n *Node) Stop() {
	if !n.isRunning {
		return
	}

	n.logger.Println("\n═══════════════════════════════════════════")
	n.logger.Println("Shutting down AIB Node...")
	n.isRunning = false

	// 1. Signal shutdown
	n.logger.Println("[1/5] Signaling all components to stop...")
	close(n.shutdownCh)

	// 2. Stop P2P
	n.logger.Println("[2/5] Stopping P2P network...")
	if n.blockSyncer != nil {
		n.blockSyncer.StopSync()
		n.logger.Println("    ✓ Block syncer stopped")
	}
	if n.peerManager != nil {
		n.peerManager.StopChainP2P()
		n.logger.Println("    ✓ Peer manager stopped")
	}

	// 3. Stop API server
	n.logger.Println("[3/5] Stopping API server...")
	if n.apiServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := n.apiServer.Stop(ctx); err != nil {
			n.logger.Printf("    API server shutdown error: %v", err)
		} else {
			n.logger.Println("    ✓ API server stopped")
		}
		cancel()
	}

	// 4. Wait for goroutines
	n.logger.Println("[4/5] Waiting for goroutines...")
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		n.logger.Println("    ✓ All goroutines stopped")
	case <-time.After(10 * time.Second):
		n.logger.Println("    ⚠ Wait timeout")
	}

	// 5. Close storage
	n.logger.Println("[5/5] Closing persistent storage...")
	if n.chainState != nil {
		if err := n.chainState.Close(); err != nil {
			n.logger.Printf("    Chain state close error: %v", err)
		} else {
			n.logger.Println("    ✓ Chain state closed")
		}
	}
	if n.utxoStore != nil {
		if err := n.utxoStore.Close(); err != nil {
			n.logger.Printf("    UTXO store close error: %v", err)
		} else {
			n.logger.Println("    ✓ UTXO store closed")
		}
	}

	uptime := time.Since(n.startTime)
	n.logger.Printf("\nNode uptime: %v", uptime)
	n.logger.Println("AIB Node stopped gracefully")
}

// ======================================================================
// CLI
// ======================================================================

func parseFlags() *NodeConfig {
	config := &NodeConfig{}

	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".aib")

	flag.StringVar(&config.DataDir, "data-dir", defaultDataDir, "Data directory")
	flag.IntVar(&config.APIPort, "api-port", 8080, "API port")
	flag.StringVar(&config.APIBind, "api-bind", "127.0.0.1", "API bind address (default loopback — signing endpoints accept private keys)")
	flag.IntVar(&config.P2PPort, "p2p-port", 0, "P2P port (default: per network)")
	flag.IntVar(&config.BlockTime, "block-time", 60, "Block time in seconds (mainnet: 60s, testnet: 30s)")
	flag.BoolVar(&config.Validator, "validator", false, "Enable validator mode")
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level")
	flag.StringVar(&config.Bootstrap, "bootstrap", "", "Bootstrap node address (default: per network)")
	flag.StringVar(&config.NodeID, "node-id", "", "Node ID (auto-generated if empty)")
	flag.StringVar(&config.Nickname, "nickname", "", "Node nickname")
	flag.StringVar(&config.Network, "network", "testnet", "Network to join: testnet or mainnet")

	flag.Parse()

	return config
}

// resolveNetworkConfig returns the NetworkConfig for the given network name.
func resolveNetworkConfig(network string) *NetworkConfig {
	switch network {
	case "mainnet":
		cfg := MainnetConfig
		return &cfg
	default:
		cfg := TestnetConfig
		return &cfg
	}
}

func main() {

	// `aib-node setup` — interactive first-run wizard (see setup.go)
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		home, _ := os.UserHomeDir()
		sd := flag.String("data-dir", filepath.Join(home, ".aib"), "data dir")
		sa := flag.Int("setup-api-port", 8080, "api port")
		sp := flag.Int("setup-p2p-port", 0, "p2p port")
		// re-parse just our flags
		fs := flag.NewFlagSet("setup", flag.ExitOnError)
		fs.StringVar(sd, "data-dir", filepath.Join(home, ".aib"), "data dir")
		fs.IntVar(sa, "api-port", 8080, "api port")
		fs.IntVar(sp, "p2p-port", 0, "p2p port")
		_ = fs.Parse(os.Args[2:])
		if *sp == 0 {
			*sp = 51413
		}
		if err := runSetup(*sd, *sa, *sp, nil); err != nil {
			fmt.Fprintf(os.Stderr, "[setup] %v\n", err)
			os.Exit(1)
		}
		return
	}

	config := parseFlags()

	node := NewNode(config)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	sig := <-sigCh
	fmt.Printf("\nReceived signal: %v\n", sig)

	node.Stop()
}

// proposerPubKeyCache: height -> proposer pubkey hex (learned from inbound blocks).
var proposerPubKeyCache sync.Map

// proposerPubKeyForHeight returns the raw pubkey for a historical block.
// For blocks we produced, it's our own key. For relayed blocks, the pubkey
// learned when we first received it (same network, same genesis assumption).
func (n *Node) proposerPubKeyForHeight(height uint64, proposerAddr [32]byte) string {
	if v, ok := proposerPubKeyCache.Load(height); ok {
		return v.(string)
	}
	// Heuristic for single-validator testnet: assume our own key produced it
	// when the proposer address matches ours.
	if proposerAddr == n.address {
		return hex.EncodeToString(n.publicKey)
	}
	return hex.EncodeToString(proposerAddr[:]) // fallback: address as pubkey (32B)
}

// learnProposerPubKey records the pubkey carried by an inbound block.
func (n *Node) learnProposerPubKey(height uint64, pubkeyHex string) {
	proposerPubKeyCache.Store(height, pubkeyHex)
}

// computeSignedHash reconstructs the hash that was signed: the header hash
// with the signature field stripped (same semantics as VerifyBlockSignature).
func computeSignedHash(b *utxoPkg.Block) [32]byte {
	saved := b.Header.Signature
	b.Header.Signature = nil
	h := b.CalculateHash()
	b.Header.Signature = saved
	return h
}

// nextPoWBits computes Bits for the next PoW block at given prev height.
func (n *Node) nextPoWBits(prevHeight uint64) uint32 {
	if prevHeight == 0 {
		return utxoPkg.PoWGenesisBits
	}
	prevBlock, _ := n.chainState.GetBlockByHeight(prevHeight)
	if prevBlock == nil {
		return utxoPkg.PoWGenesisBits
	}
	w := prevHeight - (prevHeight % utxoPkg.PoWRetargetWindow)
	if w == 0 {
		return prevBlock.Header.Bits
	}
	start, _ := n.chainState.GetBlockByHeight(w)
	if start == nil {
		return prevBlock.Header.Bits
	}
	return utxoPkg.NextWorkRequired(prevBlock.Header.Bits, prevHeight,
		start.Header.Timestamp, prevBlock.Header.Timestamp)
}

// buildValidatorSetFromPoWHistory builds the PoS validator set from the
// PoW era: stake weight = number of PoW blocks mined (hashrate share proxy).

// walletAddress returns the wallet-side address for this node's key
// (SHA256 of the public key — matches the wallet SDK derivation).
func (n *Node) walletAddress() [32]byte {
	h := sha256.Sum256(n.publicKey)
	return h
}

func (n *Node) buildValidatorSetFromPoWHistory() {
	// TRUE-STAKE model: validator weight comes ONLY from live stake UTXOs
	// on chain. A miner who wants PoS weight must create a STAKE tx.
	// The PoW-era mining history is irrelevant — coins, not hashrate.
	n.rebuildValidatorSetFromStakes()
}

// rebuildValidatorSetFromStakes rebuilds the consensus validator set from
// all live stake UTXOs (deterministic: derived from chain state alone).
func (n *Node) rebuildValidatorSetFromStakes() {
	if n.utxoStore == nil {
		return
	}
	all := n.utxoStore.GetAllUTXOsAll()
	entries := utxoPkg.BuildStakeIndex(all)
	addrs, weights := utxoPkg.StakeWeights(entries)
	n.consensus.ResetValidators()
	for i, a := range addrs {
		if weights[i] == 0 {
			continue
		}
		_ = n.consensus.AddValidatorFromStake(a, weights[i])
	}
	n.logger.Printf("[PoS] validator set rebuilt from live stakes: %d validators", len(addrs))
}
