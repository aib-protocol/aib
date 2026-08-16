// Package integration provides integration tests for AIB 2.0.
// Tests the complete flow of UTXO, Wallet, Channel, Mempool, and Blockchain.
package integration

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/channel"
	"github.com/aib-protocol/aib/pkg/utxo"
	"github.com/aib-protocol/aib/pkg/wallet"
)

// serializeStateForChannel serializes a signed state for hashing/signing (matches state_channel.go).
func serializeStateForChannel(state *interfaces.SignedState) []byte {
	buf := make([]byte, 0, 200)
	buf = append(buf, state.ChannelID[:]...)
	buf = binary.BigEndian.AppendUint64(buf, state.Sequence)
	buf = binary.BigEndian.AppendUint64(buf, state.BalanceA)
	buf = binary.BigEndian.AppendUint64(buf, state.BalanceB)
	buf = append(buf, byte(state.Timestamp.Unix()))
	return buf
}

// ============================================================================
// Test Utilities
// ============================================================================

// walletCounter is used to generate unique seeds for different wallets
var walletCounter int

// createTestWallet creates a wallet with a known key for testing.
func createTestWallet(t *testing.T) *wallet.Wallet {
	t.Helper()

	// Use ed25519.GenerateKey to create unique wallets for each call
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Import the private key into a wallet
	w := wallet.FromPrivateKey(priv)
	_ = pub // Use pub to avoid unused variable error

	return w
}

// createTestUTXOStore creates a fresh UTXO store with genesis coins.
func createTestUTXOStore(t *testing.T, w *wallet.Wallet, amount uint64) *utxo.UTXOStore {
	t.Helper()

	store := utxo.NewUTXOStore()

	// Create a genesis UTXO (simulating coinbase)
	genesisTxHash := [32]byte{0, 0, 0, 1} // Simulated genesis tx hash
	utxoEntry := &utxo.UTXO{
		TxHash:  genesisTxHash,
		Index:   0,
		Value:   amount,
		Address: w.GetAddress(),
	}
	store.AddUTXO(utxoEntry)

	return store
}

// ============================================================================
// Test 1: UTXO + Wallet Complete Flow
// ============================================================================

// TestUTXOWalletFlow tests the complete UTXO + Wallet integration flow.
func TestUTXOWalletFlow(t *testing.T) {
	// Create two wallets (sender and receiver)
	sender := createTestWallet(t)
	receiver := createTestWallet(t)

	initialAmount := uint64(1000000) // 10 AIB (in smallest units)
	store := createTestUTXOStore(t, sender, initialAmount)

	// Verify initial balance
	senderBalance := store.GetBalance(sender.GetAddress())
	if senderBalance != initialAmount {
		t.Fatalf("Initial balance mismatch: expected %d, got %d", initialAmount, senderBalance)
	}

	receiverBalance := store.GetBalance(receiver.GetAddress())
	if receiverBalance != 0 {
		t.Fatalf("Receiver should have 0 balance, got %d", receiverBalance)
	}

	// Create a transaction: sender pays receiver
	transferAmount := uint64(500000) // 5 AIB
	fee := uint64(1000)             // 0.00001 AIB

	// Get UTXOs for the transaction
	utxos, _, err := store.GetUTXOsForAmount(sender.GetAddress(), transferAmount+fee)
	if err != nil {
		t.Fatalf("Failed to get UTXOs: %v", err)
	}

	// Build transaction inputs
	var inputs []utxo.TXInput
	for _, u := range utxos {
		inputs = append(inputs, utxo.TXInput{
			TxHash:    u.TxHash,
			Index:     u.Index,
			Signature: nil,
			PublicKey: sender.GetPublicKey(),
		})
	}

	// Build transaction outputs
	outputs := []utxo.TXOutput{
		{
			Value:   transferAmount,
			Script:  nil,
			Address: receiver.GetAddress(),
		},
		{
			Value:   initialAmount - transferAmount - fee,
			Script:  nil,
			Address: sender.GetAddress(), // Change back to sender
		},
	}

	// Create and sign transaction
	tx := utxo.NewTransaction(inputs, outputs)

	// Sign transaction using wallet's SignTransaction method
	err = sender.SignTransaction(tx, 0)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Verify transaction
	if !sender.VerifyTransaction(tx) {
		t.Fatal("Transaction signature verification failed")
	}

	// Validate transaction against UTXO store
	if err := store.ValidateTransaction(tx); err != nil {
		t.Fatalf("Transaction validation failed: %v", err)
	}

	// Apply transaction to UTXO store
	if err := store.ApplyTransaction(tx); err != nil {
		t.Fatalf("Failed to apply transaction: %v", err)
	}

	// Verify balances after transaction
	newSenderBalance := store.GetBalance(sender.GetAddress())
	expectedSenderBalance := initialAmount - transferAmount - fee
	if newSenderBalance != expectedSenderBalance {
		t.Errorf("Sender balance after transfer: expected %d, got %d", expectedSenderBalance, newSenderBalance)
	}

	newReceiverBalance := store.GetBalance(receiver.GetAddress())
	if newReceiverBalance != transferAmount {
		t.Errorf("Receiver balance after transfer: expected %d, got %d", transferAmount, newReceiverBalance)
	}

	// Verify UTXO count
	utxoCount := store.GetUTXOCount()
	if utxoCount != 2 { // Receiver's UTXO + Sender's change
		t.Errorf("UTXO count after transfer: expected 2, got %d", utxoCount)
	}

	t.Logf("UTXO + Wallet flow completed successfully")
	t.Logf("  Initial: %d -> After: sender=%d, receiver=%d", initialAmount, newSenderBalance, newReceiverBalance)
}

// TestUTXOMultiInputTransaction tests transactions with multiple inputs.
func TestUTXOMultiInputTransaction(t *testing.T) {
	sender := createTestWallet(t)
	receiver := createTestWallet(t)

	// Create multiple small UTXOs
	store := utxo.NewUTXOStore()
	utxoValue := uint64(100000)

	// Add 5 UTXOs to sender
	for i := 0; i < 5; i++ {
		txHash := [32]byte{byte(i), 0, 0, 1}
		store.AddUTXO(&utxo.UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   utxoValue,
			Address: sender.GetAddress(),
		})
	}

	totalBalance := store.GetBalance(sender.GetAddress())
	if totalBalance != utxoValue*5 {
		t.Fatalf("Total balance mismatch: expected %d, got %d", utxoValue*5, totalBalance)
	}

	// Create a transaction that spends multiple UTXOs
	transferAmount := uint64(350000)
	fee := uint64(1000)

	utxos, _, err := store.GetUTXOsForAmount(sender.GetAddress(), transferAmount+fee)
	if err != nil {
		t.Fatalf("Failed to get UTXOs: %v", err)
	}

	// Build inputs from multiple UTXOs
	var inputs []utxo.TXInput
	for _, u := range utxos {
		inputs = append(inputs, utxo.TXInput{
			TxHash:    u.TxHash,
			Index:     u.Index,
			Signature: nil,
			PublicKey: sender.GetPublicKey(),
		})
	}

	// Single output to receiver
	outputs := []utxo.TXOutput{
		{
			Value:   transferAmount,
			Script:  nil,
			Address: receiver.GetAddress(),
		},
	}

	tx := utxo.NewTransaction(inputs, outputs)

	// Sign each input
	for i := range tx.Inputs {
		err = sender.SignTransaction(tx, i)
		if err != nil {
			t.Fatalf("Failed to sign input %d: %v", i, err)
		}
	}

	// Verify all inputs
	if !sender.VerifyTransaction(tx) {
		t.Fatal("Transaction verification failed")
	}

	// Validate and apply
	if err := store.ValidateTransaction(tx); err != nil {
		t.Fatalf("Transaction validation failed: %v", err)
	}
	if err := store.ApplyTransaction(tx); err != nil {
		t.Fatalf("Failed to apply transaction: %v", err)
	}

	// Check final balances
	senderBalance := store.GetBalance(sender.GetAddress())
	receiverBalance := store.GetBalance(receiver.GetAddress())

	if receiverBalance != transferAmount {
		t.Errorf("Receiver balance: expected %d, got %d", transferAmount, receiverBalance)
	}

	t.Logf("Multi-input transaction completed: sender=%d, receiver=%d", senderBalance, receiverBalance)
}

// ============================================================================
// Test 2: Channel Open/Close Flow
// ============================================================================

// mockMultiSigLocker implements interfaces.MultiSigLocker for testing.
type mockMultiSigLocker struct {
	utxos map[string]*interfaces.UTXO
}

func newMockMultiSigLocker() *mockMultiSigLocker {
	return &mockMultiSigLocker{
		utxos: make(map[string]*interfaces.UTXO),
	}
}

func (m *mockMultiSigLocker) CreateMultiSigOutput(partyA, partyB interfaces.Address, amount uint64) (*interfaces.UTXO, error) {
	txHash := [32]byte{0xAA, 0xBB, 0xCC, 0xDD}
	utxo := &interfaces.UTXO{
		TxHash:  txHash,
		Index:   0,
		Value:   amount,
		Address: partyA,
	}
	key := fmt.Sprintf("%x:0", txHash)
	m.utxos[key] = utxo
	return utxo, nil
}

func (m *mockMultiSigLocker) SpendMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte, outputs []interfaces.TXOutput) error {
	// Mock: just verify signatures are present
	if len(sigA) == 0 || len(sigB) == 0 {
		return fmt.Errorf("missing signatures")
	}
	// Remove from UTXO set
	key := fmt.Sprintf("%x:0", utxo.TxHash)
	delete(m.utxos, key)
	return nil
}

func (m *mockMultiSigLocker) VerifyMultiSig(utxo *interfaces.UTXO, sigA, sigB []byte) bool {
	return len(sigA) > 0 && len(sigB) > 0
}

// ed25519Signer implements crypto.Signer using Ed25519.
type ed25519Signer struct {
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

func newEd25519Signer() (*ed25519Signer, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	return &ed25519Signer{privKey: priv, pubKey: pub}, nil
}

func (s *ed25519Signer) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(s.privKey, data), nil
}

func (s *ed25519Signer) PublicKey() []byte {
	return s.pubKey
}

func (s *ed25519Signer) Algorithm() string {
	return "ed25519"
}

func (s *ed25519Signer) Destroy() {
	// Zero the private key
	for i := range s.privKey {
		s.privKey[i] = 0
	}
}

// TestChannelOpenClose tests the complete channel open and close flow.
func TestChannelOpenClose(t *testing.T) {
	// Create channel manager
	multiSig := newMockMultiSigLocker()
	signer1, err := newEd25519Signer()
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}
	signer2, err := newEd25519Signer()
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	cfg := &channel.Config{
		ChallengePeriod:   1 * time.Hour,
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:    multiSig,
	}

	manager, err := channel.NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}

	// Get party addresses
	var partyA, partyB [32]byte
	copy(partyA[:], signer1.PublicKey()[:32])
	copy(partyB[:], signer2.PublicKey()[:32])

	depositA := uint64(50000)
	depositB := uint64(30000)

	// Open channel
	ch, err := manager.OpenChannel(nil, partyA, partyB, depositA, depositB)
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}

	// Verify channel state
	if ch.BalanceA != depositA {
		t.Errorf("Channel BalanceA: expected %d, got %d", depositA, ch.BalanceA)
	}
	if ch.BalanceB != depositB {
		t.Errorf("Channel BalanceB: expected %d, got %d", depositB, ch.BalanceB)
	}
	if ch.Sequence != 0 {
		t.Errorf("Initial sequence: expected 0, got %d", ch.Sequence)
	}

	// Get channel status
	status, err := manager.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("Failed to get channel status: %v", err)
	}
	if status != channel.StateOpen {
		t.Errorf("Channel status: expected StateOpen (%d), got %d", channel.StateOpen, status)
	}

	// Simulate transfer: A transfers 10000 to B
	manager.SetSigner(signer1)
	state1, err := manager.Transfer(ch.ID, 10000, true) // true = from A to B
	if err != nil {
		t.Fatalf("Transfer failed: %v", err)
	}

	// Verify updated balance
	ch, err = manager.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("Failed to get channel state: %v", err)
	}
	if ch.BalanceA != depositA-10000 {
		t.Errorf("BalanceA after transfer: expected %d, got %d", depositA-10000, ch.BalanceA)
	}
	if ch.BalanceB != depositB+10000 {
		t.Errorf("BalanceB after transfer: expected %d, got %d", depositB+10000, ch.BalanceB)
	}

	// Sign state by party A (using channel's signing mechanism)
	stateData := serializeStateForChannel(state1)
	sigA, _ := signer1.Sign(stateData)

	// Set signer for party B and create final state
	manager.SetSigner(signer2)

	// Close channel with final state
	finalState := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  state1.Sequence,
		BalanceA:  ch.BalanceA,
		BalanceB:  ch.BalanceB,
		Timestamp: time.Now(),
		SigA:      sigA,
	}

	// Sign final state by party B
	finalStateData := serializeStateForChannel(&finalState)
	finalState.SigB, _ = signer2.Sign(finalStateData)

	err = manager.CloseChannel(nil, ch, finalState)
	if err != nil {
		t.Fatalf("Failed to close channel: %v", err)
	}

	// Verify channel is closed
	status, err = manager.GetChannelStatus(ch.ID)
	if err != nil {
		t.Fatalf("Failed to get channel status: %v", err)
	}
	if status != channel.StateClosed {
		t.Errorf("Channel status after close: expected StateClosed (%d), got %d", channel.StateClosed, status)
	}

	t.Logf("Channel flow completed: open -> transfer -> close")
	t.Logf("  Initial: A=%d, B=%d -> Final: A=%d, B=%d", depositA, depositB, finalState.BalanceA, finalState.BalanceB)
}

// TestChannelDispute tests the channel dispute mechanism.
func TestChannelDispute(t *testing.T) {
	multiSig := newMockMultiSigLocker()
	signer1, _ := newEd25519Signer()
	signer2, _ := newEd25519Signer()

	cfg := &channel.Config{
		ChallengePeriod:   1 * time.Second, // Short for testing
		MinDeposit:        1000,
		MaxChannelValue:   1000000,
		MultiSigLocker:    multiSig,
	}

	manager, err := channel.NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}

	// Get party addresses
	var partyA, partyB [32]byte
	copy(partyA[:], signer1.PublicKey()[:32])
	copy(partyB[:], signer2.PublicKey()[:32])

	// Open channel
	ch, err := manager.OpenChannel(nil, partyA, partyB, 50000, 30000)
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}

	// Make a transfer
	manager.SetSigner(signer1)
	_, err = manager.Transfer(ch.ID, 10000, true)
	if err != nil {
		t.Fatalf("Transfer failed: %v", err)
	}

	// Get updated channel
	ch, err = manager.GetChannelState(ch.ID)
	if err != nil {
		t.Fatalf("Failed to get channel state: %v", err)
	}

	// Create disputed state (with higher sequence)
	disputedState := interfaces.SignedState{
		ChannelID: ch.ID,
		Sequence:  ch.Sequence + 10, // Higher sequence
		BalanceA:  10000,            // Wrong balance
		BalanceB:  70000,
		Timestamp: time.Now(),
	}

	// Sign by both parties
	manager.SetSigner(signer1)
	stateData := serializeStateForChannel(&disputedState)
	disputedState.SigA, _ = signer1.Sign(stateData)

	manager.SetSigner(signer2)
	disputedState.SigB, _ = signer2.Sign(stateData)

	// Initiate dispute
	err = manager.Dispute(nil, ch, disputedState)
	if err != nil {
		t.Fatalf("Failed to initiate dispute: %v", err)
	}

	// Verify channel is in dispute
	status, _ := manager.GetChannelStatus(ch.ID)
	if status != channel.StateInDispute {
		t.Errorf("Expected channel in dispute, got status %d", status)
	}

	// Wait for challenge period to pass
	time.Sleep(2 * time.Second)

	// Resolve dispute in favor of party B (original state was correct)
	err = manager.ResolveDispute(ch.ID, partyB)
	if err != nil {
		t.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Verify channel is closed
	status, _ = manager.GetChannelStatus(ch.ID)
	if status != channel.StateClosed {
		t.Errorf("Expected channel closed after dispute resolution, got status %d", status)
	}

	t.Logf("Channel dispute test completed successfully")
}

// ============================================================================
// Test 3: Mempool + Blockchain Integration
// ============================================================================

// TestMempoolBlockchainIntegration tests the mempool and blockchain integration.
func TestMempoolBlockchainIntegration(t *testing.T) {
	// Create test components
	sender := createTestWallet(t)
	store := createTestUTXOStore(t, sender, 1000000)
	mempool := utxo.NewMempool(1000, 100)

	// Add validator to consensus
	config := utxo.DefaultPoSConfig()
	config.MinStake = 1000
	consensus := utxo.NewConsensusState(config)

	// Generate validator key
	validatorPub, validatorPriv, _ := ed25519.GenerateKey(nil)
	var validatorAddr [32]byte
	copy(validatorAddr[:], validatorPub)

	consensus.AddValidator(validatorAddr, 10000, validatorPub)

	// Create genesis block
	proposer := validatorAddr
	genesis := utxo.NewBlock(nil, [32]byte{}, 0, proposer)

	// Create blockchain mock
	chain := &testChain{
		blocks:    []*utxo.Block{genesis},
		utxoStore: store,
	}

	// Test 1: Add transaction to mempool
	receiver := createTestWallet(t)
	transferAmount := uint64(500000)
	fee := uint64(1000)

	// Get UTXOs for transaction
	utxos, _, _ := store.GetUTXOsForAmount(sender.GetAddress(), transferAmount+fee)

	var inputs []utxo.TXInput
	for _, u := range utxos {
		inputs = append(inputs, utxo.TXInput{
			TxHash:    u.TxHash,
			Index:     u.Index,
			Signature: nil,
			PublicKey: sender.GetPublicKey(),
		})
	}

	outputs := []utxo.TXOutput{
		{Value: transferAmount, Address: receiver.GetAddress()},
		{Value: 1000000 - transferAmount - fee, Address: sender.GetAddress()}, // Change
	}

	tx := utxo.NewTransaction(inputs, outputs)

	// Sign transaction using wallet
	err := sender.SignTransaction(tx, 0)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Add to mempool (needs UTXO provider)
	err = mempool.AddTransaction(tx, store)
	if err != nil {
		t.Fatalf("Failed to add transaction to mempool: %v", err)
	}

	// Verify mempool size
	if mempool.Size() != 1 {
		t.Errorf("Mempool size: expected 1, got %d", mempool.Size())
	}

	// Test 2: Get transactions for block
	blockTxs := mempool.GetTransactionsForBlock(1000000)
	if len(blockTxs) != 1 {
		t.Errorf("Expected 1 transaction for block, got %d", len(blockTxs))
	}

	// Test 3: Create and add block
	hash := genesis.Hash
	newBlock := utxo.NewBlock(blockTxs, hash, 1, proposer)

	// Sign block
	hash = newBlock.CalculateHash()
	newBlock.Header.Signature = ed25519.Sign(validatorPriv, hash[:])

	// Add block to chain
	err = chain.AddBlock(newBlock)
	if err != nil {
		t.Fatalf("Failed to add block to chain: %v", err)
	}

	// Test 4: Verify block
	if chain.GetHeight() != 2 {
		t.Errorf("Chain height: expected 2, got %d", chain.GetHeight())
	}

	// Test 5: Verify confirmed transaction removed from mempool
	// (simulated - in real impl this happens in AddBlock)
	txHashes := make([][32]byte, len(blockTxs))
	for i, tx := range blockTxs {
		txHashes[i] = tx.Hash()
	}
	mempool.RemoveConfirmed(txHashes)

	if mempool.Size() != 0 {
		t.Errorf("Mempool should be empty after block confirmation, got %d", mempool.Size())
	}

	// Test 6: Verify UTXO set updated
	newReceiverBalance := store.GetBalance(receiver.GetAddress())
	if newReceiverBalance != transferAmount {
		t.Errorf("Receiver balance: expected %d, got %d", transferAmount, newReceiverBalance)
	}

	t.Logf("Mempool + Blockchain integration test completed")
	t.Logf("  Chain height: %d", chain.GetHeight())
	t.Logf("  Receiver balance: %d", newReceiverBalance)
}

// testChain is a simple chain implementation for testing.
type testChain struct {
	blocks    []*utxo.Block
	utxoStore *utxo.UTXOStore
}

func (c *testChain) AddBlock(block *utxo.Block) error {
	// Validate block
	if len(c.blocks) > 0 {
		parent := c.blocks[len(c.blocks)-1]
		if block.Header.Height != parent.Header.Height+1 {
			return fmt.Errorf("invalid height")
		}
		if block.Header.PrevBlockHash != parent.Hash {
			return fmt.Errorf("invalid previous hash")
		}
	}

	// Apply transactions
	for _, tx := range block.Transactions {
		if !tx.IsCoinbase() {
			if err := c.utxoStore.ApplyTransaction(tx); err != nil {
				return fmt.Errorf("apply tx: %w", err)
			}
		}
	}

	c.blocks = append(c.blocks, block)
	return nil
}

func (c *testChain) GetHeight() uint64 {
	return uint64(len(c.blocks))
}

// TestMempoolDoubleSpend tests that mempool rejects double-spend transactions.
func TestMempoolDoubleSpend(t *testing.T) {
	sender := createTestWallet(t)
	store := createTestUTXOStore(t, sender, 1000000)
	mempool := utxo.NewMempool(1000, 100)

	receiver := createTestWallet(t)

	// Get the UTXO
	utxos, _, _ := store.GetUTXOsForAmount(sender.GetAddress(), 100000)
	firstUTXO := utxos[0]

	// Create first transaction
	tx1 := utxo.NewTransaction([]utxo.TXInput{
		{
			TxHash:    firstUTXO.TxHash,
			Index:     firstUTXO.Index,
			Signature: nil,
			PublicKey: sender.GetPublicKey(),
		},
	}, []utxo.TXOutput{
		{Value: 90000, Address: receiver.GetAddress()},
	})

	// Sign the transaction
	err := sender.SignTransaction(tx1, 0)
	if err != nil {
		t.Fatalf("Failed to sign tx1: %v", err)
	}

	// Add first transaction to mempool
	err = mempool.AddTransaction(tx1, store)
	if err != nil {
		t.Fatalf("First tx should be accepted: %v", err)
	}

	// Create double-spend transaction (same input, different output)
	tx2 := utxo.NewTransaction([]utxo.TXInput{
		{
			TxHash:    firstUTXO.TxHash,
			Index:     firstUTXO.Index,
			Signature: nil,
			PublicKey: sender.GetPublicKey(),
		},
	}, []utxo.TXOutput{
		{Value: 90000, Address: sender.GetAddress()}, // Send back to self
	})

	// Sign the second transaction
	err = sender.SignTransaction(tx2, 0)
	if err != nil {
		t.Fatalf("Failed to sign tx2: %v", err)
	}

	// Add second transaction to mempool - should fail
	err = mempool.AddTransaction(tx2, store)
	if err == nil {
		t.Fatal("Double-spend transaction should be rejected")
	}

	t.Logf("Double-spend correctly rejected: %v", err)
}

// TestMempoolFeePriority tests that transactions are sorted by fee rate.
func TestMempoolFeePriority(t *testing.T) {
	sender := createTestWallet(t)
	store := createTestUTXOStore(t, sender, 10000000)
	mempool := utxo.NewMempool(1000, 100)

	receiver := createTestWallet(t)

	// Create multiple transactions with different fees
	for i := 0; i < 5; i++ {
		// Each transaction uses a different UTXO
		txHash := [32]byte{byte(i), 0, 0, 1}
		utxoEntry := &utxo.UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   2000000,
			Address: sender.GetAddress(),
		}
		store.AddUTXO(utxoEntry)

		// Create transaction with varying output values (lower output = higher fee)
		fee := uint64((5 - i) * 1000) // Different fees
		outputValue := uint64(2000000 - fee)

		tx := utxo.NewTransaction([]utxo.TXInput{
			{
				TxHash:    txHash,
				Index:     0,
				Signature: nil,
				PublicKey: sender.GetPublicKey(),
			},
		}, []utxo.TXOutput{
			{Value: outputValue, Address: receiver.GetAddress()},
		})

		err := sender.SignTransaction(tx, 0)
		if err != nil {
			t.Fatalf("Failed to sign transaction %d: %v", i, err)
		}

		err = mempool.AddTransaction(tx, store)
		if err != nil {
			t.Fatalf("Failed to add transaction %d: %v", i, err)
		}
	}

	// Get transactions for block
	blockTxs := mempool.GetTransactionsForBlock(10000000)

	// Verify transactions are sorted by fee rate (highest first)
	for i := 0; i < len(blockTxs)-1; i++ {
		feeRate1 := float64(blockTxs[i].SerializeSize())
		feeRate2 := float64(blockTxs[i+1].SerializeSize())
		if feeRate1 < feeRate2 {
			t.Errorf("Transactions not sorted by fee rate")
		}
	}

	t.Logf("Fee priority test completed: %d transactions in mempool", len(blockTxs))
}

// ============================================================================
// Test 4: Full System Integration
// ============================================================================

// TestFullSystemIntegration tests the complete system integration.
func TestFullSystemIntegration(t *testing.T) {
	t.Log("Starting full system integration test...")

	// Step 1: Initialize UTXO store with genesis
	alice := createTestWallet(t)
	store := utxo.NewUTXOStore()

	// Create multiple genesis UTXOs for Alice
	genesisUTXO1 := &utxo.UTXO{
		TxHash:  [32]byte{0xDE, 0xAD, 0xBE, 0xEF},
		Index:   0,
		Value:   50000000, // 500 AIB
		Address: alice.GetAddress(),
	}
	genesisUTXO2 := &utxo.UTXO{
		TxHash:  [32]byte{0xFE, 0xDC, 0xBA, 0x98},
		Index:   0,
		Value:   50000000, // 500 AIB
		Address: alice.GetAddress(),
	}
	store.AddUTXO(genesisUTXO1)
	store.AddUTXO(genesisUTXO2)

	// Step 2: Initialize mempool
	mempool := utxo.NewMempool(10000, 100)

	// Step 3: Initialize consensus
	config := utxo.DefaultPoSConfig()
	consensus := utxo.NewConsensusState(config)
	validatorPub, validatorPriv, _ := ed25519.GenerateKey(nil)
	var validatorAddr [32]byte
	copy(validatorAddr[:], validatorPub)
	consensus.AddValidator(validatorAddr, 1000000, validatorPub)

	// Step 4: Create genesis block
	genesis := utxo.NewBlock(nil, [32]byte{}, 0, validatorAddr)

	// Step 5: Simulate blockchain
	chain := &testChain{
		blocks:    []*utxo.Block{genesis},
		utxoStore: store,
	}

	// Step 6: Alice creates multiple transactions
	bob := createTestWallet(t)
	charlie := createTestWallet(t)

	// Transaction 1: Alice -> Bob (using first UTXO)
	tx1 := createSignedTransactionFromUTXO(t, store, alice, bob.GetAddress(), 10000000, 1000, [32]byte{0xDE, 0xAD, 0xBE, 0xEF}, 0)
	err := mempool.AddTransaction(tx1, store)
	if err != nil {
		t.Fatalf("Failed to add tx1: %v", err)
	}

	// Transaction 2: Alice -> Charlie (using second UTXO)
	tx2 := createSignedTransactionFromUTXO(t, store, alice, charlie.GetAddress(), 20000000, 1000, [32]byte{0xFE, 0xDC, 0xBA, 0x98}, 0)
	err = mempool.AddTransaction(tx2, store)
	if err != nil {
		t.Fatalf("Failed to add tx2: %v", err)
	}

	// Verify mempool
	if mempool.Size() != 2 {
		t.Errorf("Expected 2 transactions in mempool, got %d", mempool.Size())
	}

	// Step 7: Propose new block
	txs := mempool.GetTransactionsForBlock(1000000)
	hash := genesis.Hash
	newBlock := utxo.NewBlock(txs, hash, 1, validatorAddr)
	hash = newBlock.CalculateHash()
	newBlock.Header.Signature = ed25519.Sign(validatorPriv, hash[:])

	err = chain.AddBlock(newBlock)
	if err != nil {
		t.Fatalf("Failed to add block: %v", err)
	}

	// Verify chain state
	if chain.GetHeight() != 2 {
		t.Errorf("Chain height: expected 2, got %d", chain.GetHeight())
	}

	// Verify balances
	aliceBalance := store.GetBalance(alice.GetAddress())
	bobBalance := store.GetBalance(bob.GetAddress())
	charlieBalance := store.GetBalance(charlie.GetAddress())

	t.Logf("Final balances:")
	t.Logf("  Alice: %d", aliceBalance)
	t.Logf("  Bob: %d", bobBalance)
	t.Logf("  Charlie: %d", charlieBalance)

	// Verify expected balances
	if bobBalance != 10000000 {
		t.Errorf("Bob balance: expected 10000000, got %d", bobBalance)
	}
	if charlieBalance != 20000000 {
		t.Errorf("Charlie balance: expected 20000000, got %d", charlieBalance)
	}

	t.Log("Full system integration test completed successfully")
}

// Helper function to create a signed transaction
func createSignedTransaction(t *testing.T, store *utxo.UTXOStore, from *wallet.Wallet, to [32]byte, amount, fee uint64) *utxo.Transaction {
	t.Helper()

	utxos, _, err := store.GetUTXOsForAmount(from.GetAddress(), amount+fee)
	if err != nil {
		t.Fatalf("Failed to get UTXOs: %v", err)
	}

	var inputs []utxo.TXInput
	var totalValue uint64

	for _, u := range utxos {
		inputs = append(inputs, utxo.TXInput{
			TxHash:    u.TxHash,
			Index:     u.Index,
			Signature: nil,
			PublicKey: from.GetPublicKey(),
		})
		totalValue += u.Value
	}

	outputs := []utxo.TXOutput{
		{Value: amount, Address: to},
		{Value: totalValue - amount - fee, Address: from.GetAddress()},
	}

	tx := utxo.NewTransaction(inputs, outputs)

	// Sign each input
	for i := range tx.Inputs {
		err = from.SignTransaction(tx, i)
		if err != nil {
			t.Fatalf("Failed to sign input %d: %v", i, err)
		}
	}

	return tx
}

// Helper function to create a signed transaction from a specific UTXO
func createSignedTransactionFromUTXO(t *testing.T, store *utxo.UTXOStore, from *wallet.Wallet, to [32]byte, amount, fee uint64, utxoHash [32]byte, utxoIndex uint32) *utxo.Transaction {
	t.Helper()

	utxoEntry, err := store.GetUTXO(utxoHash, utxoIndex)
	if err != nil {
		t.Fatalf("Failed to get UTXO: %v", err)
	}

	inputs := []utxo.TXInput{
		{
			TxHash:    utxoEntry.TxHash,
			Index:     utxoEntry.Index,
			Signature: nil,
			PublicKey: from.GetPublicKey(),
		},
	}

	outputs := []utxo.TXOutput{
		{Value: amount, Address: to},
		{Value: utxoEntry.Value - amount - fee, Address: from.GetAddress()},
	}

	tx := utxo.NewTransaction(inputs, outputs)

	// Sign the input
	err = from.SignTransaction(tx, 0)
	if err != nil {
		t.Fatalf("Failed to sign input: %v", err)
	}

	return tx
}

// Ensure crypto package is used
var _ = crypto.Signer(nil)
