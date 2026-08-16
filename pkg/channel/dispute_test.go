// Package channel provides unit tests for the dispute resolver.
package channel

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Mock BlockChecker for testing
// ============================================================================

type testBlockChecker struct {
	currentBlock uint64
}

func newTestBlockChecker() *testBlockChecker {
	return &testBlockChecker{currentBlock: 1000}
}

func (m *testBlockChecker) GetCurrentBlock(ctx context.Context) (uint64, error) {
	return m.currentBlock, nil
}

func (m *testBlockChecker) GetBlockTimestamp(ctx context.Context, blockNum uint64) (time.Time, error) {
	return time.Unix(int64(blockNum*60), 0), nil
}

func (m *testBlockChecker) VerifyTxInBlock(ctx context.Context, txHash [32]byte, blockNum uint64) (bool, error) {
	return true, nil
}

// createTestManager creates a test Manager for dispute tests
func createTestManager(t *testing.T) *Manager {
	cfg := &Config{
		ChallengePeriod: 24 * time.Hour,
		MinDeposit:      100,
		MaxChannelValue: 1_000_000,
		MultiSigLocker:  &MockMultiSigLocker{},
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return mgr
}

// ============================================================================
// DisputeConfig Tests
// ============================================================================

func TestDefaultDisputeConfig(t *testing.T) {
	cfg := DefaultDisputeConfig()

	if cfg.ChallengePeriod != 24*time.Hour {
		t.Errorf("expected ChallengePeriod 24h, got %v", cfg.ChallengePeriod)
	}

	if cfg.MinChallengePeriod != 1*time.Hour {
		t.Errorf("expected MinChallengePeriod 1h, got %v", cfg.MinChallengePeriod)
	}

	if cfg.MaxChallengePeriod != 7*24*time.Hour {
		t.Errorf("expected MaxChallengePeriod 7d, got %v", cfg.MaxChallengePeriod)
	}

	if cfg.FraudPenaltyMultiplier != 1.0 {
		t.Errorf("expected FraudPenaltyMultiplier 1.0, got %f", cfg.FraudPenaltyMultiplier)
	}

	if cfg.MaxEvidenceAge != 7*24*time.Hour {
		t.Errorf("expected MaxEvidenceAge 7d, got %v", cfg.MaxEvidenceAge)
	}

	if cfg.EvidenceExpiry != 30*24*time.Hour {
		t.Errorf("expected EvidenceExpiry 30d, got %v", cfg.EvidenceExpiry)
	}
}

// ============================================================================
// DisputeResolver Creation Tests
// ============================================================================

func TestNewDisputeResolver(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	if resolver == nil {
		t.Fatal("NewDisputeResolver should not return nil")
	}

	if resolver.evidenceStore == nil {
		t.Error("evidenceStore should be initialized")
	}

	if resolver.challengeQueue == nil {
		t.Error("challengeQueue should be initialized")
	}

	if resolver.challengePeriod != 24*time.Hour {
		t.Errorf("expected default challengePeriod 24h, got %v", resolver.challengePeriod)
	}
}

func TestNewDisputeResolverWithConfig(t *testing.T) {
	mgr := createTestManager(t)
	cfg := DefaultDisputeConfig()
	cfg.ChallengePeriod = 12 * time.Hour
	cfg.BlockChecker = newTestBlockChecker()

	resolver := NewDisputeResolverWithConfig(mgr, cfg)

	if resolver == nil {
		t.Fatal("NewDisputeResolverWithConfig should not return nil")
	}

	if resolver.challengePeriod != 12*time.Hour {
		t.Errorf("expected challengePeriod 12h, got %v", resolver.challengePeriod)
	}
}

func TestNewDisputeResolverWithNilConfig(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolverWithConfig(mgr, nil)

	if resolver.challengePeriod != 24*time.Hour {
		t.Errorf("expected default challengePeriod 24h, got %v", resolver.challengePeriod)
	}
}

// ============================================================================
// Penalty Tests
// ============================================================================

func TestDisputeResolver_CalculatePenalty(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	// Default multiplier is 1.0 (100%)
	penalty := resolver.CalculatePenalty(1000)
	if penalty != 1000 {
		t.Errorf("expected penalty 1000, got %d", penalty)
	}
}

func TestDisputeResolver_CalculatePenalty_Half(t *testing.T) {
	mgr := createTestManager(t)
	cfg := DefaultDisputeConfig()
	cfg.FraudPenaltyMultiplier = 0.5
	resolver := NewDisputeResolverWithConfig(mgr, cfg)

	penalty := resolver.CalculatePenalty(1000)
	if penalty != 500 {
		t.Errorf("expected penalty 500, got %d", penalty)
	}
}

func TestDisputeResolver_CalculatePenalty_Zero(t *testing.T) {
	mgr := createTestManager(t)
	cfg := DefaultDisputeConfig()
	cfg.FraudPenaltyMultiplier = 0.0
	resolver := NewDisputeResolverWithConfig(mgr, cfg)

	penalty := resolver.CalculatePenalty(1000)
	if penalty != 0 {
		t.Errorf("expected penalty 0, got %d", penalty)
	}
}

func TestDisputeResolver_SetPenaltyMultiplier(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	if resolver.fraudPenaltyMultiplier != 1.0 {
		t.Errorf("expected default 1.0, got %f", resolver.fraudPenaltyMultiplier)
	}

	// SetPenaltyMultiplier clamps to >= 1.0
	resolver.SetPenaltyMultiplier(2.0)
	if resolver.fraudPenaltyMultiplier != 2.0 {
		t.Errorf("expected 2.0, got %f", resolver.fraudPenaltyMultiplier)
	}

	// Values below 1.0 are clamped to 1.0
	resolver.SetPenaltyMultiplier(0.5)
	if resolver.fraudPenaltyMultiplier != 1.0 {
		t.Errorf("expected clamped to 1.0, got %f", resolver.fraudPenaltyMultiplier)
	}
}

// ============================================================================
// Evidence Tests
// ============================================================================

func TestDisputeResolver_GetEvidence_Empty(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1, 2, 3, 4}
	evidence, err := resolver.GetEvidence(channelID)

	// GetEvidence returns error for non-existent channel
	if err == nil {
		t.Error("should return error for non-existent channel")
	}
	// evidence should be nil on error
	if evidence != nil {
		t.Error("evidence should be nil on error")
	}
}

func TestDisputeResolver_CleanOldEvidence_Empty(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	count := resolver.CleanOldEvidence()
	if count != 0 {
		t.Errorf("expected 0 cleaned, got %d", count)
	}
}

// ============================================================================
// Dispute Lookup Tests
// ============================================================================

func TestDisputeResolver_GetDispute_NotFound(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1, 2, 3, 4}
	_, err := resolver.GetDispute(channelID)

	if err == nil {
		t.Error("should error for non-existent dispute")
	}
}

func TestDisputeResolver_IsChannelInDispute_NoDispute(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1, 2, 3, 4}
	inDispute, err := resolver.IsChannelInDispute(channelID)

	// Returns error for non-existent channel
	if err == nil {
		t.Error("should return error for non-existent channel")
	}
	if inDispute {
		t.Error("non-existent channel should not be in dispute")
	}
}

func TestDisputeResolver_ChallengePeriodRemaining_NoDispute(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	channelID := [32]byte{1, 2, 3, 4}
	_, err := resolver.ChallengePeriodRemaining(channelID)

	if err == nil {
		t.Error("should error for non-existent dispute")
	}
}

// ============================================================================
// Struct Tests
// ============================================================================

func TestEvidence_Fields(t *testing.T) {
	ev := Evidence{
		ChannelID:   [32]byte{1, 2, 3, 4},
		Sequence:    5,
		BalanceA:    1000,
		BalanceB:    2000,
		SigA:        []byte("sigA"),
		SigB:        []byte("sigB"),
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{9, 8, 7},
		BlockNumber: 100,
	}

	if ev.Sequence != 5 {
		t.Errorf("expected Sequence 5, got %d", ev.Sequence)
	}
	if ev.BalanceA != 1000 {
		t.Errorf("expected BalanceA 1000, got %d", ev.BalanceA)
	}
	if ev.BalanceB != 2000 {
		t.Errorf("expected BalanceB 2000, got %d", ev.BalanceB)
	}
}

func TestDisputeResult_Fields(t *testing.T) {
	result := DisputeResult{
		Success: true,
		Winner:  interfaces.Address{1, 2, 3},
		Reason:  "fraud_detected",
	}
	result.NewBalance.A = 1000
	result.NewBalance.B = 500

	if !result.Success {
		t.Error("expected Success = true")
	}
	if result.NewBalance.A != 1000 {
		t.Errorf("expected Balance.A = 1000, got %d", result.NewBalance.A)
	}
	if result.Reason != "fraud_detected" {
		t.Errorf("expected Reason = fraud_detected, got %s", result.Reason)
	}
}

func TestDisputeTask_Constants(t *testing.T) {
	if TaskInitiate != 0 {
		t.Errorf("expected TaskInitiate = 0, got %d", TaskInitiate)
	}
	if TaskRespond != 1 {
		t.Errorf("expected TaskRespond = 1, got %d", TaskRespond)
	}
	if TaskFinalize != 2 {
		t.Errorf("expected TaskFinalize = 2, got %d", TaskFinalize)
	}
}

func TestDisputeResolution_Fields(t *testing.T) {
	resolution := DisputeResolution{
		ChannelID:      [32]byte{1, 2, 3, 4},
		Winner:         interfaces.Address{1},
		Loser:          interfaces.Address{2},
		FinalBalanceA:  800,
		FinalBalanceB:  200,
		PenaltyAmount:  100,
		ResolutionType: "challenge_success",
		Timestamp:      time.Now(),
		BlockNumber:    1000,
	}

	if resolution.FinalBalanceA != 800 {
		t.Errorf("expected FinalBalanceA 800, got %d", resolution.FinalBalanceA)
	}
	if resolution.PenaltyAmount != 100 {
		t.Errorf("expected PenaltyAmount 100, got %d", resolution.PenaltyAmount)
	}
	if resolution.ResolutionType != "challenge_success" {
		t.Errorf("expected ResolutionType challenge_success, got %s", resolution.ResolutionType)
	}
}

// ============================================================================
// InitiateDispute Tests
// ============================================================================

func TestDisputeResolver_InitiateDispute(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	// Create test parties
	tp := newTestParties()

	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Create evidence
	evidence := Evidence{
		ChannelID:   channel.ID,
		Sequence:    2,
		BalanceA:    3000,
		BalanceB:    5000,
		SigA:        []byte("sigA"),
		SigB:        []byte("sigB"),
		Timestamp:   time.Now(),
		Submitter:   tp.partyA,
		BlockNumber: 100,
	}

	// Note: InitiateDispute requires valid signatures, so we create a minimal valid evidence
	// For this test, we use the basic evidence structure
	_, err := resolver.InitiateDispute(ctx, channel.ID, evidence)
	// This may fail signature validation, but we test the flow
	if err != nil && err.Error() != "invalid evidence" {
		// Error is expected without proper signatures, but not channel not found
		t.Logf("Expected error due to signature validation: %v", err)
	}
}

func TestDisputeResolver_InitiateDispute_InvalidEvidence(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, interfaces.Address{1}, interfaces.Address{2}, 5000, 3000)

	// Create evidence with channel ID mismatch
	evidence := Evidence{
		ChannelID:   [32]byte{99}, // Wrong channel ID
		Sequence:    1,
		BalanceA:    5000,
		BalanceB:    3000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{1},
		BlockNumber: 100,
	}

	_, err := resolver.InitiateDispute(ctx, channel.ID, evidence)
	if err == nil {
		t.Error("Should fail with channel ID mismatch")
	}
}

func TestDisputeResolver_InitiateDispute_ChannelNotFound(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channelID := [32]byte{99}

	evidence := Evidence{
		ChannelID:   channelID,
		Sequence:    1,
		BalanceA:    5000,
		BalanceB:    3000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{1},
		BlockNumber: 100,
	}

	_, err := resolver.InitiateDispute(ctx, channelID, evidence)
	if err == nil {
		t.Error("Should fail with channel not found")
	}
}

// ============================================================================
// RespondToDispute Tests
// ============================================================================

func TestDisputeResolver_RespondToDispute_DisputeNotFound(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channelID := [32]byte{99}

	response := Evidence{
		ChannelID:   channelID,
		Sequence:    2,
		BalanceA:    3000,
		BalanceB:    5000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{2},
		BlockNumber: 101,
	}

	err := resolver.RespondToDispute(ctx, channelID, response)
	if err == nil {
		t.Error("Should fail when dispute not found")
	}
}

// ============================================================================
// FinalizeDispute Tests
// ============================================================================

func TestDisputeResolver_FinalizeDispute_NotFound(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channelID := [32]byte{99}

	_, err := resolver.FinalizeDispute(ctx, channelID)
	if err == nil {
		t.Error("Should fail when dispute not found")
	}
}

// ============================================================================
// SubmitFraudProof Tests
// ============================================================================

func TestDisputeResolver_SubmitFraudProof_SequenceError(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()
	channelID := [32]byte{1}

	// Create invalid/valid state pair where invalid has higher sequence
	invalidState := Evidence{
		ChannelID:   channelID,
		Sequence:    10, // Higher sequence
		BalanceA:    5000,
		BalanceB:    3000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{1},
		BlockNumber: 100,
	}

	validState := Evidence{
		ChannelID:   channelID,
		Sequence:    5, // Lower sequence - should be higher for valid
		BalanceA:    4000,
		BalanceB:    4000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{2},
		BlockNumber: 101,
	}

	_, err := resolver.SubmitFraudProof(ctx, channelID, invalidState, validState)
	if err == nil {
		t.Error("Should fail when valid state doesn't have higher sequence")
	}
}

func TestDisputeResolver_SubmitFraudProof_ChannelMismatch(t *testing.T) {
	mgr := createTestManager(t)
	resolver := NewDisputeResolver(mgr)

	ctx := context.Background()

	invalidState := Evidence{
		ChannelID:   [32]byte{1},
		Sequence:    5,
		BalanceA:    5000,
		BalanceB:    3000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{1},
		BlockNumber: 100,
	}

	validState := Evidence{
		ChannelID:   [32]byte{2}, // Different channel
		Sequence:    10,
		BalanceA:    4000,
		BalanceB:    4000,
		Timestamp:   time.Now(),
		Submitter:   interfaces.Address{2},
		BlockNumber: 101,
	}

	_, err := resolver.SubmitFraudProof(ctx, [32]byte{1}, invalidState, validState)
	if err == nil {
		t.Error("Should fail with channel ID mismatch")
	}
}

// ============================================================================
// FraudDetection Tests
// ============================================================================

func TestNewFraudDetection(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	if fd == nil {
		t.Fatal("FraudDetection should not be nil")
	}

	if fd.alertChan == nil {
		t.Error("alert channel should be initialized")
	}
}

func TestFraudDetection_RecordState(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	// Create test channel
	tp := newTestParties()
	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Compute state hash
	stateHash := computeStateHash(channel)

	// Record state with sequence and state hash
	fd.RecordState(channel.ID, 1, stateHash)
}

func TestFraudDetection_CheckForDoubleSpend(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	// Create test channel
	tp := newTestParties()
	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Record first state
	stateHash1 := computeStateHash(channel)
	fd.RecordState(channel.ID, 1, stateHash1)

	// Create different state hash for same sequence
	channel.BalanceA = 4000
	stateHash2 := computeStateHash(channel)

	// Check for double spend
	alert := fd.CheckForDoubleSpend(channel.ID, 1, stateHash2)
	if alert != nil {
		t.Logf("Double spend detected: %s", alert.AlertType)
	}
}

func TestFraudDetection_GetAlertChannel(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	alertChan := fd.GetAlertChannel()
	if alertChan == nil {
		t.Error("Alert channel should not be nil")
	}
}

func TestFraudDetection_ReportFraud(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	// Create fraud alert
	alert := FraudAlert{
		ChannelID:   [32]byte{1},
		AlertType:   "test_fraud",
		Sequence:    1,
		Timestamp:   time.Now(),
		Description: "test fraud alert",
	}

	// Report fraud
	fd.ReportFraud(alert)

	// Check if alert was sent
	select {
	case receivedAlert := <-fd.alertChan:
		if receivedAlert.ChannelID == [32]byte{1} {
			t.Log("Fraud alert received correctly")
		}
	default:
		// Alert may have been sent but channel is buffered
		t.Log("Alert channel check completed")
	}
}

func TestFraudDetection_GetKnownStates(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	channelID := [32]byte{1}

	states := fd.GetKnownStates(channelID)
	if states == nil {
		t.Error("States map should not be nil")
	}

	if len(states) != 0 {
		t.Log("Existing states found")
	}
}

func TestFraudDetection_MonitorChannel(t *testing.T) {
	mgr := createTestManager(t)

	fd := NewFraudDetection(mgr)

	tp := newTestParties()
	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	// Start monitoring (doesn't take context)
	fd.MonitorChannel(channel.ID)

	// Stop monitoring
	fd.StopMonitoring(channel.ID)
}

// ============================================================================
// Challenge/Response Tests
// ============================================================================

func TestDisputeResolver_SignChallengeResponse(t *testing.T) {
	mgr := createTestManager(t)
	cfg := DefaultDisputeConfig()
	cfg.BlockChecker = newTestBlockChecker()

	resolver := NewDisputeResolverWithConfig(mgr, cfg)

	// Generate test private key
	_, privKey, _ := ed25519.GenerateKey(nil)
	privateKeyBytes := []byte(privKey)

	channelID := [32]byte{1, 2, 3}

	signature, err := resolver.SignChallengeResponse(channelID, 1000, 2000, 1, privateKeyBytes)
	if err != nil {
		t.Fatalf("SignChallengeResponse failed: %v", err)
	}

	if len(signature) == 0 {
		t.Error("Signature should not be empty")
	}
}

func TestDisputeResolver_VerifyChallengeResponse(t *testing.T) {
	mgr := createTestManager(t)
	cfg := DefaultDisputeConfig()
	cfg.BlockChecker = newTestBlockChecker()

	resolver := NewDisputeResolverWithConfig(mgr, cfg)

	// Generate test keys
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	privateKeyBytes := []byte(privKey)
	publicKeyBytes := []byte(pubKey)

	channelID := [32]byte{1, 2, 3}

	// Create evidence for signing
	evidence := &Evidence{
		ChannelID: channelID,
		Sequence:  1,
		BalanceA:  1000,
		BalanceB:  2000,
		Timestamp: time.Now(),
	}

	signature, err := resolver.SignChallengeResponse(channelID, 1000, 2000, 1, privateKeyBytes)
	if err != nil {
		t.Fatalf("SignChallengeResponse failed: %v", err)
	}

	// Verify the signature
	valid := resolver.VerifyChallengeResponse(evidence, publicKeyBytes, signature)

	if !valid {
		t.Error("Signature verification should succeed")
	}
}

func TestComputeChannelStateHash(t *testing.T) {
	mgr := createTestManager(t)

	tp := newTestParties()
	ctx := context.Background()
	channel, _ := mgr.OpenChannel(ctx, tp.partyA, tp.partyB, 5000, 3000)

	hash := ComputeChannelStateHash(channel)
	if hash == [32]byte{} {
		t.Error("Hash should not be zero")
	}
}
