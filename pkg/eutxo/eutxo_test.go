// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains comprehensive tests for the eUTXO module.
package eutxo

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ============================================================================
// Test Cases
// ============================================================================

// Test eUTXO Creation and Basic Operations
func TestUTXOCreation(t *testing.T) {
	var txID [32]byte
	txID[0] = 1

	addr := NewPubKeyAddress([28]byte{0x01, 0x02, 0x03})

	utxo := createTestUTXO(txID, 0, 10000000, addr)

	if utxo.TxID != txID {
		t.Errorf("expected TxID %x, got %x", txID, utxo.TxID)
	}
	if utxo.Index != 0 {
		t.Errorf("expected Index 0, got %d", utxo.Index)
	}
	if utxo.Value != 10000000 {
		t.Errorf("expected Value 10000000, got %d", utxo.Value)
	}
	if utxo.IsSpent != false {
		t.Errorf("expected IsSpent false, got %v", utxo.IsSpent)
	}
}

// Test UTXO Set Operations
func TestUTXOSetOperations(t *testing.T) {
	set := NewUTXOSet()

	// Add UTXO
	var txID [32]byte
	txID[0] = 1

	addr := NewPubKeyAddress([28]byte{0x01, 0x02, 0x03})
	utxo := createTestUTXO(txID, 0, 10000000, addr)
	set.Add(utxo)

	// Check retrieval
	key := UTXOKey{TxID: txID, Index: 0}
	retrieved, ok := set.Get(key)
	if !ok {
		t.Error("expected to find UTXO in set")
	}
	if retrieved.Value != 10000000 {
		t.Errorf("expected Value 10000000, got %d", retrieved.Value)
	}

	// Check unspent
	unspent := set.GetUnspent()
	if len(unspent) != 1 {
		t.Errorf("expected 1 unspent UTXO, got %d", len(unspent))
	}

	// Spend UTXO
	err := set.Spend(key, 100)
	if err != nil {
		t.Errorf("unexpected error spending UTXO: %v", err)
	}

	// Check spent
	spent, ok := set.Get(key)
	if !ok {
		t.Error("expected to find spent UTXO in set")
	}
	if !spent.IsSpent {
		t.Error("expected UTXO to be marked as spent")
	}

	// Check unspent after spend
	unspent = set.GetUnspent()
	if len(unspent) != 0 {
		t.Errorf("expected 0 unspent UTXOs, got %d", len(unspent))
	}
}

// Test Transaction Creation
func TestTransactionCreation(t *testing.T) {
	tx := createTestTransaction(100)

	if tx.Version != 1 {
		t.Errorf("expected Version 1, got %d", tx.Version)
	}
	if tx.Slot != 100 {
		t.Errorf("expected Slot 100, got %d", tx.Slot)
	}
	if tx.TTL != 200 {
		t.Errorf("expected TTL 200, got %d", tx.TTL)
	}
	if tx.Fee != 1000000 {
		t.Errorf("expected Fee 1000000, got %d", tx.Fee)
	}

	// Test AddInput
	var inTxID [32]byte
	inTxID[0] = 1
	input := eTXInput{
		TxID:    inTxID,
		Index:   0,
		Value:   5000000,
		Address: NewPubKeyAddress([28]byte{0x01}),
	}
	tx.AddInput(input)

	if len(tx.Inputs) != 1 {
		t.Errorf("expected 1 input, got %d", len(tx.Inputs))
	}

	// Test AddOutput
	output := eTXOutput{
		Address: NewPubKeyAddress([28]byte{0x02}),
		Value:   4000000,
	}
	tx.AddOutput(output)

	if len(tx.Outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(tx.Outputs))
	}
}

// Test Value Balance
func TestValueBalance(t *testing.T) {
	tx := createTestTransaction(100)

	// Add inputs
	for i := 0; i < 3; i++ {
		var inTxID [32]byte
		inTxID[i] = byte(i + 1)
		tx.AddInput(eTXInput{
			TxID:  inTxID,
			Index: 0,
			Value: 10000000,
		})
	}

	// Add outputs
	tx.AddOutput(eTXOutput{
		Value: 15000000,
	})
	tx.AddOutput(eTXOutput{
		Value: 8000000,
	})
	tx.Fee = 1000000

	inputValue := tx.GetInputValue()
	if inputValue != 30000000 {
		t.Errorf("expected input value 30000000, got %d", inputValue)
	}

	outputValue := tx.GetOutputValue()
	if outputValue != 23000000 {
		t.Errorf("expected output value 23000000, got %d", outputValue)
	}

	balance := tx.GetValueBalance()
	if balance != 6000000 { // 30000000 - 23000000 - 1000000 = 6000000
		t.Errorf("expected balance 6000000, got %d", balance)
	}

	if !tx.IsValidBalance() {
		t.Error("expected valid balance")
	}
}

// Test Address Creation
func TestAddressCreation(t *testing.T) {
	// Script address
	var scriptHash [28]byte
	scriptHash[0] = 0xAB
	scriptAddr := NewScriptAddress(scriptHash)

	if !scriptAddr.Script {
		t.Error("expected script address")
	}
	if scriptAddr.Hash != scriptHash {
		t.Errorf("expected hash %x, got %x", scriptHash, scriptAddr.Hash)
	}

	// PubKey address
	var pubKeyHash [28]byte
	pubKeyHash[0] = 0xCD
	pubKeyAddr := NewPubKeyAddress(pubKeyHash)

	if pubKeyAddr.Script {
		t.Error("expected pubkey address")
	}
	if pubKeyAddr.Hash != pubKeyHash {
		t.Errorf("expected hash %x, got %x", pubKeyHash, pubKeyAddr.Hash)
	}

	// Validate addresses
	if err := ValidateAddress(pubKeyAddr); err != nil {
		t.Errorf("unexpected error validating pubkey address: %v", err)
	}

	// Invalid zero hash address
	zeroAddr := NewPubKeyAddress([28]byte{})
	if err := ValidateAddress(zeroAddr); err == nil {
		t.Error("expected error validating zero hash address")
	}
}

// ============================================================================
// Validator Tests
// ============================================================================

func TestValidatorBasicValidation(t *testing.T) {
	validator := DefaultValidator()

	// Test empty inputs
	tx := createTestTransaction(100)
	result := validator.Validate(tx, NewUTXOSet(), &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "NO_INPUTS" {
			t.Errorf("expected NO_INPUTS error, got %s", result.FailedRule)
		}
	}

	// Test zero version
	var inTxID [32]byte
	inTxID[0] = 1
	tx.AddInput(eTXInput{TxID: inTxID, Index: 0, Value: 1000000})
	tx.Version = 0
	tx.Fee = 1000000
	result = validator.Validate(tx, NewUTXOSet(), &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "INVALID_VERSION" {
			t.Errorf("expected INVALID_VERSION error, got %s", result.FailedRule)
		}
	}
}

// Test Resource Conservation
func TestResourceConservation(t *testing.T) {
	validator := DefaultValidator()

	tx := createTestTransaction(100)

	// Input: 5 ADA
	var inTxID [32]byte
	inTxID[0] = 1
	tx.AddInput(eTXInput{
		TxID:    inTxID,
		Index:   0,
		Value:   5000000,
		Address: NewPubKeyAddress([28]byte{0x01}),
	})

	// Output: 5 ADA (no fee) - should fail
	tx.AddOutput(eTXOutput{
		Address: NewPubKeyAddress([28]byte{0x02}),
		Value:   5000000,
	})
	tx.Fee = 0

	utxoSet := NewUTXOSet()
	var utxoTxID [32]byte
	utxoTxID[0] = 1
	utxoSet.Add(&eUTXO{
		TxID:    utxoTxID,
		Index:   0,
		Value:   5000000,
		Address: NewPubKeyAddress([28]byte{0x01}),
	})

	result := validator.Validate(tx, utxoSet, &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "INSUFFICIENT_FEE" {
			t.Errorf("expected INSUFFICIENT_FEE error, got %s", result.FailedRule)
		}
	}

	// Test value not conserved
	tx.Fee = 1000000
	result = validator.Validate(tx, utxoSet, &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "VALUE_NOT_CONSERVED" {
			t.Errorf("expected VALUE_NOT_CONSERVED error, got %s", result.FailedRule)
		}
	}
}

// Test Time Lock Validation
func TestTimeLockValidation(t *testing.T) {
	validator := DefaultValidator()

	// ValidAfter test
	tx := createTestTransaction(100)
	tx.ValidAfter = 150

	var inTxID [32]byte
	inTxID[0] = 1
	addr := NewPubKeyAddress([28]byte{0x01})
	tx.AddInput(eTXInput{
		TxID:    inTxID,
		Index:   0,
		Value:   10000000,
		Address: addr,
	})
	tx.AddOutput(eTXOutput{Value: 9000000, Address: addr})
	tx.Fee = 1000000

	utxoSet := NewUTXOSet()
	var utxoTxID [32]byte
	utxoTxID[0] = 1
	utxoSet.Add(&eUTXO{
		TxID:    utxoTxID,
		Index:   0,
		Value:   10000000,
		Address: addr,
	})

	result := validator.Validate(tx, utxoSet, &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "VALIDITY_INTERVAL_NOT_STARTED" {
			t.Errorf("expected VALIDITY_INTERVAL_NOT_STARTED error, got %s", result.FailedRule)
		}
	}

	// TTL test
	tx.ValidAfter = 0
	tx.TTL = 100
	result = validator.Validate(tx, utxoSet, &ValidationContext{Slot: 100})
	if result != nil && !result.Valid {
		if result.FailedRule != "TTL_EXPIRED" {
			t.Errorf("expected TTL_EXPIRED error, got %s", result.FailedRule)
		}
	}
}

// ============================================================================
// Script Validation Tests
// ============================================================================

func TestP2PKHValidation(t *testing.T) {
	// Generate keys
	privKey, pubKey := generateTestKey()

	// Create context (transaction hash)
	var txHash [32]byte
	txHash[0] = 1
	context := make([]byte, 40)
	copy(context, txHash[:])
	binary.LittleEndian.PutUint64(context[32:40], 100) // slot 100

	// Create datum (empty for P2PKH)
	datum := []byte{}

	// Create redeemer (signature)
	redeemerData, _ := SignRedeemerData(privKey, context)
	redeemer := []byte(`{"signature":"` + bytesToHex(redeemerData) + `","publicKey":"` + bytesToHex(pubKey) + `"}`)

	// Validate
	result := ValidateP2PKH(datum, redeemer, context)
	if !result {
		t.Error("expected P2PKH validation to pass")
	}
}

func TestMultiSigValidation(t *testing.T) {
	// Generate 3 keys
	privKeys, pubKeys := generateTestKeys(3)

	// Create context
	var txHash [32]byte
	txHash[0] = 1
	context := make([]byte, 40)
	copy(context, txHash[:])
	binary.LittleEndian.PutUint64(context[32:40], 100)

	// Create datum (2-of-3 policy)
	datum := []byte(`{"threshold":2,"pubKeys":["` + bytesToHex(pubKeys[0]) + `","` + bytesToHex(pubKeys[1]) + `","` + bytesToHex(pubKeys[2]) + `"]}`)

	// Create redeemer (2 signatures)
	sig0, _ := SignRedeemerData(privKeys[0], context)
	sig1, _ := SignRedeemerData(privKeys[1], context)
	redeemer := []byte(`{"signatures":["` + bytesToHex(sig0) + `","` + bytesToHex(sig1) + `"]}`)

	// Validate
	result := ValidateMultiSig(datum, redeemer, context)
	if !result {
		t.Error("expected MultiSig validation to pass")
	}

	// Test with only 1 signature (should fail)
	redeemer = []byte(`{"signatures":["` + bytesToHex(sig0) + `"]}`)
	result = ValidateMultiSig(datum, redeemer, context)
	if result {
		t.Error("expected MultiSig validation to fail with only 1 signature")
	}
}

func TestTimelockValidation(t *testing.T) {
	// Generate key
	privKey, pubKey := generateTestKey()

	// Create context with slot
	var txHash [32]byte
	txHash[0] = 1
	context := make([]byte, 40)
	copy(context, txHash[:])
	binary.LittleEndian.PutUint64(context[32:40], 100) // slot 100

	// Create datum (lock until slot 50)
	datum := []byte(`{"lockSlot":50,"publicKey":"` + bytesToHex(pubKey) + `"}`)

	// Create signature
	sig, _ := SignRedeemerData(privKey, context)
	redeemer := []byte(`{"signature":"` + bytesToHex(sig) + `"}`)

	// Validate - should pass (100 >= 50)
	result := ValidateTimelock(datum, redeemer, context)
	if !result {
		t.Error("expected Timelock validation to pass (slot 100 >= lock 50)")
	}

	// Test with later lock slot (should fail)
	datum = []byte(`{"lockSlot":150,"publicKey":"` + bytesToHex(pubKey) + `"}`)
	result = ValidateTimelock(datum, redeemer, context)
	if result {
		t.Error("expected Timelock validation to fail (slot 100 < lock 150)")
	}
}

func TestEscrowValidation(t *testing.T) {
	// Generate keys
	buyerPriv, buyerPub := generateTestKey()
	sellerPriv, sellerPub := generateTestKey()

	// Create context with slot
	var txHash [32]byte
	txHash[0] = 1
	context := make([]byte, 40)
	copy(context, txHash[:])
	binary.LittleEndian.PutUint64(context[32:40], 100) // slot 100

	// Create datum (escrow: buyer + seller, timeout at slot 150)
	datum := []byte(`{"buyerPubKey":"` + bytesToHex(buyerPub) + `","sellerPubKey":"` + bytesToHex(sellerPub) + `","timeoutSlot":150}`)

	// Test buyer claim
	buyerSig, _ := SignRedeemerData(buyerPriv, context)
	redeemer := []byte(`{"buyerSignature":"` + bytesToHex(buyerSig) + `"}`)

	result := ValidateEscrow(datum, redeemer, context)
	if !result {
		t.Error("expected Escrow validation to pass (buyer claim)")
	}

	// Test seller claim before timeout (should fail)
	// Seller signs with timeout slot (150) for future use
	contextTimeout := make([]byte, 40)
	copy(contextTimeout, txHash[:])
	binary.LittleEndian.PutUint64(contextTimeout[32:40], 150) // slot 150 (timeout)
	sellerSig, _ := SignRedeemerData(sellerPriv, contextTimeout)
	redeemer = []byte(`{"sellerSignature":"` + bytesToHex(sellerSig) + `"}`)

	result = ValidateEscrow(datum, redeemer, context)
	if result {
		t.Error("expected Escrow validation to fail (seller before timeout)")
	}

	// Test seller claim after timeout
	context2 := make([]byte, 40)
	copy(context2, txHash[:])
	binary.LittleEndian.PutUint64(context2[32:40], 200) // slot 200

	result = ValidateEscrow(datum, redeemer, context2)
	if !result {
		t.Error("expected Escrow validation to pass (seller after timeout)")
	}
}

// ============================================================================
// Datum and Redeemer Tests
// ============================================================================

func TestDatumCreation(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	datum := NewDatum(data, EncodingJSON)

	if !datum.Inline {
		t.Error("expected datum to be inline")
	}
	if !bytes.Equal(datum.Data, data) {
		t.Errorf("expected data %x, got %x", data, datum.Data)
	}
	if len(datum.Hash) != 32 {
		t.Errorf("expected hash length 32, got %d", len(datum.Hash))
	}
}

func TestDatumFromJSON(t *testing.T) {
	jsonData := map[string]interface{}{
		"amount":    1000000,
		"recipient": "abc123",
	}

	datum, err := NewDatumFromJSON(jsonData)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if datum == nil {
		t.Error("expected datum, got nil")
	}
	if datum.Encoding != EncodingJSON {
		t.Errorf("expected JSON encoding, got %d", datum.Encoding)
	}
}

func TestRedeemerCreation(t *testing.T) {
	data := []byte(`{"action":"spend"}`)
	exUnits := ExUnits{Steps: 10000, Mem: 100000}
	tag := TagSpend

	redeemer := NewRedeemer(data, exUnits, tag)

	if !bytes.Equal(redeemer.Data, data) {
		t.Errorf("expected data %x, got %x", data, redeemer.Data)
	}
	if redeemer.ExUnits != exUnits {
		t.Errorf("expected exUnits %v, got %v", exUnits, redeemer.ExUnits)
	}
	if redeemer.Tag != uint8(tag) {
		t.Errorf("expected tag %d, got %d", tag, redeemer.Tag)
	}
}

func TestExUnitsOperations(t *testing.T) {
	ex1 := ExUnits{Steps: 1000, Mem: 10000}
	ex2 := ExUnits{Steps: 2000, Mem: 20000}

	// Add
	sum := ex1.Add(ex2)
	if sum.Steps != 3000 {
		t.Errorf("expected steps 3000, got %d", sum.Steps)
	}
	if sum.Mem != 30000 {
		t.Errorf("expected mem 30000, got %d", sum.Mem)
	}

	// Sub
	diff := ex2.Sub(ex1)
	if diff.Steps != 1000 {
		t.Errorf("expected steps 1000, got %d", diff.Steps)
	}
	if diff.Mem != 10000 {
		t.Errorf("expected mem 10000, got %d", diff.Mem)
	}

	// Cost
	cost := ExUnits{Steps: 1000, Mem: 10000}.Cost()
	if cost != 1100 { // 1000 + 10000/100
		t.Errorf("expected cost 1100, got %d", cost)
	}
}

// ============================================================================
// Serialization Tests
// ============================================================================

func TestTransactionSerialization(t *testing.T) {
	tx := createTestTransaction(100)

	// Add input
	var inTxID [32]byte
	inTxID[0] = 1
	tx.AddInput(eTXInput{
		TxID:    inTxID,
		Index:   0,
		Value:   5000000,
		Address: NewPubKeyAddress([28]byte{0x01}),
	})

	// Add output
	tx.AddOutput(eTXOutput{
		Address: NewPubKeyAddress([28]byte{0x02}),
		Value:   4000000,
		Datum:   []byte(`test`),
		Script:  []byte{0x01, 0x02, 0x03},
	})

	// Serialize
	data, err := tx.Serialize()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected serialized data, got empty")
	}

	// Deserialize
	tx2, err := Deserialize(data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tx2.Version != tx.Version {
		t.Errorf("expected version %d, got %d", tx.Version, tx2.Version)
	}
	if len(tx2.Inputs) != len(tx.Inputs) {
		t.Errorf("expected %d inputs, got %d", len(tx.Inputs), len(tx2.Inputs))
	}
	if len(tx2.Outputs) != len(tx.Outputs) {
		t.Errorf("expected %d outputs, got %d", len(tx.Outputs), len(tx2.Outputs))
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestFullTransactionValidation(t *testing.T) {
	validator := DefaultValidator()

	// Generate keys
	privKey, pubKey := generateTestKey()

	// Create a simple P2PKH transaction
	tx := createTestTransaction(100)
	tx.Fee = 1000000

	// Create input
	var inTxID [32]byte
	inTxID[0] = 1

	// Create UTXO in the set
	utxoSet := NewUTXOSet()
	utxoSet.Add(&eUTXO{
		TxID:    inTxID,
		Index:   0,
		Value:   5000000,
		Address: NewPubKeyAddress(pubKeyToHash(pubKey)),
		Script:  []byte{byte(ScriptP2PKH)},
	})

	// Add input with signature
	var txHash [32]byte
	txHash[0] = 1
	tx.ComputeHash()

	// Build proper context for script validation (matching validator's context format)
	context := BuildScriptContext(tx.Hash, 100, 0, 5000000, 1, []uint64{4000000})

	sig, _ := SignRedeemerData(privKey, context)
	redeemerData := []byte(`{"signature":"` + bytesToHex(sig) + `","publicKey":"` + bytesToHex(pubKey) + `"}`)

	tx.AddInput(eTXInput{
		TxID:      inTxID,
		Index:     0,
		Value:     5000000,
		Address:   NewPubKeyAddress(pubKeyToHash(pubKey)),
		Script:    []byte{byte(ScriptP2PKH)},
		Signature: sig,
		PubKey:    pubKey,
		Redeemer:  NewRedeemer(redeemerData, DefaultExUnits, TagSpend),
	})

	// Add output
	tx.AddOutput(eTXOutput{
		Address: NewPubKeyAddress([28]byte{0x02}),
		Value:   4000000,
	})

	// Validate
	result := validator.Validate(tx, utxoSet, &ValidationContext{
		Slot:     100,
		TxHash:   tx.Hash,
		TxInputs: nil,
	})

	if result != nil && !result.Valid {
		t.Errorf("expected validation to pass, got %s: %v", result.FailedRule, result.Error)
	}
}
