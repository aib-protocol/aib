package utxo

import (
	"math/big"
	"testing"
	"time"
)

func TestSHA256dDeterministic(t *testing.T) {
	h := &BlockHeader{Version: 3, Height: 1, Bits: PoWGenesisBits}
	a := HashHeaderSHA256d(h)
	b := HashHeaderSHA256d(h)
	if a != b {
		t.Fatal("SHA256d not deterministic")
	}
	h.Nonce = 1
	c := HashHeaderSHA256d(h)
	if a == c {
		t.Fatal("nonce does not change hash")
	}
}

func TestBitsTargetRoundtrip(t *testing.T) {
	target, err := TargetFromBits(PoWGenesisBits)
	if err != nil {
		t.Fatalf("genesis bits invalid: %v", err)
	}
	bits2, err := BitsFromTarget(target)
	if err != nil {
		t.Fatalf("target->bits failed: %v", err)
	}
	target2, _ := TargetFromBits(bits2)
	if target.Cmp(target2) != 0 {
		t.Fatalf("roundtrip mismatch: %v vs %v", target, target2)
	}
}

func TestCheckProofOfWork(t *testing.T) {
	h := &BlockHeader{Version: 3, Height: 1, Bits: PoWGenesisBits}
	h.Nonce = 0
	found := false
	for n := uint64(0); n < 100000; n++ {
		h.Nonce = n
		if CheckProofOfWork(h) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("genesis-difficulty PoW not found in 100k nonces")
	}
	// tamper: make target impossibly hard, same nonce must fail
	h2 := &BlockHeader{Version: 3, Height: 1, Bits: 0x01010000, Nonce: h.Nonce}
	if CheckProofOfWork(h2) {
		t.Fatal("hash below impossibly hard target?")
	}
}

func TestNextWorkRequiredClamp(t *testing.T) {
	// very slow window => target should grow (difficulty drop), capped at 4x
	prev, _ := TargetFromBits(PoWGenesisBits)
	ts := uint64(1700000000)
	slow := NextWorkRequired(PoWGenesisBits, PoWRetargetWindow, ts, ts+36000)
	// genesis bits == max target already; slow window must stay at max (easiest)
	if slow != PoWGenesisBits {
		t.Fatalf("capped slow window should stay at genesis bits, got %08x", slow)
	}
	harder, _ := big.NewInt(0).Div(prev, big.NewInt(8)).Uint64(), error(nil)
	_ = harder
	hb, _ := BitsFromTarget(new(big.Int).Div(prev, big.NewInt(8)))
	fast := NextWorkRequired(hb, PoWRetargetWindow, ts, ts+2)
	gotFast, _ := TargetFromBits(fast)
	prevHard, _ := TargetFromBits(hb)
	if gotFast.Cmp(prevHard) > 0 {
		t.Fatal("fast window must not decrease difficulty")
	}
	_ = time.Now
}

func TestPoWEraReward(t *testing.T) {
	// Mainnet schedule: 747 AIB initial, halving every 2,102,400 blocks.
	// Integer subsidy gives total emission 3,140,985,600 AIB — the remaining
	// 0.019% of the π×10⁹ cap is never minted (Bitcoin-style tail behavior).
	if PoWBlockReward != 747*1e8 {
		t.Fatalf("PoW reward %d != 747 AIB", PoWBlockReward)
	}
	if 2*PoWBlockReward*PoWHalvingBlocks != 3140985600*1e8 {
		t.Fatalf("total emission != 3,140,985,600 AIB, got %d AIB", 2*PoWBlockReward*PoWHalvingBlocks/1e8)
	}
}
