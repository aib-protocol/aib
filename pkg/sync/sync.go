// Package sync provides block synchronization and propagation functionality.
// This package implements P2P block sync, block propagation via gossip protocol,
// transaction propagation, and fork choice logic.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Protocol IDs
// ============================================================================

const (
	// SyncProtocol is the protocol for block synchronization requests.
	SyncProtocol p2p.ProtocolID = "/aib/sync/1.0.0"

	// BlockPropagationProtocol is the protocol for block gossip.
	BlockPropagationProtocol p2p.ProtocolID = "/aib/block/1.0.0"

	// TxPropagationProtocol is the protocol for transaction gossip.
	TxPropagationProtocol p2p.ProtocolID = "/aib/tx/1.0.0"

	// DefaultSyncInterval is the default interval between sync attempts.
	DefaultSyncInterval = 10 * time.Second

	// DefaultMaxBlocksPerRequest is the maximum blocks to request in one sync.
	DefaultMaxBlocksPerRequest = 100

	// DefaultTimeout is the default timeout for sync operations.
	DefaultTimeout = 30 * time.Second

	// MaxBlockAge is the maximum age of blocks to sync (in blocks).
	MaxBlockAge = 1000

	// GossipFanout is the number of peers to forward gossip to.
	GossipFanout = 3
)

// ============================================================================
// Blockchain Interface
// ============================================================================

// Blockchain defines the interface for blockchain operations needed by sync.
type Blockchain interface {
	AddBlock(block *Block) error
	GetBlock(height uint64) (*Block, error)
	GetLatestBlock() (*Block, error)
	GetBlockCount() uint64
	GetBlocksInRange(start, end uint64) ([]*Block, error)
	GetBlockByHash(hash []byte) (*Block, error)
}

// Block represents a block in the blockchain (copied from consensus package).
type Block struct {
	Height           uint64            `json:"height"`
	PrevBlockHash    []byte            `json:"prev_block_hash"`
	Timestamp        int64             `json:"timestamp"`
	TaskID           string            `json:"task_id"`
	FinalResult      string            `json:"final_result"`
	IsValid          bool              `json:"is_valid"`
	AgreementRate    float64           `json:"agreement_rate"`
	NodeResults      map[string]string `json:"node_results"`
	ConsensusNodes   []string          `json:"consensus_nodes"`
	DisagreeingNodes []string          `json:"disagreeing_nodes"`
	Metadata         map[string]string `json:"metadata"`
	BlockHash        []byte            `json:"hash"`
	Nonce            uint64            `json:"nonce"`
	Transactions     []*Transaction    `json:"transactions"`
}

// Transaction represents a transaction in the blockchain.
type Transaction struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    uint64 `json:"amount"`
	Fee       uint64 `json:"fee"`
	Timestamp int64  `json:"timestamp"`
	Signature []byte `json:"signature"`
	Nonce     uint64 `json:"nonce"`
	Payload   []byte `json:"payload"`
}

// Hash returns the transaction hash.
func (tx *Transaction) Hash() []byte {
	// Simplified hash calculation
	data := fmt.Sprintf("%s%s%s%d%d%d%d",
		tx.ID, tx.From, tx.To, tx.Amount, tx.Fee, tx.Timestamp, tx.Nonce)
	return []byte(data)
}

// PreviousBlockHash returns the previous block hash.
func (b *Block) PreviousBlockHash() []byte {
	if b.PrevBlockHash == nil {
		return make([]byte, 32)
	}
	return b.PrevBlockHash
}

// Hash returns the block hash.
func (b *Block) Hash() []byte {
	if b.BlockHash == nil {
		b.BlockHash = b.calculateHash()
	}
	return b.BlockHash
}

func (b *Block) calculateHash() []byte {
	// Simplified hash - in production use proper hashing
	data := fmt.Sprintf("%d%s%d%s%s%v%f%d",
		b.Height, string(b.PrevBlockHash), b.Timestamp,
		b.TaskID, b.FinalResult, b.IsValid, b.AgreementRate, b.Nonce)
	return []byte(data)
}

// ============================================================================
// SyncStatus
// ============================================================================

// SyncStatus represents the current synchronization status.
type SyncStatus struct {
	IsSyncing    bool      `json:"is_syncing"`
	StartBlock   uint64    `json:"start_block"`
	CurrentBlock uint64    `json:"current_block"`
	TargetBlock  uint64    `json:"target_block"`
	PeersCount   int       `json:"peers_count"`
	LastSyncTime time.Time `json:"last_sync_time"`
	Error        string    `json:"error,omitempty"`
}

// ============================================================================
// SyncManager
// ============================================================================

// SyncManager manages block synchronization from peers.
type SyncManager struct {
	mu         sync.RWMutex
	localChain Blockchain
	p2p        Network
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Configuration
	syncInterval    time.Duration
	maxBlocksPerReq int
	timeout         time.Duration

	// Status
	status        SyncStatus
	lastSyncError error

	// Sync state
	knownHeights    map[p2p.PeerID]uint64
	requestedBlocks map[string]time.Time // block hash -> request time
	pendingRequests map[string]context.CancelFunc

	// Callbacks
	onBlockReceived func(*Block, p2p.PeerID) error
	onChainReorg    func([]*Block, []*Block) error
}

// Network interface for P2P operations (extends p2p.Network).
type Network interface {
	SendMessage(ctx context.Context, to p2p.PeerID, proto p2p.ProtocolID, msg *p2p.Message) error
	RegisterProtocol(proto p2p.ProtocolID, handler p2p.MessageHandler) error
	GetPeers() []*p2p.PeerInfo
	PeerID() p2p.PeerID
	Connect(ctx context.Context, addrInfo p2p.AddrInfo) error
}

// Config holds SyncManager configuration.
type Config struct {
	SyncInterval    time.Duration
	MaxBlocksPerReq int
	Timeout         time.Duration
	OnBlockReceived func(*Block, p2p.PeerID) error
	OnChainReorg    func([]*Block, []*Block) error
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		SyncInterval:    DefaultSyncInterval,
		MaxBlocksPerReq: DefaultMaxBlocksPerRequest,
		Timeout:         DefaultTimeout,
	}
}

// NewSyncManager creates a new SyncManager.
func NewSyncManager(localChain Blockchain, network Network, cfg *Config) *SyncManager {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SyncManager{
		localChain:      localChain,
		p2p:             network,
		ctx:             ctx,
		cancel:          cancel,
		syncInterval:    cfg.SyncInterval,
		maxBlocksPerReq: cfg.MaxBlocksPerReq,
		timeout:         cfg.Timeout,
		onBlockReceived: cfg.OnBlockReceived,
		onChainReorg:    cfg.OnChainReorg,
		knownHeights:    make(map[p2p.PeerID]uint64),
		requestedBlocks: make(map[string]time.Time),
		pendingRequests: make(map[string]context.CancelFunc),
		status:          SyncStatus{IsSyncing: false},
	}
}

// Start starts the sync manager.
func (sm *SyncManager) Start() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return errors.New("sync: already running")
	}

	// Register sync protocol handler
	if err := sm.p2p.RegisterProtocol(SyncProtocol, sm.handleSyncMessage); err != nil {
		return fmt.Errorf("failed to register sync protocol: %w", err)
	}

	sm.running = true
	sm.status = SyncStatus{IsSyncing: false}

	// Start sync loop
	sm.wg.Add(1)
	go sm.syncLoop()

	return nil
}

// Stop stops the sync manager.
func (sm *SyncManager) Stop() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.running {
		return errors.New("sync: not running")
	}

	sm.running = false
	sm.cancel()

	// Wait for all goroutines
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished
	case <-time.After(5 * time.Second):
		// Timeout
	}

	return nil
}

// SyncFromPeers triggers synchronization from connected peers.
func (sm *SyncManager) SyncFromPeers() error {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return errors.New("sync: not running")
	}
	sm.mu.Unlock()

	// Get peers
	peers := sm.p2p.GetPeers()
	if len(peers) == 0 {
		return errors.New("sync: no peers available")
	}

	// Get local chain height
	localHeight, err := sm.getLocalChainHeight()
	if err != nil {
		return fmt.Errorf("failed to get local chain height: %w", err)
	}

	sm.mu.Lock()
	sm.status.IsSyncing = true
	sm.status.StartBlock = localHeight
	sm.mu.Unlock()

	// Find peer with highest height
	var bestPeer *p2p.PeerInfo
	var bestHeight uint64

	for _, peer := range peers {
		if !peer.Connected {
			continue
		}

		height, ok := sm.knownHeights[peer.ID]
		if !ok {
			// Request height from peer
			height = sm.requestBlockHeight(peer.ID)
			sm.knownHeights[peer.ID] = height
		}

		if height > bestHeight {
			bestHeight = height
			bestPeer = peer
		}
	}

	if bestPeer == nil {
		sm.mu.Lock()
		sm.status.IsSyncing = false
		sm.status.Error = "no connected peers"
		sm.mu.Unlock()
		return errors.New("sync: no connected peers")
	}

	// Sync blocks from best peer
	if bestHeight > localHeight {
		err = sm.syncBlocksFromPeer(bestPeer.ID, localHeight+1, bestHeight)
		if err != nil {
			sm.mu.Lock()
			sm.status.Error = err.Error()
			sm.status.IsSyncing = false
			sm.mu.Unlock()
			return fmt.Errorf("failed to sync blocks: %w", err)
		}
	}

	sm.mu.Lock()
	sm.status.IsSyncing = false
	sm.status.LastSyncTime = time.Now()
	sm.status.Error = ""
	sm.mu.Unlock()

	return nil
}

// GetSyncStatus returns the current sync status.
func (sm *SyncManager) GetSyncStatus() SyncStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status := sm.status
	status.PeersCount = len(sm.p2p.GetPeers())

	if latest, err := sm.localChain.GetLatestBlock(); err == nil {
		status.TargetBlock = latest.Height
	}

	return status
}

// ============================================================================
// Private Methods
// ============================================================================

func (sm *SyncManager) syncLoop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(sm.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			if err := sm.SyncFromPeers(); err != nil {
				// Log error but continue
				sm.mu.Lock()
				sm.lastSyncError = err
				sm.mu.Unlock()
			}
		}
	}
}

func (sm *SyncManager) getLocalChainHeight() (uint64, error) {
	latest, err := sm.localChain.GetLatestBlock()
	if err != nil {
		return 0, err
	}
	return latest.Height, nil
}

func (sm *SyncManager) requestBlockHeight(peerID p2p.PeerID) uint64 {
	// Send a getblockcount request
	req := &SyncMessage{
		Type:     "getblockcount",
		Height:   0,
		FromPeer: sm.p2p.PeerID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return 0
	}

	ctx, cancel := context.WithTimeout(sm.ctx, sm.timeout)
	defer cancel()

	msg := &p2p.Message{
		Type:     "getblockcount",
		Payload:  data,
		Sender:   sm.p2p.PeerID(),
		Protocol: SyncProtocol,
	}

	if err := sm.p2p.SendMessage(ctx, peerID, SyncProtocol, msg); err != nil {
		return 0
	}

	// In a real implementation, we would wait for response
	// For now, return 0 to indicate unknown
	return 0
}

func (sm *SyncManager) syncBlocksFromPeer(peerID p2p.PeerID, startHeight, endHeight uint64) error {
	if endHeight-startHeight > uint64(sm.maxBlocksPerReq) {
		endHeight = startHeight + uint64(sm.maxBlocksPerReq) - 1
	}

	// Request blocks
	req := &SyncMessage{
		Type:      "getblocks",
		Height:    startHeight,
		EndHeight: endHeight,
		FromPeer:  sm.p2p.PeerID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(sm.ctx, sm.timeout)
	defer cancel()

	msg := &p2p.Message{
		Type:     "getblocks",
		Payload:  data,
		Sender:   sm.p2p.PeerID(),
		Protocol: SyncProtocol,
	}

	if err := sm.p2p.SendMessage(ctx, peerID, SyncProtocol, msg); err != nil {
		return err
	}

	sm.mu.Lock()
	sm.status.CurrentBlock = startHeight
	sm.mu.Unlock()

	return nil
}

func (sm *SyncManager) handleSyncMessage(ctx context.Context, msg *p2p.Message, from p2p.PeerID) error {
	var syncMsg SyncMessage
	if err := json.Unmarshal(msg.Payload, &syncMsg); err != nil {
		return fmt.Errorf("failed to unmarshal sync message: %w", err)
	}

	switch syncMsg.Type {
	case "getblocks":
		return sm.handleGetBlocks(ctx, &syncMsg, from)
	case "getblockcount":
		return sm.handleGetBlockCount(ctx, &syncMsg, from)
	case "blocks":
		return sm.handleBlocks(ctx, &syncMsg, from)
	case "blockheight":
		return sm.handleBlockHeight(ctx, &syncMsg, from)
	default:
		return fmt.Errorf("unknown sync message type: %s", syncMsg.Type)
	}
}

func (sm *SyncManager) handleGetBlocks(ctx context.Context, msg *SyncMessage, from p2p.PeerID) error {
	blocks, err := sm.localChain.GetBlocksInRange(msg.Height, msg.EndHeight)
	if err != nil {
		return err
	}

	// Serialize blocks
	response := &SyncMessage{
		Type:     "blocks",
		Blocks:   blocks,
		FromPeer: sm.p2p.PeerID(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	responseMsg := &p2p.Message{
		Type:     "blocks",
		Payload:  data,
		Sender:   sm.p2p.PeerID(),
		Protocol: SyncProtocol,
	}

	return sm.p2p.SendMessage(ctx, from, SyncProtocol, responseMsg)
}

func (sm *SyncManager) handleGetBlockCount(ctx context.Context, msg *SyncMessage, from p2p.PeerID) error {
	height, err := sm.getLocalChainHeight()
	if err != nil {
		height = 0
	}

	response := &SyncMessage{
		Type:     "blockheight",
		Height:   height,
		FromPeer: sm.p2p.PeerID(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	responseMsg := &p2p.Message{
		Type:     "blockheight",
		Payload:  data,
		Sender:   sm.p2p.PeerID(),
		Protocol: SyncProtocol,
	}

	return sm.p2p.SendMessage(ctx, from, SyncProtocol, responseMsg)
}

func (sm *SyncManager) handleBlocks(ctx context.Context, msg *SyncMessage, from p2p.PeerID) error {
	for _, block := range msg.Blocks {
		// Validate block
		if err := sm.validateBlock(block); err != nil {
			return fmt.Errorf("invalid block: %w", err)
		}

		// Add to local chain
		if err := sm.localChain.AddBlock(block); err != nil {
			// Block might already exist
			continue
		}

		sm.mu.Lock()
		if block.Height > sm.status.CurrentBlock {
			sm.status.CurrentBlock = block.Height
		}
		sm.mu.Unlock()

		// Trigger callback
		if sm.onBlockReceived != nil {
			if err := sm.onBlockReceived(block, from); err != nil {
				return err
			}
		}
	}

	return nil
}

func (sm *SyncManager) handleBlockHeight(ctx context.Context, msg *SyncMessage, from p2p.PeerID) error {
	sm.mu.Lock()
	sm.knownHeights[from] = msg.Height
	sm.mu.Unlock()
	return nil
}

func (sm *SyncManager) validateBlock(block *Block) error {
	// Check height is within acceptable range
	localHeight, err := sm.getLocalChainHeight()
	if err != nil {
		localHeight = 0
	}

	if block.Height > localHeight+MaxBlockAge {
		return fmt.Errorf("block too far ahead: height %d, local %d", block.Height, localHeight)
	}

	// Verify previous hash
	if block.Height > 0 {
		prevBlock, err := sm.localChain.GetBlock(block.Height - 1)
		if err != nil && block.Height <= localHeight {
			return fmt.Errorf("previous block not found: %w", err)
		}
		if prevBlock != nil {
			expectedPrevHash := prevBlock.Hash()
			if string(block.PreviousBlockHash()) != string(expectedPrevHash) {
				return errors.New("previous hash mismatch")
			}
		}
	}

	return nil
}

// ============================================================================
// SyncMessage
// ============================================================================

// SyncMessage represents a message in the sync protocol.
type SyncMessage struct {
	Type        string     `json:"type"`
	Height      uint64     `json:"height"`
	EndHeight   uint64     `json:"end_height,omitempty"`
	Blocks      []*Block   `json:"blocks,omitempty"`
	BlockHashes []string   `json:"block_hashes,omitempty"`
	FromPeer    p2p.PeerID `json:"from_peer"`
}
