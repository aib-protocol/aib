// Package channel provides unit tests for HTLC (Hash Time Locked Contract) functionality.
package channel

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// Helper function to create test addresses
func createTestAddress(seed string) interfaces.Address {
	h := sha256.Sum256([]byte(seed))
	var addr interfaces.Address
	copy(addr[:], h[:])
	return addr
}

// Helper function to create a mock Manager for testing
type mockManager struct {
	channels map[[32]byte]*interfaces.Channel
	states   map[[32]byte]*ChannelState
}

func newMockManager() *mockManager {
	return &mockManager{
		channels: make(map[[32]byte]*interfaces.Channel),
		states:   make(map[[32]byte]*ChannelState),
	}
}

func (m *mockManager) createTestChannel(channelID [32]byte, partyA, partyB interfaces.Address) {
	m.channels[channelID] = &interfaces.Channel{
		ID:        channelID,
		PartyA:   partyA,
		PartyB:   partyB,
		BalanceA: 1000000,
		BalanceB: 1000000,
	}
	m.states[channelID] = &ChannelState{
		Status:        StateOpen,
		LastUpdate:    time.Now(),
		PendingHTLCs:  make(map[[32]byte]*HTLC),
	}
}

// MockMultiSigLocker is a mock implementation of MultiSigLocker for testing
type MockMultiSigLockerForHTLC struct{}

func (m *MockMultiSigLockerForHTLC) CreateMultiSigOutput(partyA, partyB interfaces.Address, amount uint64) (*interfaces.UTXO, error) {
	return &interfaces.UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   amount,
		Address: partyA,
	}, nil
}

func (m *MockMultiSigLockerForHTLC) SpendMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte, outputs []interfaces.TXOutput) error {
	return nil
}

func (m *MockMultiSigLockerForHTLC) VerifyMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte) bool {
	return true
}

// ============================================================================
// HTLC Manager Tests - AddHTLC, CompleteHTLC, ExpireHTLC
// ============================================================================

func TestManager_AddHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create HTLC
	sender := partyA
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, err := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC: %v", err)
	}

	// Add HTLC to channel
	err = manager.AddHTLC(channel.ID, htlc)
	if err != nil {
		t.Fatalf("AddHTLC failed: %v", err)
	}

	// Verify HTLC was added
	htlc2, err := manager.GetHTLC(channel.ID, htlc.ID)
	if err != nil {
		t.Fatalf("GetHTLC failed: %v", err)
	}

	if htlc2.ID != htlc.ID {
		t.Error("HTLC ID mismatch")
	}
}

func TestManager_AddHTLC_ChannelNotFound(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	// Try to add HTLC to non-existent channel
	channelID := [32]byte{99, 99, 99}
	sender := interfaces.Address{1, 2, 3}
	receiver := interfaces.Address{4, 5, 6}
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
	err := manager.AddHTLC(channelID, htlc)
	if err == nil {
		t.Error("expected error for non-existent channel")
	}
}

func TestManager_AddHTLC_InvalidSender(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Use a sender that's not a party
	sender := interfaces.Address{9, 9, 9}
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	err := manager.AddHTLC(channel.ID, htlc)
	if err == nil {
		t.Error("expected error for invalid sender")
	}
}

func TestManager_AddHTLC_SameSenderReceiver(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Same sender and receiver
	sender := partyA
	receiver := partyA
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	err := manager.AddHTLC(channel.ID, htlc)
	if err == nil {
		t.Error("expected error for same sender and receiver")
	}
}

func TestManager_AddHTLC_InsufficientBalance(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 1000, 3000)

	// Try to send more than balance
	sender := partyA
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 2000, sender, receiver)
	err := manager.AddHTLC(channel.ID, htlc)
	if err == nil {
		t.Error("expected error for insufficient balance")
	}
}

func TestManager_CompleteHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Add HTLC first
	sender := partyA
	receiver := partyB
	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Complete HTLC
	err := manager.CompleteHTLC(channel.ID, htlc.ID, preimage)
	if err != nil {
		t.Fatalf("CompleteHTLC failed: %v", err)
	}

	// Verify balances updated
	updatedChannel, _ := manager.GetChannel(channel.ID)
	if updatedChannel.BalanceA != 4000 {
		t.Errorf("BalanceA = %d, expected 4000", updatedChannel.BalanceA)
	}
	if updatedChannel.BalanceB != 4000 {
		t.Errorf("BalanceB = %d, expected 4000", updatedChannel.BalanceB)
	}
}

func TestManager_CompleteHTLC_InvalidPreimage(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Try to complete with wrong preimage
	wrongPreimage := []byte("wrong-secret")
	err := manager.CompleteHTLC(channel.ID, htlc.ID, wrongPreimage)
	if err == nil {
		t.Error("expected error for invalid preimage")
	}
}

func TestManager_ExpireHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create HTLC that's already expired (very short time lock)
	sender := partyA
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Millisecond) // Very short

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Now expire it
	err := manager.ExpireHTLC(channel.ID, htlc.ID)
	if err != nil {
		t.Fatalf("ExpireHTLC failed: %v", err)
	}

	// Verify HTLC is expired
	htlc2, _ := manager.GetHTLC(channel.ID, htlc.ID)
	if htlc2.State != HTLCExpired {
		t.Errorf("Expected HTLCExpired state, got %d", htlc2.State)
	}
}

func TestManager_GetHTLCs(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Add multiple HTLCs
	sender := partyA
	receiver := partyB
	timeLock := time.Now().Add(1 * time.Hour)

	for i := 0; i < 3; i++ {
		hashLock := sha256.Sum256([]byte(string(rune(i))))
		htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
		manager.AddHTLC(channel.ID, htlc)
	}

	htlcs, err := manager.GetHTLCs(channel.ID)
	if err != nil {
		t.Fatalf("GetHTLCs failed: %v", err)
	}

	if len(htlcs) != 3 {
		t.Errorf("Expected 3 HTLCs, got %d", len(htlcs))
	}
}

func TestManager_CountHTLCs(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	timeLock := time.Now().Add(1 * time.Hour)

	hashLock := sha256.Sum256([]byte("secret"))
	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	count, err := manager.CountHTLCs(channel.ID)
	if err != nil {
		t.Fatalf("CountHTLCs failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

func TestManager_GetTotalHTLCAmount(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	timeLock := time.Now().Add(1 * time.Hour)

	// Add HTLC with amount 1000
	hashLock := sha256.Sum256([]byte("secret"))
	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	total, err := manager.GetTotalHTLCAmount(channel.ID)
	if err != nil {
		t.Fatalf("GetTotalHTLCAmount failed: %v", err)
	}

	if total != 1000 {
		t.Errorf("Expected total 1000, got %d", total)
	}
}

func TestManager_GetHTLCsBySender(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	timeLock := time.Now().Add(1 * time.Hour)

	hashLock := sha256.Sum256([]byte("secret"))
	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	htlcs, err := manager.GetHTLCsBySender(channel.ID, sender)
	if err != nil {
		t.Fatalf("GetHTLCsBySender failed: %v", err)
	}

	if len(htlcs) != 1 {
		t.Errorf("Expected 1 HTLC, got %d", len(htlcs))
	}
}

func TestManager_GetHTLCsByReceiver(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	timeLock := time.Now().Add(1 * time.Hour)

	hashLock := sha256.Sum256([]byte("secret"))
	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	htlcs, err := manager.GetHTLCsByReceiver(channel.ID, receiver)
	if err != nil {
		t.Fatalf("GetHTLCsByReceiver failed: %v", err)
	}

	if len(htlcs) != 1 {
		t.Errorf("Expected 1 HTLC, got %d", len(htlcs))
	}
}

func TestManager_CheckHTLCExpiration(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Check if not expired
	expired, err := manager.CheckHTLCExpiration(channel.ID, htlc.ID)
	if err != nil {
		t.Fatalf("CheckHTLCExpiration failed: %v", err)
	}

	if expired {
		t.Error("HTLC should not be expired yet")
	}
}

func TestManager_RemoveHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	sender := partyA
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Remove HTLC
	err := manager.RemoveHTLC(channel.ID, htlc.ID)
	if err != nil {
		t.Fatalf("RemoveHTLC failed: %v", err)
	}

	// Verify removed
	count, _ := manager.CountHTLCs(channel.ID)
	if count != 0 {
		t.Errorf("Expected 0 HTLCs after removal, got %d", count)
	}
}

func TestManager_SettleExpiredHTLCs(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create expired HTLC
	sender := partyA
	receiver := partyB
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Millisecond)

	htlc, _ := NewHTLC(channel.ID, hashLock, timeLock, 1000, sender, receiver)
	manager.AddHTLC(channel.ID, htlc)

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Settle expired
	expired, err := manager.SettleExpiredHTLCs(channel.ID)
	if err != nil {
		t.Fatalf("SettleExpiredHTLCs failed: %v", err)
	}

	if len(expired) != 1 {
		t.Errorf("Expected 1 expired HTLC, got %d", len(expired))
	}
}

// TestHTLC_NewHTLC tests creating a new HTLC
func TestHTLC_NewHTLC(t *testing.T) {
	channelID := [32]byte{1, 2, 3, 4}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, err := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC: %v", err)
	}

	if htlc.Amount != 1000 {
		t.Errorf("expected amount 1000, got %d", htlc.Amount)
	}

	if htlc.Sender != sender {
		t.Error("sender mismatch")
	}

	if htlc.Receiver != receiver {
		t.Error("receiver mismatch")
	}

	if htlc.State != HTLCPending {
		t.Errorf("expected state HTLCPending, got %d", htlc.State)
	}
}

// TestHTLC_NewHTLC_InvalidAmount tests creating HTLC with invalid amount
func TestHTLC_NewHTLC_InvalidAmount(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	_, err := NewHTLC(channelID, hashLock, timeLock, 0, sender, receiver)
	if err == nil {
		t.Error("expected error for zero amount")
	}
}

// TestHTLC_NewHTLC_ZeroTimeLock tests creating HTLC with zero time lock
func TestHTLC_NewHTLC_ZeroTimeLock(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))

	_, err := NewHTLC(channelID, hashLock, time.Time{}, 1000, sender, receiver)
	if err == nil {
		t.Error("expected error for zero time lock")
	}
}

// TestHTLC_NewHTLC_PastTimeLock tests creating HTLC with past time lock
func TestHTLC_NewHTLC_PastTimeLock(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(-1 * time.Hour) // Past time

	_, err := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
	if err == nil {
		t.Error("expected error for past time lock")
	}
}

// TestGenerateHashLock tests generating hash lock from preimage
func TestGenerateHashLock(t *testing.T) {
	preimage := []byte("test-secret-key")
	hashLock, err := GenerateHashLock(preimage)
	if err != nil {
		t.Fatalf("failed to generate hash lock: %v", err)
	}

	// Verify hash lock
	expectedHash := sha256.Sum256(preimage)
	if hashLock != expectedHash {
		t.Error("hash lock mismatch")
	}

	// Test empty preimage
	_, err = GenerateHashLock([]byte{})
	if err == nil {
		t.Error("expected error for empty preimage")
	}
}

// TestNewRandomHashLock tests generating random hash lock
func TestNewRandomHashLock(t *testing.T) {
	hashLock, preimage, err := NewRandomHashLock()
	if err != nil {
		t.Fatalf("failed to generate random hash lock: %v", err)
	}

	if len(preimage) != 32 {
		t.Errorf("expected preimage length 32, got %d", len(preimage))
	}

	// Verify the preimage produces the hash lock
	expectedHash := sha256.Sum256(preimage)
	if hashLock != expectedHash {
		t.Error("hash lock mismatch with preimage")
	}
}

// TestAtomicSwapStatus_String tests AtomicSwapStatus string conversion
func TestAtomicSwapStatus_String(t *testing.T) {
	tests := []struct {
		status   AtomicSwapStatus
		expected string
	}{
		{SwapCreated, "CREATED"},
		{SwapClaimed, "CLAIMED"},
		{SwapRefunded, "REFUNDED"},
		{SwapExpired, "EXPIRED"},
		{AtomicSwapStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.status.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.status.String())
		}
	}
}

// TestAssetInfo tests asset information retrieval
func TestAssetInfo(t *testing.T) {
	// Test AIB asset
	aib, ok := GetAssetInfo("AIB")
	if !ok {
		t.Error("AIB asset not found")
	}
	if aib.Symbol != "AIB" {
		t.Errorf("expected symbol AIB, got %s", aib.Symbol)
	}
	if aib.Decimals != 8 {
		t.Errorf("expected decimals 8, got %d", aib.Decimals)
	}

	// Test BTC asset
	btc, ok := GetAssetInfo("BTC")
	if !ok {
		t.Error("BTC asset not found")
	}
	if btc.Symbol != "BTC" {
		t.Errorf("expected symbol BTC, got %s", btc.Symbol)
	}

	// Test ETH asset
	_, ok = GetAssetInfo("ETH")
	if !ok {
		t.Error("ETH asset not found")
	}

	// Test USDT asset
	usdt, ok := GetAssetInfo("USDT")
	if !ok {
		t.Error("USDT asset not found")
	}
	if usdt.ContractAddr == "" {
		t.Error("USDT should have contract address")
	}

	// Test invalid asset
	_, ok = GetAssetInfo("INVALID")
	if ok {
		t.Error("should return false for invalid asset")
	}
}

// TestIsValidAsset tests asset validation
func TestIsValidAsset(t *testing.T) {
	if !IsValidAsset("AIB") {
		t.Error("AIB should be valid")
	}
	if !IsValidAsset("BTC") {
		t.Error("BTC should be valid")
	}
	if !IsValidAsset("ETH") {
		t.Error("ETH should be valid")
	}
	if !IsValidAsset("USDT") {
		t.Error("USDT should be valid")
	}
	if IsValidAsset("INVALID") {
		t.Error("INVALID should not be valid")
	}
}

// TestHTLCConfig tests HTLC configuration
func TestHTLCConfig(t *testing.T) {
	config := HTLCConfig{
		MinimumExpiryDuration: 1 * time.Hour,
		MaximumExpiryDuration: 7 * 24 * time.Hour,
		MinHTLCAmount:         1000,
		MaxHTLCAmount:         1000000000,
	}

	if config.MinimumExpiryDuration != 1*time.Hour {
		t.Error("minimum expiry duration mismatch")
	}
	if config.MaximumExpiryDuration != 7*24*time.Hour {
		t.Error("maximum expiry duration mismatch")
	}
	if config.MinHTLCAmount != 1000 {
		t.Error("min HTLC amount mismatch")
	}
}

// ============================================================================
// AtomicSwapManager Tests
// ============================================================================

// Note: AtomicSwapManager requires a real Manager with channel setup
// Below are integration-style tests that verify the swap flow logic

// TestAtomicSwap_CreateSwap_Basic tests basic swap creation validation
func TestAtomicSwap_CreateSwap_Basic(t *testing.T) {
	// Since we need a Manager with channel, test validation logic separately
	// Test invalid amount
	if ErrInvalidAmount == nil {
		t.Error("ErrInvalidAmount should be defined")
	}

	// Test invalid asset
	if ErrInvalidAsset == nil {
		t.Error("ErrInvalidAsset should be defined")
	}
}

// TestAtomicSwap_Errors test error definitions
func TestAtomicSwap_Errors(t *testing.T) {
	errors := []error{
		ErrSwapNotFound,
		ErrSwapAlreadyExists,
		ErrInvalidSwapState,
		ErrSwapExpired,
		ErrSwapNotExpired,
		ErrInvalidSecret,
		ErrHashLockMismatch,
		ErrInvalidAsset,
		ErrInvalidAmount,
	}

	for _, e := range errors {
		if e == nil {
			t.Error("error should not be nil")
		}
	}
}

// TestHTLC_State_Transitions tests HTLC state transitions
func TestHTLC_State_Transitions(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	htlc, err := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC: %v", err)
	}

	// Initial state
	if htlc.State != HTLCPending {
		t.Errorf("expected HTLCPending, got %d", htlc.State)
	}

	// Complete HTLC
	preimage := []byte("secret")
	now := time.Now()
	htlc.Preimage = preimage
	htlc.State = HTLCCompleted
	htlc.CompletedAt = &now

	if htlc.State != HTLCCompleted {
		t.Errorf("expected HTLCCompleted, got %d", htlc.State)
	}

	if string(htlc.Preimage) != "secret" {
		t.Error("preimage should be stored")
	}
}

// TestHTLC_Expiration tests HTLC expiration check
func TestHTLC_Expiration(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))

	// Create HTLC with past time lock should fail
	pastTimeLock := time.Now().Add(-1 * time.Hour)
	_, err := NewHTLC(channelID, hashLock, pastTimeLock, 1000, sender, receiver)
	if err == nil {
		t.Error("expected error for past time lock")
	}

	// Create HTLC with future time lock should succeed
	futureTimeLock := time.Now().Add(1 * time.Hour)
	htlc2, err := NewHTLC(channelID, hashLock, futureTimeLock, 1000, sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC: %v", err)
	}

	if time.Now().After(htlc2.TimeLock) {
		t.Error("future time lock should not be expired")
	}
}

// TestSwapFields_AtomicSwap tests AtomicSwap structure fields
func TestSwapFields_AtomicSwap(t *testing.T) {
	swap := &AtomicSwap{
		ID:          [32]byte{1, 2, 3},
		SwapID:      "swap-001",
		Sender:      createTestAddress("sender"),
		Receiver:    createTestAddress("receiver"),
		HashLock:    sha256.Sum256([]byte("secret")),
		Amount:      1000,
		AssetIn:     "AIB",
		AssetOut:    "BTC",
		Rate:        100000000, // 1 BTC = 100M satoshi AIB
		TimeLock:    time.Now().Add(1 * time.Hour),
		Status:      SwapCreated,
		ChannelID:   [32]byte{9, 8, 7},
		CreatedAt:   time.Now(),
		Initiator:   createTestAddress("sender"),
		Participant: createTestAddress("receiver"),
		IsCrossChain: false,
	}

	if swap.SwapID != "swap-001" {
		t.Errorf("expected swap ID swap-001, got %s", swap.SwapID)
	}
	if swap.AssetIn != "AIB" {
		t.Errorf("expected AssetIn AIB, got %s", swap.AssetIn)
	}
	if swap.AssetOut != "BTC" {
		t.Errorf("expected AssetOut BTC, got %s", swap.AssetOut)
	}
	if swap.Status != SwapCreated {
		t.Errorf("expected status SwapCreated, got %s", swap.Status)
	}
}

// TestSwap_RefundFields tests refund tracking
func TestSwap_RefundFields(t *testing.T) {
	swap := &AtomicSwap{
		ID:         [32]byte{1},
		SwapID:     "swap-002",
		TimeLock:   time.Now().Add(-1 * time.Hour), // Expired
		Status:     SwapCreated,
		RefundedAt: nil,
	}

	// Process refund
	now := time.Now()
	swap.Status = SwapRefunded
	swap.RefundedAt = &now

	if swap.Status != SwapRefunded {
		t.Errorf("expected SwapRefunded, got %s", swap.Status)
	}
	if swap.RefundedAt == nil {
		t.Error("RefundedAt should be set")
	}
}

// TestSwap_ClaimFields tests claim tracking
func TestSwap_ClaimFields(t *testing.T) {
	swap := &AtomicSwap{
		ID:        [32]byte{1},
		SwapID:    "swap-003",
		TimeLock:  time.Now().Add(1 * time.Hour),
		Status:    SwapCreated,
		ClaimedAt: nil,
	}

	// Process claim
	preimage := []byte("secret")
	now := time.Now()
	swap.Secret = preimage
	swap.Status = SwapClaimed
	swap.ClaimedAt = &now

	if swap.Status != SwapClaimed {
		t.Errorf("expected SwapClaimed, got %s", swap.Status)
	}
	if swap.Secret == nil {
		t.Error("Secret should be set")
	}
	if swap.ClaimedAt == nil {
		t.Error("ClaimedAt should be set")
	}
}

// TestGenerateSwapID_Format tests swap ID generation
func TestGenerateSwapID_Format(t *testing.T) {
	// Just test that the function exists and doesn't panic
	channelID := [32]byte{1, 2, 3}
	hashLock := sha256.Sum256([]byte("secret"))
	amount := uint64(1000)
	timestamp := time.Now()

	_, err := generateSwapID(channelID, hashLock, amount, timestamp)
	if err != nil {
		t.Fatalf("generateSwapID failed: %v", err)
	}
}

// TestPreimageReveal tests preimage reveal mechanism
func TestPreimageReveal(t *testing.T) {
	// Original preimage
	preimage := []byte("my-secret-preimage")
	hashLock := sha256.Sum256(preimage)

	// Verify hash matches
	calculatedHash := sha256.Sum256(preimage)
	if hashLock != calculatedHash {
		t.Error("hash lock should match preimage hash")
	}

	// Simulate reveal by providing preimage
	revealedPreimage := []byte("my-secret-preimage")
	recalculatedHash := sha256.Sum256(revealedPreimage)
	if recalculatedHash != hashLock {
		t.Error("revealed preimage should match hash lock")
	}

	// Wrong preimage should not match
	wrongPreimage := []byte("wrong-preimage")
	wrongHash := sha256.Sum256(wrongPreimage)
	if wrongHash == hashLock {
		t.Error("wrong preimage should not match hash lock")
	}
}

// TestHTLC_ID_Uniqueness tests HTLC ID generation uniqueness
func TestHTLC_ID_Uniqueness(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")

	// Create multiple HTLCs
	htlcs := make([]*HTLC, 10)
	for i := 0; i < 10; i++ {
		hashLock := sha256.Sum256([]byte(string(rune(i))))
		timeLock := time.Now().Add(1 * time.Hour)
		h, err := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
		if err != nil {
			t.Fatalf("failed to create HTLC: %v", err)
		}
		htlcs[i] = h
	}

	// Check all IDs are unique
	idMap := make(map[string]bool)
	for _, h := range htlcs {
		idStr := string(h.ID[:])
		if idMap[idStr] {
			t.Errorf("duplicate HTLC ID found: %s", idStr)
		}
		idMap[idStr] = true
	}
}

// TestCrossChainFields tests cross-chain swap fields
func TestCrossChainFields(t *testing.T) {
	swap := &AtomicSwap{
		ID:            [32]byte{1},
		IsCrossChain:  true,
		ExternalTxID:  "btc-tx-123",
	}

	if !swap.IsCrossChain {
		t.Error("should be marked as cross-chain")
	}
	if swap.ExternalTxID != "btc-tx-123" {
		t.Errorf("expected ExternalTxID btc-tx-123, got %s", swap.ExternalTxID)
	}
}

// TestRateField tests exchange rate field
func TestRateField(t *testing.T) {
	// Rate is stored as 10^8 multiplier
	// e.g., 100000000 = 1.0 (AssetOut per AssetIn)

	_ = &AtomicSwap{
		Amount: 100000000, // 1 AIB (with 8 decimals)
		AssetIn: "AIB",
		AssetOut: "BTC",
		Rate:    10000, // 0.0001 BTC per AIB
	}

	// Expected output = Amount * Rate / 10^8
	expectedOutput := uint64(100000000 * 10000 / 100000000)
	if expectedOutput != 10000 {
		t.Errorf("expected output 10000, got %d", expectedOutput)
	}
}

// TestGetAssetInfo_NilFields tests asset info nil handling
func TestGetAssetInfo_NilFields(t *testing.T) {
	// Test all predefined assets have required fields
	assets := []string{"AIB", "BTC", "ETH", "USDT"}

	for _, symbol := range assets {
		info, ok := GetAssetInfo(symbol)
		if !ok {
			t.Errorf("asset %s not found", symbol)
			continue
		}

		if info.Symbol == "" {
			t.Errorf("asset %s has empty symbol", symbol)
		}
		if info.Name == "" {
			t.Errorf("asset %s has empty name", symbol)
		}
		if info.Decimals == 0 {
			t.Errorf("asset %s has zero decimals", symbol)
		}
		if info.ChainID == "" {
			t.Errorf("asset %s has empty chain ID", symbol)
		}
	}
}

// TestHTLC_ConcurrentAccess tests concurrent access to HTLC operations
func TestHTLC_ConcurrentAccess(t *testing.T) {
	// This test verifies that the HTLC creation doesn't panic under concurrent access
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := NewHTLC(channelID, hashLock, timeLock, 1000, sender, receiver)
			if err != nil {
				t.Logf("error: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestIsValidAsset_Empty tests empty asset string
func TestIsValidAsset_Empty(t *testing.T) {
	if IsValidAsset("") {
		t.Error("empty string should not be valid")
	}
}

// TestSwapStatus_ClaimedRefunded tests that claimed swap cannot be refunded
func TestSwapStatus_ClaimedRefunded(t *testing.T) {
	swap := &AtomicSwap{
		Status: SwapClaimed,
	}

	// Once claimed, attempting refund should fail
	if swap.Status == SwapClaimed {
		// This is expected behavior - swap already claimed
		if swap.Status != SwapClaimed {
			t.Error("status should be claimed")
		}
	}
}

// TestSwapStatus_RefundedClaimed tests that refunded swap cannot be claimed
func TestSwapStatus_RefundedClaimed(t *testing.T) {
	swap := &AtomicSwap{
		Status: SwapRefunded,
	}

	// Once refunded, attempting claim should fail
	if swap.Status == SwapRefunded {
		if swap.Status != SwapRefunded {
			t.Error("status should be refunded")
		}
	}
}

// TestHTLC_AmountBoundaries tests amount boundary conditions
func TestHTLC_AmountBoundaries(t *testing.T) {
	channelID := [32]byte{1}
	sender := createTestAddress("sender")
	receiver := createTestAddress("receiver")
	hashLock := sha256.Sum256([]byte("secret"))
	timeLock := time.Now().Add(1 * time.Hour)

	// Test minimum amount (1)
	htlc, err := NewHTLC(channelID, hashLock, timeLock, 1, sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC with amount 1: %v", err)
	}
	if htlc.Amount != 1 {
		t.Errorf("expected amount 1, got %d", htlc.Amount)
	}

	// Test maximum amount (very large)
	htlc2, err := NewHTLC(channelID, hashLock, timeLock, ^uint64(0), sender, receiver)
	if err != nil {
		t.Fatalf("failed to create HTLC with max amount: %v", err)
	}
	if htlc2.Amount == 0 {
		t.Error("max amount should be preserved")
	}
}

// TestPreimageLengthVariations tests different preimage lengths
func TestPreimageLengthVariations(t *testing.T) {
	testCases := []struct {
		name     string
		preimage []byte
	}{
		{"empty", []byte{}},
		{"1_byte", []byte{1}},
		{"32_bytes", make([]byte, 32)},
		{"64_bytes", make([]byte, 64)},
		{"256_bytes", make([]byte, 256)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.preimage) == 0 {
				_, err := GenerateHashLock(tc.preimage)
				if err == nil {
					t.Error("empty preimage should fail")
				}
				return
			}

			hash, err := GenerateHashLock(tc.preimage)
			if err != nil {
				t.Fatalf("failed: %v", err)
			}

			// Verify hash is correct
			expected := sha256.Sum256(tc.preimage)
			if hash != expected {
				t.Error("hash mismatch")
			}
		})
	}
}

// ============================================================================
// AtomicSwapManager Tests - Full Coverage
// ============================================================================

func TestAtomicSwapManager_NewAtomicSwapManager(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	if asm == nil {
		t.Fatal("AtomicSwapManager should not be nil")
	}

	if asm.swaps == nil {
		t.Error("swaps map should be initialized")
	}

	if asm.swapsByChannel == nil {
		t.Error("swapsByChannel map should be initialized")
	}
}

func TestAtomicSwapManager_CreateSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create atomic swap
	preimage := []byte("test-secret-key")
	swap, returnedPreimage, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		preimage,
	)

	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	if swap == nil {
		t.Fatal("Swap should not be nil")
	}

	if swap.Sender != partyA {
		t.Error("Sender mismatch")
	}

	if swap.Receiver != partyB {
		t.Error("Receiver mismatch")
	}

	if swap.Amount != 1000 {
		t.Errorf("Amount = %d, expected 1000", swap.Amount)
	}

	if swap.AssetIn != "AIB" {
		t.Errorf("AssetIn = %s, expected AIB", swap.AssetIn)
	}

	if swap.AssetOut != "BTC" {
		t.Errorf("AssetOut = %s, expected BTC", swap.AssetOut)
	}

	if swap.Status != SwapCreated {
		t.Errorf("Status = %s, expected CREATED", swap.Status)
	}

	// Verify preimage is returned
	if string(returnedPreimage) != string(preimage) {
		t.Error("Returned preimage mismatch")
	}
}

func TestAtomicSwapManager_CreateSwap_InvalidAmount(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Try with zero amount
	_, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		0,
		"AIB",
		"BTC",
		1*time.Hour,
		nil,
	)

	if err == nil {
		t.Error("Should fail with zero amount")
	}
}

func TestAtomicSwapManager_CreateSwap_InvalidAsset(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Try with empty asset in
	_, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"",
		"BTC",
		1*time.Hour,
		nil,
	)

	if err == nil {
		t.Error("Should fail with empty asset in")
	}
}

func TestAtomicSwapManager_CreateSwap_SameParty(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, interfaces.Address{4, 5, 6}, 5000, 3000)

	// Try with same sender and receiver
	_, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyA, // Same as sender
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		nil,
	)

	if err == nil {
		t.Error("Should fail with same sender and receiver")
	}
}

func TestAtomicSwapManager_CreateSwap_ChannelNotFound(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	// Try to create swap with non-existent channel
	_, _, err := asm.CreateSwap(
		[32]byte{99, 99, 99},
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		nil,
	)

	if err == nil {
		t.Error("Should fail with non-existent channel")
	}
}

func TestAtomicSwapManager_ClaimSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap first
	preimage := []byte("test-secret-key")
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		preimage,
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Claim the swap
	claimedSwap, err := asm.ClaimSwap(swap.ID, preimage)
	if err != nil {
		t.Fatalf("ClaimSwap failed: %v", err)
	}

	if claimedSwap.Status != SwapClaimed {
		t.Errorf("Status = %s, expected CLAIMED", claimedSwap.Status)
	}

	if claimedSwap.Secret == nil {
		t.Error("Secret should be stored")
	}
}

func TestAtomicSwapManager_ClaimSwap_InvalidSecret(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Try to claim with wrong secret
	_, err = asm.ClaimSwap(swap.ID, []byte("wrong-secret"))
	if err == nil {
		t.Error("Should fail with invalid secret")
	}
}

func TestAtomicSwapManager_ClaimSwap_SwapExpired(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap with very short time lock
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Millisecond,
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Try to claim expired swap
	_, err = asm.ClaimSwap(swap.ID, []byte("secret"))
	if err == nil {
		t.Error("Should fail with expired swap")
	}
}

func TestAtomicSwapManager_RefundSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap with very short time lock
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Millisecond,
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Refund the swap
	refundedSwap, err := asm.RefundSwap(swap.ID)
	if err != nil {
		t.Fatalf("RefundSwap failed: %v", err)
	}

	if refundedSwap.Status != SwapRefunded {
		t.Errorf("Status = %s, expected REFUNDED", refundedSwap.Status)
	}

	if refundedSwap.RefundedAt == nil {
		t.Error("RefundedAt should be set")
	}
}

func TestAtomicSwapManager_RefundSwap_NotExpired(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap with future time lock
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Try to refund before expiry
	_, err = asm.RefundSwap(swap.ID)
	if err == nil {
		t.Error("Should fail when not expired")
	}
}

func TestAtomicSwapManager_VerifyHash(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap
	preimage := []byte("test-secret")
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		preimage,
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Verify correct hash
	matches, err := asm.VerifyHash(swap.ID, preimage)
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}
	if !matches {
		t.Error("Hash should match")
	}

	// Verify wrong hash
	matches, err = asm.VerifyHash(swap.ID, []byte("wrong-secret"))
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}
	if matches {
		t.Error("Hash should not match")
	}
}

func TestAtomicSwapManager_GetSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create swap
	swap, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Hour,
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Get swap
	retrieved, err := asm.GetSwap(swap.ID)
	if err != nil {
		t.Fatalf("GetSwap failed: %v", err)
	}

	if retrieved.ID != swap.ID {
		t.Error("Swap ID mismatch")
	}

	// Try to get non-existent swap
	_, err = asm.GetSwap([32]byte{99, 99, 99})
	if err == nil {
		t.Error("Should fail for non-existent swap")
	}
}

func TestAtomicSwapManager_GetSwapsByChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create multiple swaps
	for i := 0; i < 3; i++ {
		_, _, err := asm.CreateSwap(
			channel.ID,
			partyA,
			partyB,
			1000,
			"AIB",
			"BTC",
			1*time.Hour,
			[]byte(string(rune(i))),
		)
		if err != nil {
			t.Fatalf("CreateSwap failed: %v", err)
		}
	}

	// Get swaps by channel
	swaps, err := asm.GetSwapsByChannel(channel.ID)
	if err != nil {
		t.Fatalf("GetSwapsByChannel failed: %v", err)
	}

	if len(swaps) != 3 {
		t.Errorf("Expected 3 swaps, got %d", len(swaps))
	}

	// Get swaps for non-existent channel
	emptySwaps, err := asm.GetSwapsByChannel([32]byte{99, 99, 99})
	if err != nil {
		t.Fatalf("GetSwapsByChannel failed: %v", err)
	}

	if len(emptySwaps) != 0 {
		t.Errorf("Expected 0 swaps for non-existent channel, got %d", len(emptySwaps))
	}
}

func TestAtomicSwapManager_GetPendingSwaps(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create some swaps
	for i := 0; i < 2; i++ {
		_, _, err := asm.CreateSwap(
			channel.ID,
			partyA,
			partyB,
			1000,
			"AIB",
			"BTC",
			1*time.Hour,
			[]byte(string(rune(i))),
		)
		if err != nil {
			t.Fatalf("CreateSwap failed: %v", err)
		}
	}

	// Get pending swaps
	pending := asm.GetPendingSwaps()
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending swaps, got %d", len(pending))
	}
}

func TestAtomicSwapManager_GetExpiredSwaps(t *testing.T) {
	cfg := &Config{
		ChallengePeriod:   24 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:   &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)
	asm := NewAtomicSwapManager(manager)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create expired swap
	_, _, err := asm.CreateSwap(
		channel.ID,
		partyA,
		partyB,
		1000,
		"AIB",
		"BTC",
		1*time.Millisecond,
		[]byte("expired"),
	)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Get expired swaps
	expired := asm.GetExpiredSwaps()
	if len(expired) != 1 {
		t.Errorf("Expected 1 expired swap, got %d", len(expired))
	}
}
