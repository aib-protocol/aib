package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// StakeManager 管理节点质押
type StakeManager struct {
	mu       sync.RWMutex
	stakes   map[string]*StakeInfo // nodeID -> 质押信息
	minStake float64               // 最低质押要求
}

// StakeInfo 质押信息
type StakeInfo struct {
	NodeID      string      `json:"node_id"`
	Amount      float64     `json:"amount"`       // 质押金额
	LockedUntil int64       `json:"locked_until"` // 锁定截止时间（Unix timestamp）
	Status      StakeStatus `json:"status"`       // 质押状态
	SlashTotal  float64     `json:"slash_total"`  // 累计被罚没金额
	StakeTime   int64       `json:"stake_time"`   // 质押时间
}

// StakeStatus 质押状态
type StakeStatus string

const (
	StakeActive    StakeStatus = "active"    // 活跃状态
	StakeLocked    StakeStatus = "locked"    // 锁定中（解除质押后）
	StakeSlashed   StakeStatus = "slashed"   // 已被罚没
	StakeWithdrawn StakeStatus = "withdrawn" // 已提取
)

// NewStakeManager 创建新的质押管理器
func NewStakeManager(minStake float64) *StakeManager {
	return &StakeManager{
		stakes:   make(map[string]*StakeInfo),
		minStake: minStake,
	}
}

// Stake 质押代币
func (sm *StakeManager) Stake(nodeID string, amount float64) error {
	if nodeID == "" {
		return errors.New("节点ID不能为空")
	}
	if amount <= 0 {
		return errors.New("质押金额必须大于0")
	}
	if amount < sm.minStake {
		return fmt.Errorf("质押金额必须大于最低要求: %.2f", sm.minStake)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 检查是否已质押
	if stake, exists := sm.stakes[nodeID]; exists && stake.Status != StakeWithdrawn {
		return errors.New("节点已质押，请先解除质押")
	}

	// 创建新的质押记录
	sm.stakes[nodeID] = &StakeInfo{
		NodeID:      nodeID,
		Amount:      amount,
		LockedUntil: 0, // 质押时不锁定
		Status:      StakeActive,
		SlashTotal:  0,
		StakeTime:   time.Now().Unix(),
	}

	return nil
}

// Unstake 解除质押（进入锁定期）
func (sm *StakeManager) Unstake(nodeID string) error {
	if nodeID == "" {
		return errors.New("节点ID不能为空")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return errors.New("节点未质押")
	}

	if stake.Status != StakeActive {
		return fmt.Errorf("质押状态不正确: %s", stake.Status)
	}

	// 设置锁定期（例如：7天）
	lockDuration := 7 * 24 * time.Hour
	stake.LockedUntil = time.Now().Add(lockDuration).Unix()
	stake.Status = StakeLocked

	return nil
}

// Slash 罚没质押（由 SlashEngine 调用）
func (sm *StakeManager) Slash(nodeID string, ratio float64) (float64, error) {
	if nodeID == "" {
		return 0, errors.New("节点ID不能为空")
	}
	if ratio < 0 || ratio > 1 {
		return 0, errors.New("罚没比例必须在0到1之间")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return 0, errors.New("节点未质押")
	}

	if stake.Status != StakeActive {
		return 0, fmt.Errorf("节点状态不正确: %s", stake.Status)
	}

	// 计算罚没金额
	slashAmount := stake.Amount * ratio
	stake.Amount -= slashAmount
	stake.SlashTotal += slashAmount

	// 如果全部罚没，标记为已罚没
	if stake.Amount <= 0 {
		stake.Status = StakeSlashed
		stake.Amount = 0
	}

	return slashAmount, nil
}

// GetStake 查询质押信息
func (sm *StakeManager) GetStake(nodeID string) (*StakeInfo, error) {
	if nodeID == "" {
		return nil, errors.New("节点ID不能为空")
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return nil, errors.New("节点未质押")
	}

	// 返回副本
	result := &StakeInfo{
		NodeID:      stake.NodeID,
		Amount:      stake.Amount,
		LockedUntil: stake.LockedUntil,
		Status:      stake.Status,
		SlashTotal:  stake.SlashTotal,
		StakeTime:   stake.StakeTime,
	}

	return result, nil
}

// IsEligible 检查节点是否有资格参与任务
func (sm *StakeManager) IsEligible(nodeID string) bool {
	if nodeID == "" {
		return false
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return false
	}

	// 检查状态是否为活跃
	if stake.Status != StakeActive {
		return false
	}

	// 检查质押金额是否满足最低要求
	if stake.Amount < sm.minStake {
		return false
	}

	return true
}

// GetTotalStaked 获取总质押量
func (sm *StakeManager) GetTotalStaked() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	total := 0.0
	for _, stake := range sm.stakes {
		if stake.Status == StakeActive {
			total += stake.Amount
		}
	}

	return total
}

// Withdraw 提取质押（锁定期结束后）
func (sm *StakeManager) Withdraw(nodeID string) (float64, error) {
	if nodeID == "" {
		return 0, errors.New("节点ID不能为空")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return 0, errors.New("节点未质押")
	}

	if stake.Status != StakeLocked {
		return 0, fmt.Errorf("质押状态不正确: %s", stake.Status)
	}

	// 检查锁定期是否已过
	if time.Now().Unix() < stake.LockedUntil {
		return 0, fmt.Errorf("锁定期未结束，还需等待 %d 秒",
			stake.LockedUntil-time.Now().Unix())
	}

	// 计算可提取金额
	withdrawAmount := stake.Amount

	// 更新状态
	stake.Status = StakeWithdrawn
	stake.Amount = 0

	return withdrawAmount, nil
}

// Export 导出状态
func (sm *StakeManager) Export() ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state := struct {
		MinStake float64               `json:"min_stake"`
		Stakes   map[string]*StakeInfo `json:"stakes"`
	}{
		MinStake: sm.minStake,
		Stakes:   make(map[string]*StakeInfo),
	}

	// 导出副本
	for k, v := range sm.stakes {
		state.Stakes[k] = &StakeInfo{
			NodeID:      v.NodeID,
			Amount:      v.Amount,
			LockedUntil: v.LockedUntil,
			Status:      v.Status,
			SlashTotal:  v.SlashTotal,
			StakeTime:   v.StakeTime,
		}
	}

	return json.Marshal(state)
}

// Import 导入状态
func (sm *StakeManager) Import(data []byte) error {
	var state struct {
		MinStake float64               `json:"min_stake"`
		Stakes   map[string]*StakeInfo `json:"stakes"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.minStake = state.MinStake
	sm.stakes = make(map[string]*StakeInfo)

	for k, v := range state.Stakes {
		sm.stakes[k] = &StakeInfo{
			NodeID:      v.NodeID,
			Amount:      v.Amount,
			LockedUntil: v.LockedUntil,
			Status:      v.Status,
			SlashTotal:  v.SlashTotal,
			StakeTime:   v.StakeTime,
		}
	}

	return nil
}
