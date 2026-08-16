// Package founder implements the founder allocation system for AIB 2.0.
// Manages founder vesting, verification, and distribution.
package founder

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

const (
	// MaxFounders is the maximum number of founders allowed.
	MaxFounders = 10

	// TotalFounderAllocation is the total AIB allocated to founders (31415).
	TotalFounderAllocation = 31415

	// AllocationPerFounder is the AIB allocation per founder (3141).
	AllocationPerFounder = 3141

	// FounderLockPeriod is the initial lock period (1 year).
	FounderLockPeriod = 365 * 24 * time.Hour

	// VestingMonths is the number of months for vesting after lock period.
	VestingMonths = 12
)

// FounderStatus represents the current status of a founder's allocation.
type FounderStatus string

const (
	StatusLocked    FounderStatus = "locked"     // Initial lock period
	StatusVesting   FounderStatus = "vesting"    // Vesting in progress
	StatusCompleted FounderStatus = "completed"  // All tokens vested
	StatusClaimed   FounderStatus = "claimed"    // All tokens claimed
)

// Founder represents a founder with their allocation details.
type Founder struct {
	ID           string        `json:"id"`            // Unique identifier
	Name         string        `json:"name"`          // Display name
	Address      string        `json:"address"`       // AIB address (Bech32m)
	AddressBytes [32]byte      `json:"-"`             // Raw address bytes
	PublicKey    string        `json:"public_key"`    // Hex encoded public key
	PubKeyBytes  []byte        `json:"-"`             // Raw public key bytes
	TotalAmount  uint64        `json:"total_amount"`  // Total allocation (3141 AIB)
	Claimed      uint64        `json:"claimed"`       // Amount already claimed
	Status       FounderStatus `json:"status"`        // Current status
	StartTime    time.Time     `json:"start_time"`    // Vesting start time
	UnlockTime   time.Time     `json:"unlock_time"`   // End of lock period
	EndTime      time.Time     `json:"end_time"`      // Full vesting completion
	Metadata     FounderMetadata `json:"metadata"`    // Additional metadata
}

// FounderMetadata holds additional information about a founder.
type FounderMetadata struct {
	Description string    `json:"description,omitempty"`
	Role        string    `json:"role,omitempty"`        // Role in the project
	Social      string    `json:"social,omitempty"`      // Social media/handle
	JoinedAt    time.Time `json:"joined_at,omitempty"`
}

// VestingInfo contains current vesting information for a founder.
type VestingInfo struct {
	FounderID      string        `json:"founder_id"`
	TotalAmount    uint64        `json:"total_amount"`
	ClaimedAmount  uint64        `json:"claimed_amount"`
	VestedAmount   uint64        `json:"vested_amount"`     // Amount currently vested
	ClaimableAmount uint64       `json:"claimable_amount"`   // Amount available to claim
	LockedAmount   uint64        `json:"locked_amount"`      // Amount still locked
	Status         FounderStatus `json:"status"`
	Progress       float64       `json:"progress"`           // Vesting progress (0-1)
	NextUnlock     time.Time     `json:"next_unlock"`        // Next vesting unlock time
	UnlockAmount   uint64        `json:"unlock_amount"`      // Amount to unlock next
	Schedule       []VestingEntry `json:"schedule"`         // Full vesting schedule
}

// VestingEntry represents a single vesting schedule entry.
type VestingEntry struct {
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Amount    uint64    `json:"amount"`
	Claimed   bool      `json:"claimed"`
}

// FounderList represents a list of founders with validation.
type FounderList struct {
	Founders []*Founder `json:"founders"`
	Version  uint64     `json:"version"`       // Config version for updates
	mu       sync.RWMutex
}

// NewFounderList creates a new empty founder list.
func NewFounderList() *FounderList {
	return &FounderList{
		Founders: make([]*Founder, 0, MaxFounders),
		Version:  1,
	}
}

// Add adds a founder to the list with validation.
func (fl *FounderList) Add(f *Founder) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if len(fl.Founders) >= MaxFounders {
		return fmt.Errorf("maximum founders (%d) reached", MaxFounders)
	}

	// If AddressBytes is already set, skip address decoding
	// This allows loading from config where we derive address from public key
	if f.AddressBytes == [32]byte{} {
		// Validate address
		_, addrBytes, err := utxo.DecodeBech32m(f.Address)
		if err != nil {
			return fmt.Errorf("invalid address: %w", err)
		}
		copy(f.AddressBytes[:], addrBytes)
	}

	// If PubKeyBytes is not set, decode from hex
	if len(f.PubKeyBytes) == 0 {
		pubKey, err := hex.DecodeString(f.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key hex: %w", err)
		}
		if len(pubKey) != 32 {
			return fmt.Errorf("invalid public key length: expected 32, got %d", len(pubKey))
		}
		f.PubKeyBytes = pubKey
	}

	// Verify address matches public key (if both are set)
	if f.AddressBytes != [32]byte{} && len(f.PubKeyBytes) == 32 {
		expectedAddr := utxo.AddressFromPublicKey(f.PubKeyBytes)
		if expectedAddr != f.AddressBytes {
			return fmt.Errorf("address does not match public key")
		}
	}

	// Check for duplicate
	for _, existing := range fl.Founders {
		if existing.ID == f.ID {
			return fmt.Errorf("founder ID %s already exists", f.ID)
		}
		if existing.Address == f.Address {
			return fmt.Errorf("address %s already registered", f.Address)
		}
	}

	// Set default values only if not already set
	if f.TotalAmount == 0 {
		f.TotalAmount = AllocationPerFounder
	}
	if f.Status == "" {
		f.Status = StatusLocked
	}
	if f.UnlockTime.IsZero() {
		f.UnlockTime = f.StartTime.Add(FounderLockPeriod)
	}
	if f.EndTime.IsZero() {
		f.EndTime = f.UnlockTime.Add(time.Duration(VestingMonths) * 30 * 24 * time.Hour)
	}

	fl.Founders = append(fl.Founders, f)
	return nil
}

// Get retrieves a founder by ID.
func (fl *FounderList) Get(id string) (*Founder, bool) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	for _, f := range fl.Founders {
		if f.ID == id {
			return f, true
		}
	}
	return nil, false
}

// GetByAddress retrieves a founder by address.
func (fl *FounderList) GetByAddress(address string) (*Founder, bool) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	for _, f := range fl.Founders {
		if f.Address == address {
			return f, true
		}
	}
	return nil, false
}

// List returns all founders.
func (fl *FounderList) List() []*Founder {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	result := make([]*Founder, len(fl.Founders))
	copy(result, fl.Founders)
	return result
}

// Count returns the number of founders.
func (fl *FounderList) Count() int {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	return len(fl.Founders)
}

// TotalAllocated returns the total amount allocated to all founders.
func (fl *FounderList) TotalAllocated() uint64 {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	return uint64(len(fl.Founders)) * AllocationPerFounder
}

// Validate checks if the founder list is valid.
func (fl *FounderList) Validate() error {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	if len(fl.Founders) == 0 {
		return fmt.Errorf("founder list cannot be empty")
	}

	if len(fl.Founders) > MaxFounders {
		return fmt.Errorf("too many founders: %d > %d", len(fl.Founders), MaxFounders)
	}

	ids := make(map[string]bool)
	addresses := make(map[string]bool)

	for i, f := range fl.Founders {
		if f.ID == "" {
			return fmt.Errorf("founder at index %d has empty ID", i)
		}
		if ids[f.ID] {
			return fmt.Errorf("duplicate founder ID: %s", f.ID)
		}
		ids[f.ID] = true

		if f.Address == "" {
			return fmt.Errorf("founder %s has empty address", f.ID)
		}
		if addresses[f.Address] {
			return fmt.Errorf("duplicate address: %s", f.Address)
		}
		addresses[f.Address] = true

		if f.TotalAmount != AllocationPerFounder {
			return fmt.Errorf("founder %s has invalid amount: %d != %d",
				f.ID, f.TotalAmount, AllocationPerFounder)
		}
	}

	return nil
}

// ToJSON converts the founder list to JSON.
func (fl *FounderList) ToJSON() ([]byte, error) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	return json.MarshalIndent(fl, "", "  ")
}

// FromJSON loads a founder list from JSON.
func (fl *FounderList) FromJSON(data []byte) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	var newList FounderList
	if err := json.Unmarshal(data, &newList); err != nil {
		return fmt.Errorf("failed to parse founder list: %w", err)
	}

	// Validate the loaded list
	if err := newList.Validate(); err != nil {
		return fmt.Errorf("invalid founder list: %w", err)
	}

	// Rebuild addresses from Bech32m
	for _, f := range newList.Founders {
		_, addrBytes, err := utxo.DecodeBech32m(f.Address)
		if err != nil {
			return fmt.Errorf("invalid address for founder %s: %w", f.ID, err)
		}
		copy(f.AddressBytes[:], addrBytes)

		if f.PublicKey != "" {
			pubKey, err := hex.DecodeString(f.PublicKey)
			if err != nil {
				return fmt.Errorf("invalid public key for founder %s: %w", f.ID, err)
			}
			f.PubKeyBytes = pubKey
		}
	}

	fl.Founders = newList.Founders
	fl.Version = newList.Version
	return nil
}

// MultiSigConfig represents the multi-signature configuration for founder releases.
type MultiSigConfig struct {
	RequiredSigs   int      `json:"required_sigs"`   // Number of signatures required
	SignerAddresses []string `json:"signer_addresses"` // Authorized signer addresses
}

// DefaultMultiSigConfig returns the default multi-sig configuration (3-of-5).
func DefaultMultiSigConfig() *MultiSigConfig {
	return &MultiSigConfig{
		RequiredSigs:   3,
		SignerAddresses: make([]string, 0, 5),
	}
}

// ReleaseRequest represents a request to release vested tokens.
type ReleaseRequest struct {
	FounderID   string    `json:"founder_id"`
	Amount      uint64    `json:"amount"`
	RequestTime time.Time `json:"request_time"`
	Signatures  []string  `json:"signatures"` // Hex encoded signatures from multi-sig signers
}
