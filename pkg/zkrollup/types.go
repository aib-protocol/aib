// Package zkrollup provides Merkle proof-based batch verification for rollup operations.
// This implementation uses SHA-256 Merkle trees instead of ZK-SNARKs.
package zkrollup

import (
	"encoding/binary"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Internal Types (not exposed in interface)
// ============================================================================

// MerkleTree represents a binary Merkle tree for batch verification.
type MerkleTree struct {
	Root   [32]byte    // Root hash of the tree
	Leaves [][32]byte  // Leaf nodes (channel operation hashes)
	Levels [][][32]byte // All levels from leaves to root
}

// MerkleProof represents a proof path from a leaf to the root.
type MerkleProof struct {
	Leaf     [32]byte   // The leaf value being proven
	Index    uint64     // Position of leaf in the tree
	Siblings [][32]byte // Sibling hashes along the path
	Root     [32]byte   // Expected root hash
}

// ChannelOpData is the internal representation of a channel operation for hashing.
type ChannelOpData struct {
	Type      uint8
	ChannelID [32]byte
	Sequence  uint64
	FinalA    uint64
	FinalB    uint64
	Timestamp int64
}

// BatchConfig holds configuration for batch verification.
type BatchConfig struct {
	MaxOperations    uint64
	ValidationWindow time.Duration
	CheckpointPeriod uint64
}

// DefaultBatchConfig returns the default configuration.
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		MaxOperations:    1024,
		ValidationWindow: 24 * time.Hour,
		CheckpointPeriod: 100,
	}
}

// operationResult holds the result of validating a single operation.
type operationResult struct {
	Index    uint64
	Valid    bool
	ChannelID [32]byte
	Error    error
}

// verificationResult holds the overall batch verification result.
type verificationResult struct {
	Valid        bool
	ProcessedOps uint64
	FailedOps    uint64
	Timestamp    time.Time
	Error        error
}

// ============================================================================
// Type Conversions from Interface Types
// ============================================================================

// ToChannelOpData converts an interfaces.ChannelOp to internal ChannelOpData.
func ToChannelOpData(op interfaces.ChannelOp, timestamp ...int64) ChannelOpData {
	ts := time.Now().Unix()
	if len(timestamp) > 0 {
		ts = timestamp[0]
	}
	return ChannelOpData{
		Type:      op.Type,
		ChannelID: op.ChannelID,
		Sequence:  op.Sequence,
		FinalA:    op.FinalA,
		FinalB:    op.FinalB,
		Timestamp: ts,
	}
}

// ToInterfaceChannelOp converts internal ChannelOpData to interfaces.ChannelOp.
func ToInterfaceChannelOp(data ChannelOpData) interfaces.ChannelOp {
	return interfaces.ChannelOp{
		Type:      data.Type,
		ChannelID: data.ChannelID,
		Sequence:  data.Sequence,
		FinalA:    data.FinalA,
		FinalB:    data.FinalB,
	}
}

// ToInterfaceChannelOpWithTimestamp converts internal ChannelOpData to interfaces.ChannelOp with timestamp.
// Note: interfaces.ChannelOp doesn't have a Timestamp field, so we return the timestamp separately.
func ToInterfaceChannelOpWithTimestamp(data ChannelOpData) (interfaces.ChannelOp, int64) {
	return interfaces.ChannelOp{
		Type:      data.Type,
		ChannelID: data.ChannelID,
		Sequence:  data.Sequence,
		FinalA:    data.FinalA,
		FinalB:    data.FinalB,
	}, data.Timestamp
}

// serializeChannelOp serializes a ChannelOpData for hashing.
func serializeChannelOp(op ChannelOpData) []byte {
	buf := make([]byte, 0, 96) // 1 + 32 + 8 + 8 + 8 + 8 = 65 bytes
	buf = append(buf, op.Type)
	buf = append(buf, op.ChannelID[:]...)
	var seqBuf [8]byte
	binary.BigEndian.PutUint64(seqBuf[:], op.Sequence)
	buf = append(buf, seqBuf[:]...)
	var finalABuf [8]byte
	binary.BigEndian.PutUint64(finalABuf[:], op.FinalA)
	buf = append(buf, finalABuf[:]...)
	var finalBBuf [8]byte
	binary.BigEndian.PutUint64(finalBBuf[:], op.FinalB)
	buf = append(buf, finalBBuf[:]...)
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(op.Timestamp))
	buf = append(buf, tsBuf[:]...)
	return buf
}
