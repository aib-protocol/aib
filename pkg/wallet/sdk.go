// Package wallet provides the WalletSDK for AIB blockchain.
// This is a unified SDK that wraps wallet and payment functionality.
package wallet

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// SDKConfig contains configuration options for the WalletSDK.
type SDKConfig struct {
	// PrivateKey is an optional private key for wallet recovery.
	// If nil, a new wallet will be generated.
	PrivateKey []byte

	// L2Threshold is the amount threshold for preferring L2 payments.
	// Payments below this threshold will use L2 when available.
	L2Threshold uint64

	// FeePerByte is the fee rate for L1 transactions (in smallest units).
	FeePerByte uint64

	// UTXOStore is an optional UTXO store for balance queries.
	// If nil, a new store will be created.
	UTXOStore *utxo.UTXOStore
}

// BalanceResult represents the result of a balance query.
type BalanceResult struct {
	L1Balance uint64 // Layer 1 (on-chain) balance
	L2Balance uint64 // Layer 2 (off-chain/channel) balance
	Total     uint64 // Total balance (L1 + L2)
}

// SendResult represents the result of a send operation.
type SendResult struct {
	TxHash      [32]byte // Transaction hash or channel update ID
	Amount      uint64   // Amount sent
	Fee         uint64   // Fee paid
	Method      string   // "L1" or "L2"
	Success     bool     // Whether the operation succeeded
	Error       string   // Error message if failed
	ConfirmTime uint64   // Confirmation time in milliseconds
}

// TransactionRecord represents a transaction record for history.
type TransactionRecord struct {
	TxHash     [32]byte // Transaction hash
	Direction  string    // "sent" or "received"
	Amount     uint64    // Amount transferred
	Fee        uint64    // Fee paid
	Timestamp  uint64    // Unix timestamp
	Confirmations uint64 // Number of confirmations
	Method     string    // "L1" or "L2"
}

// WalletSDK provides a unified interface for wallet operations.
type WalletSDK struct {
	wallet     *Wallet
	payment    *PaymentManager
	utxoStore  *utxo.UTXOStore
	config     *SDKConfig
	history    []TransactionRecord // In-memory transaction history
	mu         sync.RWMutex
}

// NewWalletSDK creates a new WalletSDK instance with the given configuration.
func NewWalletSDK(config *SDKConfig) (*WalletSDK, error) {
	// Apply default configuration
	if config == nil {
		config = &SDKConfig{}
	}

	// Create or recover wallet
	var wallet *Wallet
	var err error

	if config.PrivateKey != nil {
		wallet, err = FromPrivateKeyBytes(config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create wallet from private key: %w", err)
		}
	} else {
		wallet, err = NewWallet()
		if err != nil {
			return nil, fmt.Errorf("failed to generate new wallet: %w", err)
		}
	}

	// Use provided UTXO store or create new one
	utxoStore := config.UTXOStore
	if utxoStore == nil {
		utxoStore = utxo.NewUTXOStore()
	}

	// Create payment manager
	payment := NewPaymentManager(wallet)
	payment.SetUTXOStore(utxoStore)

	// Apply configuration
	if config.L2Threshold > 0 {
		payment.SetL2Threshold(config.L2Threshold)
	}

	if config.FeePerByte > 0 {
		payment.SetFeePerByte(config.FeePerByte)
	}

	sdk := &WalletSDK{
		wallet:    wallet,
		payment:   payment,
		utxoStore: utxoStore,
		config:    config,
		history:   make([]TransactionRecord, 0),
	}

	return sdk, nil
}

// NewWalletSDKFromKey creates a new WalletSDK from an existing private key.
func NewWalletSDKFromKey(privateKey ed25519.PrivateKey) (*WalletSDK, error) {
	privKeyBytes := privateKey.Seed()
	return NewWalletSDK(&SDKConfig{
		PrivateKey: privKeyBytes,
	})
}

// GetAddress returns the wallet's address.
func (sdk *WalletSDK) GetAddress() [32]byte {
	return sdk.wallet.GetAddress()
}

// GetAddressHex returns the wallet's address as a hex string.
func (sdk *WalletSDK) GetAddressHex() string {
	return sdk.wallet.GetAddressHex()
}

// GetAddressString returns the wallet's address as a Bech32m encoded string.
func (sdk *WalletSDK) GetAddressString() (string, error) {
	return sdk.wallet.GetAddressString()
}

// GetPublicKey returns the wallet's public key.
func (sdk *WalletSDK) GetPublicKey() ed25519.PublicKey {
	return sdk.wallet.GetPublicKey()
}

// Balance returns the current balance (L1 + L2).
func (sdk *WalletSDK) Balance() (*BalanceResult, error) {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()

	l1Balance, l2Balance, err := sdk.payment.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &BalanceResult{
		L1Balance: l1Balance,
		L2Balance: l2Balance,
		Total:     l1Balance + l2Balance,
	}, nil
}

// Send sends amount to the specified address.
// This method automatically chooses between L1 and L2 based on amount and availability.
func (sdk *WalletSDK) Send(toAddress string, amount uint64) (*SendResult, error) {
	// Parse address string to [32]byte
	var toAddr [32]byte
	addrBytes, err := parseAddress(toAddress)
	if err != nil {
		return &SendResult{Success: false, Error: err.Error()}, err
	}
	copy(toAddr[:], addrBytes[:32])

	// Send payment
	result := sdk.payment.SmartSend(toAddr, amount)

	// Convert to SDK result
	sdkResult := &SendResult{
		TxHash:      result.TxHash,
		Amount:      result.Amount,
		Fee:         result.Fee,
		Method:      result.Method.String(),
		Success:     result.Success,
		Error:       result.Error,
		ConfirmTime: result.ConfirmTime,
	}

	// Add to history if successful
	if result.Success {
		sdk.mu.Lock()
		sdk.history = append(sdk.history, TransactionRecord{
			TxHash:    result.TxHash,
			Direction: "sent",
			Amount:    result.Amount,
			Fee:       result.Fee,
			Timestamp: 0, // Would be set from actual timestamp
			Method:    result.Method.String(),
		})
		sdk.mu.Unlock()
	}

	return sdkResult, nil
}

// SendL1 sends amount via L1 (on-chain) transaction.
func (sdk *WalletSDK) SendL1(toAddress string, amount uint64) (*SendResult, error) {
	var toAddr [32]byte
	addrBytes, err := parseAddress(toAddress)
	if err != nil {
		return &SendResult{Success: false, Error: err.Error()}, err
	}
	copy(toAddr[:], addrBytes[:32])

	result := sdk.payment.SendL1(toAddr, amount)

	sdkResult := &SendResult{
		TxHash:      result.TxHash,
		Amount:      result.Amount,
		Fee:         result.Fee,
		Method:      result.Method.String(),
		Success:     result.Success,
		Error:       result.Error,
		ConfirmTime: result.ConfirmTime,
	}

	if result.Success {
		sdk.mu.Lock()
		sdk.history = append(sdk.history, TransactionRecord{
			TxHash:    result.TxHash,
			Direction: "sent",
			Amount:    result.Amount,
			Fee:       result.Fee,
			Method:    "L1",
		})
		sdk.mu.Unlock()
	}

	return sdkResult, nil
}

// SendL2 sends amount via L2 (off-chain) channel.
func (sdk *WalletSDK) SendL2(toAddress string, amount uint64, channelID string) (*SendResult, error) {
	var toAddr [32]byte
	addrBytes, err := parseAddress(toAddress)
	if err != nil {
		return &SendResult{Success: false, Error: err.Error()}, err
	}
	copy(toAddr[:], addrBytes[:32])

	result := sdk.payment.SendL2(toAddr, amount, channelID)

	sdkResult := &SendResult{
		TxHash:      result.TxHash,
		Amount:      result.Amount,
		Fee:         result.Fee,
		Method:      result.Method.String(),
		Success:     result.Success,
		Error:       result.Error,
		ConfirmTime: result.ConfirmTime,
	}

	if result.Success {
		sdk.mu.Lock()
		sdk.history = append(sdk.history, TransactionRecord{
			TxHash:    result.TxHash,
			Direction: "sent",
			Amount:    result.Amount,
			Fee:       result.Fee,
			Method:    "L2",
		})
		sdk.mu.Unlock()
	}

	return sdkResult, nil
}

// Receive returns the wallet's receiving address.
// Returns hex format by default, which is more reliable.
func (sdk *WalletSDK) Receive() (string, error) {
	// Use hex format as default to avoid Bech32m encoding issues
	return sdk.wallet.GetAddressHex(), nil
}

// History returns the transaction history.
// The limit parameter specifies the maximum number of records to return.
// A limit of 0 returns all records.
func (sdk *WalletSDK) History(limit int) ([]TransactionRecord, error) {
	sdk.mu.RLock()
	defer sdk.mu.RUnlock()

	if limit <= 0 || limit > len(sdk.history) {
		limit = len(sdk.history)
	}

	// Return most recent first (reverse order)
	result := make([]TransactionRecord, limit)
	for i := 0; i < limit; i++ {
		result[i] = sdk.history[len(sdk.history)-1-i]
	}

	return result, nil
}

// AddL2Channel adds an L2 payment channel to the SDK.
func (sdk *WalletSDK) AddL2Channel(channel *L2Channel) {
	sdk.payment.AddL2Channel(channel)
}

// GetL2Channel retrieves an L2 channel by ID.
func (sdk *WalletSDK) GetL2Channel(channelID string) *L2Channel {
	return sdk.payment.GetL2Channel(channelID)
}

// SetL2Threshold sets the threshold amount for L2 payment preference.
func (sdk *WalletSDK) SetL2Threshold(amount uint64) {
	sdk.payment.SetL2Threshold(amount)
}

// SetFeePerByte sets the fee per byte for L1 transactions.
func (sdk *WalletSDK) SetFeePerByte(fee uint64) {
	sdk.payment.SetFeePerByte(fee)
}

// GetUTXOStore returns the underlying UTXO store.
func (sdk *WalletSDK) GetUTXOStore() *utxo.UTXOStore {
	return sdk.utxoStore
}

// GetPaymentManager returns the underlying payment manager.
func (sdk *WalletSDK) GetPaymentManager() *PaymentManager {
	return sdk.payment
}

// Sign signs the given data using the wallet's private key.
func (sdk *WalletSDK) Sign(data []byte) []byte {
	return sdk.wallet.Sign(data)
}

// Verify verifies a signature for the given data.
func (sdk *WalletSDK) Verify(data, signature []byte) bool {
	return sdk.wallet.Verify(data, signature)
}

// ExportPrivateKey returns the private key as raw bytes.
// WARNING: This exposes the private key - handle with extreme care.
func (sdk *WalletSDK) ExportPrivateKey() []byte {
	return sdk.wallet.ExportPrivateKey()
}

// AddUTXO adds a UTXO to the store (for testing or receiving funds).
func (sdk *WalletSDK) AddUTXO(txHash [32]byte, index uint32, value uint64) error {
	utxo := &utxo.UTXO{
		TxHash:  txHash,
		Index:   index,
		Value:   value,
		Address: sdk.wallet.GetAddress(),
	}
	sdk.utxoStore.AddUTXO(utxo)
	return nil
}

// parseAddress parses an address string to bytes.
// It supports hex format and Bech32m format.
func parseAddress(addr string) ([]byte, error) {
	// Try hex format first
	if len(addr) == 64 {
		var result [32]byte
		_, err := hex.Decode(result[:], []byte(addr))
		if err == nil {
			return result[:], nil
		}
	}

	// Try Bech32m format using utxo package
	addrObj, err := utxo.AddressFromString(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address format: %w", err)
	}
	return addrObj[:], nil
}
