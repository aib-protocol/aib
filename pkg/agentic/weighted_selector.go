// Package agentic provides weighted selection for block producers.
package agentic

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ============================================================
// Weighted Selector
// ============================================================

// WeightedSelector selects block producers based on reputation-weighted probability.
type WeightedSelector struct {
	mu sync.RWMutex

	// Configuration
	config *SelectorConfig

	// Reputation manager reference
	reputationManager *ReputationManager

	// Stake manager reference
	stakeManager *StakingManager

	// Selection history (for analysis)
	selectionHistory []*SelectionRecord

	// Running totals for fairness tracking
	totalSelections map[string]uint64
}

// SelectorConfig holds configuration for the weighted selector.
type SelectorConfig struct {
	// Weight exponents
	ReputationExponent float64 `json:"reputation_exponent"` // α, default 1.5
	StakeExponent      float64 `json:"stake_exponent"`      // β, default 0.5
	HistoryExponent    float64 `json:"history_exponent"`    // γ, default 0.3
	CapacityExponent   float64 `json:"capacity_exponent"`   // δ, default 0.2

	// Minimum requirements
	MinReputation      float64 `json:"min_reputation"`      // Minimum reputation to be eligible
	MinStake           uint64  `json:"min_stake"`           // Minimum stake to be eligible
	MaxSuspicion       float64 `json:"max_suspicion"`       // Maximum suspicion score allowed

	// Fairness parameters
	FairnessWindow     int     `json:"fairness_window"`     // Consider last N selections
	FairnessPenalty    float64 `json:"fairness_penalty"`    // Penalty for recent selection

	// Randomness seed (for deterministic testing)
	Seed int64 `json:"seed,omitempty"`
}

// DefaultSelectorConfig returns default selector configuration.
func DefaultSelectorConfig() *SelectorConfig {
	return &SelectorConfig{
		ReputationExponent: 1.5,
		StakeExponent:      0.5,
		HistoryExponent:    0.3,
		CapacityExponent:   0.2,
		MinReputation:      0.0,
		MinStake:           100,
		MaxSuspicion:       90.0,
		FairnessWindow:     100,
		FairnessPenalty:    0.1,
		Seed:               0, // Use crypto/rand
	}
}

// SelectionRecord records a selection event.
type SelectionRecord struct {
	NodeID        string    `json:"node_id"`
	Weight        float64   `json:"weight"`
	Probability   float64   `json:"probability"`
	TotalWeight   float64   `json:"total_weight"`
	Timestamp     time.Time `json:"timestamp"`
	BlockHeight   uint64    `json:"block_height"`
	TaskID        string    `json:"task_id,omitempty"`
}

// NewWeightedSelector creates a new weighted selector.
func NewWeightedSelector(
	config *SelectorConfig,
	reputationManager *ReputationManager,
	stakeManager *StakingManager,
) *WeightedSelector {
	if config == nil {
		config = DefaultSelectorConfig()
	}
	return &WeightedSelector{
		config:            config,
		reputationManager: reputationManager,
		stakeManager:      stakeManager,
		selectionHistory:  make([]*SelectionRecord, 0),
		totalSelections:   make(map[string]uint64),
	}
}

// ============================================================
// Weight Calculation
// ============================================================

// NodeWeight represents a node's weight for selection.
type NodeWeight struct {
	NodeID            string  `json:"node_id"`
	ReputationScore   float64 `json:"reputation_score"`
	StakeAmount       uint64  `json:"stake_amount"`
	HistoricalBlocks  uint64  `json:"historical_blocks"`
	CapacityScore     float64 `json:"capacity_score"`
	SuspicionScore    float64 `json:"suspicion_score"`
	FairnessFactor    float64 `json:"fairness_factor"`
	RawWeight         float64 `json:"raw_weight"`
	FinalWeight       float64 `json:"final_weight"`
	Eligible          bool    `json:"eligible"`
	IneligibleReason  string  `json:"ineligible_reason,omitempty"`
}

// CalculateWeight calculates the selection weight for a node.
func (ws *WeightedSelector) CalculateWeight(nodeID string) (*NodeWeight, error) {
	nw := &NodeWeight{
		NodeID: nodeID,
	}

	// Get reputation score
	repScore, err := ws.reputationManager.GetScore(nodeID)
	if err != nil {
		nw.Eligible = false
		nw.IneligibleReason = "node not found"
		return nw, nil
	}

	repScore.mu.RLock()
	nw.ReputationScore = repScore.TotalScore
	nw.SuspicionScore = repScore.SuspicionScore
	nw.HistoricalBlocks = repScore.TotalBlocksProduced
	repScore.mu.RUnlock()

	// Check eligibility
	if nw.ReputationScore < ws.config.MinReputation {
		nw.Eligible = false
		nw.IneligibleReason = fmt.Sprintf("reputation %.2f below minimum %.2f",
			nw.ReputationScore, ws.config.MinReputation)
		return nw, nil
	}

	if nw.SuspicionScore > ws.config.MaxSuspicion {
		nw.Eligible = false
		nw.IneligibleReason = fmt.Sprintf("suspicion %.2f above maximum %.2f",
			nw.SuspicionScore, ws.config.MaxSuspicion)
		return nw, nil
	}

	// Get stake (only if stakeManager is provided)
	if ws.stakeManager != nil {
		stakeInfo, err := ws.stakeManager.GetStakeInfo(stringToNodeID(nodeID))
		if err != nil || stakeInfo.Amount < ws.config.MinStake {
			nw.Eligible = false
			nw.IneligibleReason = fmt.Sprintf("stake below minimum %d", ws.config.MinStake)
			return nw, nil
		}
		nw.StakeAmount = stakeInfo.Amount
	} else {
		// No stake manager - skip stake check, use default
		nw.StakeAmount = 100 // Default stake for testing
	}

	// Calculate capacity score (simplified - in production, would measure actual capacity)
	nw.CapacityScore = 50.0 // Default capacity

	// Calculate fairness factor
	nw.FairnessFactor = ws.calculateFairnessFactor(nodeID)

	// Calculate raw weight using the formula:
	// W = (R/100)^α × (S/1000)^β × (H+1)^γ × (C/100)^δ
	reputationFactor := math.Pow(nw.ReputationScore/100.0, ws.config.ReputationExponent)
	stakeFactor := math.Pow(float64(nw.StakeAmount)/1000.0, ws.config.StakeExponent)
	historyFactor := math.Pow(float64(nw.HistoricalBlocks+1), ws.config.HistoryExponent)
	capacityFactor := math.Pow(nw.CapacityScore/100.0, ws.config.CapacityExponent)

	nw.RawWeight = reputationFactor * stakeFactor * historyFactor * capacityFactor

	// Apply fairness factor
	nw.FinalWeight = nw.RawWeight * nw.FairnessFactor

	// Apply suspicion penalty
	suspicionPenalty := 1.0 - (nw.SuspicionScore / 100.0)
	nw.FinalWeight *= suspicionPenalty

	// Ensure non-negative
	if nw.FinalWeight < 0 {
		nw.FinalWeight = 0
	}

	nw.Eligible = true
	return nw, nil
}

// calculateFairnessFactor calculates a factor to prevent recent nodes from being selected too often.
func (ws *WeightedSelector) calculateFairnessFactor(nodeID string) float64 {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	recentSelections := 0
	windowStart := len(ws.selectionHistory) - ws.config.FairnessWindow
	if windowStart < 0 {
		windowStart = 0
	}

	for i := windowStart; i < len(ws.selectionHistory); i++ {
		if ws.selectionHistory[i].NodeID == nodeID {
			recentSelections++
		}
	}

	if recentSelections == 0 {
		return 1.0
	}

	// Reduce weight based on recent selections
	penalty := 1.0 - (float64(recentSelections) * ws.config.FairnessPenalty)
	if penalty < 0.1 {
		penalty = 0.1 // Minimum factor
	}

	return penalty
}

// ============================================================
// Selection Algorithm
// ============================================================

// SelectionResult represents the result of a selection.
type SelectionResult struct {
	SelectedNode    string        `json:"selected_node"`
	SelectedWeight  float64       `json:"selected_weight"`
	Probability     float64       `json:"probability"`
	TotalWeight     float64       `json:"total_weight"`
	EligibleNodes   int           `json:"eligible_nodes"`
	AllWeights      []*NodeWeight `json:"all_weights"`
	RandomValue     float64       `json:"random_value"`
	Timestamp       time.Time     `json:"timestamp"`
}

// Select selects a block producer using weighted random selection.
func (ws *WeightedSelector) Select(excludeNodes []string) (*SelectionResult, error) {
	// Get all node scores
	allScores := ws.reputationManager.GetAllScores()

	// Calculate weights for all nodes
	var weights []*NodeWeight
	totalWeight := 0.0

	// Build exclusion map for O(1) lookup
	excludeMap := make(map[string]bool)
	for _, exclude := range excludeNodes {
		excludeMap[exclude] = true
	}

	for _, score := range allScores {
		nodeID := string(score.NodeID)

		// Skip excluded nodes
		if excludeMap[nodeID] {
			continue
		}

		nw, _ := ws.CalculateWeight(nodeID)
		if nw.Eligible && nw.FinalWeight > 0 {
			weights = append(weights, nw)
			totalWeight += nw.FinalWeight
		}
	}

	if len(weights) == 0 {
		return nil, fmt.Errorf("selector: no eligible nodes")
	}

	// Generate cryptographically secure random value
	randomValue := ws.secureRandomFloat()

	// Weighted random selection (lottery)
	lottery := randomValue * totalWeight

	cumulative := 0.0
	var selected *NodeWeight

	// Sort weights for deterministic processing (optional but helps with reproducibility)
	sort.Slice(weights, func(i, j int) bool {
		return weights[i].NodeID < weights[j].NodeID
	})

	for _, nw := range weights {
		cumulative += nw.FinalWeight
		if cumulative >= lottery {
			selected = nw
			break
		}
	}

	// Fallback (shouldn't happen but safety)
	if selected == nil {
		selected = weights[len(weights)-1]
	}

	// Record selection
	record := &SelectionRecord{
		NodeID:      selected.NodeID,
		Weight:      selected.FinalWeight,
		Probability: selected.FinalWeight / totalWeight,
		TotalWeight: totalWeight,
		Timestamp:   time.Now(),
	}

	ws.mu.Lock()
	ws.selectionHistory = append(ws.selectionHistory, record)
	ws.totalSelections[selected.NodeID]++
	ws.mu.Unlock()

	return &SelectionResult{
		SelectedNode:   selected.NodeID,
		SelectedWeight: selected.FinalWeight,
		Probability:    selected.FinalWeight / totalWeight,
		TotalWeight:    totalWeight,
		EligibleNodes:  len(weights),
		AllWeights:     weights,
		RandomValue:    randomValue,
		Timestamp:      time.Now(),
	}, nil
}

// secureRandomFloat generates a cryptographically secure random float in [0, 1).
func (ws *WeightedSelector) secureRandomFloat() float64 {
	if ws.config.Seed != 0 {
		// Deterministic mode for testing
		return float64(ws.config.Seed%1000000) / 1000000.0
	}

	// Cryptographically secure random
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback to timestamp on error
		return float64(time.Now().UnixNano()%1000000) / 1000000.0
	}

	// Convert to float in [0, 1)
	value := binary.BigEndian.Uint64(buf[:])
	return float64(value) / float64(^uint64(0))
}

// ============================================================
// Statistics and Analysis
// ============================================================

// SelectionStats represents statistics about selections.
type SelectionStats struct {
	TotalSelections      uint64            `json:"total_selections"`
	UniqueNodes          int               `json:"unique_nodes"`
	SelectionsByNode     map[string]uint64 `json:"selections_by_node"`
	FairnessScore        float64           `json:"fairness_score"` // Gini coefficient
	EntropyScore         float64           `json:"entropy_score"` // Shannon entropy
	AvgProbability       float64           `json:"avg_probability"`
	MaxConsecutiveSelects int              `json:"max_consecutive_selects"`
}

// GetStats returns statistics about recent selections.
func (ws *WeightedSelector) GetStats() *SelectionStats {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	stats := &SelectionStats{
		TotalSelections:  uint64(len(ws.selectionHistory)),
		SelectionsByNode: make(map[string]uint64),
	}

	if len(ws.selectionHistory) == 0 {
		return stats
	}

	// Count selections by node
	for _, record := range ws.selectionHistory {
		stats.SelectionsByNode[record.NodeID]++
	}
	stats.UniqueNodes = len(stats.SelectionsByNode)

	// Calculate Gini coefficient (fairness)
	stats.FairnessScore = ws.calculateGiniCoefficient()

	// Calculate Shannon entropy
	stats.EntropyScore = ws.calculateEntropy()

	// Average probability
	var totalProb float64
	for _, record := range ws.selectionHistory {
		totalProb += record.Probability
	}
	stats.AvgProbability = totalProb / float64(len(ws.selectionHistory))

	// Max consecutive selections
	stats.MaxConsecutiveSelects = ws.calculateMaxConsecutive()

	return stats
}

// calculateGiniCoefficient calculates the Gini coefficient of selection distribution.
// 0 = perfect equality, 1 = perfect inequality
func (ws *WeightedSelector) calculateGiniCoefficient() float64 {
	if len(ws.totalSelections) < 2 {
		return 0
	}

	// Get values and sort
	values := make([]float64, 0, len(ws.totalSelections))
	for _, count := range ws.totalSelections {
		values = append(values, float64(count))
	}
	sort.Float64s(values)

	n := float64(len(values))
	var sum, cumSum float64

	for i, v := range values {
		sum += v
		cumSum += (2*float64(i+1) - n - 1) * v
	}

	if sum == 0 {
		return 0
	}

	return cumSum / (n * sum)
}

// calculateEntropy calculates Shannon entropy of selection distribution.
// Higher entropy = more uniform distribution
func (ws *WeightedSelector) calculateEntropy() float64 {
	if len(ws.selectionHistory) == 0 {
		return 0
	}

	total := float64(len(ws.selectionHistory))
	var entropy float64

	for _, count := range ws.totalSelections {
		if count > 0 {
			p := float64(count) / total
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// calculateMaxConsecutive finds the maximum consecutive selections of any node.
func (ws *WeightedSelector) calculateMaxConsecutive() int {
	if len(ws.selectionHistory) == 0 {
		return 0
	}

	maxConsec := 1
	currentConsec := 1
	lastNode := ws.selectionHistory[0].NodeID

	for i := 1; i < len(ws.selectionHistory); i++ {
		if ws.selectionHistory[i].NodeID == lastNode {
			currentConsec++
			if currentConsec > maxConsec {
				maxConsec = currentConsec
			}
		} else {
			currentConsec = 1
			lastNode = ws.selectionHistory[i].NodeID
		}
	}

	return maxConsec
}

// GetSelectionHistory returns recent selection history.
func (ws *WeightedSelector) GetSelectionHistory(limit int) []*SelectionRecord {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if limit <= 0 || limit > len(ws.selectionHistory) {
		limit = len(ws.selectionHistory)
	}

	start := len(ws.selectionHistory) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*SelectionRecord, limit)
	copy(result, ws.selectionHistory[start:])

	return result
}

// ============================================================
// Batch Selection (for verification committees)
// ============================================================

// SelectCommittee selects multiple nodes for a verification committee.
func (ws *WeightedSelector) SelectCommittee(size int, excludeNodes []string) ([]string, error) {
	var selected []string
	excludeMap := make(map[string]bool)
	for _, id := range excludeNodes {
		excludeMap[id] = true
	}

	for i := 0; i < size; i++ {
		// Add already selected to exclusion list
		allExclude := make([]string, len(excludeNodes))
		copy(allExclude, excludeNodes)
		allExclude = append(allExclude, selected...)

		result, err := ws.Select(allExclude)
		if err != nil {
			if len(selected) > 0 {
				return selected, nil // Return what we have
			}
			return nil, fmt.Errorf("selector: failed to select committee member %d: %w", i, err)
		}

		selected = append(selected, result.SelectedNode)
	}

	return selected, nil
}

// ============================================================
// Simulation and Testing
// ============================================================

// SimulateSelections runs multiple selections to verify distribution.
func (ws *WeightedSelector) SimulateSelections(iterations int) (map[string]int, error) {
	distribution := make(map[string]int)

	for i := 0; i < iterations; i++ {
		result, err := ws.Select(nil)
		if err != nil {
			return nil, err
		}
		distribution[result.SelectedNode]++
	}

	return distribution, nil
}

// VerifyFairness checks if the selection distribution is fair.
func (ws *WeightedSelector) VerifyFairness(tolerance float64) (bool, *FairnessReport) {
	stats := ws.GetStats()

	report := &FairnessReport{
		GiniCoefficient: stats.FairnessScore,
		Entropy:         stats.EntropyScore,
		ExpectedEntropy: math.Log2(float64(stats.UniqueNodes)),
	}

	// Check Gini coefficient (lower is more fair)
	report.GiniOK = stats.FairnessScore < tolerance

	// Check entropy (higher is more fair)
	report.EntropyOK = stats.EntropyScore > math.Log2(float64(stats.UniqueNodes))*0.8

	report.IsFair = report.GiniOK && report.EntropyOK

	return report.IsFair, report
}

// FairnessReport represents a fairness analysis report.
type FairnessReport struct {
	GiniCoefficient float64 `json:"gini_coefficient"`
	Entropy         float64 `json:"entropy"`
	ExpectedEntropy float64 `json:"expected_entropy"`
	GiniOK          bool    `json:"gini_ok"`
	EntropyOK       bool    `json:"entropy_ok"`
	IsFair          bool    `json:"is_fair"`
}
