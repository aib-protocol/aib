package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// StakeManager manages node staking
type StakeManager struct {
	mu       sync.RWMutex
	stakes   map[string]*StakeInfo // nodeID -> stake info
	minStake float64               // minimum stake requirement
}

// StakeInfo stake / stakinginfo
type StakeInfo struct {
	NodeID      string      `json:"node_id"`
	Amount      float64     `json:"amount"`       // staked amount
	LockedUntil int64       `json:"locked_until"` // lock deadline (Unix timestamp)
	Status      StakeStatus `json:"status"`       // stake status
	SlashTotal  float64     `json:"slash_total"`  // total slashed amount
	StakeTime   int64       `json:"stake_time"`   // stake time
}

// StakeStatus stakestatus
type StakeStatus string

const (
	StakeActive    StakeStatus = "active"    // active
	StakeLocked    StakeStatus = "locked"    // locked (after unstaking)
	StakeSlashed   StakeStatus = "slashed"   // slashed
	StakeWithdrawn StakeStatus = "withdrawn" // withdrawn
)

// NewStakeManager creates a new stake manager
func NewStakeManager(minStake float64) *StakeManager {
	return &StakeManager{
		stakes:   make(map[string]*StakeInfo),
		minStake: minStake,
	}
}

// Stake staketoken
func (sm *StakeManager) Stake(nodeID string, amount float64) error {
	if nodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if amount <= 0 {
		return errors.New("stake amount must be greater than 0")
	}
	if amount < sm.minStake {
		return fmt.Errorf("stake amount must exceed the minimum requirement: %.2f", sm.minStake)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// check if already staked
	if stake, exists := sm.stakes[nodeID]; exists && stake.Status != StakeWithdrawn {
		return errors.New("node already staked; unstake first")
	}

	// create a new stake record
	sm.stakes[nodeID] = &StakeInfo{
		NodeID:      nodeID,
		Amount:      amount,
		LockedUntil: 0, // not locked at stake time
		Status:      StakeActive,
		SlashTotal:  0,
		StakeTime:   time.Now().Unix(),
	}

	return nil
}

// Unstake unstake (enters lock period)
func (sm *StakeManager) Unstake(nodeID string) error {
	if nodeID == "" {
		return errors.New("node ID cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return errors.New("node is not staked")
	}

	if stake.Status != StakeActive {
		return fmt.Errorf("invalid stake status: %s", stake.Status)
	}

	// set lock period (e.g., 7 days)
	lockDuration := 7 * 24 * time.Hour
	stake.LockedUntil = time.Now().Add(lockDuration).Unix()
	stake.Status = StakeLocked

	return nil
}

// Slash slash stake (called by SlashEngine)
func (sm *StakeManager) Slash(nodeID string, ratio float64) (float64, error) {
	if nodeID == "" {
		return 0, errors.New("node ID cannot be empty")
	}
	if ratio < 0 || ratio > 1 {
		return 0, errors.New("slash ratio must be between 0 and 1")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return 0, errors.New("node is not staked")
	}

	if stake.Status != StakeActive {
		return 0, fmt.Errorf("invalid node status: %s", stake.Status)
	}

	// computeslashamount
	slashAmount := stake.Amount * ratio
	stake.Amount -= slashAmount
	stake.SlashTotal += slashAmount

	// if fully slashed, mark as slashed
	if stake.Amount <= 0 {
		stake.Status = StakeSlashed
		stake.Amount = 0
	}

	return slashAmount, nil
}

// GetStake querystake / stakinginfo
func (sm *StakeManager) GetStake(nodeID string) (*StakeInfo, error) {
	if nodeID == "" {
		return nil, errors.New("node ID cannot be empty")
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return nil, errors.New("node is not staked")
	}

	// return a copy
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

// IsEligible checks whether a node is eligible for tasks
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

	// check that status is active
	if stake.Status != StakeActive {
		return false
	}

	// check that staked amount meets the minimum requirement
	if stake.Amount < sm.minStake {
		return false
	}

	return true
}

// GetTotalStaked gets total staked amount
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

// Withdraw withdraw stake (after lock period ends)
func (sm *StakeManager) Withdraw(nodeID string) (float64, error) {
	if nodeID == "" {
		return 0, errors.New("node ID cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, exists := sm.stakes[nodeID]
	if !exists {
		return 0, errors.New("node is not staked")
	}

	if stake.Status != StakeLocked {
		return 0, fmt.Errorf("invalid stake status: %s", stake.Status)
	}

	// check whether the lock period has ended
	if time.Now().Unix() < stake.LockedUntil {
		return 0, fmt.Errorf("lock period not over, %d seconds remaining",
			stake.LockedUntil-time.Now().Unix())
	}

	// calculate withdrawable amount
	withdrawAmount := stake.Amount

	// updatestatus
	stake.Status = StakeWithdrawn
	stake.Amount = 0

	return withdrawAmount, nil
}

// Export exportstatus
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

	// export a copy
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

// Import importstatus
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
