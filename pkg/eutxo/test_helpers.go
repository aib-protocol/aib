// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains test utilities and helper functions.
package eutxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
)

// ============================================================================
// Test Helpers
// ============================================================================

// generateTestKey generates a test Ed25519 key pair.
func generateTestKey() (ed25519.PrivateKey, ed25519.PublicKey) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return privKey, pubKey
}

// generateTestKeys generates multiple test key pairs.
func generateTestKeys(count int) ([]ed25519.PrivateKey, []ed25519.PublicKey) {
	privateKeys := make([]ed25519.PrivateKey, count)
	publicKeys := make([]ed25519.PublicKey, count)

	for i := 0; i < count; i++ {
		privateKeys[i], publicKeys[i] = generateTestKey()
	}

	return privateKeys, publicKeys
}

// createTestTransaction creates a simple test transaction.
func createTestTransaction(slot uint64) *eUTXOTransaction {
	tx := NewTransaction(1, slot)
	tx.TTL = slot + 100
	tx.Fee = 1000000 // 1 ADA
	return tx
}

// createTestUTXO creates a test UTXO.
func createTestUTXO(txID [32]byte, index uint32, value uint64, addr Address) *eUTXO {
	return &eUTXO{
		TxID:      txID,
		Index:     index,
		Value:     value,
		Address:   addr,
		Datum:     nil,
		Script:    nil,
		IsSpent:   false,
		CreatedAt: 0,
	}
}

// SignRedeemerData signs data with a private key.
func SignRedeemerData(privKey ed25519.PrivateKey, data []byte) ([]byte, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidSignature
	}
	sig := ed25519.Sign(privKey, data)
	return sig, nil
}

// pubKeyToHash converts a public key to a 28-byte hash.
func pubKeyToHash(pubKey ed25519.PublicKey) [28]byte {
	var hash [28]byte
	copy(hash[:], pubKey[:28])
	return hash
}

// bytesToHex converts bytes to hex string (for JSON encoding).
func bytesToHex(b []byte) string {
	hex := make([]byte, len(b)*2)
	for i, v := range b {
		hex[i*2] = "0123456789abcdef"[v>>4]
		hex[i*2+1] = "0123456789abcdef"[v&0x0f]
	}
	return string(hex)
}

// BuildScriptContext builds a context for script validation matching validator's context.
func BuildScriptContext(txHash [32]byte, slot uint64, inputIndex int, inputValue uint64, outputCount int, outputValues []uint64) []byte {
	var buf bytes.Buffer

	// Write transaction hash
	buf.Write(txHash[:])

	// Write current slot
	binary.Write(&buf, binary.LittleEndian, slot)

	// Write input index
	binary.Write(&buf, binary.LittleEndian, uint32(inputIndex))

	// Write input value
	binary.Write(&buf, binary.LittleEndian, inputValue)

	// Write output count
	binary.Write(&buf, binary.LittleEndian, uint32(outputCount))

	// Write output values
	for _, out := range outputValues {
		binary.Write(&buf, binary.LittleEndian, out)
	}

	return buf.Bytes()
}
