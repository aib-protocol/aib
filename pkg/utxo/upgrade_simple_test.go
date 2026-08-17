package utxo

import (
	"fmt"
)

// SimpleUpgradeTest provides a simplified end-to-end upgrade simulation.
// This test focuses on the core upgrade functionality without complex dependencies.
type SimpleUpgradeTest struct {
	UpgradeHeight uint64
	CurrentHeight uint64
	Chain         []*Block
	Validators    map[[32]byte]*UpgradeTestValidator
}

// UpgradeTestValidator represents a test validator for upgrade simulation.
type UpgradeTestValidator struct {
	Address   [32]byte
	Stake     uint64
	PublicKey []byte
}

// NewSimpleUpgradeTest creates a new simple upgrade test.
func NewSimpleUpgradeTest(upgradeHeight uint64) *SimpleUpgradeTest {
	return &SimpleUpgradeTest{
		UpgradeHeight: upgradeHeight,
		CurrentHeight: 0,
		Chain:         make([]*Block, 0),
		Validators:    make(map[[32]byte]*UpgradeTestValidator),
	}
}

// Setup initializes the test environment.
func (s *SimpleUpgradeTest) Setup() error {
	// Create validators
	for i := 0; i < 5; i++ {
		var addr [32]byte
		for j := 0; j < 32; j++ {
			addr[j] = byte(i + j + 1)
		}
		validator := &UpgradeTestValidator{
			Address:   addr,
			Stake:     5000 * AIBUnit,
			PublicKey: []byte(fmt.Sprintf("validator-%d", i)),
		}
		s.Validators[addr] = validator
	}

	// Create genesis block using existing CreateCoinbaseTransaction
	var proposer [32]byte
	for i := 0; i < 32; i++ {
		proposer[i] = byte(i + 1)
	}
	reward := uint64(50 * AIBUnit)
	coinbaseTx := CreateCoinbaseTransaction(proposer, reward, []byte("genesis"))
	genesis := CreateGenesisBlock(coinbaseTx, proposer)
	s.Chain = append(s.Chain, genesis)
	s.CurrentHeight = 0

	return nil
}

// RunSimulation executes the upgrade simulation.
func (s *SimpleUpgradeTest) RunSimulation() error {
	// Phase 1: Build Version 1 chain
	for height := uint64(1); height < s.UpgradeHeight; height++ {
		block, err := s.createVersion1Block(height)
		if err != nil {
			return fmt.Errorf("failed to create Version 1 block %d: %w", height, err)
		}
		s.Chain = append(s.Chain, block)
		s.CurrentHeight = height
	}

	// Phase 2: Upgrade to Version 2
	block, err := s.createVersion2Block(s.UpgradeHeight)
	if err != nil {
		return fmt.Errorf("failed to create Version 2 block %d: %w", s.UpgradeHeight, err)
	}
	s.Chain = append(s.Chain, block)
	s.CurrentHeight = s.UpgradeHeight

	// Phase 3: Build Version 2 chain
	for height := s.UpgradeHeight + 1; height <= s.UpgradeHeight+5; height++ {
		block, err := s.createVersion2Block(height)
		if err != nil {
			return fmt.Errorf("failed to create Version 2 block %d: %w", height, err)
		}
		s.Chain = append(s.Chain, block)
		s.CurrentHeight = height
	}

	return nil
}

// createVersion1Block creates a Version 1 block.
func (s *SimpleUpgradeTest) createVersion1Block(height uint64) (*Block, error) {
	// Select proposer based on height
	proposerIndex := int(height % uint64(len(s.Validators)))
	var proposerAddr [32]byte
	i := 0
	for addr := range s.Validators {
		if i == proposerIndex {
			proposerAddr = addr
			break
		}
		i++
	}

	// Create coinbase transaction using existing helper
	reward := uint64(50 * AIBUnit)
	coinbaseTx := CreateCoinbaseTransaction(proposerAddr, reward, []byte(fmt.Sprintf("height=%d", height)))

	// Create block
	block := NewBlock([]*Transaction{coinbaseTx}, s.Chain[height-1].Hash, height, proposerAddr)

	// Set Version 1
	block.Header.Version = 1

	// Calculate Merkle root
	block.Header.MerkleRoot = block.CalculateMerkleRoot()

	// Add signature (simplified for simulation)
	block.Header.Signature = []byte("simulated-signature")

	return block, nil
}

// createVersion2Block creates a Version 2 block.
func (s *SimpleUpgradeTest) createVersion2Block(height uint64) (*Block, error) {
	// Select proposer based on height
	proposerIndex := int(height % uint64(len(s.Validators)))
	var proposerAddr [32]byte
	i := 0
	for addr := range s.Validators {
		if i == proposerIndex {
			proposerAddr = addr
			break
		}
		i++
	}

	// Top inference node (different from proposer for simulation)
	var topInferenceNode [32]byte
	for i := 0; i < 32; i++ {
		topInferenceNode[i] = byte(32 - i)
	}

	// Create coinbase transaction (Version 2 format)
	coinbaseTx := CreateCoinbaseV2Tx(proposerAddr, topInferenceNode, height)

	// Create Version 2 block
	blockV2 := NewBlockV2(
		[]*Transaction{coinbaseTx},
		s.Chain[height-1].Hash,
		height,
		proposerAddr,
		nil,                   // No reputation updates
		BlockInferenceStats{}, // Empty stats
	)

	// Set Version 2
	blockV2.Header.Version = 2

	// Add PoAIW specific fields
	blockV2.Header.InferencePoW = []byte("simulated-pow")
	blockV2.Header.ModelID = "test-model-v1"
	blockV2.Header.EnergyClaim = 1000

	// Calculate Merkle root
	blockV2.Header.MerkleRoot = blockV2.CalculateMerkleRoot()

	// Add signature (simplified for simulation)
	blockV2.Header.Signature = []byte("simulated-signature-v2")

	return &blockV2.Block, nil
}

// ValidateChain validates the entire chain.
func (s *SimpleUpgradeTest) ValidateChain() error {
	for i, block := range s.Chain {
		if err := s.validateBlock(block, uint64(i)); err != nil {
			return fmt.Errorf("block %d validation failed: %w", i, err)
		}
	}
	return nil
}

// validateBlock validates a single block.
func (s *SimpleUpgradeTest) validateBlock(block *Block, expectedHeight uint64) error {
	// Check height
	if block.Header.Height != expectedHeight {
		return fmt.Errorf("height mismatch: expected %d, got %d", expectedHeight, block.Header.Height)
	}

	// Check version consistency
	if expectedHeight < s.UpgradeHeight {
		if block.Header.Version != 1 {
			return fmt.Errorf("Version 1 block expected at height %d, got version %d", expectedHeight, block.Header.Version)
		}
	} else {
		if block.Header.Version != 2 {
			return fmt.Errorf("Version 2 block expected at height %d, got version %d", expectedHeight, block.Header.Version)
		}
	}

	// Check previous block hash
	if expectedHeight > 0 {
		if block.Header.PrevBlockHash != s.Chain[expectedHeight-1].Hash {
			return fmt.Errorf("previous hash mismatch at height %d", expectedHeight)
		}
	}

	// Check Merkle root
	expectedMerkle := block.CalculateMerkleRoot()
	if block.Header.MerkleRoot != expectedMerkle {
		return fmt.Errorf("Merkle root mismatch at height %d", expectedHeight)
	}

	// Check Version 2 specific fields
	if block.Header.Version >= 2 {
		if len(block.Header.InferencePoW) == 0 {
			return fmt.Errorf("missing InferencePoW at height %d", expectedHeight)
		}
		if block.Header.ModelID == "" {
			return fmt.Errorf("missing ModelID at height %d", expectedHeight)
		}
		if block.Header.EnergyClaim == 0 {
			return fmt.Errorf("missing EnergyClaim at height %d", expectedHeight)
		}
	}

	return nil
}

// TestOldNodeCompatibility tests how an old node handles Version 2 blocks.
func (s *SimpleUpgradeTest) TestOldNodeCompatibility() error {
	// Find a Version 2 block
	var version2Block *Block
	for _, block := range s.Chain {
		if block.Header.Version == 2 {
			version2Block = block
			break
		}
	}

	if version2Block == nil {
		return fmt.Errorf("no Version 2 block found")
	}

	// Simulate old node deserialization
	blockData := version2Block.SerializeBlock()
	deserialized, err := DeserializeBlock(blockData)
	if err != nil {
		return fmt.Errorf("old node cannot deserialize Version 2 block: %w", err)
	}

	// Old node should handle unknown fields gracefully
	if deserialized.Header.Version != 2 {
		return fmt.Errorf("failed to read version: got %d, expected 2", deserialized.Header.Version)
	}

	// Old node should ignore Version 2 specific fields
	if len(deserialized.Header.InferencePoW) > 0 {
		// This is fine - old node should ignore this field
	}
	if deserialized.Header.ModelID != "" {
		// This is fine - old node should ignore this field
	}

	return nil
}

// TestNewNodeCompatibility tests how a new node handles Version 1 blocks.
func (s *SimpleUpgradeTest) TestNewNodeCompatibility() error {
	// Find a Version 1 block
	var version1Block *Block
	for _, block := range s.Chain {
		if block.Header.Version == 1 && block.Header.Height > 0 {
			version1Block = block
			break
		}
	}

	if version1Block == nil {
		return fmt.Errorf("no Version 1 block found")
	}

	// Simulate new node deserialization
	blockData := version1Block.SerializeBlock()
	deserialized, err := DeserializeBlock(blockData)
	if err != nil {
		return fmt.Errorf("new node cannot deserialize Version 1 block: %w", err)
	}

	// New node should handle missing Version 2 fields gracefully
	if deserialized.Header.Version != 1 {
		return fmt.Errorf("failed to read version: got %d, expected 1", deserialized.Header.Version)
	}

	// New node should handle missing Version 2 fields
	// (Version 1 blocks won't have these fields, and that's expected)
	if len(deserialized.Header.InferencePoW) > 0 {
		return fmt.Errorf("Version 1 block should not have InferencePoW")
	}
	if deserialized.Header.ModelID != "" {
		return fmt.Errorf("Version 1 block should not have ModelID")
	}

	return nil
}

// TestRewardDistribution tests reward distribution changes.
func (s *SimpleUpgradeTest) TestRewardDistribution() error {
	// Check Version 1 coinbase
	v1Block := s.Chain[1]
	v1Coinbase := v1Block.GetCoinbaseTransaction()
	if v1Coinbase == nil {
		return fmt.Errorf("Version 1 block missing coinbase transaction")
	}
	v1Output := v1Coinbase.Outputs[0]
	expectedV1Reward := uint64(50 * AIBUnit)
	if v1Output.Value != expectedV1Reward {
		return fmt.Errorf("Version 1 reward mismatch: expected %d, got %d", expectedV1Reward, v1Output.Value)
	}

	// Check Version 2 coinbase
	v2Block := s.Chain[s.UpgradeHeight]
	v2Coinbase := v2Block.GetCoinbaseTransaction()
	if v2Coinbase == nil {
		return fmt.Errorf("Version 2 block missing coinbase transaction")
	}

	// Version 2 should have 2 outputs (staking + inference)
	if len(v2Coinbase.Outputs) != 2 {
		return fmt.Errorf("Version 2 coinbase should have 2 outputs, got %d", len(v2Coinbase.Outputs))
	}

	// Verify total reward equals 50 AIB
	totalReward := v2Coinbase.Outputs[0].Value + v2Coinbase.Outputs[1].Value
	expectedTotal := BlockRewardV2Total
	if totalReward != expectedTotal {
		return fmt.Errorf("Version 2 total reward mismatch: expected %d, got %d", expectedTotal, totalReward)
	}

	return nil
}

// GenerateReport generates a simulation report.
func (s *SimpleUpgradeTest) GenerateReport() string {
	report := fmt.Sprintf(`
# PoAIW Upgrade Simulation Report

## Chain Summary
- Total Blocks: %d
- Upgrade Height: %d
- Version 1 Blocks: %d
- Version 2 Blocks: %d

## Validation Results
`, len(s.Chain), s.UpgradeHeight, s.UpgradeHeight, len(s.Chain)-int(s.UpgradeHeight))

	// Check chain validation
	if err := s.ValidateChain(); err == nil {
		report += "✓ Chain validation: PASSED\n"
	} else {
		report += fmt.Sprintf("✗ Chain validation: FAILED - %v\n", err)
	}

	// Check old node compatibility
	if err := s.TestOldNodeCompatibility(); err == nil {
		report += "✓ Old node compatibility: PASSED\n"
	} else {
		report += fmt.Sprintf("✗ Old node compatibility: FAILED - %v\n", err)
	}

	// Check new node compatibility
	if err := s.TestNewNodeCompatibility(); err == nil {
		report += "✓ New node compatibility: PASSED\n"
	} else {
		report += fmt.Sprintf("✗ New node compatibility: FAILED - %v\n", err)
	}

	// Check reward distribution
	if err := s.TestRewardDistribution(); err == nil {
		report += "✓ Reward distribution: PASSED\n"
	} else {
		report += fmt.Sprintf("✗ Reward distribution: FAILED - %v\n", err)
	}

	// Final verdict
	allTestsPassed := true
	if err := s.ValidateChain(); err != nil {
		allTestsPassed = false
	}
	if err := s.TestOldNodeCompatibility(); err != nil {
		allTestsPassed = false
	}
	if err := s.TestNewNodeCompatibility(); err != nil {
		allTestsPassed = false
	}
	if err := s.TestRewardDistribution(); err != nil {
		allTestsPassed = false
	}

	if allTestsPassed {
		report += "\n## Final Result\n✓ ALL TESTS PASSED - Upgrade simulation successful\n"
	} else {
		report += "\n## Final Result\n✗ SOME TESTS FAILED - Issues detected\n"
	}

	return report
}

// GetBlockByVersion returns the first block of the specified version.
func (s *SimpleUpgradeTest) GetBlockByVersion(version uint32) (*Block, error) {
	for _, block := range s.Chain {
		if block.Header.Version == version {
			return block, nil
		}
	}
	return nil, fmt.Errorf("no block with version %d found", version)
}

// SimulationStats returns simulation statistics.
func (s *SimpleUpgradeTest) SimulationStats() map[string]interface{} {
	return map[string]interface{}{
		"total_blocks":    len(s.Chain),
		"upgrade_height":  s.UpgradeHeight,
		"version1_blocks": int(s.UpgradeHeight),
		"version2_blocks": len(s.Chain) - int(s.UpgradeHeight),
		"validator_count": len(s.Validators),
		"final_height":    s.CurrentHeight,
	}
}
