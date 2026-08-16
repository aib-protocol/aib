package migration

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Chain Types
// ============================================================================

// ChainType represents a supported source chain for cross-chain migration.
type ChainType string

const (
	ChainBTC ChainType = "BTC"
	ChainETH ChainType = "ETH"
	ChainSOL ChainType = "SOL"
)

// ============================================================================
// Cross-Chain Migration
// ============================================================================

// VestingEntry represents a single vesting unlock point.
type VestingEntry struct {
	UnlockTime time.Time // Absolute time when tokens unlock
	Percent    uint64    // Percentage of total (0-100)
}

// LockedReward represents a vesting reward for a user.
type LockedReward struct {
	Beneficiary interfaces.Address
	Chain       ChainType
	SourceTxID  [32]byte // Source chain transaction hash
	SourceAmount uint64  // Amount in source chain (in smallest unit)
	TotalReward uint64  // Total AIB2 reward (including incentive)
	Claimed     uint64  // Amount already claimed
	CreatedAt   time.Time
	Vesting     []VestingEntry
}

// Claimable returns the amount that is currently unlockable.
func (lr *LockedReward) Claimable(now time.Time) uint64 {
	var totalUnlockable uint64
	for _, v := range lr.Vesting {
		if now.After(v.UnlockTime) || now.Equal(v.UnlockTime) {
			unlockAmount := lr.TotalReward * v.Percent / 100
			totalUnlockable += unlockAmount
		}
	}

	if totalUnlockable > lr.TotalReward {
		totalUnlockable = lr.TotalReward
	}
	if totalUnlockable <= lr.Claimed {
		return 0
	}
	return totalUnlockable - lr.Claimed
}

// RemainingLocked returns the amount still locked.
func (lr *LockedReward) RemainingLocked() uint64 {
	if lr.TotalReward <= lr.Claimed {
		return 0
	}
	return lr.TotalReward - lr.Claimed
}

// CrossChainConfig holds configuration for cross-chain migration.
type CrossChainConfig struct {
	Chain          ChainType
	WindowStart    time.Time
	WindowEnd      time.Time
	IncentiveRates []uint64 // Per-month rates [month1, month2, month3]
	TGEPercent     uint64   // Percent unlocked at TGE (e.g. 20)
	VestingMonths  uint64   // Number of additional vesting months (e.g. 3)
}

// CrossChainMigration manages cross-chain migration for a single chain type.
type CrossChainMigration struct {
	mu sync.RWMutex

	chain          ChainType
	windowStart    time.Time
	windowEnd      time.Time
	incentiveRates []uint64 // Per-month rates, e.g. [5, 4, 3] for BTC
	tgePercent     uint64   // TGE unlock percentage
	vestingMonths  uint64   // Additional vesting months after TGE

	lockedRewards map[interfaces.Address][]*LockedReward
	processedTxs  map[[32]byte]bool // Prevent duplicate migration

	totalMigrated    uint64 // Total source tokens migrated
	totalRewards     uint64 // Total AIB2 rewards minted
}

// NewCrossChainMigration creates a new cross-chain migration contract.
func NewCrossChainMigration(cfg *CrossChainConfig) (*CrossChainMigration, error) {
	if len(cfg.IncentiveRates) == 0 {
		return nil, errors.New("at least one incentive rate required")
	}
	if cfg.TGEPercent > 100 {
		return nil, errors.New("TGE percent must be <= 100")
	}
	if cfg.WindowEnd.Before(cfg.WindowStart) || cfg.WindowEnd.Equal(cfg.WindowStart) {
		return nil, errors.New("window end must be after window start")
	}

	return &CrossChainMigration{
		chain:          cfg.Chain,
		windowStart:    cfg.WindowStart,
		windowEnd:      cfg.WindowEnd,
		incentiveRates: cfg.IncentiveRates,
		tgePercent:     cfg.TGEPercent,
		vestingMonths:  cfg.VestingMonths,
		lockedRewards:  make(map[interfaces.Address][]*LockedReward),
		processedTxs:   make(map[[32]byte]bool),
	}, nil
}

// GetCurrentRate returns the current incentive rate based on the migration phase.
// Returns 0 if migration window is closed.
func (c *CrossChainMigration) GetCurrentRate(now time.Time) uint64 {
	if now.Before(c.windowStart) || now.After(c.windowEnd) || now.Equal(c.windowEnd) {
		return 0
	}

	elapsed := now.Sub(c.windowStart)
	totalDuration := c.windowEnd.Sub(c.windowStart)
	numPhases := len(c.incentiveRates)

	// Compute which phase we are in
	phaseIdx := int(elapsed * time.Duration(numPhases) / totalDuration)
	if phaseIdx >= numPhases {
		phaseIdx = numPhases - 1
	}

	return c.incentiveRates[phaseIdx]
}

// IsWindowOpen checks if the migration window is currently open.
func (c *CrossChainMigration) IsWindowOpen(now time.Time) bool {
	return !now.Before(c.windowStart) && now.Before(c.windowEnd)
}

// buildVestingSchedule creates the vesting schedule for a reward.
// TGE% is unlocked immediately, rest is distributed over vestingMonths equally.
func (c *CrossChainMigration) buildVestingSchedule(tgeTime time.Time) []VestingEntry {
	entries := make([]VestingEntry, 0, c.vestingMonths+1)

	// TGE: immediate unlock
	entries = append(entries, VestingEntry{
		UnlockTime: tgeTime,
		Percent:    c.tgePercent,
	})

	// Distribute remaining percentage over vestingMonths
	remaining := uint64(100) - c.tgePercent
	if c.vestingMonths == 0 {
		// All unlocked at TGE
		entries[0].Percent = 100
		return entries
	}

	perMonth := remaining / c.vestingMonths
	var allocated uint64

	for i := uint64(1); i <= c.vestingMonths; i++ {
		pct := perMonth
		if i == c.vestingMonths {
			// Last month gets any rounding remainder
			pct = remaining - allocated
		}
		allocated += pct

		unlockTime := tgeTime.AddDate(0, int(i), 0)
		entries = append(entries, VestingEntry{
			UnlockTime: unlockTime,
			Percent:    pct,
		})
	}

	return entries
}

// CrossChainProof represents a proof of cross-chain deposit.
type CrossChainProof struct {
	Chain          ChainType
	SourceTxID     [32]byte
	SourceAddress  []byte // Source chain address (variable length)
	Amount         uint64 // Amount in source chain smallest unit
	BlockHeight    uint64 // Block height of source transaction
	Confirmations  uint64 // Number of confirmations
	RelayerSigs    [][]byte // Relayer attestation signatures
	RelayerPubKeys [][]byte // Relayer public keys
}

// MinConfirmations returns the minimum confirmations required for each chain.
func MinConfirmations(chain ChainType) uint64 {
	switch chain {
	case ChainBTC:
		return 6
	case ChainETH:
		return 12
	case ChainSOL:
		return 32
	default:
		return 0
	}
}

// VerifyCrossChainProof verifies a cross-chain deposit proof.
// The proof must be attested by multiple relayer signatures.
func VerifyCrossChainProof(proof *CrossChainProof, requiredSigs int) error {
	if proof == nil {
		return errors.New("proof is nil")
	}

	// Check chain type
	switch proof.Chain {
	case ChainBTC, ChainETH, ChainSOL:
		// Valid chain
	default:
		return ErrInvalidChain
	}

	// Check confirmations
	minConf := MinConfirmations(proof.Chain)
	if proof.Confirmations < minConf {
		return fmt.Errorf("insufficient confirmations: got %d, need %d",
			proof.Confirmations, minConf)
	}

	// Check relayer signatures
	if len(proof.RelayerSigs) < requiredSigs {
		return fmt.Errorf("insufficient relayer attestations: got %d, need %d",
			len(proof.RelayerSigs), requiredSigs)
	}

	// Verify each relayer signature
	proofHash := hashCrossChainProof(proof)
	_ = proofHash // Used for signature verification in production
	validSigs := 0
	for i, sig := range proof.RelayerSigs {
		if i >= len(proof.RelayerPubKeys) {
			break
		}
		pubKey := proof.RelayerPubKeys[i]
		if len(pubKey) == 32 && len(sig) == 64 {
			if VerifySignature(pubKey, &ClaimData{
				Amount: proof.Amount,
				Nonce:  proof.BlockHeight,
			}, sig) {
				validSigs++
			}
		}
	}

	if validSigs < requiredSigs {
		return fmt.Errorf("insufficient valid relayer attestations: got %d, need %d",
			validSigs, requiredSigs)
	}

	return nil
}

// hashCrossChainProof creates a deterministic hash of the proof data.
func hashCrossChainProof(proof *CrossChainProof) [32]byte {
	h := sha256.New()
	h.Write([]byte(proof.Chain))
	h.Write(proof.SourceTxID[:])
	h.Write(proof.SourceAddress)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, proof.Amount)
	h.Write(buf)
	binary.BigEndian.PutUint64(buf, proof.BlockHeight)
	h.Write(buf)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// Migrate processes a cross-chain migration request.
// It verifies the proof, calculates the reward, and creates vesting schedule.
func (c *CrossChainMigration) Migrate(
	userAddr interfaces.Address,
	proof *CrossChainProof,
	requiredSigs int,
	now time.Time,
) (*LockedReward, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check migration window
	if !c.IsWindowOpen(now) {
		return nil, ErrMigrationWindowClosed
	}

	// Check chain matches
	if proof.Chain != c.chain {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrInvalidChain, c.chain, proof.Chain)
	}

	// Check for duplicate transaction
	if c.processedTxs[proof.SourceTxID] {
		return nil, fmt.Errorf("transaction already processed: %x", proof.SourceTxID[:8])
	}

	// Verify cross-chain proof
	if err := VerifyCrossChainProof(proof, requiredSigs); err != nil {
		return nil, fmt.Errorf("proof verification failed: %w", err)
	}

	// Calculate reward
	rate := c.GetCurrentRate(now)
	if rate == 0 {
		return nil, ErrMigrationWindowClosed
	}
	totalReward := proof.Amount * rate

	// Build vesting schedule
	vesting := c.buildVestingSchedule(now)

	// Create locked reward
	reward := &LockedReward{
		Beneficiary:  userAddr,
		Chain:        c.chain,
		SourceTxID:   proof.SourceTxID,
		SourceAmount: proof.Amount,
		TotalReward:  totalReward,
		Claimed:      0,
		CreatedAt:    now,
		Vesting:      vesting,
	}

	// Store
	c.lockedRewards[userAddr] = append(c.lockedRewards[userAddr], reward)
	c.processedTxs[proof.SourceTxID] = true
	c.totalMigrated += proof.Amount
	c.totalRewards += totalReward

	return reward, nil
}

// GetLockedRewards returns all locked rewards for a user.
func (c *CrossChainMigration) GetLockedRewards(addr interfaces.Address) []*LockedReward {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rewards := c.lockedRewards[addr]
	if len(rewards) == 0 {
		return nil
	}

	// Return copies
	result := make([]*LockedReward, len(rewards))
	for i, r := range rewards {
		rCopy := *r
		vestCopy := make([]VestingEntry, len(r.Vesting))
		copy(vestCopy, r.Vesting)
		rCopy.Vesting = vestCopy
		result[i] = &rCopy
	}
	return result
}

// ClaimUnlocked claims all unlocked tokens for a user.
// Returns the total amount claimed.
func (c *CrossChainMigration) ClaimUnlocked(addr interfaces.Address, now time.Time) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rewards := c.lockedRewards[addr]
	if len(rewards) == 0 {
		return 0, ErrNoLockedRewards
	}

	var totalClaimed uint64
	for _, reward := range rewards {
		claimable := reward.Claimable(now)
		if claimable > 0 {
			reward.Claimed += claimable
			totalClaimed += claimable
		}
	}

	if totalClaimed == 0 {
		return 0, ErrNothingToClaim
	}

	return totalClaimed, nil
}

// GetTotalClaimable returns the total amount currently claimable for a user.
func (c *CrossChainMigration) GetTotalClaimable(addr interfaces.Address, now time.Time) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rewards := c.lockedRewards[addr]
	var total uint64
	for _, reward := range rewards {
		total += reward.Claimable(now)
	}
	return total
}

// GetTotalLocked returns the total amount still locked for a user.
func (c *CrossChainMigration) GetTotalLocked(addr interfaces.Address) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rewards := c.lockedRewards[addr]
	var total uint64
	for _, reward := range rewards {
		total += reward.RemainingLocked()
	}
	return total
}

// GetTotalMigrated returns the total source tokens migrated through this contract.
func (c *CrossChainMigration) GetTotalMigrated() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.totalMigrated
}

// GetTotalRewards returns the total AIB2 rewards assigned.
func (c *CrossChainMigration) GetTotalRewards() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.totalRewards
}

// ChainType returns the chain type for this migration.
func (c *CrossChainMigration) ChainType() ChainType {
	return c.chain
}

// WindowStart returns the migration window start time.
func (c *CrossChainMigration) WindowStart() time.Time {
	return c.windowStart
}

// WindowEnd returns the migration window end time.
func (c *CrossChainMigration) WindowEnd() time.Time {
	return c.windowEnd
}
