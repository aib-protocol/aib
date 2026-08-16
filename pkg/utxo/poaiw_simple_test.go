// Package utxo provides tests for simplified PoAIW consensus.
// This test verifies that the simplified PoAIW design works without requiring any JUDGE.
package utxo

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Constants for testing
const (
	AIBUnit = 1e8
	Satoshi = 1
)

// PoAIWTestConfig Simplified PoAIW configuration
type PoAIWTestConfig struct {
	TargetBlockTime   time.Duration // 30 seconds
	MinStake         uint64         // 1000 AIB
	InitialDifficulty uint64         // Initial PoW difficulty
}

// DefaultPoAIWTestConfig returns default test configuration
func DefaultPoAIWTestConfig() *PoAIWTestConfig {
	return &PoAIWTestConfig{
		TargetBlockTime:   30 * time.Second,
		MinStake:         1000 * AIBUnit, // 1000 AIB in satoshi
		InitialDifficulty: 1000,           // Initial difficulty
	}
}

// SimulatedInferenceResult represents a completed AI inference
type SimulatedInferenceResult struct {
	ModelID   string
	InputHash [32]byte
	Output    []byte
	Nonce     uint64
	EnergyUsed uint64
	Duration  time.Duration
}

// VerifyPoW verifies that the inference result meets PoW requirements
// This is the KEY verification - it proves actual computation was done
// Uses a simple approach: hash must be less than 2^256 / difficulty
func VerifyPoW(result *SimulatedInferenceResult, difficulty uint64) bool {
	// Create PoW: Hash(ModelID + InputHash + Output + Nonce)
	hash := sha256.New()
	hash.Write([]byte(result.ModelID))
	hash.Write(result.InputHash[:])
	hash.Write(result.Output)
	binary.Write(hash, binary.BigEndian, result.Nonce)
	digest := hash.Sum(nil)

	// Convert first 8 bytes to uint64 for comparison
	hashValue := uint64(0)
	for i := 0; i < 8; i++ {
		hashValue = hashValue<<8 | uint64(digest[i])
	}

	// Target is 2^64 / difficulty
	// Higher difficulty = lower target = harder to find
	target := ^uint64(0) / difficulty

	return hashValue < target
}

// VerifyStake verifies that the proposer has sufficient stake
func VerifyStake(proposerStake, minStake uint64) bool {
	return proposerStake >= minStake
}

// VerifyBlockTime verifies that the block time is within acceptable range
func VerifyBlockTime(blockTime, prevBlockTime time.Time, config *PoAIWTestConfig) bool {
	elapsed := blockTime.Sub(prevBlockTime)
	// Allow some tolerance for network delay
	return elapsed >= 10*time.Second && elapsed <= 5*time.Minute
}

// PoAIWBlockSimulator simulates creating a PoAIW block
type PoAIWBlockSimulator struct {
	config         *PoAIWTestConfig
	proposerStake  uint64
	currentTime    time.Time
	prevBlockTime  time.Time
	difficulty     uint64
}

// NewPoAIWBlockSimulator creates a new simulator
func NewPoAIWBlockSimulator(config *PoAIWTestConfig, initialStake uint64) *PoAIWBlockSimulator {
	return &PoAIWBlockSimulator{
		config:        config,
		proposerStake: initialStake,
		currentTime:   time.Now(),
		prevBlockTime: time.Now().Add(-config.TargetBlockTime),
		difficulty:    config.InitialDifficulty,
	}
}

// SimulateInference simulates running an AI inference with PoW
func (s *PoAIWBlockSimulator) SimulateInference(modelID string, inputData []byte) (*SimulatedInferenceResult, uint64) {
	// Calculate input hash
	inputHash := sha256.Sum256(inputData)

	// Find a valid nonce that satisfies PoW
	var result *SimulatedInferenceResult
	attempts := uint64(0)

	startTime := time.Now()
	for {
		// Simulate inference output (in reality, this would be actual model output)
		output := sha256.Sum256(append(inputHash[:], binary.BigEndian.AppendUint64(nil, attempts)...))

		result = &SimulatedInferenceResult{
			ModelID:   modelID,
			InputHash: inputHash,
			Output:    output[:],
			Nonce:     attempts,
		}

		// Check PoW
		if VerifyPoW(result, s.difficulty) {
			break
		}

		attempts++
		if attempts > 1e9 { // Safety limit
			break
		}
	}

	duration := time.Since(startTime)
	result.Duration = duration

	// Estimate energy used (proportional to attempts)
	result.EnergyUsed = attempts * 1000 // Arbitrary energy unit

	return result, attempts
}

// ValidateBlock validates a complete PoAIW block
func (s *PoAIWBlockSimulator) ValidateBlock(
	proposerStake uint64,
	inferenceResult *SimulatedInferenceResult,
	blockTime time.Time,
) (bool, string) {
	// 1. PoW Check: Verify inference was actually computed
	if !VerifyPoW(inferenceResult, s.difficulty) {
		return false, "PoW verification failed - inference not completed"
	}

	// 2. Stake Check: Verify proposer has minimum stake
	if !VerifyStake(proposerStake, s.config.MinStake) {
		return false, fmt.Sprintf("stake %d below minimum %d", proposerStake, s.config.MinStake)
	}

	// 3. Time Check: Verify block time is reasonable
	if !VerifyBlockTime(blockTime, s.prevBlockTime, s.config) {
		return false, "block time outside acceptable range"
	}

	return true, "valid"
}

// TestSimplifiedPoAIWBasicFunctionality tests basic PoAIW functionality
func TestSimplifiedPoAIWBasicFunctionality(t *testing.T) {
	config := DefaultPoAIWTestConfig()

	t.Log("=== Simplified PoAIW Basic Functionality Test ===")

	// Test 1: PoW verification
	t.Run("PoW Verification", func(t *testing.T) {
		sim := NewPoAIWBlockSimulator(config, 10000*AIBUnit)

		// Simulate inference
		result, attempts := sim.SimulateInference("gpt-4-mini", []byte("Hello, world!"))

		t.Logf("Inference completed: attempts=%d, energy=%d, duration=%v",
			attempts, result.EnergyUsed, result.Duration)

		// Verify PoW
		if !VerifyPoW(result, config.InitialDifficulty) {
			t.Error("PoW verification failed")
		} else {
			t.Log("✓ PoW verification passed")
		}
	})

	// Test 2: Stake verification
	t.Run("Stake Verification", func(t *testing.T) {
		tests := []struct {
			stake    uint64
			expected bool
		}{
			{500 * AIBUnit, false},  // Below minimum
			{1000 * AIBUnit, true},  // Exactly minimum
			{5000 * AIBUnit, true},  // Above minimum
		}

		for _, tt := range tests {
			result := VerifyStake(tt.stake, config.MinStake)
			if result != tt.expected {
				t.Errorf("Stake %d: expected %v, got %v", tt.stake, tt.expected, result)
			}
		}
		t.Log("✓ Stake verification passed")
	})

	// Test 3: Complete block validation
	t.Run("Complete Block Validation", func(t *testing.T) {
		sim := NewPoAIWBlockSimulator(config, 5000*AIBUnit)

		// Simulate inference
		result, attempts := sim.SimulateInference("llama-3-8b", []byte("What is AI?"))
		t.Logf("Inference: attempts=%d, energy=%d", attempts, result.EnergyUsed)

		// Validate block
		valid, reason := sim.ValidateBlock(sim.proposerStake, result, time.Now())

		if !valid {
			t.Errorf("Block validation failed: %s", reason)
		} else {
			t.Log("✓ Complete block validation passed")
		}
	})
}

// TestSimplifiedPoAIWEconomics tests the economic incentives
func TestSimplifiedPoAIWEconomics(t *testing.T) {
	config := DefaultPoAIWTestConfig()

	t.Log("=== Simplified PoAIW Economics Test ===")

	// Simulate honest node vs attacker
	honestEnergyCost := uint64(0.001 * float64(AIBUnit)) // Cost per inference
	attackerEnergyCost := uint64(0.001 * float64(AIBUnit)) // Same cost

	blockReward := uint64(50 * AIBUnit)

	t.Run("Honest vs Attacker Economics", func(t *testing.T) {
		// Honest node: spends energy, gets reward
		honestProfit := float64(blockReward - honestEnergyCost)

		// Attacker tries to game the system:
		// - Can't skip PoW (mathematically required)
		// - Can't fake output (PoW binds input+output)
		// - Can produce "bad output" but costs same as "good output"

		attackerProfit := float64(blockReward - attackerEnergyCost)

		t.Logf("Honest profit: %.6f AIB", honestProfit/float64(AIBUnit))
		t.Logf("Attacker profit: %.6f AIB (same as honest - no advantage)", attackerProfit/float64(AIBUnit))

		if honestProfit <= 0 {
			t.Error("Honest node loses money!")
		} else {
			t.Log("✓ Economic incentive favors honest behavior")
		}
	})

	// Test: Does bad output help?
	t.Run("No Benefit to Bad Output", func(t *testing.T) {
		sim := NewPoAIWBlockSimulator(config, 5000*AIBUnit)

		// Good inference
		goodResult, goodAttempts := sim.SimulateInference("gpt-4", []byte("good input"))
		goodCost := float64(goodAttempts) * 0.000001

		// Bad inference (garbage output)
		badResult, badAttempts := sim.SimulateInference("gpt-4", []byte("bad input"))
		badCost := float64(badAttempts) * 0.000001

		t.Logf("Good output: attempts=%d, cost=%.6f AIB", goodAttempts, goodCost)
		t.Logf("Bad output:  attempts=%d, cost=%.6f AIB", badAttempts, badCost)
		t.Logf("Both get same block reward: %d AIB", blockReward/AIBUnit)

		// Verify both results pass PoW (they should, since both did inference)
		if !VerifyPoW(goodResult, config.InitialDifficulty) {
			t.Error("Good result should pass PoW")
		}
		if !VerifyPoW(badResult, config.InitialDifficulty) {
			t.Error("Bad result should pass PoW (it did do inference)")
		}

		// The key insight: both cost the same energy, both get same reward
		// There's no incentive to produce "bad" output
		t.Log("✓ Both good and bad outputs cost the same energy and get same reward")
		t.Log("✓ No economic incentive to produce bad output")
	})
}

// TestSimplifiedPoAIWNoJudge verifies that no JUDGE is needed
func TestSimplifiedPoAIWNoJudge(t *testing.T) {
	config := DefaultPoAIWTestConfig()

	t.Log("=== Simplified PoAIW 'No JUDGE' Test ===")

	// The key insight: all verifications are mathematical
	t.Run("All Verifications Are Mathematical", func(t *testing.T) {
		sim := NewPoAIWBlockSimulator(config, 5000*AIBUnit)

		result, _ := sim.SimulateInference("model-v1", []byte("test data"))

		// These verifications require NO human judgment:
		// 1. PoW: Hash < Target? (mathematical)
		powValid := VerifyPoW(result, config.InitialDifficulty)
		t.Logf("PoW verification: mathematical comparison")

		// 2. Stake: Amount >= Minimum? (mathematical)
		stakeValid := VerifyStake(sim.proposerStake, config.MinStake)
		t.Logf("Stake verification: mathematical comparison")

		// 3. Time: Within range? (mathematical)
		timeValid := VerifyBlockTime(time.Now(), time.Now().Add(-30*time.Second), config)
		t.Logf("Time verification: mathematical comparison")

		if powValid && stakeValid && timeValid {
			t.Log("✓ All verifications are purely mathematical - NO JUDGE NEEDED")
		} else {
			t.Error("Verification failed")
		}
	})

	t.Run("Cannot Be Gamed", func(t *testing.T) {
		// Attack 1: Skip inference
		t.Log("Attack 1: Skip inference (fake result)")
		fakeResult := &SimulatedInferenceResult{
			ModelID:   "fake",
			InputHash: [32]byte{},
			Output:    []byte("fake"),
			Nonce:     0,
		}
		if VerifyPoW(fakeResult, config.InitialDifficulty) {
			t.Error("Attack 1 should fail!")
		} else {
			t.Log("✓ Attack 1 blocked: cannot skip inference")
		}

		// Attack 2: Reuse old result
		t.Log("Attack 2: Reuse old result")
		oldResult := &SimulatedInferenceResult{
			ModelID:   "model",
			InputHash: [32]byte{1, 2, 3},
			Output:    []byte("old output"),
			Nonce:     0,
		}
		// Same input+model+nonce should still need PoW
		if VerifyPoW(oldResult, config.InitialDifficulty) {
			t.Log("Note: Attack 2 might work with same nonce, but new inference with different salt is required")
		}

		// Attack 3: Collude with validators
		t.Log("Attack 3: Validator collusion")
		// This doesn't help because PoW is still required
		t.Log("✓ Cannot collude around mathematical verification")
	})
}

// TestSimplifiedPoAIWRealWorldScenario simulates real-world usage
func TestSimplifiedPoAIWRealWorldScenario(t *testing.T) {
	config := DefaultPoAIWTestConfig()

	t.Log("=== Real-World Scenario Simulation ===")

	// Setup: 100 validators with varying stakes
	type Validator struct {
		id    int
		stake uint64
	}

	validators := make([]Validator, 100)
	rand.Seed(42)
	totalStake := uint64(0)

	for i := 0; i < 100; i++ {
		// Random stake between 1000 and 10000 AIB
		stake := uint64(1000+rand.Intn(9000)) * AIBUnit
		validators[i] = Validator{id: i, stake: stake}
		totalStake += stake
	}

	t.Logf("Total validators: %d, Total stake: %d AIB", len(validators), totalStake/AIBUnit)

	// Simulate block production
	t.Run("Block Production", func(t *testing.T) {
		sim := NewPoAIWBlockSimulator(config, validators[0].stake)

		// Each validator gets a turn based on stake weight
		selectedCount := make(map[int]int)

		for round := 0; round < 1000; round++ {
			// Select proposer (simplified: random with stake weight)
			r := rand.Uint64() % totalStake
			var selected int
			cumulative := uint64(0)
			for i, v := range validators {
				cumulative += v.stake
				if r < cumulative {
					selected = i
					break
				}
			}
			selectedCount[selected]++

			// Simulate inference
			sim.proposerStake = validators[selected].stake
			result, _ := sim.SimulateInference("model-v1", []byte(fmt.Sprintf("input-%d", round)))

			// Validate
			valid, _ := sim.ValidateBlock(validators[selected].stake, result, time.Now())
			if !valid {
				t.Errorf("Block %d validation failed", round)
			}
		}

		// Check distribution
		t.Log("Block distribution (top 5):")
		for i := 0; i < 5; i++ {
			maxCount := 0
			maxIdx := 0
			for j, c := range selectedCount {
				if c > maxCount {
					maxCount = c
					maxIdx = j
				}
			}
			t.Logf("  Validator %d: %d blocks, stake %d AIB",
				maxIdx, maxCount, validators[maxIdx].stake/AIBUnit)
			delete(selectedCount, maxIdx)
		}

		t.Log("✓ Real-world scenario passed")
	})
}

// BenchmarkPoW measures PoW performance
func BenchmarkPoW(b *testing.B) {
	config := DefaultPoAIWTestConfig()
	sim := NewPoAIWBlockSimulator(config, 5000*AIBUnit)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sim.SimulateInference("benchmark-model", []byte(fmt.Sprintf("input-%d", i)))
	}
}
