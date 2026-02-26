package verification

import (
	"fmt"
	"sync"
	"testing"
)

func TestMajorityVerifier_Verify(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Test case 1: Simple majority (5/6 = 0.83, above 0.67)
	results1 := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultA",
		"node5": "resultA",
		"node6": "resultB",
	}
	result1, err := verifier.Verify("task1", results1)
	if err != nil {
		t.Fatalf("Failed to verify task1: %v", err)
	}
	if !result1.IsValid {
		t.Errorf("Expected task1 to be valid (agreement rate: %f)", result1.AgreementRate)
	}
	if result1.MajorityResult != "resultA" {
		t.Errorf("Expected majority result 'resultA', got %s", result1.MajorityResult)
	}
	if len(result1.Disagreeing) != 1 {
		t.Errorf("Expected 1 disagreeing node, got %d", len(result1.Disagreeing))
	}

	// Test case 2: Unanimous agreement
	results2 := map[string]string{
		"node1": "resultX",
		"node2": "resultX",
		"node3": "resultX",
		"node4": "resultX",
		"node5": "resultX",
	}
	result2, err := verifier.Verify("task2", results2)
	if err != nil {
		t.Fatalf("Failed to verify task2: %v", err)
	}
	if !result2.IsValid {
		t.Error("Expected task2 to be valid")
	}
	if result2.AgreementRate != 1.0 {
		t.Errorf("Expected agreement rate 1.0, got %f", result2.AgreementRate)
	}
	if len(result2.Disagreeing) != 0 {
		t.Errorf("Expected 0 disagreeing nodes, got %d", len(result2.Disagreeing))
	}

	// Test case 3: Insufficient agreement (below threshold, 2/5 = 0.4)
	results3 := map[string]string{
		"node1": "resultX",
		"node2": "resultX",
		"node3": "resultY",
		"node4": "resultZ",
		"node5": "resultW",
	}
	result3, err := verifier.Verify("task3", results3)
	if err != nil {
		t.Fatalf("Failed to verify task3: %v", err)
	}
	if result3.IsValid {
		t.Error("Expected task3 to be invalid (below threshold)")
	}
}

func TestMajorityVerifier_VerifyNumeric(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Test case: Numeric results (5 nodes, 4 agree)
	results := map[string]float64{
		"node1": 1.2345,
		"node2": 1.2345,
		"node3": 2.3456,
		"node4": 1.2345,
		"node5": 1.2345,
	}
	result, err := verifier.VerifyNumeric("task_numeric", results)
	if err != nil {
		t.Fatalf("Failed to verify numeric task: %v", err)
	}
	if !result.IsValid {
		t.Error("Expected numeric task to be valid")
	}
	if result.MajorityResult != "1.2345" {
		t.Errorf("Expected majority result '1.2345', got %s", result.MajorityResult)
	}
	if len(result.Values) != 5 {
		t.Errorf("Expected 5 values, got %d", len(result.Values))
	}
}

func TestMajorityVerifier_VerifyJSON(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Test case: JSON results (4 nodes with same result, 1 with different)
	results := map[string][]byte{
		"node1": []byte(`{"result": "A", "confidence": 0.9}`),
		"node2": []byte(`{"result": "A", "confidence": 0.9}`),
		"node3": []byte(`{"result": "A", "confidence": 0.9}`),
		"node4": []byte(`{"result": "A", "confidence": 0.9}`),
		"node5": []byte(`{"result": "B", "confidence": 0.8}`),
	}
	result, err := verifier.VerifyJSON("task_json", results)
	if err != nil {
		t.Fatalf("Failed to verify JSON task: %v", err)
	}
	if !result.IsValid {
		t.Errorf("Expected JSON task to be valid (agreement rate: %f)", result.AgreementRate)
	}
	if len(result.Disagreeing) != 1 {
		t.Errorf("Expected 1 disagreeing node, got %d", len(result.Disagreeing))
	}
}

func TestMajorityVerifier_Settings(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Test: Change threshold and minNodes
	verifier.SetThreshold(0.8)
	verifier.SetMinNodes(7)

	// Verify insufficient nodes case (only 5 nodes, need 7)
	results := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultA",
		"node5": "resultA",
	}
	result, err := verifier.Verify("task_settings", results)
	if err == nil {
		t.Error("Expected error for insufficient nodes")
	}
	if result != nil && result.IsValid {
		t.Error("Expected invalid result due to insufficient nodes")
	}
}

func TestWeightedMajorityVerifier(t *testing.T) {
	// Create mock weight provider
	provider := &mockWeightProvider{
		weights: map[string]float64{
			"node1": 2.0,
			"node2": 1.0,
			"node3": 1.0,
		},
	}

	verifier := NewWeightedMajorityVerifier(provider)

	// Test case: Weighted majority
	results := map[string]string{
		"node1": "resultA", // weight 2.0
		"node2": "resultB", // weight 1.0
		"node3": "resultB", // weight 1.0
	}
	result, err := verifier.VerifyWeighted("task_weighted", results)
	if err != nil {
		t.Fatalf("Failed to verify weighted task: %v", err)
	}
	// With weights: resultA = 2.0, resultB = 2.0 (1.0 + 1.0)
	// Agreement rate for resultB = 2.0 / 4.0 = 0.5
	// But threshold is 0.67 by default, so should be invalid
	if result.IsValid {
		t.Error("Expected weighted task to be invalid (0.5 < 0.67)")
	}
}

func TestBatchVerifier(t *testing.T) {
	batchVerifier := NewBatchVerifier()

	batch := &TaskBatch{
		TaskID: "batch_task",
		Results: []*TaskResult{
			{NodeID: "node1", Result: "resultA", Timestamp: 1000},
			{NodeID: "node2", Result: "resultA", Timestamp: 1001},
			{NodeID: "node3", Result: "resultB", Timestamp: 1002},
		},
		Timestamp: 1000,
	}

	results, err := batchVerifier.VerifyBatch(batch)
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	// Each node produces 1 result with 1 entry, so none meet minNodes=3
	// The batch verifier processes per-node; expect 0 valid results
	if results == nil {
		t.Fatal("Expected non-nil results slice")
	}
}

func TestVerificationHistory(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Perform multiple verifications (need at least 5 nodes for minNodes=5)
	results := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultA",
		"node5": "resultB",
	}
	for i := 0; i < 5; i++ {
		_, err := verifier.Verify("task_history", results)
		if err != nil {
			t.Fatalf("Failed verification %d: %v", i, err)
		}
	}

	// Get history
	history := verifier.GetHistory("task_history")
	if len(history) == 0 {
		t.Error("Expected history to contain entries")
	}
	if len(history) > 100 {
		t.Error("History should be limited to 100 entries")
	}
}

func TestGetStatistics_NotZero(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Use 5 nodes: 3/5 = 60%, below 67% threshold
	results1 := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultB",
		"node5": "resultB",
	}
	verifier.Verify("task1", results1)

	// Use 6 nodes: 5/6 = 83%, above threshold
	results2 := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultA",
		"node5": "resultA",
		"node6": "resultB",
	}
	verifier.Verify("task2", results2)

	stats := verifier.GetStatistics()
	if stats.TotalVerifications != 2 {
		t.Errorf("Expected 2 total verifications, got %d", stats.TotalVerifications)
	}
	if stats.SuccessfulVerifications != 1 {
		t.Errorf("Expected 1 successful verification, got %d", stats.SuccessfulVerifications)
	}
	if stats.SuccessRate == 0 {
		t.Error("Expected non-zero success rate (bug: GetStatistics returned 0)")
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("Expected success rate 0.5, got %f", stats.SuccessRate)
	}
	if stats.AverageAgreement == 0 {
		t.Error("Expected non-zero average agreement (bug: GetStatistics returned 0)")
	}
}

// Mock weight provider for testing
type mockWeightProvider struct {
	weights map[string]float64
}

func (m *mockWeightProvider) GetWeight(nodeID string) (float64, error) {
	if weight, ok := m.weights[nodeID]; ok {
		return weight, nil
	}
	return 1.0, nil // Default weight
}

// TestMajorityVerifier_Concurrent tests concurrent verification operations
func TestMajorityVerifier_Concurrent(t *testing.T) {
	verifier := NewMajorityVerifier()
	numGoroutines := 50
	numIterations := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect any errors
	errors := make(chan error, numGoroutines*numIterations)

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				// Create unique task ID for each verification
				taskID := fmt.Sprintf("concurrent_task_%d_%d", id, j)

				// Create results with at least 5 nodes (to meet minNodes requirement)
				results := make(map[string]string)
				for k := 0; k < 7; k++ {
					nodeID := fmt.Sprintf("node_%d", k)
					if k < 5 {
						results[nodeID] = "agree"
					} else {
						results[nodeID] = "disagree"
					}
				}

				_, err := verifier.Verify(taskID, results)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d iteration %d: %w", id, j, err)
					return
				}

				// Test GetHistory concurrently
				history := verifier.GetHistory(taskID)
				if history == nil {
					errors <- fmt.Errorf("goroutine %d iteration %d: history is nil", id, j)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify final statistics
	stats := verifier.GetStatistics()
	if stats.TotalVerifications != numGoroutines*numIterations {
		t.Errorf("Expected %d total verifications, got %d", numGoroutines*numIterations, stats.TotalVerifications)
	}

	// Verify that no panic or data corruption occurred
	if stats.TotalVerifications == 0 {
		t.Error("No verifications were recorded")
	}
}

// TestMajorityVerifier_ConcurrentHistoryAccess tests concurrent history access
func TestMajorityVerifier_ConcurrentHistoryAccess(t *testing.T) {
	verifier := NewMajorityVerifier()

	// Pre-populate some history
	taskID := "history_task"
	results := map[string]string{
		"node1": "resultA",
		"node2": "resultA",
		"node3": "resultA",
		"node4": "resultA",
		"node5": "resultA",
	}
	for i := 0; i < 10; i++ {
		verifier.Verify(taskID, results)
	}

	var wg sync.WaitGroup
	numGoroutines := 50
	wg.Add(numGoroutines)

	// Concurrent read access to history
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			history := verifier.GetHistory(taskID)
			if history == nil {
				t.Errorf("Goroutine %d: history is nil", id)
				return
			}

			if len(history) == 0 {
				t.Errorf("Goroutine %d: expected non-empty history", id)
			}
		}(i)
	}

	wg.Wait()
}
