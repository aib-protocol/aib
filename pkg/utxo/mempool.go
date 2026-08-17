// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Mempool Module
package utxo

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MempoolEntry represents a transaction in the mempool.
type MempoolEntry struct {
	Tx      *Transaction
	AddedAt time.Time
	Fee     uint64
	FeeRate float64 // fee per byte
}

// Mempool holds unconfirmed transactions.
type Mempool struct {
	entries    map[[32]byte]*MempoolEntry // txHash -> entry
	spentUTXOs map[string]bool            // 已使用的UTXO（防双花）
	maxSize    int
	minFee     uint64
	mu         sync.RWMutex
}

// NewMempool creates a new mempool with the specified max size and min fee.
func NewMempool(maxSize int, minFee uint64) *Mempool {
	return &Mempool{
		entries:    make(map[[32]byte]*MempoolEntry),
		spentUTXOs: make(map[string]bool),
		maxSize:    maxSize,
		minFee:     minFee,
	}
}

// AddTransaction adds a transaction to the mempool.
// It validates the transaction signature, UTXO availability, double-spend, and fee.
func (m *Mempool) AddTransaction(tx *Transaction, utxoProvider UTXOProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	txHash := tx.Hash()

	// Check if transaction already exists in mempool
	if _, exists := m.entries[txHash]; exists {
		return fmt.Errorf("transaction already in mempool: %x", txHash)
	}

	// Check mempool size limit
	if len(m.entries) >= m.maxSize {
		return fmt.Errorf("mempool full: max size %d", m.maxSize)
	}

	// Validate transaction inputs and check for double spend
	for i, input := range tx.Inputs {
		utxoKey := UTXOKey(input.TxHash, input.Index)

		// Check if UTXO is already spent in mempool (double spend check)
		if m.spentUTXOs[utxoKey] {
			return fmt.Errorf("double spend detected in mempool: %s", utxoKey)
		}

		// Check if UTXO exists in the UTXO set
		utxo, err := utxoProvider.GetUTXO(input.TxHash, input.Index)
		if err != nil {
			return fmt.Errorf("UTXO not found: %s", err)
		}

		// Verify signature
		if !tx.VerifyInput(i) {
			return fmt.Errorf("invalid signature for input %d", i)
		}

		// Mark UTXO as spent in mempool
		m.spentUTXOs[utxoKey] = true

		// Store UTXO value for fee calculation
		_ = utxo.Value // Used implicitly in fee calculation
	}

	// Verify fee meets minimum requirement
	actualFee, err := tx.GetFee(utxoProvider)
	if err != nil {
		return fmt.Errorf("failed to calculate fee: %w", err)
	}

	if actualFee < m.minFee {
		return fmt.Errorf("fee %d below minimum %d", actualFee, m.minFee)
	}

	// Calculate fee rate (fee per byte)
	txSize := tx.SerializeSize()
	if txSize == 0 {
		txSize = 1 // Avoid division by zero
	}
	feeRate := float64(actualFee) / float64(txSize)

	// Create entry
	entry := &MempoolEntry{
		Tx:      tx,
		AddedAt: time.Now(),
		Fee:     actualFee,
		FeeRate: feeRate,
	}

	m.entries[txHash] = entry

	return nil
}

// RemoveTransaction removes a transaction from the mempool.
func (m *Mempool) RemoveTransaction(txHash [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.entries[txHash]
	if !exists {
		return
	}

	// Release UTXOs
	for _, input := range entry.Tx.Inputs {
		utxoKey := UTXOKey(input.TxHash, input.Index)
		delete(m.spentUTXOs, utxoKey)
	}

	delete(m.entries, txHash)
}

// GetTransaction returns a transaction from the mempool by hash.
func (m *Mempool) GetTransaction(txHash [32]byte) *Transaction {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if entry, exists := m.entries[txHash]; exists {
		return entry.Tx
	}
	return nil
}

// GetTransactionsForBlock returns transactions sorted by fee rate for block inclusion.
// It selects transactions up to maxBlockSize bytes.
func (m *Mempool) GetTransactionsForBlock(maxBlockSize int) []*Transaction {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Sort entries by fee rate (descending)
	entries := make([]*MempoolEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FeeRate > entries[j].FeeRate
	})

	// Select transactions up to maxBlockSize
	var result []*Transaction
	var totalSize int

	for _, entry := range entries {
		txSize := entry.Tx.SerializeSize()
		if totalSize+txSize > maxBlockSize {
			continue
		}

		result = append(result, entry.Tx)
		totalSize += txSize
	}

	return result
}

// RemoveConfirmed removes confirmed transactions from the mempool.
func (m *Mempool) RemoveConfirmed(txHashes [][32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, txHash := range txHashes {
		entry, exists := m.entries[txHash]
		if !exists {
			continue
		}

		// Release UTXOs
		for _, input := range entry.Tx.Inputs {
			utxoKey := UTXOKey(input.TxHash, input.Index)
			delete(m.spentUTXOs, utxoKey)
		}

		delete(m.entries, txHash)
	}
}

// Size returns the number of transactions in the mempool.
func (m *Mempool) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.entries)
}

// Prune removes expired transactions from the mempool.
func (m *Mempool) Prune(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for txHash, entry := range m.entries {
		if entry.AddedAt.Before(cutoff) {
			// Release UTXOs
			for _, input := range entry.Tx.Inputs {
				utxoKey := UTXOKey(input.TxHash, input.Index)
				delete(m.spentUTXOs, utxoKey)
			}
			delete(m.entries, txHash)
		}
	}
}

// GetFeeRates returns the fee rates of all transactions in the mempool, sorted descending (for testing).
func (m *Mempool) GetFeeRates() []float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rates := make([]float64, 0, len(m.entries))
	for _, entry := range m.entries {
		rates = append(rates, entry.FeeRate)
	}
	// Sort descending (highest fee rate first)
	sort.Slice(rates, func(i, j int) bool {
		return rates[i] > rates[j]
	})
	return rates
}

// GetSpentUTXOs returns a copy of the spent UTXO set (for testing).
func (m *Mempool) GetSpentUTXOs() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]bool)
	for k, v := range m.spentUTXOs {
		result[k] = v
	}
	return result
}

// GetAllEntries returns all entries in the mempool (for API queries).
func (m *Mempool) GetAllEntries() []*MempoolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*MempoolEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}
	return entries
}
