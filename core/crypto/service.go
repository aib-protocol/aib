// Package crypto provides cryptographic primitives for the AIB 2.0 protocol.
//
// MODULE_CORE_01: This is the foundation module upon which all other modules depend.
// It provides signing (secp256k1, Ed25519), hashing (SHA-256d, BLAKE3),
// VRF (ECVRF based on secp256k1), Merkle trees, and ZK proof verification interfaces.
package crypto

// Signer provides digital signature operations.
type Signer interface {
	// Sign produces a signature for the given message digest.
	Sign(digest []byte) ([]byte, error)

	// PublicKey returns the serialized public key.
	PublicKey() []byte

	// Algorithm returns the algorithm name (e.g., "secp256k1", "ed25519").
	Algorithm() string

	// Destroy zeros the private key material.
	Destroy()
}

// Verifier verifies digital signatures.
type Verifier interface {
	// Verify checks a signature against a message digest and public key.
	Verify(pubKey, digest, signature []byte) (bool, error)

	// Algorithm returns the algorithm name.
	Algorithm() string
}

// Hasher provides hash functions.
type Hasher interface {
	// Hash computes the hash of the given data.
	Hash(data []byte) []byte

	// Algorithm returns the hash algorithm name (e.g., "sha256d", "blake3").
	Algorithm() string

	// Size returns the hash output size in bytes.
	Size() int
}

// VRFProver generates verifiable random function outputs.
type VRFProver interface {
	// Prove generates a VRF output and proof for the given seed.
	Prove(seed []byte) (output [32]byte, proof []byte, err error)

	// PublicKey returns the VRF public key.
	PublicKey() []byte
}

// VRFVerifier verifies VRF proofs.
type VRFVerifier interface {
	// Verify checks a VRF proof and returns whether it is valid.
	Verify(pubKey, seed []byte, output [32]byte, proof []byte) (bool, error)
}

// MerkleTree provides Merkle tree operations.
type MerkleTree interface {
	// Root returns the Merkle root hash.
	Root() []byte

	// Proof generates a Merkle proof for the leaf at the given index.
	Proof(index int) ([][]byte, error)

	// Verify checks a Merkle proof for a given leaf and root.
	Verify(leaf, root []byte, proof [][]byte, index int) bool
}

// ZKVerifier verifies zero-knowledge proofs.
// Concrete implementations are provided in MODULE_CONSENSUS_03.
type ZKVerifier interface {
	// Verify checks a zero-knowledge proof against the given public inputs.
	Verify(proof *ZKProof) (bool, error)

	// ProofType returns the ZK proof system name (e.g., "groth16", "plonk").
	ProofType() string
}

// ZKProof represents a zero-knowledge proof with its public inputs.
type ZKProof struct {
	// ProofData is the serialized proof.
	ProofData []byte

	// PublicInputs are the public inputs to the proof.
	PublicInputs [][]byte

	// VerificationKey identifies which verification key to use.
	VerificationKey []byte

	// ProofType indicates the proof system (e.g., "groth16", "plonk").
	ProofType string
}
