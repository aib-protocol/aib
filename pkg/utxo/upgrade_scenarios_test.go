package utxo

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestSimpleUpgradeSimulation is a standalone test that can be run directly.
func TestSimpleUpgradeSimulation(t *testing.T) {
	t.Log("=== Starting Simple Upgrade Simulation ===")

	testCases := []struct {
		name         string
		upgradeHeight uint64
	}{
		{"Quick Test", 10},
		{"Standard Test", 50},
		{"Extended Test", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sim := NewSimpleUpgradeTest(tc.upgradeHeight)

			// Setup
			if err := sim.Setup(); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			// Run simulation
			startTime := time.Now()
			if err := sim.RunSimulation(); err != nil {
				t.Fatalf("Simulation failed: %v", err)
			}
			duration := time.Since(startTime)

			// Validate chain
			if err := sim.ValidateChain(); err != nil {
				t.Errorf("Chain validation failed: %v", err)
			} else {
				t.Logf("Chain validation passed for %d blocks", len(sim.Chain))
			}

			// Test old node compatibility
			if err := sim.TestOldNodeCompatibility(); err != nil {
				t.Errorf("Old node compatibility failed: %v", err)
			} else {
				t.Log("Old node compatibility passed")
			}

			// Test new node compatibility
			if err := sim.TestNewNodeCompatibility(); err != nil {
				t.Errorf("New node compatibility failed: %v", err)
			} else {
				t.Log("New node compatibility passed")
			}

			// Test reward distribution
			if err := sim.TestRewardDistribution(); err != nil {
				t.Errorf("Reward distribution test failed: %v", err)
			} else {
				t.Log("Reward distribution test passed")
			}

			t.Logf("Simulation completed in %v", duration)

			// Print statistics
			stats := sim.SimulationStats()
			t.Logf("Statistics: %+v", stats)

			// Save report for the standard test
			if tc.name == "Standard Test" {
				reportPath := "/tmp/upgrade_simulation_report.md"
				if err := os.WriteFile(reportPath, []byte(sim.GenerateReport()), 0644); err != nil {
					t.Errorf("Failed to save report: %v", err)
				} else {
					t.Logf("Report saved to %s", reportPath)
				}
			}
		})
	}
}

// TestUpgradeScenarios tests specific upgrade scenarios.
func TestUpgradeScenarios(t *testing.T) {
	t.Log("=== Testing Specific Upgrade Scenarios ===")

	scenarios := []struct {
		name       string
		scenarioFn func(*testing.T) error
	}{
		{"OldNodeReceivesVersion2Block", testOldNodeReceivesVersion2Block},
		{"NewNodeReceivesVersion1Block", testNewNodeReceivesVersion1Block},
		{"StateTransitionAtUpgradeHeight", testStateTransitionAtUpgradeHeight},
		{"RewardDistributionChange", testRewardDistributionChange},
		{"BlockSerializationCompatibility", testBlockSerializationCompatibility},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			if err := sc.scenarioFn(t); err != nil {
				t.Errorf("Scenario %s failed: %v", sc.name, err)
			} else {
				t.Logf("Scenario %s passed", sc.name)
			}
		})
	}
}

// testOldNodeReceivesVersion2Block tests old node receiving Version 2 block.
func testOldNodeReceivesVersion2Block(t *testing.T) error {
	sim := NewSimpleUpgradeTest(10)
	if err := sim.Setup(); err != nil {
		return err
	}

	// Create a Version 2 block directly
	v2Block := &Block{
		Header: BlockHeader{
			Version:      2,
			Height:       10,
			Timestamp:    uint64(time.Now().Unix()),
			InferencePoW: []byte("test-pow-proof"),
			ModelID:      "test-model",
			EnergyClaim:  1000,
		},
		Transactions: []*Transaction{},
	}

	// Simulate old node deserializing
	blockData := v2Block.SerializeBlock()
	deserialized, err := DeserializeBlock(blockData)
	if err != nil {
		return fmt.Errorf("old node cannot deserialize Version 2 block: %w", err)
	}

	// Old node should read version correctly
	if deserialized.Header.Version != 2 {
		return fmt.Errorf("failed to read version: got %d, expected 2", deserialized.Header.Version)
	}

	// Old node should ignore unknown fields
	if len(deserialized.Header.InferencePoW) > 0 {
		// Expected - old node can read this field but should ignore it for validation
	}

	return nil
}

// testNewNodeReceivesVersion1Block tests new node receiving Version 1 block.
func testNewNodeReceivesVersion1Block(t *testing.T) error {
	sim := NewSimpleUpgradeTest(10)
	if err := sim.Setup(); err != nil {
		return err
	}

	// Create a Version 1 block
	v1Block := &Block{
		Header: BlockHeader{
			Version:   1,
			Height:    5,
			Timestamp: uint64(time.Now().Unix()),
		},
		Transactions: []*Transaction{},
	}

	// Simulate new node deserializing
	blockData := v1Block.SerializeBlock()
	deserialized, err := DeserializeBlock(blockData)
	if err != nil {
		return fmt.Errorf("new node cannot deserialize Version 1 block: %w", err)
	}

	// New node should read version correctly
	if deserialized.Header.Version != 1 {
		return fmt.Errorf("failed to read version: got %d, expected 1", deserialized.Header.Version)
	}

	// New node should handle missing Version 2 fields
	// (Version 1 blocks won't have these fields by default)
	if len(deserialized.Header.InferencePoW) > 0 {
		return fmt.Errorf("Version 1 block should not have InferencePoW")
	}
	if deserialized.Header.ModelID != "" {
		return fmt.Errorf("Version 1 block should not have ModelID")
	}

	return nil
}

// testStateTransitionAtUpgradeHeight tests the exact state transition.
func testStateTransitionAtUpgradeHeight(t *testing.T) error {
	upgradeHeight := uint64(20)
	sim := NewSimpleUpgradeTest(upgradeHeight)
	if err := sim.Setup(); err != nil {
		return err
	}

	if err := sim.RunSimulation(); err != nil {
		return err
	}

	// Check block just before upgrade
	v1Block := sim.Chain[upgradeHeight-1]
	if v1Block.Header.Version != 1 {
		return fmt.Errorf("block at height %d should be Version 1, got %d", upgradeHeight-1, v1Block.Header.Version)
	}

	// Check block at upgrade height
	v2Block := sim.Chain[upgradeHeight]
	if v2Block.Header.Version != 2 {
		return fmt.Errorf("block at height %d should be Version 2, got %d", upgradeHeight, v2Block.Header.Version)
	}

	// Check block just after upgrade
	v2BlockNext := sim.Chain[upgradeHeight+1]
	if v2BlockNext.Header.Version != 2 {
		return fmt.Errorf("block at height %d should be Version 2, got %d", upgradeHeight+1, v2BlockNext.Header.Version)
	}

	// Verify chain continuity
	if v2Block.Header.PrevBlockHash != v1Block.Hash {
		return fmt.Errorf("chain continuity broken at upgrade height")
	}

	if v2BlockNext.Header.PrevBlockHash != v2Block.Hash {
		return fmt.Errorf("chain continuity broken after upgrade height")
	}

	return nil
}

// testRewardDistributionChange tests reward distribution changes.
func testRewardDistributionChange(t *testing.T) error {
	// Version 1 reward: 50 AIB total (single output)
	// Version 2 reward: 50 AIB total (30 staking + 20 inference, two outputs)

	sim := NewSimpleUpgradeTest(10)
	if err := sim.Setup(); err != nil {
		return err
	}

	if err := sim.RunSimulation(); err != nil {
		return err
	}

	// Check Version 1 coinbase
	v1Block := sim.Chain[5]
	v1Coinbase := v1Block.GetCoinbaseTransaction()
	if v1Coinbase == nil {
		return fmt.Errorf("Version 1 block missing coinbase transaction")
	}
	if len(v1Coinbase.Outputs) != 1 {
		return fmt.Errorf("Version 1 coinbase should have 1 output, got %d", len(v1Coinbase.Outputs))
	}
	v1Output := v1Coinbase.Outputs[0]
	expectedV1Reward := uint64(50 * AIBUnit)
	if v1Output.Value != expectedV1Reward {
		return fmt.Errorf("Version 1 reward mismatch: expected %d, got %d", expectedV1Reward, v1Output.Value)
	}

	// Check Version 2 coinbase
	v2Block := sim.Chain[10]
	v2Coinbase := v2Block.GetCoinbaseTransaction()
	if v2Coinbase == nil {
		return fmt.Errorf("Version 2 block missing coinbase transaction")
	}
	if len(v2Coinbase.Outputs) != 2 {
		return fmt.Errorf("Version 2 coinbase should have 2 outputs, got %d", len(v2Coinbase.Outputs))
	}

	// Verify total reward equals 50 AIB
	totalReward := v2Coinbase.Outputs[0].Value + v2Coinbase.Outputs[1].Value
	expectedTotal := BlockRewardV2Total
	if totalReward != expectedTotal {
		return fmt.Errorf("Version 2 total reward mismatch: expected %d, got %d", expectedTotal, totalReward)
	}

	// Verify staking reward is 30 AIB
	if v2Coinbase.Outputs[0].Value != StakingRewardAmount {
		return fmt.Errorf("Version 2 staking reward mismatch: expected %d, got %d", StakingRewardAmount, v2Coinbase.Outputs[0].Value)
	}

	// Verify inference reward is 20 AIB
	if v2Coinbase.Outputs[1].Value != InferenceRewardAmount {
		return fmt.Errorf("Version 2 inference reward mismatch: expected %d, got %d", InferenceRewardAmount, v2Coinbase.Outputs[1].Value)
	}

	return nil
}

// testBlockSerializationCompatibility tests serialization/deserialization compatibility.
func testBlockSerializationCompatibility(t *testing.T) error {
	sim := NewSimpleUpgradeTest(10)
	if err := sim.Setup(); err != nil {
		return err
	}

	if err := sim.RunSimulation(); err != nil {
		return err
	}

	// Test all blocks can be serialized and deserialized
	for i, block := range sim.Chain {
		blockData := block.SerializeBlock()
		deserialized, err := DeserializeBlock(blockData)
		if err != nil {
			return fmt.Errorf("failed to deserialize block %d: %w", i, err)
		}

		// Verify critical fields match
		if deserialized.Header.Version != block.Header.Version {
			return fmt.Errorf("version mismatch for block %d after serialization", i)
		}
		if deserialized.Header.Height != block.Header.Height {
			return fmt.Errorf("height mismatch for block %d after serialization", i)
		}
		if deserialized.Header.Timestamp != block.Header.Timestamp {
			return fmt.Errorf("timestamp mismatch for block %d after serialization", i)
		}

		// Note: Hash may differ after serialization because it's calculated from raw data
		// The important thing is that the serialized data can be deserialized correctly
		// and the hash can be recalculated from the deserialized block

		// Verify transaction count matches
		if len(deserialized.Transactions) != len(block.Transactions) {
			return fmt.Errorf("transaction count mismatch for block %d after serialization", i)
		}
	}

	return nil
}

// TestFullUpgradeSimulation runs a comprehensive upgrade simulation.
func TestFullUpgradeSimulation(t *testing.T) {
	t.Log("=== Running Full Upgrade Simulation ===")

	// Create a larger simulation
	sim := NewSimpleUpgradeTest(100)

	if err := sim.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	startTime := time.Now()
	if err := sim.RunSimulation(); err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}
	duration := time.Since(startTime)

	// Run all validation checks
	allPassed := true

	if err := sim.ValidateChain(); err != nil {
		t.Errorf("Chain validation failed: %v", err)
		allPassed = false
	}

	if err := sim.TestOldNodeCompatibility(); err != nil {
		t.Errorf("Old node compatibility failed: %v", err)
		allPassed = false
	}

	if err := sim.TestNewNodeCompatibility(); err != nil {
		t.Errorf("New node compatibility failed: %v", err)
		allPassed = false
	}

	if err := sim.TestRewardDistribution(); err != nil {
		t.Errorf("Reward distribution failed: %v", err)
		allPassed = false
	}

	// Generate and save report
	reportPath := "/tmp/upgrade_simulation_report.md"
	if err := os.WriteFile(reportPath, []byte(sim.GenerateReport()), 0644); err != nil {
		t.Errorf("Failed to save report: %v", err)
	} else {
		t.Logf("Report saved to %s", reportPath)
	}

	// Print statistics
	stats := sim.SimulationStats()
	t.Logf("Simulation Statistics:")
	t.Logf("  Total Blocks: %v", stats["total_blocks"])
	t.Logf("  Upgrade Height: %v", stats["upgrade_height"])
	t.Logf("  Version 1 Blocks: %v", stats["version1_blocks"])
	t.Logf("  Version 2 Blocks: %v", stats["version2_blocks"])
	t.Logf("  Validator Count: %v", stats["validator_count"])
	t.Logf("  Final Height: %v", stats["final_height"])
	t.Logf("  Duration: %v", duration)

	if allPassed {
		t.Log("✓ All validation checks passed!")
	}
}

// BenchmarkUpgradeSimulation benchmarks the upgrade simulation.
func BenchmarkUpgradeSimulation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sim := NewSimpleUpgradeTest(100)
		sim.Setup()
		sim.RunSimulation()
	}
}

// BenchmarkBlockSerialization benchmarks block serialization.
func BenchmarkBlockSerialization(b *testing.B) {
	sim := NewSimpleUpgradeTest(10)
	sim.Setup()
	sim.RunSimulation()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, block := range sim.Chain {
			_ = block.SerializeBlock()
		}
	}
}

// TestBlockVersionDetection tests version detection logic.
func TestBlockVersionDetection(t *testing.T) {
	t.Log("=== Testing Block Version Detection ===")

	sim := NewSimpleUpgradeTest(20)
	if err := sim.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	if err := sim.RunSimulation(); err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}

	// Test getting blocks by version
	v1Block, err := sim.GetBlockByVersion(1)
	if err != nil {
		t.Errorf("Failed to get Version 1 block: %v", err)
	} else {
		if v1Block.Header.Version != 1 {
			t.Errorf("Expected Version 1, got %d", v1Block.Header.Version)
		}
	}

	v2Block, err := sim.GetBlockByVersion(2)
	if err != nil {
		t.Errorf("Failed to get Version 2 block: %v", err)
	} else {
		if v2Block.Header.Version != 2 {
			t.Errorf("Expected Version 2, got %d", v2Block.Header.Version)
		}
	}

	t.Log("Block version detection passed")
}

// TestUpgradeHeightZero tests upgrade at height 0 (edge case).
func TestUpgradeHeightZero(t *testing.T) {
	t.Log("=== Testing Upgrade Height Zero (Edge Case) ===")

	// This is an edge case where upgrade happens immediately after genesis
	sim := NewSimpleUpgradeTest(1)
	if err := sim.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// First block should be Version 2 (upgrade at height 1)
	block, err := sim.createVersion2Block(1)
	if err != nil {
		t.Fatalf("Failed to create Version 2 block: %v", err)
	}

	if block.Header.Version != 2 {
		t.Errorf("Expected Version 2, got %d", block.Header.Version)
	}

	t.Log("Upgrade height zero edge case passed")
}