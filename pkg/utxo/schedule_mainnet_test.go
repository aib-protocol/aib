package utxo

import "testing"

func TestPoWSubsidySchedule(t *testing.T) {
	if r := PoWSubsidyAtHeight(1); r != 747*1e8 {
		t.Fatalf("h1 subsidy = %d, want %d", r, uint64(747*1e8))
	}
	if r := PoWSubsidyAtHeight(2102401); r != 747*1e8/2 {
		t.Fatalf("first halving subsidy = %d, want %d", r, uint64(747*1e8/2))
	}
	if r := PoWSubsidyAtHeight(0); r != 0 {
		t.Fatalf("genesis should have zero subsidy, got %d", r)
	}
	// total emission ≈ 2*R0*B
	if PoWBlockReward != 747*1e8 || PoWHalvingBlocks != 2102400 {
		t.Fatal("constants drifted")
	}
}
