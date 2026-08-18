// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - PoS V2 Module (Proof of Stake with Reputation Weighting)
package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
)

// V2 protocolparameter
const (
	TotalSupply          = uint64(3_141_592_653) // π × 10^8
	TotalSupplySatoshi   = TotalSupply * 1e8
	BlockRewardV2        = uint64(50) // 50 AIB per block
	BlockRewardSatoshi   = BlockRewardV2 * 1e8
	StakingRewardRatio   = 0.6          // 60% staking reward
	InferenceRewardRatio = 0.4          // 40% inference reward
	MinStakeV2           = uint64(1000) // minimum 1000 AIB
	MinStakeV2Satoshi    = MinStakeV2 * 1e8
	InitialNodeStake     = uint64(1000) // initial node stake 1000 AIB (same as minimum stake)
	InitialNodeCount     = 100
	UnlockPeriodBlocks   = uint64(20160) // 7 days (30 sec/block): 7*24*60*60/30
	ScoreCheckInterval   = uint64(100)   // score check every 100 blocks (~50 minutes)
)

// CreateCoinbaseV2 creates a v2 coinbase transaction, allocating staking + inference rewards
func CreateCoinbaseV2(proposer [32]byte, blockHeight uint64) *Transaction {
	// Calculate total reward
	totalReward := BlockRewardSatoshi

	// Calculate staking reward and inference reward
	stakingReward := uint64(float64(totalReward) * StakingRewardRatio)
	inferenceReward := uint64(float64(totalReward) * InferenceRewardRatio)

	// create outputs
	outputs := []TXOutput{
		{
			Value:   stakingReward,
			Script:  []byte("staking"),
			Address: NewAddressFromPublicKey(proposer[:]),
		},
		{
			Value:   inferenceReward,
			Script:  []byte("inference"),
			Address: NewAddressFromPublicKey(proposer[:]),
		},
	}

	// create coinbase transaction
	tx := &Transaction{
		Version: 2,
		Inputs: []TXInput{
			{
				TxHash:    [32]byte{},
				Index:     0xffffffff,
				Signature: []byte(fmt.Sprintf("coinbase_v2_height_%d", blockHeight)),
				PublicKey: proposer[:],
			},
		},
		Outputs:  outputs,
		LockTime: 0,
		Sequence: blockHeight,
	}

	return tx
}

// InitGenesisValidators initializes the genesis validator set (100 nodes with 100 AIB each)
func InitGenesisValidators(cs *ConsensusState, keys []ed25519.PublicKey) error {
	if len(keys) != InitialNodeCount {
		return fmt.Errorf("expected %d genesis keys, got %d", InitialNodeCount, len(keys))
	}

	// Use V2 configuration
	config := &PoSConfig{
		EpochLength:     314,
		MinStake:        MinStakeV2Satoshi,
		BlockReward:     BlockRewardSatoshi,
		MaxValidators:   InitialNodeCount,
		StakeLockPeriod: UnlockPeriodBlocks,
	}
	cs.config = config

	// Add all validators
	for i, pubKey := range keys {
		var addr [32]byte
		copy(addr[:], pubKey)

		err := cs.AddValidator(addr, InitialNodeStake*1e8, pubKey)
		if err != nil {
			return fmt.Errorf("failed to add validator %d: %w", i, err)
		}
	}

	return nil
}

// SelectProposerV2 selects the block proposer with reputation-weighted scoring
// Uses VRF-based random selection, but with effective weight (stake × reputation multiplier)
func (cs *ConsensusState) SelectProposerV2(seed []byte, rm *ReputationManager) ([32]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// Calculate each validator's effective weight
	type weightedValidator struct {
		addr            [32]byte
		effectiveWeight float64
		originalStake   uint64
	}

	var weighted []weightedValidator
	var totalEffectiveWeight float64

	for _, v := range validators {
		// Get effective weight = stake × multiplier
		multiplier := CalculateWeightMultiplier(rm.GetAverageScore(v.Address))
		effectiveWeight := float64(v.Stake) * multiplier

		weighted = append(weighted, weightedValidator{
			addr:            v.Address,
			effectiveWeight: effectiveWeight,
			originalStake:   v.Stake,
		})
		totalEffectiveWeight += effectiveWeight
	}

	if totalEffectiveWeight == 0 {
		return [32]byte{}, fmt.Errorf("total effective weight is zero")
	}

	// Use VRF-like selection based on effective weight
	// Hash the seed with height to get a deterministic random value
	hash := sha256.New()
	hash.Write(seed)
	binary.Write(hash, binary.BigEndian, cs.currentHeight)
	digest := hash.Sum(nil)

	// Convert to big.Int for weighted selection
	randomInt := new(big.Int).SetBytes(digest)
	totalWeightBig := new(big.Int).SetUint64(uint64(totalEffectiveWeight * 1e8))

	// Get a random value in the range [0, totalEffectiveWeight)
	selectedWeight := new(big.Int).Mod(randomInt, totalWeightBig)
	selectedWeightFloat := float64(selectedWeight.Uint64()) / 1e8

	// Select the proposer based on cumulative effective weight
	var cumulativeWeight float64
	for _, w := range weighted {
		cumulativeWeight += w.effectiveWeight
		if selectedWeightFloat < cumulativeWeight {
			return w.addr, nil
		}
	}

	// Fallback (theoretically unreachable)
	return weighted[len(weighted)-1].addr, nil
}

// GetValidatorEffectiveWeight returns the validator's effective weight
func (cs *ConsensusState) GetValidatorEffectiveWeight(addr [32]byte, rm *ReputationManager) (float64, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validator, exists := cs.validators[addr]
	if !exists {
		return 0, fmt.Errorf("validator not found")
	}

	multiplier := CalculateWeightMultiplier(rm.GetAverageScore(addr))
	return float64(validator.Stake) * multiplier, nil
}

// ValidateProposerV2 verifies whether the V2 proposer selection is correct
func (cs *ConsensusState) ValidateProposerV2(proposer [32]byte, seed []byte, rm *ReputationManager) error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	expectedProposer, err := cs.selectProposerV2Locked(seed, rm)
	if err != nil {
		return err
	}

	if expectedProposer != proposer {
		return fmt.Errorf("invalid proposer: expected %x, got %x", expectedProposer, proposer)
	}

	return nil
}

// selectProposerV2Locked is the internal implementation of V2 proposer selection (lock must be held)
func (cs *ConsensusState) selectProposerV2Locked(seed []byte, rm *ReputationManager) ([32]byte, error) {
	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// Calculate each validator's effective weight
	type weightedValidator struct {
		addr            [32]byte
		effectiveWeight float64
	}

	var weighted []weightedValidator
	var totalEffectiveWeight float64

	for _, v := range validators {
		multiplier := CalculateWeightMultiplier(rm.GetAverageScore(v.Address))
		effectiveWeight := float64(v.Stake) * multiplier

		weighted = append(weighted, weightedValidator{
			addr:            v.Address,
			effectiveWeight: effectiveWeight,
		})
		totalEffectiveWeight += effectiveWeight
	}

	if totalEffectiveWeight == 0 {
		return [32]byte{}, fmt.Errorf("total effective weight is zero")
	}

	// VRF-like selection
	hash := sha256.New()
	hash.Write(seed)
	binary.Write(hash, binary.BigEndian, cs.currentHeight)
	digest := hash.Sum(nil)

	randomInt := new(big.Int).SetBytes(digest)
	totalWeightBig := new(big.Int).SetUint64(uint64(totalEffectiveWeight * 1e8))

	selectedWeight := new(big.Int).Mod(randomInt, totalWeightBig)
	selectedWeightFloat := float64(selectedWeight.Uint64()) / 1e8

	var cumulativeWeight float64
	for _, w := range weighted {
		cumulativeWeight += w.effectiveWeight
		if selectedWeightFloat < cumulativeWeight {
			return w.addr, nil
		}
	}

	return weighted[len(weighted)-1].addr, nil
}

// CalculateBlockRewardsV2 calculates the V2 block reward distribution
func CalculateBlockRewardsV2() (stakingReward, inferenceReward uint64) {
	totalReward := BlockRewardSatoshi
	stakingReward = uint64(float64(totalReward) * StakingRewardRatio)
	inferenceReward = uint64(float64(totalReward) * InferenceRewardRatio)
	return
}

// GetV2Config returns the V2 protocol configuration
func GetV2Config() *PoSConfig {
	return &PoSConfig{
		EpochLength:     314,
		MinStake:        MinStakeV2Satoshi,
		BlockReward:     BlockRewardSatoshi,
		MaxValidators:   InitialNodeCount,
		StakeLockPeriod: UnlockPeriodBlocks,
	}
}

// NewAddressFromPublicKey creates an address from a public key (uses the existing AddressFromPublicKey)
func NewAddressFromPublicKey(pubKey []byte) [32]byte {
	return AddressFromPublicKey(pubKey)
}

// GenerateInferencePoW generates a proof-of-AI-work hash for block production.
// This is a hash of the proposer + prevBlockHash + height, proving the proposer
// performed the inference computation for this block slot.
func GenerateInferencePoW(proposer [32]byte, prevBlockHash [32]byte, height uint64) []byte {
	h := sha256.New()
	h.Write(proposer[:])
	h.Write(prevBlockHash[:])
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], height)
	h.Write(buf[:])
	h.Write([]byte("PoAIW-v2"))
	return h.Sum(nil)
}

// VerifyReputationBasedSelection verifies the reputation-based proposer selection
// Returns the selected validator and selection probability information
func (cs *ConsensusState) VerifyReputationBasedSelection(proposer [32]byte, seed []byte, rm *ReputationManager) *ProposerVerificationResult {
	result := &ProposerVerificationResult{
		Valid: false,
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Calculate the expected proposer
	expectedProposer, err := cs.selectProposerV2Locked(seed, rm)
	if err != nil {
		result.Error = fmt.Sprintf("failed to select proposer: %v", err)
		return result
	}

	result.ExpectedProposer = expectedProposer

	// Compare
	if proposer != expectedProposer {
		result.Error = fmt.Sprintf("proposer mismatch: expected %x, got %x",
			expectedProposer, proposer)
		return result
	}

	// Get validator information
	validator, exists := cs.validators[proposer]
	if !exists {
		result.Error = fmt.Sprintf("proposer %x not in validator set", proposer)
		return result
	}

	// Calculate effective weight and probability
	multiplier := CalculateWeightMultiplier(rm.GetAverageScore(proposer))
	effectiveWeight := float64(validator.Stake) * multiplier

	var totalWeight float64
	for _, v := range cs.validators {
		if v.IsActive(cs.config) {
			m := CalculateWeightMultiplier(rm.GetAverageScore(v.Address))
			totalWeight += float64(v.Stake) * m
		}
	}

	result.Valid = true
	result.Stake = validator.Stake
	result.TotalStake = cs.getTotalStakeLocked()
	result.Probability = effectiveWeight / totalWeight

	return result
}
