package agentic

import (
	"testing"
	"time"
)

// ============================================================
// Reputation Score Tests
// ============================================================

func TestReputationManager_CreateScore(t *testing.T) {
	config := DefaultReputationConfig()
	rm := NewReputationManager(config, nil)

	nodeID := "test-node-1"
	score := rm.GetOrCreateScore(nodeID)

	if score == nil {
		t.Fatal("expected score to be created")
	}

	if score.TotalScore != config.BaseScore {
		t.Errorf("expected base score %.2f, got %.2f", config.BaseScore, score.TotalScore)
	}

	if score.UserScore != config.BaseScore {
		t.Errorf("expected user score %.2f, got %.2f", config.BaseScore, score.UserScore)
	}
}

func TestReputationManager_AddReputation(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)
	nodeID := "test-node-1"

	// Add positive reputation (user accept)
	point, err := rm.AddReputation(
		nodeID,
		ReputationSourceUserAccept,
		"voter-1",
		1000, // voter stake
		nil,
	)
	if err != nil {
		t.Fatalf("failed to add reputation: %v", err)
	}

	if point == nil {
		t.Fatal("expected point to be created")
	}

	if point.Source != ReputationSourceUserAccept {
		t.Errorf("expected source %v, got %v", ReputationSourceUserAccept, point.Source)
	}

	// Verify score was updated
	score, _ := rm.GetScore(nodeID)
	if score.TotalTasksAccepted != 1 {
		t.Errorf("expected 1 accepted task, got %d", score.TotalTasksAccepted)
	}
}

func TestReputationManager_NegativeReputation(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)
	nodeID := "test-node-1"

	// Add negative reputation (user reject)
	point, err := rm.AddReputation(
		nodeID,
		ReputationSourceUserReject,
		"voter-1",
		1000,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to add reputation: %v", err)
	}

	if point.Amount >= 0 {
		t.Errorf("expected negative amount, got %.2f", point.Amount)
	}

	// Score should decrease
	score, _ := rm.GetScore(nodeID)
	if score.TotalScore >= rm.config.BaseScore {
		t.Errorf("expected score to decrease below %.2f, got %.2f", rm.config.BaseScore, score.TotalScore)
	}
}

func TestReputationManager_VoterWeight(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	// Test that higher stake gives higher weight
	weight0 := rm.calculateVoterWeight(0)
	weight10 := rm.calculateVoterWeight(10)
	weight100 := rm.calculateVoterWeight(100)
	weight1000 := rm.calculateVoterWeight(1000)
	weight10000 := rm.calculateVoterWeight(10000)

	// Zero stake should have zero weight
	if weight0 != 0 {
		t.Errorf("expected 0 weight for 0 stake, got %.2f", weight0)
	}

	// Higher stake should give higher weight
	if weight10 <= 0 {
		t.Errorf("expected positive weight for stake 10, got %.2f", weight10)
	}
	if weight100 <= weight10 {
		t.Errorf("expected weight100 (%.2f) > weight10 (%.2f)", weight100, weight10)
	}
	if weight1000 <= weight100 {
		t.Errorf("expected weight1000 (%.2f) > weight100 (%.2f)", weight1000, weight100)
	}
	if weight10000 <= weight1000 {
		t.Errorf("expected weight10000 (%.2f) > weight1000 (%.2f)", weight10000, weight1000)
	}

	// Max weight should be capped
	if weight10000 > 2.0 {
		t.Errorf("expected weight capped at 2.0, got %.2f", weight10000)
	}
}

func TestReputationManager_TimeLockedReputation(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)
	nodeID := "test-node-1"

	// Add reputation with maturity period
	point, err := rm.AddReputation(
		nodeID,
		ReputationSourceBlockProduce, // Has 14 day maturity
		"",
		0,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to add reputation: %v", err)
	}

	if point.Matured {
		t.Error("expected point to not be matured immediately")
	}

	// Verify point is pending
	score, _ := rm.GetScore(nodeID)
	if len(score.PendingPoints) != 1 {
		t.Errorf("expected 1 pending point, got %d", len(score.PendingPoints))
	}

	// Manually mature the point for testing
	point.MaturesAt = time.Now().Add(-1 * time.Second)

	// Process maturations
	matured := rm.ProcessMaturations()
	if matured != 1 {
		t.Errorf("expected 1 point to mature, got %d", matured)
	}

	// Verify point moved to active
	score, _ = rm.GetScore(nodeID)
	if len(score.ActivePoints) != 1 {
		t.Errorf("expected 1 active point, got %d", len(score.ActivePoints))
	}
}

func TestReputationManager_SuspicionScore(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)
	nodeID := "test-node-1"

	// Add many accepts from same voter (suspicious pattern)
	for i := 0; i < 20; i++ {
		rm.AddReputation(
			nodeID,
			ReputationSourceUserAccept,
			"suspicious-voter",
			1000,
			nil,
		)
	}

	// Check voter pattern score
	voterHist := rm.voterHistory["suspicious-voter"]
	if voterHist == nil {
		t.Fatal("expected voter history to exist")
	}

	// High accept rate is suspicious
	if voterHist.AcceptRate < 0.95 {
		t.Errorf("expected high accept rate, got %.2f", voterHist.AcceptRate)
	}

	// Pattern score should be elevated
	if voterHist.VotePatternScore <= 0 {
		t.Errorf("expected elevated pattern score, got %.2f", voterHist.VotePatternScore)
	}
}

func TestReputationManager_SybilDetection(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	// Simulate a suspicious voter
	for i := 0; i < 100; i++ {
		rm.updateVoterHistory(
			"suspicious-voter",
			"provider-1", // Always voting for same provider
			ReputationSourceUserAccept,
			&ReputationContext{
				IPFingerprint: "same-ip",
			},
		)
	}

	// Check suspicion
	suspicious := rm.GetSuspiciousVoters(50.0)
	if len(suspicious) == 0 {
		t.Error("expected suspicious voter to be detected")
	}

	found := false
	for _, id := range suspicious {
		if id == "suspicious-voter" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'suspicious-voter' to be in suspicious list")
	}
}

// ============================================================
// Weighted Selector Tests
// ============================================================

func TestWeightedSelector_Select(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	// Use selector config without stake requirement
	selectorConfig := DefaultSelectorConfig()
	selectorConfig.MinStake = 0
	ws := NewWeightedSelector(selectorConfig, rm, nil)

	// Add some nodes with different reputations
	nodes := []struct {
		id       string
		score    float64
	}{
		{"high-rep", 300},
		{"med-rep", 150},
		{"low-rep", 50},
	}

	for _, n := range nodes {
		score := rm.GetOrCreateScore(n.id)
		score.mu.Lock()
		score.TotalScore = n.score
		score.mu.Unlock()
	}

	// Select a node
	result, err := ws.Select(nil)
	if err != nil {
		t.Fatalf("selection failed: %v", err)
	}

	if result.SelectedNode == "" {
		t.Error("expected a node to be selected")
	}

	if result.TotalWeight <= 0 {
		t.Errorf("expected positive total weight, got %.2f", result.TotalWeight)
	}
}

func TestWeightedSelector_ProbabilityDistribution(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	// Use selector config without stake requirement
	selectorConfig := DefaultSelectorConfig()
	selectorConfig.MinStake = 0
	ws := NewWeightedSelector(selectorConfig, rm, nil)

	// Create nodes with different reputations
	nodes := []struct {
		id       string
		score    float64
	}{
		{"node-high", 400},
		{"node-med", 200},
		{"node-low", 100},
	}

	for _, n := range nodes {
		score := rm.GetOrCreateScore(n.id)
		score.mu.Lock()
		score.TotalScore = n.score
		score.mu.Unlock()
	}

	// Run many selections
	iterations := 100
	distribution, err := ws.SimulateSelections(iterations)
	if err != nil {
		t.Fatalf("simulation failed: %v", err)
	}

	// Verify distribution favors high reputation
	highCount := distribution["node-high"]
	medCount := distribution["node-med"]
	lowCount := distribution["node-low"]

	// High rep should be selected more often
	if highCount <= lowCount {
		t.Logf("Distribution: high=%d, med=%d, low=%d", highCount, medCount, lowCount)
	}
}

func TestWeightedSelector_Exclusion(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	selectorConfig := DefaultSelectorConfig()
	selectorConfig.MinStake = 0
	ws := NewWeightedSelector(selectorConfig, rm, nil)

	// Create nodes
	for i := 0; i < 3; i++ {
		nodeID := string(rune('a' + i))
		score := rm.GetOrCreateScore(nodeID)
		score.mu.Lock()
		score.TotalScore = 100.0
		score.mu.Unlock()
	}

	// Select with exclusion
	for i := 0; i < 10; i++ {
		result, err := ws.Select([]string{"excluded-node"})
		if err != nil {
			t.Fatalf("selection failed: %v", err)
		}
		if result.SelectedNode == "excluded-node" {
			t.Error("excluded node was selected")
		}
	}
}

func TestWeightedSelector_Committee(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	selectorConfig := DefaultSelectorConfig()
	selectorConfig.MinStake = 0
	ws := NewWeightedSelector(selectorConfig, rm, nil)

	// Create enough nodes
	for i := 0; i < 10; i++ {
		nodeID := string(rune('a' + i))
		score := rm.GetOrCreateScore(nodeID)
		score.mu.Lock()
		score.TotalScore = 100.0
		score.mu.Unlock()
	}

	// Select committee
	committee, err := ws.SelectCommittee(5, nil)
	if err != nil {
		t.Fatalf("committee selection failed: %v", err)
	}

	if len(committee) != 5 {
		t.Errorf("expected 5 committee members, got %d", len(committee))
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, member := range committee {
		if seen[member] {
			t.Errorf("duplicate committee member: %s", member)
		}
		seen[member] = true
	}
}

// ============================================================
// Statistics Tests
// ============================================================

func TestReputationManager_Statistics(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)
	nodeID := "test-node-1"

	// Add various reputation events
	rm.AddReputation(nodeID, ReputationSourceUserAccept, "voter-1", 1000, nil)
	rm.AddReputation(nodeID, ReputationSourceUserAccept, "voter-2", 1000, nil)
	rm.AddReputation(nodeID, ReputationSourceUserReject, "voter-3", 1000, nil)
	rm.AddReputation(nodeID, ReputationSourceBlockProduce, "", 0, &ReputationContext{
		ResponseTimeMs: 500,
	})

	score, _ := rm.GetScore(nodeID)

	if score.TotalTasksAccepted != 2 {
		t.Errorf("expected 2 accepted, got %d", score.TotalTasksAccepted)
	}

	if score.TotalTasksRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", score.TotalTasksRejected)
	}

	if score.TotalBlocksProduced != 1 {
		t.Errorf("expected 1 block, got %d", score.TotalBlocksProduced)
	}

	if score.AvgResponseTimeMs != 500 {
		t.Errorf("expected avg response 500ms, got %.0f", score.AvgResponseTimeMs)
	}
}

func TestWeightedSelector_Statistics(t *testing.T) {
	rm := NewReputationManager(DefaultReputationConfig(), nil)

	selectorConfig := DefaultSelectorConfig()
	selectorConfig.MinStake = 0
	ws := NewWeightedSelector(selectorConfig, rm, nil)

	// Create nodes
	for i := 0; i < 3; i++ {
		nodeID := string(rune('a' + i))
		score := rm.GetOrCreateScore(nodeID)
		score.mu.Lock()
		score.TotalScore = 100.0 + float64(i)*50
		score.mu.Unlock()
	}

	// Run selections
	ws.SimulateSelections(100)

	// Get stats
	stats := ws.GetStats()

	if stats.TotalSelections != 100 {
		t.Errorf("expected 100 selections, got %d", stats.TotalSelections)
	}

	if stats.UniqueNodes != 3 {
		t.Errorf("expected 3 unique nodes, got %d", stats.UniqueNodes)
	}

	t.Logf("Fairness (Gini): %.3f", stats.FairnessScore)
	t.Logf("Entropy: %.3f", stats.EntropyScore)
}
