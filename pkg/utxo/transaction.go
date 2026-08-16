// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Core UTXO Module
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// Transaction represents a UTXO-based transaction.
type Transaction struct {
	Version  uint32
	Inputs   []TXInput
	Outputs  []TXOutput
	LockTime uint32
	Sequence uint64 // Sequence number for replay protection
}

// TXInput represents a transaction input (reference to a UTXO).
type TXInput struct {
	TxHash    [32]byte
	Index     uint32
	Signature []byte
	PublicKey []byte
}

// TXOutput represents a transaction output.
type TXOutput struct {
	Value   uint64
	Script  []byte
	Address interfaces.Address
}

// UTXO represents an unspent transaction output (extends TXOutput with reference).
type UTXO struct {
	TxHash  [32]byte
	Index   uint32
	Value   uint64
	Script  []byte
	Address interfaces.Address
}

// NewTransaction creates a new transaction with the given inputs and outputs.
func NewTransaction(inputs []TXInput, outputs []TXOutput) *Transaction {
	return &Transaction{
		Version:  1,
		Inputs:   inputs,
		Outputs:  outputs,
		LockTime: 0,
	}
}

// Hash computes the transaction hash (double SHA256).
func (tx *Transaction) Hash() [32]byte {
	data := tx.Serialize()
	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])
	return hash2
}

// Serialize converts the transaction to bytes.
func (tx *Transaction) Serialize() []byte {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Input count
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Inputs)))

	// Inputs
	for _, in := range tx.Inputs {
		buf.Write(in.TxHash[:])
		binary.Write(&buf, binary.LittleEndian, in.Index)
		binary.Write(&buf, binary.LittleEndian, uint32(len(in.Signature)))
		buf.Write(in.Signature)
		binary.Write(&buf, binary.LittleEndian, uint32(len(in.PublicKey)))
		buf.Write(in.PublicKey)
	}

	// Output count
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Outputs)))

	// Outputs
	for _, out := range tx.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Value)
		binary.Write(&buf, binary.LittleEndian, uint32(len(out.Script)))
		buf.Write(out.Script)
		buf.Write(out.Address[:])
	}

	// LockTime
	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	// Sequence (replay protection)
	binary.Write(&buf, binary.LittleEndian, tx.Sequence)

	return buf.Bytes()
}

// DeserializeTransaction parses a transaction from bytes.
func DeserializeTransaction(data []byte) (*Transaction, error) {
	buf := bytes.NewReader(data)
	tx := &Transaction{}

	// Version
	if err := binary.Read(buf, binary.LittleEndian, &tx.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	// Input count
	var inputCount uint32
	if err := binary.Read(buf, binary.LittleEndian, &inputCount); err != nil {
		return nil, fmt.Errorf("failed to read input count: %w", err)
	}

	// Inputs
	tx.Inputs = make([]TXInput, inputCount)
	for i := uint32(0); i < inputCount; i++ {
		if _, err := buf.Read(tx.Inputs[i].TxHash[:]); err != nil {
			return nil, fmt.Errorf("failed to read input hash: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &tx.Inputs[i].Index); err != nil {
			return nil, fmt.Errorf("failed to read input index: %w", err)
		}

		var sigLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &sigLen); err != nil {
			return nil, fmt.Errorf("failed to read sig length: %w", err)
		}
		tx.Inputs[i].Signature = make([]byte, sigLen)
		if _, err := buf.Read(tx.Inputs[i].Signature); err != nil {
			return nil, fmt.Errorf("failed to read signature: %w", err)
		}

		var pkLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &pkLen); err != nil {
			return nil, fmt.Errorf("failed to read pk length: %w", err)
		}
		tx.Inputs[i].PublicKey = make([]byte, pkLen)
		if _, err := buf.Read(tx.Inputs[i].PublicKey); err != nil {
			return nil, fmt.Errorf("failed to read public key: %w", err)
		}
	}

	// Output count
	var outputCount uint32
	if err := binary.Read(buf, binary.LittleEndian, &outputCount); err != nil {
		return nil, fmt.Errorf("failed to read output count: %w", err)
	}

	// Outputs
	tx.Outputs = make([]TXOutput, outputCount)
	for i := uint32(0); i < outputCount; i++ {
		if err := binary.Read(buf, binary.LittleEndian, &tx.Outputs[i].Value); err != nil {
			return nil, fmt.Errorf("failed to read output value: %w", err)
		}

		var scriptLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &scriptLen); err != nil {
			return nil, fmt.Errorf("failed to read script length: %w", err)
		}
		tx.Outputs[i].Script = make([]byte, scriptLen)
		if _, err := buf.Read(tx.Outputs[i].Script); err != nil {
			return nil, fmt.Errorf("failed to read script: %w", err)
		}

		if _, err := buf.Read(tx.Outputs[i].Address[:]); err != nil {
			return nil, fmt.Errorf("failed to read address: %w", err)
		}
	}

	// LockTime
	if err := binary.Read(buf, binary.LittleEndian, &tx.LockTime); err != nil {
		return nil, fmt.Errorf("failed to read locktime: %w", err)
	}

	// Sequence (optional - default 0 if not present)
	if buf.Len() >= 8 {
		if err := binary.Read(buf, binary.LittleEndian, &tx.Sequence); err != nil {
			// If we can't read sequence, default to 0
			tx.Sequence = 0
		}
	}

	return tx, nil
}

// SerializeForSigning returns the transaction data for signing (without signatures).
func (tx *Transaction) SerializeForSigning() []byte {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Input count
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Inputs)))

	// Inputs (without signatures)
	for _, in := range tx.Inputs {
		buf.Write(in.TxHash[:])
		binary.Write(&buf, binary.LittleEndian, in.Index)
		// Write empty signature placeholder
		binary.Write(&buf, binary.LittleEndian, uint32(0))
		binary.Write(&buf, binary.LittleEndian, uint32(len(in.PublicKey)))
		buf.Write(in.PublicKey)
	}

	// Output count
	binary.Write(&buf, binary.LittleEndian, uint32(len(tx.Outputs)))

	// Outputs
	for _, out := range tx.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Value)
		binary.Write(&buf, binary.LittleEndian, uint32(len(out.Script)))
		buf.Write(out.Script)
		buf.Write(out.Address[:])
	}

	// LockTime
	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	return buf.Bytes()
}

// SignInput signs a specific input with the provided private key.
func (tx *Transaction) SignInput(inputIndex int, privKey ed25519.PrivateKey) error {
	if inputIndex < 0 || inputIndex >= len(tx.Inputs) {
		return fmt.Errorf("invalid input index: %d", inputIndex)
	}

	// Get the data to sign
	data := tx.SerializeForSigning()

	// Sign
	sig := ed25519.Sign(privKey, data)

	tx.Inputs[inputIndex].Signature = sig
	tx.Inputs[inputIndex].PublicKey = privKey.Public().(ed25519.PublicKey)

	return nil
}

// VerifyInput verifies the signature for a specific input.
func (tx *Transaction) VerifyInput(inputIndex int) bool {
	if inputIndex < 0 || inputIndex >= len(tx.Inputs) {
		return false
	}

	input := tx.Inputs[inputIndex]

	if len(input.Signature) == 0 || len(input.PublicKey) == 0 {
		return false
	}

	// Get the data that was signed
	data := tx.SerializeForSigning()

	return ed25519.Verify(input.PublicKey, data, input.Signature)
}

// VerifyAllInputs verifies all input signatures.
func (tx *Transaction) VerifyAllInputs() bool {
	for i := range tx.Inputs {
		if !tx.VerifyInput(i) {
			return false
		}
	}
	return true
}

// TotalInputValue returns the sum of all input values (requires UTXO lookup).
func (tx *Transaction) TotalInputValue(utxoProvider UTXOProvider) (uint64, error) {
	var total uint64
	for _, input := range tx.Inputs {
		utxo, err := utxoProvider.GetUTXO(input.TxHash, input.Index)
		if err != nil {
			return 0, fmt.Errorf("failed to get UTXO: %w", err)
		}
		total += utxo.Value
	}
	return total, nil
}

// TotalOutputValue returns the sum of all output values.
func (tx *Transaction) TotalOutputValue() uint64 {
	var total uint64
	for _, output := range tx.Outputs {
		total += output.Value
	}
	return total
}

// CalculateFee calculates the transaction fee based on size and fee rate.
// Fee = (inputs serialization + outputs serialization) * feePerByte
func (tx *Transaction) CalculateFee(feePerByte uint64) uint64 {
	if tx.IsCoinbase() {
		return 0
	}

	// Calculate serialized size
	size := tx.SerializeSize()
	fee := uint64(size) * feePerByte

	// Minimum fee of 1 satoshi
	if fee < 1 {
		fee = 1
	}

	return fee
}

// SerializeSize returns the approximate serialized size of the transaction.
func (tx *Transaction) SerializeSize() int {
	size := 4 // version
	size += 4 // input count

	for _, in := range tx.Inputs {
		size += 32 // TxHash
		size += 4  // Index
		size += 4  // sig length
		size += len(in.Signature)
		size += 4  // pubkey length
		size += len(in.PublicKey)
	}

	size += 4 // output count

	for _, out := range tx.Outputs {
		size += 8  // value
		size += 4  // script length
		size += len(out.Script)
		size += 32 // address
	}

	size += 4 // locktime

	return size
}

// GetInputsValue returns total value from inputs (requires UTXO lookup).
func (tx *Transaction) GetInputsValue(utxoProvider UTXOProvider) (uint64, error) {
	if tx.IsCoinbase() {
		return 0, nil
	}

	var total uint64
	for _, in := range tx.Inputs {
		utxo, err := utxoProvider.GetUTXO(in.TxHash, in.Index)
		if err != nil {
			return 0, fmt.Errorf("failed to get UTXO: %w", err)
		}
		total += utxo.Value
	}
	return total, nil
}

// GetFee calculates the actual fee: inputs - outputs.
// Returns error if inputs < outputs (invalid transaction).
func (tx *Transaction) GetFee(utxoProvider UTXOProvider) (uint64, error) {
	if tx.IsCoinbase() {
		return 0, nil
	}

	inputsValue, err := tx.GetInputsValue(utxoProvider)
	if err != nil {
		return 0, err
	}

	outputsValue := tx.TotalOutputValue()

	if inputsValue < outputsValue {
		return 0, fmt.Errorf("inputs value %d < outputs value %d", inputsValue, outputsValue)
	}

	return inputsValue - outputsValue, nil
}

// IsCoinbase checks if this is a coinbase transaction.
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 &&
		tx.Inputs[0].TxHash == [32]byte{} &&
		tx.Inputs[0].Index == 0xffffffff
}

// ToInterfacesTXInput converts to interfaces.TXInput.
func (in *TXInput) ToInterfacesTXInput() interfaces.TXInput {
	return interfaces.TXInput{
		TxHash:    in.TxHash,
		Index:     in.Index,
		Signature: in.Signature,
		PublicKey: in.PublicKey,
	}
}

// ToInterfacesTXOutput converts to interfaces.TXOutput.
func (out *TXOutput) ToInterfacesTXOutput() interfaces.TXOutput {
	return interfaces.TXOutput{
		Value:   out.Value,
		Script:  out.Script,
		Address: out.Address,
	}
}

// ToInterfacesUTXO converts to *interfaces.UTXO.
func (u *UTXO) ToInterfacesUTXO() *interfaces.UTXO {
	return &interfaces.UTXO{
		TxHash:  u.TxHash,
		Index:   u.Index,
		Value:   u.Value,
		Script:  u.Script,
		Address: u.Address,
	}
}

// UTXOProvider interface for transaction validation.
type UTXOProvider interface {
	GetUTXO(txHash [32]byte, index uint32) (*UTXO, error)
}
