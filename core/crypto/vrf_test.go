package crypto

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestECVRFProveAndVerify(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	verifier := NewECVRFVerifier()

	seed := []byte("QS/AV lottery seed for block 12345")

	output, proof, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	valid, err := verifier.Verify(prover.PublicKey(), seed, output, proof)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("VRF proof should be valid")
	}
}

func TestECVRFDeterministic(t *testing.T) {
	// Same (sk, seed) should always produce the same output
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	seed := []byte("deterministic test seed")

	output1, proof1, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove 1: %v", err)
	}

	output2, proof2, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove 2: %v", err)
	}

	if output1 != output2 {
		t.Fatal("VRF output should be deterministic for same (sk, seed)")
	}

	if !bytes.Equal(proof1, proof2) {
		t.Fatal("VRF proof should be deterministic for same (sk, seed)")
	}
}

func TestECVRFDifferentSeeds(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)

	output1, _, err := prover.Prove([]byte("seed1"))
	if err != nil {
		t.Fatalf("Prove 1: %v", err)
	}

	output2, _, err := prover.Prove([]byte("seed2"))
	if err != nil {
		t.Fatalf("Prove 2: %v", err)
	}

	if output1 == output2 {
		t.Fatal("different seeds should produce different outputs")
	}
}

func TestECVRFDifferentKeys(t *testing.T) {
	privKey1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey 1: %v", err)
	}
	privKey2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey 2: %v", err)
	}

	prover1 := NewECVRFProver(privKey1)
	prover2 := NewECVRFProver(privKey2)

	seed := []byte("same seed for different keys")

	output1, _, err := prover1.Prove(seed)
	if err != nil {
		t.Fatalf("Prove 1: %v", err)
	}
	output2, _, err := prover2.Prove(seed)
	if err != nil {
		t.Fatalf("Prove 2: %v", err)
	}

	if output1 == output2 {
		t.Fatal("different keys should produce different outputs for same seed")
	}
}

func TestECVRFWrongKey(t *testing.T) {
	privKey1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey 1: %v", err)
	}
	privKey2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey 2: %v", err)
	}

	prover := NewECVRFProver(privKey1)
	verifier := NewECVRFVerifier()

	seed := []byte("test seed")
	output, proof, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	// Verify with wrong public key
	wrongPub := privKey2.PubKey().SerializeCompressed()
	valid, err := verifier.Verify(wrongPub, seed, output, proof)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("VRF proof should be invalid for wrong public key")
	}
}

func TestECVRFWrongSeed(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	verifier := NewECVRFVerifier()

	output, proof, err := prover.Prove([]byte("original seed"))
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	// Verify with wrong seed
	valid, err := verifier.Verify(prover.PublicKey(), []byte("wrong seed"), output, proof)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("VRF proof should be invalid for wrong seed")
	}
}

func TestECVRFTamperedProof(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	verifier := NewECVRFVerifier()

	seed := []byte("test seed")
	output, proof, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	// Tamper with proof (flip a byte in the scalar part)
	tamperedProof := make([]byte, len(proof))
	copy(tamperedProof, proof)
	tamperedProof[80] ^= 0xFF

	valid, err := verifier.Verify(prover.PublicKey(), seed, output, tamperedProof)
	// May return error or false, both are acceptable
	if valid {
		t.Fatal("VRF proof should be invalid after tampering")
	}
}

func TestECVRFTamperedOutput(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	verifier := NewECVRFVerifier()

	seed := []byte("test seed")
	output, proof, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	// Tamper with output
	output[0] ^= 0xFF

	valid, err := verifier.Verify(prover.PublicKey(), seed, output, proof)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("VRF should detect tampered output")
	}
}

func TestECVRFFromBytes(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	keyBytes := privKey.Serialize()
	prover, err := NewECVRFProverFromBytes(keyBytes)
	if err != nil {
		t.Fatalf("NewECVRFProverFromBytes: %v", err)
	}

	verifier := NewECVRFVerifier()
	seed := []byte("from bytes test")

	output, proof, err := prover.Prove(seed)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	valid, err := verifier.Verify(prover.PublicKey(), seed, output, proof)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("VRF proof from restored key should be valid")
	}

	ZeroBytes(keyBytes)
}

func TestECVRFInvalidProofLength(t *testing.T) {
	verifier := NewECVRFVerifier()
	var output [32]byte
	_, err := verifier.Verify(make([]byte, 33), []byte("seed"), output, []byte("short"))
	if err == nil {
		t.Fatal("should reject invalid proof length")
	}
}

func TestECVRFInvalidKeyBytes(t *testing.T) {
	_, err := NewECVRFProverFromBytes([]byte("short"))
	if err == nil {
		t.Fatal("should reject invalid key length")
	}
}

func TestECVRFProofLength(t *testing.T) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	_, proof, err := prover.Prove([]byte("length test"))
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	// Proof should be 97 bytes: 33 (Gamma) + 32 (c) + 32 (s)
	if len(proof) != 97 {
		t.Fatalf("proof should be 97 bytes, got %d", len(proof))
	}
}
