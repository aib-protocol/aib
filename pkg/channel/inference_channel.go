package channel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Inference channel errors
var (
	ErrInvalidLevel         = errors.New("invalid inference level")
	ErrChannelAlreadyClosed = errors.New("channel already closed")
	ErrChannelNotOpen       = errors.New("channel is not open")
)

// Inference API tier pricing (satoshi per call)
var InferencePrices = map[uint8]uint64{
	1: 100000,   // Level 1: 0.001 AIB
	2: 1000000,  // Level 2: 0.01 AIB
	3: 10000000, // Level 3: 0.1 AIB
}

const (
	// MinDeposit is the minimum deposit amount
	MinDeposit uint64 = 1000000 // 0.01 AIB
)

// InferenceChannelStatus channelstatus
type InferenceChannelStatus uint8

const (
	// ICOpen means the channel is open
	ICOpen InferenceChannelStatus = iota
	// ICSettling means the channel is settling
	ICSettling
	// ICClosed means the channel is closed
	ICClosed
	// ICDisputed means the channel is in dispute
	ICDisputed
)

// InferenceChannel is a payment channel for inference
type InferenceChannel struct {
	ChannelID       [32]byte
	UserPubKey      [32]byte
	NodePubKey      [32]byte
	UserBalance     uint64 // user balance
	NodeBalance     uint64 // node balance
	TotalDeposit    uint64 // total deposited amount
	Level           uint8  // API tier 1/2/3
	InferenceCount  uint64 // number of inferences
	SequenceNum     uint64 // sequence number (anti-replay)
	Status          InferenceChannelStatus
	CreatedAt       uint64
	ClosedAt        uint64
	ChallengeEnd    *time.Time
	ChallengeReason string
	mu              sync.Mutex
}

// InferenceChannelManager manages all inference channels
type InferenceChannelManager struct {
	channels map[[32]byte]*InferenceChannel
	mu       sync.RWMutex
}

// NewInferenceChannelManager creates a new inference channel manager
func NewInferenceChannelManager() *InferenceChannelManager {
	return &InferenceChannelManager{
		channels: make(map[[32]byte]*InferenceChannel),
	}
}

// generateInferenceChannelID generates a unique channel ID
func generateInferenceChannelID(userPubKey, nodePubKey [32]byte, nonce uint64) [32]byte {
	h := sha256.New()
	h.Write(userPubKey[:])
	h.Write(nodePubKey[:])
	h.Write(binary.BigEndian.AppendUint64(nil, nonce))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// CreateChannel creates a channel (user deposits AIB upfront)
func (m *InferenceChannelManager) CreateChannel(
	userPubKey, nodePubKey [32]byte,
	userDeposit uint64,
	level uint8,
) (*InferenceChannel, error) {
	// Validate the tier
	if level < 1 || level > 3 {
		return nil, ErrInvalidLevel
	}

	// Validate the deposit
	if userDeposit < MinDeposit {
		return nil, fmt.Errorf("%w: minimum deposit is %d", ErrInvalidBalance, MinDeposit)
	}

	// Generate a random nonce
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := binary.BigEndian.Uint64(nonceBytes)

	// generatechannelID
	channelID := generateInferenceChannelID(userPubKey, nodePubKey, nonce)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check whether the channel already exists
	if _, exists := m.channels[channelID]; exists {
		return nil, ErrAlreadyExists
	}

	// createchannel
	now := uint64(time.Now().Unix())
	channel := &InferenceChannel{
		ChannelID:      channelID,
		UserPubKey:     userPubKey,
		NodePubKey:     nodePubKey,
		UserBalance:    userDeposit,
		NodeBalance:    0,
		TotalDeposit:   userDeposit,
		Level:          level,
		InferenceCount: 0,
		SequenceNum:    0,
		Status:         ICOpen,
		CreatedAt:      now,
	}

	m.channels[channelID] = channel

	return channel, nil
}

// RecordInference updates balances after an inference call (off-chain, not on-chain)
func (c *InferenceChannel) RecordInference() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status != ICOpen {
		return ErrChannelNotOpen
	}

	// Get the fee for the current tier
	fee, ok := InferencePrices[c.Level]
	if !ok {
		return ErrInvalidLevel
	}

	// Check whether the balance is sufficient
	if c.UserBalance < fee {
		return ErrInsufficientBalance
	}

	// Deduct the fee
	c.UserBalance -= fee
	c.NodeBalance += fee

	// Update the counters
	c.InferenceCount++
	c.SequenceNum++

	return nil
}

// Settle settles on-chain
func (c *InferenceChannel) Settle() (*SettlementData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status != ICOpen && c.Status != ICSettling {
		return nil, ErrChannelAlreadyClosed
	}

	// Mark as settling
	c.Status = ICSettling

	// createsettlementdata
	settlement := &SettlementData{
		ChannelID:      c.ChannelID,
		FinalUserBal:   c.UserBalance,
		FinalNodeBal:   c.NodeBalance,
		InferenceCount: c.InferenceCount,
		SequenceNum:    c.SequenceNum,
	}

	return settlement, nil
}

// Challenge raises a dispute (when cheating is detected)
func (c *InferenceChannel) Challenge(reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status == ICClosed {
		return ErrChannelAlreadyClosed
	}

	// Set the dispute status
	c.Status = ICDisputed
	c.ChallengeReason = reason

	// Set the challenge end time (24 hours from now)
	challengeEnd := time.Now().Add(24 * time.Hour)
	c.ChallengeEnd = &challengeEnd

	return nil
}

// GetChannel getchannelinfo
func (m *InferenceChannelManager) GetChannel(id [32]byte) (*InferenceChannel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, exists := m.channels[id]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// return a copy (manual field copy: never duplicate a sync.Mutex)
	chCopy := InferenceChannel{
		ChannelID:      channel.ChannelID,
		UserPubKey:     channel.UserPubKey,
		NodePubKey:     channel.NodePubKey,
		UserBalance:    channel.UserBalance,
		NodeBalance:    channel.NodeBalance,
		TotalDeposit:   channel.TotalDeposit,
		Level:          channel.Level,
		InferenceCount: channel.InferenceCount,
		SequenceNum:    channel.SequenceNum,
		Status:         channel.Status,
		CreatedAt:      channel.CreatedAt,
		ClosedAt:       channel.ClosedAt,
	}
	if channel.ChallengeEnd != nil {
		t := *channel.ChallengeEnd
		chCopy.ChallengeEnd = &t
	}
	chCopy.ChallengeReason = channel.ChallengeReason
	return &chCopy, nil
}

// Close closes the channel
func (c *InferenceChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status == ICClosed {
		return ErrChannelAlreadyClosed
	}

	// Mark as closed
	c.Status = ICClosed
	c.ClosedAt = uint64(time.Now().Unix())

	return nil
}

// GetChannels returnsallchannel
func (m *InferenceChannelManager) GetChannels() []*InferenceChannel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*InferenceChannel, 0, len(m.channels))
	for _, ch := range m.channels {
		result = append(result, ch.snapshot())
	}
	return result
}

// snapshot returns a field-wise copy of the channel without copying its
// mutex (copying a sync.Mutex is flagged by go vet and is unsafe).
func (c *InferenceChannel) snapshot() *InferenceChannel {
	cp := InferenceChannel{
		ChannelID:       c.ChannelID,
		UserPubKey:      c.UserPubKey,
		NodePubKey:      c.NodePubKey,
		UserBalance:     c.UserBalance,
		NodeBalance:     c.NodeBalance,
		TotalDeposit:    c.TotalDeposit,
		Level:           c.Level,
		InferenceCount:  c.InferenceCount,
		SequenceNum:     c.SequenceNum,
		Status:          c.Status,
		CreatedAt:       c.CreatedAt,
		ClosedAt:        c.ClosedAt,
		ChallengeReason: c.ChallengeReason,
	}
	if c.ChallengeEnd != nil {
		t := *c.ChallengeEnd
		cp.ChallengeEnd = &t
	}
	return &cp
}

// GetChannelCount returnschannelcount
func (m *InferenceChannelManager) GetChannelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels)
}

// SettlementData settlementdata
type SettlementData struct {
	ChannelID      [32]byte
	FinalUserBal   uint64
	FinalNodeBal   uint64
	InferenceCount uint64
	SequenceNum    uint64
}

// IsValid verifysettlementdata
func (s *SettlementData) IsValid() error {
	// Verify total balance conservation
	total := s.FinalUserBal + s.FinalNodeBal
	if total == 0 {
		return errors.New("zero total balance")
	}
	return nil
}

// GetChannelIDString returns the string form of the channel ID
func (c *InferenceChannel) GetChannelIDString() string {
	return fmt.Sprintf("%x", c.ChannelID)
}

// GetFee returns the inference fee for the current tier
func (c *InferenceChannel) GetFee() uint64 {
	return InferencePrices[c.Level]
}

// GetRemainingInferences returns the number of remaining inferences
func (c *InferenceChannel) GetRemainingInferences() uint64 {
	fee := c.GetFee()
	if fee == 0 {
		return 0
	}
	return c.UserBalance / fee
}

// IsExpired checks whether the channel has expired
func (c *InferenceChannel) IsExpired() bool {
	// Channels expire 30 days after creation
	expiryTime := time.Unix(int64(c.CreatedAt), 0).Add(30 * 24 * time.Hour)
	return time.Now().After(expiryTime)
}

// CanSettle checks whether the channel can be settled
func (c *InferenceChannel) CanSettle() bool {
	return c.Status == ICOpen || c.Status == ICSettling
}

// IsInDispute checks whether the channel is in dispute
func (c *InferenceChannel) IsInDispute() bool {
	return c.Status == ICDisputed
}

// IsClosed checks whether the channel is closed
func (c *InferenceChannel) IsClosed() bool {
	return c.Status == ICClosed
}
