package fingerprint

import (
	"testing"

	"github.com/aib-protocol/aib/core/crypto"
)

func TestGenerateModelFingerprint(t *testing.T) {
	// Create test weights
	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
		{Layer: 0, NeuronIn: 0, NeuronOut: 1, Value: -0.456},
		{Layer: 1, NeuronIn: 0, NeuronOut: 0, Value: 0.789},
	}

	// Generate keypair
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	nodeID := pubKey

	// Test: valid fingerprint generation
	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", nodeID, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	if fp == nil {
		t.Fatal("Generated fingerprint is nil")
	}

	if len(fp.WeightsHash) == 0 {
		t.Error("Weights hash is empty")
	}

	if fp.Architecture != "llama2-7b" {
		t.Errorf("Architecture mismatch: got %s, want llama2-7b", fp.Architecture)
	}

	if fp.Version != "1.0.0" {
		t.Errorf("Version mismatch: got %s, want 1.0.0", fp.Version)
	}

	if len(fp.Signature) == 0 {
		t.Error("Signature is empty")
	}
}

func TestGenerateModelFingerprintErrors(t *testing.T) {
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	// Test: empty weights
	_, err = GenerateModelFingerprint([]ModelWeight{}, "llama2-7b", "1.0.0", pubKey, privKey)
	if err == nil {
		t.Error("Expected error for empty weights")
	}

	// Test: empty architecture
	_, err = GenerateModelFingerprint([]ModelWeight{{Value: 0.123}}, "", "1.0.0", pubKey, privKey)
	if err == nil {
		t.Error("Expected error for empty architecture")
	}

	// Test: empty version
	_, err = GenerateModelFingerprint([]ModelWeight{{Value: 0.123}}, "llama2-7b", "", pubKey, privKey)
	if err == nil {
		t.Error("Expected error for empty version")
	}

	// Test: empty node ID
	_, err = GenerateModelFingerprint([]ModelWeight{{Value: 0.123}}, "llama2-7b", "1.0.0", nil, privKey)
	if err == nil {
		t.Error("Expected error for empty node ID")
	}
}

func TestVerifyFingerprint(t *testing.T) {
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
	}

	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", pubKey, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	// Test: valid verification
	valid, err := VerifyFingerprint(fp, pubKey)
	if err != nil {
		t.Fatalf("Verification failed: %v", err)
	}
	if !valid {
		t.Error("Expected valid fingerprint")
	}

	// Test: invalid with wrong public key
	wrongPubKey, _, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}
	valid, err = VerifyFingerprint(fp, wrongPubKey)
	if err != nil || valid {
		t.Error("Expected invalid fingerprint with wrong public key")
	}

	// Test: nil fingerprint
	valid, err = VerifyFingerprint(nil, pubKey)
	if err == nil || valid {
		t.Error("Expected error for nil fingerprint")
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	pubKey1, privKey1, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	pubKey2, privKey2, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights1 := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
	}

	weights2 := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.456},
	}

	fp1, err := GenerateModelFingerprint(weights1, "llama2-7b", "1.0.0", pubKey1, privKey1)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	fp2, err := GenerateModelFingerprint(weights2, "gpt4-13b", "2.0.0", pubKey2, privKey2)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	// Test: register first fingerprint
	err = registry.Register(fp1, pubKey1)
	if err != nil {
		t.Fatalf("Failed to register fingerprint: %v", err)
	}

	if registry.Size() != 1 {
		t.Errorf("Registry size mismatch: got %d, want 1", registry.Size())
	}

	// Test: register second fingerprint
	err = registry.Register(fp2, pubKey2)
	if err != nil {
		t.Fatalf("Failed to register second fingerprint: %v", err)
	}

	if registry.Size() != 2 {
		t.Errorf("Registry size mismatch: got %d, want 2", registry.Size())
	}

	// Test: duplicate registration should fail
	err = registry.Register(fp1, pubKey1)
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Test: has fingerprint
	if !registry.HasFingerprint(fp1.WeightsHash) {
		t.Error("Expected fingerprint to be found")
	}

	if registry.HasFingerprint([]byte("nonexistent")) {
		t.Error("Expected false for non-existent fingerprint")
	}

	// Test: get fingerprint
	retrieved := registry.GetFingerprint(fp1.WeightsHash)
	if retrieved == nil {
		t.Error("Failed to retrieve fingerprint")
	}

	if !bytesEqual(retrieved.WeightsHash, fp1.WeightsHash) {
		t.Error("Retrieved fingerprint hash mismatch")
	}

	// Test: get node models
	models := registry.GetNodeModels(pubKey1)
	if len(models) != 1 {
		t.Errorf("Expected 1 model for node, got %d", len(models))
	}

	if models[0].Architecture != "llama2-7b" {
		t.Error("Retrieved model architecture mismatch")
	}
}

func TestRegistryBan(t *testing.T) {
	registry := NewRegistry()

	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
	}

	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", pubKey, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	// Register fingerprint
	err = registry.Register(fp, pubKey)
	if err != nil {
		t.Fatalf("Failed to register fingerprint: %v", err)
	}

	// Ban node
	registry.BanNode(pubKey)

	// Check if node is banned
	if !registry.IsBanned(pubKey) {
		t.Error("Expected node to be banned")
	}

	// Check if fingerprint was removed
	if registry.HasFingerprint(fp.WeightsHash) {
		t.Error("Expected fingerprint to be removed after ban")
	}

	// Check if node models were removed
	models := registry.GetNodeModels(pubKey)
	if len(models) != 0 {
		t.Error("Expected node models to be empty after ban")
	}
}

func TestVerificationRequest(t *testing.T) {
	// Create a verification request
	fingerprintHash := []byte("test_fingerprint_hash")
	challenge := []byte("random_challenge_data")

	req := NewVerificationRequest(fingerprintHash, challenge, 300) // 5 minute TTL

	if req.FingerprintHash == nil {
		t.Error("Fingerprint hash is nil")
	}

	if req.ExpiresAt <= req.Timestamp {
		t.Error("ExpiresAt should be after Timestamp")
	}

	if req.IsExpired() {
		t.Error("Request should not be expired")
	}
}

func TestCreateModelCommitment(t *testing.T) {
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
		{Layer: 0, NeuronIn: 0, NeuronOut: 1, Value: -0.456},
	}

	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", pubKey, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	commitment, err := CreateModelCommitment(fp, weights, pubKey)
	if err != nil {
		t.Fatalf("Failed to create commitment: %v", err)
	}

	if commitment == nil {
		t.Fatal("Commitment is nil")
	}

	if commitment.Fingerprint != fp {
		t.Error("Fingerprint mismatch in commitment")
	}

	if len(commitment.MerkleProof) == 0 {
		t.Error("Merkle proof is empty")
	}

	// Verify commitment
	valid, err := VerifyModelCommitment(commitment, fp.WeightsHash)
	if err != nil {
		t.Fatalf("Failed to verify commitment: %v", err)
	}
	if !valid {
		t.Error("Expected valid commitment")
	}

	// Test invalid commitment (wrong hash)
	valid, err = VerifyModelCommitment(commitment, []byte("wrong_hash"))
	if err == nil || valid {
		t.Error("Expected invalid commitment with wrong hash")
	}
}

func TestHashFingerprint(t *testing.T) {
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
	}

	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", pubKey, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	hash, err := HashFingerprint(fp)
	if err != nil {
		t.Fatalf("Failed to hash fingerprint: %v", err)
	}

	if len(hash) != 32 {
		t.Errorf("Expected 32-byte hash, got %d", len(hash))
	}

	// Same fingerprint should produce same hash
	hash2, err := HashFingerprint(fp)
	if err != nil {
		t.Fatalf("Failed to hash fingerprint: %v", err)
	}

	if !bytesEqual(hash, hash2) {
		t.Error("Same fingerprint should produce same hash")
	}
}

func TestRegistryExportImport(t *testing.T) {
	registry := NewRegistry()

	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	weights := []ModelWeight{
		{Layer: 0, NeuronIn: 0, NeuronOut: 0, Value: 0.123},
	}

	fp, err := GenerateModelFingerprint(weights, "llama2-7b", "1.0.0", pubKey, privKey)
	if err != nil {
		t.Fatalf("Failed to generate fingerprint: %v", err)
	}

	err = registry.Register(fp, pubKey)
	if err != nil {
		t.Fatalf("Failed to register fingerprint: %v", err)
	}

	// Export
	data, err := registry.Export()
	if err != nil {
		t.Fatalf("Failed to export registry: %v", err)
	}

	if len(data) == 0 {
		t.Error("Exported data is empty")
	}

	// Import into new registry
	newRegistry := NewRegistry()
	err = newRegistry.Import(data)
	if err != nil {
		t.Fatalf("Failed to import registry: %v", err)
	}

	// Verify imported data
	if newRegistry.Size() != 1 {
		t.Errorf("Imported registry size mismatch: got %d, want 1", newRegistry.Size())
	}

	retrieved := newRegistry.GetFingerprint(fp.WeightsHash)
	if retrieved == nil {
		t.Error("Failed to retrieve fingerprint from imported registry")
	}

	if retrieved.Architecture != "llama2-7b" {
		t.Error("Imported fingerprint architecture mismatch")
	}
}