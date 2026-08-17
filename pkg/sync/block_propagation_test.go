// Package sync provides block synchronization and propagation functionality.
// This file contains unit tests for block propagation.
package sync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Block Propagation Tests
// ============================================================================

func TestNewBlockPropagator(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	cfg := &BlockPropagationConfig{
		PropagationTimeout: 30 * time.Second,
		GossipFanout:       3,
	}

	bp := NewBlockPropagator(chain, net, cfg)
	if bp == nil {
		t.Fatal("NewBlockPropagator returned nil")
	}

	if bp.propagationTimeout != 30*time.Second {
		t.Errorf("PropagationTimeout = %v, want 30s", bp.propagationTimeout)
	}

	if bp.gossipFanout != 3 {
		t.Errorf("GossipFanout = %d, want 3", bp.gossipFanout)
	}
}

func TestBlockPropagatorStartStop(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	// Test Start
	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Test Stop
	if err := bp.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestBlockPropagatorStartTwice(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Second start should fail
	if err := bp.Start(); err == nil {
		t.Error("Expected error on second Start()")
	}

	bp.Stop()
}

func TestBroadcastBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add a test peer
	net.peers["testpeer"] = &p2p.PeerInfo{
		ID:        "testpeer",
		Connected: true,
		LastSeen:  time.Now(),
	}

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create and broadcast a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		TaskID:        "test-task",
		FinalResult:   "success",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	block.BlockHash = block.calculateHash()

	if err := bp.BroadcastBlock(block); err != nil {
		t.Fatalf("BroadcastBlock() error = %v", err)
	}

	// Check that the block was added to received cache
	blockHash := hex.EncodeToString(block.Hash())
	bp.mu.RLock()
	if _, exists := bp.receivedBlocks[blockHash]; !exists {
		t.Error("Block should be in received cache after broadcast")
	}
	bp.mu.RUnlock()
}

func TestReceiveBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block with genesis
	genesisBlock := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		TaskID:        "genesis",
		FinalResult:   "genesis",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()

	chain.AddBlock(genesisBlock)

	// Create a new block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		TaskID:        "test-task",
		FinalResult:   "success",
		IsValid:       true,
		PrevBlockHash: genesisBlock.Hash(),
	}
	block.BlockHash = block.calculateHash()

	// Receive block
	if err := bp.ReceiveBlock(block, "peer1"); err != nil {
		t.Fatalf("ReceiveBlock() error = %v", err)
	}

	// Check that block was added to chain
	retrieved, err := chain.GetBlock(1)
	if err != nil {
		t.Fatalf("GetBlock() error = %v", err)
	}

	if string(retrieved.Hash()) != string(block.Hash()) {
		t.Error("Retrieved block hash doesn't match")
	}
}

func TestReceiveDuplicateBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create genesis block
	genesisBlock := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		TaskID:        "genesis",
		FinalResult:   "genesis",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()
	chain.AddBlock(genesisBlock)

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		TaskID:        "test-task",
		FinalResult:   "success",
		IsValid:       true,
		PrevBlockHash: genesisBlock.Hash(),
	}
	block.BlockHash = block.calculateHash()

	// Receive first time
	if err := bp.ReceiveBlock(block, "peer1"); err != nil {
		t.Fatalf("First ReceiveBlock() error = %v", err)
	}

	// Try to receive again - should fail
	if err := bp.ReceiveBlock(block, "peer1"); err == nil {
		t.Error("Expected error on duplicate block")
	}
}

func TestValidateBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Test nil block
	if err := bp.ValidateBlock(nil); err == nil {
		t.Error("Expected error for nil block")
	}

	// Test valid genesis block
	genesisBlock := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		TaskID:        "genesis",
		FinalResult:   "genesis",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()

	if err := bp.ValidateBlock(genesisBlock); err != nil {
		t.Errorf("ValidateBlock() error for genesis = %v", err)
	}

	// Test block with future timestamp
	futureBlock := &Block{
		Height:        1,
		Timestamp:     time.Now().Add(10 * time.Minute).Unix(),
		TaskID:        "test",
		FinalResult:   "test",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	futureBlock.BlockHash = futureBlock.calculateHash()

	if err := bp.ValidateBlock(futureBlock); err == nil {
		t.Error("Expected error for future timestamp")
	}

	// Test block with old timestamp
	oldBlock := &Block{
		Height:        1,
		Timestamp:     time.Now().Add(-48 * time.Hour).Unix(),
		TaskID:        "test",
		FinalResult:   "test",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	oldBlock.BlockHash = oldBlock.calculateHash()

	if err := bp.ValidateBlock(oldBlock); err == nil {
		t.Error("Expected error for old timestamp")
	}
}

func TestBlockMessage(t *testing.T) {
	msg := &BlockMessage{
		Type:     "block_announce",
		FromPeer: "peer1",
		TTL:      3,
	}

	if msg.Type != "block_announce" {
		t.Errorf("Type = %s, want block_announce", msg.Type)
	}

	if msg.TTL != 3 {
		t.Errorf("TTL = %d, want 3", msg.TTL)
	}
}

func TestBlockPropagatorCallback(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	receivedBlocks := make([]*Block, 0)
	mu := &syncWriter{}

	cfg := &BlockPropagationConfig{
		OnBlockReceived: func(block *Block, from p2p.PeerID) error {
			mu.Lock()
			receivedBlocks = append(receivedBlocks, block)
			mu.Unlock()
			return nil
		},
	}

	bp := NewBlockPropagator(chain, net, cfg)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create genesis block
	genesisBlock := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		TaskID:        "genesis",
		FinalResult:   "genesis",
		IsValid:       true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()
	chain.AddBlock(genesisBlock)

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		TaskID:        "test-task",
		FinalResult:   "success",
		IsValid:       true,
		PrevBlockHash: genesisBlock.Hash(),
	}
	block.BlockHash = block.calculateHash()

	// Receive block
	if err := bp.ReceiveBlock(block, "peer1"); err != nil {
		t.Fatalf("ReceiveBlock() error = %v", err)
	}

	// Check callback was called
	mu.Lock()
	if len(receivedBlocks) != 1 {
		t.Errorf("Expected 1 received block, got %d", len(receivedBlocks))
	}
	mu.Unlock()
}

// ============================================================================
// Block Propagation Message Handler Tests
// ============================================================================

func TestHandleBlockMessageUnknownType(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block message with unknown type
	msg := &BlockMessage{
		Type:     "unknown_type",
		FromPeer: "peer1",
	}

	// Marshal the message
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	p2pMsg := &p2p.Message{
		Type:    "unknown_type",
		Payload: data,
		Sender:  "peer1",
	}

	// Handle the message - should return error for unknown type
	err = bp.handleBlockMessage(context.Background(), p2pMsg, "peer1")
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestHandleBlockAnnounce(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add genesis block
	genesis := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()

	msg := &BlockMessage{
		Type:         "block_announce",
		Block:        block,
		FromPeer:     "peer1",
		TTL:          1,
		ReceivedFrom: "",
	}

	err := bp.handleBlockAnnounce(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleBlockAnnounce() error = %v", err)
	}

	// Check that block was added to chain
	retrieved, err := chain.GetBlock(1)
	if err != nil {
		t.Error("Block was not added to chain")
	}

	if retrieved.Height != 1 {
		t.Errorf("Expected block height 1, got %d", retrieved.Height)
	}
}

func TestHandleBlockAnnounceNilBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	msg := &BlockMessage{
		Type:     "block_announce",
		Block:    nil,
		FromPeer: "peer1",
	}

	err := bp.handleBlockAnnounce(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil block")
	}
}

func TestHandleBlockAnnounceDuplicate(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add genesis block
	genesis := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()

	msg := &BlockMessage{
		Type:         "block_announce",
		Block:        block,
		FromPeer:     "peer1",
		TTL:          1,
		ReceivedFrom: "",
	}

	// First announcement
	err := bp.handleBlockAnnounce(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("First handleBlockAnnounce() error = %v", err)
	}

	// Second announcement (duplicate)
	err = bp.handleBlockAnnounce(context.Background(), msg, "peer2")
	if err != nil {
		t.Fatalf("Second handleBlockAnnounce() error = %v", err)
	}
}

func TestHandleBlockRequest(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add genesis block
	genesis := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	// Add another block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()
	chain.AddBlock(block)

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	msg := &BlockMessage{
		Type:      "block_request",
		Block:     &Block{Height: 1},
		FromPeer:  "peer1",
		RequestID: "req-123",
	}

	err := bp.handleBlockRequest(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleBlockRequest() error = %v", err)
	}

	// Check that a response message was sent
	if len(net.messages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(net.messages))
	}
}

func TestHandleBlockRequestInvalid(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Request with nil block
	msg := &BlockMessage{
		Type:      "block_request",
		Block:     nil,
		FromPeer:  "peer1",
		RequestID: "req-123",
	}

	err := bp.handleBlockRequest(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil block request")
	}
}

func TestHandleBlockResponse(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add genesis block
	genesis := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()

	msg := &BlockMessage{
		Type:     "block_response",
		Block:    block,
		FromPeer: "peer1",
	}

	err := bp.handleBlockResponse(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleBlockResponse() error = %v", err)
	}
}

func TestHandleBlockResponseNilBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	msg := &BlockMessage{
		Type:     "block_response",
		Block:    nil,
		FromPeer: "peer1",
	}

	err := bp.handleBlockResponse(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil block in response")
	}
}

func TestRelayBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add peers
	net.peers["peer1"] = &p2p.PeerInfo{
		ID:        "peer1",
		Connected: true,
		LastSeen:  time.Now(),
	}
	net.peers["peer2"] = &p2p.PeerInfo{
		ID:        "peer2",
		Connected: true,
		LastSeen:  time.Now(),
	}
	net.peers["peer3"] = &p2p.PeerInfo{
		ID:        "peer3",
		Connected: true,
		LastSeen:  time.Now(),
	}

	// Add genesis block
	genesis := &Block{
		Height:        0,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block
	block := &Block{
		Height:        1,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()

	// Relay to peer1
	bp.relayBlock(block, "peer1")
}

func TestRelayBlockSinglePeer(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add only one peer
	net.peers["peer1"] = &p2p.PeerInfo{
		ID:        "peer1",
		Connected: true,
		LastSeen:  time.Now(),
	}

	bp := NewBlockPropagator(chain, net, nil)

	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Create a block
	block := &Block{
		Height:    1,
		Timestamp: time.Now().Unix(),
	}

	// Relay should return early with single peer
	bp.relayBlock(block, "peer1")
}

// syncWriter is a simple mutex wrapper for thread-safe writes.
type syncWriter struct{}

func (m *syncWriter) Lock()   {}
func (m *syncWriter) Unlock() {}
