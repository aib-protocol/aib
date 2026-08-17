// Package utxo implements UTXO-based transaction system for AIB blockchain.
// BlockV2 extends the Block with reputation scoring and inference statistics.
package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// V2 区块奖励常量
const (
	BlockRewardV2Total    = uint64(50 * 1e8) // 50 AIB in satoshi
	StakingRewardAmount   = uint64(30 * 1e8) // 30 AIB
	InferenceRewardAmount = uint64(20 * 1e8) // 20 AIB
)

// BlockReputationScore represents a node's aggregated reputation score for blockchain storage.
// This is a lightweight version designed for serialization in blocks.
type BlockReputationScore struct {
	NodeID [32]byte // Node identifier

	// Aggregated score
	TotalScore float64 `json:"total_score"`

	// Multi-dimensional scores
	UserScore    float64 `json:"user_score"`
	TechScore    float64 `json:"tech_score"`
	HistoryScore float64 `json:"history_score"`
	StakeScore   float64 `json:"stake_score"`

	// Statistics
	TotalTasksCompleted uint64 `json:"total_tasks_completed"`
	TotalBlocksProduced uint64 `json:"total_blocks_produced"`
	TotalSlashes        uint64 `json:"total_slashes"`

	// Timestamp
	UpdatedAt uint64 `json:"updated_at"` // Unix timestamp

	// Signature for validation
	Signature []byte `json:"signature,omitempty"`
}

// BlockInferenceStats contains inference statistics for a block.
type BlockInferenceStats struct {
	TotalInferences uint64     // Total inferences during this block period
	ActiveNodes     uint32     // Number of active inference nodes
	AvgLatencyMs    uint64     // Average latency in milliseconds
	TopContributors [][32]byte // Top inference contributors
}

// BlockV2 extends Block with reputation data and inference statistics.
type BlockV2 struct {
	Block                                    // Embed original Block
	ReputationUpdates []BlockReputationScore // Reputation updates
	InferenceStats    BlockInferenceStats    // Inference statistics
}

// NewBlockV2 creates a new v2 block with reputation updates and inference stats.
func NewBlockV2(
	transactions []*Transaction,
	prevBlockHash [32]byte,
	height uint64,
	proposer [32]byte,
	reputationUpdates []BlockReputationScore,
	inferenceStats BlockInferenceStats,
) *BlockV2 {
	// Create base block
	block := NewBlock(transactions, prevBlockHash, height, proposer)

	// Return extended v2 block
	return &BlockV2{
		Block:             *block,
		ReputationUpdates: reputationUpdates,
		InferenceStats:    inferenceStats,
	}
}

// CreateCoinbaseV2Tx creates a v2 coinbase transaction (50 AIB).
// Splits into 2 outputs: 30 to staker/proposer, 20 to inference contributor.
func CreateCoinbaseV2Tx(proposer [32]byte, topInferenceNode [32]byte, height uint64) *Transaction {
	// Empty input (coinbase)
	inputs := []TXInput{
		{
			TxHash: [32]byte{},
			Index:  0xffffffff,
		},
	}

	// 2 outputs: 30 AIB to proposer (staking), 20 AIB to inference node
	outputs := []TXOutput{
		{
			Value:   StakingRewardAmount,
			Script:  []byte("staking"),
			Address: interfaces.Address(proposer),
		},
		{
			Value:   InferenceRewardAmount,
			Script:  []byte("inference"),
			Address: interfaces.Address(topInferenceNode),
		},
	}

	tx := NewTransaction(inputs, outputs)
	tx.LockTime = uint32(height)

	return tx
}

// ValidateReputationUpdates validates the reputation data in the block.
func (b *BlockV2) ValidateReputationUpdates() error {
	// Check each reputation update
	for i, rep := range b.ReputationUpdates {
		if err := rep.validate(); err != nil {
			return fmt.Errorf("reputation update %d validation failed: %w", i, err)
		}
	}

	// Validate inference stats
	if err := b.InferenceStats.validate(); err != nil {
		return fmt.Errorf("inference stats validation failed: %w", err)
	}

	return nil
}

// validate checks if a reputation score is valid.
func (rep *BlockReputationScore) validate() error {
	// Check node ID is not empty
	if rep.NodeID == [32]byte{} {
		return fmt.Errorf("empty node ID")
	}

	// Check scores are within reasonable bounds
	if rep.TotalScore < -200 || rep.TotalScore > 500 {
		return fmt.Errorf("total score out of bounds: %f", rep.TotalScore)
	}

	if rep.UserScore < 0 || rep.UserScore > 100 {
		return fmt.Errorf("user score out of bounds: %f", rep.UserScore)
	}

	if rep.TechScore < 0 || rep.TechScore > 100 {
		return fmt.Errorf("tech score out of bounds: %f", rep.TechScore)
	}

	if rep.HistoryScore < 0 || rep.HistoryScore > 100 {
		return fmt.Errorf("history score out of bounds: %f", rep.HistoryScore)
	}

	if rep.StakeScore < 0 || rep.StakeScore > 100 {
		return fmt.Errorf("stake score out of bounds: %f", rep.StakeScore)
	}

	return nil
}

// validate checks if inference stats are valid.
func (s *BlockInferenceStats) validate() error {
	// Total inferences should not exceed reasonable limits per block
	if s.TotalInferences > 1e9 {
		return fmt.Errorf("total inferences exceeds limit: %d", s.TotalInferences)
	}

	// Active nodes should be reasonable
	if s.ActiveNodes > 10000 {
		return fmt.Errorf("active nodes exceeds limit: %d", s.ActiveNodes)
	}

	// Latency should be reasonable (max 1 hour = 3600000ms)
	if s.AvgLatencyMs > 3600000 {
		return fmt.Errorf("average latency exceeds limit: %d", s.AvgLatencyMs)
	}

	return nil
}

// CalculateReputationRoot calculates the Merkle root of reputation data.
func (b *BlockV2) CalculateReputationRoot() [32]byte {
	if len(b.ReputationUpdates) == 0 {
		return [32]byte{}
	}

	// Hash each reputation update
	hashes := make([][32]byte, len(b.ReputationUpdates))
	for i, rep := range b.ReputationUpdates {
		hashes[i] = rep.Hash()
	}

	// Build Merkle tree
	for len(hashes) > 1 {
		if len(hashes)%2 != 0 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}

		nextLevel := make([][32]byte, len(hashes)/2)
		for i := 0; i < len(hashes); i += 2 {
			concat := append(hashes[i][:], hashes[i+1][:]...)
			hash := sha256.Sum256(concat)
			nextLevel[i/2] = sha256.Sum256(hash[:])
		}

		hashes = nextLevel
	}

	return hashes[0]
}

// Hash computes the hash of a reputation score.
func (rep *BlockReputationScore) Hash() [32]byte {
	data := rep.Serialize()
	hash1 := sha256.Sum256(data)
	return sha256.Sum256(hash1[:])
}

// Serialize serializes a reputation score to bytes.
func (rep *BlockReputationScore) Serialize() []byte {
	var buf bytes.Buffer

	buf.Write(rep.NodeID[:])
	binary.Write(&buf, binary.LittleEndian, rep.TotalScore)
	binary.Write(&buf, binary.LittleEndian, rep.UserScore)
	binary.Write(&buf, binary.LittleEndian, rep.TechScore)
	binary.Write(&buf, binary.LittleEndian, rep.HistoryScore)
	binary.Write(&buf, binary.LittleEndian, rep.StakeScore)
	binary.Write(&buf, binary.LittleEndian, rep.TotalTasksCompleted)
	binary.Write(&buf, binary.LittleEndian, rep.TotalBlocksProduced)
	binary.Write(&buf, binary.LittleEndian, rep.TotalSlashes)
	binary.Write(&buf, binary.LittleEndian, rep.UpdatedAt)

	return buf.Bytes()
}

// DeserializeBlockReputationScore deserializes a reputation score from bytes.
func DeserializeBlockReputationScore(data []byte) (*BlockReputationScore, error) {
	rep := &BlockReputationScore{}
	buf := bytes.NewReader(data)

	if _, err := buf.Read(rep.NodeID[:]); err != nil {
		return nil, fmt.Errorf("failed to read node ID: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.TotalScore); err != nil {
		return nil, fmt.Errorf("failed to read total score: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.UserScore); err != nil {
		return nil, fmt.Errorf("failed to read user score: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.TechScore); err != nil {
		return nil, fmt.Errorf("failed to read tech score: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.HistoryScore); err != nil {
		return nil, fmt.Errorf("failed to read history score: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.StakeScore); err != nil {
		return nil, fmt.Errorf("failed to read stake score: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.TotalTasksCompleted); err != nil {
		return nil, fmt.Errorf("failed to read tasks completed: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.TotalBlocksProduced); err != nil {
		return nil, fmt.Errorf("failed to read blocks produced: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.TotalSlashes); err != nil {
		return nil, fmt.Errorf("failed to read slashes: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &rep.UpdatedAt); err != nil {
		return nil, fmt.Errorf("failed to read updated at: %w", err)
	}

	return rep, nil
}

// GetTotalBlockRewardV2 returns the total block reward (50 AIB).
func (b *BlockV2) GetTotalBlockRewardV2() uint64 {
	return BlockRewardV2Total
}

// GetStakingReward returns the staking reward portion (30 AIB).
func (b *BlockV2) GetStakingReward() uint64 {
	return StakingRewardAmount
}

// GetInferenceReward returns the inference reward portion (20 AIB).
func (b *BlockV2) GetInferenceReward() uint64 {
	return InferenceRewardAmount
}

// GetV2CoinbaseTransaction returns the coinbase transaction from v2 block.
func (b *BlockV2) GetV2CoinbaseTransaction() *Transaction {
	if len(b.Transactions) > 0 {
		return b.Transactions[0]
	}
	return nil
}

// ValidateCoinbaseDistributionV2 validates the v2 coinbase reward distribution.
func (b *BlockV2) ValidateCoinbaseDistributionV2() error {
	coinbase := b.GetV2CoinbaseTransaction()
	if coinbase == nil {
		return fmt.Errorf("no coinbase transaction")
	}

	if !coinbase.IsCoinbase() {
		return fmt.Errorf("first transaction is not coinbase")
	}

	// Check total value = 50 AIB
	totalValue := coinbase.TotalOutputValue()
	if totalValue != BlockRewardV2Total {
		return fmt.Errorf("coinbase value mismatch: expected %d, got %d",
			BlockRewardV2Total, totalValue)
	}

	// Check we have exactly 2 outputs
	if len(coinbase.Outputs) != 2 {
		return fmt.Errorf("expected 2 outputs, got %d", len(coinbase.Outputs))
	}

	// Check first output = 30 AIB (staking)
	if coinbase.Outputs[0].Value != StakingRewardAmount {
		return fmt.Errorf("staking reward mismatch: expected %d, got %d",
			StakingRewardAmount, coinbase.Outputs[0].Value)
	}

	// Check second output = 20 AIB (inference)
	if coinbase.Outputs[1].Value != InferenceRewardAmount {
		return fmt.Errorf("inference reward mismatch: expected %d, got %d",
			InferenceRewardAmount, coinbase.Outputs[1].Value)
	}

	return nil
}

// CreateBlockReputationScore creates a new reputation score with current timestamp.
func CreateBlockReputationScore(
	nodeID [32]byte,
	userScore, techScore, historyScore, stakeScore float64,
	totalTasksCompleted, totalBlocksProduced, totalSlashes uint64,
) *BlockReputationScore {
	return &BlockReputationScore{
		NodeID:              nodeID,
		UserScore:           userScore,
		TechScore:           techScore,
		HistoryScore:        historyScore,
		StakeScore:          stakeScore,
		TotalScore:          (userScore*0.30 + techScore*0.25 + historyScore*0.25 + stakeScore*0.20),
		TotalTasksCompleted: totalTasksCompleted,
		TotalBlocksProduced: totalBlocksProduced,
		TotalSlashes:        totalSlashes,
		UpdatedAt:           uint64(time.Now().Unix()),
	}
}
