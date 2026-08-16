// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Mempool Tests
package utxo

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"
)

// mockUTXOProvider implements UTXOProvider for testing.
type mockUTXOProvider struct {
	utxos map[string]*UTXO
}

func newMockUTXOProvider() *mockUTXOProvider {
	return &mockUTXOProvider{
		utxos: make(map[string]*UTXO),
	}
}

func (m *mockUTXOProvider) addUTXO(utxo *UTXO) {
	key := UTXOKey(utxo.TxHash, utxo.Index)
	m.utxos[key] = utxo
}

func (m *mockUTXOProvider) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	key := UTXOKey(txHash, index)
	utxo, exists := m.utxos[key]
	if !exists {
		return nil, fmt.Errorf("UTXO not found: %s", key)
	}
	return utxo, nil
}

// generateKeyPair generates a new ed25519 key pair for testing.
func generateKeyPair() (ed25519.PrivateKey, ed25519.PublicKey, [32]byte) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	var addr [32]byte
	copy(addr[:], pubKey[:32])
	return privKey, pubKey, addr
}

// createTestTransaction creates a signed transaction for testing.
func createTestTransaction(privKey ed25519.PrivateKey, addr [32]byte, inputUTXOs []*UTXO, outputValue uint64, fee uint64) *Transaction {
	// Calculate total input value
	var totalInput uint64
	for _, utxo := range inputUTXOs {
		totalInput += utxo.Value
	}

	// Create inputs
	inputs := make([]TXInput, len(inputUTXOs))
	for i, utxo := range inputUTXOs {
		inputs[i] = TXInput{
			TxHash:    utxo.TxHash,
			Index:     utxo.Index,
			Signature: nil, // Will sign after
			PublicKey: privKey.Public().(ed25519.PublicKey),
		}
	}

	// Create output
	outputs := []TXOutput{
		{
			Value:   outputValue,
			Script:  []byte{},
			Address: addr,
		},
	}

	// Create change output if there's change
	change := totalInput - outputValue - fee
	if change > 0 {
		outputs = append(outputs, TXOutput{
			Value:   change,
			Script:  []byte{},
			Address: addr,
		})
	}

	tx := NewTransaction(inputs, outputs)

	// Sign all inputs
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			panic(err)
		}
	}

	return tx
}

// createFundingTransaction creates a transaction that funds an address.
func createFundingTransaction(addr [32]byte, amount uint64) (*Transaction, []*UTXO) {
	// Create coinbase-like input
	var zeroHash [32]byte
	input := TXInput{
		TxHash:    zeroHash,
		Index:     0,
		Signature: []byte("coinbase"),
		PublicKey: []byte{},
	}

	output := TXOutput{
		Value:   amount,
		Script:  []byte{},
		Address: addr,
	}

	tx := NewTransaction([]TXInput{input}, []TXOutput{output})
	txHash := tx.Hash()

	// Create UTXOs from this transaction
	utxos := []*UTXO{
		{
			TxHash:  txHash,
			Index:   0,
			Value:   amount,
			Script:  []byte{},
			Address: addr,
		},
	}

	return tx, utxos
}

// TestNewMempool tests mempool creation.
func TestNewMempool(t *testing.T) {
	mempool := NewMempool(1000, 100)

	if mempool == nil {
		t.Fatal("expected non-nil mempool")
	}

	if mempool.maxSize != 1000 {
		t.Errorf("expected maxSize 1000, got %d", mempool.maxSize)
	}

	if mempool.minFee != 100 {
		t.Errorf("expected minFee 100, got %d", mempool.minFee)
	}

	if mempool.Size() != 0 {
		t.Errorf("expected empty mempool, got size %d", mempool.Size())
	}
}

// TestAddTransaction tests adding valid transactions.
func TestAddTransaction(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	// Create key pair and funding
	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	// Add funding UTXOs to provider
	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	// Create transaction spending the funding
	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)

	// Add to mempool
	err := mempool.AddTransaction(tx, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check mempool size
	if mempool.Size() != 1 {
		t.Errorf("expected mempool size 1, got %d", mempool.Size())
	}

	// Check transaction can be retrieved
	txHash := tx.Hash()
	if mempool.GetTransaction(txHash) == nil {
		t.Error("expected to retrieve transaction")
	}
}

// TestAddTransaction_Duplicate tests adding duplicate transaction.
func TestAddTransaction_Duplicate(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)

	// Add first time
	err := mempool.AddTransaction(tx, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Try adding duplicate
	err = mempool.AddTransaction(tx, provider)
	if err == nil {
		t.Error("expected error for duplicate transaction")
	}
}

// TestAddTransaction_DoubleSpend tests double spend detection.
func TestAddTransaction_DoubleSpend(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	// Create first transaction spending the UTXO
	tx1 := createTestTransaction(privKey, addr, fundingUTXOs, 400, 50)

	// Add first transaction
	err := mempool.AddTransaction(tx1, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Create second transaction spending the same UTXO (double spend)
	tx2 := createTestTransaction(privKey, addr, fundingUTXOs, 300, 50)

	// Try adding double spend - should fail
	err = mempool.AddTransaction(tx2, provider)
	if err == nil {
		t.Error("expected error for double spend")
	}
}

// TestAddTransaction_InvalidFee tests fee validation.
func TestAddTransaction_InvalidFee(t *testing.T) {
	mempool := NewMempool(1000, 100) // minFee 100
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	// Create transaction with only 50 fee (below minimum)
	tx := createTestTransaction(privKey, addr, fundingUTXOs, 900, 50)

	err := mempool.AddTransaction(tx, provider)
	if err == nil {
		t.Error("expected error for fee below minimum")
	}
}

// TestAddTransaction_MempoolFull tests mempool size limit.
func TestAddTransaction_MempoolFull(t *testing.T) {
	maxSize := 5
	mempool := NewMempool(maxSize, 10)
	provider := newMockUTXOProvider()

	// Add transactions up to max
	for i := 0; i < maxSize; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)

		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}

		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
		err := mempool.AddTransaction(tx, provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}

	// Try adding one more - should fail
	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
	err := mempool.AddTransaction(tx, provider)
	if err == nil {
		t.Error("expected error when mempool is full")
	}
}

// TestRemoveTransaction tests removing transactions.
func TestRemoveTransaction(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
	txHash := tx.Hash()

	err := mempool.AddTransaction(tx, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Remove transaction
	mempool.RemoveTransaction(txHash)

	// Check mempool is empty
	if mempool.Size() != 0 {
		t.Errorf("expected mempool size 0, got %d", mempool.Size())
	}

	// Check spent UTXOs are released
	spentUTXOs := mempool.GetSpentUTXOs()
	if len(spentUTXOs) != 0 {
		t.Errorf("expected no spent UTXOs, got %d", len(spentUTXOs))
	}
}

// TestGetTransactionsForBlock tests fee-based transaction selection.
func TestGetTransactionsForBlock(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	var txHashes [][32]byte

	// Add transactions with different fees
	for i := 0; i < 5; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)

		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}

		// Fee increases with index
		fee := uint64((i + 1) * 20)
		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, fee)

		err := mempool.AddTransaction(tx, provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		txHashes = append(txHashes, tx.Hash())
	}

	// Get transactions for block (maxBlockSize = 10000)
	transactions := mempool.GetTransactionsForBlock(10000)

	// Should get all 5 transactions
	if len(transactions) != 5 {
		t.Errorf("expected 5 transactions, got %d", len(transactions))
	}

	// Check they're ordered by fee rate (highest first)
	feeRates := mempool.GetFeeRates()
	for i := 1; i < len(feeRates); i++ {
		if feeRates[i-1] < feeRates[i] {
			t.Errorf("transactions not sorted by fee rate: %v", feeRates)
		}
	}
}

// TestGetTransactionsForBlock_Limit tests block size limit.
func TestGetTransactionsForBlock_Limit(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	// Add several transactions
	for i := 0; i < 10; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)

		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}

		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
		err := mempool.AddTransaction(tx, provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}

	// Get transactions with small block size
	// This should limit how many are selected
	transactions := mempool.GetTransactionsForBlock(500) // Small block size

	// Should get fewer than all transactions due to size limit
	if len(transactions) >= 10 {
		t.Errorf("expected fewer than 10 transactions due to size limit, got %d", len(transactions))
	}
}

// TestRemoveConfirmed tests removing multiple confirmed transactions.
func TestRemoveConfirmed(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	var txHashes [][32]byte

	// Add 3 transactions
	for i := 0; i < 3; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)

		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}

		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
		err := mempool.AddTransaction(tx, provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		txHashes = append(txHashes, tx.Hash())
	}

	// "Confirm" first 2 transactions
	mempool.RemoveConfirmed(txHashes[:2])

	// Check mempool size
	if mempool.Size() != 1 {
		t.Errorf("expected mempool size 1, got %d", mempool.Size())
	}
}

// TestPrune tests pruning expired transactions.
func TestPrune(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
	err := mempool.AddTransaction(tx, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Manually set AddedAt to old time
	txHash := tx.Hash()
	mempool.mu.Lock()
	if entry, exists := mempool.entries[txHash]; exists {
		entry.AddedAt = time.Now().Add(-2 * time.Hour)
	}
	mempool.mu.Unlock()

	// Prune with 1 hour max age
	mempool.Prune(1 * time.Hour)

	// Transaction should be pruned
	if mempool.Size() != 0 {
		t.Errorf("expected mempool size 0 after prune, got %d", mempool.Size())
	}
}

// TestPrune_RecentTransactions tests that recent transactions are not pruned.
func TestPrune_RecentTransactions(t *testing.T) {
	mempool := NewMempool(1000, 10)
	provider := newMockUTXOProvider()

	privKey, _, addr := generateKeyPair()
	_, fundingUTXOs := createFundingTransaction(addr, 1000)

	for _, utxo := range fundingUTXOs {
		provider.addUTXO(utxo)
	}

	tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
	err := mempool.AddTransaction(tx, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Prune with 1 hour max age (transaction is recent)
	mempool.Prune(1 * time.Hour)

	// Transaction should NOT be pruned
	if mempool.Size() != 1 {
		t.Errorf("expected mempool size 1, got %d", mempool.Size())
	}
}

// TestMempool_Concurrency tests concurrent access to mempool.
func TestMempool_Concurrency(t *testing.T) {
	mempool := NewMempool(100, 10)
	provider := newMockUTXOProvider()

	done := make(chan bool)

	// Concurrent additions
	go func() {
		for i := 0; i < 10; i++ {
			privKey, _, addr := generateKeyPair()
			_, fundingUTXOs := createFundingTransaction(addr, 1000)

			for _, utxo := range fundingUTXOs {
				provider.addUTXO(utxo)
			}

			tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
			mempool.AddTransaction(tx, provider)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 10; i++ {
			mempool.Size()
			mempool.GetFeeRates()
		}
		done <- true
	}()

	<-done
	<-done

	// Just verify no deadlock or panic occurred
}
