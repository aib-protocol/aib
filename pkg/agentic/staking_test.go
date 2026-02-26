package agentic

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	return &Config{
		NodeID:          interfaces.NodeID{1, 2, 3},
		PrivateKey:      privKey,
		PublicKey:       privKey.Public().(ed25519.PublicKey),
		Address:         interfaces.Address{4, 5, 6},
		MinStake:        1000,
		SlashThreshold:  500,
		ReputationDecay: 0.9,
		MaxNodes:        100,
		ServiceTimeout:  30 * time.Second,
	}
}

func newTestStakingManager(t *testing.T) *StakingManager {
	t.Helper()
	cfg := newTestConfig(t)
	sm, err := NewStakingManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create staking manager: %v", err)
	}
	return sm
}

func TestStakingManager_Stake(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	amount := uint64(10000)
	lockDuration := 30 * 24 * time.Hour

	// Stake
	err := sm.Stake(nodeID, amount, lockDuration)
	if err != nil {
		t.Fatalf("Stake failed: %v", err)
	}

	// Verify stake info
	info, err := sm.GetStakeInfo(nodeID)
	if err != nil {
		t.Fatalf("GetStakeInfo failed: %v", err)
	}

	if info.Amount != amount {
		t.Errorf("Amount = %d, expected %d", info.Amount, amount)
	}

	// Add more stake
	err = sm.Stake(nodeID, 5000, lockDuration)
	if err != nil {
		t.Fatalf("Additional stake failed: %v", err)
	}

	info, _ = sm.GetStakeInfo(nodeID)
	if info.Amount != 15000 {
		t.Errorf("Amount = %d, expected 15000", info.Amount)
	}
}

func TestStakingManager_StakeBelowMinimum(t *testing.T) {
	sm := newTestStakingManager(t)
	sm.config.MinStake = 1000

	nodeID := interfaces.NodeID{1, 2, 3}

	err := sm.Stake(nodeID, 500, 30*24*time.Hour)
	if err == nil {
		t.Error("Should fail with stake below minimum")
	}
}

func TestStakingManager_Unstake(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	amount := uint64(10000)

	// Stake first
	sm.Stake(nodeID, amount, 30*24*time.Hour)

	// Partial unstake
	err := sm.Unstake(nodeID, 3000)
	if err != nil {
		t.Fatalf("Unstake failed: %v", err)
	}

	info, _ := sm.GetStakeInfo(nodeID)
	if info.Amount != 7000 {
		t.Errorf("Amount = %d, expected 7000", info.Amount)
	}

	// Full unstake
	err = sm.Unstake(nodeID, 7000)
	if err != nil {
		t.Fatalf("Full unstake failed: %v", err)
	}

	// Should be in pending now
	info, _ = sm.GetStakeInfo(nodeID)
	if info.Amount != 7000 {
		t.Errorf("Pending amount = %d, expected 7000", info.Amount)
	}
}

func TestStakingManager_UnstakeInsufficientFunds(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 1000, 30*24*time.Hour)

	err := sm.Unstake(nodeID, 2000)
	if err == nil {
		t.Error("Should fail with insufficient funds")
	}
}

func TestStakingManager_UnstakeNonexistent(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}

	err := sm.Unstake(nodeID, 1000)
	if err == nil {
		t.Error("Should fail with nonexistent node")
	}
}

func TestStakingManager_Slash(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 10000, 30*24*time.Hour)

	err := sm.Slash(nodeID, 1000, SlashReasonDowntime, []byte("evidence"), 100)
	if err != nil {
		t.Fatalf("Slash failed: %v", err)
	}

	info, _ := sm.GetStakeInfo(nodeID)
	if info.Amount != 9000 {
		t.Errorf("Amount = %d, expected 9000", info.Amount)
	}

	if info.SlashCount != 1 {
		t.Errorf("SlashCount = %d, expected 1", info.SlashCount)
	}

	if info.TotalSlashed != 1000 {
		t.Errorf("TotalSlashed = %d, expected 1000", info.TotalSlashed)
	}

	if info.LastSlashTime == nil {
		t.Error("LastSlashTime should not be nil")
	}

	// Check slash records
	records := sm.GetSlashRecords(nodeID)
	if len(records) != 1 {
		t.Errorf("Slash records count = %d, expected 1", len(records))
	}

	if records[0].Reason != SlashReasonDowntime {
		t.Errorf("Slash reason = %v, expected %v", records[0].Reason, SlashReasonDowntime)
	}
}

func TestStakingManager_SlashExceedsStake(t *testing.T) {
	sm := newTestStakingManager(t)
	sm.config.MinStake = 100 // Lower min stake for this test

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 500, 30*24*time.Hour)

	err := sm.Slash(nodeID, 1000, SlashReasonMalicious, nil, 100)
	if err != nil {
		t.Fatalf("Slash failed: %v", err)
	}

	info, _ := sm.GetStakeInfo(nodeID)
	if info.Amount != 0 {
		t.Errorf("Amount = %d, expected 0 (capped at balance)", info.Amount)
	}
}

func TestStakingManager_HasMinimumStake(t *testing.T) {
	sm := newTestStakingManager(t)
	sm.config.MinStake = 1000

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 1500, 30*24*time.Hour)

	if !sm.HasMinimumStake(nodeID) {
		t.Error("Should have minimum stake")
	}

	// Slash below minimum
	sm.Slash(nodeID, 600, SlashReasonDowntime, nil, 100)

	if sm.HasMinimumStake(nodeID) {
		t.Error("Should not have minimum stake after slashing")
	}
}

func TestStakingManager_CanSlash(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 10000, 30*24*time.Hour)

	if !sm.CanSlash(nodeID, SlashReasonDowntime) {
		t.Error("Should be able to slash")
	}

	// Slash once
	sm.Slash(nodeID, 1000, SlashReasonDowntime, nil, 100)

	// Should be on cooldown
	if sm.CanSlash(nodeID, SlashReasonDowntime) {
		t.Error("Should not be able to slash during cooldown")
	}
}

func TestStakingManager_GetEffectiveStake(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 10000, 30*24*time.Hour)

	effectiveStake := sm.GetEffectiveStake(nodeID)
	if effectiveStake != 10000 {
		t.Errorf("EffectiveStake = %d, expected 10000", effectiveStake)
	}

	// Non-existent node
	unknownID := interfaces.NodeID{9, 9, 9}
	if sm.GetEffectiveStake(unknownID) != 0 {
		t.Error("Non-existent node should have 0 effective stake")
	}
}

func TestStakingManager_CalculateSlashAmount(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID := interfaces.NodeID{1, 2, 3}
	sm.Stake(nodeID, 100000, 30*24*time.Hour)

	tests := []struct {
		reason   SlashReason
		expected uint64
	}{
		{SlashReasonDowntime, 1000},           // 1% of 100000
		{SlashReasonInvalidResponse, 5000},    // 5% of 100000
		{SlashReasonTimeout, 2000},            // 2% of 100000
		{SlashReasonMalicious, 50000},         // 50% of 100000
		{SlashReasonConsensusViolation, 20000}, // 20% of 100000
	}

	for _, test := range tests {
		amount := sm.CalculateSlashAmount(nodeID, test.reason)
		if amount != test.expected {
			t.Errorf("CalculateSlashAmount(%s) = %d, expected %d", test.reason, amount, test.expected)
		}
	}
}

func TestStakingManager_GetAllStakeInfo(t *testing.T) {
	sm := newTestStakingManager(t)

	for i := 0; i < 5; i++ {
		nodeID := interfaces.NodeID{}
		nodeID[0] = byte(i)
		sm.Stake(nodeID, uint64(i+1)*1000, 30*24*time.Hour)
	}

	all := sm.GetAllStakeInfo()
	if len(all) != 5 {
		t.Errorf("All stake count = %d, expected 5", len(all))
	}
}

func TestStakingManager_GetAllSlashRecords(t *testing.T) {
	sm := newTestStakingManager(t)

	nodeID1 := interfaces.NodeID{1}
	nodeID2 := interfaces.NodeID{2}
	sm.Stake(nodeID1, 10000, 30*24*time.Hour)
	sm.Stake(nodeID2, 10000, 30*24*time.Hour)

	sm.Slash(nodeID1, 100, SlashReasonDowntime, nil, 1)
	sm.Slash(nodeID2, 200, SlashReasonTimeout, nil, 2)

	all := sm.GetAllSlashRecords()
	if len(all) != 2 {
		t.Errorf("All slash records count = %d, expected 2", len(all))
	}
}
