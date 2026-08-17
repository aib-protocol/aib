// Package inference provides AI inference node services with lightning network payments.
package inference

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// RatingManager manages node ratings and reputation
type RatingManager struct {
	nodes          map[[32]byte]*InferenceNode
	pendingRatings map[[32]byte][]RatingSubmission
	ratingHistory  map[[32]byte][]RatingRecord
	mu             sync.RWMutex

	// Configuration
	minStake        uint64
	reputationDecay float64
	maxHistory      int
}

// RatingSubmission represents a rating submission from a user
type RatingSubmission struct {
	NodePubKey [32]byte
	UserPubKey [32]byte
	Score      float64 // 0-10
	Reason     string
	Signature  []byte
	Timestamp  int64
}

// RatingRecord represents a stored rating
type RatingRecord struct {
	NodePubKey  [32]byte
	UserPubKey  [32]byte
	Score       float64
	Reason      string
	Signature   []byte
	Timestamp   time.Time
	BlockHeight uint64
	Confirmed   bool
}

// RatingStats contains rating statistics for a node
type RatingStats struct {
	TotalRatings   uint64
	AvgScore       float64
	MinScore       float64
	MaxScore       float64
	ScoreStdDev    float64
	RecentAvgScore float64 // Last 7 days
}

// ErrInvalidRating is returned when rating is invalid
var ErrInvalidRating = fmt.Errorf("invalid rating")

// ErrRatingExists is returned when rating already exists
var ErrRatingExists = fmt.Errorf("rating already exists")

// ErrNodeNotTracked is returned when node is not tracked
var ErrNodeNotTracked = fmt.Errorf("node not tracked")

// ErrInvalidSignature is returned when signature is invalid
var ErrInvalidSignature = fmt.Errorf("invalid signature")

// DefaultReputationDecay is the default reputation decay rate per day
const DefaultReputationDecay = 0.01

// MaxRatingsPerNode is the maximum number of ratings to keep per node
const MaxRatingsPerNode = 1000

// NewRatingManager creates a new rating manager
func NewRatingManager() *RatingManager {
	return &RatingManager{
		nodes:           make(map[[32]byte]*InferenceNode),
		pendingRatings:  make(map[[32]byte][]RatingSubmission),
		ratingHistory:   make(map[[32]byte][]RatingRecord),
		reputationDecay: DefaultReputationDecay,
		maxHistory:      MaxRatingsPerNode,
	}
}

// NewRatingManagerWithConfig creates a new rating manager with configuration
func NewRatingManagerWithConfig(minStake uint64, decay float64) *RatingManager {
	rm := &RatingManager{
		nodes:           make(map[[32]byte]*InferenceNode),
		pendingRatings:  make(map[[32]byte][]RatingSubmission),
		ratingHistory:   make(map[[32]byte][]RatingRecord),
		reputationDecay: decay,
		maxHistory:      MaxRatingsPerNode,
	}
	if decay <= 0 {
		rm.reputationDecay = DefaultReputationDecay
	}
	return rm
}

// RegisterNode registers a node with the rating manager
func (rm *RatingManager) RegisterNode(node *InferenceNode) error {
	if node == nil {
		return fmt.Errorf("nil node")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if already registered
	if _, exists := rm.nodes[node.PubKey]; exists {
		return fmt.Errorf("node already registered: %s", hex.EncodeToString(node.PubKey[:]))
	}

	rm.nodes[node.PubKey] = node
	rm.pendingRatings[node.PubKey] = []RatingSubmission{}
	rm.ratingHistory[node.PubKey] = []RatingRecord{}

	return nil
}

// UnregisterNode unregisters a node from the rating manager
func (rm *RatingManager) UnregisterNode(pubKey [32]byte) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.nodes[pubKey]; !exists {
		return ErrNodeNotTracked
	}

	delete(rm.nodes, pubKey)
	delete(rm.pendingRatings, pubKey)
	delete(rm.ratingHistory, pubKey)

	return nil
}

// SubmitRating submits a rating for a node
func (rm *RatingManager) SubmitRating(submission RatingSubmission) error {
	// Validate submission
	if err := rm.validateSubmission(&submission); err != nil {
		return err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if node exists
	node, exists := rm.nodes[submission.NodePubKey]
	if !exists {
		return ErrNodeNotTracked
	}

	// Check for duplicate rating from same user
	for _, existing := range rm.pendingRatings[submission.NodePubKey] {
		if existing.UserPubKey == submission.UserPubKey {
			return ErrRatingExists
		}
	}

	// Add to pending ratings
	rm.pendingRatings[submission.NodePubKey] = append(rm.pendingRatings[submission.NodePubKey], submission)

	// Update node reputation
	rm.updateNodeReputationInternal(node)

	return nil
}

// validateSubmission validates a rating submission
func (rm *RatingManager) validateSubmission(submission *RatingSubmission) error {
	// Validate score range
	if submission.Score < 0 || submission.Score > 10 {
		return fmt.Errorf("%w: score must be between 0 and 10", ErrInvalidRating)
	}

	// Validate timestamps
	if submission.Timestamp == 0 {
		submission.Timestamp = time.Now().Unix()
	}

	// Check if timestamp is not in the future (allow 5 minute tolerance)
	if submission.Timestamp > time.Now().Add(5*time.Minute).Unix() {
		return fmt.Errorf("%w: timestamp too far in future", ErrInvalidRating)
	}

	// Check if timestamp is not too old (max 30 days)
	if submission.Timestamp < time.Now().Add(-30*24*time.Hour).Unix() {
		return fmt.Errorf("%w: timestamp too old", ErrInvalidRating)
	}

	// Validate reason length
	if len(submission.Reason) > 500 {
		return fmt.Errorf("%w: reason too long", ErrInvalidRating)
	}

	// If signature provided, verify it
	if len(submission.Signature) > 0 {
		if !rm.verifySignature(submission) {
			return ErrInvalidSignature
		}
	}

	return nil
}

// verifySignature verifies a rating submission signature
func (rm *RatingManager) verifySignature(submission *RatingSubmission) bool {
	// In production, this would verify the user's signature
	// For now, accept all signatures if provided
	return len(submission.Signature) > 0
}

// ConfirmRating confirms a pending rating (simulates block confirmation)
func (rm *RatingManager) ConfirmRating(nodePubKey [32]byte, userPubKey [32]byte, blockHeight uint64) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Find and remove from pending
	pending := rm.pendingRatings[nodePubKey]
	found := -1
	for i, r := range pending {
		if r.UserPubKey == userPubKey {
			found = i
			break
		}
	}

	if found < 0 {
		return fmt.Errorf("rating not found in pending")
	}

	submission := pending[found]
	pending = append(pending[:found], pending[found+1:]...)
	rm.pendingRatings[nodePubKey] = pending

	// Add to history
	record := RatingRecord{
		NodePubKey:  submission.NodePubKey,
		UserPubKey:  submission.UserPubKey,
		Score:       submission.Score,
		Reason:      submission.Reason,
		Signature:   submission.Signature,
		Timestamp:   time.Unix(submission.Timestamp, 0),
		BlockHeight: blockHeight,
		Confirmed:   true,
	}

	history := rm.ratingHistory[nodePubKey]
	history = append(history, record)

	// Trim history if needed
	if len(history) > rm.maxHistory {
		history = history[len(history)-rm.maxHistory:]
	}
	rm.ratingHistory[nodePubKey] = history

	// Update node reputation
	if node, exists := rm.nodes[nodePubKey]; exists {
		rm.updateNodeReputationInternal(node)
	}

	return nil
}

// UpdateNodeReputation updates a node's reputation based on all ratings
func (rm *RatingManager) UpdateNodeReputation(nodePubKey [32]byte) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	node, exists := rm.nodes[nodePubKey]
	if !exists {
		return ErrNodeNotTracked
	}

	rm.updateNodeReputationInternal(node)
	return nil
}

// updateNodeReputationInternal internal method to update reputation
func (rm *RatingManager) updateNodeReputationInternal(node *InferenceNode) {
	history := rm.ratingHistory[node.PubKey]
	if len(history) == 0 {
		return
	}

	// Calculate weighted average score
	// Recent ratings have more weight
	var totalWeight float64
	var weightedSum float64

	now := time.Now()
	for _, record := range history {
		if !record.Confirmed {
			continue
		}

		// Weight decreases with age
		age := now.Sub(record.Timestamp)
		daysOld := age.Hours() / 24

		// Exponential decay: weight = e^(-decay * days)
		weight := math.Exp(-rm.reputationDecay * daysOld)

		weightedSum += record.Score * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		newReputation := weightedSum / totalWeight
		node.Reputation = newReputation
	}
}

// ApplyReputationDecay applies reputation decay to all nodes
func (rm *RatingManager) ApplyReputationDecay() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	for _, node := range rm.nodes {
		history := rm.ratingHistory[node.PubKey]
		if len(history) == 0 {
			// If no ratings, reputation slowly decays
			node.Reputation = math.Max(0, node.Reputation-0.1)
			continue
		}

		// Check if recent ratings exist
		latestRating := history[len(history)-1]
		daysSinceLastRating := now.Sub(latestRating.Timestamp).Hours() / 24

		// Decay if no recent ratings
		if daysSinceLastRating > 7 {
			decayAmount := rm.reputationDecay * daysSinceLastRating
			node.Reputation = math.Max(0, node.Reputation-decayAmount)
		}
	}
}

// GetNodeRatings returns all confirmed ratings for a node
func (rm *RatingManager) GetNodeRatings(nodePubKey [32]byte) []RatingRecord {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	history := rm.ratingHistory[nodePubKey]
	result := make([]RatingRecord, 0, len(history))

	for _, r := range history {
		if r.Confirmed {
			result = append(result, r)
		}
	}

	return result
}

// GetPendingRatings returns all pending ratings for a node
func (rm *RatingManager) GetPendingRatings(nodePubKey [32]byte) []RatingSubmission {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.pendingRatings[nodePubKey]
}

// GetRatingStats returns rating statistics for a node
func (rm *RatingManager) GetRatingStats(nodePubKey [32]byte) (*RatingStats, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	history := rm.ratingHistory[nodePubKey]
	if len(history) == 0 {
		return &RatingStats{}, nil
	}

	var total float64
	var count int
	var minScore float64 = 10
	var maxScore float64 = 0
	scores := make([]float64, 0, len(history))

	for _, r := range history {
		if r.Confirmed {
			total += r.Score
			count++
			if r.Score < minScore {
				minScore = r.Score
			}
			if r.Score > maxScore {
				maxScore = r.Score
			}
			scores = append(scores, r.Score)
		}
	}

	if count == 0 {
		return &RatingStats{}, nil
	}

	avgScore := total / float64(count)

	// Calculate standard deviation
	var variance float64
	for _, s := range scores {
		diff := s - avgScore
		variance += diff * diff
	}
	variance /= float64(count)
	stdDev := math.Sqrt(variance)

	// Calculate recent average (last 7 days)
	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	var recentTotal float64
	var recentCount int
	for _, r := range history {
		if r.Confirmed && r.Timestamp.After(weekAgo) {
			recentTotal += r.Score
			recentCount++
		}
	}
	recentAvg := 0.0
	if recentCount > 0 {
		recentAvg = recentTotal / float64(recentCount)
	}

	return &RatingStats{
		TotalRatings:   uint64(count),
		AvgScore:       avgScore,
		MinScore:       minScore,
		MaxScore:       maxScore,
		ScoreStdDev:    stdDev,
		RecentAvgScore: recentAvg,
	}, nil
}

// GetTopNodes returns top N nodes by reputation
func (rm *RatingManager) GetTopNodes(n int) []*InferenceNode {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	type nodeRep struct {
		node       *InferenceNode
		reputation float64
	}

	nodes := make([]nodeRep, 0, len(rm.nodes))
	for _, node := range rm.nodes {
		if node.IsOnline {
			nodes = append(nodes, nodeRep{node, node.Reputation})
		}
	}

	// Sort by reputation descending
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].reputation > nodes[j].reputation
	})

	// Take top N
	if n > len(nodes) {
		n = len(nodes)
	}

	result := make([]*InferenceNode, n)
	for i := 0; i < n; i++ {
		result[i] = nodes[i].node
	}

	return result
}

// GetNodeCount returns the number of registered nodes
func (rm *RatingManager) GetNodeCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.nodes)
}

// GetAllNodes returns all registered nodes
func (rm *RatingManager) GetAllNodes() []*InferenceNode {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*InferenceNode, 0, len(rm.nodes))
	for _, node := range rm.nodes {
		result = append(result, node)
	}
	return result
}

// SerializeRating serializes a rating to JSON
func (rm *RatingManager) SerializeRating(record RatingRecord) ([]byte, error) {
	return json.Marshal(record)
}

// DeserializeRating deserializes a rating from JSON
func (rm *RatingManager) DeserializeRating(data []byte) (*RatingRecord, error) {
	var record RatingRecord
	err := json.Unmarshal(data, &record)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// VerifyRating verifies a rating's authenticity
func (rm *RatingManager) VerifyRating(record RatingRecord) bool {
	// In production, this would verify:
	// 1. User's signature on the rating
	// 2. User's public key belongs to a valid user
	// 3. Rating was submitted within valid time window

	return record.Confirmed && record.Score >= 0 && record.Score <= 10
}

// StartDecayTicker starts a background ticker for reputation decay
func (rm *RatingManager) StartDecayTicker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			rm.ApplyReputationDecay()
		}
	}()
}

// Stop stops the rating manager
func (rm *RatingManager) Stop() {
	// In production, this would clean up resources
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Clear pending ratings
	rm.pendingRatings = make(map[[32]byte][]RatingSubmission)
}

// Helper functions

// BytesToPubKey converts bytes to a public key array
func BytesToPubKey(data []byte) ([32]byte, error) {
	if len(data) != 32 {
		return [32]byte{}, fmt.Errorf("invalid public key length")
	}

	var pubKey [32]byte
	copy(pubKey[:], data)
	return pubKey, nil
}

// PubKeyToBytes converts a public key array to bytes
func PubKeyToBytes(pubKey [32]byte) []byte {
	return pubKey[:]
}

// GenerateTestRating generates a test rating submission
func GenerateTestRating(nodePubKey [32]byte, userPubKey [32]byte, score float64) RatingSubmission {
	return RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      score,
		Reason:     "Test rating",
		Signature:  []byte("test-signature"),
		Timestamp:  time.Now().Unix(),
	}
}
