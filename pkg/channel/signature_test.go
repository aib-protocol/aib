package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

func newSignatureTestManager(t *testing.T) *Manager {
	t.Helper()

	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      1000,
		MaxChannelValue: 1000000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	return m
}

func createSignatureTestChannel(t *testing.T, m *Manager) (*interfaces.Channel, *testParties) {
	t.Helper()

	tp := newTestParties()
	ch, err := m.OpenChannel(context.Background(), tp.partyA, tp.partyB, 5000, 3000)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	return ch, tp
}

func TestSignature_Ed25519Verification_ValidSignatures(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	if err := m.verifySignatures(ch, &state); err != nil {
		t.Fatalf("valid signatures should pass verification: %v", err)
	}
}

func TestSignature_Ed25519Verification_InvalidSignatureLength(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	state.SigB = make([]byte, ed25519.SignatureSize-1)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("invalid signature length should be rejected")
	}
}

func TestSignature_Ed25519Verification_ZeroedSignatureRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	state.SigA = make([]byte, ed25519.SignatureSize)
	stateData := serializeState(&state)
	state.SigB = ed25519.Sign(tp.privKeyB, stateData)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("zeroed signature must be rejected")
	}
}

func TestSignature_MultiSigAggregation_BothSignaturesRequired(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := &interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4200,
		BalanceB:  3800,
		Timestamp: time.Now(),
	}
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)

	sigB := ed25519.Sign(tp.privKeyB, stateData)
	if err := m.AddCounterpartySignature(state, sigB); err != nil {
		t.Fatalf("AddCounterpartySignature should accept valid counterparty signature: %v", err)
	}

	if len(state.SigA) != ed25519.SignatureSize || len(state.SigB) != ed25519.SignatureSize {
		t.Fatal("both signatures must exist after aggregation")
	}

	if err := m.verifySignatures(ch, state); err != nil {
		t.Fatalf("aggregated signatures should verify: %v", err)
	}
}

func TestSignature_MultiSigAggregation_WrongCounterpartySignatureRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	_, wrongPriv, _ := ed25519.GenerateKey(rand.Reader)

	state := &interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4200,
		BalanceB:  3800,
		Timestamp: time.Now(),
	}
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	forgedSigB := ed25519.Sign(wrongPriv, stateData)

	if err := m.AddCounterpartySignature(state, forgedSigB); err == nil {
		t.Fatal("forged counterparty signature must be rejected")
	}
}

func TestSignature_Recovery_PublicKeyVerificationFromSignature(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  2,
		BalanceA:  4100,
		BalanceB:  3900,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	sigA := ed25519.Sign(tp.privKeyA, stateData)

	if !ed25519.Verify(tp.pubKeyA, stateData, sigA) {
		t.Fatal("signature must verify with original public key")
	}

	if ed25519.Verify(tp.pubKeyB, stateData, sigA) {
		t.Fatal("signature must not verify with non-matching public key")
	}
}

func TestSignature_Recovery_StateBindingPreventsReplay(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	original := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4300,
		BalanceB:  3700,
		Timestamp: time.Now(),
	}
	tp.signState(&original)

	tampered := original
	tampered.BalanceA = 4200
	tampered.BalanceB = 3800

	if err := m.verifySignatures(ch, &tampered); err == nil {
		t.Fatal("signature replay on modified state must fail")
	}
}

func TestSignature_ForgeryDefense_WrongPrivateKeyRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	_, wrongPriv, _ := ed25519.GenerateKey(rand.Reader)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(wrongPriv, stateData)
	state.SigB = ed25519.Sign(tp.privKeyB, stateData)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("signature created by wrong private key must be rejected")
	}
}

func TestSignature_ForgeryDefense_BitFlipTamperingRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	state.SigA[0] ^= 0x01

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("tampered signature must be rejected")
	}
}

func TestSignature_ForgeryDefense_CrossChannelReplayRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch1, tp := createSignatureTestChannel(t, m)
	ch2, _ := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch1.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	replayed := state
	replayed.ChannelID = ch2.ID

	if err := m.verifySignatures(ch2, &replayed); err == nil {
		t.Fatal("cross-channel signature replay must be rejected")
	}
}

func TestSignature_ForgeryDefense_SequenceNumberReplayRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	oldState := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  0,
		BalanceA:  5000,
		BalanceB:  3000,
		Timestamp: time.Now(),
	}
	tp.signState(&oldState)

	newState := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	newState.SigA = oldState.SigA
	newState.SigB = oldState.SigB

	if err := m.verifySignatures(ch, &newState); err == nil {
		t.Fatal("signature from old sequence must be rejected for new sequence")
	}
}

func TestSignature_MultiSigAggregation_SingleSignatureUpdate(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := &interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4200,
		BalanceB:  3800,
		Timestamp: time.Now(),
	}

	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)

	if err := m.AddCounterpartySignature(state, ed25519.Sign(tp.privKeyB, stateData)); err != nil {
		t.Fatalf("AddCounterpartySignature should accept valid counterparty signature: %v", err)
	}

	if len(state.SigA) == 0 || len(state.SigB) == 0 {
		t.Fatal("both signatures must be present")
	}
}

func TestSignature_MultiSigAggregation_BothSignaturesAlreadyPresent(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := &interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4200,
		BalanceB:  3800,
		Timestamp: time.Now(),
	}
	stateData := serializeState(state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	state.SigB = ed25519.Sign(tp.privKeyB, stateData)

	extraSig := ed25519.Sign(tp.privKeyA, stateData)
	if err := m.AddCounterpartySignature(state, extraSig); err == nil {
		t.Fatal("adding signature when both are present should fail")
	}
}

func TestSignature_Ed25519Verification_EmptySignatureRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigB = ed25519.Sign(tp.privKeyB, stateData)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("empty signature A should be rejected")
	}
}

func TestSignature_Recovery_SignatureDeterministic(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)

	sig1 := ed25519.Sign(tp.privKeyA, stateData)
	sig2 := ed25519.Sign(tp.privKeyA, stateData)

	if !ed25519.Verify(tp.pubKeyA, stateData, sig1) {
		t.Fatal("first signature must verify")
	}
	if !ed25519.Verify(tp.pubKeyA, stateData, sig2) {
		t.Fatal("second signature must verify")
	}
}

func TestSignature_ForgeryDefense_ModifiedChannelIDRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	state.ChannelID[0] ^= 0xFF

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("signature with modified channel ID must be rejected")
	}
}

func TestSignature_ForgeryDefense_ModifiedTimestampRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	tp.signState(&state)

	originalTimestamp := state.Timestamp
	state.Timestamp = state.Timestamp.Add(1 * time.Hour)

	_, err := m.UpdateState(ch, state)
	if err == nil {
		t.Fatal("update with modified timestamp after signing should fail")
	}

	state.Timestamp = originalTimestamp
}

func TestSignature_Ed25519Verification_LongMessage(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)

	longMessage := make([]byte, 10000)
	for i := range longMessage {
		longMessage[i] = byte(i % 256)
	}

	sig := ed25519.Sign(privKey, longMessage)
	if len(sig) != ed25519.SignatureSize {
		t.Fatalf("signature length must be %d, got %d", ed25519.SignatureSize, len(sig))
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	if !ed25519.Verify(pubKey, longMessage, sig) {
		t.Fatal("signature on long message must verify")
	}
}

func TestSignature_ForgeryDefense_SignatureLengthWrongPartyB(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	state.SigB = make([]byte, ed25519.SignatureSize-2)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("wrong signature length for B should be rejected")
	}
}

func TestSignature_ForgeryDefense_CrossPartySignatureRejected(t *testing.T) {
	m := newSignatureTestManager(t)
	ch, tp := createSignatureTestChannel(t, m)

	_, partyCPriv, _ := ed25519.GenerateKey(rand.Reader)

	state := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  4500,
		BalanceB:  3500,
		Timestamp: time.Now(),
	}
	stateData := serializeState(&state)
	state.SigA = ed25519.Sign(tp.privKeyA, stateData)
	state.SigB = ed25519.Sign(partyCPriv, stateData)

	if err := m.verifySignatures(ch, &state); err == nil {
		t.Fatal("signature from unrelated party must be rejected")
	}
}
