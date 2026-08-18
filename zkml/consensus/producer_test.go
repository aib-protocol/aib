package consensus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockProducer_NewBlockProducer(t *testing.T) {
	store := NewMemoryStorage()
	bc, err := NewBlockchain(store, DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	bp := NewBlockProducer(bc, DefaultConfig())

	if bp == nil {
		t.Fatal("expected non-nil BlockProducer")
	}

	if bp.IsRunning() {
		t.Error("expected producer to be stopped initially")
	}
}

func TestBlockProducer_StartStop(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.AutoProduce = false // Disable auto-produce for testing
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	bp := NewBlockProducer(bc, config)

	bp.Start()
	time.Sleep(50 * time.Millisecond)

	if !bp.IsRunning() {
		t.Error("expected producer to be running after Start()")
	}

	// Double start should be safe
	bp.Start()
	if !bp.IsRunning() {
		t.Error("expected producer to still be running after double Start()")
	}

	bp.Stop()
	time.Sleep(50 * time.Millisecond)

	if bp.IsRunning() {
		t.Error("expected producer to be stopped after Stop()")
	}

	// Double stop should be safe
	bp.Stop()
	if bp.IsRunning() {
		t.Error("expected producer to still be stopped after double Stop()")
	}
}

func TestBlockProducer_GetStats(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	bp := NewBlockProducer(bc, config)
	stats := bp.GetStats()

	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	running, ok := stats["running"].(bool)
	if !ok || running {
		t.Error("expected running to be false")
	}

	interval, ok := stats["block_interval"].(string)
	if !ok || interval == "" {
		t.Error("expected block_interval in stats")
	}

	autoProduce, ok := stats["auto_produce"].(bool)
	if !ok || !autoProduce {
		t.Error("expected auto_produce to be true for default config")
	}
}

func TestBlockProducer_AutoProduceDisabled(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.AutoProduce = false

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	bp := NewBlockProducer(bc, config)

	bp.Start()
	defer bp.Stop()

	// Start the blockchain to create genesis block
	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()

	// Wait for potential block production
	time.Sleep(150 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	if finalCount > initialCount {
		t.Error("expected no blocks to be produced when AutoProduce is disabled")
	}
}

func TestMonitor_NewMonitor(t *testing.T) {
	store := NewMemoryStorage()
	bc, err := NewBlockchain(store, DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, DefaultConfig())

	if monitor == nil {
		t.Fatal("expected non-nil Monitor")
	}

	if monitor.IsRunning() {
		t.Error("expected monitor to be stopped initially")
	}
}

func TestMonitor_StartStop(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, config)

	monitor.Start()
	time.Sleep(50 * time.Millisecond)

	if !monitor.IsRunning() {
		t.Error("expected monitor to be running after Start()")
	}

	// Double start should be safe
	monitor.Start()
	if !monitor.IsRunning() {
		t.Error("expected monitor to still be running after double Start()")
	}

	monitor.Stop()
	time.Sleep(50 * time.Millisecond)

	if monitor.IsRunning() {
		t.Error("expected monitor to be stopped after Stop()")
	}

	// Double stop should be safe
	monitor.Stop()
	if monitor.IsRunning() {
		t.Error("expected monitor to still be stopped after double Stop()")
	}
}

func TestMonitor_GetStats(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, config)
	stats := monitor.GetStats()

	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	running, ok := stats["running"].(bool)
	if !ok || running {
		t.Error("expected running to be false")
	}

	minAgreement, ok := stats["min_agreement_rate"].(float64)
	if !ok || minAgreement == 0 {
		t.Error("expected min_agreement_rate in stats")
	}
}

func TestMonitor_HandleEvent(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.5

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	monitor := NewMonitor(bc, config)
	monitor.Start()
	defer monitor.Stop()

	initialCount := bc.GetBlockCount()

	// Send a valid event
	event := &BlockEvent{
		TaskID:        "test-task-1",
		FinalResult:   "test result",
		IsValid:       true,
		AgreementRate: 0.8,
		NodeResults: map[string]string{
			"node1": "result1",
			"node2": "result2",
		},
		ConsensusNodes: []string{"node1", "node2"},
		Metadata:       map[string]string{"test": "data"},
		Timestamp:      time.Now().Unix(),
		BlockHeight:    initialCount,
	}

	// Send event through blockchain's event channel
	bc.SendEvent(event)

	// Give time for monitor to process
	time.Sleep(100 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	if finalCount <= initialCount {
		t.Error("expected block count to increase after valid event")
	}
}

func TestMonitor_RejectLowAgreement(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.7 // High threshold

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	monitor := NewMonitor(bc, config)
	monitor.Start()
	defer monitor.Stop()

	initialCount := bc.GetBlockCount()

	// Send event with low agreement rate
	event := &BlockEvent{
		TaskID:        "test-task-low",
		FinalResult:   "test result",
		IsValid:       true,
		AgreementRate: 0.5, // Below 0.7 threshold
		NodeResults: map[string]string{
			"node1": "result1",
		},
		ConsensusNodes: []string{"node1"},
		Timestamp:      time.Now().Unix(),
		BlockHeight:    initialCount,
	}

	bc.SendEvent(event)

	// Give time for monitor to process
	time.Sleep(100 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Block count should not increase for low agreement events
	if finalCount > initialCount {
		t.Error("expected block count to stay same for low agreement event")
	}
}

func TestMonitor_HandleNilEvent(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, config)
	monitor.Start()
	defer monitor.Stop()

	// This should not panic
	monitor.handleEvent(nil)
}

func TestMonitor_HandleEmptyTaskID(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, config)
	monitor.Start()
	defer monitor.Stop()

	initialCount := bc.GetBlockCount()

	event := &BlockEvent{
		TaskID:        "", // Empty task ID
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 1.0,
		Timestamp:     time.Now().Unix(),
	}

	bc.SendEvent(event)

	time.Sleep(100 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Should not create a block for empty task ID
	if finalCount > initialCount {
		t.Error("expected no block to be created for empty task ID")
	}
}

func TestBlockProducer_ConcurrentStartStop(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	bp := NewBlockProducer(bc, config)

	done := make(chan bool)

	// Concurrent starts
	for i := 0; i < 10; i++ {
		go func() {
			bp.Start()
			done <- true
		}()
	}

	// Wait for all starts
	for i := 0; i < 10; i++ {
		<-done
	}

	if !bp.IsRunning() {
		t.Error("expected producer to be running after concurrent starts")
	}

	// Concurrent stops
	for i := 0; i < 10; i++ {
		go func() {
			bp.Stop()
			done <- true
		}()
	}

	// Wait for all stops
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should eventually stop
	time.Sleep(100 * time.Millisecond)
	if bp.IsRunning() {
		t.Error("expected producer to be stopped after concurrent stops")
	}
}

func TestMonitor_ConcurrentStartStop(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	monitor := NewMonitor(bc, config)

	done := make(chan bool)

	// Concurrent starts
	for i := 0; i < 10; i++ {
		go func() {
			monitor.Start()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if !monitor.IsRunning() {
		t.Error("expected monitor to be running after concurrent starts")
	}

	// Concurrent stops
	for i := 0; i < 10; i++ {
		go func() {
			monitor.Stop()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	time.Sleep(100 * time.Millisecond)
	if monitor.IsRunning() {
		t.Error("expected monitor to be stopped after concurrent stops")
	}
}

// ============================================================================
// block production order tests
// ============================================================================

// TestBlockGenerationOrder tests that blocks are generated in correct order
func TestBlockGenerationOrder(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.AutoProduce = false // Disable auto-produce for controlled testing

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Add multiple blocks with explicit events
	initialCount := bc.GetBlockCount()

	// Create blocks sequentially
	for i := 0; i < 5; i++ {
		event := &BlockEvent{
			TaskID:        "task-order-test",
			FinalResult:   "result",
			IsValid:       true,
			AgreementRate: 1.0,
			NodeResults: map[string]string{
				"node1": "result1",
			},
			ConsensusNodes: []string{"node1"},
			Timestamp:      time.Now().Unix(),
			BlockHeight:    initialCount + uint64(i),
		}

		if err := bc.AddBlockEvent(event); err != nil {
			t.Fatalf("failed to add block event %d: %v", i, err)
		}

		// Small delay to ensure order
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for all blocks to be added
	time.Sleep(100 * time.Millisecond)

	finalCount := bc.GetBlockCount()
	expectedCount := initialCount + 5

	if finalCount != expectedCount {
		t.Errorf("expected %d blocks, got %d", expectedCount, finalCount)
	}

	// Verify block chain order
	blocks, err := bc.GetBlocksInRange(0, finalCount-1)
	if err != nil {
		t.Fatalf("failed to get blocks: %v", err)
	}

	// Verify each block's previous hash links correctly
	for i := 1; i < len(blocks); i++ {
		current := blocks[i]
		previous := blocks[i-1]

		if current.Height != previous.Height+1 {
			t.Errorf("block height not sequential: height %d, previous %d",
				current.Height, previous.Height)
		}

		// Verify previous hash links correctly
		if string(current.PreviousBlockHash()) != string(previous.Hash()) {
			t.Errorf("block %d previous hash does not match block %d hash", current.Height, previous.Height)
		}
	}
}

// TestBlockHashChainVerification tests that the block hash chain is valid
func TestBlockHashChainVerification(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Add some blocks
	for i := 0; i < 3; i++ {
		event := &BlockEvent{
			TaskID:         "chain-test",
			FinalResult:    "result",
			IsValid:        true,
			AgreementRate:  1.0,
			NodeResults:    map[string]string{"node": "result"},
			ConsensusNodes: []string{"node"},
			Timestamp:      time.Now().Unix(),
		}

		if err := bc.AddBlockEvent(event); err != nil {
			t.Fatalf("failed to add block: %v", err)
		}
	}

	// Verify the entire chain
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed: %v", err)
	}
}

// ============================================================================
// consensus node selection tests
// ============================================================================

// TestConsensusNodeSelection tests that consensus nodes are correctly selected
func TestConsensusNodeSelection(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Test case 1: Single node consensus
	event1 := &BlockEvent{
		TaskID:        "single-node",
		FinalResult:   "result1",
		IsValid:       true,
		AgreementRate: 1.0,
		NodeResults: map[string]string{
			"node1": "result",
		},
		ConsensusNodes: []string{"node1"},
		Timestamp:      time.Now().Unix(),
	}

	if err := bc.AddBlockEvent(event1); err != nil {
		t.Fatalf("failed to add block: %v", err)
	}

	// Test case 2: Multiple nodes consensus
	event2 := &BlockEvent{
		TaskID:        "multi-node",
		FinalResult:   "result2",
		IsValid:       true,
		AgreementRate: 0.8,
		NodeResults: map[string]string{
			"node1": "result",
			"node2": "result",
			"node3": "result",
		},
		ConsensusNodes: []string{"node1", "node2", "node3"},
		Timestamp:      time.Now().Unix(),
	}

	if err := bc.AddBlockEvent(event2); err != nil {
		t.Fatalf("failed to add block: %v", err)
	}

	// Verify blocks contain correct consensus nodes
	block1, err := bc.GetBlock(1)
	if err != nil {
		t.Fatalf("failed to get block 1: %v", err)
	}

	if len(block1.ConsensusNodes) != 1 || block1.ConsensusNodes[0] != "node1" {
		t.Errorf("expected consensus nodes [node1], got %v", block1.ConsensusNodes)
	}

	block2, err := bc.GetBlock(2)
	if err != nil {
		t.Fatalf("failed to get block 2: %v", err)
	}

	if len(block2.ConsensusNodes) != 3 {
		t.Errorf("expected 3 consensus nodes, got %d", len(block2.ConsensusNodes))
	}

	// Verify agreement rate
	if block2.AgreementRate != 0.8 {
		t.Errorf("expected agreement rate 0.8, got %f", block2.AgreementRate)
	}
}

// TestConsensusNodesWithDisagreement tests consensus with disagreeing nodes
func TestConsensusNodesWithDisagreement(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.5

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Create event with disagreeing nodes (2 out of 4 agree)
	event := &BlockEvent{
		TaskID:        "disagreement-test",
		FinalResult:   "agreed-result",
		IsValid:       true,
		AgreementRate: 0.5, // 50% agreement
		NodeResults: map[string]string{
			"node1": "result-a",
			"node2": "result-a",
			"node3": "result-b",
			"node4": "result-b",
		},
		ConsensusNodes:   []string{"node1", "node2"},
		DisagreeingNodes: []string{"node3", "node4"},
		Timestamp:        time.Now().Unix(),
	}

	if err := bc.AddBlockEvent(event); err != nil {
		t.Fatalf("failed to add block: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	block, err := bc.GetBlock(1)
	if err != nil {
		t.Fatalf("failed to get block: %v", err)
	}

	if len(block.ConsensusNodes) != 2 {
		t.Errorf("expected 2 consensus nodes, got %d", len(block.ConsensusNodes))
	}

	if len(block.DisagreeingNodes) != 2 {
		t.Errorf("expected 2 disagreeing nodes, got %d", len(block.DisagreeingNodes))
	}
}

// ============================================================================
// event processing order tests
// ============================================================================

// TestEventProcessingOrder tests that events are processed in order
func TestEventProcessingOrder(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.5

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()

	// Send multiple events in order
	events := make([]*BlockEvent, 5)
	for i := 0; i < 5; i++ {
		events[i] = &BlockEvent{
			TaskID:         "task-order",
			FinalResult:    "result",
			IsValid:        true,
			AgreementRate:  1.0,
			NodeResults:    map[string]string{"node": "result"},
			ConsensusNodes: []string{"node"},
			Timestamp:      time.Now().Unix(),
		}

		if err := bc.SendEvent(events[i]); err != nil {
			t.Fatalf("failed to send event: %v", err)
		}
	}

	// Wait for all events to be processed
	time.Sleep(200 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// All 5 events should result in blocks (genesis + 5 = 6)
	if finalCount != initialCount+5 {
		t.Errorf("expected %d blocks, got %d", initialCount+5, finalCount)
	}

	// Verify blocks are in order by checking heights
	for i := 1; i <= 5; i++ {
		block, err := bc.GetBlock(uint64(i))
		if err != nil {
			t.Errorf("failed to get block at height %d: %v", i, err)
		}
		if block.Height != uint64(i) {
			t.Errorf("expected block height %d, got %d", i, block.Height)
		}
	}
}

// TestEventProcessingWithDifferentAgreements tests events with different agreement rates
func TestEventProcessingWithDifferentAgreements(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.7

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()

	// Event 1: High agreement (should be accepted)
	event1 := &BlockEvent{
		TaskID:         "high-agreement",
		FinalResult:    "result",
		IsValid:        true,
		AgreementRate:  0.9,
		NodeResults:    map[string]string{"node": "result"},
		ConsensusNodes: []string{"node"},
		Timestamp:      time.Now().Unix(),
	}

	// Event 2: Low agreement (should be rejected)
	event2 := &BlockEvent{
		TaskID:         "low-agreement",
		FinalResult:    "result",
		IsValid:        true,
		AgreementRate:  0.5,
		NodeResults:    map[string]string{"node": "result"},
		ConsensusNodes: []string{"node"},
		Timestamp:      time.Now().Unix(),
	}

	// Event 3: Another high agreement (should be accepted)
	event3 := &BlockEvent{
		TaskID:         "high-agreement-2",
		FinalResult:    "result",
		IsValid:        true,
		AgreementRate:  1.0,
		NodeResults:    map[string]string{"node": "result"},
		ConsensusNodes: []string{"node"},
		Timestamp:      time.Now().Unix(),
	}

	bc.SendEvent(event1)
	bc.SendEvent(event2)
	bc.SendEvent(event3)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Only 2 events should be accepted (genesis + 2 = 3)
	expectedCount := initialCount + 2
	if finalCount != expectedCount {
		t.Errorf("expected %d blocks (2 accepted), got %d", expectedCount, finalCount)
	}
}

// ============================================================================
// concurrency safety tests
// ============================================================================

// TestBlockchainConcurrentBlockAdd tests concurrent block additions
func TestBlockchainConcurrentBlockAdd(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()
	numGoroutines := 20
	blocksPerGoroutine := 5

	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < blocksPerGoroutine; j++ {
				event := &BlockEvent{
					TaskID:         "concurrent-test",
					FinalResult:    "result",
					IsValid:        true,
					AgreementRate:  1.0,
					NodeResults:    map[string]string{"node": "result"},
					ConsensusNodes: []string{"node"},
					Timestamp:      time.Now().Unix(),
				}

				if err := bc.AddBlockEvent(event); err == nil {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Verify chain is still valid
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed after concurrent adds: %v", err)
	}

	// Note: Due to potential height conflicts, not all blocks may be added
	// The important thing is the chain remains consistent
	_ = successCount
	_ = finalCount

	// Verify the chain has at least some blocks
	if finalCount <= initialCount {
		t.Error("expected at least some blocks to be added")
	}
}

// TestBlockchainConcurrentEventSend tests concurrent event sending
func TestBlockchainConcurrentEventSend(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.5

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()
	numGoroutines := 10

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				event := &BlockEvent{
					TaskID:         "concurrent-event",
					FinalResult:    "result",
					IsValid:        true,
					AgreementRate:  1.0,
					NodeResults:    map[string]string{"node": "result"},
					ConsensusNodes: []string{"node"},
					Timestamp:      time.Now().Unix(),
				}

				bc.SendEvent(event)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all events to be processed
	time.Sleep(300 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Verify chain is valid
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed after concurrent events: %v", err)
	}

	// At least some events should have been processed
	if finalCount <= initialCount {
		t.Error("expected at least some events to be processed into blocks")
	}
}

// TestMonitorConcurrentEventHandle tests concurrent event handling by monitor
func TestMonitorConcurrentEventHandle(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.MinAgreementRate = 0.5

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	monitor := NewMonitor(bc, config)
	monitor.Start()
	defer monitor.Stop()

	initialCount := bc.GetBlockCount()
	numEvents := 50

	// Send many events concurrently
	var wg sync.WaitGroup

	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			event := &BlockEvent{
				TaskID:         "concurrent-monitor",
				FinalResult:    "result",
				IsValid:        true,
				AgreementRate:  1.0,
				NodeResults:    map[string]string{"node": "result"},
				ConsensusNodes: []string{"node"},
				Timestamp:      time.Now().Unix(),
			}

			bc.SendEvent(event)
		}(i)
	}

	wg.Wait()

	// Wait for monitor to process all events
	time.Sleep(500 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Verify chain integrity
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed after concurrent monitor handling: %v", err)
	}

	// Most events should have been processed (accounting for channel capacity)
	if finalCount < initialCount+10 {
		t.Errorf("expected most events to be processed, got %d new blocks from %d events",
			finalCount-initialCount, numEvents)
	}
}

// TestProducerConcurrentBlockProduction tests concurrent block production
func TestProducerConcurrentBlockProduction(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.BlockInterval = 50 * time.Millisecond // Fast interval for testing
	config.AutoProduce = true

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	initialCount := bc.GetBlockCount()

	// Wait for auto-production to generate blocks
	time.Sleep(300 * time.Millisecond)

	finalCount := bc.GetBlockCount()

	// Should have produced at least one block
	if finalCount <= initialCount {
		t.Error("expected auto-producer to generate blocks")
	}

	// Verify chain is valid
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed: %v", err)
	}
}

// TestMultipleProducersAndMonitors tests multiple producers and monitors
func TestMultipleProducersAndMonitors(t *testing.T) {
	store := NewMemoryStorage()
	config := DefaultConfig()
	config.AutoProduce = false

	bc, err := NewBlockchain(store, config)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	if err := bc.Start(); err != nil {
		t.Fatalf("failed to start blockchain: %v", err)
	}
	defer bc.Stop()

	// Create additional producer and monitor
	producer2 := NewBlockProducer(bc, config)
	monitor2 := NewMonitor(bc, config)

	producer2.Start()
	monitor2.Start()

	defer producer2.Stop()
	defer monitor2.Stop()

	// Add blocks through different components
	for i := 0; i < 5; i++ {
		event := &BlockEvent{
			TaskID:         "multi-component",
			FinalResult:    "result",
			IsValid:        true,
			AgreementRate:  1.0,
			NodeResults:    map[string]string{"node": "result"},
			ConsensusNodes: []string{"node"},
			Timestamp:      time.Now().Unix(),
		}

		bc.SendEvent(event)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify chain is valid
	if err := bc.VerifyChain(); err != nil {
		t.Errorf("chain verification failed with multiple components: %v", err)
	}
}
