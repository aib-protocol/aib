// Package relayer provides cross-chain relayer functionality.
// This file implements the main Relayer structure and its operations.
package relayer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ============================================================================
// Relayer Interface
// ============================================================================

// RelayerManager defines the interface for managing a relayer node.
type RelayerManager interface {
	// Register registers the relayer on the network
	Register(req *RegisterRequest) error

	// CreateSwap creates a new cross-chain swap
	CreateSwap(req *SwapRequest) (*CrossChainTx, error)

	// SubmitProof submits a merkle proof for a transaction
	SubmitProof(txHash string, proof *MerkleProof) error

	// ReleaseFunds releases funds on the destination chain
	ReleaseFunds(txHash string) error

	// GetStatus returns the current status of the relayer
	GetStatus() (*RelayerStatusInfo, error)

	// GetTransaction returns a transaction by ID
	GetTransaction(txHash string) (*CrossChainTx, error)

	// ListTransactions returns all transactions
	ListTransactions() []*CrossChainTx
}

// RelayerStatusInfo contains detailed relayer status information.
type RelayerStatusInfo struct {
	RelayerID       string     `json:"relayer_id"`
	Status          RelayerStatus `json:"status"`
	Stake           string     `json:"stake"`
	Reputation      float64    `json:"reputation"`
	TotalTXs        uint64     `json:"total_txs"`
	SuccessRate     float64    `json:"success_rate"`
	ActiveTXs       uint64     `json:"active_txs"`
	SupportedChains []ChainType `json:"supported_chains"`
	FeeRate         string     `json:"fee_rate"`
	Uptime          time.Duration `json:"uptime"`
}

// ============================================================================
// Main Relayer Implementation
// ============================================================================

// RelayerNode represents a cross-chain relayer node.
type RelayerNode struct {
	id              string
	address         Address
	nodeID          string
	adapters        *AdapterManager
	status          RelayerStatus
	stake           *big.Int
	supportedChains []ChainType
	feeRate         *big.Int
	reputation      float64
	totalTXs        uint64
	successRate     float64
	createdAt       time.Time
	lastActiveAt    time.Time
	transactions    map[string]*CrossChainTx
	pendingTasks    map[string]*Task
	mu              sync.RWMutex
	taskQueue       chan *Task
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// Task represents a relayer task.
type Task struct {
	ID          string
	Type        TaskType
	TxHash      string
	RelayerID   string
	Priority    int
	CreatedAt   time.Time
	CompletedAt *time.Time
	Status      string
}

// TaskType represents the type of task.
type TaskType string

const (
	TaskLockFunds    TaskType = "LockFunds"
	TaskUnlockFunds  TaskType = "UnlockFunds"
	TaskSubmitProof  TaskType = "SubmitProof"
	TaskVerifyTx     TaskType = "VerifyTx"
)

// NewRelayerNode creates a new RelayerNode instance.
func NewRelayerNode(
	id string,
	addr Address,
	nodeID string,
	chains []ChainType,
	stake *big.Int,
	feeRate *big.Int,
) *RelayerNode {
	adapters := NewAdapterManager()

	// Register adapters for supported chains
	for _, chain := range chains {
		adapter, err := NewChainAdapter(chain)
		if err != nil {
			// Log error but continue with other adapters
			continue
		}
		adapters.RegisterAdapter(adapter)
	}

	now := time.Now()

	return &RelayerNode{
		id:              id,
		address:         addr,
		nodeID:          nodeID,
		adapters:        adapters,
		status:          StatusActive,
		stake:           stake,
		supportedChains: chains,
		feeRate:         feeRate,
		reputation:      100.0,
		totalTXs:        0,
		successRate:     1.0,
		createdAt:       now,
		lastActiveAt:    now,
		transactions:    make(map[string]*CrossChainTx),
		pendingTasks:    make(map[string]*Task),
		taskQueue:       make(chan *Task, 1000),
		stopCh:          make(chan struct{}),
	}
}

// Register registers the relayer on the network.
func (r *RelayerNode) Register(req *RegisterRequest) error {
	if req == nil {
		return fmt.Errorf("register request is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate stake
	if req.Stake == nil || req.Stake.Sign() <= 0 {
		return fmt.Errorf("invalid stake amount")
	}

	// Validate fee rate
	if req.FeeRate == nil || req.FeeRate.Sign() <= 0 {
		return fmt.Errorf("invalid fee rate")
	}

	// Update relayer properties
	r.stake = req.Stake
	r.feeRate = req.FeeRate
	r.supportedChains = req.SupportedChains

	// Register adapters for new chains
	for _, chain := range req.SupportedChains {
		adapter, err := NewChainAdapter(chain)
		if err != nil {
			continue
		}
		r.adapters.RegisterAdapter(adapter)
	}

	r.lastActiveAt = time.Now()

	return nil
}

// CreateSwap creates a new cross-chain swap.
func (r *RelayerNode) CreateSwap(req *SwapRequest) (*CrossChainTx, error) {
	if req == nil {
		return nil, fmt.Errorf("swap request is nil")
	}

	// Validate swap request
	if err := ValidateSwapRequest(req); err != nil {
		return nil, fmt.Errorf("invalid swap request: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if relayer is active
	if r.status != StatusActive {
		return nil, fmt.Errorf("relayer is not active: %s", r.status)
	}

	// Check if relayer supports both chains
	hasSource := false
	hasDest := false
	for _, chain := range r.supportedChains {
		if chain == req.SourceChain {
			hasSource = true
		}
		if chain == req.DestChain {
			hasDest = true
		}
	}
	if !hasSource || !hasDest {
		return nil, fmt.Errorf("relayer does not support required chains")
	}

	// Get source chain adapter
	adapter, err := r.adapters.GetAdapter(req.SourceChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get source chain adapter: %w", err)
	}

	// Create transaction
	tx := req.ToCrossChainTx(r.id)
	tx.Fee = CalculateFee(req.Amount, r.feeRate)

	// Lock funds on source chain
	lockTxHash, err := adapter.LockFunds(req)
	if err != nil {
		r.updateStats(false)
		return nil, fmt.Errorf("failed to lock funds: %w", err)
	}

	tx.SourceTxHash = Hash{
		Chain: req.SourceChain,
		Data:  sha256.Sum256([]byte(lockTxHash)),
	}
	tx.Status = TxStatusLocked

	// Store transaction
	r.transactions[tx.ID] = tx

	// Create task for monitoring
	task := &Task{
		ID:        fmt.Sprintf("task-%s", tx.ID),
		Type:      TaskVerifyTx,
		TxHash:    lockTxHash,
		RelayerID: r.id,
		Priority:  1,
		CreatedAt: time.Now(),
		Status:    "pending",
	}
	r.pendingTasks[task.ID] = task

	r.lastActiveAt = time.Now()

	return tx, nil
}

// SubmitProof submits a merkle proof for a transaction.
func (r *RelayerNode) SubmitProof(txHash string, proof *MerkleProof) error {
	if txHash == "" {
		return fmt.Errorf("transaction hash is required")
	}

	if proof == nil {
		return fmt.Errorf("merkle proof is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find transaction
	var tx *CrossChainTx
	for _, t := range r.transactions {
		if t.SourceTxHash.String() == txHash {
			tx = t
			break
		}
	}

	if tx == nil {
		return fmt.Errorf("transaction not found: %s", txHash)
	}

	// Get source chain adapter
	adapter, err := r.adapters.GetAdapter(tx.SourceChain)
	if err != nil {
		return fmt.Errorf("failed to get adapter: %w", err)
	}

	// Submit proof
	if err := adapter.SubmitProof(txHash, proof); err != nil {
		r.updateStats(false)
		return fmt.Errorf("failed to submit proof: %w", err)
	}

	// Update transaction
	tx.SetProof(proof)

	r.lastActiveAt = time.Now()
	r.updateStats(true)

	return nil
}

// ReleaseFunds releases funds on the destination chain.
func (r *RelayerNode) ReleaseFunds(txHash string) error {
	if txHash == "" {
		return fmt.Errorf("transaction hash is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find transaction
	var tx *CrossChainTx
	for _, t := range r.transactions {
		if t.SourceTxHash.String() == txHash {
			tx = t
			break
		}
	}

	if tx == nil {
		return fmt.Errorf("transaction not found: %s", txHash)
	}

	// Check transaction status
	if tx.Status != TxStatusProofReady {
		return fmt.Errorf("transaction not ready for release: %s", tx.Status)
	}

	// Get destination chain adapter
	adapter, err := r.adapters.GetAdapter(tx.DestChain)
	if err != nil {
		return fmt.Errorf("failed to get destination adapter: %w", err)
	}

	// Create swap request from transaction
	req := &SwapRequest{
		ID:           tx.ID,
		SourceChain:  tx.SourceChain,
		DestChain:    tx.DestChain,
		Sender:       tx.Sender,
		Recipient:    tx.Recipient,
		Amount:       tx.Amount,
		Token:        tx.Token,
		Deadline:     tx.Expiry,
	}

	// Unlock funds on destination chain
	unlockTxHash, err := adapter.UnlockFunds(req, tx.Proof)
	if err != nil {
		r.updateStats(false)
		return fmt.Errorf("failed to unlock funds: %w", err)
	}

	tx.DestTxHash = Hash{
		Chain: tx.DestChain,
		Data:  sha256.Sum256([]byte(unlockTxHash)),
	}
	tx.Status = TxStatusCompleted

	// Remove completed task
	for id, task := range r.pendingTasks {
		if task.TxHash == txHash {
			delete(r.pendingTasks, id)
			break
		}
	}

	r.lastActiveAt = time.Now()
	r.updateStats(true)

	return nil
}

// GetStatus returns the current status of the relayer.
func (r *RelayerNode) GetStatus() (*RelayerStatusInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Count active transactions
	activeTXs := uint64(0)
	for _, tx := range r.transactions {
		if tx.Status != TxStatusCompleted && tx.Status != TxStatusFailed {
			activeTXs++
		}
	}

	return &RelayerStatusInfo{
		RelayerID:       r.id,
		Status:          r.status,
		Stake:           r.stake.String(),
		Reputation:      r.reputation,
		TotalTXs:        r.totalTXs,
		SuccessRate:     r.successRate,
		ActiveTXs:       activeTXs,
		SupportedChains: r.supportedChains,
		FeeRate:         r.feeRate.String(),
		Uptime:          time.Since(r.createdAt),
	}, nil
}

// GetTransaction returns a transaction by ID.
func (r *RelayerNode) GetTransaction(txHash string) (*CrossChainTx, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tx, ok := r.transactions[txHash]
	if !ok {
		return nil, fmt.Errorf("transaction not found: %s", txHash)
	}

	return tx, nil
}

// ListTransactions returns all transactions.
func (r *RelayerNode) ListTransactions() []*CrossChainTx {
	r.mu.RLock()
	defer r.mu.RUnlock()

	txs := make([]*CrossChainTx, 0, len(r.transactions))
	for _, tx := range r.transactions {
		txs = append(txs, tx)
	}

	return txs
}

// SetStatus sets the relayer status.
func (r *RelayerNode) SetStatus(status RelayerStatus) {
	if !status.IsValid() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.status = status
	r.lastActiveAt = time.Now()
}

// Start starts the relayer's background tasks.
func (r *RelayerNode) Start() {
	r.wg.Add(1)
	go r.taskProcessor()
}

// Stop stops the relayer.
func (r *RelayerNode) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// taskProcessor processes background tasks.
func (r *RelayerNode) taskProcessor() {
	defer r.wg.Done()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case task := <-r.taskQueue:
			r.processTask(task)
		case <-ticker.C:
			r.processPendingTasks()
		}
	}
}

// processTask processes a single task.
func (r *RelayerNode) processTask(task *Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch task.Type {
	case TaskVerifyTx:
		r.verifyTransaction(task)
	case TaskSubmitProof:
		r.submitTransactionProof(task)
	}
}

// verifyTransaction verifies a transaction on the source chain.
func (r *RelayerNode) verifyTransaction(task *Task) {
	r.mu.RLock()
	tx := r.transactions[task.TxHash]
	r.mu.RUnlock()

	if tx == nil {
		return
	}

	adapter, err := r.adapters.GetAdapter(tx.SourceChain)
	if err != nil {
		return
	}

	confirmations, err := adapter.GetConfirmations(task.TxHash)
	if err != nil {
		return
	}

	required := adapter.GetRequiredConfirmations()

	r.mu.Lock()
	if confirmations >= required {
		tx.Status = TxStatusConfirmed
		tx.Confirmations = confirmations
	}
	r.mu.Unlock()
}

// submitTransactionProof submits proof for a transaction.
func (r *RelayerNode) submitTransactionProof(task *Task) {
	r.mu.RLock()
	tx := r.transactions[task.TxHash]
	r.mu.RUnlock()

	if tx == nil || tx.Status != TxStatusConfirmed {
		return
	}

	adapter, err := r.adapters.GetAdapter(tx.SourceChain)
	if err != nil {
		return
	}

	proof, err := adapter.GetMerkleProof(task.TxHash)
	if err != nil {
		return
	}

	r.mu.Lock()
	tx.SetProof(proof)
	r.mu.Unlock()
}

// processPendingTasks processes all pending tasks.
func (r *RelayerNode) processPendingTasks() {
	r.mu.RLock()
	tasks := make([]*Task, 0, len(r.pendingTasks))
	for _, task := range r.pendingTasks {
		tasks = append(tasks, task)
	}
	r.mu.RUnlock()

	for _, task := range tasks {
		r.processTask(task)
	}
}

// updateStats updates the relayer statistics.
func (r *RelayerNode) updateStats(success bool) {
	r.totalTXs++
	if success {
		r.successRate = (r.successRate*float64(r.totalTXs-1) + 1.0) / float64(r.totalTXs)
	} else {
		r.successRate = (r.successRate * float64(r.totalTXs-1)) / float64(r.totalTXs)
	}
	r.reputation = r.successRate * 100
}

// ============================================================================
// Serialization Methods
// ============================================================================

// Serialize serializes the relayer node to JSON.
func (r *RelayerNode) Serialize() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type RelayerJSON struct {
		ID              string         `json:"id"`
		Address         Address        `json:"address"`
		NodeID          string         `json:"node_id"`
		Status          RelayerStatus  `json:"status"`
		Stake           string         `json:"stake"`
		SupportedChains []ChainType    `json:"supported_chains"`
		FeeRate         string         `json:"fee_rate"`
		Reputation      float64        `json:"reputation"`
		TotalTXs        uint64         `json:"total_txs"`
		SuccessRate     float64        `json:"success_rate"`
		CreatedAt       int64          `json:"created_at"`
		LastActiveAt    int64          `json:"last_active_at"`
	}

	data := RelayerJSON{
		ID:              r.id,
		Address:         r.address,
		NodeID:          r.nodeID,
		Status:          r.status,
		Stake:           r.stake.String(),
		SupportedChains: r.supportedChains,
		FeeRate:         r.feeRate.String(),
		Reputation:      r.reputation,
		TotalTXs:        r.totalTXs,
		SuccessRate:     r.successRate,
		CreatedAt:       r.createdAt.Unix(),
		LastActiveAt:    r.lastActiveAt.Unix(),
	}

	return json.Marshal(data)
}

// DeserializeRelayerNode deserializes a relayer node from JSON.
func DeserializeRelayerNode(data []byte) (*RelayerNode, error) {
	type RelayerJSON struct {
		ID              string         `json:"id"`
		Address         Address        `json:"address"`
		NodeID          string         `json:"node_id"`
		Status          RelayerStatus  `json:"status"`
		Stake           string         `json:"stake"`
		SupportedChains []ChainType    `json:"supported_chains"`
		FeeRate         string         `json:"fee_rate"`
		Reputation      float64        `json:"reputation"`
		TotalTXs        uint64         `json:"total_txs"`
		SuccessRate     float64        `json:"success_rate"`
		CreatedAt       int64          `json:"created_at"`
		LastActiveAt    int64          `json:"last_active_at"`
	}

	var jsonData RelayerJSON
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, fmt.Errorf("failed to deserialize: %w", err)
	}

	stake, ok := new(big.Int).SetString(jsonData.Stake, 10)
	if !ok {
		return nil, fmt.Errorf("invalid stake value")
	}

	feeRate, ok := new(big.Int).SetString(jsonData.FeeRate, 10)
	if !ok {
		return nil, fmt.Errorf("invalid fee rate value")
	}

	relayer := NewRelayerNode(
		jsonData.ID,
		jsonData.Address,
		jsonData.NodeID,
		jsonData.SupportedChains,
		stake,
		feeRate,
	)

	relayer.status = jsonData.Status
	relayer.reputation = jsonData.Reputation
	relayer.totalTXs = jsonData.TotalTXs
	relayer.successRate = jsonData.SuccessRate
	relayer.createdAt = time.Unix(jsonData.CreatedAt, 0)
	relayer.lastActiveAt = time.Unix(jsonData.LastActiveAt, 0)

	return relayer, nil
}

// ============================================================================
// Factory Functions
// ============================================================================

// CreateRelayer creates a new relayer with the given parameters.
func CreateRelayer(
	nodeID string,
	addr Address,
	chains []ChainType,
	stake *big.Int,
	feeRate *big.Int,
) (*RelayerNode, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("node ID is required")
	}

	if len(chains) == 0 {
		return nil, fmt.Errorf("at least one supported chain is required")
	}

	if stake == nil || stake.Sign() <= 0 {
		return nil, fmt.Errorf("invalid stake amount")
	}

	if feeRate == nil || feeRate.Sign() <= 0 {
		return nil, fmt.Errorf("invalid fee rate")
	}

	// Generate relayer ID from node ID
	relayerID := GenerateRelayerID([]byte(nodeID))

	return NewRelayerNode(relayerID, addr, nodeID, chains, stake, feeRate), nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// NewAddressFromHex creates an Address from hex string.
func NewAddressFromHex(chain ChainType, hexStr string) (*Address, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex: %w", err)
	}

	return &Address{
		Chain: chain,
		Data:  data,
	}, nil
}

// GenerateNodeID generates a unique node ID.
func GenerateNodeID() string {
	data := fmt.Sprintf("node-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}

// CalculateTotalFee calculates the total fee for a transaction.
func CalculateTotalFee(amount, feeRate *big.Int) *big.Int {
	return CalculateFee(amount, feeRate)
}

// EstimateCompletionTime estimates the time to complete a cross-chain swap.
func EstimateCompletionTime(sourceChain ChainType) time.Duration {
	switch sourceChain {
	case ChainBTC:
		return time.Minute * 60 // ~1 hour for BTC
	case ChainETH:
		return time.Minute * 15 // ~15 minutes for ETH
	case ChainSOL:
		return time.Minute * 5 // ~5 minutes for Solana
	default:
		return time.Hour
	}
}

// ============================================================================
// Validation Functions
// ============================================================================

// ValidateRelayer validates a relayer configuration.
func ValidateRelayer(relayer *RelayerNode) error {
	if relayer == nil {
		return fmt.Errorf("relayer is nil")
	}

	if relayer.id == "" {
		return fmt.Errorf("relayer ID is empty")
	}

	if len(relayer.supportedChains) == 0 {
		return fmt.Errorf("no supported chains")
	}

	if relayer.stake == nil || relayer.stake.Sign() <= 0 {
		return fmt.Errorf("invalid stake")
	}

	if relayer.feeRate == nil || relayer.feeRate.Sign() <= 0 {
		return fmt.Errorf("invalid fee rate")
	}

	return nil
}

// CanRelay checks if a relayer can process a specific swap.
func CanRelay(relayer *RelayerNode, sourceChain, destChain ChainType) bool {
	if relayer.status != StatusActive {
		return false
	}

	hasSource := false
	hasDest := false

	for _, chain := range relayer.supportedChains {
		if chain == sourceChain {
			hasSource = true
		}
		if chain == destChain {
			hasDest = true
		}
	}

	return hasSource && hasDest
}

// SelectBestRelayer selects the best relayer from a list based on reputation and fee.
func SelectBestRelayers(relayers []*RelayerNode, sourceChain, destChain ChainType, count int) []*RelayerNode {
	// Filter capable relayers
	var capable []*RelayerNode
	for _, r := range relayers {
		if CanRelay(r, sourceChain, destChain) && r.reputation >= MinRelayerReputation {
			capable = append(capable, r)
		}
	}

	// Sort by reputation (descending) and fee rate (ascending)
	for i := 0; i < len(capable)-1; i++ {
		for j := i + 1; j < len(capable); j++ {
			if capable[i].reputation < capable[j].reputation ||
				(capable[i].reputation == capable[j].reputation &&
					capable[i].feeRate.Cmp(capable[j].feeRate) > 0) {
				capable[i], capable[j] = capable[j], capable[i]
			}
		}
	}

	// Return top N
	if count > len(capable) {
		count = len(capable)
	}

	return capable[:count]
}
