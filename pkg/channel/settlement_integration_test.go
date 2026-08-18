package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// settlementIntegrationParties encapsulates the keys and addresses of integration test parties
// Real signing flow: sign the actual state with ed25519
// No fake signature data is used
type settlementIntegrationParties struct {
	privKeyA ed25519.PrivateKey
	pubKeyA  ed25519.PublicKey
	privKeyB ed25519.PrivateKey
	pubKeyB  ed25519.PublicKey
	partyA   interfaces.Address
	partyB   interfaces.Address
}

func newSettlementIntegrationParties(t *testing.T) *settlementIntegrationParties {
	t.Helper()

	pubA, privA, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate PartyA key: %v", err)
	}
	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate PartyB key: %v", err)
	}

	var addrA interfaces.Address
	var addrB interfaces.Address
	copy(addrA[:], pubA[:32])
	copy(addrB[:], pubB[:32])

	return &settlementIntegrationParties{
		privKeyA: privA,
		pubKeyA:  pubA,
		privKeyB: privB,
		pubKeyB:  pubB,
		partyA:   addrA,
		partyB:   addrB,
	}
}

func (p *settlementIntegrationParties) signState(state *interfaces.SignedState) {
	data := serializeState(state)
	state.SigA = ed25519.Sign(p.privKeyA, data)
	state.SigB = ed25519.Sign(p.privKeyB, data)
}

func newSettlementIntegrationManagers(t *testing.T, challengePeriod time.Duration) (*Manager, *SettlementManager, *mockMultiSig) {
	t.Helper()

	multiSig := newMockMultiSig()

	mgr, err := NewManager(&Config{
		ChallengePeriod: challengePeriod,
		MinDeposit:      1,
		MaxChannelValue: 10_000_000,
		MultiSigLocker:  multiSig,
	})
	if err != nil {
		t.Fatalf("failed to create channel manager: %v", err)
	}

	sm, err := NewSettlementManager(mgr, &SettlementConfig{
		ChallengePeriod:     challengePeriod,
		ConfirmationDepth:   1,
		MinSettlementAmount: 1,
		MultiSigLocker:      multiSig,
	})
	if err != nil {
		t.Fatalf("failed to create settlement manager: %v", err)
	}

	return mgr, sm, multiSig
}

// 1. Channel open flow
func TestSettlementIntegration_ChannelOpenFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr, _, _ := newSettlementIntegrationManagers(t, 50*time.Millisecond)
	parties := newSettlementIntegrationParties(t)

	depositA := uint64(7000)
	depositB := uint64(3000)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, depositA, depositB)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	if ch.BalanceA != depositA || ch.BalanceB != depositB {
		t.Fatalf("incorrect initial channel balances: got A=%d B=%d", ch.BalanceA, ch.BalanceB)
	}

	status, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus failed: %v", err)
	}
	if status != StateOpen {
		t.Fatalf("expected channel state StateOpen(%d), got %d", StateOpen, status)
	}

	stored, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState failed: %v", err)
	}
	if stored.BalanceA+stored.BalanceB != depositA+depositB {
		t.Fatalf("channel total funds not conserved: got %d want %d", stored.BalanceA+stored.BalanceB, depositA+depositB)
	}
}

// 2. Normal settlement flow
func TestSettlementIntegration_CooperativeSettlementFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr, sm, _ := newSettlementIntegrationManagers(t, 100*time.Millisecond)
	parties := newSettlementIntegrationParties(t)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, 6000, 4000)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	// Real off-chain state evolution
	_, err = mgr.Transfer(ch.ID, 1000, true) // A -> B
	if err != nil {
		t.Fatalf("Transfer(A->B) failed: %v", err)
	}
	_, err = mgr.Transfer(ch.ID, 500, false) // B -> A
	if err != nil {
		t.Fatalf("Transfer(B->A) failed: %v", err)
	}

	latest, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState failed: %v", err)
	}

	finalState := &interfaces.SignedState{
		ChannelID: latest.ID,
		Sequence:  latest.Sequence + 1,
		BalanceA:  5600,
		BalanceB:  4400,
		Timestamp: time.Now(),
	}
	parties.signState(finalState)

	settlement, err := sm.BuildSettlement(ctx, ch.ID, finalState)
	if err != nil {
		t.Fatalf("BuildSettlement failed: %v", err)
	}
	if settlement.Status != SettlementPending {
		t.Fatalf("expected SettlementPending, got %d", settlement.Status)
	}

	settlement, err = sm.ExecuteSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ExecuteSettlement failed: %v", err)
	}
	if settlement.Status != SettlementConfirming {
		t.Fatalf("expected SettlementConfirming, got %d", settlement.Status)
	}

	err = sm.ConfirmSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ConfirmSettlement failed: %v", err)
	}

	status, err := sm.GetSettlementStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetSettlementStatus failed: %v", err)
	}
	if status != SettlementComplete {
		t.Fatalf("expected SettlementComplete, got %d", status)
	}

	channelStatus, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus failed: %v", err)
	}
	if channelStatus != StateClosed {
		t.Fatalf("expected channel closed StateClosed(%d), got %d", StateClosed, channelStatus)
	}
}

// 3. Dispute resolution flow
func TestSettlementIntegration_DisputeResolutionFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	challengePeriod := 20 * time.Millisecond
	mgr, sm, _ := newSettlementIntegrationManagers(t, challengePeriod)
	parties := newSettlementIntegrationParties(t)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, 5000, 5000)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	// First produce a real state update
	_, err = mgr.Transfer(ch.ID, 1200, true)
	if err != nil {
		t.Fatalf("Transfer failed: %v", err)
	}

	latest, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState failed: %v", err)
	}

	// Force settlement signed by both parties (simulating a cooperative force-close scenario)
	forceState := interfaces.SignedState{
		ChannelID: latest.ID,
		Sequence:  latest.Sequence,
		BalanceA:  latest.BalanceA,
		BalanceB:  latest.BalanceB,
		Timestamp: time.Now(),
	}
	parties.signState(&forceState)

	settlement, err := sm.ForceClose(ctx, ch.ID, forceState, parties.partyA)
	if err != nil {
		t.Fatalf("ForceClose failed: %v", err)
	}
	if settlement.Type != SettlementForce {
		t.Fatalf("expected SettlementForce, got %d", settlement.Type)
	}

	// Confirmation should fail during the challenge period
	_, err = sm.ConfirmForceClose(ctx, ch.ID)
	if err == nil {
		t.Fatalf("ConfirmForceClose should fail before the challenge period ends")
	}

	time.Sleep(challengePeriod + 5*time.Millisecond)

	settlement, err = sm.ConfirmForceClose(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ConfirmForceClose failed: %v", err)
	}
	if settlement.Status != SettlementConfirming {
		t.Fatalf("expected SettlementConfirming, got %d", settlement.Status)
	}

	channelStatus, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus failed: %v", err)
	}
	if channelStatus != StateClosed {
		t.Fatalf("expected channel closed StateClosed(%d), got %d", StateClosed, channelStatus)
	}
}

// 4. Fund unlock flow
func TestSettlementIntegration_FundUnlockFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr, sm, multiSig := newSettlementIntegrationManagers(t, 100*time.Millisecond)
	parties := newSettlementIntegrationParties(t)

	depositA := uint64(8000)
	depositB := uint64(2000)
	total := depositA + depositB

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, depositA, depositB)
	if err != nil {
		t.Fatalf("OpenChannel failed: %v", err)
	}

	multiSig.LockFunds(ch.ID, total)
	if got := multiSig.GetLockedFunds(ch.ID); got != total {
		t.Fatalf("incorrect locked funds amount: got %d want %d", got, total)
	}

	finalState := &interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  1,
		BalanceA:  7500,
		BalanceB:  2500,
		Timestamp: time.Now(),
	}
	parties.signState(finalState)

	_, err = sm.BuildSettlement(ctx, ch.ID, finalState)
	if err != nil {
		t.Fatalf("BuildSettlement failed: %v", err)
	}

	_, err = sm.ExecuteSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ExecuteSettlement failed: %v", err)
	}

	// mockMultiSig records the unlocked amount in SpendMultiSig
	var unlockKey [32]byte
	copy(unlockKey[:], parties.partyA[:])
	unlocked := multiSig.GetUnlockedFunds(unlockKey)
	if unlocked != total {
		t.Fatalf("incorrect unlocked funds amount: got %d want %d", unlocked, total)
	}
}
