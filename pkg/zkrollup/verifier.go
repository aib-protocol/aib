package zkrollup

import (
	"errors"
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// Verifier errors.
var (
	ErrVerifierNotInitialized = errors.New("verifier not initialized")
	ErrInvalidBatchFormat     = errors.New("invalid batch format")
	ErrVerificationFailed     = errors.New("batch verification failed")
	ErrStateRootUpdateFailed  = errors.New("state root update failed")
	ErrBatchReplay            = errors.New("batch already processed")
	ErrInvalidGenesisBatch    = errors.New("invalid genesis batch")
)

// BatchVerifier implements the ZKBatchVerifier interface using Merkle proofs.
type BatchVerifier struct {
	mu               sync.RWMutex
	currentRoot      [32]byte
	validator        *BatchValidator
	processor        *BatchProcessor
	config           *BatchValidationConfig
	processedBatches map[[32]byte]bool
	initialized      bool
}

// NewBatchVerifier creates a new batch verifier with the given initial state root.
func NewBatchVerifier(initialRoot [32]byte) *BatchVerifier {
	config := DefaultBatchValidationConfig()
	validator := NewBatchValidator(config)
	processor := NewBatchProcessor(validator, initialRoot)

	return &BatchVerifier{
		currentRoot:      initialRoot,
		validator:        validator,
		processor:        processor,
		config:           config,
		processedBatches: make(map[[32]byte]bool),
		initialized:      true,
	}
}

// VerifyBatch implements the ZKBatchVerifier interface.
// It verifies a batch of channel operations using Merkle proofs.
func (bv *BatchVerifier) VerifyBatch(batch *interfaces.ZKProofBatch) (bool, error) {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	if !bv.initialized {
		return false, ErrVerifierNotInitialized
	}

	if batch == nil {
		return false, ErrInvalidBatchFormat
	}

	// Check for replay
	batchHash := HashBatch(batch)
	if bv.processedBatches[batchHash] {
		return false, ErrBatchReplay
	}

	// Validate batch structure
	if err := bv.validator.ValidateBatch(batch); err != nil {
		return false, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}

	// Verify previous state root matches current
	if batch.PrevStateRoot != bv.currentRoot {
		return false, fmt.Errorf("previous state root mismatch: expected %x, got %x",
			bv.currentRoot, batch.PrevStateRoot)
	}

	// Process the batch
	result, err := bv.processor.ProcessBatch(batch)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}

	if !result.Success {
		return false, fmt.Errorf("%w: batch processing failed", ErrVerificationFailed)
	}

	// Mark as processed
	bv.processedBatches[batchHash] = true

	// Update state root
	bv.currentRoot = batch.NewStateRoot

	return true, nil
}

// GetCurrentStateRoot implements the ZKBatchVerifier interface.
// Returns the current verified state root.
func (bv *BatchVerifier) GetCurrentStateRoot() [32]byte {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return bv.currentRoot
}

// UpdateStateRoot implements the ZKBatchVerifier interface.
// Updates the state root (typically used for initialization or recovery).
func (bv *BatchVerifier) UpdateStateRoot(newRoot [32]byte) error {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	if !bv.initialized {
		return ErrVerifierNotInitialized
	}

	// Validate the new root
	emptyRoot := [32]byte{}
	if newRoot == emptyRoot {
		return fmt.Errorf("%w: cannot set empty state root", ErrStateRootUpdateFailed)
	}

	bv.currentRoot = newRoot
	bv.processor.SetCurrentRoot(newRoot)

	return nil
}

// GetConfig returns the current validation configuration.
func (bv *BatchVerifier) GetConfig() *BatchValidationConfig {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return bv.config
}

// UpdateConfig updates the validation configuration.
func (bv *BatchVerifier) UpdateConfig(config *BatchValidationConfig) {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	bv.config = config
	bv.validator.UpdateConfig(config)
}

// IsBatchProcessed checks if a batch has been processed.
func (bv *BatchVerifier) IsBatchProcessed(batchHash [32]byte) bool {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return bv.processedBatches[batchHash]
}

// GetProcessedBatchCount returns the number of batches processed.
func (bv *BatchVerifier) GetProcessedBatchCount() int {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return len(bv.processedBatches)
}

// Reset clears the processed batch history (use with caution).
func (bv *BatchVerifier) Reset() {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	bv.processedBatches = make(map[[32]byte]bool)
}

// VerifyGenesisBatch verifies and processes a genesis batch.
func (bv *BatchVerifier) VerifyGenesisBatch(batch *interfaces.ZKProofBatch) error {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	if !bv.initialized {
		return ErrVerifierNotInitialized
	}

	// Genesis batch must have empty previous root
	emptyRoot := [32]byte{}
	if batch.PrevStateRoot != emptyRoot {
		return fmt.Errorf("%w: genesis batch must have empty previous root", ErrInvalidGenesisBatch)
	}

	// Verify the batch
	valid, err := bv.VerifyBatch(batch)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGenesisBatch, err)
	}

	if !valid {
		return ErrInvalidGenesisBatch
	}

	return nil
}

// GetValidator returns the internal batch validator.
func (bv *BatchVerifier) GetValidator() *BatchValidator {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return bv.validator
}

// GetProcessor returns the internal batch processor.
func (bv *BatchVerifier) GetProcessor() *BatchProcessor {
	bv.mu.RLock()
	defer bv.mu.RUnlock()

	return bv.processor
}

// Close cleans up resources.
func (bv *BatchVerifier) Close() error {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	bv.initialized = false
	return nil
}

// Ensure BatchVerifier implements the ZKBatchVerifier interface.
var _ interfaces.ZKBatchVerifier = (*BatchVerifier)(nil)

// NewVerifierWithConfig creates a new verifier with custom configuration.
func NewVerifierWithConfig(initialRoot [32]byte, config *BatchValidationConfig) *BatchVerifier {
	if config == nil {
		config = DefaultBatchValidationConfig()
	}

	validator := NewBatchValidator(config)
	processor := NewBatchProcessor(validator, initialRoot)

	return &BatchVerifier{
		currentRoot:      initialRoot,
		validator:        validator,
		processor:        processor,
		config:           config,
		processedBatches: make(map[[32]byte]bool),
		initialized:      true,
	}
}

// VerifyBatchWithProof verifies a batch with an explicit Merkle proof.
func (bv *BatchVerifier) VerifyBatchWithProof(batch *interfaces.ZKProofBatch, proof *MerkleProof) (bool, error) {
	bv.mu.Lock()
	defer bv.mu.Unlock()

	if !bv.initialized {
		return false, ErrVerifierNotInitialized
	}

	if batch == nil || proof == nil {
		return false, ErrInvalidBatchFormat
	}

	// Verify proof root matches batch new state root
	if proof.Root != batch.NewStateRoot {
		return false, fmt.Errorf("proof root mismatch: expected %x, got %x",
			batch.NewStateRoot, proof.Root)
	}

	// Verify the proof
	computedRoot := ComputeRootFromProof(proof)
	if computedRoot != proof.Root {
		return false, ErrInvalidProof
	}

	// Verify batch structure
	if err := bv.validator.ValidateBatch(batch); err != nil {
		return false, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}

	// Verify previous state root matches current
	if batch.PrevStateRoot != bv.currentRoot {
		return false, fmt.Errorf("previous state root mismatch: expected %x, got %x",
			bv.currentRoot, batch.PrevStateRoot)
	}

	// Mark as processed
	batchHash := HashBatch(batch)
	bv.processedBatches[batchHash] = true

	// Update state root
	bv.currentRoot = batch.NewStateRoot

	return true, nil
}

// BatchVerificationResult contains detailed verification results.
type BatchVerificationResult struct {
	Success         bool
	Verified        bool
	StateRoot       [32]byte
	OperationsCount uint64
	Error           error
	Details         string
}

// VerifyBatchDetailed performs detailed batch verification with comprehensive results.
func (bv *BatchVerifier) VerifyBatchDetailed(batch *interfaces.ZKProofBatch) *BatchVerificationResult {
	result := &BatchVerificationResult{
		OperationsCount: batch.TxCount,
		StateRoot:       bv.GetCurrentStateRoot(),
	}

	valid, err := bv.VerifyBatch(batch)
	if err != nil {
		result.Error = err
		result.Details = err.Error()
		return result
	}

	result.Success = valid
	result.Verified = valid
	result.StateRoot = bv.GetCurrentStateRoot()

	if valid {
		result.Details = "Batch verified successfully"
	} else {
		result.Details = "Batch verification failed"
	}

	return result
}
