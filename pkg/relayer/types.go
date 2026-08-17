// Package relayer implements cross-chain relayer functionality for AIB protocol.
// It provides mechanisms for bridging assets between different blockchain networks
// including Bitcoin, Ethereum, Solana, and AIB.
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
// Chain Types
// ============================================================================

// ChainType represents the type of blockchain network.
type ChainType string

const (
	ChainBTC ChainType = "BTC" // Bitcoin
	ChainETH ChainType = "ETH" // Ethereum
	ChainSOL ChainType = "SOL" // Solana
	ChainAIB ChainType = "AIB" // AIB native chain
)

// String returns the string representation of ChainType.
func (c ChainType) String() string {
	return string(c)
}

// ============================================================================
// Relayer Status
// ============================================================================

// RelayerStatus represents the operational status of a relayer.
type RelayerStatus string

const (
	StatusActive   RelayerStatus = "Active"   // Fully operational
	StatusInactive RelayerStatus = "Inactive" // Temporarily inactive
	StatusSlashed  RelayerStatus = "Slashed"  // Punished for misbehavior
)

// String returns the string representation of RelayerStatus.
func (s RelayerStatus) String() string {
	return string(s)
}

// IsValid checks if the status is valid.
func (s RelayerStatus) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusSlashed:
		return true
	default:
		return false
	}
}

// ============================================================================
// Transaction Status
// ============================================================================

// TxStatus represents the status of a cross-chain transaction.
type TxStatus string

const (
	TxStatusPending    TxStatus = "Pending"    // Waiting for confirmation
	TxStatusLocked     TxStatus = "Locked"     // Funds locked on source chain
	TxStatusConfirmed  TxStatus = "Confirmed"  // Source chain confirmed
	TxStatusProofReady TxStatus = "ProofReady" // Merkle proof ready
	TxStatusCompleted  TxStatus = "Completed"  // Completed successfully
	TxStatusFailed     TxStatus = "Failed"     // Transaction failed
	TxStatusDisputed   TxStatus = "Disputed"   // Under dispute
)

// String returns the string representation of TxStatus.
func (s TxStatus) String() string {
	return string(s)
}

// ============================================================================
// Address and Hash Types
// ============================================================================

// Address represents a blockchain address (variable length based on chain).
type Address struct {
	Chain ChainType
	Data  []byte
}

// NewAddress creates a new Address with the given chain and data.
func NewAddress(chain ChainType, data []byte) *Address {
	return &Address{
		Chain: chain,
		Data:  make([]byte, len(data)),
	}
}

// String returns the hex string representation of the address.
func (a *Address) String() string {
	return hex.EncodeToString(a.Data)
}

// Bytes returns the raw bytes of the address.
func (a *Address) Bytes() []byte {
	return a.Data
}

// Hash represents a blockchain transaction or block hash.
type Hash struct {
	Chain ChainType
	Data  [32]byte
}

// NewHash creates a new Hash from bytes.
func NewHash(chain ChainType, data []byte) *Hash {
	h := &Hash{Chain: chain}
	if len(data) > 32 {
		data = data[:32]
	}
	copy(h.Data[32-len(data):], data)
	return h
}

// String returns the hex string representation of the hash.
func (h *Hash) String() string {
	return hex.EncodeToString(h.Data[:])
}

// Bytes returns the raw bytes of the hash.
func (h *Hash) Bytes() []byte {
	return h.Data[:]
}

// ============================================================================
// Relayer Structure
// ============================================================================

// Relayer represents a cross-chain relayer node.
type Relayer struct {
	ID              string            // Unique identifier (node public key hash)
	Address         Address           // Relayer address on AIB chain
	NodeID          string            // P2P node ID
	SupportedChains []ChainType       // List of chains this relayer supports
	Stake           *big.Int          // Staked amount (in smallest unit)
	Status          RelayerStatus     // Current operational status
	Reputation      float64           // Reputation score (0-100)
	TotalTXs        uint64            // Total transactions processed
	SuccessRate     float64           // Success rate (0-1)
	FeeRate         *big.Int          // Fee rate (per transaction)
	CreatedAt       time.Time         // Registration timestamp
	LastActiveAt    time.Time         // Last active timestamp
	Metadata        map[string]string // Additional metadata
	mu              sync.RWMutex      // Mutex for concurrent access
}

// NewRelayer creates a new Relayer instance.
func NewRelayer(id string, addr Address, nodeID string, chains []ChainType, stake *big.Int) *Relayer {
	now := time.Now()
	return &Relayer{
		ID:              id,
		Address:         addr,
		NodeID:          nodeID,
		SupportedChains: chains,
		Stake:           stake,
		Status:          StatusActive,
		Reputation:      100.0,
		TotalTXs:        0,
		SuccessRate:     1.0,
		FeeRate:         big.NewInt(1000), // Default fee: 1000 satoshi equivalent
		CreatedAt:       now,
		LastActiveAt:    now,
		Metadata:        make(map[string]string),
	}
}

// GetStatus returns the current status of the relayer.
func (r *Relayer) GetStatus() RelayerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Status
}

// SetStatus sets the status of the relayer.
func (r *Relayer) SetStatus(status RelayerStatus) {
	if !status.IsValid() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = status
	r.LastActiveAt = time.Now()
}

// UpdateStats updates the relayer statistics.
func (r *Relayer) UpdateStats(success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.TotalTXs++
	r.LastActiveAt = time.Now()

	if success {
		// Incrementally update success rate
		r.SuccessRate = (r.SuccessRate*float64(r.TotalTXs-1) + 1.0) / float64(r.TotalTXs)
	} else {
		r.SuccessRate = (r.SuccessRate * float64(r.TotalTXs-1)) / float64(r.TotalTXs)
	}

	// Update reputation based on success rate
	r.Reputation = r.SuccessRate * 100
}

// SupportsChain checks if the relayer supports a specific chain.
func (r *Relayer) SupportsChain(chain ChainType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.SupportedChains {
		if c == chain {
			return true
		}
	}
	return false
}

// CanProcess checks if the relayer can process a transaction for the given chains.
func (r *Relayer) CanProcess(sourceChain, destChain ChainType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Status != StatusActive {
		return false
	}

	hasSource := false
	hasDest := false

	for _, c := range r.SupportedChains {
		if c == sourceChain {
			hasSource = true
		}
		if c == destChain {
			hasDest = true
		}
	}

	return hasSource && hasDest
}

// Serialize serializes the relayer to JSON.
func (r *Relayer) Serialize() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return json.Marshal(r)
}

// DeserializeRelayer deserializes a relayer from JSON.
func DeserializeRelayer(data []byte) (*Relayer, error) {
	r := &Relayer{}
	err := json.Unmarshal(data, r)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize relayer: %w", err)
	}
	r.mu = sync.RWMutex{}
	return r, nil
}

// ============================================================================
// Cross-Chain Transaction
// ============================================================================

// CrossChainTx represents a cross-chain transaction.
type CrossChainTx struct {
	ID            string       // Unique transaction ID
	SourceChain   ChainType    // Source blockchain
	DestChain     ChainType    // Destination blockchain
	SourceTxHash  Hash         // Transaction hash on source chain
	DestTxHash    Hash         // Transaction hash on destination chain
	Sender        Address      // Sender address on source chain
	Recipient     Address      // Recipient address on destination chain
	Amount        *big.Int     // Amount to transfer
	Token         string       // Token symbol (e.g., "BTC", "ETH")
	Status        TxStatus     // Current transaction status
	CreatedAt     time.Time    // Creation timestamp
	UpdatedAt     time.Time    // Last update timestamp
	Confirmations uint64       // Number of confirmations
	Proof         *MerkleProof // Merkle proof for verification
	RelayerID     string       // Relayer processing this transaction
	Fee           *big.Int     // Relayer fee
	Expiry        time.Time    // Expiration time
	mu            sync.RWMutex // Mutex for concurrent access
}

// NewCrossChainTx creates a new cross-chain transaction.
func NewCrossChainTx(
	id string,
	sourceChain, destChain ChainType,
	sender, recipient Address,
	amount *big.Int,
	token string,
	relayerID string,
	expiry time.Duration,
) *CrossChainTx {
	now := time.Now()
	return &CrossChainTx{
		ID:          id,
		SourceChain: sourceChain,
		DestChain:   destChain,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      amount,
		Token:       token,
		Status:      TxStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		RelayerID:   relayerID,
		Fee:         big.NewInt(0),
		Expiry:      now.Add(expiry),
	}
}

// GetStatus returns the current status of the transaction.
func (tx *CrossChainTx) GetStatus() TxStatus {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return tx.Status
}

// SetStatus sets the status of the transaction.
func (tx *CrossChainTx) SetStatus(status TxStatus) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.Status = status
	tx.UpdatedAt = time.Now()
}

// UpdateConfirmations updates the confirmation count.
func (tx *CrossChainTx) UpdateConfirmations(conf uint64) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.Confirmations = conf
	tx.UpdatedAt = time.Now()
}

// SetProof sets the merkle proof for the transaction.
func (tx *CrossChainTx) SetProof(proof *MerkleProof) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.Proof = proof
	tx.Status = TxStatusProofReady
	tx.UpdatedAt = time.Now()
}

// IsExpired checks if the transaction has expired.
func (tx *CrossChainTx) IsExpired() bool {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return time.Now().After(tx.Expiry)
}

// Serialize serializes the transaction to JSON.
func (tx *CrossChainTx) Serialize() ([]byte, error) {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return json.Marshal(tx)
}

// DeserializeCrossChainTx deserializes a transaction from JSON.
func DeserializeCrossChainTx(data []byte) (*CrossChainTx, error) {
	tx := &CrossChainTx{}
	err := json.Unmarshal(data, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize cross-chain tx: %w", err)
	}
	tx.mu = sync.RWMutex{}
	return tx, nil
}

// ============================================================================
// Merkle Proof
// ============================================================================

// MerkleProof represents a merkle proof for cross-chain verification.
type MerkleProof struct {
	TxHash      Hash      // Transaction hash
	BlockHash   Hash      // Block containing the transaction
	BlockNumber uint64    // Block number
	Index       uint64    // Transaction index in block
	Proof       [][]byte  // Merkle proof path
	Chain       ChainType // Source chain
}

// NewMerkleProof creates a new MerkleProof.
func NewMerkleProof(txHash, blockHash Hash, blockNumber, index uint64, proof [][]byte, chain ChainType) *MerkleProof {
	return &MerkleProof{
		TxHash:      txHash,
		BlockHash:   blockHash,
		BlockNumber: blockNumber,
		Index:       index,
		Proof:       proof,
		Chain:       chain,
	}
}

// Verify verifies the merkle proof.
func (p *MerkleProof) Verify(merkleRoot Hash) bool {
	if p == nil || len(p.Proof) == 0 {
		return false
	}

	// Start with transaction hash
	current := p.TxHash.Data[:]

	// Iterate through proof path
	for _, node := range p.Proof {
		// Concatenate current hash with proof node
		combined := make([]byte, len(current)+len(node))
		copy(combined, current)
		copy(combined[len(current):], node)

		// Hash the combination
		hash := sha256.Sum256(combined)
		current = hash[:]
	}

	// Compare final hash with merkle root
	if len(current) != 32 {
		return false
	}
	for i := 0; i < 32; i++ {
		if current[i] != merkleRoot.Data[i] {
			return false
		}
	}
	return true
}

// Serialize serializes the proof to JSON.
func (p *MerkleProof) Serialize() ([]byte, error) {
	return json.Marshal(p)
}

// DeserializeMerkleProof deserializes a proof from JSON.
func DeserializeMerkleProof(data []byte) (*MerkleProof, error) {
	p := &MerkleProof{}
	err := json.Unmarshal(data, p)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize merkle proof: %w", err)
	}
	return p, nil
}

// ============================================================================
// Swap Request
// ============================================================================

// SwapRequest represents a request to swap tokens across chains.
type SwapRequest struct {
	ID          string    // Unique request ID
	SourceChain ChainType // Source blockchain
	DestChain   ChainType // Destination blockchain
	Sender      Address   // Sender address on source chain
	Recipient   Address   // Recipient address on destination chain
	Amount      *big.Int  // Amount to swap
	Token       string    // Token symbol
	RelayerFee  *big.Int  // Maximum relayer fee willing to pay
	Deadline    time.Time // Swap deadline
	SecretHash  []byte    // Hash of secret for HTLC
	CreatedAt   time.Time // Creation timestamp
}

// NewSwapRequest creates a new SwapRequest.
func NewSwapRequest(
	id string,
	sourceChain, destChain ChainType,
	sender, recipient Address,
	amount *big.Int,
	token string,
	relayerFee *big.Int,
	deadline time.Duration,
	secretHash []byte,
) *SwapRequest {
	return &SwapRequest{
		ID:          id,
		SourceChain: sourceChain,
		DestChain:   destChain,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      amount,
		Token:       token,
		RelayerFee:  relayerFee,
		Deadline:    time.Now().Add(deadline),
		SecretHash:  secretHash,
		CreatedAt:   time.Now(),
	}
}

// IsExpired checks if the swap request has expired.
func (r *SwapRequest) IsExpired() bool {
	return time.Now().After(r.Deadline)
}

// ToCrossChainTx converts the swap request to a cross-chain transaction.
func (r *SwapRequest) ToCrossChainTx(relayerID string) *CrossChainTx {
	return NewCrossChainTx(
		r.ID,
		r.SourceChain,
		r.DestChain,
		r.Sender,
		r.Recipient,
		r.Amount,
		r.Token,
		relayerID,
		time.Until(r.Deadline),
	)
}

// ============================================================================
// Register Request
// ============================================================================

// RegisterRequest represents a request to register a relayer.
type RegisterRequest struct {
	NodeID          string            // P2P node ID
	Address         Address           // Relayer address on AIB chain
	SupportedChains []ChainType       // List of supported chains
	Stake           *big.Int          // Staked amount
	FeeRate         *big.Int          // Fee rate
	Metadata        map[string]string // Additional metadata
}

// NewRegisterRequest creates a new RegisterRequest.
func NewRegisterRequest(
	nodeID string,
	addr Address,
	chains []ChainType,
	stake *big.Int,
	feeRate *big.Int,
) *RegisterRequest {
	return &RegisterRequest{
		NodeID:          nodeID,
		Address:         addr,
		SupportedChains: chains,
		Stake:           stake,
		FeeRate:         feeRate,
		Metadata:        make(map[string]string),
	}
}

// ============================================================================
// Dispute Types
// ============================================================================

// Dispute represents a dispute in the relayer network.
type Dispute struct {
	ID         string     // Unique dispute ID
	TxHash     string     // Transaction ID under dispute
	Reporter   string     // Who reported the dispute
	Reason     string     // Reason for dispute
	Evidence   []byte     // Evidence data
	Status     string     // Dispute status
	CreatedAt  time.Time  // Creation timestamp
	ResolvedAt *time.Time // Resolution timestamp
}

// DisputeResolution represents the resolution of a dispute.
type DisputeResolution struct {
	DisputeID  string   // Dispute ID
	Winner     string   // Winner (relayer ID)
	Loser      string   // Loser (relayer ID)
	Resolution string   // Resolution description
	Penalty    *big.Int // Penalty amount
}

// ============================================================================
// Constants
// ============================================================================

const (
	// DefaultRelayerStake is the default minimum stake required (in satoshi units)
	DefaultRelayerStake = 100000000 // 1 BTC equivalent

	// DefaultConfirmationBlocks is the default number of confirmations required
	DefaultConfirmationBlocks = 6

	// MaxProofDepth is the maximum depth of merkle proof
	MaxProofDepth = 64

	// DefaultTxExpiry is the default transaction expiry time
	DefaultTxExpiry = time.Hour * 24

	// MinRelayerReputation is the minimum reputation required to process transactions
	MinRelayerReputation = 50.0

	// SlashingRatio is the ratio of stake to be slashed for misbehavior
	SlashingRatio = 0.5 // 50%
)

// ============================================================================
// Utility Functions
// ============================================================================

// GenerateTxID generates a unique transaction ID.
func GenerateTxID(sourceChain, destChain ChainType, sender Address, timestamp time.Time) string {
	data := fmt.Sprintf("%s-%s-%s-%d", sourceChain, destChain, sender.String(), timestamp.UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GenerateRelayerID generates a unique relayer ID from a public key.
func GenerateRelayerID(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	return hex.EncodeToString(hash[:16])
}

// CalculateFee calculates the relayer fee based on amount and fee rate.
func CalculateFee(amount, feeRate *big.Int) *big.Int {
	// Fee = amount * feeRate / 100000000
	fee := new(big.Int).Mul(amount, feeRate)
	fee.Div(fee, big.NewInt(100000000))
	return fee
}

// ValidateAddress validates an address for a given chain.
func ValidateAddress(addr Address) error {
	if len(addr.Data) == 0 {
		return fmt.Errorf("address data is empty")
	}

	switch addr.Chain {
	case ChainBTC:
		// Bitcoin addresses are 26-35 bytes (P2PKH) or 62 bytes (P2SH)
		if len(addr.Data) < 26 || len(addr.Data) > 35 {
			if len(addr.Data) != 62 {
				return fmt.Errorf("invalid Bitcoin address length: %d", len(addr.Data))
			}
		}
	case ChainETH:
		// Ethereum addresses are 20 bytes
		if len(addr.Data) != 20 {
			return fmt.Errorf("invalid Ethereum address length: %d", len(addr.Data))
		}
	case ChainSOL:
		// Solana addresses are 32 bytes (public keys)
		if len(addr.Data) != 32 {
			return fmt.Errorf("invalid Solana address length: %d", len(addr.Data))
		}
	case ChainAIB:
		// AIB addresses are 32 bytes
		if len(addr.Data) != 32 {
			return fmt.Errorf("invalid AIB address length: %d", len(addr.Data))
		}
	default:
		return fmt.Errorf("unknown chain type: %s", addr.Chain)
	}

	return nil
}
