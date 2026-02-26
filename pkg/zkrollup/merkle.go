package zkrollup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Common errors for Merkle tree operations.
var (
	ErrEmptyTree     = errors.New("merkle tree is empty")
	ErrInvalidIndex  = errors.New("invalid leaf index")
	ErrLevelMismatch = errors.New("merkle level mismatch")
	ErrInvalidProof  = errors.New("invalid merkle proof")
	ErrRootMismatch  = errors.New("merkle root mismatch")
	ErrInvalidLeaf   = errors.New("invalid leaf data")
	ErrTreeNotBuilt  = errors.New("merkle tree not built")
)

// NewMerkleTree creates a new Merkle tree from a slice of byte arrays.
func NewMerkleTree(leaves [][32]byte) (*MerkleTree, error) {
	if len(leaves) == 0 {
		return nil, ErrEmptyTree
	}

	tree := &MerkleTree{
		Leaves: leaves,
		Levels: make([][][32]byte, 0),
	}

	if err := tree.Build(); err != nil {
		return nil, fmt.Errorf("failed to build merkle tree: %w", err)
	}

	return tree, nil
}

// NewMerkleTreeFromChannelOps creates a Merkle tree from channel operations.
func NewMerkleTreeFromChannelOps(ops []ChannelOpData) (*MerkleTree, error) {
	if len(ops) == 0 {
		return nil, ErrEmptyTree
	}

	leaves := make([][32]byte, len(ops))
	for i, op := range ops {
		leaves[i] = HashChannelOp(op)
	}

	return NewMerkleTree(leaves)
}

// Build constructs the Merkle tree levels from leaves to root.
func (mt *MerkleTree) Build() error {
	if len(mt.Leaves) == 0 {
		return ErrEmptyTree
	}

	// Clear any existing levels
	mt.Levels = make([][][32]byte, 0)

	// Start with leaves as the first level
	currentLevel := make([][32]byte, len(mt.Leaves))
	copy(currentLevel, mt.Leaves)
	mt.Levels = append(mt.Levels, currentLevel)

	// Build up the tree
	for len(currentLevel) > 1 {
		nextLevel := mt.buildNextLevel(currentLevel)
		mt.Levels = append(mt.Levels, nextLevel)
		currentLevel = nextLevel
	}

	// The root is the single element in the last level
	mt.Root = currentLevel[0]
	return nil
}

// buildNextLevel computes the next level up in the Merkle tree.
func (mt *MerkleTree) buildNextLevel(level [][32]byte) [][32]byte {
	levelLen := len(level)
	if levelLen == 0 {
		return nil
	}

	// If odd number of elements, duplicate the last one
	if levelLen%2 == 1 {
		level = append(level, level[levelLen-1])
		levelLen++
	}

	nextLevel := make([][32]byte, levelLen/2)
	for i := 0; i < levelLen; i += 2 {
		nextLevel[i/2] = HashPair(level[i], level[i+1])
	}

	return nextLevel
}

// GetProof generates a Merkle proof for the leaf at the given index.
func (mt *MerkleTree) GetProof(leafIndex uint64) (*MerkleProof, error) {
	if len(mt.Levels) == 0 {
		return nil, ErrTreeNotBuilt
	}

	if leafIndex >= uint64(len(mt.Leaves)) {
		return nil, ErrInvalidIndex
	}

	leaf := mt.Leaves[leafIndex]
	siblings := make([][32]byte, 0)

	// Traverse up the tree collecting sibling hashes
	index := leafIndex
	for level := 0; level < len(mt.Levels)-1; level++ {
		levelNodes := mt.Levels[level]
		levelLen := uint64(len(levelNodes))

		// If odd number of nodes, the last one is duplicated
		if levelLen%2 == 1 {
			levelLen++
		}

		// Determine sibling index
		var siblingIndex uint64
		if index%2 == 0 {
			siblingIndex = index + 1
			if siblingIndex >= levelLen {
				siblingIndex = index // Duplicated node
			}
		} else {
			siblingIndex = index - 1
		}

		// Get the sibling hash (use the last node if duplicated)
		var siblingHash [32]byte
		if siblingIndex >= uint64(len(levelNodes)) {
			siblingHash = levelNodes[len(levelNodes)-1]
		} else {
			siblingHash = levelNodes[siblingIndex]
		}

		siblings = append(siblings, siblingHash)
		index = index / 2
	}

	return &MerkleProof{
		Leaf:     leaf,
		Index:    leafIndex,
		Siblings: siblings,
		Root:     mt.Root,
	}, nil
}

// VerifyProof verifies a Merkle proof against the tree root.
func (mt *MerkleTree) VerifyProof(proof *MerkleProof) bool {
	if proof == nil || len(mt.Levels) == 0 {
		return false
	}

	computedRoot := ComputeRootFromProof(proof)
	return computedRoot == mt.Root
}

// String returns a string representation of the Merkle tree.
func (mt *MerkleTree) String() string {
	return fmt.Sprintf("MerkleTree{Root: %s, Leaves: %d}",
		hex.EncodeToString(mt.Root[:]), len(mt.Leaves))
}

// Height returns the height of the Merkle tree.
func (mt *MerkleTree) Height() int {
	return len(mt.Levels)
}

// LeafCount returns the number of leaves in the tree.
func (mt *MerkleTree) LeafCount() int {
	return len(mt.Leaves)
}

// ============================================================================
// Utility Functions
// ============================================================================

// HashChannelOp computes the SHA-256 hash of a channel operation.
func HashChannelOp(op ChannelOpData) [32]byte {
	data := serializeChannelOp(op)
	return sha256.Sum256(data)
}

// HashPair computes the hash of two 32-byte arrays.
// Left hash is prepended with 0x00, right with 0x01 to prevent collision attacks.
func HashPair(left, right [32]byte) [32]byte {
	data := make([]byte, 65)
	data[0] = 0x00 // Prefix for left
	copy(data[1:33], left[:])
	data[33] = 0x01 // Prefix for right
	copy(data[34:], right[:])
	return sha256.Sum256(data)
}

// HashLeaf computes a leaf hash with domain separation.
func HashLeaf(data []byte) [32]byte {
	prefix := []byte{0x00} // Leaf prefix
	combined := append(prefix, data...)
	return sha256.Sum256(combined)
}

// HashInternal computes an internal node hash with domain separation.
func HashInternal(left, right [32]byte) [32]byte {
	data := make([]byte, 65)
	data[0] = 0x01 // Internal node prefix
	copy(data[1:33], left[:])
	copy(data[33:], right[:])
	return sha256.Sum256(data)
}

// ComputeRootFromProof computes the Merkle root from a proof.
func ComputeRootFromProof(proof *MerkleProof) [32]byte {
	current := proof.Leaf
	index := proof.Index

	for _, sibling := range proof.Siblings {
		if index%2 == 0 {
			// Current is left, sibling is right
			current = HashPair(current, sibling)
		} else {
			// Current is right, sibling is left
			current = HashPair(sibling, current)
		}
		index = index / 2
	}

	return current
}

// VerifyProofStandalone verifies a Merkle proof without a tree instance.
func VerifyProofStandalone(leaf [32]byte, proof *MerkleProof) bool {
	if proof == nil {
		return false
	}

	// Verify leaf matches
	if leaf != proof.Leaf {
		return false
	}

	computedRoot := ComputeRootFromProof(proof)
	return computedRoot == proof.Root
}

// EmptyTreeRoot returns the root hash of an empty tree.
func EmptyTreeRoot() [32]byte {
	return sha256.Sum256([]byte("empty_merkle_tree"))
}

// MerkleRootFromLeaves computes the root hash directly from leaves without building the full tree.
func MerkleRootFromLeaves(leaves [][32]byte) ([32]byte, error) {
	if len(leaves) == 0 {
		return EmptyTreeRoot(), nil
	}

	currentLevel := make([][32]byte, len(leaves))
	copy(currentLevel, leaves)

	for len(currentLevel) > 1 {
		nextLevel := make([][32]byte, 0, (len(currentLevel)+1)/2)

		for i := 0; i < len(currentLevel); i += 2 {
			if i+1 < len(currentLevel) {
				nextLevel = append(nextLevel, HashPair(currentLevel[i], currentLevel[i+1]))
			} else {
				// Duplicate last node if odd
				nextLevel = append(nextLevel, HashPair(currentLevel[i], currentLevel[i]))
			}
		}

		currentLevel = nextLevel
	}

	return currentLevel[0], nil
}
