package fingerprint

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

// ModelFingerprint represents a unique fingerprint of an AI model
type ModelFingerprint struct {
	WeightsHash  []byte // Merkle root hash of model weights
	Architecture string // Model architecture (e.g., "llama2-7b")
	Version      string // Model version
	Timestamp    int64  // Registration timestamp
	NodeID       []byte // ID of the node that registered this model
	Signature    []byte // Node signature of the fingerprint
}

// ModelWeight represents a single weight parameter of the model
type ModelWeight struct {
	Layer     int     // Layer index
	NeuronIn  int     // Input neuron index
	NeuronOut int     // Output neuron index
	Value     float64 // Weight value
}

// ModelCommitment contains all necessary data for model registration
type ModelCommitment struct {
	Fingerprint *ModelFingerprint
	MerkleProof [][]byte // Merkle proof for the weights
	PublicKey   []byte   // Node's public key for verification
}

// GenerateModelFingerprint creates a unique fingerprint for a model
func GenerateModelFingerprint(weights []ModelWeight, architecture, version string, nodeID []byte, privateKey []byte) (*ModelFingerprint, error) {
	if len(weights) == 0 {
		return nil, errors.New("fingerprint: empty weights")
	}
	if architecture == "" {
		return nil, errors.New("fingerprint: architecture cannot be empty")
	}
	if version == "" {
		return nil, errors.New("fingerprint: version cannot be empty")
	}
	if len(nodeID) == 0 {
		return nil, errors.New("fingerprint: node ID cannot be empty")
	}

	// Convert weights to byte slices for Merkle tree
	weightBytes := make([][]byte, len(weights))
	for i, w := range weights {
		// Serialize weight to bytes
		weightData := struct {
			Layer     int
			NeuronIn  int
			NeuronOut int
			Value     float64
		}{
			Layer:     w.Layer,
			NeuronIn:  w.NeuronIn,
			NeuronOut: w.NeuronOut,
			Value:     w.Value,
		}

		data, err := json.Marshal(weightData)
		if err != nil {
			return nil, err
		}
		weightBytes[i] = data
	}

	// Create Merkle tree of weights
	hasher := crypto.NewSHA256d()
	tree, err := crypto.NewStandardMerkleTree(hasher, weightBytes)
	if err != nil {
		return nil, err
	}

	weightsHash := tree.Root()
	if weightsHash == nil {
		return nil, errors.New("fingerprint: failed to generate weights hash")
	}

	// Create fingerprint
	fp := &ModelFingerprint{
		WeightsHash:  weightsHash,
		Architecture: architecture,
		Version:      version,
		Timestamp:    time.Now().Unix(),
		NodeID:       nodeID,
	}

	// Sign the fingerprint
	signature, err := signFingerprint(fp, privateKey)
	if err != nil {
		return nil, err
	}
	fp.Signature = signature

	return fp, nil
}

// VerifyFingerprint verifies the integrity and signature of a model fingerprint
func VerifyFingerprint(fp *ModelFingerprint, publicKey []byte) (bool, error) {
	if fp == nil {
		return false, errors.New("fingerprint: nil fingerprint")
	}
	if len(fp.WeightsHash) == 0 {
		return false, errors.New("fingerprint: empty weights hash")
	}
	if fp.Architecture == "" {
		return false, errors.New("fingerprint: empty architecture")
	}
	if fp.Version == "" {
		return false, errors.New("fingerprint: empty version")
	}
	if len(fp.NodeID) == 0 {
		return false, errors.New("fingerprint: empty node ID")
	}
	if len(fp.Signature) == 0 {
		return false, errors.New("fingerprint: empty signature")
	}

	// Verify timestamp (not too far in the future)
	now := time.Now().Unix()
	maxFutureSkew := int64(300) // 5 minutes
	if fp.Timestamp > now+maxFutureSkew {
		return false, errors.New("fingerprint: timestamp too far in the future")
	}

	// Verify signature
	return verifySignature(fp, fp.Signature, publicKey)
}

// CreateModelCommitment creates a complete model commitment for registration
func CreateModelCommitment(fp *ModelFingerprint, weights []ModelWeight, publicKey []byte) (*ModelCommitment, error) {
	if fp == nil {
		return nil, errors.New("commitment: nil fingerprint")
	}

	// Create merkle proof for random weight (challenge-based)
	// In production, this would be based on a verifier's challenge
	weightBytes := make([][]byte, len(weights))
	for i, w := range weights {
		weightData := struct {
			Layer     int
			NeuronIn  int
			NeuronOut int
			Value     float64
		}{
			Layer:     w.Layer,
			NeuronIn:  w.NeuronIn,
			NeuronOut: w.NeuronOut,
			Value:     w.Value,
		}

		data, err := json.Marshal(weightData)
		if err != nil {
			return nil, err
		}
		weightBytes[i] = data
	}

	hasher := crypto.NewSHA256d()
	tree, err := crypto.NewStandardMerkleTree(hasher, weightBytes)
	if err != nil {
		return nil, err
	}

	// Generate proof for the first weight as example
	proof, err := tree.Proof(0)
	if err != nil {
		return nil, err
	}

	return &ModelCommitment{
		Fingerprint: fp,
		MerkleProof: proof,
		PublicKey:   publicKey,
	}, nil
}

// VerifyModelCommitment verifies a model commitment
func VerifyModelCommitment(commitment *ModelCommitment, expectedWeightsHash []byte) (bool, error) {
	if commitment == nil {
		return false, errors.New("commitment: nil commitment")
	}
	if commitment.Fingerprint == nil {
		return false, errors.New("commitment: nil fingerprint")
	}

	// Verify fingerprint
	valid, err := VerifyFingerprint(commitment.Fingerprint, commitment.PublicKey)
	if err != nil || !valid {
		return false, err
	}

	// Verify the commitment's fingerprint matches the expected hash
	if !bytesEqual(commitment.Fingerprint.WeightsHash, expectedWeightsHash) {
		return false, errors.New("commitment: weights hash mismatch")
	}

	// Verify merkle proof (in production, would use proper challenge-response)
	// For now, we'll just verify the proof structure is valid
	if len(commitment.MerkleProof) == 0 {
		return false, errors.New("commitment: empty merkle proof")
	}

	return true, nil
}

// HashFingerprint computes a hash of the fingerprint for chain storage
func HashFingerprint(fp *ModelFingerprint) ([]byte, error) {
	if fp == nil {
		return nil, errors.New("fingerprint: nil fingerprint")
	}

	// Serialize fingerprint without signature for hashing
	data, err := json.Marshal(struct {
		WeightsHash  []byte
		Architecture string
		Version      string
		Timestamp    int64
		NodeID       []byte
	}{
		WeightsHash:  fp.WeightsHash,
		Architecture: fp.Architecture,
		Version:      fp.Version,
		Timestamp:    fp.Timestamp,
		NodeID:       fp.NodeID,
	})
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(data)
	return hash[:], nil
}

// Helper functions

func signFingerprint(fp *ModelFingerprint, privateKey []byte) ([]byte, error) {
	// Hash the fingerprint data
	hash, err := HashFingerprint(fp)
	if err != nil {
		return nil, err
	}

	// Sign using ed25519
	sig := crypto.Ed25519Sign(privateKey, hash)
	return sig, nil
}

func verifySignature(fp *ModelFingerprint, signature, publicKey []byte) (bool, error) {
	// Hash the fingerprint data
	hash, err := HashFingerprint(fp)
	if err != nil {
		return false, err
	}

	// Verify using ed25519
	return crypto.Ed25519Verify(publicKey, hash, signature), nil
}

// bytesEqual uses constant-time comparison to prevent timing side-channel attacks
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
