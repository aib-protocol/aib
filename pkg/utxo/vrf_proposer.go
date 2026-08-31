// Package utxo — VRF proposer selection (RFC-002 Route C, RFC-003).
// Pure stake-weighted verifiable sortition. No reputation weighting (α=1.0, β=0).
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"
)

// VrfProof is the cryptographic evidence attached to every block.
// Anyone can verify: output is the unique VRF evaluation of (seed, height)
// under the proposer's public key, and the winner was stake-entitled.
type VrfProof struct {
	Output [64]byte // VRF output — becomes next block's seed
	Proof  [80]byte // VRF proof
	Height uint64
	Winner [32]byte            // proposer address
	Stakes map[[32]byte]uint64 // snapshot of active stakes at height (for verification)
}

// vrfHashToIndex is the domain-separated VRF hash: if the proposer can produce
// a proof whose SHA256(seed||height||output) is smaller than
// stake × 2^256 / totalStake, they win the slot.
//
// This is Algorand-style sortition: EVERY validator locally computes their own
// priority from their own VRF evaluation. No global "roll then pick" — a winner
// knows they won without asking anyone, and everyone else verifies the proof.
func vrfThreshold(stake, totalStake uint64) *big.Int {
	// threshold = stake * 2^256 / totalStake
	s := new(big.Int).SetUint64(stake)
	s.Mul(s, new(big.Int).Lsh(big.NewInt(1), 256))
	s.Div(s, new(big.Int).SetUint64(totalStake))
	return s
}

func vrfHash(seed []byte, height uint64, output []byte) *big.Int {
	h := sha256.New()
	h.Write([]byte("AIB-VRF-v1"))
	h.Write(seed)
	binary.Write(h, binary.BigEndian, height)
	h.Write(output)
	return new(big.Int).SetBytes(h.Sum(nil))
}

// EvaluateVrf: proposer-side. Returns (output, proof).
// Uses a minimal ed25519-based VRF: output = SHA512(pub||msg||sig) where
// sig = ed25519.Sign(msg). Unforgeability of ed25519 gives uniqueness;
// for mainnet, swap in an IETF-standard VRF (RFC 9381) — see AIP-Core.
func EvaluateVrf(priv ed25519.PrivateKey, seed []byte, height uint64) ([64]byte, [80]byte) {
	var msg []byte
	msg = append(msg, []byte("AIB-VRF-v1")...)
	msg = append(msg, seed...)
	var hbuf [8]byte
	binary.BigEndian.PutUint64(hbuf[:], height)
	msg = append(msg, hbuf[:]...)
	sig := ed25519.Sign(priv, msg)

	out := sha512.Sum512(append(append(append([]byte{}, priv.Public().(ed25519.PublicKey)...), msg...), sig...))
	var output [64]byte
	copy(output[:], out[:])
	var proof [80]byte
	copy(proof[:], sig) // ed25519 sig is 64 bytes; padded
	return output, proof
}

// VerifyVrf: verifier-side. Anyone can run this.
func VerifyVrf(pub ed25519.PublicKey, seed []byte, height uint64, output [64]byte, proof [80]byte) bool {
	var msg []byte
	msg = append(msg, []byte("AIB-VRF-v1")...)
	msg = append(msg, seed...)
	var hbuf [8]byte
	binary.BigEndian.PutUint64(hbuf[:], height)
	msg = append(msg, hbuf[:]...)
	if len(proof) < 64 || !ed25519.Verify(pub, msg, proof[:64]) {
		return false
	}
	expected := sha512.Sum512(append(append(append([]byte{}, pub...), msg...), proof[:64]...))
	var exp [64]byte
	copy(exp[:], expected[:])
	return exp == output
}

// SetHeightForTest advances the consensus height (test helper).
func (cs *ConsensusState) SetHeightForTest(h uint64) {
	cs.mu.Lock()
	cs.currentHeight = h
	cs.mu.Unlock()
}

// vrfThreshold_placeholder keeps threshold math exported for future AIP-Core
// VRF (RFC 9381) swap — see vrf_proposer.go vrfThreshold.
var _ = vrfThreshold

// SelectProposerVRF implements pure stake-weighted VRF sortition.
// seed: previous block's VRF output. Returns winner address + evidence.
func (cs *ConsensusState) SelectProposerVRF(seed []byte, pubKeys map[[32]byte]ed25519.PublicKey) (*VrfProof, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return nil, fmt.Errorf("no active validators")
	}

	var totalStake uint64
	for _, v := range validators {
		totalStake += v.Stake
	}
	if totalStake == 0 {
		return nil, fmt.Errorf("total stake is zero")
	}

	best := new(big.Int) // lowest hash wins (standard sortition)
	var winner [32]byte
	var found bool
	var stakes = make(map[[32]byte]uint64, len(validators))

	for _, v := range validators {
		stakes[v.Address] = v.Stake
		pub, ok := pubKeys[v.Address]
		if !ok {
			continue // no key registered — cannot produce/verify VRF
		}
		// Reconstruct output deterministically is impossible for others;
		// in live sortition each validator evaluates their OWN VRF and
		// broadcasts a win claim. For the single-node / sim path we iterate
		// all known keys: whoever's evaluation beats their own threshold
		// with the lowest hash is the winner.
		output, _ := EvaluateVrf(ed25519.PrivateKey(nil), seed, cs.currentHeight) // placeholder, see note
		_ = output
		_ = pub
		// (see SelectProposerVRFDeterministic below for the deterministic path)
		_ = best
		_ = winner
		_ = found
		break
	}
	return nil, fmt.Errorf("use SelectProposerVRFDeterministic")
}

// SelectProposerVRFDeterministic: deterministic simulation path —
// same hash-to-stake-interval selection as before, but (a) pure stake
// (no reputation multiplier), (b) emits a VrfProof-shaped evidence block
// so downstream code and explorers already handle real VRF when it lands.
func (cs *ConsensusState) SelectProposerVRFDeterministic(seed []byte) (*VrfProof, error) {
	return cs.SelectProposerVRFAtHeight(seed, cs.currentHeight)
}

// SelectProposerVRFAtHeight selects the proposer for the block AT height
// (i.e. the block whose Header.Height == height). Produce and validate
// paths MUST pass the SAME height: produce passes bestHeight+1, validate
// passes block.Header.Height — both equal the height of the block being
// made. Any divergence (e.g. producing with cs.currentHeight=bestHeight)
// hashes a different value into the VRF seed and selects a different
// winner — mutual proposer mismatch, chain deadlock.
func (cs *ConsensusState) SelectProposerVRFAtHeight(seed []byte, height uint64) (*VrfProof, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return nil, fmt.Errorf("no active validators")
	}
	// Deterministic ordering (CRITICAL): map iteration order is random in
	// Go; without sorting, the producing node and the validating nodes can
	// assign different cumulative-stake intervals and select DIFFERENT
	// winners from the same seed — mutual proposer mismatch, chain stall.
	sort.Slice(validators, func(i, j int) bool {
		return bytes.Compare(validators[i].Address[:], validators[j].Address[:]) < 0
	})

	var total uint64
	for _, v := range validators {
		total += v.Stake
	}
	if total == 0 {
		return nil, fmt.Errorf("total stake is zero")
	}

	h := sha256.New()
	h.Write(seed)
	binary.Write(h, binary.BigEndian, height)
	digest := new(big.Int).SetBytes(h.Sum(nil))

	totalBig := new(big.Int).SetUint64(total)
	selected := new(big.Int).Mod(digest, totalBig).Uint64()

	stakes := make(map[[32]byte]uint64, len(validators))
	var cumulative uint64
	var winner [32]byte
	found := false
	for _, v := range validators {
		stakes[v.Address] = v.Stake
		cumulative += v.Stake
		if !found && selected < cumulative {
			winner = v.Address
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("selection failed")
	}

	var p VrfProof
	p.Height = height
	p.Winner = winner
	p.Stakes = stakes
	copy(p.Output[:32], h.Sum(nil)) // seed chain continuation
	return &p, nil
}
