package economy

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RewardDistributor 管理奖励分发
type RewardDistributor struct {
	mu             sync.RWMutex
	balances       map[string]float64 // nodeID -> 余额
	rewardHistory  []*RewardEvent     // 全局奖励历史
	baseReward     float64            // 基础奖励（每任务）
	pocuMultiplier float64            // PoCU 乘数
	maxHistory     int                // 最大历史记录数
}

// RewardEvent 奖励事件
type RewardEvent struct {
	ID        string     `json:"id"`
	NodeID    string     `json:"node_id"`
	Amount    float64    `json:"amount"`
	Type      RewardType `json:"type"`
	TaskID    string     `json:"task_id"`
	Timestamp int64      `json:"timestamp"`
}

// RewardType 奖励类型
type RewardType string

const (
	RewardInference  RewardType = "inference"  // 推理奖励
	RewardValidation RewardType = "validation" // 验证奖励
	RewardReporter   RewardType = "reporter"   // 举报奖励
)

// NewRewardDistributor 创建新的奖励分发器
func NewRewardDistributor(baseReward float64) *RewardDistributor {
	return &RewardDistributor{
		balances:       make(map[string]float64),
		rewardHistory:  make([]*RewardEvent, 0),
		baseReward:     baseReward,
		pocuMultiplier: 1.0, // 默认乘数为 1.0
		maxHistory:     10000,
	}
}

// SetPoCUMultiplier 设置 PoCU 乘数
func (rd *RewardDistributor) SetPoCUMultiplier(multiplier float64) error {
	if multiplier <= 0 {
		return errors.New("PoCU 乘数必须大于0")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.pocuMultiplier = multiplier
	return nil
}

// DistributeTaskReward 分发任务奖励
func (rd *RewardDistributor) DistributeTaskReward(taskID string, nodeIDs []string) error {
	if taskID == "" {
		return errors.New("任务ID不能为空")
	}
	if len(nodeIDs) == 0 {
		return errors.New("节点列表不能为空")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	now := time.Now().Unix()
	// 计算每个节点的奖励金额 = 基础奖励 * PoCU乘数 / 参与节点数
	rewardPerNode := rd.baseReward * rd.pocuMultiplier / float64(len(nodeIDs))

	for _, nodeID := range nodeIDs {
		if nodeID == "" {
			continue
		}

		// 增加余额
		rd.balances[nodeID] += rewardPerNode

		// 记录奖励事件
		event := &RewardEvent{
			ID:        rd.generateEventID(nodeID, taskID, now),
			NodeID:    nodeID,
			Amount:    rewardPerNode,
			Type:      RewardInference,
			TaskID:    taskID,
			Timestamp: now,
		}
		rd.addHistory(event)
	}

	return nil
}

// DistributeValidationReward 分发验证奖励
func (rd *RewardDistributor) DistributeValidationReward(nodeID string, taskID string) error {
	if nodeID == "" {
		return errors.New("节点ID不能为空")
	}
	if taskID == "" {
		return errors.New("任务ID不能为空")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	now := time.Now().Unix()
	// 验证奖励为基础奖励的一半
	amount := rd.baseReward * 0.5

	rd.balances[nodeID] += amount

	event := &RewardEvent{
		ID:        rd.generateEventID(nodeID, taskID, now),
		NodeID:    nodeID,
		Amount:    amount,
		Type:      RewardValidation,
		TaskID:    taskID,
		Timestamp: now,
	}
	rd.addHistory(event)

	return nil
}

// DistributeReporterReward 分发举报奖励
func (rd *RewardDistributor) DistributeReporterReward(nodeID string, amount float64, taskID string) error {
	if nodeID == "" {
		return errors.New("节点ID不能为空")
	}
	if amount <= 0 {
		return errors.New("奖励金额必须大于0")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	now := time.Now().Unix()

	rd.balances[nodeID] += amount

	event := &RewardEvent{
		ID:        rd.generateEventID(nodeID, taskID, now),
		NodeID:    nodeID,
		Amount:    amount,
		Type:      RewardReporter,
		TaskID:    taskID,
		Timestamp: now,
	}
	rd.addHistory(event)

	return nil
}

// GetBalance 查询余额
func (rd *RewardDistributor) GetBalance(nodeID string) float64 {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	return rd.balances[nodeID]
}

// GetHistory 查询节点的奖励历史
func (rd *RewardDistributor) GetHistory(nodeID string) []*RewardEvent {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	var history []*RewardEvent
	for _, event := range rd.rewardHistory {
		if event.NodeID == nodeID {
			// 返回副本
			eventCopy := &RewardEvent{
				ID:        event.ID,
				NodeID:    event.NodeID,
				Amount:    event.Amount,
				Type:      event.Type,
				TaskID:    event.TaskID,
				Timestamp: event.Timestamp,
			}
			history = append(history, eventCopy)
		}
	}

	return history
}

// GetTotalDistributed 获取总分发量
func (rd *RewardDistributor) GetTotalDistributed() float64 {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	total := 0.0
	for _, balance := range rd.balances {
		total += balance
	}

	return total
}

// addHistory 添加历史记录（内部方法，需持有锁）
func (rd *RewardDistributor) addHistory(event *RewardEvent) {
	rd.rewardHistory = append(rd.rewardHistory, event)

	// 超出限制时移除最早的记录
	if len(rd.rewardHistory) > rd.maxHistory {
		rd.rewardHistory = rd.rewardHistory[len(rd.rewardHistory)-rd.maxHistory:]
	}
}

// generateEventID 生成唯一事件ID
func (rd *RewardDistributor) generateEventID(nodeID, taskID string, timestamp int64) string {
	data := fmt.Sprintf("reward:%s:%s:%d:%d",
		nodeID, taskID, timestamp, len(rd.rewardHistory))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

// Export 导出状态
func (rd *RewardDistributor) Export() ([]byte, error) {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	state := struct {
		Balances       map[string]float64 `json:"balances"`
		RewardHistory  []*RewardEvent     `json:"reward_history"`
		BaseReward     float64            `json:"base_reward"`
		PoCUMultiplier float64            `json:"pocu_multiplier"`
		MaxHistory     int                `json:"max_history"`
	}{
		Balances:       make(map[string]float64),
		RewardHistory:  make([]*RewardEvent, len(rd.rewardHistory)),
		BaseReward:     rd.baseReward,
		PoCUMultiplier: rd.pocuMultiplier,
		MaxHistory:     rd.maxHistory,
	}

	// 复制余额
	for k, v := range rd.balances {
		state.Balances[k] = v
	}

	// 复制历史记录
	for i, event := range rd.rewardHistory {
		state.RewardHistory[i] = &RewardEvent{
			ID:        event.ID,
			NodeID:    event.NodeID,
			Amount:    event.Amount,
			Type:      event.Type,
			TaskID:    event.TaskID,
			Timestamp: event.Timestamp,
		}
	}

	return json.Marshal(state)
}

// Import 导入状态
func (rd *RewardDistributor) Import(data []byte) error {
	var state struct {
		Balances       map[string]float64 `json:"balances"`
		RewardHistory  []*RewardEvent     `json:"reward_history"`
		BaseReward     float64            `json:"base_reward"`
		PoCUMultiplier float64            `json:"pocu_multiplier"`
		MaxHistory     int                `json:"max_history"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.balances = make(map[string]float64)
	for k, v := range state.Balances {
		rd.balances[k] = v
	}

	rd.rewardHistory = make([]*RewardEvent, len(state.RewardHistory))
	for i, event := range state.RewardHistory {
		rd.rewardHistory[i] = &RewardEvent{
			ID:        event.ID,
			NodeID:    event.NodeID,
			Amount:    event.Amount,
			Type:      event.Type,
			TaskID:    event.TaskID,
			Timestamp: event.Timestamp,
		}
	}

	rd.baseReward = state.BaseReward
	rd.pocuMultiplier = state.PoCUMultiplier
	rd.maxHistory = state.MaxHistory
	if rd.maxHistory == 0 {
		rd.maxHistory = 10000
	}

	return nil
}