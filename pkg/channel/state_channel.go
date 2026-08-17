// Package channel implements Lightning-style state channels for AIB 2.0.
// It provides HTLC support, dispute resolution, and cross-chain operations.
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

// Errors
var (
	ErrChannelNotFound     = errors.New("channel not found")
	ErrInvalidSignature    = errors.New("invalid signature")
	ErrInvalidSequence     = errors.New("invalid sequence number")
	ErrInvalidBalance      = errors.New("invalid balance")
	ErrChannelClosed       = errors.New("channel is closed")
	ErrChannelInDispute    = errors.New("channel is in dispute")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrDisputeTimeout      = errors.New("dispute timeout not reached")
	ErrInvalidState        = errors.New("invalid state")
	ErrAlreadyExists       = errors.New("channel already exists")
)

// State represents the current state of a channel
const (
	StateOpen = iota
	StateClosing
	StateClosed
	StateInDispute
)

// Manager implements the ChannelManager interface for state channel management.
type Manager struct {
	channels   map[[32]byte]*interfaces.Channel
	states     map[[32]byte]*ChannelState
	disputes   map[[32]byte]*DisputeRecord
	multiSig   interfaces.MultiSigLocker
	signer     crypto.Signer
	signerLock sync.RWMutex
	mu         sync.RWMutex

	// Configuration
	challengePeriod time.Duration
	minDeposit      uint64
	maxChannelValue uint64
}

// ChannelState tracks the runtime state of a channel
type ChannelState struct {
	Status        int
	LastUpdate    time.Time
	PendingHTLCs  map[[32]byte]*HTLC
	ReceivedNonce uint64
}

// DisputeRecord tracks an active or resolved dispute
type DisputeRecord struct {
	ChannelID       [32]byte
	Challenger      interfaces.Address
	ChallengedState interfaces.SignedState
	ChallengeStart  time.Time
	ChallengeEnd    time.Time
	Resolved        bool
	Winner          interfaces.Address
	Resolution      string
}

// Config holds configuration for the channel manager
type Config struct {
	ChallengePeriod time.Duration
	MinDeposit      uint64
	MaxChannelValue uint64
	MultiSigLocker  interfaces.MultiSigLocker
}

// NewManager creates a new channel manager with the given configuration.
func NewManager(cfg *Config) (*Manager, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if cfg.MultiSigLocker == nil {
		return nil, errors.New("multi-sig locker is required")
	}
	if cfg.ChallengePeriod == 0 {
		cfg.ChallengePeriod = 24 * time.Hour
	}

	return &Manager{
		channels:        make(map[[32]byte]*interfaces.Channel),
		states:          make(map[[32]byte]*ChannelState),
		disputes:        make(map[[32]byte]*DisputeRecord),
		multiSig:        cfg.MultiSigLocker,
		challengePeriod: cfg.ChallengePeriod,
		minDeposit:      cfg.MinDeposit,
		maxChannelValue: cfg.MaxChannelValue,
	}, nil
}

// SetSigner sets the signing key for the manager.
func (m *Manager) SetSigner(signer crypto.Signer) {
	m.signerLock.Lock()
	defer m.signerLock.Unlock()
	m.signer = signer
}

// generateChannelID creates a unique channel ID from party addresses and nonce.
func generateChannelID(partyA, partyB interfaces.Address, nonce uint64) [32]byte {
	h := sha256.New()
	h.Write(partyA[:])
	h.Write(partyB[:])
	binary.BigEndian.PutUint64(make([]byte, 8), nonce)
	h.Write(binary.BigEndian.AppendUint64(nil, nonce))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// OpenChannel opens a new state channel with initial deposits.
func (m *Manager) OpenChannel(ctx context.Context, partyA, partyB interfaces.Address, depositA, depositB uint64) (*interfaces.Channel, error) {
	// Validate deposits
	if depositA < m.minDeposit || depositB < m.minDeposit {
		return nil, fmt.Errorf("%w: minimum deposit is %d", ErrInvalidBalance, m.minDeposit)
	}

	totalValue := depositA + depositB
	if totalValue > m.maxChannelValue {
		return nil, fmt.Errorf("%w: max channel value is %d", ErrInvalidBalance, m.maxChannelValue)
	}

	// Generate channel ID using random nonce
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := binary.BigEndian.Uint64(nonceBytes)
	channelID := generateChannelID(partyA, partyB, nonce)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if channel already exists
	if _, exists := m.channels[channelID]; exists {
		return nil, ErrAlreadyExists
	}

	// Create the funding output via MultiSigLocker
	utxo, err := m.multiSig.CreateMultiSigOutput(partyA, partyB, totalValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-sig output: %w", err)
	}

	// Create channel
	now := time.Now()
	channel := &interfaces.Channel{
		ID:        channelID,
		PartyA:    partyA,
		PartyB:    partyB,
		BalanceA:  depositA,
		BalanceB:  depositB,
		Sequence:  0,
		CreatedAt: now,
	}

	// Compute initial state hash
	stateHash := computeStateHash(channel)
	channel.StateHash = stateHash

	// Store channel and state
	m.channels[channelID] = channel
	m.states[channelID] = &ChannelState{
		Status:       StateOpen,
		LastUpdate:   now,
		PendingHTLCs: make(map[[32]byte]*HTLC),
	}

	// Store the UTXO reference in channel metadata
	_ = utxo

	return channel, nil
}

// computeStateHash computes the hash of a channel state.
func computeStateHash(ch *interfaces.Channel) [32]byte {
	h := sha256.New()
	h.Write(ch.ID[:])
	h.Write(ch.PartyA[:])
	h.Write(ch.PartyB[:])
	binary.BigEndian.PutUint64(make([]byte, 8), ch.BalanceA)
	h.Write(binary.BigEndian.AppendUint64(nil, ch.BalanceA))
	binary.BigEndian.PutUint64(make([]byte, 8), ch.BalanceB)
	h.Write(binary.BigEndian.AppendUint64(nil, ch.BalanceB))
	binary.BigEndian.PutUint64(make([]byte, 8), ch.Sequence)
	h.Write(binary.BigEndian.AppendUint64(nil, ch.Sequence))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// serializeState serializes a signed state for hashing/signing.
func serializeState(state *interfaces.SignedState) []byte {
	buf := make([]byte, 0, 200)
	buf = append(buf, state.ChannelID[:]...)
	buf = binary.BigEndian.AppendUint64(buf, state.Sequence)
	buf = binary.BigEndian.AppendUint64(buf, state.BalanceA)
	buf = binary.BigEndian.AppendUint64(buf, state.BalanceB)
	buf = append(buf, byte(state.Timestamp.Unix()))
	return buf
}

// UpdateState updates the channel state with a new signed state.
func (m *Manager) UpdateState(ch *interfaces.Channel, newState interfaces.SignedState) (*interfaces.SignedState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify channel exists
	channel, exists := m.channels[ch.ID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// Check channel state
	channelState := m.states[ch.ID]
	if channelState.Status == StateClosed {
		return nil, ErrChannelClosed
	}
	if channelState.Status == StateInDispute {
		return nil, ErrChannelInDispute
	}

	// Verify channel ID matches
	if newState.ChannelID != ch.ID {
		return nil, ErrInvalidState
	}

	// Verify sequence number is greater than current
	if newState.Sequence <= channel.Sequence {
		return nil, fmt.Errorf("%w: new sequence %d <= current %d", ErrInvalidSequence, newState.Sequence, channel.Sequence)
	}

	// Verify signatures
	if err := m.verifySignatures(channel, &newState); err != nil {
		return nil, err
	}

	// Verify balance conservation (no money created)
	oldTotal := channel.BalanceA + channel.BalanceB
	newTotal := newState.BalanceA + newState.BalanceB
	if newTotal != oldTotal {
		return nil, fmt.Errorf("%w: total balance changed from %d to %d", ErrInvalidBalance, oldTotal, newTotal)
	}

	// Update channel state
	channel.BalanceA = newState.BalanceA
	channel.BalanceB = newState.BalanceB
	channel.Sequence = newState.Sequence
	channel.StateHash = computeStateHash(channel)

	channelState.LastUpdate = time.Now()

	// Return the signed state as confirmation
	return &newState, nil
}

// verifySignatures verifies both party signatures on a state.
func (m *Manager) verifySignatures(channel *interfaces.Channel, state *interfaces.SignedState) error {
	// Serialize state for verification
	stateData := serializeState(state)

	// Verify Party A signature
	if len(state.SigA) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature A length", ErrInvalidSignature)
	}
	if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, state.SigA) {
		return fmt.Errorf("%w: party A signature verification failed", ErrInvalidSignature)
	}

	// Verify Party B signature
	if len(state.SigB) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature B length", ErrInvalidSignature)
	}
	if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, state.SigB) {
		return fmt.Errorf("%w: party B signature verification failed", ErrInvalidSignature)
	}

	return nil
}

// CloseChannel closes the channel with the final state.
func (m *Manager) CloseChannel(ctx context.Context, ch *interfaces.Channel, finalState interfaces.SignedState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify channel exists
	channel, exists := m.channels[ch.ID]
	if !exists {
		return ErrChannelNotFound
	}

	// Check channel state
	channelState := m.states[ch.ID]
	if channelState.Status == StateClosed {
		return ErrChannelClosed
	}

	// Verify channel ID matches
	if finalState.ChannelID != ch.ID {
		return ErrInvalidState
	}

	// Verify signatures
	if err := m.verifySignatures(channel, &finalState); err != nil {
		return err
	}

	// Verify balance conservation
	oldTotal := channel.BalanceA + channel.BalanceB
	newTotal := finalState.BalanceA + finalState.BalanceB
	if newTotal != oldTotal {
		return fmt.Errorf("%w: total balance changed from %d to %d", ErrInvalidBalance, oldTotal, newTotal)
	}

	// Create settlement outputs
	outputs := []interfaces.TXOutput{
		{Value: finalState.BalanceA, Address: channel.PartyA},
		{Value: finalState.BalanceB, Address: channel.PartyB},
	}

	// Find the funding UTXO
	// In a real implementation, we'd track the funding UTXO
	// For now, we simulate the spend operation
	utxo := &interfaces.UTXO{
		TxHash:  channel.StateHash, // Using state hash as placeholder
		Index:   0,
		Value:   oldTotal,
		Address: channel.PartyA, // Placeholder
	}

	// Get signatures from state
	sigA := finalState.SigA
	sigB := finalState.SigB

	// Spend the multi-sig output
	if err := m.multiSig.SpendMultiSig(utxo, sigA, sigB, outputs); err != nil {
		return fmt.Errorf("failed to spend multi-sig output: %w", err)
	}

	// Mark channel as closed
	channelState.Status = StateClosed
	channel.BalanceA = finalState.BalanceA
	channel.BalanceB = finalState.BalanceB
	channel.Sequence = finalState.Sequence

	return nil
}

// Dispute initiates a dispute with evidence.
func (m *Manager) Dispute(ctx context.Context, ch *interfaces.Channel, evidence interfaces.SignedState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify channel exists
	channel, exists := m.channels[ch.ID]
	if !exists {
		return ErrChannelNotFound
	}

	// Check channel state
	channelState := m.states[ch.ID]
	if channelState.Status == StateClosed {
		return ErrChannelClosed
	}
	if channelState.Status == StateInDispute {
		return ErrChannelInDispute
	}

	// Verify evidence signatures
	if err := m.verifySignatures(channel, &evidence); err != nil {
		return fmt.Errorf("invalid evidence: %w", err)
	}

	// Verify evidence is newer than current state
	if evidence.Sequence <= channel.Sequence {
		return fmt.Errorf("%w: evidence sequence %d <= current %d", ErrInvalidSequence, evidence.Sequence, channel.Sequence)
	}

	// Start dispute period
	now := time.Now()
	challengeEnd := now.Add(m.challengePeriod)

	dispute := &DisputeRecord{
		ChannelID:       ch.ID,
		Challenger:      m.getCurrentAddress(), // This would come from context
		ChallengedState: evidence,
		ChallengeStart:  now,
		ChallengeEnd:    challengeEnd,
		Resolved:        false,
	}

	m.disputes[ch.ID] = dispute
	channelState.Status = StateInDispute
	channel.DisputeEnd = &challengeEnd

	return nil
}

// getCurrentAddress returns the current node's address (placeholder).
func (m *Manager) getCurrentAddress() interfaces.Address {
	m.signerLock.RLock()
	defer m.signerLock.RUnlock()

	if m.signer == nil {
		return interfaces.Address{}
	}

	pubKey := m.signer.PublicKey()
	if len(pubKey) >= 32 {
		var addr interfaces.Address
		copy(addr[:], pubKey[:32])
		return addr
	}
	return interfaces.Address{}
}

// GetChannelState returns the current channel state.
func (m *Manager) GetChannelState(channelID [32]byte) (*interfaces.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// Return a copy
	chCopy := *channel
	return &chCopy, nil
}

// GetChannelStatus returns the runtime status of a channel.
func (m *Manager) GetChannelStatus(channelID [32]byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[channelID]
	if !exists {
		return -1, ErrChannelNotFound
	}

	return state.Status, nil
}

// GetDispute returns the dispute record for a channel.
func (m *Manager) GetDispute(channelID [32]byte) (*DisputeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dispute, exists := m.disputes[channelID]
	if !exists {
		return nil, errors.New("no dispute found for channel")
	}

	return dispute, nil
}

// ListChannels returns all active channels.
func (m *Manager) ListChannels() []*interfaces.Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*interfaces.Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		chCopy := *ch
		result = append(result, &chCopy)
	}
	return result
}

// ResolveDispute resolves a dispute after the challenge period.
func (m *Manager) ResolveDispute(channelID [32]byte, honestParty interfaces.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dispute, exists := m.disputes[channelID]
	if !exists {
		return errors.New("dispute not found")
	}

	if dispute.Resolved {
		return errors.New("dispute already resolved")
	}

	// Check if challenge period has passed
	if time.Now().Before(dispute.ChallengeEnd) {
		return ErrDisputeTimeout
	}

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState := m.states[channelID]

	// Apply penalty - all funds go to honest party
	if honestParty == channel.PartyA {
		channel.BalanceA = channel.BalanceA + channel.BalanceB
		channel.BalanceB = 0
	} else if honestParty == channel.PartyB {
		channel.BalanceB = channel.BalanceB + channel.BalanceA
		channel.BalanceA = 0
	} else {
		return errors.New("invalid honest party address")
	}

	// Mark dispute as resolved
	dispute.Resolved = true
	dispute.Winner = honestParty
	dispute.Resolution = "Penalty applied to dishonest party"

	// Close the channel
	channelState.Status = StateClosed

	return nil
}

// CreateSignedState creates a signed state for the current channel state.
func (m *Manager) CreateSignedState(channelID [32]byte) (*interfaces.SignedState, error) {
	m.signerLock.RLock()
	defer m.signerLock.RUnlock()

	if m.signer == nil {
		return nil, errors.New("no signer configured")
	}

	m.mu.RLock()
	channel, exists := m.channels[channelID]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrChannelNotFound
	}
	m.mu.RUnlock()

	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  channel.Sequence,
		BalanceA:  channel.BalanceA,
		BalanceB:  channel.BalanceB,
		Timestamp: time.Now(),
	}

	// Serialize and sign
	stateData := serializeState(state)
	sig, err := m.signer.Sign(stateData)
	if err != nil {
		return nil, fmt.Errorf("failed to sign state: %w", err)
	}

	// Determine which party we are and set the appropriate signature
	pubKey := m.signer.PublicKey()
	if len(pubKey) >= 32 {
		var ourAddr interfaces.Address
		copy(ourAddr[:], pubKey[:32])

		if ourAddr == channel.PartyA {
			state.SigA = sig
		} else if ourAddr == channel.PartyB {
			state.SigB = sig
		}
	}

	return state, nil
}

// AddCounterpartySignature adds the counterparty's signature to a state.
func (m *Manager) AddCounterpartySignature(state *interfaces.SignedState, counterpartySig []byte) error {
	m.mu.RLock()
	channel, exists := m.channels[state.ChannelID]
	if !exists {
		m.mu.RUnlock()
		return ErrChannelNotFound
	}
	m.mu.RUnlock()

	// Serialize state for verification
	stateData := serializeState(state)

	// Determine which signature this is
	if len(state.SigA) == 0 {
		// Verify against Party A
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, counterpartySig) {
			return ErrInvalidSignature
		}
		state.SigA = counterpartySig
	} else if len(state.SigB) == 0 {
		// Verify against Party B
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, counterpartySig) {
			return ErrInvalidSignature
		}
		state.SigB = counterpartySig
	} else {
		return errors.New("both signatures already present")
	}

	return nil
}

// Transfer initiates a balance transfer within a channel.
func (m *Manager) Transfer(channelID [32]byte, amount uint64, fromPartyA bool) (*interfaces.SignedState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	channelState := m.states[channelID]
	if channelState.Status != StateOpen {
		return nil, fmt.Errorf("channel is not open (status: %d)", channelState.Status)
	}

	// Check balance
	if fromPartyA {
		if channel.BalanceA < amount {
			return nil, ErrInsufficientBalance
		}
		channel.BalanceA -= amount
		channel.BalanceB += amount
	} else {
		if channel.BalanceB < amount {
			return nil, ErrInsufficientBalance
		}
		channel.BalanceB -= amount
		channel.BalanceA += amount
	}

	// Increment sequence
	channel.Sequence++

	// Update state hash
	channel.StateHash = computeStateHash(channel)
	channelState.LastUpdate = time.Now()

	// Create unsigned state for signing
	state := &interfaces.SignedState{
		ChannelID: channel.ID,
		Sequence:  channel.Sequence,
		BalanceA:  channel.BalanceA,
		BalanceB:  channel.BalanceB,
		Timestamp: time.Now(),
	}

	return state, nil
}

// ForceClose initiates a unilateral channel close with the latest state.
func (m *Manager) ForceClose(channelID [32]byte, latestState interfaces.SignedState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState := m.states[channelID]
	if channelState.Status != StateOpen {
		return fmt.Errorf("channel cannot be closed (status: %d)", channelState.Status)
	}

	// Verify signatures
	if err := m.verifySignatures(channel, &latestState); err != nil {
		return err
	}

	// Verify this is a valid state for this channel
	if latestState.ChannelID != channelID {
		return ErrInvalidState
	}

	// Start closing process - this enters a dispute-like period
	now := time.Now()
	challengeEnd := now.Add(m.challengePeriod)

	channel.DisputeEnd = &challengeEnd
	channelState.Status = StateClosing

	// Store the closing state
	// In a full implementation, we'd submit this to the blockchain
	_ = latestState

	return nil
}

// FinalizeClose completes a force close after the challenge period.
func (m *Manager) FinalizeClose(channelID [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState := m.states[channelID]
	if channelState.Status != StateClosing {
		return fmt.Errorf("channel is not in closing state (status: %d)", channelState.Status)
	}

	// Check if challenge period has passed
	if channel.DisputeEnd == nil {
		return errors.New("challenge end time not set")
	}

	if time.Now().Before(*channel.DisputeEnd) {
		return ErrDisputeTimeout
	}

	// Close the channel
	channelState.Status = StateClosed

	return nil
}

// GetChannel returns the channel by ID (exported version).
func (m *Manager) GetChannel(channelID [32]byte) (*interfaces.Channel, error) {
	return m.GetChannelState(channelID)
}
