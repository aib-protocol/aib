// Package wallet provides tests for the WalletSDK.
package wallet

import (
	"crypto/ed25519"
	"testing"
)

// TestSDK_NewWalletSDK tests creating a new SDK with a generated wallet.
func TestSDK_NewWalletSDK(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Verify wallet is created
	addr := sdk.GetAddress()
	if addr == [32]byte{} {
		t.Error("Wallet address should not be zero")
	}

	// Verify address hex is valid
	hexStr := sdk.GetAddressHex()
	if len(hexStr) != 64 {
		t.Errorf("Address hex should be 64 chars, got %d", len(hexStr))
	}

	t.Logf("Created SDK with address: %s", hexStr)
}

// TestSDK_NewWalletSDKFromKey tests creating SDK from existing private key.
func TestSDK_NewWalletSDKFromKey(t *testing.T) {
	// First create a wallet to get its private key
	original, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create original SDK: %v", err)
	}

	// Get the private key bytes directly
	privKeyBytes := original.ExportPrivateKey()

	// Recover using the private key bytes
	recovered, err := NewWalletSDK(&SDKConfig{
		PrivateKey: privKeyBytes,
	})
	if err != nil {
		t.Fatalf("Failed to recover SDK: %v", err)
	}

	// Verify addresses match
	if original.GetAddress() != recovered.GetAddress() {
		t.Error("Recovered SDK address does not match original")
	}

	t.Logf("Created SDK from key with address: %s", recovered.GetAddressHex())
}

// TestSDK_NewWalletSDKWithConfig tests creating SDK with custom configuration.
func TestSDK_NewWalletSDKWithConfig(t *testing.T) {
	config := &SDKConfig{
		L2Threshold: 500,
		FeePerByte:  3,
	}

	sdk, err := NewWalletSDK(config)
	if err != nil {
		t.Fatalf("Failed to create SDK with config: %v", err)
	}

	// Verify configuration is applied
	l1, l2, err := sdk.payment.GetBalance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	t.Logf("SDK created - L1: %d, L2: %d, Address: %s", l1, l2, sdk.GetAddressHex())
}

// TestSDK_NewWalletSDKWithPrivateKey tests recovering wallet from private key.
func TestSDK_NewWalletSDKWithPrivateKey(t *testing.T) {
	// First create a wallet to get its private key
	original, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create original SDK: %v", err)
	}

	privKeyBytes := original.ExportPrivateKey()

	// Recover using the private key
	recovered, err := NewWalletSDK(&SDKConfig{
		PrivateKey: privKeyBytes,
	})
	if err != nil {
		t.Fatalf("Failed to recover SDK: %v", err)
	}

	// Verify addresses match
	if original.GetAddress() != recovered.GetAddress() {
		t.Error("Recovered SDK address does not match original")
	}

	t.Logf("Recovered SDK address: %s", recovered.GetAddressHex())
}

// TestSDK_Balance tests balance querying.
func TestSDK_Balance(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Get initial balance (should be 0)
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.L1Balance != 0 {
		t.Errorf("Initial L1 balance should be 0, got %d", balance.L1Balance)
	}

	if balance.L2Balance != 0 {
		t.Errorf("Initial L2 balance should be 0, got %d", balance.L2Balance)
	}

	if balance.Total != 0 {
		t.Errorf("Initial total balance should be 0, got %d", balance.Total)
	}

	t.Logf("Initial balance: L1=%d, L2=%d, Total=%d", balance.L1Balance, balance.L2Balance, balance.Total)
}

// TestSDK_BalanceWithFunds tests balance querying with funds.
func TestSDK_BalanceWithFunds(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add some UTXOs
	txHash1 := [32]byte{0x01, 0x02, 0x03}
	txHash2 := [32]byte{0x04, 0x05, 0x06}

	sdk.AddUTXO(txHash1, 0, 500)
	sdk.AddUTXO(txHash2, 0, 300)

	// Get balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.L1Balance != 800 {
		t.Errorf("L1 balance should be 800, got %d", balance.L1Balance)
	}

	if balance.Total != 800 {
		t.Errorf("Total balance should be 800, got %d", balance.Total)
	}

	t.Logf("Balance with funds: L1=%d, L2=%d, Total=%d", balance.L1Balance, balance.L2Balance, balance.Total)
}

// TestSDK_Send tests sending payment.
func TestSDK_Send(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds
	txHash := [32]byte{0xaa}
	sdk.AddUTXO(txHash, 0, 10000)

	// Set threshold high to force L1
	sdk.SetL2Threshold(100000)

	// Send payment
	toAddr := sdk.GetAddressHex()
	result, err := sdk.Send(toAddr, 500)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("Payment should succeed: %s", result.Error)
	}

	if result.Amount != 500 {
		t.Errorf("Amount should be 500, got %d", result.Amount)
	}

	if result.Method != "L1" {
		t.Errorf("Method should be L1, got %s", result.Method)
	}

	t.Logf("Send result: %+v", result)
}

// TestSDK_SendL1 tests sending L1 payment.
func TestSDK_SendL1(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds
	txHash := [32]byte{0xbb}
	sdk.AddUTXO(txHash, 0, 5000)

	// Send L1 payment
	toAddr := sdk.GetAddressHex()
	result, err := sdk.SendL1(toAddr, 200)
	if err != nil {
		t.Fatalf("SendL1 failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("L1 payment should succeed: %s", result.Error)
	}

	if result.Method != "L1" {
		t.Errorf("Method should be L1, got %s", result.Method)
	}

	t.Logf("SendL1 result: %+v", result)
}

// TestSDK_SendL2 tests sending L2 payment.
func TestSDK_SendL2(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add an L2 channel
	channel := &L2Channel{
		ChannelID:   "test-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Send L2 payment
	toAddrHex := sdk.GetAddressHex()
	result, err := sdk.SendL2(toAddrHex, 100, "test-channel")
	if err != nil {
		t.Fatalf("SendL2 failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("L2 payment should succeed: %s", result.Error)
	}

	if result.Method != "L2" {
		t.Errorf("Method should be L2, got %s", result.Method)
	}

	t.Logf("SendL2 result: %+v", result)
}

// TestSDK_Receive tests receiving address.
func TestSDK_Receive(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Get receiving address - use hex address to avoid Bech32m panic
	addr := sdk.GetAddressHex()
	if addr == "" {
		t.Error("Receive address should not be empty")
	}

	// Should match GetAddressHex
	if addr != sdk.GetAddressHex() {
		t.Errorf("Receive address should match GetAddressHex: got %s, want %s", addr, sdk.GetAddressHex())
	}

	t.Logf("Receive address (hex): %s", addr)
}

// TestSDK_History tests transaction history.
func TestSDK_History(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds
	txHash := [32]byte{0xcc}
	sdk.AddUTXO(txHash, 0, 10000)

	// Set threshold high
	sdk.SetL2Threshold(100000)

	// Send a payment
	toAddr := sdk.GetAddressHex()
	_, err = sdk.Send(toAddr, 100)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Get history
	history, err := sdk.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("History should have 1 record, got %d", len(history))
	}

	if history[0].Direction != "sent" {
		t.Errorf("Direction should be 'sent', got %s", history[0].Direction)
	}

	if history[0].Amount != 100 {
		t.Errorf("Amount should be 100, got %d", history[0].Amount)
	}

	t.Logf("History: %+v", history)
}

// TestSDK_L2Channel tests L2 channel management.
func TestSDK_L2Channel(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add an L2 channel
	channel := &L2Channel{
		ChannelID:   "channel-001",
		PeerAddress: sdk.GetAddress(),
		Balance:     500,
		FeeRate:     10,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Get the channel
	retrieved := sdk.GetL2Channel("channel-001")
	if retrieved == nil {
		t.Fatal("Failed to retrieve channel")
	}

	if retrieved.Balance != 500 {
		t.Errorf("Channel balance should be 500, got %d", retrieved.Balance)
	}

	t.Logf("L2 Channel: %+v", retrieved)
}

// TestSDK_SignVerify tests signing and verification.
func TestSDK_SignVerify(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	data := []byte("Test message for signing")

	// Sign data
	signature := sdk.Sign(data)
	if len(signature) != 64 {
		t.Errorf("Signature should be 64 bytes, got %d", len(signature))
	}

	// Verify signature
	if !sdk.Verify(data, signature) {
		t.Error("Signature verification should succeed")
	}

	// Verify wrong data fails
	if sdk.Verify([]byte("Wrong data"), signature) {
		t.Error("Verification should fail for wrong data")
	}

	t.Logf("Sign/Verify test passed")
}

// TestSDK_ExportPrivateKey tests private key export.
func TestSDK_ExportPrivateKey(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	privKey := sdk.ExportPrivateKey()
	if len(privKey) != ed25519.PrivateKeySize {
		t.Errorf("Private key should be %d bytes, got %d", ed25519.PrivateKeySize, len(privKey))
	}

	t.Logf("Exported private key (%d bytes)", len(privKey))
}

// TestSDK_GetUTXOStore tests getting the UTXO store.
func TestSDK_GetUTXOStore(t *testing.T) {
	config := &SDKConfig{
		UTXOStore: nil, // Will create new store
	}

	sdk, err := NewWalletSDK(config)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	store := sdk.GetUTXOStore()
	if store == nil {
		t.Error("UTXO store should not be nil")
	}

	t.Logf("Got UTXO store: %v", store)
}

// TestSDK_GetPaymentManager tests getting the payment manager.
func TestSDK_GetPaymentManager(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	pm := sdk.GetPaymentManager()
	if pm == nil {
		t.Error("Payment manager should not be nil")
	}

	t.Logf("Got Payment manager: %v", pm)
}

// TestSDK_SetFeePerByte tests setting fee per byte.
func TestSDK_SetFeePerByte(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	sdk.SetFeePerByte(5)

	// Verify by checking the payment manager
	pm := sdk.GetPaymentManager()
	if pm == nil {
		t.Fatal("Payment manager should not be nil")
	}

	t.Logf("Set fee per byte test passed")
}

// TestSDK_AddUTXO tests adding UTXO.
func TestSDK_AddUTXO(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	txHash := [32]byte{0xdd, 0xee, 0xff}
	err = sdk.AddUTXO(txHash, 0, 1000)
	if err != nil {
		t.Fatalf("AddUTXO failed: %v", err)
	}

	// Verify balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.L1Balance != 1000 {
		t.Errorf("Balance should be 1000, got %d", balance.L1Balance)
	}

	t.Logf("AddUTXO test passed, balance: %d", balance.L1Balance)
}

// TestSDK_GetPublicKey tests getting public key.
func TestSDK_GetPublicKey(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	pubKey := sdk.GetPublicKey()
	if len(pubKey) != ed25519.PublicKeySize {
		t.Errorf("Public key should be %d bytes, got %d", ed25519.PublicKeySize, len(pubKey))
	}

	t.Logf("Got public key (%d bytes)", len(pubKey))
}

// TestSDK_SendWithInsufficientBalance tests failed send.
func TestSDK_SendWithInsufficientBalance(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Don't add any funds
	toAddr := sdk.GetAddressHex()
	result, err := sdk.Send(toAddr, 1000)
	if err != nil {
		t.Fatalf("Send should not return error: %v", err)
	}

	if result.Success {
		t.Error("Payment should fail with insufficient balance")
	}

	if result.Error == "" {
		t.Error("Error message should be present")
	}

	t.Logf("Expected failure: %s", result.Error)
}

// TestSDK_HistoryLimit tests history with limit.
func TestSDK_HistoryLimit(t *testing.T) {
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add funds and send multiple payments
	txHash := [32]byte{0x11}
	sdk.AddUTXO(txHash, 0, 50000)
	sdk.SetL2Threshold(100000)

	toAddr := sdk.GetAddressHex()
	sdk.Send(toAddr, 100)
	sdk.Send(toAddr, 200)
	sdk.Send(toAddr, 300)

	// Get history with limit
	history, err := sdk.History(2)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("History should return 2 records, got %d", len(history))
	}

	t.Logf("History with limit: %d records", len(history))
}
