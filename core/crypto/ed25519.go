package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
)

// Ed25519Signer implements Signer using Ed25519.
// Used for AI node identity authentication.
type Ed25519Signer struct {
	privKey ed25519.PrivateKey
}

// NewEd25519Signer creates a new Ed25519 signer with a randomly generated key pair.
func NewEd25519Signer() (*Ed25519Signer, error) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ed25519: generate key: %w", err)
	}
	return &Ed25519Signer{privKey: privKey}, nil
}

// NewEd25519SignerFromSeed creates an Ed25519 signer from a 32-byte seed.
func NewEd25519SignerFromSeed(seed []byte) (*Ed25519Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("ed25519: seed must be %d bytes", ed25519.SeedSize)
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	return &Ed25519Signer{privKey: privKey}, nil
}

// Sign produces an Ed25519 signature for the given message.
// Unlike secp256k1, Ed25519 signs the message directly (not a digest).
func (s *Ed25519Signer) Sign(message []byte) ([]byte, error) {
	if s.privKey == nil {
		return nil, errors.New("ed25519: signer destroyed")
	}
	return ed25519.Sign(s.privKey, message), nil
}

// PublicKey returns the 32-byte Ed25519 public key.
func (s *Ed25519Signer) PublicKey() []byte {
	if s.privKey == nil {
		return nil
	}
	pub := s.privKey.Public().(ed25519.PublicKey)
	result := make([]byte, ed25519.PublicKeySize)
	copy(result, pub)
	return result
}

// Algorithm returns "ed25519".
func (s *Ed25519Signer) Algorithm() string {
	return "ed25519"
}

// Destroy zeros the private key material.
func (s *Ed25519Signer) Destroy() {
	if s.privKey != nil {
		ZeroBytes(s.privKey)
		s.privKey = nil
	}
}

// Seed returns a copy of the 32-byte seed. Caller is responsible for zeroing.
func (s *Ed25519Signer) Seed() []byte {
	if s.privKey == nil {
		return nil
	}
	seed := s.privKey.Seed()
	result := make([]byte, len(seed))
	copy(result, seed)
	return result
}

// Ed25519Verifier implements Verifier for Ed25519 signatures.
type Ed25519Verifier struct{}

// NewEd25519Verifier creates a new Ed25519 verifier.
func NewEd25519Verifier() *Ed25519Verifier {
	return &Ed25519Verifier{}
}

// Verify checks an Ed25519 signature against a public key and message.
func (v *Ed25519Verifier) Verify(pubKey, message, signature []byte) (bool, error) {
	if len(pubKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("ed25519: public key must be %d bytes", ed25519.PublicKeySize)
	}
	if len(signature) != ed25519.SignatureSize {
		return false, fmt.Errorf("ed25519: signature must be %d bytes", ed25519.SignatureSize)
	}
	return ed25519.Verify(ed25519.PublicKey(pubKey), message, signature), nil
}

// Algorithm returns "ed25519".
func (v *Ed25519Verifier) Algorithm() string {
	return "ed25519"
}

// Ed25519GenerateKey generates a new Ed25519 key pair.
// Returns publicKey, privateKey, error.
func Ed25519GenerateKey() (publicKey, privateKey []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("ed25519: generate key: %w", err)
	}
	return pub, priv, nil
}

// Ed25519Sign signs a message using Ed25519 private key.
func Ed25519Sign(privateKey, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

// Ed25519Verify verifies an Ed25519 signature.
func Ed25519Verify(publicKey, message, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(publicKey), message, signature)
}
