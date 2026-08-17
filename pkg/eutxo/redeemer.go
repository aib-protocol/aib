// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains Redeemer handling and execution units management.
package eutxo

import (
	"encoding/json"
)

// ============================================================================
// Redeemer Types and Definitions
// ============================================================================

// Redeemer represents the data provided when spending an eUTXO.
// It contains the execution data and resource limits for script validation.
type Redeemer struct {
	Data    []byte  // Redeemer data (depends on script type)
	ExUnits ExUnits // Execution units (CPU + Memory budget)
	Index   uint32  // Redeemer index (for multiple redeemers)
	Tag     uint8   // Redeemer tag (Spend, Mint, Cert, etc.)
}

// ExUnits represents execution units for script execution.
// These are resource limits for CPU and Memory usage.
type ExUnits struct {
	Steps uint64 // CPU steps (computation units)
	Mem   uint64 // Memory usage (memory units)
}

// RedeemerTag represents the type of redeemer.
type RedeemerTag uint8

const (
	// TagSpend is for spending UTXOs
	TagSpend RedeemerTag = iota
	// TagMint is for token minting
	TagMint
	// TagCert is for certificate validation
	TagCert
	// TagReward is for reward withdrawals
	TagReward
)

// RedeemerData represents structured redeemer data.
type RedeemerData struct {
	Constructor uint16          // Constructor index
	Fields      []RedeemerField // Redeemer fields
}

// RedeemerField represents a field in redeemer data.
type RedeemerField struct {
	Value interface{} // Field value (int, string, bytes, or nested)
	Tag   uint64      // CBOR tag for type information
}

// ============================================================================
// Redeemer Creation and Validation
// ============================================================================

// NewRedeemer creates a new Redeemer with the given data and execution units.
func NewRedeemer(data []byte, exUnits ExUnits, tag RedeemerTag) *Redeemer {
	return &Redeemer{
		Data:    data,
		ExUnits: exUnits,
		Tag:     uint8(tag),
	}
}

// NewRedeemerFromJSON creates a Redeemer from JSON data.
func NewRedeemerFromJSON(data interface{}, exUnits ExUnits, tag RedeemerTag) (*Redeemer, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return NewRedeemer(jsonData, exUnits, tag), nil
}

// NewRedeemerFromCBOR creates a Redeemer from CBOR data.
func NewRedeemerFromCBOR(cbor *RedeemerData, exUnits ExUnits, tag RedeemerTag) (*Redeemer, error) {
	data, err := cbor.Marshal()
	if err != nil {
		return nil, err
	}
	return NewRedeemer(data, exUnits, tag), nil
}

// ValidateRedeemer validates redeemer structure and execution units.
func ValidateRedeemer(redeemer *Redeemer) error {
	if redeemer == nil {
		return ErrInvalidRedeemer
	}

	// Validate execution units
	if redeemer.ExUnits.Steps > MaxExUnits.Steps {
		return ErrExStepsExceeded
	}
	if redeemer.ExUnits.Mem > MaxExUnits.Mem {
		return ErrExMemExceeded
	}

	// Validate data size
	if len(redeemer.Data) > MaxRedeemerSize {
		return ErrRedeemerTooLarge
	}

	// Validate tag
	if redeemer.Tag > 3 { // Only 4 valid tags
		return ErrInvalidRedeemerTag
	}

	// Validate data format based on tag
	switch RedeemerTag(redeemer.Tag) {
	case TagSpend:
		return validateSpendRedeemer(redeemer)
	case TagMint:
		return validateMintRedeemer(redeemer)
	case TagCert:
		return validateCertRedeemer(redeemer)
	case TagReward:
		return validateRewardRedeemer(redeemer)
	default:
		return ErrInvalidRedeemerTag
	}
}

// validateSpendRedeemer validates spend-type redeemers.
func validateSpendRedeemer(redeemer *Redeemer) error {
	// Spend redeemers typically contain witness signatures or data
	// For P2PK: signature data
	// For scripts: script execution data

	// Basic size validation
	if len(redeemer.Data) == 0 {
		return ErrEmptyRedeemer
	}

	return nil
}

// validateMintRedeemer validates mint-type redeemers.
func validateMintRedeemer(redeemer *Redeemer) error {
	// Mint redeemers contain token policy data
	// Should be validated against the token policy script

	// Check if data is present
	if len(redeemer.Data) == 0 {
		return ErrEmptyRedeemer
	}

	return nil
}

// validateCertRedeemer validates certificate-type redeemers.
func validateCertRedeemer(redeemer *Redeemer) error {
	// Certificate redeemers contain delegation or registration data
	// Should be validated against the certificate policy

	// Check if data is present
	if len(redeemer.Data) == 0 {
		return ErrEmptyRedeemer
	}

	return nil
}

// validateRewardRedeemer validates reward withdrawal redeemers.
func validateRewardRedeemer(redeemer *Redeemer) error {
	// Reward redeemers contain withdrawal authorization data
	// Should be validated against the reward address

	// Check if data is present
	if len(redeemer.Data) == 0 {
		return ErrEmptyRedeemer
	}

	return nil
}

// ============================================================================
// Redeemer Parsing and Serialization
// ============================================================================

// ParseRedeemerData parses structured redeemer data.
func ParseRedeemerData(data []byte) (*RedeemerData, error) {
	if len(data) == 0 {
		return nil, ErrEmptyRedeemer
	}

	// Parse as JSON first (most common format)
	var jsonData interface{}
	err := json.Unmarshal(data, &jsonData)
	if err == nil {
		return parseJSONRedeemerData(jsonData)
	}

	// If not JSON, parse as raw CBOR data
	return parseCBORRedeemerData(data)
}

// parseJSONRedeemerData parses JSON-encoded redeemer data.
func parseJSONRedeemerData(data interface{}) (*RedeemerData, error) {
	switch d := data.(type) {
	case map[string]interface{}:
		// Parse as structured JSON
		var redeemerData RedeemerData

		if constructor, ok := d["constructor"].(float64); ok {
			redeemerData.Constructor = uint16(constructor)
		}

		if fields, ok := d["fields"].([]interface{}); ok {
			redeemerData.Fields = make([]RedeemerField, len(fields))
			for i, field := range fields {
				redeemerData.Fields[i] = RedeemerField{
					Value: field,
					Tag:   0, // No tag in JSON
				}
			}
		}

		return &redeemerData, nil
	case []interface{}:
		// Parse as array
		var redeemerData RedeemerData
		redeemerData.Constructor = 0 // Default constructor for arrays
		redeemerData.Fields = make([]RedeemerField, len(d))
		for i, field := range d {
			redeemerData.Fields[i] = RedeemerField{
				Value: field,
				Tag:   0,
			}
		}
		return &redeemerData, nil
	default:
		// Single value
		return &RedeemerData{
			Constructor: 0,
			Fields: []RedeemerField{
				{Value: d, Tag: 0},
			},
		}, nil
	}
}

// parseCBORRedeemerData parses CBOR-encoded redeemer data.
func parseCBORRedeemerData(data []byte) (*RedeemerData, error) {
	// CBOR parsing logic (simplified)
	var redeemerData RedeemerData

	// First byte is constructor + flags
	if len(data) < 1 {
		return nil, ErrInvalidRedeemerFormat
	}

	constructor := uint16(data[0])
	redeemerData.Constructor = constructor & 0x3FF // Extract constructor index

	// Parse fields (simplified)
	// This is a placeholder for actual CBOR parsing logic
	redeemerData.Fields = []RedeemerField{}

	return &redeemerData, nil
}

// RedeemerData Marshal method (placeholder)
func (r *RedeemerData) Marshal() ([]byte, error) {
	// Convert to JSON for simplicity
	data := map[string]interface{}{
		"constructor": r.Constructor,
		"fields":      r.Fields,
	}
	return json.Marshal(data)
}

// ============================================================================
// Execution Units Management
// ============================================================================

// MaxExUnits represents the maximum allowed execution units.
var MaxExUnits = ExUnits{
	Steps: 10000000, // 10M CPU steps
	Mem:   1000000,  // 1M memory units
}

// DefaultExUnits represents default execution units for simple operations.
var DefaultExUnits = ExUnits{
	Steps: 10000,  // 10K CPU steps
	Mem:   100000, // 100K memory units
}

// CalculateExUnits estimates execution units for a given operation.
func CalculateExUnits(operation string, dataSize int) ExUnits {
	baseSteps := uint64(1000)
	baseMem := uint64(10000)

	// Calculate based on data size
	dataSteps := uint64(dataSize * 10) // 10 steps per byte
	dataMem := uint64(dataSize * 100)  // 100 mem units per byte

	switch operation {
	case "signature":
		return ExUnits{
			Steps: baseSteps + dataSteps + 50000, // Signature verification cost
			Mem:   baseMem + dataMem + 500000,    // Signature verification memory
		}
	case "hash":
		return ExUnits{
			Steps: baseSteps + dataSteps + 10000, // Hash computation cost
			Mem:   baseMem + dataMem + 100000,    // Hash computation memory
		}
	case "script":
		return ExUnits{
			Steps: baseSteps + dataSteps + 100000, // Script execution cost
			Mem:   baseMem + dataMem + 1000000,    // Script execution memory
		}
	default:
		return ExUnits{
			Steps: baseSteps + dataSteps,
			Mem:   baseMem + dataMem,
		}
	}
}

// ExUnitsCost calculates the cost of execution units.
func (u ExUnits) Cost() uint64 {
	// Simple cost calculation (steps * 1 + mem * 0.01)
	return u.Steps + u.Mem/100
}

// ExUnitsAdd adds two ExUnits values.
func (u ExUnits) Add(other ExUnits) ExUnits {
	return ExUnits{
		Steps: u.Steps + other.Steps,
		Mem:   u.Mem + other.Mem,
	}
}

// ExUnitsSub subtracts another ExUnits value.
func (u ExUnits) Sub(other ExUnits) ExUnits {
	steps := u.Steps
	mem := u.Mem

	if u.Steps >= other.Steps {
		steps = u.Steps - other.Steps
	}

	if u.Mem >= other.Mem {
		mem = u.Mem - other.Mem
	}

	return ExUnits{
		Steps: steps,
		Mem:   mem,
	}
}

// ExUnitsLess returns true if u is less than other.
func (u ExUnits) Less(other ExUnits) bool {
	return u.Steps < other.Steps || (u.Steps == other.Steps && u.Mem < other.Mem)
}

// ExUnitsEqual returns true if u equals other.
func (u ExUnits) Equal(other ExUnits) bool {
	return u.Steps == other.Steps && u.Mem == other.Mem
}

// ============================================================================
// Redeemer Utilities
// ============================================================================

// RedeemerSize returns the size of the redeemer in bytes.
func RedeemerSize(redeemer *Redeemer) int {
	if redeemer == nil {
		return 0
	}
	return len(redeemer.Data)
}

// RedeemerType returns the type of redeemer based on tag.
func RedeemerType(tag RedeemerTag) string {
	switch tag {
	case TagSpend:
		return "Spend"
	case TagMint:
		return "Mint"
	case TagCert:
		return "Certificate"
	case TagReward:
		return "Reward"
	default:
		return "Unknown"
	}
}

// RedeemerFromJSON creates a redeemer from JSON data.
func RedeemerFromJSON(jsonData []byte, exUnits ExUnits, tag RedeemerTag) (*Redeemer, error) {
	var structData map[string]interface{}
	err := json.Unmarshal(jsonData, &structData)
	if err != nil {
		return nil, err
	}

	// Convert to JSON bytes
	jsonBytes, err := json.Marshal(structData)
	if err != nil {
		return nil, err
	}

	return NewRedeemer(jsonBytes, exUnits, tag), nil
}

// RedeemerToJSON converts a redeemer to JSON format.
func RedeemerToJSON(redeemer *Redeemer) ([]byte, error) {
	if redeemer == nil {
		return nil, ErrInvalidRedeemer
	}

	// Try to parse as JSON first
	var jsonData interface{}
	err := json.Unmarshal(redeemer.Data, &jsonData)
	if err == nil {
		return redeemer.Data, nil
	}

	// If not valid JSON, wrap in a structure
	wrapped := map[string]interface{}{
		"data":    redeemer.Data,
		"exUnits": redeemer.ExUnits,
		"tag":     redeemer.Tag,
		"type":    RedeemerType(RedeemerTag(redeemer.Tag)),
	}
	return json.Marshal(wrapped)
}

// ============================================================================
// Redeemer Examples
// ============================================================================

// NewSpendRedeemer creates a standard spend-type redeemer.
func NewSpendRedeemer(signature []byte, pubKey []byte) *Redeemer {
	data, _ := json.Marshal(map[string]interface{}{
		"signature": signature,
		"publicKey": pubKey,
	})
	return NewRedeemer(data, DefaultExUnits, TagSpend)
}

// NewMintRedeemer creates a token minting redeemer.
func NewMintRedeemer(policyData []byte, tokenName []byte, amount uint64) *Redeemer {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":    policyData,
		"tokenName": tokenName,
		"amount":    amount,
	})
	return NewRedeemer(data, ExUnits{Steps: 50000, Mem: 500000}, TagMint)
}

// NewCertRedeemer creates a certificate validation redeemer.
func NewCertRedeemer(certData []byte, stakeKey []byte) *Redeemer {
	data, _ := json.Marshal(map[string]interface{}{
		"certificate": certData,
		"stakeKey":    stakeKey,
	})
	return NewRedeemer(data, ExUnits{Steps: 20000, Mem: 200000}, TagCert)
}

// NewRewardRedeemer creates a reward withdrawal redeemer.
func NewRewardRedeemer(credential []byte, amount uint64) *Redeemer {
	data, _ := json.Marshal(map[string]interface{}{
		"credential": credential,
		"amount":     amount,
	})
	return NewRedeemer(data, ExUnits{Steps: 10000, Mem: 100000}, TagReward)
}
