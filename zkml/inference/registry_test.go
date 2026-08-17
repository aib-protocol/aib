package inference

import (
	"testing"
	"time"
)

func TestNewModelRegistry(t *testing.T) {
	registry := NewModelRegistry()

	if registry == nil {
		t.Fatal("expected non-nil ModelRegistry")
	}

	if registry.models == nil {
		t.Error("expected non-nil models map")
	}

	if registry.performance == nil {
		t.Error("expected non-nil performance map")
	}

	if registry.votingWeights == nil {
		t.Error("expected non-nil votingWeights map")
	}
}

func TestModelRegistry_RegisterModel(t *testing.T) {
	registry := NewModelRegistry()

	info := &ModelInfo{
		ModelID: "test-model",
		Name:    "Test Model",
		Type:    ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
	}

	err := registry.RegisterModel(info)
	if err != nil {
		t.Fatalf("failed to register model: %v", err)
	}

	// Check that weight was set to default
	if info.Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", info.Weight)
	}

	// Check that model can be retrieved
	retrieved := registry.GetModelInfo("test-model")
	if retrieved == nil {
		t.Error("expected to retrieve registered model")
	}

	if retrieved.ModelID != "test-model" {
		t.Errorf("expected model ID test-model, got %s", retrieved.ModelID)
	}
}

func TestModelRegistry_RegisterModelNil(t *testing.T) {
	registry := NewModelRegistry()

	err := registry.RegisterModel(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModelRegistry_GetModelInfo(t *testing.T) {
	registry := NewModelRegistry()

	// Non-existent model
	info := registry.GetModelInfo("nonexistent")
	if info != nil {
		t.Error("expected nil for non-existent model")
	}

	// Register and retrieve
	registry.RegisterModel(&ModelInfo{
		ModelID: "model1",
		Name:    "Model 1",
	})

	retrieved := registry.GetModelInfo("model1")
	if retrieved == nil {
		t.Error("expected to retrieve registered model")
	}
}

func TestModelRegistry_GetPerformance(t *testing.T) {
	registry := NewModelRegistry()

	// Nil model ID
	perf := registry.GetPerformance(nil)
	if perf != nil {
		t.Error("expected nil for nil model ID")
	}

	// Non-existent model
	perf = registry.GetPerformance([]byte("nonexistent"))
	if perf != nil {
		t.Error("expected nil for non-existent model")
	}

	// Register and check performance
	info := &ModelInfo{
		ModelID: "model1",
		Performance: &ModelPerformance{
			TaskCompletionRate: 0.95,
			UserSatisfaction:   4.5,
			AvgResponseTime:    100 * time.Millisecond,
			CostPerTask:        0.01,
			ReliabilityScore:   0.98,
		},
	}
	registry.RegisterModel(info)

	perf = registry.GetPerformance([]byte("model1"))
	if perf == nil {
		t.Error("expected to retrieve performance")
	}

	if perf.TaskCompletionRate != 0.95 {
		t.Errorf("expected task completion rate 0.95, got %f", perf.TaskCompletionRate)
	}
}

func TestModelRegistry_UpdatePerformance(t *testing.T) {
	registry := NewModelRegistry()

	// Update without registering - should be no-op
	err := registry.UpdatePerformance("nonexistent", &ModelPerformance{
		TaskCompletionRate: 0.9,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Register and update
	registry.RegisterModel(&ModelInfo{ModelID: "model1"})

	newPerf := &ModelPerformance{
		TaskCompletionRate: 0.99,
		UserSatisfaction:   4.8,
	}

	err = registry.UpdatePerformance("model1", newPerf)
	if err != nil {
		t.Fatalf("failed to update performance: %v", err)
	}

	perf := registry.GetPerformance([]byte("model1"))
	if perf.TaskCompletionRate != 0.99 {
		t.Errorf("expected 0.99, got %f", perf.TaskCompletionRate)
	}

	// Update with nil - should be no-op
	err = registry.UpdatePerformance("model1", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModelRegistry_ListModels(t *testing.T) {
	registry := NewModelRegistry()

	// Empty list
	models := registry.ListModels()
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}

	// Register models
	registry.RegisterModel(&ModelInfo{ModelID: "model1", Name: "Model 1"})
	registry.RegisterModel(&ModelInfo{ModelID: "model2", Name: "Model 2"})

	models = registry.ListModels()
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestModelRegistry_GetWeight(t *testing.T) {
	registry := NewModelRegistry()

	// Non-existent model - should return default weight
	weight := registry.GetWeight("nonexistent")
	if weight != 1.0 {
		t.Errorf("expected default weight 1.0, got %f", weight)
	}

	// Register and check default weight
	registry.RegisterModel(&ModelInfo{ModelID: "model1"})
	weight = registry.GetWeight("model1")
	if weight != 1.0 {
		t.Errorf("expected default weight 1.0, got %f", weight)
	}

	// Set custom weight
	registry.SetWeight("model1", 2.5)
	weight = registry.GetWeight("model1")
	if weight != 2.5 {
		t.Errorf("expected weight 2.5, got %f", weight)
	}
}

func TestModelRegistry_SetWeight(t *testing.T) {
	registry := NewModelRegistry()

	// Set weight for non-existent model - should be no-op
	err := registry.SetWeight("nonexistent", 2.0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Register and set weight
	registry.RegisterModel(&ModelInfo{ModelID: "model1"})

	err = registry.SetWeight("model1", 1.5)
	if err != nil {
		t.Fatalf("failed to set weight: %v", err)
	}

	weight := registry.GetWeight("model1")
	if weight != 1.5 {
		t.Errorf("expected weight 1.5, got %f", weight)
	}

	// Set invalid weight - should be no-op
	err = registry.SetWeight("model1", 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = registry.SetWeight("model1", -1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewProposalManager(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	if pm == nil {
		t.Fatal("expected non-nil ProposalManager")
	}

	if pm.proposals == nil {
		t.Error("expected non-nil proposals map")
	}

	if pm.votes == nil {
		t.Error("expected non-nil votes map")
	}

	if pm.registry != registry {
		t.Error("expected registry to be set")
	}
}

func TestProposalManager_SubmitProposal(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	proposal := &GovernanceProposal{
		ID:          "prop-1",
		Type:        ProposalTypeWeightAdjustment,
		Title:       "Test Proposal",
		Description: "Test description",
		Evidence:    map[string]interface{}{"model_id": "model1", "proposed_weight": 2.0},
		Proposer:    "proposer1",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	err := pm.SubmitProposal(proposal)
	if err != nil {
		t.Fatalf("failed to submit proposal: %v", err)
	}

	// Check proposal status was set to active
	if proposal.Status != ProposalStatusActive {
		t.Errorf("expected status active, got %s", proposal.Status)
	}

	// Check proposal can be retrieved
	retrieved := pm.GetProposal("prop-1")
	if retrieved == nil {
		t.Error("expected to retrieve submitted proposal")
	}

	// Submit nil proposal - should be no-op
	err = pm.SubmitProposal(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProposalManager_Vote(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	// Vote on non-existent proposal - should be no-op
	err := pm.Vote("nonexistent", "voter1", true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Submit and vote
	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Deadline: time.Now().Add(24 * time.Hour),
	})

	err = pm.Vote("prop-1", "voter1", true)
	if err != nil {
		t.Fatalf("failed to vote: %v", err)
	}

	err = pm.Vote("prop-1", "voter2", false)
	if err != nil {
		t.Fatalf("failed to vote: %v", err)
	}
}

func TestProposalManager_GetProposal(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	// Non-existent proposal
	proposal := pm.GetProposal("nonexistent")
	if proposal != nil {
		t.Error("expected nil for non-existent proposal")
	}

	// Submit and retrieve
	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Deadline: time.Now().Add(24 * time.Hour),
	})

	proposal = pm.GetProposal("prop-1")
	if proposal == nil {
		t.Error("expected to retrieve submitted proposal")
	}

	if proposal.ID != "prop-1" {
		t.Errorf("expected proposal ID prop-1, got %s", proposal.ID)
	}
}

func TestProposalManager_FinalizeProposal(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	// Register a model
	registry.RegisterModel(&ModelInfo{ModelID: "model1", Weight: 1.0})

	// Finalize non-existent proposal
	proposal, err := pm.FinalizeProposal("nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if proposal != nil {
		t.Error("expected nil for non-existent proposal")
	}

	// Submit proposal with enough yes votes
	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Evidence: map[string]interface{}{"model_id": "model1", "proposed_weight": 2.0},
		Deadline: time.Now().Add(24 * time.Hour),
	})

	// Add votes: 3 yes, 1 no (75% yes - above 67% threshold)
	pm.Vote("prop-1", "voter1", true)
	pm.Vote("prop-1", "voter2", true)
	pm.Vote("prop-1", "voter3", true)
	pm.Vote("prop-1", "voter4", false)

	proposal, err = pm.FinalizeProposal("prop-1")
	if err != nil {
		t.Fatalf("failed to finalize proposal: %v", err)
	}

	if proposal.Status != ProposalStatusPassed {
		t.Errorf("expected passed status, got %s", proposal.Status)
	}

	// Check weight was updated
	weight := registry.GetWeight("model1")
	if weight != 2.0 {
		t.Errorf("expected weight 2.0, got %f", weight)
	}
}

func TestProposalManager_FinalizeProposalRejected(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Deadline: time.Now().Add(24 * time.Hour),
	})

	// Add votes: 1 yes, 2 no (33% yes - below 67% threshold)
	pm.Vote("prop-1", "voter1", true)
	pm.Vote("prop-1", "voter2", false)
	pm.Vote("prop-1", "voter3", false)

	proposal, err := pm.FinalizeProposal("prop-1")
	if err != nil {
		t.Fatalf("failed to finalize proposal: %v", err)
	}

	if proposal.Status != ProposalStatusRejected {
		t.Errorf("expected rejected status, got %s", proposal.Status)
	}
}

func TestProposalManager_FinalizeProposalNoVotes(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Deadline: time.Now().Add(24 * time.Hour),
	})

	// No votes
	proposal, err := pm.FinalizeProposal("prop-1")
	if err != nil {
		t.Fatalf("failed to finalize proposal: %v", err)
	}

	// Should be rejected when no votes
	if proposal.Status != ProposalStatusRejected {
		t.Errorf("expected rejected status, got %s", proposal.Status)
	}
}

func TestProposalManager_ListActiveProposals(t *testing.T) {
	registry := NewModelRegistry()
	pm := NewProposalManager(registry)

	// Empty list
	proposals := pm.ListActiveProposals()
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals, got %d", len(proposals))
	}

	// Submit proposals
	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-1",
		Type:     ProposalTypeWeightAdjustment,
		Status:   ProposalStatusActive,
		Deadline: time.Now().Add(24 * time.Hour),
	})
	pm.SubmitProposal(&GovernanceProposal{
		ID:       "prop-2",
		Type:     ProposalTypeModelRegistration,
		Status:   ProposalStatusActive,
		Deadline: time.Now().Add(24 * time.Hour),
	})

	proposals = pm.ListActiveProposals()
	if len(proposals) != 2 {
		t.Errorf("expected 2 proposals, got %d", len(proposals))
	}
}
