package channel

import (
	"testing"
	"time"
)

func TestCreateInferenceChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	// Generate test public keys
	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// createchannel
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// verifychannelcreatesuccess
	if channel.UserBalance != 10000000 {
		t.Errorf("Expected UserBalance 10000000, got %d", channel.UserBalance)
	}
	if channel.NodeBalance != 0 {
		t.Errorf("Expected NodeBalance 0, got %d", channel.NodeBalance)
	}
	if channel.Level != 2 {
		t.Errorf("Expected Level 2, got %d", channel.Level)
	}
	if channel.Status != ICOpen {
		t.Errorf("Expected status ICOpen, got %d", channel.Status)
	}
	if channel.InferenceCount != 0 {
		t.Errorf("Expected InferenceCount 0, got %d", channel.InferenceCount)
	}

	// Verify the channel can be retrieved by ID
	channel2, err := manager.GetChannel(channel.ChannelID)
	if err != nil {
		t.Fatalf("GetChannel failed: %v", err)
	}
	if channel2.ChannelID != channel.ChannelID {
		t.Errorf("Channel ID mismatch")
	}
}

func TestCreateInferenceChannel_InvalidLevel(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Test an invalid level
	_, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 0)
	if err != ErrInvalidLevel {
		t.Errorf("Expected ErrInvalidLevel for level 0, got %v", err)
	}

	_, err = manager.CreateChannel(userPubKey, nodePubKey, 10000000, 4)
	if err != ErrInvalidLevel {
		t.Errorf("Expected ErrInvalidLevel for level 4, got %v", err)
	}
}

func TestCreateInferenceChannel_InsufficientDeposit(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Test a deposit below the minimum
	_, err := manager.CreateChannel(userPubKey, nodePubKey, 1000, 2)
	if err == nil {
		t.Error("Expected error for insufficient deposit")
	}
}

func TestRecordInference(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Create a Level 2 channel (fee 1000000 satoshi = 0.01 AIB)
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	initialBalance := channel.UserBalance

	// Perform inference
	err = channel.RecordInference()
	if err != nil {
		t.Fatalf("RecordInference failed: %v", err)
	}

	// verifybalanceupdate
	fee := InferencePrices[2] // 1000000
	expectedBalance := initialBalance - fee
	if channel.UserBalance != expectedBalance {
		t.Errorf("Expected UserBalance %d, got %d", expectedBalance, channel.UserBalance)
	}
	if channel.NodeBalance != fee {
		t.Errorf("Expected NodeBalance %d, got %d", fee, channel.NodeBalance)
	}
	if channel.InferenceCount != 1 {
		t.Errorf("Expected InferenceCount 1, got %d", channel.InferenceCount)
	}
	if channel.SequenceNum != 1 {
		t.Errorf("Expected SequenceNum 1, got %d", channel.SequenceNum)
	}

	// Perform another inference
	err = channel.RecordInference()
	if err != nil {
		t.Fatalf("Second RecordInference failed: %v", err)
	}

	if channel.InferenceCount != 2 {
		t.Errorf("Expected InferenceCount 2, got %d", channel.InferenceCount)
	}
	if channel.SequenceNum != 2 {
		t.Errorf("Expected SequenceNum 2, got %d", channel.SequenceNum)
	}
}

func TestRecordInference_InsufficientBalance(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Create a Level 3 channel (fee 10000000 satoshi = 0.1 AIB)
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 5000000, 3)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Attempt an inference (insufficient balance)
	err = channel.RecordInference()
	if err != ErrInsufficientBalance {
		t.Errorf("Expected ErrInsufficientBalance, got %v", err)
	}
}

func TestRecordInference_ClosedChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Close the channel
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Attempt an inference (channel already closed)
	err = channel.RecordInference()
	if err != ErrChannelNotOpen {
		t.Errorf("Expected ErrChannelNotOpen, got %v", err)
	}
}

func TestSettle(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Create a channel and perform inferences
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Perform several inferences
	_ = channel.RecordInference()
	_ = channel.RecordInference()
	_ = channel.RecordInference()

	// settlement
	settlement, err := channel.Settle()
	if err != nil {
		t.Fatalf("Settle failed: %v", err)
	}

	// verifysettlementdata
	if settlement.FinalUserBal != channel.UserBalance {
		t.Errorf("FinalUserBal mismatch: %d vs %d", settlement.FinalUserBal, channel.UserBalance)
	}
	if settlement.FinalNodeBal != channel.NodeBalance {
		t.Errorf("FinalNodeBal mismatch: %d vs %d", settlement.FinalNodeBal, channel.NodeBalance)
	}
	if settlement.InferenceCount != 3 {
		t.Errorf("Expected InferenceCount 3, got %d", settlement.InferenceCount)
	}

	// Verify the settlement data is valid
	err = settlement.IsValid()
	if err != nil {
		t.Errorf("Settlement validation failed: %v", err)
	}
}

func TestSettle_ClosedChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Close the channel
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Attempt to settle
	_, err = channel.Settle()
	if err != ErrChannelAlreadyClosed {
		t.Errorf("Expected ErrChannelAlreadyClosed, got %v", err)
	}
}

func TestChallenge(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Challenge the channel
	reason := "Fraudulent state submitted"
	err = channel.Challenge(reason)
	if err != nil {
		t.Fatalf("Challenge failed: %v", err)
	}

	// verifychannelstatus
	if channel.Status != ICDisputed {
		t.Errorf("Expected status ICDisputed, got %d", channel.Status)
	}
	if channel.ChallengeReason != reason {
		t.Errorf("Challenge reason mismatch: %s vs %s", channel.ChallengeReason, reason)
	}
	if channel.ChallengeEnd == nil {
		t.Error("ChallengeEnd should be set")
	} else {
		// Verify the challenge end time is approximately 24 hours from now
		expectedEnd := time.Now().Add(24 * time.Hour)
		diff := expectedEnd.Sub(*channel.ChallengeEnd)
		if diff < -time.Minute || diff > time.Minute {
			t.Errorf("ChallengeEnd time not approximately 24 hours from now")
		}
	}

	// verify IsInDispute
	if !channel.IsInDispute() {
		t.Error("Expected IsInDispute to return true")
	}
}

func TestChallenge_AlreadyClosed(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Close the channel
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Attempt to challenge
	err = channel.Challenge("test reason")
	if err != ErrChannelAlreadyClosed {
		t.Errorf("Expected ErrChannelAlreadyClosed, got %v", err)
	}
}

func TestCloseChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// createchannel
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Close the channel
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// verifystatus
	if channel.Status != ICClosed {
		t.Errorf("Expected status ICClosed, got %d", channel.Status)
	}
	if channel.ClosedAt == 0 {
		t.Error("ClosedAt should be set")
	}

	// verify IsClosed
	if !channel.IsClosed() {
		t.Error("Expected IsClosed to return true")
	}

	// verify CanSettle
	if channel.CanSettle() {
		t.Error("Expected CanSettle to return false for closed channel")
	}
}

func TestCloseChannel_AlreadyClosed(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// First close
	err = channel.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close
	err = channel.Close()
	if err != ErrChannelAlreadyClosed {
		t.Errorf("Expected ErrChannelAlreadyClosed, got %v", err)
	}
}

func TestGetChannel_NotFound(t *testing.T) {
	manager := NewInferenceChannelManager()

	// Attempt to get a non-existent channel
	var notFoundID [32]byte
	notFoundID[0] = 0xFF

	_, err := manager.GetChannel(notFoundID)
	if err != ErrChannelNotFound {
		t.Errorf("Expected ErrChannelNotFound, got %v", err)
	}
}

func TestGetRemainingInferences(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// Create a Level 1 channel (fee 100000 satoshi)
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 1)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// Verify the remaining inference count
	remaining := channel.GetRemainingInferences()
	expected := uint64(10000000 / 100000) // 100
	if remaining != expected {
		t.Errorf("Expected %d remaining inferences, got %d", expected, remaining)
	}

	// Perform inferences
	_ = channel.RecordInference()
	_ = channel.RecordInference()

	// Verify the updated remaining inference count
	remaining = channel.GetRemainingInferences()
	expected = uint64((10000000 - 2*100000) / 100000)
	if remaining != expected {
		t.Errorf("Expected %d remaining inferences, got %d", expected, remaining)
	}
}

func TestMultipleChannels(t *testing.T) {
	manager := NewInferenceChannelManager()

	// Create multiple channels
	for i := 0; i < 5; i++ {
		userPubKey := [32]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		nodePubKey := [32]byte{byte(i + 1), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

		_, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 1)
		if err != nil {
			t.Fatalf("CreateChannel %d failed: %v", i, err)
		}
	}

	// Verify the channel count
	count := manager.GetChannelCount()
	if count != 5 {
		t.Errorf("Expected 5 channels, got %d", count)
	}

	// verify GetChannels
	channels := manager.GetChannels()
	if len(channels) != 5 {
		t.Errorf("Expected 5 channels, got %d", len(channels))
	}
}

func TestInferencePrices(t *testing.T) {
	// Verify the price table
	if InferencePrices[1] != 100000 {
		t.Errorf("Level 1 price should be 100000, got %d", InferencePrices[1])
	}
	if InferencePrices[2] != 1000000 {
		t.Errorf("Level 2 price should be 1000000, got %d", InferencePrices[2])
	}
	if InferencePrices[3] != 10000000 {
		t.Errorf("Level 3 price should be 10000000, got %d", InferencePrices[3])
	}

	// Verify fee retrieval
	channel := &InferenceChannel{Level: 1}
	if channel.GetFee() != 100000 {
		t.Errorf("Level 1 fee should be 100000, got %d", channel.GetFee())
	}

	channel.Level = 2
	if channel.GetFee() != 1000000 {
		t.Errorf("Level 2 fee should be 1000000, got %d", channel.GetFee())
	}

	channel.Level = 3
	if channel.GetFee() != 10000000 {
		t.Errorf("Level 3 fee should be 10000000, got %d", channel.GetFee())
	}
}
