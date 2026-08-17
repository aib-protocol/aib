// Package inference provides AI inference node services with lightning network payments.
package inference

import (
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

// TestNewRatingManager tests creating a new rating manager
func TestNewRatingManager(t *testing.T) {
	rm := NewRatingManager()

	if rm == nil {
		t.Fatal("NewRatingManager returned nil")
	}

	if rm.nodes == nil {
		t.Error("expected nodes map to be initialized")
	}

	if rm.pendingRatings == nil {
		t.Error("expected pendingRatings map to be initialized")
	}

	if rm.ratingHistory == nil {
		t.Error("expected ratingHistory map to be initialized")
	}

	if rm.reputationDecay != DefaultReputationDecay {
		t.Errorf("expected default reputation decay, got %f", rm.reputationDecay)
	}
}

// TestNewRatingManagerWithConfig tests creating a rating manager with config
func TestNewRatingManagerWithConfig(t *testing.T) {
	rm := NewRatingManagerWithConfig(500000, 0.02)

	if rm == nil {
		t.Fatal("NewRatingManagerWithConfig returned nil")
	}

	if rm.reputationDecay != 0.02 {
		t.Errorf("expected 0.02 reputation decay, got %f", rm.reputationDecay)
	}

	// Test with invalid (zero/negative) decay - should use default
	rm2 := NewRatingManagerWithConfig(100000, 0)
	if rm2.reputationDecay != DefaultReputationDecay {
		t.Errorf("expected default decay for zero input, got %f", rm2.reputationDecay)
	}

	rm3 := NewRatingManagerWithConfig(100000, -0.1)
	if rm3.reputationDecay != DefaultReputationDecay {
		t.Errorf("expected default decay for negative input, got %f", rm3.reputationDecay)
	}
}

// TestRegisterNode tests registering a node with the rating manager
func TestRegisterNode(t *testing.T) {
	rm := NewRatingManager()

	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Should succeed
	err := rm.RegisterNode(node)
	if err != nil {
		t.Errorf("RegisterNode failed: %v", err)
	}

	// Should fail for nil node
	err = rm.RegisterNode(nil)
	if err == nil {
		t.Error("expected error for nil node")
	}

	// Should fail for duplicate registration
	err = rm.RegisterNode(node)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

// TestUnregisterNode tests unregistering a node
func TestUnregisterNode(t *testing.T) {
	rm := NewRatingManager()

	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Should fail for non-existent node
	err := rm.UnregisterNode(pubKey)
	if err != ErrNodeNotTracked {
		t.Errorf("expected ErrNodeNotTracked, got %v", err)
	}

	// Register then unregister
	rm.RegisterNode(node)
	err = rm.UnregisterNode(pubKey)
	if err != nil {
		t.Errorf("UnregisterNode failed: %v", err)
	}

	// Should fail after unregister
	err = rm.UnregisterNode(pubKey)
	if err != ErrNodeNotTracked {
		t.Errorf("expected ErrNodeNotTracked after unregister, got %v", err)
	}
}

// TestSubmitRating tests submitting ratings
func TestSubmitRating(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])
	var userPubKey [32]byte
	rand.Read(userPubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	// Submit valid rating
	submission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      8.5,
		Reason:     "Good service",
		Timestamp:  time.Now().Unix(),
	}

	err := rm.SubmitRating(submission)
	if err != nil {
		t.Errorf("SubmitRating failed: %v", err)
	}

	// Submit duplicate rating (same user)
	err = rm.SubmitRating(submission)
	if err != ErrRatingExists {
		t.Errorf("expected ErrRatingExists, got %v", err)
	}

	// Submit rating for non-existent node
	var otherPubKey [32]byte
	rand.Read(otherPubKey[:])
	invalidSubmission := RatingSubmission{
		NodePubKey: otherPubKey,
		UserPubKey: userPubKey,
		Score:      5.0,
	}
	err = rm.SubmitRating(invalidSubmission)
	if err != ErrNodeNotTracked {
		t.Errorf("expected ErrNodeNotTracked, got %v", err)
	}

	// Test invalid score - too high
	highScoreSubmission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: [32]byte{2},
		Score:      11.0,
	}
	err = rm.SubmitRating(highScoreSubmission)
	if err == nil {
		t.Error("expected error for score > 10")
	}

	// Test invalid score - negative
	lowScoreSubmission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: [32]byte{3},
		Score:      -1.0,
	}
	err = rm.SubmitRating(lowScoreSubmission)
	if err == nil {
		t.Error("expected error for negative score")
	}

	// Test reason too long
	longReasonSubmission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: [32]byte{4},
		Score:      5.0,
		Reason:     string(make([]byte, 501)),
	}
	err = rm.SubmitRating(longReasonSubmission)
	if err == nil {
		t.Error("expected error for reason too long")
	}

	// Test future timestamp
	futureSubmission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: [32]byte{5},
		Score:      5.0,
		Timestamp:  time.Now().Add(1 * time.Hour).Unix(),
	}
	err = rm.SubmitRating(futureSubmission)
	if err == nil {
		t.Error("expected error for future timestamp")
	}

	// Test old timestamp (>30 days)
	oldSubmission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: [32]byte{6},
		Score:      5.0,
		Timestamp:  time.Now().Add(-31 * 24 * time.Hour).Unix(),
	}
	err = rm.SubmitRating(oldSubmission)
	if err == nil {
		t.Error("expected error for old timestamp")
	}
}

// TestConfirmRating tests confirming pending ratings
func TestConfirmRating(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])
	var userPubKey [32]byte
	rand.Read(userPubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	// Submit a rating
	submission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      7.0,
		Reason:     "Test rating",
		Timestamp:  time.Now().Unix(),
	}
	rm.SubmitRating(submission)

	// Confirm the rating
	err := rm.ConfirmRating(nodePubKey, userPubKey, 100)
	if err != nil {
		t.Errorf("ConfirmRating failed: %v", err)
	}

	// Verify rating is in history
	history := rm.GetNodeRatings(nodePubKey)
	if len(history) != 1 {
		t.Errorf("expected 1 rating in history, got %d", len(history))
	}

	if !history[0].Confirmed {
		t.Error("expected rating to be confirmed")
	}

	// Try to confirm non-existent rating
	err = rm.ConfirmRating(nodePubKey, [32]byte{99}, 100)
	if err == nil {
		t.Error("expected error for non-existent rating")
	}
}

// TestUpdateNodeReputation tests updating node reputation
func TestUpdateNodeReputation(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	initialRep := node.Reputation // 5.0

	// Should fail for non-existent node
	err := rm.UpdateNodeReputation([32]byte{99})
	if err != ErrNodeNotTracked {
		t.Errorf("expected ErrNodeNotTracked, got %v", err)
	}

	// Add ratings and confirm them
	var userPubKey [32]byte
	rand.Read(userPubKey[:])

	submission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      9.0,
	}
	rm.SubmitRating(submission)
	rm.ConfirmRating(nodePubKey, userPubKey, 100)

	// Update reputation
	err = rm.UpdateNodeReputation(nodePubKey)
	if err != nil {
		t.Errorf("UpdateNodeReputation failed: %v", err)
	}

	// Reputation should have changed
	if node.Reputation == initialRep {
		t.Error("expected reputation to change after rating")
	}
}

// TestApplyReputationDecay tests applying reputation decay
func TestApplyReputationDecay(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	node.Reputation = 7.0
	rm.RegisterNode(node)

	// Apply decay
	rm.ApplyReputationDecay()

	// With no ratings, reputation should decay
	if node.Reputation >= 7.0 {
		t.Error("expected reputation to decay")
	}
}

// TestGetNodeRatings tests getting node ratings
func TestGetNodeRatings(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	// Get ratings for non-existent node
	ratings := rm.GetNodeRatings(nodePubKey)
	if len(ratings) != 0 {
		t.Errorf("expected 0 ratings, got %d", len(ratings))
	}

	// Add and confirm ratings
	var userPubKey1 [32]byte
	rand.Read(userPubKey1[:])
	submission1 := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey1,
		Score:      8.0,
	}
	rm.SubmitRating(submission1)
	rm.ConfirmRating(nodePubKey, userPubKey1, 100)

	// Add pending rating (not confirmed)
	var userPubKey2 [32]byte
	rand.Read(userPubKey2[:])
	submission2 := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey2,
		Score:      9.0,
	}
	rm.SubmitRating(submission2)

	// Get all ratings - should only return confirmed
	ratings = rm.GetNodeRatings(nodePubKey)
	if len(ratings) != 1 {
		t.Errorf("expected 1 confirmed rating, got %d", len(ratings))
	}
}

// TestGetPendingRatings tests getting pending ratings
func TestGetPendingRatings(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	// Get pending for untracked node
	pending := rm.GetPendingRatings(nodePubKey)
	// This will return nil/empty since node isn't tracked

	// Add pending rating
	var userPubKey [32]byte
	rand.Read(userPubKey[:])
	submission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      7.0,
	}
	rm.SubmitRating(submission)

	pending = rm.GetPendingRatings(nodePubKey)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending rating, got %d", len(pending))
	}
}

// TestGetRatingStats tests getting rating statistics
func TestGetRatingStats(t *testing.T) {
	rm := NewRatingManager()

	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])

	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	// Get stats for node with no ratings
	stats, err := rm.GetRatingStats(nodePubKey)
	if err != nil {
		t.Errorf("GetRatingStats failed: %v", err)
	}
	if stats.TotalRatings != 0 {
		t.Errorf("expected 0 total ratings, got %d", stats.TotalRatings)
	}

	// Add multiple ratings
	for i := 0; i < 5; i++ {
		var userPubKey [32]byte
		rand.Read(userPubKey[:])
		submission := RatingSubmission{
			NodePubKey: nodePubKey,
			UserPubKey: userPubKey,
			Score:      float64(i + 5), // 5, 6, 7, 8, 9
		}
		rm.SubmitRating(submission)
		rm.ConfirmRating(nodePubKey, userPubKey, uint64(100+i))
	}

	stats, err = rm.GetRatingStats(nodePubKey)
	if err != nil {
		t.Errorf("GetRatingStats failed: %v", err)
	}

	if stats.TotalRatings != 5 {
		t.Errorf("expected 5 total ratings, got %d", stats.TotalRatings)
	}

	if stats.AvgScore != 7.0 {
		t.Errorf("expected avg 7.0, got %f", stats.AvgScore)
	}

	if stats.MinScore != 5.0 {
		t.Errorf("expected min 5.0, got %f", stats.MinScore)
	}

	if stats.MaxScore != 9.0 {
		t.Errorf("expected max 9.0, got %f", stats.MaxScore)
	}

	// Standard deviation should be > 0 for non-uniform scores
	if stats.ScoreStdDev <= 0 {
		t.Error("expected positive standard deviation")
	}
}

// TestGetTopNodes tests getting top nodes by reputation
func TestGetTopNodes(t *testing.T) {
	rm := NewRatingManager()

	// Create multiple nodes with different reputations
	for i := 0; i < 5; i++ {
		var pubKey [32]byte
		rand.Read(pubKey[:])
		node := NewInferenceNode(pubKey, 1, 1000000)
		node.Reputation = float64(10 - i) // 10, 9, 8, 7, 6
		if i == 0 || i == 2 {             // Only 2 are online
			node.Register()
		}
		rm.RegisterNode(node)
	}

	// Get top 3
	topNodes := rm.GetTopNodes(3)
	if len(topNodes) != 2 { // Only 2 are online
		t.Errorf("expected 2 online nodes, got %d", len(topNodes))
	}

	// Check they are sorted by reputation (highest first)
	if len(topNodes) >= 2 {
		if topNodes[0].Reputation < topNodes[1].Reputation {
			t.Error("expected top nodes to be sorted by reputation descending")
		}
	}

	// Test with n=0
	topNodes = rm.GetTopNodes(0)
	if len(topNodes) != 0 {
		t.Errorf("expected 0 nodes for n=0, got %d", len(topNodes))
	}

	// Test with n > total
	topNodes = rm.GetTopNodes(100)
	if len(topNodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(topNodes))
	}
}

// TestGetNodeCount tests getting node count
func TestGetNodeCount(t *testing.T) {
	rm := NewRatingManager()

	if rm.GetNodeCount() != 0 {
		t.Error("expected 0 initial nodes")
	}

	// Add some nodes
	for i := 0; i < 3; i++ {
		var pubKey [32]byte
		rand.Read(pubKey[:])
		node := NewInferenceNode(pubKey, 1, 1000000)
		rm.RegisterNode(node)
	}

	if rm.GetNodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", rm.GetNodeCount())
	}
}

// TestGetAllNodes tests getting all nodes
func TestGetAllNodes(t *testing.T) {
	rm := NewRatingManager()

	allNodes := rm.GetAllNodes()
	if len(allNodes) != 0 {
		t.Error("expected empty list initially")
	}

	// Add nodes
	var pubKey1 [32]byte
	rand.Read(pubKey1[:])
	node1 := NewInferenceNode(pubKey1, 1, 1000000)
	rm.RegisterNode(node1)

	allNodes = rm.GetAllNodes()
	if len(allNodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(allNodes))
	}
}

// TestSerializeRating tests rating serialization
func TestSerializeRating(t *testing.T) {
	rm := NewRatingManager()

	record := RatingRecord{
		NodePubKey:  [32]byte{1, 2, 3},
		UserPubKey:  [32]byte{4, 5, 6},
		Score:       8.5,
		Reason:      "Test",
		Timestamp:   time.Now(),
		BlockHeight: 100,
		Confirmed:   true,
	}

	data, err := rm.SerializeRating(record)
	if err != nil {
		t.Fatalf("SerializeRating failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty serialized data")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Errorf("invalid JSON: %v", err)
	}
}

// TestDeserializeRating tests rating deserialization
func TestDeserializeRating(t *testing.T) {
	rm := NewRatingManager()

	original := RatingRecord{
		NodePubKey:  [32]byte{1, 2, 3},
		UserPubKey:  [32]byte{4, 5, 6},
		Score:       8.5,
		Reason:      "Test",
		Timestamp:   time.Now(),
		BlockHeight: 100,
		Confirmed:   true,
	}

	// Serialize
	data, err := rm.SerializeRating(original)
	if err != nil {
		t.Fatalf("SerializeRating failed: %v", err)
	}

	// Deserialize
	parsed, err := rm.DeserializeRating(data)
	if err != nil {
		t.Fatalf("DeserializeRating failed: %v", err)
	}

	if parsed.Score != original.Score {
		t.Errorf("expected score %.2f, got %.2f", original.Score, parsed.Score)
	}

	if parsed.Reason != original.Reason {
		t.Errorf("expected reason %s, got %s", original.Reason, parsed.Reason)
	}

	// Test invalid data
	_, err = rm.DeserializeRating([]byte("invalid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestVerifyRating tests rating verification
func TestVerifyRating(t *testing.T) {
	rm := NewRatingManager()

	// Valid confirmed rating
	validRecord := RatingRecord{
		Score:     5.0,
		Confirmed: true,
	}
	if !rm.VerifyRating(validRecord) {
		t.Error("expected valid rating to pass verification")
	}

	// Invalid - unconfirmed
	unconfirmedRecord := RatingRecord{
		Score:     5.0,
		Confirmed: false,
	}
	if rm.VerifyRating(unconfirmedRecord) {
		t.Error("expected unconfirmed rating to fail")
	}

	// Invalid - score too low
	lowScoreRecord := RatingRecord{
		Score:     -1.0,
		Confirmed: true,
	}
	if rm.VerifyRating(lowScoreRecord) {
		t.Error("expected low score to fail")
	}

	// Invalid - score too high
	highScoreRecord := RatingRecord{
		Score:     11.0,
		Confirmed: true,
	}
	if rm.VerifyRating(highScoreRecord) {
		t.Error("expected high score to fail")
	}
}

// TestBytesToPubKey tests converting bytes to public key
func TestBytesToPubKey(t *testing.T) {
	// Valid 32 bytes
	validBytes := make([]byte, 32)
	validBytes[0] = 1

	pubKey, err := BytesToPubKey(validBytes)
	if err != nil {
		t.Errorf("BytesToPubKey failed: %v", err)
	}

	if pubKey[0] != 1 {
		t.Error("expected first byte to be 1")
	}

	// Invalid - too short
	_, err = BytesToPubKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short input")
	}

	// Invalid - too long
	_, err = BytesToPubKey(make([]byte, 33))
	if err == nil {
		t.Error("expected error for long input")
	}
}

// TestPubKeyToBytes tests converting public key to bytes
func TestPubKeyToBytes(t *testing.T) {
	var pubKey [32]byte
	pubKey[0] = 1
	pubKey[31] = 255

	bytes := PubKeyToBytes(pubKey)

	if len(bytes) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(bytes))
	}

	if bytes[0] != 1 {
		t.Error("expected first byte to be 1")
	}

	if bytes[31] != 255 {
		t.Error("expected last byte to be 255")
	}
}

// TestGenerateTestRating tests generating test ratings
func TestGenerateTestRating(t *testing.T) {
	var nodePubKey [32]byte
	nodePubKey[0] = 1
	var userPubKey [32]byte
	userPubKey[0] = 2

	rating := GenerateTestRating(nodePubKey, userPubKey, 7.5)

	if rating.NodePubKey != nodePubKey {
		t.Error("expected node pubkey to match")
	}

	if rating.UserPubKey != userPubKey {
		t.Error("expected user pubkey to match")
	}

	if rating.Score != 7.5 {
		t.Errorf("expected score 7.5, got %f", rating.Score)
	}

	if rating.Reason != "Test rating" {
		t.Error("expected default reason")
	}

	if rating.Timestamp == 0 {
		t.Error("expected timestamp to be set")
	}
}

// TestRatingManagerStop tests stopping the rating manager
func TestRatingManagerStop(t *testing.T) {
	rm := NewRatingManager()

	// Add some pending ratings
	var nodePubKey [32]byte
	rand.Read(nodePubKey[:])
	node := NewInferenceNode(nodePubKey, 1, 1000000)
	rm.RegisterNode(node)

	var userPubKey [32]byte
	rand.Read(userPubKey[:])
	submission := RatingSubmission{
		NodePubKey: nodePubKey,
		UserPubKey: userPubKey,
		Score:      7.0,
	}
	rm.SubmitRating(submission)

	// Stop should clear pending ratings
	rm.Stop()

	// After stop, pending ratings should be cleared
	pending := rm.GetPendingRatings(nodePubKey)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after stop, got %d", len(pending))
	}
}
