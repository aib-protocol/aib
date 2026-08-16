package channel

import (
	"testing"
	"time"
)

func TestCreateInferenceChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	// 生成测试公钥
	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// 创建通道
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 验证通道创建成功
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

	// 验证可以通过ID获取通道
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

	// 测试无效等级
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

	// 测试低于最小存款
	_, err := manager.CreateChannel(userPubKey, nodePubKey, 1000, 2)
	if err == nil {
		t.Error("Expected error for insufficient deposit")
	}
}

func TestRecordInference(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// 创建 Level 2 通道（费用 1000000 satoshi = 0.01 AIB）
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	initialBalance := channel.UserBalance

	// 执行推理
	err = channel.RecordInference()
	if err != nil {
		t.Fatalf("RecordInference failed: %v", err)
	}

	// 验证余额更新
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

	// 再次推理
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

	// 创建 Level 3 通道（费用 10000000 satoshi = 0.1 AIB）
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 5000000, 3)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 尝试推理（余额不足）
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

	// 关闭通道
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 尝试推理（通道已关闭）
	err = channel.RecordInference()
	if err != ErrChannelNotOpen {
		t.Errorf("Expected ErrChannelNotOpen, got %v", err)
	}
}

func TestSettle(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// 创建通道并执行推理
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 执行几次推理
	_ = channel.RecordInference()
	_ = channel.RecordInference()
	_ = channel.RecordInference()

	// 结算
	settlement, err := channel.Settle()
	if err != nil {
		t.Fatalf("Settle failed: %v", err)
	}

	// 验证结算数据
	if settlement.FinalUserBal != channel.UserBalance {
		t.Errorf("FinalUserBal mismatch: %d vs %d", settlement.FinalUserBal, channel.UserBalance)
	}
	if settlement.FinalNodeBal != channel.NodeBalance {
		t.Errorf("FinalNodeBal mismatch: %d vs %d", settlement.FinalNodeBal, channel.NodeBalance)
	}
	if settlement.InferenceCount != 3 {
		t.Errorf("Expected InferenceCount 3, got %d", settlement.InferenceCount)
	}

	// 验证结算数据有效性
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

	// 关闭通道
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 尝试结算
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

	// 挑战
	reason := "Fraudulent state submitted"
	err = channel.Challenge(reason)
	if err != nil {
		t.Fatalf("Challenge failed: %v", err)
	}

	// 验证通道状态
	if channel.Status != ICDisputed {
		t.Errorf("Expected status ICDisputed, got %d", channel.Status)
	}
	if channel.ChallengeReason != reason {
		t.Errorf("Challenge reason mismatch: %s vs %s", channel.ChallengeReason, reason)
	}
	if channel.ChallengeEnd == nil {
		t.Error("ChallengeEnd should be set")
	} else {
		// 验证挑战结束时间约为24小时后
		expectedEnd := time.Now().Add(24 * time.Hour)
		diff := expectedEnd.Sub(*channel.ChallengeEnd)
		if diff < -time.Minute || diff > time.Minute {
			t.Errorf("ChallengeEnd time not approximately 24 hours from now")
		}
	}

	// 验证 IsInDispute
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

	// 关闭通道
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 尝试挑战
	err = channel.Challenge("test reason")
	if err != ErrChannelAlreadyClosed {
		t.Errorf("Expected ErrChannelAlreadyClosed, got %v", err)
	}
}

func TestCloseChannel(t *testing.T) {
	manager := NewInferenceChannelManager()

	userPubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodePubKey := [32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}

	// 创建通道
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 100000000, 2)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 关闭通道
	err = channel.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 验证状态
	if channel.Status != ICClosed {
		t.Errorf("Expected status ICClosed, got %d", channel.Status)
	}
	if channel.ClosedAt == 0 {
		t.Error("ClosedAt should be set")
	}

	// 验证 IsClosed
	if !channel.IsClosed() {
		t.Error("Expected IsClosed to return true")
	}

	// 验证 CanSettle
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

	// 第一次关闭
	err = channel.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// 第二次关闭
	err = channel.Close()
	if err != ErrChannelAlreadyClosed {
		t.Errorf("Expected ErrChannelAlreadyClosed, got %v", err)
	}
}

func TestGetChannel_NotFound(t *testing.T) {
	manager := NewInferenceChannelManager()

	// 尝试获取不存在的通道
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

	// 创建 Level 1 通道（费用 100000 satoshi）
	channel, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 1)
	if err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 验证剩余推理次数
	remaining := channel.GetRemainingInferences()
	expected := uint64(10000000 / 100000) // 100
	if remaining != expected {
		t.Errorf("Expected %d remaining inferences, got %d", expected, remaining)
	}

	// 执行推理
	_ = channel.RecordInference()
	_ = channel.RecordInference()

	// 验证更新后的剩余推理次数
	remaining = channel.GetRemainingInferences()
	expected = uint64((10000000 - 2*100000) / 100000)
	if remaining != expected {
		t.Errorf("Expected %d remaining inferences, got %d", expected, remaining)
	}
}

func TestMultipleChannels(t *testing.T) {
	manager := NewInferenceChannelManager()

	// 创建多个通道
	for i := 0; i < 5; i++ {
		userPubKey := [32]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		nodePubKey := [32]byte{byte(i + 1), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

		_, err := manager.CreateChannel(userPubKey, nodePubKey, 10000000, 1)
		if err != nil {
			t.Fatalf("CreateChannel %d failed: %v", i, err)
		}
	}

	// 验证通道数量
	count := manager.GetChannelCount()
	if count != 5 {
		t.Errorf("Expected 5 channels, got %d", count)
	}

	// 验证 GetChannels
	channels := manager.GetChannels()
	if len(channels) != 5 {
		t.Errorf("Expected 5 channels, got %d", len(channels))
	}
}

func TestInferencePrices(t *testing.T) {
	// 验证价格表
	if InferencePrices[1] != 100000 {
		t.Errorf("Level 1 price should be 100000, got %d", InferencePrices[1])
	}
	if InferencePrices[2] != 1000000 {
		t.Errorf("Level 2 price should be 1000000, got %d", InferencePrices[2])
	}
	if InferencePrices[3] != 10000000 {
		t.Errorf("Level 3 price should be 10000000, got %d", InferencePrices[3])
	}

	// 验证费用获取
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
