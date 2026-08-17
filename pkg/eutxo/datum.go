// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains Datum handling and validation.
package eutxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// ============================================================================
// Datum Types and Definitions
// ============================================================================

// Datum represents state data attached to an eUTXO.
// Can be inline (stored with UTXO) or hash-only (stored separately).
type Datum struct {
	Data     []byte        // Actual datum data
	Hash     [32]byte      // SHA256 hash of the datum
	Inline   bool          // Whether datum is stored inline or as hash
	Encoding DatumEncoding // Encoding format
}

// DatumEncoding represents the encoding format of datum data.
type DatumEncoding uint8

const (
	// EncodingRaw is raw binary data (default)
	EncodingRaw DatumEncoding = iota
	// EncodingJSON is JSON-encoded data
	EncodingJSON
	// EncodingCBOR is CBOR-encoded data (Cardano standard)
	EncodingCBOR
	// EncodingPlutusData is Plutus Data format
	EncodingPlutusData
)

// DatumCBOR represents a CBOR-encoded datum (Cardano standard).
type DatumCBOR struct {
	Constructor uint16       // Constructor index
	Fields      []DatumField // Field values
}

// DatumField represents a field in a CBOR datum.
type DatumField struct {
	Value interface{} // Could be int, string, bytes, or nested DatumField
	Tag   uint64      // CBOR tag for type information
}

// DatumPlutusData represents Plutus Data format.
type DatumPlutusData struct {
	Constructor uint16        // Constructor index
	Fields      []PlutusValue // Field values (Plutus types)
	Bytes       []byte        // Raw bytes for Bytes constructor
	Int         int64         // Integer value for Int constructor
	List        []PlutusValue // List of values
	Map         []PlutusPair  // Map of key-value pairs
}

// PlutusValue represents a Plutus Data value.
type PlutusValue struct {
	Type  PlutusType
	Value interface{}
}

// PlutusType represents Plutus Data types.
type PlutusType uint8

const (
	PlutusTypeConstr PlutusType = iota
	PlutusTypeMap
	PlutusTypeList
	PlutusTypeInt
	PlutusTypeBytes
)

// PlutusPair represents a key-value pair in Plutus Map.
type PlutusPair struct {
	Key   PlutusValue
	Value PlutusValue
}

// ============================================================================
// Datum Creation and Validation
// ============================================================================

// NewDatum creates a new Datum with the given data.
func NewDatum(data []byte, encoding DatumEncoding) *Datum {
	d := &Datum{
		Data:     data,
		Hash:     sha256.Sum256(data),
		Inline:   true,
		Encoding: encoding,
	}
	return d
}

// NewDatumFromHash creates a Datum from a hash (hash-only datum).
func NewDatumFromHash(hash [32]byte) *Datum {
	return &Datum{
		Data:     nil,
		Hash:     hash,
		Inline:   false,
		Encoding: EncodingRaw,
	}
}

// NewDatumFromJSON creates a Datum from JSON data.
func NewDatumFromJSON(data interface{}) (*Datum, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return NewDatum(jsonData, EncodingJSON), nil
}

// NewDatumFromCBOR creates a Datum from CBOR data.
func NewDatumFromCBOR(cbor *DatumCBOR) (*Datum, error) {
	data, err := cbor.Marshal()
	if err != nil {
		return nil, err
	}
	return NewDatum(data, EncodingCBOR), nil
}

// ValidateDatum validates datum structure and encoding.
func ValidateDatum(datum *Datum) error {
	if datum == nil {
		return ErrInvalidDatum
	}

	if datum.Inline {
		if len(datum.Data) == 0 {
			return ErrEmptyDatum
		}
		if len(datum.Data) > MaxDatumSize {
			return ErrDatumTooLarge
		}
	}

	// Verify hash matches data
	if datum.Inline {
		computedHash := sha256.Sum256(datum.Data)
		if !bytes.Equal(computedHash[:], datum.Hash[:]) {
			return ErrDatumHashMismatch
		}
	}

	// Validate encoding-specific constraints
	switch datum.Encoding {
	case EncodingJSON:
		return validateJSONDatum(datum)
	case EncodingCBOR:
		return validateCBORDatum(datum)
	case EncodingPlutusData:
		return validatePlutusDatum(datum)
	case EncodingRaw:
		return nil // No additional validation for raw data
	default:
		return ErrInvalidDatumEncoding
	}
}

// validateJSONDatum validates JSON-encoded datum.
func validateJSONDatum(datum *Datum) error {
	if !datum.Inline {
		return nil // Hash-only datums don't need JSON validation
	}

	var jsonStruct interface{}
	err := json.Unmarshal(datum.Data, &jsonStruct)
	if err != nil {
		return ErrInvalidJSONDatum
	}

	// Check size constraints for JSON objects
	if len(datum.Data) > MaxJSONDatumSize {
		return ErrJSONDatumTooLarge
	}

	return nil
}

// validateCBORDatum validates CBOR-encoded datum.
func validateCBORDatum(datum *Datum) error {
	if !datum.Inline {
		return nil
	}

	// Parse CBOR structure
	cbor, err := ParseCBOR(datum.Data)
	if err != nil {
		return ErrInvalidCBORDatum
	}

	// Validate CBOR structure
	if cbor.Constructor > 1023 { // Cardano limit
		return ErrInvalidCBORConstructor
	}

	if len(cbor.Fields) > 255 { // Cardano limit
		return ErrCBORTooManyFields
	}

	return nil
}

// validatePlutusDatum validates Plutus Data datum.
func validatePlutusDatum(datum *Datum) error {
	if !datum.Inline {
		return nil
	}

	// Parse Plutus Data structure
	plutusData, err := ParsePlutusData(datum.Data)
	if err != nil {
		return ErrInvalidPlutusDatum
	}

	// Validate Plutus Data structure
	if err := validatePlutusValue(&PlutusValue{Type: PlutusTypeConstr, Value: plutusData}); err != nil {
		return err
	}

	return nil
}

// validatePlutusValue recursively validates a Plutus value.
func validatePlutusValue(value *PlutusValue) error {
	switch value.Type {
	case PlutusTypeConstr:
		// Constructor validation
		if len(value.Value.([]PlutusValue)) > 255 {
			return ErrPlutusTooManyFields
		}
		for _, field := range value.Value.([]PlutusValue) {
			if err := validatePlutusValue(&field); err != nil {
				return err
			}
		}
	case PlutusTypeMap:
		// Map validation
		pairs := value.Value.([]PlutusPair)
		if len(pairs) > 255 {
			return ErrPlutusTooManyPairs
		}
		for _, pair := range pairs {
			if err := validatePlutusValue(&pair.Key); err != nil {
				return err
			}
			if err := validatePlutusValue(&pair.Value); err != nil {
				return err
			}
		}
	case PlutusTypeList:
		// List validation
		list := value.Value.([]PlutusValue)
		if len(list) > 255 {
			return ErrPlutusTooManyElements
		}
		for _, element := range list {
			if err := validatePlutusValue(&element); err != nil {
				return err
			}
		}
	case PlutusTypeInt:
		// Integer validation
		intVal := value.Value.(int64)
		if intVal > MaxPlutusInt || intVal < MinPlutusInt {
			return ErrPlutusIntOutOfRange
		}
	case PlutusTypeBytes:
		// Bytes validation
		bytesVal := value.Value.([]byte)
		if len(bytesVal) > MaxPlutusBytes {
			return ErrPlutusBytesTooLarge
		}
	default:
		return ErrInvalidPlutusType
	}
	return nil
}

// ============================================================================
// Datum Parsing and Serialization
// ============================================================================

// ParseCBOR parses CBOR-encoded datum data.
func ParseCBOR(data []byte) (*DatumCBOR, error) {
	if len(data) == 0 {
		return nil, ErrEmptyDatum
	}

	// CBOR parsing logic (simplified)
	// In a real implementation, you'd use a proper CBOR library
	var cbor DatumCBOR

	// First byte is constructor + flags
	if len(data) < 1 {
		return nil, ErrInvalidCBORFormat
	}

	constructor := uint16(data[0])
	cbor.Constructor = constructor & 0x3FF // Extract constructor index

	// Parse fields (simplified)
	// This is a placeholder for actual CBOR parsing logic
	cbor.Fields = []DatumField{}

	return &cbor, nil
}

// ParsePlutusData parses Plutus Data encoded datum.
func ParsePlutusData(data []byte) (*DatumPlutusData, error) {
	if len(data) == 0 {
		return nil, ErrEmptyDatum
	}

	// Plutus Data parsing logic (simplified)
	// In a real implementation, you'd use proper Plutus Data parsing
	var plutusData DatumPlutusData

	// First byte determines type
	if len(data) < 1 {
		return nil, ErrInvalidPlutusDataFormat
	}

	tag := data[0]
	switch tag {
	case 0: // Int
		if len(data) < 9 { // 1 byte tag + 8 bytes int
			return nil, ErrInvalidPlutusDataFormat
		}
		var intVal int64
		err := binary.Read(bytes.NewReader(data[1:9]), binary.LittleEndian, &intVal)
		if err != nil {
			return nil, err
		}
		plutusData.Int = intVal
	case 1: // Bytes
		if len(data) < 5 { // 1 byte tag + 4 bytes length
			return nil, ErrInvalidPlutusDataFormat
		}
		var length uint32
		err := binary.Read(bytes.NewReader(data[1:5]), binary.LittleEndian, &length)
		if err != nil {
			return nil, err
		}
		if len(data) < 5+int(length) {
			return nil, ErrInvalidPlutusDataFormat
		}
		plutusData.Bytes = data[5 : 5+length]
	case 2: // List
		// Parse list (simplified)
		plutusData.List = []PlutusValue{}
	case 3: // Map
		// Parse map (simplified)
		plutusData.Map = []PlutusPair{}
	case 4: // Constr
		// Parse constructor (simplified)
		plutusData.Constructor = 0
		plutusData.Fields = []PlutusValue{}
	default:
		return nil, ErrInvalidPlutusDataTag
	}

	return &plutusData, nil
}

// CBOR Marshal method (placeholder)
func (c *DatumCBOR) Marshal() ([]byte, error) {
	// Simplified CBOR serialization
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, c.Constructor)
	binary.Write(&buf, binary.LittleEndian, uint16(len(c.Fields)))
	for range c.Fields {
		// Serialize field (placeholder)
		buf.WriteByte(0) // Placeholder
	}
	return buf.Bytes(), nil
}

// ============================================================================
// Datum Utilities
// ============================================================================

// DatumToJSON converts a datum to JSON format.
func DatumToJSON(datum *Datum) ([]byte, error) {
	if datum == nil {
		return nil, ErrInvalidDatum
	}

	switch datum.Encoding {
	case EncodingJSON:
		return datum.Data, nil
	case EncodingRaw:
		// Try to parse as JSON if it's valid
		var jsonStruct interface{}
		err := json.Unmarshal(datum.Data, &jsonStruct)
		if err == nil {
			return datum.Data, nil
		}
		// If not valid JSON, wrap in a structure
		wrapped := map[string]interface{}{
			"data": datum.Data,
			"hash": fmt.Sprintf("%x", datum.Hash),
		}
		return json.Marshal(wrapped)
	case EncodingCBOR:
		cbor, err := ParseCBOR(datum.Data)
		if err != nil {
			return nil, err
		}
		return json.Marshal(cbor)
	case EncodingPlutusData:
		plutusData, err := ParsePlutusData(datum.Data)
		if err != nil {
			return nil, err
		}
		return json.Marshal(plutusData)
	default:
		return nil, ErrInvalidDatumEncoding
	}
}

// DatumFromJSON creates a datum from JSON data.
func DatumFromJSON(jsonData []byte) (*Datum, error) {
	var structData map[string]interface{}
	err := json.Unmarshal(jsonData, &structData)
	if err != nil {
		return nil, err
	}

	// Check if it's already a wrapped datum
	if _, hasData := structData["data"]; hasData {
		if dataBytes, ok := structData["data"].([]byte); ok {
			return NewDatum(dataBytes, EncodingJSON), nil
		}
	}

	// Convert to JSON bytes
	jsonBytes, err := json.Marshal(structData)
	if err != nil {
		return nil, err
	}

	return NewDatum(jsonBytes, EncodingJSON), nil
}

// DatumSize returns the size of the datum in bytes.
func DatumSize(datum *Datum) int {
	if datum == nil {
		return 0
	}
	if datum.Inline {
		return len(datum.Data)
	}
	return 32 // Hash size
}

// ============================================================================
// Cardano Compatibility Constants
// ============================================================================

const (
	// MaxDatumSize is the maximum size of a datum in bytes
	MaxDatumSize = 16384 // 16KB

	// MaxJSONDatumSize is the maximum size of a JSON datum
	MaxJSONDatumSize = 8192 // 8KB

	// MaxPlutusInt is the maximum Plutus integer value
	MaxPlutusInt = 9223372036854775807 // 2^63 - 1

	// MinPlutusInt is the minimum Plutus integer value
	MinPlutusInt = -9223372036854775808 // -2^63

	// MaxPlutusBytes is the maximum size of Plutus bytes
	MaxPlutusBytes = 4096 // 4KB
)
