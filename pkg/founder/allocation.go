// Package founder implements the founder allocation system for AIB 2.0.
// This file implements linear vesting allocation.
package founder

import (
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// AllocationManager manages founder token allocations and vesting.
type AllocationManager struct {
	founders     *FounderList
	claims       map[string]*ClaimRecord // founder ID -> claim record
	multiSig     *MultiSigConfig
	vestingStart time.Time
	mu           sync.RWMutex
}

// ClaimRecord tracks claimed amounts for each founder.
type ClaimRecord struct {
	FounderID     string       `json:"founder_id"`
	TotalClaimed  uint64       `json:"total_claimed"`
	ClaimHistory  []ClaimEntry `json:"claim_history"`
	LastClaimTime time.Time    `json:"last_claim_time"`
}

// ClaimEntry represents a single claim transaction.
type ClaimEntry struct {
	TxHash    string    `json:"tx_hash"`
	Amount    uint64    `json:"amount"`
	Timestamp time.Time `json:"timestamp"`
	BlockNum  uint64    `json:"block_num,omitempty"`
}

// NewAllocationManager creates a new allocation manager.
func NewAllocationManager(founders *FounderList, vestingStart time.Time) *AllocationManager {
	return &AllocationManager{
		founders:     founders,
		claims:       make(map[string]*ClaimRecord),
		multiSig:     DefaultMultiSigConfig(),
		vestingStart: vestingStart,
	}
}

// SetMultiSigConfig sets the multi-signature configuration.
func (am *AllocationManager) SetMultiSigConfig(config *MultiSigConfig) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.multiSig = config
}

// GetVestingInfo returns current vesting information for a founder.
func (am *AllocationManager) GetVestingInfo(founderID string) (*VestingInfo, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	founder, exists := am.founders.Get(founderID)
	if !exists {
		return nil, fmt.Errorf("founder %s not found", founderID)
	}

	now := time.Now()
	info := &VestingInfo{
		FounderID:     founder.ID,
		TotalAmount:   founder.TotalAmount,
		ClaimedAmount: founder.Claimed,
		Status:        founder.Status,
		Schedule:      am.calculateVestingSchedule(founder),
	}

	// Get claim record
	claim, exists := am.claims[founderID]
	if exists {
		info.ClaimedAmount = claim.TotalClaimed
	}

	// Calculate vesting based on current time
	info.VestedAmount = am.calculateVestedAmount(founder, now)
	info.ClaimableAmount = 0
	if info.VestedAmount > info.ClaimedAmount {
		info.ClaimableAmount = info.VestedAmount - info.ClaimedAmount
	}
	info.LockedAmount = founder.TotalAmount - info.VestedAmount

	// Calculate progress (0 to 1)
	if founder.TotalAmount > 0 {
		info.Progress = float64(info.VestedAmount) / float64(founder.TotalAmount)
	}

	// Find next unlock
	info.NextUnlock, info.UnlockAmount = am.findNextUnlock(founder, now)

	// Update status based on vesting progress
	if now.Before(founder.UnlockTime) {
		info.Status = StatusLocked
	} else if info.VestedAmount >= founder.TotalAmount {
		info.Status = StatusCompleted
	} else {
		info.Status = StatusVesting
	}

	return info, nil
}

// calculateVestedAmount calculates how much has vested at the given time.
func (am *AllocationManager) calculateVestedAmount(founder *Founder, at time.Time) uint64 {
	// During lock period, nothing is vested
	if at.Before(founder.UnlockTime) {
		return 0
	}

	// After full vesting period, everything is vested
	if at.After(founder.EndTime) || at.Equal(founder.EndTime) {
		return founder.TotalAmount
	}

	// Calculate vested amount based on linear vesting
	// Vesting starts after lock period and continues for VestingMonths
	vestingDuration := founder.EndTime.Sub(founder.UnlockTime)
	elapsed := at.Sub(founder.UnlockTime)

	// Linear vesting: vested = total * elapsed / duration
	vested := uint64(float64(founder.TotalAmount) * float64(elapsed) / float64(vestingDuration))

	return vested
}

// calculateVestingSchedule generates the full vesting schedule for a founder.
func (am *AllocationManager) calculateVestingSchedule(founder *Founder) []VestingEntry {
	entries := make([]VestingEntry, VestingMonths+1)

	monthlyAmount := founder.TotalAmount / VestingMonths
	remainder := founder.TotalAmount % VestingMonths

	// Lock period entry (0% unlocked)
	entries[0] = VestingEntry{
		Index:     0,
		Timestamp: founder.StartTime,
		Amount:    0,
		Claimed:   false,
	}

	// Monthly vesting entries
	cumulative := uint64(0)
	for i := 1; i <= VestingMonths; i++ {
		amount := monthlyAmount
		if i == VestingMonths {
			// Add remainder to last month
			amount += remainder
		}
		cumulative += amount

		unlockTime := founder.UnlockTime.Add(time.Duration(i) * 30 * 24 * time.Hour)

		entries[i] = VestingEntry{
			Index:     i,
			Timestamp: unlockTime,
			Amount:    cumulative,
			Claimed:   false,
		}
	}

	return entries
}

// findNextUnlock finds the next vesting unlock time and amount.
func (am *AllocationManager) findNextUnlock(founder *Founder, now time.Time) (time.Time, uint64) {
	schedule := am.calculateVestingSchedule(founder)

	for i := 1; i < len(schedule); i++ {
		if schedule[i].Timestamp.After(now) {
			// Calculate the incremental amount for this unlock
			return schedule[i].Timestamp, schedule[i].Amount - schedule[i-1].Amount
		}
	}

	// All unlocked
	return time.Time{}, 0
}

// ClaimTokens processes a token claim request.
func (am *AllocationManager) ClaimTokens(founderID string, amount uint64, txHash string, blockNum uint64) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	founder, exists := am.founders.Get(founderID)
	if !exists {
		return fmt.Errorf("founder %s not found", founderID)
	}

	// Get vesting info
	vested := am.calculateVestedAmount(founder, time.Now())

	// Get or create claim record
	claim, exists := am.claims[founderID]
	if !exists {
		claim = &ClaimRecord{
			FounderID:    founderID,
			ClaimHistory: make([]ClaimEntry, 0),
		}
		am.claims[founderID] = claim
	}

	// Check claimable amount
	claimable := vested - claim.TotalClaimed
	if amount > claimable {
		return fmt.Errorf("insufficient vested tokens: requested %d, claimable %d", amount, claimable)
	}

	// Record the claim
	claim.TotalClaimed += amount
	claim.LastClaimTime = time.Now()
	claim.ClaimHistory = append(claim.ClaimHistory, ClaimEntry{
		TxHash:    txHash,
		Amount:    amount,
		Timestamp: time.Now(),
		BlockNum:  blockNum,
	})

	// Update founder's claimed amount
	founder.Claimed = claim.TotalClaimed

	// Update status if fully claimed
	if founder.Claimed >= founder.TotalAmount {
		founder.Status = StatusClaimed
	}

	return nil
}

// GetClaimHistory returns the claim history for a founder.
func (am *AllocationManager) GetClaimHistory(founderID string) (*ClaimRecord, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	claim, exists := am.claims[founderID]
	if !exists {
		return nil, fmt.Errorf("no claims found for founder %s", founderID)
	}
	return claim, nil
}

// GetClaimableAmount returns the amount available for claim by a founder.
func (am *AllocationManager) GetClaimableAmount(founderID string) (uint64, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	founder, exists := am.founders.Get(founderID)
	if !exists {
		return 0, fmt.Errorf("founder %s not found", founderID)
	}

	vested := am.calculateVestedAmount(founder, time.Now())

	claim, exists := am.claims[founderID]
	if !exists {
		return vested, nil
	}

	if vested <= claim.TotalClaimed {
		return 0, nil
	}

	return vested - claim.TotalClaimed, nil
}

// GetTotalVested returns the total vested amount across all founders.
func (am *AllocationManager) GetTotalVested() uint64 {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var total uint64
	now := time.Now()

	for _, founder := range am.founders.List() {
		total += am.calculateVestedAmount(founder, now)
	}

	return total
}

// GetTotalClaimed returns the total claimed amount across all founders.
func (am *AllocationManager) GetTotalClaimed() uint64 {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var total uint64
	for _, claim := range am.claims {
		total += claim.TotalClaimed
	}
	return total
}

// GetAllocationStats returns allocation statistics.
func (am *AllocationManager) GetAllocationStats() *AllocationStats {
	am.mu.RLock()
	defer am.mu.RUnlock()

	stats := &AllocationStats{
		TotalFounders:    am.founders.Count(),
		TotalAllocated:   am.founders.TotalAllocated(),
		VestingStart:     am.vestingStart,
		FoundersByStatus: make(map[FounderStatus]int),
	}

	now := time.Now()

	for _, founder := range am.founders.List() {
		stats.TotalVested += am.calculateVestedAmount(founder, now)
		stats.TotalLocked += founder.TotalAmount - am.calculateVestedAmount(founder, now)

		// Count by status
		claim, exists := am.claims[founder.ID]
		if exists {
			stats.TotalClaimed += claim.TotalClaimed
		}

		// Update status
		if now.Before(founder.UnlockTime) {
			stats.FoundersByStatus[StatusLocked]++
		} else if founder.Status == StatusClaimed {
			stats.FoundersByStatus[StatusClaimed]++
		} else if am.calculateVestedAmount(founder, now) >= founder.TotalAmount {
			stats.FoundersByStatus[StatusCompleted]++
		} else {
			stats.FoundersByStatus[StatusVesting]++
		}
	}

	return stats
}

// AllocationStats holds allocation statistics.
type AllocationStats struct {
	TotalFounders    int                   `json:"total_founders"`
	TotalAllocated   uint64                `json:"total_allocated"`
	TotalVested      uint64                `json:"total_vested"`
	TotalClaimed     uint64                `json:"total_claimed"`
	TotalLocked      uint64                `json:"total_locked"`
	VestingStart     time.Time             `json:"vesting_start"`
	FoundersByStatus map[FounderStatus]int `json:"founders_by_status"`
}

// CreateClaimTransaction creates a transaction for claiming tokens.
func (am *AllocationManager) CreateClaimTransaction(founderID string, amount uint64, utxoStore *utxo.UTXOStore) (*utxo.Transaction, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	founder, exists := am.founders.Get(founderID)
	if !exists {
		return nil, fmt.Errorf("founder %s not found", founderID)
	}

	// Verify claimable amount
	vested := am.calculateVestedAmount(founder, time.Now())
	claim, _ := am.claims[founderID]
	claimed := uint64(0)
	if claim != nil {
		claimed = claim.TotalClaimed
	}

	claimable := vested - claimed
	if amount > claimable {
		return nil, fmt.Errorf("insufficient claimable tokens: %d > %d", amount, claimable)
	}

	// Create transaction output to founder's address
	output := utxo.TXOutput{
		Value:   amount,
		Address: founder.AddressBytes,
	}

	// Create transaction
	tx := utxo.NewTransaction(nil, []utxo.TXOutput{output})

	return tx, nil
}
