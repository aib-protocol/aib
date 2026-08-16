// Package aib provides the AIB 2.0 SDK for Go.
// Transaction handling for UTXO-based transactions.
package aib

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Transaction represents a UTXO-based transaction.
type Transaction struct {
	Version    uint32
	Inputs     []TXInput
	Outputs    []TXOutput
	LockTime   uint32
	Sequence   uint64
}

// TXInput represents a transaction input (reference to a previous UTXO).
type TXInput struct {
	TxHash    [32]byte // Previous transaction hash
	Index     uint32   // Output index in previous transaction
	Signature []byte   // Signature proving ownership
	PublicKey []byte   // Public key of the sender
	Sequence  uint64   // Sequence number for replace-by-fee
}

// TXOutput represents a transaction output (new UTXO).
type TXOutput struct {
	Address   Address // Recipient address
	Amount    uint64  // Amount in smallest units (like satoshis)
	AssetID   [32]byte
	Metadata  []byte
}

// NewTransaction creates a new transaction with the specified inputs and outputs.
func NewTransaction(version uint32, inputs []TXInput, outputs []TXOutput) *Transaction {
	return &Transaction{
		Version:  version,
		Inputs:   inputs,
		Outputs:  outputs,
		LockTime: 0,
		Sequence: 0,
	}
}

// TXInput defines input parameters for creating a new transaction input.
type TXInputParams struct {
	TxHash    string // Previous transaction hash in hex
	Index     uint32 // Output index
	PublicKey []byte // Sender's public key
}

// TXOutputParams defines output parameters for creating a new transaction output.
type TXOutputParams struct {
	Address string // Recipient address (Bech32m)
	Amount  uint64 // Amount in smallest units
	AssetID string // Asset ID in hex (optional)
}

// BuildTransaction creates a transaction from input and output parameters.
func BuildTransaction(inputs []TXInputParams, outputs []TXOutputParams) (*Transaction, error) {
	tx := &Transaction{
		Version:  1,
		Inputs:   make([]TXInput, 0, len(inputs)),
		Outputs:  make([]TXOutput, 0, len(outputs)),
		LockTime: 0,
		Sequence: 0,
	}

	// Process inputs
	for _, in := range inputs {
		txHash, err := hex.DecodeString(in.TxHash)
		if err != nil {
			return nil, fmt.Errorf("invalid tx hash: %w", err)
		}

		if len(txHash) != 32 {
			return nil, fmt.Errorf("tx hash must be 32 bytes")
		}

		var txHashBytes [32]byte
		copy(txHashBytes[:], txHash)

		tx.Inputs = append(tx.Inputs, TXInput{
			TxHash:    txHashBytes,
			Index:     in.Index,
			PublicKey: in.PublicKey,
			Sequence:  0xFFFFFFFFFFFFFFFF,
		})
	}

	// Process outputs
	for _, out := range outputs {
		// Decode Bech32m address
		addressBytes, err := DecodeBech32m(out.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid address: %w", err)
		}

		var addr Address
		copy(addr[:], addressBytes)

		var assetID [32]byte
		if out.AssetID != "" {
			assetIDBytes, err := hex.DecodeString(out.AssetID)
			if err != nil {
				return nil, fmt.Errorf("invalid asset id: %w", err)
			}
			if len(assetIDBytes) == 32 {
				copy(assetID[:], assetIDBytes)
			}
		}

		tx.Outputs = append(tx.Outputs, TXOutput{
			Address:   addr,
			Amount:    out.Amount,
			AssetID:   assetID,
			Metadata:  nil,
		})
	}

	return tx, nil
}

// Hash computes the transaction hash (transaction ID).
func (tx *Transaction) Hash() [32]byte {
	// Serialize transaction
	data := tx.Serialize()

	// Double SHA256 for transaction hash
	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])

	return hash2
}

// Serialize serializes the transaction to bytes.
func (tx *Transaction) Serialize() []byte {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Number of inputs
	var numInputs = uint64(len(tx.Inputs))
	binary.Write(&buf, binary.LittleEndian, numInputs)

	// Inputs
	for _, in := range tx.Inputs {
		buf.Write(in.TxHash[:])
		binary.Write(&buf, binary.LittleEndian, in.Index)

		// Signature
		var sigLen = uint64(len(in.Signature))
		binary.Write(&buf, binary.LittleEndian, sigLen)
		if sigLen > 0 {
			buf.Write(in.Signature)
		}

		// Public key
		var pkLen = uint64(len(in.PublicKey))
		binary.Write(&buf, binary.LittleEndian, pkLen)
		if pkLen > 0 {
			buf.Write(in.PublicKey)
		}

		binary.Write(&buf, binary.LittleEndian, in.Sequence)
	}

	// Number of outputs
	var numOutputs = uint64(len(tx.Outputs))
	binary.Write(&buf, binary.LittleEndian, numOutputs)

	// Outputs
	for _, out := range tx.Outputs {
		buf.Write(out.Address[:])
		binary.Write(&buf, binary.LittleEndian, out.Amount)
		buf.Write(out.AssetID[:])

		// Metadata
		var metaLen = uint64(len(out.Metadata))
		binary.Write(&buf, binary.LittleEndian, metaLen)
		if metaLen > 0 {
			buf.Write(out.Metadata)
		}
	}

	// LockTime
	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	return buf.Bytes()
}

// DeserializeTransaction deserializes a transaction from bytes.
func DeserializeTransaction(data []byte) (*Transaction, error) {
	tx := &Transaction{}
	buf := bytes.NewBuffer(data)

	// Version
	if err := binary.Read(buf, binary.LittleEndian, &tx.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	// Number of inputs
	var numInputs uint64
	if err := binary.Read(buf, binary.LittleEndian, &numInputs); err != nil {
		return nil, fmt.Errorf("failed to read input count: %w", err)
	}

	tx.Inputs = make([]TXInput, numInputs)
	for i := uint64(0); i < numInputs; i++ {
		if _, err := buf.Read(tx.Inputs[i].TxHash[:]); err != nil {
			return nil, fmt.Errorf("failed to read tx hash: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &tx.Inputs[i].Index); err != nil {
			return nil, fmt.Errorf("failed to read index: %w", err)
		}

		// Signature
		var sigLen uint64
		if err := binary.Read(buf, binary.LittleEndian, &sigLen); err != nil {
			return nil, fmt.Errorf("failed to read sig len: %w", err)
		}
		if sigLen > 0 {
			tx.Inputs[i].Signature = make([]byte, sigLen)
			if _, err := buf.Read(tx.Inputs[i].Signature); err != nil {
				return nil, fmt.Errorf("failed to read signature: %w", err)
			}
		}

		// Public key
		var pkLen uint64
		if err := binary.Read(buf, binary.LittleEndian, &pkLen); err != nil {
			return nil, fmt.Errorf("failed to read pk len: %w", err)
		}
		if pkLen > 0 {
			tx.Inputs[i].PublicKey = make([]byte, pkLen)
			if _, err := buf.Read(tx.Inputs[i].PublicKey); err != nil {
				return nil, fmt.Errorf("failed to read public key: %w", err)
			}
		}

		if err := binary.Read(buf, binary.LittleEndian, &tx.Inputs[i].Sequence); err != nil {
			return nil, fmt.Errorf("failed to read sequence: %w", err)
		}
	}

	// Number of outputs
	var numOutputs uint64
	if err := binary.Read(buf, binary.LittleEndian, &numOutputs); err != nil {
		return nil, fmt.Errorf("failed to read output count: %w", err)
	}

	tx.Outputs = make([]TXOutput, numOutputs)
	for i := uint64(0); i < numOutputs; i++ {
		if _, err := buf.Read(tx.Outputs[i].Address[:]); err != nil {
			return nil, fmt.Errorf("failed to read address: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &tx.Outputs[i].Amount); err != nil {
			return nil, fmt.Errorf("failed to read amount: %w", err)
		}
		if _, err := buf.Read(tx.Outputs[i].AssetID[:]); err != nil {
			return nil, fmt.Errorf("failed to read asset id: %w", err)
		}

		// Metadata
		var metaLen uint64
		if err := binary.Read(buf, binary.LittleEndian, &metaLen); err != nil {
			return nil, fmt.Errorf("failed to read meta len: %w", err)
		}
		if metaLen > 0 {
			tx.Outputs[i].Metadata = make([]byte, metaLen)
			if _, err := buf.Read(tx.Outputs[i].Metadata); err != nil {
				return nil, fmt.Errorf("failed to read metadata: %w", err)
			}
		}
	}

	// LockTime
	if err := binary.Read(buf, binary.LittleEndian, &tx.LockTime); err != nil {
		return nil, fmt.Errorf("failed to read lock time: %w", err)
	}

	return tx, nil
}

// GetFee calculates the transaction fee (inputs sum - outputs sum).
func (tx *Transaction) GetFee() (uint64, error) {
	var inputSum, outputSum uint64

	for range tx.Inputs {
		// Note: This requires tracking UTXO values
		// For simplicity, returning 0 here
	}

	for _, out := range tx.Outputs {
		outputSum += out.Amount
	}

	return inputSum - outputSum, nil
}

// GetTxID returns the transaction ID as a hex string.
func (tx *Transaction) GetTxID() string {
	hash := tx.Hash()
	return hex.EncodeToString(hash[:])
}

// DecodeBech32m decodes a Bech32m address to raw bytes.
// This is a simplified implementation for demonstration.
func DecodeBech32m(address string) ([]byte, error) {
	// Simplified bech32m decoding
	// In production, use a proper bech32m library
	if len(address) < 6 {
		return nil, fmt.Errorf("address too short")
	}

	// Remove hrp and separator
	separatorIndex := -1
	for i := 0; i < len(address); i++ {
		if address[i] == '1' {
			separatorIndex = i
			break
		}
	}

	if separatorIndex == -1 {
		return nil, fmt.Errorf("invalid address format")
	}

	hrp := address[:separatorIndex]
	if hrp != "aib" {
		return nil, fmt.Errorf("invalid HRP: expected 'aib', got '%s'", hrp)
	}

	// For demonstration, just return the data part as base32 decoded
	// This is a simplified version
	charset := "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	dataStr := address[separatorIndex+1:]

	// Skip checksum (last 6 characters)
	dataStr = dataStr[:len(dataStr)-6]

	// Simple base32-like decoding
	data := make([]byte, 0, len(dataStr)*5/8)
	var buffer int
	var bitsLeft int

	for _, c := range dataStr {
		index := -1
		for i, ch := range charset {
			if ch == c {
				index = i
				break
			}
		}
		if index == -1 {
			return nil, fmt.Errorf("invalid character: %c", c)
		}

		buffer = (buffer << 5) | index
		bitsLeft += 5

		if bitsLeft >= 8 {
			bitsLeft -= 8
			data = append(data, byte(buffer>>bitsLeft))
		}
	}

	return data[:32], nil // Return first 32 bytes as address
}
