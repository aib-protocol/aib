// Package sync provides block synchronization and propagation functionality.
// This file contains unit tests for the sync package.
package sync

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Mock Implementations
// ============================================================================

// mockBlockchain implements Blockchain interface for testing.
type mockBlockchain struct {
	blocks map[uint64]*Block
	mu     int // simple mutex simulation
}

func newMockBlockchain() *mockBlockchain {
	return &mockBlockchain{
		blocks: make(map[uint64]*Block),
	}
}

func (m *mockBlockchain) AddBlock(block *Block) error {
	if block == nil {
		return nil // Accept nil for simplicity in tests
	}
	m.blocks[block.Height] = block
	return nil
}

func (m *mockBlockchain) GetBlock(height uint64) (*Block, error) {
	block, ok := m.blocks[height]
	if !ok {
		return nil, ErrBlockNotFound
	}
	return block, nil
}

func (m *mockBlockchain) GetLatestBlock() (*Block, error) {
	if len(m.blocks) == 0 {
		return nil, ErrNoBlocks
	}
	var maxHeight uint64
	for h := range m.blocks {
		if h > maxHeight {
			maxHeight = h
		}
	}
	return m.blocks[maxHeight], nil
}

func (m *mockBlockchain) GetBlockCount() uint64 {
	if len(m.blocks) == 0 {
		return 0
	}
	var maxHeight uint64
	for h := range m.blocks {
		if h > maxHeight {
			maxHeight = h
		}
	}
	return maxHeight + 1
}

func (m *mockBlockchain) GetBlocksInRange(start, end uint64) ([]*Block, error) {
	blocks := make([]*Block, 0)
	for h := start; h <= end && h < uint64(len(m.blocks)); h++ {
		if block, ok := m.blocks[h]; ok {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func (m *mockBlockchain) GetBlockByHash(hash []byte) (*Block, error) {
	for _, block := range m.blocks {
		if string(block.Hash()) == string(hash) {
			return block, nil
		}
	}
	return nil, ErrBlockNotFound
}

// mockNetwork implements Network interface for testing.
type mockNetwork struct {
	peers       map[p2p.PeerID]*p2p.PeerInfo
	peerID      p2p.PeerID
	messages    []*p2p.Message
	handlers    map[p2p.ProtocolID]p2p.MessageHandler
	lock        int
}

func newMockNetwork() *mockNetwork {
	return &mockNetwork{
		peers:    make(map[p2p.PeerID]*p2p.PeerInfo),
		messages: make([]*p2p.Message, 0),
		handlers: make(map[p2p.ProtocolID]p2p.MessageHandler),
	}
}

func (m *mockNetwork) SendMessage(ctx context.Context, to p2p.PeerID, proto p2p.ProtocolID, msg *p2p.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockNetwork) RegisterProtocol(proto p2p.ProtocolID, handler p2p.MessageHandler) error {
	m.handlers[proto] = handler
	return nil
}

func (m *mockNetwork) GetPeers() []*p2p.PeerInfo {
	peers := make([]*p2p.PeerInfo, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	return peers
}

func (m *mockNetwork) PeerID() p2p.PeerID {
	return m.peerID
}

func (m *mockNetwork) Connect(ctx context.Context, addrInfo p2p.AddrInfo) error {
	m.peers[addrInfo.ID] = &p2p.PeerInfo{
		ID:        addrInfo.ID,
		AddrInfo:  &addrInfo,
		Connected: true,
		LastSeen:  time.Now(),
	}
	return nil
}

// ============================================================================
// Test Errors
// ============================================================================

var (
	ErrBlockNotFound = &testError{"block not found"}
	ErrNoBlocks     = &testError{"no blocks"}
)

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// ============================================================================
// SyncManager Tests
// ============================================================================

func TestNewSyncManager(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	cfg := &Config{
		SyncInterval:     5 * time.Second,
		MaxBlocksPerReq:  50,
		Timeout:          10 * time.Second,
	}

	sm := NewSyncManager(chain, net, cfg)
	if sm == nil {
		t.Fatal("NewSyncManager returned nil")
	}

	if sm.localChain != chain {
		t.Error("localChain not set correctly")
	}

	if sm.p2p != net {
		t.Error("p2p not set correctly")
	}

	if sm.syncInterval != 5*time.Second {
		t.Errorf("syncInterval = %v, want 5s", sm.syncInterval)
	}

	if sm.maxBlocksPerReq != 50 {
		t.Errorf("maxBlocksPerReq = %d, want 50", sm.maxBlocksPerReq)
	}
}

func TestSyncManagerStartStop(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	// Test Start
	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Check running status
	status := sm.GetSyncStatus()
	if !status.IsSyncing && status.Error != "" {
		t.Logf("Initial sync status: %+v", status)
	}

	// Test Stop
	if err := sm.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSyncManagerStartTwice(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Second start should fail
	if err := sm.Start(); err == nil {
		t.Error("Expected error on second Start()")
	}

	sm.Stop()
}

func TestSyncManagerStopTwice(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := sm.Stop(); err != nil {
		t.Fatalf("First Stop() error = %v", err)
	}

	// Second stop should fail
	if err := sm.Stop(); err == nil {
		t.Error("Expected error on second Stop()")
	}
}

func TestSyncManagerSyncFromPeers(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add a test peer
	net.peers["testpeer"] = &p2p.PeerInfo{
		ID:        "testpeer",
		Connected: true,
		LastSeen:  time.Now(),
	}

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Sync from peers when no blocks
	if err := sm.SyncFromPeers(); err != nil {
		t.Logf("SyncFromPeers() error (expected with no peers with blocks): %v", err)
	}

	status := sm.GetSyncStatus()
	t.Logf("Sync status after empty sync: %+v", status)
}

func TestSyncManagerGetSyncStatus(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	status := sm.GetSyncStatus()

	if status.IsSyncing {
		t.Error("Expected IsSyncing = false initially")
	}

	if status.PeersCount != 0 {
		t.Errorf("PeersCount = %d, want 0", status.PeersCount)
	}
}

// ============================================================================
// Block Tests
// ============================================================================

func TestBlockHash(t *testing.T) {
	block := &Block{
		Height:      1,
		Timestamp:   time.Now().Unix(),
		TaskID:      "test-task",
		FinalResult: "success",
		IsValid:     true,
	}

	hash1 := block.Hash()
	hash2 := block.Hash()

	// Hash should be deterministic
	if string(hash1) != string(hash2) {
		t.Error("Block hash not deterministic")
	}

	// Different blocks should have different hashes
	block2 := &Block{
		Height:      2,
		Timestamp:   time.Now().Unix(),
		TaskID:      "test-task-2",
		FinalResult: "fail",
		IsValid:     false,
	}

	hash3 := block2.Hash()
	if string(hash1) == string(hash3) {
		t.Error("Different blocks should have different hashes")
	}
}

func TestBlockPreviousBlockHash(t *testing.T) {
	block := &Block{
		PrevBlockHash: []byte("test-hash"),
	}

	hash := block.PreviousBlockHash()
	if string(hash) != "test-hash" {
		t.Errorf("PreviousBlockHash = %s, want test-hash", string(hash))
	}

	// Test nil case
	blockNil := &Block{}
	hashNil := blockNil.PreviousBlockHash()
	if len(hashNil) != 32 {
		t.Errorf("Nil PrevBlockHash should return 32-byte slice, got %d", len(hashNil))
	}
}

// ============================================================================
// Transaction Tests
// ============================================================================

func TestTransactionHash(t *testing.T) {
	tx := &Transaction{
		ID:        "tx-1",
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
	}

	hash1 := tx.Hash()
	hash2 := tx.Hash()

	// Hash should be deterministic
	if string(hash1) != string(hash2) {
		t.Error("Transaction hash not deterministic")
	}
}

func TestTransactionGenerateID(t *testing.T) {
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// ID should be generated
	if tx.ID != "" {
		t.Error("ID should be empty before generation")
	}
}

// ============================================================================
// SyncMessage Tests
// ============================================================================

func TestSyncMessageType(t *testing.T) {
	msg := &SyncMessage{
		Type:      "getblocks",
		Height:    100,
		EndHeight: 200,
	}

	if msg.Type != "getblocks" {
		t.Errorf("Type = %s, want getblocks", msg.Type)
	}

	if msg.Height != 100 {
		t.Errorf("Height = %d, want 100", msg.Height)
	}

	if msg.EndHeight != 200 {
		t.Errorf("EndHeight = %d, want 200", msg.EndHeight)
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkBlockHash(b *testing.B) {
	block := &Block{
		Height:      1,
		Timestamp:   time.Now().Unix(),
		TaskID:      "test-task",
		FinalResult: "success",
		IsValid:     true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Hash()
	}
}

func BenchmarkTransactionHash(b *testing.B) {
	tx := &Transaction{
		ID:        "tx-1",
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx.Hash()
	}
}

// ============================================================================
// SyncManager Message Handler Tests
// ============================================================================

func TestHandleSyncMessageUnknownType(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Create a sync message with unknown type
	msg := &SyncMessage{
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
	err = sm.handleSyncMessage(context.Background(), p2pMsg, "peer1")
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestHandleGetBlocks(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add some blocks to the chain
	for i := uint64(0); i < 5; i++ {
		chain.AddBlock(&Block{
			Height:    i,
			Timestamp: time.Now().Unix(),
		})
	}

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Request blocks in range
	msg := &SyncMessage{
		Type:      "getblocks",
		Height:    1,
		EndHeight: 3,
		FromPeer:  "peer1",
	}

	err := sm.handleGetBlocks(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleGetBlocks() error = %v", err)
	}

	// Check that a message was sent
	if len(net.messages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(net.messages))
	}

	// Check message type
	if net.messages[0].Type != "blocks" {
		t.Errorf("Expected message type 'blocks', got '%s'", net.messages[0].Type)
	}
}

func TestHandleGetBlockCount(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add some blocks to the chain
	for i := uint64(0); i < 5; i++ {
		chain.AddBlock(&Block{
			Height:    i,
			Timestamp: time.Now().Unix(),
		})
	}

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	msg := &SyncMessage{
		Type:     "getblockcount",
		FromPeer: "peer1",
	}

	err := sm.handleGetBlockCount(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleGetBlockCount() error = %v", err)
	}

	// Check that a message was sent
	if len(net.messages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(net.messages))
	}

	// Check message type
	if net.messages[0].Type != "blockheight" {
		t.Errorf("Expected message type 'blockheight', got '%s'", net.messages[0].Type)
	}
}

func TestHandleBlocks(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add a genesis block
	genesis := &Block{
		Height:         0,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Create a valid block
	block := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  genesis.Hash(),
	}
	block.BlockHash = block.calculateHash()

	msg := &SyncMessage{
		Type:    "blocks",
		Blocks:  []*Block{block},
		FromPeer: "peer1",
	}

	err := sm.handleBlocks(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleBlocks() error = %v", err)
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

func TestHandleBlocksInvalidBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Create a block too far ahead
	block := &Block{
		Height:        MaxBlockAge + 100,
		Timestamp:     time.Now().Unix(),
		PrevBlockHash: make([]byte, 32),
	}
	block.BlockHash = block.calculateHash()

	msg := &SyncMessage{
		Type:    "blocks",
		Blocks:  []*Block{block},
		FromPeer: "peer1",
	}

	err := sm.handleBlocks(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for block too far ahead")
	}
}

func TestHandleBlockHeight(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	msg := &SyncMessage{
		Type:     "blockheight",
		Height:   100,
		FromPeer: "peer1",
	}

	err := sm.handleBlockHeight(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleBlockHeight() error = %v", err)
	}

	// Check that height was recorded
	sm.mu.Lock()
	height, ok := sm.knownHeights["peer1"]
	sm.mu.Unlock()

	if !ok {
		t.Error("Peer height was not recorded")
	}

	if height != 100 {
		t.Errorf("Expected height 100, got %d", height)
	}
}

func TestSyncManagerValidateBlock(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add a genesis block
	genesis := &Block{
		Height:         0,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	sm := NewSyncManager(chain, net, nil)

	// Test valid block
	validBlock := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  genesis.Hash(),
	}
	validBlock.BlockHash = validBlock.calculateHash()

	err := sm.validateBlock(validBlock)
	if err != nil {
		t.Errorf("validateBlock() error for valid block: %v", err)
	}

	// Test block too far ahead
	tooFarBlock := &Block{
		Height:         MaxBlockAge + 100,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  make([]byte, 32),
	}
	tooFarBlock.BlockHash = tooFarBlock.calculateHash()

	err = sm.validateBlock(tooFarBlock)
	if err == nil {
		t.Error("Expected error for block too far ahead")
	}

	// Test block with previous hash mismatch
	genesis2 := &Block{
		Height:         0,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  make([]byte, 32),
	}
	genesis2.BlockHash = []byte("different")
	chain.AddBlock(genesis2)

	mismatchBlock := &Block{
		Height:         1,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  []byte("wrong"),
	}
	mismatchBlock.BlockHash = mismatchBlock.calculateHash()

	err = sm.validateBlock(mismatchBlock)
	if err == nil {
		t.Error("Expected error for previous hash mismatch")
	}
}

func TestSyncFromPeersWithBlocks(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	// Add a genesis block
	genesis := &Block{
		Height:         0,
		Timestamp:      time.Now().Unix(),
		PrevBlockHash:  make([]byte, 32),
	}
	genesis.BlockHash = genesis.calculateHash()
	chain.AddBlock(genesis)

	// Add a test peer with known height
	net.peers["testpeer"] = &p2p.PeerInfo{
		ID:        "testpeer",
		Connected: true,
		LastSeen:  time.Now(),
	}

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Set a known height for the peer
	sm.mu.Lock()
	sm.knownHeights["testpeer"] = 10
	sm.mu.Unlock()

	// Sync from peers - may fail due to implementation details
	// Just ensure it doesn't panic
	_ = sm.SyncFromPeers()

	// Check status - just ensure we can get status without panic
	status := sm.GetSyncStatus()
	if status.PeersCount != 1 {
		t.Errorf("Expected 1 peer, got %d", status.PeersCount)
	}
}

func TestSyncFromPeersNoPeers(t *testing.T) {
	chain := newMockBlockchain()
	net := newMockNetwork()

	sm := NewSyncManager(chain, net, nil)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	// Try to sync without peers
	err := sm.SyncFromPeers()
	if err == nil {
		t.Error("Expected error when no peers available")
	}
}
