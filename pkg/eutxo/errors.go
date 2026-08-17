// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains error definitions and common constants.
package eutxo

import (
	"errors"
	"fmt"
)

// ============================================================================
// Error Definitions
// ============================================================================

// Basic validation errors
var (
	ErrInvalidVersion             = errors.New("invalid transaction version")
	ErrNoInputs                   = errors.New("transaction has no inputs")
	ErrNoOutputs                  = errors.New("transaction has no outputs")
	ErrInsufficientFee            = errors.New("transaction fee is below minimum")
	ErrInputNotFound              = errors.New("input UTXO not found")
	ErrInputSpent                 = errors.New("input UTXO is already spent")
	ErrInputValueMismatch         = errors.New("input value does not match UTXO")
	ErrInputAddressMismatch       = errors.New("input address does not match UTXO")
	ErrZeroOutputValue            = errors.New("output value cannot be zero")
	ErrOutputValueTooSmall        = errors.New("output value is below minimum")
	ErrDatumTooLarge              = errors.New("datum size exceeds maximum")
	ErrScriptTooLarge             = errors.New("script size exceeds maximum")
	ErrInvalidAddress             = errors.New("invalid address format")
	ErrValueNotConserved          = errors.New("inputs do not cover outputs + fee")
	ErrValidityIntervalNotStarted = errors.New("transaction validity interval has not started")
	ErrTTLExpired                 = errors.New("transaction TTL has expired")
	ErrInvalidSignature           = errors.New("invalid signature")
	ErrDatumMissing               = errors.New("datum is required but missing")
	ErrDatumHashMismatch          = errors.New("datum hash does not match data")
	ErrScriptHashMismatch         = errors.New("script hash does not match data")
	ErrUTXONotFound               = errors.New("UTXO not found in set")
	ErrUTXOSpent                  = errors.New("UTXO is already spent")
)

// Script execution errors
var (
	ErrEmptyScript           = errors.New("script is empty")
	ErrInvalidScriptType     = errors.New("invalid script type")
	ErrScriptExecutionFailed = errors.New("script execution failed")
	ErrExUnitsExceeded       = errors.New("execution units exceeded")
	ErrExStepsExceeded       = errors.New("execution steps exceeded")
	ErrExMemExceeded         = errors.New("execution memory exceeded")
)

// Datum and redeemer errors
var (
	ErrInvalidDatum            = errors.New("invalid datum")
	ErrEmptyDatum              = errors.New("datum is empty")
	ErrInvalidDatumEncoding    = errors.New("invalid datum encoding")
	ErrInvalidJSONDatum        = errors.New("invalid JSON datum")
	ErrJSONDatumTooLarge       = errors.New("JSON datum too large")
	ErrInvalidCBORDatum        = errors.New("invalid CBOR datum")
	ErrInvalidCBORConstructor  = errors.New("invalid CBOR constructor")
	ErrCBORTooManyFields       = errors.New("CBOR datum has too many fields")
	ErrInvalidPlutusDatum      = errors.New("invalid Plutus datum")
	ErrInvalidPlutusType       = errors.New("invalid Plutus type")
	ErrPlutusTooManyFields     = errors.New("Plutus datum has too many fields")
	ErrPlutusTooManyPairs      = errors.New("Plutus map has too many pairs")
	ErrPlutusTooManyElements   = errors.New("Plutus list has too many elements")
	ErrPlutusIntOutOfRange     = errors.New("Plutus integer out of range")
	ErrPlutusBytesTooLarge     = errors.New("Plutus bytes too large")
	ErrInvalidCBORFormat       = errors.New("invalid CBOR format")
	ErrInvalidPlutusDataFormat = errors.New("invalid Plutus Data format")
	ErrInvalidPlutusDataTag    = errors.New("invalid Plutus Data tag")
)

// Redeemer errors
var (
	ErrInvalidRedeemer       = errors.New("invalid redeemer")
	ErrEmptyRedeemer         = errors.New("redeemer is empty")
	ErrInvalidRedeemerTag    = errors.New("invalid redeemer tag")
	ErrInvalidRedeemerFormat = errors.New("invalid redeemer format")
	ErrRedeemerTooLarge      = errors.New("redeemer too large")
)

// Transaction structure errors
var (
	ErrInvalidTransaction  = errors.New("invalid transaction structure")
	ErrTransactionTooLarge = errors.New("transaction exceeds maximum size")
	ErrTooManyInputs       = errors.New("transaction has too many inputs")
	ErrTooManyOutputs      = errors.New("transaction has too many outputs")
)

// ============================================================================
// Common Constants
// ============================================================================

// Maximum sizes
const (
	MaxTxSize       = 16384 // 16KB maximum transaction size
	MaxInputs       = 255   // Maximum inputs per transaction
	MaxOutputs      = 255   // Maximum outputs per transaction
	MaxScriptSize   = 16384 // 16KB maximum script size
	MaxRedeemerSize = 8192  // 8KB maximum redeemer size
)

// Execution limits
const (
	MaxExUnitsSteps = 10000000 // 10M maximum execution steps
	MaxExUnitsMem   = 1000000  // 1M maximum execution memory
)

// Address and key sizes
const (
	AddressSize       = 28 // Address payload size in bytes
	HashSize          = 32 // SHA256 hash size
	TxIDSize          = 32 // Transaction ID size
	Ed25519PubKeySize = 32 // Ed25519 public key size
	Ed25519SigSize    = 64 // Ed25519 signature size
)

// Script constants
const (
	ScriptHeaderSize = 1 // Script type byte
	ScriptTypeOffset = 0 // Offset of script type in script
)

// ============================================================================
// Utility Functions
// ============================================================================

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	switch err {
	case ErrInvalidVersion, ErrNoInputs, ErrNoOutputs, ErrInsufficientFee,
		ErrInputNotFound, ErrInputSpent, ErrInputValueMismatch, ErrInputAddressMismatch,
		ErrZeroOutputValue, ErrOutputValueTooSmall, ErrDatumTooLarge, ErrScriptTooLarge,
		ErrInvalidAddress, ErrValueNotConserved, ErrValidityIntervalNotStarted,
		ErrTTLExpired, ErrInvalidSignature, ErrDatumMissing, ErrDatumHashMismatch,
		ErrScriptHashMismatch, ErrUTXONotFound, ErrUTXOSpent:
		return true
	default:
		return false
	}
}

// IsScriptError checks if an error is a script execution error.
func IsScriptError(err error) bool {
	switch err {
	case ErrEmptyScript, ErrInvalidScriptType, ErrScriptExecutionFailed,
		ErrExUnitsExceeded, ErrExStepsExceeded, ErrExMemExceeded:
		return true
	default:
		return false
	}
}

// IsDatumError checks if an error is a datum-related error.
func IsDatumError(err error) bool {
	switch err {
	case ErrInvalidDatum, ErrEmptyDatum, ErrInvalidDatumEncoding,
		ErrInvalidJSONDatum, ErrJSONDatumTooLarge, ErrInvalidCBORDatum,
		ErrInvalidCBORConstructor, ErrCBORTooManyFields, ErrInvalidPlutusDatum,
		ErrInvalidPlutusType, ErrPlutusTooManyFields, ErrPlutusTooManyPairs,
		ErrPlutusTooManyElements, ErrPlutusIntOutOfRange, ErrPlutusBytesTooLarge,
		ErrInvalidCBORFormat, ErrInvalidPlutusDataFormat, ErrInvalidPlutusDataTag:
		return true
	default:
		return false
	}
}

// IsRedeemerError checks if an error is a redeemer-related error.
func IsRedeemerError(err error) bool {
	switch err {
	case ErrInvalidRedeemer, ErrEmptyRedeemer, ErrInvalidRedeemerTag,
		ErrInvalidRedeemerFormat, ErrRedeemerTooLarge:
		return true
	default:
		return false
	}
}

// IsTransactionError checks if an error is a transaction structure error.
func IsTransactionError(err error) bool {
	switch err {
	case ErrInvalidTransaction, ErrTransactionTooLarge, ErrTooManyInputs, ErrTooManyOutputs:
		return true
	default:
		return false
	}
}

// ErrorWithCode wraps an error with additional context.
type ErrorWithCode struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface.
func (e *ErrorWithCode) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// NewErrorWithCode creates a new ErrorWithCode.
func NewErrorWithCode(code, message string, err error) *ErrorWithCode {
	return &ErrorWithCode{
		Code:    code,
		Message: message,
		Err:     err,
	}
}
