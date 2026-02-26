package channel

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// MockMultiSigLocker is a mock implementation of MultiSigLocker for testing
type MockMultiSigLocker struct{}

func (m *MockMultiSigLocker) CreateMultiSigOutput(partyA, partyB interfaces.Address, amount uint64) (*interfaces.UTXO, error) {
	return &interfaces.UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   amount,
		Address: partyA,
	}, nil
}

func (m *MockMultiSigLocker) SpendMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte, outputs []interfaces.TXOutput) error {
	return nil
}

func (m *MockMultiSigLocker) VerifyMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte) bool {
	return true
}

// MockSigner is a mock implementation of Signer for testing
type MockSigner struct {
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

func NewMockSigner() *MockSigner {
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	return &MockSigner{
		privKey: privKey,
		pubKey:  pubKey,
	}
}

func (m *MockSigner) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(m.privKey, data), nil
}

func (m *MockSigner) PublicKey() []byte {
	return m.pubKey
}

func (m *MockSigner) Algorithm() string {
	return "ed25519"
}

func (m *MockSigner) Destroy() {
	// No-op for mock
}

// ============================================================================
// Manager Tests
// ============================================================================

func TestNewManager(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager == nil {
		t.Fatal("Manager should not be nil")
	}

	if manager.challengePeriod != 24*time.Hour {
		t.Errorf("Challenge period = %v, expected 24h", manager.challengePeriod)
	}
}

func TestNewManager_InvalidConfig(t *testing.T) {
	// Nil config
	_, err := NewManager(nil)
	if err == nil {
		t.Error("Should fail with nil config")
	}

	// Missing multi-sig locker
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
	}
	_, err = NewManager(cfg)
	if err == nil {
		t.Error("Should fail without multi-sig locker")
	}
}

func TestManager_OpenChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}
	depositA := uint64(5000)
	depositB := uint64(3000)

	ctx := context.Background()
	channel, err := manager.OpenChannel(ctx, partyA, partyB, depositA, depositB)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	if channel == nil {
		t.Fatal("Channel should not be nil")
	}

	if channel.BalanceA != depositA {
		t.Errorf("BalanceA = %d, expected %d", channel.BalanceA, depositA)
	}

	if channel.BalanceB != depositB {
		t.Errorf("BalanceB = %d, expected %d", channel.BalanceB, depositB)
	}

	if channel.Sequence != 0 {
		t.Errorf("Sequence = %d, expected 0", channel.Sequence)
	}
}

func TestManager_OpenChannel_InsufficientDeposit(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	_, err := manager.OpenChannel(ctx, partyA, partyB, 500, 3000)
	if err == nil {
		t.Error("Should fail with insufficient deposit")
	}
}

func TestManager_OpenChannel_ExceedsMaxValue(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 10000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	_, err := manager.OpenChannel(ctx, partyA, partyB, 6000, 5000)
	if err == nil {
		t.Error("Should fail when exceeding max channel value")
	}
}

func TestManager_GetChannel(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Get channel by ID
	got, err := manager.GetChannel(channel.ID)
	if err != nil {
		t.Fatalf("GetChannel failed: %v", err)
	}

	if got.ID != channel.ID {
		t.Error("Channel ID mismatch")
	}
}

func TestManager_GetChannel_NotFound(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	_, err := manager.GetChannel([32]byte{99})
	if err == nil {
		t.Error("Should fail with non-existent channel")
	}
}

func TestManager_ListChannels(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)
	manager.OpenChannel(ctx, partyB, partyA, 3000, 5000)

	channels := manager.ListChannels()
	if len(channels) != 2 {
		t.Errorf("Channel count = %d, expected 2", len(channels))
	}
}

func TestManager_Transfer(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Transfer from A to B
	state, err := manager.Transfer(channel.ID, 1000, true)
	if err != nil {
		t.Fatalf("Transfer failed: %v", err)
	}

	if state.BalanceA != 4000 {
		t.Errorf("BalanceA = %d, expected 4000", state.BalanceA)
	}

	if state.BalanceB != 4000 {
		t.Errorf("BalanceB = %d, expected 4000", state.BalanceB)
	}

	if state.Sequence != 1 {
		t.Errorf("Sequence = %d, expected 1", state.Sequence)
	}
}

func TestManager_Transfer_InsufficientBalance(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	partyA := interfaces.Address{1, 2, 3}
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Try to transfer more than balance
	_, err := manager.Transfer(channel.ID, 6000, true)
	if err == nil {
		t.Error("Should fail with insufficient balance")
	}
}

func TestManager_ForceClose(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	// Generate keys for both parties
	pubKeyA, privKeyA, _ := ed25519.GenerateKey(nil)
	pubKeyB, privKeyB, _ := ed25519.GenerateKey(nil)

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubKeyA[:32])
	copy(partyB[:], pubKeyB[:32])

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Create a properly signed state
	state := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}

	// Serialize and sign the state
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(privKeyA, stateData)
	state.SigB = ed25519.Sign(privKeyB, stateData)

	err := manager.ForceClose(channel.ID, state)
	if err != nil {
		t.Fatalf("ForceClose failed: %v", err)
	}

	status, _ := manager.GetChannelStatus(channel.ID)
	if status != StateClosing {
		t.Errorf("Status = %d, expected StateClosing", status)
	}
}

func TestManager_FinalizeClose(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond, // Very short for testing
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	// Generate keys for both parties
	pubKeyA, privKeyA, _ := ed25519.GenerateKey(nil)
	pubKeyB, privKeyB, _ := ed25519.GenerateKey(nil)

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubKeyA[:32])
	copy(partyB[:], pubKeyB[:32])

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	// Force close first with proper signatures
	state := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(privKeyA, stateData)
	state.SigB = ed25519.Sign(privKeyB, stateData)
	manager.ForceClose(channel.ID, state)

	// Wait for challenge period
	time.Sleep(2 * time.Millisecond)

	// Finalize close
	err := manager.FinalizeClose(channel.ID)
	if err != nil {
		t.Fatalf("FinalizeClose failed: %v", err)
	}

	status, _ := manager.GetChannelStatus(channel.ID)
	if status != StateClosed {
		t.Errorf("Status = %d, expected StateClosed", status)
	}
}

// Helper to create a manager with ed25519 key parties
type testParties struct {
	pubKeyA  ed25519.PublicKey
	privKeyA ed25519.PrivateKey
	pubKeyB  ed25519.PublicKey
	privKeyB ed25519.PrivateKey
	partyA   interfaces.Address
	partyB   interfaces.Address
}

func newTestParties() *testParties {
	pubKeyA, privKeyA, _ := ed25519.GenerateKey(nil)
	pubKeyB, privKeyB, _ := ed25519.GenerateKey(nil)

	var partyA, partyB interfaces.Address
	copy(partyA[:], pubKeyA[:32])
	copy(partyB[:], pubKeyB[:32])

	return &testParties{
		pubKeyA: pubKeyA, privKeyA: privKeyA,
		pubKeyB: pubKeyB, privKeyB: privKeyB,
		partyA: partyA, partyB: partyB,
	}
}

func (tp *testParties) signState(state *interfaces.SignedState) {
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	state.SigB = ed25519.Sign(tp.privKeyB, stateData)
}

func TestManager_Dispute(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	tp := newTestParties()

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create evidence (newer state) with real signatures
	evidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&evidence)

	err := manager.Dispute(ctx, channel, evidence)
	if err != nil {
		t.Fatalf("Dispute failed: %v", err)
	}

	status, _ := manager.GetChannelStatus(channel.ID)
	if status != StateInDispute {
		t.Errorf("Status = %d, expected StateInDispute", status)
	}

	// Get dispute record
	dispute, err := manager.GetDispute(channel.ID)
	if err != nil {
		t.Fatalf("GetDispute failed: %v", err)
	}

	if dispute.ChannelID != channel.ID {
		t.Error("Dispute channel ID mismatch")
	}

	if dispute.Resolved {
		t.Error("Dispute should not be resolved initially")
	}
}

func TestManager_ResolveDispute(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond, // Very short for testing
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	tp := newTestParties()

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create dispute with real signatures
	evidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&evidence)
	manager.Dispute(ctx, channel, evidence)

	// Wait for challenge period
	time.Sleep(2 * time.Millisecond)

	// Resolve dispute in favor of party A
	err := manager.ResolveDispute(channel.ID, tp.partyA)
	if err != nil {
		t.Fatalf("ResolveDispute failed: %v", err)
	}

	// Check dispute is resolved
	dispute, _ := manager.GetDispute(channel.ID)
	if !dispute.Resolved {
		t.Error("Dispute should be resolved")
	}

	if dispute.Winner != tp.partyA {
		t.Error("Party A should be the winner")
	}

	// Check channel is closed
	status, _ := manager.GetChannelStatus(channel.ID)
	if status != StateClosed {
		t.Errorf("Status = %d, expected StateClosed", status)
	}
}

func TestManager_CreateSignedState(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	// Set signer
	signer := NewMockSigner()
	manager.SetSigner(signer)

	// Use signer's public key as party A address
	var partyA interfaces.Address
	copy(partyA[:], signer.PublicKey()[:32])
	partyB := interfaces.Address{4, 5, 6}

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, partyA, partyB, 5000, 3000)

	state, err := manager.CreateSignedState(channel.ID)
	if err != nil {
		t.Fatalf("CreateSignedState failed: %v", err)
	}

	if state.ChannelID != channel.ID {
		t.Error("Channel ID mismatch")
	}

	if len(state.SigA) == 0 {
		t.Error("State should have SigA since signer matches partyA")
	}
}

func TestManager_AddCounterpartySignature(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)
	tp := newTestParties()

	ctx := context.Background()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create a state and sign it as party A
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  0,
		BalanceA:  5000,
		BalanceB:  3000,
		Timestamp: time.Now(),
	}

	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)

	// Add party B's signature
	counterpartySig := ed25519.Sign(tp.privKeyB, stateData)
	err := manager.AddCounterpartySignature(state, counterpartySig)
	if err != nil {
		t.Fatalf("AddCounterpartySignature failed: %v", err)
	}

	// Check both signatures are present
	if len(state.SigA) == 0 || len(state.SigB) == 0 {
		t.Error("Both signatures should be present")
	}
}
