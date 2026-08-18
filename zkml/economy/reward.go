package economy

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RewardDistributor manages reward distribution
type RewardDistributor struct {
	mu             sync.RWMutex
	balances       map[string]float64 // nodeID -> balance
	rewardHistory  []*RewardEvent     // global reward history
	baseReward     float64            // base reward (per task)
	pocuMultiplier float64            // PoCU multiplier
	maxHistory     int                // maximum number of history records
}

// RewardEvent rewardevent
type RewardEvent struct {
	ID        string     `json:"id"`
	NodeID    string     `json:"node_id"`
	Amount    float64    `json:"amount"`
	Type      RewardType `json:"type"`
	TaskID    string     `json:"task_id"`
	Timestamp int64      `json:"timestamp"`
}

// RewardType rewardtype
type RewardType string

const (
	RewardInference  RewardType = "inference"  // inference reward
	RewardValidation RewardType = "validation" // validation reward
	RewardReporter   RewardType = "reporter"   // reporter reward
)

// NewRewardDistributor creates a new reward distributor
func NewRewardDistributor(baseReward float64) *RewardDistributor {
	return &RewardDistributor{
		balances:       make(map[string]float64),
		rewardHistory:  make([]*RewardEvent, 0),
		baseReward:     baseReward,
		pocuMultiplier: 1.0, // default multiplier is 1.0
		maxHistory:     10000,
	}
}

// SetPoCUMultiplier sets the PoCU multiplier
func (rd *RewardDistributor) SetPoCUMultiplier(multiplier float64) error {
	if multiplier <= 0 {
		return errors.New("PoCU multiplier must be greater than 0")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.pocuMultiplier = multiplier
	return nil
}

// DistributeTaskReward distributes task rewards
func (rd *RewardDistributor) DistributeTaskReward(taskID string, nodeIDs []string) error {
	if taskID == "" {
		return errors.New("task ID cannot be empty")
	}
	if len(nodeIDs) == 0 {
		return errors.New("node list cannot be empty")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	now := time.Now().Unix()
	// reward per node = base reward * PoCU multiplier / number of participating nodes
	rewardPerNode := rd.baseReward * rd.pocuMultiplier / float64(len(nodeIDs))

	for _, nodeID := range nodeIDs {
		if nodeID == "" {
			continue
		}

		// add to balance
		rd.balances[nodeID] += rewardPerNode

		// record reward event
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

// DistributeValidationReward distributeverifyreward
func (rd *RewardDistributor) DistributeValidationReward(nodeID string, taskID string) error {
	if nodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if taskID == "" {
		return errors.New("task ID cannot be empty")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	now := time.Now().Unix()
	// validation reward is half of the base reward
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

// DistributeReporterReward distributereportreward
func (rd *RewardDistributor) DistributeReporterReward(nodeID string, amount float64, taskID string) error {
	if nodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if amount <= 0 {
		return errors.New("reward amount must be greater than 0")
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

// GetBalance querybalance
func (rd *RewardDistributor) GetBalance(nodeID string) float64 {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	return rd.balances[nodeID]
}

// GetHistory querynoderewardhistory
func (rd *RewardDistributor) GetHistory(nodeID string) []*RewardEvent {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	var history []*RewardEvent
	for _, event := range rd.rewardHistory {
		if event.NodeID == nodeID {
			// return a copy
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

// GetTotalDistributed returns the total amount distributed
func (rd *RewardDistributor) GetTotalDistributed() float64 {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	total := 0.0
	for _, balance := range rd.balances {
		total += balance
	}

	return total
}

// addHistory appends a history record (internal method, must hold the lock)
func (rd *RewardDistributor) addHistory(event *RewardEvent) {
	rd.rewardHistory = append(rd.rewardHistory, event)

	// remove the oldest records when the limit is exceeded
	if len(rd.rewardHistory) > rd.maxHistory {
		rd.rewardHistory = rd.rewardHistory[len(rd.rewardHistory)-rd.maxHistory:]
	}
}

// generateEventID generates a unique event ID
func (rd *RewardDistributor) generateEventID(nodeID, taskID string, timestamp int64) string {
	data := fmt.Sprintf("reward:%s:%s:%d:%d",
		nodeID, taskID, timestamp, len(rd.rewardHistory))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

// Export exportstatus
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

	// copy balances
	for k, v := range rd.balances {
		state.Balances[k] = v
	}

	// copy history records
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

// Import importstatus
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
