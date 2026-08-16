// Package relayer provides cross-chain relayer functionality.
// This file implements the ChainAdapter interface and its implementations.
package relayer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ============================================================================
// Chain Adapter Interface
// ============================================================================

// ChainAdapter defines the interface for blockchain-specific implementations.
// Each chain (Bitcoin, Ethereum, Solana) has its own adapter that handles
// chain-specific operations like locking/unlocking funds and verifying transactions.
type ChainAdapter interface {
	// GetChainType returns the type of blockchain this adapter supports.
	GetChainType() ChainType

	// LockFunds locks funds on the source chain for a cross-chain swap.
	// Returns the lock transaction hash on success.
	LockFunds(req *SwapRequest) (string, error)

	// UnlockFunds unlocks funds on the destination chain using the merkle proof.
	// Returns the unlock transaction hash on success.
	UnlockFunds(req *SwapRequest, proof *MerkleProof) (string, error)

	// SubmitProof submits a merkle proof to the destination chain for verification.
	SubmitProof(txHash string, proof *MerkleProof) error

	// VerifyTx verifies a transaction on the chain and returns details if valid.
	// Returns (tx data, confirmations, error)
	VerifyTx(txHash string) (*CrossChainTx, uint64, error)

	// GetBlockHeight returns the current block height of the chain.
	GetBlockHeight() (uint64, error)

	// GetConfirmations returns the number of confirmations for a transaction.
	GetConfirmations(txHash string) (uint64, error)

	// GetMerkleProof generates a merkle proof for a transaction.
	GetMerkleProof(txHash string) (*MerkleProof, error)

	// GetTxByHash retrieves transaction details by hash.
	GetTxByHash(txHash string) (map[string]interface{}, error)

	// WaitForConfirmations waits for a transaction to reach required confirmations.
	WaitForConfirmations(txHash string, required uint64, timeout time.Duration) error

	// GetRequiredConfirmations returns the required confirmation count for this chain.
	GetRequiredConfirmations() uint64
}

// ============================================================================
// Base Chain Adapter
// ============================================================================

// BaseChainAdapter provides common functionality for all chain adapters.
type BaseChainAdapter struct {
	chainType       ChainType
	confirmations   uint64
	mu              sync.RWMutex
	// Simulated state for testing
	lockedFunds     map[string]*LockedFund
	transactions    map[string]*ChainTransaction
}

// LockedFund represents locked funds on a chain.
type LockedFund struct {
	RequestID   string
	Amount      *big.Int
	Recipient   Address
	LockTxHash  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// ChainTransaction represents a transaction on a specific chain.
type ChainTransaction struct {
	Hash        string
	BlockHash   string
	BlockNumber uint64
	Index       uint64
	From        Address
	To          Address
	Amount      *big.Int
	Token       string
	Confirmations uint64
	Timestamp   time.Time
	Status      string
}

// NewBaseChainAdapter creates a new base chain adapter.
func NewBaseChainAdapter(chainType ChainType, confirmations uint64) *BaseChainAdapter {
	return &BaseChainAdapter{
		chainType:     chainType,
		confirmations: confirmations,
		lockedFunds:   make(map[string]*LockedFund),
		transactions:  make(map[string]*ChainTransaction),
	}
}

// GetChainType returns the chain type.
func (b *BaseChainAdapter) GetChainType() ChainType {
	return b.chainType
}

// GetConfirmations returns the required confirmations.
func (b *BaseChainAdapter) GetRequiredConfirmations() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.confirmations
}

// SetConfirmations sets the required confirmations.
func (b *BaseChainAdapter) SetConfirmations(conf uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.confirmations = conf
}

// addTransaction adds a simulated transaction to the chain.
func (b *BaseChainAdapter) addTransaction(tx *ChainTransaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transactions[tx.Hash] = tx
}

// getTransaction retrieves a transaction by hash.
func (b *BaseChainAdapter) getTransaction(txHash string) (*ChainTransaction, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	tx, ok := b.transactions[txHash]
	return tx, ok
}

// addLockedFund adds a locked fund record.
func (b *BaseChainAdapter) addLockedFund(reqID string, fund *LockedFund) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lockedFunds[reqID] = fund
}

// getLockedFund retrieves a locked fund by request ID.
func (b *BaseChainAdapter) getLockedFund(reqID string) (*LockedFund, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	fund, ok := b.lockedFunds[reqID]
	return fund, ok
}

// ============================================================================
// Bitcoin Adapter
// ============================================================================

// BitcoinAdapter implements the ChainAdapter for Bitcoin.
type BitcoinAdapter struct {
	*BaseChainAdapter
}

// NewBitcoinAdapter creates a new Bitcoin adapter.
func NewBitcoinAdapter() *BitcoinAdapter {
	return &BitcoinAdapter{
		BaseChainAdapter: NewBaseChainAdapter(ChainBTC, 6), // 6 confirmations for BTC
	}
}

// LockFunds locks BTC for a cross-chain swap using a P2SH HTLC.
// Returns the Bitcoin transaction hash.
func (a *BitcoinAdapter) LockFunds(req *SwapRequest) (string, error) {
	if req.Amount == nil || req.Amount.Sign() <= 0 {
		return "", fmt.Errorf("invalid amount")
	}

	// Generate lock transaction
	// In real implementation, this would create a Bitcoin transaction
	// with an HTLC script that locks funds until the secret is revealed
	txHash := a.generateTxHash(req)

	// Create locked fund record
	fund := &LockedFund{
		RequestID:   req.ID,
		Amount:      req.Amount,
		Recipient:  req.Recipient,
		LockTxHash:  txHash,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.Deadline,
	}
	a.addLockedFund(req.ID, fund)

	// Create transaction record
	chainTx := &ChainTransaction{
		Hash:        txHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: 800000, // Simulated block number
		Index:       0,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       "BTC",
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return txHash, nil
}

// UnlockFunds unlocks BTC using the merkle proof.
func (a *BitcoinAdapter) UnlockFunds(req *SwapRequest, proof *MerkleProof) (string, error) {
	// Verify the proof first
	if proof == nil {
		return "", fmt.Errorf("merkle proof is required")
	}

	// In real implementation, this would:
	// 1. Verify the merkle proof against the block header
	// 2. Create a transaction that spends the HTLC with the secret
	// 3. Broadcast the transaction

	// Generate unlock transaction
	unlockTxHash := a.generateTxHash(req)

	// Create transaction record
	chainTx := &ChainTransaction{
		Hash:        unlockTxHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: proof.BlockNumber + 1,
		Index:       0,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       "BTC",
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return unlockTxHash, nil
}

// SubmitProof submits the merkle proof to the Bitcoin network.
func (a *BitcoinAdapter) SubmitProof(txHash string, proof *MerkleProof) error {
	if proof == nil {
		return fmt.Errorf("proof is required")
	}

	// In real implementation, this would submit the proof to Bitcoin network
	// For SPV (Simplified Payment Verification) clients

	// Verify the proof structure
	if len(proof.Proof) == 0 {
		return fmt.Errorf("invalid proof: empty proof path")
	}

	return nil
}

// VerifyTx verifies a Bitcoin transaction.
func (a *BitcoinAdapter) VerifyTx(txHash string) (*CrossChainTx, uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, 0, fmt.Errorf("transaction not found: %s", txHash)
	}

	crossChainTx := &CrossChainTx{
		SourceChain: ChainBTC,
		DestChain:   ChainAIB, // Simulated destination
		SourceTxHash: Hash{Chain: ChainBTC},
		Sender:      tx.From,
		Recipient:   tx.To,
		Amount:      tx.Amount,
		Token:       tx.Token,
		Status:      TxStatusCompleted,
	}

	return crossChainTx, tx.Confirmations, nil
}

// GetBlockHeight returns the current Bitcoin block height.
func (a *BitcoinAdapter) GetBlockHeight() (uint64, error) {
	// In real implementation, this would query a Bitcoin node
	// Simulated block height
	return 800001, nil
}

// GetConfirmations returns the number of confirmations for a transaction.
func (a *BitcoinAdapter) GetConfirmations(txHash string) (uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return 0, fmt.Errorf("transaction not found: %s", txHash)
	}
	return tx.Confirmations, nil
}

// GetMerkleProof generates a merkle proof for a transaction.
func (a *BitcoinAdapter) GetMerkleProof(txHash string) (*MerkleProof, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found: %s", txHash)
	}

	// Generate simulated merkle proof
	// In real implementation, this would query the Bitcoin network
	// for the merkle branch
	proof := &MerkleProof{
		TxHash:      Hash{Chain: ChainBTC},
		BlockHash:   Hash{Chain: ChainBTC},
		BlockNumber: tx.BlockNumber,
		Index:       tx.Index,
		Proof:       a.generateMerkleProof(txHash),
		Chain:       ChainBTC,
	}

	return proof, nil
}

// GetTxByHash retrieves Bitcoin transaction details.
func (a *BitcoinAdapter) GetTxByHash(txHash string) (map[string]interface{}, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found: %s", txHash)
	}

	return map[string]interface{}{
		"hash":         tx.Hash,
		"block_hash":   tx.BlockHash,
		"block_number": tx.BlockNumber,
		"index":        tx.Index,
		"from":         tx.From.String(),
		"to":           tx.To.String(),
		"amount":       tx.Amount.String(),
		"token":        tx.Token,
		"confirmations": tx.Confirmations,
		"timestamp":    tx.Timestamp.Unix(),
		"status":       tx.Status,
	}, nil
}

// WaitForConfirmations waits for a transaction to reach required confirmations.
func (a *BitcoinAdapter) WaitForConfirmations(txHash string, required uint64, timeout time.Duration) error {
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tx, ok := a.getTransaction(txHash)
			if !ok {
				return fmt.Errorf("transaction not found")
			}
			if tx.Confirmations >= required {
				return nil
			}
			// Simulate confirmation increment
			tx.Confirmations++

			if time.Since(start) > timeout {
				return fmt.Errorf("timeout waiting for confirmations")
			}
		}
	}
}

// generateTxHash generates a simulated transaction hash.
func (a *BitcoinAdapter) generateTxHash(req *SwapRequest) string {
	data := fmt.Sprintf("%s-%s-%s-%s-%s",
		req.ID, req.SourceChain, req.DestChain,
		req.Sender.String(), req.Amount.String())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// generateBlockHash generates a simulated block hash.
func (a *BitcoinAdapter) generateBlockHash() string {
	data := fmt.Sprintf("block-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// generateMerkleProof generates a simulated merkle proof.
func (a *BitcoinAdapter) generateMerkleProof(txHash string) [][]byte {
	// Generate simulated merkle proof nodes
	proof := make([][]byte, 0)
	for i := 0; i < 3; i++ { // Simulated proof depth
		data := fmt.Sprintf("proof-%s-%d", txHash, i)
		hash := sha256.Sum256([]byte(data))
		proof = append(proof, hash[:])
	}
	return proof
}

// ============================================================================
// Ethereum Adapter
// ============================================================================

// EthereumAdapter implements the ChainAdapter for Ethereum.
type EthereumAdapter struct {
	*BaseChainAdapter
}

// NewEthereumAdapter creates a new Ethereum adapter.
func NewEthereumAdapter() *EthereumAdapter {
	return &EthereumAdapter{
		BaseChainAdapter: NewBaseChainAdapter(ChainETH, 12), // 12 confirmations for ETH
	}
}

// LockFunds locks ETH/ERC-20 tokens for a cross-chain swap.
func (a *EthereumAdapter) LockFunds(req *SwapRequest) (string, error) {
	if req.Amount == nil || req.Amount.Sign() <= 0 {
		return "", fmt.Errorf("invalid amount")
	}

	// Generate lock transaction
	txHash := a.generateTxHash(req)

	// Create locked fund record
	fund := &LockedFund{
		RequestID:   req.ID,
		Amount:      req.Amount,
		Recipient:  req.Recipient,
		LockTxHash:  txHash,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.Deadline,
	}
	a.addLockedFund(req.ID, fund)

	// Create transaction record
	chainTx := &ChainTransaction{
		Hash:        txHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: 18000000, // Simulated block number
		Index:       5,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       req.Token,
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return txHash, nil
}

// UnlockFunds unlocks ETH/ERC-20 tokens using the merkle proof.
func (a *EthereumAdapter) UnlockFunds(req *SwapRequest, proof *MerkleProof) (string, error) {
	if proof == nil {
		return "", fmt.Errorf("merkle proof is required")
	}

	// Generate unlock transaction
	unlockTxHash := a.generateUnlockTxHash(req)

	chainTx := &ChainTransaction{
		Hash:        unlockTxHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: proof.BlockNumber + 1,
		Index:       0,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       req.Token,
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return unlockTxHash, nil
}

// SubmitProof submits the merkle proof to Ethereum (via oracle or relay).
func (a *EthereumAdapter) SubmitProof(txHash string, proof *MerkleProof) error {
	if proof == nil {
		return fmt.Errorf("proof is required")
	}

	// In real implementation, this would:
	// 1. Submit the proof to an on-chain oracle contract
	// 2. The oracle would verify the proof and emit an event
	// 3. The receiving contract would process the event

	return nil
}

// VerifyTx verifies an Ethereum transaction.
func (a *EthereumAdapter) VerifyTx(txHash string) (*CrossChainTx, uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, 0, fmt.Errorf("transaction not found: %s", txHash)
	}

	return &CrossChainTx{
		SourceChain: ChainETH,
		DestChain:   ChainAIB,
		SourceTxHash: Hash{Chain: ChainETH},
		Sender:      tx.From,
		Recipient:   tx.To,
		Amount:      tx.Amount,
		Token:       tx.Token,
		Status:      TxStatusCompleted,
	}, tx.Confirmations, nil
}

// GetBlockHeight returns the current Ethereum block height.
func (a *EthereumAdapter) GetBlockHeight() (uint64, error) {
	return 18000001, nil
}

// GetConfirmations returns the number of confirmations for a transaction.
func (a *EthereumAdapter) GetConfirmations(txHash string) (uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return 0, fmt.Errorf("transaction not found: %s", txHash)
	}
	return tx.Confirmations, nil
}

// GetMerkleProof generates a merkle proof for a transaction.
func (a *EthereumAdapter) GetMerkleProof(txHash string) (*MerkleProof, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found")
	}

	return &MerkleProof{
		TxHash:      Hash{Chain: ChainETH},
		BlockHash:   Hash{Chain: ChainETH},
		BlockNumber: tx.BlockNumber,
		Index:       tx.Index,
		Proof:       a.generateMerkleProof(txHash),
		Chain:       ChainETH,
	}, nil
}

// GetTxByHash retrieves Ethereum transaction details.
func (a *EthereumAdapter) GetTxByHash(txHash string) (map[string]interface{}, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found: %s", txHash)
	}

	return map[string]interface{}{
		"hash":         tx.Hash,
		"block_hash":   tx.BlockHash,
		"block_number": tx.BlockNumber,
		"index":        tx.Index,
		"from":         tx.From.String(),
		"to":           tx.To.String(),
		"amount":       tx.Amount.String(),
		"token":        tx.Token,
		"confirmations": tx.Confirmations,
		"timestamp":    tx.Timestamp.Unix(),
		"status":       tx.Status,
	}, nil
}

// WaitForConfirmations waits for a transaction to reach required confirmations.
func (a *EthereumAdapter) WaitForConfirmations(txHash string, required uint64, timeout time.Duration) error {
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tx, ok := a.getTransaction(txHash)
			if !ok {
				return fmt.Errorf("transaction not found")
			}
			if tx.Confirmations >= required {
				return nil
			}
			tx.Confirmations++

			if time.Since(start) > timeout {
				return fmt.Errorf("timeout waiting for confirmations")
			}
		}
	}
}

// generateTxHash generates a simulated Ethereum transaction hash.
func (a *EthereumAdapter) generateTxHash(req *SwapRequest) string {
	hash := sha256.Sum256([]byte(req.ID + "eth"))
	return hex.EncodeToString(hash[:])
}

// generateUnlockTxHash generates a simulated unlock transaction hash.
func (a *EthereumAdapter) generateUnlockTxHash(req *SwapRequest) string {
	hash := sha256.Sum256([]byte("unlock-" + req.ID))
	return hex.EncodeToString(hash[:])
}

// generateBlockHash generates a simulated Ethereum block hash.
func (a *EthereumAdapter) generateBlockHash() string {
	hash := sha256.Sum256([]byte(time.Now().Format(time.RFC3339)))
	return hex.EncodeToString(hash[:])
}

// generateMerkleProof generates a simulated merkle proof.
func (a *EthereumAdapter) generateMerkleProof(txHash string) [][]byte {
	proof := make([][]byte, 0)
	for i := 0; i < 4; i++ {
		data := fmt.Sprintf("eth-proof-%s-%d", txHash, i)
		hash := sha256.Sum256([]byte(data))
		proof = append(proof, hash[:])
	}
	return proof
}

// ============================================================================
// Solana Adapter
// ============================================================================

// SolanaAdapter implements the ChainAdapter for Solana.
type SolanaAdapter struct {
	*BaseChainAdapter
}

// NewSolanaAdapter creates a new Solana adapter.
func NewSolanaAdapter() *SolanaAdapter {
	return &SolanaAdapter{
		BaseChainAdapter: NewBaseChainAdapter(ChainSOL, 32), // 32 confirmations for Solana
	}
}

// LockFunds locks SPL tokens for a cross-chain swap.
func (a *SolanaAdapter) LockFunds(req *SwapRequest) (string, error) {
	if req.Amount == nil || req.Amount.Sign() <= 0 {
		return "", fmt.Errorf("invalid amount")
	}

	txHash := a.generateTxHash(req)

	fund := &LockedFund{
		RequestID:   req.ID,
		Amount:      req.Amount,
		Recipient:  req.Recipient,
		LockTxHash:  txHash,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.Deadline,
	}
	a.addLockedFund(req.ID, fund)

	chainTx := &ChainTransaction{
		Hash:        txHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: 230000000, // Simulated
		Index:       10,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       req.Token,
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return txHash, nil
}

// UnlockFunds unlocks SPL tokens using the merkle proof.
func (a *SolanaAdapter) UnlockFunds(req *SwapRequest, proof *MerkleProof) (string, error) {
	if proof == nil {
		return "", fmt.Errorf("merkle proof is required")
	}

	unlockTxHash := a.generateUnlockTxHash(req)

	chainTx := &ChainTransaction{
		Hash:        unlockTxHash,
		BlockHash:   a.generateBlockHash(),
		BlockNumber: proof.BlockNumber + 1,
		Index:       0,
		From:        req.Sender,
		To:          req.Recipient,
		Amount:      req.Amount,
		Token:       req.Token,
		Confirmations: 0,
		Timestamp:   time.Now(),
		Status:      "confirmed",
	}
	a.addTransaction(chainTx)

	return unlockTxHash, nil
}

// SubmitProof submits the merkle proof to Solana.
func (a *SolanaAdapter) SubmitProof(txHash string, proof *MerkleProof) error {
	if proof == nil {
		return fmt.Errorf("proof is required")
	}
	// In real implementation, submit proof to Solana program
	return nil
}

// VerifyTx verifies a Solana transaction.
func (a *SolanaAdapter) VerifyTx(txHash string) (*CrossChainTx, uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, 0, fmt.Errorf("transaction not found: %s", txHash)
	}

	return &CrossChainTx{
		SourceChain: ChainSOL,
		DestChain:   ChainAIB,
		SourceTxHash: Hash{Chain: ChainSOL},
		Sender:      tx.From,
		Recipient:   tx.To,
		Amount:      tx.Amount,
		Token:       tx.Token,
		Status:      TxStatusCompleted,
	}, tx.Confirmations, nil
}

// GetBlockHeight returns the current Solana block height.
func (a *SolanaAdapter) GetBlockHeight() (uint64, error) {
	return 230000001, nil
}

// GetConfirmations returns the number of confirmations for a transaction.
func (a *SolanaAdapter) GetConfirmations(txHash string) (uint64, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return 0, fmt.Errorf("transaction not found: %s", txHash)
	}
	return tx.Confirmations, nil
}

// GetMerkleProof generates a merkle proof for a transaction.
func (a *SolanaAdapter) GetMerkleProof(txHash string) (*MerkleProof, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found")
	}

	return &MerkleProof{
		TxHash:      Hash{Chain: ChainSOL},
		BlockHash:   Hash{Chain: ChainSOL},
		BlockNumber: tx.BlockNumber,
		Index:       tx.Index,
		Proof:       a.generateMerkleProof(txHash),
		Chain:       ChainSOL,
	}, nil
}

// GetTxByHash retrieves Solana transaction details.
func (a *SolanaAdapter) GetTxByHash(txHash string) (map[string]interface{}, error) {
	tx, ok := a.getTransaction(txHash)
	if !ok {
		return nil, fmt.Errorf("transaction not found: %s", txHash)
	}

	return map[string]interface{}{
		"hash":         tx.Hash,
		"block_hash":   tx.BlockHash,
		"block_number": tx.BlockNumber,
		"index":        tx.Index,
		"from":         tx.From.String(),
		"to":           tx.To.String(),
		"amount":       tx.Amount.String(),
		"token":        tx.Token,
		"confirmations": tx.Confirmations,
		"timestamp":    tx.Timestamp.Unix(),
		"status":       tx.Status,
	}, nil
}

// WaitForConfirmations waits for a transaction to reach required confirmations.
func (a *SolanaAdapter) WaitForConfirmations(txHash string, required uint64, timeout time.Duration) error {
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tx, ok := a.getTransaction(txHash)
			if !ok {
				return fmt.Errorf("transaction not found")
			}
			if tx.Confirmations >= required {
				return nil
			}
			tx.Confirmations++

			if time.Since(start) > timeout {
				return fmt.Errorf("timeout waiting for confirmations")
			}
		}
	}
}

// generateTxHash generates a simulated Solana transaction hash (base58 encoded).
func (a *SolanaAdapter) generateTxHash(req *SwapRequest) string {
	data := sha256.Sum256([]byte(req.ID + "sol"))
	// Simulate base58 encoding (simplified)
	return hex.EncodeToString(data[:32])
}

// generateUnlockTxHash generates a simulated unlock transaction hash.
func (a *SolanaAdapter) generateUnlockTxHash(req *SwapRequest) string {
	data := sha256.Sum256([]byte("unlock-" + req.ID))
	return hex.EncodeToString(data[:32])
}

// generateBlockHash generates a simulated Solana block hash.
func (a *SolanaAdapter) generateBlockHash() string {
	data := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
	return hex.EncodeToString(data[:32])
}

// generateMerkleProof generates a simulated merkle proof.
func (a *SolanaAdapter) generateMerkleProof(txHash string) [][]byte {
	proof := make([][]byte, 0)
	for i := 0; i < 5; i++ {
		data := sha256.Sum256([]byte(fmt.Sprintf("sol-proof-%s-%d", txHash, i)))
		proof = append(proof, data[:])
	}
	return proof
}

// ============================================================================
// Adapter Factory
// ============================================================================

// NewChainAdapter creates a new ChainAdapter for the specified chain type.
func NewChainAdapter(chainType ChainType) (ChainAdapter, error) {
	switch chainType {
	case ChainBTC:
		return NewBitcoinAdapter(), nil
	case ChainETH:
		return NewEthereumAdapter(), nil
	case ChainSOL:
		return NewSolanaAdapter(), nil
	default:
		return nil, fmt.Errorf("unsupported chain type: %s", chainType)
	}
}

// ============================================================================
// Adapter Manager
// ============================================================================

// AdapterManager manages multiple chain adapters.
type AdapterManager struct {
	adapters map[ChainType]ChainAdapter
	mu       sync.RWMutex
}

// NewAdapterManager creates a new AdapterManager.
func NewAdapterManager() *AdapterManager {
	return &AdapterManager{
		adapters: make(map[ChainType]ChainAdapter),
	}
}

// RegisterAdapter registers a chain adapter.
func (m *AdapterManager) RegisterAdapter(adapter ChainAdapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	chainType := adapter.GetChainType()
	m.adapters[chainType] = adapter

	return nil
}

// GetAdapter returns the adapter for a specific chain type.
func (m *AdapterManager) GetAdapter(chainType ChainType) (ChainAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	adapter, ok := m.adapters[chainType]
	if !ok {
		return nil, fmt.Errorf("adapter not found for chain: %s", chainType)
	}

	return adapter, nil
}

// GetSupportedChains returns all supported chain types.
func (m *AdapterManager) GetSupportedChains() []ChainType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chains := make([]ChainType, 0, len(m.adapters))
	for chainType := range m.adapters {
		chains = append(chains, chainType)
	}

	return chains
}

// ============================================================================
// JSON Serialization Helpers
// ============================================================================

// ToJSON serializes an adapter to JSON.
func ToJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to serialize: %w", err)
	}
	return string(data), nil
}

// FromJSON deserializes JSON to a value.
func FromJSON(data string, v interface{}) error {
	if err := json.Unmarshal([]byte(data), v); err != nil {
		return fmt.Errorf("failed to deserialize: %w", err)
	}
	return nil
}

// ============================================================================
// Utility Functions
// ============================================================================

// ParseChainType parses a string to ChainType.
func ParseChainType(s string) (ChainType, error) {
	switch s {
	case "BTC":
		return ChainBTC, nil
	case "ETH":
		return ChainETH, nil
	case "SOL":
		return ChainSOL, nil
	case "AIB":
		return ChainAIB, nil
	default:
		return "", fmt.Errorf("unknown chain type: %s", s)
	}
}

// IsValidChain checks if a chain type is valid.
func IsValidChain(chain ChainType) bool {
	switch chain {
	case ChainBTC, ChainETH, ChainSOL, ChainAIB:
		return true
	default:
		return false
	}
}

// NormalizeAmount normalizes the amount for a given chain.
func NormalizeAmount(amount *big.Int, decimals int) *big.Int {
	// Multiply by 10^decimals to get the smallest unit
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Int).Mul(amount, factor)
}

// DenormalizeAmount converts from smallest unit to human-readable amount.
func DenormalizeAmount(amount *big.Int, decimals int) *big.Int {
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Int).Div(amount, divisor)
}

// ============================================================================
// Cross-Chain Utilities
// ============================================================================

// ValidateSwapRequest validates a swap request.
func ValidateSwapRequest(req *SwapRequest) error {
	if req == nil {
		return fmt.Errorf("swap request is nil")
	}

	if req.Amount == nil || req.Amount.Sign() <= 0 {
		return fmt.Errorf("invalid amount")
	}

	if !IsValidChain(req.SourceChain) {
		return fmt.Errorf("invalid source chain: %s", req.SourceChain)
	}

	if !IsValidChain(req.DestChain) {
		return fmt.Errorf("invalid destination chain: %s", req.DestChain)
	}

	if req.SourceChain == req.DestChain {
		return fmt.Errorf("source and destination chain must be different")
	}

	if req.IsExpired() {
		return fmt.Errorf("swap request has expired")
	}

	if len(req.Sender.Data) == 0 {
		return fmt.Errorf("sender address is empty")
	}

	if len(req.Recipient.Data) == 0 {
		return fmt.Errorf("recipient address is empty")
	}

	return nil
}

// CompareChainAddresses compares addresses across chains (for display purposes).
func CompareChainAddresses(addr1, addr2 Address) bool {
	return bytes.Equal(addr1.Data, addr2.Data)
}
