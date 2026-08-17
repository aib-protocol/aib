// Package sync provides block synchronization and propagation functionality.
// This file contains unit tests for transaction propagation.
package sync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Transaction Propagation Tests
// ============================================================================

func TestNewTransactionPropagator(t *testing.T) {
	net := newMockNetwork()

	cfg := &TransactionPropagationConfig{
		PropagationTimeout: 15 * time.Second,
		MaxPendingTx:       10000,
		MempoolExpiry:      1 * time.Hour,
		GossipFanout:       3,
	}

	tp := NewTransactionPropagator(net, cfg)
	if tp == nil {
		t.Fatal("NewTransactionPropagator returned nil")
	}

	if tp.propagationTimeout != 15*time.Second {
		t.Errorf("PropagationTimeout = %v, want 15s", tp.propagationTimeout)
	}

	if tp.maxPendingTx != 10000 {
		t.Errorf("MaxPendingTx = %d, want 10000", tp.maxPendingTx)
	}

	if tp.mempoolExpiry != 1*time.Hour {
		t.Errorf("MempoolExpiry = %v, want 1h", tp.mempoolExpiry)
	}

	if tp.gossipFanout != 3 {
		t.Errorf("GossipFanout = %d, want 3", tp.gossipFanout)
	}
}

func TestTransactionPropagatorStartStop(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	// Test Start
	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Test Stop
	if err := tp.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestTransactionPropagatorStartTwice(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Second start should fail
	if err := tp.Start(); err == nil {
		t.Error("Expected error on second Start()")
	}

	tp.Stop()
}

func TestBroadcastTransaction(t *testing.T) {
	net := newMockNetwork()

	// Add a test peer
	net.peers["testpeer"] = &p2p.PeerInfo{
		ID:        "testpeer",
		Connected: true,
		LastSeen:  time.Now(),
	}

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create and broadcast a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	if err := tp.BroadcastTransaction(tx); err != nil {
		t.Fatalf("BroadcastTransaction() error = %v", err)
	}

	// Check that the transaction was added to mempool
	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.RLock()
	if _, exists := tp.mempool[txHash]; !exists {
		t.Error("Transaction should be in mempool after broadcast")
	}
	tp.mu.RUnlock()
}

func TestReceiveTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Receive transaction
	if err := tp.ReceiveTransaction(tx, "peer1"); err != nil {
		t.Fatalf("ReceiveTransaction() error = %v", err)
	}

	// Check that transaction was added to mempool
	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.RLock()
	if _, exists := tp.mempool[txHash]; !exists {
		t.Error("Transaction should be in mempool after receive")
	}
	tp.mu.RUnlock()
}

func TestReceiveDuplicateTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Receive first time
	if err := tp.ReceiveTransaction(tx, "peer1"); err != nil {
		t.Fatalf("First ReceiveTransaction() error = %v", err)
	}

	// Try to receive again - should fail
	if err := tp.ReceiveTransaction(tx, "peer1"); err == nil {
		t.Error("Expected error on duplicate transaction")
	}
}

func TestValidateTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Test nil transaction
	if err := tp.validateTransaction(nil); err == nil {
		t.Error("Expected error for nil transaction")
	}

	// Test missing sender
	tx := &Transaction{
		To:        "receiver",
		Amount:    100,
		Timestamp: time.Now().Unix(),
		Signature: []byte("test"),
	}

	if err := tp.validateTransaction(tx); err == nil {
		t.Error("Expected error for missing sender")
	}

	// Test missing recipient
	tx2 := &Transaction{
		From:      "sender",
		Amount:    100,
		Timestamp: time.Now().Unix(),
		Signature: []byte("test"),
	}

	if err := tp.validateTransaction(tx2); err == nil {
		t.Error("Expected error for missing recipient")
	}

	// Test zero amount
	tx3 := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    0,
		Timestamp: time.Now().Unix(),
		Signature: []byte("test"),
	}

	if err := tp.validateTransaction(tx3); err == nil {
		t.Error("Expected error for zero amount")
	}

	// Test missing signature
	tx4 := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Timestamp: time.Now().Unix(),
		Signature: nil,
	}

	if err := tp.validateTransaction(tx4); err == nil {
		t.Error("Expected error for missing signature")
	}

	// Test future timestamp
	tx5 := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Timestamp: time.Now().Add(10 * time.Minute).Unix(),
		Signature: []byte("test"),
	}

	if err := tp.validateTransaction(tx5); err == nil {
		t.Error("Expected error for future timestamp")
	}

	// Test old timestamp
	tx6 := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Timestamp: time.Now().Add(-48 * time.Hour).Unix(),
		Signature: []byte("test"),
	}

	if err := tp.validateTransaction(tx6); err == nil {
		t.Error("Expected error for old timestamp")
	}

	// Test valid transaction
	tx7 := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	if err := tp.validateTransaction(tx7); err != nil {
		t.Errorf("Unexpected error for valid transaction: %v", err)
	}
}

func TestGetTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create and add a transaction to mempool
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.Lock()
	tp.mempool[txHash] = tx
	tp.mu.Unlock()

	// Get transaction
	retrieved, exists := tp.GetTransaction(txHash)
	if !exists {
		t.Error("Transaction should exist in mempool")
	}

	if retrieved.From != "sender" {
		t.Errorf("From = %s, want sender", retrieved.From)
	}

	// Get non-existent transaction
	_, exists = tp.GetTransaction("nonexistent")
	if exists {
		t.Error("Non-existent transaction should not be found")
	}
}

func TestGetMempoolSize(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Check empty mempool
	size := tp.GetMempoolSize()
	if size != 0 {
		t.Errorf("Empty mempool size = %d, want 0", size)
	}

	// Add transactions
	for i := 0; i < 5; i++ {
		tx := &Transaction{
			From:      "sender",
			To:        "receiver",
			Amount:    uint64(i * 100),
			Fee:       1,
			Timestamp: time.Now().Unix(),
			Nonce:     uint64(i),
			Signature: []byte("test-signature"),
		}

		txHash := hex.EncodeToString(tx.Hash())
		tp.mu.Lock()
		tp.mempool[txHash] = tx
		tp.mu.Unlock()
	}

	// Check mempool size
	size = tp.GetMempoolSize()
	if size != 5 {
		t.Errorf("Mempool size = %d, want 5", size)
	}
}

func TestTransactionMessage(t *testing.T) {
	msg := &TransactionMessage{
		Type:      "tx_announce",
		FromPeer:  "peer1",
		TTL:       3,
		RequestID: "req-123",
	}

	if msg.Type != "tx_announce" {
		t.Errorf("Type = %s, want tx_announce", msg.Type)
	}

	if msg.TTL != 3 {
		t.Errorf("TTL = %d, want 3", msg.TTL)
	}

	if msg.RequestID != "req-123" {
		t.Errorf("RequestID = %s, want req-123", msg.RequestID)
	}
}

func TestTransactionPropagatorCallback(t *testing.T) {
	net := newMockNetwork()

	receivedTxs := make([]*Transaction, 0)
	var mu syncWriter

	cfg := &TransactionPropagationConfig{
		OnTxReceived: func(tx *Transaction, from p2p.PeerID) error {
			mu.Lock()
			receivedTxs = append(receivedTxs, tx)
			mu.Unlock()
			return nil
		},
	}

	tp := NewTransactionPropagator(net, cfg)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Receive transaction
	if err := tp.ReceiveTransaction(tx, "peer1"); err != nil {
		t.Fatalf("ReceiveTransaction() error = %v", err)
	}

	// Check callback was called
	mu.Lock()
	if len(receivedTxs) != 1 {
		t.Errorf("Expected 1 received transaction, got %d", len(receivedTxs))
	}
	mu.Unlock()
}

// ============================================================================
// Transaction Propagation Message Handler Tests
// ============================================================================

func TestHandleTransactionMessageUnknownType(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction message with unknown type
	msg := &TransactionMessage{
		Type:     "unknown_type",
		FromPeer: "peer1",
	}

	// Marshal the message
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	p2pMsg := &p2p.Message{
		Type:    "unknown_type",
		Payload: data,
		Sender:  "peer1",
	}

	// Handle the message - should return error for unknown type
	err = tp.handleTransactionMessage(context.Background(), p2pMsg, "peer1")
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestHandleTransactionAnnounce(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	msg := &TransactionMessage{
		Type:         "tx_announce",
		Transaction:  tx,
		FromPeer:     "peer1",
		TTL:          1,
		ReceivedFrom: "",
	}

	err := tp.handleTransactionAnnounce(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleTransactionAnnounce() error = %v", err)
	}

	// Check that transaction was added to mempool
	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.RLock()
	_, exists := tp.mempool[txHash]
	tp.mu.RUnlock()

	if !exists {
		t.Error("Transaction should be in mempool after announce")
	}
}

func TestHandleTransactionAnnounceNilTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	msg := &TransactionMessage{
		Type:        "tx_announce",
		Transaction: nil,
		FromPeer:    "peer1",
	}

	err := tp.handleTransactionAnnounce(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil transaction")
	}
}

func TestHandleTransactionAnnounceDuplicate(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	msg := &TransactionMessage{
		Type:         "tx_announce",
		Transaction:  tx,
		FromPeer:     "peer1",
		TTL:          1,
		ReceivedFrom: "",
	}

	// First announcement
	err := tp.handleTransactionAnnounce(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("First handleTransactionAnnounce() error = %v", err)
	}

	// Second announcement (duplicate)
	err = tp.handleTransactionAnnounce(context.Background(), msg, "peer2")
	if err != nil {
		t.Fatalf("Second handleTransactionAnnounce() error = %v", err)
	}
}

func TestHandleTransactionRequest(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Add a transaction to mempool
	tx := &Transaction{
		ID:        "tx-1",
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.Lock()
	tp.mempool[txHash] = tx
	tp.mu.Unlock()

	msg := &TransactionMessage{
		Type:        "tx_request",
		Transaction: tx,
		FromPeer:    "peer1",
		RequestID:   "req-123",
	}

	err := tp.handleTransactionRequest(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleTransactionRequest() error = %v", err)
	}

	// Check that a response message was sent
	if len(net.messages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(net.messages))
	}
}

func TestHandleTransactionRequestInvalid(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Request with nil transaction
	msg := &TransactionMessage{
		Type:        "tx_request",
		Transaction: nil,
		FromPeer:    "peer1",
		RequestID:   "req-123",
	}

	err := tp.handleTransactionRequest(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil transaction request")
	}
}

func TestHandleTransactionRequestNotFound(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Request a transaction not in mempool
	tx := &Transaction{
		ID:        "nonexistent",
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	msg := &TransactionMessage{
		Type:        "tx_request",
		Transaction: tx,
		FromPeer:    "peer1",
		RequestID:   "req-123",
	}

	err := tp.handleTransactionRequest(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for transaction not found")
	}
}

func TestHandleTransactionResponse(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	msg := &TransactionMessage{
		Type:        "tx_response",
		Transaction: tx,
		FromPeer:    "peer1",
	}

	err := tp.handleTransactionResponse(context.Background(), msg, "peer1")
	if err != nil {
		t.Fatalf("handleTransactionResponse() error = %v", err)
	}
}

func TestHandleTransactionResponseNilTransaction(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	msg := &TransactionMessage{
		Type:        "tx_response",
		Transaction: nil,
		FromPeer:    "peer1",
	}

	err := tp.handleTransactionResponse(context.Background(), msg, "peer1")
	if err == nil {
		t.Error("Expected error for nil transaction in response")
	}
}

func TestGetPendingTxCount(t *testing.T) {
	net := newMockNetwork()

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Check empty count
	count := tp.GetPendingTxCount()
	if count != 0 {
		t.Errorf("Expected 0 pending txs, got %d", count)
	}

	// Add a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.Lock()
	tp.pendingTxs[txHash] = time.Now()
	tp.mu.Unlock()

	// Check count
	count = tp.GetPendingTxCount()
	if count != 1 {
		t.Errorf("Expected 1 pending tx, got %d", count)
	}
}

func TestRelayTransaction(t *testing.T) {
	net := newMockNetwork()

	// Add peers
	net.peers["peer1"] = &p2p.PeerInfo{
		ID:        "peer1",
		Connected: true,
		LastSeen:  time.Now(),
	}
	net.peers["peer2"] = &p2p.PeerInfo{
		ID:        "peer2",
		Connected: true,
		LastSeen:  time.Now(),
	}
	net.peers["peer3"] = &p2p.PeerInfo{
		ID:        "peer3",
		Connected: true,
		LastSeen:  time.Now(),
	}

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Relay to peer1
	tp.relayTransaction(tx, "peer1")
}

func TestRelayTransactionSinglePeer(t *testing.T) {
	net := newMockNetwork()

	// Add only one peer
	net.peers["peer1"] = &p2p.PeerInfo{
		ID:        "peer1",
		Connected: true,
		LastSeen:  time.Now(),
	}

	tp := NewTransactionPropagator(net, nil)

	if err := tp.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tp.Stop()

	// Create a transaction
	tx := &Transaction{
		From:      "sender",
		To:        "receiver",
		Amount:    100,
		Fee:       1,
		Timestamp: time.Now().Unix(),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Relay should return early with single peer
	tp.relayTransaction(tx, "peer1")
}
