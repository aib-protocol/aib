package crypto

import (
	"bytes"
	"errors"
)

// StandardMerkleTree implements a standard binary Merkle tree.
// Used for transaction trees (Bitcoin-compatible structure).
type StandardMerkleTree struct {
	hasher Hasher
	leaves [][]byte
	levels [][][]byte // levels[0] = leaf hashes, levels[last] = [root]
}

// NewStandardMerkleTree creates a Merkle tree from the given leaves using the specified hasher.
func NewStandardMerkleTree(hasher Hasher, leaves [][]byte) (*StandardMerkleTree, error) {
	if len(leaves) == 0 {
		return nil, errors.New("merkle: at least one leaf required")
	}

	tree := &StandardMerkleTree{
		hasher: hasher,
		leaves: make([][]byte, len(leaves)),
	}

	for i, leaf := range leaves {
		tree.leaves[i] = make([]byte, len(leaf))
		copy(tree.leaves[i], leaf)
	}

	tree.build()
	return tree, nil
}

// build constructs the Merkle tree level by level.
func (t *StandardMerkleTree) build() {
	level := make([][]byte, len(t.leaves))
	for i, leaf := range t.leaves {
		level[i] = t.hasher.Hash(leaf)
	}

	t.levels = [][][]byte{level}

	for len(level) > 1 {
		// If odd, duplicate last node (Bitcoin convention)
		if len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}

		nextLevel := make([][]byte, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := make([]byte, 0, len(level[i])+len(level[i+1]))
			combined = append(combined, level[i]...)
			combined = append(combined, level[i+1]...)
			nextLevel[i/2] = t.hasher.Hash(combined)
		}
		t.levels = append(t.levels, nextLevel)
		level = nextLevel
	}
}

// Root returns the Merkle root hash.
func (t *StandardMerkleTree) Root() []byte {
	if len(t.levels) == 0 {
		return nil
	}
	topLevel := t.levels[len(t.levels)-1]
	if len(topLevel) == 0 {
		return nil
	}
	return topLevel[0]
}

// Proof generates a Merkle proof for the leaf at the given index.
// Returns sibling hashes from leaf to root.
func (t *StandardMerkleTree) Proof(index int) ([][]byte, error) {
	if index < 0 || index >= len(t.leaves) {
		return nil, errors.New("merkle: index out of range")
	}

	proof := make([][]byte, 0, len(t.levels)-1)
	idx := index

	for lvl := 0; lvl < len(t.levels)-1; lvl++ {
		level := t.levels[lvl]

		// Determine sibling
		var sibling []byte
		if idx%2 == 0 {
			// Sibling is to the right
			if idx+1 < len(level) {
				sibling = level[idx+1]
			} else {
				// Odd case: duplicate self (Bitcoin convention)
				sibling = level[idx]
			}
		} else {
			sibling = level[idx-1]
		}

		sibCopy := make([]byte, len(sibling))
		copy(sibCopy, sibling)
		proof = append(proof, sibCopy)

		idx /= 2
	}

	return proof, nil
}

// Verify checks a Merkle proof for a given leaf data, root, proof path, and leaf index.
func (t *StandardMerkleTree) Verify(leaf, root []byte, proof [][]byte, index int) bool {
	return VerifyMerkleProof(t.hasher, leaf, root, proof, index)
}

// VerifyMerkleProof verifies a Merkle proof without needing the full tree.
func VerifyMerkleProof(hasher Hasher, leaf, root []byte, proof [][]byte, index int) bool {
	current := hasher.Hash(leaf)

	for _, sibling := range proof {
		var combined []byte
		if index%2 == 0 {
			combined = append(current, sibling...)
		} else {
			combined = append(sibling, current...)
		}
		current = hasher.Hash(combined)
		index /= 2
	}

	return bytes.Equal(current, root)
}

// SparseMerkleTree implements a sparse Merkle tree for state proofs.
// Supports efficient existence and non-existence proofs.
type SparseMerkleTree struct {
	hasher   Hasher
	root     []byte
	depth    int
	store    map[string][]byte // key -> value
	defaults [][]byte          // default hashes for each level
}

// NewSparseMerkleTree creates a new sparse Merkle tree with the given depth.
func NewSparseMerkleTree(hasher Hasher, depth int) *SparseMerkleTree {
	if depth <= 0 || depth > 256 {
		depth = 256
	}

	// Precompute default hashes for each level
	defaults := make([][]byte, depth+1)
	defaults[0] = hasher.Hash(nil) // empty leaf hash
	for i := 1; i <= depth; i++ {
		combined := append(defaults[i-1], defaults[i-1]...)
		defaults[i] = hasher.Hash(combined)
	}

	return &SparseMerkleTree{
		hasher:   hasher,
		root:     defaults[depth],
		depth:    depth,
		store:    make(map[string][]byte),
		defaults: defaults,
	}
}

// Root returns the current root hash.
func (t *SparseMerkleTree) Root() []byte {
	return t.root
}

// Update sets a key-value pair and recalculates the root.
func (t *SparseMerkleTree) Update(key, value []byte) {
	// Hash the key to get the path
	keyHash := t.hasher.Hash(key)

	// Hash the value to get the leaf
	leafHash := t.hasher.Hash(value)

	// Store the mapping
	t.store[string(keyHash)] = leafHash

	// Rebuild the path from leaf to root
	t.root = t.computeRoot(keyHash, leafHash, 0)
}

// Get retrieves a value's hash for a given key.
func (t *SparseMerkleTree) Get(key []byte) []byte {
	keyHash := t.hasher.Hash(key)
	if val, ok := t.store[string(keyHash)]; ok {
		return val
	}
	return t.defaults[0]
}

// ProveInclusion generates a proof that a key exists in the tree.
func (t *SparseMerkleTree) ProveInclusion(key []byte) ([][]byte, error) {
	keyHash := t.hasher.Hash(key)
	if _, ok := t.store[string(keyHash)]; !ok {
		return nil, errors.New("sparse merkle: key not found")
	}

	siblings := make([][]byte, t.depth)
	for i := 0; i < t.depth; i++ {
		bit := getBit(keyHash, i)
		siblingPath := flipBit(keyHash, i)
		if val, ok := t.store[string(siblingPath)]; ok {
			siblings[i] = val
		} else {
			siblings[i] = t.defaults[0]
		}
		_ = bit // bit determines left/right positioning during verification
	}
	return siblings, nil
}

// VerifyInclusion verifies an inclusion proof.
func (t *SparseMerkleTree) VerifyInclusion(key, value []byte, proof [][]byte) bool {
	if len(proof) != t.depth {
		return false
	}

	keyHash := t.hasher.Hash(key)
	current := t.hasher.Hash(value)

	for i := 0; i < t.depth; i++ {
		bit := getBit(keyHash, i)
		var combined []byte
		if bit == 0 {
			combined = append(current, proof[i]...)
		} else {
			combined = append(proof[i], current...)
		}
		current = t.hasher.Hash(combined)
	}

	return bytes.Equal(current, t.root)
}

// computeRoot computes the root from a leaf hash and key path.
func (t *SparseMerkleTree) computeRoot(keyHash, leafHash []byte, level int) []byte {
	if level >= t.depth {
		return leafHash
	}

	current := leafHash
	for i := 0; i < t.depth; i++ {
		bit := getBit(keyHash, i)
		sibling := t.defaults[i]

		// Check if any stored key has the sibling path
		for storedKeyStr, storedVal := range t.store {
			storedKey := []byte(storedKeyStr)
			if !bytes.Equal(storedKey, keyHash) && matchesUpTo(storedKey, keyHash, i) && getBit(storedKey, i) != bit {
				sibling = t.recomputeSubtree(storedKey, storedVal, i)
				break
			}
		}

		var combined []byte
		if bit == 0 {
			combined = append(current, sibling...)
		} else {
			combined = append(sibling, current...)
		}
		current = t.hasher.Hash(combined)
	}
	return current
}

// recomputeSubtree recomputes the hash for a subtree rooted at the given level.
func (t *SparseMerkleTree) recomputeSubtree(keyHash, leafHash []byte, startLevel int) []byte {
	current := leafHash
	for i := 0; i < startLevel; i++ {
		bit := getBit(keyHash, i)
		var combined []byte
		if bit == 0 {
			combined = append(current, t.defaults[i]...)
		} else {
			combined = append(t.defaults[i], current...)
		}
		current = t.hasher.Hash(combined)
	}
	return current
}

// getBit returns the bit at position pos in data (0 or 1).
func getBit(data []byte, pos int) byte {
	byteIdx := pos / 8
	bitIdx := uint(7 - pos%8)
	if byteIdx >= len(data) {
		return 0
	}
	return (data[byteIdx] >> bitIdx) & 1
}

// flipBit returns a copy of data with the bit at position pos flipped.
func flipBit(data []byte, pos int) []byte {
	result := make([]byte, len(data))
	copy(result, data)
	byteIdx := pos / 8
	bitIdx := uint(7 - pos%8)
	if byteIdx < len(result) {
		result[byteIdx] ^= 1 << bitIdx
	}
	return result
}

// matchesUpTo checks if two byte slices have the same bits up to (but not including) position pos.
func matchesUpTo(a, b []byte, pos int) bool {
	for i := 0; i < pos; i++ {
		if getBit(a, i) != getBit(b, i) {
			return false
		}
	}
	return true
}
