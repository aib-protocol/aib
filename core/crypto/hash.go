package crypto

import (
	"crypto/sha256"

	"github.com/zeebo/blake3"
)

// SHA256d implements Hasher using double SHA-256 (Bitcoin compatible).
type SHA256d struct{}

// NewSHA256d creates a new SHA-256d hasher.
func NewSHA256d() *SHA256d {
	return &SHA256d{}
}

// Hash computes SHA-256(SHA-256(data)), compatible with Bitcoin's hash function.
func (h *SHA256d) Hash(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// Algorithm returns "sha256d".
func (h *SHA256d) Algorithm() string {
	return "sha256d"
}

// Size returns 32 (bytes).
func (h *SHA256d) Size() int {
	return 32
}

// Hash256 is a convenience function for double SHA-256.
func Hash256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

// BLAKE3Hasher implements Hasher using BLAKE3.
type BLAKE3Hasher struct{}

// NewBLAKE3 creates a new BLAKE3 hasher.
func NewBLAKE3() *BLAKE3Hasher {
	return &BLAKE3Hasher{}
}

// Hash computes BLAKE3(data).
func (h *BLAKE3Hasher) Hash(data []byte) []byte {
	hash := blake3.Sum256(data)
	return hash[:]
}

// Algorithm returns "blake3".
func (h *BLAKE3Hasher) Algorithm() string {
	return "blake3"
}

// Size returns 32 (bytes).
func (h *BLAKE3Hasher) Size() int {
	return 32
}

// HashBLAKE3 is a convenience function for BLAKE3 hashing.
func HashBLAKE3(data []byte) [32]byte {
	return blake3.Sum256(data)
}

// SingleSHA256 computes a single SHA-256 hash.
func SingleSHA256(data []byte) [32]byte {
	return sha256.Sum256(data)
}
