package utxo

import (
	"crypto/ed25519"
	"testing"
)

func newTestState(t *testing.T) *ConsensusState {
	t.Helper()
	cs := NewConsensusState(&PoSConfig{MinStake: 1, MaxValidators: 100})
	return cs
}

// TestPureStakeSelectionNoReputation: pure stake weighting (α=1.0, β=0 —
// RFC-002 Route C). 1:2:7 stakes must win ~10%/20%/70% over many draws.
func TestPureStakeSelectionNoReputation(t *testing.T) {
	cs := newTestState(t)

	var a, b, c [32]byte
	a[0], b[0], c[0] = 1, 2, 3
	if err := cs.AddValidator(a, 100*1e8, nil); err != nil {
		t.Fatal(err)
	}
	if err := cs.AddValidator(b, 200*1e8, nil); err != nil {
		t.Fatal(err)
	}
	if err := cs.AddValidator(c, 700*1e8, nil); err != nil {
		t.Fatal(err)
	}

	seed := []byte("genesis-seed")
	counts := map[[32]byte]int{}
	const rounds = 3000
	for i := 0; i < rounds; i++ {
		p, err := cs.SelectProposerVRFDeterministic(seed)
		if err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		counts[p.Winner]++
		seed = p.Output[:32]
	}

	pa := float64(counts[a]) / rounds
	pb := float64(counts[b]) / rounds
	pc := float64(counts[c]) / rounds
	t.Logf("a(10%%)=%.1f%% b(20%%)=%.1f%% c(70%%)=%.1f%%", pa*100, pb*100, pc*100)
	if pa < 0.06 || pa > 0.14 {
		t.Errorf("a expected ~10%% got %.1f%%", pa*100)
	}
	if pb < 0.15 || pb > 0.25 {
		t.Errorf("b expected ~20%% got %.1f%%", pb*100)
	}
	if pc < 0.64 || pc > 0.76 {
		t.Errorf("c expected ~70%% got %.1f%%", pc*100)
	}
}

// TestWinnerChangesWithHeight: same seed, different heights must reshuffle.
func TestWinnerChangesWithHeight(t *testing.T) {
	cs := newTestState(t)
	var a, b [32]byte
	a[0], b[0] = 1, 2
	if err := cs.AddValidator(a, 500*1e8, nil); err != nil {
		t.Fatal(err)
	}
	if err := cs.AddValidator(b, 500*1e8, nil); err != nil {
		t.Fatal(err)
	}
	cs.SetHeightForTest(1)

	distinct := map[[32]byte]bool{}
	seed := []byte("s")
	for h := uint64(1); h <= 50; h++ {
		cs.SetHeightForTest(h)
		p, err := cs.SelectProposerVRFDeterministic(seed)
		if err != nil {
			t.Fatal(err)
		}
		distinct[p.Winner] = true
	}
	if len(distinct) < 2 {
		t.Errorf("50 heights at 50/50 stake never switched winner — not height-sensitive")
	}
}

// TestEvidenceShape: proof carries winner + stake snapshot + chained output.
func TestEvidenceShape(t *testing.T) {
	cs := newTestState(t)
	var a [32]byte
	a[0] = 9
	if err := cs.AddValidator(a, 1000*1e8, nil); err != nil {
		t.Fatal(err)
	}
	cs.SetHeightForTest(42)
	p, err := cs.SelectProposerVRFDeterministic([]byte("seed"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Winner != a {
		t.Errorf("single validator must always win")
	}
	if p.Height != 42 {
		t.Errorf("height %d != 42", p.Height)
	}
	if p.Stakes[a] != 1000*1e8 {
		t.Errorf("stake snapshot missing")
	}
	if len(p.Output) != 64 {
		t.Errorf("output must be 64 bytes")
	}
}

// TestVrfSignVerify: ed25519-based VRF proof verifies, and tampering fails.
func TestVrfSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	seed := []byte("blockseed")
	out, proof := EvaluateVrf(priv, seed, 7)
	if !VerifyVrf(pub, seed, 7, out, proof) {
		t.Fatal("valid proof must verify")
	}
	// wrong height must fail
	if VerifyVrf(pub, seed, 8, out, proof) {
		t.Fatal("proof must not verify for different height")
	}
	// tampered proof must fail
	proof[0] ^= 0xFF
	if VerifyVrf(pub, seed, 7, out, proof) {
		t.Fatal("tampered proof must fail")
	}
}
