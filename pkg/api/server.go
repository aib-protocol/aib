package api

import (
	"github.com/aib-protocol/aib/pkg/utxo"
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

// BlockHeader defines the block header interface
type BlockHeader interface {
	GetHeight() uint64
	GetTimestamp() uint64
	GetPrevBlockHash() [32]byte
	GetProposer() [32]byte
}

// Block defines the block interface
type Block interface {
	GetHeader() BlockHeader
	GetHash() [32]byte
	GetTransactions() int
}

// ChainReader is an interface for reading blockchain data
type ChainReader interface {
	GetHeight() uint64
	GetLatestBlock() Block
	GetBestBlockHeight() (uint64, error)
	GetBlockByHash(hash [32]byte) (Block, error)
	GetBlockByHeight(height uint64) (*utxo.Block, error)
}

// MigrationHubAPI defines the migration hub API interface
type MigrationHubAPI interface {
	GetMigrationStatus() *migration.MigrationStatus
	GetUserMigrationInfo(addr interfaces.Address) *migration.UserMigrationInfo
	ClaimAIB1(targetAddr interfaces.Address, amount uint64, pubKey []byte, signature []byte, nonce uint64) error
	ClaimUnlocked(userAddr interfaces.Address) (uint64, error)
	GetCrossChainRate(chain string) (uint64, error)
	GetAIB1Balance(addr interfaces.Address) (uint64, bool)
	IsAIB1Claimed(addr interfaces.Address) bool
}

// Server is the HTTP server
type Server struct {
	httpServer     *http.Server
	mux            *http.ServeMux
	port           int
	mu             sync.RWMutex
	startTime      time.Time
	miningStats    func() map[string]interface{}
	walletInfoFn   func() map[string]interface{}
	peersFn        func() []PeerEntry
	chain          ChainReader
	migrationHub   MigrationHubAPI
	utxoStore      utxoStoreInterface
	mempool        mempoolInterface
	consensusState consensusConfigInterface
	governance     governanceInterface
	p2pNetwork     p2pNetworkInterface
	chainID        string
	apiKeys        []string // API keys for authentication
}

// p2pNetworkInterface P2P networkinterface
type p2pNetworkInterface interface {
	GetPeerList() []PeerEntry
}

// PeerEntry is a peer info entry (aligned with p2p.PeerListEntry)
type PeerEntry struct {
	ID        string
	Address   string
	LastSeen  time.Time
	Connected bool
}

// P2PNetworkAdapter adapts p2p.Network to p2pNetworkInterface
type P2PNetworkAdapter struct {
	getPeerList func() []PeerEntry
}

// NewP2PNetworkAdapter creates the adapter
func NewP2PNetworkAdapter(getPeerList func() []PeerEntry) *P2PNetworkAdapter {
	return &P2PNetworkAdapter{getPeerList: getPeerList}
}

// GetPeerList getnodelist
func (a *P2PNetworkAdapter) GetPeerList() []PeerEntry {
	return a.getPeerList()
}

// SetChain sets the blockchain reference
func (s *Server) SetChain(chain ChainReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chain = chain
}

// GetChain returns the blockchain reference
func (s *Server) GetChain() ChainReader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chain
}

// SetMigrationHub sets the migration hub reference
func (s *Server) SetMigrationHub(hub MigrationHubAPI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrationHub = hub
}

// GetMigrationHub returns the migration hub reference
func (s *Server) GetMigrationHub() MigrationHubAPI {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.migrationHub
}

// SetUTXOStore sets the UTXO store reference
func (s *Server) SetUTXOStore(store utxoStoreInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.utxoStore = store
}

// GetUTXOStore returns the UTXO store reference
func (s *Server) GetUTXOStore() utxoStoreInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.utxoStore
}

// SetMempool sets the mempool reference
func (s *Server) SetMempool(mp mempoolInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mempool = mp
}

// GetMempool returns the mempool reference
func (s *Server) GetMempool() mempoolInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mempool
}

// SetConsensusState sets the consensus state reference
func (s *Server) SetConsensusState(cs consensusConfigInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consensusState = cs
}

// GetConsensusState returns the consensus state reference
func (s *Server) GetConsensusState() consensusConfigInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consensusState
}

// SetGovernance sets the governance module reference
func (s *Server) SetGovernance(gov governanceInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.governance = gov
}

// GetGovernance returns the governance module reference
func (s *Server) GetGovernance() governanceInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.governance
}

// SetP2PNetwork sets the P2P network reference
func (s *Server) SetP2PNetwork(network p2pNetworkInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p2pNetwork = network
}

// GetP2PNetwork returns the P2P network reference
func (s *Server) GetP2PNetwork() p2pNetworkInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p2pNetwork
}

// SetChainID sets the chain ID
func (s *Server) SetChainID(chainID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chainID = chainID
}

// GetChainID returns the chain ID
func (s *Server) GetChainID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chainID
}

// SetAPIKeys sets the API authentication keys
func (s *Server) SetAPIKeys(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKeys = keys
}

// GetAPIKeys returns the API authentication keys
func (s *Server) GetAPIKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKeys
}

// NewServer creates a new API server
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	server := &Server{
		mux:       mux,
		port:      port,
		startTime: time.Now(),
	}

	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", port),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server
}

// RegisterRoutes registers all routes
func (s *Server) RegisterRoutes() {
	// authWrap wraps AuthMiddleware around handlers that require authentication
	authWrap := func(handler http.HandlerFunc) http.Handler {
		if len(s.apiKeys) > 0 {
			return AuthMiddleware(s.apiKeys)(http.HandlerFunc(handler))
		}
		// Authentication is disabled when no API keys are configured (development mode)
		return handler
	}

	// =========================================================================
	// Public endpoints - no authentication required
	// =========================================================================

	// Health check - basic
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/status", s.handleStatus)

	// Mining/miner observability (fee-burn testnet)
	s.mux.HandleFunc("/v1/mining", s.handleMining)
	s.mux.HandleFunc("/v1/wallet/info", s.handleWalletInfo)

	// P2P nodelist
	s.mux.HandleFunc("/v1/peers", s.handlePeers)

	// Health check - enhanced
	s.mux.HandleFunc("/health/detailed", s.handleHealthDetailed)

	// Kubernetes probes
	s.mux.HandleFunc("/healthz", s.handleLiveness)
	s.mux.HandleFunc("/readyz", s.handleReadiness)

	// Blocks (read-only queries)
	s.mux.HandleFunc("/v1/block/latest", s.handleGetLatestBlock)
	s.mux.HandleFunc("/v1/block/", s.handleGetBlock)

	// Transaction queries (read-only)
	s.mux.HandleFunc("/v1/transactions", s.handleTransactionsList)
	s.mux.HandleFunc("/v1/transaction/", s.handleTransactionDetail)

	// Balance queries (read-only)
	s.mux.HandleFunc("/v1/balance/", s.handleGetBalance)

	// Blockchain queries - read-only endpoints
	s.mux.HandleFunc("/v1/utxo/", s.handleUTXOByAddress)
	s.mux.HandleFunc("/v1/validators", s.handleValidators)
	s.mux.HandleFunc("/v1/mempool", s.handleMempool)
	s.mux.HandleFunc("/v1/staking", s.handleStaking)
	s.mux.HandleFunc("/v1/proposals", s.handleProposals)

	// AI model/node list (read-only)
	s.mux.HandleFunc("/v1/ai/models", s.handleListModels)
	s.mux.HandleFunc("/v1/ai/nodes", s.handleListNodes)

	// Migration queries (read-only)
	s.mux.HandleFunc("/api/migration/snapshot", s.handleMigrationSnapshot)
	s.mux.HandleFunc("/api/migration/rates", s.handleMigrationRates)
	s.mux.HandleFunc("/api/migration/status/", s.handleMigrationStatus)
	s.mux.HandleFunc("/api/migration/claimable/", s.handleMigrationClaimable)
	s.mux.HandleFunc("/api/migration/estimate", s.handleMigrationEstimate)

	// =========================================================================
	// Authenticated endpoints - involve fund operations
	// =========================================================================

	// Transaction submission
	s.mux.Handle("/v1/transaction", authWrap(s.handleSubmitTransaction))

	// Wallet management
	s.mux.Handle("/v1/wallet/create", authWrap(s.handleCreateWallet))
	s.mux.Handle("/v1/wallet/restore", authWrap(s.handleRestoreWallet))
	s.mux.Handle("/v1/wallet/import", authWrap(s.handleImportWallet))
	s.mux.Handle("/v1/wallet/export", authWrap(s.handleExportWallet))
	s.mux.Handle("/v1/wallet/balance", authWrap(s.handleWalletBalance))
	s.mux.Handle("/v1/wallet/send", authWrap(s.handleSendTransaction))

	// Staking operations (involve funds) — true-stake model
	s.mux.Handle("/v1/stake", authWrap(s.handleStakeCreate))
	s.mux.Handle("/v1/unstake", authWrap(s.handleStakeRelease))

	// Staking queries (read-only)
	s.mux.HandleFunc("/v1/wallet/stake", s.handleGetStake)
	s.mux.HandleFunc("/v1/stake/info/", s.handleStakeInfo)

	// Channel operations
	s.mux.Handle("/v1/channel/open", authWrap(s.handleChannelOpen))
	s.mux.Handle("/v1/channel/close", authWrap(s.handleChannelClose))
	s.mux.Handle("/v1/channel/pay", authWrap(s.handleChannelPay))

	// Channel queries (read-only)
	s.mux.HandleFunc("/v1/channel/", s.handleGetChannel)
	s.mux.HandleFunc("/v1/channels", s.handleListChannels)

	// AI inference (billing involved)
	s.mux.Handle("/v1/ai/inference", authWrap(s.handleInference))

	// Migration operations (involve funds)
	s.mux.Handle("/api/migration/claim-aib1", authWrap(s.handleClaimAIB1))
	s.mux.Handle("/api/migration/claim-unlocked", authWrap(s.handleClaimUnlocked))

	// Apply the logging middleware
	s.httpServer.Handler = corsMiddleware(LoggingMiddleware(s.mux))
}

// Start starts the server
func (s *Server) Start() error {
	s.RegisterRoutes()
	log.Printf("Starting API server on port %d", s.port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop stops the server
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("Stopping API server on port %d", s.port)
	return s.httpServer.Shutdown(ctx)
}

// Uptime returns how long the server has been running
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// ============================================================================
// Helper functions
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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetPeersProvider plugs the live P2P peer list into the API.
func (s *Server) SetPeersProvider(fn func() []PeerEntry) {
	s.mu.Lock()
	s.peersFn = fn
	s.mu.Unlock()
}
