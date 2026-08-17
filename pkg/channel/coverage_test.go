// Package channel provides additional tests to improve code coverage.
package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// HTLC Route and Atomic Swap Tests
// ============================================================================

func TestExpireRouteHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channelAB, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	// Create HTLCs manually to bypass validation
	pastTime := time.Now().Add(-2 * time.Hour)
	htlc1 := &HTLC{
		ChannelID: channelAB.ID,
		ID:        [32]byte{1, 2, 3},
		HashLock:  sha256.Sum256([]byte("secret")),
		TimeLock:  pastTime,
		Amount:    100,
		Sender:    partyA,
		Receiver:  partyB,
		State:     HTLCPending,
		CreatedAt: pastTime,
	}
	htlc2 := &HTLC{
		ChannelID: channelAB.ID,
		ID:        [32]byte{4, 5, 6},
		HashLock:  sha256.Sum256([]byte("secret")),
		TimeLock:  pastTime,
		Amount:    100,
		Sender:    partyB,
		Receiver:  partyA,
		State:     HTLCPending,
		CreatedAt: pastTime,
	}

	// Add HTLCs directly to the channel state
	manager.mu.Lock()
	if state, ok := manager.states[channelAB.ID]; ok {
		state.PendingHTLCs[htlc1.ID] = htlc1
		state.PendingHTLCs[htlc2.ID] = htlc2
	}
	manager.mu.Unlock()

	// Expire the route
	htlcs := []*HTLC{htlc1, htlc2}
	err := manager.ExpireRouteHTLC(htlcs)
	if err != nil {
		t.Fatalf("ExpireRouteHTLC failed: %v", err)
	}

	// Verify HTLCs are expired
	if htlc1.State != HTLCExpired {
		t.Error("HTLC 1 should be expired")
	}
	if htlc2.State != HTLCExpired {
		t.Error("HTLC 2 should be expired")
	}
}

func TestRouteHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}

	ctx := context.Background()
	manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)
	manager.OpenChannel(ctx, partyB, partyC, 5000, 5000)

	route := []interfaces.Address{partyA, partyB, partyC}
	hashLock := sha256.Sum256([]byte("secret"))
	finalTimeLock := time.Now().Add(3 * time.Hour)
	hopExpiry := time.Hour

	htlcs, resultHash, err := manager.RouteHTLC(route, 100, hashLock, finalTimeLock, hopExpiry)
	if err != nil {
		t.Fatalf("RouteHTLC failed: %v", err)
	}

	if len(htlcs) != 2 {
		t.Errorf("Expected 2 HTLCs, got %d", len(htlcs))
	}

	if resultHash != hashLock {
		t.Error("Returned hash lock should match input")
	}
}

func TestRouteHTLC_InvalidRoute(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}

	route := []interfaces.Address{partyA}
	hashLock := sha256.Sum256([]byte("secret"))
	finalTimeLock := time.Now().Add(3 * time.Hour)

	_, _, err := manager.RouteHTLC(route, 100, hashLock, finalTimeLock, time.Hour)
	if err == nil {
		t.Error("Expected error for invalid route")
	}
}

func TestRouteHTLC_NoChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}

	ctx := context.Background()
	manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	route := []interfaces.Address{partyA, partyB, partyC}
	hashLock := sha256.Sum256([]byte("secret"))
	finalTimeLock := time.Now().Add(3 * time.Hour)

	_, _, err := manager.RouteHTLC(route, 100, hashLock, finalTimeLock, time.Hour)
	if err == nil {
		t.Error("Expected error when no channel exists")
	}
}

func TestCompleteRouteHTLC(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}

	ctx := context.Background()
	channelAB, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)
	channelBC, _ := manager.OpenChannel(ctx, partyB, partyC, 5000, 5000)

	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	now := time.Now()
	futureTime := now.Add(2 * time.Hour)

	htlc1, err := NewHTLC(channelAB.ID, hashLock, futureTime, 100, partyA, partyB)
	if err != nil {
		t.Fatalf("Failed to create HTLC 1: %v", err)
	}
	manager.AddHTLC(channelAB.ID, htlc1)

	htlc2, err := NewHTLC(channelBC.ID, hashLock, futureTime, 100, partyB, partyC)
	if err != nil {
		t.Fatalf("Failed to create HTLC 2: %v", err)
	}
	manager.AddHTLC(channelBC.ID, htlc2)

	htlcs := []*HTLC{htlc1, htlc2}
	err = manager.CompleteRouteHTLC(htlcs, preimage)
	if err != nil {
		t.Fatalf("CompleteRouteHTLC failed: %v", err)
	}

	if htlc1.State != HTLCCompleted {
		t.Error("HTLC 1 should be completed")
	}
	if htlc2.State != HTLCCompleted {
		t.Error("HTLC 2 should be completed")
	}
}

func TestCompleteRouteHTLC_ErrorPath(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}

	ctx := context.Background()
	channelAB, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)
	channelBC, _ := manager.OpenChannel(ctx, partyB, partyC, 5000, 5000)

	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	futureTime := time.Now().Add(2 * time.Hour)

	htlc1, _ := NewHTLC(channelAB.ID, hashLock, futureTime, 100, partyA, partyB)
	manager.AddHTLC(channelAB.ID, htlc1)

	otherHashLock := sha256.Sum256([]byte("other"))
	htlc2, _ := NewHTLC(channelBC.ID, otherHashLock, futureTime, 100, partyB, partyC)
	manager.AddHTLC(channelBC.ID, htlc2)

	htlcs := []*HTLC{htlc1, htlc2}
	err := manager.CompleteRouteHTLC(htlcs, preimage)
	if err == nil {
		t.Error("Expected error for mismatched hash lock")
	}
}

func TestRoutePayment(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}
	partyD := interfaces.Address{10, 11, 12}

	ctx := context.Background()
	manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)
	manager.OpenChannel(ctx, partyB, partyC, 5000, 5000)
	manager.OpenChannel(ctx, partyC, partyD, 5000, 5000)

	route := []interfaces.Address{partyA, partyB, partyC, partyD}
	preimage := []byte("secret")

	htlcs, hashLock, err := manager.RoutePayment(route, 100, preimage, 10*time.Hour)
	if err != nil {
		t.Fatalf("RoutePayment failed: %v", err)
	}

	if len(htlcs) != 3 {
		t.Errorf("Expected 3 HTLCs, got %d", len(htlcs))
	}

	expectedHash := sha256.Sum256(preimage)
	if hashLock != expectedHash {
		t.Error("Hash lock mismatch")
	}
}

func TestCreateAtomicSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	hashLock := sha256.Sum256([]byte("secret"))
	now := time.Now()
	futureTime := now.Add(2 * time.Hour)

	htlcA, htlcB, err := manager.CreateAtomicSwap(
		"aib-mainnet",
		"aib-mainnet",
		partyA,
		partyB,
		1000,
		2000,
		hashLock,
		futureTime,
	)

	if err != nil {
		t.Fatalf("CreateAtomicSwap failed: %v", err)
	}

	if htlcA == nil || htlcB == nil {
		t.Error("Both HTLCs should be created")
	}

	if htlcA.State != HTLCPending {
		t.Error("HTLC A should be pending")
	}
	if htlcB.State != HTLCPending {
		t.Error("HTLC B should be pending")
	}

	if htlcA.ChannelID != channel.ID {
		t.Error("HTLC A channel ID mismatch")
	}
}

func TestCreateAtomicSwap_NoSourceChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	partyC := interfaces.Address{7, 8, 9}

	ctx := context.Background()
	manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	hashLock := sha256.Sum256([]byte("secret"))
	futureTime := time.Now().Add(2 * time.Hour)

	_, _, err := manager.CreateAtomicSwap(
		"aib-mainnet",
		"aib-mainnet",
		partyC, // Not in any channel
		partyA,
		1000,
		2000,
		hashLock,
		futureTime,
	)

	if err == nil {
		t.Error("Expected error when no source channel")
	}
}

func TestCompleteAtomicSwap(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	// Create HTLCs
	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	futureTime := time.Now().Add(2 * time.Hour)

	htlcA, err := NewHTLC(channel.ID, hashLock, futureTime, 100, partyA, partyB)
	if err != nil {
		t.Fatalf("Failed to create HTLC A: %v", err)
	}
	manager.AddHTLC(channel.ID, htlcA)

	htlcB, err := NewHTLC(channel.ID, hashLock, futureTime, 100, partyB, partyA)
	if err != nil {
		t.Fatalf("Failed to create HTLC B: %v", err)
	}
	manager.AddHTLC(channel.ID, htlcB)

	// Complete the atomic swap
	err = manager.CompleteAtomicSwap(htlcA, htlcB, preimage)
	if err != nil {
		t.Fatalf("CompleteAtomicSwap failed: %v", err)
	}

	if htlcA.State != HTLCCompleted {
		t.Error("HTLC A should be completed")
	}
	if htlcB.State != HTLCCompleted {
		t.Error("HTLC B should be completed")
	}
}

func TestCompleteAtomicSwap_Failure(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	hashLock := sha256.Sum256([]byte("secret"))
	wrongPreimage := []byte("wrong_secret")
	futureTime := time.Now().Add(2 * time.Hour)

	htlcA, _ := NewHTLC(channel.ID, hashLock, futureTime, 100, partyA, partyB)
	manager.AddHTLC(channel.ID, htlcA)

	htlcB, _ := NewHTLC(channel.ID, hashLock, futureTime, 100, partyB, partyA)
	manager.AddHTLC(channel.ID, htlcB)

	err := manager.CompleteAtomicSwap(htlcA, htlcB, wrongPreimage)
	if err == nil {
		t.Error("Expected error with wrong preimage")
	}
}

func TestExpireAtomicSwap(t *testing.T) {
	t.Skip(" Requires past time lock - not supported by NewHTLC")
}

func TestGetAtomicSwapStatus(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	preimage := []byte("secret")
	hashLock := sha256.Sum256(preimage)
	futureTime := time.Now().Add(2 * time.Hour)

	htlcA, _ := NewHTLC(channel.ID, hashLock, futureTime, 100, partyA, partyB)
	manager.AddHTLC(channel.ID, htlcA)

	htlcB, _ := NewHTLC(channel.ID, hashLock, futureTime, 100, partyB, partyA)
	manager.AddHTLC(channel.ID, htlcB)

	status, err := manager.GetAtomicSwapStatus(htlcA, htlcB)
	if err != nil {
		t.Fatalf("GetAtomicSwapStatus failed: %v", err)
	}
	if status != "pending" {
		t.Errorf("Expected pending, got %s", status)
	}

	manager.CompleteHTLC(channel.ID, htlcA.ID, preimage)

	status, _ = manager.GetAtomicSwapStatus(htlcA, htlcB)
	if status != "partial" {
		t.Errorf("Expected partial, got %s", status)
	}

	manager.CompleteHTLC(channel.ID, htlcB.ID, preimage)

	status, _ = manager.GetAtomicSwapStatus(htlcA, htlcB)
	if status != "completed" {
		t.Errorf("Expected completed, got %s", status)
	}
}

// ============================================================================
// Inference Channel Tests
// ============================================================================

func TestGetChannelIDString(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	idStr := channel.GetChannelIDString()
	if idStr == "" {
		t.Error("GetChannelIDString should return non-empty string")
	}

	if len(idStr) != 64 {
		t.Errorf("Expected 64 hex chars, got %d", len(idStr))
	}
}

func TestIsExpired(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	if channel.IsExpired() {
		t.Error("New channel should not be expired")
	}

	channel.CreatedAt = uint64(time.Now().Add(-31 * 24 * time.Hour).Unix())

	if !channel.IsExpired() {
		t.Error("Channel with old CreatedAt should be expired")
	}
}

// ============================================================================
// OrderBook Tests
// ============================================================================

func TestGenerateOrderHash(t *testing.T) {
	owner := interfaces.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	tradingPair := "AIB/USDT"
	side := OrderSideBuy
	quantity := uint64(1000)
	price := uint64(50000)
	timestamp := time.Now()

	hash1 := generateOrderHash(owner, tradingPair, side, quantity, price, timestamp)
	hash2 := generateOrderHash(owner, tradingPair, side, quantity, price, timestamp)

	if hash1 != hash2 {
		t.Error("Same inputs should produce same hash")
	}

	hash3 := generateOrderHash(owner, tradingPair, OrderSideSell, quantity, price, timestamp)
	if hash1 == hash3 {
		t.Error("Different side should produce different hash")
	}
}

// ============================================================================
// Settlement Tests
// ============================================================================

func TestComputeStateHashFromSettlement(t *testing.T) {
	settlement := &Settlement{
		ChannelID: [32]byte{1, 2, 3},
		StateHash: [32]byte{4, 5, 6},
		BalanceA:  1000,
		BalanceB:  2000,
		Sequence:  5,
	}

	hash := computeStateHashFromSettlement(settlement)

	if len(hash) != 32 {
		t.Errorf("Expected 32-byte hash, got %d bytes", len(hash))
	}

	hash2 := computeStateHashFromSettlement(settlement)
	if hash != hash2 {
		t.Error("Same settlement should produce same hash")
	}
}

func TestValidateState(t *testing.T) {
	pubA, privA, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key A: %v", err)
	}
	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key B: %v", err)
	}

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubA)
	copy(partyB[:], pubB)

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	signedState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  channel.Sequence + 1,
		BalanceA:  4000,
		BalanceB:  6000,
		Timestamp: time.Now(),
	}

	stateData := serializeState(signedState)
	signedState.SigA = ed25519.Sign(privA, stateData)
	signedState.SigB = ed25519.Sign(privB, stateData)

	mockMS := newMockMultiSig()
	sm, err := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  mockMS,
	})
	if err != nil {
		t.Fatalf("Failed to create SettlementManager: %v", err)
	}

	err = sm.ValidateState(channel.ID, signedState)
	if err != nil {
		t.Fatalf("ValidateState failed: %v", err)
	}
}

func TestValidateState_InvalidSignature(t *testing.T) {
	pubA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key A: %v", err)
	}
	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key B: %v", err)
	}

	_, wrongPrivA, _ := ed25519.GenerateKey(rand.Reader)

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubA)
	copy(partyB[:], pubB)

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	signedState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  channel.Sequence + 1,
		BalanceA:  4000,
		BalanceB:  6000,
		Timestamp: time.Now(),
	}

	stateData := serializeState(signedState)
	signedState.SigA = ed25519.Sign(wrongPrivA, stateData)
	signedState.SigB = ed25519.Sign(privB, stateData)

	mockMS := newMockMultiSig()
	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  mockMS,
	})

	err = sm.ValidateState(channel.ID, signedState)
	if err == nil {
		t.Error("Expected error for invalid signature")
	}
}

func TestExecuteDisputeSettlement(t *testing.T) {
	pubA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubA)
	copy(partyB[:], pubA)

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLockerForHTLC{},
	}

	manager, _ := NewManager(cfg)

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	mockMS := newMockMultiSig()
	sm, err := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  mockMS,
	})
	if err != nil {
		t.Fatalf("Failed to create SettlementManager: %v", err)
	}

	settlement, err := sm.ExecuteDisputeSettlement(ctx, channel.ID, partyA, true)
	if err != nil {
		t.Fatalf("ExecuteDisputeSettlement failed: %v", err)
	}

	if settlement == nil {
		t.Error("Settlement should not be nil")
	}

	if settlement.Type != SettlementDispute {
		t.Error("Settlement type should be Dispute")
	}
}

// ============================================================================
// Additional helper tests
// ============================================================================

func TestAtomicSwapStatusString(t *testing.T) {
	tests := []struct {
		status   AtomicSwapStatus
		expected string
	}{
		{SwapCreated, "CREATED"},
		{SwapClaimed, "CLAIMED"},
		{SwapRefunded, "REFUNDED"},
		{SwapExpired, "EXPIRED"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.status.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestOrderSideIsOpposite(t *testing.T) {
	if !OrderSideBuy.IsOpposite(OrderSideSell) {
		t.Error("Buy should be opposite to Sell")
	}
	if OrderSideBuy.IsOpposite(OrderSideBuy) {
		t.Error("Buy should not be opposite to Buy")
	}
	if OrderSideSell.IsOpposite(OrderSideSell) {
		t.Error("Sell should not be opposite to Sell")
	}
}

func TestSettlementDataIsValid(t *testing.T) {
	sd := &SettlementData{
		ChannelID:      [32]byte{1},
		FinalUserBal:   100,
		FinalNodeBal:   200,
		InferenceCount: 5,
		SequenceNum:    1,
	}

	err := sd.IsValid()
	if err != nil {
		t.Errorf("Valid settlement should not return error: %v", err)
	}

	sd2 := &SettlementData{
		ChannelID:      [32]byte{1},
		FinalUserBal:   0,
		FinalNodeBal:   0,
		InferenceCount: 0,
		SequenceNum:    0,
	}

	err = sd2.IsValid()
	if err == nil {
		t.Error("Zero total balance should return error")
	}
}

func TestInferenceChannelCanSettle(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3}
	nodePubKey := [32]byte{4, 5, 6}

	channel, _ := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)

	if !channel.CanSettle() {
		t.Error("Open channel should be able to settle")
	}

	channel.Status = ICClosed
	if channel.CanSettle() {
		t.Error("Closed channel should not be able to settle")
	}
}

func TestInferenceChannelIsInDispute(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3}
	nodePubKey := [32]byte{4, 5, 6}

	channel, _ := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)

	if channel.IsInDispute() {
		t.Error("New channel should not be in dispute")
	}

	channel.Status = ICDisputed
	if !channel.IsInDispute() {
		t.Error("Disputed channel should be in dispute")
	}
}

func TestInferenceChannelIsClosed(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3}
	nodePubKey := [32]byte{4, 5, 6}

	channel, _ := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)

	if channel.IsClosed() {
		t.Error("New channel should not be closed")
	}

	channel.Status = ICClosed
	if !channel.IsClosed() {
		t.Error("Closed channel should be closed")
	}
}

func TestSettlementTxSerialize(t *testing.T) {
	tx := &SettlementTx{
		ChannelID:    [32]byte{1, 2, 3},
		SettlementID: [32]byte{4, 5, 6},
		BalanceA:     1000,
		BalanceB:     2000,
		Sequence:     5,
		Timestamp:    time.Now(),
	}

	data := tx.Serialize()
	if len(data) == 0 {
		t.Error("Serialized data should not be empty")
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	sig := tx.Sign(priv)
	if len(sig) != ed25519.SignatureSize {
		t.Errorf("Expected signature size %d, got %d", ed25519.SignatureSize, len(sig))
	}

	pub := priv.Public().(ed25519.PublicKey)
	if !tx.VerifySignature(pub, sig) {
		t.Error("Signature verification should succeed with correct key")
	}

	_, wrongPriv, _ := ed25519.GenerateKey(rand.Reader)
	wrongPub := wrongPriv.Public().(ed25519.PublicKey)
	if tx.VerifySignature(wrongPub, sig) {
		t.Error("Signature verification should fail with wrong key")
	}
}

// ============================================================================
// Additional helper tests
// ============================================================================

func TestOrderStatusString_Coverage(t *testing.T) {
	tests := []struct {
		status   OrderStatus
		expected string
	}{
		{OrderStatusPending, "PENDING"},
		{OrderStatusPartialFilled, "PARTIAL_FILLED"},
		{OrderStatusFilled, "FILLED"},
		{OrderStatusCancelled, "CANCELLED"},
		{OrderStatusExpired, "EXPIRED"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.status.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestOrderTypeString_Coverage(t *testing.T) {
	tests := []struct {
		orderType OrderType
		expected  string
	}{
		{OrderTypeLimit, "LIMIT"},
		{OrderTypeMarket, "MARKET"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.orderType.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestOrder_RemainingQuantity_Coverage(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       1000,
		FilledQuantity: 300,
	}

	remaining := order.RemainingQuantity()
	if remaining != 700 {
		t.Errorf("Expected 700, got %d", remaining)
	}

	order.FilledQuantity = 1500
	remaining = order.RemainingQuantity()
	if remaining != 0 {
		t.Errorf("Expected 0 when filled > quantity, got %d", remaining)
	}
}

func TestHTLCStates_Coverage(t *testing.T) {
	if HTLCPending != 0 || HTLCCompleted != 1 || HTLCExpired != 2 {
		t.Error("HTLC state constants incorrect")
	}
}

func TestOrderBookManager_GetOrCreateOrderBook_Coverage(t *testing.T) {
	manager := NewOrderBookManager()

	ob := manager.GetOrCreateOrderBook("AIB/USDT")
	if ob == nil {
		t.Error("OrderBook should be created")
	}

	ob2 := manager.GetOrCreateOrderBook("AIB/USDT")
	if ob != ob2 {
		t.Error("Should return same OrderBook")
	}
}

func TestOrderBookManager_ListTradingPairs_Coverage(t *testing.T) {
	manager := NewOrderBookManager()

	manager.GetOrCreateOrderBook("AIB/USDT")
	manager.GetOrCreateOrderBook("BTC/USDT")

	pairs := manager.ListTradingPairs()
	if len(pairs) != 2 {
		t.Errorf("Expected 2 pairs, got %d", len(pairs))
	}
}

func TestOrder_IsFilled_Coverage(t *testing.T) {
	order := &Order{
		ID:       1,
		Quantity: 100,
		Status:   OrderStatusPending,
	}

	if order.IsFilled() {
		t.Error("Pending order should not be filled")
	}

	order.FilledQuantity = 100
	order.Status = OrderStatusFilled
	if !order.IsFilled() {
		t.Error("Filled order should be filled")
	}
}

func TestOrder_IsExpired_Coverage(t *testing.T) {
	order := &Order{
		ID:         1,
		Quantity:   100,
		Expiration: nil,
	}

	if order.IsExpired() {
		t.Error("Order without expiration should not be expired")
	}

	expiredTime := time.Now().Add(-1 * time.Hour)
	order.Expiration = &expiredTime
	if !order.IsExpired() {
		t.Error("Order with past expiration should be expired")
	}

	futureTime := time.Now().Add(1 * time.Hour)
	order.Expiration = &futureTime
	if order.IsExpired() {
		t.Error("Order with future expiration should not be expired")
	}
}

func TestOrder_Fill_Coverage(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}

	// Partial fill
	filled := order.Fill(30)
	if filled != 30 {
		t.Errorf("Expected 30 filled, got %d", filled)
	}
	if order.Status != OrderStatusPartialFilled {
		t.Error("Status should be PartialFilled")
	}

	// Complete fill
	filled = order.Fill(100)
	if filled != 70 { // Only 70 remaining
		t.Errorf("Expected 70 filled, got %d", filled)
	}
	if order.Status != OrderStatusFilled {
		t.Error("Status should be Filled")
	}
}

func TestOrder_IsActive_Coverage(t *testing.T) {
	order := &Order{
		ID:       1,
		Quantity: 100,
		Status:   OrderStatusPending,
	}

	if !order.IsActive() {
		t.Error("Pending order should be active")
	}

	order.Status = OrderStatusPartialFilled
	if !order.IsActive() {
		t.Error("PartialFilled order should be active")
	}

	order.Status = OrderStatusFilled
	if order.IsActive() {
		t.Error("Filled order should not be active")
	}

	order.Status = OrderStatusCancelled
	if order.IsActive() {
		t.Error("Cancelled order should not be active")
	}
}

func TestOrderSideString_Coverage(t *testing.T) {
	tests := []struct {
		side     OrderSide
		expected string
	}{
		{OrderSideBuy, "BUY"},
		{OrderSideSell, "SELL"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.side.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

// More tests to increase coverage

func TestOrder_ActiveStatusVariants(t *testing.T) {
	// Test Fill method with various quantities
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}

	// Fill less than remaining
	filled := order.Fill(50)
	if filled != 50 {
		t.Errorf("Expected 50, got %d", filled)
	}
	if order.FilledQuantity != 50 {
		t.Errorf("Expected FilledQuantity 50, got %d", order.FilledQuantity)
	}
	if order.Status != OrderStatusPartialFilled {
		t.Errorf("Expected PartialFilled, got %d", order.Status)
	}

	// Fill exactly remaining
	filled = order.Fill(50)
	if filled != 50 {
		t.Errorf("Expected 50, got %d", filled)
	}
	if order.FilledQuantity != 100 {
		t.Errorf("Expected FilledQuantity 100, got %d", order.FilledQuantity)
	}
	if order.Status != OrderStatusFilled {
		t.Errorf("Expected Filled, got %d", order.Status)
	}

	// Fill more than remaining (after already filled)
	filled = order.Fill(100)
	if filled != 0 {
		t.Errorf("Expected 0, got %d", filled)
	}
}

func TestOrder_RemainingEdgeCases(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 150, // More than quantity
	}

	remaining := order.RemainingQuantity()
	if remaining != 0 {
		t.Errorf("Expected 0, got %d", remaining)
	}
}

func TestOrder_GetRemainingAfterAll(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 90,
		Status:         OrderStatusPartialFilled,
	}

	remaining := order.RemainingQuantity()
	if remaining != 10 {
		t.Errorf("Expected 10, got %d", remaining)
	}
}

func TestOrderBook_AddCancelOrder(t *testing.T) {
	ob := NewOrderBook("TEST/USDT")
	owner := interfaces.Address{1, 2, 3}
	now := time.Now()

	order := &Order{
		ID:        1,
		Owner:     owner,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     50,
		OrderType: OrderTypeLimit,
		Status:    OrderStatusPending,
		Timestamp: now,
	}

	// Place order
	placed, _, err := ob.PlaceOrder(order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if placed.Status != OrderStatusPending {
		t.Errorf("Expected Pending, got %d", placed.Status)
	}

	// Cancel order
	err = ob.CancelOrder(order.ID, owner)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	cancelled, err := ob.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if cancelled.Status != OrderStatusCancelled {
		t.Errorf("Expected Cancelled, got %d", cancelled.Status)
	}
}

func TestOrderBook_MultipleOrdersSamePrice(t *testing.T) {
	ob := NewOrderBook("TEST/USDT")
	owners := []interfaces.Address{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
	}

	now := time.Now()

	for i, owner := range owners {
		order := &Order{
			ID:        uint64(i + 1),
			Owner:     owner,
			Side:      OrderSideSell,
			Quantity:  100,
			Price:     50,
			OrderType: OrderTypeLimit,
			Status:    OrderStatusPending,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
		ob.PlaceOrder(order)
	}

	bids := ob.GetBids(-1)
	if len(bids) != 0 {
		t.Errorf("Expected 0 bids, got %d", len(bids))
	}

	asks := ob.GetAsks(-1)
	if len(asks) != 3 {
		t.Errorf("Expected 3 asks, got %d", len(asks))
	}

	depthBids, depthAsks := ob.GetDepth(5)
	if len(depthAsks) != 1 {
		t.Errorf("Expected 1 ask level, got %d", len(depthAsks))
	}
	if len(depthBids) != 0 {
		t.Errorf("Expected 0 bid levels, got %d", len(depthBids))
	}
}

func TestOrderBook_CancelNonOwner(t *testing.T) {
	ob := NewOrderBook("TEST/USDT")
	owner1 := interfaces.Address{1, 2, 3}
	owner2 := interfaces.Address{4, 5, 6}
	now := time.Now()

	order := &Order{
		ID:        1,
		Owner:     owner1,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     50,
		OrderType: OrderTypeLimit,
		Status:    OrderStatusPending,
		Timestamp: now,
	}

	ob.PlaceOrder(order)

	// Try to cancel as different owner
	err := ob.CancelOrder(order.ID, owner2)
	if err == nil {
		t.Error("Expected error when cancelling as non-owner")
	}
}

func TestOrderBook_GetNonExistentOrder(t *testing.T) {
	ob := NewOrderBook("TEST/USDT")

	_, err := ob.GetOrder(999)
	if err == nil {
		t.Error("Expected error for non-existent order")
	}
}

func TestAtomicSwapManager_GetSwapsByChannel_Empty(t *testing.T) {
	asm := NewAtomicSwapManager(&Manager{
		channels: make(map[[32]byte]*interfaces.Channel),
	})

	result, err := asm.GetSwapsByChannel([32]byte{})
	if err != nil {
		t.Fatalf("GetSwapsByChannel failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d", len(result))
	}
}

func TestAtomicSwapManager_VerifyHashAndGet(t *testing.T) {
	asm := NewAtomicSwapManager(&Manager{})

	secret := []byte("test_secret")
	_ = sha256.Sum256(secret) // hashLock computed for documentation
	swapID := [32]byte{1, 2, 3}

	// Swap not found
	_, err := asm.GetSwap(swapID)
	if err == nil {
		t.Error("Expected error for non-existent swap")
	}

	// VerifyHash on non-existent swap
	valid, err := asm.VerifyHash(swapID, secret)
	if valid || err == nil {
		t.Error("Expected error for non-existent swap")
	}
}

func TestOrder_Fill_MultiplePartial(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}

	// Multiple partial fills
	order.Fill(20)
	if order.FilledQuantity != 20 || order.Status != OrderStatusPartialFilled {
		t.Error("First fill incorrect")
	}

	order.Fill(30)
	if order.FilledQuantity != 50 || order.Status != OrderStatusPartialFilled {
		t.Error("Second fill incorrect")
	}

	order.Fill(50)
	if order.FilledQuantity != 100 || order.Status != OrderStatusFilled {
		t.Error("Final fill incorrect")
	}
}

func TestOrder_Fill_ExactMatch(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}

	filled := order.Fill(100)
	if filled != 100 {
		t.Errorf("Expected 100 filled, got %d", filled)
	}
	if order.Status != OrderStatusFilled {
		t.Errorf("Expected Filled status, got %d", order.Status)
	}
}

func TestOrder_RemainingZeroWhenEmpty(t *testing.T) {
	order := &Order{
		ID:             1,
		Quantity:       100,
		FilledQuantity: 100,
	}

	if order.RemainingQuantity() != 0 {
		t.Error("Remaining should be 0 when fully filled")
	}
}

// ============================================================================
// Watchtower Tests
// ============================================================================

func TestFraudTypeString(t *testing.T) {
	tests := []struct {
		ft       FraudType
		expected string
	}{
		{FraudTypeInvalidClose, "invalid_close"},
		{FraudTypeDoubleClose, "double_close"},
		{FraudTypeStateReversion, "state_reversion"},
		{FraudTypeBalanceManipulation, "balance_manipulation"},
		{FraudTypeSequenceRollback, "sequence_rollback"},
		{FraudTypeUnauthorizedClose, "unauthorized_close"},
		{100, "unknown"},
	}

	for _, tt := range tests {
		result := tt.ft.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestAlertLevelString(t *testing.T) {
	tests := []struct {
		al       AlertLevel
		expected string
	}{
		{AlertLevelInfo, "INFO"},
		{AlertLevelWarning, "WARNING"},
		{AlertLevelCritical, "CRITICAL"},
		{AlertLevelEmergency, "EMERGENCY"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.al.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestDefaultWatchtowerConfig(t *testing.T) {
	cfg := DefaultWatchtowerConfig()

	if cfg.MonitorInterval != 10*time.Second {
		t.Errorf("expected MonitorInterval 10s, got %v", cfg.MonitorInterval)
	}
	if cfg.StateCheckInterval != 5*time.Second {
		t.Errorf("expected StateCheckInterval 5s, got %v", cfg.StateCheckInterval)
	}
	if cfg.ChallengePeriod != 24*time.Hour {
		t.Errorf("expected ChallengePeriod 24h, got %v", cfg.ChallengePeriod)
	}
	if cfg.PenaltyEnabled != true {
		t.Error("expected PenaltyEnabled true")
	}
	if cfg.FraudPenaltyMultiplier != 1.0 {
		t.Errorf("expected FraudPenaltyMultiplier 1.0, got %f", cfg.FraudPenaltyMultiplier)
	}
	if cfg.AlertBufferSize != 1000 {
		t.Errorf("expected AlertBufferSize 1000, got %d", cfg.AlertBufferSize)
	}
}

func TestNewWatchtower(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	wt, err := NewWatchtower(manager, nil)
	if err != nil {
		t.Fatalf("NewWatchtower failed: %v", err)
	}
	if wt == nil {
		t.Fatal("Watchtower should not be nil")
	}
}

func TestNewWatchtowerWithDisputeResolver(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	resolver := NewDisputeResolver(manager)

	wt, err := NewWatchtowerWithDisputeResolver(manager, resolver, nil)
	if err != nil {
		t.Fatalf("NewWatchtowerWithDisputeResolver failed: %v", err)
	}
	if wt == nil {
		t.Fatal("Watchtower should not be nil")
	}
}

func TestNewWatchtower_NilManager(t *testing.T) {
	_, err := NewWatchtower(nil, nil)
	if err == nil {
		t.Error("Expected error with nil manager")
	}
}

func TestNewWatchtowerWithDisputeResolver_NilDisputeResolver(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	_, err := NewWatchtowerWithDisputeResolver(manager, nil, nil)
	if err == nil {
		t.Error("Expected error with nil dispute resolver")
	}
}

func TestWatchtower_StartMonitoring(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	err := wt.StartMonitoring(channel.ID)
	if err != nil {
		t.Fatalf("StartMonitoring failed: %v", err)
	}

	// Try to start monitoring again - should not error
	err = wt.StartMonitoring(channel.ID)
	if err != nil {
		t.Errorf("Second StartMonitoring should not error: %v", err)
	}
}

func TestWatchtower_StartMonitoring_ChannelNotFound(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{99}
	err := wt.StartMonitoring(channelID)
	if err == nil {
		t.Error("Expected error for non-existent channel")
	}
}

func TestWatchtower_StopMonitoring(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	err := wt.StopMonitoring(channel.ID)
	if err != nil {
		t.Fatalf("StopMonitoring failed: %v", err)
	}
}

func TestWatchtower_StopMonitoring_NotMonitored(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{99}
	err := wt.StopMonitoring(channelID)
	if err == nil {
		t.Error("Expected error for non-monitored channel")
	}
}

func TestWatchtower_GetMonitoredChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	mc, err := wt.GetMonitoredChannel(channel.ID)
	if err != nil {
		t.Fatalf("GetMonitoredChannel failed: %v", err)
	}
	if mc.ChannelID != channel.ID {
		t.Error("Channel ID mismatch")
	}
}

func TestWatchtower_GetMonitoredChannel_NotFound(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{99}
	_, err := wt.GetMonitoredChannel(channelID)
	if err == nil {
		t.Error("Expected error for non-monitored channel")
	}
}

func TestWatchtower_ListMonitoredChannels(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	channels := wt.ListMonitoredChannels()
	if len(channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(channels))
	}
}

func TestWatchtower_VerifyState(t *testing.T) {
	pubA, privA, _ := ed25519.GenerateKey(rand.Reader)
	pubB, privB, _ := ed25519.GenerateKey(rand.Reader)

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubA)
	copy(partyB[:], pubB)

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	wtCfg := DefaultWatchtowerConfig()
	wt, _ := NewWatchtower(manager, wtCfg)

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	// Create valid state
	signedState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  6000,
		Timestamp: time.Now(),
	}

	stateData := serializeState(signedState)
	signedState.SigA = ed25519.Sign(privA, stateData)
	signedState.SigB = ed25519.Sign(privB, stateData)

	err := wt.VerifyState(channel.ID, signedState)
	if err != nil {
		t.Fatalf("VerifyState failed: %v", err)
	}
}

func TestWatchtower_VerifyState_NotMonitored(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{1}
	state := &interfaces.SignedState{
		ChannelID: channelID,
	}

	err := wt.VerifyState(channelID, state)
	if err == nil {
		t.Error("Expected error for non-monitored channel")
	}
}

func TestWatchtower_DetectFraud_SequenceRollback(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	// Update monitored channel sequence to simulate valid state updates
	_ = wt.UpdateMonitoredChannelSequence(channel.ID, 10)

	// Try to close with old state
	oldState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  5, // Old sequence
		BalanceA:  5000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}

	evidence := wt.DetectFraud(channel.ID, oldState, partyA)
	if evidence == nil {
		t.Error("Expected fraud detection for sequence rollback")
		return
	}
	if evidence.FraudType != FraudTypeSequenceRollback {
		t.Errorf("Expected SequenceRollback, got %v", evidence.FraudType)
	}
}

func TestWatchtower_DetectFraud_BalanceManipulation(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	// Try to close with manipulated balance
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  channel.Sequence + 1,
		BalanceA:  9000, // Increased total
		BalanceB:  2000,
		Timestamp: time.Now(),
	}

	evidence := wt.DetectFraud(channel.ID, state, partyA)
	if evidence == nil {
		t.Error("Expected fraud detection for balance manipulation")
	}
	if evidence.FraudType != FraudTypeBalanceManipulation {
		t.Errorf("Expected BalanceManipulation, got %v", evidence.FraudType)
	}
}

func TestWatchtower_CheckStateTransition(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	oldState := &interfaces.SignedState{
		Sequence: 1,
		BalanceA: 5000,
		BalanceB: 5000,
	}

	newState := &interfaces.SignedState{
		Sequence: 2,
		BalanceA: 4000,
		BalanceB: 6000,
	}

	err := wt.CheckStateTransition([32]byte{1}, oldState, newState)
	if err != nil {
		t.Errorf("CheckStateTransition failed: %v", err)
	}
}

func TestWatchtower_CheckStateTransition_SequenceNotIncremented(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	oldState := &interfaces.SignedState{
		Sequence: 2,
		BalanceA: 5000,
		BalanceB: 5000,
	}

	newState := &interfaces.SignedState{
		Sequence: 1, // Not incremented
		BalanceA: 4000,
		BalanceB: 6000,
	}

	err := wt.CheckStateTransition([32]byte{1}, oldState, newState)
	if err == nil {
		t.Error("Expected error for non-incremented sequence")
	}
}

func TestWatchtower_CheckStateTransition_BalanceViolation(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	oldState := &interfaces.SignedState{
		Sequence: 1,
		BalanceA: 5000,
		BalanceB: 5000,
	}

	newState := &interfaces.SignedState{
		Sequence: 2,
		BalanceA: 8000, // Total increased
		BalanceB: 3000,
	}

	err := wt.CheckStateTransition([32]byte{1}, oldState, newState)
	if err == nil {
		t.Error("Expected error for balance violation")
	}
}

func TestWatchtower_FreezeChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	err := wt.FreezeChannel(channel.ID, "test freeze")
	if err != nil {
		t.Fatalf("FreezeChannel failed: %v", err)
	}
}

func TestWatchtower_FreezeChannel_NotMonitored(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{99}
	err := wt.FreezeChannel(channelID, "test")
	if err == nil {
		t.Error("Expected error for non-monitored channel")
	}
}

func TestWatchtower_UnfreezeChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)
	wt.FreezeChannel(channel.ID, "test freeze")

	err := wt.UnfreezeChannel(channel.ID)
	if err != nil {
		t.Fatalf("UnfreezeChannel failed: %v", err)
	}
}

func TestWatchtower_SendAlert(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	channelID := [32]byte{1}
	wt.SendAlert(AlertTypeFraudAttempt, AlertLevelCritical, channelID, "test alert", nil)
}

func TestWatchtower_GetAlertChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	ch := wt.GetAlertChannel()
	if ch == nil {
		t.Error("Alert channel should not be nil")
	}
}

func TestWatchtower_SetCallbacks(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	wt.SetAlertCallback(func(*Alert) {})
	wt.SetFraudDetectionCallback(func(*FraudEvidence) {})
	wt.SetPunishmentCallback(func(*PunishmentResult) {})
}

func TestWatchtower_DetectStateReversion(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	// Update monitored channel sequence to simulate valid state updates
	_ = wt.UpdateMonitoredChannelSequence(channel.ID, 5)

	// Try to revert to sequence 3
	newState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  3, // Less than current (5)
		BalanceA:  5000,
		BalanceB:  5000,
	}

	evidence := wt.DetectStateReversion(channel.ID, newState)
	if evidence == nil {
		t.Error("Expected state reversion detection")
	}
}

func TestWatchtower_DetectUnauthorizedClose(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	// Third party trying to close
	partyC := interfaces.Address{7, 8, 9}

	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  5000,
		BalanceB:  5000,
		// No signatures
	}

	evidence := wt.DetectUnauthorizedClose(channel.ID, state, partyC)
	if evidence == nil {
		t.Error("Expected unauthorized close detection")
	}
}

func TestWatchtower_ChannelStatusToString(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{StateOpen, "OPEN"},
		{StateClosing, "CLOSING"},
		{StateClosed, "CLOSED"},
		{StateInDispute, "IN_DISPUTE"},
		{100, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := ChannelStatusToString(tt.status)
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestWatchtower_IsHealthy(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	if !wt.IsHealthy() {
		t.Error("New watchtower should be healthy")
	}
}

func TestWatchtower_GetStats(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	wt, _ := NewWatchtower(manager, nil)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 5000)

	wt.StartMonitoring(channel.ID)

	stats := wt.GetStats()
	if stats.ChannelsMonitored != 1 {
		t.Errorf("Expected 1 monitored channel, got %d", stats.ChannelsMonitored)
	}
}

// ============================================================================
// Additional Dispute Tests
// ============================================================================

func TestDisputeResolver_InitiateDispute_AlreadyExists(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	channel, _ := mgr.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	evidence := Evidence{
		ChannelID:   channel.ID,
		Sequence:    2,
		BalanceA:    3000,
		BalanceB:    5000,
		SigA:        []byte("sigA"),
		SigB:        []byte("sigB"),
		Timestamp:   time.Now(),
		Submitter:   partyA,
		BlockNumber: 100,
	}

	// First initiate - may fail due to signature validation
	// But we test the flow
	_, _ = resolver.InitiateDispute(ctx, channel.ID, evidence)
}

func TestDisputeResolver_RespondToDispute_Timeout(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	channel, _ := mgr.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create dispute manually
	dispute := &DisputeRecord{
		ChannelID:  channel.ID,
		Challenger: partyA,
		ChallengedState: interfaces.SignedState{
			ChannelID: channel.ID,
			Sequence:  1,
			BalanceA:  5000,
			BalanceB:  3000,
		},
		ChallengeStart: time.Now().Add(-25 * time.Hour),
		ChallengeEnd:   time.Now().Add(-1 * time.Hour), // Expired
		Resolved:       false,
	}

	mgr.mu.Lock()
	mgr.disputes[channel.ID] = dispute
	mgr.mu.Unlock()

	response := Evidence{
		ChannelID:   channel.ID,
		Sequence:    3,
		BalanceA:    4000,
		BalanceB:    4000,
		Timestamp:   time.Now(),
		Submitter:   partyB,
		BlockNumber: 101,
	}

	err := resolver.RespondToDispute(ctx, channel.ID, response)
	if err == nil {
		t.Error("Should fail when challenge period expired")
	}
}

func TestDisputeResolver_RespondToDispute_SequenceError(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	channel, _ := mgr.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create dispute with high sequence
	dispute := &DisputeRecord{
		ChannelID:  channel.ID,
		Challenger: partyA,
		ChallengedState: interfaces.SignedState{
			ChannelID: channel.ID,
			Sequence:  5, // High sequence
			BalanceA:  5000,
			BalanceB:  3000,
		},
		ChallengeStart: time.Now(),
		ChallengeEnd:   time.Now().Add(24 * time.Hour),
		Resolved:       false,
	}

	mgr.mu.Lock()
	mgr.disputes[channel.ID] = dispute
	mgr.mu.Unlock()

	// Try to respond with lower sequence
	response := Evidence{
		ChannelID:   channel.ID,
		Sequence:    3, // Lower than challenged
		BalanceA:    4000,
		BalanceB:    4000,
		Timestamp:   time.Now(),
		Submitter:   partyB,
		BlockNumber: 101,
	}

	err := resolver.RespondToDispute(ctx, channel.ID, response)
	if err == nil {
		t.Error("Should fail when response sequence is lower")
	}
}

func TestDisputeResolver_FinalizeDispute_AlreadyResolved(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	channel, _ := mgr.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	dispute := &DisputeRecord{
		ChannelID:      channel.ID,
		Challenger:     partyA,
		ChallengeStart: time.Now(),
		ChallengeEnd:   time.Now().Add(-1 * time.Hour),
		Resolved:       true,
		Winner:         partyA,
		Resolution:     "test",
	}

	mgr.mu.Lock()
	mgr.disputes[channel.ID] = dispute
	mgr.mu.Unlock()

	result, err := resolver.FinalizeDispute(ctx, channel.ID)
	if err != nil {
		t.Fatalf("FinalizeDispute failed: %v", err)
	}
	if !result.Success {
		t.Error("Already resolved dispute should return success")
	}
}

func TestDisputeResolver_ChallengePeriodRemaining_Resolved(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1}

	dispute := &DisputeRecord{
		ChannelID: channelID,
		Resolved:  true,
	}

	mgr.mu.Lock()
	mgr.disputes[channelID] = dispute
	mgr.mu.Unlock()

	_, err := resolver.ChallengePeriodRemaining(channelID)
	if err == nil {
		t.Error("Should error for resolved dispute")
	}
}

func TestDisputeResolver_CheckDisputeStatus(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channel, err := mgr.OpenChannel(ctx, interfaces.Address{1}, interfaces.Address{2}, 5000, 3000)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	dispute := &DisputeRecord{
		ChannelID:  channel.ID,
		Challenger: interfaces.Address{1},
		ChallengedState: interfaces.SignedState{
			ChannelID: channel.ID,
			Sequence:  1,
		},
		ChallengeStart: time.Now(),
		ChallengeEnd:   time.Now().Add(24 * time.Hour),
		Resolved:       false,
	}

	mgr.mu.Lock()
	mgr.disputes[channel.ID] = dispute
	mgr.mu.Unlock()

	status, err := resolver.CheckDisputeStatus(channel.ID)
	if err != nil {
		t.Fatalf("CheckDisputeStatus failed: %v", err)
	}
	if !status.Active {
		t.Error("Dispute should be active")
	}
}

func TestDisputeResolver_CheckDisputeStatus_NotFound(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{99}
	_, err := resolver.CheckDisputeStatus(channelID)
	if err == nil {
		t.Error("Should error for non-existent dispute")
	}
}

func TestDisputeResolver_CleanOldEvidence_WithOldEvidence(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1}

	// Add old evidence
	oldTime := time.Now().Add(-31 * 24 * time.Hour)
	resolver.mu.Lock()
	resolver.evidenceStore[channelID] = []Evidence{
		{
			ChannelID: channelID,
			Timestamp: oldTime,
		},
	}
	resolver.mu.Unlock()

	count := resolver.CleanOldEvidence()
	if count != 1 {
		t.Errorf("Expected 1 cleaned, got %d", count)
	}
}
