// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Address Module with Bech32m encoding (BIP 350)
package utxo

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// Bech32mHRP is the Human Readable Part for AIB addresses.
const (
	// Bech32mHRP is the Human Readable Part for AIB addresses.
	Bech32mHRP = "aib"
)

// charset is the bech32 character set for encoding.
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// EncodeBech32m encodes a 32-byte address to Bech32m format.
// Uses BIP 350 bech32m encoding with 6-character checksum.
func EncodeBech32m(hrp string, data []byte) (string, error) {
	if len(data) != 32 {
		return "", fmt.Errorf("data must be 32 bytes, got %d", len(data))
	}

	// Convert to 5-bit groups
	converted := convertBits(data, 8, 5, true)
	if converted == nil {
		return "", fmt.Errorf("failed to convert bits")
	}

	// Calculate checksum (BIP 350)
	values := make([]byte, len(converted)+6)
	copy(values, converted)

	// Generate checksum
	checksum := bech32mChecksum(hrp, values[:len(converted)])
	copy(values[len(converted):], checksum)

	// Encode to bech32m string
	var buf bytes.Buffer
	buf.WriteString(hrp)
	buf.WriteString("1")

	for _, v := range values {
		if int(v) >= len(charset) {
			return "", fmt.Errorf("invalid bech32m value: %d", v)
		}
		buf.WriteByte(charset[v])
	}

	return buf.String(), nil
}

// DecodeBech32m decodes a Bech32m encoded address.
// Returns the HRP and 32-byte data payload.
func DecodeBech32m(encoded string) (string, []byte, error) {
	if len(encoded) < 14 {
		return "", nil, fmt.Errorf("encoded string too short")
	}

	// Find the separator
	sepIndex := -1
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '1' {
			sepIndex = i
			break
		}
	}

	if sepIndex == -1 {
		return "", nil, fmt.Errorf("invalid bech32 string: no separator")
	}

	hrp := encoded[:sepIndex]
	if hrp != Bech32mHRP {
		return "", nil, fmt.Errorf("invalid HRP: expected %s, got %s", Bech32mHRP, hrp)
	}

	dataStr := encoded[sepIndex+1:]
	if len(dataStr) < 6 {
		return "", nil, fmt.Errorf("data too short")
	}

	// Decode characters
	values := make([]byte, len(dataStr))
	for i, c := range []byte(dataStr) {
		idx := bytes.IndexByte([]byte(charset), c)
		if idx == -1 {
			return "", nil, fmt.Errorf("invalid character: %c", c)
		}
		values[i] = byte(idx)
	}

	// Verify checksum
	if !verifyBech32mChecksum(hrp, values) {
		return "", nil, fmt.Errorf("checksum verification failed")
	}

	// Remove checksum
	dataLen := len(values) - 6
	values = values[:dataLen]

	// Convert from 5-bit groups to bytes
	decoded := convertBits(values, 5, 8, false)
	if decoded == nil || len(decoded) != 32 {
		return "", nil, fmt.Errorf("failed to convert bits")
	}

	return hrp, decoded, nil
}

// convertBits converts between different bit groups.
func convertBits(data []byte, fromBits, toBits int, pad bool) []byte {
	acc := 0
	bits := 0
	result := make([]byte, 0, len(data)*fromBits/toBits+1)
	maxv := (1 << toBits) - 1

	for _, b := range data {
		acc = (acc << fromBits) | int(b)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((acc>>bits)&maxv))
		}
	}

	if pad && bits > 0 {
		result = append(result, byte((acc<<(toBits-bits))&maxv))
	}

	return result
}

// bech32mChecksum generates a 6-byte checksum for bech32m encoding.
// Based on BIP 350.
func bech32mChecksum(hrp string, data []byte) []byte {
	// Compute bech32m polymod
	values := expandHrp(hrp)
	values = append(values, data...)
	polymod := bech32mPolymod(values) ^ 1

	result := make([]byte, 6)
	for i := 0; i < 6; i++ {
		result[i] = byte((polymod >> (5 * (5 - i))) & 31)
	}

	return result
}

// bech32mPolymod computes the bech32m checksum polynomial.
func bech32mPolymod(values []byte) int {
	// Generator constants for bech32m
	generators := []int{
		0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3,
	}
	chk := 1
	for _, v := range values {
		top := chk >> 25
		chk = (chk & 0x1ffffff) << 5 ^ int(v)
		for i := 0; i < 5; i++ {
			if (top>>i)&1 == 1 {
				chk ^= generators[i]
			}
		}
	}
	return chk
}

// expandHrp expands the HRP into values for checksum computation.
func expandHrp(hrp string) []byte {
	result := make([]byte, len(hrp)*2+1)
	for i, c := range []byte(hrp) {
		result[i] = c >> 5
		result[i+len(hrp)+1] = c & 31
	}
	return result
}

// verifyBech32mChecksum verifies the bech32m checksum.
func verifyBech32mChecksum(hrp string, values []byte) bool {
	expanded := expandHrp(hrp)
	combined := make([]byte, len(expanded)+len(values))
	copy(combined, expanded)
	copy(combined[len(expanded):], values)

	return bech32mPolymod(combined)^1 == 0
}

// AddressFromPublicKey creates an address from a public key.
// Uses SHA256 of the public key as the address.
func AddressFromPublicKey(pubKey []byte) [32]byte {
	// Hash the public key
	hash := sha256.Sum256(pubKey)
	var addr [32]byte
	copy(addr[:], hash[:32])
	return addr
}

// AddressFromBytes creates an address from bytes.
func AddressFromBytes(data []byte) ([32]byte, error) {
	if len(data) != 32 {
		return [32]byte{}, fmt.Errorf("data must be 32 bytes")
	}
	var addr [32]byte
	copy(addr[:], data)
	return addr, nil
}

// AddressToString converts an address to Bech32m string.
func AddressToString(addr [32]byte) (string, error) {
	return EncodeBech32m(Bech32mHRP, addr[:])
}

// AddressFromString parses a Bech32m string to an address.
func AddressFromString(encoded string) ([32]byte, error) {
	_, data, err := DecodeBech32m(encoded)
	if err != nil {
		return [32]byte{}, err
	}

	var addr [32]byte
	copy(addr[:], data)
	return addr, nil
}

// ValidateAddress validates a Bech32m encoded address.
func ValidateAddress(encoded string) error {
	_, data, err := DecodeBech32m(encoded)
	if err != nil {
		return err
	}

	if len(data) != 32 {
		return fmt.Errorf("invalid address length: %d", len(data))
	}

	return nil
}

// AddressEqual compares two addresses.
func AddressEqual(a, b [32]byte) bool {
	return a == b
}

// IsZeroAddress checks if an address is zero.
func IsZeroAddress(addr [32]byte) bool {
	return addr == [32]byte{}
}
