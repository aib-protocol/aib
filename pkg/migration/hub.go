package migration

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Token Minter Interface
// ============================================================================

// TokenMinter defines the interface for minting and burning tokens.
type TokenMinter interface {
	Mint(to interfaces.Address, amount uint64) error
	Burn(from interfaces.Address, amount uint64) error
	BalanceOf(addr interfaces.Address) (uint64, error)
}

// ============================================================================
// MigrationHub Configuration
// ============================================================================

// HubConfig holds the full configuration for MigrationHub.
type HubConfig struct {
	// AIB1 snapshot
	AIB1SnapshotRoot [32]byte
	SnapshotTime     time.Time // 2026-01-01
	ClaimDeadline    time.Time // 2028-01-01

	// Cross-chain migration window
	MigrationWindowStart time.Time // 2026-01-01
	MigrationWindowEnd   time.Time // 2026-04-01

	// Incentive rates per month [month1, month2, month3]
	BTCIncentiveRates []uint64 // [5, 4, 3]
	ETHIncentiveRates []uint64 // [4, 3, 2]
	SOLIncentiveRates []uint64 // [3, 2, 1]

	// Vesting
	TGEPercent    uint64 // 20
	VestingMonths uint64 // 3

	// Cross-chain verification
	RequiredRelayerSigs int // Minimum relayer signatures required

	// Admin
	Admin interfaces.Address

	// Token minter
	Minter TokenMinter
}

// DefaultHubConfig returns the default MigrationHub configuration.
func DefaultHubConfig() *HubConfig {
	snapshotTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	claimDeadline := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	return &HubConfig{
		SnapshotTime:         snapshotTime,
		ClaimDeadline:        claimDeadline,
		MigrationWindowStart: snapshotTime,
		MigrationWindowEnd:   windowEnd,
		BTCIncentiveRates:    []uint64{5, 4, 3},
		ETHIncentiveRates:    []uint64{4, 3, 2},
		SOLIncentiveRates:    []uint64{3, 2, 1},
		TGEPercent:           20,
		VestingMonths:        3,
		RequiredRelayerSigs:  3,
	}
}

// ============================================================================
// MigrationHub
// ============================================================================

// MigrationHub is the central contract that orchestrates all migration activities.
// It manages AIB1 snapshot claims, cross-chain migrations (BTC/ETH/SOL),
// vesting schedules, and token minting.
type MigrationHub struct {
	mu sync.RWMutex

	config HubConfig

	// Sub-contracts
	aib1Migration *AIB1Migration
	btcMigration  *CrossChainMigration
	ethMigration  *CrossChainMigration
	solMigration  *CrossChainMigration

	// Token minter
	minter TokenMinter

	// Events log
	events []MigrationEvent
}

// MigrationEventType represents the type of migration event.
type MigrationEventType uint8

const (
	EventAIB1Claim MigrationEventType = iota
	EventCrossChainMigrate
	EventTokensClaimed
)

// MigrationEvent records a migration action.
type MigrationEvent struct {
	Type      MigrationEventType
	Address   interfaces.Address
	Amount    uint64
	Chain     ChainType
	Timestamp time.Time
}

// NewMigrationHub creates a new MigrationHub with the given configuration.
func NewMigrationHub(cfg *HubConfig) (*MigrationHub, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if cfg.Minter == nil {
		return nil, errors.New("token minter is required")
	}

	// Create AIB1 migration
	aib1 := NewAIB1Migration(&AIB1Config{
		SnapshotRoot:  cfg.AIB1SnapshotRoot,
		SnapshotTime:  cfg.SnapshotTime,
		ClaimDeadline: cfg.ClaimDeadline,
	})

	// Create BTC migration
	btc, err := NewCrossChainMigration(&CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    cfg.MigrationWindowStart,
		WindowEnd:      cfg.MigrationWindowEnd,
		IncentiveRates: cfg.BTCIncentiveRates,
		TGEPercent:     cfg.TGEPercent,
		VestingMonths:  cfg.VestingMonths,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create BTC migration: %w", err)
	}

	// Create ETH migration
	eth, err := NewCrossChainMigration(&CrossChainConfig{
		Chain:          ChainETH,
		WindowStart:    cfg.MigrationWindowStart,
		WindowEnd:      cfg.MigrationWindowEnd,
		IncentiveRates: cfg.ETHIncentiveRates,
		TGEPercent:     cfg.TGEPercent,
		VestingMonths:  cfg.VestingMonths,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ETH migration: %w", err)
	}

	// Create SOL migration
	sol, err := NewCrossChainMigration(&CrossChainConfig{
		Chain:          ChainSOL,
		WindowStart:    cfg.MigrationWindowStart,
		WindowEnd:      cfg.MigrationWindowEnd,
		IncentiveRates: cfg.SOLIncentiveRates,
		TGEPercent:     cfg.TGEPercent,
		VestingMonths:  cfg.VestingMonths,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SOL migration: %w", err)
	}

	return &MigrationHub{
		config:        *cfg,
		aib1Migration: aib1,
		btcMigration:  btc,
		ethMigration:  eth,
		solMigration:  sol,
		minter:        cfg.Minter,
		events:        make([]MigrationEvent, 0),
	}, nil
}

// ============================================================================
// AIB1 Claim
// ============================================================================

// LoadAIB1Snapshot loads AIB1 snapshot data into the migration contract.
func (h *MigrationHub) LoadAIB1Snapshot(records []SnapshotRecord) error {
	return h.aib1Migration.LoadSnapshot(records)
}

// ClaimAIB1 allows a user to claim AIB2 tokens based on AIB1 snapshot.
// The user must prove ownership of the AIB1 private key via Ed25519 signature.
func (h *MigrationHub) ClaimAIB1(
	targetAddr interfaces.Address,
	amount uint64,
	pubKey []byte,
	signature []byte,
	nonce uint64,
) error {
	// Verify and record claim (thread-safe internally)
	if err := h.aib1Migration.Claim(targetAddr, amount, pubKey, signature, nonce); err != nil {
		return fmt.Errorf("AIB1 claim failed: %w", err)
	}

	// Mint tokens (1:1 mapping, no vesting)
	if err := h.minter.Mint(targetAddr, amount); err != nil {
		return fmt.Errorf("failed to mint AIB1 claim tokens: %w", err)
	}

	// Log event
	h.mu.Lock()
	h.events = append(h.events, MigrationEvent{
		Type:      EventAIB1Claim,
		Address:   targetAddr,
		Amount:    amount,
		Timestamp: time.Now(),
	})
	h.mu.Unlock()

	return nil
}

// GetAIB1Balance returns the snapshot balance for a given address.
func (h *MigrationHub) GetAIB1Balance(addr interfaces.Address) (uint64, bool) {
	return h.aib1Migration.GetSnapshotBalance(addr)
}

// IsAIB1Claimed checks if an address has already claimed AIB1 tokens.
func (h *MigrationHub) IsAIB1Claimed(addr interfaces.Address) bool {
	return h.aib1Migration.IsClaimed(addr)
}

// IsAIB1ClaimOpen checks if AIB1 claim window is still open.
func (h *MigrationHub) IsAIB1ClaimOpen() bool {
	return h.aib1Migration.IsClaimWindowOpen()
}

// ============================================================================
// Cross-Chain Migration
// ============================================================================

// getCrossChainContract returns the cross-chain migration contract for the given chain.
func (h *MigrationHub) getCrossChainContract(chain ChainType) (*CrossChainMigration, error) {
	switch chain {
	case ChainBTC:
		return h.btcMigration, nil
	case ChainETH:
		return h.ethMigration, nil
	case ChainSOL:
		return h.solMigration, nil
	default:
		return nil, ErrInvalidChain
	}
}

// MigrateBTC processes a BTC cross-chain migration.
func (h *MigrationHub) MigrateBTC(
	userAddr interfaces.Address,
	proof *CrossChainProof,
) error {
	return h.migrateChain(userAddr, proof, ChainBTC)
}

// MigrateETH processes an ETH cross-chain migration.
func (h *MigrationHub) MigrateETH(
	userAddr interfaces.Address,
	proof *CrossChainProof,
) error {
	return h.migrateChain(userAddr, proof, ChainETH)
}

// MigrateSOL processes a SOL cross-chain migration.
func (h *MigrationHub) MigrateSOL(
	userAddr interfaces.Address,
	proof *CrossChainProof,
) error {
	return h.migrateChain(userAddr, proof, ChainSOL)
}

// migrateChain is the internal implementation for cross-chain migration.
func (h *MigrationHub) migrateChain(
	userAddr interfaces.Address,
	proof *CrossChainProof,
	expectedChain ChainType,
) error {
	// Verify chain type
	if proof.Chain != expectedChain {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidChain, expectedChain, proof.Chain)
	}

	now := time.Now()

	// Get the contract
	contract, err := h.getCrossChainContract(expectedChain)
	if err != nil {
		return err
	}

	// Process migration (thread-safe internally)
	reward, err := contract.Migrate(userAddr, proof, h.config.RequiredRelayerSigs, now)
	if err != nil {
		return fmt.Errorf("%s migration failed: %w", expectedChain, err)
	}

	// Mint TGE tokens (immediately unlockable portion)
	tgeAmount := reward.Claimable(now)
	if tgeAmount > 0 {
		if err := h.minter.Mint(userAddr, tgeAmount); err != nil {
			return fmt.Errorf("failed to mint TGE tokens: %w", err)
		}
		// Mark as claimed
		reward.Claimed = tgeAmount
	}

	// Log event
	h.mu.Lock()
	h.events = append(h.events, MigrationEvent{
		Type:      EventCrossChainMigrate,
		Address:   userAddr,
		Amount:    reward.TotalReward,
		Chain:     expectedChain,
		Timestamp: now,
	})
	h.mu.Unlock()

	return nil
}

// ============================================================================
// Claim Unlocked Tokens
// ============================================================================

// ClaimUnlocked allows a user to claim all unlocked tokens from cross-chain migrations.
func (h *MigrationHub) ClaimUnlocked(userAddr interfaces.Address) (uint64, error) {
	now := time.Now()
	var totalClaimed uint64

	// Claim from each chain
	for _, chain := range []ChainType{ChainBTC, ChainETH, ChainSOL} {
		contract, err := h.getCrossChainContract(chain)
		if err != nil {
			continue
		}

		claimed, err := contract.ClaimUnlocked(userAddr, now)
		if err != nil {
			if errors.Is(err, ErrNoLockedRewards) || errors.Is(err, ErrNothingToClaim) {
				continue
			}
			return totalClaimed, fmt.Errorf("claim from %s failed: %w", chain, err)
		}

		// Mint claimed tokens
		if claimed > 0 {
			if err := h.minter.Mint(userAddr, claimed); err != nil {
				return totalClaimed, fmt.Errorf("failed to mint claimed tokens: %w", err)
			}
			totalClaimed += claimed
		}
	}

	if totalClaimed == 0 {
		return 0, ErrNothingToClaim
	}

	// Log event
	h.mu.Lock()
	h.events = append(h.events, MigrationEvent{
		Type:      EventTokensClaimed,
		Address:   userAddr,
		Amount:    totalClaimed,
		Timestamp: now,
	})
	h.mu.Unlock()

	return totalClaimed, nil
}

// ============================================================================
// Query Functions
// ============================================================================

// MigrationStatus holds the overall migration status.
type MigrationStatus struct {
	// AIB1
	AIB1TotalMigrated uint64
	AIB1ClaimOpen     bool

	// BTC
	BTCTotalMigrated uint64
	BTCTotalRewards  uint64
	BTCWindowOpen    bool
	BTCCurrentRate   uint64

	// ETH
	ETHTotalMigrated uint64
	ETHTotalRewards  uint64
	ETHWindowOpen    bool
	ETHCurrentRate   uint64

	// SOL
	SOLTotalMigrated uint64
	SOLTotalRewards  uint64
	SOLWindowOpen    bool
	SOLCurrentRate   uint64

	// Timestamps
	MigrationWindowStart time.Time
	MigrationWindowEnd   time.Time
	AIB1ClaimDeadline    time.Time
}

// GetMigrationStatus returns the overall migration status.
func (h *MigrationHub) GetMigrationStatus() *MigrationStatus {
	now := time.Now()

	return &MigrationStatus{
		AIB1TotalMigrated: h.aib1Migration.GetTotalMigrated(),
		AIB1ClaimOpen:     h.aib1Migration.IsClaimWindowOpen(),

		BTCTotalMigrated: h.btcMigration.GetTotalMigrated(),
		BTCTotalRewards:  h.btcMigration.GetTotalRewards(),
		BTCWindowOpen:    h.btcMigration.IsWindowOpen(now),
		BTCCurrentRate:   h.btcMigration.GetCurrentRate(now),

		ETHTotalMigrated: h.ethMigration.GetTotalMigrated(),
		ETHTotalRewards:  h.ethMigration.GetTotalRewards(),
		ETHWindowOpen:    h.ethMigration.IsWindowOpen(now),
		ETHCurrentRate:   h.ethMigration.GetCurrentRate(now),

		SOLTotalMigrated: h.solMigration.GetTotalMigrated(),
		SOLTotalRewards:  h.solMigration.GetTotalRewards(),
		SOLWindowOpen:    h.solMigration.IsWindowOpen(now),
		SOLCurrentRate:   h.solMigration.GetCurrentRate(now),

		MigrationWindowStart: h.config.MigrationWindowStart,
		MigrationWindowEnd:   h.config.MigrationWindowEnd,
		AIB1ClaimDeadline:    h.config.ClaimDeadline,
	}
}

// UserMigrationInfo holds migration info for a specific user.
type UserMigrationInfo struct {
	// AIB1
	AIB1SnapshotBalance uint64
	AIB1Claimed         bool

	// Cross-chain locked rewards
	BTCLockedRewards []*LockedReward
	ETHLockedRewards []*LockedReward
	SOLLockedRewards []*LockedReward

	// Totals
	TotalClaimable uint64
	TotalLocked    uint64
}

// GetUserMigrationInfo returns migration info for a specific user.
func (h *MigrationHub) GetUserMigrationInfo(addr interfaces.Address) *UserMigrationInfo {
	now := time.Now()
	balance, _ := h.aib1Migration.GetSnapshotBalance(addr)

	return &UserMigrationInfo{
		AIB1SnapshotBalance: balance,
		AIB1Claimed:         h.aib1Migration.IsClaimed(addr),

		BTCLockedRewards: h.btcMigration.GetLockedRewards(addr),
		ETHLockedRewards: h.ethMigration.GetLockedRewards(addr),
		SOLLockedRewards: h.solMigration.GetLockedRewards(addr),

		TotalClaimable: h.btcMigration.GetTotalClaimable(addr, now) +
			h.ethMigration.GetTotalClaimable(addr, now) +
			h.solMigration.GetTotalClaimable(addr, now),

		TotalLocked: h.btcMigration.GetTotalLocked(addr) +
			h.ethMigration.GetTotalLocked(addr) +
			h.solMigration.GetTotalLocked(addr),
	}
}

// GetEvents returns all migration events.
func (h *MigrationHub) GetEvents() []MigrationEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]MigrationEvent, len(h.events))
	copy(result, h.events)
	return result
}

// GetCrossChainRate returns the current incentive rate for a chain.
func (h *MigrationHub) GetCrossChainRate(chain ChainType) (uint64, error) {
	contract, err := h.getCrossChainContract(chain)
	if err != nil {
		return 0, err
	}
	return contract.GetCurrentRate(time.Now()), nil
}

// IsMigrationWindowOpen checks if the cross-chain migration window is open.
func (h *MigrationHub) IsMigrationWindowOpen() bool {
	now := time.Now()
	return !now.Before(h.config.MigrationWindowStart) && now.Before(h.config.MigrationWindowEnd)
}
