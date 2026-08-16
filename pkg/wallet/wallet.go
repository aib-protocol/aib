// Package wallet provides wallet SDK for AIB blockchain.
// Supports key management, signing, and payment operations.
package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// DefaultL2Threshold is the default threshold for L2 payments (100 AIB in smallest units).
const DefaultL2Threshold uint64 = 100

// Wallet represents a cryptographic wallet with key pair and address.
type Wallet struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	address    [32]byte
	mu         sync.RWMutex
}

// NewWallet generates a new wallet with a random key pair.
func NewWallet() (*Wallet, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Derive address from public key using SHA256
	address := sha256.Sum256(publicKey)
	var addr [32]byte
	copy(addr[:], address[:])

	return &Wallet{
		privateKey: privateKey,
		publicKey:  publicKey,
		address:    addr,
	}, nil
}

// FromPrivateKey creates a wallet from an existing private key.
func FromPrivateKey(privKey ed25519.PrivateKey) *Wallet {
	publicKey := privKey.Public().(ed25519.PublicKey)

	// Derive address from public key using SHA256
	hash := sha256.Sum256(publicKey)
	var addr [32]byte
	copy(addr[:], hash[:])

	return &Wallet{
		privateKey: privKey,
		publicKey:  publicKey,
		address:    addr,
	}
}

// FromPrivateKeyBytes creates a wallet from raw private key bytes.
func FromPrivateKeyBytes(privKeyBytes []byte) (*Wallet, error) {
	if len(privKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: expected %d, got %d",
			ed25519.PrivateKeySize, len(privKeyBytes))
	}

	privKey := ed25519.PrivateKey(privKeyBytes)
	return FromPrivateKey(privKey), nil
}

// GetAddress returns the wallet's address as a 32-byte array.
func (w *Wallet) GetAddress() [32]byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.address
}

// GetAddressHex returns the wallet's address as a hex string.
func (w *Wallet) GetAddressHex() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return hex.EncodeToString(w.address[:])
}

// GetAddressString returns the wallet's address as a Bech32m encoded string.
func (w *Wallet) GetAddressString() (string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return utxo.AddressToString(w.address)
}

// GetPublicKey returns the wallet's public key.
func (w *Wallet) GetPublicKey() ed25519.PublicKey {
	w.mu.RLock()
	defer w.mu.RUnlock()
	pub := make(ed25519.PublicKey, len(w.publicKey))
	copy(pub, w.publicKey)
	return pub
}

// Sign signs the given data using the wallet's private key.
func (w *Wallet) Sign(data []byte) []byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return ed25519.Sign(w.privateKey, data)
}

// Verify verifies a signature for the given data using the wallet's public key.
func (w *Wallet) Verify(data, signature []byte) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return ed25519.Verify(w.publicKey, data, signature)
}

// ExportPrivateKey returns the private key as raw bytes.
// WARNING: This exposes the private key - handle with extreme care.
func (w *Wallet) ExportPrivateKey() []byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	priv := make([]byte, len(w.privateKey))
	copy(priv, w.privateKey)
	return priv
}

// ImportPrivateKey imports a private key from raw bytes.
func (w *Wallet) ImportPrivateKey(privKeyBytes []byte) error {
	if len(privKeyBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key size: expected %d, got %d",
			ed25519.PrivateKeySize, len(privKeyBytes))
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.privateKey = ed25519.PrivateKey(privKeyBytes)
	w.publicKey = w.privateKey.Public().(ed25519.PublicKey)

	// Recompute address
	hash := sha256.Sum256(w.publicKey)
	copy(w.address[:], hash[:])

	return nil
}

// SignTransaction signs a transaction input with the wallet's private key.
func (w *Wallet) SignTransaction(tx *utxo.Transaction, inputIndex int) error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return tx.SignInput(inputIndex, w.privateKey)
}

// VerifyTransaction verifies all input signatures in a transaction.
func (w *Wallet) VerifyTransaction(tx *utxo.Transaction) bool {
	return tx.VerifyAllInputs()
}

// GetBalance returns the wallet's balance from a UTXO store.
func (w *Wallet) GetBalance(store *utxo.UTXOStore) uint64 {
	addr := w.GetAddress()
	return store.GetBalance(addr)
}
