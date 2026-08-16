// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains additional coverage tests for the eUTXO module.
package eutxo

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

// ============================================================================
// Redeemer Coverage Tests
// ============================================================================

func TestRedeemerSize(t *testing.T) {
	data := []byte(`{"action":"spend","amount":100}`)

	redeemer := NewRedeemer(data, ExUnits{Steps: 10000, Mem: 100000}, TagSpend)
	size := RedeemerSize(redeemer)

	// Size returns only the data length
	expectedSize := len(data)
	if size != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, size)
	}

	// Test with larger data
	largeData := bytes.Repeat([]byte("x"), 1000)
	largeRedeemer := NewRedeemer(largeData, ExUnits{Steps: 10000, Mem: 100000}, TagSpend)
	largeSize := RedeemerSize(largeRedeemer)
	expectedLargeSize := len(largeData)
	if largeSize != expectedLargeSize {
		t.Errorf("expected large size %d, got %d", expectedLargeSize, largeSize)
	}
}

func TestRedeemerType(t *testing.T) {
	data := []byte(`{"action":"spend"}`)

	tests := []struct {
		name     string
		tag      RedeemerTag
		expected string
	}{
		{"Spend", TagSpend, "Spend"},
		{"Mint", TagMint, "Mint"},
		{"Cert", TagCert, "Certificate"},
		{"Reward", TagReward, "Reward"},
		{"Unknown", 255, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redeemer := NewRedeemer(data, ExUnits{Steps: 10000, Mem: 100000}, tt.tag)
			redeemerType := RedeemerType(tt.tag)
			if redeemerType != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, redeemerType)
			}
			if redeemer.Tag != uint8(tt.tag) {
				t.Errorf("expected tag %d, got %d", uint8(tt.tag), redeemer.Tag)
			}
		})
	}
}

func TestRedeemerFromJSON(t *testing.T) {
	jsonBytes := []byte(`{"action":"spend","amount":100}`)

	redeemer, err := RedeemerFromJSON(jsonBytes, ExUnits{Steps: 10000, Mem: 100000}, TagSpend)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if redeemer.Tag != uint8(TagSpend) {
		t.Errorf("expected tag %d, got %d", TagSpend, redeemer.Tag)
	}
	if redeemer.ExUnits.Steps != 10000 {
		t.Errorf("expected steps 10000, got %d", redeemer.ExUnits.Steps)
	}
	if redeemer.ExUnits.Mem != 100000 {
		t.Errorf("expected mem 100000, got %d", redeemer.ExUnits.Mem)
	}

	// Test invalid JSON
	_, err = RedeemerFromJSON([]byte("invalid"), ExUnits{}, TagSpend)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRedeemerToJSON(t *testing.T) {
	data := []byte(`{"action":"spend"}`)
	exUnits := ExUnits{Steps: 10000, Mem: 100000}
	tag := TagSpend

	redeemer := NewRedeemer(data, exUnits, tag)

	jsonData, err := RedeemerToJSON(redeemer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("Expected non-empty JSON data")
	}
}

func TestNewSpendRedeemer(t *testing.T) {
	signature := []byte{1, 2, 3}
	pubKey := []byte{4, 5, 6}
	redeemer := NewSpendRedeemer(signature, pubKey)

	if redeemer.Tag != uint8(TagSpend) {
		t.Errorf("expected tag %d, got %d", TagSpend, redeemer.Tag)
	}
	if redeemer.ExUnits != DefaultExUnits {
		t.Errorf("expected default exUnits, got %v", redeemer.ExUnits)
	}
}

func TestNewMintRedeemer(t *testing.T) {
	policyData := []byte{1, 2, 3}
	tokenName := []byte("ABC")
	amount := uint64(1000)
	redeemer := NewMintRedeemer(policyData, tokenName, amount)

	if redeemer.Tag != uint8(TagMint) {
		t.Errorf("expected tag %d, got %d", TagMint, redeemer.Tag)
	}
}

func TestNewCertRedeemer(t *testing.T) {
	certData := []byte{1, 2, 3}
	pubKey := []byte{4, 5, 6}
	redeemer := NewCertRedeemer(certData, pubKey)

	if redeemer.Tag != uint8(TagCert) {
		t.Errorf("expected tag %d, got %d", TagCert, redeemer.Tag)
	}
}

func TestNewRewardRedeemer(t *testing.T) {
	stakeKey := []byte{1, 2, 3}
	amount := uint64(1000)
	redeemer := NewRewardRedeemer(stakeKey, amount)

	if redeemer.Tag != uint8(TagReward) {
		t.Errorf("expected tag %d, got %d", TagReward, redeemer.Tag)
	}
}

// ============================================================================
// Script Creation Tests
// ============================================================================

func TestCreateScript(t *testing.T) {
	// Test P2PKH script
	script, err := CreateScript(ScriptP2PKH, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// P2PKH script is just 1 byte
	if len(script) != 1 {
		t.Errorf("expected script with 1 byte, got %d", len(script))
	}
	if script[0] != byte(ScriptP2PKH) {
		t.Errorf("expected script type %d, got %d", ScriptP2PKH, script[0])
	}

	// Test MultiSig script
	params := MultiSigParams{
		Threshold: 2,
		PubKeys:   [][]byte{{1, 2, 3}, {4, 5, 6}},
	}
	multiSigScript, err := CreateScript(ScriptMultiSig, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if multiSigScript[0] != byte(ScriptMultiSig) {
		t.Errorf("expected script type %d, got %d", ScriptMultiSig, multiSigScript[0])
	}

	// Test invalid script type
	_, err = CreateScript(255, nil)
	if err == nil {
		t.Error("expected error for invalid script type")
	}
}

func TestCreateP2PKHScript(t *testing.T) {
	script, err := CreateP2PKHScript()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script[0] != byte(ScriptP2PKH) {
		t.Errorf("expected P2PKH script type, got %d", script[0])
	}
}

func TestCreateMultiSigScript(t *testing.T) {
	params := MultiSigParams{
		Threshold: 2,
		PubKeys:   [][]byte{{1, 2, 3}, {4, 5, 6}},
	}
	script, err := CreateMultiSigScript(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script[0] != byte(ScriptMultiSig) {
		t.Errorf("expected MultiSig script type, got %d", script[0])
	}
}

func TestCreateTimelockScript(t *testing.T) {
	params := TimelockParams{
		LockSlot:  1000,
		PublicKey: []byte{1, 2, 3},
	}
	script, err := CreateTimelockScript(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script[0] != byte(ScriptTimelock) {
		t.Errorf("expected Timelock script type, got %d", script[0])
	}
}

func TestCreateEscrowScript(t *testing.T) {
	params := EscrowParams{
		TimeoutSlot:  1000,
		BuyerPubKey:  []byte{1, 2, 3},
		SellerPubKey: []byte{4, 5, 6},
	}
	script, err := CreateEscrowScript(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script[0] != byte(ScriptEscrow) {
		t.Errorf("expected Escrow script type, got %d", script[0])
	}
}

// ============================================================================
// Validator Helper Tests
// ============================================================================

func TestValidatorHelpers(t *testing.T) {
	v := DefaultValidator()

	// Test verifySignature with valid data
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	message := []byte("test message")
	signature := ed25519.Sign(privKey, message)

	if !v.verifySignature(pubKey, signature, message) {
		t.Error("expected valid signature to verify")
	}

	// Test with invalid public key length
	invalidPubKey := []byte{0x01, 0x02}
	if v.verifySignature(invalidPubKey, signature, message) {
		t.Error("expected verification to fail with invalid pubKey length")
	}

	// Test with invalid signature length
	invalidSig := []byte{0x01, 0x02}
	if v.verifySignature(pubKey, invalidSig, message) {
		t.Error("expected verification to fail with invalid signature length")
	}

	// Test with wrong signature
	wrongSig := ed25519.Sign(privKey, []byte("different message"))
	if v.verifySignature(pubKey, wrongSig, message) {
		t.Error("expected verification to fail with wrong signature")
	}
}

// ============================================================================
// ExUnits Comparison Tests
// ============================================================================

func TestExUnitsLess(t *testing.T) {
	ex1 := ExUnits{Steps: 1000, Mem: 10000}
	ex2 := ExUnits{Steps: 2000, Mem: 10000}
	ex3 := ExUnits{Steps: 1000, Mem: 20000}
	ex4 := ExUnits{Steps: 1000, Mem: 10000}

	if !ex1.Less(ex2) {
		t.Error("expected ex1 < ex2 (steps)")
	}
	if !ex1.Less(ex3) {
		t.Error("expected ex1 < ex3 (mem)")
	}
	if ex1.Less(ex4) {
		t.Error("expected ex1 not < ex4 (equal)")
	}
}

func TestExUnitsEqual(t *testing.T) {
	ex1 := ExUnits{Steps: 1000, Mem: 10000}
	ex2 := ExUnits{Steps: 1000, Mem: 10000}
	ex3 := ExUnits{Steps: 2000, Mem: 10000}

	if !ex1.Equal(ex2) {
		t.Error("expected ex1 == ex2")
	}
	if ex1.Equal(ex3) {
		t.Error("expected ex1 != ex3")
	}
}

// ============================================================================
// Address Tests
// ============================================================================

func TestAddressStringCoverage(t *testing.T) {
	var hash [28]byte
	hash[0] = 0xAB
	hash[27] = 0xCD

	// Test script address
	scriptAddr := NewScriptAddress(hash)
	str := scriptAddr.String()
	if str == "" {
		t.Error("expected non-empty string")
	}

	// Test pubkey address
	pubkeyAddr := NewPubKeyAddress(hash)
	str = pubkeyAddr.String()
	if str == "" {
		t.Error("expected non-empty string")
	}
}

// ============================================================================
// Datum Hash Tests
// ============================================================================

func TestDatumHashCalculation(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	datum1 := NewDatum(data, EncodingJSON)
	datum2 := NewDatum(data, EncodingJSON)

	// Same data should produce same hash
	if !bytes.Equal(datum1.Hash[:], datum2.Hash[:]) {
		t.Error("same data should produce same hash")
	}

	// Different data should produce different hash
	datum3 := NewDatum([]byte(`{"key":"different"}`), EncodingJSON)
	if bytes.Equal(datum1.Hash[:], datum3.Hash[:]) {
		t.Error("different data should produce different hash")
	}
}
