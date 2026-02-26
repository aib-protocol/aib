package economy

import (
	"encoding/json"
)

// Economy 聚合质押和奖励
type Economy struct {
	Stakes  *StakeManager
	Rewards *RewardDistributor
}

// NewEconomy 创建新的经济模型
func NewEconomy(minStake, baseReward float64) *Economy {
	return &Economy{
		Stakes:  NewStakeManager(minStake),
		Rewards: NewRewardDistributor(baseReward),
	}
}

// ProcessTaskCompletion 处理任务完成后的奖励分发
// 检查节点资格后分发奖励
func (e *Economy) ProcessTaskCompletion(taskID string, nodeIDs []string) ([]string, error) {
	// 过滤出有资格的节点
	eligibleNodes := make([]string, 0)
	for _, nodeID := range nodeIDs {
		if e.Stakes.IsEligible(nodeID) {
			eligibleNodes = append(eligibleNodes, nodeID)
		}
	}

	if len(eligibleNodes) == 0 {
		return nil, nil
	}

	// 给有资格的节点分发奖励
	if err := e.Rewards.DistributeTaskReward(taskID, eligibleNodes); err != nil {
		return eligibleNodes, err
	}

	return eligibleNodes, nil
}

// ProcessSlash 处理罚没并分发举报奖励
// 返回：罚没金额，举报奖励金额，error
func (e *Economy) ProcessSlash(offenderID string, reporterID string, ratio float64, taskID string) (float64, float64, error) {
	// 执行罚没
	slashAmount, err := e.Stakes.Slash(offenderID, ratio)
	if err != nil {
		return 0, 0, err
	}

	// 没有举报者则不分发举报奖励
	if reporterID == "" {
		return slashAmount, 0, nil
	}

	// 分发举报奖励（罚没金额的 20% 作为举报奖励）
	reporterRewardRatio := 0.2
	reporterReward := slashAmount * reporterRewardRatio

	if reporterReward > 0 {
		if err := e.Rewards.DistributeReporterReward(reporterID, reporterReward, taskID); err != nil {
			return slashAmount, 0, err
		}
	}

	return slashAmount, reporterReward, nil
}

// GetNodeSummary 获取节点经济摘要
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

// NodeSummary 节点经济摘要
type NodeSummary struct {
	NodeID      string      `json:"node_id"`
	StakeAmount float64     `json:"stake_amount"`
	StakeStatus StakeStatus `json:"stake_status"`
	SlashTotal  float64     `json:"slash_total"`
	Balance     float64     `json:"balance"`
	Eligible    bool        `json:"eligible"`
}

// Export 导出整体经济状态
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

// Import 导入整体经济状态
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