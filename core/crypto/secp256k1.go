package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// Secp256k1Signer implements Signer using secp256k1 ECDSA.
type Secp256k1Signer struct {
	privKey *secp256k1.PrivateKey
}

// NewSecp256k1Signer creates a new signer with a randomly generated secp256k1 key pair.
func NewSecp256k1Signer() (*Secp256k1Signer, error) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("secp256k1: generate key: %w", err)
	}
	return &Secp256k1Signer{privKey: privKey}, nil
}

// NewSecp256k1SignerFromBytes creates a signer from a 32-byte private key.
func NewSecp256k1SignerFromBytes(keyBytes []byte) (*Secp256k1Signer, error) {
	if len(keyBytes) != 32 {
		return nil, errors.New("secp256k1: private key must be 32 bytes")
	}
	privKey := secp256k1.PrivKeyFromBytes(keyBytes)
	return &Secp256k1Signer{privKey: privKey}, nil
}

// Sign produces an ECDSA signature for the given 32-byte digest.
func (s *Secp256k1Signer) Sign(digest []byte) ([]byte, error) {
	if s.privKey == nil {
		return nil, errors.New("secp256k1: signer destroyed")
	}
	if len(digest) != 32 {
		return nil, errors.New("secp256k1: digest must be 32 bytes")
	}
	sig := ecdsa.Sign(s.privKey, digest)
	return sig.Serialize(), nil
}

// PublicKey returns the compressed 33-byte public key.
func (s *Secp256k1Signer) PublicKey() []byte {
	if s.privKey == nil {
		return nil
	}
	return s.privKey.PubKey().SerializeCompressed()
}

// Algorithm returns "secp256k1".
func (s *Secp256k1Signer) Algorithm() string {
	return "secp256k1"
}

// Destroy zeros the private key material.
func (s *Secp256k1Signer) Destroy() {
	if s.privKey != nil {
		s.privKey.Zero()
		s.privKey = nil
	}
}

// PrivateKeyBytes returns a copy of the 32-byte private key. Caller is responsible for zeroing.
func (s *Secp256k1Signer) PrivateKeyBytes() []byte {
	if s.privKey == nil {
		return nil
	}
	b := s.privKey.Serialize()
	result := make([]byte, len(b))
	copy(result, b)
	return result
}

// Secp256k1Verifier implements Verifier for secp256k1 ECDSA signatures.
type Secp256k1Verifier struct{}

// NewSecp256k1Verifier creates a new secp256k1 verifier.
func NewSecp256k1Verifier() *Secp256k1Verifier {
	return &Secp256k1Verifier{}
}

// Verify checks a DER-encoded ECDSA signature against a compressed public key and 32-byte digest.
func (v *Secp256k1Verifier) Verify(pubKey, digest, signature []byte) (bool, error) {
	if len(digest) != 32 {
		return false, errors.New("secp256k1: digest must be 32 bytes")
	}
	pk, err := secp256k1.ParsePubKey(pubKey)
	if err != nil {
		return false, fmt.Errorf("secp256k1: parse pubkey: %w", err)
	}
	sig, err := ecdsa.ParseDERSignature(signature)
	if err != nil {
		return false, fmt.Errorf("secp256k1: parse signature: %w", err)
	}
	return sig.Verify(digest, pk), nil
}

// Algorithm returns "secp256k1".
func (v *Secp256k1Verifier) Algorithm() string {
	return "secp256k1"
}

// GenerateSecp256k1KeyPair generates a random secp256k1 key pair.
// Returns (privateKey, compressedPublicKey, error).
func GenerateSecp256k1KeyPair() ([]byte, []byte, error) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("secp256k1: generate key: %w", err)
	}
	privBytes := privKey.Serialize()
	pubBytes := privKey.PubKey().SerializeCompressed()

	// Zero the original private key
	privKey.Zero()

	return privBytes, pubBytes, nil
}

// ZeroBytes overwrites a byte slice with zeros.
func ZeroBytes(b []byte) {
	// Use crypto/rand to read random bytes first, then zero
	// This provides some defense against compiler optimization
	rand.Read(b)
	for i := range b {
		b[i] = 0
	}
}
