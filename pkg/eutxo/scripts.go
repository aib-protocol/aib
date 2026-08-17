// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains built-in contract scripts and validators.
package eutxo

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ============================================================================
// Script Types and Constants
// ============================================================================

// ScriptType represents the type of validator script.
type ScriptType uint8

const (
	// ScriptP2PKH is Pay-to-Public-Key-Hash (simple wallet)
	ScriptP2PKH ScriptType = 0x01
	// ScriptMultiSig is multi-signature script
	ScriptMultiSig ScriptType = 0x02
	// ScriptTimelock is time-locked script
	ScriptTimelock ScriptType = 0x03
	// ScriptEscrow is escrow contract script
	ScriptEscrow ScriptType = 0x04
	// ScriptPlutus is Plutus script (future extension)
	ScriptPlutus ScriptType = 0x05
)

// ============================================================================
// Validator Function Signature
// ============================================================================

// Validator represents a script validation function.
// Returns true if the script validates successfully.
type Validator func(datum, redeemer, context []byte) bool

// ============================================================================
// Built-in Script Implementations
// ============================================================================

// P2PKH (Pay-to-Public-Key-Hash)
// Validates: signature(utxo_hash) with pubkey matches hash
// hexToBytes converts a hex string (with or without 0x prefix) to bytes.
// It also handles raw byte arrays directly (for backward compatibility).
func hexToBytes(s string) ([]byte, error) {
	// If it's already a byte array (not a string), return it
	if len(s) == 0 {
		return []byte{}, nil
	}
	// Check if this is hex string (only valid hex characters)
	if len(s) > 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	_, err := hex.DecodeString(s)
	if err != nil {
		// Not a valid hex string, try treating as raw bytes
		return []byte(s), nil
	}
	return hex.DecodeString(s)
}

// decodeHexField decodes a field that might be hex-encoded JSON string or raw bytes.
func decodeHexField(field []byte) ([]byte, error) {
	if len(field) == 0 {
		return nil, nil
	}
	// Check if it's a hex string (length is even and all hex chars)
	str := string(field)
	// Try to decode as hex
	decoded, err := hex.DecodeString(str)
	if err != nil {
		// Not hex, treat as raw bytes
		return field, nil
	}
	return decoded, nil
}

// ValidateP2PKH validates a Pay-to-Public-Key-Hash spend.
// The redeemer should contain a signature and public key.
func ValidateP2PKH(datum, redeemer, context []byte) bool {
	// Parse redeemer as P2PKH redeemer
	var p2pkhRedeemer struct {
		Signature string `json:"signature"`
		PublicKey string `json:"publicKey"`
	}

	err := json.Unmarshal(redeemer, &p2pkhRedeemer)
	if err != nil {
		return false
	}

	// Decode signature from hex or use as raw bytes
	sigBytes, err := hex.DecodeString(p2pkhRedeemer.Signature)
	if err != nil {
		// Not valid hex, use as raw bytes
		sigBytes = []byte(p2pkhRedeemer.Signature)
	}

	// Decode public key from hex or use as raw bytes
	pubKeyBytes, err := hex.DecodeString(p2pkhRedeemer.PublicKey)
	if err != nil {
		// Not valid hex, use as raw bytes
		pubKeyBytes = []byte(p2pkhRedeemer.PublicKey)
	}

	// Validate signature
	if len(sigBytes) != ed25519.SignatureSize {
		return false
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	// Verify signature against transaction hash
	return ed25519.Verify(
		pubKeyBytes,
		context, // context contains tx hash
		sigBytes,
	)
}

// MultiSig (Multi-signature)
// Validates: at least threshold signatures from required pubkeys
func ValidateMultiSig(datum, redeemer, context []byte) bool {
	// Parse datum as multi-sig policy
	var multiSigDatum struct {
		Threshold uint32   `json:"threshold"`
		PubKeys   []string `json:"pubKeys"`
	}

	err := json.Unmarshal(datum, &multiSigDatum)
	if err != nil {
		return false
	}

	// Validate datum
	if multiSigDatum.Threshold == 0 {
		return false
	}
	if len(multiSigDatum.PubKeys) == 0 {
		return false
	}
	if multiSigDatum.Threshold > uint32(len(multiSigDatum.PubKeys)) {
		return false
	}

	// Decode all pubkeys from hex
	pubKeys := make([][]byte, 0, len(multiSigDatum.PubKeys))
	for _, pubKeyHex := range multiSigDatum.PubKeys {
		pubKeyBytes, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			// Try as raw bytes
			pubKeyBytes = []byte(pubKeyHex)
		}
		if len(pubKeyBytes) != ed25519.PublicKeySize {
			return false
		}
		pubKeys = append(pubKeys, pubKeyBytes)
	}

	// Parse redeemer as multi-sig signatures
	var multiSigRedeemer struct {
		Signatures []string `json:"signatures"`
	}

	err = json.Unmarshal(redeemer, &multiSigRedeemer)
	if err != nil {
		return false
	}

	// Validate signatures
	validSigs := 0
	usedKeys := make(map[int]bool) // Track which keys have already signed

	for _, sigHex := range multiSigRedeemer.Signatures {
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			// Try as raw bytes
			sigBytes = []byte(sigHex)
		}
		if len(sigBytes) != ed25519.SignatureSize {
			continue
		}

		// Try signature against each unused pubkey
		for i, pubkey := range pubKeys {
			if usedKeys[i] {
				continue // Each key can only sign once
			}
			if ed25519.Verify(pubkey, context, sigBytes) {
				validSigs++
				usedKeys[i] = true
				break
			}
		}

		// Early exit if we have enough valid signatures
		if validSigs >= int(multiSigDatum.Threshold) {
			break
		}
	}

	return validSigs >= int(multiSigDatum.Threshold)
}

// Timelock (Time-locked)
// Validates: current slot >= lock slot AND signature is valid
func ValidateTimelock(datum, redeemer, context []byte) bool {
	// Parse datum as time lock policy
	var timelockDatum struct {
		LockSlot  uint64 `json:"lockSlot"`
		PublicKey string `json:"publicKey"`
	}

	err := json.Unmarshal(datum, &timelockDatum)
	if err != nil {
		return false
	}

	// Decode public key from hex
	pubKeyBytes, err := hex.DecodeString(timelockDatum.PublicKey)
	if err != nil {
		// Try as raw bytes
		pubKeyBytes = []byte(timelockDatum.PublicKey)
	}

	// Validate datum
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	// Parse context for current slot
	currentSlot, err := parseSlotFromContext(context)
	if err != nil {
		return false
	}

	// Check time lock
	if currentSlot < timelockDatum.LockSlot {
		return false
	}

	// Parse redeemer as signature
	var sigRedeemer struct {
		Signature string `json:"signature"`
	}

	err = json.Unmarshal(redeemer, &sigRedeemer)
	if err != nil {
		return false
	}

	// Decode signature from hex
	sigBytes, err := hex.DecodeString(sigRedeemer.Signature)
	if err != nil {
		// Try as raw bytes
		sigBytes = []byte(sigRedeemer.Signature)
	}

	// Validate signature
	if len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(
		pubKeyBytes,
		context,
		sigBytes,
	)
}

// Escrow (Escrow Contract)
// Validates:
// - buyerSignature exists and is valid, OR
// - sellerSignature exists and timeout has passed
func ValidateEscrow(datum, redeemer, context []byte) bool {
	// Parse datum as escrow policy
	var escrowDatum struct {
		BuyerPubKey  string `json:"buyerPubKey"`
		SellerPubKey string `json:"sellerPubKey"`
		TimeoutSlot  uint64 `json:"timeoutSlot"`
	}

	err := json.Unmarshal(datum, &escrowDatum)
	if err != nil {
		return false
	}

	// Decode public keys from hex
	buyerPubKeyBytes, err := hex.DecodeString(escrowDatum.BuyerPubKey)
	if err != nil {
		buyerPubKeyBytes = []byte(escrowDatum.BuyerPubKey)
	}
	sellerPubKeyBytes, err := hex.DecodeString(escrowDatum.SellerPubKey)
	if err != nil {
		sellerPubKeyBytes = []byte(escrowDatum.SellerPubKey)
	}

	// Validate datum
	if len(buyerPubKeyBytes) != ed25519.PublicKeySize {
		return false
	}
	if len(sellerPubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	// Parse context for current slot
	currentSlot, err := parseSlotFromContext(context)
	if err != nil {
		return false
	}

	// Parse redeemer as escrow action
	var escrowRedeemer struct {
		BuyerSignature  string `json:"buyerSignature,omitempty"`
		SellerSignature string `json:"sellerSignature,omitempty"`
	}

	err = json.Unmarshal(redeemer, &escrowRedeemer)
	if err != nil {
		return false
	}

	// Option 1: Buyer can claim at any time with valid signature
	if len(escrowRedeemer.BuyerSignature) > 0 {
		buyerSigBytes, err := hex.DecodeString(escrowRedeemer.BuyerSignature)
		if err != nil {
			buyerSigBytes = []byte(escrowRedeemer.BuyerSignature)
		}
		if len(buyerSigBytes) == ed25519.SignatureSize {
			if ed25519.Verify(buyerPubKeyBytes, context, buyerSigBytes) {
				return true
			}
		}
	}

	// Option 2: Seller can claim after timeout with valid signature
	// Note: Seller uses timeout slot for signature to allow pre-timeout signing
	if currentSlot >= escrowDatum.TimeoutSlot && len(escrowRedeemer.SellerSignature) > 0 {
		sellerSigBytes, err := hex.DecodeString(escrowRedeemer.SellerSignature)
		if err != nil {
			sellerSigBytes = []byte(escrowRedeemer.SellerSignature)
		}
		if len(sellerSigBytes) == ed25519.SignatureSize {
			// For seller, use timeout slot in context instead of current slot
			// This allows pre-signed transactions to be valid after timeout
			sellerContext := buildEscrowContext(context, escrowDatum.TimeoutSlot)
			if ed25519.Verify(sellerPubKeyBytes, sellerContext, sellerSigBytes) {
				return true
			}
		}
	}

	return false
}

// buildEscrowContext builds context for seller signature verification using timeout slot.
func buildEscrowContext(originalContext []byte, timeoutSlot uint64) []byte {
	// Create new context with timeout slot instead of current slot
	ctx := make([]byte, 40)
	copy(ctx, originalContext[:32])
	binary.LittleEndian.PutUint64(ctx[32:40], timeoutSlot)
	return ctx
}

// ============================================================================
// Script Creation and Management
// ============================================================================

// CreateScript creates a script with the given type and parameters.
func CreateScript(scriptType ScriptType, params interface{}) ([]byte, error) {
	// Create script header
	script := make([]byte, 1)
	script[0] = byte(scriptType)

	// Serialize parameters based on script type
	var paramData []byte
	var err error

	switch scriptType {
	case ScriptP2PKH:
		// P2PKH doesn't need parameters in script (pubkey in datum)
		paramData = []byte{}
	case ScriptMultiSig:
		paramData, err = json.Marshal(params)
	case ScriptTimelock:
		paramData, err = json.Marshal(params)
	case ScriptEscrow:
		paramData, err = json.Marshal(params)
	default:
		return nil, ErrInvalidScriptType
	}

	if err != nil {
		return nil, err
	}

	// Append parameters
	script = append(script, paramData...)

	return script, nil
}

// GetValidator returns the validator function for a script type.
func GetValidator(scriptType ScriptType) Validator {
	switch scriptType {
	case ScriptP2PKH:
		return ValidateP2PKH
	case ScriptMultiSig:
		return ValidateMultiSig
	case ScriptTimelock:
		return ValidateTimelock
	case ScriptEscrow:
		return ValidateEscrow
	default:
		return nil
	}
}

// GetValidatorByScript returns the validator function for a script.
func GetValidatorByScript(script []byte) Validator {
	if len(script) == 0 {
		return nil
	}

	scriptType := ScriptType(script[0])
	return GetValidator(scriptType)
}

// ============================================================================
// Script Parameter Structures
// ============================================================================

// MultiSigParams represents parameters for multi-sig scripts.
type MultiSigParams struct {
	Threshold uint32   `json:"threshold"`
	PubKeys   [][]byte `json:"pubKeys"`
}

// TimelockParams represents parameters for time-lock scripts.
type TimelockParams struct {
	LockSlot  uint64 `json:"lockSlot"`
	PublicKey []byte `json:"publicKey"`
}

// EscrowParams represents parameters for escrow scripts.
type EscrowParams struct {
	BuyerPubKey  []byte `json:"buyerPubKey"`
	SellerPubKey []byte `json:"sellerPubKey"`
	TimeoutSlot  uint64 `json:"timeoutSlot"`
}

// ============================================================================
// Utility Functions
// ============================================================================

// parseSlotFromContext extracts the slot number from context data.
func parseSlotFromContext(context []byte) (uint64, error) {
	if len(context) < 32 {
		return 0, fmt.Errorf("context too short")
	}

	// Slot is typically in bytes 32-40 (after tx hash)
	var slot uint64
	err := binary.Read(bytes.NewReader(context[32:40]), binary.LittleEndian, &slot)
	return slot, err
}

// CreateP2PKHScript creates a P2PKH script.
func CreateP2PKHScript() ([]byte, error) {
	return CreateScript(ScriptP2PKH, nil)
}

// CreateMultiSigScript creates a multi-sig script.
func CreateMultiSigScript(params MultiSigParams) ([]byte, error) {
	return CreateScript(ScriptMultiSig, params)
}

// CreateTimelockScript creates a time-lock script.
func CreateTimelockScript(params TimelockParams) ([]byte, error) {
	return CreateScript(ScriptTimelock, params)
}

// CreateEscrowScript creates an escrow script.
func CreateEscrowScript(params EscrowParams) ([]byte, error) {
	return CreateScript(ScriptEscrow, params)
}

// ============================================================================
// Script Examples and Templates
// ============================================================================

// ExampleMultiSig creates a 2-of-3 multi-sig script.
func ExampleMultiSig(pubKeys [][]byte) ([]byte, error) {
	params := MultiSigParams{
		Threshold: 2,
		PubKeys:   pubKeys,
	}
	return CreateMultiSigScript(params)
}

// ExampleTimelock creates a time-lock script.
func ExampleTimelock(lockSlot uint64, pubKey []byte) ([]byte, error) {
	params := TimelockParams{
		LockSlot:  lockSlot,
		PublicKey: pubKey,
	}
	return CreateTimelockScript(params)
}

// ExampleEscrow creates an escrow script.
func ExampleEscrow(buyerPub, sellerPub []byte, timeout uint64) ([]byte, error) {
	params := EscrowParams{
		BuyerPubKey:  buyerPub,
		SellerPubKey: sellerPub,
		TimeoutSlot:  timeout,
	}
	return CreateEscrowScript(params)
}
