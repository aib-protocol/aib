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
	slow := NextWorkRequired(PoWGenesisBits, PoWRetargetWindow, ts, ts+3600)
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
	if PoWBlockReward != 3141500000 {
		t.Fatalf("PoW reward %d != 31.415 AIB", PoWBlockReward)
	}
	if PoWBlockReward*PoWEraBlocks != 3141500000000 {
		t.Fatalf("total era reward != 31415 AIB")
	}
}
