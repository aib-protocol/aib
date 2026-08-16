// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains the validator implementation for eUTXO transactions.
package eutxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
)

// ExecutionResult contains the result of script execution.
type ExecutionResult struct {
	Valid   bool
	Error   error
	ExUnits ExUnits
}

// ValidationContext contains all information needed for validation.
type ValidationContext struct {
	Slot       uint64      // Current slot number
	TxInputs   []*eUTXO    // UTXOs being spent (resolved from UTXO set)
	TxOutputs  []eTXOutput // Outputs being created
	TxHash     [32]byte    // Transaction hash
	Signatures [][]byte    // Additional signatures
}

// eUTXOValidator validates eUTXO transactions.
type eUTXOValidator struct {
	MinFee          uint64 // Minimum transaction fee
	MinDatumSize    uint32 // Minimum datum size
	MaxDatumSize    uint32 // Maximum datum size (0 = no limit)
	MaxScriptSize   uint32 // Maximum script size (0 = no limit)
	MaxTxSize       uint32 // Maximum transaction size in bytes
	MinTxValue      uint64 // Minimum value for a transaction output
	MaxUTXOPerAddr  int    // Maximum UTXOs per address (0 = no limit)
}

// DefaultValidator returns a validator with default parameters.
func DefaultValidator() *eUTXOValidator {
	return &eUTXOValidator{
		MinFee:         1000000,      // 1 ADA minimum fee
		MinDatumSize:   0,            // No minimum
		MaxDatumSize:   16384,        // 16KB max datum
		MaxScriptSize:  16384,        // 16KB max script
		MaxTxSize:      16384,        // 16KB max transaction
		MinTxValue:     1000000,      // 1 ADA minimum output
		MaxUTXOPerAddr: 100,          // Max 100 UTXOs per address
	}
}

// ValidationResult contains the result of validation.
type ValidationResult struct {
	Valid       bool
	FailedRule  string
	InputIndex  int
	OutputIndex int
	Error       error
}

// Validate performs full validation of an eUTXO transaction.
func (v *eUTXOValidator) Validate(
	tx *eUTXOTransaction,
	utxoSet UTXOSet,
	ctx *ValidationContext,
) *ValidationResult {

	// 1. Basic transaction validation
	if result := v.validateBasic(tx); result != nil {
		return result
	}

	// 2. Validate inputs exist and are unspent
	if result := v.validateInputs(tx, utxoSet); result != nil {
		return result
	}

	// 3. Validate outputs
	if result := v.validateOutputs(tx); result != nil {
		return result
	}

	// 4. Resource conservation (inputs >= outputs + fee)
	if result := v.validateValueConservation(tx); result != nil {
		return result
	}

	// 5. Time lock validation
	if result := v.validateTimeLocks(tx, ctx); result != nil {
		return result
	}

	// 6. Script validation
	if result := v.validateScripts(tx, utxoSet, ctx); result != nil {
		return result
	}

	// 7. Datum integrity validation
	if result := v.validateDatums(tx, utxoSet); result != nil {
		return result
	}

	return &ValidationResult{Valid: true}
}

// validateBasic validates basic transaction structure.
func (v *eUTXOValidator) validateBasic(tx *eUTXOTransaction) *ValidationResult {
	// Check version
	if tx.Version == 0 {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "INVALID_VERSION",
			Error:      ErrInvalidVersion,
		}
	}

	// Check inputs not empty
	if len(tx.Inputs) == 0 {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "NO_INPUTS",
			Error:      ErrNoInputs,
		}
	}

	// Check outputs not empty
	if len(tx.Outputs) == 0 {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "NO_OUTPUTS",
			Error:      ErrNoOutputs,
		}
	}

	// Check fee is sufficient
	if tx.Fee < v.MinFee {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "INSUFFICIENT_FEE",
			Error:      ErrInsufficientFee,
		}
	}

	return nil
}

// validateInputs validates that all inputs reference valid, unspent UTXOs.
func (v *eUTXOValidator) validateInputs(tx *eUTXOTransaction, utxoSet UTXOSet) *ValidationResult {
	for i, in := range tx.Inputs {
		key := UTXOKey{TxID: in.TxID, Index: in.Index}
		utxo, ok := utxoSet.Get(key)

		if !ok {
			return &ValidationResult{
				Valid:       false,
				FailedRule:  "INPUT_NOT_FOUND",
				InputIndex:  i,
				Error:       ErrInputNotFound,
			}
		}

		if utxo.IsSpent {
			return &ValidationResult{
				Valid:       false,
				FailedRule:  "INPUT_SPENT",
				InputIndex:  i,
				Error:       ErrInputSpent,
			}
		}

		// Verify value matches (prevent value manipulation)
		if in.Value != utxo.Value {
			return &ValidationResult{
				Valid:       false,
				FailedRule:  "INPUT_VALUE_MISMATCH",
				InputIndex:  i,
				Error:       ErrInputValueMismatch,
			}
		}

		// Verify address matches (prevent address spoofing)
		if !bytes.Equal(in.Address.Hash[:], utxo.Address.Hash[:]) {
			return &ValidationResult{
				Valid:       false,
				FailedRule:  "INPUT_ADDRESS_MISMATCH",
				InputIndex:  i,
				Error:       ErrInputAddressMismatch,
			}
		}
	}

	return nil
}

// validateOutputs validates transaction outputs.
func (v *eUTXOValidator) validateOutputs(tx *eUTXOTransaction) *ValidationResult {
	for i, out := range tx.Outputs {
		// Check output value is positive
		if out.Value == 0 {
			return &ValidationResult{
				Valid:        false,
				FailedRule:   "ZERO_OUTPUT_VALUE",
				OutputIndex:  i,
				Error:        ErrZeroOutputValue,
			}
		}

		// Check minimum output value
		if out.Value < v.MinTxValue && !isZeroValueOutput(out) {
			return &ValidationResult{
				Valid:        false,
				FailedRule:   "OUTPUT_VALUE_TOO_SMALL",
				OutputIndex:  i,
				Error:        ErrOutputValueTooSmall,
			}
		}

		// Check datum size
		if v.MaxDatumSize > 0 && len(out.Datum) > int(v.MaxDatumSize) {
			return &ValidationResult{
				Valid:        false,
				FailedRule:   "DATUM_TOO_LARGE",
				OutputIndex:  i,
				Error:        ErrDatumTooLarge,
			}
		}

		// Check script size
		if v.MaxScriptSize > 0 && len(out.Script) > int(v.MaxScriptSize) {
			return &ValidationResult{
				Valid:        false,
				FailedRule:   "SCRIPT_TOO_LARGE",
				OutputIndex:  i,
				Error:        ErrScriptTooLarge,
			}
		}

		// Validate address
		if err := ValidateAddress(out.Address); err != nil {
			return &ValidationResult{
				Valid:        false,
				FailedRule:   "INVALID_OUTPUT_ADDRESS",
				OutputIndex:  i,
				Error:        err,
			}
		}

		// Check datum hash consistency
		if len(out.Datum) > 0 {
			computedHash := sha256.Sum256(out.Datum)
			if !bytes.Equal(computedHash[:], out.DatumHash[:]) {
				return &ValidationResult{
					Valid:        false,
					FailedRule:   "DATUM_HASH_MISMATCH",
					OutputIndex:  i,
					Error:        ErrDatumHashMismatch,
				}
			}
		}

		// Check script hash consistency
		if len(out.Script) > 0 {
			computedHash := sha256.Sum256(out.Script)
			if !bytes.Equal(computedHash[:], out.ScriptHash[:]) {
				return &ValidationResult{
					Valid:        false,
					FailedRule:   "SCRIPT_HASH_MISMATCH",
					OutputIndex:  i,
					Error:        ErrScriptHashMismatch,
				}
			}
		}
	}

	return nil
}

// validateValueConservation ensures inputs >= outputs + fee.
func (v *eUTXOValidator) validateValueConservation(tx *eUTXOTransaction) *ValidationResult {
	inputValue := tx.GetInputValue()
	outputValue := tx.GetOutputValue()
	totalNeeded := outputValue + tx.Fee

	if inputValue < totalNeeded {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "VALUE_NOT_CONSERVED",
			Error:      ErrValueNotConserved,
		}
	}

	return nil
}

// validateTimeLocks validates time lock constraints.
func (v *eUTXOValidator) validateTimeLocks(tx *eUTXOTransaction, ctx *ValidationContext) *ValidationResult {
	currentSlot := ctx.Slot

	// Check ValidAfter (transaction not valid before this slot)
	if tx.ValidAfter > 0 && currentSlot < tx.ValidAfter {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "VALIDITY_INTERVAL_NOT_STARTED",
			Error:      ErrValidityIntervalNotStarted,
		}
	}

	// Check TTL (transaction must be valid before this slot)
	if tx.TTL > 0 && currentSlot >= tx.TTL {
		return &ValidationResult{
			Valid:      false,
			FailedRule: "TTL_EXPIRED",
			Error:      ErrTTLExpired,
		}
	}

	return nil
}

// validateScripts executes validator scripts for each input.
func (v *eUTXOValidator) validateScripts(
	tx *eUTXOTransaction,
	utxoSet UTXOSet,
	ctx *ValidationContext,
) *ValidationResult {

	for i, in := range tx.Inputs {
		key := UTXOKey{TxID: in.TxID, Index: in.Index}
		utxo, ok := utxoSet.Get(key)
		if !ok {
			continue // Already checked in validateInputs
		}

		// Get the script to execute (prefer input script, fallback to UTXO script)
		script := in.Script
		if len(script) == 0 {
			script = utxo.Script
		}

		// If no script, this is a pubkey (P2PK) style input
		if len(script) == 0 {
			// Validate signature for P2PK inputs
			if len(in.Signature) > 0 && len(in.PubKey) > 0 {
				if !v.verifySignature(in.PubKey, in.Signature, ctx.TxHash[:]) {
					return &ValidationResult{
						Valid:       false,
						FailedRule:  "INVALID_SIGNATURE",
						InputIndex:  i,
						Error:       ErrInvalidSignature,
					}
				}
			}
			continue
		}

		// Build validation context for script
		scriptCtx := v.buildScriptContext(tx, utxo, &in, i)

		// Execute the validator script
		result := v.executeScript(script, utxo.Datum, in.Redeemer, scriptCtx)
		if !result.Valid {
			return &ValidationResult{
				Valid:       false,
				FailedRule:  "SCRIPT_VALIDATION_FAILED",
				InputIndex:  i,
				Error:       result.Error,
			}
		}

		// Check execution budget
		if in.Redeemer != nil {
			if result.ExUnits.Steps > MaxExUnits.Steps || result.ExUnits.Mem > MaxExUnits.Mem {
				return &ValidationResult{
					Valid:       false,
					FailedRule:  "EX_UNITS_EXCEEDED",
					InputIndex:  i,
					Error:       ErrExUnitsExceeded,
				}
			}
		}
	}

	return nil
}

// validateDatums validates datum integrity.
func (v *eUTXOValidator) validateDatums(tx *eUTXOTransaction, utxoSet UTXOSet) *ValidationResult {
	for i, in := range tx.Inputs {
		key := UTXOKey{TxID: in.TxID, Index: in.Index}
		utxo, ok := utxoSet.Get(key)
		if !ok {
			continue // Already checked
		}

		// Get datum (prefer input datum, fallback to UTXO inline datum)
		datum := in.Datum
		if len(datum) == 0 {
			datum = utxo.Datum
		}

		// If UTXO has a datum hash, verify it matches
		if !isZeroHashUTXO(utxo.DatumHash[:]) {
			if len(datum) == 0 {
				return &ValidationResult{
					Valid:       false,
					FailedRule:  "DATUM_MISSING",
					InputIndex:  i,
					Error:       ErrDatumMissing,
				}
			}
			computedHash := sha256.Sum256(datum)
			if !bytes.Equal(computedHash[:], utxo.DatumHash[:]) {
				return &ValidationResult{
					Valid:       false,
					FailedRule:  "DATUM_HASH_MISMATCH",
					InputIndex:  i,
					Error:       ErrDatumHashMismatch,
				}
			}
		}
	}

	return nil
}

// verifySignature verifies an Ed25519 signature.
func (v *eUTXOValidator) verifySignature(pubKey, signature, message []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pubKey, message, signature)
}

// buildScriptContext builds the validation context for script execution.
func (v *eUTXOValidator) buildScriptContext(
	tx *eUTXOTransaction,
	utxo *eUTXO,
	input *eTXInput,
	inputIndex int,
) []byte {
	var buf bytes.Buffer

	// Write transaction hash
	buf.Write(tx.Hash[:])

	// Write current slot
	binary.Write(&buf, binary.LittleEndian, tx.Slot)

	// Write input index
	binary.Write(&buf, binary.LittleEndian, uint32(inputIndex))

	// Write input value
	binary.Write(&buf, binary.LittleEndian, input.Value)

	// Write output count
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Outputs)))

	// Write output values
	for _, out := range tx.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Value)
	}

	return buf.Bytes()
}

// executeScript executes a validator script and returns the result.
func (v *eUTXOValidator) executeScript(
	script []byte,
	datum []byte,
	redeemer *Redeemer,
	context []byte,
) *ExecutionResult {

	// Determine script type from first byte
	if len(script) == 0 {
		return &ExecutionResult{
			Valid:   false,
			Error:   ErrEmptyScript,
		}
	}

	scriptType := ScriptType(script[0])
	_ = script[1:] // scriptBody (not used for built-in scripts)

	var validator Validator
	switch scriptType {
	case ScriptP2PKH:
		validator = ValidateP2PKH
	case ScriptMultiSig:
		validator = ValidateMultiSig
	case ScriptTimelock:
		validator = ValidateTimelock
	case ScriptEscrow:
		validator = ValidateEscrow
	default:
		// Unknown script type - try to execute as-is
		return &ExecutionResult{
			Valid:    true, // Allow unknown scripts (fail-open for extensibility)
			ExUnits:  ExUnits{Steps: 1000, Mem: 10000},
		}
	}

	// Prepare redeemer data
	var redeemerData []byte
	if redeemer != nil {
		redeemerData = redeemer.Data
	}

	// Execute validator
	valid := validator(datum, redeemerData, context)

	return &ExecutionResult{
		Valid:   valid,
		ExUnits: ExUnits{Steps: 10000, Mem: 100000}, // Estimated execution cost
	}
}

// isZeroValueOutput checks if an output is a zero-value placeholder.
func isZeroValueOutput(out eTXOutput) bool {
	return out.Value == 0 && len(out.Datum) == 0 && len(out.Script) == 0
}

// isZeroHashUTXO checks if a hash is all zeros (validator version).
func isZeroHashUTXO(h []byte) bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}
