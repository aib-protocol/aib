// Package wallet provides payment routing and L1/L2 transaction management.
package wallet

import (
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// PaymentMethod represents the layer used for payment.
type PaymentMethod int

const (
	// PaymentL1 represents a Layer 1 (on-chain) payment.
	PaymentL1 PaymentMethod = iota
	// PaymentL2 represents a Layer 2 (off-chain/channel) payment.
	PaymentL2
)

// String returns the string representation of the payment method.
func (pm PaymentMethod) String() string {
	switch pm {
	case PaymentL1:
		return "L1"
	case PaymentL2:
		return "L2"
	default:
		return "Unknown"
	}
}

// PaymentResult represents the result of a payment operation.
type PaymentResult struct {
	Method      PaymentMethod // L1 or L2
	TxHash      [32]byte      // Transaction hash (L1) or channel update ID (L2)
	Fee         uint64        // Fee paid
	Amount      uint64        // Amount sent
	Success     bool          // Whether the payment succeeded
	Error       string        // Error message if failed
	ConfirmTime uint64        // Confirmation time (ms)
}

// String returns a human-readable result summary.
func (pr *PaymentResult) String() string {
	if pr.Success {
		return fmt.Sprintf("Payment successful: Method=%s, Amount=%d, Fee=%d, TxHash=%x",
			pr.Method, pr.Amount, pr.Fee, pr.TxHash)
	}
	return fmt.Sprintf("Payment failed: Method=%s, Error=%s", pr.Method, pr.Error)
}

// L2Channel represents a Layer 2 payment channel.
type L2Channel struct {
	ChannelID   string
	PeerAddress [32]byte
	Balance     uint64
	FeeRate     uint64
	IsActive    bool
}

// PaymentManager manages payment routing between L1 and L2.
type PaymentManager struct {
	wallet      *Wallet
	l2Threshold uint64                // Below this value, prefer L2
	feePerByte  uint64                // Current fee rate for L1 transactions
	l2Channels  map[string]*L2Channel // Available L2 channels
	utxoStore   *utxo.UTXOStore       // UTXO store for balance queries
	mu          sync.RWMutex
}

// NewPaymentManager creates a new payment manager.
func NewPaymentManager(wallet *Wallet) *PaymentManager {
	return &PaymentManager{
		wallet:      wallet,
		l2Threshold: DefaultL2Threshold,
		feePerByte:  1, // Default fee rate
		l2Channels:  make(map[string]*L2Channel),
	}
}

// SetUTXOStore sets the UTXO store for balance queries.
func (pm *PaymentManager) SetUTXOStore(store *utxo.UTXOStore) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.utxoStore = store
}

// SetL2Threshold sets the threshold amount for L2 payment preference.
// Payments below this threshold will prefer L2 when available.
func (pm *PaymentManager) SetL2Threshold(amount uint64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.l2Threshold = amount
}

// SetFeePerByte sets the current fee rate for L1 transactions.
func (pm *PaymentManager) SetFeePerByte(fee uint64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.feePerByte = fee
}

// AddL2Channel adds an L2 payment channel to the manager.
func (pm *PaymentManager) AddL2Channel(channel *L2Channel) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.l2Channels[channel.ChannelID] = channel
}

// RemoveL2Channel removes an L2 payment channel.
func (pm *PaymentManager) RemoveL2Channel(channelID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.l2Channels, channelID)
}

// GetL2Channel retrieves an L2 channel by ID.
func (pm *PaymentManager) GetL2Channel(channelID string) *L2Channel {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.l2Channels[channelID]
}

// GetActiveL2Channel returns the first active L2 channel with sufficient balance.
func (pm *PaymentManager) GetActiveL2Channel(minBalance uint64) *L2Channel {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, ch := range pm.l2Channels {
		if ch.IsActive && ch.Balance >= minBalance {
			return ch
		}
	}
	return nil
}

// SmartSend automatically chooses between L1 and L2 based on amount and availability.
// For amounts below l2Threshold, it prefers L2 if an active channel exists.
func (pm *PaymentManager) SmartSend(to [32]byte, amount uint64) *PaymentResult {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Check if we should use L2
	if amount < pm.l2Threshold {
		// Look for an active L2 channel
		for _, ch := range pm.l2Channels {
			if ch.IsActive && ch.Balance >= amount {
				pm.mu.RUnlock()
				result := pm.SendL2(to, amount, ch.ChannelID)
				pm.mu.RLock()
				return result
			}
		}
	}

	// Fall back to L1
	pm.mu.RUnlock()
	result := pm.SendL1(to, amount)
	pm.mu.RLock()
	return result
}

// SendL1 creates and signs an L1 (on-chain) transaction.
// This selects UTXOs, creates a transaction, and signs it.
func (pm *PaymentManager) SendL1(to [32]byte, amount uint64) *PaymentResult {
	pm.mu.RLock()
	store := pm.utxoStore
	fromAddr := pm.wallet.GetAddress()
	pm.mu.RUnlock()

	if store == nil {
		return &PaymentResult{
			Method:  PaymentL1,
			Success: false,
			Error:   "UTXO store not configured",
		}
	}

	// Select UTXOs to cover amount + fee
	// Estimate fee (approximate: 200 bytes * feePerByte)
	estimatedFee := uint64(200) * pm.feePerByte
	totalNeeded := amount + estimatedFee

	utxos, totalValue, err := store.GetUTXOsForAmount(fromAddr, totalNeeded)
	if err != nil {
		return &PaymentResult{
			Method:  PaymentL1,
			Success: false,
			Error:   fmt.Sprintf("insufficient balance: %v", err),
		}
	}

	// Calculate actual fee
	actualFee := totalValue - amount
	if actualFee < 1 {
		actualFee = 1
	}

	// Create transaction inputs
	inputs := make([]utxo.TXInput, len(utxos))
	for i, u := range utxos {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// Create transaction outputs
	// Output 0: recipient
	// Output 1: change (if any)
	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: to,
			Script:  nil,
		},
	}

	changeAmount := totalValue - amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: fromAddr,
			Script:  nil,
		})
	}

	// Create transaction
	tx := utxo.NewTransaction(inputs, outputs)

	// Sign all inputs
	for i := range inputs {
		if err := pm.wallet.SignTransaction(tx, i); err != nil {
			return &PaymentResult{
				Method:  PaymentL1,
				Success: false,
				Error:   fmt.Sprintf("failed to sign input %d: %v", i, err),
			}
		}
	}

	// Get transaction hash
	txHash := tx.Hash()

	return &PaymentResult{
		Method:      PaymentL1,
		TxHash:      txHash,
		Fee:         actualFee,
		Amount:      amount,
		Success:     true,
		ConfirmTime: 0, // Would be set after confirmation
	}
}

// SendL2 sends a payment via an L2 channel.
// This is a simplified implementation - actual L2 logic depends on the channel protocol.
func (pm *PaymentManager) SendL2(to [32]byte, amount uint64, channelID string) *PaymentResult {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	ch, exists := pm.l2Channels[channelID]
	if !exists {
		return &PaymentResult{
			Method:  PaymentL2,
			Success: false,
			Error:   fmt.Sprintf("channel not found: %s", channelID),
		}
	}

	if !ch.IsActive {
		return &PaymentResult{
			Method:  PaymentL2,
			Success: false,
			Error:   "channel is not active",
		}
	}

	if ch.Balance < amount {
		return &PaymentResult{
			Method:  PaymentL2,
			Success: false,
			Error:   fmt.Sprintf("insufficient channel balance: have %d, need %d", ch.Balance, amount),
		}
	}

	// Calculate L2 fee (typically much lower than L1)
	l2Fee := (amount * ch.FeeRate) / 10000 // Fee rate as basis points

	// Update channel balance
	ch.Balance -= amount

	// In a real implementation, this would:
	// 1. Create a channel update message
	// 2. Sign it with the wallet
	// 3. Send to the peer
	// 4. Wait for confirmation

	// For now, generate a mock hash from channel data
	txHash := [32]byte{}
	copy(txHash[:], []byte(channelID))

	return &PaymentResult{
		Method:      PaymentL2,
		TxHash:      txHash,
		Fee:         l2Fee,
		Amount:      amount,
		Success:     true,
		ConfirmTime: 100, // L2 is faster
	}
}

// EstimateFee estimates the fee for an L1 transaction of the given size.
func (pm *PaymentManager) EstimateFee(txSizeBytes uint64) uint64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return txSizeBytes * pm.feePerByte
}

// GetBalance returns the wallet's total balance (L1 + L2).
func (pm *PaymentManager) GetBalance() (l1Balance, l2Balance uint64, err error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Get L1 balance from UTXO store
	if pm.utxoStore != nil {
		l1Balance = pm.utxoStore.GetBalance(pm.wallet.GetAddress())
	}

	// Sum L2 channel balances
	for _, ch := range pm.l2Channels {
		if ch.IsActive {
			l2Balance += ch.Balance
		}
	}

	return l1Balance, l2Balance, nil
}
