package verification

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// MajorityVerifier implements majority vote verification as fallback for ZKML
type MajorityVerifier struct {
	mu           sync.RWMutex
	threshold    float64       // Agreement threshold (default 0.67 = 67%)
	minNodes     int           // Minimum number of nodes required
	maxDeviation float64       // Maximum allowed deviation for numeric results
	history     map[string]*VerificationHistory
}

// VerificationHistory stores past verification results
type VerificationHistory struct {
	mu            sync.RWMutex
	results       []*HistoricalResult
	maxHistory    int
}

// HistoricalResult stores a single historical verification result
type HistoricalResult struct {
	TaskID       string
	Results      map[string]string
	MajorityResult string
	AgreementRate float64
	Timestamp    int64
}

// VerificationResult contains the result of majority verification
type VerificationResult struct {
	IsValid        bool
	MajorityResult string
	AgreementRate  float64
	NodeResults    map[string]string
	Disagreeing   []string
}

// NewMajorityVerifier creates a new majority verifier
func NewMajorityVerifier() *MajorityVerifier {
	return &MajorityVerifier{
		threshold:    0.67,     // 67% threshold
		minNodes:     5,         // At least 5 nodes (raised from 3 to prevent 2-node collusion)
		maxDeviation: 0.1,       // 10% deviation for numeric results
		history:     make(map[string]*VerificationHistory),
	}
}

// SetThreshold sets the agreement threshold
func (v *MajorityVerifier) SetThreshold(threshold float64) {
	if threshold > 0.5 && threshold <= 1.0 {
		v.mu.Lock()
		v.threshold = threshold
		v.mu.Unlock()
	}
}

// SetMinNodes sets the minimum number of nodes required
func (v *MajorityVerifier) SetMinNodes(minNodes int) {
	if minNodes >= 1 {
		v.mu.Lock()
		v.minNodes = minNodes
		v.mu.Unlock()
	}
}

// Verify performs majority vote verification
func (v *MajorityVerifier) Verify(taskID string, results map[string]string) (*VerificationResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if taskID == "" {
		return nil, errors.New("verification: empty task ID")
	}
	if results == nil || len(results) == 0 {
		return nil, errors.New("verification: nil or empty results")
	}

	// Check minimum nodes
	if len(results) < v.minNodes {
		return &VerificationResult{
			IsValid:        false,
			MajorityResult: "",
			AgreementRate:  0,
			NodeResults:    results,
			Disagreeing:    []string{},
		}, fmt.Errorf("verification: insufficient nodes (got %d, need %d)", len(results), v.minNodes)
	}

	// Count results
	resultCounts := make(map[string]int)
	for _, result := range results {
		resultCounts[result]++
	}

	// Find majority result
	var majorityResult string
	maxCount := 0
	for result, count := range resultCounts {
		if count > maxCount {
			maxCount = count
			majorityResult = result
		}
	}

	// Calculate agreement rate
	agreementRate := float64(maxCount) / float64(len(results))

	// Identify disagreeing nodes
	var disagreeing []string
	for nodeID, result := range results {
		if result != majorityResult {
			disagreeing = append(disagreeing, nodeID)
		}
	}

	// Determine if valid
	isValid := agreementRate >= v.threshold

	// Store in history
	v.storeResult(taskID, results, majorityResult, agreementRate)

	return &VerificationResult{
		IsValid:        isValid,
		MajorityResult: majorityResult,
		AgreementRate:  agreementRate,
		NodeResults:    results,
		Disagreeing:    disagreeing,
	}, nil
}

// VerifyNumeric performs majority verification for numeric results
func (v *MajorityVerifier) VerifyNumeric(taskID string, results map[string]float64) (*NumericVerificationResult, error) {
	if taskID == "" {
		return nil, errors.New("verification: empty task ID")
	}
	if results == nil || len(results) == 0 {
		return nil, errors.New("verification: nil or empty results")
	}

	// Convert float64 results to strings for counting
	stringResults := make(map[string]string)
	values := make([]float64, 0, len(results))
	for nodeID, value := range results {
		// Round to 4 decimal places for comparison
		rounded := math.Round(value*10000) / 10000
		stringResults[nodeID] = fmt.Sprintf("%.4f", rounded)
		values = append(values, rounded)
	}

	// Use standard verify
	result, err := v.Verify(taskID, stringResults)
	if err != nil {
		return nil, err
	}

	// Calculate statistics
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	return &NumericVerificationResult{
		VerificationResult: *result,
		Mean:               mean,
		StdDev:             stdDev,
		Values:             values,
	}, nil
}

// NumericVerificationResult contains numeric verification details
type NumericVerificationResult struct {
	VerificationResult
	Mean   float64
	StdDev float64
	Values []float64
}

// VerifyJSON performs majority verification for JSON results
func (v *MajorityVerifier) VerifyJSON(taskID string, results map[string][]byte) (*VerificationResult, error) {
	if taskID == "" {
		return nil, errors.New("verification: empty task ID")
	}
	if results == nil || len(results) == 0 {
		return nil, errors.New("verification: nil or empty results")
	}

	// Hash each result for comparison
	hashResults := make(map[string]string)
	for nodeID, result := range results {
		hash := sha256.Sum256(result)
		hashResults[nodeID] = fmt.Sprintf("%x", hash)
	}

	return v.Verify(taskID, hashResults)
}

// GetHistory retrieves verification history for a task
func (v *MajorityVerifier) GetHistory(taskID string) []*HistoricalResult {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if history, ok := v.history[taskID]; ok {
		history.mu.RLock()
		defer history.mu.RUnlock()
		results := make([]*HistoricalResult, len(history.results))
		copy(results, history.results)
		return results
	}
	return nil
}

// GetStatistics returns verification statistics
func (v *MajorityVerifier) GetStatistics() *VerifierStatistics {
	v.mu.RLock()
	defer v.mu.RUnlock()

	totalVerifications := 0
	successfulVerifications := 0
	totalAgreement := 0.0

	for _, history := range v.history {
		history.mu.RLock()
		totalVerifications += len(history.results)
		for _, r := range history.results {
			totalAgreement += r.AgreementRate
			if r.AgreementRate >= v.threshold {
				successfulVerifications++
			}
		}
		history.mu.RUnlock()
	}

	successRate := 0.0
	averageAgreement := 0.0
	if totalVerifications > 0 {
		successRate = float64(successfulVerifications) / float64(totalVerifications)
		averageAgreement = totalAgreement / float64(totalVerifications)
	}

	return &VerifierStatistics{
		TotalVerifications:      totalVerifications,
		SuccessfulVerifications: successfulVerifications,
		SuccessRate:             successRate,
		AverageAgreement:        averageAgreement,
	}
}

// VerifierStatistics contains overall verifier statistics
type VerifierStatistics struct {
	TotalVerifications     int
	SuccessfulVerifications int
	SuccessRate            float64
	AverageAgreement       float64
}

// storeResult stores a verification result in history
func (v *MajorityVerifier) storeResult(taskID string, results map[string]string, majorityResult string, agreementRate float64) {
	if v.history[taskID] == nil {
		v.history[taskID] = &VerificationHistory{
			results:    make([]*HistoricalResult, 0),
			maxHistory: 100,
		}
	}

	history := v.history[taskID]
	history.mu.Lock()
	defer history.mu.Unlock()

	result := &HistoricalResult{
		TaskID:         taskID,
		Results:        results,
		MajorityResult: majorityResult,
		AgreementRate:  agreementRate,
		Timestamp:      time.Now().Unix(),
	}

	history.results = append(history.results, result)

	// Trim history if needed
	if len(history.results) > history.maxHistory {
		history.results = history.results[len(history.results)-history.maxHistory:]
	}
}

// TaskResult represents a task result from a node
type TaskResult struct {
	NodeID    string
	Result    string
	Timestamp int64
}

// TaskBatch represents a batch of task results
type TaskBatch struct {
	TaskID    string
	Results   []*TaskResult
	Timestamp int64
}

// BatchVerifier handles batch verification of multiple tasks
type BatchVerifier struct {
	verifier *MajorityVerifier
}

// NewBatchVerifier creates a new batch verifier
func NewBatchVerifier() *BatchVerifier {
	return &BatchVerifier{
		verifier: NewMajorityVerifier(),
	}
}

// VerifyBatch verifies a batch of task results
func (bv *BatchVerifier) VerifyBatch(batch *TaskBatch) ([]*VerificationResult, error) {
	if batch == nil || len(batch.Results) == 0 {
		return nil, errors.New("batch: nil or empty batch")
	}

	// Group results by node
	nodeResults := make(map[string]map[string]string)
	for _, result := range batch.Results {
		if nodeResults[result.NodeID] == nil {
			nodeResults[result.NodeID] = make(map[string]string)
		}
		// For batch verification, use result as both key and value
		nodeResults[result.NodeID][result.NodeID] = result.Result
	}

	// Verify each node's results
	results := make([]*VerificationResult, 0)
	for nodeID, resultsMap := range nodeResults {
		result, err := bv.verifier.Verify(nodeID+"_"+batch.TaskID, resultsMap)
		if err != nil {
			continue // Skip failed verifications
		}
		results = append(results, result)
	}

	return results, nil
}

// WeightedMajorityVerifier implements weighted majority voting
type WeightedMajorityVerifier struct {
	verifier       *MajorityVerifier
	nodeWeights    map[string]float64
	weightProvider WeightProvider
}

// WeightProvider provides weights for nodes
type WeightProvider interface {
	GetWeight(nodeID string) (float64, error)
}

// NewWeightedMajorityVerifier creates a new weighted majority verifier
func NewWeightedMajorityVerifier(provider WeightProvider) *WeightedMajorityVerifier {
	return &WeightedMajorityVerifier{
		verifier:    NewMajorityVerifier(),
		nodeWeights: make(map[string]float64),
		weightProvider: provider,
	}
}

// SetWeight sets the weight for a specific node
func (v *WeightedMajorityVerifier) SetWeight(nodeID string, weight float64) {
	if weight > 0 {
		v.nodeWeights[nodeID] = weight
	}
}

// VerifyWeighted performs weighted majority verification
func (v *WeightedMajorityVerifier) VerifyWeighted(taskID string, results map[string]string) (*VerificationResult, error) {
	if taskID == "" {
		return nil, errors.New("weighted verification: empty task ID")
	}
	if results == nil || len(results) == 0 {
		return nil, errors.New("weighted verification: nil or empty results")
	}

	// Calculate weighted counts
	weightedCounts := make(map[string]float64)
	totalWeight := 0.0

	for nodeID, result := range results {
		weight := v.nodeWeights[nodeID]
		if weight <= 0 {
			weight = 1.0 // Default weight
		}

		if v.weightProvider != nil {
			if w, err := v.weightProvider.GetWeight(nodeID); err == nil && w > 0 {
				weight = w
			}
		}

		weightedCounts[result] += weight
		totalWeight += weight
	}

	// Find weighted majority
	var majorityResult string
	maxWeight := 0.0
	for result, weight := range weightedCounts {
		if weight > maxWeight {
			maxWeight = weight
			majorityResult = result
		}
	}

	// Calculate weighted agreement
	agreementRate := maxWeight / totalWeight

	// Determine threshold (weighted)
	v.verifier.mu.RLock()
	threshold := v.verifier.threshold
	v.verifier.mu.RUnlock()

	isValid := agreementRate >= threshold

	// Identify disagreeing nodes
	var disagreeing []string
	for nodeID, result := range results {
		if result != majorityResult {
			disagreeing = append(disagreeing, nodeID)
		}
	}

	return &VerificationResult{
		IsValid:        isValid,
		MajorityResult: majorityResult,
		AgreementRate:  agreementRate,
		NodeResults:    results,
		Disagreeing:    disagreeing,
	}, nil
}

// SortedResults returns results sorted by agreement rate
func (v *MajorityVerifier) SortedResults(taskID string) []string {
	history := v.GetHistory(taskID)
	if len(history) == 0 {
		return nil
	}

	// Sort by agreement rate descending
	sort.Slice(history, func(i, j int) bool {
		return history[i].AgreementRate > history[j].AgreementRate
	})

	results := make([]string, len(history))
	for i, h := range history {
		results[i] = fmt.Sprintf("%.2f%% - %s", h.AgreementRate*100, h.MajorityResult)
	}

	return results
}
