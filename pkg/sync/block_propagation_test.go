// Package sync provides block synchronization and propagation functionality.
// This file contains unit tests for block propagation.
package sync

import (
	"encoding/hex"
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
		Height:      1,
		Timestamp:   time.Now().Unix(),
		TaskID:      "test-task",
		FinalResult: "success",
		IsValid:     true,
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
		Height:      0,
		Timestamp:   time.Now().Unix(),
		TaskID:      "genesis",
		FinalResult: "genesis",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()

	chain.AddBlock(genesisBlock)

	// Create a new block
	block := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		TaskID:         "test-task",
		FinalResult:    "success",
		IsValid:        true,
		PrevBlockHash:  genesisBlock.Hash(),
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
		Height:      0,
		Timestamp:   time.Now().Unix(),
		TaskID:      "genesis",
		FinalResult: "genesis",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()
	chain.AddBlock(genesisBlock)

	// Create a block
	block := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		TaskID:         "test-task",
		FinalResult:    "success",
		IsValid:        true,
		PrevBlockHash:  genesisBlock.Hash(),
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
		Height:      0,
		Timestamp:   time.Now().Unix(),
		TaskID:      "genesis",
		FinalResult: "genesis",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()

	if err := bp.ValidateBlock(genesisBlock); err != nil {
		t.Errorf("ValidateBlock() error for genesis = %v", err)
	}

	// Test block with future timestamp
	futureBlock := &Block{
		Height:      1,
		Timestamp:   time.Now().Add(10 * time.Minute).Unix(),
		TaskID:      "test",
		FinalResult: "test",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	futureBlock.BlockHash = futureBlock.calculateHash()

	if err := bp.ValidateBlock(futureBlock); err == nil {
		t.Error("Expected error for future timestamp")
	}

	// Test block with old timestamp
	oldBlock := &Block{
		Height:      1,
		Timestamp:   time.Now().Add(-48 * time.Hour).Unix(),
		TaskID:      "test",
		FinalResult: "test",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	oldBlock.BlockHash = oldBlock.calculateHash()

	if err := bp.ValidateBlock(oldBlock); err == nil {
		t.Error("Expected error for old timestamp")
	}
}

func TestBlockMessage(t *testing.T) {
	msg := &BlockMessage{
		Type:      "block_announce",
		FromPeer: "peer1",
		TTL:       3,
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
		Height:      0,
		Timestamp:   time.Now().Unix(),
		TaskID:      "genesis",
		FinalResult: "genesis",
		IsValid:     true,
		PrevBlockHash: make([]byte, 32),
	}
	genesisBlock.BlockHash = genesisBlock.calculateHash()
	chain.AddBlock(genesisBlock)

	// Create a block
	block := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		TaskID:         "test-task",
		FinalResult:    "success",
		IsValid:        true,
		PrevBlockHash:  genesisBlock.Hash(),
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

// syncWriter is a simple mutex wrapper for thread-safe writes.
type syncWriter struct{}

func (m *syncWriter) Lock()   {}
func (m *syncWriter) Unlock() {}
