package consensus

import (
	"fmt"
	"testing"
	"time"
)

func TestBlockchain_BasicFlow(t *testing.T) {
	// Create blockchain with memory storage
	bc, err := NewBlockchain(nil, nil)
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}

	// Start blockchain
	if err := bc.Start(); err != nil {
		t.Fatalf("Failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Verify genesis block exists
	if bc.GetBlockCount() != 1 {
		t.Errorf("Expected 1 block (genesis), got %d", bc.GetBlockCount())
	}

	genesis, err := bc.GetBlock(0)
	if err != nil {
		t.Fatalf("Failed to get genesis block: %v", err)
	}

	if genesis.Height != 0 {
		t.Errorf("Genesis block height should be 0, got %d", genesis.Height)
	}

	// Create a block event from ZKML verification
	event := &BlockEvent{
		TaskID:         "task-123",
		FinalResult:    "4",
		IsValid:        true,
		AgreementRate:  1.0,
		NodeResults:    map[string]string{"node1": "4", "node2": "4", "node3": "4"},
		ConsensusNodes: []string{"node1", "node2", "node3"},
		Metadata:       map[string]string{"model": "glm-5", "prompt": "2+2"},
		Timestamp:      time.Now().Unix(),
		BlockHeight:    1,
	}

	// Add block event
	if err := bc.AddBlockEvent(event); err != nil {
		t.Fatalf("Failed to add block event: %v", err)
	}

	// Verify block was added
	if bc.GetBlockCount() != 2 {
		t.Errorf("Expected 2 blocks, got %d", bc.GetBlockCount())
	}

	block1, err := bc.GetBlock(1)
	if err != nil {
		t.Fatalf("Failed to get block 1: %v", err)
	}

	if block1.TaskID != "task-123" {
		t.Errorf("Expected TaskID 'task-123', got '%s'", block1.TaskID)
	}

	if block1.FinalResult != "4" {
		t.Errorf("Expected FinalResult '4', got '%s'", block1.FinalResult)
	}

	if !block1.IsValid {
		t.Error("Expected block to be valid")
	}

	if block1.AgreementRate != 1.0 {
		t.Errorf("Expected AgreementRate 1.0, got %f", block1.AgreementRate)
	}

	// Verify chain integrity
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("Chain verification failed: %v", err)
	}

	// Test invalid block (low agreement rate)
	event2 := &BlockEvent{
		TaskID:           "task-456",
		FinalResult:      "inconsistent",
		IsValid:          true, // Marked as valid but low agreement
		AgreementRate:    0.3,  // Below minimum 0.5
		NodeResults:      map[string]string{"node1": "A", "node2": "B", "node3": "C"},
		ConsensusNodes:   []string{"node1"},
		DisagreeingNodes: []string{"node2", "node3"},
		Timestamp:        time.Now().Unix(),
		BlockHeight:      2,
	}

	// This should fail because agreement rate is below minimum
	err = bc.AddBlockEvent(event2)
	if err == nil {
		t.Error("Expected error for block with low agreement rate, got nil")
	}

	// Verify chain still has only 2 blocks
	if bc.GetBlockCount() != 2 {
		t.Errorf("Expected 2 blocks after failed addition, got %d", bc.GetBlockCount())
	}
}

func TestBlockchain_GetBlocksInRange(t *testing.T) {
	bc, _ := NewBlockchain(nil, nil)
	bc.Start()
	defer bc.Stop()

	// Add multiple blocks
	for i := 1; i <= 5; i++ {
		event := &BlockEvent{
			TaskID:         fmt.Sprintf("task-%d", i),
			FinalResult:    fmt.Sprintf("result-%d", i),
			IsValid:        true,
			AgreementRate:  1.0,
			NodeResults:    map[string]string{"node1": "ok"},
			ConsensusNodes: []string{"node1"},
			Timestamp:      time.Now().Unix(),
			BlockHeight:    uint64(i),
		}
		bc.AddBlockEvent(event)
	}

	// Should have 6 blocks total (genesis + 5)
	if bc.GetBlockCount() != 6 {
		t.Errorf("Expected 6 blocks, got %d", bc.GetBlockCount())
	}

	// Test getting blocks in range
	blocks, err := bc.GetBlocksInRange(1, 3)
	if err != nil {
		t.Fatalf("Failed to get blocks in range: %v", err)
	}

	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(blocks))
	}

	// Verify block heights
	for i, block := range blocks {
		expectedHeight := uint64(1 + i)
		if block.Height != expectedHeight {
			t.Errorf("Expected block %d to have height %d, got %d", i, expectedHeight, block.Height)
		}
	}
}

func TestBlockchain_Callbacks(t *testing.T) {
	bc, _ := NewBlockchain(nil, nil)

	// Track callback invocations
	blockProduced := false

	// Set callbacks
	bc.SetBlockProducedCallback(func(block *Block) {
		blockProduced = true
		if block.TaskID != "callback-test" {
			t.Errorf("Expected TaskID 'callback-test', got '%s'", block.TaskID)
		}
	})

	bc.Start()
	defer bc.Stop()

	// Add a block
	event := &BlockEvent{
		TaskID:         "callback-test",
		FinalResult:    "test",
		IsValid:        true,
		AgreementRate:  1.0,
		NodeResults:    map[string]string{"node1": "test"},
		ConsensusNodes: []string{"node1"},
		Timestamp:      time.Now().Unix(),
		BlockHeight:    1,
	}

	bc.AddBlockEvent(event)

	// Give callbacks time to fire
	time.Sleep(100 * time.Millisecond)

	if !blockProduced {
		t.Error("Block produced callback was not invoked")
	}
}

func TestBlockchain_GetBlockchainInfo(t *testing.T) {
	bc, _ := NewBlockchain(nil, nil)
	bc.Start()
	defer bc.Stop()

	// Genesis block should exist

	info := bc.GetBlockchainInfo()

	if info["running"] != true {
		t.Error("Expected blockchain to be running")
	}

	// Genesis only, so 1 block
	if info["block_count"].(int) != 1 {
		t.Errorf("Expected 1 block (genesis only), got %v", info["block_count"])
	}

	// Genesis is always valid
	if info["valid_blocks"].(int) != 1 {
		t.Errorf("Expected 1 valid block (genesis), got %v", info["valid_blocks"])
	}

	if info["invalid_blocks"].(int) != 0 {
		t.Errorf("Expected 0 invalid blocks, got %v", info["invalid_blocks"])
	}
}
