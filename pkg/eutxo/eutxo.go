// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This module provides support for:
// - Datum: State data attached to UTXOs
// - Redeemer: Validation data provided when spending UTXOs
// - Validator Scripts: Programmable validation logic
// - Resource Conservation: Strict input/output balance
package eutxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// ============================================================================
// Core eUTXO Data Structures
// ============================================================================

// eUTXO represents an Extended UTXO with Datum and Script support.
// This is the core data structure for Cardano-style smart contracts.
type eUTXO struct {
	TxID       [32]byte // Transaction ID that created this UTXO
	Index      uint32   // Output index within the transaction
	Value      uint64   // Lovelace/value locked in this UTXO
	Address    Address  // Script address or public key hash address
	Datum      []byte   // Inline datum data (state)
	DatumHash  [32]byte // Datum hash (for datum embedded in witness)
	Script     []byte   // Validator script (inline)
	ScriptHash [32]byte // Script hash (for script embedded in witness)
	IsSpent    bool     // Whether this UTXO has been spent
	CreatedAt  uint64   // Slot number when created
	SpentAt    uint64   // Slot number when spent (0 if unspent)
}

// eTXInput represents an input to an eUTXO transaction.
// It references a previous UTXO and provides the redeemer.
type eTXInput struct {
	TxID      [32]byte  // Previous transaction ID
	Index     uint32    // Output index in previous transaction
	Value     uint64    // Value being spent (for quick validation)
	Address   Address   // Address of the UTXO being spent
	Script    []byte    // Script to execute (from witness)
	Datum     []byte    // Datum (from witness, if not inline)
	Redeemer  *Redeemer // Redeemer data
	Signature []byte    // Ed25519 signature (for P2PK)
	PubKey    []byte    // Public key (for P2PK)
}

// eTXOutput represents an output of an eUTXO transaction.
type eTXOutput struct {
	Address    Address
	Value      uint64
	Datum      []byte   // Inline datum (optional)
	DatumHash  [32]byte // Datum hash (when datum not inline)
	Script     []byte   // Validator script (optional)
	ScriptHash [32]byte // Script hash (when script not inline)
}

// eUTXOTransaction represents an Extended UTXO transaction.
type eUTXOTransaction struct {
	Version    uint32      // Transaction version
	Inputs     []eTXInput  // Inputs being spent
	Outputs    []eTXOutput // Outputs being created
	Fee        uint64      // Transaction fee
	TTL        uint64      // Time-to-Live (slot number)
	ValidAfter uint64      // Earliest valid slot (optional)
	Metadata   []byte      // Transaction metadata (optional)
	Datums     [][]byte    // Additional datums (non-inline)
	Signatures [][]byte    // Additional signatures
	Slot       uint64      // Current slot number (for validation)
	Hash       [32]byte    // Transaction hash (computed)
}

// Address represents an eUTXO address (script or pubkey).
type Address struct {
	Script bool     // Is this a script address?
	Hash   [28]byte // Payload hash (script hash or pubkey hash)
	Extra  []byte   // Extra data (for future extensions)
}

// TxIDHash represents a transaction ID (double SHA256 hash).
type TxIDHash [32]byte

// ComputeTxID computes the transaction ID from transaction data.
func ComputeTxID(tx *eUTXOTransaction) TxIDHash {
	var buf bytes.Buffer

	// Write version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Write inputs
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Inputs)))
	for _, in := range tx.Inputs {
		buf.Write(in.TxID[:])
		binary.Write(&buf, binary.LittleEndian, in.Index)
	}

	// Write outputs
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Outputs)))
	for _, out := range tx.Outputs {
		buf.Write(out.Address.Hash[:])
		binary.Write(&buf, binary.LittleEndian, out.Value)
		if len(out.Datum) > 0 {
			buf.Write(out.Datum)
		}
		if len(out.Script) > 0 {
			buf.Write(out.Script)
		}
	}

	// Write fee, TTL, ValidAfter
	binary.Write(&buf, binary.LittleEndian, tx.Fee)
	binary.Write(&buf, binary.LittleEndian, tx.TTL)
	binary.Write(&buf, binary.LittleEndian, tx.ValidAfter)

	// Double SHA256
	hash1 := sha256.Sum256(buf.Bytes())
	hash := sha256.Sum256(hash1[:])
	return TxIDHash(hash)
}

// GetTxID returns the transaction ID as [32]byte.
func (tx *eUTXOTransaction) GetTxID() [32]byte {
	return tx.Hash
}

// ComputeHash computes and sets the transaction hash.
func (tx *eUTXOTransaction) ComputeHash() {
	tx.Hash = ComputeTxID(tx)
}

// ============================================================================
// UTXO Set Management
// ============================================================================

// UTXOSet represents a set of eUTXOs.
type UTXOSet map[UTXOKey]*eUTXO

// UTXOKey is the key for looking up a UTXO.
type UTXOKey struct {
	TxID  [32]byte
	Index uint32
}

// NewUTXOSet creates a new empty UTXO set.
func NewUTXOSet() UTXOSet {
	return make(UTXOSet)
}

// Get retrieves a UTXO by key.
func (s UTXOSet) Get(key UTXOKey) (*eUTXO, bool) {
	utxo, ok := s[key]
	return utxo, ok
}

// Add adds a UTXO to the set.
func (s UTXOSet) Add(utxo *eUTXO) {
	key := UTXOKey{TxID: utxo.TxID, Index: utxo.Index}
	s[key] = utxo
}

// Remove removes a UTXO from the set.
func (s UTXOSet) Remove(key UTXOKey) {
	delete(s, key)
}

// Spend marks a UTXO as spent.
func (s UTXOSet) Spend(key UTXOKey, slot uint64) error {
	utxo, ok := s[key]
	if !ok {
		return ErrUTXONotFound
	}
	if utxo.IsSpent {
		return ErrUTXOSpent
	}
	utxo.IsSpent = true
	utxo.SpentAt = slot
	return nil
}

// GetSpent retrieves all spent UTXOs.
func (s UTXOSet) GetSpent() []*eUTXO {
	var result []*eUTXO
	for _, utxo := range s {
		if utxo.IsSpent {
			result = append(result, utxo)
		}
	}
	return result
}

// GetUnspent retrieves all unspent UTXOs.
func (s UTXOSet) GetUnspent() []*eUTXO {
	var result []*eUTXO
	for _, utxo := range s {
		if !utxo.IsSpent {
			result = append(result, utxo)
		}
	}
	return result
}

// GetByAddress retrieves all UTXOs at a specific address.
func (s UTXOSet) GetByAddress(addr Address) []*eUTXO {
	var result []*eUTXO
	for _, utxo := range s {
		if !utxo.IsSpent && bytes.Equal(utxo.Address.Hash[:], addr.Hash[:]) {
			result = append(result, utxo)
		}
	}
	return result
}

// TotalValue calculates the total value in the UTXO set.
func (s UTXOSet) TotalValue() uint64 {
	var total uint64
	for _, utxo := range s {
		if !utxo.IsSpent {
			total += utxo.Value
		}
	}
	return total
}

// ============================================================================
// Address Helpers
// ============================================================================

// NewScriptAddress creates a new script address from a script hash.
func NewScriptAddress(scriptHash [28]byte) Address {
	return Address{
		Script: true,
		Hash:   scriptHash,
	}
}

// NewPubKeyAddress creates a new public key address from a pubkey hash.
func NewPubKeyAddress(pubKeyHash [28]byte) Address {
	return Address{
		Script: false,
		Hash:   pubKeyHash,
	}
}

// ValidateAddress validates an address format.
func ValidateAddress(addr Address) error {
	// Check if hash is valid (non-zero for most cases)
	if !addr.Script && isZeroHash(addr.Hash[:]) {
		return ErrInvalidAddress
	}
	return nil
}

// isZeroHash checks if a byte slice is all zeros.
func isZeroHash(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// AddressFromUtxo converts a utxo address [32]byte to eutxo.Address.
func AddressFromUtxo(utxoAddr [32]byte) Address {
	var eutxoAddr Address
	copy(eutxoAddr.Hash[:], utxoAddr[:28])
	return eutxoAddr
}

// AddressToUtxo converts an eutxo.Address to utxo address [32]byte.
func AddressToUtxo(addr Address) [32]byte {
	var uaddr [32]byte
	copy(uaddr[:], addr.Hash[:])
	return uaddr
}

// String returns a string representation of the address.
func (a Address) String() string {
	return fmt.Sprintf("Address{Script:%v, Hash:%x}", a.Script, a.Hash)
}

// ============================================================================
// Transaction Building Helpers
// ============================================================================

// NewTransaction creates a new eUTXO transaction with default values.
func NewTransaction(version uint32, slot uint64) *eUTXOTransaction {
	return &eUTXOTransaction{
		Version:    version,
		Slot:       slot,
		Inputs:     make([]eTXInput, 0),
		Outputs:    make([]eTXOutput, 0),
		ValidAfter: 0,
		TTL:        0,
		Fee:        0,
	}
}

// AddInput adds an input to the transaction.
func (tx *eUTXOTransaction) AddInput(in eTXInput) {
	tx.Inputs = append(tx.Inputs, in)
}

// AddOutput adds an output to the transaction.
func (tx *eUTXOTransaction) AddOutput(out eTXOutput) {
	tx.Outputs = append(tx.Outputs, out)
}

// AddDatum adds a datum to the transaction.
func (tx *eUTXOTransaction) AddDatum(datum []byte) int {
	tx.Datums = append(tx.Datums, datum)
	return len(tx.Datums) - 1
}

// Sign signs the transaction with the given private key.
func (tx *eUTXOTransaction) Sign(privateKey []byte) error {
	tx.ComputeHash()
	sig := ed25519.Sign(privateKey, tx.Hash[:])
	tx.Signatures = append(tx.Signatures, sig)
	return nil
}

// GetInputValue calculates the total input value.
func (tx *eUTXOTransaction) GetInputValue() uint64 {
	var total uint64
	for _, in := range tx.Inputs {
		total += in.Value
	}
	return total
}

// GetOutputValue calculates the total output value.
func (tx *eUTXOTransaction) GetOutputValue() uint64 {
	var total uint64
	for _, out := range tx.Outputs {
		total += out.Value
	}
	return total
}

// GetValueBalance returns the value balance (inputs - outputs).
func (tx *eUTXOTransaction) GetValueBalance() int64 {
	return int64(tx.GetInputValue()) - int64(tx.GetOutputValue()+tx.Fee)
}

// IsValidBalance checks if the transaction has valid balance.
func (tx *eUTXOTransaction) IsValidBalance() bool {
	return tx.GetValueBalance() >= 0
}

// SortInputs sorts inputs by (TxID, Index).
func (tx *eUTXOTransaction) SortInputs() {
	sort.Slice(tx.Inputs, func(i, j int) bool {
		li := bytes.Compare(tx.Inputs[i].TxID[:], tx.Inputs[j].TxID[:])
		if li != 0 {
			return li < 0
		}
		return tx.Inputs[i].Index < tx.Inputs[j].Index
	})
}

// SortOutputs sorts outputs by (Address, Value).
func (tx *eUTXOTransaction) SortOutputs() {
	sort.Slice(tx.Outputs, func(i, j int) bool {
		li := bytes.Compare(tx.Outputs[i].Address.Hash[:], tx.Outputs[j].Address.Hash[:])
		if li != 0 {
			return li < 0
		}
		return tx.Outputs[i].Value < tx.Outputs[j].Value
	})
}

// ============================================================================
// Import/Export
// ============================================================================

// Serialize serializes the transaction to bytes.
func (tx *eUTXOTransaction) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Inputs
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Inputs)))
	for _, in := range tx.Inputs {
		buf.Write(in.TxID[:])
		binary.Write(&buf, binary.LittleEndian, in.Index)
		binary.Write(&buf, binary.LittleEndian, in.Value)
		buf.Write(in.Address.Hash[:])
	}

	// Outputs
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Outputs)))
	for _, out := range tx.Outputs {
		buf.Write(out.Address.Hash[:])
		binary.Write(&buf, binary.LittleEndian, out.Value)
		// Datum
		binary.Write(&buf, binary.LittleEndian, uint32(len(out.Datum)))
		buf.Write(out.Datum)
		// Script
		binary.Write(&buf, binary.LittleEndian, uint32(len(out.Script)))
		buf.Write(out.Script)
	}

	// Fee, TTL, ValidAfter
	binary.Write(&buf, binary.LittleEndian, tx.Fee)
	binary.Write(&buf, binary.LittleEndian, tx.TTL)
	binary.Write(&buf, binary.LittleEndian, tx.ValidAfter)

	return buf.Bytes(), nil
}

// Deserialize deserializes a transaction from bytes.
func Deserialize(data []byte) (*eUTXOTransaction, error) {
	r := bytes.NewReader(data)

	tx := &eUTXOTransaction{}

	// Version
	binary.Read(r, binary.LittleEndian, &tx.Version)

	// Inputs
	var numInputs uint32
	binary.Read(r, binary.LittleEndian, &numInputs)
	tx.Inputs = make([]eTXInput, numInputs)
	for i := uint32(0); i < numInputs; i++ {
		binary.Read(r, binary.LittleEndian, &tx.Inputs[i].TxID)
		binary.Read(r, binary.LittleEndian, &tx.Inputs[i].Index)
		binary.Read(r, binary.LittleEndian, &tx.Inputs[i].Value)
		binary.Read(r, binary.LittleEndian, &tx.Inputs[i].Address.Hash)
	}

	// Outputs
	var numOutputs uint32
	binary.Read(r, binary.LittleEndian, &numOutputs)
	tx.Outputs = make([]eTXOutput, numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		binary.Read(r, binary.LittleEndian, &tx.Outputs[i].Address.Hash)
		binary.Read(r, binary.LittleEndian, &tx.Outputs[i].Value)

		var datumLen uint32
		binary.Read(r, binary.LittleEndian, &datumLen)
		tx.Outputs[i].Datum = make([]byte, datumLen)
		r.Read(tx.Outputs[i].Datum)

		var scriptLen uint32
		binary.Read(r, binary.LittleEndian, &scriptLen)
		tx.Outputs[i].Script = make([]byte, scriptLen)
		r.Read(tx.Outputs[i].Script)
	}

	// Fee, TTL, ValidAfter
	binary.Read(r, binary.LittleEndian, &tx.Fee)
	binary.Read(r, binary.LittleEndian, &tx.TTL)
	binary.Read(r, binary.LittleEndian, &tx.ValidAfter)

	tx.ComputeHash()

	return tx, nil
}
