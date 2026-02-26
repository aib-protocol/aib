// Package channel implements Lightning-style state channels for AIB 2.0.
// HTLC support for conditional payments with hashlocks and timelocks.
package channel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// HTLC represents a Hash Time-Locked Contract.
type HTLC struct {
	ChannelID    [32]byte
	ID           [32]byte
	HashLock     [32]byte
	TimeLock     time.Time
	Amount       uint64
	Sender       interfaces.Address
	Receiver     interfaces.Address
	Preimage     []byte
	State        int // 0=pending, 1=completed, 2=expired
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

// HTLCStates defines the possible states of an HTLC
const (
	HTLCPending = iota
	HTLCCompleted
	HTLCExpired
)

// HTLCConfig holds HTLC-related configuration
type HTLCConfig struct {
	MinimumExpiryDuration time.Duration
	MaximumExpiryDuration time.Duration
	MinHTLCAmount         uint64
	MaxHTLCAmount         uint64
}

// NewHTLC creates a new pending HTLC.
func NewHTLC(
	channelID [32]byte,
	hashLock [32]byte,
	timeLock time.Time,
	amount uint64,
	sender, receiver interfaces.Address,
) (*HTLC, error) {
	// Validate parameters
	if amount == 0 {
		return nil, errors.New("HTLC amount must be greater than zero")
	}

	if timeLock.IsZero() {
		return nil, errors.New("time lock must be set")
	}

	if time.Now().After(timeLock) {
		return nil, errors.New("time lock must be in the future")
	}

	// Generate unique HTLC ID
	htlcID, err := generateHTLCID(channelID, hashLock, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HTLC ID: %w", err)
	}

	return &HTLC{
		ChannelID:    channelID,
		ID:           htlcID,
		HashLock:     hashLock,
		TimeLock:     timeLock,
		Amount:       amount,
		Sender:       sender,
		Receiver:     receiver,
		State:        HTLCPending,
		CreatedAt:    time.Now(),
	}, nil
}

// generateHTLCID creates a unique identifier for an HTLC.
func generateHTLCID(channelID [32]byte, hashLock [32]byte, amount uint64) ([32]byte, error) {
	h := sha256.New()
	h.Write(channelID[:])
	h.Write(hashLock[:])
	binary.BigEndian.PutUint64(make([]byte, 8), amount)
	h.Write(binary.BigEndian.AppendUint64(nil, amount))

	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return [32]byte{}, err
	}
	h.Write(nonce)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

// GenerateHashLock creates a hash lock from a preimage.
func GenerateHashLock(preimage []byte) ([32]byte, error) {
	if len(preimage) == 0 {
		return [32]byte{}, errors.New("preimage must not be empty")
	}
	return sha256.Sum256(preimage), nil
}

// NewRandomHashLock generates a random hash lock with corresponding preimage.
func NewRandomHashLock() ([32]byte, []byte, error) {
	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return [32]byte{}, nil, err
	}
	hashLock := sha256.Sum256(preimage)
	return hashLock, preimage, nil
}

// AddHTLC adds an HTLC to a channel.
func (m *Manager) AddHTLC(channelID [32]byte, htlc *HTLC) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	if channelState.Status != StateOpen {
		return fmt.Errorf("channel not open: status %d", channelState.Status)
	}

	// Verify HTLC is for this channel
	if htlc.ChannelID != channelID {
		return errors.New("HTLC channel ID mismatch")
	}

	// Verify sender and receiver are valid parties
	if htlc.Sender != channel.PartyA && htlc.Sender != channel.PartyB {
		return errors.New("invalid sender")
	}
	if htlc.Receiver != channel.PartyA && htlc.Receiver != channel.PartyB {
		return errors.New("invalid receiver")
	}
	if htlc.Sender == htlc.Receiver {
		return errors.New("sender and receiver cannot be the same")
	}

	// Check if HTLC already exists
	if _, exists := channelState.PendingHTLCs[htlc.ID]; exists {
		return errors.New("HTLC already exists")
	}

	// Check sender's available balance including existing pending HTLCs
	senderBalance := getAvailableBalance(m, channel, channelState, htlc.Sender)
	if htlc.Amount > senderBalance {
		return ErrInsufficientBalance
	}

	// Add to pending HTLCs
	channelState.PendingHTLCs[htlc.ID] = htlc

	return nil
}

// getAvailableBalance calculates the available balance for a party,
// considering pending HTLCs.
func getAvailableBalance(m *Manager, channel *interfaces.Channel, state *ChannelState, addr interfaces.Address) uint64 {
	if addr == channel.PartyA {
		available := channel.BalanceA
		for _, htlc := range state.PendingHTLCs {
			if htlc.Sender == channel.PartyA {
				available -= htlc.Amount
			}
		}
		if available < 0 {
			return 0
		}
		return uint64(available)
	} else if addr == channel.PartyB {
		available := channel.BalanceB
		for _, htlc := range state.PendingHTLCs {
			if htlc.Sender == channel.PartyB {
				available -= htlc.Amount
			}
		}
		if available < 0 {
			return 0
		}
		return uint64(available)
	}
	return 0
}

// GetHTLC retrieves an HTLC by ID.
func (m *Manager) GetHTLC(channelID, htlcID [32]byte) (*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return nil, errors.New("HTLC not found")
	}

	return htlc, nil
}

// GetHTLCs returns all pending HTLCs for a channel.
func (m *Manager) GetHTLCs(channelID [32]byte) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		htlcs = append(htlcs, htlc)
	}
	return htlcs, nil
}

// CompleteHTLC completes an HTLC with the preimage.
func (m *Manager) CompleteHTLC(channelID, htlcID [32]byte, preimage []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return errors.New("HTLC not found")
	}

	if htlc.State != HTLCPending {
		return fmt.Errorf("HTLC not pending: state %d", htlc.State)
	}

	// Verify preimage matches hash lock
	calculatedHash := sha256.Sum256(preimage)
	if calculatedHash != htlc.HashLock {
		return errors.New("invalid preimage")
	}

	// Update balances
	if htlc.Sender == channel.PartyA && htlc.Receiver == channel.PartyB {
		channel.BalanceA -= htlc.Amount
		channel.BalanceB += htlc.Amount
	} else if htlc.Sender == channel.PartyB && htlc.Receiver == channel.PartyA {
		channel.BalanceB -= htlc.Amount
		channel.BalanceA += htlc.Amount
	}

	// Increment channel sequence number
	channel.Sequence++

	// Complete the HTLC
	htlc.State = HTLCCompleted
	htlc.Preimage = preimage
	now := time.Now()
	htlc.CompletedAt = &now

	// Update state hash and last update time
	channel.StateHash = computeStateHash(channel)
	channelState.LastUpdate = now

	return nil
}

// ExpireHTLC expires an HTLC that has timed out.
func (m *Manager) ExpireHTLC(channelID, htlcID [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return errors.New("HTLC not found")
	}

	if htlc.State != HTLCPending {
		return fmt.Errorf("HTLC not pending: state %d", htlc.State)
	}

	// Check if time lock has expired
	if time.Now().Before(htlc.TimeLock) {
		return errors.New("time lock not expired")
	}

	// Mark as expired
	htlc.State = HTLCExpired
	now := time.Now()
	htlc.CompletedAt = &now

	return nil
}

// RemoveHTLC removes an HTLC from pending HTLCs.
func (m *Manager) RemoveHTLC(channelID, htlcID [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	delete(channelState.PendingHTLCs, htlcID)
	return nil
}

// SettleExpiredHTLCs settles all expired HTLCs for a channel.
func (m *Manager) SettleExpiredHTLCs(channelID [32]byte) ([]*HTLC, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var expired []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.State == HTLCPending && time.Now().After(htlc.TimeLock) {
			htlc.State = HTLCExpired
			now := time.Now()
			htlc.CompletedAt = &now
			expired = append(expired, htlc)
		}
	}

	return expired, nil
}

// CountHTLCs returns the number of pending HTLCs for a channel.
func (m *Manager) CountHTLCs(channelID [32]byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return 0, ErrChannelNotFound
	}

	return len(channelState.PendingHTLCs), nil
}

// GetTotalHTLCAmount returns the total pending HTLC amount for a channel.
func (m *Manager) GetTotalHTLCAmount(channelID [32]byte) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return 0, ErrChannelNotFound
	}

	var total uint64
	for _, htlc := range channelState.PendingHTLCs {
		total += htlc.Amount
	}
	return total, nil
}

// GetHTLCsBySender returns all pending HTLCs sent by a specific party.
func (m *Manager) GetHTLCsBySender(channelID [32]byte, sender interfaces.Address) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.Sender == sender {
			htlcs = append(htlcs, htlc)
		}
	}
	return htlcs, nil
}

// GetHTLCsByReceiver returns all pending HTLCs received by a specific party.
func (m *Manager) GetHTLCsByReceiver(channelID [32]byte, receiver interfaces.Address) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.Receiver == receiver {
			htlcs = append(htlcs, htlc)
		}
	}
	return htlcs, nil
}

// CheckHTLCExpiration checks if an HTLC is expired.
func (m *Manager) CheckHTLCExpiration(channelID, htlcID [32]byte) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	htlc, err := m.GetHTLC(channelID, htlcID)
	if err != nil {
		return false, err
	}

	return time.Now().After(htlc.TimeLock), nil
}

// RouteHTLC sets up a multi-hop HTLC route.
func (m *Manager) RouteHTLC(
	route []interfaces.Address,
	amount uint64,
	hashLock [32]byte,
	finalTimeLock time.Time,
	hopExpiry time.Duration,
) ([]*HTLC, [32]byte, error) {
	if len(route) < 2 {
		return nil, hashLock, errors.New("route must have at least two parties")
	}

	var htlcs []*HTLC
	currentTimeLock := finalTimeLock

	for i := 0; i < len(route)-1; i++ {
		sender := route[i]
		receiver := route[i+1]

		// Find channel between sender and receiver
		var channel *interfaces.Channel
		for _, ch := range m.channels {
			if (ch.PartyA == sender && ch.PartyB == receiver) ||
				(ch.PartyA == receiver && ch.PartyB == sender) {
				channel = ch
				break
			}
		}

		if channel == nil {
			return nil, hashLock, fmt.Errorf("no channel between %x and %x", sender, receiver)
		}

		// Create HTLC for this hop
		htlc, err := NewHTLC(
			channel.ID,
			hashLock,
			currentTimeLock,
			amount,
			sender,
			receiver,
		)
		if err != nil {
			return nil, hashLock, err
		}

		// Add to channel
		if err := m.AddHTLC(channel.ID, htlc); err != nil {
			return nil, hashLock, err
		}

		htlcs = append(htlcs, htlc)

		// Decrement time lock for next hop
		currentTimeLock = currentTimeLock.Add(-hopExpiry)
		if currentTimeLock.Before(time.Now()) {
			// Cleanup previously added HTLCs
			for _, addedHTLC := range htlcs {
				m.RemoveHTLC(addedHTLC.ChannelID, addedHTLC.ID)
			}
			return nil, hashLock, errors.New("time lock for intermediate hop would be in the past")
		}
	}

	return htlcs, hashLock, nil
}

// RoutePayment sets up a multi-hop payment with HTLCs.
func (m *Manager) RoutePayment(
	route []interfaces.Address,
	amount uint64,
	preimage []byte,
	expiry time.Duration,
) ([]*HTLC, [32]byte, error) {
	hashLock := sha256.Sum256(preimage)
	finalTimeLock := time.Now().Add(expiry)
	hopExpiry := time.Second * 3600 // 1 hour per hop

	return m.RouteHTLC(route, amount, hashLock, finalTimeLock, hopExpiry)
}

// CompleteRouteHTLC completes a multi-hop HTLC route.
func (m *Manager) CompleteRouteHTLC(htlcs []*HTLC, preimage []byte) error {
	for _, htlc := range htlcs {
		if err := m.CompleteHTLC(htlc.ChannelID, htlc.ID, preimage); err != nil {
			// If any HTLC fails, try to expire the ones we completed
			for _, completed := range htlcs {
				if completed != htlc { // Skip the failing one
					m.ExpireHTLC(completed.ChannelID, completed.ID)
				}
			}
			return err
		}
	}
	return nil
}

// ExpireRouteHTLC expires all HTLCs in a route.
func (m *Manager) ExpireRouteHTLC(htlcs []*HTLC) error {
	for _, htlc := range htlcs {
		if err := m.ExpireHTLC(htlc.ChannelID, htlc.ID); err != nil {
			// Continue with other HTLCs even if one fails
			continue
		}
	}
	return nil
}

// CreateAtomicSwap creates an atomic swap between two chains using HTLCs.
func (m *Manager) CreateAtomicSwap(
	sourceChain, destinationChain string,
	sourceParty, destinationParty interfaces.Address,
	amountA, amountB uint64,
	hashLock [32]byte,
	timeLock time.Time,
) (*HTLC, *HTLC, error) {
	// Find source channel
	var sourceChannel *interfaces.Channel
	for _, ch := range m.channels {
		if (ch.PartyA == sourceParty || ch.PartyB == sourceParty) &&
			isChainSupported(sourceChain, ch) {
			sourceChannel = ch
			break
		}
	}
	if sourceChannel == nil {
		return nil, nil, errors.New("no supported channel on source chain")
	}

	// Find destination channel
	var destChannel *interfaces.Channel
	for _, ch := range m.channels {
		if (ch.PartyA == destinationParty || ch.PartyB == destinationParty) &&
			isChainSupported(destinationChain, ch) {
			destChannel = ch
			break
		}
	}
	if destChannel == nil {
		return nil, nil, errors.New("no supported channel on destination chain")
	}

	// Create HTLCs for both sides
	htlcA, err := NewHTLC(
		sourceChannel.ID,
		hashLock,
		timeLock,
		amountA,
		sourceParty,
		destinationParty,
	)
	if err != nil {
		return nil, nil, err
	}

	htlcB, err := NewHTLC(
		destChannel.ID,
		hashLock,
		timeLock,
		amountB,
		destinationParty,
		sourceParty,
	)
	if err != nil {
		return nil, nil, err
	}

	// Add both HTLCs to their respective channels
	if err := m.AddHTLC(sourceChannel.ID, htlcA); err != nil {
		return nil, nil, err
	}

	if err := m.AddHTLC(destChannel.ID, htlcB); err != nil {
		if err2 := m.RemoveHTLC(htlcA.ChannelID, htlcA.ID); err2 != nil {
			// Log the error but continue, since we failed to add the second HTLC
		}
		return nil, nil, err
	}

	return htlcA, htlcB, nil
}

// isChainSupported checks if a chain is supported by a channel.
func isChainSupported(chain string, ch *interfaces.Channel) bool {
	// In a real implementation, this would check channel metadata
	// For now, we assume all channels support all chains
	_ = chain
	return true
}

// CompleteAtomicSwap completes an atomic swap by revealing the preimage.
func (m *Manager) CompleteAtomicSwap(htlcA, htlcB *HTLC, preimage []byte) error {
	// First complete both HTLCs
	err1 := m.CompleteHTLC(htlcA.ChannelID, htlcA.ID, preimage)
	err2 := m.CompleteHTLC(htlcB.ChannelID, htlcB.ID, preimage)

	if err1 != nil || err2 != nil {
		// If either fails, try to expire both
		if errExp1 := m.ExpireHTLC(htlcA.ChannelID, htlcA.ID); errExp1 != nil {
			// Log error
		}
		if errExp2 := m.ExpireHTLC(htlcB.ChannelID, htlcB.ID); errExp2 != nil {
			// Log error
		}

		if err1 != nil {
			return err1
		}
		return err2
	}

	return nil
}

// ExpireAtomicSwap expires an atomic swap.
func (m *Manager) ExpireAtomicSwap(htlcA, htlcB *HTLC) error {
	err1 := m.ExpireHTLC(htlcA.ChannelID, htlcA.ID)
	err2 := m.ExpireHTLC(htlcB.ChannelID, htlcB.ID)

	if err1 != nil || err2 != nil {
		if err1 != nil {
			return err1
		}
		return err2
	}

	return nil
}

// GetAtomicSwapStatus returns the status of an atomic swap.
func (m *Manager) GetAtomicSwapStatus(htlcA, htlcB *HTLC) (string, error) {
	statusA, err := m.GetHTLC(htlcA.ChannelID, htlcA.ID)
	if err != nil {
		return "failed", err
	}

	statusB, err := m.GetHTLC(htlcB.ChannelID, htlcB.ID)
	if err != nil {
		return "failed", err
	}

	if statusA.State == HTLCCompleted && statusB.State == HTLCCompleted {
		return "completed", nil
	} else if statusA.State == HTLCExpired && statusB.State == HTLCExpired {
		return "expired", nil
	} else if statusA.State == HTLCCompleted || statusB.State == HTLCCompleted {
		return "partial", nil
	} else {
		return "pending", nil
	}
}
