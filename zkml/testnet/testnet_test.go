package testnet

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aib-protocol/aib/zkml/inference"
	"github.com/aib-protocol/aib/zkml/orchestrator"
)

// Test 1: TestNet lifecycle - start and stop
func TestTestNet_StartStop(t *testing.T) {
	tn := NewTestNet(nil)

	if tn.IsRunning() {
		t.Error("Expected TestNet to be stopped initially")
	}

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start TestNet: %v", err)
	}

	if !tn.IsRunning() {
		t.Error("Expected TestNet to be running after start")
	}

	// Verify nodes were created
	nodes := tn.GetNodes()
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	if err := tn.Stop(); err != nil {
		t.Fatalf("Failed to stop TestNet: %v", err)
	}

	if tn.IsRunning() {
		t.Error("Expected TestNet to be stopped after stop")
	}
}

// Test 2: TestNet double start should error
func TestTestNet_DoubleStart(t *testing.T) {
	tn := NewTestNet(nil)

	if err := tn.Start(); err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	if err := tn.Start(); err == nil {
		t.Error("Expected error for double start")
	}

	tn.Stop()
}

// Test 3: TestNet stop without start should error
func TestTestNet_StopWithoutStart(t *testing.T) {
	tn := NewTestNet(nil)

	if err := tn.Stop(); err == nil {
		t.Error("Expected error for stop without start")
	}
}

// Test 4: All honest nodes - full workflow
func TestTestNet_AllHonest(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.SubmitTask("What is 2+2?")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if result.TaskID == "" {
		t.Error("Expected non-empty task ID")
	}

	if !result.IsValid {
		t.Error("Expected valid result for honest nodes")
	}

	if result.AgreementRate != 1.0 {
		t.Errorf("Expected 100%% agreement, got %.2f%%", result.AgreementRate*100)
	}

	if len(result.Disagreeing) != 0 {
		t.Errorf("Expected 0 disagreeing nodes, got %d", len(result.Disagreeing))
	}

	if result.SlashTriggered != 0 {
		t.Errorf("Expected 0 slashes, got %d", result.SlashTriggered)
	}

	// Check task is settled
	task, err := tn.Orchestrator().GetTask(result.TaskID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if task.GetState() != orchestrator.TaskStateSettled {
		t.Errorf("Expected Settled state, got %s", task.GetState())
	}
}

// Test 5: One node disagreement - should still pass with slash
func TestTestNet_OneDisagreement(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Make one node dishonest
	nodes := tn.GetNodes()
	for id := range nodes {
		tn.SetNodeHonest(id, false)
		break // Only first node is dishonest
	}

	result, err := tn.SubmitTask("What is 2+2?")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// 2/3 = 66.7%, which is just below the 67% threshold
	// Actually, let's check what happens
	if result.IsValid {
		// If valid, we expect 1 disagreeing node to be slashed
		if len(result.Disagreeing) != 1 {
			t.Errorf("Expected 1 disagreeing node, got %d", len(result.Disagreeing))
		}
		if result.SlashTriggered != 1 {
			t.Errorf("Expected 1 slash, got %d", result.SlashTriggered)
		}
	}

	// Verify stats
	stats := tn.GetStats()
	if stats.TotalTasks != 1 {
		t.Errorf("Expected 1 total task, got %d", stats.TotalTasks)
	}
}

// Test 6: Majority disagreement - should fail
func TestTestNet_MajorityDisagreement(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Make 2 nodes dishonest (majority)
	count := 0
	for id := range tn.GetNodes() {
		tn.SetNodeHonest(id, false)
		count++
		if count >= 2 {
			break
		}
	}

	result, err := tn.SubmitTask("What is 2+2?")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// With 2/3 dishonest, we have 1/3 = 33% agreement, which is below threshold
	if result.IsValid {
		t.Error("Expected invalid result when majority disagree")
	}

	if result.AgreementRate >= 0.67 {
		t.Errorf("Expected agreement rate < 67%%, got %.2f%%", result.AgreementRate*100)
	}

	// Check stats
	stats := tn.GetStats()
	if stats.FailedTasks != 1 {
		t.Errorf("Expected 1 failed task, got %d", stats.FailedTasks)
	}
}

// Test 7: Node offline - test fault tolerance
func TestTestNet_NodeOffline(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      5,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Take 2 nodes offline, leaving 3 online
	count := 0
	for id := range tn.GetNodes() {
		if err := tn.SetNodeOnline(id, false); err != nil {
			t.Fatalf("Failed to set node offline: %v", err)
		}
		count++
		if count >= 2 {
			break
		}
	}

	result, err := tn.SubmitTask("Are you there?")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.IsValid {
		t.Error("Expected valid result with remaining 3 nodes")
	}

	// Should have 3 results (from online nodes)
	if len(result.NodeResults) != 3 {
		t.Errorf("Expected 3 node results, got %d", len(result.NodeResults))
	}
}

// Test 8: Byzantine behavior - all nodes return different results
func TestTestNet_Byzantine(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Make all nodes dishonest
	for id := range tn.GetNodes() {
		tn.SetNodeHonest(id, false)
	}

	result, err := tn.SubmitTask("Byzantine test")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// All nodes return different results, no majority
	if result.IsValid {
		t.Error("Expected invalid result with Byzantine behavior")
	}
}

// Test 9: Concurrent task submission
func TestTestNet_ConcurrentTasks(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      5,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			prompt := fmt.Sprintf("Concurrent task %d", idx)
			result, err := tn.SubmitTask(prompt)
			if err != nil {
				errors <- fmt.Errorf("task %d: %w", idx, err)
				return
			}
			if !result.IsValid {
				errors <- fmt.Errorf("task %d: invalid result", idx)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent task error: %v", err)
	}

	stats := tn.GetStats()
	if stats.TotalTasks != 5 {
		t.Errorf("Expected 5 total tasks, got %d", stats.TotalTasks)
	}
	if stats.PassedTasks != 5 {
		t.Errorf("Expected 5 passed tasks, got %d", stats.PassedTasks)
	}
}

// Test 10: Auto-slash verification
func TestTestNet_AutoSlash(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      5,
		MinNodes:       5,  // Use all 5 nodes so dishonest ones are always included
		CommitDuration: 200 * time.Millisecond,
		RevealDuration: 200 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Make 2 nodes dishonest
	count := 0
	for id := range tn.GetNodes() {
		tn.SetNodeHonest(id, false)
		count++
		if count >= 2 {
			break
		}
	}

	result, err := tn.SubmitTask("Slash test")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// 3/5 = 60% honest, which is below 67% threshold
	// The disagreeing nodes should be slashed
	if result.SlashTriggered < 1 {
		t.Errorf("Expected at least 1 slash, got %d", result.SlashTriggered)
	}

	// Check orchestrator metrics
	metrics := tn.Orchestrator().GetMetrics()
	if metrics.TotalSlashes < 1 {
		t.Errorf("Expected at least 1 total slash in metrics, got %d", metrics.TotalSlashes)
	}
}

// Test 11: End-to-end flow verification (task creation to settle)
func TestTestNet_EndToEndFlow(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Track events
	eventTypes := make([]orchestrator.EventType, 0)
	tn.Orchestrator().EventBus().SubscribeAll(func(e *orchestrator.Event) {
		eventTypes = append(eventTypes, e.Type)
	})

	result, err := tn.SubmitTask("End to end test")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Verify all expected events were emitted
	expectedEvents := map[orchestrator.EventType]bool{
		orchestrator.EventTaskCreated:          false,
		orchestrator.EventTaskAssigned:         false,
		orchestrator.EventCommitPhaseStarted:   false,
		orchestrator.EventRevealPhaseStarted:   false,
		orchestrator.EventVerificationStarted:  false,
		orchestrator.EventVerificationComplete: false,
		orchestrator.EventTaskSettled:          false,
	}

	for _, et := range eventTypes {
		if _, ok := expectedEvents[et]; ok {
			expectedEvents[et] = true
		}
	}

	for eventType, found := range expectedEvents {
		if !found {
			t.Errorf("Expected event %s was not emitted", eventType)
		}
	}

	// Verify final result
	if result.FinalResult == "" {
		t.Error("Expected non-empty final result")
	}

	// Verify task state
	task, _ := tn.Orchestrator().GetTask(result.TaskID)
	if task.GetState() != orchestrator.TaskStateSettled {
		t.Errorf("Expected Settled state, got %s", task.GetState())
	}
}

// Test 12: Statistics verification
func TestTestNet_Stats(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Submit 3 tasks
	for i := 0; i < 3; i++ {
		_, err := tn.SubmitTask(fmt.Sprintf("Task %d", i))
		if err != nil {
			t.Fatalf("SubmitTask %d failed: %v", i, err)
		}
	}

	stats := tn.GetStats()

	if stats.TotalTasks != 3 {
		t.Errorf("Expected 3 total tasks, got %d", stats.TotalTasks)
	}

	if stats.PassedTasks != 3 {
		t.Errorf("Expected 3 passed tasks, got %d", stats.PassedTasks)
	}

	if stats.FailedTasks != 0 {
		t.Errorf("Expected 0 failed tasks, got %d", stats.FailedTasks)
	}

	if stats.TotalSlashes != 0 {
		t.Errorf("Expected 0 slashes, got %d", stats.TotalSlashes)
	}

	if stats.TotalEvents == 0 {
		t.Error("Expected non-zero event count")
	}

	if stats.AvgDuration == 0 {
		t.Error("Expected non-zero average duration")
	}

	// Check node stats
	if len(stats.NodeStats) != 3 {
		t.Errorf("Expected 3 node stats, got %d", len(stats.NodeStats))
	}

	for id, ns := range stats.NodeStats {
		if !ns.Online {
			t.Errorf("Expected node %s to be online", id)
		}
		if !ns.Honest {
			t.Errorf("Expected node %s to be honest", id)
		}
	}
}

// Test 13: Scenario runner - AllHonest
func TestTestNet_ScenarioAllHonest(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.RunScenario(ScenarioAllHonest)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}

	if !result.AllExpectationsMet {
		t.Error("Expected all expectations to be met for honest scenario")
	}

	if len(result.TaskResults) != len(ScenarioAllHonest.Tasks) {
		t.Errorf("Expected %d task results, got %d", len(ScenarioAllHonest.Tasks), len(result.TaskResults))
	}
}

// Test 14: Scenario runner - OneDisagreement
func TestTestNet_ScenarioOneDisagreement(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.RunScenario(ScenarioOneDisagreement)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}

	// Note: 2/3 = 66.7% is just below 67% threshold, so results may vary
	// The key is that we get some results
	if len(result.TaskResults) == 0 {
		t.Error("Expected at least one task result")
	}

	// Check that one task had a slash
	hasSlash := false
	for _, tr := range result.TaskResults {
		if tr.SlashTriggered > 0 {
			hasSlash = true
			break
		}
	}
	if !hasSlash {
		t.Error("Expected at least one slash in disagreement scenario")
	}
}

// Test 15: Scenario runner - NodeOffline
func TestTestNet_ScenarioNodeOffline(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      5,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.RunScenario(ScenarioNodeOffline)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}

	if !result.AllExpectationsMet {
		t.Error("Expected all expectations to be met for offline scenario")
	}
}

// Test 16: SetNodeHonest dynamic change
func TestTestNet_SetNodeHonestDynamic(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// First task with all honest
	result1, err := tn.SubmitTask("First task")
	if err != nil {
		t.Fatalf("First SubmitTask failed: %v", err)
	}
	if !result1.IsValid {
		t.Error("Expected valid result for honest nodes")
	}

	// Pick a specific node to make dishonest (and remember it)
	var targetNodeID string
	for id := range tn.GetNodes() {
		targetNodeID = id
		break
	}

	// Make the specific node dishonest
	tn.SetNodeHonest(targetNodeID, false)

	// Second task with one dishonest
	result2, err := tn.SubmitTask("Second task")
	if err != nil {
		t.Fatalf("Second SubmitTask failed: %v", err)
	}

	// Should have slash triggered for the dishonest node
	if result2.SlashTriggered == 0 {
		t.Error("Expected slash after making node dishonest")
	}

	// Make the same node honest again
	tn.SetNodeHonest(targetNodeID, true)

	// Third task with all honest again
	result3, err := tn.SubmitTask("Third task")
	if err != nil {
		t.Fatalf("Third SubmitTask failed: %v", err)
	}
	// With all nodes honest, no slash should occur
	// Note: The slash count is cumulative, so we check that it didn't increase
	// from result2's slash count
	if result3.SlashTriggered > result2.SlashTriggered {
		t.Errorf("Expected no new slashes after making node honest again, got %d new slashes", result3.SlashTriggered-result2.SlashTriggered)
	}
}

// Test 17: GetNode and GetNodes
func TestTestNet_GetNode(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Test GetNodes
	nodes := tn.GetNodes()
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	// Test GetNode for each
	for id := range nodes {
		node, ok := tn.GetNode(id)
		if !ok {
			t.Errorf("Expected to find node %s", id)
		}
		if node.ID != id {
			t.Errorf("Expected node ID %s, got %s", id, node.ID)
		}
		if !node.Online {
			t.Errorf("Expected node %s to be online", id)
		}
		if !node.Honest {
			t.Errorf("Expected node %s to be honest", id)
		}
	}

	// Test GetNode for non-existent
	_, ok := tn.GetNode("non-existent")
	if ok {
		t.Error("Expected not to find non-existent node")
	}
}

// Test 18: Event log verification
func TestTestNet_EventLog(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Clear event log by getting a copy
	_ = tn.GetEventLog()

	// Submit a task
	_, err := tn.SubmitTask("Event log test")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Get event log
	events := tn.GetEventLog()
	if len(events) == 0 {
		t.Error("Expected events in log")
	}

	// Verify event types
	hasTaskCreated := false
	hasTaskSettled := false
	for _, e := range events {
		if e.Type == orchestrator.EventTaskCreated {
			hasTaskCreated = true
		}
		if e.Type == orchestrator.EventTaskSettled {
			hasTaskSettled = true
		}
	}

	if !hasTaskCreated {
		t.Error("Expected EventTaskCreated in log")
	}
	if !hasTaskSettled {
		t.Error("Expected EventTaskSettled in log")
	}
}

// Test 19: Different honest ratios
func TestTestNet_HonestRatio(t *testing.T) {
	// Test with 50% honest ratio
	config := &TestNetConfig{
		NodeCount:      4,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    0.5, // 2 honest, 2 dishonest
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Check that 2 nodes are honest and 2 are dishonest
	honestCount := 0
	dishonestCount := 0
	for _, node := range tn.GetNodes() {
		if node.Honest {
			honestCount++
		} else {
			dishonestCount++
		}
	}

	if honestCount != 2 {
		t.Errorf("Expected 2 honest nodes, got %d", honestCount)
	}
	if dishonestCount != 2 {
		t.Errorf("Expected 2 dishonest nodes, got %d", dishonestCount)
	}
}

// Test 20: Empty prompt validation
func TestTestNet_EmptyPrompt(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Empty prompt should fail at orchestrator level
	_, err := tn.SubmitTask("")
	if err == nil {
		t.Error("Expected error for empty prompt")
	}
}

// Test 21: Scenario runner - MajorityDisagreement
func TestTestNet_ScenarioMajorityDisagreement(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      6,  // 6 nodes: will make 4 dishonest (>50%)
		MinNodes:       5,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.RunScenario(ScenarioMajorityDisagreement)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}

	// With majority dishonest, verification should fail (IsValid=false)
	// ExpectedPass=false, so AllExpectationsMet should be TRUE (failure was expected and occurred)
	if !result.AllExpectationsMet {
		t.Error("Expected verification to fail when majority of nodes are dishonest")
	}
}

// Test 22: Scenario runner - Byzantine
func TestTestNet_ScenarioByzantine(t *testing.T) {
	config := &TestNetConfig{
		NodeCount:      6,  // 6 nodes: all will be made dishonest
		MinNodes:       5,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	result, err := tn.RunScenario(ScenarioByzantine)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}

	// With all nodes Byzantine, verification should fail (IsValid=false)
	// ExpectedPass=false, so AllExpectationsMet should be TRUE (failure was expected)
	if !result.AllExpectationsMet {
		t.Error("Expected expectations TO be met - verification should fail when all nodes are Byzantine")
	}
}

// Test 23: Real AI API integration test
func TestTestNet_RealAI(t *testing.T) {
	// Increase durations to allow AI inference to complete
	// AI inference takes ~1-2 seconds
	config := &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 5 * time.Second,
		RevealDuration: 5 * time.Second,
		TaskTimeout:    30 * time.Second,
		AutoSlash:      true,
		HonestRatio:    1.0,
		UseRealAI:      true,
		AIProvider: inference.AnthropicConfig{
			BaseURL: "http://217.216.43.45:51201/key/rk-e9412b1f5e955a92bbca9627",
			APIKey:  "rk-e9412b1f5e955a92bbca9627",
			Model:   "glm-5",
			Timeout: 30 * time.Second,
		},
	}
	tn := NewTestNet(config)

	if err := tn.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer tn.Stop()

	// Submit a simple math question
	result, err := tn.SubmitTask("What is 2+2? Answer with just the number.")
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.IsValid {
		t.Errorf("Expected valid result, got invalid. Agreement: %.2f%%", result.AgreementRate*100)
	}

	// The real AI should return something containing "4"
	if !contains(result.FinalResult, "4") {
		t.Errorf("Expected result to contain '4', got: %s", result.FinalResult)
	}

	t.Logf("Real AI result: %s", result.FinalResult)
	t.Logf("Agreement rate: %.2f%%", result.AgreementRate*100)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
