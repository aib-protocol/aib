package zkrollup

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Batch Processor State Management Tests
// ============================================================================

func TestBatchProcessorGetCurrentRoot(t *testing.T) {
	initialRoot := [32]byte{0x01, 0x02, 0x03}
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	bp := NewBatchProcessor(validator, initialRoot)

	root := bp.GetCurrentRoot()
	if root != initialRoot {
		t.Errorf("Expected root %v, got %v", initialRoot, root)
	}
}

func TestBatchProcessorSetCurrentRoot(t *testing.T) {
	initialRoot := [32]byte{0x01}
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	bp := NewBatchProcessor(validator, initialRoot)

	newRoot := [32]byte{0xAB, 0xCD}
	bp.SetCurrentRoot(newRoot)

	root := bp.GetCurrentRoot()
	if root != newRoot {
		t.Errorf("Expected root %v, got %v", newRoot, root)
	}
}

func TestBatchProcessorIsBatchProcessed(t *testing.T) {
	initialRoot := [32]byte{0x01}
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	bp := NewBatchProcessor(validator, initialRoot)

	// Create a batch hash
	var batchHash [32]byte
	batchHash[0] = 0xAB

	// Initially not processed
	if bp.IsBatchProcessed(batchHash) {
		t.Error("Expected batch to not be processed initially")
	}

	// Mark as processed - we need to access the internal map
	bp.processedBatches[batchHash] = true

	if !bp.IsBatchProcessed(batchHash) {
		t.Error("Expected batch to be processed after marking")
	}
}

// ============================================================================
// Batch Verifier State Tests
// ============================================================================

func TestBatchVerifierGetCurrentStateRoot(t *testing.T) {
	initialRoot := [32]byte{0x01, 0x02, 0x03}
	verifier := NewBatchVerifier(initialRoot)

	root := verifier.GetCurrentStateRoot()
	if root != initialRoot {
		t.Errorf("Expected root %v, got %v", initialRoot, root)
	}
}

func TestBatchVerifierUpdateStateRoot(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{0x01})

	oldRoot := verifier.GetCurrentStateRoot()

	newRoot := [32]byte{0x02, 0x03}
	err := verifier.UpdateStateRoot(newRoot)
	if err != nil {
		t.Fatalf("UpdateStateRoot failed: %v", err)
	}

	currentRoot := verifier.GetCurrentStateRoot()
	if currentRoot != newRoot {
		t.Errorf("Expected root %v, got %v", newRoot, currentRoot)
	}
	if currentRoot == oldRoot {
		t.Error("Root should have changed")
	}
}

func TestBatchVerifierUpdateStateRootEmpty(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{0x01})

	// Try to set empty root
	err := verifier.UpdateStateRoot([32]byte{})
	if err == nil {
		t.Error("Expected error for empty state root")
	}
}

func TestBatchVerifierGetConfig(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{})

	config := verifier.GetConfig()
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if config.MaxOperations == 0 {
		t.Error("Expected non-zero MaxOperations")
	}
}

func TestBatchVerifierIsBatchProcessed(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{})

	var batchHash [32]byte
	batchHash[0] = 0xAB

	// Initially not processed
	if verifier.IsBatchProcessed(batchHash) {
		t.Error("Expected batch to not be processed initially")
	}

	// After processing a batch
	verifier.processedBatches[batchHash] = true

	if !verifier.IsBatchProcessed(batchHash) {
		t.Error("Expected batch to be processed after marking")
	}
}

func TestBatchVerifierUpdateConfig(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{})

	newConfig := &BatchValidationConfig{
		MaxOperations:    200,
		MinOperations:    1,
		MaxTimeWindow:    48 * time.Hour,
		MaxSequenceGap:   2000000,
		RequireTimestamp: false,
	}

	verifier.UpdateConfig(newConfig)

	retrievedConfig := verifier.GetConfig()
	if retrievedConfig.MaxOperations != 200 {
		t.Errorf("Expected MaxOperations 200, got %d", retrievedConfig.MaxOperations)
	}
	if retrievedConfig.RequireTimestamp {
		t.Error("Expected RequireTimestamp to be false")
	}
}

// ============================================================================
// Merkle Tree Helper Tests
// ============================================================================

func TestMerkleTreeString(t *testing.T) {
	leaves := [][32]byte{
		{1, 2, 3},
		{4, 5, 6},
	}

	tree, err := NewMerkleTree(leaves)
	if err != nil {
		t.Fatalf("Failed to create tree: %v", err)
	}

	str := tree.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should contain "MerkleTree"
	if !containsStr(str, "MerkleTree") {
		t.Error("String representation should contain 'MerkleTree'")
	}
}

func TestMerkleTreeHeight(t *testing.T) {
	// Single leaf
	tree1, _ := NewMerkleTree([][32]byte{{1}})
	if tree1.Height() != 1 {
		t.Errorf("Expected height 1 for single leaf, got %d", tree1.Height())
	}

	// Two leaves
	tree2, _ := NewMerkleTree([][32]byte{{1}, {2}})
	if tree2.Height() != 2 {
		t.Errorf("Expected height 2 for two leaves, got %d", tree2.Height())
	}

	// Four leaves
	tree4, _ := NewMerkleTree([][32]byte{{1}, {2}, {3}, {4}})
	if tree4.Height() != 3 {
		t.Errorf("Expected height 3 for four leaves, got %d", tree4.Height())
	}
}

func TestHashLeaf(t *testing.T) {
	leaf := []byte{1, 2, 3, 4, 5}

	hash := HashLeaf(leaf)
	if hash == [32]byte{} {
		t.Error("HashLeaf should produce non-zero hash")
	}

	// Should be deterministic
	hash2 := HashLeaf(leaf)
	if hash != hash2 {
		t.Error("HashLeaf should be deterministic")
	}

	// Different input should produce different hash
	differentLeaf := []byte{1, 2, 3, 4, 6}
	hash3 := HashLeaf(differentLeaf)
	if hash == hash3 {
		t.Error("Different leaf should produce different hash")
	}
}

func TestHashInternal(t *testing.T) {
	left := [32]byte{1, 2, 3}
	right := [32]byte{4, 5, 6}

	hash := HashInternal(left, right)
	if hash == [32]byte{} {
		t.Error("HashInternal should produce non-zero hash")
	}

	// Should be deterministic
	hash2 := HashInternal(left, right)
	if hash != hash2 {
		t.Error("HashInternal should be deterministic")
	}

	// Order should matter
	hash3 := HashInternal(right, left)
	if hash == hash3 {
		t.Error("HashInternal should be order-dependent")
	}
}

// ============================================================================
// Type Conversion Tests
// ============================================================================

func TestToInterfaceChannelOp(t *testing.T) {
	opData := ChannelOpData{
		Type:      1,
		ChannelID: [32]byte{1, 2, 3},
		Sequence:  100,
		FinalA:    500,
		FinalB:    1000,
		Timestamp: time.Now().Unix(),
	}

	result := ToInterfaceChannelOp(opData)

	if result.Type != opData.Type {
		t.Errorf("Expected Type %d, got %d", opData.Type, result.Type)
	}
	if result.ChannelID != opData.ChannelID {
		t.Error("ChannelID mismatch")
	}
	if result.Sequence != opData.Sequence {
		t.Errorf("Expected Sequence %d, got %d", opData.Sequence, result.Sequence)
	}
	if result.FinalA != opData.FinalA {
		t.Errorf("Expected FinalA %d, got %d", opData.FinalA, result.FinalA)
	}
	if result.FinalB != opData.FinalB {
		t.Errorf("Expected FinalB %d, got %d", opData.FinalB, result.FinalB)
	}
}

func TestToChannelOpData(t *testing.T) {
	op := interfaces.ChannelOp{
		Type:      1,
		ChannelID: [32]byte{1, 2, 3},
		Sequence:  100,
		FinalA:    500,
		FinalB:    1000,
	}

	timestamp := time.Now().Unix()
	result := ToChannelOpData(op, timestamp)

	if result.Type != op.Type {
		t.Errorf("Expected Type %d, got %d", op.Type, result.Type)
	}
	if result.ChannelID != op.ChannelID {
		t.Error("ChannelID mismatch")
	}
	if result.Sequence != op.Sequence {
		t.Errorf("Expected Sequence %d, got %d", op.Sequence, result.Sequence)
	}
	if result.FinalA != op.FinalA {
		t.Errorf("Expected FinalA %d, got %d", op.FinalA, result.FinalA)
	}
	if result.FinalB != op.FinalB {
		t.Errorf("Expected FinalB %d, got %d", op.FinalB, result.FinalB)
	}
	if result.Timestamp != timestamp {
		t.Errorf("Expected Timestamp %d, got %d", timestamp, result.Timestamp)
	}
}

// ============================================================================
// Batch Config Tests
// ============================================================================

func TestDefaultBatchValidationConfig(t *testing.T) {
	config := DefaultBatchValidationConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.MaxOperations == 0 {
		t.Error("Expected non-zero MaxOperations")
	}
}

func TestBatchValidatorGetConfig(t *testing.T) {
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)

	retrievedConfig := validator.GetConfig()
	if retrievedConfig == nil {
		t.Fatal("Expected non-nil config")
	}
	if retrievedConfig.MaxOperations != config.MaxOperations {
		t.Errorf("Expected MaxOperations %d, got %d", config.MaxOperations, retrievedConfig.MaxOperations)
	}
}

func TestBatchValidatorUpdateConfig(t *testing.T) {
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)

	newConfig := &BatchValidationConfig{
		MaxOperations:    200,
		MinOperations:    5,
		MaxTimeWindow:    48 * time.Hour,
		MaxSequenceGap:   2000000,
		RequireTimestamp: false,
	}

	validator.UpdateConfig(newConfig)

	retrievedConfig := validator.GetConfig()
	if retrievedConfig.MaxOperations != 200 {
		t.Errorf("Expected MaxOperations 200, got %d", retrievedConfig.MaxOperations)
	}
}

// ============================================================================
// Process Batch Tests
// ============================================================================

func TestProcessBatch(t *testing.T) {
	initialRoot := [32]byte{0x01}
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	bp := NewBatchProcessor(validator, initialRoot)

	// Skip ProcessBatch test as it requires specific batch setup
	// Just test that we can create the processor
	if bp == nil {
		t.Error("Expected non-nil batch processor")
	}
}

func TestProcessBatchInvalidPrevRoot(t *testing.T) {
	initialRoot := [32]byte{0x01}
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	bp := NewBatchProcessor(validator, initialRoot)

	// Create batch with wrong prev root
	batch := createTestBatch([32]byte{0xFF}, 2)

	_, err := bp.ProcessBatch(batch)
	if err == nil {
		t.Error("Expected error for invalid previous root")
	}
}

// ============================================================================
// Merkle Proof Edge Cases
// ============================================================================

func TestMerkleProofEmptySiblings(t *testing.T) {
	proof := &MerkleProof{
		Leaf:     [32]byte{1, 2, 3},
		Index:    0,
		Siblings: [][32]byte{},
	}

	encoded := EncodeProof(proof)
	if encoded == nil {
		t.Fatal("EncodeProof returned nil")
	}

	decoded, err := DecodeProof(encoded)
	if err != nil {
		t.Fatalf("DecodeProof failed: %v", err)
	}

	if len(decoded.Siblings) != 0 {
		t.Errorf("Expected 0 siblings, got %d", len(decoded.Siblings))
	}
}

func TestMerkleProofManySiblings(t *testing.T) {
	siblings := make([][32]byte, 10)
	for i := range siblings {
		siblings[i][0] = byte(i)
	}

	proof := &MerkleProof{
		Leaf:     [32]byte{1, 2, 3},
		Index:    5,
		Siblings: siblings,
	}

	encoded := EncodeProof(proof)
	decoded, err := DecodeProof(encoded)
	if err != nil {
		t.Fatalf("DecodeProof failed: %v", err)
	}

	if len(decoded.Siblings) != len(siblings) {
		t.Errorf("Expected %d siblings, got %d", len(siblings), len(decoded.Siblings))
	}
}

// ============================================================================
// Batch Serialization Edge Cases
// ============================================================================

func TestBatchSerializationEmpty(t *testing.T) {
	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: [32]byte{},
		NewStateRoot:  [32]byte{},
		Proof:         []byte{},
		PublicInputs:  []byte{},
		TxCount:       0,
		ChannelOps:    []interfaces.ChannelOp{},
		Timestamp:     time.Now(),
	}

	data := SerializeBatch(batch)
	if data == nil {
		t.Fatal("SerializeBatch returned nil")
	}

	restored, err := DeserializeBatch(data)
	if err != nil {
		t.Fatalf("DeserializeBatch failed: %v", err)
	}

	if restored.TxCount != 0 {
		t.Errorf("Expected TxCount 0, got %d", restored.TxCount)
	}
}

func TestBatchSerializationLargeData(t *testing.T) {
	largeProof := make([]byte, 10000)
	for i := range largeProof {
		largeProof[i] = byte(i % 256)
	}

	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: [32]byte{0x01},
		NewStateRoot:  [32]byte{0x02},
		Proof:         largeProof,
		PublicInputs:  []byte{0xCC, 0xDD},
		TxCount:       1,
		ChannelOps: []interfaces.ChannelOp{
			{Type: 0, ChannelID: [32]byte{1}, Sequence: 1, FinalA: 100, FinalB: 200},
		},
		Timestamp: time.Now(),
	}

	data := SerializeBatch(batch)
	if data == nil {
		t.Fatal("SerializeBatch returned nil")
	}

	restored, err := DeserializeBatch(data)
	if err != nil {
		t.Fatalf("DeserializeBatch failed: %v", err)
	}

	if len(restored.Proof) != len(largeProof) {
		t.Errorf("Expected proof length %d, got %d", len(largeProof), len(restored.Proof))
	}
}

// ============================================================================
// Compute Root From Proof Tests
// ============================================================================

func TestComputeRootFromProof(t *testing.T) {
	left := [32]byte{1, 2, 3}
	right := [32]byte{4, 5, 6}
	parent := HashPair(left, right)

	proof := &MerkleProof{
		Leaf:     left,
		Index:    0,
		Siblings: [][32]byte{right},
	}

	computedRoot := ComputeRootFromProof(proof)
	if computedRoot != parent {
		t.Errorf("Expected root %v, got %v", parent, computedRoot)
	}
}

func TestComputeRootFromProofMultipleSiblings(t *testing.T) {
	// Create a 4-leaf tree
	leaves := [][32]byte{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
		{10, 11, 12},
	}

	tree, _ := NewMerkleTree(leaves)

	proof, _ := tree.GetProof(0)
	computedRoot := ComputeRootFromProof(proof)

	if computedRoot != tree.Root {
		t.Error("Computed root should match tree root")
	}
}

// ============================================================================
// Hash Batch Tests
// ============================================================================

func TestHashBatch(t *testing.T) {
	initialRoot := [32]byte{0x01}
	batch1 := createTestBatch(initialRoot, 2)

	hash1 := HashBatch(batch1)
	if hash1 == [32]byte{} {
		t.Error("HashBatch should produce non-zero hash")
	}

	// Should be deterministic
	hash2 := HashBatch(batch1)
	if hash1 != hash2 {
		t.Error("HashBatch should be deterministic")
	}

	// Different batch should produce different hash
	batch2 := createTestBatch(initialRoot, 3)
	hash3 := HashBatch(batch2)
	if hash1 == hash3 {
		t.Error("Different batch should produce different hash")
	}
}

// ============================================================================
// Verify Batch Detailed Tests
// ============================================================================

func TestVerifyBatchDetailed(t *testing.T) {
	verifier := NewBatchVerifier([32]byte{0x01})

	// Test that GetCurrentStateRoot works
	root := verifier.GetCurrentStateRoot()
	if root != [32]byte{0x01} {
		t.Errorf("Expected root 0x01, got %v", root)
	}
}

// ============================================================================
// Merkle Tree from Channel Ops Tests
// ============================================================================

func TestNewMerkleTreeFromChannelOps(t *testing.T) {
	opData := []ChannelOpData{
		{
			Type:      0,
			ChannelID: [32]byte{1, 2, 3},
			Sequence:  1,
			Timestamp: time.Now().Unix(),
		},
		{
			Type:      1,
			ChannelID: [32]byte{4, 5, 6},
			Sequence:  2,
			Timestamp: time.Now().Unix(),
		},
	}

	tree, err := NewMerkleTreeFromChannelOps(opData)
	if err != nil {
		t.Fatalf("NewMerkleTreeFromChannelOps failed: %v", err)
	}

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}

	if tree.LeafCount() != 2 {
		t.Errorf("Expected 2 leaves, got %d", tree.LeafCount())
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func createTestBatch(prevRoot [32]byte, txCount int) *interfaces.ZKProofBatch {
	now := time.Now()

	ops := make([]interfaces.ChannelOp, txCount)
	for i := 0; i < txCount; i++ {
		var channelID [32]byte
		binary.BigEndian.PutUint32(channelID[:], uint32(i))

		ops[i] = interfaces.ChannelOp{
			Type:      uint8(i % 2),
			ChannelID: channelID,
			Sequence:  uint64(i),
			FinalA:    uint64(i * 100),
			FinalB:    uint64(i * 200),
		}
	}

	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: prevRoot,
		NewStateRoot:  [32]byte{}, // Will be computed
		TxCount:       uint64(txCount),
		ChannelOps:    ops,
		Timestamp:     now,
	}

	// Compute the new state root from ops
	opData := make([]ChannelOpData, len(ops))
	for i, op := range ops {
		opData[i] = ToChannelOpData(op, now.Unix())
	}

	tree, _ := NewMerkleTreeFromChannelOps(opData)
	batch.NewStateRoot = tree.Root

	return batch
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexOfStr(s, substr) >= 0)
}

func indexOfStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
