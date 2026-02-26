// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Consensus Module (Proof of Stake)
package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// PoSConfig contains Proof of Stake parameters.
type PoSConfig struct {
	EpochLength     uint64        // Number of blocks per epoch
	MinStake        uint64        // Minimum stake required (in smallest unit)
	BlockReward     uint64        // Block reward for proposer
	MaxValidators   int           // Maximum number of validators
	StakeLockPeriod uint64        // Minimum blocks before unstake
	EpochDuration   time.Duration // Duration between epochs

	// Transaction fee parameters
	BaseFeePerByte   uint64 // Minimum fee per byte (in smallest unit)
	PriorityFeePerByte uint64 // Default priority fee per byte
}

// DefaultPoSConfig returns the default PoS configuration.
func DefaultPoSConfig() *PoSConfig {
	return &PoSConfig{
		EpochLength:     100,
		MinStake:        1000 * 1e8, // 1000 AIB
		BlockReward:     10 * 1e8,   // 10 AIB
		MaxValidators:   100,
		StakeLockPeriod: 100,
		EpochDuration:   time.Hour,

		// Transaction fees: 1 AIB = 1e8 satoshi
		BaseFeePerByte:    10,    // 10 satoshi per byte (0.1 AIB per KB)
		PriorityFeePerByte: 20,   // 20 satoshi per byte (0.2 AIB per KB)
	}
}

// Validator represents a PoS validator.
type Validator struct {
	Address       [32]byte
	Stake         uint64
	JoinedAt      uint64 // Block height when validator joined
	LastProposed  uint64 // Last block height when this validator proposed
	TotalRewards  uint64 // Total rewards earned
	Commission    uint8  // Commission rate (0-100)
	PublicKey     ed25519.PublicKey
}

// IsActive returns true if the validator is active (has sufficient stake).
func (v *Validator) IsActive(config *PoSConfig) bool {
	return v.Stake >= config.MinStake
}

// StakeInfo represents a stake delegation.
type StakeInfo struct {
	Delegator    [32]byte
	Validator    [32]byte
	Amount       uint64
	StakedAt     uint64
	UnstakeAt    *uint64 // nil if still staked
	PendingStake uint64  // For pending unstake
}

// ConsensusState holds the current consensus state.
type ConsensusState struct {
	config           *PoSConfig
	validators       map[[32]byte]*Validator
	stakes           map[[32]byte]*StakeInfo // key: delegator+validator
	currentEpoch     uint64
	currentHeight    uint64
	lastBlockTime    time.Time
	proposerQueue    [][32]byte // Ordered list of validators for proposing
	mu               sync.RWMutex
}

// NewConsensusState creates a new consensus state.
func NewConsensusState(config *PoSConfig) *ConsensusState {
	return &ConsensusState{
		config:        config,
		validators:    make(map[[32]byte]*Validator),
		stakes:        make(map[[32]byte]*StakeInfo),
		currentEpoch:  0,
		currentHeight: 0,
		lastBlockTime: time.Now(),
	}
}

// AddValidator adds a new validator to the consensus.
func (cs *ConsensusState) AddValidator(address [32]byte, stake uint64, publicKey ed25519.PublicKey) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if stake < cs.config.MinStake {
		return fmt.Errorf("stake %d is below minimum %d", stake, cs.config.MinStake)
	}

	if len(cs.validators) >= cs.config.MaxValidators {
		return fmt.Errorf("maximum number of validators reached")
	}

	if _, exists := cs.validators[address]; exists {
		return fmt.Errorf("validator already exists")
	}

	cs.validators[address] = &Validator{
		Address:   address,
		Stake:     stake,
		JoinedAt:  cs.currentHeight,
		PublicKey: publicKey,
	}

	// Rebuild proposer queue
	cs.rebuildProposerQueue()

	return nil
}

// RemoveValidator removes a validator from the consensus.
func (cs *ConsensusState) RemoveValidator(address [32]byte) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	validator, exists := cs.validators[address]
	if !exists {
		return fmt.Errorf("validator does not exist")
	}

	// Check stake lock period
	if cs.currentHeight-validator.JoinedAt < cs.config.StakeLockPeriod {
		return fmt.Errorf("stake is still locked for %d blocks", cs.config.StakeLockPeriod-(cs.currentHeight-validator.JoinedAt))
	}

	delete(cs.validators, address)

	// Rebuild proposer queue
	cs.rebuildProposerQueue()

	return nil
}

// GetValidator returns a validator by address.
func (cs *ConsensusState) GetValidator(address [32]byte) (*Validator, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validator, exists := cs.validators[address]
	if !exists {
		return nil, fmt.Errorf("validator not found")
	}

	return validator, nil
}

// GetActiveValidators returns all active validators.
func (cs *ConsensusState) GetActiveValidators() []*Validator {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := make([]*Validator, 0, len(cs.validators))
	for _, v := range cs.validators {
		if v.IsActive(cs.config) {
			validators = append(validators, v)
		}
	}

	return validators
}

// SelectProposer selects the block proposer for the current height.
// Uses weighted random selection based on stake.
func (cs *ConsensusState) SelectProposer(seed []byte) ([32]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := cs.GetActiveValidators()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// Calculate total stake
	var totalStake uint64
	for _, v := range validators {
		totalStake += v.Stake
	}

	if totalStake == 0 {
		return [32]byte{}, fmt.Errorf("total stake is zero")
	}

	// Use VRF-like selection based on seed and height
	// Hash the seed with height to get a deterministic random value
	hash := sha256.New()
	hash.Write(seed)
	binary.Write(hash, binary.BigEndian, cs.currentHeight)
	digest := hash.Sum(nil)

	// Convert to big.Int for weighted selection
	randomInt := new(big.Int).SetBytes(digest)
	totalStakeBig := new(big.Int).SetUint64(totalStake)

	// Get random value in range [0, totalStake)
	selectedStake := new(big.Int).Mod(randomInt, totalStakeBig)

	// Select proposer based on cumulative stake
	var cumulativeStake uint64
	for _, v := range validators {
		cumulativeStake += v.Stake
		if selectedStake.Uint64() < cumulativeStake {
			return v.Address, nil
		}
	}

	// Fallback (should not reach here)
	return validators[len(validators)-1].Address, nil
}

// rebuildProposerQueue rebuilds the proposer selection queue.
func (cs *ConsensusState) rebuildProposerQueue() {
	validators := cs.GetActiveValidators()
	if len(validators) == 0 {
		cs.proposerQueue = nil
		return
	}

	// Create queue with weighted validators
	cs.proposerQueue = make([][32]byte, 0, len(validators))
	for _, v := range validators {
		cs.proposerQueue = append(cs.proposerQueue, v.Address)
	}
}

// ProcessNewBlock processes a new block and updates the consensus state.
func (cs *ConsensusState) ProcessNewBlock(block *Block) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Validate block height
	if block.Header.Height != cs.currentHeight+1 {
		return fmt.Errorf("invalid block height: expected %d, got %d", cs.currentHeight+1, block.Header.Height)
	}

	// Update validator last proposed
	if validator, exists := cs.validators[block.Header.Proposer]; exists {
		validator.LastProposed = block.Header.Height
		validator.TotalRewards += cs.config.BlockReward
	}

	// Update state
	cs.currentHeight = block.Header.Height
	cs.lastBlockTime = time.Unix(int64(block.Header.Timestamp), 0)

	// Check for epoch end
	if cs.currentHeight%cs.config.EpochLength == 0 {
		cs.currentEpoch++
		cs.onEpochEnd()
	}

	return nil
}

// onEpochEnd handles epoch end logic.
func (cs *ConsensusState) onEpochEnd() {
	// Rebuild proposer queue with potentially changed stakes
	cs.rebuildProposerQueue()

	// Distribute rewards or perform other epoch-end tasks
}

// GetCurrentHeight returns the current blockchain height.
func (cs *ConsensusState) GetCurrentHeight() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.currentHeight
}

// GetCurrentEpoch returns the current epoch number.
func (cs *ConsensusState) GetCurrentEpoch() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.currentEpoch
}

// GetValidatorCount returns the total number of validators.
func (cs *ConsensusState) GetValidatorCount() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.validators)
}

// GetTotalStake returns the total staked amount.
func (cs *ConsensusState) GetTotalStake() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var total uint64
	for _, v := range cs.validators {
		total += v.Stake
	}
	return total
}

// ValidateProposer validates that the given address is the valid proposer for the current height.
func (cs *ConsensusState) ValidateProposer(proposer [32]byte, seed []byte) error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	expectedProposer, err := cs.SelectProposer(seed)
	if err != nil {
		return err
	}

	if expectedProposer != proposer {
		return fmt.Errorf("invalid proposer: expected %x, got %x", expectedProposer, proposer)
	}

	return nil
}
