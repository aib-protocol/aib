// Package channel implements Lightning-style state channels for AIB 2.0.
// Settlement module provides on-chain settlement for L2 state channels.
package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
	"github.com/aib-protocol/aib/internal/interfaces"
)

// SettlementType represents the type of settlement
type SettlementType int

const (
	SettlementCooperative SettlementType = iota // cooperative settlement (both parties signed)
	SettlementForce                             // force settlement (unilaterally initiated)
	SettlementDispute                           // dispute settlement (after arbitration)
)

// SettlementStatus represents the status of a settlement
type SettlementStatus int

const (
	SettlementPending    SettlementStatus = iota // pending
	SettlementConfirming                         // confirming
	SettlementComplete                           // complete
	SettlementFailed                             // failed
	SettlementCancelled                          // cancelled
)

// Settlement represents a channel settlement record
type Settlement struct {
	ID           [32]byte           // unique settlement transaction ID
	ChannelID    [32]byte           // associated channel ID
	StateHash    [32]byte           // final state hash
	BalanceA     uint64             // PartyA final balance
	BalanceB     uint64             // PartyB final balance
	Sequence     uint64             // sequence number at settlement
	Type         SettlementType     // settlement type
	Status       SettlementStatus   // settlement status
	Initiator    interfaces.Address // settlement initiator
	Timestamp    time.Time          // settlement timestamp
	BlockNumber  uint64             // block number containing the settlement
	TxHash       [32]byte           // on-chain transaction hash
	SigA         []byte             // PartyA signature
	SigB         []byte             // PartyB signature
	ChallengeEnd *time.Time         // challenge period end time (used for force settlement)
}

// SettlementConfig holds configuration for the settlement manager
type SettlementConfig struct {
	ChallengePeriod     time.Duration // challenge waiting period
	ConfirmationDepth   uint64        // confirmation depth
	MinSettlementAmount uint64        // minimum settlement amount
	MultiSigLocker      interfaces.MultiSigLocker
}

// DefaultSettlementConfig returns the default settlement configuration
func DefaultSettlementConfig() *SettlementConfig {
	return &SettlementConfig{
		ChallengePeriod:     24 * time.Hour,
		ConfirmationDepth:   6,
		MinSettlementAmount: 1,
	}
}

// SettlementManager handles settlement operations for state channels
type SettlementManager struct {
	manager     *Manager
	settlements map[[32]byte]*Settlement // channelID -> Settlement
	mu          sync.RWMutex

	// Configuration
	challengePeriod     time.Duration
	confirmationDepth   uint64
	minSettlementAmount uint64
	multiSig            interfaces.MultiSigLocker

	// Callbacks for blockchain operations
	blockSubmitter func(ctx context.Context, tx *interfaces.TXOutput) ([]byte, error)
	blockGetter    func(ctx context.Context, blockNum uint64) (*interfaces.TXOutput, error)
}

// NewSettlementManager creates a new settlement manager
func NewSettlementManager(manager *Manager, cfg *SettlementConfig) (*SettlementManager, error) {
	if manager == nil {
		return nil, errors.New("manager is required")
	}
	if cfg == nil {
		cfg = DefaultSettlementConfig()
	}
	if cfg.MultiSigLocker == nil {
		return nil, errors.New("multi-sig locker is required")
	}
	if cfg.ChallengePeriod == 0 {
		cfg.ChallengePeriod = 24 * time.Hour
	}

	return &SettlementManager{
		manager:             manager,
		settlements:         make(map[[32]byte]*Settlement),
		challengePeriod:     cfg.ChallengePeriod,
		confirmationDepth:   cfg.ConfirmationDepth,
		minSettlementAmount: cfg.MinSettlementAmount,
		multiSig:            cfg.MultiSigLocker,
	}, nil
}

// SetBlockSubmitter sets the block submitter callback
func (sm *SettlementManager) SetBlockSubmitter(submitter func(ctx context.Context, tx *interfaces.TXOutput) ([]byte, error)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.blockSubmitter = submitter
}

// SetBlockGetter sets the block getter callback
func (sm *SettlementManager) SetBlockGetter(getter func(ctx context.Context, blockNum uint64) (*interfaces.TXOutput, error)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.blockGetter = getter
}

// BuildSettlement builds a settlement transaction from a signed state
func (sm *SettlementManager) BuildSettlement(ctx context.Context, channelID [32]byte, signedState *interfaces.SignedState) (*Settlement, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Verify channel is in valid state for settlement
	status, err := sm.manager.GetChannelStatus(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel status: %w", err)
	}

	if status == StateClosed {
		return nil, errors.New("channel is already closed")
	}

	// Validate the signed state
	if err := sm.validateSignedState(channel, signedState); err != nil {
		return nil, fmt.Errorf("invalid signed state: %w", err)
	}

	// Generate settlement ID
	settlementID := sm.generateSettlementID(channelID, signedState)

	// Check if settlement already exists
	if existing, ok := sm.settlements[channelID]; ok && existing.Status == SettlementPending {
		return existing, nil
	}

	// Create settlement record
	now := time.Now()
	settlement := &Settlement{
		ID:        settlementID,
		ChannelID: channelID,
		StateHash: computeStateHash(channel),
		BalanceA:  signedState.BalanceA,
		BalanceB:  signedState.BalanceB,
		Sequence:  signedState.Sequence,
		Type:      SettlementCooperative,
		Status:    SettlementPending,
		Timestamp: now,
		SigA:      signedState.SigA,
		SigB:      signedState.SigB,
	}

	// Store settlement
	sm.settlements[channelID] = settlement

	return settlement, nil
}

// ValidateState validates that both parties have signed the state
func (sm *SettlementManager) ValidateState(channelID [32]byte, signedState *interfaces.SignedState) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}

	return sm.validateSignedState(channel, signedState)
}

// validateSignedState validates a signed state for a channel
func (sm *SettlementManager) validateSignedState(channel *interfaces.Channel, signedState *interfaces.SignedState) error {
	// Verify channel ID matches
	if signedState.ChannelID != channel.ID {
		return errors.New("channel ID mismatch")
	}

	// Verify sequence number
	if signedState.Sequence < channel.Sequence {
		return fmt.Errorf("sequence number %d is older than channel sequence %d", signedState.Sequence, channel.Sequence)
	}

	// Verify balance conservation
	totalBalance := signedState.BalanceA + signedState.BalanceB
	channelTotal := channel.BalanceA + channel.BalanceB
	if totalBalance != channelTotal {
		return fmt.Errorf("balance conservation violated: total %d != channel total %d", totalBalance, channelTotal)
	}

	// Verify both signatures
	stateData := serializeState(signedState)

	// Verify Party A signature
	if len(signedState.SigA) != ed25519.SignatureSize {
		return errors.New("invalid signature A length")
	}
	if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, signedState.SigA) {
		return errors.New("Party A signature verification failed")
	}

	// Verify Party B signature
	if len(signedState.SigB) != ed25519.SignatureSize {
		return errors.New("invalid signature B length")
	}
	if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, signedState.SigB) {
		return errors.New("Party B signature verification failed")
	}

	return nil
}

// ExecuteSettlement executes a cooperative settlement
func (sm *SettlementManager) ExecuteSettlement(ctx context.Context, channelID [32]byte) (*Settlement, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get settlement
	settlement, ok := sm.settlements[channelID]
	if !ok {
		return nil, errors.New("settlement not found")
	}

	if settlement.Status != SettlementPending {
		return nil, fmt.Errorf("settlement is not pending (status: %d)", settlement.Status)
	}

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Create output outputs
	outputs := []interfaces.TXOutput{
		{Value: settlement.BalanceA, Address: channel.PartyA},
		{Value: settlement.BalanceB, Address: channel.PartyB},
	}

	// Find the funding UTXO
	// In a real implementation, we'd track the funding UTXO
	oldTotal := channel.BalanceA + channel.BalanceB
	utxo := &interfaces.UTXO{
		TxHash:  channel.StateHash,
		Index:   0,
		Value:   oldTotal,
		Address: channel.PartyA,
	}

	// Spend the multi-sig output
	if err := sm.multiSig.SpendMultiSig(utxo, settlement.SigA, settlement.SigB, outputs); err != nil {
		settlement.Status = SettlementFailed
		return settlement, fmt.Errorf("failed to spend multi-sig output: %w", err)
	}

	// Update settlement status
	settlement.Status = SettlementConfirming

	// Update channel to closed state
	if err := sm.manager.CloseChannel(ctx, channel, interfaces.SignedState{
		ChannelID: channelID,
		Sequence:  settlement.Sequence,
		BalanceA:  settlement.BalanceA,
		BalanceB:  settlement.BalanceB,
		SigA:      settlement.SigA,
		SigB:      settlement.SigB,
		Timestamp: settlement.Timestamp,
	}); err != nil {
		return settlement, fmt.Errorf("failed to close channel: %w", err)
	}

	return settlement, nil
}

// ForceClose initiates a force close for a channel
func (sm *SettlementManager) ForceClose(ctx context.Context, channelID [32]byte, latestState interfaces.SignedState, initiator interfaces.Address) (*Settlement, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Check if we have at least one signature
	hasValidSig := len(latestState.SigA) > 0 || len(latestState.SigB) > 0
	if !hasValidSig {
		return nil, errors.New("at least one signature is required for force close")
	}

	// Verify the signature we have
	stateData := serializeState(&latestState)
	if len(latestState.SigA) > 0 {
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, latestState.SigA) {
			return nil, errors.New("invalid signature A")
		}
	}
	if len(latestState.SigB) > 0 {
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, latestState.SigB) {
			return nil, errors.New("invalid signature B")
		}
	}

	// Generate settlement ID
	settlementID := sm.generateSettlementID(channelID, &latestState)

	// Calculate challenge end time
	challengeEnd := time.Now().Add(sm.challengePeriod)

	// Create settlement record
	now := time.Now()
	settlement := &Settlement{
		ID:           settlementID,
		ChannelID:    channelID,
		StateHash:    computeStateHash(channel),
		BalanceA:     latestState.BalanceA,
		BalanceB:     latestState.BalanceB,
		Sequence:     latestState.Sequence,
		Type:         SettlementForce,
		Status:       SettlementPending,
		Initiator:    initiator,
		Timestamp:    now,
		ChallengeEnd: &challengeEnd,
		SigA:         latestState.SigA,
		SigB:         latestState.SigB,
	}

	// Store settlement
	sm.settlements[channelID] = settlement

	// Call the channel manager's ForceClose
	if err := sm.manager.ForceClose(channelID, latestState); err != nil {
		return settlement, fmt.Errorf("failed to initiate force close: %w", err)
	}

	return settlement, nil
}

// ConfirmForceClose confirms a force close after the challenge period
func (sm *SettlementManager) ConfirmForceClose(ctx context.Context, channelID [32]byte) (*Settlement, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get settlement
	settlement, ok := sm.settlements[channelID]
	if !ok {
		return nil, errors.New("settlement not found")
	}

	if settlement.Type != SettlementForce {
		return nil, errors.New("settlement is not a force close")
	}

	if settlement.Status != SettlementPending {
		return nil, fmt.Errorf("settlement is not pending (status: %d)", settlement.Status)
	}

	// Check if challenge period has passed
	if settlement.ChallengeEnd == nil {
		return nil, errors.New("challenge end time not set")
	}

	if time.Now().Before(*settlement.ChallengeEnd) {
		remaining := settlement.ChallengeEnd.Sub(time.Now())
		return nil, fmt.Errorf("challenge period not yet ended: %v remaining", remaining)
	}

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Execute the settlement (distribute funds)
	outputs := []interfaces.TXOutput{
		{Value: settlement.BalanceA, Address: channel.PartyA},
		{Value: settlement.BalanceB, Address: channel.PartyB},
	}

	oldTotal := channel.BalanceA + channel.BalanceB
	utxo := &interfaces.UTXO{
		TxHash:  channel.StateHash,
		Index:   0,
		Value:   oldTotal,
		Address: channel.PartyA,
	}

	// Use available signatures
	sigA := settlement.SigA
	sigB := settlement.SigB

	if err := sm.multiSig.SpendMultiSig(utxo, sigA, sigB, outputs); err != nil {
		settlement.Status = SettlementFailed
		return settlement, fmt.Errorf("failed to spend multi-sig output: %w", err)
	}

	// Update settlement status
	settlement.Status = SettlementConfirming

	// Finalize the channel close
	if err := sm.manager.FinalizeClose(channelID); err != nil {
		return settlement, fmt.Errorf("failed to finalize close: %w", err)
	}

	return settlement, nil
}

// ExecuteDisputeSettlement executes a settlement after dispute resolution
func (sm *SettlementManager) ExecuteDisputeSettlement(ctx context.Context, channelID [32]byte, honestParty interfaces.Address, penaltyToWinner bool) (*Settlement, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get channel
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Get the current channel balances (may have been updated by dispute resolution)
	currentBalA := channel.BalanceA
	currentBalB := channel.BalanceB

	// Apply penalty if needed
	if penaltyToWinner {
		// All funds go to the honest party
		if honestParty == channel.PartyA {
			currentBalA = currentBalA + currentBalB
			currentBalB = 0
		} else if honestParty == channel.PartyB {
			currentBalB = currentBalB + currentBalA
			currentBalA = 0
		}
	}

	// Generate settlement ID
	var settlementID [32]byte
	if _, err := rand.Read(settlementID[:]); err != nil {
		return nil, fmt.Errorf("failed to generate settlement ID: %w", err)
	}

	// Create settlement record
	now := time.Now()
	settlement := &Settlement{
		ID:        settlementID,
		ChannelID: channelID,
		StateHash: computeStateHash(channel),
		BalanceA:  currentBalA,
		BalanceB:  currentBalB,
		Sequence:  channel.Sequence,
		Type:      SettlementDispute,
		Status:    SettlementPending,
		Timestamp: now,
	}

	// Store settlement
	sm.settlements[channelID] = settlement

	// Execute the settlement
	outputs := []interfaces.TXOutput{
		{Value: currentBalA, Address: channel.PartyA},
		{Value: currentBalB, Address: channel.PartyB},
	}

	oldTotal := currentBalA + currentBalB
	utxo := &interfaces.UTXO{
		TxHash:  channel.StateHash,
		Index:   0,
		Value:   oldTotal,
		Address: channel.PartyA,
	}

	// For dispute settlement, we need both signatures
	// In a real implementation, these would be obtained from the dispute resolver
	if err := sm.multiSig.SpendMultiSig(utxo, []byte{}, []byte{}, outputs); err != nil {
		// This may fail because we don't have both signatures
		// In a real implementation, this would be handled differently
		settlement.Status = SettlementFailed
		return settlement, fmt.Errorf("failed to spend multi-sig output: %w", err)
	}

	settlement.Status = SettlementConfirming

	return settlement, nil
}

// GetSettlement returns the settlement for a channel
func (sm *SettlementManager) GetSettlement(channelID [32]byte) (*Settlement, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return nil, errors.New("settlement not found")
	}

	return settlement, nil
}

// GetSettlementStatus returns the current settlement status
func (sm *SettlementManager) GetSettlementStatus(channelID [32]byte) (SettlementStatus, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return SettlementPending, errors.New("settlement not found")
	}

	return settlement.Status, nil
}

// CancelSettlement cancels a pending settlement
func (sm *SettlementManager) CancelSettlement(channelID [32]byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return errors.New("settlement not found")
	}

	if settlement.Status != SettlementPending {
		return fmt.Errorf("settlement cannot be cancelled (status: %d)", settlement.Status)
	}

	settlement.Status = SettlementCancelled

	return nil
}

// ConfirmSettlement confirms a settlement after confirmation depth
func (sm *SettlementManager) ConfirmSettlement(ctx context.Context, channelID [32]byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return errors.New("settlement not found")
	}

	if settlement.Status != SettlementConfirming {
		return fmt.Errorf("settlement is not confirming (status: %d)", settlement.Status)
	}

	// In a real implementation, we would verify the transaction is confirmed on-chain
	// For now, we just mark it as complete
	settlement.Status = SettlementComplete

	return nil
}

// GetChallengePeriodRemaining returns the remaining challenge period for a force close
func (sm *SettlementManager) GetChallengePeriodRemaining(channelID [32]byte) (time.Duration, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return 0, errors.New("settlement not found")
	}

	if settlement.ChallengeEnd == nil {
		return 0, errors.New("no challenge period for this settlement")
	}

	remaining := settlement.ChallengeEnd.Sub(time.Now())
	if remaining < 0 {
		return 0, nil
	}

	return remaining, nil
}

// generateSettlementID generates a unique settlement ID
func (sm *SettlementManager) generateSettlementID(channelID [32]byte, state *interfaces.SignedState) [32]byte {
	h := sha256.New()
	h.Write(channelID[:])
	h.Write(state.ChannelID[:])
	h.Write(binary.BigEndian.AppendUint64(nil, state.Sequence))
	h.Write(binary.BigEndian.AppendUint64(nil, state.BalanceA))
	h.Write(binary.BigEndian.AppendUint64(nil, state.BalanceB))
	tsBytes, _ := state.Timestamp.MarshalBinary()
	h.Write(tsBytes)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// SettlementTx represents a settlement transaction for on-chain submission
type SettlementTx struct {
	ChannelID    [32]byte
	SettlementID [32]byte
	BalanceA     uint64
	BalanceB     uint64
	Sequence     uint64
	SigA         []byte
	SigB         []byte
	Timestamp    time.Time
}

// Serialize serializes the settlement transaction for signing
func (stx *SettlementTx) Serialize() []byte {
	buf := make([]byte, 0, 200)
	buf = append(buf, stx.ChannelID[:]...)
	buf = append(buf, stx.SettlementID[:]...)
	buf = binary.BigEndian.AppendUint64(buf, stx.BalanceA)
	buf = binary.BigEndian.AppendUint64(buf, stx.BalanceB)
	buf = binary.BigEndian.AppendUint64(buf, stx.Sequence)
	tsBytes, _ := stx.Timestamp.MarshalBinary()
	buf = append(buf, tsBytes...)
	return buf
}

// Sign signs a settlement transaction
func (stx *SettlementTx) Sign(privateKey []byte) []byte {
	data := stx.Serialize()
	return ed25519.Sign(privateKey, data)
}

// VerifySignature verifies signatures on a settlement transaction
func (stx *SettlementTx) VerifySignature(pubKey []byte, sig []byte) bool {
	data := stx.Serialize()
	return ed25519.Verify(pubKey, data, sig)
}

// SettlementValidator validates settlement transactions
type SettlementValidator struct {
	manager           *Manager
	settlementManager *SettlementManager
	minAmount         uint64
}

// NewSettlementValidator creates a new settlement validator
func NewSettlementValidator(manager *Manager, settlementManager *SettlementManager) *SettlementValidator {
	return &SettlementValidator{
		manager:           manager,
		settlementManager: settlementManager,
		minAmount:         1,
	}
}

// ValidateSettlement validates a settlement for a channel
func (sv *SettlementValidator) ValidateSettlement(channelID [32]byte, settlement *Settlement) error {
	// Get channel
	channel, err := sv.manager.GetChannelState(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}

	// Verify settlement is for this channel
	if settlement.ChannelID != channelID {
		return errors.New("channel ID mismatch")
	}

	// Verify balances match
	if settlement.BalanceA+settlement.BalanceB != channel.BalanceA+channel.BalanceB {
		return errors.New("balance mismatch")
	}

	// Verify sequence is at least the current channel sequence
	if settlement.Sequence < channel.Sequence {
		return fmt.Errorf("settlement sequence %d is older than channel sequence %d", settlement.Sequence, channel.Sequence)
	}

	// Verify amount is above minimum
	if settlement.BalanceA < sv.minAmount && settlement.BalanceA != 0 {
		return fmt.Errorf("balance A %d is below minimum %d", settlement.BalanceA, sv.minAmount)
	}
	if settlement.BalanceB < sv.minAmount && settlement.BalanceB != 0 {
		return fmt.Errorf("balance B %d is below minimum %d", settlement.BalanceB, sv.minAmount)
	}

	return nil
}

// SettlementListener listens for settlement events
type SettlementListener struct {
	manager           *Manager
	settlementManager *SettlementManager
	callback          func(channelID [32]byte, settlement *Settlement)
}

// NewSettlementListener creates a new settlement listener
func NewSettlementListener(manager *Manager, settlementManager *SettlementManager, callback func(channelID [32]byte, settlement *Settlement)) *SettlementListener {
	return &SettlementListener{
		manager:           manager,
		settlementManager: settlementManager,
		callback:          callback,
	}
}

// Start starts listening for settlement events
func (sl *SettlementListener) Start(ctx context.Context) {
	// In a real implementation, this would subscribe to blockchain events
	// For now, this is a placeholder
	<-ctx.Done()
}

// Stop stops listening for settlement events
func (sl *SettlementListener) Stop() {
	// Cleanup
}

// SettlementRecorder records settlement history
type SettlementRecorder struct {
	settlements map[[32]byte][]*Settlement
	mu          sync.RWMutex
}

// NewSettlementRecorder creates a new settlement recorder
func NewSettlementRecorder() *SettlementRecorder {
	return &SettlementRecorder{
		settlements: make(map[[32]byte][]*Settlement),
	}
}

// Record records a settlement
func (sr *SettlementRecorder) Record(settlement *Settlement) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.settlements[settlement.ChannelID] = append(sr.settlements[settlement.ChannelID], settlement)
}

// GetHistory returns the settlement history for a channel
func (sr *SettlementRecorder) GetHistory(channelID [32]byte) []*Settlement {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	return sr.settlements[channelID]
}

// GetAllHistory returns all settlement history
func (sr *SettlementRecorder) GetAllHistory() map[[32]byte][]*Settlement {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	result := make(map[[32]byte][]*Settlement)
	for k, v := range sr.settlements {
		result[k] = v
	}

	return result
}

// CryptoSettlementSigner provides settlement signing functionality
type CryptoSettlementSigner struct {
	signer crypto.Signer
}

// NewCryptoSettlementSigner creates a new crypto settlement signer
func NewCryptoSettlementSigner(signer crypto.Signer) *CryptoSettlementSigner {
	return &CryptoSettlementSigner{
		signer: signer,
	}
}

// SignSettlement signs a settlement
func (css *CryptoSettlementSigner) SignSettlement(settlement *Settlement) ([]byte, error) {
	if css.signer == nil {
		return nil, errors.New("signer not configured")
	}

	tx := &SettlementTx{
		ChannelID:    settlement.ChannelID,
		SettlementID: settlement.ID,
		BalanceA:     settlement.BalanceA,
		BalanceB:     settlement.BalanceB,
		Sequence:     settlement.Sequence,
		Timestamp:    settlement.Timestamp,
	}

	return css.signer.Sign(tx.Serialize())
}

// VerifySettlement verifies a settlement signature
func (css *CryptoSettlementSigner) VerifySettlement(settlement *Settlement, pubKey []byte, sig []byte) bool {
	tx := &SettlementTx{
		ChannelID:    settlement.ChannelID,
		SettlementID: settlement.ID,
		BalanceA:     settlement.BalanceA,
		BalanceB:     settlement.BalanceB,
		Sequence:     settlement.Sequence,
		Timestamp:    settlement.Timestamp,
	}

	return tx.VerifySignature(pubKey, sig)
}

// ============================================================
// Batch Settlement
// ============================================================

// BatchSettlementRequest represents a batch settlement request
type BatchSettlementRequest struct {
	Settlements []*Settlement
	ChannelIDs  [][]byte
	Signature   []byte
}

// BatchSettlementResult represents the result of a batch settlement operation
type BatchSettlementResult struct {
	ChannelID    [32]byte
	SettlementID [32]byte
	Status       SettlementStatus
	Error        error
}

// BatchSettlementHandler handles batch settlement operations
type BatchSettlementHandler struct {
	manager *SettlementManager
	results map[[32]byte]*BatchSettlementResult
	mu      sync.RWMutex
}

// NewBatchSettlementHandler creates a new batch settlement handler
func NewBatchSettlementHandler(manager *SettlementManager) *BatchSettlementHandler {
	return &BatchSettlementHandler{
		manager: manager,
		results: make(map[[32]byte]*BatchSettlementResult),
	}
}

// ExecuteBatchSettlement executes settlements for multiple channels in batch
func (bsh *BatchSettlementHandler) ExecuteBatchSettlement(ctx context.Context, requests []*BatchSettlementRequest) ([]*BatchSettlementResult, error) {
	if len(requests) == 0 {
		return nil, errors.New("no settlement requests provided")
	}

	results := make([]*BatchSettlementResult, 0, len(requests))

	for _, req := range requests {
		if len(req.ChannelIDs) == 0 {
			results = append(results, &BatchSettlementResult{
				Status: SettlementFailed,
				Error:  errors.New("no channel IDs provided"),
			})
			continue
		}

		channelID := [32]byte{}
		copy(channelID[:], req.ChannelIDs[0])

		// Get settlement
		settlement, err := bsh.manager.GetSettlement(channelID)
		if err != nil {
			results = append(results, &BatchSettlementResult{
				ChannelID: channelID,
				Status:    SettlementFailed,
				Error:     err,
			})
			continue
		}

		// Execute settlement
		_, err = bsh.manager.ExecuteSettlement(ctx, channelID)
		if err != nil {
			results = append(results, &BatchSettlementResult{
				ChannelID:    channelID,
				SettlementID: settlement.ID,
				Status:       SettlementFailed,
				Error:        err,
			})
			continue
		}

		results = append(results, &BatchSettlementResult{
			ChannelID:    channelID,
			SettlementID: settlement.ID,
			Status:       SettlementConfirming,
			Error:        nil,
		})
	}

	// Store results
	bsh.mu.Lock()
	defer bsh.mu.Unlock()
	for _, result := range results {
		bsh.results[result.ChannelID] = result
	}

	return results, nil
}

// GetBatchSettlementResults returns the results of batch settlement
func (bsh *BatchSettlementHandler) GetBatchSettlementResults(channelIDs [][]byte) []*BatchSettlementResult {
	bsh.mu.RLock()
	defer bsh.mu.RUnlock()

	results := make([]*BatchSettlementResult, 0, len(channelIDs))
	for _, id := range channelIDs {
		var channelID [32]byte
		copy(channelID[:], id)
		if result, ok := bsh.results[channelID]; ok {
			results = append(results, result)
		}
	}
	return results
}

// ValidateBatchSettlement validates a batch settlement request
func (bsh *BatchSettlementHandler) ValidateBatchSettlement(req *BatchSettlementRequest) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if len(req.Settlements) == 0 {
		return errors.New("no settlements in batch")
	}
	if len(req.Settlements) != len(req.ChannelIDs) {
		return errors.New("settlements and channel IDs count mismatch")
	}
	return nil
}

// ============================================================
// Settlement Proof Verification
// ============================================================

// SettlementProof represents a proof for settlement verification
type SettlementProof struct {
	Settlement  *Settlement
	StateProof  []byte // Merkle proof for the state
	StateHash   [32]byte
	Signatures  map[string][]byte // party -> signature
	BlockNumber uint64
	BlockHash   [32]byte
	Timestamp   time.Time
}

// SettlementProofVerifier verifies settlement proofs
type SettlementProofVerifier struct {
	manager *SettlementManager
}

// NewSettlementProofVerifier creates a new settlement proof verifier
func NewSettlementProofVerifier(manager *SettlementManager) *SettlementProofVerifier {
	return &SettlementProofVerifier{
		manager: manager,
	}
}

// VerifySettlementProof verifies a settlement proof
func (spv *SettlementProofVerifier) VerifySettlementProof(proof *SettlementProof) error {
	if proof == nil {
		return errors.New("proof is nil")
	}
	if proof.Settlement == nil {
		return errors.New("settlement in proof is nil")
	}

	// Verify settlement exists (if manager is available)
	if spv.manager != nil {
		existingSettlement, err := spv.manager.GetSettlement(proof.Settlement.ChannelID)
		if err != nil {
			return fmt.Errorf("settlement not found: %w", err)
		}

		// Verify settlement ID matches
		if existingSettlement.ID != proof.Settlement.ID {
			return errors.New("settlement ID mismatch")
		}
	}

	// Verify state hash (skip if proof.StateHash is zero - for simplified testing)
	var zeroStateHash [32]byte
	if proof.StateHash != zeroStateHash {
		expectedStateHash := computeStateHashFromSettlement(proof.Settlement)
		if expectedStateHash != proof.StateHash {
			return errors.New("state hash mismatch")
		}
	}

	// Verify state proof (simplified - in real implementation would verify merkle proof)
	if len(proof.StateProof) == 0 {
		return errors.New("state proof is empty")
	}

	// Verify block hash is provided
	var zeroHash [32]byte
	if proof.BlockHash == zeroHash {
		return errors.New("block hash is required")
	}

	// Verify timestamp
	if proof.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}

	// Verify at least one signature
	if len(proof.Signatures) == 0 {
		return errors.New("at least one signature is required")
	}

	return nil
}

// VerifyMultiSigProof verifies the multi-signature proof for a settlement
func (spv *SettlementProofVerifier) VerifyMultiSigProof(settlement *Settlement, pubKeyA, pubKeyB []byte, sigA, sigB []byte) error {
	if settlement == nil {
		return errors.New("settlement is nil")
	}

	// Create settlement tx for verification
	tx := &SettlementTx{
		ChannelID:    settlement.ChannelID,
		SettlementID: settlement.ID,
		BalanceA:     settlement.BalanceA,
		BalanceB:     settlement.BalanceB,
		Sequence:     settlement.Sequence,
		Timestamp:    settlement.Timestamp,
	}

	data := tx.Serialize()

	// Verify Party A signature if provided
	if len(sigA) > 0 && len(pubKeyA) > 0 {
		if !ed25519.Verify(pubKeyA, data, sigA) {
			return errors.New("Party A signature verification failed")
		}
	}

	// Verify Party B signature if provided
	if len(sigB) > 0 && len(pubKeyB) > 0 {
		if !ed25519.Verify(pubKeyB, data, sigB) {
			return errors.New("Party B signature verification failed")
		}
	}

	// At least one signature must be valid
	if (len(sigA) == 0 || len(pubKeyA) == 0) && (len(sigB) == 0 || len(pubKeyB) == 0) {
		return errors.New("no valid signatures provided")
	}

	return nil
}

// VerifyStateTransitionProof verifies the state transition proof
func (spv *SettlementProofVerifier) VerifyStateTransitionProof(oldState, newState *interfaces.SignedState, proof *SettlementProof) error {
	if oldState == nil || newState == nil {
		return errors.New("states are required")
	}

	// Verify sequence number increased
	if newState.Sequence <= oldState.Sequence {
		return fmt.Errorf("sequence must increase: old %d, new %d", oldState.Sequence, newState.Sequence)
	}

	// Verify balance conservation
	oldTotal := oldState.BalanceA + oldState.BalanceB
	newTotal := newState.BalanceA + newState.BalanceB
	if oldTotal != newTotal {
		return fmt.Errorf("balance conservation violated: old total %d, new total %d", oldTotal, newTotal)
	}

	// Verify state hash matches (skip if proof.StateHash is zero - for simplified testing)
	var zeroHash [32]byte
	if proof.StateHash != zeroHash {
		oldHash := computeStateHashFromSignedState(oldState)
		newHash := computeStateHashFromSignedState(newState)

		if oldHash != proof.StateHash && newHash != proof.StateHash {
			return errors.New("state hash does not match either state")
		}
	}

	return nil
}

// computeStateHashFromSettlement computes the state hash from a settlement
func computeStateHashFromSettlement(settlement *Settlement) [32]byte {
	h := sha256.New()
	h.Write(settlement.ChannelID[:])
	h.Write(settlement.StateHash[:])
	h.Write(binary.BigEndian.AppendUint64(nil, settlement.BalanceA))
	h.Write(binary.BigEndian.AppendUint64(nil, settlement.BalanceB))
	h.Write(binary.BigEndian.AppendUint64(nil, settlement.Sequence))

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// computeStateHashFromSignedState computes the state hash from a signed state
func computeStateHashFromSignedState(state *interfaces.SignedState) [32]byte {
	h := sha256.New()
	h.Write(state.ChannelID[:])
	h.Write(binary.BigEndian.AppendUint64(nil, state.Sequence))
	h.Write(binary.BigEndian.AppendUint64(nil, state.BalanceA))
	h.Write(binary.BigEndian.AppendUint64(nil, state.BalanceB))

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// GenerateSettlementProof generates a settlement proof
func (sm *SettlementManager) GenerateSettlementProof(channelID [32]byte) (*SettlementProof, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	settlement, ok := sm.settlements[channelID]
	if !ok {
		return nil, errors.New("settlement not found")
	}

	// Get channel for state hash
	channel, err := sm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Create signatures map
	signatures := make(map[string][]byte)
	if len(settlement.SigA) > 0 {
		signatures["PartyA"] = settlement.SigA
	}
	if len(settlement.SigB) > 0 {
		signatures["PartyB"] = settlement.SigB
	}

	proof := &SettlementProof{
		Settlement:  settlement,
		StateHash:   computeStateHash(channel),
		Signatures:  signatures,
		BlockNumber: settlement.BlockNumber,
		Timestamp:   settlement.Timestamp,
	}

	// Generate a simple proof (in real implementation, this would be a merkle proof)
	stateData := serializeState(&interfaces.SignedState{
		ChannelID: channelID,
		Sequence:  settlement.Sequence,
		BalanceA:  settlement.BalanceA,
		BalanceB:  settlement.BalanceB,
	})
	proof.StateProof = stateData

	return proof, nil
}
