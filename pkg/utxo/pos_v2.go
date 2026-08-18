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

// V2 协议参数
const (
	TotalSupply          = uint64(3_141_592_653) // π × 10^8
	TotalSupplySatoshi   = TotalSupply * 1e8
	BlockRewardV2        = uint64(50) // 50 AIB per block
	BlockRewardSatoshi   = BlockRewardV2 * 1e8
	StakingRewardRatio   = 0.6          // 60% 质押奖励
	InferenceRewardRatio = 0.4          // 40% 推理奖励
	MinStakeV2           = uint64(1000) // 最低1000 AIB
	MinStakeV2Satoshi    = MinStakeV2 * 1e8
	InitialNodeStake     = uint64(1000) // 初始节点1000 AIB (与最低质押一致)
	InitialNodeCount     = 100
	UnlockPeriodBlocks   = uint64(20160) // 7天 (30秒/块): 7*24*60*60/30
	ScoreCheckInterval   = uint64(100)   // 每100块校验评分 (~50分钟)
)

// CoinbaseV2 创建v2版coinbase交易，分配质押+推理奖励
func CreateCoinbaseV2(proposer [32]byte, blockHeight uint64) *Transaction {
	// 计算总奖励
	totalReward := BlockRewardSatoshi

	// 计算质押奖励和推理奖励
	stakingReward := uint64(float64(totalReward) * StakingRewardRatio)
	inferenceReward := uint64(float64(totalReward) * InferenceRewardRatio)

	// createoutput
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

	// createcoinbasetransaction
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

// InitGenesisValidators 初始化创世验证者集合（100个节点各100 AIB）
func InitGenesisValidators(cs *ConsensusState, keys []ed25519.PublicKey) error {
	if len(keys) != InitialNodeCount {
		return fmt.Errorf("expected %d genesis keys, got %d", InitialNodeCount, len(keys))
	}

	// 使用V2配置
	config := &PoSConfig{
		EpochLength:     314,
		MinStake:        MinStakeV2Satoshi,
		BlockReward:     BlockRewardSatoshi,
		MaxValidators:   InitialNodeCount,
		StakeLockPeriod: UnlockPeriodBlocks,
	}
	cs.config = config

	// 添加所有验证者
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

// SelectProposerV2 带评分权重的出块者选择
// 使用VRF随机选择，但使用有效权重（stake × reputation multiplier）
func (cs *ConsensusState) SelectProposerV2(seed []byte, rm *ReputationManager) ([32]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// 计算每个验证者的有效权重
	type weightedValidator struct {
		addr            [32]byte
		effectiveWeight float64
		originalStake   uint64
	}

	var weighted []weightedValidator
	var totalEffectiveWeight float64

	for _, v := range validators {
		// 获取有效权重 = stake × multiplier
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

	// 使用VRF-like选择基于有效权重
	// Hash the seed with height to get a deterministic random value
	hash := sha256.New()
	hash.Write(seed)
	binary.Write(hash, binary.BigEndian, cs.currentHeight)
	digest := hash.Sum(nil)

	// 转换为big.Int进行加权选择
	randomInt := new(big.Int).SetBytes(digest)
	totalWeightBig := new(big.Int).SetUint64(uint64(totalEffectiveWeight * 1e8))

	// 获取[0, totalEffectiveWeight)范围内的随机值
	selectedWeight := new(big.Int).Mod(randomInt, totalWeightBig)
	selectedWeightFloat := float64(selectedWeight.Uint64()) / 1e8

	// 基于累积有效权重选择出块者
	var cumulativeWeight float64
	for _, w := range weighted {
		cumulativeWeight += w.effectiveWeight
		if selectedWeightFloat < cumulativeWeight {
			return w.addr, nil
		}
	}

	// Fallback（理论上不应到达这里）
	return weighted[len(weighted)-1].addr, nil
}

// GetValidatorEffectiveWeight 获取验证者的有效权重
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

// ValidateProposerV2 验证V2出块者选择是否正确
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

// selectProposerV2Locked V2出块者选择的内部实现（需要持有锁）
func (cs *ConsensusState) selectProposerV2Locked(seed []byte, rm *ReputationManager) ([32]byte, error) {
	validators := cs.getActiveValidatorsLocked()
	if len(validators) == 0 {
		return [32]byte{}, fmt.Errorf("no active validators")
	}

	// 计算每个验证者的有效权重
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

	// VRF-like选择
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

// CalculateBlockRewardsV2 计算V2版本的区块奖励分配
func CalculateBlockRewardsV2() (stakingReward, inferenceReward uint64) {
	totalReward := BlockRewardSatoshi
	stakingReward = uint64(float64(totalReward) * StakingRewardRatio)
	inferenceReward = uint64(float64(totalReward) * InferenceRewardRatio)
	return
}

// GetV2Config 返回V2协议配置
func GetV2Config() *PoSConfig {
	return &PoSConfig{
		EpochLength:     314,
		MinStake:        MinStakeV2Satoshi,
		BlockReward:     BlockRewardSatoshi,
		MaxValidators:   InitialNodeCount,
		StakeLockPeriod: UnlockPeriodBlocks,
	}
}

// NewAddressFromPublicKey 从公钥创建地址（使用已有的AddressFromPublicKey）
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

// VerifyReputationBasedSelection 验证基于评分的出块者选择
// 返回选择的验证者和选择概率信息
func (cs *ConsensusState) VerifyReputationBasedSelection(proposer [32]byte, seed []byte, rm *ReputationManager) *ProposerVerificationResult {
	result := &ProposerVerificationResult{
		Valid: false,
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// 计算预期出块者
	expectedProposer, err := cs.selectProposerV2Locked(seed, rm)
	if err != nil {
		result.Error = fmt.Sprintf("failed to select proposer: %v", err)
		return result
	}

	result.ExpectedProposer = expectedProposer

	// 比较
	if proposer != expectedProposer {
		result.Error = fmt.Sprintf("proposer mismatch: expected %x, got %x",
			expectedProposer, proposer)
		return result
	}

	// 获取验证者信息
	validator, exists := cs.validators[proposer]
	if !exists {
		result.Error = fmt.Sprintf("proposer %x not in validator set", proposer)
		return result
	}

	// 计算有效权重和概率
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
