package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/migration"
)

// BlockHeader 定义区块头接口
type BlockHeader interface {
	GetHeight() uint64
	GetTimestamp() uint64
	GetPrevBlockHash() [32]byte
	GetProposer() [32]byte
}

// Block 定义区块接口
type Block interface {
	GetHeader() BlockHeader
	GetHash() [32]byte
	GetTransactions() int
}

// ChainReader 接口用于读取区块链数据
type ChainReader interface {
	GetHeight() uint64
	GetLatestBlock() Block
	GetBestBlockHeight() (uint64, error)
	GetBlockByHash(hash [32]byte) (Block, error)
}

// MigrationHubAPI 定义迁移中心 API 接口
type MigrationHubAPI interface {
	GetMigrationStatus() *migration.MigrationStatus
	GetUserMigrationInfo(addr interfaces.Address) *migration.UserMigrationInfo
	ClaimAIB1(targetAddr interfaces.Address, amount uint64, pubKey []byte, signature []byte, nonce uint64) error
	ClaimUnlocked(userAddr interfaces.Address) (uint64, error)
	GetCrossChainRate(chain string) (uint64, error)
	GetAIB1Balance(addr interfaces.Address) (uint64, bool)
	IsAIB1Claimed(addr interfaces.Address) bool
}

// Server HTTP 服务器
type Server struct {
	httpServer      *http.Server
	mux             *http.ServeMux
	port            int
	mu              sync.RWMutex
	startTime       time.Time
	chain           ChainReader
	migrationHub    MigrationHubAPI
	utxoStore       utxoStoreInterface
	mempool         mempoolInterface
	consensusState  consensusConfigInterface
	governance      governanceInterface
	p2pNetwork      p2pNetworkInterface
	chainID         string
	apiKeys         []string // API keys for authentication
}

// p2pNetworkInterface P2P 网络接口
type p2pNetworkInterface interface {
	GetPeerList() []PeerEntry
}

// PeerEntry 节点信息条目（与 p2p.PeerListEntry 对齐）
type PeerEntry struct {
	ID        string
	Address   string
	LastSeen  time.Time
	Connected bool
}

// P2PNetworkAdapter 适配 p2p.Network 到 p2pNetworkInterface
type P2PNetworkAdapter struct {
	getPeerList func() []PeerEntry
}

// NewP2PNetworkAdapter 创建适配器
func NewP2PNetworkAdapter(getPeerList func() []PeerEntry) *P2PNetworkAdapter {
	return &P2PNetworkAdapter{getPeerList: getPeerList}
}

// GetPeerList 获取节点列表
func (a *P2PNetworkAdapter) GetPeerList() []PeerEntry {
	return a.getPeerList()
}

// SetChain 设置区块链引用
func (s *Server) SetChain(chain ChainReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chain = chain
}

// GetChain 获取区块链引用
func (s *Server) GetChain() ChainReader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chain
}

// SetMigrationHub 设置迁移中心引用
func (s *Server) SetMigrationHub(hub MigrationHubAPI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrationHub = hub
}

// GetMigrationHub 获取迁移中心引用
func (s *Server) GetMigrationHub() MigrationHubAPI {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.migrationHub
}

// SetUTXOStore 设置 UTXO 存储引用
func (s *Server) SetUTXOStore(store utxoStoreInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.utxoStore = store
}

// GetUTXOStore 获取 UTXO 存储引用
func (s *Server) GetUTXOStore() utxoStoreInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.utxoStore
}

// SetMempool 设置内存池引用
func (s *Server) SetMempool(mp mempoolInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mempool = mp
}

// GetMempool 获取内存池引用
func (s *Server) GetMempool() mempoolInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mempool
}

// SetConsensusState 设置共识状态引用
func (s *Server) SetConsensusState(cs consensusConfigInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consensusState = cs
}

// GetConsensusState 获取共识状态引用
func (s *Server) GetConsensusState() consensusConfigInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consensusState
}

// SetGovernance 设置治理模块引用
func (s *Server) SetGovernance(gov governanceInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.governance = gov
}

// GetGovernance 获取治理模块引用
func (s *Server) GetGovernance() governanceInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.governance
}

// SetP2PNetwork 设置 P2P 网络引用
func (s *Server) SetP2PNetwork(network p2pNetworkInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p2pNetwork = network
}

// GetP2PNetwork 获取 P2P 网络引用
func (s *Server) GetP2PNetwork() p2pNetworkInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p2pNetwork
}

// SetChainID 设置链 ID
func (s *Server) SetChainID(chainID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chainID = chainID
}

// GetChainID 获取链 ID
func (s *Server) GetChainID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chainID
}

// SetAPIKeys 设置 API 认证密钥
func (s *Server) SetAPIKeys(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKeys = keys
}

// GetAPIKeys 获取 API 认证密钥
func (s *Server) GetAPIKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKeys
}

// NewServer 创建新的 API 服务器
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	server := &Server{
		mux:       mux,
		port:      port,
		startTime: time.Now(),
	}

	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server
}

// RegisterRoutes 注册所有路由
func (s *Server) RegisterRoutes() {
	// authWrap 为需要认证的 handler 包装 AuthMiddleware
	authWrap := func(handler http.HandlerFunc) http.Handler {
		if len(s.apiKeys) > 0 {
			return AuthMiddleware(s.apiKeys)(http.HandlerFunc(handler))
		}
		// 未配置 API keys 时不启用认证（开发模式）
		return handler
	}

	// =========================================================================
	// 公开端点 - 无需认证
	// =========================================================================

	// 健康检查 - 基础
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/status", s.handleStatus)

	// P2P 节点列表
	s.mux.HandleFunc("/v1/peers", s.handlePeers)

	// 健康检查 - 增强版
	s.mux.HandleFunc("/health/detailed", s.handleHealthDetailed)

	// Kubernetes probes
	s.mux.HandleFunc("/healthz", s.handleLiveness)
	s.mux.HandleFunc("/readyz", s.handleReadiness)

	// 区块（只读查询）
	s.mux.HandleFunc("/v1/block/latest", s.handleGetLatestBlock)
	s.mux.HandleFunc("/v1/block/", s.handleGetBlock)

	// 交易查询（只读）
	s.mux.HandleFunc("/v1/transactions", s.handleTransactionsList)
	s.mux.HandleFunc("/v1/transaction/", s.handleTransactionDetail)

	// 余额查询（只读）
	s.mux.HandleFunc("/v1/balance/", s.handleGetBalance)

	// 区块链查询 - 只读端点
	s.mux.HandleFunc("/v1/utxo/", s.handleUTXOByAddress)
	s.mux.HandleFunc("/v1/validators", s.handleValidators)
	s.mux.HandleFunc("/v1/mempool", s.handleMempool)
	s.mux.HandleFunc("/v1/staking", s.handleStaking)
	s.mux.HandleFunc("/v1/proposals", s.handleProposals)

	// AI 模型/节点列表（只读）
	s.mux.HandleFunc("/v1/ai/models", s.handleListModels)
	s.mux.HandleFunc("/v1/ai/nodes", s.handleListNodes)

	// 迁移查询（只读）
	s.mux.HandleFunc("/api/migration/snapshot", s.handleMigrationSnapshot)
	s.mux.HandleFunc("/api/migration/rates", s.handleMigrationRates)
	s.mux.HandleFunc("/api/migration/status/", s.handleMigrationStatus)
	s.mux.HandleFunc("/api/migration/claimable/", s.handleMigrationClaimable)
	s.mux.HandleFunc("/api/migration/estimate", s.handleMigrationEstimate)

	// =========================================================================
	// 需要认证的端点 - 涉及资金操作
	// =========================================================================

	// 交易提交
	s.mux.Handle("/v1/transaction", authWrap(s.handleSubmitTransaction))

	// 钱包管理
	s.mux.Handle("/v1/wallet/create", authWrap(s.handleCreateWallet))
	s.mux.Handle("/v1/wallet/restore", authWrap(s.handleRestoreWallet))
	s.mux.Handle("/v1/wallet/import", authWrap(s.handleImportWallet))
	s.mux.Handle("/v1/wallet/export", authWrap(s.handleExportWallet))
	s.mux.Handle("/v1/wallet/balance", authWrap(s.handleWalletBalance))
	s.mux.Handle("/v1/wallet/send", authWrap(s.handleSendTransaction))

	// 质押操作（涉及资金）
	s.mux.Handle("/v1/stake", authWrap(s.handleStake))
	s.mux.Handle("/v1/unstake", authWrap(s.handleUnstake))

	// 质押查询（只读）
	s.mux.HandleFunc("/v1/wallet/stake", s.handleGetStake)

	// 通道操作
	s.mux.Handle("/v1/channel/open", authWrap(s.handleChannelOpen))
	s.mux.Handle("/v1/channel/close", authWrap(s.handleChannelClose))
	s.mux.Handle("/v1/channel/pay", authWrap(s.handleChannelPay))

	// 通道查询（只读）
	s.mux.HandleFunc("/v1/channel/", s.handleGetChannel)
	s.mux.HandleFunc("/v1/channels", s.handleListChannels)

	// AI 推理（涉及计费）
	s.mux.Handle("/v1/ai/inference", authWrap(s.handleInference))

	// 迁移操作（涉及资金）
	s.mux.Handle("/api/migration/claim-aib1", authWrap(s.handleClaimAIB1))
	s.mux.Handle("/api/migration/claim-unlocked", authWrap(s.handleClaimUnlocked))

	// 应用日志中间件
	s.httpServer.Handler = LoggingMiddleware(s.mux)
}

// Start 启动服务器
func (s *Server) Start() error {
	s.RegisterRoutes()
	log.Printf("Starting API server on port %d", s.port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("Stopping API server on port %d", s.port)
	return s.httpServer.Shutdown(ctx)
}

// Uptime 返回服务器运行时间
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// ============================================================================
// 辅助函数
// ============================================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
	})
}

func writeError(w http.ResponseWriter, status int, code, message, details string) {
	writeJSON(w, status, APIResponse{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func parsePathVar(r *http.Request, prefix string) string {
	path := r.URL.Path
	if len(path) > len(prefix) {
		return path[len(prefix):]
	}
	return ""
}
