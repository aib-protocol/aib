package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// ECVRF implements a Verifiable Random Function based on secp256k1.
// Reference: RFC 9381 (ECVRF-SECP256K1-SHA256-TAI)
//
// This VRF is the mathematical foundation for QS/AV lottery fairness.
// Given a secret key and seed, it produces a deterministic but unpredictable output
// along with a proof that anyone can verify using only the public key.

// ECVRFProver generates VRF outputs and proofs.
type ECVRFProver struct {
	privKey *secp256k1.PrivateKey
}

// NewECVRFProver creates a new ECVRF prover from a secp256k1 private key.
func NewECVRFProver(privKey *secp256k1.PrivateKey) *ECVRFProver {
	return &ECVRFProver{privKey: privKey}
}

// NewECVRFProverFromBytes creates a new ECVRF prover from a 32-byte private key.
func NewECVRFProverFromBytes(keyBytes []byte) (*ECVRFProver, error) {
	if len(keyBytes) != 32 {
		return nil, errors.New("ecvrf: private key must be 32 bytes")
	}
	privKey := secp256k1.PrivKeyFromBytes(keyBytes)
	return &ECVRFProver{privKey: privKey}, nil
}

// Prove generates a VRF output and proof for the given seed.
// The output is deterministic: same (sk, seed) always produces the same output.
func (p *ECVRFProver) Prove(seed []byte) (output [32]byte, proof []byte, err error) {
	if p.privKey == nil {
		return output, nil, errors.New("ecvrf: prover has nil key")
	}

	// Step 1: Hash seed to curve point H
	H := hashToCurvePoint(p.privKey.PubKey(), seed)

	// Step 2: Gamma = sk * H
	// We need to multiply H by the private key scalar
	skBytes := p.privKey.Key.Bytes()
	var skScalar secp256k1.ModNScalar
	skScalar.SetBytes(&skBytes)

	var HJacobian, gammaJacobian secp256k1.JacobianPoint
	H.AsJacobian(&HJacobian)
	secp256k1.ScalarMultNonConst(&skScalar, &HJacobian, &gammaJacobian)
	gammaJacobian.ToAffine()
	gamma := secp256k1.NewPublicKey(&gammaJacobian.X, &gammaJacobian.Y)

	// Step 3: Generate nonce k deterministically (RFC 6979 style)
	k := generateNonce(p.privKey, seed)

	// Step 4: U = k * G (generator point)
	var kScalar secp256k1.ModNScalar
	kScalar.SetBytes(&k)
	var UJacobian secp256k1.JacobianPoint
	secp256k1.ScalarBaseMultNonConst(&kScalar, &UJacobian)
	UJacobian.ToAffine()
	U := secp256k1.NewPublicKey(&UJacobian.X, &UJacobian.Y)

	// Step 5: V = k * H
	var VJacobian secp256k1.JacobianPoint
	secp256k1.ScalarMultNonConst(&kScalar, &HJacobian, &VJacobian)
	VJacobian.ToAffine()
	V := secp256k1.NewPublicKey(&VJacobian.X, &VJacobian.Y)

	// Step 6: c = hash(G, H, pk, Gamma, U, V) mod n
	c := computeChallenge(H, p.privKey.PubKey(), gamma, U, V)

	// Step 7: s = k - c * sk mod n
	var cScalar, s secp256k1.ModNScalar
	cScalar.SetBytes(&c)
	s.Set(&kScalar)
	var csk secp256k1.ModNScalar
	csk.Mul2(&cScalar, &skScalar)
	s.Add(csk.Negate())

	// Step 8: output = SHA256(Gamma_serialized)
	gammaBytes := gamma.SerializeCompressed()
	output = sha256.Sum256(gammaBytes)

	// Encode proof: Gamma (33 bytes) || c (32 bytes) || s (32 bytes)
	proof = make([]byte, 0, 97)
	proof = append(proof, gammaBytes...)
	cBytes := c
	proof = append(proof, cBytes[:]...)
	sBytes := s.Bytes()
	proof = append(proof, sBytes[:]...)

	// Zero sensitive material
	skScalar.Zero()
	kScalar.Zero()
	s.Zero()

	return output, proof, nil
}

// PublicKey returns the compressed public key of the prover.
func (p *ECVRFProver) PublicKey() []byte {
	if p.privKey == nil {
		return nil
	}
	return p.privKey.PubKey().SerializeCompressed()
}

// ECVRFVerifier verifies VRF proofs.
type ECVRFVerifier struct{}

// NewECVRFVerifier creates a new ECVRF verifier.
func NewECVRFVerifier() *ECVRFVerifier {
	return &ECVRFVerifier{}
}

// Verify checks a VRF proof against a public key and seed.
func (v *ECVRFVerifier) Verify(pubKeyBytes, seed []byte, output [32]byte, proof []byte) (bool, error) {
	if len(proof) != 97 {
		return false, fmt.Errorf("ecvrf: proof must be 97 bytes, got %d", len(proof))
	}

	// Parse public key
	pubKey, err := secp256k1.ParsePubKey(pubKeyBytes)
	if err != nil {
		return false, fmt.Errorf("ecvrf: parse pubkey: %w", err)
	}

	// Parse proof components
	gamma, err := secp256k1.ParsePubKey(proof[:33])
	if err != nil {
		return false, fmt.Errorf("ecvrf: parse gamma: %w", err)
	}
	var cBytes [32]byte
	copy(cBytes[:], proof[33:65])
	var sBytes [32]byte
	copy(sBytes[:], proof[65:97])

	var cScalar, sScalar secp256k1.ModNScalar
	cScalar.SetBytes(&cBytes)
	sScalar.SetBytes(&sBytes)

	// Recompute H = hash_to_curve(pk, seed)
	H := hashToCurvePoint(pubKey, seed)

	// Recompute U = s*G + c*pk
	var sG, cPK, UJacobian secp256k1.JacobianPoint
	secp256k1.ScalarBaseMultNonConst(&sScalar, &sG)
	var pkJacobian secp256k1.JacobianPoint
	pubKey.AsJacobian(&pkJacobian)
	secp256k1.ScalarMultNonConst(&cScalar, &pkJacobian, &cPK)
	secp256k1.AddNonConst(&sG, &cPK, &UJacobian)
	UJacobian.ToAffine()
	U := secp256k1.NewPublicKey(&UJacobian.X, &UJacobian.Y)

	// Recompute V = s*H + c*Gamma
	var sH, cGamma, VJacobian secp256k1.JacobianPoint
	var HJacobian, gammaJacobian secp256k1.JacobianPoint
	H.AsJacobian(&HJacobian)
	gamma.AsJacobian(&gammaJacobian)
	secp256k1.ScalarMultNonConst(&sScalar, &HJacobian, &sH)
	secp256k1.ScalarMultNonConst(&cScalar, &gammaJacobian, &cGamma)
	secp256k1.AddNonConst(&sH, &cGamma, &VJacobian)
	VJacobian.ToAffine()
	V := secp256k1.NewPublicKey(&VJacobian.X, &VJacobian.Y)

	// Recompute challenge
	cPrime := computeChallenge(H, pubKey, gamma, U, V)

	// Verify c == c'
	if cBytes != cPrime {
		return false, nil
	}

	// Verify output == SHA256(Gamma)
	expectedOutput := sha256.Sum256(gamma.SerializeCompressed())
	if output != expectedOutput {
		return false, nil
	}

	return true, nil
}

// hashToCurvePoint hashes a public key and seed to a secp256k1 curve point.
// Uses the "try-and-increment" method.
func hashToCurvePoint(pubKey *secp256k1.PublicKey, seed []byte) *secp256k1.PublicKey {
	pkBytes := pubKey.SerializeCompressed()

	for ctr := uint32(0); ctr < 256; ctr++ {
		h := sha256.New()
		h.Write([]byte("AIB_VRF_H2C_"))
		h.Write(pkBytes)
		h.Write(seed)
		h.Write([]byte{byte(ctr >> 24), byte(ctr >> 16), byte(ctr >> 8), byte(ctr)})
		hash := h.Sum(nil)

		// Try to parse as compressed point (prefix 0x02)
		candidate := make([]byte, 33)
		candidate[0] = 0x02
		copy(candidate[1:], hash)

		point, err := secp256k1.ParsePubKey(candidate)
		if err == nil {
			return point
		}
	}

	// Should never happen in practice with 256 tries
	panic("ecvrf: failed to hash to curve point after 256 attempts")
}

// generateNonce generates a deterministic nonce for VRF proof generation.
// Uses HMAC-SHA256 similar to RFC 6979.
func generateNonce(privKey *secp256k1.PrivateKey, seed []byte) [32]byte {
	skBytes := privKey.Serialize()
	defer ZeroBytes(skBytes)

	h := hmac.New(sha256.New, skBytes)
	h.Write([]byte("AIB_VRF_NONCE_"))
	h.Write(seed)
	hash := h.Sum(nil)

	var nonce [32]byte
	copy(nonce[:], hash)

	// Ensure nonce is in valid range (< curve order)
	curveOrder, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	n := new(big.Int).SetBytes(nonce[:])
	n.Mod(n, curveOrder)
	if n.Sign() == 0 {
		n.SetInt64(1)
	}
	nBytes := n.Bytes()
	var result [32]byte
	copy(result[32-len(nBytes):], nBytes)
	return result
}

// computeChallenge computes the Fiat-Shamir challenge hash.
func computeChallenge(H, pubKey, gamma, U, V *secp256k1.PublicKey) [32]byte {
	h := sha256.New()
	h.Write([]byte("AIB_VRF_CHALLENGE_"))
	h.Write(H.SerializeCompressed())
	h.Write(pubKey.SerializeCompressed())
	h.Write(gamma.SerializeCompressed())
	h.Write(U.SerializeCompressed())
	h.Write(V.SerializeCompressed())
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
