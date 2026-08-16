package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// settlementIntegrationParties 封装集成测试参与方密钥与地址
// 真实签名流程：使用 ed25519 对实际状态进行签名
// 不使用任何虚假签名数据
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
		t.Fatalf("生成 PartyA 密钥失败: %v", err)
	}
	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成 PartyB 密钥失败: %v", err)
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
		t.Fatalf("创建通道管理器失败: %v", err)
	}

	sm, err := NewSettlementManager(mgr, &SettlementConfig{
		ChallengePeriod:     challengePeriod,
		ConfirmationDepth:   1,
		MinSettlementAmount: 1,
		MultiSigLocker:      multiSig,
	})
	if err != nil {
		t.Fatalf("创建结算管理器失败: %v", err)
	}

	return mgr, sm, multiSig
}

// 1. 通道开启流程
func TestSettlementIntegration_ChannelOpenFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr, _, _ := newSettlementIntegrationManagers(t, 50*time.Millisecond)
	parties := newSettlementIntegrationParties(t)

	depositA := uint64(7000)
	depositB := uint64(3000)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, depositA, depositB)
	if err != nil {
		t.Fatalf("OpenChannel 失败: %v", err)
	}

	if ch.BalanceA != depositA || ch.BalanceB != depositB {
		t.Fatalf("通道初始余额不正确: got A=%d B=%d", ch.BalanceA, ch.BalanceB)
	}

	status, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus 失败: %v", err)
	}
	if status != StateOpen {
		t.Fatalf("期望通道状态 StateOpen(%d), 实际 %d", StateOpen, status)
	}

	stored, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState 失败: %v", err)
	}
	if stored.BalanceA+stored.BalanceB != depositA+depositB {
		t.Fatalf("通道总金额不守恒: got %d want %d", stored.BalanceA+stored.BalanceB, depositA+depositB)
	}
}

// 2. 正常结算流程
func TestSettlementIntegration_CooperativeSettlementFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr, sm, _ := newSettlementIntegrationManagers(t, 100*time.Millisecond)
	parties := newSettlementIntegrationParties(t)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, 6000, 4000)
	if err != nil {
		t.Fatalf("OpenChannel 失败: %v", err)
	}

	// 真实链下状态演进
	_, err = mgr.Transfer(ch.ID, 1000, true) // A -> B
	if err != nil {
		t.Fatalf("Transfer(A->B) 失败: %v", err)
	}
	_, err = mgr.Transfer(ch.ID, 500, false) // B -> A
	if err != nil {
		t.Fatalf("Transfer(B->A) 失败: %v", err)
	}

	latest, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState 失败: %v", err)
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
		t.Fatalf("BuildSettlement 失败: %v", err)
	}
	if settlement.Status != SettlementPending {
		t.Fatalf("期望 SettlementPending, 实际 %d", settlement.Status)
	}

	settlement, err = sm.ExecuteSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ExecuteSettlement 失败: %v", err)
	}
	if settlement.Status != SettlementConfirming {
		t.Fatalf("期望 SettlementConfirming, 实际 %d", settlement.Status)
	}

	err = sm.ConfirmSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ConfirmSettlement 失败: %v", err)
	}

	status, err := sm.GetSettlementStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetSettlementStatus 失败: %v", err)
	}
	if status != SettlementComplete {
		t.Fatalf("期望 SettlementComplete, 实际 %d", status)
	}

	channelStatus, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus 失败: %v", err)
	}
	if channelStatus != StateClosed {
		t.Fatalf("期望通道关闭 StateClosed(%d), 实际 %d", StateClosed, channelStatus)
	}
}

// 3. 争议处理流程
func TestSettlementIntegration_DisputeResolutionFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	challengePeriod := 20 * time.Millisecond
	mgr, sm, _ := newSettlementIntegrationManagers(t, challengePeriod)
	parties := newSettlementIntegrationParties(t)

	ch, err := mgr.OpenChannel(ctx, parties.partyA, parties.partyB, 5000, 5000)
	if err != nil {
		t.Fatalf("OpenChannel 失败: %v", err)
	}

	// 先产生一笔真实状态更新
	_, err = mgr.Transfer(ch.ID, 1200, true)
	if err != nil {
		t.Fatalf("Transfer 失败: %v", err)
	}

	latest, err := mgr.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelState 失败: %v", err)
	}

	// 双方签名的强制结算（模拟合作强制关闭场景）
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
		t.Fatalf("ForceClose 失败: %v", err)
	}
	if settlement.Type != SettlementForce {
		t.Fatalf("期望 SettlementForce, 实际 %d", settlement.Type)
	}

	// 争议期内确认应失败
	_, err = sm.ConfirmForceClose(ctx, ch.ID)
	if err == nil {
		t.Fatalf("争议期未结束时 ConfirmForceClose 应失败")
	}

	time.Sleep(challengePeriod + 5*time.Millisecond)

	settlement, err = sm.ConfirmForceClose(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ConfirmForceClose 失败: %v", err)
	}
	if settlement.Status != SettlementConfirming {
		t.Fatalf("期望 SettlementConfirming, 实际 %d", settlement.Status)
	}

	channelStatus, err := mgr.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelStatus 失败: %v", err)
	}
	if channelStatus != StateClosed {
		t.Fatalf("期望通道关闭 StateClosed(%d), 实际 %d", StateClosed, channelStatus)
	}
}

// 4. 资金解锁流程
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
		t.Fatalf("OpenChannel 失败: %v", err)
	}

	multiSig.LockFunds(ch.ID, total)
	if got := multiSig.GetLockedFunds(ch.ID); got != total {
		t.Fatalf("资金锁定金额不正确: got %d want %d", got, total)
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
		t.Fatalf("BuildSettlement 失败: %v", err)
	}

	_, err = sm.ExecuteSettlement(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ExecuteSettlement 失败: %v", err)
	}

	// mockMultiSig 在 SpendMultiSig 中记录解锁金额
	var unlockKey [32]byte
	copy(unlockKey[:], parties.partyA[:])
	unlocked := multiSig.GetUnlockedFunds(unlockKey)
	if unlocked != total {
		t.Fatalf("资金解锁金额不正确: got %d want %d", unlocked, total)
	}
}
