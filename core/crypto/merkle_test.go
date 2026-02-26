package crypto

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMerkleTreeSingleLeaf(t *testing.T) {
	hasher := NewSHA256d()
	tree, err := NewStandardMerkleTree(hasher, [][]byte{[]byte("single leaf")})
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	if root == nil {
		t.Fatal("root should not be nil")
	}

	expected := hasher.Hash([]byte("single leaf"))
	if !bytes.Equal(root, expected) {
		t.Fatal("single leaf root should equal hash of leaf")
	}
}

func TestMerkleTreeTwoLeaves(t *testing.T) {
	hasher := NewSHA256d()
	leaves := [][]byte{[]byte("leaf0"), []byte("leaf1")}
	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	if root == nil {
		t.Fatal("root should not be nil")
	}

	// Manually compute expected root
	h0 := hasher.Hash([]byte("leaf0"))
	h1 := hasher.Hash([]byte("leaf1"))
	combined := append(h0, h1...)
	expectedRoot := hasher.Hash(combined)

	if !bytes.Equal(root, expectedRoot) {
		t.Fatal("two-leaf root mismatch")
	}
}

func TestMerkleTreeProofAndVerify(t *testing.T) {
	hasher := NewSHA256d()
	leaves := [][]byte{
		[]byte("tx0"),
		[]byte("tx1"),
		[]byte("tx2"),
		[]byte("tx3"),
	}

	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()

	for i, leaf := range leaves {
		proof, err := tree.Proof(i)
		if err != nil {
			t.Fatalf("Proof(%d): %v", i, err)
		}

		valid := tree.Verify(leaf, root, proof, i)
		if !valid {
			t.Fatalf("Merkle proof for leaf %d should be valid", i)
		}
	}
}

func TestMerkleTreeOddLeaves(t *testing.T) {
	hasher := NewSHA256d()
	leaves := [][]byte{
		[]byte("tx0"),
		[]byte("tx1"),
		[]byte("tx2"),
	}

	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	if root == nil {
		t.Fatal("root should not be nil for odd leaf count")
	}

	// Verify all proofs
	for i, leaf := range leaves {
		proof, err := tree.Proof(i)
		if err != nil {
			t.Fatalf("Proof(%d): %v", i, err)
		}

		valid := tree.Verify(leaf, root, proof, i)
		if !valid {
			t.Fatalf("Merkle proof for leaf %d (odd tree) should be valid", i)
		}
	}
}

func TestMerkleTreeWrongLeaf(t *testing.T) {
	hasher := NewSHA256d()
	leaves := [][]byte{[]byte("tx0"), []byte("tx1"), []byte("tx2"), []byte("tx3")}

	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	proof, err := tree.Proof(0)
	if err != nil {
		t.Fatalf("Proof: %v", err)
	}

	// Wrong leaf should fail
	valid := tree.Verify([]byte("wrong"), root, proof, 0)
	if valid {
		t.Fatal("proof should be invalid for wrong leaf")
	}
}

func TestMerkleTreeProofOutOfRange(t *testing.T) {
	hasher := NewSHA256d()
	tree, err := NewStandardMerkleTree(hasher, [][]byte{[]byte("leaf")})
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	_, err = tree.Proof(-1)
	if err == nil {
		t.Fatal("should reject negative index")
	}

	_, err = tree.Proof(1)
	if err == nil {
		t.Fatal("should reject out-of-range index")
	}
}

func TestMerkleTreeEmpty(t *testing.T) {
	hasher := NewSHA256d()
	_, err := NewStandardMerkleTree(hasher, [][]byte{})
	if err == nil {
		t.Fatal("should reject empty leaves")
	}
}

func TestMerkleTreeLarger(t *testing.T) {
	hasher := NewSHA256d()
	leaves := make([][]byte, 16)
	for i := range leaves {
		leaves[i] = []byte(fmt.Sprintf("transaction_%d", i))
	}

	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	for i, leaf := range leaves {
		proof, err := tree.Proof(i)
		if err != nil {
			t.Fatalf("Proof(%d): %v", i, err)
		}
		valid := tree.Verify(leaf, root, proof, i)
		if !valid {
			t.Fatalf("proof for leaf %d in 16-leaf tree should be valid", i)
		}
	}
}

func TestVerifyMerkleProofStandalone(t *testing.T) {
	hasher := NewSHA256d()
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}

	tree, err := NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		t.Fatalf("NewStandardMerkleTree: %v", err)
	}

	root := tree.Root()
	proof, err := tree.Proof(2)
	if err != nil {
		t.Fatalf("Proof: %v", err)
	}

	// Use standalone verification
	valid := VerifyMerkleProof(hasher, []byte("c"), root, proof, 2)
	if !valid {
		t.Fatal("standalone Merkle proof verification should work")
	}
}

func TestSparseMerkleTreeBasic(t *testing.T) {
	hasher := NewSHA256d()
	smt := NewSparseMerkleTree(hasher, 8) // small depth for testing

	emptyRoot := smt.Root()
	if emptyRoot == nil {
		t.Fatal("empty root should not be nil")
	}

	// Insert a value
	smt.Update([]byte("key1"), []byte("value1"))

	newRoot := smt.Root()
	if bytes.Equal(emptyRoot, newRoot) {
		t.Fatal("root should change after insert")
	}

	// Get the value hash
	valHash := smt.Get([]byte("key1"))
	expectedHash := hasher.Hash([]byte("value1"))
	if !bytes.Equal(valHash, expectedHash) {
		t.Fatal("Get should return hash of value")
	}
}

func TestSparseMerkleTreeNonExistentKey(t *testing.T) {
	hasher := NewSHA256d()
	smt := NewSparseMerkleTree(hasher, 8)

	smt.Update([]byte("key1"), []byte("value1"))

	// Non-existent key should return default hash
	val := smt.Get([]byte("nonexistent"))
	defaultHash := hasher.Hash(nil)
	if !bytes.Equal(val, defaultHash) {
		t.Fatal("non-existent key should return default hash")
	}
}

func TestSparseMerkleTreeMultipleKeys(t *testing.T) {
	hasher := NewSHA256d()
	smt := NewSparseMerkleTree(hasher, 8)

	smt.Update([]byte("key1"), []byte("value1"))
	root1 := make([]byte, len(smt.Root()))
	copy(root1, smt.Root())

	smt.Update([]byte("key2"), []byte("value2"))
	root2 := smt.Root()

	if bytes.Equal(root1, root2) {
		t.Fatal("root should change when adding second key")
	}
}

func TestSparseMerkleTreeUpdateSameKey(t *testing.T) {
	hasher := NewSHA256d()
	smt := NewSparseMerkleTree(hasher, 8)

	smt.Update([]byte("key1"), []byte("value1"))
	root1 := make([]byte, len(smt.Root()))
	copy(root1, smt.Root())

	smt.Update([]byte("key1"), []byte("value2"))
	root2 := smt.Root()

	if bytes.Equal(root1, root2) {
		t.Fatal("root should change when updating value")
	}
}
