// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Consensus Module (Proof of Stake)
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"sync"
	"time"
)

// block time configuration
const (
	TargetBlockTime   = 60 * time.Second // target block time (60s) - L1 prioritizes stability/security; high-speed tx goes to L2
	MaxBlockTimeDrift = 5 * time.Minute  // max time drift (attack mitigation)
	MinBlockTime      = 10 * time.Second // minimum block time
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
	BaseFeePerByte     uint64 // Minimum fee per byte (in smallest unit)
	PriorityFeePerByte uint64 // Default priority fee per byte
}

// DefaultPoSConfig returns the default PoS configuration.
func DefaultPoSConfig() *PoSConfig {
	return &PoSConfig{
		EpochLength:     314,        // π-related
		MinStake:        1000 * 1e8, // 1000 AIB
		BlockReward:     50 * 1e8,   // 50 AIB
		MaxValidators:   100,
		StakeLockPeriod: 100,
		EpochDuration:   100 * TargetBlockTime, // ~100 min/epoch

		// Transaction fees: 1 AIB = 1e8 satoshi
		BaseFeePerByte:     10, // 10 satoshi per byte (0.1 AIB per KB)
		PriorityFeePerByte: 20, // 20 satoshi per byte (0.2 AIB per KB)
	}
}

// Validator represents a PoS validator.
type Validator struct {
	Address      [32]byte
	Stake        uint64
	JoinedAt     uint64 // Block height when validator joined
	LastProposed uint64 // Last block height when this validator proposed
	TotalRewards uint64 // Total rewards earned
	Commission   uint8  // Commission rate (0-100)
	PublicKey    ed25519.PublicKey
	FromPoW      bool // registered from PoW-era mining history (bootstrap set)
	// True-stake mode: weight comes ONLY from live stake UTXOs on chain.
	StakeUTXOCnt uint64
}

// IsActive returns true if the validator is active (has sufficient stake).
// PoW-era validators are always active: their mined blocks ARE the stake.
func (v *Validator) IsActive(config *PoSConfig) bool {
	if v.FromPoW {
		return true
	}
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
	config        *PoSConfig
	validators    map[[32]byte]*Validator
	stakes        map[[32]byte]*StakeInfo // key: delegator+validator
	currentEpoch  uint64
	currentHeight uint64
	lastBlockTime time.Time
	proposerQueue [][32]byte // Ordered list of validators for proposing
	mu            sync.RWMutex

	onEmptyValidatorSet func()
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

// HasValidator reports whether the address is already a validator.
func (cs *ConsensusState) HasValidator(address [32]byte) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	_, ok := cs.validators[address]
	return ok
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

// AddValidatorFromPoW registers a validator derived from PoW-era mining
// history (weight = blocks mined). Bypasses MinStake: the PoW era is the
// stake — small miners must not be filtered out of the bootstrap set.
func (cs *ConsensusState) AddValidatorFromPoW(address [32]byte, blocksMined uint64, publicKey ed25519.PublicKey) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if blocksMined == 0 {
		return fmt.Errorf("no blocks mined")
	}
	if _, exists := cs.validators[address]; exists {
		return nil // already registered — idempotent
	}
	stake := blocksMined * 1e8 // 1 AIB weight per mined block
	cs.validators[address] = &Validator{
		Address:   address,
		Stake:     stake,
		JoinedAt:  cs.currentHeight,
		PublicKey: publicKey,
		FromPoW:   true,
	}
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
	return cs.getActiveValidatorsLocked()
}

// getActiveValidatorsLocked returns active validators. Caller must hold mu.
func (cs *ConsensusState) getActiveValidatorsLocked() []*Validator {
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
	return cs.selectProposerLocked(seed)
}

// selectProposerLocked selects the proposer. Caller must hold mu.
func (cs *ConsensusState) selectProposerLocked(seed []byte) ([32]byte, error) {
	return cs.selectProposerAtHeightLocked(seed, cs.currentHeight)
}

// selectProposerAtHeightLocked selects the proposer for a SPECIFIC height.
// Consensus-critical: the height must come from the block being produced or
// validated, never from mutable local state, or nodes at different sync
// positions compute different expected proposers.
func (cs *ConsensusState) selectProposerAtHeightLocked(seed []byte, height uint64) ([32]byte, error) {
	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// Deterministic ordering: map iteration order is random in Go; without
	// sorting, two nodes with identical state can select different proposers.
	slices.SortFunc(validators, func(a, b *Validator) int {
		return bytes.Compare(a.Address[:], b.Address[:])
	})

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
	binary.Write(hash, binary.BigEndian, height)
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
// Caller must hold mu.
func (cs *ConsensusState) rebuildProposerQueue() {
	// Build active validators list inline (no GetActiveValidators call)
	validators := make([]*Validator, 0, len(cs.validators))
	for _, v := range cs.validators {
		if v.IsActive(cs.config) {
			validators = append(validators, v)
		}
	}

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

// RestoreHeight restores the consensus height tracker after a node restart
// (chain data is loaded from DB but consensus in-memory height starts at 0).
func (cs *ConsensusState) RestoreHeight(height uint64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if height > cs.currentHeight {
		cs.currentHeight = height
	}
}

// ProcessNewBlock processes a new block and updates the consensus state.
func (cs *ConsensusState) ProcessNewBlock(block *Block) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Validate block height - allow height 0 for genesis, otherwise must be currentHeight+1
	if block.Header.Height != 0 && block.Header.Height != cs.currentHeight+1 {
		return fmt.Errorf("invalid block height: expected %d, got %d", cs.currentHeight+1, block.Header.Height)
	}

	// Skip validator updates for genesis block
	if block.Header.Height > 0 {
		// Update validator last proposed
		if validator, exists := cs.validators[block.Header.Proposer]; exists {
			validator.LastProposed = block.Header.Height
			validator.TotalRewards += cs.config.BlockReward
		}
	}

	// Update state
	cs.currentHeight = block.Header.Height
	cs.lastBlockTime = time.Unix(int64(block.Header.Timestamp), 0)

	// Check for epoch end
	if cs.currentHeight > 0 && cs.currentHeight%cs.config.EpochLength == 0 {
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

	expectedProposer, err := cs.selectProposerLocked(seed)
	if err != nil {
		return err
	}

	if expectedProposer != proposer {
		return fmt.Errorf("invalid proposer: expected %x, got %x", expectedProposer, proposer)
	}

	return nil
}

// VerifyBlockProposer verifies that the block proposer was correctly selected.
// This is the key function that allows anyone to verify the block producer.
// Returns the validation result.
func (cs *ConsensusState) VerifyBlockProposer(block *Block, prevBlock *Block) *ProposerVerificationResult {
	result := &ProposerVerificationResult{
		Valid: false,
	}

	// Step 1: Get the random seed from previous block
	var seed []byte
	if prevBlock != nil {
		seed = prevBlock.Header.VRFSeed[:]
	} else {
		// Genesis block - use zero seed
		seed = make([]byte, 32)
	}

	// A syncing node may validate PoS-era blocks before the validator set
	// has been built from PoW history — rebuild on demand.
	if len(cs.GetActiveValidators()) == 0 && cs.onEmptyValidatorSet != nil {
		cs.onEmptyValidatorSet()
	}

	// Bootstrap exception: if the validator set is empty (no stakes on chain
	// yet), any node may propose the transition block that carries the first
	// stake transaction. The mempool-enforced stake tx is what activates the
	// validator set; without this the chain deadlocks at the PoW/PoS boundary.
	if len(cs.GetActiveValidators()) == 0 && block.Header.Height == PoWEraBlocks+1 {
		result.Valid = true
		result.Bootstrap = true
		return result
	}

	// Step 2 & 3 & 4 & 5 & 6: Do all validation under a single lock
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Calculate expected proposer (deterministic: seed + the BLOCK's height)
	expectedProposer, err := cs.selectProposerAtHeightLocked(seed, block.Header.Height)
	if err != nil {
		result.Error = fmt.Sprintf("failed to select proposer: %v", err)
		return result
	}
	result.ExpectedProposer = expectedProposer

	// Header.Proposer IS the wallet address now (ProposerKey carries the
	// public key for signature verification). Compare directly.
	if block.Header.Proposer != expectedProposer {
		result.Error = fmt.Sprintf("proposer mismatch: expected %x, got %x",
			expectedProposer, block.Header.Proposer)
		return result
	}

	// Verify VRF proof (if provided)
	if len(block.Header.VRFProof) > 0 {
		// VRF proof verification would go here
		// For now, we verify that the proposer knows their private key
		// by checking they can sign the block
		if len(block.Header.Signature) == 0 {
			result.Error = "missing proposer signature"
			return result
		}
	}

	// Verify proposer exists in validator set
	validator, exists := cs.validators[block.Header.Proposer]
	if !exists {
		result.Error = fmt.Sprintf("proposer %x not in validator set", block.Header.Proposer)
		return result
	}

	// Verify proposer has minimum stake (PoW-era validators exempt — their
	// mined blocks are the stake; the bootstrap set must not be filtered)
	if !validator.FromPoW && validator.Stake < cs.config.MinStake {
		result.Error = fmt.Sprintf("proposer stake %d below minimum %d",
			validator.Stake, cs.config.MinStake)
		return result
	}

	result.Valid = true
	result.Stake = validator.Stake
	result.TotalStake = cs.getTotalStakeLocked()

	return result
}

// getTotalStakeLocked returns total stake. Caller must hold mu.
func (cs *ConsensusState) getTotalStakeLocked() uint64 {
	var total uint64
	for _, v := range cs.validators {
		total += v.Stake
	}
	return total
}

// ProposerVerificationResult contains the result of proposer verification.
type ProposerVerificationResult struct {
	Valid            bool
	Bootstrap        bool // accepted via empty-validator-set bootstrap exception
	Error            string
	ExpectedProposer [32]byte
	Stake            uint64
	TotalStake       uint64
	Probability      float64
}

// CalculateValidatorStateRoot computes the Merkle root of all validator states.
// This is stored in the block header so anyone can verify the validator set.
func (cs *ConsensusState) CalculateValidatorStateRoot() ([32]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.validators) == 0 {
		return [32]byte{}, nil
	}

	// Collect all validator data
	type validatorData struct {
		Address  [32]byte
		Stake    uint64
		PubKey   []byte
		LastProp uint64
	}

	var validators []validatorData
	for addr, v := range cs.validators {
		validators = append(validators, validatorData{
			Address:  addr,
			Stake:    v.Stake,
			PubKey:   v.PublicKey,
			LastProp: v.LastProposed,
		})
	}

	// Sort by address for deterministic ordering
	sort.Slice(validators, func(i, j int) bool {
		return bytes.Compare(validators[i].Address[:], validators[j].Address[:]) < 0
	})

	// Build Merkle tree from validator hashes
	var hashes [][]byte
	for _, v := range validators {
		hash := sha256.New()
		hash.Write(v.Address[:])
		binary.Write(hash, binary.BigEndian, v.Stake)
		binary.Write(hash, binary.BigEndian, v.LastProp)
		hash.Write(v.PubKey)
		hashes = append(hashes, hash.Sum(nil))
	}

	// Calculate Merkle root
	for len(hashes) > 1 {
		if len(hashes)%2 == 1 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}
		var newHashes [][]byte
		for i := 0; i < len(hashes); i += 2 {
			hash := sha256.New()
			hash.Write(hashes[i])
			hash.Write(hashes[i+1])
			newHashes = append(newHashes, hash.Sum(nil))
		}
		hashes = newHashes
	}

	var result [32]byte
	copy(result[:], hashes[0])
	return result, nil
}

// VerifyValidatorStateRoot verifies that the given state root matches current validators.
func (cs *ConsensusState) VerifyValidatorStateRoot(root [32]byte) (bool, error) {
	calculated, err := cs.CalculateValidatorStateRoot()
	if err != nil {
		return false, err
	}
	return calculated == root, nil
}

// SetEmptyValidatorSetHook installs a callback fired when proposer selection
// finds no active validators (e.g. a syncing node entering the PoS era).
func (cs *ConsensusState) SetEmptyValidatorSetHook(fn func()) { cs.onEmptyValidatorSet = fn }

// ResetValidators clears the validator set (for stake-index rebuilds).
func (cs *ConsensusState) ResetValidators() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.validators = make(map[[32]byte]*Validator)
}

// AddValidatorFromStake registers a validator whose weight is real locked
// AIB (true-stake model). No MinStake gate: any staked amount earns
// proportional sortition weight. PublicKey is unknown here — block
// signature verification uses Header.Proposer (the key) and wallet-address
// comparison happens at selection time.
func (cs *ConsensusState) AddValidatorFromStake(address [32]byte, stake uint64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if v, ok := cs.validators[address]; ok {
		v.Stake = stake
		return nil
	}
	cs.validators[address] = &Validator{
		Address:  address,
		Stake:    stake,
		JoinedAt: cs.currentHeight,
		// TRUE-STAKE (V4): staked coins alone confer sortition weight —
		// no minimum. IsActive() must not gate on MinStake (which only
		// applies to legacy delegated stakes).
		FromPoW: true,
	}
	return nil
}
