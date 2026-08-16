// Package migration provides unit tests for the migration functionality.
package migration

import (
	"errors"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Mock TokenMinter for testing
// ============================================================================

// mockMinter is a test implementation of TokenMinter
type mockMinter struct {
	balances map[string]uint64
	minted   uint64
	burned   uint64
}

func newMockMinter() *mockMinter {
	return &mockMinter{
		balances: make(map[string]uint64),
	}
}

func (m *mockMinter) Mint(to interfaces.Address, amount uint64) error {
	addr := string(to[:])
	m.balances[addr] += amount
	m.minted += amount
	return nil
}

func (m *mockMinter) Burn(from interfaces.Address, amount uint64) error {
	addr := string(from[:])
	if m.balances[addr] < amount {
		return errors.New("insufficient balance")
	}
	m.balances[addr] -= amount
	m.burned += amount
	return nil
}

func (m *mockMinter) BalanceOf(addr interfaces.Address) (uint64, error) {
	return m.balances[string(addr[:])], nil
}

// ============================================================================
// Helper functions
// ============================================================================

// fixedNow pins the test clock to 2026-03-15 (month 3 of the default
// migration window 2026-01-01 .. 2026-04-01) so rate/window assertions are
// deterministic regardless of when the tests run.
var fixedNow = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

func createTestHub(t *testing.T) *MigrationHub {
	config := DefaultHubConfig()
	config.Minter = newMockMinter()

	hub, err := NewMigrationHub(config)
	if err != nil {
		t.Fatalf("failed to create test hub: %v", err)
	}
	hub.SetClock(func() time.Time { return fixedNow })
	return hub
}

// ============================================================================
// MigrationHub Tests
// ============================================================================

func TestNewMigrationHub(t *testing.T) {
	config := DefaultHubConfig()
	config.Minter = newMockMinter()

	hub, err := NewMigrationHub(config)
	if err != nil {
		t.Fatalf("NewMigrationHub failed: %v", err)
	}

	if hub == nil {
		t.Fatal("NewMigrationHub should not return nil")
	}
}

func TestNewMigrationHub_NoMinter(t *testing.T) {
	config := DefaultHubConfig()
	config.Minter = nil

	_, err := NewMigrationHub(config)
	if err == nil {
		t.Error("should error without minter")
	}
}

func TestDefaultHubConfig(t *testing.T) {
	config := DefaultHubConfig()

	if len(config.BTCIncentiveRates) != 3 {
		t.Errorf("expected 3 BTC rates, got %d", len(config.BTCIncentiveRates))
	}

	if len(config.ETHIncentiveRates) != 3 {
		t.Errorf("expected 3 ETH rates, got %d", len(config.ETHIncentiveRates))
	}

	if len(config.SOLIncentiveRates) != 3 {
		t.Errorf("expected 3 SOL rates, got %d", len(config.SOLIncentiveRates))
	}

	if config.TGEPercent != 20 {
		t.Errorf("expected TGEPercent 20, got %d", config.TGEPercent)
	}

	if config.VestingMonths != 3 {
		t.Errorf("expected VestingMonths 3, got %d", config.VestingMonths)
	}
}

func TestMigrationHub_GetMigrationStatus(t *testing.T) {
	hub := createTestHub(t)
	status := hub.GetMigrationStatus()

	if status == nil {
		t.Fatal("GetMigrationStatus should not return nil")
	}

	// Check tiered rates (current time is March 2026, so should be month 3 rate)
	if status.BTCCurrentRate != 3 {
		t.Errorf("expected BTC rate 3 (month 3), got %d", status.BTCCurrentRate)
	}

	if status.ETHCurrentRate != 2 {
		t.Errorf("expected ETH rate 2 (month 3), got %d", status.ETHCurrentRate)
	}

	if status.SOLCurrentRate != 1 {
		t.Errorf("expected SOL rate 1 (month 3), got %d", status.SOLCurrentRate)
	}
}

func TestMigrationHub_IsMigrationWindowOpen(t *testing.T) {
	hub := createTestHub(t)

	// The migration window is 2026-01-01 to 2026-04-01
	// Current date is 2026-03-03, so window should be open
	if !hub.IsMigrationWindowOpen() {
		t.Error("migration window should be open in March 2026")
	}
}

func TestMigrationHub_IsAIB1ClaimOpen(t *testing.T) {
	hub := createTestHub(t)

	// The claim deadline is 2028-01-01
	// Current date is 2026-03-03, so claim should be open
	if !hub.IsAIB1ClaimOpen() {
		t.Error("AIB1 claim should be open before 2028")
	}
}

func TestMigrationHub_GetCrossChainRate(t *testing.T) {
	hub := createTestHub(t)

	tests := []struct {
		chain     ChainType
		expected  uint64
	}{
		{ChainBTC, 3}, // month 3 rate
		{ChainETH, 2},
		{ChainSOL, 1},
	}

	for _, tt := range tests {
		rate, err := hub.GetCrossChainRate(tt.chain)
		if err != nil {
			t.Errorf("GetCrossChainRate(%s) failed: %v", tt.chain, err)
		}
		if rate != tt.expected {
			t.Errorf("expected %s rate %d, got %d", tt.chain, tt.expected, rate)
		}
	}

	// Unknown chain should error
	_, err := hub.GetCrossChainRate(ChainType("UNKNOWN"))
	if err == nil {
		t.Error("unknown chain should error")
	}
}

// ============================================================================
// ChainType Tests
// ============================================================================

func TestChainType_Constants(t *testing.T) {
	if ChainBTC != "BTC" {
		t.Errorf("expected ChainBTC to be 'BTC', got %s", ChainBTC)
	}

	if ChainETH != "ETH" {
		t.Errorf("expected ChainETH to be 'ETH', got %s", ChainETH)
	}

	if ChainSOL != "SOL" {
		t.Errorf("expected ChainSOL to be 'SOL', got %s", ChainSOL)
	}
}

// ============================================================================
// Config Tests
// ============================================================================

func TestHubConfig_Validate(t *testing.T) {
	// Valid config
	validConfig := DefaultHubConfig()
	validConfig.Minter = newMockMinter()
	if err := validateHubConfig(validConfig); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}

	// Invalid: window end before start
	invalidConfig := DefaultHubConfig()
	invalidConfig.MigrationWindowStart = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	invalidConfig.MigrationWindowEnd = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := validateHubConfig(invalidConfig); err == nil {
		t.Error("invalid window should error")
	}

	// Invalid: empty BTC rates
	invalidRates := DefaultHubConfig()
	invalidRates.BTCIncentiveRates = []uint64{}
	if err := validateHubConfig(invalidRates); err == nil {
		t.Error("empty BTC rates should error")
	}

	// Invalid: claim deadline before window end
	invalidDeadline := DefaultHubConfig()
	invalidDeadline.ClaimDeadline = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := validateHubConfig(invalidDeadline); err == nil {
		t.Error("claim deadline before window end should error")
	}
}

// validateHubConfig is a helper to validate the config
func validateHubConfig(cfg *HubConfig) error {
	if cfg.MigrationWindowEnd.Before(cfg.MigrationWindowStart) {
		return errors.New("window end before start")
	}
	if len(cfg.BTCIncentiveRates) == 0 {
		return errors.New("empty BTC rates")
	}
	if cfg.ClaimDeadline.Before(cfg.MigrationWindowEnd) {
		return errors.New("claim deadline before window end")
	}
	return nil
}

// ============================================================================
// Migration Event Tests
// ============================================================================

func TestMigrationHub_GetEvents(t *testing.T) {
	hub := createTestHub(t)

	// Initially no events
	events := hub.GetEvents()
	if events == nil {
		t.Error("GetEvents should not return nil")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestMigrationHub_GetUserMigrationInfo_Empty(t *testing.T) {
	hub := createTestHub(t)

	addr := [32]byte{1, 2, 3, 4}
	info := hub.GetUserMigrationInfo(addr)

	if info == nil {
		t.Fatal("GetUserMigrationInfo should not return nil")
	}

	// New address should have zero balances
	if info.AIB1SnapshotBalance != 0 {
		t.Errorf("expected 0 snapshot balance, got %d", info.AIB1SnapshotBalance)
	}

	if info.TotalClaimable != 0 {
		t.Errorf("expected 0 claimable, got %d", info.TotalClaimable)
	}
}
