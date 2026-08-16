// Package channel implements Lightning-style state channels for AIB 2.0.
// Dispute resolution for fraudulent close attempts and challenge period management.
package channel

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// DisputeResolver handles dispute resolution for state channels.
type DisputeResolver struct {
	manager                *Manager
	evidenceStore          map[[32]byte][]Evidence
	challengeQueue         chan DisputeTask
	mu                     sync.RWMutex
	blockChecker           BlockChecker
	penaltyRecipient       interfaces.Address // Address to receive fraud penalties (treasury/burn)

	// Configuration
	challengePeriod        time.Duration
	maxEvidenceAge        time.Duration
	evidenceExpiry        time.Duration
	fraudPenaltyMultiplier float64
	minChallengePeriod     time.Duration
	maxChallengePeriod     time.Duration
}

// DisputeResolution represents the result of dispute resolution.
type DisputeResolution struct {
	ChannelID      [32]byte
	Winner         interfaces.Address
	Loser          interfaces.Address
	FinalBalanceA  uint64
	FinalBalanceB  uint64
	PenaltyAmount  uint64
	ResolutionType string // "challenge_success", "counter_evidence", "timeout", "fraud_proof"
	Timestamp      time.Time
	BlockNumber    uint64
}

// Evidence represents evidence submitted during a dispute.
type Evidence struct {
	ChannelID    [32]byte
	Sequence     uint64
	BalanceA     uint64
	BalanceB     uint64
	SigA         []byte
	SigB         []byte
	Timestamp    time.Time
	Submitter    interfaces.Address
	BlockNumber  uint64
}

// DisputeTask represents a dispute resolution task.
type DisputeTask struct {
	ChannelID    [32]byte
	TaskType     int // 0=initiate, 0=respond, 2=finalize
	Evidence     *Evidence
	Result       chan DisputeResult
}

// DisputeResult represents the result of a dispute resolution.
type DisputeResult struct {
	Success    bool
	Winner     interfaces.Address
	NewBalance struct {
		A uint64
		B uint64
	}
	Reason string
}

// Dispute constants
const (
	TaskInitiate = iota
	TaskRespond
	TaskFinalize
)

// BlockChecker provides blockchain state verification for disputes.
type BlockChecker interface {
	// GetCurrentBlock returns the current block number.
	GetCurrentBlock(ctx context.Context) (uint64, error)
	// GetBlockTimestamp returns the timestamp of a block.
	GetBlockTimestamp(ctx context.Context, blockNum uint64) (time.Time, error)
	// VerifyTxInBlock verifies if a transaction exists in a block.
	VerifyTxInBlock(ctx context.Context, txHash [32]byte, blockNum uint64) (bool, error)
}

// DisputeConfig holds configuration for the dispute resolver.
type DisputeConfig struct {
	ChallengePeriod        time.Duration
	MinChallengePeriod     time.Duration
	MaxChallengePeriod     time.Duration
	MaxEvidenceAge        time.Duration
	EvidenceExpiry        time.Duration
	FraudPenaltyMultiplier float64
	PenaltyRecipient      interfaces.Address
	BlockChecker          BlockChecker
}

// DefaultDisputeConfig returns the default dispute configuration.
func DefaultDisputeConfig() *DisputeConfig {
	return &DisputeConfig{
		ChallengePeriod:        24 * time.Hour,
		MinChallengePeriod:     1 * time.Hour,
		MaxChallengePeriod:     7 * 24 * time.Hour,
		MaxEvidenceAge:         7 * 24 * time.Hour,
		EvidenceExpiry:         30 * 24 * time.Hour,
		FraudPenaltyMultiplier: 1.0, // 100% penalty (all funds to honest party)
		PenaltyRecipient:       interfaces.Address{}, // Burn to zero address by default
	}
}

// NewDisputeResolver creates a new dispute resolver.
func NewDisputeResolver(manager *Manager) *DisputeResolver {
	return NewDisputeResolverWithConfig(manager, DefaultDisputeConfig())
}

// NewDisputeResolverWithConfig creates a new dispute resolver with custom config.
func NewDisputeResolverWithConfig(manager *Manager, cfg *DisputeConfig) *DisputeResolver {
	if cfg == nil {
		cfg = DefaultDisputeConfig()
	}

	return &DisputeResolver{
		manager:                manager,
		evidenceStore:          make(map[[32]byte][]Evidence),
		challengeQueue:         make(chan DisputeTask, 100),
		challengePeriod:        cfg.ChallengePeriod,
		minChallengePeriod:     cfg.MinChallengePeriod,
		maxChallengePeriod:     cfg.MaxChallengePeriod,
		maxEvidenceAge:         cfg.MaxEvidenceAge,
		evidenceExpiry:         cfg.EvidenceExpiry,
		fraudPenaltyMultiplier: cfg.FraudPenaltyMultiplier,
		penaltyRecipient:       cfg.PenaltyRecipient,
		blockChecker:           cfg.BlockChecker,
	}
}

// InitiateDispute initiates a dispute for a channel.
func (dr *DisputeResolver) InitiateDispute(
	ctx context.Context,
	channelID [32]byte,
	evidence Evidence,
) (*DisputeRecord, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	// Verify evidence is valid
	if err := dr.validateEvidence(channelID, &evidence); err != nil {
		return nil, fmt.Errorf("invalid evidence: %w", err)
	}

	// Check if dispute already exists
	existingDispute, err := dr.manager.GetDispute(channelID)
	if err == nil && !existingDispute.Resolved {
		return existingDispute, errors.New("dispute already in progress")
	}

	// Convert Evidence to SignedState for the ChannelManager
	signedState := interfaces.SignedState{
		ChannelID: channelID,
		Sequence:  evidence.Sequence,
		BalanceA:  evidence.BalanceA,
		BalanceB:  evidence.BalanceB,
		SigA:      evidence.SigA,
		SigB:      evidence.SigB,
		Timestamp: evidence.Timestamp,
	}

	channel, err := dr.manager.GetChannelState(channelID)
	if err != nil {
		return nil, err
	}

	// Call the channel manager's Dispute method
	if err := dr.manager.Dispute(ctx, channel, signedState); err != nil {
		return nil, fmt.Errorf("failed to initiate dispute: %w", err)
	}

	// Store the evidence
	dr.evidenceStore[channelID] = append(dr.evidenceStore[channelID], evidence)

	// Get the updated dispute
	dispute, err := dr.manager.GetDispute(channelID)
	if err != nil {
		return nil, err
	}

	return dispute, nil
}

// RespondToDispute responds to a dispute with counter-evidence.
func (dr *DisputeResolver) RespondToDispute(
	ctx context.Context,
	channelID [32]byte,
	response Evidence,
) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	// Verify response evidence is valid
	if err := dr.validateEvidence(channelID, &response); err != nil {
		return fmt.Errorf("invalid response evidence: %w", err)
	}

	// Get the dispute
	dispute, err := dr.manager.GetDispute(channelID)
	if err != nil {
		return fmt.Errorf("dispute not found: %w", err)
	}

	// Check if dispute is still active
	if dispute.Resolved {
		return errors.New("dispute already resolved")
	}

	// Check if challenge period has passed
	if time.Now().After(dispute.ChallengeEnd) {
		return ErrDisputeTimeout
	}

	// Validate that the response has a higher sequence than the challenged state
	if response.Sequence <= dispute.ChallengedState.Sequence {
		return fmt.Errorf("response sequence %d must be greater than challenged sequence %d",
			response.Sequence, dispute.ChallengedState.Sequence)
	}

	// Store the response evidence
	dr.evidenceStore[channelID] = append(dr.evidenceStore[channelID], response)

	// The higher sequence evidence will be used for final resolution
	return nil
}

// FinalizeDispute finalizes a dispute after the challenge period.
func (dr *DisputeResolver) FinalizeDispute(ctx context.Context, channelID [32]byte) (*DisputeResult, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	// Get the dispute
	dispute, err := dr.manager.GetDispute(channelID)
	if err != nil {
		return nil, fmt.Errorf("dispute not found: %w", err)
	}

	// Check if already resolved
	if dispute.Resolved {
		return &DisputeResult{
			Success: true,
			Winner:  dispute.Winner,
			Reason:  dispute.Resolution,
		}, nil
	}

	// Check if challenge period has passed
	if time.Now().Before(dispute.ChallengeEnd) {
		return nil, fmt.Errorf("challenge period not yet ended: %v remaining", dispute.ChallengeEnd.Sub(time.Now()))
	}

	// Get all evidence
	evidenceList := dr.evidenceStore[channelID]

	// Find the evidence with the highest sequence
	var highestEvidence *Evidence
	for i := range evidenceList {
		ev := &evidenceList[i]
		if highestEvidence == nil || ev.Sequence > highestEvidence.Sequence {
			highestEvidence = ev
		}
	}

	// If we have higher sequence evidence than the challenged state, use it
	if highestEvidence != nil && highestEvidence.Sequence > dispute.ChallengedState.Sequence {
		// Update channel with the higher sequence state
		channel, err := dr.manager.GetChannelState(channelID)
		if err != nil {
			return nil, err
		}

		// Determine honest party based on who submitted the higher sequence
		channel.BalanceA = highestEvidence.BalanceA
		channel.BalanceB = highestEvidence.BalanceB
		channel.Sequence = highestEvidence.Sequence
		channel.StateHash = computeStateHash(channel)

		// Set the winner as the submitter of the highest sequence evidence
		winner := highestEvidence.Submitter

		// Resolve the dispute
		if err := dr.manager.ResolveDispute(channelID, winner); err != nil {
			return nil, fmt.Errorf("failed to resolve dispute: %w", err)
		}

		return &DisputeResult{
			Success: true,
			Winner:  winner,
			NewBalance: struct {
				A uint64
				B uint64
			}{
				A: highestEvidence.BalanceA,
				B: highestEvidence.BalanceB,
			},
			Reason: "Higher sequence evidence accepted",
		}, nil
	}

	// No higher sequence - the challenged state stands, check if we can penalize
	// In this case, the challenger wins by default (they had the more recent state)
	winner := dispute.Challenger

	if err := dr.manager.ResolveDispute(channelID, winner); err != nil {
		return nil, fmt.Errorf("failed to resolve dispute: %w", err)
	}

	return &DisputeResult{
		Success: true,
		Winner:  winner,
		Reason:  "Challenge successful - no counter-evidence",
	}, nil
}

// validateEvidence validates evidence for a channel.
func (dr *DisputeResolver) validateEvidence(channelID [32]byte, evidence *Evidence) error {
	if evidence == nil {
		return errors.New("evidence is nil")
	}

	if evidence.ChannelID != channelID {
		return errors.New("channel ID mismatch")
	}

	if evidence.Sequence == 0 {
		return errors.New("sequence must be greater than 0")
	}

	// Verify at least one signature
	if len(evidence.SigA) == 0 && len(evidence.SigB) == 0 {
		return errors.New("at least one signature required")
	}

	// Get channel to verify signatures
	channel, err := dr.manager.GetChannelState(channelID)
	if err != nil {
		return fmt.Errorf("channel not found: %w", err)
	}

	// Verify signatures if present
	if len(evidence.SigA) > 0 {
		stateData := serializeEvidence(evidence)
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, evidence.SigA) {
			return errors.New("invalid signature A")
		}
	}

	if len(evidence.SigB) > 0 {
		stateData := serializeEvidence(evidence)
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, evidence.SigB) {
			return errors.New("invalid signature B")
		}
	}

	return nil
}

// serializeEvidence serializes evidence for signing/verification.
func serializeEvidence(e *Evidence) []byte {
	buf := make([]byte, 0, 200)
	buf = append(buf, e.ChannelID[:]...)
	buf = binary.BigEndian.AppendUint64(buf, e.Sequence)
	buf = binary.BigEndian.AppendUint64(buf, e.BalanceA)
	buf = binary.BigEndian.AppendUint64(buf, e.BalanceB)
	buf = append(buf, byte(e.Timestamp.Unix()))
	return buf
}

// SubmitFraudProof submits a fraud proof for a channel close attempt.
func (dr *DisputeResolver) SubmitFraudProof(
	ctx context.Context,
	channelID [32]byte,
	invalidState Evidence,
	validState Evidence,
) (*DisputeResult, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	// Verify the invalid state was already on-chain (the fraudulent close)
	// and the valid state has a higher sequence
	if invalidState.Sequence >= validState.Sequence {
		return nil, errors.New("valid state must have higher sequence than invalid state")
	}

	// Verify both states are for this channel
	if invalidState.ChannelID != channelID || validState.ChannelID != channelID {
		return nil, errors.New("channel ID mismatch")
	}

	// Verify signatures on valid state
	if err := dr.validateEvidence(channelID, &validState); err != nil {
		return nil, fmt.Errorf("invalid valid state: %w", err)
	}

	// Get channel
	channel, err := dr.manager.GetChannelState(channelID)
	if err != nil {
		return nil, err
	}

	// Determine who is the honest party (the one who signed the valid state)
	var honestParty interfaces.Address
	if len(validState.SigA) > 0 {
		honestParty = channel.PartyA
	} else if len(validState.SigB) > 0 {
		honestParty = channel.PartyB
	}

	// Apply penalty to the fraudulent party
	if err := dr.manager.ResolveDispute(channelID, honestParty); err != nil {
		return nil, fmt.Errorf("failed to resolve dispute: %w", err)
	}

	return &DisputeResult{
		Success: true,
		Winner:  honestParty,
		NewBalance: struct {
			A uint64
			B uint64
		}{
			A: validState.BalanceA,
			B: validState.BalanceB,
		},
		Reason: "Fraud proof accepted - penalty applied to dishonest party",
	}, nil
}

// CheckDisputeStatus checks the current status of a dispute.
func (dr *DisputeResolver) CheckDisputeStatus(channelID [32]byte) (*DisputeStatus, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	dispute, err := dr.manager.GetDispute(channelID)
	if err != nil {
		return nil, err
	}

	evidenceList := dr.evidenceStore[channelID]

	status := &DisputeStatus{
		ChannelID:       channelID,
		Active:          !dispute.Resolved,
		Challenger:      dispute.Challenger,
		ChallengedSeq:   dispute.ChallengedState.Sequence,
		ChallengeStart:  dispute.ChallengeStart,
		ChallengeEnd:    dispute.ChallengeEnd,
		TimeRemaining:   dispute.ChallengeEnd.Sub(time.Now()),
		EvidenceCount:   len(evidenceList),
		Resolved:        dispute.Resolved,
		Winner:          dispute.Winner,
	}

	if !dispute.Resolved && status.TimeRemaining < 0 {
		status.TimeRemaining = 0
	}

	return status, nil
}

// DisputeStatus represents the current status of a dispute.
type DisputeStatus struct {
	ChannelID       [32]byte
	Active          bool
	Challenger      interfaces.Address
	ChallengedSeq   uint64
	ChallengeStart  time.Time
	ChallengeEnd    time.Time
	TimeRemaining   time.Duration
	EvidenceCount   int
	Resolved        bool
	Winner          interfaces.Address
}

// GetEvidence returns all evidence for a channel dispute.
func (dr *DisputeResolver) GetEvidence(channelID [32]byte) ([]Evidence, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	evidence, exists := dr.evidenceStore[channelID]
	if !exists {
		return nil, errors.New("no evidence found")
	}

	return evidence, nil
}

// CleanOldEvidence removes evidence older than the configured expiry.
func (dr *DisputeResolver) CleanOldEvidence() int {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	cutoff := time.Now().Add(-dr.evidenceExpiry)
	cleaned := 0

	for channelID, evidenceList := range dr.evidenceStore {
		var filtered []Evidence
		for _, e := range evidenceList {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			} else {
				cleaned++
			}
		}
		dr.evidenceStore[channelID] = filtered
	}

	return cleaned
}

// GetDispute returns the dispute record for a channel.
func (dr *DisputeResolver) GetDispute(channelID [32]byte) (*DisputeRecord, error) {
	return dr.manager.GetDispute(channelID)
}

// ChallengePeriodRemaining returns the remaining challenge period for a channel.
func (dr *DisputeResolver) ChallengePeriodRemaining(channelID [32]byte) (time.Duration, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	dispute, err := dr.manager.GetDispute(channelID)
	if err != nil {
		return 0, err
	}

	if dispute.Resolved {
		return 0, errors.New("dispute already resolved")
	}

	remaining := dispute.ChallengeEnd.Sub(time.Now())
	if remaining < 0 {
		return 0, nil
	}

	return remaining, nil
}

// IsChannelInDispute checks if a channel is currently in dispute.
func (dr *DisputeResolver) IsChannelInDispute(channelID [32]byte) (bool, error) {
	status, err := dr.CheckDisputeStatus(channelID)
	if err != nil {
		return false, err
	}

	return status.Active, nil
}

// CalculatePenalty calculates the penalty for fraudulent behavior.
func (dr *DisputeResolver) CalculatePenalty(fraudAmount uint64) uint64 {
	return uint64(float64(fraudAmount) * dr.fraudPenaltyMultiplier)
}

// SetPenaltyMultiplier sets the fraud penalty multiplier.
func (dr *DisputeResolver) SetPenaltyMultiplier(multiplier float64) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if multiplier < 1.0 {
		dr.fraudPenaltyMultiplier = 1.0
	} else {
		dr.fraudPenaltyMultiplier = multiplier
	}
}

// FraudDetection monitors channels for potential fraud.
type FraudDetection struct {
	manager      *Manager
	knownStates  map[[32]byte]map[uint64][32]byte // channelID -> sequence -> stateHash
	alertChan    chan FraudAlert
	mu           sync.RWMutex
}

// FraudAlert represents a fraud alert.
type FraudAlert struct {
	ChannelID     [32]byte
	AlertType     string
	Sequence      uint64
	ReportedBy    interfaces.Address
	Timestamp     time.Time
	Description   string
}

// NewFraudDetection creates a new fraud detection system.
func NewFraudDetection(manager *Manager) *FraudDetection {
	return &FraudDetection{
		manager:     manager,
		knownStates: make(map[[32]byte]map[uint64][32]byte),
		alertChan:   make(chan FraudAlert, 100),
	}
}

// RecordState records a known state for a channel.
func (fd *FraudDetection) RecordState(channelID [32]byte, sequence uint64, stateHash [32]byte) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.knownStates[channelID] == nil {
		fd.knownStates[channelID] = make(map[uint64][32]byte)
	}

	fd.knownStates[channelID][sequence] = stateHash
}

// CheckForDoubleSpend checks if there's a conflicting state for the same sequence.
func (fd *FraudDetection) CheckForDoubleSpend(channelID [32]byte, sequence uint64, stateHash [32]byte) *FraudAlert {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	existingHash, exists := fd.knownStates[channelID][sequence]
	if exists && existingHash != stateHash {
		return &FraudAlert{
			ChannelID:   channelID,
			AlertType:   "double_spend",
			Sequence:    sequence,
			Timestamp:   time.Now(),
			Description: fmt.Sprintf("double spend detected: sequence %d has conflicting state hashes", sequence),
		}
	}

	return nil
}

// MonitorChannel starts monitoring a channel for fraud.
func (fd *FraudDetection) MonitorChannel(channelID [32]byte) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.knownStates[channelID] == nil {
		fd.knownStates[channelID] = make(map[uint64][32]byte)
	}
}

// StopMonitoring stops monitoring a channel.
func (fd *FraudDetection) StopMonitoring(channelID [32]byte) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	delete(fd.knownStates, channelID)
}

// GetAlertChannel returns the channel for fraud alerts.
func (fd *FraudDetection) GetAlertChannel() <-chan FraudAlert {
	return fd.alertChan
}

// ReportFraud reports potential fraud.
func (fd *FraudDetection) ReportFraud(alert FraudAlert) {
	fd.alertChan <- alert
}

// GetKnownStates returns all known states for a channel.
func (fd *FraudDetection) GetKnownStates(channelID [32]byte) map[uint64][32]byte {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	result := make(map[uint64][32]byte)
	for seq, hash := range fd.knownStates[channelID] {
		result[seq] = hash
	}
	return result
}

// SignChallengeResponse signs a challenge response for dispute resolution.
func (dr *DisputeResolver) SignChallengeResponse(
	channelID [32]byte,
	balanceA, balanceB uint64,
	sequence uint64,
	privateKey []byte,
) ([]byte, error) {
	evidence := &Evidence{
		ChannelID: channelID,
		Sequence:  sequence,
		BalanceA:  balanceA,
		BalanceB:  balanceB,
		Timestamp: time.Now(),
	}

	data := serializeEvidence(evidence)
	return ed25519.Sign(privateKey, data), nil
}

// VerifyChallengeResponse verifies a challenge response.
func (dr *DisputeResolver) VerifyChallengeResponse(
	evidence *Evidence,
	publicKey []byte,
	signature []byte,
) bool {
	data := serializeEvidence(evidence)
	return ed25519.Verify(publicKey, data, signature)
}

// Helper function to compute state hash (exported for use in dispute)
func ComputeChannelStateHash(ch *interfaces.Channel) [32]byte {
	return computeStateHash(ch)
}
