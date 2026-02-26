package slashing

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultSlashConfig(t *testing.T) {
	config := DefaultSlashConfig()

	if config.FraudProof != 1.0 {
		t.Errorf("Expected FraudProof 1.0, got %f", config.FraudProof)
	}
	if config.SybilAttack != 1.0 {
		t.Errorf("Expected SybilAttack 1.0, got %f", config.SybilAttack)
	}
	if config.CopyResult != 0.5 {
		t.Errorf("Expected CopyResult 0.5, got %f", config.CopyResult)
	}
	if config.NoWork != 0.25 {
		t.Errorf("Expected NoWork 0.25, got %f", config.NoWork)
	}
	if config.Misbehavior != 0.1 {
		t.Errorf("Expected Misbehavior 0.1, got %f", config.Misbehavior)
	}
	if config.ReporterRewardRatio != 0.2 {
		t.Errorf("Expected ReporterRewardRatio 0.2, got %f", config.ReporterRewardRatio)
	}
}

func TestSlashEngine_ShouldSlash(t *testing.T) {
	engine := NewSlashEngine(nil)

	tests := []struct {
		name     string
		evidence *Evidence
		want     bool
		ratio    float64
	}{
		{
			name: "FraudProof",
			evidence: &Evidence{
				Type:      FraudProof,
				Offender:  []byte("node1"),
				Timestamp: time.Now().Unix(),
				ProofData: []byte("proof"),
				Severity:  8,
			},
			want:  true,
			ratio: 1.0,
		},
		{
			name: "SybilAttack",
			evidence: &Evidence{
				Type:      SybilAttack,
				Offender:  []byte("node2"),
				Timestamp: time.Now().Unix(),
				ProofData: []byte("proof"),
				Severity:  8,
			},
			want:  true,
			ratio: 1.0,
		},
		{
			name: "CopyResult",
			evidence: &Evidence{
				Type:      CopyResult,
				Offender:  []byte("node3"),
				Timestamp: time.Now().Unix(),
				ProofData: []byte("proof"),
				Severity:  5,
			},
			want:  true,
			ratio: 0.5,
		},
		{
			name: "NoWork",
			evidence: &Evidence{
				Type:      NoWork,
				Offender:  []byte("node4"),
				Timestamp: time.Now().Unix(),
				ProofData: []byte("proof"),
				Severity:  3,
			},
			want:  true,
			ratio: 0.25,
		},
		{
			name:     "nil evidence",
			evidence: nil,
			want:     false,
			ratio:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSlash, ratio := engine.ShouldSlash(tt.evidence)
			if shouldSlash != tt.want {
				t.Errorf("ShouldSlash() = %v, want %v", shouldSlash, tt.want)
			}
			if ratio != tt.ratio {
				t.Errorf("Slash ratio = %f, want %f", ratio, tt.ratio)
			}
		})
	}
}

func TestSlashEngine_ExecuteSlash(t *testing.T) {
	engine := NewSlashEngine(nil)

	evidence := &Evidence{
		Type:      FraudProof,
		Offender:  []byte("node_offender"),
		Reporter:  []byte("node_reporter"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("fraud_proof_data"),
		Severity:  8,
	}

	event, err := engine.ExecuteSlash(evidence)
	if err != nil {
		t.Fatalf("ExecuteSlash failed: %v", err)
	}
	if event == nil {
		t.Fatal("Expected slash event, got nil")
	}
	if event.Amount != 1000.0 { // 100% of 1000 stake
		t.Errorf("Expected slash amount 1000.0, got %f", event.Amount)
	}
	if event.Reason != FraudProof {
		t.Errorf("Expected reason FraudProof, got %s", event.Reason)
	}

	// Check reporter reward (20% of 1000.0 = 200.0)
	expectedReward := 1000.0 * 0.2
	if event.ReporterReward != expectedReward {
		t.Errorf("Expected reporter reward %f, got %f", expectedReward, event.ReporterReward)
	}
	if string(event.Reporter) != "node_reporter" {
		t.Errorf("Expected reporter 'node_reporter', got '%s'", string(event.Reporter))
	}

	// Check accumulated reporter reward via GetReporterReward
	reward := engine.GetReporterReward([]byte("node_reporter"))
	if reward != expectedReward {
		t.Errorf("Expected accumulated reporter reward %f, got %f", expectedReward, reward)
	}

	// Check history
	history := engine.GetSlashHistory([]byte("node_offender"))
	if len(history) != 1 {
		t.Errorf("Expected 1 slash event in history, got %d", len(history))
	}

	// Check total slashed
	total := engine.GetTotalSlashed([]byte("node_offender"))
	if total != 1000.0 {
		t.Errorf("Expected total slashed 1000.0, got %f", total)
	}
}

func TestSlashEngine_BanAfterMultipleSlashes(t *testing.T) {
	engine := NewSlashEngine(nil)

	offender := []byte("repeat_offender")

	for i := 0; i < 3; i++ {
		evidence := &Evidence{
			Type:      Misbehavior,
			Offender:  offender,
			Reporter:  []byte("reporter"),
			Timestamp: time.Now().Unix(),
			ProofData: []byte("proof"),
			Severity:  5,
		}
		engine.ExecuteSlash(evidence)
	}

	// After 3 slashes, node should be banned
	if !engine.IsBanned(offender) {
		t.Error("Expected node to be banned after 3 slashes")
	}
}

func TestSlashEngine_SubmitEvidence(t *testing.T) {
	engine := NewSlashEngine(nil)

	evidence := &Evidence{
		Type:      CopyResult,
		Offender:  []byte("copier_node"),
		Reporter:  []byte("honest_node"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("copy_evidence"),
		Severity:  5, // Will be overridden by protocol to 5 (CopyResult base)
	}

	err := engine.SubmitEvidence(evidence)
	if err != nil {
		t.Fatalf("SubmitEvidence failed: %v", err)
	}

	// Verify severity was protocol-determined (CopyResult base = 5)
	if evidence.Severity != 5 {
		t.Errorf("Expected protocol-determined severity 5, got %d", evidence.Severity)
	}

	// Submit FraudProof (should auto-slash because protocol sets severity=9)
	severeEvidence := &Evidence{
		Type:      FraudProof,
		Offender:  []byte("fraud_node"),
		Reporter:  []byte("honest_node"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("fraud_evidence"),
		Severity:  1, // Reporter tries to set low, but protocol overrides to 9
	}

	err = engine.SubmitEvidence(severeEvidence)
	if err != nil {
		t.Fatalf("SubmitEvidence (severe) failed: %v", err)
	}

	// Verify severity was overridden by protocol (FraudProof base = 9)
	if severeEvidence.Severity != 9 {
		t.Errorf("Expected protocol-determined severity 9, got %d", severeEvidence.Severity)
	}

	// Check that severe evidence triggered auto-slash
	history := engine.GetSlashHistory([]byte("fraud_node"))
	if len(history) != 1 {
		t.Errorf("Expected auto-slash for FraudProof (severity=9 >= 8), got %d slashes", len(history))
	}
}

func TestSlashEngine_Appeal(t *testing.T) {
	engine := NewSlashEngine(nil)

	evidence := &Evidence{
		Type:      NoWork,
		Offender:  []byte("lazy_node"),
		Reporter:  []byte("monitor"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("no_work_proof"),
		Severity:  3,
	}

	event, err := engine.ExecuteSlash(evidence)
	if err != nil {
		t.Fatalf("ExecuteSlash failed: %v", err)
	}

	// Should be able to appeal within window
	if !engine.CanAppeal(event) {
		t.Error("Expected to be able to appeal")
	}

	// Submit appeal
	appealEvidence := &Evidence{
		Type:      NoWork,
		Offender:  []byte("lazy_node"),
		Reporter:  []byte("lazy_node"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("appeal_proof"),
		Severity:  1,
	}

	err = engine.Appeal(event.ID, appealEvidence)
	if err != nil {
		t.Fatalf("Appeal failed: %v", err)
	}
}

func TestSlashEngine_Statistics(t *testing.T) {
	engine := NewSlashEngine(nil)

	// Execute some slashes
	types := []ViolationType{FraudProof, CopyResult, NoWork}
	for i, vType := range types {
		evidence := &Evidence{
			Type:      vType,
			Offender:  []byte("node" + string(rune('A'+i))),
			Reporter:  []byte("reporter"),
			Timestamp: time.Now().Unix(),
			ProofData: []byte("proof"),
			Severity:  5,
		}
		engine.ExecuteSlash(evidence)
	}

	stats := engine.GetStatistics()
	if stats.TotalSlashes != 3 {
		t.Errorf("Expected 3 total slashes, got %d", stats.TotalSlashes)
	}
	if stats.TotalAmount <= 0 {
		t.Error("Expected positive total amount")
	}
}

func TestSlashEngine_ExportImport(t *testing.T) {
	engine := NewSlashEngine(nil)

	evidence := &Evidence{
		Type:      FraudProof,
		Offender:  []byte("export_node"),
		Reporter:  []byte("reporter"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  5,
	}
	engine.ExecuteSlash(evidence)

	// Export
	data, err := engine.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Import into new engine
	newEngine := NewSlashEngine(nil)
	err = newEngine.Import(data)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	history := newEngine.GetSlashHistory([]byte("export_node"))
	if len(history) != 1 {
		t.Errorf("Expected 1 slash in imported history, got %d", len(history))
	}
}

func TestEvidenceCollector(t *testing.T) {
	collector := NewEvidenceCollector()

	evidence := &Evidence{
		Type:      CopyResult,
		Offender:  []byte("copier"),
		Reporter:  []byte("reporter"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  5,
	}

	// Submit
	err := collector.Submit(evidence)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if collector.PendingCount() != 1 {
		t.Errorf("Expected 1 pending, got %d", collector.PendingCount())
	}

	// Get pending
	pending := collector.GetPending()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending, got %d", len(pending))
	}

	// Verify
	err = collector.Verify(evidence.ID)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if collector.PendingCount() != 0 {
		t.Errorf("Expected 0 pending after verify, got %d", collector.PendingCount())
	}
	if collector.VerifiedCount() != 1 {
		t.Errorf("Expected 1 verified, got %d", collector.VerifiedCount())
	}
}

func TestEvidenceCollector_Reject(t *testing.T) {
	collector := NewEvidenceCollector()

	evidence := &Evidence{
		Type:      NoWork,
		Offender:  []byte("node"),
		Reporter:  []byte("reporter"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  3,
	}

	collector.Submit(evidence)

	err := collector.Reject(evidence.ID, "insufficient evidence")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	if collector.PendingCount() != 0 {
		t.Errorf("Expected 0 pending after reject, got %d", collector.PendingCount())
	}
}

func TestEvidenceCollector_Validation(t *testing.T) {
	collector := NewEvidenceCollector()

	// Test: nil evidence
	err := collector.Submit(nil)
	if err == nil {
		t.Error("Expected error for nil evidence")
	}

	// Test: empty offender
	err = collector.Submit(&Evidence{
		Type:      FraudProof,
		Offender:  nil,
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  5,
	})
	if err == nil {
		t.Error("Expected error for empty offender")
	}

	// Test: severity out of range
	err = collector.Submit(&Evidence{
		Type:      FraudProof,
		Offender:  []byte("node"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  15,
	})
	if err == nil {
		t.Error("Expected error for severity out of range")
	}
}

func TestEvidenceCollector_ExportImport(t *testing.T) {
	collector := NewEvidenceCollector()

	evidence := &Evidence{
		Type:      FraudProof,
		Offender:  []byte("node"),
		Reporter:  []byte("reporter"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof"),
		Severity:  5,
	}
	collector.Submit(evidence)

	data, err := collector.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	newCollector := NewEvidenceCollector()
	err = newCollector.Import(data)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if newCollector.PendingCount() != 1 {
		t.Errorf("Expected 1 pending in imported collector, got %d", newCollector.PendingCount())
	}
}

func TestSybilDetector(t *testing.T) {
	detector := NewSybilDetector()

	// Create two very similar profiles
	node1 := []byte("node1")
	node2 := []byte("node2")
	node3 := []byte("node3")

	detector.UpdateProfile(node1,
		[]float64{1.0, 2.0, 3.0, 4.0, 5.0},
		[]float64{0.1, 0.2, 0.3, 0.4},
		[]float64{10, 20, 30},
	)

	// Very similar to node1 (potential sybil)
	detector.UpdateProfile(node2,
		[]float64{1.01, 2.01, 3.01, 4.01, 5.01},
		[]float64{0.101, 0.201, 0.301, 0.401},
		[]float64{10.1, 20.1, 30.1},
	)

	// Different from node1
	detector.UpdateProfile(node3,
		[]float64{100, 200, 300, 400, 500},
		[]float64{9.0, 8.0, 7.0, 6.0},
		[]float64{999, 888, 777},
	)

	// Check node1 vs node2 (should be sybil)
	isSybil, similarNode, similarity := detector.DetectSybil(node1)
	if !isSybil {
		t.Errorf("Expected sybil detection for node1 (similarity: %f)", similarity)
	}
	if string(similarNode) != string(node2) {
		t.Error("Expected most similar node to be node2")
	}

	// Get suspicious pairs
	pairs := detector.GetSuspiciousPairs()
	if len(pairs) == 0 {
		t.Error("Expected at least one suspicious pair")
	}
}

// TestSlashEngine_Concurrent tests concurrent slash engine operations
func TestSlashEngine_Concurrent(t *testing.T) {
	engine := NewSlashEngine(nil)
	numGoroutines := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 types of operations

	errors := make(chan error, numGoroutines*3)

	// Concurrent evidence submission
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			offender := []byte(fmt.Sprintf("offender_%d", id%10)) // 10 unique offenders
			reporter := []byte(fmt.Sprintf("reporter_%d", id))

			evidence := &Evidence{
				Type:      CopyResult,
				Offender:  offender,
				Reporter:  reporter,
				Timestamp: time.Now().Unix(),
				ProofData: []byte(fmt.Sprintf("proof_%d", id)),
				Severity:  5,
			}

			if err := engine.SubmitEvidence(evidence); err != nil {
				errors <- fmt.Errorf("submit evidence: %w", err)
			}
		}(i)
	}

	// Concurrent slash execution
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			offender := []byte(fmt.Sprintf("slash_offender_%d", id%10))
			reporter := []byte(fmt.Sprintf("slash_reporter_%d", id))

			evidence := &Evidence{
				Type:      NoWork,
				Offender:  offender,
				Reporter:  reporter,
				Timestamp: time.Now().Unix(),
				ProofData: []byte(fmt.Sprintf("slash_proof_%d", id)),
				Severity:  3,
			}

			_, err := engine.ExecuteSlash(evidence)
			if err != nil {
				// Some may fail due to ban, that's expected
				if !strings.Contains(err.Error(), "banned") && !strings.Contains(err.Error(), "already at max") {
					errors <- fmt.Errorf("execute slash: %w", err)
				}
			}
		}(i)
	}

	// Concurrent queries
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			nodeID := []byte(fmt.Sprintf("offender_%d", id%10))

			// Query history
			_ = engine.GetSlashHistory(nodeID)

			// Query total slashed
			_ = engine.GetTotalSlashed(nodeID)

			// Query banned status
			_ = engine.IsBanned(nodeID)

			// Query statistics
			_ = engine.GetStatistics()
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent slash engine error: %v", err)
	}

	// Verify statistics
	stats := engine.GetStatistics()
	if stats.TotalSlashes == 0 {
		t.Error("Expected some slashes to be executed")
	}
}

// TestEvidenceCollector_Concurrent tests concurrent evidence collector operations
func TestEvidenceCollector_Concurrent(t *testing.T) {
	collector := NewEvidenceCollector()
	numGoroutines := 50
	numSubmissions := 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 types of operations

	errors := make(chan error, numGoroutines*3)

	// Concurrent Submit operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numSubmissions; j++ {
				offender := []byte(fmt.Sprintf("offender_%d_%d", id, j))
				reporter := []byte(fmt.Sprintf("reporter_%d", id))

				evidence := &Evidence{
					Type:      Misbehavior,
					Offender:  offender,
					Reporter:  reporter,
					Timestamp: time.Now().Unix(),
					ProofData: []byte(fmt.Sprintf("proof_%d_%d", id, j)),
					Severity:  4,
				}

				if err := collector.Submit(evidence); err != nil {
					// Queue may fill up, that's expected
					if !strings.Contains(err.Error(), "queue full") && !strings.Contains(err.Error(), "duplicate") {
						errors <- fmt.Errorf("submit evidence: %w", err)
					}
				}
			}
		}(i)
	}

	// Concurrent GetPending and PendingCount operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Get pending count
			count := collector.PendingCount()
			if count < 0 {
				errors <- fmt.Errorf("negative pending count: %d", count)
			}

			// Get pending evidence
			pending := collector.GetPending()
			if pending == nil {
				errors <- fmt.Errorf("pending slice is nil")
			}

			// Get verified count
			verifiedCount := collector.VerifiedCount()
			if verifiedCount < 0 {
				errors <- fmt.Errorf("negative verified count: %d", verifiedCount)
			}
		}(i)
	}

	// Concurrent Verify and Reject operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Try to verify (may fail if not in queue)
			pending := collector.GetPending()
			if len(pending) > 0 {
				evidenceID := pending[0].ID
				_ = collector.Verify(evidenceID)
			}

			// Try to process next
			_, _ = collector.ProcessNext()
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent evidence collector error: %v", err)
	}

	// Verify final state
	pendingCount := collector.PendingCount()
	if pendingCount < 0 {
		t.Errorf("Invalid pending count: %d", pendingCount)
	}
}

// TestSybilDetector_Concurrent tests concurrent sybil detection
func TestSybilDetector_Concurrent(t *testing.T) {
	detector := NewSybilDetector()
	numGoroutines := 50
	numProfiles := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	// Concurrent profile updates and detections
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			nodeID := []byte(fmt.Sprintf("node_%d", id%numProfiles))

			// Update profile
			detector.UpdateProfile(
				nodeID,
				[]float64{1.0, 2.0, 3.0, 4.0, 5.0},
				[]float64{0.1, 0.2, 0.3, 0.4},
				[]float64{10, 20, 30},
			)

			// Detect sybil
			_, _, _ = detector.DetectSybil(nodeID)

			// Get suspicious pairs
			_ = detector.GetSuspiciousPairs()
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent sybil detector error: %v", err)
	}

	// Verify suspicious pairs
	pairs := detector.GetSuspiciousPairs()
	// With all nodes having the same profile, should detect sybils
	if len(pairs) > 0 {
		t.Logf("Detected %d suspicious sybil pairs (expected for identical profiles)", len(pairs))
	}
}
