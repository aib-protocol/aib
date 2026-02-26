package crypto

import (
	"crypto/sha256"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func BenchmarkHash256(b *testing.B) {
	data := []byte("benchmark data for SHA-256d hashing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Hash256(data)
	}
}

func BenchmarkHashBLAKE3(b *testing.B) {
	data := []byte("benchmark data for BLAKE3 hashing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashBLAKE3(data)
	}
}

func BenchmarkSingleSHA256(b *testing.B) {
	data := []byte("benchmark data for single SHA-256")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SingleSHA256(data)
	}
}

func BenchmarkSecp256k1Sign(b *testing.B) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		b.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	digest := sha256.Sum256([]byte("benchmark sign data"))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := signer.Sign(digest[:])
		if err != nil {
			b.Fatalf("Sign: %v", err)
		}
	}
}

func BenchmarkSecp256k1Verify(b *testing.B) {
	signer, err := NewSecp256k1Signer()
	if err != nil {
		b.Fatalf("NewSecp256k1Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewSecp256k1Verifier()
	digest := sha256.Sum256([]byte("benchmark verify data"))
	sig, err := signer.Sign(digest[:])
	if err != nil {
		b.Fatalf("Sign: %v", err)
	}

	pubKey := signer.PublicKey()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(pubKey, digest[:], sig)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

func BenchmarkEd25519Sign(b *testing.B) {
	signer, err := NewEd25519Signer()
	if err != nil {
		b.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	message := []byte("benchmark ed25519 sign data")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("Sign: %v", err)
		}
	}
}

func BenchmarkEd25519Verify(b *testing.B) {
	signer, err := NewEd25519Signer()
	if err != nil {
		b.Fatalf("NewEd25519Signer: %v", err)
	}
	defer signer.Destroy()

	verifier := NewEd25519Verifier()
	message := []byte("benchmark ed25519 verify data")
	sig, err := signer.Sign(message)
	if err != nil {
		b.Fatalf("Sign: %v", err)
	}

	pubKey := signer.PublicKey()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(pubKey, message, sig)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

func BenchmarkECVRFProve(b *testing.B) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		b.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	seed := []byte("benchmark VRF seed")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := prover.Prove(seed)
		if err != nil {
			b.Fatalf("Prove: %v", err)
		}
	}
}

func BenchmarkECVRFVerify(b *testing.B) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		b.Fatalf("GeneratePrivateKey: %v", err)
	}

	prover := NewECVRFProver(privKey)
	verifier := NewECVRFVerifier()
	seed := []byte("benchmark VRF verify seed")

	output, proof, err := prover.Prove(seed)
	if err != nil {
		b.Fatalf("Prove: %v", err)
	}

	pubKey := prover.PublicKey()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(pubKey, seed, output, proof)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

func BenchmarkMerkleTreeBuild(b *testing.B) {
	hasher := NewSHA256d()
	leaves := make([][]byte, 1024)
	for i := range leaves {
		h := sha256.Sum256([]byte{byte(i >> 8), byte(i)})
		leaves[i] = h[:]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewStandardMerkleTree(hasher, leaves)
	}
}

func BenchmarkMerkleProofVerify(b *testing.B) {
	hasher := NewSHA256d()
	leaves := make([][]byte, 1024)
	for i := range leaves {
		h := sha256.Sum256([]byte{byte(i >> 8), byte(i)})
		leaves[i] = h[:]
	}

	tree, _ := NewStandardMerkleTree(hasher, leaves)
	root := tree.Root()
	proof, _ := tree.Proof(512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyMerkleProof(hasher, leaves[512], root, proof, 512)
	}
}
