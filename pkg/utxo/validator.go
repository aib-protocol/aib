// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Block Validator Module
package utxo

import (
	"fmt"
	"time"
)

// ============================================================================
// Block Validator Interface and Implementation
// ============================================================================

// BlockValidator defines the interface for block validation.
type BlockValidator interface {
	ValidateBlock(block *Block) error
	ValidateHeader(header *BlockHeader) error
	ValidateTransaction(tx *Transaction) error
}

// DefaultBlockValidator is the default implementation of BlockValidator.
type DefaultBlockValidator struct {
	utxoProvider UTXOProvider
	consensus    *ConsensusState
}

// NewDefaultBlockValidator creates a new default block validator.
func NewDefaultBlockValidator(utxoProvider UTXOProvider, consensus *ConsensusState) *DefaultBlockValidator {
	return &DefaultBlockValidator{
		utxoProvider: utxoProvider,
		consensus:    consensus,
	}
}

// ValidateBlock performs full block validation.
func (v *DefaultBlockValidator) ValidateBlock(block *Block) error {
	// 1. Validate header
	if err := v.ValidateHeader(&block.Header); err != nil {
		return fmt.Errorf("header validation failed: %w", err)
	}

	// 2. Validate PoAIW (Proof of AI Work) - only for Version >= 2
	if err := v.validatePoAIW(block); err != nil {
		return fmt.Errorf("PoAIW validation failed: %w", err)
	}

	// 3. Validate transactions
	for i, tx := range block.Transactions {
		if err := v.ValidateTransaction(tx); err != nil {
			return fmt.Errorf("transaction %d validation failed: %w", i, err)
		}
	}

	// 4. Validate Merkle root
	expectedMerkle := block.CalculateMerkleRoot()
	if expectedMerkle != block.Header.MerkleRoot {
		return fmt.Errorf("merkle root mismatch")
	}

	// 5. Validate block hash
	expectedHash := block.CalculateHash()
	if expectedHash != block.Hash {
		return fmt.Errorf("block hash mismatch")
	}

	return nil
}

// ValidateHeader validates a block header.
func (v *DefaultBlockValidator) ValidateHeader(header *BlockHeader) error {
	// Check version
	if header.Version == 0 {
		return fmt.Errorf("invalid version: 0")
	}

	// Check timestamp is not too far in the future
	now := uint64(time.Now().Unix())
	if header.Timestamp > now+300 {
		return fmt.Errorf("header timestamp too far in the future")
	}

	// Check proposer is not empty
	if header.Proposer == ([32]byte{}) {
		return fmt.Errorf("proposer is empty")
	}

	// Check signature exists
	if len(header.Signature) == 0 {
		return fmt.Errorf("signature is missing")
	}

	return nil
}

// ValidateTransaction validates a transaction.
func (v *DefaultBlockValidator) ValidateTransaction(tx *Transaction) error {
	// Coinbase transactions have special validation
	if tx.IsCoinbase() {
		return v.validateCoinbase(tx)
	}

	// Non-coinbase transaction validation
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("transaction has no inputs")
	}

	if len(tx.Outputs) == 0 {
		return fmt.Errorf("transaction has no outputs")
	}

	// Validate all input signatures
	for i := range tx.Inputs {
		if !tx.VerifyInput(i) {
			return fmt.Errorf("invalid signature for input %d", i)
		}
	}

	// If UTXO provider is available, validate inputs exist
	if v.utxoProvider != nil {
		for i, input := range tx.Inputs {
			_, err := v.utxoProvider.GetUTXO(input.TxHash, input.Index)
			if err != nil {
				return fmt.Errorf("input %d: UTXO not found: %w", i, err)
			}
		}

		// Validate fee
		fee, err := tx.GetFee(v.utxoProvider)
		if err != nil {
			return fmt.Errorf("failed to calculate fee: %w", err)
		}
		if fee < MinTransactionFee {
			return fmt.Errorf("fee %d below minimum %d", fee, MinTransactionFee)
		}
	}

	return nil
}

// validateCoinbase validates a coinbase transaction.
func (v *DefaultBlockValidator) validateCoinbase(tx *Transaction) error {
	// Coinbase must have exactly one input
	if len(tx.Inputs) != 1 {
		return fmt.Errorf("coinbase must have exactly one input")
	}

	// Coinbase input must have null hash and max index
	if tx.Inputs[0].TxHash != [32]byte{} {
		return fmt.Errorf("coinbase input txhash must be zero")
	}
	if tx.Inputs[0].Index != 0xffffffff {
		return fmt.Errorf("coinbase input index must be 0xffffffff")
	}

	// Coinbase must have at least one output
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("coinbase must have at least one output")
	}

	return nil
}

// ============================================================================
// Extended Validator with Chain Context
// ============================================================================

// ChainBlockValidator validates blocks with chain context.
type ChainBlockValidator struct {
	chainState *ChainState
}

// NewChainBlockValidator creates a new chain-aware block validator.
func NewChainBlockValidator(chainState *ChainState) *ChainBlockValidator {
	return &ChainBlockValidator{
		chainState: chainState,
	}
}

// ValidateBlockWithContext validates a block in the context of the chain.
func (v *ChainBlockValidator) ValidateBlockWithContext(block *Block) error {
	// Use chain state's validation
	return v.chainState.ValidateBlock(block)
}

// ValidateBlockForInsertion validates a block before insertion into the chain.
func (v *ChainBlockValidator) ValidateBlockForInsertion(block *Block) error {
	// Check height is not too far ahead
	currentHeight := v.chainState.GetBestBlockHeight()
	if block.Header.Height > currentHeight+1 {
		return fmt.Errorf("block height %d is too far ahead of current height %d",
			block.Header.Height, currentHeight)
	}

	// If height is 0, this is genesis
	if block.Header.Height == 0 {
		return v.validateGenesis(block)
	}

	// Check parent exists
	if !v.chainState.HasBlock(block.Header.Height - 1) {
		return fmt.Errorf("parent block at height %d not found", block.Header.Height-1)
	}

	// Get parent block
	parent, err := v.chainState.GetBlockByHeight(block.Header.Height - 1)
	if err != nil {
		return fmt.Errorf("failed to get parent block: %w", err)
	}

	// Verify previous block hash
	if block.Header.PrevBlockHash != parent.Hash {
		return fmt.Errorf("previous block hash does not match parent")
	}

	// Verify timestamp is after parent
	if block.Header.Timestamp <= parent.Header.Timestamp {
		return fmt.Errorf("block timestamp must be after parent")
	}

	// Verify height is correct
	if block.Header.Height != parent.Header.Height+1 {
		return fmt.Errorf("block height must be parent height + 1")
	}

	// Use full chain validation
	return v.chainState.ValidateBlock(block)
}

// validateGenesis validates a genesis block.
func (v *ChainBlockValidator) validateGenesis(block *Block) error {
	if !block.IsGenesis() {
		return fmt.Errorf("block at height 0 is not a genesis block")
	}

	// Genesis block should have no previous block hash
	if block.Header.PrevBlockHash != ([32]byte{}) {
		return fmt.Errorf("genesis block should have zero previous block hash")
	}

	return nil
}

// ============================================================================
// Validation Result Types
// ============================================================================

// ValidationError contains detailed validation error information.
type ValidationError struct {
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// ValidationErrorCodes defines common validation error codes.
var ValidationErrorCodes = struct {
	InvalidStructure   string
	InvalidTimestamp   string
	InvalidParent      string
	InvalidTransaction string
	InvalidSignature   string
	InvalidProposer    string
}{
	InvalidStructure:   "INVALID_STRUCTURE",
	InvalidTimestamp:   "INVALID_TIMESTAMP",
	InvalidParent:      "INVALID_PARENT",
	InvalidTransaction: "INVALID_TRANSACTION",
	InvalidSignature:   "INVALID_SIGNATURE",
	InvalidProposer:    "INVALID_PROPOSER",
}

// ============================================================================
// Validation Helpers
// ============================================================================

// IsValidTimestamp checks if a timestamp is valid for a new block.
//
// The parent-to-child gap has NO upper bound on purpose: after any outage
// longer than MaxBlockTimeDrift, the next block's gap to its parent is
// already history that cannot be fixed, so bounding it would deadlock the
// chain permanently. Monotonicity (after parent) plus the future bound are
// the safety-relevant properties; tip freshness is enforced separately by
// ChainState.validateBlockTimestamp for blocks near the chain tip.
func IsValidTimestamp(timestamp uint64, parentTimestamp uint64) bool {
	// Timestamp must be after parent
	if timestamp <= parentTimestamp {
		return false
	}

	// Timestamp must not be too far in the future
	now := uint64(time.Now().Unix())
	if timestamp > now+300 {
		return false
	}

	// Check minimum block time
	diff := time.Duration(timestamp-parentTimestamp) * time.Second
	if diff < MinBlockTime {
		return false
	}

	return true
}

// ValidateTransactionInputs validates all transaction inputs.
func ValidateTransactionInputs(tx *Transaction, utxoProvider UTXOProvider) error {
	if tx.IsCoinbase() {
		return nil // Coinbase has no inputs to validate
	}

	for i, input := range tx.Inputs {
		utxo, err := utxoProvider.GetUTXO(input.TxHash, input.Index)
		if err != nil {
			return fmt.Errorf("input %d: UTXO not found: %w", i, err)
		}

		// Verify the input public key matches the UTXO address
		if input.PublicKey == nil || len(input.PublicKey) == 0 {
			return fmt.Errorf("input %d: missing public key", i)
		}

		// Verify signature
		if !tx.VerifyInput(i) {
			return fmt.Errorf("input %d: invalid signature", i)
		}

		// Verify the public key matches the UTXO address
		derivedAddr := AddressFromPublicKey(input.PublicKey)
		if derivedAddr != utxo.Address {
			return fmt.Errorf("input %d: public key does not match UTXO address", i)
		}

		_ = utxo // UTXO value is used for fee calculation
	}

	return nil
}

// ValidateTransactionFees validates transaction fees meet minimum requirements.
func ValidateTransactionFees(tx *Transaction, utxoProvider UTXOProvider) error {
	if tx.IsCoinbase() {
		return nil // Coinbase doesn't have fees
	}

	fee, err := tx.GetFee(utxoProvider)
	if err != nil {
		return fmt.Errorf("failed to calculate fee: %w", err)
	}

	if fee < MinTransactionFee {
		return fmt.Errorf("fee %d below minimum %d", fee, MinTransactionFee)
	}

	return nil
}

// CheckForDoubleSpend checks if a transaction attempts to double spend.
func CheckForDoubleSpend(txs []*Transaction) error {
	seen := make(map[string]bool)

	for txIndex, tx := range txs {
		if tx.IsCoinbase() {
			continue
		}

		for inputIndex, input := range tx.Inputs {
			key := fmt.Sprintf("%x:%d", input.TxHash, input.Index)
			if seen[key] {
				return fmt.Errorf("double spend detected: tx %d input %d", txIndex, inputIndex)
			}
			seen[key] = true
		}
	}

	return nil
}

// ============================================================================
// Block Acceptance Rules
// ============================================================================

// BlockAcceptanceResult contains the result of block acceptance evaluation.
type BlockAcceptanceResult struct {
	CanAccept bool
	Reason    string
}

// EvaluateBlockAcceptance determines if a block should be accepted.
func (cs *ChainState) EvaluateBlockAcceptance(block *Block) *BlockAcceptanceResult {
	result := &BlockAcceptanceResult{CanAccept: true}

	// Check if block is already in chain
	if cs.HasBlock(block.Header.Height) {
		existing, err := cs.GetBlockByHeight(block.Header.Height)
		if err == nil && existing.Hash == block.Hash {
			result.CanAccept = false
			result.Reason = "block already in chain"
			return result
		}
	}

	// Check height
	if block.Header.Height > cs.bestHeight+1 {
		result.CanAccept = false
		result.Reason = fmt.Sprintf("block height %d is too far ahead (current: %d)",
			block.Header.Height, cs.bestHeight)
		return result
	}

	// For blocks at current height + 1, check if parent is best block
	if block.Header.Height == cs.bestHeight+1 {
		if block.Header.PrevBlockHash != cs.bestHash {
			result.CanAccept = false
			result.Reason = "block does not extend current best chain"
			return result
		}
	}

	return result
}

// ============================================================================
// PoAIW Validation (Version 2+)
// ============================================================================

// validatePoAIW validates PoAIW-specific fields for blocks with Version >= 2
func (v *DefaultBlockValidator) validatePoAIW(block *Block) error {
	// Only validate PoAIW for Version 2+ blocks
	if block.Header.Version < 2 {
		return nil
	}

	// For Version 2+, InferencePoW must be present
	if len(block.Header.InferencePoW) == 0 {
		return fmt.Errorf("PoAIW: InferencePoW is required for Version 2+ blocks")
	}

	// ModelID must be specified
	if block.Header.ModelID == "" {
		return fmt.Errorf("PoAIW: ModelID is required for Version 2+ blocks")
	}

	// Verify PoW hash meets difficulty requirement
	if !verifyInferencePoW(block.Header) {
		return fmt.Errorf("PoAIW: InferencePoW verification failed")
	}

	// Verify proposer has minimum stake
	if !v.verifyMinStake(block.Header.Proposer) {
		return fmt.Errorf("PoAIW: proposer has insufficient stake")
	}

	return nil
}

// verifyMinStake checks if the proposer has at least MinStakeV2 staked.
func (v *DefaultBlockValidator) verifyMinStake(proposer [32]byte) bool {
	if v.consensus == nil {
		return true // no consensus state available, skip check
	}
	validator, err := v.consensus.GetValidator(proposer)
	if err != nil {
		return false
	}
	return validator.Stake >= MinStakeV2Satoshi
}

// verifyInferencePoW verifies the inference proof-of-work hash.
// It recomputes the expected PoW from the block header fields and checks it matches.
func verifyInferencePoW(header BlockHeader) bool {
	if len(header.InferencePoW) < 32 {
		return false
	}
	expected := GenerateInferencePoW(header.Proposer, header.PrevBlockHash, header.Height)
	if len(expected) != len(header.InferencePoW) {
		return false
	}
	for i := range expected {
		if expected[i] != header.InferencePoW[i] {
			return false
		}
	}
	return true
}
