// Package migration implements token migration from AIB1 snapshot and cross-chain bridging.
// It provides AIB1 snapshot mapping, BTC/ETH/SOL cross-chain migration with vesting schedules.
package migration

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Errors
// ============================================================================

var (
	ErrAlreadyClaimed         = errors.New("already claimed")
	ErrClaimExpired            = errors.New("claim deadline expired")
	ErrInvalidSignature        = errors.New("invalid signature")
	ErrSnapshotNotFound        = errors.New("snapshot not found")
	ErrAmountExceedsBalance   = errors.New("amount exceeds snapshot balance")
	ErrMigrationWindowClosed   = errors.New("migration window closed")
	ErrInvalidChain           = errors.New("invalid chain type")
	ErrNoLockedRewards        = errors.New("no locked rewards found")
	ErrNothingToClaim         = errors.New("nothing to claim")
	ErrInvalidProof           = errors.New("invalid merkle proof")
)

// ============================================================================
// AIB1 Migration
// ============================================================================

// SnapshotRecord represents an account record in the snapshot.
type SnapshotRecord struct {
	Address interfaces.Address
	Balance uint64
}

// AIB1Migration handles AIB1 snapshot-to-AIB2 migration.
// Users can claim their AIB2 tokens by providing Ed25519 signature
// proving ownership of the AIB1 private key.
type AIB1Migration struct {
	mu sync.RWMutex

	snapshotRoot   [32]byte
	snapshotTime   time.Time
	claimDeadline  time.Time
	claims         map[interfaces.Address]bool
	snapshotData   map[interfaces.Address]uint64
	totalMigrated  uint64
	hasher         crypto.Hasher

	// now returns the current time; overridable for tests.
	now func() time.Time
}

// AIB1Config holds configuration for AIB1 migration.
type AIB1Config struct {
	SnapshotRoot  [32]byte
	SnapshotTime  time.Time
	ClaimDeadline time.Time
}

// NewAIB1Migration creates a new AIB1 migration contract.
func NewAIB1Migration(cfg *AIB1Config) *AIB1Migration {
	return &AIB1Migration{
		snapshotRoot:  cfg.SnapshotRoot,
		snapshotTime:  cfg.SnapshotTime,
		claimDeadline: cfg.ClaimDeadline,
		claims:        make(map[interfaces.Address]bool),
		snapshotData:  make(map[interfaces.Address]uint64),
		hasher:        crypto.NewSHA256d(),
		now:           time.Now,
	}
}

// LoadSnapshot loads snapshot data from a list of records.
// In production, this would load from a verified Merkle tree.
func (m *AIB1Migration) LoadSnapshot(records []SnapshotRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, record := range records {
		m.snapshotData[record.Address] = record.Balance
	}

	return nil
}

// GetSnapshotBalance returns the balance for an address in the snapshot.
func (m *AIB1Migration) GetSnapshotBalance(addr interfaces.Address) (uint64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	balance, exists := m.snapshotData[addr]
	return balance, exists
}

// IsClaimed checks if an address has already claimed.
func (m *AIB1Migration) IsClaimed(addr interfaces.Address) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.claims[addr]
}

// HasClaimedWithAmount checks if an address has claimed and returns the amount.
func (m *AIB1Migration) HasClaimedWithAmount(addr interfaces.Address) (bool, uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.claims[addr] {
		return false, 0
	}

	// Return the claimed amount from snapshot
	balance := m.snapshotData[addr]
	return true, balance
}

// ClaimData represents the data signed for AIB1 claim.
type ClaimData struct {
	Address interfaces.Address
	Amount  uint64
	Nonce   uint64
}

// SerializeClaimData serializes claim data for signing.
func SerializeClaimData(data *ClaimData) []byte {
	buf := make([]byte, 0, 80)
	buf = append(buf, data.Address[:]...)
	buf = binary.BigEndian.AppendUint64(buf, data.Amount)
	buf = binary.BigEndian.AppendUint64(buf, data.Nonce)
	return buf
}

// VerifySignature verifies an Ed25519 signature for claim data.
func VerifySignature(pubKey []byte, data *ClaimData, signature []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}

	message := SerializeClaimData(data)
	return ed25519.Verify(ed25519.PublicKey(pubKey), message, signature)
}

// VerifyMerkleProof verifies a Merkle proof for snapshot inclusion.
func (m *AIB1Migration) VerifyMerkleProof(addr interfaces.Address, balance uint64, proof [][]byte) bool {
	// Serialize the leaf: address + balance
	leaf := make([]byte, 40)
	copy(leaf[:32], addr[:])
	binary.BigEndian.PutUint64(leaf[32:], balance)

	return crypto.VerifyMerkleProof(m.hasher, leaf, m.snapshotRoot[:], proof, 0)
}

// Claim allows a user to claim their AIB2 tokens.
// The user provides:
// - targetAddr: the AIB2 address to receive tokens
// - amount: the amount to claim (must match snapshot balance)
// - pubKey: the Ed25519 public key from AIB1 (32 bytes)
// - signature: Ed25519 signature proving ownership of the private key
// - nonce: a unique nonce to prevent replay attacks
func (m *AIB1Migration) Claim(targetAddr interfaces.Address, amount uint64, pubKey []byte, signature []byte, nonce uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if claim deadline has passed
	if m.now().After(m.claimDeadline) {
		return ErrClaimExpired
	}

	// Derive address from public key
	var addr interfaces.Address
	if len(pubKey) >= 32 {
		copy(addr[:], pubKey[:32])
	}

	// Check if already claimed
	if m.claims[addr] {
		return ErrAlreadyClaimed
	}

	// Get snapshot balance
	balance, exists := m.snapshotData[addr]
	if !exists {
		return ErrSnapshotNotFound
	}

	// Verify amount matches snapshot
	if amount != balance {
		return ErrAmountExceedsBalance
	}

	// Verify signature
	claimData := &ClaimData{
		Address: addr,
		Amount:  amount,
		Nonce:   nonce,
	}
	if !VerifySignature(pubKey, claimData, signature) {
		return ErrInvalidSignature
	}

	// Record the claim
	m.claims[addr] = true
	m.totalMigrated += amount

	return nil
}

// ClaimWithMerkle allows claiming with Merkle proof instead of signature.
func (m *AIB1Migration) ClaimWithMerkle(targetAddr interfaces.Address, amount uint64, proof [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if claim deadline has passed
	if m.now().After(m.claimDeadline) {
		return ErrClaimExpired
	}

	// Derive address from target address
	addr := targetAddr

	// Check if already claimed
	if m.claims[addr] {
		return ErrAlreadyClaimed
	}

	// Get snapshot balance
	balance, exists := m.snapshotData[addr]
	if !exists {
		return ErrSnapshotNotFound
	}

	// Verify amount matches snapshot
	if amount != balance {
		return ErrAmountExceedsBalance
	}

	// Verify Merkle proof
	if !m.VerifyMerkleProof(addr, balance, proof) {
		return ErrInvalidProof
	}

	// Record the claim
	m.claims[addr] = true
	m.totalMigrated += amount

	return nil
}

// GetTotalMigrated returns the total amount of tokens migrated.
func (m *AIB1Migration) GetTotalMigrated() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalMigrated
}

// GetSnapshotRoot returns the snapshot Merkle root.
func (m *AIB1Migration) GetSnapshotRoot() [32]byte {
	return m.snapshotRoot
}

// GetClaimDeadline returns the claim deadline.
func (m *AIB1Migration) GetClaimDeadline() time.Time {
	return m.claimDeadline
}

// IsClaimWindowOpen checks if the claim window is still open.
func (m *AIB1Migration) IsClaimWindowOpen() bool {
	return m.now().Before(m.claimDeadline)
}

// SetClock overrides the claim-clock. Intended for tests.
func (m *AIB1Migration) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	m.now = now
}

// ============================================================================
// AIB1 Migration with Merkle Tree Builder
// ============================================================================

// SnapshotMerkleTree builds and manages the snapshot Merkle tree.
type SnapshotMerkleTree struct {
	tree   *crypto.StandardMerkleTree
	hasher crypto.Hasher
	leaves map[string]struct {
		balance uint64
		index   int
	}
}

// NewSnapshotMerkleTree creates a new snapshot Merkle tree.
func NewSnapshotMerkleTree(records []SnapshotRecord) (*SnapshotMerkleTree, error) {
	hasher := crypto.NewSHA256d()

	// Create leaves: address + balance
	leaves := make([][]byte, len(records))
	leafMap := make(map[string]struct {
		balance uint64
		index   int
	})

	for i, record := range records {
		leaf := make([]byte, 40)
		copy(leaf[:32], record.Address[:])
		binary.BigEndian.PutUint64(leaf[32:], record.Balance)
		leaves[i] = leaf
		leafMap[string(record.Address[:])] = struct {
			balance uint64
			index   int
		}{record.Balance, i}
	}

	tree, err := crypto.NewStandardMerkleTree(hasher, leaves)
	if err != nil {
		return nil, fmt.Errorf("failed to build Merkle tree: %w", err)
	}

	return &SnapshotMerkleTree{
		tree:   tree,
		hasher: hasher,
		leaves: leafMap,
	}, nil
}

// Root returns the Merkle root.
func (s *SnapshotMerkleTree) Root() []byte {
	return s.tree.Root()
}

// GetProof returns the Merkle proof for an address.
func (s *SnapshotMerkleTree) GetProof(addr interfaces.Address) ([][]byte, bool) {
	leafData, exists := s.leaves[string(addr[:])]
	if !exists {
		return nil, false
	}
	proof, err := s.tree.Proof(leafData.index)
	if err != nil {
		return nil, false
	}
	return proof, true
}

// GetBalance returns the balance for an address.
func (s *SnapshotMerkleTree) GetBalance(addr interfaces.Address) (uint64, bool) {
	leafData, exists := s.leaves[string(addr[:])]
	if !exists {
		return 0, false
	}
	return leafData.balance, true
}
