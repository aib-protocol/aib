// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Multi-Signature Module (2-of-2)
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ScriptOp represents an opcode for the scripting language.
type ScriptOp byte

const (
	// Data push opcodes
	OP_DATA ScriptOp = 0x00

	// Control flow opcodes
	OP_DUP          ScriptOp = 0x76
	//OP_EQUAL       ScriptOp = 0x87
	//OP_EQUALVERIFY ScriptOp = 0x88

	// Crypto opcodes
	OP_CHECKSIG       ScriptOp = 0xac
	OP_CHECKMULTISIG  ScriptOp = 0xae

	// Multi-sig specific
	OP_CHECKSIGVERIFY ScriptOp = 0xaf
	OP_SWAP           ScriptOp = 0x7c

	// Stack opcodes
	OP_DROP ScriptOp = 0x75

	// Numeric opcodes
	OP_1 ScriptOp = 0x51
	OP_2 ScriptOp = 0x52
	OP_3 ScriptOp = 0x53
)

// ScriptType represents the type of locking script.
type ScriptType byte

const (
	ScriptTypeP2PKH  ScriptType = 0x01 // Pay to Public Key Hash
	ScriptTypeMulti2 ScriptType = 0x02 // 2-of-2 Multi-signature
	ScriptTypeMulti3 ScriptType = 0x03 // 2-of-3 Multi-signature
)

// MultiSigScript represents a multi-signature locking script.
type MultiSigScript struct {
	RequiredSigs uint8
	PublicKeys   [][]byte
}

// MultiSigStore stores multi-signature UTXOs for spending.
type MultiSigStore struct {
	utxos   map[string]*UTXO
	scripts map[string]*MultiSigScript
	mu      sync.RWMutex
}

// NewMultiSigStore creates a new multi-signature store.
func NewMultiSigStore() *MultiSigStore {
	return &MultiSigStore{
		utxos:   make(map[string]*UTXO),
		scripts: make(map[string]*MultiSigScript),
	}
}

// CreateP2PKHScript creates a Pay-to-Public-Key-Hash locking script.
// Format: OP_DUP OP_HASH160 <pubkey_hash> OP_EQUALVERIFY OP_CHECKSIG
func CreateP2PKHScript(pubKeyHash [32]byte) []byte {
	script := []byte{
		byte(OP_DUP),
		// OP_HASH160 would be here (simplified)
	}
	// Append pubkey hash
	script = append(script, pubKeyHash[:]...)
	script = append(script, byte(OP_CHECKSIG))

	return script
}

// CreateMultiSig2Script creates a 2-of-2 multi-signature locking script.
// Format: OP_2 <pubkeyA> <pubkeyB> OP_2 OP_CHECKMULTISIG
func CreateMultiSig2Script(pubKeyA, pubKeyB []byte) []byte {
	script := []byte{byte(OP_2)}

	// Append public keys (with length prefix)
	script = append(script, byte(len(pubKeyA)))
	script = append(script, pubKeyA...)
	script = append(script, byte(len(pubKeyB)))
	script = append(script, pubKeyB...)

	script = append(script, byte(OP_2))
	script = append(script, byte(OP_CHECKMULTISIG))

	return script
}

// ParseMultiSigScript parses a multi-signature script.
func ParseMultiSigScript(script []byte) (*MultiSigScript, error) {
	if len(script) < 5 {
		return nil, fmt.Errorf("script too short for multi-sig")
	}

	if script[0] != byte(OP_2) && script[0] != byte(OP_3) {
		return nil, fmt.Errorf("invalid multi-sig required sigs")
	}

	requiredSigs := script[0]

	// Parse public keys
	var publicKeys [][]byte
	offset := 1

	for offset < len(script)-2 {
		if script[offset] > byte(OP_DROP) { // Not an opcode
			pubKeyLen := int(script[offset])
			offset++

			if offset+pubKeyLen > len(script) {
				return nil, fmt.Errorf("invalid pub key length")
			}

			pubKey := make([]byte, pubKeyLen)
			copy(pubKey, script[offset:offset+pubKeyLen])
			publicKeys = append(publicKeys, pubKey)
			offset += pubKeyLen
		} else {
			break
		}
	}

	// Check ending
	if script[offset] != byte(requiredSigs) {
		return nil, fmt.Errorf("total sigs mismatch")
	}

	if script[offset+1] != byte(OP_CHECKMULTISIG) {
		return nil, fmt.Errorf("invalid multi-sig opcode")
	}

	return &MultiSigScript{
		RequiredSigs: requiredSigs,
		PublicKeys:   publicKeys,
	}, nil
}

// VerifyMultiSig verifies 2-of-2 multi-signature for a transaction output.
func VerifyMultiSig(script []byte, txHash [32]byte, sig1, pubKey1, sig2, pubKey2 []byte) bool {
	// Parse script
	ms, err := ParseMultiSigScript(script)
	if err != nil {
		return false
	}

	if ms.RequiredSigs != 2 || len(ms.PublicKeys) != 2 {
		return false
	}

	// Create combined data for verification
	data := txHash[:]

	// Verify at least 2 valid signatures
	validCount := 0

	// Check first signature against all public keys
	if len(sig1) > 0 && len(pubKey1) > 0 {
		if ed25519.Verify(pubKey1, data, sig1) {
			validCount++
		}
	}

	// Check second signature
	if len(sig2) > 0 && len(pubKey2) > 0 {
		if ed25519.Verify(pubKey2, data, sig2) {
			validCount++
		}
	}

	// Also check cross combinations (in case signatures are swapped)
	if validCount < 2 {
		if len(sig1) > 0 && len(pubKey2) > 0 {
			if ed25519.Verify(pubKey2, data, sig1) {
				validCount++
			}
		}
		if len(sig2) > 0 && len(pubKey1) > 0 {
			if ed25519.Verify(pubKey1, data, sig2) {
				validCount++
			}
		}
	}

	return validCount >= 2
}

// MultiSigStoreAdapter implements the interfaces.MultiSigLocker.
type MultiSigStoreAdapter struct {
	utxoStore *UTXOStore
	multiSig  *MultiSigStore
}

// NewMultiSigStoreAdapter creates a new multi-signature adapter.
func NewMultiSigStoreAdapter(utxoStore *UTXOStore) *MultiSigStoreAdapter {
	return &MultiSigStoreAdapter{
		utxoStore: utxoStore,
		multiSig:  NewMultiSigStore(),
	}
}

// CreateMultiSigOutput creates a 2-of-2 multi-sig output.
func (a *MultiSigStoreAdapter) CreateMultiSigOutput(partyA, partyB interfaces.Address, amount uint64) (*interfaces.UTXO, error) {
	// Generate a temporary hash for the new UTXO (will be updated when transaction is created)
	tempHash := sha256.Sum256([]byte("temp"))

	// Create multi-signature script with placeholder public keys
	// In a real implementation, these would be retrieved from the addresses
	script := CreateMultiSig2Script(partyA[:], partyB[:])

	utxo := &UTXO{
		TxHash:  tempHash,
		Index:   0,
		Value:   amount,
		Script:  script,
		Address: partyA, // Primary party
	}

	// Store the script
	key := UTXOKey(tempHash, 0)
	a.multiSig.scripts[key] = &MultiSigScript{
		RequiredSigs: 2,
		PublicKeys:   [][]byte{partyA[:], partyB[:]},
	}

	return utxo.ToInterfacesUTXO(), nil
}

// SpendMultiSig spends a multi-sig output with both signatures.
func (a *MultiSigStoreAdapter) SpendMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte, outputs []interfaces.TXOutput) error {
	key := UTXOKey(utxo.TxHash, utxo.Index)

	// Check if multi-sig
	ms, exists := a.multiSig.scripts[key]
	if !exists {
		return fmt.Errorf("multi-sig script not found")
	}

	if ms.RequiredSigs != 2 {
		return fmt.Errorf("invalid required signatures")
	}

	// Spend the UTXO (will be done by transaction application)
	// Create the new UTXOs from outputs
	txHash := sha256.Sum256([]byte("tx"))

	for i, out := range outputs {
		newUTXO := &UTXO{
			TxHash:  txHash,
			Index:   uint32(i),
			Value:   out.Value,
			Script:  out.Script,
			Address: out.Address,
		}
		a.utxoStore.AddUTXO(newUTXO)
	}

	// Remove the spent multi-sig
	delete(a.multiSig.utxos, key)
	delete(a.multiSig.scripts, key)

	return nil
}

// VerifyMultiSig verifies both signatures for a multi-sig output.
func (a *MultiSigStoreAdapter) VerifyMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte) bool {
	key := UTXOKey(utxo.TxHash, utxo.Index)

	ms, exists := a.multiSig.scripts[key]
	if !exists {
		return false
	}

	if len(ms.PublicKeys) != 2 {
		return false
	}

	// Verify signatures
	data := utxo.TxHash[:]

	validCount := 0
	for i, pubKey := range ms.PublicKeys {
		sig := sigA
		if i == 1 {
			sig = sigB
		}

		if len(sig) > 0 && ed25519.Verify(pubKey, data, sig) {
			validCount++
		}
	}

	return validCount >= 2
}

// CreateMultiSigTransaction creates a transaction that spends a multi-sig UTXO.
func CreateMultiSigTransaction(
	inputUTXO *UTXO,
	sigA, pubKeyA []byte,
	sigB, pubKeyB []byte,
	outputs []TXOutput,
) (*Transaction, error) {
	// Create input with both signatures
	input := TXInput{
		TxHash: inputUTXO.TxHash,
		Index:  inputUTXO.Index,
	}

	// Combine signatures for the input
	sigData := bytes.NewBuffer(nil)
	sigData.Write(sigA)
	sigData.Write(sigB)
	sigData.Write(pubKeyA)
	sigData.Write(pubKeyB)

	input.Signature = sigData.Bytes()
	input.PublicKey = pubKeyA // Primary public key

	tx := NewTransaction([]TXInput{input}, outputs)

	// Set the combined signature data
	tx.Inputs[0].Signature = bytes.Join([][]byte{sigA, sigB}, []byte{})

	return tx, nil
}

// IsMultiSigScript checks if a script is a multi-signature script.
func IsMultiSigScript(script []byte) bool {
	if len(script) < 3 {
		return false
	}

	// Check for multi-sig pattern (starts with OP_2 or OP_3, ends with OP_CHECKMULTISIG)
	return (script[0] == byte(OP_2) || script[0] == byte(OP_3)) &&
		script[len(script)-1] == byte(OP_CHECKMULTISIG)
}

// GetScriptType returns the type of the locking script.
func GetScriptType(script []byte) ScriptType {
	if IsMultiSigScript(script) {
		ms, err := ParseMultiSigScript(script)
		if err == nil {
			if ms.RequiredSigs == 2 && len(ms.PublicKeys) == 2 {
				return ScriptTypeMulti2
			}
			if ms.RequiredSigs == 2 && len(ms.PublicKeys) == 3 {
				return ScriptTypeMulti3
			}
		}
	}

	// Default to P2PKH
	return ScriptTypeP2PKH
}

// VerifySignature verifies a signature against a public key for given data.
func VerifySignature(pubKey, data, sig []byte) bool {
	return ed25519.Verify(pubKey, data, sig)
}

// CombineSignatures combines multiple signatures into a single byte slice.
func CombineSignatures(signatures ...[]byte) []byte {
	var result []byte
	for _, sig := range signatures {
		result = append(result, sig...)
	}
	return result
}

// SplitCombinedSignatures splits a combined signature slice into individual signatures.
func SplitCombinedSignatures(combined []byte, count int) [][]byte {
	if count <= 0 {
		return nil
	}

	sigLen := 64 // Ed25519 signature length
	signatures := make([][]byte, count)

	for i := 0; i < count; i++ {
		start := i * sigLen
		if start+sigLen > len(combined) {
			break
		}
		signatures[i] = combined[start : start+sigLen]
	}

	return signatures
}
