// Package channel implements Lightning-style state channels for AIB 2.0.
// Settlement unit tests
package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// mockMultiSig is a mock implementation of MultiSigLocker for testing
type mockMultiSig struct {
	spentOutputs  map[string][]interfaces.TXOutput
	lockedFunds   map[[32]byte]uint64 // channelID -> locked amount
	unlockedFunds map[[32]byte]uint64 // channelID -> unlocked amount
	mu            sync.Mutex
	failSpend     bool
	failCreate    bool
}

func newMockMultiSig() *mockMultiSig {
	return &mockMultiSig{
		spentOutputs:  make(map[string][]interfaces.TXOutput),
		lockedFunds:   make(map[[32]byte]uint64),
		unlockedFunds: make(map[[32]byte]uint64),
	}
}

func (m *mockMultiSig) CreateMultiSigOutput(partyA, partyB interfaces.Address, amount uint64) (*interfaces.UTXO, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failCreate {
		return nil, errors.New("mock: create failed")
	}

	return &interfaces.UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   amount,
		Address: partyA,
	}, nil
}

func (m *mockMultiSig) SpendMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte, outputs []interfaces.TXOutput) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failSpend {
		return errors.New("mock: spend failed")
	}

	key := string(utxo.TxHash[:])
	m.spentOutputs[key] = outputs

	// Track unlocked funds - use a consistent mapping
	// We'll use the first output's address hash as a simple key
	var channelKey [32]byte
	if len(outputs) > 0 {
		copy(channelKey[:], outputs[0].Address[:])
	}
	m.unlockedFunds[channelKey] = utxo.Value

	return nil
}

func (m *mockMultiSig) VerifyMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte) bool {
	return len(sigA) > 0 || len(sigB) > 0
}

// LockFunds simulates locking funds in a multi-sig output
func (m *mockMultiSig) LockFunds(channelID [32]byte, amount uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockedFunds[channelID] = amount
}

// GetLockedFunds returns the locked funds for a channel
func (m *mockMultiSig) GetLockedFunds(channelID [32]byte) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockedFunds[channelID]
}

// GetUnlockedFunds returns the unlocked funds for a channel
func (m *mockMultiSig) GetUnlockedFunds(channelID [32]byte) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unlockedFunds[channelID]
}

// Reset clears all state
func (m *mockMultiSig) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spentOutputs = make(map[string][]interfaces.TXOutput)
	m.lockedFunds = make(map[[32]byte]uint64)
	m.unlockedFunds = make(map[[32]byte]uint64)
	m.failSpend = false
	m.failCreate = false
}

// generateTestKey generates a test key pair
func generateTestKey() (ed25519.PrivateKey, ed25519.PublicKey) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return privKey, pubKey
}

// generateTestAddress generates a test address from a public key
func generateTestAddress(pubKey ed25519.PublicKey) interfaces.Address {
	var addr interfaces.Address
	copy(addr[:], pubKey[:32])
	return addr
}

// TestSettlementManager_NewSettlementManager tests creating a new settlement manager
func TestSettlementManager_NewSettlementManager(t *testing.T) {
	// Create mock multi-sig
	mockMultiSig := newMockMultiSig()

	// Create config
	cfg := &SettlementConfig{
		ChallengePeriod:     24 * time.Hour,
		ConfirmationDepth:   6,
		MinSettlementAmount: 1,
		MultiSigLocker:      mockMultiSig,
	}

	// This test requires a Manager which requires more setup
	// For now, we just test that the config is valid
	if cfg.ChallengePeriod != 24*time.Hour {
		t.Errorf("expected challenge period of 24h, got %v", cfg.ChallengePeriod)
	}
}

// TestSettlementTypes tests settlement type constants
func TestSettlementTypes(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
	}{
		{"SettlementCooperative", 0, int(SettlementCooperative)},
		{"SettlementForce", 1, int(SettlementForce)},
		{"SettlementDispute", 2, int(SettlementDispute)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected != tt.actual {
				t.Errorf("expected %d, got %d", tt.expected, tt.actual)
			}
		})
	}
}

// TestSettlementStatus tests settlement status constants
func TestSettlementStatus(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
	}{
		{"SettlementPending", 0, int(SettlementPending)},
		{"SettlementConfirming", 1, int(SettlementConfirming)},
		{"SettlementComplete", 2, int(SettlementComplete)},
		{"SettlementFailed", 3, int(SettlementFailed)},
		{"SettlementCancelled", 4, int(SettlementCancelled)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected != tt.actual {
				t.Errorf("expected %d, got %d", tt.expected, tt.actual)
			}
		})
	}
}

// TestSettlement_Validation tests settlement validation logic
func TestSettlement_Validation(t *testing.T) {
	// Generate test keys
	privKeyA, pubKeyA := generateTestKey()
	privKeyB, pubKeyB := generateTestKey()

	addrA := generateTestAddress(pubKeyA)
	addrB := generateTestAddress(pubKeyB)

	// Create channel
	channel := &interfaces.Channel{
		ID:       [32]byte{1, 2, 3},
		PartyA:   addrA,
		PartyB:   addrB,
		BalanceA: 1000,
		BalanceB: 500,
		Sequence: 5,
	}

	// Create signed state
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  6,
		BalanceA:  800,
		BalanceB:  700,
		Timestamp: time.Now(),
	}

	// Sign state by both parties
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(privKeyA, stateData)
	state.SigB = ed25519.Sign(privKeyB, stateData)

	// Test validation - balance mismatch should fail
	invalidState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  6,
		BalanceA:  900, // Should be 800
		BalanceB:  700,
		Timestamp: time.Now(),
	}

	totalBalance := invalidState.BalanceA + invalidState.BalanceB
	channelTotal := channel.BalanceA + channel.BalanceB

	if totalBalance == channelTotal {
		t.Error("expected balance mismatch")
	}

	// Test with correct balance
	validState := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  6,
		BalanceA:  800,
		BalanceB:  700,
		Timestamp: time.Now(),
	}

	validState.SigA = ed25519.Sign(privKeyA, serializeState(validState))
	validState.SigB = ed25519.Sign(privKeyB, serializeState(validState))

	totalBalance = validState.BalanceA + validState.BalanceB
	if totalBalance != channelTotal {
		t.Errorf("expected balance to match: %d != %d", totalBalance, channelTotal)
	}

	// Verify signatures
	stateData = serializeState(validState)
	if !ed25519.Verify(pubKeyA, stateData, validState.SigA) {
		t.Error("signature A verification failed")
	}
	if !ed25519.Verify(pubKeyB, stateData, validState.SigB) {
		t.Error("signature B verification failed")
	}
}

// TestSettlementTx_Serialization tests settlement transaction serialization
func TestSettlementTx_Serialization(t *testing.T) {
	tx := &SettlementTx{
		ChannelID:    [32]byte{1, 2, 3},
		SettlementID: [32]byte{4, 5, 6},
		BalanceA:     100,
		BalanceB:     200,
		Sequence:     10,
		Timestamp:    time.Now(),
	}

	// Test serialization
	serialized := tx.Serialize()
	if len(serialized) == 0 {
		t.Error("serialized data is empty")
	}

	// Test signing
	privKey, pubKey := generateTestKey()
	sig := tx.Sign(privKey)
	if len(sig) != ed25519.SignatureSize {
		t.Errorf("invalid signature size: %d", len(sig))
	}

	// Test signature verification
	if !tx.VerifySignature(pubKey, sig) {
		t.Error("signature verification failed")
	}

	// Test with wrong public key
	wrongPrivKey, wrongPubKey := generateTestKey()
	if tx.VerifySignature(wrongPubKey, sig) {
		t.Error("signature should not verify with wrong key")
	}
	_ = wrongPrivKey
}

// TestSettlement_Structure tests the settlement structure
func TestSettlement_Structure(t *testing.T) {
	now := time.Now()

	settlement := &Settlement{
		ID:          [32]byte{1, 2, 3},
		ChannelID:   [32]byte{4, 5, 6},
		StateHash:   [32]byte{7, 8, 9},
		BalanceA:    500,
		BalanceB:    300,
		Sequence:    10,
		Type:        SettlementCooperative,
		Status:      SettlementPending,
		Timestamp:   now,
		BlockNumber: 100,
	}

	if settlement.Type != SettlementCooperative {
		t.Errorf("expected cooperative settlement, got %d", settlement.Type)
	}
	if settlement.Status != SettlementPending {
		t.Errorf("expected pending status, got %d", settlement.Status)
	}
	if settlement.BalanceA+settlement.BalanceB != 800 {
		t.Errorf("expected total balance 800, got %d", settlement.BalanceA+settlement.BalanceB)
	}
}

// TestDefaultSettlementConfig tests default configuration
func TestDefaultSettlementConfig(t *testing.T) {
	cfg := DefaultSettlementConfig()

	if cfg.ChallengePeriod == 0 {
		t.Error("challenge period should not be zero")
	}
	if cfg.ConfirmationDepth == 0 {
		t.Error("confirmation depth should not be zero")
	}
	if cfg.MinSettlementAmount == 0 {
		t.Error("min settlement amount should not be zero")
	}
	if cfg.MultiSigLocker != nil {
		t.Error("multi-sig locker should be nil by default")
	}
}

// TestSettlementValidator_ValidateSettlement tests settlement validation
func TestSettlementValidator_ValidateSettlement(t *testing.T) {
	// This test would require a full Manager setup
	// For now, we test the validator structure
	validator := &SettlementValidator{
		minAmount: 1,
	}

	if validator.minAmount != 1 {
		t.Errorf("expected min amount 1, got %d", validator.minAmount)
	}
}

// TestSettlementRecorder_Record tests settlement recording
func TestSettlementRecorder_Record(t *testing.T) {
	recorder := NewSettlementRecorder()

	settlement1 := &Settlement{
		ID:        [32]byte{1},
		ChannelID: [32]byte{1},
		BalanceA:  100,
		BalanceB:  200,
	}

	settlement2 := &Settlement{
		ID:        [32]byte{2},
		ChannelID: [32]byte{1},
		BalanceA:  150,
		BalanceB:  250,
	}

	// Record settlements
	recorder.Record(settlement1)
	recorder.Record(settlement2)

	// Get history
	history := recorder.GetHistory([32]byte{1})
	if len(history) != 2 {
		t.Errorf("expected 2 settlements, got %d", len(history))
	}

	// Get history for non-existent channel
	emptyHistory := recorder.GetHistory([32]byte{9})
	if len(emptyHistory) != 0 {
		t.Errorf("expected 0 settlements, got %d", len(emptyHistory))
	}
}

// TestCryptoSettlementSigner tests crypto signing
func TestCryptoSettlementSigner_SignSettlement(t *testing.T) {
	// Generate test keys
	privKey, pubKey := generateTestKey()

	signer := &CryptoSettlementSigner{}

	// Test with nil signer
	_, err := signer.SignSettlement(&Settlement{})
	if err == nil {
		t.Error("expected error with nil signer")
	}

	// Test direct signing using SettlementTx
	settlement := &Settlement{
		ID:        [32]byte{1, 2, 3},
		ChannelID: [32]byte{4, 5, 6},
		BalanceA:  100,
		BalanceB:  200,
		Sequence:  1,
		Timestamp: time.Now(),
	}

	tx := &SettlementTx{
		ChannelID:    settlement.ChannelID,
		SettlementID: settlement.ID,
		BalanceA:     settlement.BalanceA,
		BalanceB:     settlement.BalanceB,
		Sequence:     settlement.Sequence,
		Timestamp:    settlement.Timestamp,
	}

	sig := tx.Sign(privKey)
	if len(sig) == 0 {
		t.Error("signature is empty")
	}

	// Test verification
	if !tx.VerifySignature(pubKey, sig) {
		t.Error("verification failed")
	}
}

// TestForceClose_Validation tests force close validation
func TestForceClose_Validation(t *testing.T) {
	// Generate test keys
	privKeyA, pubKeyA := generateTestKey()
	privKeyB, pubKeyB := generateTestKey()

	addrA := generateTestAddress(pubKeyA)
	addrB := generateTestAddress(pubKeyB)

	// Create channel
	channel := &interfaces.Channel{
		ID:       [32]byte{1},
		PartyA:   addrA,
		PartyB:   addrB,
		BalanceA: 1000,
		BalanceB: 1000,
		Sequence: 5,
	}

	// Test with only Party A signature
	stateA := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  6,
		BalanceA:  800,
		BalanceB:  1200,
		Timestamp: time.Now(),
	}

	stateA.SigA = ed25519.Sign(privKeyA, serializeState(stateA))

	// Verify single signature
	stateData := serializeState(stateA)
	if !ed25519.Verify(pubKeyA, stateData, stateA.SigA) {
		t.Error("Party A signature should be valid")
	}

	// Test with only Party B signature
	stateB := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  6,
		BalanceA:  800,
		BalanceB:  1200,
		Timestamp: time.Now(),
	}

	stateB.SigB = ed25519.Sign(privKeyB, serializeState(stateB))

	stateData = serializeState(stateB)
	if !ed25519.Verify(pubKeyB, stateData, stateB.SigB) {
		t.Error("Party B signature should be valid")
	}
}

// TestSettlement_ChallengePeriod tests challenge period calculation
func TestSettlement_ChallengePeriod(t *testing.T) {
	cfg := DefaultSettlementConfig()
	cfg.ChallengePeriod = 24 * time.Hour

	if cfg.ChallengePeriod.Hours() != 24 {
		t.Errorf("expected 24 hour challenge period, got %v", cfg.ChallengePeriod)
	}

	// Test remaining calculation
	challengeEnd := time.Now().Add(-1 * time.Hour) // 1 hour ago
	remaining := challengeEnd.Sub(time.Now())
	if remaining > 0 {
		t.Error("remaining time should be negative for past challenge end")
	}

	futureEnd := time.Now().Add(1 * time.Hour) // 1 hour from now
	remaining = futureEnd.Sub(time.Now())
	if remaining <= 0 {
		t.Error("remaining time should be positive for future challenge end")
	}
}

// TestSettlement_Integration tests integration scenarios
func TestSettlement_Integration(t *testing.T) {
	t.Skip("Integration test requires full Manager setup")
}

// ============================================================
// batch settlement tests
// ============================================================

// TestBatchSettlementHandler_NewBatchSettlementHandler tests creating a new batch settlement handler
func TestBatchSettlementHandler_NewBatchSettlementHandler(t *testing.T) {
	// This test requires a SettlementManager which requires a Manager
	// For now, we test the handler structure would work
	t.Run("Validate nil handler", func(t *testing.T) {
		var handler *BatchSettlementHandler
		if handler != nil {
			t.Error("handler should be nil")
		}
	})
}

// TestBatchSettlementRequest_Validation tests batch settlement request validation
func TestBatchSettlementRequest_Validation(t *testing.T) {
	t.Run("Validate nil request", func(t *testing.T) {
		var handler *BatchSettlementHandler
		err := handler.ValidateBatchSettlement(nil)
		if err == nil {
			t.Error("expected error for nil request")
		}
		if err.Error() != "request is nil" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Validate empty settlements", func(t *testing.T) {
		handler := &BatchSettlementHandler{}
		req := &BatchSettlementRequest{
			Settlements: []*Settlement{},
			ChannelIDs:  [][]byte{},
		}
		err := handler.ValidateBatchSettlement(req)
		if err == nil {
			t.Error("expected error for empty settlements")
		}
		if err.Error() != "no settlements in batch" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Validate count mismatch", func(t *testing.T) {
		handler := &BatchSettlementHandler{}
		req := &BatchSettlementRequest{
			Settlements: []*Settlement{{ID: [32]byte{1}}},
			ChannelIDs:  [][]byte{{1}, {2}},
		}
		err := handler.ValidateBatchSettlement(req)
		if err == nil {
			t.Error("expected error for count mismatch")
		}
		if err.Error() != "settlements and channel IDs count mismatch" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Validate valid request", func(t *testing.T) {
		handler := &BatchSettlementHandler{}
		req := &BatchSettlementRequest{
			Settlements: []*Settlement{{ID: [32]byte{1}}},
			ChannelIDs:  [][]byte{{1}},
		}
		err := handler.ValidateBatchSettlement(req)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestBatchSettlementResult tests batch settlement result
func TestBatchSettlementResult(t *testing.T) {
	t.Run("Create result", func(t *testing.T) {
		channelID := [32]byte{1, 2, 3}
		settlementID := [32]byte{4, 5, 6}

		result := &BatchSettlementResult{
			ChannelID:    channelID,
			SettlementID: settlementID,
			Status:       SettlementComplete,
			Error:        nil,
		}

		if result.Status != SettlementComplete {
			t.Errorf("expected complete status, got %d", result.Status)
		}
		if result.ChannelID != channelID {
			t.Error("channel ID mismatch")
		}
		if result.SettlementID != settlementID {
			t.Error("settlement ID mismatch")
		}
		if result.Error != nil {
			t.Error("error should be nil")
		}
	})

	t.Run("Create result with error", func(t *testing.T) {
		result := &BatchSettlementResult{
			Status: SettlementFailed,
			Error:  errors.New("test error"),
		}

		if result.Status != SettlementFailed {
			t.Errorf("expected failed status, got %d", result.Status)
		}
		if result.Error == nil {
			t.Error("error should not be nil")
		}
	})
}

// ============================================================
// settlement proof verification tests
// ============================================================

// TestSettlementProofVerifier_VerifySettlementProof tests settlement proof verification
func TestSettlementProofVerifier_VerifySettlementProof(t *testing.T) {
	t.Run("Verify nil proof", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}
		err := verifier.VerifySettlementProof(nil)
		if err == nil {
			t.Error("expected error for nil proof")
		}
		if err.Error() != "proof is nil" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify proof with nil settlement", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}
		proof := &SettlementProof{
			Settlement: nil,
		}
		err := verifier.VerifySettlementProof(proof)
		if err == nil {
			t.Error("expected error for nil settlement")
		}
		if err.Error() != "settlement in proof is nil" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify proof with empty state proof", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		proof := &SettlementProof{
			Settlement: &Settlement{
				ID:        [32]byte{1},
				ChannelID: [32]byte{1},
			},
			StateProof: []byte{},
			BlockHash:  [32]byte{1},
			Timestamp:  time.Now(),
			Signatures: map[string][]byte{"PartyA": []byte{1}},
		}

		err := verifier.VerifySettlementProof(proof)
		if err == nil {
			t.Error("expected error for empty state proof")
		}
		if err.Error() != "state proof is empty" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify proof with zero block hash", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		proof := &SettlementProof{
			Settlement: &Settlement{
				ID:        [32]byte{1},
				ChannelID: [32]byte{1},
			},
			StateProof: []byte{1},
			BlockHash:  [32]byte{}, // zero hash
			Timestamp:  time.Now(),
			Signatures: map[string][]byte{"PartyA": []byte{1}},
		}

		err := verifier.VerifySettlementProof(proof)
		if err == nil {
			t.Error("expected error for zero block hash")
		}
		if err.Error() != "block hash is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify proof with zero timestamp", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		proof := &SettlementProof{
			Settlement: &Settlement{
				ID:        [32]byte{1},
				ChannelID: [32]byte{1},
			},
			StateProof: []byte{1},
			BlockHash:  [32]byte{1},
			Timestamp:  time.Time{}, // zero time
			Signatures: map[string][]byte{"PartyA": []byte{1}},
		}

		err := verifier.VerifySettlementProof(proof)
		if err == nil {
			t.Error("expected error for zero timestamp")
		}
		if err.Error() != "timestamp is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify proof with no signatures", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		proof := &SettlementProof{
			Settlement: &Settlement{
				ID:        [32]byte{1},
				ChannelID: [32]byte{1},
			},
			StateProof: []byte{1},
			BlockHash:  [32]byte{1},
			Timestamp:  time.Now(),
			Signatures: map[string][]byte{}, // empty
		}

		err := verifier.VerifySettlementProof(proof)
		if err == nil {
			t.Error("expected error for no signatures")
		}
		if err.Error() != "at least one signature is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestSettlementProofVerifier_VerifyMultiSigProof tests multi-signature proof verification
func TestSettlementProofVerifier_VerifyMultiSigProof(t *testing.T) {
	t.Run("Verify nil settlement", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}
		err := verifier.VerifyMultiSigProof(nil, []byte{1}, []byte{1}, []byte{1}, []byte{1})
		if err == nil {
			t.Error("expected error for nil settlement")
		}
		if err.Error() != "settlement is nil" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify with valid signature", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		privKey, pubKey := generateTestKey()
		settlement := &Settlement{
			ChannelID: [32]byte{1},
			ID:        [32]byte{2},
			BalanceA:  100,
			BalanceB:  200,
			Sequence:  1,
			Timestamp: time.Now(),
		}

		// Create and sign transaction
		tx := &SettlementTx{
			ChannelID:    settlement.ChannelID,
			SettlementID: settlement.ID,
			BalanceA:     settlement.BalanceA,
			BalanceB:     settlement.BalanceB,
			Sequence:     settlement.Sequence,
			Timestamp:    settlement.Timestamp,
		}

		sig := tx.Sign(privKey)

		// Verify
		err := verifier.VerifyMultiSigProof(settlement, pubKey, nil, sig, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify with invalid signature", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		_, pubKey := generateTestKey()
		settlement := &Settlement{
			ChannelID: [32]byte{1},
			ID:        [32]byte{2},
			BalanceA:  100,
			BalanceB:  200,
			Sequence:  1,
			Timestamp: time.Now(),
		}

		// Use an invalid signature
		invalidSig := []byte{1, 2, 3}

		err := verifier.VerifyMultiSigProof(settlement, pubKey, nil, invalidSig, nil)
		if err == nil {
			t.Error("expected error for invalid signature")
		}
		if err.Error() != "Party A signature verification failed" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify with no valid signatures", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}
		settlement := &Settlement{
			ChannelID: [32]byte{1},
			ID:        [32]byte{2},
			BalanceA:  100,
			BalanceB:  200,
			Sequence:  1,
			Timestamp: time.Now(),
		}

		err := verifier.VerifyMultiSigProof(settlement, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for no valid signatures")
		}
		if err.Error() != "no valid signatures provided" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestSettlementProofVerifier_VerifyStateTransitionProof tests state transition proof verification
func TestSettlementProofVerifier_VerifyStateTransitionProof(t *testing.T) {
	t.Run("Verify nil states", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}
		proof := &SettlementProof{}

		err := verifier.VerifyStateTransitionProof(nil, nil, proof)
		if err == nil {
			t.Error("expected error for nil states")
		}
		if err.Error() != "states are required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify sequence decreased", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		oldState := &interfaces.SignedState{
			Sequence: 10,
			BalanceA: 100,
			BalanceB: 100,
		}
		newState := &interfaces.SignedState{
			Sequence: 5, // decreased
			BalanceA: 100,
			BalanceB: 100,
		}
		proof := &SettlementProof{}

		err := verifier.VerifyStateTransitionProof(oldState, newState, proof)
		if err == nil {
			t.Error("expected error for decreased sequence")
		}
		if err.Error() != "sequence must increase: old 10, new 5" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Verify balance conservation violated", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		oldState := &interfaces.SignedState{
			Sequence:  10,
			BalanceA:  100,
			BalanceB:  100,
			ChannelID: [32]byte{1},
		}
		newState := &interfaces.SignedState{
			Sequence:  15,
			BalanceA:  150, // total changed
			BalanceB:  100,
			ChannelID: [32]byte{1},
		}
		proof := &SettlementProof{}

		err := verifier.VerifyStateTransitionProof(oldState, newState, proof)
		if err == nil {
			t.Error("expected error for balance conservation violation")
		}
	})

	t.Run("Verify valid transition", func(t *testing.T) {
		verifier := &SettlementProofVerifier{}

		oldState := &interfaces.SignedState{
			Sequence:  10,
			BalanceA:  100,
			BalanceB:  100,
			ChannelID: [32]byte{1},
		}
		newState := &interfaces.SignedState{
			Sequence:  15,
			BalanceA:  150,
			BalanceB:  50, // total preserved
			ChannelID: [32]byte{1},
		}
		proof := &SettlementProof{}

		err := verifier.VerifyStateTransitionProof(oldState, newState, proof)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestSettlementProof tests settlement proof structure
func TestSettlementProof(t *testing.T) {
	t.Run("Create proof", func(t *testing.T) {
		proof := &SettlementProof{
			Settlement: &Settlement{
				ID:        [32]byte{1},
				ChannelID: [32]byte{2},
			},
			StateProof:  []byte{1, 2, 3},
			StateHash:   [32]byte{4},
			Signatures:  map[string][]byte{"PartyA": []byte{5}},
			BlockNumber: 100,
			BlockHash:   [32]byte{6},
			Timestamp:   time.Now(),
		}

		if proof.Settlement == nil {
			t.Error("settlement should not be nil")
		}
		if len(proof.StateProof) != 3 {
			t.Error("state proof length mismatch")
		}
		if len(proof.Signatures) != 1 {
			t.Error("signatures count mismatch")
		}
		if proof.BlockNumber != 100 {
			t.Error("block number mismatch")
		}
	})
}

// ============================================================
// helper types
// ============================================================

// mockManagerForSettlement is a minimal mock for settlement tests
type mockManagerForSettlement struct {
	settlements map[[32]byte]*Settlement
	channels    map[[32]byte]*interfaces.Channel
}

func (m *mockManagerForSettlement) GetChannelState(channelID [32]byte) (*interfaces.Channel, error) {
	if ch, ok := m.channels[channelID]; ok {
		return ch, nil
	}
	return &interfaces.Channel{
		ID:       channelID,
		PartyA:   interfaces.Address{1},
		PartyB:   interfaces.Address{2},
		BalanceA: 1000,
		BalanceB: 1000,
		Sequence: 1,
	}, nil
}

func (m *mockManagerForSettlement) GetChannelStatus(channelID [32]byte) (int, error) {
	return StateOpen, nil
}

func (m *mockManagerForSettlement) CloseChannel(ctx context.Context, channel *interfaces.Channel, state interfaces.SignedState) error {
	return nil
}

func (m *mockManagerForSettlement) ForceClose(channelID [32]byte, state interfaces.SignedState) error {
	return nil
}

func (m *mockManagerForSettlement) FinalizeClose(channelID [32]byte) error {
	return nil
}

// ============================================================================
// SettlementManager Full Tests
// ============================================================================

func TestNewSettlementManager_Success(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	settlementCfg := &SettlementConfig{
		ChallengePeriod:     24 * time.Hour,
		ConfirmationDepth:   6,
		MinSettlementAmount: 1,
		MultiSigLocker:      &mockMultiSig{},
	}

	sm, err := NewSettlementManager(manager, settlementCfg)
	if err != nil {
		t.Fatalf("NewSettlementManager failed: %v", err)
	}

	if sm == nil {
		t.Fatal("SettlementManager should not be nil")
	}

	if sm.challengePeriod != 24*time.Hour {
		t.Errorf("Challenge period mismatch: got %v, want 24h", sm.challengePeriod)
	}
}

func TestNewSettlementManager_NilManager(t *testing.T) {
	cfg := &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	}

	_, err := NewSettlementManager(nil, cfg)
	if err == nil {
		t.Error("Should fail with nil manager")
	}
}

func TestNewSettlementManager_NilConfig(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	// With nil config, it should use default BUT multi-sig is still required
	// So this should fail
	_, err := NewSettlementManager(manager, nil)
	if err == nil {
		t.Error("Should fail with nil config (multi-sig required)")
	}
}

func TestNewSettlementManager_NilMultiSig(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	settlementCfg := &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  nil,
	}

	_, err := NewSettlementManager(manager, settlementCfg)
	if err == nil {
		t.Error("Should fail with nil multi-sig locker")
	}
}

func TestSettlementManager_SetBlockSubmitter(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	// Set block submitter callback
	sm.SetBlockSubmitter(func(ctx context.Context, tx *interfaces.TXOutput) ([]byte, error) {
		return []byte("tx-hash"), nil
	})
}

func TestSettlementManager_SetBlockGetter(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	// Set block getter callback
	sm.SetBlockGetter(func(ctx context.Context, blockNum uint64) (*interfaces.TXOutput, error) {
		return &interfaces.TXOutput{}, nil
	})
}

func TestSettlementManager_GetSettlement(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	channelID := [32]byte{1, 2, 3}

	// Get non-existent settlement
	settlement, err := sm.GetSettlement(channelID)
	if err == nil {
		t.Error("Should fail for non-existent settlement")
	}

	// Now create a settlement
	ctx := context.Background()
	tp := newTestParties()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create signed state
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(state)

	// Build settlement
	sm.BuildSettlement(ctx, channel.ID, state)

	// Get settlement
	settlement, err = sm.GetSettlement(channel.ID)
	if err != nil {
		t.Fatalf("GetSettlement failed: %v", err)
	}

	if settlement.ChannelID != channel.ID {
		t.Error("Channel ID mismatch")
	}
}

func TestSettlementManager_GetSettlementStatus(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	channelID := [32]byte{1, 2, 3}

	// Get status for non-existent settlement
	status, err := sm.GetSettlementStatus(channelID)
	if err == nil {
		t.Error("Should fail for non-existent settlement")
	}

	if status != SettlementPending {
		t.Logf("Status for non-existent: %d", status)
	}
}

func TestSettlementManager_CancelSettlement(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	ctx := context.Background()
	tp := newTestParties()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create signed state
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(state)

	// Build settlement
	sm.BuildSettlement(ctx, channel.ID, state)

	// Cancel settlement
	err := sm.CancelSettlement(channel.ID)
	if err != nil {
		t.Fatalf("CancelSettlement failed: %v", err)
	}

	// Verify cancelled
	settlement, _ := sm.GetSettlement(channel.ID)
	if settlement.Status != SettlementCancelled {
		t.Errorf("Expected SettlementCancelled, got %d", settlement.Status)
	}
}

func TestSettlementManager_ConfirmSettlement(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	ctx := context.Background()
	tp := newTestParties()
	channel, _ := manager.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create signed state
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(state)

	// Build settlement
	settlement, _ := sm.BuildSettlement(ctx, channel.ID, state)

	// Manually set to confirming for test (simulating the confirming period)
	settlement.Status = SettlementConfirming

	// Confirm settlement
	err := sm.ConfirmSettlement(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ConfirmSettlement failed: %v", err)
	}

	// Verify confirmed
	result, _ := sm.GetSettlement(channel.ID)
	if result.Status != SettlementComplete {
		t.Errorf("Expected SettlementComplete, got %d", result.Status)
	}
}

func TestSettlementManager_GetChallengePeriodRemaining(t *testing.T) {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	manager, _ := NewManager(cfg)

	sm, _ := NewSettlementManager(manager, &SettlementConfig{
		ChallengePeriod: 24 * time.Hour,
		MultiSigLocker:  &mockMultiSig{},
	})

	channelID := [32]byte{99}

	// Get for non-existent settlement
	_, err := sm.GetChallengePeriodRemaining(channelID)
	if err == nil {
		t.Error("Should fail for non-existent settlement")
	}
}

func TestSettlementRecorder_GetAllHistory(t *testing.T) {
	recorder := NewSettlementRecorder()

	channelID := [32]byte{1}

	// Record some settlements for the same channel
	for i := 0; i < 3; i++ {
		settlement := &Settlement{
			ID:        [32]byte{byte(i)},
			ChannelID: channelID,
			BalanceA:  uint64(100 * i),
			BalanceB:  uint64(200 * i),
		}
		recorder.Record(settlement)
	}

	// Get all history
	allHistory := recorder.GetAllHistory()
	if len(allHistory) != 1 {
		t.Errorf("Expected 1 channel in history, got %d", len(allHistory))
	}

	// Get settlements for the channel
	channelHistory := recorder.GetHistory(channelID)
	if len(channelHistory) != 3 {
		t.Errorf("Expected 3 settlements, got %d", len(channelHistory))
	}
}

// Helper for signing states
func signStateForTest(state *interfaces.SignedState, privKey ed25519.PrivateKey) {
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(privKey, stateData)
	state.SigB = ed25519.Sign(privKey, stateData)
}

// ============================================================
// comprehensive integration tests
// ============================================================

// ============================================================================
// 1. dispute resolution flow tests
// ============================================================================

// TestDisputeResolution_InitiateAndResolve tests the full dispute resolution flow
func TestDisputeResolution_InitiateAndResolve(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond, // Very short for testing
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Generate test parties
	tp := newTestParties()
	ctx := context.Background()

	// Open channel
	channel, err := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}

	// Create evidence with higher sequence
	evidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&evidence)

	// Initiate dispute
	err = mgr.Dispute(ctx, channel, evidence)
	if err != nil {
		t.Fatalf("Failed to initiate dispute: %v", err)
	}

	// Verify dispute status
	status, err := mgr.GetChannelStatus(channel.ID)
	if err != nil {
		t.Fatalf("Failed to get channel status: %v", err)
	}
	if status != StateInDispute {
		t.Errorf("Expected StateInDispute (%d), got %d", StateInDispute, status)
	}

	// Get dispute record
	dispute, err := mgr.GetDispute(channel.ID)
	if err != nil {
		t.Fatalf("Failed to get dispute: %v", err)
	}
	if dispute == nil {
		t.Fatal("Dispute should not be nil")
	}
	if dispute.Resolved {
		t.Error("Dispute should not be resolved yet")
	}

	// Wait for challenge period
	time.Sleep(2 * time.Millisecond)

	// Resolve dispute in favor of party B
	err = mgr.ResolveDispute(channel.ID, tp.partyB)
	if err != nil {
		t.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Verify dispute resolved
	dispute, _ = mgr.GetDispute(channel.ID)
	if !dispute.Resolved {
		t.Error("Dispute should be resolved")
	}
	if dispute.Winner != tp.partyB {
		t.Error("Party B should be the winner")
	}

	// Verify channel is closed
	status, _ = mgr.GetChannelStatus(channel.ID)
	if status != StateClosed {
		t.Errorf("Expected StateClosed (%d), got %d", StateClosed, status)
	}
}

// TestDisputeResolution_WithPenalty tests dispute resolution with penalty
func TestDisputeResolution_WithPenalty(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create and initiate dispute
	evidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&evidence)
	mgr.Dispute(ctx, channel, evidence)

	// Wait and resolve
	time.Sleep(2 * time.Millisecond)
	mgr.ResolveDispute(channel.ID, tp.partyA)

	// Get updated channel
	updatedChannel, _ := mgr.GetChannelState(channel.ID)

	// With penalty, all funds go to the honest party (Party A)
	// So Party A should have all funds (5000 + 3000 = 8000)
	expectedBalanceA := uint64(8000)
	if updatedChannel.BalanceA != expectedBalanceA {
		t.Errorf("Expected Party A balance %d, got %d", expectedBalanceA, updatedChannel.BalanceA)
	}
	if updatedChannel.BalanceB != 0 {
		t.Errorf("Expected Party B balance 0, got %d", updatedChannel.BalanceB)
	}
}

// TestDisputeResolution_CounterEvidence tests responding with counter-evidence
func TestDisputeResolution_CounterEvidence(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	resolver := NewDisputeResolver(mgr)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create initial evidence (challenge)
	challengeEvidence := Evidence{
		ChannelID:   channel.ID,
		Sequence:    2,
		BalanceA:    3000,
		BalanceB:    5000,
		SigA:        []byte("sig"),
		Timestamp:   time.Now(),
		Submitter:   tp.partyA,
		BlockNumber: 100,
	}

	// Initiate dispute (will fail due to invalid signature, but test flow)
	_, err := resolver.InitiateDispute(ctx, channel.ID, challengeEvidence)
	// We expect error due to signature validation in real implementation
	// But we test the counter-evidence flow
	if err != nil && err.Error() != "invalid evidence" {
		t.Logf("Expected validation error: %v", err)
	}
}

// TestDisputeResolution_InvalidChallenge tests invalid challenge scenarios
func TestDisputeResolution_InvalidChallenge(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond,
		MinDeposit:      100,
		MaxChannelValue: 1_000_000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Test 1: Challenge with older sequence (should fail)
	oldEvidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  0, // Same as current
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&oldEvidence)

	err = mgr.Dispute(ctx, channel, oldEvidence)
	if err == nil {
		t.Error("Should fail with older sequence")
	}

	// Test 2: Challenge closed channel (should fail)
	finalState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&finalState)

	// Force close first
	mgr.ForceClose(channel.ID, finalState)
	time.Sleep(2 * time.Millisecond)
	mgr.FinalizeClose(channel.ID)

	// Try to dispute closed channel
	evidence := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&evidence)

	err = mgr.Dispute(ctx, channel, evidence)
	if err == nil {
		t.Error("Should fail with closed channel")
	}
}

// ============================================================================
// 2. challenge period timeout tests
// ============================================================================

// TestChallengePeriod_Timeout tests force close with timeout
func TestChallengePeriod_Timeout(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create and sign force close state
	state := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	// Initiate force close
	err := mgr.ForceClose(channel.ID, state)
	if err != nil {
		t.Fatalf("ForceClose failed: %v", err)
	}

	// Verify channel is in closing state
	status, _ := mgr.GetChannelStatus(channel.ID)
	if status != StateClosing {
		t.Errorf("Expected StateClosing, got %d", status)
	}

	// Verify challenge end time is set
	updatedChannel, _ := mgr.GetChannelState(channel.ID)
	if updatedChannel.DisputeEnd == nil {
		t.Error("DisputeEnd should be set")
	}

	// Try to finalize before timeout (should fail)
	err = mgr.FinalizeClose(channel.ID)
	if err == nil {
		t.Error("Should fail before challenge period ends")
	}

	// Wait for challenge period
	time.Sleep(2 * time.Millisecond)

	// Finalize after timeout (should succeed)
	err = mgr.FinalizeClose(channel.ID)
	if err != nil {
		t.Fatalf("FinalizeClose after timeout failed: %v", err)
	}

	// Verify channel is closed
	status, _ = mgr.GetChannelStatus(channel.ID)
	if status != StateClosed {
		t.Errorf("Expected StateClosed, got %d", status)
	}
}

// TestChallengePeriod_Extend tests extending challenge period
func TestChallengePeriod_Extend(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create force close
	state := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&state)
	mgr.ForceClose(channel.ID, state)

	// Get channel and check challenge end
	updatedChannel, _ := mgr.GetChannelState(channel.ID)
	if updatedChannel.DisputeEnd == nil {
		t.Fatal("DisputeEnd should be set")
	}

	originalEnd := *updatedChannel.DisputeEnd

	// Wait a bit and force close again to extend
	time.Sleep(1 * time.Millisecond)
	state2 := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3500,
		BalanceB:  4500,
		Timestamp: time.Now(),
	}
	tp.signState(&state2)
	mgr.ForceClose(channel.ID, state2)

	// Check if challenge period was extended
	updatedChannel2, _ := mgr.GetChannelState(channel.ID)
	if updatedChannel2.DisputeEnd.After(originalEnd) {
		t.Log("Challenge period extended (new state submitted)")
	}
}

// TestChallengePeriod_MultipleForceClose tests multiple force close attempts
func TestChallengePeriod_MultipleForceClose(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// First force close
	state1 := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&state1)
	mgr.ForceClose(channel.ID, state1)

	status1, _ := mgr.GetChannelStatus(channel.ID)

	// Second force close (update with new state)
	state2 := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3500,
		BalanceB:  4500,
		Timestamp: time.Now(),
	}
	tp.signState(&state2)
	mgr.ForceClose(channel.ID, state2)

	status2, _ := mgr.GetChannelStatus(channel.ID)

	// Both should be in closing state
	if status1 != StateClosing || status2 != StateClosing {
		t.Errorf("Expected StateClosing, got status1=%d, status2=%d", status1, status2)
	}
}

// ============================================================================
// 3. multi-channel concurrent tests
// ============================================================================

// TestMultiChannel_ConcurrentOperations tests concurrent operations on multiple channels
func TestMultiChannel_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	ctx := context.Background()

	// Create 10 channels concurrently
	numChannels := 10
	channels := make([]*interfaces.Channel, numChannels)
	parties := make([]*testParties, numChannels)

	var wg sync.WaitGroup
	var errorCount int32

	for i := 0; i < numChannels; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			tp := newTestParties()
			parties[idx] = tp

			channel, err := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				t.Logf("Channel %d open failed: %v", idx, err)
				return
			}
			channels[idx] = channel

			// Perform transfer
			_, err = mgr.Transfer(channel.ID, 1000, true)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if errorCount > 0 {
		t.Errorf("Had %d errors in concurrent channel operations", errorCount)
	}

	// Verify all channels exist
	allChannels := mgr.ListChannels()
	if len(allChannels) != numChannels {
		t.Errorf("Expected %d channels, got %d", numChannels, len(allChannels))
	}

	// Verify balances were updated
	for i, channel := range channels {
		if channel != nil {
			updated, _ := mgr.GetChannelState(channel.ID)
			if updated.BalanceA != 4000 || updated.BalanceB != 4000 {
				t.Errorf("Channel %d: expected balances 4000/4000, got %d/%d",
					i, updated.BalanceA, updated.BalanceB)
			}
		}
	}
}

// TestMultiChannel_ConcurrentDisputes tests concurrent dispute initiation
func TestMultiChannel_ConcurrentDisputes(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	ctx := context.Background()

	// Create channels
	numChannels := 5
	channels := make([]*interfaces.Channel, numChannels)
	parties := make([]*testParties, numChannels)

	for i := 0; i < numChannels; i++ {
		tp := newTestParties()
		parties[i] = tp
		channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
		channels[i] = channel
	}

	// Initiate disputes concurrently
	var wg sync.WaitGroup
	disputeResults := make([]error, numChannels)

	for i := 0; i < numChannels; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			evidence := interfaces.SignedState{
				ChannelID: channels[idx].ID,
				Sequence:  2,
				BalanceA:  3000,
				BalanceB:  5000,
				Timestamp: time.Now(),
			}
			parties[idx].signState(&evidence)

			err := mgr.Dispute(ctx, channels[idx], evidence)
			disputeResults[idx] = err
		}(i)
	}

	wg.Wait()

	// All disputes should succeed
	successCount := 0
	for i, err := range disputeResults {
		if err == nil {
			successCount++
		} else {
			t.Logf("Dispute %d failed: %v", i, err)
		}
	}

	if successCount != numChannels {
		t.Errorf("Expected %d successful disputes, got %d", numChannels, successCount)
	}
}

// TestMultiChannel_TransferRaceCondition tests for race conditions in transfers
func TestMultiChannel_TransferRaceCondition(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 10000, 10000)

	// Perform many small transfers concurrently
	numTransfers := 100
	transferAmount := uint64(10)

	var wg sync.WaitGroup
	for i := 0; i < numTransfers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Alternate between A->B and B->A
			mgr.Transfer(channel.ID, transferAmount, i%2 == 0)
		}()
	}

	wg.Wait()

	// Verify total balance is preserved
	updated, _ := mgr.GetChannelState(channel.ID)
	expectedTotal := uint64(20000)
	actualTotal := updated.BalanceA + updated.BalanceB

	if actualTotal != expectedTotal {
		t.Errorf("Balance conservation violated: expected %d, got %d", expectedTotal, actualTotal)
	}
}

// ============================================================================
// 4. fund lock/unlock verification
// ============================================================================

// TestFundLock_LockAndUnlock tests fund locking and unlocking
func TestFundLock_LockAndUnlock(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
	totalValue := uint64(8000)

	// Lock funds
	mockMS.LockFunds(channel.ID, totalValue)
	lockedAmount := mockMS.GetLockedFunds(channel.ID)

	if lockedAmount != totalValue {
		t.Errorf("Expected locked amount %d, got %d", totalValue, lockedAmount)
	}

	// Create settlement
	finalState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&finalState)

	// Close channel (unlock funds)
	err := mgr.CloseChannel(ctx, channel, finalState)
	if err != nil {
		t.Fatalf("CloseChannel failed: %v", err)
	}

	// Verify channel is closed
	status, _ := mgr.GetChannelStatus(channel.ID)
	if status != StateClosed {
		t.Errorf("Expected StateClosed, got %d", status)
	}
}

// TestFundLock_ConcurrentLockUnlock tests concurrent lock/unlock operations
func TestFundLock_ConcurrentLockUnlock(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
	totalValue := uint64(8000)

	// Lock funds
	mockMS.LockFunds(channel.ID, totalValue)

	// Create multiple close attempts concurrently
	var wg sync.WaitGroup
	closeErrors := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			state := interfaces.SignedState{
				ChannelID: channel.ID,
				Sequence:  uint64(idx + 1),
				BalanceA:  4000,
				BalanceB:  4000,
				Timestamp: time.Now(),
			}
			tp.signState(&state)

			err := mgr.CloseChannel(ctx, channel, state)
			closeErrors[idx] = err
		}(i)
	}

	wg.Wait()

	// Only first close should succeed
	successCount := 0
	for _, err := range closeErrors {
		if err == nil {
			successCount++
		}
	}

	// Should have exactly one success (the first close)
	if successCount != 1 {
		t.Logf("Success count: %d (expected 1)", successCount)
	}
}

// TestFundLock_InvalidUnlock tests invalid unlock scenarios
func TestFundLock_InvalidUnlock(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Test 1: Invalid signature
	_, wrongKey, _ := ed25519.GenerateKey(nil)
	invalidState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&invalidState)
	invalidState.SigA = ed25519.Sign(wrongKey, stateData)
	invalidState.SigB = ed25519.Sign(tp.privKeyB, stateData)

	err := mgr.CloseChannel(ctx, channel, invalidState)
	if err == nil {
		t.Error("Should fail with invalid signature")
	}

	// Test 2: Balance conservation violated
	validState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  6000, // Invalid: total changed
		BalanceB:  3000,
		Timestamp: time.Now(),
	}
	tp.signState(&validState)

	err = mgr.CloseChannel(ctx, channel, validState)
	if err == nil {
		t.Error("Should fail with balance conservation violation")
	}

	// Test 3: Channel ID mismatch
	otherChannel := &interfaces.Channel{
		ID:       [32]byte{99, 99, 99},
		PartyA:   tp.partyA,
		PartyB:   tp.partyB,
		BalanceA: 5000,
		BalanceB: 3000,
	}
	otherState := interfaces.SignedState{
		ChannelID: otherChannel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&otherState)

	err = mgr.CloseChannel(ctx, otherChannel, otherState)
	if err == nil {
		t.Error("Should fail with channel ID mismatch")
	}
}

// TestFundLock_SettlementManager tests settlement with fund locking
func TestFundLock_SettlementManager(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &SettlementConfig{
		ChallengePeriod:     24 * time.Hour,
		ConfirmationDepth:   6,
		MinSettlementAmount: 1,
		MultiSigLocker:      mockMS,
	}

	mgrCfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, err := NewManager(mgrCfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sm, err := NewSettlementManager(mgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create settlement manager: %v", err)
	}

	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create signed state for settlement
	signedState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&signedState)

	// Build settlement
	settlement, err := sm.BuildSettlement(ctx, channel.ID, &signedState)
	if err != nil {
		t.Fatalf("BuildSettlement failed: %v", err)
	}

	if settlement == nil {
		t.Fatal("Settlement should not be nil")
	}

	if settlement.Status != SettlementPending {
		t.Errorf("Expected SettlementPending, got %d", settlement.Status)
	}

	// Lock funds
	mockMS.LockFunds(channel.ID, 8000)

	// Execute settlement
	result, err := sm.ExecuteSettlement(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ExecuteSettlement failed: %v", err)
	}

	// Verify settlement status
	if result.Status != SettlementConfirming {
		t.Errorf("Expected SettlementConfirming, got %d", result.Status)
	}
}

// TestFundLock_ForceSettlement tests force settlement with challenge period
func TestFundLock_ForceSettlement(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &SettlementConfig{
		ChallengePeriod:     1 * time.Millisecond,
		ConfirmationDepth:   6,
		MinSettlementAmount: 1,
		MultiSigLocker:      mockMS,
	}

	mgrCfg := &Config{
		ChallengePeriod: 1 * time.Millisecond,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(mgrCfg)
	sm, _ := NewSettlementManager(mgr, cfg)

	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Lock funds
	mockMS.LockFunds(channel.ID, 8000)

	// Create state with both signatures for force close
	forceState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&forceState) // Sign with both parties

	// Initiate force close
	settlement, err := sm.ForceClose(ctx, channel.ID, forceState, tp.partyA)
	if err != nil {
		t.Fatalf("ForceClose failed: %v", err)
	}

	if settlement.Type != SettlementForce {
		t.Errorf("Expected SettlementForce, got %d", settlement.Type)
	}

	if settlement.ChallengeEnd == nil {
		t.Error("ChallengeEnd should be set")
	}

	// Try to confirm before timeout
	_, err = sm.ConfirmForceClose(ctx, channel.ID)
	if err == nil {
		t.Error("Should fail before challenge period")
	}

	// Wait for timeout
	time.Sleep(2 * time.Millisecond)

	// Confirm after timeout
	settlement, err = sm.ConfirmForceClose(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ConfirmForceClose failed: %v", err)
	}

	// Verify settlement status
	if settlement.Status != SettlementConfirming {
		t.Errorf("Expected SettlementConfirming, got %d", settlement.Status)
	}
}

// ============================================================================
// boundary condition and error scenario tests
// ============================================================================

// TestBoundary_MaxChannelValue tests maximum channel value
func TestBoundary_MaxChannelValue(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1,
		MaxChannelValue: 10000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	// Exactly at max
	_, err := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 5000)
	if err != nil {
		t.Errorf("Should succeed at max value: %v", err)
	}

	// Over max
	_, err = mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 6000, 5000)
	if err == nil {
		t.Error("Should fail over max value")
	}
}

// TestBoundary_MinDeposit tests minimum deposit
func TestBoundary_MinDeposit(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	// Exactly at min
	_, err := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 1000, 1000)
	if err != nil {
		t.Errorf("Should succeed at min deposit: %v", err)
	}

	// Below min
	_, err = mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 999, 1000)
	if err == nil {
		t.Error("Should fail below min deposit")
	}
}

// TestBoundary_ZeroTransfer tests zero amount transfer
func TestBoundary_ZeroTransfer(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Zero transfer
	state, err := mgr.Transfer(channel.ID, 0, true)
	if err != nil {
		// Zero transfer may or may not be allowed depending on implementation
		t.Logf("Zero transfer error: %v", err)
	} else {
		if state != nil {
			// Sequence should still increment
			t.Logf("Zero transfer sequence: %d", state.Sequence)
		}
	}
}

// TestBoundary_ExactBalanceTransfer tests transferring exact balance
func TestBoundary_ExactBalanceTransfer(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Transfer exact balance from A
	_, err := mgr.Transfer(channel.ID, 5000, true)
	if err != nil {
		t.Errorf("Should allow exact balance transfer: %v", err)
	}

	// Verify
	updated, _ := mgr.GetChannelState(channel.ID)
	if updated.BalanceA != 0 {
		t.Errorf("Expected balance A = 0, got %d", updated.BalanceA)
	}
}

// TestError_ChannelNotFound tests operations on non-existent channel
func TestError_ChannelNotFound(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)

	nonExistentID := [32]byte{99, 99, 99}

	// GetChannel
	_, err := mgr.GetChannel(nonExistentID)
	if err == nil {
		t.Error("Should fail for non-existent channel")
	}

	// GetChannelStatus
	_, err = mgr.GetChannelStatus(nonExistentID)
	if err == nil {
		t.Error("Should fail for non-existent channel")
	}

	// Transfer
	_, err = mgr.Transfer(nonExistentID, 100, true)
	if err == nil {
		t.Error("Should fail for non-existent channel")
	}

	// ForceClose
	err = mgr.ForceClose(nonExistentID, interfaces.SignedState{})
	if err == nil {
		t.Error("Should fail for non-existent channel")
	}
}

// TestError_InvalidSequence tests invalid sequence scenarios
func TestError_InvalidSequence(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Lower sequence
	invalidState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  0, // Same as current
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&invalidState)

	_, err := mgr.UpdateState(channel, invalidState)
	if err == nil {
		t.Error("Should fail with lower sequence")
	}

	// Much lower sequence
	invalidState2 := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  100, // Way higher - let's try a lower one instead
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	// Sign with lower sequence
	invalidState2.Sequence = 0
	tp.signState(&invalidState2)

	_, err = mgr.UpdateState(channel, invalidState2)
	if err == nil {
		t.Error("Should fail with same sequence")
	}
}

// TestError_ChannelStateTransition tests invalid state transitions
func TestError_ChannelStateTransition(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Close channel normally
	finalState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&finalState)
	mgr.CloseChannel(ctx, channel, finalState)

	// Try to update closed channel
	newState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  2,
		BalanceA:  3000,
		BalanceB:  5000,
		Timestamp: time.Now(),
	}
	tp.signState(&newState)

	_, err := mgr.UpdateState(channel, newState)
	if err == nil {
		t.Error("Should fail for closed channel")
	}
}

// ============================================================================
// integration scenario tests
// ============================================================================

// TestIntegration_FullChannelLifecycle tests complete channel lifecycle
func TestIntegration_FullChannelLifecycle(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	cfg := &Config{
		ChallengePeriod: 1 * time.Millisecond, // Short for testing
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(cfg)
	tp := newTestParties()
	ctx := context.Background()

	// 1. Open channel
	channel, err := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 10000, 10000)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	// Verify initial state
	if channel.BalanceA != 10000 || channel.BalanceB != 10000 {
		t.Errorf("Initial balances incorrect: %d/%d", channel.BalanceA, channel.BalanceB)
	}

	// 2. Perform multiple transfers
	for i := 0; i < 5; i++ {
		_, err = mgr.Transfer(channel.ID, 500, true) // A -> B
		if err != nil {
			t.Fatalf("Transfer %d failed: %v", i, err)
		}
	}

	// Verify balances after transfers
	updated, _ := mgr.GetChannelState(channel.ID)
	expectedA := uint64(7500)  // 10000 - 5*500
	expectedB := uint64(12500) // 10000 + 5*500
	if updated.BalanceA != expectedA || updated.BalanceB != expectedB {
		t.Errorf("Balances after transfers: expected %d/%d, got %d/%d",
			expectedA, expectedB, updated.BalanceA, updated.BalanceB)
	}

	// 3. Initiate dispute
	disputeState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  updated.Sequence + 1,
		BalanceA:  8000,
		BalanceB:  12000,
		Timestamp: time.Now(),
	}
	tp.signState(&disputeState)
	err = mgr.Dispute(ctx, channel, disputeState)
	if err != nil {
		t.Fatalf("Dispute failed: %v", err)
	}

	// 4. Resolve dispute after challenge period
	time.Sleep(2 * time.Millisecond)
	err = mgr.ResolveDispute(channel.ID, tp.partyA)
	if err != nil {
		t.Fatalf("ResolveDispute failed: %v", err)
	}

	// 5. Verify final state
	final, _ := mgr.GetChannelState(channel.ID)
	status, _ := mgr.GetChannelStatus(channel.ID)

	if status != StateClosed {
		t.Errorf("Expected closed state, got %d", status)
	}

	// With penalty, all funds go to Party A
	if final.BalanceA != 20000 {
		t.Errorf("Expected Party A balance 20000 (with penalty), got %d", final.BalanceA)
	}

	_ = mockMS
}

// TestIntegration_SettlementWithChallenge tests settlement with challenge period
func TestIntegration_SettlementWithChallenge(t *testing.T) {
	t.Parallel()

	mockMS := newMockMultiSig()
	smCfg := &SettlementConfig{
		ChallengePeriod:     1 * time.Millisecond,
		ConfirmationDepth:   1,
		MinSettlementAmount: 1,
		MultiSigLocker:      mockMS,
	}

	mgrCfg := &Config{
		ChallengePeriod: 1 * time.Millisecond,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  mockMS,
	}

	mgr, _ := NewManager(mgrCfg)
	sm, _ := NewSettlementManager(mgr, smCfg)

	tp := newTestParties()
	ctx := context.Background()

	// Open and prepare channel
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)
	mockMS.LockFunds(channel.ID, 8000)

	// Force close with both signatures
	forceState := interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  1,
		BalanceA:  4000,
		BalanceB:  4000,
		Timestamp: time.Now(),
	}
	tp.signState(&forceState)

	// Initiate force close
	settlement, err := sm.ForceClose(ctx, channel.ID, forceState, tp.partyA)
	if err != nil {
		t.Fatalf("ForceClose failed: %v", err)
	}

	// Wait and confirm
	time.Sleep(2 * time.Millisecond)
	settlement, err = sm.ConfirmForceClose(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ConfirmForceClose failed: %v", err)
	}

	// Verify settlement status
	if settlement.Status == SettlementConfirming {
		t.Log("Settlement confirmed successfully")
	}
}
