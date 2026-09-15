package utxo

import (
	"crypto/ed25519"
	"crypto/rand"
	"math/big"
	"testing"
)

// Regression: difficulty must only change at window boundaries and must NOT
// compound within a window. The old implementation retargeted at EVERY block
// (clamped 4x each time), so a fast window multiplied difficulty by 4 per
// block and wedged the chain (live incident: h64..h78 unmineable at h79).
func TestNextPoWBitsForHeight_NoPerBlockCompounding(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	cs, err := NewChainState(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("chain state: %v", err)
	}
	// Genesis at h0 with genesis bits.
	var gp [32]byte
	copy(gp[:], []byte("genesis"))
	gcb := CreateCoinbaseTransaction(gp, 0, []byte("test-genesis"))
	genesis := NewBlock([]*Transaction{gcb}, [32]byte{}, 0, gp)
	genesis.Header.Version = 3
	genesis.Header.Bits = PoWGenesisBits
	genesis.Header.Timestamp = uint64(1700000000)
	for nonce := uint64(0); ; nonce++ {
		genesis.Header.Nonce = nonce
		if CheckProofOfWork(&genesis.Header) {
			break
		}
	}
	genesis.Hash = genesis.CalculateHash()
	if err := cs.InitGenesis(genesis); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	var prevHash = genesis.Hash
	// Build a synthetic PoW chain with fast timestamps (1s/block vs 60s target).
	prevBits := PoWGenesisBits
	for h := uint64(1); h <= 3*PoWRetargetWindow; h++ {
		cb := CreateCoinbaseTransaction([32]byte{}, 0, []byte("test"))
		b := NewBlock([]*Transaction{cb}, prevHash, h, [32]byte{})
		b.Header.Version = 3
		b.Header.Bits = prevBits
		b.Header.Timestamp = uint64(1700000000 + h)
		copy(b.Header.ProposerKey[:], pub)
		for nonce := uint64(0); ; nonce++ {
			b.Header.Nonce = nonce
			if CheckProofOfWork(&b.Header) {
				break
			}
		}
		b.Hash = b.CalculateHash()
		if err := b.SignBlock(priv); err != nil {
			t.Fatalf("sign h%d: %v", h, err)
		}
		if err := cs.AddBlock(b); err != nil {
			t.Fatalf("add h%d: %v", h, err)
		}
		prevHash = b.Hash

		next := cs.NextPoWBitsForHeight(h)
		if h%PoWRetargetWindow != 0 {
			// Inside a window: bits MUST equal parent's — no compounding.
			if next != prevBits {
				t.Fatalf("h%d: in-window bits changed %08x -> %08x (compounding bug)", h, prevBits, next)
			}
		} else {
			// Boundary with 1s blocks vs 60s target: difficulty up, target down.
			if next == prevBits {
				t.Fatalf("h%d: boundary did not retarget", h)
			}
			pt, _ := TargetFromBits(prevBits)
			nt, _ := TargetFromBits(next)
			if nt.Cmp(pt) >= 0 {
				t.Fatalf("h%d: fast window must lower target (raise difficulty)", h)
			}
			// Drop bounded by the 4x clamp (allow mantissa-encoding
			// rounding of ~1%): nt >= pt/4 * 0.99.
			minAllowed := new(big.Int).Div(pt, big.NewInt(4))
			minAllowed.Div(minAllowed, big.NewInt(100))
			minAllowed.Mul(minAllowed, big.NewInt(99))
			if nt.Cmp(minAllowed) < 0 {
				t.Fatalf("h%d: clamp violated — target dropped more than 4x", h)
			}
		}
		prevBits = next
	}
}
