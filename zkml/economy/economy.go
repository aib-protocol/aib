package economy

import (
	"encoding/json"
)

// Economy aggregates staking and rewards
type Economy struct {
	Stakes  *StakeManager
	Rewards *RewardDistributor
}

// NewEconomy creates a new economy model
func NewEconomy(minStake, baseReward float64) *Economy {
	return &Economy{
		Stakes:  NewStakeManager(minStake),
		Rewards: NewRewardDistributor(baseReward),
	}
}

// ProcessTaskCompletion handles reward distribution after task completion
// Checks node eligibility before distributing rewards
func (e *Economy) ProcessTaskCompletion(taskID string, nodeIDs []string) ([]string, error) {
	// filter out eligible nodes
	eligibleNodes := make([]string, 0)
	for _, nodeID := range nodeIDs {
		if e.Stakes.IsEligible(nodeID) {
			eligibleNodes = append(eligibleNodes, nodeID)
		}
	}

	if len(eligibleNodes) == 0 {
		return nil, nil
	}

	// distribute rewards to eligible nodes
	if err := e.Rewards.DistributeTaskReward(taskID, eligibleNodes); err != nil {
		return eligibleNodes, err
	}

	return eligibleNodes, nil
}

// ProcessSlash handles slashing and distributes the reporter reward
// Returns: slash amount, reporter reward amount, error
func (e *Economy) ProcessSlash(offenderID string, reporterID string, ratio float64, taskID string) (float64, float64, error) {
	// execute the slash
	slashAmount, err := e.Stakes.Slash(offenderID, ratio)
	if err != nil {
		return 0, 0, err
	}

	// no reporter means no reporter reward
	if reporterID == "" {
		return slashAmount, 0, nil
	}

	// distribute the reporter reward (20% of the slashed amount goes to the reporter)
	reporterRewardRatio := 0.2
	reporterReward := slashAmount * reporterRewardRatio

	if reporterReward > 0 {
		if err := e.Rewards.DistributeReporterReward(reporterID, reporterReward, taskID); err != nil {
			return slashAmount, 0, err
		}
	}

	return slashAmount, reporterReward, nil
}

// GetNodeSummary returns the economic summary of a node
func (e *Economy) GetNodeSummary(nodeID string) *NodeSummary {
	summary := &NodeSummary{
		NodeID:   nodeID,
		Eligible: e.Stakes.IsEligible(nodeID),
		Balance:  e.Rewards.GetBalance(nodeID),
	}

	if stake, err := e.Stakes.GetStake(nodeID); err == nil {
		summary.StakeAmount = stake.Amount
		summary.StakeStatus = stake.Status
		summary.SlashTotal = stake.SlashTotal
	}

	return summary
}

// NodeSummary is the economic summary of a node
type NodeSummary struct {
	NodeID      string      `json:"node_id"`
	StakeAmount float64     `json:"stake_amount"`
	StakeStatus StakeStatus `json:"stake_status"`
	SlashTotal  float64     `json:"slash_total"`
	Balance     float64     `json:"balance"`
	Eligible    bool        `json:"eligible"`
}

// Export exports the overall economy state
func (e *Economy) Export() ([]byte, error) {
	stakeData, err := e.Stakes.Export()
	if err != nil {
		return nil, err
	}

	rewardData, err := e.Rewards.Export()
	if err != nil {
		return nil, err
	}

	state := struct {
		Stakes  json.RawMessage `json:"stakes"`
		Rewards json.RawMessage `json:"rewards"`
	}{
		Stakes:  stakeData,
		Rewards: rewardData,
	}

	return json.Marshal(state)
}

// Import imports the overall economy state
func (e *Economy) Import(data []byte) error {
	var state struct {
		Stakes  json.RawMessage `json:"stakes"`
		Rewards json.RawMessage `json:"rewards"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if err := e.Stakes.Import(state.Stakes); err != nil {
		return err
	}

	if err := e.Rewards.Import(state.Rewards); err != nil {
		return err
	}

	return nil
}
