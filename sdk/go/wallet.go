// Package aib provides the AIB 2.0 SDK for Go.
// This SDK supports wallet management, transaction signing, and blockchain interaction.
//
// AIB 2.0 Specifications:
//   - Cryptographic Algorithm: Ed25519
//   - Address Format: Bech32m (HRP: "aib")
//   - Transaction Model: UTXO
package aib

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Address represents a 32-byte AIB blockchain address.
type Address [32]byte

// Wallet represents a cryptographic wallet with Ed25519 key pair.
type Wallet struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	address    Address
}

// NewWallet creates a new wallet with a randomly generated key pair.
func NewWallet() (*Wallet, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	wallet := &Wallet{
		privateKey: privateKey,
		publicKey:  publicKey,
	}

	// Derive address from public key using SHA256
	addressHash := sha256.Sum256(publicKey)
	copy(wallet.address[:], addressHash[:])

	return wallet, nil
}

// NewWalletFromSeed creates a wallet from a 32-byte seed.
func NewWalletFromSeed(seed []byte) (*Wallet, error) {
	if len(seed) != 32 {
		return nil, fmt.Errorf("seed must be 32 bytes, got %d", len(seed))
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	wallet := &Wallet{
		privateKey: privateKey,
		publicKey:  publicKey,
	}

	// Derive address from public key
	addressHash := sha256.Sum256(publicKey)
	copy(wallet.address[:], addressHash[:])

	return wallet, nil
}

// NewWalletFromPrivateKey imports a wallet from an existing private key.
func NewWalletFromPrivateKey(privateKeyHex string) (*Wallet, error) {
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}

	if len(privateKeyBytes) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(privateKeyBytes))
	}

	privateKey := ed25519.NewKeyFromSeed(privateKeyBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	wallet := &Wallet{
		privateKey: privateKey,
		publicKey:  publicKey,
	}

	// Derive address from public key
	addressHash := sha256.Sum256(publicKey)
	copy(wallet.address[:], addressHash[:])

	return wallet, nil
}

// GetAddress returns the wallet's address as a byte slice.
func (w *Wallet) GetAddress() Address {
	return w.address
}

// GetAddressString returns the wallet's address in Bech32m format.
func (w *Wallet) GetAddressString() string {
	return EncodeBech32m("aib", w.address[:])
}

// GetPublicKey returns the wallet's public key.
func (w *Wallet) GetPublicKey() []byte {
	return w.publicKey
}

// GetPrivateKey returns the wallet's private key (WARNING: Keep this secure!).
func (w *Wallet) GetPrivateKey() []byte {
	return w.privateKey
}

// GetPrivateKeyHex returns the wallet's private key as hex string (WARNING: Keep this secure!).
func (w *Wallet) GetPrivateKeyHex() string {
	return hex.EncodeToString(w.privateKey)
}

// Sign signs a message with the wallet's private key.
func (w *Wallet) Sign(message []byte) []byte {
	return ed25519.Sign(w.privateKey, message)
}

// Verify verifies a signature for a message using the wallet's public key.
func (w *Wallet) Verify(message []byte, signature []byte) bool {
	return ed25519.Verify(w.publicKey, message, signature)
}

// SignTransaction signs a transaction with the wallet's private key.
func (w *Wallet) SignTransaction(tx *Transaction) error {
	txHash := tx.Hash()

	// Sign the transaction hash
	signature := ed25519.Sign(w.privateKey, txHash[:])

	// Add signature and public key to all inputs
	for i := range tx.Inputs {
		tx.Inputs[i].Signature = signature
		tx.Inputs[i].PublicKey = w.publicKey
	}

	return nil
}

// EncodeBech32m encodes data to Bech32m format.
// This is a simplified implementation for demonstration.
// For production, use a proper bech32m library.
func EncodeBech32m(hrp string, data []byte) string {
	// Simplified bech32m encoding
	// In production, use github.com/libsv/go-bek or similar
	checksum := sha256.Sum256(append([]byte(hrp), data...))
	checksum = sha256.Sum256(checksum[:])

	// Convert to base32-like encoding
	charset := "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	encoded := ""

	// Take 5-bit chunks
	for i := 0; i < len(data)*8-3; i += 5 {
		index := (int(data[i/8]) >> (8 - (i % 8) - 5) & 0x1F)
		encoded += string(charset[index%32])
	}

	// Add checksum (last 6 characters)
	for i := 0; i < 6; i++ {
		idx := checksum[i] % 32
		encoded += string(charset[idx])
	}

	return hrp + "1" + encoded
}
