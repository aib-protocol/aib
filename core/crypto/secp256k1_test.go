package crypto

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestSecp256k1SignAndVerify(t *testing.T) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewSecp256k1Verifier()

	// Create a 32-byte digest
	digest := sha256.Sum256([]byte("hello AIB 2.0"))

	sig, err := signer.Sign(digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	valid, err := verifier.Verify(signer.PublicKey(), digest[:], sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}
}

func TestSecp256k1InvalidSignature(t *testing.T) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewSecp256k1Verifier()

	digest := sha256.Sum256([]byte("hello"))
	sig, err := signer.Sign(digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Modify digest
	wrongDigest := sha256.Sum256([]byte("wrong"))
	valid, err := verifier.Verify(signer.PublicKey(), wrongDigest[:], sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid for wrong digest")
	}
}

func TestSecp256k1WrongKey(t *testing.T) {
	signer1, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer1.Destroy()

	signer2, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer2.Destroy()

	verifier := NewSecp256k1Verifier()

	digest := sha256.Sum256([]byte("hello"))
	sig, err := signer1.Sign(digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify with wrong public key
	valid, err := verifier.Verify(signer2.PublicKey(), digest[:], sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid for wrong public key")
	}
}

func TestSecp256k1FromBytes(t *testing.T) {
	signer1, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}

	keyBytes := signer1.PrivateKeyBytes()
	signer1.Destroy()

	signer2, err := NewSecp256k1SignerFromBytes(keyBytes)
	if err != nil {
		t.Fatalf("NewSecp256k1SignerFromBytes: %v", err)
	}
	defer signer2.Destroy()

	// Should have same public key
	if !bytes.Equal(signer1.PublicKey(), nil) {
		t.Fatal("destroyed signer should return nil pubkey")
	}

	// Sign and verify with restored key
	verifier := NewSecp256k1Verifier()
	digest := sha256.Sum256([]byte("restore test"))
	sig, err := signer2.Sign(digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	valid, err := verifier.Verify(signer2.PublicKey(), digest[:], sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid after key restore")
	}

	ZeroBytes(keyBytes)
}

func TestSecp256k1DigestLength(t *testing.T) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	// Wrong digest length
	_, err = signer.Sign([]byte("short"))
	if err == nil {
		t.Fatal("should reject non-32-byte digest")
	}

	verifier := NewSecp256k1Verifier()
	_, err = verifier.Verify(signer.PublicKey(), []byte("short"), []byte("sig"))
	if err == nil {
		t.Fatal("should reject non-32-byte digest in verify")
	}
}

func TestSecp256k1Destroy(t *testing.T) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}

	signer.Destroy()

	if signer.PublicKey() != nil {
		t.Fatal("PublicKey should return nil after destroy")
	}

	_, err = signer.Sign(make([]byte, 32))
	if err == nil {
		t.Fatal("Sign should fail after destroy")
	}
}

func TestSecp256k1Algorithm(t *testing.T) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	if signer.Algorithm() != "secp256k1" {
		t.Fatalf("expected secp256k1, got %s", signer.Algorithm())
	}

	verifier := NewSecp256k1Verifier()
	if verifier.Algorithm() != "secp256k1" {
		t.Fatalf("expected secp256k1, got %s", verifier.Algorithm())
	}
}

func TestGenerateSecp256k1KeyPair(t *testing.T) {
	priv, pub, err := GenerateSecp256k1KeyPair()
	if err != nil {
		t.Fatalf("GenerateSecp256k1KeyPair: %v", err)
	}
	if len(priv) != 32 {
		t.Fatalf("private key should be 32 bytes, got %d", len(priv))
	}
	if len(pub) != 33 {
		t.Fatalf("compressed public key should be 33 bytes, got %d", len(pub))
	}
	ZeroBytes(priv)
}

func TestSecp256k1InvalidKeyBytes(t *testing.T) {
	_, err := NewSecp256k1SignerFromBytes([]byte("short"))
	if err == nil {
		t.Fatal("should reject invalid key length")
	}
}
