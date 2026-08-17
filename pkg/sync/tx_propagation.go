// Package sync provides block synchronization and propagation functionality.
// This file implements transaction propagation.
package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Transaction Propagation
// ============================================================================

// TransactionPropagator handles transaction propagation.
type TransactionPropagator struct {
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	p2p     Network

	// Configuration
	propagationTimeout time.Duration
	maxPendingTx       int
	mempoolExpiry      time.Duration

	// Mempool
	mempool      map[string]*Transaction // tx hash -> transaction
	pendingTxs   map[string]time.Time    // tx hash -> received time
	announcedTxs map[string]time.Time    // tx hash -> announcement time

	// Propagation
	gossipFanout int

	// Callbacks
	onTxReceived  func(*Transaction, p2p.PeerID) error
	onTxValidated func(*Transaction) error
}

// TransactionPropagationConfig holds configuration for transaction propagation.
type TransactionPropagationConfig struct {
	PropagationTimeout time.Duration
	MaxPendingTx       int
	MempoolExpiry      time.Duration
	GossipFanout       int
	OnTxReceived       func(*Transaction, p2p.PeerID) error
	OnTxValidated      func(*Transaction) error
}

// DefaultTransactionPropagationConfig returns default configuration.
func DefaultTransactionPropagationConfig() *TransactionPropagationConfig {
	return &TransactionPropagationConfig{
		PropagationTimeout: 15 * time.Second,
		MaxPendingTx:       10000,
		MempoolExpiry:      1 * time.Hour,
		GossipFanout:       GossipFanout,
	}
}

// NewTransactionPropagator creates a new TransactionPropagator.
func NewTransactionPropagator(p2p Network, cfg *TransactionPropagationConfig) *TransactionPropagator {
	if cfg == nil {
		cfg = DefaultTransactionPropagationConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TransactionPropagator{
		p2p:                p2p,
		ctx:                ctx,
		cancel:             cancel,
		propagationTimeout: cfg.PropagationTimeout,
		maxPendingTx:       cfg.MaxPendingTx,
		mempoolExpiry:      cfg.MempoolExpiry,
		gossipFanout:       cfg.GossipFanout,
		mempool:            make(map[string]*Transaction),
		pendingTxs:         make(map[string]time.Time),
		announcedTxs:       make(map[string]time.Time),
		onTxReceived:       cfg.OnTxReceived,
		onTxValidated:      cfg.OnTxValidated,
	}
}

// Start starts the transaction propagator.
func (tp *TransactionPropagator) Start() error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.running {
		return errors.New("transaction propagator: already running")
	}

	// Register transaction protocol
	if err := tp.p2p.RegisterProtocol(TxPropagationProtocol, tp.handleTransactionMessage); err != nil {
		return fmt.Errorf("failed to register transaction protocol: %w", err)
	}

	tp.running = true

	// Start cleanup worker
	tp.wg.Add(1)
	go tp.cleanupWorker()

	return nil
}

// Stop stops the transaction propagator.
func (tp *TransactionPropagator) Stop() error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if !tp.running {
		return errors.New("transaction propagator: not running")
	}

	tp.running = false
	tp.cancel()

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		tp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	return nil
}

// BroadcastTransaction broadcasts a new transaction to the network.
func (tp *TransactionPropagator) BroadcastTransaction(tx *Transaction) error {
	if tx == nil {
		return errors.New("transaction propagator: nil transaction")
	}

	tp.mu.Lock()
	if !tp.running {
		tp.mu.Unlock()
		return errors.New("transaction propagator: not running")
	}
	tp.mu.Unlock()

	// Generate transaction ID if not present
	if tx.ID == "" {
		tx.ID = tp.generateTxID(tx)
	}

	// Validate transaction before broadcasting
	if err := tp.validateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Add to mempool
	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.Lock()
	tp.mempool[txHash] = tx
	tp.pendingTxs[txHash] = time.Now()
	tp.mu.Unlock()

	// Create transaction announcement message
	announce := &TransactionMessage{
		Type:         "tx_announce",
		Transaction:  tx,
		FromPeer:     tp.p2p.PeerID(),
		TTL:          tp.gossipFanout,
		ReceivedFrom: "",
	}

	return tp.gossipTransaction(tx, announce)
}

// ReceiveTransaction receives a transaction from a peer.
func (tp *TransactionPropagator) ReceiveTransaction(tx *Transaction, from p2p.PeerID) error {
	if tx == nil {
		return errors.New("transaction propagator: nil transaction")
	}

	tp.mu.Lock()
	if !tp.running {
		tp.mu.Unlock()
		return errors.New("transaction propagator: not running")
	}
	tp.mu.Unlock()

	// Generate transaction ID if not present
	if tx.ID == "" {
		tx.ID = tp.generateTxID(tx)
	}

	// Check for duplicate
	txHash := hex.EncodeToString(tx.Hash())
	tp.mu.Lock()
	if _, exists := tp.mempool[txHash]; exists {
		tp.mu.Unlock()
		return errors.New("transaction already in mempool")
	}

	if _, exists := tp.pendingTxs[txHash]; exists {
		tp.mu.Unlock()
		return errors.New("transaction already pending")
	}
	tp.pendingTxs[txHash] = time.Now()
	tp.mu.Unlock()

	// Validate transaction
	if err := tp.validateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Add to mempool
	tp.mu.Lock()
	tp.mempool[txHash] = tx
	tp.mu.Unlock()

	// Trigger callback
	if tp.onTxReceived != nil {
		if err := tp.onTxReceived(tx, from); err != nil {
			return err
		}
	}

	// Relay to other peers
	tp.relayTransaction(tx, from)

	return nil
}

// GetTransaction returns a transaction from mempool by hash.
func (tp *TransactionPropagator) GetTransaction(txHash string) (*Transaction, bool) {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	tx, exists := tp.mempool[txHash]
	return tx, exists
}

// GetMempoolSize returns the number of transactions in mempool.
func (tp *TransactionPropagator) GetMempoolSize() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return len(tp.mempool)
}

// GetPendingTxCount returns the number of pending transactions.
func (tp *TransactionPropagator) GetPendingTxCount() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return len(tp.pendingTxs)
}

// ============================================================================
// Private Methods
// ============================================================================

func (tp *TransactionPropagator) handleTransactionMessage(ctx context.Context, msg *p2p.Message, from p2p.PeerID) error {
	var txMsg TransactionMessage
	if err := json.Unmarshal(msg.Payload, &txMsg); err != nil {
		return fmt.Errorf("failed to unmarshal transaction message: %w", err)
	}

	switch txMsg.Type {
	case "tx_announce":
		return tp.handleTransactionAnnounce(ctx, &txMsg, from)
	case "tx_request":
		return tp.handleTransactionRequest(ctx, &txMsg, from)
	case "tx_response":
		return tp.handleTransactionResponse(ctx, &txMsg, from)
	default:
		return fmt.Errorf("unknown transaction message type: %s", txMsg.Type)
	}
}

func (tp *TransactionPropagator) handleTransactionAnnounce(ctx context.Context, msg *TransactionMessage, from p2p.PeerID) error {
	if msg.Transaction == nil {
		return errors.New("nil transaction in announcement")
	}

	txHash := hex.EncodeToString(msg.Transaction.Hash())

	// Check for duplicate announcement
	tp.mu.Lock()
	if _, exists := tp.announcedTxs[txHash]; exists {
		tp.mu.Unlock()
		return nil // Already announced
	}
	tp.announcedTxs[txHash] = time.Now()

	// Store original sender for TTL
	originPeer := from
	if msg.ReceivedFrom != "" {
		originPeer = msg.ReceivedFrom
	}
	tp.mu.Unlock()

	// Process transaction
	tx := msg.Transaction
	if err := tp.validateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Add to mempool (if not already)
	tp.mu.Lock()
	if _, exists := tp.mempool[txHash]; !exists {
		tp.mempool[txHash] = tx
	}
	tp.mu.Unlock()

	// Trigger callback
	if tp.onTxReceived != nil {
		tp.onTxReceived(tx, from)
	}

	// Relay to other peers if TTL > 0
	if msg.TTL > 0 {
		msg.TTL--
		msg.ReceivedFrom = originPeer
		tp.relayTransaction(tx, originPeer)
	}

	return nil
}

func (tp *TransactionPropagator) handleTransactionRequest(ctx context.Context, msg *TransactionMessage, from p2p.PeerID) error {
	if msg.Transaction == nil || msg.Transaction.ID == "" {
		return errors.New("invalid transaction request")
	}

	// Fetch requested transaction from mempool
	txHash := hex.EncodeToString(msg.Transaction.Hash())
	tp.mu.RLock()
	tx, exists := tp.mempool[txHash]
	tp.mu.RUnlock()

	if !exists {
		return errors.New("transaction not found")
	}

	// Send response
	response := &TransactionMessage{
		Type:        "tx_response",
		Transaction: tx,
		FromPeer:    tp.p2p.PeerID(),
		RequestID:   msg.RequestID,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	responseMsg := &p2p.Message{
		Type:     "tx_response",
		Payload:  data,
		Sender:   tp.p2p.PeerID(),
		Protocol: TxPropagationProtocol,
	}

	return tp.p2p.SendMessage(ctx, from, TxPropagationProtocol, responseMsg)
}

func (tp *TransactionPropagator) handleTransactionResponse(ctx context.Context, msg *TransactionMessage, from p2p.PeerID) error {
	if msg.Transaction == nil {
		return errors.New("nil transaction in response")
	}

	// Process received transaction
	return tp.ReceiveTransaction(msg.Transaction, from)
}

func (tp *TransactionPropagator) gossipTransaction(tx *Transaction, msg *TransactionMessage) error {
	peers := tp.p2p.GetPeers()
	if len(peers) == 0 {
		return nil
	}

	// Shuffle peers for randomness
	shuffledPeers := make([]*p2p.PeerInfo, len(peers))
	copy(shuffledPeers, peers)

	// Select random peers to send to
	numPeers := tp.gossipFanout
	if len(shuffledPeers) < numPeers {
		numPeers = len(shuffledPeers)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Send to selected peers
	for i := 0; i < numPeers; i++ {
		peerMsg := &p2p.Message{
			Type:     msg.Type,
			Payload:  data,
			Sender:   tp.p2p.PeerID(),
			Protocol: TxPropagationProtocol,
		}

		ctx, cancel := context.WithTimeout(tp.ctx, tp.propagationTimeout)
		if err := tp.p2p.SendMessage(ctx, shuffledPeers[i].ID, TxPropagationProtocol, peerMsg); err != nil {
			cancel()
			continue
		}
		cancel()
	}

	return nil
}

func (tp *TransactionPropagator) relayTransaction(tx *Transaction, from p2p.PeerID) {
	peers := tp.p2p.GetPeers()
	if len(peers) <= 1 {
		return
	}

	msg := &TransactionMessage{
		Type:         "tx_announce",
		Transaction:  tx,
		FromPeer:     tp.p2p.PeerID(),
		TTL:          tp.gossipFanout - 1,
		ReceivedFrom: from,
	}

	tp.gossipTransaction(tx, msg)
}

func (tp *TransactionPropagator) cleanupWorker() {
	defer tp.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-tp.ctx.Done():
			return
		case <-ticker.C:
			tp.cleanupOldEntries()
		}
	}
}

func (tp *TransactionPropagator) cleanupOldEntries() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	now := time.Now()

	// Clean up expired mempool entries
	for hash, tx := range tp.mempool {
		if now.Sub(time.Unix(tx.Timestamp, 0)) > tp.mempoolExpiry {
			delete(tp.mempool, hash)
			delete(tp.pendingTxs, hash)
			delete(tp.announcedTxs, hash)
		}
	}

	// Clean up old pending transactions
	for hash, t := range tp.pendingTxs {
		if now.Sub(t) > tp.mempoolExpiry {
			delete(tp.pendingTxs, hash)
		}
	}

	// Clean up old announcements
	for hash, t := range tp.announcedTxs {
		if now.Sub(t) > tp.mempoolExpiry {
			delete(tp.announcedTxs, hash)
		}
	}

	// Limit mempool size
	if len(tp.mempool) > tp.maxPendingTx {
		// Remove oldest transactions
		oldest := make([]string, 0, len(tp.mempool)-tp.maxPendingTx)
		for hash, tx := range tp.mempool {
			// Simple LRU strategy - remove based on timestamp
			if len(oldest) < len(tp.mempool)-tp.maxPendingTx {
				oldest = append(oldest, hash)
			} else {
				// Find oldest among current selection
				oldestTime := time.Unix(tx.Timestamp, 0)
				for i, oldHash := range oldest {
					if oldTx, ok := tp.mempool[oldHash]; ok {
						if time.Unix(oldTx.Timestamp, 0).Before(oldestTime) {
							oldest[i] = hash
							oldestTime = time.Unix(tx.Timestamp, 0)
						}
					}
				}
			}
		}

		for _, hash := range oldest {
			delete(tp.mempool, hash)
			delete(tp.pendingTxs, hash)
			delete(tp.announcedTxs, hash)
		}
	}
}

func (tp *TransactionPropagator) validateTransaction(tx *Transaction) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}

	// Check required fields
	if tx.From == "" {
		return errors.New("missing sender")
	}
	if tx.To == "" {
		return errors.New("missing recipient")
	}
	if tx.Amount == 0 {
		return errors.New("zero amount")
	}

	// Verify signature
	if len(tx.Signature) == 0 {
		return errors.New("missing signature")
	}

	// Verify timestamp is not too far in the future
	maxFutureTime := time.Now().Add(5 * time.Minute).Unix()
	if tx.Timestamp > maxFutureTime {
		return errors.New("transaction timestamp too far in the future")
	}

	// Verify timestamp is not too old
	minTimestamp := time.Now().Add(-24 * time.Hour).Unix()
	if tx.Timestamp < minTimestamp {
		return errors.New("transaction timestamp too old")
	}

	return nil
}

func (tp *TransactionPropagator) generateTxID(tx *Transaction) string {
	data := fmt.Sprintf("%s%s%s%d%d%d",
		tx.From, tx.To, string(tx.Signature), tx.Amount, tx.Fee, tx.Timestamp)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ============================================================================
// TransactionMessage
// ============================================================================

// TransactionMessage represents a message in the transaction propagation protocol.
type TransactionMessage struct {
	Type         string       `json:"type"`
	Transaction  *Transaction `json:"transaction,omitempty"`
	FromPeer     p2p.PeerID   `json:"from_peer"`
	TTL          int          `json:"ttl"`
	ReceivedFrom p2p.PeerID   `json:"received_from,omitempty"`
	RequestID    string       `json:"request_id,omitempty"`
}
