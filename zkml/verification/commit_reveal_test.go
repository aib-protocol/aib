package verification

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestCommitReveal_BasicFlow(t *testing.T) {
	commitDuration := 10 * time.Second
	revealDuration := 10 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-basic-flow"
	nodeID := "node1"
	result := "inference-result-42"
	nonce := []byte("random-nonce-123")

	// Start task
	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Should be in commit phase
	if !cr.IsCommitPhase(taskID) {
		t.Fatal("Expected to be in commit phase")
	}
	if cr.IsRevealPhase(taskID) {
		t.Fatal("Did not expect to be in reveal phase")
	}

	// Compute commit hash
	commitHash := ComputeCommitHash(result, nonce)

	// Commit
	if err := cr.Commit(taskID, nodeID, commitHash); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Simulate time passing: advance past commit phase into reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	// Should now be in reveal phase
	if cr.IsCommitPhase(taskID) {
		t.Fatal("Should not be in commit phase anymore")
	}
	if !cr.IsRevealPhase(taskID) {
		t.Fatal("Expected to be in reveal phase")
	}

	// Reveal
	if err := cr.Reveal(taskID, nodeID, result, nonce); err != nil {
		t.Fatalf("Reveal failed: %v", err)
	}

	// Advance past reveal phase to get results
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+revealDuration.Nanoseconds()+1)
	}

	// Get results
	results, err := cr.GetResults(taskID)
	if err != nil {
		t.Fatalf("GetResults failed: %v", err)
	}
	if results[nodeID] != result {
		t.Errorf("Expected result %q, got %q", result, results[nodeID])
	}
}

func TestCommitReveal_MultipleNodes(t *testing.T) {
	commitDuration := 10 * time.Second
	revealDuration := 10 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-multi-node"

	nodes := []struct {
		nodeID string
		result string
		nonce  []byte
	}{
		{"node1", "result-A", []byte("nonce1")},
		{"node2", "result-A", []byte("nonce2")},
		{"node3", "result-B", []byte("nonce3")},
	}

	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// All nodes commit
	for _, n := range nodes {
		commitHash := ComputeCommitHash(n.result, n.nonce)
		if err := cr.Commit(taskID, n.nodeID, commitHash); err != nil {
			t.Fatalf("Commit from %s failed: %v", n.nodeID, err)
		}
	}

	// Advance to reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	// All nodes reveal
	for _, n := range nodes {
		if err := cr.Reveal(taskID, n.nodeID, n.result, n.nonce); err != nil {
			t.Fatalf("Reveal from %s failed: %v", n.nodeID, err)
		}
	}

	// Advance past reveal phase
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+revealDuration.Nanoseconds()+1)
	}

	results, err := cr.GetResults(taskID)
	if err != nil {
		t.Fatalf("GetResults failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
	if results["node1"] != "result-A" {
		t.Errorf("Expected node1 result 'result-A', got %q", results["node1"])
	}
	if results["node3"] != "result-B" {
		t.Errorf("Expected node3 result 'result-B', got %q", results["node3"])
	}
}

func TestCommitReveal_CommitAfterDeadline(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-late-commit"
	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Advance past commit deadline
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	commitHash := ComputeCommitHash("result", []byte("nonce"))
	err := cr.Commit(taskID, "node1", commitHash)
	if err == nil {
		t.Fatal("Expected error for commit after deadline, got nil")
	}
}

func TestCommitReveal_RevealAfterDeadline(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-late-reveal"
	result := "my-result"
	nonce := []byte("my-nonce")

	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	commitHash := ComputeCommitHash(result, nonce)
	if err := cr.Commit(taskID, "node1", commitHash); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Advance past both commit and reveal deadlines
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+revealDuration.Nanoseconds()+1)
	}

	err := cr.Reveal(taskID, "node1", result, nonce)
	if err == nil {
		t.Fatal("Expected error for reveal after deadline, got nil")
	}
}

func TestCommitReveal_RevealBeforeCommitPhaseEnds(t *testing.T) {
	commitDuration := 10 * time.Second
	revealDuration := 10 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-early-reveal"
	result := "my-result"
	nonce := []byte("my-nonce")

	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	commitHash := ComputeCommitHash(result, nonce)
	if err := cr.Commit(taskID, "node1", commitHash); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Try to reveal during commit phase (nowFunc is still default = start time)
	err := cr.Reveal(taskID, "node1", result, nonce)
	if err == nil {
		t.Fatal("Expected error for reveal during commit phase, got nil")
	}
}

func TestCommitReveal_HashMismatch(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-hash-mismatch"
	result := "correct-result"
	nonce := []byte("correct-nonce")
	wrongResult := "wrong-result"

	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	commitHash := ComputeCommitHash(result, nonce)
	if err := cr.Commit(taskID, "node1", commitHash); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Advance to reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	// Try to reveal with wrong result
	err := cr.Reveal(taskID, "node1", wrongResult, nonce)
	if err == nil {
		t.Fatal("Expected error for hash mismatch, got nil")
	}

	// Try with wrong nonce
	err = cr.Reveal(taskID, "node1", result, []byte("wrong-nonce"))
	if err == nil {
		t.Fatal("Expected error for wrong nonce, got nil")
	}
}

func TestCommitReveal_RevealWithoutCommit(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-no-commit"
	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Advance to reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	err := cr.Reveal(taskID, "node1", "some-result", []byte("some-nonce"))
	if err == nil {
		t.Fatal("Expected error for reveal without commit, got nil")
	}
}

func TestCommitReveal_TaskNotFound(t *testing.T) {
	cr := NewCommitRevealVerifier(5*time.Second, 5*time.Second)

	// Commit to non-existent task
	err := cr.Commit("nonexistent", "node1", []byte("hash"))
	if err == nil {
		t.Fatal("Expected error for non-existent task commit")
	}

	// Reveal for non-existent task
	err = cr.Reveal("nonexistent", "node1", "result", []byte("nonce"))
	if err == nil {
		t.Fatal("Expected error for non-existent task reveal")
	}

	// GetResults for non-existent task
	_, err = cr.GetResults("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent task GetResults")
	}

	// IsCommitPhase for non-existent task
	if cr.IsCommitPhase("nonexistent") {
		t.Fatal("Expected false for non-existent task IsCommitPhase")
	}

	// IsRevealPhase for non-existent task
	if cr.IsRevealPhase("nonexistent") {
		t.Fatal("Expected false for non-existent task IsRevealPhase")
	}
}

func TestCommitReveal_DuplicateTask(t *testing.T) {
	cr := NewCommitRevealVerifier(5*time.Second, 5*time.Second)

	if err := cr.StartTask("task-dup"); err != nil {
		t.Fatalf("First StartTask failed: %v", err)
	}

	err := cr.StartTask("task-dup")
	if err == nil {
		t.Fatal("Expected error for duplicate task start")
	}
}

func TestCommitReveal_DuplicateCommit(t *testing.T) {
	cr := NewCommitRevealVerifier(10*time.Second, 10*time.Second)

	taskID := "task-dup-commit"
	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	commitHash := ComputeCommitHash("result", []byte("nonce"))
	if err := cr.Commit(taskID, "node1", commitHash); err != nil {
		t.Fatalf("First commit failed: %v", err)
	}

	err := cr.Commit(taskID, "node1", commitHash)
	if err == nil {
		t.Fatal("Expected error for duplicate commit")
	}
}

func TestCommitReveal_DuplicateReveal(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-dup-reveal"
	result := "my-result"
	nonce := []byte("my-nonce")

	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	commitHash := ComputeCommitHash(result, nonce)
	if err := cr.Commit(taskID, "node1", commitHash); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Advance to reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	if err := cr.Reveal(taskID, "node1", result, nonce); err != nil {
		t.Fatalf("First reveal failed: %v", err)
	}

	err := cr.Reveal(taskID, "node1", result, nonce)
	if err == nil {
		t.Fatal("Expected error for duplicate reveal")
	}
}

func TestCommitReveal_EmptyInputs(t *testing.T) {
	cr := NewCommitRevealVerifier(5*time.Second, 5*time.Second)

	// Empty task ID on StartTask
	if err := cr.StartTask(""); err == nil {
		t.Fatal("Expected error for empty task ID on StartTask")
	}

	// Empty task ID on Commit
	if err := cr.Commit("", "node1", []byte("hash")); err == nil {
		t.Fatal("Expected error for empty task ID on Commit")
	}

	// Empty node ID on Commit
	if err := cr.StartTask("task-empty"); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}
	if err := cr.Commit("task-empty", "", []byte("hash")); err == nil {
		t.Fatal("Expected error for empty node ID on Commit")
	}

	// Empty commit hash
	if err := cr.Commit("task-empty", "node1", nil); err == nil {
		t.Fatal("Expected error for nil commit hash")
	}
	if err := cr.Commit("task-empty", "node1", []byte{}); err == nil {
		t.Fatal("Expected error for empty commit hash")
	}

	// Empty task ID on Reveal
	if err := cr.Reveal("", "node1", "result", []byte("nonce")); err == nil {
		t.Fatal("Expected error for empty task ID on Reveal")
	}

	// Empty node ID on Reveal
	startTime := cr.taskTimestamps["task-empty"]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+5*time.Second.Nanoseconds()+1)
	}
	if err := cr.Reveal("task-empty", "", "result", []byte("nonce")); err == nil {
		t.Fatal("Expected error for empty node ID on Reveal")
	}

	// Empty task ID on GetResults
	if _, err := cr.GetResults(""); err == nil {
		t.Fatal("Expected error for empty task ID on GetResults")
	}
}

func TestCommitReveal_GetResultsBeforeRevealEnds(t *testing.T) {
	commitDuration := 5 * time.Second
	revealDuration := 5 * time.Second
	cr := NewCommitRevealVerifier(commitDuration, revealDuration)

	taskID := "task-early-results"
	if err := cr.StartTask(taskID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Try to get results during commit phase
	_, err := cr.GetResults(taskID)
	if err == nil {
		t.Fatal("Expected error for GetResults during commit phase")
	}

	// Advance to reveal phase
	startTime := cr.taskTimestamps[taskID]
	cr.nowFunc = func() time.Time {
		return time.Unix(0, startTime+commitDuration.Nanoseconds()+1)
	}

	// Try to get results during reveal phase
	_, err = cr.GetResults(taskID)
	if err == nil {
		t.Fatal("Expected error for GetResults during reveal phase")
	}
}

func TestCommitReveal_ComputeCommitHash(t *testing.T) {
	result := "test-result"
	nonce := []byte("test-nonce")

	hash1 := ComputeCommitHash(result, nonce)
	hash2 := ComputeCommitHash(result, nonce)

	// Same inputs should produce same hash
	if len(hash1) != sha256.Size {
		t.Errorf("Expected hash length %d, got %d", sha256.Size, len(hash1))
	}
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Fatal("Same inputs produced different hashes")
		}
	}

	// Different inputs should produce different hash
	hash3 := ComputeCommitHash(result, []byte("different-nonce"))
	same := true
	for i := range hash1 {
		if hash1[i] != hash3[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("Different inputs produced same hash")
	}

	hash4 := ComputeCommitHash("different-result", nonce)
	same = true
	for i := range hash1 {
		if hash1[i] != hash4[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("Different results produced same hash")
	}
}
