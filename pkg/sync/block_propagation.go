// Package sync provides block synchronization and propagation functionality.
// This file implements block propagation via gossip protocol.
package sync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Block Propagation
// ============================================================================

// BlockPropagator handles block propagation via gossip protocol.
type BlockPropagator struct {
	mu         sync.RWMutex
	localChain Blockchain
	p2p        Network
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Configuration
	propagationTimeout time.Duration
	gossipFanout       int

	// Received blocks cache (for deduplication)
	receivedBlocks  map[string]time.Time // block hash -> received time
	announcedBlocks map[string]time.Time // block hash -> announcement time

	// Pending validation
	pendingValidation chan *Block

	// Callbacks
	onBlockValidated func(*Block, p2p.PeerID) error
	onBlockReceived  func(*Block, p2p.PeerID) error
}

// BlockPropagationConfig holds configuration for block propagation.
type BlockPropagationConfig struct {
	PropagationTimeout time.Duration
	GossipFanout       int
	OnBlockValidated   func(*Block, p2p.PeerID) error
	OnBlockReceived    func(*Block, p2p.PeerID) error
}

// DefaultBlockPropagationConfig returns default configuration.
func DefaultBlockPropagationConfig() *BlockPropagationConfig {
	return &BlockPropagationConfig{
		PropagationTimeout: 30 * time.Second,
		GossipFanout:       GossipFanout,
	}
}

// NewBlockPropagator creates a new BlockPropagator.
func NewBlockPropagator(localChain Blockchain, p2p Network, cfg *BlockPropagationConfig) *BlockPropagator {
	if cfg == nil {
		cfg = DefaultBlockPropagationConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BlockPropagator{
		localChain:         localChain,
		p2p:                p2p,
		ctx:                ctx,
		cancel:             cancel,
		propagationTimeout: cfg.PropagationTimeout,
		gossipFanout:       cfg.GossipFanout,
		receivedBlocks:     make(map[string]time.Time),
		announcedBlocks:    make(map[string]time.Time),
		pendingValidation:  make(chan *Block, 100),
		onBlockValidated:   cfg.OnBlockValidated,
		onBlockReceived:    cfg.OnBlockReceived,
	}
}

// Start starts the block propagator.
func (bp *BlockPropagator) Start() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.running {
		return errors.New("block propagator: already running")
	}

	// Register block propagation protocol
	if err := bp.p2p.RegisterProtocol(BlockPropagationProtocol, bp.handleBlockMessage); err != nil {
		return fmt.Errorf("failed to register block protocol: %w", err)
	}

	bp.running = true

	// Start validation worker
	bp.wg.Add(1)
	go bp.validationWorker()

	return nil
}

// Stop stops the block propagator.
func (bp *BlockPropagator) Stop() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if !bp.running {
		return errors.New("block propagator: not running")
	}

	bp.running = false
	bp.cancel()

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		bp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	return nil
}

// BroadcastBlock broadcasts a new block to the network.
func (bp *BlockPropagator) BroadcastBlock(block *Block) error {
	if block == nil {
		return errors.New("block propagator: nil block")
	}

	bp.mu.Lock()
	if !bp.running {
		bp.mu.Unlock()
		return errors.New("block propagator: not running")
	}
	bp.mu.Unlock()

	// Validate block before broadcasting
	if err := bp.ValidateBlock(block); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	// Add to received cache to prevent echoing back
	blockHash := hex.EncodeToString(block.Hash())
	bp.mu.Lock()
	bp.receivedBlocks[blockHash] = time.Now()
	bp.mu.Unlock()

	// Create block announcement message
	announce := &BlockMessage{
		Type:         "block_announce",
		Block:        block,
		FromPeer:     bp.p2p.PeerID(),
		TTL:          bp.gossipFanout,
		ReceivedFrom: "",
	}

	return bp.gossipBlock(block, announce)
}

// ReceiveBlock receives a block from a peer.
func (bp *BlockPropagator) ReceiveBlock(block *Block, from p2p.PeerID) error {
	if block == nil {
		return errors.New("block propagator: nil block")
	}

	bp.mu.Lock()
	if !bp.running {
		bp.mu.Unlock()
		return errors.New("block propagator: not running")
	}
	bp.mu.Unlock()

	// Check for duplicate
	blockHash := hex.EncodeToString(block.Hash())
	bp.mu.Lock()
	if _, exists := bp.receivedBlocks[blockHash]; exists {
		bp.mu.Unlock()
		return errors.New("block already received")
	}
	bp.receivedBlocks[blockHash] = time.Now()
	bp.mu.Unlock()

	// Validate block
	if err := bp.ValidateBlock(block); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	// Add to local chain
	if err := bp.localChain.AddBlock(block); err != nil {
		return fmt.Errorf("failed to add block to chain: %w", err)
	}

	// Trigger callback
	if bp.onBlockReceived != nil {
		if err := bp.onBlockReceived(block, from); err != nil {
			return err
		}
	}

	// Relay to other peers (gossip)
	bp.relayBlock(block, from)

	return nil
}

// ValidateBlock validates a block.
func (bp *BlockPropagator) ValidateBlock(block *Block) error {
	if block == nil {
		return errors.New("block is nil")
	}

	// Validate basic fields
	if block.Height == 0 && block.Timestamp == 0 {
		return errors.New("invalid genesis block")
	}

	// Verify block hash
	expectedHash := block.calculateHash()
	if hex.EncodeToString(block.Hash()) != hex.EncodeToString(expectedHash) {
		return errors.New("block hash mismatch")
	}

	// Verify previous block hash
	if block.Height > 0 {
		prevBlock, err := bp.localChain.GetBlock(block.Height - 1)
		if err != nil {
			// Previous block might not exist during initial sync
			// Only validate if we have the previous block
			latest, latestErr := bp.localChain.GetLatestBlock()
			if latestErr == nil && block.Height <= latest.Height {
				return fmt.Errorf("previous block not found: %w", err)
			}
		} else {
			if string(block.PreviousBlockHash()) != string(prevBlock.Hash()) {
				return errors.New("previous block hash mismatch")
			}
		}
	}

	// Verify timestamp is not too far in the future
	maxFutureTime := time.Now().Add(5 * time.Minute).Unix()
	if block.Timestamp > maxFutureTime {
		return errors.New("block timestamp too far in the future")
	}

	// Verify timestamp is not too old
	minTimestamp := time.Now().Add(-24 * time.Hour).Unix()
	if block.Timestamp < minTimestamp {
		return errors.New("block timestamp too old")
	}

	return nil
}

// ============================================================================
// Private Methods
// ============================================================================

func (bp *BlockPropagator) handleBlockMessage(ctx context.Context, msg *p2p.Message, from p2p.PeerID) error {
	var blockMsg BlockMessage
	if err := json.Unmarshal(msg.Payload, &blockMsg); err != nil {
		return fmt.Errorf("failed to unmarshal block message: %w", err)
	}

	switch blockMsg.Type {
	case "block_announce":
		return bp.handleBlockAnnounce(ctx, &blockMsg, from)
	case "block_request":
		return bp.handleBlockRequest(ctx, &blockMsg, from)
	case "block_response":
		return bp.handleBlockResponse(ctx, &blockMsg, from)
	default:
		return fmt.Errorf("unknown block message type: %s", blockMsg.Type)
	}
}

func (bp *BlockPropagator) handleBlockAnnounce(ctx context.Context, msg *BlockMessage, from p2p.PeerID) error {
	if msg.Block == nil {
		return errors.New("nil block in announcement")
	}

	blockHash := hex.EncodeToString(msg.Block.Hash())

	// Check for duplicate announcement
	bp.mu.Lock()
	if _, exists := bp.announcedBlocks[blockHash]; exists {
		bp.mu.Unlock()
		return nil // Already announced
	}
	bp.announcedBlocks[blockHash] = time.Now()

	// Store original sender for TTL
	originPeer := from
	if msg.ReceivedFrom != "" {
		originPeer = msg.ReceivedFrom
	}
	bp.mu.Unlock()

	// Validate and add block
	block := msg.Block
	if err := bp.ValidateBlock(block); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	// Add to chain
	if err := bp.localChain.AddBlock(block); err != nil {
		// Block might already exist or be invalid
		// Don't return error, just don't relay
	}

	// Trigger callback
	if bp.onBlockReceived != nil {
		bp.onBlockReceived(block, from)
	}

	// Relay to other peers if TTL > 0
	if msg.TTL > 0 {
		msg.TTL--
		msg.ReceivedFrom = originPeer
		bp.relayBlock(block, originPeer)
	}

	return nil
}

func (bp *BlockPropagator) handleBlockRequest(ctx context.Context, msg *BlockMessage, from p2p.PeerID) error {
	if msg.Block == nil || msg.Block.Height == 0 {
		return errors.New("invalid block request")
	}

	// Fetch requested block
	block, err := bp.localChain.GetBlock(msg.Block.Height)
	if err != nil {
		return fmt.Errorf("block not found: %w", err)
	}

	// Send response
	response := &BlockMessage{
		Type:      "block_response",
		Block:     block,
		FromPeer:  bp.p2p.PeerID(),
		RequestID: msg.RequestID,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	responseMsg := &p2p.Message{
		Type:     "block_response",
		Payload:  data,
		Sender:   bp.p2p.PeerID(),
		Protocol: BlockPropagationProtocol,
	}

	return bp.p2p.SendMessage(ctx, from, BlockPropagationProtocol, responseMsg)
}

func (bp *BlockPropagator) handleBlockResponse(ctx context.Context, msg *BlockMessage, from p2p.PeerID) error {
	if msg.Block == nil {
		return errors.New("nil block in response")
	}

	// Validate and add block
	return bp.ReceiveBlock(msg.Block, from)
}

func (bp *BlockPropagator) gossipBlock(block *Block, msg *BlockMessage) error {
	peers := bp.p2p.GetPeers()
	if len(peers) == 0 {
		return nil
	}

	// Shuffle peers for randomness
	shuffledPeers := make([]*p2p.PeerInfo, len(peers))
	copy(shuffledPeers, peers)

	// Select random peers to send to
	numPeers := bp.gossipFanout
	if len(shuffledPeers) < numPeers {
		numPeers = len(shuffledPeers)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Send to selected peers
	for i := 0; i < numPeers; i++ {
		peerMsg := &p2p.Message{
			Type:     msg.Type,
			Payload:  data,
			Sender:   bp.p2p.PeerID(),
			Protocol: BlockPropagationProtocol,
		}

		ctx, cancel := context.WithTimeout(bp.ctx, bp.propagationTimeout)
		if err := bp.p2p.SendMessage(ctx, shuffledPeers[i].ID, BlockPropagationProtocol, peerMsg); err != nil {
			cancel()
			continue
		}
		cancel()
	}

	return nil
}

func (bp *BlockPropagator) relayBlock(block *Block, from p2p.PeerID) {
	peers := bp.p2p.GetPeers()
	if len(peers) <= 1 {
		return
	}

	msg := &BlockMessage{
		Type:         "block_announce",
		Block:        block,
		FromPeer:     bp.p2p.PeerID(),
		TTL:          bp.gossipFanout - 1,
		ReceivedFrom: from,
	}

	bp.gossipBlock(block, msg)
}

func (bp *BlockPropagator) validationWorker() {
	defer bp.wg.Done()

	for {
		select {
		case <-bp.ctx.Done():
			return
		case block := <-bp.pendingValidation:
			if err := bp.ValidateBlock(block); err != nil {
				// Log validation error
				continue
			}

			// Add to chain
			if err := bp.localChain.AddBlock(block); err != nil {
				continue
			}

			// Trigger callback
			if bp.onBlockValidated != nil {
				bp.onBlockValidated(block, "")
			}
		}
	}
}

// cleanupOldEntries removes old entries from caches.
func (bp *BlockPropagator) cleanupOldEntries() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	now := time.Now()
	expiry := 1 * time.Hour

	for hash, t := range bp.receivedBlocks {
		if now.Sub(t) > expiry {
			delete(bp.receivedBlocks, hash)
		}
	}

	for hash, t := range bp.announcedBlocks {
		if now.Sub(t) > expiry {
			delete(bp.announcedBlocks, hash)
		}
	}
}

// ============================================================================
// BlockMessage
// ============================================================================

// BlockMessage represents a message in the block propagation protocol.
type BlockMessage struct {
	Type         string     `json:"type"`
	Block        *Block     `json:"block,omitempty"`
	FromPeer     p2p.PeerID `json:"from_peer"`
	TTL          int        `json:"ttl"`
	ReceivedFrom p2p.PeerID `json:"received_from,omitempty"`
	RequestID    string     `json:"request_id,omitempty"`
}
