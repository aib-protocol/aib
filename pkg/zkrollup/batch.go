package zkrollup

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// Batch verification errors.
var (
	ErrInvalidBatch          = errors.New("invalid batch")
	ErrInvalidStateRoot      = errors.New("invalid state root")
	ErrInvalidPrevRoot       = errors.New("invalid previous state root")
	ErrInvalidOperations     = errors.New("invalid operations")
	ErrInvalidSequence       = errors.New("invalid sequence number")
	ErrInvalidChannelOp      = errors.New("invalid channel operation")
	ErrBatchTooLarge         = errors.New("batch exceeds maximum size")
	ErrBatchTooSmall         = errors.New("batch is empty")
	ErrInvalidTimestamp      = errors.New("invalid timestamp")
	ErrStateTransitionFailed = errors.New("state transition verification failed")
)

// Batch processing errors.
var (
	ErrBatchNil               = errors.New("batch is nil")
	ErrNoOperations           = errors.New("no operations in batch")
	ErrStateRootEmpty         = errors.New("state root is empty")
	ErrOperationCountMismatch = errors.New("operation count mismatch")
	ErrInvalidBatchHash       = errors.New("invalid batch hash")
	ErrBatchAlreadyProcessed  = errors.New("batch already processed")
	ErrInvalidSignature       = errors.New("invalid signature")
	ErrChannelNotFound        = errors.New("channel not found")
	ErrInsufficientBalance    = errors.New("insufficient balance")
	ErrInvalidStateTransitionOp = errors.New("invalid state transition")
)

// ============================================================================
// Batch Validation Configuration
// ============================================================================

// BatchValidationConfig holds validation parameters.
type BatchValidationConfig struct {
	MaxOperations    uint64
	MinOperations    uint64
	MaxTimeWindow    time.Duration
	MaxSequenceGap   uint64
	RequireTimestamp bool
}

// DefaultBatchValidationConfig returns the default configuration.
func DefaultBatchValidationConfig() *BatchValidationConfig {
	return &BatchValidationConfig{
		MaxOperations:    1024,
		MinOperations:    1,
		MaxTimeWindow:    24 * time.Hour,
		MaxSequenceGap:   1000000,
		RequireTimestamp: true,
	}
}

// ============================================================================
// Batch Validator
// ============================================================================

// BatchValidator handles batch validation using Merkle proofs.
type BatchValidator struct {
	config    *BatchValidationConfig
	prevRoot  [32]byte
	timestamp time.Time
}

// NewBatchValidator creates a new batch validator.
func NewBatchValidator(config *BatchValidationConfig) *BatchValidator {
	if config == nil {
		config = DefaultBatchValidationConfig()
	}
	return &BatchValidator{
		config:    config,
		timestamp: time.Now(),
	}
}

// ValidateBatch performs comprehensive batch validation.
func (bv *BatchValidator) ValidateBatch(batch *interfaces.ZKProofBatch) error {
	if batch == nil {
		return ErrInvalidBatch
	}

	// Validate basic structure
	if err := bv.validateStructure(batch); err != nil {
		return fmt.Errorf("structure validation failed: %w", err)
	}

	// Validate state roots
	if err := bv.validateStateRoots(batch); err != nil {
		return fmt.Errorf("state root validation failed: %w", err)
	}

	// Validate operations
	if err := bv.validateOperations(batch); err != nil {
		return fmt.Errorf("operation validation failed: %w", err)
	}

	// Validate Merkle proof
	if err := bv.validateMerkleProof(batch); err != nil {
		return fmt.Errorf("merkle proof validation failed: %w", err)
	}

	return nil
}

// validateStructure checks the batch structure validity.
func (bv *BatchValidator) validateStructure(batch *interfaces.ZKProofBatch) error {
	if batch.TxCount == 0 {
		return ErrBatchTooSmall
	}
	if batch.TxCount > bv.config.MaxOperations {
		return ErrBatchTooLarge
	}

	if uint64(len(batch.ChannelOps)) != batch.TxCount {
		return fmt.Errorf("channel operation count mismatch: expected %d, got %d",
			batch.TxCount, len(batch.ChannelOps))
	}

	if bv.config.RequireTimestamp {
		if batch.Timestamp.IsZero() {
			return ErrInvalidTimestamp
		}
		age := time.Since(batch.Timestamp)
		if age < 0 {
			age = -age
		}
		if age > bv.config.MaxTimeWindow {
			return fmt.Errorf("batch timestamp too old: %v", age)
		}
	}

	return nil
}

// validateStateRoots checks the state root transitions.
func (bv *BatchValidator) validateStateRoots(batch *interfaces.ZKProofBatch) error {
	emptyRoot := [32]byte{}

	if batch.NewStateRoot == emptyRoot {
		return ErrInvalidStateRoot
	}

	isGenesis := batch.PrevStateRoot == emptyRoot

	// Verify state transition (roots should be different for non-genesis)
	if batch.NewStateRoot == batch.PrevStateRoot && !isGenesis {
		return ErrStateTransitionFailed
	}

	return nil
}

// validateOperations validates each channel operation.
func (bv *BatchValidator) validateOperations(batch *interfaces.ZKProofBatch) error {
	seenChannels := make(map[[32]byte]uint64)

	for i, op := range batch.ChannelOps {
		if op.Type > 3 {
			return fmt.Errorf("invalid operation type %d at index %d", op.Type, i)
		}

		emptyID := [32]byte{}
		if op.ChannelID == emptyID {
			return fmt.Errorf("empty channel ID at index %d", i)
		}

		if op.Sequence == 0 && op.Type != 0 {
			return fmt.Errorf("invalid sequence number at index %d", i)
		}

		if lastSeq, exists := seenChannels[op.ChannelID]; exists {
			if op.Sequence <= lastSeq {
				return fmt.Errorf("sequence number regression for channel %x: %d <= %d",
					op.ChannelID, op.Sequence, lastSeq)
			}
			gap := op.Sequence - lastSeq
			if gap > bv.config.MaxSequenceGap {
				return fmt.Errorf("sequence gap too large for channel %x: %d",
					op.ChannelID, gap)
			}
		}
		seenChannels[op.ChannelID] = op.Sequence

		if op.FinalA > 0 && op.FinalB > 0 {
			sum := op.FinalA + op.FinalB
			if sum < op.FinalA || sum < op.FinalB {
				return fmt.Errorf("balance overflow at index %d", i)
			}
		}
	}

	return nil
}

// validateMerkleProof validates the Merkle proof in the batch.
func (bv *BatchValidator) validateMerkleProof(batch *interfaces.ZKProofBatch) error {
	if len(batch.Proof) == 0 {
		return bv.verifyByReconstruction(batch)
	}

	proof, err := DecodeProof(batch.Proof)
	if err != nil {
		return fmt.Errorf("failed to decode proof: %w", err)
	}

	if proof.Root != batch.NewStateRoot {
		return ErrRootMismatch
	}

	return bv.verifyByReconstruction(batch)
}

// verifyByReconstruction rebuilds the Merkle tree from operations and verifies the root.
func (bv *BatchValidator) verifyByReconstruction(batch *interfaces.ZKProofBatch) error {
	ops := make([]ChannelOpData, len(batch.ChannelOps))
	for i, op := range batch.ChannelOps {
		ops[i] = ToChannelOpData(op, batch.Timestamp.Unix())
	}

	tree, err := NewMerkleTreeFromChannelOps(ops)
	if err != nil {
		return fmt.Errorf("failed to build merkle tree: %w", err)
	}

	if tree.Root != batch.NewStateRoot {
		return fmt.Errorf("merkle root mismatch: computed %x, expected %x",
			tree.Root, batch.NewStateRoot)
	}

	return nil
}

// SetPreviousRoot sets the expected previous state root for validation.
func (bv *BatchValidator) SetPreviousRoot(root [32]byte) {
	bv.prevRoot = root
}

// GetConfig returns the validator's configuration.
func (bv *BatchValidator) GetConfig() *BatchValidationConfig {
	return bv.config
}

// UpdateConfig updates the validator's configuration.
func (bv *BatchValidator) UpdateConfig(config *BatchValidationConfig) {
	if config != nil {
		bv.config = config
	}
}

// ============================================================================
// Batch Processor
// ============================================================================

// BatchResult represents the result of batch processing.
type BatchResult struct {
	Success          bool
	ProcessedAt      time.Time
	OperationsValid  uint64
	OperationsFailed uint64
	NewStateRoot     [32]byte
	ProofVerified    bool
	Errors           []error
}

// BatchProcessor handles batch operations.
type BatchProcessor struct {
	validator        *BatchValidator
	currentRoot      [32]byte
	processedBatches map[[32]byte]bool
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(validator *BatchValidator, initialRoot [32]byte) *BatchProcessor {
	return &BatchProcessor{
		validator:        validator,
		currentRoot:      initialRoot,
		processedBatches: make(map[[32]byte]bool),
	}
}

// ProcessBatch processes a complete batch.
func (bp *BatchProcessor) ProcessBatch(batch *interfaces.ZKProofBatch) (*BatchResult, error) {
	if batch == nil {
		return nil, ErrBatchNil
	}

	result := &BatchResult{
		Success:     true,
		ProcessedAt: time.Now(),
		Errors:      make([]error, 0),
	}

	// Check for replay
	batchHash := HashBatch(batch)
	if bp.processedBatches[batchHash] {
		result.Success = false
		result.Errors = append(result.Errors, ErrBatchAlreadyProcessed)
		return result, ErrBatchAlreadyProcessed
	}

	// Validate batch structure
	if err := bp.validator.ValidateBatch(batch); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err)
		return result, err
	}

	// Verify previous state root matches
	if batch.PrevStateRoot != bp.currentRoot {
		result.Success = false
		err := fmt.Errorf("previous state root mismatch: expected %x, got %x",
			bp.currentRoot, batch.PrevStateRoot)
		result.Errors = append(result.Errors, err)
		return result, err
	}

	// Process each operation
	for i, op := range batch.ChannelOps {
		valid, err := bp.validateOperation(&op)
		if err != nil || !valid {
			result.OperationsFailed++
			result.Success = false
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("operation %d: %w", i, err))
			}
		} else {
			result.OperationsValid++
		}
	}

	if result.OperationsFailed > 0 {
		result.Success = false
		return result, fmt.Errorf("%d operations failed", result.OperationsFailed)
	}

	// Update state
	bp.currentRoot = batch.NewStateRoot
	result.NewStateRoot = batch.NewStateRoot
	result.ProofVerified = true
	bp.processedBatches[batchHash] = true

	return result, nil
}

// validateOperation validates a single channel operation.
func (bp *BatchProcessor) validateOperation(op *interfaces.ChannelOp) (bool, error) {
	if op == nil {
		return false, ErrInvalidChannelOp
	}

	emptyID := [32]byte{}
	if op.ChannelID == emptyID {
		return false, ErrChannelNotFound
	}

	switch op.Type {
	case 0: // Open
		if op.Sequence > 1 {
			return false, fmt.Errorf("invalid sequence for open operation")
		}
		if op.FinalA > 0 || op.FinalB > 0 {
			sum := op.FinalA + op.FinalB
			if sum < op.FinalA || sum < op.FinalB {
				return false, ErrInsufficientBalance
			}
		}

	case 1: // Update
		if op.Sequence == 0 {
			return false, ErrInvalidSequence
		}

	case 2: // Close
		if op.Sequence == 0 {
			return false, ErrInvalidSequence
		}
		sum := op.FinalA + op.FinalB
		if sum < op.FinalA || sum < op.FinalB {
			return false, ErrInsufficientBalance
		}

	case 3: // Dispute
		if op.Sequence == 0 {
			return false, ErrInvalidSequence
		}

	default:
		return false, ErrInvalidChannelOp
	}

	return true, nil
}

// GetCurrentRoot returns the current state root.
func (bp *BatchProcessor) GetCurrentRoot() [32]byte {
	return bp.currentRoot
}

// SetCurrentRoot sets the current state root (for initialization/recovery).
func (bp *BatchProcessor) SetCurrentRoot(root [32]byte) {
	bp.currentRoot = root
}

// IsBatchProcessed checks if a batch has already been processed.
func (bp *BatchProcessor) IsBatchProcessed(batchHash [32]byte) bool {
	return bp.processedBatches[batchHash]
}

// ============================================================================
// Proof Encoding/Decoding
// ============================================================================

// EncodeProof encodes a Merkle proof into bytes.
func EncodeProof(proof *MerkleProof) []byte {
	if proof == nil {
		return nil
	}

	// Format: [leaf:32][index:8][num_siblings:4][siblings...][root:32]
	buf := make([]byte, 0, 76+len(proof.Siblings)*32)

	buf = append(buf, proof.Leaf[:]...)

	indexBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBytes, proof.Index)
	buf = append(buf, indexBytes...)

	numSiblings := uint32(len(proof.Siblings))
	numBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(numBytes, numSiblings)
	buf = append(buf, numBytes...)

	for _, sib := range proof.Siblings {
		buf = append(buf, sib[:]...)
	}

	buf = append(buf, proof.Root[:]...)

	return buf
}

// DecodeProof decodes a Merkle proof from bytes.
func DecodeProof(data []byte) (*MerkleProof, error) {
	if len(data) < 76 {
		return nil, errors.New("proof data too short")
	}

	proof := &MerkleProof{}

	copy(proof.Leaf[:], data[0:32])
	proof.Index = binary.BigEndian.Uint64(data[32:40])

	numSiblings := binary.BigEndian.Uint32(data[40:44])
	expectedLen := 76 + int(numSiblings)*32
	if len(data) != expectedLen {
		return nil, fmt.Errorf("invalid proof length: expected %d, got %d", expectedLen, len(data))
	}

	proof.Siblings = make([][32]byte, numSiblings)
	offset := 44
	for i := uint32(0); i < numSiblings; i++ {
		copy(proof.Siblings[i][:], data[offset:offset+32])
		offset += 32
	}

	copy(proof.Root[:], data[offset:offset+32])

	return proof, nil
}

// ============================================================================
// Batch Hashing and Serialization
// ============================================================================

// HashBatch computes a unique hash for a batch.
func HashBatch(batch *interfaces.ZKProofBatch) [32]byte {
	if batch == nil {
		return [32]byte{}
	}

	data := make([]byte, 0, 128+len(batch.Proof)+len(batch.PublicInputs))

	data = append(data, batch.PrevStateRoot[:]...)
	data = append(data, batch.NewStateRoot[:]...)
	data = append(data, batch.Proof...)
	data = append(data, batch.PublicInputs...)

	var countBytes [8]byte
	binary.BigEndian.PutUint64(countBytes[:], batch.TxCount)
	data = append(data, countBytes[:]...)

	for _, op := range batch.ChannelOps {
		opData := ToChannelOpData(op, batch.Timestamp.Unix())
		opHash := HashChannelOp(opData)
		data = append(data, opHash[:]...)
	}

	var tsBytes [8]byte
	binary.BigEndian.PutUint64(tsBytes[:], uint64(batch.Timestamp.Unix()))
	data = append(data, tsBytes[:]...)

	return sha256.Sum256(data)
}

// SerializeBatch serializes a batch for storage/transmission.
func SerializeBatch(batch *interfaces.ZKProofBatch) []byte {
	if batch == nil {
		return nil
	}

	data := make([]byte, 0, 256+len(batch.Proof)+len(batch.PublicInputs)+len(batch.ChannelOps)*64)

	// Header: version (1 byte)
	data = append(data, 0x01)

	// State roots
	data = append(data, batch.PrevStateRoot[:]...)
	data = append(data, batch.NewStateRoot[:]...)

	// Proof length and data
	var proofLen [4]byte
	binary.BigEndian.PutUint32(proofLen[:], uint32(len(batch.Proof)))
	data = append(data, proofLen[:]...)
	data = append(data, batch.Proof...)

	// Public inputs length and data
	var pubLen [4]byte
	binary.BigEndian.PutUint32(pubLen[:], uint32(len(batch.PublicInputs)))
	data = append(data, pubLen[:]...)
	data = append(data, batch.PublicInputs...)

	// Transaction count
	var txCount [8]byte
	binary.BigEndian.PutUint64(txCount[:], batch.TxCount)
	data = append(data, txCount[:]...)

	// Channel operations count
	var opsCount [4]byte
	binary.BigEndian.PutUint32(opsCount[:], uint32(len(batch.ChannelOps)))
	data = append(data, opsCount[:]...)

	// Each channel operation
	for _, op := range batch.ChannelOps {
		data = append(data, op.Type)
		data = append(data, op.ChannelID[:]...)
		var seq [8]byte
		binary.BigEndian.PutUint64(seq[:], op.Sequence)
		data = append(data, seq[:]...)
		var finalA [8]byte
		binary.BigEndian.PutUint64(finalA[:], op.FinalA)
		data = append(data, finalA[:]...)
		var finalB [8]byte
		binary.BigEndian.PutUint64(finalB[:], op.FinalB)
		data = append(data, finalB[:]...)
	}

	// Timestamp
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(batch.Timestamp.Unix()))
	data = append(data, ts[:]...)

	return data
}

// DeserializeBatch deserializes a batch from bytes.
func DeserializeBatch(data []byte) (*interfaces.ZKProofBatch, error) {
	if len(data) < 86 {
		return nil, errors.New("data too short for batch")
	}

	offset := 0

	// Version check
	version := data[offset]
	offset++
	if version != 0x01 {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}

	batch := &interfaces.ZKProofBatch{}

	// State roots
	copy(batch.PrevStateRoot[:], data[offset:offset+32])
	offset += 32
	copy(batch.NewStateRoot[:], data[offset:offset+32])
	offset += 32

	// Proof
	proofLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	batch.Proof = make([]byte, proofLen)
	copy(batch.Proof, data[offset:offset+int(proofLen)])
	offset += int(proofLen)

	// Public inputs
	pubLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	batch.PublicInputs = make([]byte, pubLen)
	copy(batch.PublicInputs, data[offset:offset+int(pubLen)])
	offset += int(pubLen)

	// Transaction count
	batch.TxCount = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Channel operations
	opsCount := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	batch.ChannelOps = make([]interfaces.ChannelOp, opsCount)

	for i := uint32(0); i < opsCount; i++ {
		op := &batch.ChannelOps[i]
		op.Type = data[offset]
		offset++
		copy(op.ChannelID[:], data[offset:offset+32])
		offset += 32
		op.Sequence = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		op.FinalA = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		op.FinalB = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	// Timestamp
	batch.Timestamp = time.Unix(int64(binary.BigEndian.Uint64(data[offset:offset+8])), 0)

	return batch, nil
}

// VerifyMerklePath verifies a single step in a Merkle path.
func VerifyMerklePath(left, right, expectedParent [32]byte) bool {
	computed := HashPair(left, right)
	return computed == expectedParent
}

// SimpleHash computes a simple SHA-256 hash of data.
func SimpleHash(data []byte) [32]byte {
	return sha256.Sum256(data)
}
