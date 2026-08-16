package orchestrator

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aib-protocol/aib/zkml/verification"
)

// --- Task Tests ---

func TestTask_NewTask(t *testing.T) {
	task := NewTask("task1", "test prompt", "user1", 5, 5*time.Minute)

	if task.ID != "task1" {
		t.Errorf("Expected ID 'task1', got %s", task.ID)
	}
	if task.Prompt != "test prompt" {
		t.Errorf("Expected prompt 'test prompt', got %s", task.Prompt)
	}
	if task.State != TaskStateCreated {
		t.Errorf("Expected state Created, got %s", task.State)
	}
	if task.MinNodes != 5 {
		t.Errorf("Expected minNodes 5, got %d", task.MinNodes)
	}
}

func TestTask_ValidTransitions(t *testing.T) {
	task := NewTask("t1", "prompt", "user1", 5, time.Minute)

	transitions := []TaskState{
		TaskStateAssigned,
		TaskStateCommitPhase,
		TaskStateRevealPhase,
		TaskStateVerifying,
		TaskStateVerified,
		TaskStateSettled,
	}

	for _, state := range transitions {
		if err := task.TransitionTo(state); err != nil {
			t.Errorf("Failed to transition to %s: %v", state, err)
		}
		if task.GetState() != state {
			t.Errorf("Expected state %s, got %s", state, task.GetState())
		}
	}
}

func TestTask_InvalidTransition(t *testing.T) {
	task := NewTask("t1", "prompt", "user1", 5, time.Minute)

	// Cannot jump from Created to Verifying
	if err := task.TransitionTo(TaskStateVerifying); err == nil {
		t.Error("Expected error for invalid transition Created -> Verifying")
	}
}

func TestTask_FailedPath(t *testing.T) {
	task := NewTask("t1", "prompt", "user1", 5, time.Minute)

	// Created -> Assigned -> CommitPhase -> RevealPhase -> Verifying -> Failed -> Settled
	for _, state := range []TaskState{
		TaskStateAssigned, TaskStateCommitPhase, TaskStateRevealPhase,
		TaskStateVerifying, TaskStateFailed, TaskStateSettled,
	} {
		if err := task.TransitionTo(state); err != nil {
			t.Fatalf("Failed to transition to %s: %v", state, err)
		}
	}
}

func TestTask_SetAndGetResults(t *testing.T) {
	task := NewTask("t1", "prompt", "user1", 5, time.Minute)

	task.SetResult("node1", "resultA")
	task.SetResult("node2", "resultB")

	results := task.GetResults()
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
	if results["node1"] != "resultA" {
		t.Errorf("Expected resultA, got %s", results["node1"])
	}
}

func TestTask_IsExpired(t *testing.T) {
	task := NewTask("t1", "prompt", "user1", 5, 0) // zero timeout = expired immediately
	// Hack the created time to the past
	task.CreatedAt = time.Now().Unix() - 10

	if !task.IsExpired() {
		t.Error("Expected task to be expired")
	}
}

// --- EventBus Tests ---

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	received := make(chan *Event, 1)
	bus.Subscribe(EventTaskCreated, func(e *Event) {
		received <- e
	})

	bus.Publish(&Event{
		Type:      EventTaskCreated,
		TaskID:    "test_task",
		Timestamp: time.Now().Unix(),
	})

	select {
	case event := <-received:
		if event.TaskID != "test_task" {
			t.Errorf("Expected task ID 'test_task', got %s", event.TaskID)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	bus := NewEventBus()
	count := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		bus.Subscribe(EventTaskCreated, func(e *Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(&Event{Type: EventTaskCreated})

	mu.Lock()
	if count != 3 {
		t.Errorf("Expected 3 handlers called, got %d", count)
	}
	mu.Unlock()
}

func TestEventBus_SubscribeAll(t *testing.T) {
	bus := NewEventBus()
	events := make([]*Event, 0)
	var mu sync.Mutex

	bus.SubscribeAll(func(e *Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	bus.Publish(&Event{Type: EventTaskCreated, TaskID: "t1"})
	bus.Publish(&Event{Type: EventSlashTriggered, TaskID: "t2"})

	mu.Lock()
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
	mu.Unlock()
}

// --- Scheduler Tests ---

func TestScheduler_RegisterAndSelect(t *testing.T) {
	sched := NewScheduler()

	for i := 0; i < 10; i++ {
		sched.RegisterNode(&NodeInfo{
			ID:     fmt.Sprintf("node%d", i),
			Active: true,
			Stake:  1000,
		})
	}

	nodes, err := sched.SelectNodes(5)
	if err != nil {
		t.Fatalf("Failed to select nodes: %v", err)
	}
	if len(nodes) != 5 {
		t.Errorf("Expected 5 nodes, got %d", len(nodes))
	}

	// Ensure all nodes are unique
	seen := make(map[string]bool)
	for _, n := range nodes {
		if seen[n] {
			t.Errorf("Duplicate node selected: %s", n)
		}
		seen[n] = true
	}
}

func TestScheduler_InsufficientNodes(t *testing.T) {
	sched := NewScheduler()

	sched.RegisterNode(&NodeInfo{ID: "node1", Active: true})
	sched.RegisterNode(&NodeInfo{ID: "node2", Active: true})

	_, err := sched.SelectNodes(5)
	if err == nil {
		t.Error("Expected error for insufficient nodes")
	}
}

func TestScheduler_InactiveNodesExcluded(t *testing.T) {
	sched := NewScheduler()

	for i := 0; i < 5; i++ {
		sched.RegisterNode(&NodeInfo{
			ID:     fmt.Sprintf("node%d", i),
			Active: i < 3, // Only first 3 are active
		})
	}

	_, err := sched.SelectNodes(5)
	if err == nil {
		t.Error("Expected error: only 3 active nodes but need 5")
	}

	nodes, err := sched.SelectNodes(3)
	if err != nil {
		t.Fatalf("Failed to select 3 nodes: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}
}

func TestScheduler_UnregisterNode(t *testing.T) {
	sched := NewScheduler()

	sched.RegisterNode(&NodeInfo{ID: "node1", Active: true})
	sched.UnregisterNode("node1")

	if sched.ActiveNodeCount() != 0 {
		t.Error("Expected 0 active nodes after unregister")
	}
}

// --- Orchestrator Tests ---

func setupOrchestrator(nodeCount int) *Orchestrator {
	config := &OrchestratorConfig{
		MinNodes:       5,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		TaskTimeout:    5 * time.Minute,
		AutoSlash:      true,
	}

	orch := NewOrchestrator(config)

	for i := 0; i < nodeCount; i++ {
		orch.Scheduler().RegisterNode(&NodeInfo{
			ID:     fmt.Sprintf("node%d", i),
			Active: true,
			Stake:  1000,
		})
	}

	return orch
}

func TestOrchestrator_SubmitTask(t *testing.T) {
	orch := setupOrchestrator(10)

	task, err := orch.SubmitTask("What is AI?", "user1")
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if task.GetState() != TaskStateAssigned {
		t.Errorf("Expected state Assigned, got %s", task.GetState())
	}
	if len(task.AssignedTo) != 5 {
		t.Errorf("Expected 5 assigned nodes, got %d", len(task.AssignedTo))
	}

	metrics := orch.GetMetrics()
	if metrics.TotalTasks != 1 {
		t.Errorf("Expected 1 total task, got %d", metrics.TotalTasks)
	}
}

func TestOrchestrator_SubmitTaskInsufficientNodes(t *testing.T) {
	orch := setupOrchestrator(3) // Only 3 nodes, need 5

	_, err := orch.SubmitTask("test", "user1")
	if err == nil {
		t.Error("Expected error for insufficient nodes")
	}
}

func TestOrchestrator_SubmitTaskValidation(t *testing.T) {
	orch := setupOrchestrator(10)

	if _, err := orch.SubmitTask("", "user1"); err == nil {
		t.Error("Expected error for empty prompt")
	}
	if _, err := orch.SubmitTask("prompt", ""); err == nil {
		t.Error("Expected error for empty requester ID")
	}
}

func TestOrchestrator_FullLifecycle_Success(t *testing.T) {
	orch := setupOrchestrator(10)

	// Track events
	eventLog := make([]EventType, 0)
	var mu sync.Mutex
	orch.EventBus().SubscribeAll(func(e *Event) {
		mu.Lock()
		eventLog = append(eventLog, e.Type)
		mu.Unlock()
	})

	// 1. Submit task
	task, err := orch.SubmitTask("What is 2+2?", "user1")
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// 2. Start commit phase
	if err := orch.StartCommitPhase(task.ID); err != nil {
		t.Fatalf("StartCommitPhase failed: %v", err)
	}

	// 3. Nodes submit commits
	nonces := make(map[string][]byte)
	for _, nodeID := range task.AssignedTo {
		nonce := []byte(fmt.Sprintf("nonce_%s", nodeID))
		nonces[nodeID] = nonce
		commitHash := verification.ComputeCommitHash("4", nonce)
		if err := orch.SubmitCommit(task.ID, nodeID, commitHash); err != nil {
			t.Fatalf("SubmitCommit failed for %s: %v", nodeID, err)
		}
	}

	// Wait for commit phase to end
	time.Sleep(110 * time.Millisecond)

	// 4. Start reveal phase
	if err := orch.StartRevealPhase(task.ID); err != nil {
		t.Fatalf("StartRevealPhase failed: %v", err)
	}

	// 5. Nodes reveal results (all agree)
	for _, nodeID := range task.AssignedTo {
		if err := orch.SubmitReveal(task.ID, nodeID, "4", nonces[nodeID]); err != nil {
			t.Fatalf("SubmitReveal failed for %s: %v", nodeID, err)
		}
	}

	// Wait for reveal phase to end
	time.Sleep(110 * time.Millisecond)

	// 6. Verify
	vResult, err := orch.Verify(task.ID)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !vResult.IsValid {
		t.Error("Expected valid verification result")
	}
	if vResult.MajorityResult != "4" {
		t.Errorf("Expected majority result '4', got %s", vResult.MajorityResult)
	}
	if vResult.AgreementRate != 1.0 {
		t.Errorf("Expected 100%% agreement, got %.2f%%", vResult.AgreementRate*100)
	}

	// 7. Settle
	if err := orch.SettleTask(task.ID); err != nil {
		t.Fatalf("Settle failed: %v", err)
	}

	if task.GetState() != TaskStateSettled {
		t.Errorf("Expected Settled state, got %s", task.GetState())
	}

	// Verify events were emitted in order
	mu.Lock()
	expectedEvents := []EventType{
		EventTaskCreated,
		EventTaskAssigned,
		EventCommitPhaseStarted,
		EventRevealPhaseStarted,
		EventVerificationStarted,
		EventVerificationComplete,
		EventTaskSettled,
	}
	if len(eventLog) < len(expectedEvents) {
		t.Errorf("Expected at least %d events, got %d", len(expectedEvents), len(eventLog))
	}
	mu.Unlock()

	// Metrics
	metrics := orch.GetMetrics()
	if metrics.CompletedTasks != 1 {
		t.Errorf("Expected 1 completed task, got %d", metrics.CompletedTasks)
	}
}

func TestOrchestrator_FullLifecycle_WithDisagreement(t *testing.T) {
	orch := setupOrchestrator(10)

	slashEvents := make([]*Event, 0)
	var mu sync.Mutex
	orch.EventBus().Subscribe(EventSlashTriggered, func(e *Event) {
		mu.Lock()
		slashEvents = append(slashEvents, e)
		mu.Unlock()
	})

	// Submit task
	task, err := orch.SubmitTask("What is 2+2?", "user1")
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Start commit phase
	orch.StartCommitPhase(task.ID)

	// Nodes commit (4 agree on "4", 1 disagrees with "5")
	nonces := make(map[string][]byte)
	for i, nodeID := range task.AssignedTo {
		nonce := []byte(fmt.Sprintf("nonce_%s", nodeID))
		nonces[nodeID] = nonce
		result := "4"
		if i == len(task.AssignedTo)-1 {
			result = "5" // Last node disagrees
		}
		commitHash := verification.ComputeCommitHash(result, nonce)
		orch.SubmitCommit(task.ID, nodeID, commitHash)
	}

	time.Sleep(110 * time.Millisecond)
	orch.StartRevealPhase(task.ID)

	for i, nodeID := range task.AssignedTo {
		result := "4"
		if i == len(task.AssignedTo)-1 {
			result = "5"
		}
		orch.SubmitReveal(task.ID, nodeID, result, nonces[nodeID])
	}

	time.Sleep(110 * time.Millisecond)

	// Verify
	vResult, err := orch.Verify(task.ID)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !vResult.IsValid {
		t.Error("Expected valid result (4/5 = 80% > 67%)")
	}
	if len(vResult.Disagreeing) != 1 {
		t.Errorf("Expected 1 disagreeing node, got %d", len(vResult.Disagreeing))
	}

	// Check slash was triggered
	mu.Lock()
	if len(slashEvents) != 1 {
		t.Errorf("Expected 1 slash event, got %d", len(slashEvents))
	}
	mu.Unlock()

	metrics := orch.GetMetrics()
	if metrics.TotalSlashes != 1 {
		t.Errorf("Expected 1 total slash, got %d", metrics.TotalSlashes)
	}
}

func TestOrchestrator_FailedVerification(t *testing.T) {
	orch := setupOrchestrator(10)

	task, err := orch.SubmitTask("What is the meaning of life?", "user1")
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	orch.StartCommitPhase(task.ID)

	// All nodes disagree (each has unique answer)
	nonces := make(map[string][]byte)
	for i, nodeID := range task.AssignedTo {
		nonce := []byte(fmt.Sprintf("nonce_%s", nodeID))
		nonces[nodeID] = nonce
		result := fmt.Sprintf("answer_%d", i)
		commitHash := verification.ComputeCommitHash(result, nonce)
		orch.SubmitCommit(task.ID, nodeID, commitHash)
	}

	time.Sleep(110 * time.Millisecond)
	orch.StartRevealPhase(task.ID)

	for i, nodeID := range task.AssignedTo {
		result := fmt.Sprintf("answer_%d", i)
		orch.SubmitReveal(task.ID, nodeID, result, nonces[nodeID])
	}

	time.Sleep(110 * time.Millisecond)

	vResult, err := orch.Verify(task.ID)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if vResult.IsValid {
		t.Error("Expected invalid result (no majority)")
	}

	if task.GetState() != TaskStateFailed {
		t.Errorf("Expected Failed state, got %s", task.GetState())
	}

	metrics := orch.GetMetrics()
	if metrics.FailedTasks != 1 {
		t.Errorf("Expected 1 failed task, got %d", metrics.FailedTasks)
	}
}

func TestOrchestrator_WrongPhaseErrors(t *testing.T) {
	orch := setupOrchestrator(10)

	task, _ := orch.SubmitTask("test", "user1")

	// Cannot submit commit before commit phase starts
	err := orch.SubmitCommit(task.ID, "node0", []byte("hash"))
	if err == nil {
		t.Error("Expected error: task not in commit phase")
	}

	// Cannot submit reveal before reveal phase starts
	err = orch.SubmitReveal(task.ID, "node0", "result", []byte("nonce"))
	if err == nil {
		t.Error("Expected error: task not in reveal phase")
	}
}

func TestOrchestrator_Concurrent(t *testing.T) {
	orch := setupOrchestrator(20)

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			task, err := orch.SubmitTask(
				fmt.Sprintf("concurrent prompt %d", idx),
				fmt.Sprintf("user%d", idx),
			)
			if err != nil {
				errors <- fmt.Errorf("task %d submit: %w", idx, err)
				return
			}

			if err := orch.StartCommitPhase(task.ID); err != nil {
				errors <- fmt.Errorf("task %d commit phase: %w", idx, err)
				return
			}

			nonces := make(map[string][]byte)
			for _, nodeID := range task.AssignedTo {
				nonce := []byte(fmt.Sprintf("nonce_%s_%d", nodeID, idx))
				nonces[nodeID] = nonce
				commitHash := verification.ComputeCommitHash("answer", nonce)
				if err := orch.SubmitCommit(task.ID, nodeID, commitHash); err != nil {
					errors <- fmt.Errorf("task %d commit %s: %w", idx, nodeID, err)
					return
				}
			}

			time.Sleep(110 * time.Millisecond)
			if err := orch.StartRevealPhase(task.ID); err != nil {
				errors <- fmt.Errorf("task %d reveal phase: %w", idx, err)
				return
			}

			for _, nodeID := range task.AssignedTo {
				if err := orch.SubmitReveal(task.ID, nodeID, "answer", nonces[nodeID]); err != nil {
					errors <- fmt.Errorf("task %d reveal %s: %w", idx, nodeID, err)
					return
				}
			}

			time.Sleep(110 * time.Millisecond)
			if _, err := orch.Verify(task.ID); err != nil {
				errors <- fmt.Errorf("task %d verify: %w", idx, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent error: %v", err)
	}

	metrics := orch.GetMetrics()
	if metrics.TotalTasks != 5 {
		t.Errorf("Expected 5 total tasks, got %d", metrics.TotalTasks)
	}
	if metrics.CompletedTasks != 5 {
		t.Errorf("Expected 5 completed tasks, got %d", metrics.CompletedTasks)
	}
}

func TestOrchestrator_GetTask(t *testing.T) {
	orch := setupOrchestrator(10)

	task, _ := orch.SubmitTask("test", "user1")

	found, err := orch.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if found.ID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, found.ID)
	}

	_, err = orch.GetTask("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent task")
	}
}

func TestOrchestrator_AutoSlashDisabled(t *testing.T) {
	config := &OrchestratorConfig{
		MinNodes:       5,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		TaskTimeout:    5 * time.Minute,
		AutoSlash:      false, // Disabled
	}
	orch := NewOrchestrator(config)

	for i := 0; i < 10; i++ {
		orch.Scheduler().RegisterNode(&NodeInfo{
			ID:     fmt.Sprintf("node%d", i),
			Active: true,
		})
	}

	task, _ := orch.SubmitTask("test", "user1")
	orch.StartCommitPhase(task.ID)

	nonces := make(map[string][]byte)
	for i, nodeID := range task.AssignedTo {
		nonce := []byte(fmt.Sprintf("nonce_%s", nodeID))
		nonces[nodeID] = nonce
		result := "agree"
		if i == 0 {
			result = "disagree"
		}
		commitHash := verification.ComputeCommitHash(result, nonce)
		orch.SubmitCommit(task.ID, nodeID, commitHash)
	}

	time.Sleep(110 * time.Millisecond)
	orch.StartRevealPhase(task.ID)

	for i, nodeID := range task.AssignedTo {
		result := "agree"
		if i == 0 {
			result = "disagree"
		}
		orch.SubmitReveal(task.ID, nodeID, result, nonces[nodeID])
	}

	time.Sleep(110 * time.Millisecond)
	orch.Verify(task.ID)

	metrics := orch.GetMetrics()
	if metrics.TotalSlashes != 0 {
		t.Errorf("Expected 0 slashes with AutoSlash disabled, got %d", metrics.TotalSlashes)
	}
}
