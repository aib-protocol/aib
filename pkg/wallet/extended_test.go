// Package wallet provides additional tests for wallet features.
// These tests cover multi-address management, transaction history, balance sync, and backup/restore.
package wallet

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// ==================== Multi-Address Management Tests ====================

// TestMultiAddress_CreateMultipleWallets tests creating and managing multiple wallets.
func TestMultiAddress_CreateMultipleWallets(t *testing.T) {
	// Create multiple wallets
	wallets := make([]*Wallet, 5)
	addresses := make(map[string]bool)

	for i := 0; i < 5; i++ {
		wallet, err := NewWallet()
		if err != nil {
			t.Fatalf("Failed to create wallet %d: %v", i, err)
		}
		wallets[i] = wallet

		addrHex := wallet.GetAddressHex()
		if addresses[addrHex] {
			t.Errorf("Duplicate address generated: %s", addrHex)
		}
		addresses[addrHex] = true

		t.Logf("Created wallet %d with address: %s", i, addrHex)
	}

	if len(addresses) != 5 {
		t.Errorf("Expected 5 unique addresses, got %d", len(addresses))
	}
}

// TestMultiAddress_AddressList tests managing a list of wallet addresses.
func TestMultiAddress_AddressList(t *testing.T) {
	// Create a simple address list manager
	type AddressEntry struct {
		Address  [32]byte
		Label    string
		IsActive bool
	}

	entries := make([]AddressEntry, 0)

	// Create wallets and add to list
	for i := 0; i < 3; i++ {
		wallet, err := NewWallet()
		if err != nil {
			t.Fatalf("Failed to create wallet: %v", err)
		}

		entry := AddressEntry{
			Address:  wallet.GetAddress(),
			Label:    string(rune('A' + i)),
			IsActive: true,
		}
		entries = append(entries, entry)
	}

	// Verify all addresses are unique
	for i, e1 := range entries {
		for j, e2 := range entries {
			if i != j && e1.Address == e2.Address {
				t.Errorf("Duplicate address at index %d and %d", i, j)
			}
		}
	}

	// Test address lookup
	found := false
	for _, e := range entries {
		if e.Label == "B" {
			found = true
			t.Logf("Found address with label B: %s", hex.EncodeToString(e.Address[:]))
		}
	}

	if !found {
		t.Error("Failed to find address with label B")
	}

	// Test deactivation
	for i := range entries {
		if entries[i].Label == "A" {
			entries[i].IsActive = false
		}
	}

	activeCount := 0
	for _, e := range entries {
		if e.IsActive {
			activeCount++
		}
	}

	if activeCount != 2 {
		t.Errorf("Expected 2 active addresses, got %d", activeCount)
	}

	t.Logf("Address list test passed: %d total, %d active", len(entries), activeCount)
}

// TestMultiAddress_UTXOMultiAddressBalance tests balance query across multiple addresses.
func TestMultiAddress_UTXOMultiAddressBalance(t *testing.T) {
	// Create multiple wallets
	wallet1, _ := NewWallet()
	wallet2, _ := NewWallet()
	wallet3, _ := NewWallet()

	// Create a shared UTXO store
	store := utxo.NewUTXOStore()

	// Add UTXOs for each wallet
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x01},
		Index:   0,
		Value:   1000,
		Address: wallet1.GetAddress(),
	})
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x02},
		Index:   0,
		Value:   2000,
		Address: wallet2.GetAddress(),
	})
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x03},
		Index:   0,
		Value:   3000,
		Address: wallet3.GetAddress(),
	})

	// Query balances for each wallet
	balance1 := store.GetBalance(wallet1.GetAddress())
	balance2 := store.GetBalance(wallet2.GetAddress())
	balance3 := store.GetBalance(wallet3.GetAddress())

	if balance1 != 1000 {
		t.Errorf("Wallet1 balance should be 1000, got %d", balance1)
	}
	if balance2 != 2000 {
		t.Errorf("Wallet2 balance should be 2000, got %d", balance2)
	}
	if balance3 != 3000 {
		t.Errorf("Wallet3 balance should be 3000, got %d", balance3)
	}

	// Total balance across all wallets
	totalBalance := balance1 + balance2 + balance3
	if totalBalance != 6000 {
		t.Errorf("Total balance should be 6000, got %d", totalBalance)
	}

	t.Logf("Multi-address balances - Wallet1: %d, Wallet2: %d, Wallet3: %d, Total: %d",
		balance1, balance2, balance3, totalBalance)
}

// TestMultiAddress_PaymentFromMultipleWallets tests sending payments from multiple wallets.
func TestMultiAddress_PaymentFromMultipleWallets(t *testing.T) {
	// Create sender wallets
	sender1, _ := NewWallet()
	sender2, _ := NewWallet()
	receiver, _ := NewWallet()

	// Setup payment managers
	pm1 := NewPaymentManager(sender1)
	pm2 := NewPaymentManager(sender2)

	// Setup UTXO stores with funds
	store1 := utxo.NewUTXOStore()
	store1.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x10},
		Index:   0,
		Value:   5000,
		Address: sender1.GetAddress(),
	})
	pm1.SetUTXOStore(store1)

	store2 := utxo.NewUTXOStore()
	store2.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x20},
		Index:   0,
		Value:   3000,
		Address: sender2.GetAddress(),
	})
	pm2.SetUTXOStore(store2)

	// Send payments from each wallet
	result1 := pm1.SendL1(receiver.GetAddress(), 1000)
	if !result1.Success {
		t.Fatalf("Payment from sender1 failed: %s", result1.Error)
	}

	result2 := pm2.SendL1(receiver.GetAddress(), 500)
	if !result2.Success {
		t.Fatalf("Payment from sender2 failed: %s", result2.Error)
	}

	t.Logf("Multi-wallet payment test - Sender1: %s, Sender2: %s",
		result1.String(), result2.String())
}

// TestMultiAddress_WalletSet tests managing a set of wallets.
func TestMultiAddress_WalletSet(t *testing.T) {
	type WalletInfo struct {
		wallet *Wallet
		name   string
	}

	walletSet := make([]WalletInfo, 0)

	// Add wallets with names
	names := []string{"Alice", "Bob", "Charlie", "Alice"} // Duplicate name for testing

	for _, name := range names {
		wallet, err := NewWallet()
		if err != nil {
			t.Fatalf("Failed to create wallet: %v", err)
		}
		walletSet = append(walletSet, WalletInfo{
			wallet: wallet,
			name:   name,
		})
	}

	// Find wallet by name
	findByName := func(target string) *Wallet {
		for _, wi := range walletSet {
			if wi.name == target {
				return wi.wallet
			}
		}
		return nil
	}

	// Test finding existing wallet
	alice := findByName("Alice")
	if alice == nil {
		t.Error("Should find Alice")
	}

	// Test finding non-existent wallet
	noOne := findByName("David")
	if noOne != nil {
		t.Error("Should not find David")
	}

	// Count wallets by name
	nameCount := make(map[string]int)
	for _, wi := range walletSet {
		nameCount[wi.name]++
	}

	if nameCount["Alice"] != 2 {
		t.Errorf("Expected 2 Alice wallets, got %d", nameCount["Alice"])
	}
	if nameCount["Bob"] != 1 {
		t.Errorf("Expected 1 Bob wallet, got %d", nameCount["Bob"])
	}

	t.Logf("Wallet set test - Total: %d, Names: %v", len(walletSet), nameCount)
}

// ==================== Transaction History Tests ====================

// TestHistory_EmptyHistory tests querying empty transaction history.
func TestHistory_EmptyHistory(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Query history with no transactions
	history, err := sdk.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d records", len(history))
	}

	t.Log("Empty history test passed")
}

// TestHistory_LimitZero tests history with zero limit.
func TestHistory_LimitZero(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds and send transactions
	txHash := [32]byte{0xaa}
	sdk.AddUTXO(txHash, 0, 10000)
	sdk.SetL2Threshold(100000)

	toAddr := sdk.GetAddressHex()
	sdk.Send(toAddr, 100)
	sdk.Send(toAddr, 200)
	sdk.Send(toAddr, 300)

	// Query with limit 0 (should return all)
	history, err := sdk.History(0)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("Expected 3 records with limit 0, got %d", len(history))
	}

	t.Logf("History with limit 0: %d records", len(history))
}

// TestHistory_LimitGreaterThanAvailable tests history limit larger than available.
func TestHistory_LimitGreaterThanAvailable(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds and send one transaction
	txHash := [32]byte{0xbb}
	sdk.AddUTXO(txHash, 0, 10000)
	sdk.SetL2Threshold(100000)

	toAddr := sdk.GetAddressHex()
	sdk.Send(toAddr, 100)

	// Query with limit larger than available
	history, err := sdk.History(100)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 record, got %d", len(history))
	}

	t.Logf("History with large limit: %d records", len(history))
}

// TestHistory_NegativeLimit tests history with negative limit.
func TestHistory_NegativeLimit(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds and send transactions
	txHash := [32]byte{0xcc}
	sdk.AddUTXO(txHash, 0, 10000)
	sdk.SetL2Threshold(100000)

	toAddr := sdk.GetAddressHex()
	sdk.Send(toAddr, 100)
	sdk.Send(toAddr, 200)

	// Query with negative limit (should return all)
	history, err := sdk.History(-1)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("Expected 2 records with negative limit, got %d", len(history))
	}

	t.Logf("History with negative limit: %d records", len(history))
}

// TestHistory_Order tests that history returns most recent first.
func TestHistory_Order(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds and send multiple transactions
	txHash := [32]byte{0xdd}
	sdk.AddUTXO(txHash, 0, 100000)
	sdk.SetL2Threshold(100000)

	// Send transactions with different amounts
	toAddr := sdk.GetAddressHex()
	sdk.Send(toAddr, 10)
	sdk.Send(toAddr, 20)
	sdk.Send(toAddr, 30)

	// Get history
	history, err := sdk.History(0)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	// Verify order: most recent first
	if len(history) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(history))
	}

	// The first record should have the largest amount (30)
	// because we send in order: 10, 20, 30 and history returns most recent first
	if history[0].Amount != 30 {
		t.Errorf("First (most recent) should be 30, got %d", history[0].Amount)
	}

	if history[1].Amount != 20 {
		t.Errorf("Second should be 20, got %d", history[1].Amount)
	}

	if history[2].Amount != 10 {
		t.Errorf("Third should be 10, got %d", history[2].Amount)
	}

	t.Logf("History order test passed: [%d, %d, %d]",
		history[0].Amount, history[1].Amount, history[2].Amount)
}

// TestHistory_L1L2Transactions tests history records for both L1 and L2 transactions.
func TestHistory_L1L2Transactions(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds
	txHash := [32]byte{0xee}
	sdk.AddUTXO(txHash, 0, 10000)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "history-test-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Set threshold to use L2 for small amounts
	sdk.SetL2Threshold(100)

	toAddr := sdk.GetAddressHex()

	// Send L1 payment
	resultL1, err := sdk.SendL1(toAddr, 500)
	if err != nil {
		t.Fatalf("SendL1 failed: %v", err)
	}
	if !resultL1.Success {
		t.Fatalf("L1 payment failed: %s", resultL1.Error)
	}

	// Send L2 payment
	resultL2, err := sdk.SendL2(toAddr, 50, "history-test-channel")
	if err != nil {
		t.Fatalf("SendL2 failed: %v", err)
	}
	if !resultL2.Success {
		t.Fatalf("L2 payment failed: %s", resultL2.Error)
	}

	// Get history
	history, err := sdk.History(0)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(history))
	}

	// Check methods in history
	var hasL1, hasL2 bool
	for _, tx := range history {
		if tx.Method == "L1" {
			hasL1 = true
		}
		if tx.Method == "L2" {
			hasL2 = true
		}
	}

	if !hasL1 {
		t.Error("History should contain L1 transaction")
	}
	if !hasL2 {
		t.Error("History should contain L2 transaction")
	}

	t.Logf("History L1/L2 test - L1: %v, L2: %v", hasL1, hasL2)
}

// TestHistory_FailedTransactionNotRecorded tests that failed transactions are not recorded.
func TestHistory_FailedTransactionNotRecorded(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Don't add any funds

	// Try to send (should fail)
	toAddr := sdk.GetAddressHex()
	result, err := sdk.Send(toAddr, 1000)
	if err == nil && result.Success {
		t.Fatal("Payment should fail with no funds")
	}

	// Get history - should be empty
	history, err := sdk.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("Failed transaction should not be recorded, got %d records", len(history))
	}

	t.Log("Failed transaction not recorded test passed")
}

// ==================== Balance Sync Tests ====================

// TestBalanceSync_InitialBalance tests initial balance is zero.
func TestBalanceSync_InitialBalance(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.L1Balance != 0 || balance.L2Balance != 0 || balance.Total != 0 {
		t.Errorf("Initial balance should be zero, got L1=%d, L2=%d, Total=%d",
			balance.L1Balance, balance.L2Balance, balance.Total)
	}

	t.Logf("Initial balance: L1=%d, L2=%d, Total=%d",
		balance.L1Balance, balance.L2Balance, balance.Total)
}

// TestBalanceSync_AfterUTXOAdd tests balance after adding UTXOs.
func TestBalanceSync_AfterUTXOAdd(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add multiple UTXOs
	type UTXOEntry struct {
		txHash [32]byte
		index  uint32
		value  uint64
	}
	utxos := []UTXOEntry{
		{txHash: [32]byte{0x01}, index: 0, value: 100},
		{txHash: [32]byte{0x02}, index: 0, value: 200},
		{txHash: [32]byte{0x03}, index: 0, value: 300},
		{txHash: [32]byte{0x04}, index: 0, value: 400},
	}

	for _, u := range utxos {
		err := sdk.AddUTXO(u.txHash, u.index, u.value)
		if err != nil {
			t.Fatalf("AddUTXO failed: %v", err)
		}
	}

	// Check balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	expected := uint64(1000) // 100 + 200 + 300 + 400
	if balance.L1Balance != expected {
		t.Errorf("Expected balance %d, got %d", expected, balance.L1Balance)
	}

	if balance.Total != expected {
		t.Errorf("Expected total %d, got %d", expected, balance.Total)
	}

	t.Logf("Balance after adding UTXOs: %d", balance.L1Balance)
}

// TestBalanceSync_L2ChannelBalance tests balance with L2 channels.
func TestBalanceSync_L2ChannelBalance(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add L1 funds
	sdk.AddUTXO([32]byte{0x10}, 0, 5000)

	// Add multiple L2 channels
	channels := []*L2Channel{
		{
			ChannelID:   "channel-1",
			PeerAddress: sdk.GetAddress(),
			Balance:     1000,
			FeeRate:     5,
			IsActive:    true,
		},
		{
			ChannelID:   "channel-2",
			PeerAddress: sdk.GetAddress(),
			Balance:     2000,
			FeeRate:     10,
			IsActive:    true,
		},
		{
			ChannelID:   "channel-inactive",
			PeerAddress: sdk.GetAddress(),
			Balance:     500,
			FeeRate:     5,
			IsActive:    false, // Inactive channel should not count
		},
	}

	for _, ch := range channels {
		sdk.AddL2Channel(ch)
	}

	// Get balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.L1Balance != 5000 {
		t.Errorf("L1 balance should be 5000, got %d", balance.L1Balance)
	}

	// Only active channels count: 1000 + 2000 = 3000
	if balance.L2Balance != 3000 {
		t.Errorf("L2 balance should be 3000 (active only), got %d", balance.L2Balance)
	}

	expectedTotal := uint64(8000)
	if balance.Total != expectedTotal {
		t.Errorf("Total balance should be %d, got %d", expectedTotal, balance.Total)
	}

	t.Logf("Balance sync test - L1: %d, L2: %d, Total: %d",
		balance.L1Balance, balance.L2Balance, balance.Total)
}

// TestBalanceSync_AfterPayment tests balance after making payments.
func TestBalanceSync_AfterPayment(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add initial funds
	sdk.AddUTXO([32]byte{0x20}, 0, 10000)

	// Get initial balance
	initialBalance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get initial balance: %v", err)
	}

	t.Logf("Initial balance: %d", initialBalance.L1Balance)

	// Set threshold high to force L1
	sdk.SetL2Threshold(100000)

	// Send payment
	toAddr := sdk.GetAddressHex()
	result, err := sdk.Send(toAddr, 1000)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Payment should succeed: %s", result.Error)
	}

	// Send-to-self 不应改变总余额（仅内部转移/找零）
	if finalBalance, err := sdk.Balance(); err != nil {
		t.Fatalf("Failed to get final balance: %v", err)
	} else {
		t.Logf("After payment - Amount: %d, Fee: %d, Final L1: %d",
			result.Amount, result.Fee, finalBalance.L1Balance)
		if finalBalance.Total != initialBalance.Total {
			t.Errorf("Total balance changed on self-transfer: initial=%d final=%d",
				initialBalance.Total, finalBalance.Total)
		}
	}
}

// TestBalanceSync_AfterL2Payment tests balance after L2 payment.
func TestBalanceSync_AfterL2Payment(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add L1 funds (for any fees)
	sdk.AddUTXO([32]byte{0x30}, 0, 1000)

	// Add L2 channel with initial balance
	channel := &L2Channel{
		ChannelID:   "l2-sync-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     5000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Get initial balance
	initialBalance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get initial balance: %v", err)
	}

	initialL2 := initialBalance.L2Balance
	t.Logf("Initial L2 balance: %d", initialL2)

	// Send L2 payment
	toAddr := sdk.GetAddressHex()
	result, err := sdk.SendL2(toAddr, 1000, "l2-sync-channel")
	if err != nil {
		t.Fatalf("SendL2 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("L2 payment should succeed: %s", result.Error)
	}

	// Get final balance
	finalBalance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get final balance: %v", err)
	}

	// L2 balance should decrease by amount (5000 - 1000 = 4000)
	expectedL2 := uint64(4000)
	if finalBalance.L2Balance != expectedL2 {
		t.Errorf("L2 balance should be %d, got %d", expectedL2, finalBalance.L2Balance)
	}

	t.Logf("After L2 payment - Amount: %d, Fee: %d, Final L2: %d",
		result.Amount, result.Fee, finalBalance.L2Balance)
}

// TestBalanceSync_MultipleChannelChanges tests balance with multiple channel updates.
func TestBalanceSync_MultipleChannelChanges(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "multi-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     10000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Simulate multiple payments through the channel
	amounts := []uint64{1000, 2000, 500, 1500}

	for _, amt := range amounts {
		result := sdk.GetPaymentManager().SendL2(
			func() [32]byte { return sdk.GetAddress() }(),
			amt,
			"multi-channel",
		)
		if !result.Success {
			t.Logf("Payment of %d failed: %s", amt, result.Error)
		}
	}

	// Get final balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	// Original: 10000, spent: 1000+2000+500+1500 = 5000
	expectedL2 := uint64(5000)
	if balance.L2Balance != expectedL2 {
		t.Errorf("L2 balance should be %d, got %d", expectedL2, balance.L2Balance)
	}

	t.Logf("Multiple channel updates - Final L2 balance: %d", balance.L2Balance)
}

// TestBalanceSync_ConcurrentAccess tests balance with concurrent access.
func TestBalanceSync_ConcurrentAccess(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add initial funds
	sdk.AddUTXO([32]byte{0x40}, 0, 100000)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "concurrent-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     10000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Simulate concurrent balance reads
	done := make(chan uint64, 10)

	for i := 0; i < 10; i++ {
		go func() {
			balance, _ := sdk.Balance()
			done <- balance.Total
		}()
	}

	// Collect results
	var results []uint64
	for i := 0; i < 10; i++ {
		results = append(results, <-done)
	}

	// All results should be consistent
	first := results[0]
	for _, r := range results {
		if r != first {
			t.Errorf("Inconsistent balance: got %d, expected %d", r, first)
		}
	}

	t.Logf("Concurrent access test passed, balance: %d", first)
}

// ==================== Backup/Restore Tests ====================

// TestBackupRestore_ExportImportPrivateKey tests complete backup and restore of private key.
func TestBackupRestore_ExportImportPrivateKey(t *testing.T) {
	// Create original wallet
	original, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create original wallet: %v", err)
	}

	originalAddr := original.GetAddress()
	originalHex := original.GetAddressHex()
	originalPubKey := original.GetPublicKey()

	// Export private key (backup)
	backupData := original.ExportPrivateKey()
	if len(backupData) != ed25519.PrivateKeySize {
		t.Errorf("Backup data should be %d bytes, got %d", ed25519.PrivateKeySize, len(backupData))
	}

	t.Logf("Backed up private key (%d bytes)", len(backupData))

	// Import private key to new wallet (restore)
	restored, err := FromPrivateKeyBytes(backupData)
	if err != nil {
		t.Fatalf("Failed to restore wallet: %v", err)
	}

	// Verify address matches
	if originalAddr != restored.GetAddress() {
		t.Errorf("Restored address does not match original: %s vs %s",
			originalHex, restored.GetAddressHex())
	}

	// Verify public key matches
	if !bytes.Equal(originalPubKey, restored.GetPublicKey()) {
		t.Error("Restored public key does not match original")
	}

	// Verify signing capability
	data := []byte("Test message for verification")
	originalSig := original.Sign(data)
	restoredSig := restored.Sign(data)

	// Both should produce valid signatures for the same data
	if !original.Verify(data, originalSig) {
		t.Error("Original signature verification failed")
	}
	if !restored.Verify(data, restoredSig) {
		t.Error("Restored signature verification failed")
	}

	t.Logf("Backup/Restore test passed - Address: %s", originalHex)
}

// TestBackupRestore_ExportImportMultipleWallets tests backing up and restoring multiple wallets.
func TestBackupRestore_ExportImportMultipleWallets(t *testing.T) {
	type WalletBackup struct {
		PrivateKey []byte
		Label      string
	}

	// Create multiple wallets with labels
	originalWallets := []struct {
		wallet *Wallet
		label  string
	}{
		{nil, "Primary"},
		{nil, "Secondary"},
		{nil, "Backup"},
	}

	// Create and backup wallets
	backups := make([]WalletBackup, len(originalWallets))
	for i := range originalWallets {
		wallet, err := NewWallet()
		if err != nil {
			t.Fatalf("Failed to create wallet: %v", err)
		}
		originalWallets[i].wallet = wallet

		backups[i] = WalletBackup{
			PrivateKey: wallet.ExportPrivateKey(),
			Label:      originalWallets[i].label,
		}
	}

	// Verify backup data is unique for each wallet
	for i, b1 := range backups {
		for j, b2 := range backups {
			if i != j && bytes.Equal(b1.PrivateKey, b2.PrivateKey) {
				t.Errorf("Duplicate private key for wallets %d and %d", i, j)
			}
		}
	}

	// Restore wallets from backups
	restoredWallets := make([]*Wallet, len(backups))
	for i, backup := range backups {
		wallet, err := FromPrivateKeyBytes(backup.PrivateKey)
		if err != nil {
			t.Fatalf("Failed to restore wallet %s: %v", backup.Label, err)
		}
		restoredWallets[i] = wallet
	}

	// Verify all addresses are unique
	addresses := make(map[string]bool)
	for i, wallet := range restoredWallets {
		addr := wallet.GetAddressHex()
		if addresses[addr] {
			t.Errorf("Duplicate restored address for wallet %d: %s", i, addr)
		}
		addresses[addr] = true
	}

	// Verify restored addresses match originals
	for i := range originalWallets {
		originalAddr := originalWallets[i].wallet.GetAddressHex()
		restoredAddr := restoredWallets[i].GetAddressHex()
		if originalAddr != restoredAddr {
			t.Errorf("Restored address %s does not match original %s",
				restoredAddr, originalAddr)
		}
	}

	t.Logf("Multiple wallet backup/restore test passed - %d wallets", len(originalWallets))
}

// TestBackupRestore_SDKBackupRestore tests SDK backup and restore.
func TestBackupRestore_SDKBackupRestore(t *testing.T) {
	// Create original SDK with configuration
	original, err := NewWalletSDK(&SDKConfig{
		L2Threshold: 500,
		FeePerByte:  3,
	})
	if err != nil {
		t.Fatalf("Failed to create original SDK: %v", err)
	}

	// Add UTXOs
	original.AddUTXO([32]byte{0x50}, 0, 10000)
	original.AddUTXO([32]byte{0x51}, 0, 20000)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "sdk-backup-channel",
		PeerAddress: original.GetAddress(),
		Balance:     5000,
		FeeRate:     5,
		IsActive:    true,
	}
	original.AddL2Channel(channel)

	// Get original state
	originalAddr := original.GetAddressHex()
	originalBalance, _ := original.Balance()

	t.Logf("Original SDK - Address: %s, Balance: L1=%d, L2=%d",
		originalAddr, originalBalance.L1Balance, originalBalance.L2Balance)

	// Backup private key
	backupKey := original.ExportPrivateKey()

	// Restore SDK from backup
	restored, err := NewWalletSDK(&SDKConfig{
		PrivateKey: backupKey,
	})
	if err != nil {
		t.Fatalf("Failed to restore SDK: %v", err)
	}

	// Verify address matches
	if originalAddr != restored.GetAddressHex() {
		t.Errorf("Restored address %s does not match original %s",
			restored.GetAddressHex(), originalAddr)
	}

	// The restored SDK has its own UTXO store - we need to add funds separately
	// to verify the wallet works (same address, but separate UTXO store)
	restored.AddUTXO([32]byte{0x60}, 0, 15000)

	restoredBalance, _ := restored.Balance()

	t.Logf("Restored SDK - Balance: L1=%d, L2=%d",
		restoredBalance.L1Balance, restoredBalance.L2Balance)

	// Verify signing works on restored SDK
	testData := []byte("Test data for signature verification")
	sig := restored.Sign(testData)
	if !restored.Verify(testData, sig) {
		t.Error("Signature verification failed on restored SDK")
	}

	t.Logf("SDK backup/restore test passed")
}

// TestBackupRestore_InvalidKeyRestore tests restoring with invalid private key.
func TestBackupRestore_InvalidKeyRestore(t *testing.T) {
	// Try to restore with invalid (short) private key
	invalidKey := []byte{0x01, 0x02, 0x03} // Too short

	_, err := FromPrivateKeyBytes(invalidKey)
	if err == nil {
		t.Error("Should fail with invalid private key")
	}

	t.Logf("Invalid key restore correctly failed: %v", err)
}

// TestBackupRestore_EmptyKeyRestore tests restoring with empty private key.
func TestBackupRestore_EmptyKeyRestore(t *testing.T) {
	// Try to restore with empty private key
	_, err := FromPrivateKeyBytes([]byte{})
	if err == nil {
		t.Error("Should fail with empty private key")
	}

	t.Logf("Empty key restore correctly failed: %v", err)
}

// TestBackupRestore_KeyRotation tests key rotation (changing private key).
func TestBackupRestore_KeyRotation(t *testing.T) {
	// Create initial wallet
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	initialAddr := wallet.GetAddressHex()
	t.Logf("Initial address: %s", initialAddr)

	// Generate new key for rotation
	newWallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create new wallet: %v", err)
	}
	newKey := newWallet.ExportPrivateKey()

	// Import new key into existing wallet (rotate key)
	err = wallet.ImportPrivateKey(newKey)
	if err != nil {
		t.Fatalf("Failed to import new key: %v", err)
	}

	// Address should now match the new key
	newAddr := wallet.GetAddressHex()
	if newAddr == initialAddr {
		t.Error("Address should change after key rotation")
	}

	// Verify the new address matches the new wallet
	expectedAddr := newWallet.GetAddressHex()
	if newAddr != expectedAddr {
		t.Errorf("Rotated address %s does not match expected %s", newAddr, expectedAddr)
	}

	// Verify signing works with new key
	data := []byte("Test after rotation")
	sig := wallet.Sign(data)
	if !wallet.Verify(data, sig) {
		t.Error("Signature verification failed after rotation")
	}

	t.Logf("Key rotation test passed - Old: %s, New: %s", initialAddr, newAddr)
}

// TestBackupRestore_SerializedBackup tests serializing and deserializing backup data.
func TestBackupRestore_SerializedBackup(t *testing.T) {
	// Create wallet
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Create backup with additional metadata
	type WalletBackupData struct {
		PrivateKey []byte
		CreatedAt  uint64
		Label      string
		Version    string
	}

	backup := WalletBackupData{
		PrivateKey: wallet.ExportPrivateKey(),
		CreatedAt:  1234567890,
		Label:      "My Wallet",
		Version:    "1.0",
	}

	// In a real scenario, this would be serialized (JSON, etc.)
	// Here we just verify the data can be used
	if len(backup.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("Backup private key should be %d bytes", ed25519.PrivateKeySize)
	}

	// Restore from backup
	restored, err := FromPrivateKeyBytes(backup.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to restore from backup: %v", err)
	}

	// Verify metadata (label would need to be stored separately)
	if restored.GetAddressHex() != wallet.GetAddressHex() {
		t.Error("Restored wallet address does not match original")
	}

	t.Logf("Serialized backup test passed - Label: %s, Version: %s", backup.Label, backup.Version)
}

// TestBackupRestore_VerifySignatureAfterRestore tests that signatures can be verified after restore.
func TestBackupRestore_VerifySignatureAfterRestore(t *testing.T) {
	// Create original wallet and sign some data
	original, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Sign multiple pieces of data
	testMessages := [][]byte{
		[]byte("First message"),
		[]byte("Second message"),
		[]byte("Third message - longer message for testing"),
	}

	signatures := make(map[string][]byte)
	for _, msg := range testMessages {
		sig := original.Sign(msg)
		signatures[string(msg)] = sig
	}

	// Backup
	backup := original.ExportPrivateKey()

	// Restore
	restored, err := FromPrivateKeyBytes(backup)
	if err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	// Verify all signatures with restored wallet
	for _, msg := range testMessages {
		sig := signatures[string(msg)]
		if !restored.Verify(msg, sig) {
			t.Errorf("Failed to verify signature for message: %s", msg)
		}
	}

	// Verify wrong data fails
	for _, msg := range testMessages {
		sig := signatures[string(msg)]
		wrongMsg := []byte(string(msg) + "modified")
		if restored.Verify(wrongMsg, sig) {
			t.Errorf("Verification should fail for modified message: %s", wrongMsg)
		}
	}

	t.Logf("Signature verification after restore test passed - %d messages verified", len(testMessages))
}
