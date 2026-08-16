package eutxo

import (
	"testing"
)

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestIsValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrInvalidVersion", ErrInvalidVersion, true},
		{"ErrNoInputs", ErrNoInputs, true},
		{"ErrNoOutputs", ErrNoOutputs, true},
		{"ErrInsufficientFee", ErrInsufficientFee, true},
		{"ErrInputNotFound", ErrInputNotFound, true},
		{"ErrInputSpent", ErrInputSpent, true},
		{"ErrInputValueMismatch", ErrInputValueMismatch, true},
		{"ErrInputAddressMismatch", ErrInputAddressMismatch, true},
		{"ErrZeroOutputValue", ErrZeroOutputValue, true},
		{"ErrOutputValueTooSmall", ErrOutputValueTooSmall, true},
		{"ErrDatumTooLarge", ErrDatumTooLarge, true},
		{"ErrScriptTooLarge", ErrScriptTooLarge, true},
		{"ErrInvalidAddress", ErrInvalidAddress, true},
		{"ErrValueNotConserved", ErrValueNotConserved, true},
		{"ErrValidityIntervalNotStarted", ErrValidityIntervalNotStarted, true},
		{"ErrTTLExpired", ErrTTLExpired, true},
		{"ErrInvalidSignature", ErrInvalidSignature, true},
		{"ErrDatumMissing", ErrDatumMissing, true},
		{"ErrDatumHashMismatch", ErrDatumHashMismatch, true},
		{"ErrScriptHashMismatch", ErrScriptHashMismatch, true},
		{"ErrUTXONotFound", ErrUTXONotFound, true},
		{"ErrUTXOSpent", ErrUTXOSpent, true},
		{"nil error", nil, false},
		{"random error", ErrEmptyDatum, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidationError(tt.err)
			if result != tt.expected {
				t.Errorf("IsValidationError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsScriptError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrEmptyScript", ErrEmptyScript, true},
		{"ErrInvalidScriptType", ErrInvalidScriptType, true},
		{"ErrScriptExecutionFailed", ErrScriptExecutionFailed, true},
		{"ErrExUnitsExceeded", ErrExUnitsExceeded, true},
		{"ErrExStepsExceeded", ErrExStepsExceeded, true},
		{"ErrExMemExceeded", ErrExMemExceeded, true},
		{"nil error", nil, false},
		{"random error", ErrInvalidDatum, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsScriptError(tt.err)
			if result != tt.expected {
				t.Errorf("IsScriptError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsDatumError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrInvalidDatum", ErrInvalidDatum, true},
		{"ErrEmptyDatum", ErrEmptyDatum, true},
		{"ErrInvalidDatumEncoding", ErrInvalidDatumEncoding, true},
		{"ErrInvalidJSONDatum", ErrInvalidJSONDatum, true},
		{"ErrJSONDatumTooLarge", ErrJSONDatumTooLarge, true},
		{"ErrInvalidCBORDatum", ErrInvalidCBORDatum, true},
		{"ErrInvalidCBORConstructor", ErrInvalidCBORConstructor, true},
		{"ErrCBORTooManyFields", ErrCBORTooManyFields, true},
		{"ErrInvalidPlutusDatum", ErrInvalidPlutusDatum, true},
		{"ErrInvalidPlutusType", ErrInvalidPlutusType, true},
		{"ErrPlutusTooManyFields", ErrPlutusTooManyFields, true},
		{"ErrPlutusTooManyPairs", ErrPlutusTooManyPairs, true},
		{"ErrPlutusTooManyElements", ErrPlutusTooManyElements, true},
		{"ErrPlutusIntOutOfRange", ErrPlutusIntOutOfRange, true},
		{"ErrPlutusBytesTooLarge", ErrPlutusBytesTooLarge, true},
		{"ErrInvalidCBORFormat", ErrInvalidCBORFormat, true},
		{"ErrInvalidPlutusDataFormat", ErrInvalidPlutusDataFormat, true},
		{"ErrInvalidPlutusDataTag", ErrInvalidPlutusDataTag, true},
		{"nil error", nil, false},
		{"random error", ErrEmptyScript, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDatumError(tt.err)
			if result != tt.expected {
				t.Errorf("IsDatumError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRedeemerError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrInvalidRedeemer", ErrInvalidRedeemer, true},
		{"ErrEmptyRedeemer", ErrEmptyRedeemer, true},
		{"ErrInvalidRedeemerTag", ErrInvalidRedeemerTag, true},
		{"ErrInvalidRedeemerFormat", ErrInvalidRedeemerFormat, true},
		{"ErrRedeemerTooLarge", ErrRedeemerTooLarge, true},
		{"nil error", nil, false},
		{"random error", ErrInvalidDatum, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRedeemerError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRedeemerError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsTransactionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrInvalidTransaction", ErrInvalidTransaction, true},
		{"ErrTransactionTooLarge", ErrTransactionTooLarge, true},
		{"ErrTooManyInputs", ErrTooManyInputs, true},
		{"ErrTooManyOutputs", ErrTooManyOutputs, true},
		{"nil error", nil, false},
		{"random error", ErrInvalidDatum, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransactionError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransactionError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestErrorWithCode(t *testing.T) {
	// Test ErrorWithCode with wrapped error
	err := NewErrorWithCode("TEST001", "test error", ErrInvalidDatum)
	expected := "[TEST001] test error: invalid datum"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}

	// Test ErrorWithCode without wrapped error
	err2 := NewErrorWithCode("TEST002", "another error", nil)
	expected2 := "[TEST002] another error"
	if err2.Error() != expected2 {
		t.Errorf("Error() = %v, want %v", err2.Error(), expected2)
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are properly defined
	if MaxTxSize != 16384 {
		t.Errorf("MaxTxSize = %d, want 16384", MaxTxSize)
	}
	if MaxInputs != 255 {
		t.Errorf("MaxInputs = %d, want 255", MaxInputs)
	}
	if MaxOutputs != 255 {
		t.Errorf("MaxOutputs = %d, want 255", MaxOutputs)
	}
	if MaxScriptSize != 16384 {
		t.Errorf("MaxScriptSize = %d, want 16384", MaxScriptSize)
	}
	if MaxRedeemerSize != 8192 {
		t.Errorf("MaxRedeemerSize = %d, want 8192", MaxRedeemerSize)
	}
	if MaxExUnitsSteps != 10000000 {
		t.Errorf("MaxExUnitsSteps = %d, want 10000000", MaxExUnitsSteps)
	}
	if MaxExUnitsMem != 1000000 {
		t.Errorf("MaxExUnitsMem = %d, want 1000000", MaxExUnitsMem)
	}
	if AddressSize != 28 {
		t.Errorf("AddressSize = %d, want 28", AddressSize)
	}
	if HashSize != 32 {
		t.Errorf("HashSize = %d, want 32", HashSize)
	}
	if TxIDSize != 32 {
		t.Errorf("TxIDSize = %d, want 32", TxIDSize)
	}
	if Ed25519PubKeySize != 32 {
		t.Errorf("Ed25519PubKeySize = %d, want 32", Ed25519PubKeySize)
	}
	if Ed25519SigSize != 64 {
		t.Errorf("Ed25519SigSize = %d, want 64", Ed25519SigSize)
	}
}

// ============================================================================
// Address Tests
// ============================================================================

func TestAddressString(t *testing.T) {
	addr := Address{
		Script: true,
		Hash:   [28]byte{1, 2, 3},
	}
	str := addr.String()
	if len(str) == 0 {
		t.Error("Address.String() should not be empty")
	}
}

func TestNewScriptAddress(t *testing.T) {
	var scriptHash [28]byte
	scriptHash[0] = 0xAB

	addr := NewScriptAddress(scriptHash)
	if !addr.Script {
		t.Error("Expected script address")
	}
	if addr.Hash != scriptHash {
		t.Error("Hash mismatch")
	}
}

func TestNewPubKeyAddress(t *testing.T) {
	var pubKeyHash [28]byte
	pubKeyHash[0] = 0xCD

	addr := NewPubKeyAddress(pubKeyHash)
	if addr.Script {
		t.Error("Expected pubkey address")
	}
	if addr.Hash != pubKeyHash {
		t.Error("Hash mismatch")
	}
}

func TestValidateAddress(t *testing.T) {
	// Valid pubkey address
	var hash [28]byte
	hash[0] = 0x01
	addr := Address{Script: false, Hash: hash}
	if err := ValidateAddress(addr); err != nil {
		t.Errorf("ValidateAddress() failed for valid pubkey: %v", err)
	}

	// Invalid pubkey address (zero hash)
	var zeroHash [28]byte
	addrZero := Address{Script: false, Hash: zeroHash}
	if err := ValidateAddress(addrZero); err == nil {
		t.Error("Expected error for zero hash pubkey address")
	}

	// Script address (can have zero hash)
	var scriptHash [28]byte
	addrScript := Address{Script: true, Hash: scriptHash}
	if err := ValidateAddress(addrScript); err != nil {
		t.Errorf("ValidateAddress() failed for script address: %v", err)
	}
}

// ============================================================================
// UTXO Set Tests
// ============================================================================

func TestUTXOSetRemove(t *testing.T) {
	set := NewUTXOSet()

	var txID [32]byte
	txID[0] = 1

	utxo := &eUTXO{
		TxID:  txID,
		Index: 0,
		Value: 1000,
	}

	// Add UTXO
	key := UTXOKey{TxID: txID, Index: 0}
	set.Add(utxo)

	// Remove UTXO
	set.Remove(key)

	// Verify removed
	if _, ok := set.Get(key); ok {
		t.Error("UTXO should be removed")
	}
}

// ============================================================================
// Transaction Tests
// ============================================================================

func TestNewTransaction(t *testing.T) {
	tx := NewTransaction(1, 100)
	if tx == nil {
		t.Fatal("NewTransaction returned nil")
	}
	if tx.Version != 1 {
		t.Errorf("Version = %d, want 1", tx.Version)
	}
	if tx.Slot != 100 {
		t.Errorf("Slot = %d, want 100", tx.Slot)
	}
	if tx.Inputs == nil {
		t.Error("Inputs should be initialized")
	}
	if tx.Outputs == nil {
		t.Error("Outputs should be initialized")
	}
}

func TestTransactionAddInput(t *testing.T) {
	tx := NewTransaction(1, 100)
	input := eTXInput{
		TxID:  [32]byte{1},
		Index: 0,
		Value: 1000,
	}
	tx.AddInput(input)
	if len(tx.Inputs) != 1 {
		t.Errorf("Inputs length = %d, want 1", len(tx.Inputs))
	}
}

func TestTransactionAddOutput(t *testing.T) {
	tx := NewTransaction(1, 100)
	output := eTXOutput{
		Value: 500,
	}
	tx.AddOutput(output)
	if len(tx.Outputs) != 1 {
		t.Errorf("Outputs length = %d, want 1", len(tx.Outputs))
	}
}

func TestTransactionAddDatum(t *testing.T) {
	tx := NewTransaction(1, 100)
	datum := []byte("test datum")
	index := tx.AddDatum(datum)
	if index != 0 {
		t.Errorf("Index = %d, want 0", index)
	}
	if len(tx.Datums) != 1 {
		t.Errorf("Datums length = %d, want 1", len(tx.Datums))
	}
}

func TestGetInputValue(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{Value: 1000})
	tx.AddInput(eTXInput{Value: 2000})
	if total := tx.GetInputValue(); total != 3000 {
		t.Errorf("GetInputValue() = %d, want 3000", total)
	}
}

func TestGetOutputValue(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddOutput(eTXOutput{Value: 500})
	tx.AddOutput(eTXOutput{Value: 300})
	if total := tx.GetOutputValue(); total != 800 {
		t.Errorf("GetOutputValue() = %d, want 800", total)
	}
}

func TestGetValueBalance(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{Value: 1000})
	tx.AddOutput(eTXOutput{Value: 700})
	tx.Fee = 200
	if balance := tx.GetValueBalance(); balance != 100 {
		t.Errorf("GetValueBalance() = %d, want 100", balance)
	}
}

func TestIsValidBalance(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{Value: 1000})
	tx.AddOutput(eTXOutput{Value: 700})
	tx.Fee = 200
	if !tx.IsValidBalance() {
		t.Error("Expected valid balance")
	}

	// Invalid balance
	tx2 := NewTransaction(1, 100)
	tx2.AddInput(eTXInput{Value: 500})
	tx2.AddOutput(eTXOutput{Value: 700})
	tx2.Fee = 100
	if tx2.IsValidBalance() {
		t.Error("Expected invalid balance")
	}
}

func TestSortInputs(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.Inputs = []eTXInput{
		{TxID: [32]byte{2}, Index: 1},
		{TxID: [32]byte{1}, Index: 0},
	}
	tx.SortInputs()
	if tx.Inputs[0].TxID[0] != 1 {
		t.Error("Inputs not sorted correctly")
	}
}

func TestSortOutputs(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.Outputs = []eTXOutput{
		{Address: Address{Hash: [28]byte{2}}},
		{Address: Address{Hash: [28]byte{1}}},
	}
	tx.SortOutputs()
	if tx.Outputs[0].Address.Hash[0] != 1 {
		t.Error("Outputs not sorted correctly")
	}
}

func TestTransactionSerialize(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{
		TxID:  [32]byte{1},
		Index: 0,
		Value: 1000,
		Address: Address{
			Hash: [28]byte{1},
		},
	})
	tx.AddOutput(eTXOutput{
		Value: 500,
		Address: Address{
			Hash: [28]byte{2},
		},
	})
	tx.Fee = 100
	tx.TTL = 200
	tx.ValidAfter = 50

	data, err := tx.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("Serialized data should not be empty")
	}
}

func TestDeserialize(t *testing.T) {
	// Create and serialize a transaction
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{
		TxID:  [32]byte{1},
		Index: 0,
		Value: 1000,
		Address: Address{
			Hash: [28]byte{1},
		},
	})
	tx.AddOutput(eTXOutput{
		Value: 500,
		Address: Address{
			Hash: [28]byte{2},
		},
	})
	tx.Fee = 100
	tx.ComputeHash()

	data, err := tx.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}

	// Deserialize
	deserialized, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize() error: %v", err)
	}
	if deserialized.Version != tx.Version {
		t.Errorf("Version = %d, want %d", deserialized.Version, tx.Version)
	}
	if len(deserialized.Inputs) != len(tx.Inputs) {
		t.Errorf("Inputs length = %d, want %d", len(deserialized.Inputs), len(tx.Inputs))
	}
	if len(deserialized.Outputs) != len(tx.Outputs) {
		t.Errorf("Outputs length = %d, want %d", len(deserialized.Outputs), len(tx.Outputs))
	}
}

func TestComputeTxID(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.AddInput(eTXInput{
		TxID:  [32]byte{1},
		Index: 0,
		Value: 1000,
		Address: Address{
			Hash: [28]byte{1},
		},
	})
	txID := ComputeTxID(tx)
	if txID == [32]byte{} {
		t.Error("TxID should not be empty")
	}
}

func TestGetTxID(t *testing.T) {
	tx := NewTransaction(1, 100)
	tx.ComputeHash()
	txID := tx.GetTxID()
	if txID == [32]byte{} {
		t.Error("GetTxID() should not be empty after ComputeHash()")
	}
}

func TestAddressFromUtxo(t *testing.T) {
	var utxoAddr [32]byte
	utxoAddr[0] = 0xAB
	utxoAddr[1] = 0xCD

	addr := AddressFromUtxo(utxoAddr)
	if addr.Hash[0] != 0xAB || addr.Hash[1] != 0xCD {
		t.Error("AddressFromUtxo conversion failed")
	}
}

func TestAddressToUtxo(t *testing.T) {
	addr := Address{
		Hash: [28]byte{0x12, 0x34},
	}

	uaddr := AddressToUtxo(addr)
	if uaddr[0] != 0x12 || uaddr[1] != 0x34 {
		t.Error("AddressToUtxo conversion failed")
	}
}
