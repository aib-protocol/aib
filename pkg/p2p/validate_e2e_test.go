package p2p

import (
	"fmt"
	"testing"

	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// TestValidateFetchedBlocksE2E: build 3 real chained blocks, serialize
// them as a serving node would (handleGetBlocks), then validate them
// as the fetching node would. All 3 must survive.
func TestValidateFetchedBlocksE2E(t *testing.T) {
	// Build a tiny chain using the real block constructors.
	b0 := utxoPkg.NewBlock(nil, [32]byte{1}, 0, [32]byte{9})
	b0.Hash = b0.CalculateHash()
	b1 := utxoPkg.NewBlock(nil, b0.Hash, 1, [32]byte{9})
	b1.Hash = b1.CalculateHash()
	b2 := utxoPkg.NewBlock(nil, b1.Hash, 2, [32]byte{9})
	b2.Hash = b2.CalculateHash()

	toBD := func(b *utxoPkg.Block) BlockData {
		return BlockData{
			Height:        b.Header.Height,
			Hash:          fmt.Sprintf("%x", b.Hash[:]),
			PrevBlockHash: fmt.Sprintf("%x", b.Header.PrevBlockHash[:]),
			RawBlock:      b.SerializeBlock(),
		}
	}
	blocks := []BlockData{toBD(b0), toBD(b1), toBD(b2)}

	// lookup: height 0 body locally known (like a full node)
	lookup := func(h uint64) (string, bool) {
		if h == 0 {
			return fmt.Sprintf("%x", b0.Hash[:]), true
		}
		return "", false
	}
	out := ValidateFetchedBlocks(blocks, 0, 2, "", lookup)
	t.Logf("survivors: %+v", out)
	if len(out) != 3 {
		for _, o := range out {
			t.Logf("alive h=%d hash=%s", o.Height, o.Hash[:12])
		}
		t.Fatalf("expected 3 valid blocks, got %d", len(out))
	}
}
