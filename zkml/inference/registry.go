package inference

import (
	"sync"
)

// ModelRegistry is the model registry.
//
// It maintains information, weights, and performance metrics for all registered models.
// Registering a new model is permissionless;
// weights are determined through governance.
type ModelRegistry struct {
	mu            sync.RWMutex
	models        map[string]*ModelInfo        // modelID -> ModelInfo
	performance   map[string]*ModelPerformance // modelID -> performance data
	votingWeights map[string]float64           // modelID -> weight in voting
}

// NewModelRegistry creates a new model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:        make(map[string]*ModelInfo),
		performance:   make(map[string]*ModelPerformance),
		votingWeights: make(map[string]float64),
	}
}

// RegisterModel registers a new model.
//
// Anyone can register (permissionless);
// the initial weight is 1.0 (baseline).
func (r *ModelRegistry) RegisterModel(info *ModelInfo) error {
	if info == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.Weight = 1.0 // initial weight
	if info.Performance == nil {
		info.Performance = &ModelPerformance{}
	}

	r.models[info.ModelID] = info
	r.performance[info.ModelID] = info.Performance
	r.votingWeights[info.ModelID] = 1.0 // initial voting weight

	return nil
}

// GetModelInfo returns the model info.
func (r *ModelRegistry) GetModelInfo(modelID string) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.models[modelID]
}

// GetPerformance returns the model performance data.
func (r *ModelRegistry) GetPerformance(modelID []byte) *ModelPerformance {
	if modelID == nil {
		return nil
	}

	modelIDStr := string(modelID)
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.performance[modelIDStr]
}

// UpdatePerformance updates the model performance data.
//
// Nodes periodically submit performance data.
func (r *ModelRegistry) UpdatePerformance(modelID string, perf *ModelPerformance) error {
	if perf == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.models[modelID] == nil {
		return nil // model not registered
	}

	r.performance[modelID] = perf
	r.models[modelID].Performance = perf

	return nil
}

// ListModels lists all registered models.
func (r *ModelRegistry) ListModels() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelInfo, 0, len(r.models))
	for _, info := range r.models {
		result = append(result, info)
	}

	return result
}

// GetWeight returns the model's current weight.
func (r *ModelRegistry) GetWeight(modelID string) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if info, ok := r.models[modelID]; ok {
		return info.Weight
	}
	return 1.0 // default weight
}

// SetWeight sets the model weight.
//
// May only be called through a governance proposal.
func (r *ModelRegistry) SetWeight(modelID string, weight float64) error {
	if weight <= 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.models[modelID] == nil {
		return nil
	}

	r.models[modelID].Weight = weight
	r.votingWeights[modelID] = weight // update voting weight in sync

	return nil
}

// ProposalManager is the governance proposal manager.

type ProposalManager struct {
	mu        sync.RWMutex
	proposals map[string]*GovernanceProposal
	votes     map[string][]*GovernanceVote // proposalID -> votes
	registry  *ModelRegistry
}

// NewProposalManager creates a proposal manager.
func NewProposalManager(registry *ModelRegistry) *ProposalManager {
	return &ProposalManager{
		proposals: make(map[string]*GovernanceProposal),
		votes:     make(map[string][]*GovernanceVote),
		registry:  registry,
	}
}

// SubmitProposal submits a governance proposal.
//
// Anyone can submit, based on real data.
func (pm *ProposalManager) SubmitProposal(proposal *GovernanceProposal) error {
	if proposal == nil {
		return nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal.Status = ProposalStatusActive
	pm.proposals[proposal.ID] = proposal
	pm.votes[proposal.ID] = make([]*GovernanceVote, 0)

	return nil
}

// Vote casts a vote on a proposal.
//
// Holding AIB tokens is required to vote.
func (pm *ProposalManager) Vote(proposalID, voter string, vote bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal := pm.proposals[proposalID]
	if proposal == nil {
		return nil // proposal does not exist
	}

	if proposal.Status != ProposalStatusActive {
		return nil // proposal is not in the voting period
	}

	// add vote
	governanceVote := &GovernanceVote{
		ProposalID: proposalID,
		Voter:      voter,
		Vote:       vote,
		Timestamp:  proposal.Deadline,
	}

	pm.votes[proposalID] = append(pm.votes[proposalID], governanceVote)

	return nil
}

// GetProposal returns the proposal info.
func (pm *ProposalManager) GetProposal(proposalID string) *GovernanceProposal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.proposals[proposalID]
}

// FinalizeProposal completes a proposal.
//
// It checks the voting result and executes automatically.
func (pm *ProposalManager) FinalizeProposal(proposalID string) (*GovernanceProposal, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal := pm.proposals[proposalID]
	if proposal == nil {
		return nil, nil
	}

	votes := pm.votes[proposalID]

	// tally votes
	yesCount := 0
	totalCount := 0

	for _, v := range votes {
		totalCount++
		if v.Vote {
			yesCount++
		}
	}

	// check threshold (67% majority)
	if totalCount > 0 && float64(yesCount)/float64(totalCount) >= 0.67 {
		proposal.Status = ProposalStatusPassed

		// execute the proposal
		switch proposal.Type {
		case ProposalTypeWeightAdjustment:
			modelID, _ := proposal.Evidence["model_id"].(string)
			weight, _ := proposal.Evidence["proposed_weight"].(float64)
			pm.registry.SetWeight(modelID, weight)
		}

		return proposal, nil
	}

	proposal.Status = ProposalStatusRejected
	return proposal, nil
}

// ListActiveProposals lists active proposals.
func (pm *ProposalManager) ListActiveProposals() []*GovernanceProposal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*GovernanceProposal, 0)
	for _, p := range pm.proposals {
		if p.Status == ProposalStatusActive {
			result = append(result, p)
		}
	}

	return result
}
