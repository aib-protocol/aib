package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSHA256dBasic(t *testing.T) {
	hasher := NewSHA256d()

	result := hasher.Hash([]byte("hello"))
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}

	// Verify double hashing manually
	first := sha256.Sum256([]byte("hello"))
	second := sha256.Sum256(first[:])
	if !bytes.Equal(result, second[:]) {
		t.Fatal("SHA256d result doesn't match manual double hash")
	}
}

func TestSHA256dBitcoinCompatibility(t *testing.T) {
	// Bitcoin uses SHA-256d for block hashes.
	// Test with known empty input.
	hasher := NewSHA256d()

	// SHA256d("") should be:
	// SHA256(SHA256("")) = SHA256(e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855)
	emptyHash := hasher.Hash([]byte(""))

	first := sha256.Sum256([]byte(""))
	expectedFirst := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hex.EncodeToString(first[:]) != expectedFirst {
		t.Fatalf("first SHA256 mismatch")
	}

	second := sha256.Sum256(first[:])
	if !bytes.Equal(emptyHash, second[:]) {
		t.Fatal("SHA256d empty string mismatch")
	}
}

func TestSHA256dDeterministic(t *testing.T) {
	hasher := NewSHA256d()
	data := []byte("deterministic test data")

	result1 := hasher.Hash(data)
	result2 := hasher.Hash(data)

	if !bytes.Equal(result1, result2) {
		t.Fatal("SHA256d should be deterministic")
	}
}

func TestHash256Convenience(t *testing.T) {
	hasher := NewSHA256d()
	data := []byte("convenience function test")

	result1 := hasher.Hash(data)
	result2 := Hash256(data)

	if !bytes.Equal(result1, result2[:]) {
		t.Fatal("Hash256 should match SHA256d.Hash")
	}
}

func TestSHA256dMetadata(t *testing.T) {
	hasher := NewSHA256d()
	if hasher.Algorithm() != "sha256d" {
		t.Fatalf("expected sha256d, got %s", hasher.Algorithm())
	}
	if hasher.Size() != 32 {
		t.Fatalf("expected 32, got %d", hasher.Size())
	}
}

func TestBLAKE3Basic(t *testing.T) {
	hasher := NewBLAKE3()

	result := hasher.Hash([]byte("hello"))
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}
}

func TestBLAKE3Deterministic(t *testing.T) {
	hasher := NewBLAKE3()
	data := []byte("deterministic BLAKE3 test")

	result1 := hasher.Hash(data)
	result2 := hasher.Hash(data)

	if !bytes.Equal(result1, result2) {
		t.Fatal("BLAKE3 should be deterministic")
	}
}

func TestBLAKE3DifferentInputs(t *testing.T) {
	hasher := NewBLAKE3()

	result1 := hasher.Hash([]byte("input1"))
	result2 := hasher.Hash([]byte("input2"))

	if bytes.Equal(result1, result2) {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestHashBLAKE3Convenience(t *testing.T) {
	hasher := NewBLAKE3()
	data := []byte("convenience function test")

	result1 := hasher.Hash(data)
	result2 := HashBLAKE3(data)

	if !bytes.Equal(result1, result2[:]) {
		t.Fatal("HashBLAKE3 should match BLAKE3Hasher.Hash")
	}
}

func TestBLAKE3Metadata(t *testing.T) {
	hasher := NewBLAKE3()
	if hasher.Algorithm() != "blake3" {
		t.Fatalf("expected blake3, got %s", hasher.Algorithm())
	}
	if hasher.Size() != 32 {
		t.Fatalf("expected 32, got %d", hasher.Size())
	}
}

func TestSingleSHA256(t *testing.T) {
	data := []byte("single hash test")
	result := SingleSHA256(data)
	expected := sha256.Sum256(data)
	if result != expected {
		t.Fatal("SingleSHA256 mismatch")
	}
}

func TestSHA256dDifferentFromSingle(t *testing.T) {
	data := []byte("double vs single")
	single := SingleSHA256(data)
	double := Hash256(data)
	if single == double {
		t.Fatal("double hash should differ from single hash")
	}
}

func TestBLAKE3EmptyInput(t *testing.T) {
	hasher := NewBLAKE3()
	result := hasher.Hash([]byte{})
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes for empty input, got %d", len(result))
	}
}
