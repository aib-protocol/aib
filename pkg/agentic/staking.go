// Package agentic provides AI service layer with standard API compatibility.
package agentic

import (
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// StakingManager manages staking and slashing operations.
type StakingManager struct {
	mu           sync.RWMutex
	stakes       map[interfaces.NodeID]*StakeInfo
	pending      map[interfaces.NodeID]*StakeInfo
	slashRecords []*SlashRecord

	config *Config
}

// NewStakingManager creates a new staking manager.
func NewStakingManager(cfg *Config) (*StakingManager, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &StakingManager{
		stakes:       make(map[interfaces.NodeID]*StakeInfo),
		pending:      make(map[interfaces.NodeID]*StakeInfo),
		slashRecords: []*SlashRecord{},
		config:       cfg,
	}, nil
}

// Stake adds stake for a node.
func (sm *StakingManager) Stake(nodeID interfaces.NodeID, amount uint64, lockDuration time.Duration) error {
	if amount < sm.config.MinStake {
		return fmt.Errorf("stake amount %d less than minimum %d", amount, sm.config.MinStake)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	lockedUntil := now.Add(lockDuration)

	if existing, ok := sm.stakes[nodeID]; ok {
		existing.Amount += amount
		if lockedUntil.After(existing.LockedUntil) {
			existing.LockedUntil = lockedUntil
		}
	} else {
		sm.stakes[nodeID] = &StakeInfo{
			NodeID:        nodeID,
			Amount:        amount,
			LockedUntil:   lockedUntil,
			SlashCount:    0,
			TotalSlashed:  0,
			LastSlashTime: nil,
		}
	}

	return nil
}

// Unstake requests to unstake from a node.
func (sm *StakingManager) Unstake(nodeID interfaces.NodeID, amount uint64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, ok := sm.stakes[nodeID]
	if !ok {
		return fmt.Errorf("no stake found for node %x", nodeID)
	}

	if amount > stake.Amount {
		return fmt.Errorf("unstake amount %d exceeds available stake %d", amount, stake.Amount)
	}

	// If unstaking all, move to pending
	if amount == stake.Amount {
		delete(sm.stakes, nodeID)
		stake.LockedUntil = time.Now().Add(7 * 24 * time.Hour) // 7 day unstaking period
		sm.pending[nodeID] = stake
	} else {
		stake.Amount -= amount
	}

	return nil
}

// ClaimUnstake claims a pending unstake.
func (sm *StakingManager) ClaimUnstake(nodeID interfaces.NodeID) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, ok := sm.pending[nodeID]
	if !ok {
		return fmt.Errorf("no pending unstake for node %x", nodeID)
	}

	if time.Now().Before(stake.LockedUntil) {
		return fmt.Errorf("unstake not ready, locked until %v", stake.LockedUntil)
	}

	delete(sm.pending, nodeID)
	return nil
}

// Slash slashes a node's stake.
func (sm *StakingManager) Slash(nodeID interfaces.NodeID, amount uint64, reason SlashReason, evidence []byte, blockHeight uint64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stake, ok := sm.stakes[nodeID]
	if !ok {
		return fmt.Errorf("no stake found for node %x", nodeID)
	}

	if amount > stake.Amount {
		amount = stake.Amount
	}

	stake.Amount -= amount
	stake.TotalSlashed += amount
	stake.SlashCount++
	now := time.Now()
	stake.LastSlashTime = &now

	sm.slashRecords = append(sm.slashRecords, &SlashRecord{
		NodeID:      nodeID,
		Amount:      amount,
		Reason:      reason,
		Evidence:    evidence,
		Timestamp:   now,
		BlockHeight: blockHeight,
	})

	return nil
}

// GetStakeInfo returns staking information for a node.
func (sm *StakingManager) GetStakeInfo(nodeID interfaces.NodeID) (*StakeInfo, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if stake, ok := sm.stakes[nodeID]; ok {
		return stake, nil
	}

	if stake, ok := sm.pending[nodeID]; ok {
		return stake, nil
	}

	return nil, fmt.Errorf("no stake found for node %x", nodeID)
}

// GetAllStakeInfo returns all staking information.
func (sm *StakingManager) GetAllStakeInfo() []*StakeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []*StakeInfo
	for _, stake := range sm.stakes {
		result = append(result, stake)
	}

	for _, stake := range sm.pending {
		result = append(result, stake)
	}

	return result
}

// GetSlashRecords returns slash records for a node.
func (sm *StakingManager) GetSlashRecords(nodeID interfaces.NodeID) []*SlashRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []*SlashRecord
	for _, record := range sm.slashRecords {
		if record.NodeID == nodeID {
			result = append(result, record)
		}
	}

	return result
}

// GetAllSlashRecords returns all slash records.
func (sm *StakingManager) GetAllSlashRecords() []*SlashRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]*SlashRecord, len(sm.slashRecords))
	copy(result, sm.slashRecords)
	return result
}

// HasMinimumStake checks if a node has minimum required stake.
func (sm *StakingManager) HasMinimumStake(nodeID interfaces.NodeID) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, ok := sm.stakes[nodeID]
	if !ok {
		return false
	}

	return stake.Amount >= sm.config.MinStake
}

// CanSlash checks if a node can be slashed.
func (sm *StakingManager) CanSlash(nodeID interfaces.NodeID, reason SlashReason) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, ok := sm.stakes[nodeID]
	if !ok || stake.Amount == 0 {
		return false
	}

	// Prevent too frequent slashing
	if stake.LastSlashTime != nil {
		cooldown := 24 * time.Hour // 24 hour cooldown
		if time.Now().Before(stake.LastSlashTime.Add(cooldown)) {
			return false
		}
	}

	return true
}

// GetEffectiveStake returns effective stake after slashing.
func (sm *StakingManager) GetEffectiveStake(nodeID interfaces.NodeID) uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, ok := sm.stakes[nodeID]
	if !ok {
		return 0
	}

	return stake.Amount
}

// CalculateSlashAmount calculates the amount to slash based on reason.
func (sm *StakingManager) CalculateSlashAmount(nodeID interfaces.NodeID, reason SlashReason) uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stake, ok := sm.stakes[nodeID]
	if !ok {
		return 0
	}

	var percentage float64

	switch reason {
	case SlashReasonDowntime:
		percentage = 0.01 // 1%
	case SlashReasonInvalidResponse:
		percentage = 0.05 // 5%
	case SlashReasonTimeout:
		percentage = 0.02 // 2%
	case SlashReasonMalicious:
		percentage = 0.50 // 50%
	case SlashReasonConsensusViolation:
		percentage = 0.20 // 20%
	default:
		percentage = 0.01 // Default 1%
	}

	amount := uint64(float64(stake.Amount) * percentage)
	if amount < 100 {
		amount = 100 // Minimum slash amount
	}

	return amount
}

// GetSlashReasonByCode returns slash reason from string code.
func GetSlashReasonByCode(code string) SlashReason {
	switch code {
	case "downtime":
		return SlashReasonDowntime
	case "invalid_response":
		return SlashReasonInvalidResponse
	case "timeout":
		return SlashReasonTimeout
	case "malicious":
		return SlashReasonMalicious
	case "consensus_violation":
		return SlashReasonConsensusViolation
	default:
		return SlashReasonUnknown
	}
}

// CleanupExpiredPending removes expired pending stakes.
func (sm *StakingManager) CleanupExpiredPending() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var removed int
	now := time.Now()

	for nodeID, stake := range sm.pending {
		if now.After(stake.LockedUntil) {
			delete(sm.pending, nodeID)
			removed++
		}
	}

	return removed
}

// PruneOldSlashRecords removes slash records older than 365 days.
func (sm *StakingManager) PruneOldSlashRecords() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var kept []*SlashRecord
	cutoff := time.Now().Add(-365 * 24 * time.Hour)

	for _, record := range sm.slashRecords {
		if record.Timestamp.After(cutoff) {
			kept = append(kept, record)
		}
	}

	removed := len(sm.slashRecords) - len(kept)
	sm.slashRecords = kept

	return removed
}

// UpdateStake updates stake information.
func (sm *StakingManager) UpdateStake(nodeID interfaces.NodeID, stake *StakeInfo) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.stakes[nodeID] = stake
	return nil
}
