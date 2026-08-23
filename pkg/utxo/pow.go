package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"
)

// =====================================================================
// Bitcoin-style double-SHA256 Proof of Work (Consensus V3 era).
// Blocks 1..PoWEraBlocks are Version 3 PoW blocks; after that the
// chain switches to pure-stake VRF PoS (RFC-002 Route C).
// =====================================================================

// PoW era parameters.
const (
	PoWEraBlocks     uint64 = 10000             // blocks 1..10000 are PoW
	PoWBlockReward   uint64 = 31415000000       // 3.1415 AIB per block (1e8 units) => 31415 AIB total
	PoWTargetSpacing        = 360 * time.Millisecond // testnet fast era: 10000 blocks ≈ 1 hour
	PoWRetargetWindow uint64 = 64                // retarget every 64 blocks, clamp [1/4, 4x]
	// Genesis / easiest target: difficulty-1 style easy limit (Bitcoin testnet-like)
	PoWGenesisBits uint32 = 0x207fffff // very easy target for instant genesis & fast start
)

// PoWMaxTarget is the easiest allowed target (from PoWGenesisBits).
func PoWMaxTarget() *big.Int {
	target, _ := TargetFromBits(PoWGenesisBits)
	return target
}

// HashHeaderSHA256d computes Bitcoin-style double SHA256 over the
// serialized header (includes Nonce and Bits).
func HashHeaderSHA256d(h *BlockHeader) [32]byte {
	data := h.Serialize()
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	return h2
}

// CheckProofOfWork returns true when SHA256d(header) as LE integer is
// below the target encoded in header.Bits.
func CheckProofOfWork(h *BlockHeader) bool {
	target, err := TargetFromBits(h.Bits)
	if err != nil || target.Sign() <= 0 {
		return false
	}
	hash := HashHeaderSHA256d(h)
	hashInt := new(big.Int).SetBytes(rev(hash[:])) // hash bytes are big-endian display; compare as LE256 like Bitcoin
	return hashInt.Cmp(target) < 0
}

// rev reverses a byte slice (copy).
func rev(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}

// TargetFromBits converts Bitcoin compact bits to a big.Int target.
func TargetFromBits(bits uint32) (*big.Int, error) {
	mantissa := int64(bits & 0x007fffff)
	isNegative := bits&0x00800000 != 0
	exponent := int64(bits >> 24)

	if mantissa == 0 {
		return big.NewInt(0), fmt.Errorf("invalid bits: zero mantissa")
	}
	if exponent > 34 {
		return big.NewInt(0), fmt.Errorf("invalid bits: exponent too large")
	}

	target := big.NewInt(mantissa)
	if exponent <= 3 {
		target.Lsh(target, uint(8*(3-exponent)))
	} else {
		target.Lsh(target, uint(8*(exponent-3)))
	}
	if isNegative {
		return big.NewInt(0), fmt.Errorf("invalid bits: negative target")
	}
	return target, nil
}

// BitsFromTarget converts a target big.Int to compact bits form.
func BitsFromTarget(target *big.Int) (uint32, error) {
	if target.Sign() <= 0 {
		return 0, fmt.Errorf("target must be positive")
	}
	// find number of bytes needed
	nBytes := (target.BitLen() + 7) / 8
	if nBytes > 32 {
		return 0, fmt.Errorf("target too large")
	}
	var exponent uint32
	var mantissa big.Int
	if nBytes <= 3 {
		exponent = 3
		mantissa.Set(target)
	} else {
		exponent = uint32(nBytes)
		// mantissa = target >> (8*(nBytes-3))
		mantissa.Rsh(target, uint(8*(nBytes-3)))
	}
	mant := mantissa.Uint64()
	if mant&0x00800000 != 0 {
		// mantissa overflows 23 bits — shift right by one, bump exponent
		mant >>= 1
		exponent++
	}
	bits := uint32(exponent<<24) | uint32(mant)
	return bits, nil
}

// NextWorkRequired computes the Bits for the next PoW block using a
// simple per-window retarget: target *= actual_span / target_span,
// clamped to [1/4, 4x] of the previous target, capped at PoWMaxTarget.
// prevHeight is the height of the tip block.
func NextWorkRequired(prevBits uint32, prevHeight uint64, windowStartTimestamp, tipTimestamp uint64) uint32 {
	// Before first full window: keep genesis bits
	if prevHeight < PoWRetargetWindow {
		return prevBits
	}
	prevTarget, err := TargetFromBits(prevBits)
	if err != nil || prevTarget.Sign() <= 0 {
		return PoWGenesisBits
	}

	actualSec := int64(tipTimestamp) - int64(windowStartTimestamp)
	// timestamps are SECONDS; convert target span to seconds as float-then-scale
	targetMs := int64(float64(PoWRetargetWindow) * float64(PoWTargetSpacing) / float64(time.Millisecond))
	target64 := targetMs
	actual := actualSec * 1000 // ms
	if target64 <= 0 {
		return prevBits
	}

	// clamp actual timespan to [targetSpan/4, targetSpan*4]
	minSpan := target64 / 4
	maxSpan := target64 * 4
	if actual < minSpan {
		actual = minSpan
	}
	if actual > maxSpan {
		actual = maxSpan
	}
	if actual <= 0 {
		return prevBits
	}

	// newTarget = prevTarget * actual / targetSpan
	newTarget := new(big.Int).Mul(prevTarget, big.NewInt(actual))
	newTarget.Div(newTarget, big.NewInt(target64))

	// cap at max target
	maxT := PoWMaxTarget()
	if newTarget.Cmp(maxT) > 0 {
		newTarget.Set(maxT)
	}
	bits, err := BitsFromTarget(newTarget)
	if err != nil {
		return PoWGenesisBits
	}
	// re-encode cap: encoding roundtrip may round up past max target
	encT, err2 := TargetFromBits(bits)
	if err2 == nil && encT.Cmp(maxT) > 0 {
		return PoWGenesisBits
	}
	return bits
}

// SerializePoWExtra appends Nonce+Bits at the very end of the serialized
// header — Version 3 extension, old deserializers unaffected.
func SerializePoWExtra(buf *bytes.Buffer, nonce uint64, bits uint32) {
	binary.Write(buf, binary.LittleEndian, nonce)
	binary.Write(buf, binary.LittleEndian, bits)
}
