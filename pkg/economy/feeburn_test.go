package economy

import (
	"math/big"
	"testing"
)

// TestPhiTrialIs7Percent: the decision of 2026-08-22 is locked in code.
func TestPhiTrialIs7Percent(t *testing.T) {
	phi := PhiFixed()
	if phi.Cmp(big.NewRat(7, 100)) != 0 {
		t.Errorf("phi must be 7%%, got %s", phi.RatString())
	}
}

// TestBurnSplitUnderTarget: low volume → all fees paid, nothing burned.
func TestBurnSplitUnderTarget(t *testing.T) {
	// Cap = φ·S/year = 0.07 × 10M AIB / 365 days = 1917 AIB/epoch.
	// fees = 100 AIB < cap → pay all, burn 0, APR = 100×365/10M = 0.365% (< 7% ✓)
	pay, burn, apr, err := BurnSplit(100*1e8, 10_000_000*1e8, 3_141_592_653*1e8, 86400)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pay=%d burn=%d apr=%s", pay, burn, apr.RatString())
	if pay != 100*1e8 || burn != 0 {
		t.Errorf("under ceiling: pay=%d burn=%d", pay, burn)
	}
	// realized APR must not exceed the 7% target
	if apr.Cmp(PhiFixed()) > 0 {
		t.Errorf("realized APR %s exceeds φ=7%% target — flywheel broken", apr.RatString())
	}
}

// TestBurnSplitHighStake: massive stake + tiny volume → all fees paid, APR tiny.
func TestBurnSplitHighStake(t *testing.T) {
	// stake = 1B AIB, fees = 100 AIB/day
	pay, burn, apr, err := BurnSplit(100*1e8, 1_000_000_000*1e8, 3_141_592_653*1e8, 86400)
	if err != nil {
		t.Fatal(err)
	}
	if pay != 100*1e8 || burn != 0 {
		t.Errorf("under target: expect full pay no burn, got pay=%d burn=%d", pay, burn)
	}
	// APR = 36500/1B ≈ 0.00365% — near zero, correctly reflects weak flow
	if apr.Cmp(big.NewRat(365, 10_000_000)) > 0 {
		t.Errorf("APR should be tiny, got %s", apr.RatString())
	}
	t.Logf("pay=%d burn=%d apr=%s", pay, burn, apr.RatString())
}

// TestBurnSplitOverCeiling: tiny stake + big volume → payment capped at φ·S, excess burned.
func TestBurnSplitOverCeiling(t *testing.T) {
	// stake = 100k AIB → cap/epoch = 0.07×100k/365 = 19.17 AIB
	// fees = 100 AIB > 19.17 → pay 19, burn 81, APR = exactly 7%
	pay, burn, apr, err := BurnSplit(100*1e8, 100_000*1e8, 3_141_592_653*1e8, 86400)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pay=%d burn=%d apr=%s", pay, burn, apr.RatString())
	if burn == 0 {
		t.Errorf("over ceiling must burn: pay=%d burn=%d", pay, burn)
	}
	if apr.Cmp(PhiFixed()) > 0 {
		t.Errorf("APR %s must be capped at 7%%", apr.RatString())
	}
}

// TestEconomicIdentity: the soul of RFC-002 — APR ≤ φ·T/S always.
func TestEconomicIdentity(t *testing.T) {
	cases := []struct{ fees, stake uint64 }{
		{1 * 1e8, 100_000 * 1e8},
		{10_000 * 1e8, 10_000_000 * 1e8},
		{1_000_000 * 1e8, 50_000_000 * 1e8},
	}
	for i, c := range cases {
		_, _, apr, err := BurnSplit(c.fees, c.stake, 3_141_592_653*1e8, 86400)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if apr.Cmp(PhiFixed()) > 0 {
			t.Errorf("case %d: APR %s exceeded 7%% ceiling", i, apr.RatString())
		}
	}
}

// TestZeroFeesErrors: no flow, no reward — idle earns nothing.
func TestZeroFeesErrors(t *testing.T) {
	if _, _, _, err := BurnSplit(0, 1000*1e8, 3_141_592_653*1e8, 86400); err == nil {
		t.Errorf("zero fees must be an explicit error (no flow = no reward)")
	}
}
