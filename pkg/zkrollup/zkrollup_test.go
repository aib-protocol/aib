package zkrollup

import (
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

func TestMerkleTree(t *testing.T) {
	// Test basic Merkle tree construction
	leaves := [][32]byte{
		{1, 2, 3, 4},
		{5, 6, 7, 8},
		{9, 10, 11, 12},
		{13, 14, 15, 16},
	}

	tree, err := NewMerkleTree(leaves)
	if err != nil {
		t.Fatalf("Failed to create Merkle tree: %v", err)
	}

	if tree.LeafCount() != 4 {
		t.Errorf("Expected 4 leaves, got %d", tree.LeafCount())
	}

	// Test proof generation and verification
	proof, err := tree.GetProof(0)
	if err != nil {
		t.Fatalf("Failed to get proof: %v", err)
	}

	if !tree.VerifyProof(proof) {
		t.Error("Proof verification failed")
	}

	// Test standalone verification
	if !VerifyProofStandalone(proof.Leaf, proof) {
		t.Error("Standalone proof verification failed")
	}
}

func TestMerkleTreeFromChannelOps(t *testing.T) {
	ops := []ChannelOpData{
		{Type: 0, ChannelID: [32]byte{1}, Sequence: 1, FinalA: 100, FinalB: 100},
		{Type: 1, ChannelID: [32]byte{2}, Sequence: 2, FinalA: 200, FinalB: 200},
	}

	tree, err := NewMerkleTreeFromChannelOps(ops)
	if err != nil {
		t.Fatalf("Failed to create tree from ops: %v", err)
	}

	if tree.LeafCount() != 2 {
		t.Errorf("Expected 2 leaves, got %d", tree.LeafCount())
	}
}

func TestBatchVerifier(t *testing.T) {
	initialRoot := [32]byte{0x01}
	verifier := NewBatchVerifier(initialRoot)

	// Create a simple batch
	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: initialRoot,
		NewStateRoot:  [32]byte{0x02},
		TxCount:       2,
		ChannelOps: []interfaces.ChannelOp{
			{Type: 0, ChannelID: [32]byte{1}, Sequence: 0, FinalA: 100, FinalB: 100},
			{Type: 1, ChannelID: [32]byte{2}, Sequence: 1, FinalA: 200, FinalB: 200},
		},
		Timestamp: time.Now(),
	}

	// Build Merkle tree to get valid root
	ops := []ChannelOpData{
		ToChannelOpData(batch.ChannelOps[0], batch.Timestamp.Unix()),
		ToChannelOpData(batch.ChannelOps[1], batch.Timestamp.Unix()),
	}
	tree, err := NewMerkleTreeFromChannelOps(ops)
	if err != nil {
		t.Fatalf("Failed to create tree: %v", err)
	}
	batch.NewStateRoot = tree.Root

	// Verify batch
	valid, err := verifier.VerifyBatch(batch)
	if err != nil {
		t.Fatalf("Batch verification failed: %v", err)
	}
	if !valid {
		t.Error("Expected batch to be valid")
	}

	// Check state root was updated
	if verifier.GetCurrentStateRoot() != batch.NewStateRoot {
		t.Error("State root was not updated")
	}
}

func TestBatchValidator(t *testing.T) {
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)

	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: [32]byte{0x01},
		NewStateRoot:  [32]byte{0x02},
		TxCount:       1,
		ChannelOps: []interfaces.ChannelOp{
			{Type: 0, ChannelID: [32]byte{1}, Sequence: 0, FinalA: 100, FinalB: 100},
		},
		Timestamp: time.Now(),
	}

	// Build tree for valid root
	ops := []ChannelOpData{
		ToChannelOpData(batch.ChannelOps[0], batch.Timestamp.Unix()),
	}
	tree, _ := NewMerkleTreeFromChannelOps(ops)
	batch.NewStateRoot = tree.Root

	err := validator.ValidateBatch(batch)
	if err != nil {
		t.Errorf("Expected batch to be valid, got error: %v", err)
	}
}

func TestBatchSerialization(t *testing.T) {
	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: [32]byte{0x01, 0x02},
		NewStateRoot:  [32]byte{0x03, 0x04},
		Proof:         []byte{0xAA, 0xBB},
		PublicInputs:  []byte{0xCC, 0xDD},
		TxCount:       1,
		ChannelOps: []interfaces.ChannelOp{
			{Type: 0, ChannelID: [32]byte{1}, Sequence: 1, FinalA: 100, FinalB: 200},
		},
		Timestamp: time.Now(),
	}

	// Serialize
	data := SerializeBatch(batch)
	if data == nil {
		t.Fatal("SerializeBatch returned nil")
	}

	// Deserialize
	restored, err := DeserializeBatch(data)
	if err != nil {
		t.Fatalf("DeserializeBatch failed: %v", err)
	}

	// Verify
	if restored.TxCount != batch.TxCount {
		t.Errorf("TxCount mismatch: expected %d, got %d", batch.TxCount, restored.TxCount)
	}
	if restored.Timestamp.Unix() != batch.Timestamp.Unix() {
		t.Errorf("Timestamp mismatch")
	}
	if len(restored.ChannelOps) != len(batch.ChannelOps) {
		t.Errorf("ChannelOps length mismatch")
	}
}

func TestProofEncoding(t *testing.T) {
	proof := &MerkleProof{
		Leaf:     [32]byte{1, 2, 3},
		Index:    0,
		Siblings: [][32]byte{{4, 5, 6}, {7, 8, 9}},
		Root:     [32]byte{10, 11, 12},
	}

	encoded := EncodeProof(proof)
	if encoded == nil {
		t.Fatal("EncodeProof returned nil")
	}

	decoded, err := DecodeProof(encoded)
	if err != nil {
		t.Fatalf("DecodeProof failed: %v", err)
	}

	if decoded.Index != proof.Index {
		t.Errorf("Index mismatch: expected %d, got %d", proof.Index, decoded.Index)
	}
	if decoded.Leaf != proof.Leaf {
		t.Error("Leaf mismatch")
	}
	if decoded.Root != proof.Root {
		t.Error("Root mismatch")
	}
	if len(decoded.Siblings) != len(proof.Siblings) {
		t.Error("Siblings length mismatch")
	}
}

func TestHashPair(t *testing.T) {
	left := [32]byte{1, 2, 3}
	right := [32]byte{4, 5, 6}

	hash1 := HashPair(left, right)
	hash2 := HashPair(left, right)

	if hash1 != hash2 {
		t.Error("HashPair should be deterministic")
	}

	// Different order should produce different hash
	hash3 := HashPair(right, left)
	if hash1 == hash3 {
		t.Error("HashPair should be order-dependent")
	}
}

func TestEmptyTreeRoot(t *testing.T) {
	root := EmptyTreeRoot()
	emptyRoot := [32]byte{}

	if root == emptyRoot {
		t.Error("Empty tree root should not be all zeros")
	}
}

func TestBatchHash(t *testing.T) {
	batch := &interfaces.ZKProofBatch{
		PrevStateRoot: [32]byte{0x01},
		NewStateRoot:  [32]byte{0x02},
		TxCount:       1,
		ChannelOps: []interfaces.ChannelOp{
			{Type: 0, ChannelID: [32]byte{1}, Sequence: 1, FinalA: 100, FinalB: 100},
		},
		Timestamp: time.Now(),
	}

	hash1 := HashBatch(batch)
	hash2 := HashBatch(batch)

	if hash1 != hash2 {
		t.Error("HashBatch should be deterministic")
	}

	// Different batch should produce different hash
	batch.TxCount = 2
	hash3 := HashBatch(batch)
	if hash1 == hash3 {
		t.Error("Different batches should have different hashes")
	}
}

func TestMerkleRootFromLeaves(t *testing.T) {
	leaves := [][32]byte{
		{1, 2, 3},
		{4, 5, 6},
	}

	root, err := MerkleRootFromLeaves(leaves)
	if err != nil {
		t.Fatalf("MerkleRootFromLeaves failed: %v", err)
	}

	// Should match tree root
	tree, _ := NewMerkleTree(leaves)
	if root != tree.Root {
		t.Error("Root from MerkleRootFromLeaves should match tree root")
	}
}

func TestVerifyMerklePath(t *testing.T) {
	left := [32]byte{1, 2, 3}
	right := [32]byte{4, 5, 6}
	parent := HashPair(left, right)

	if !VerifyMerklePath(left, right, parent) {
		t.Error("VerifyMerklePath should return true for valid path")
	}

	wrongParent := [32]byte{9, 9, 9}
	if VerifyMerklePath(left, right, wrongParent) {
		t.Error("VerifyMerklePath should return false for invalid path")
	}
}

func BenchmarkHashPair(b *testing.B) {
	left := [32]byte{1, 2, 3}
	right := [32]byte{4, 5, 6}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashPair(left, right)
	}
}

func BenchmarkMerkleTreeBuild(b *testing.B) {
	leaves := make([][32]byte, 100)
	for i := range leaves {
		leaves[i] = [32]byte{byte(i)}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewMerkleTree(leaves)
	}
}
