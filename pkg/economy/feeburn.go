// Package economy — Fee-Burn economy (RFC-002).
//
// Core identity:  APR = φ·T / S
//   φ (phi)  — protocol cut of transaction flow (TRIAL FIXED: 7%)
//   T        — annual settled transaction volume
//   S        — total staked supply (the security budget)
//
// The protocol burns a share of every fee, so stakers earn exactly the
// burn-driven scarcity; idle coins earn nothing and are diluted — the
// anti-sleeping-giant mechanism. No inflation, no treasury, no premine.
package economy

import (
	"fmt"
	"math/big"
)

// Trial parameters (decision 2026-08-22: φ = 7% fixed, option B, trial run).
const (
	PhiFixedNumerator   = 7  // φ = 7%
	PhiFixedDenominator = 100
)

// PhiFixed returns the trial φ as a big.Rat (7/100).
func PhiFixed() *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(PhiFixedNumerator), big.NewInt(PhiFixedDenominator))
}

// State is the per-epoch economic snapshot.
type State struct {
	EpochHeight    uint64   // epoch boundary height
	TxFeeSatoshi   uint64   // T: fees paid by transactions this epoch (in satoshi)
	FeeBurned      uint64   // fees burned this epoch (removed from supply forever)
	TotalStakeSat  uint64   // S: total active stake (satoshi)
	EffectiveSupply uint64  // circulating supply for APR math (satoshi)
}

// BurnSplit divides an epoch's fees: burn share φ_burn, staker share rest.
// Trial simplification (documented in RFC-002 §6.1 open question 3):
//   100% of fees go to stakers pro-rata this epoch; the *inflation ceiling*
//   of the year is bounded by φ·T — if realized staker APR would exceed
//   the φ target, the excess above target is burned instead of paid.
func BurnSplit(feesSatoshi uint64, stakersStakeSatoshi, totalSupplySatoshi uint64, epochSeconds uint64) (payToStakers, burn uint64, targetApr *big.Rat, err error) {
	if feesSatoshi == 0 {
		return 0, 0, nil, fmt.Errorf("no fees this epoch")
	}
	if totalSupplySatoshi == 0 || stakersStakeSatoshi == 0 || epochSeconds == 0 {
		return 0, 0, nil, fmt.Errorf("invalid state: stake=%d supply=%d epochSec=%d", stakersStakeSatoshi, totalSupplySatoshi, epochSeconds)
	}

	// Payment ceiling derived from the TARGET, not from fees:
	// stakers may earn at most φ × S per year → cap = φ·S·epochSec/secsPerYear.
	// (The old fee-derived cap inverted the identity — burn happened even when
	//  realized APR was below target, double-squeezing stakers.)
	secsPerYear := big.NewInt(31536000)
	stakeBig := new(big.Int).SetUint64(stakersStakeSatoshi)
	capBig := new(big.Int).Mul(stakeBig, big.NewInt(PhiFixedNumerator))
	capBig.Mul(capBig, new(big.Int).SetUint64(epochSeconds))
	capBig.Div(capBig, new(big.Int).Mul(secsPerYear, big.NewInt(PhiFixedDenominator)))
	if capBig.Sign() == 0 {
		capBig.SetInt64(1) // sub-satoshi rounding floor
	}

	// naive full payout = all fees to stakers
	feesBig := new(big.Int).SetUint64(feesSatoshi)
	if feesBig.Cmp(capBig) <= 0 {
		// paying all fees stays under the φ·S/year ceiling — pay everything, burn nothing
		return feesBig.Uint64(), 0, AprFrom(feesSatoshi, stakersStakeSatoshi, epochSeconds), nil
	}
	// paying all fees would exceed the ceiling — pay the cap, burn the excess
	burned := new(big.Int).Sub(feesBig, capBig)
	return capBig.Uint64(), burned.Uint64(), AprFrom(capBig.Uint64(), stakersStakeSatoshi, epochSeconds), nil
}

// AprFrom computes realized staker APR = payout_annualized / stake.
func AprFrom(epochPayoutSatoshi, stakeSatoshi uint64, epochSeconds uint64) *big.Rat {
	if stakeSatoshi == 0 || epochSeconds == 0 {
		return new(big.Rat)
	}
	secsPerYear := big.NewInt(31536000)
	annualized := new(big.Int).Mul(new(big.Int).SetUint64(epochPayoutSatoshi), secsPerYear)
	annualized.Div(annualized, bigInt(epochSeconds))
	return new(big.Rat).SetFrac(annualized, new(big.Int).SetUint64(stakeSatoshi))
}

func bigInt(v uint64) *big.Int { return new(big.Int).SetUint64(v) }
