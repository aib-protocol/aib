package crypto

import (
	"bytes"
	"testing"
)

func TestEd25519SignAndVerify(t *testing.T) {
	signer, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewEd25519Verifier()

	message := []byte("hello AIB 2.0 AI node")

	sig, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	valid, err := verifier.Verify(signer.PublicKey(), message, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}
}

func TestEd25519InvalidSignature(t *testing.T) {
	signer, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewEd25519Verifier()

	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Wrong message
	valid, err := verifier.Verify(signer.PublicKey(), []byte("wrong"), sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid for wrong message")
	}
}

func TestEd25519WrongKey(t *testing.T) {
	signer1, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer1.Destroy()

	signer2, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer2.Destroy()

	verifier := NewEd25519Verifier()

	sig, err := signer1.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	valid, err := verifier.Verify(signer2.PublicKey(), []byte("hello"), sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid for wrong key")
	}
}

func TestEd25519FromSeed(t *testing.T) {
	signer1, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}

	seed := signer1.Seed()
	pub1 := signer1.PublicKey()
	signer1.Destroy()

	signer2, err := NewEd25519SignerFromSeed(seed)
	if err != nil {
		t.Fatalf("NewEd25519SignerFromSeed: %v", err)
	}
	defer signer2.Destroy()

	pub2 := signer2.PublicKey()
	if !bytes.Equal(pub1, pub2) {
		t.Fatal("public keys should match after restore from seed")
	}

	// Verify sign/verify works
	verifier := NewEd25519Verifier()
	msg := []byte("seed restore test")
	sig, err := signer2.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	valid, err := verifier.Verify(pub2, msg, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid after seed restore")
	}

	ZeroBytes(seed)
}

func TestEd25519Destroy(t *testing.T) {
	signer, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}

	signer.Destroy()

	if signer.PublicKey() != nil {
		t.Fatal("PublicKey should return nil after destroy")
	}

	_, err = signer.Sign([]byte("test"))
	if err == nil {
		t.Fatal("Sign should fail after destroy")
	}
}

func TestEd25519Algorithm(t *testing.T) {
	signer, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	if signer.Algorithm() != "ed25519" {
		t.Fatalf("expected ed25519, got %s", signer.Algorithm())
	}

	verifier := NewEd25519Verifier()
	if verifier.Algorithm() != "ed25519" {
		t.Fatalf("expected ed25519, got %s", verifier.Algorithm())
	}
}

func TestEd25519InvalidSeedLength(t *testing.T) {
	_, err := NewEd25519SignerFromSeed([]byte("short"))
	if err == nil {
		t.Fatal("should reject invalid seed length")
	}
}

func TestEd25519InvalidPubKeyLength(t *testing.T) {
	verifier := NewEd25519Verifier()
	_, err := verifier.Verify([]byte("short"), []byte("msg"), make([]byte, 64))
	if err == nil {
		t.Fatal("should reject invalid public key length")
	}
}

func TestEd25519InvalidSigLength(t *testing.T) {
	verifier := NewEd25519Verifier()
	_, err := verifier.Verify(make([]byte, 32), []byte("msg"), []byte("short"))
	if err == nil {
		t.Fatal("should reject invalid signature length")
	}
}

func TestEd25519EmptyMessage(t *testing.T) {
	signer, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewEd25519Verifier()

	// Empty message should work
	sig, err := signer.Sign([]byte{})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	valid, err := verifier.Verify(signer.PublicKey(), []byte{}, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("empty message signature should be valid")
	}
}
