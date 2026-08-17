package aal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// Address represents a 20-byte EVM-compatible address
type Address [20]byte

// Hash represents a 32-byte hash
type Hash [32]byte

// BytesToAddress converts a byte slice to an Address
func BytesToAddress(b []byte) Address {
	var a Address
	a.SetBytes(b)
	return a
}

// SetBytes sets the address to the value of b
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-20:]
	}
	copy(a[20-len(b):], b)
}

// Bytes returns the address as a byte slice
func (a Address) Bytes() []byte {
	return a[:]
}

// Hex returns the hex string representation
func (a Address) Hex() string {
	return fmt.Sprintf("0x%x", a[:])
}

// String returns the hex string representation
func (a Address) String() string {
	return a.Hex()
}

// BytesToHash converts a byte slice to a Hash
func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

// SetBytes sets the hash to the value of b
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-32:]
	}
	copy(h[32-len(b):], b)
}

// Bytes returns the hash as a byte slice
func (h Hash) Bytes() []byte {
	return h[:]
}

// Hex returns the hex string representation
func (h Hash) Hex() string {
	return fmt.Sprintf("0x%x", h[:])
}

// String returns the hex string representation
func (h Hash) String() string {
	return h.Hex()
}

// BigToHash converts a big.Int to a Hash
func BigToHash(b *big.Int) Hash {
	return BytesToHash(b.Bytes())
}

// HashToBig converts a Hash to a big.Int
func HashToBig(h Hash) *big.Int {
	return new(big.Int).SetBytes(h[:])
}

// BigToAddress converts a big.Int to an Address
func BigToAddress(b *big.Int) Address {
	return BytesToAddress(b.Bytes())
}

// ConvertInterfacesAddress converts interfaces.Address (32 bytes) to aal.Address (20 bytes)
func ConvertInterfacesAddress(addr interfaces.Address) Address {
	// Use first 20 bytes for EVM compatibility
	return BytesToAddress(addr[:20])
}

// ConvertToInterfacesAddress converts aal.Address (20 bytes) to interfaces.Address (32 bytes)
func ConvertToInterfacesAddress(addr Address) interfaces.Address {
	var result interfaces.Address
	copy(result[:], addr[:])
	return result
}

// Keccak256Hash computes a hash using SHA256 (simplified, not keccak256)
// Note: For production, consider using actual keccak256
func Keccak256Hash(data []byte) Hash {
	h := sha256.Sum256(data)
	return BytesToHash(h[:])
}

// Log represents an event log emitted during contract execution
type Log struct {
	Address     Address // contract address
	Topics      []Hash  // list of topics
	Data        []byte  // event data
	BlockNumber uint64  // block number
	TxHash      Hash    // transaction hash
	TxIndex     uint    // transaction index
	BlockHash   Hash    // block hash
	Index       uint    // log index
}

// Rules represents consensus rules for a block
type Rules struct {
	IsHomestead      bool
	IsEIP150         bool
	IsEIP155         bool
	IsEIP158         bool
	IsByzantium      bool
	IsConstantinople bool
	IsPetersburg     bool
	IsIstanbul       bool
	IsBerlin         bool
	IsLondon         bool
}

// ChainConfig represents the chain configuration
type ChainConfig struct {
	ChainID     *big.Int
	BlockNumber *big.Int
	Rules       Rules
}

// DefaultChainConfig returns a default chain configuration
func DefaultChainConfig() *ChainConfig {
	return &ChainConfig{
		ChainID:     big.NewInt(8888),
		BlockNumber: big.NewInt(0),
		Rules: Rules{
			IsHomestead:      true,
			IsEIP150:         true,
			IsEIP155:         true,
			IsEIP158:         true,
			IsByzantium:      true,
			IsConstantinople: true,
			IsPetersburg:     true,
			IsIstanbul:       true,
			IsBerlin:         true,
			IsLondon:         true,
		},
	}
}

// ParseHexAddress parses a hex string to Address
func ParseHexAddress(s string) (Address, error) {
	var a Address
	// Remove 0x prefix if present
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return a, err
	}
	a.SetBytes(b)
	return a, nil
}

// ParseHexHash parses a hex string to Hash
func ParseHexHash(s string) (Hash, error) {
	var h Hash
	// Remove 0x prefix if present
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return h, err
	}
	h.SetBytes(b)
	return h, nil
}
