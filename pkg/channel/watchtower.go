// Package channel implements Lightning-style state channels for AIB 2.0.
// Watchtower provides channel monitoring, fraud detection, and penalty triggering.
package channel

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Watchtower Error Definitions
// ============================================================================

var (
	ErrChannelNotMonitored    = errors.New("channel is not being monitored")
	ErrWatchtowerInvalidState = errors.New("watchtower: invalid state")
	ErrFraudDetected          = errors.New("fraud detected")
	ErrChannelFrozen          = errors.New("channel is frozen")
	ErrPunishmentFailed       = errors.New("punishment execution failed")
	ErrAlertChannelFull       = errors.New("alert channel is full")
)

// ============================================================================
// Fraud Type Definitions
// ============================================================================

// FraudType represents the type of fraudulent behavior
type FraudType int

const (
	FraudTypeInvalidClose        FraudType = iota // 违规关闭（使用过期状态）
	FraudTypeDoubleClose                          // 双重关闭尝试
	FraudTypeStateReversion                       // 状态回滚
	FraudTypeBalanceManipulation                  // 余额操纵
	FraudTypeSequenceRollback                     // 序列号回滚
	FraudTypeUnauthorizedClose                    // 未授权关闭
)

// String returns the string representation of FraudType
func (ft FraudType) String() string {
	switch ft {
	case FraudTypeInvalidClose:
		return "invalid_close"
	case FraudTypeDoubleClose:
		return "double_close"
	case FraudTypeStateReversion:
		return "state_reversion"
	case FraudTypeBalanceManipulation:
		return "balance_manipulation"
	case FraudTypeSequenceRollback:
		return "sequence_rollback"
	case FraudTypeUnauthorizedClose:
		return "unauthorized_close"
	default:
		return "unknown"
	}
}

// ============================================================================
// Alert Level Definitions
// ============================================================================

// AlertLevel represents the severity level of an alert
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelCritical
	AlertLevelEmergency
)

// String returns the string representation of AlertLevel
func (al AlertLevel) String() string {
	switch al {
	case AlertLevelInfo:
		return "INFO"
	case AlertLevelWarning:
		return "WARNING"
	case AlertLevelCritical:
		return "CRITICAL"
	case AlertLevelEmergency:
		return "EMERGENCY"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// Alert Type Definitions
// ============================================================================

// AlertType represents the type of alert
type AlertType int

const (
	AlertTypeTransactionAnomaly  AlertType = iota // 异常交易
	AlertTypeChannelAnomaly                       // 通道异常
	AlertTypeNetworkIssue                         // 网络问题
	AlertTypeFraudAttempt                         // 欺诈尝试
	AlertTypePunishmentTriggered                  // 惩罚触发
	AlertTypeChannelFrozen                        // 通道冻结
	AlertTypeStateChange                          // 状态变更
)

// ============================================================================
// Core Data Structures
// ============================================================================

// WatchtowerConfig holds configuration for the Watchtower service
type WatchtowerConfig struct {
	// Monitoring intervals
	MonitorInterval    time.Duration // 监控检查间隔
	StateCheckInterval time.Duration // 状态检查间隔

	// Challenge period
	ChallengePeriod time.Duration // 争议期时长

	// Fraud detection thresholds
	MaxStateAge           time.Duration // 最大状态年龄
	MaxSequenceDiff       uint64        // 最大序列号差异
	MinConfirmationBlocks uint64        // 最小确认块数

	// Penalty settings
	PenaltyEnabled         bool               // 是否启用惩罚
	PenaltyRecipient       interfaces.Address // 惩罚接收地址
	FraudPenaltyMultiplier float64            // 欺诈惩罚倍数

	// Alert settings
	AlertBufferSize int              // 告警缓冲区大小
	AlertThresholds *AlertThresholds // 告警阈值

	// External dependencies
	BlockChecker BlockChecker // 区块检查器

	// Network settings
	P2PPeers []string // P2P 节点列表
}

// AlertThresholds holds alert threshold configuration
type AlertThresholds struct {
	MaxPendingHTLCs      int           // 最大待处理 HTLC 数量
	MaxChannelInactivity time.Duration // 最大通道不活动时间
	MaxFailedAttempts    int           // 最大失败尝试次数
	StateStaleness       time.Duration // 状态过期时间
}

// DefaultWatchtowerConfig returns the default Watchtower configuration
func DefaultWatchtowerConfig() *WatchtowerConfig {
	return &WatchtowerConfig{
		MonitorInterval:        10 * time.Second,
		StateCheckInterval:     5 * time.Second,
		ChallengePeriod:        24 * time.Hour,
		MaxStateAge:            1 * time.Hour,
		MaxSequenceDiff:        100,
		MinConfirmationBlocks:  6,
		PenaltyEnabled:         true,
		FraudPenaltyMultiplier: 1.0,
		AlertBufferSize:        1000,
		AlertThresholds: &AlertThresholds{
			MaxPendingHTLCs:      50,
			MaxChannelInactivity: 24 * time.Hour,
			MaxFailedAttempts:    3,
			StateStaleness:       30 * time.Minute,
		},
		P2PPeers: []string{},
	}
}

// Watchtower is the main service for monitoring channels and detecting fraud
type Watchtower struct {
	// Configuration
	config *WatchtowerConfig

	// Channel manager reference
	manager *Manager

	// Dispute resolver reference
	disputeResolver *DisputeResolver

	// Monitoring state
	monitoredChannels map[[32]byte]*MonitoredChannel // channelID -> state
	mu                sync.RWMutex

	// Channels for alerts and events
	alertChan  chan *Alert
	fraudChan  chan *FraudEvidence
	punishChan chan *PunishmentTask

	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Statistics
	stats WatchtowerStats

	// Notification callbacks
	onAlert         func(*Alert)
	onFraudDetected func(*FraudEvidence)
	onPunishment    func(*PunishmentResult)
}

// MonitoredChannel holds the monitoring state for a channel
type MonitoredChannel struct {
	ChannelID   [32]byte
	PartyA      interfaces.Address
	PartyB      interfaces.Address
	Status      int
	Sequence    uint64
	BalanceA    uint64
	BalanceB    uint64
	StateHash   [32]byte
	LastUpdate  time.Time
	LastChecked time.Time

	// Signature tracking
	LatestSignedState *interfaces.SignedState

	// Fraud detection state
	KnownStates   map[uint64][32]byte // sequence -> stateHash
	CloseAttempts []CloseAttempt
	IsFrozen      bool
	FrozenAt      *time.Time
	FrozenBy      interfaces.Address

	// Monitoring metadata
	MonitorStartTime time.Time
	AlertCount       int
}

// CloseAttempt records a close attempt
type CloseAttempt struct {
	Sequence    uint64
	BalanceA    uint64
	BalanceB    uint64
	Initiator   interfaces.Address
	Timestamp   time.Time
	BlockNumber uint64
	IsOnChain   bool
}

// Alert represents a Watchtower alert
type Alert struct {
	ID           string
	Type         AlertType
	Level        AlertLevel
	ChannelID    [32]byte
	Message      string
	Details      map[string]interface{}
	Timestamp    time.Time
	Acknowledged bool
}

// FraudEvidence holds evidence of fraudulent behavior
type FraudEvidence struct {
	ChannelID    [32]byte
	FraudType    FraudType
	Sequence     uint64
	InvalidState *interfaces.SignedState
	ValidState   *interfaces.SignedState
	Attacker     interfaces.Address
	Victim       interfaces.Address
	Timestamp    time.Time
	BlockNumber  uint64
	Description  string
	Proof        []byte
}

// PunishmentTask represents a punishment task
type PunishmentTask struct {
	ChannelID   [32]byte
	FraudType   FraudType
	Evidence    *FraudEvidence
	HonestParty interfaces.Address
	Result      chan *PunishmentResult
}

// PunishmentResult represents the result of a punishment action
type PunishmentResult struct {
	ChannelID     [32]byte
	Success       bool
	NewBalanceA   uint64
	NewBalanceB   uint64
	PenaltyAmount uint64
	Reason        string
	Timestamp     time.Time
	TxHash        [32]byte
}

// WatchtowerStats holds Watchtower statistics
type WatchtowerStats struct {
	ChannelsMonitored   int64
	AlertsGenerated     int64
	FraudDetected       int64
	PunishmentsExecuted int64
	StateVerifications  int64
	LastUpdate          time.Time
}

// ============================================================================
// Watchtower Factory Methods
// ============================================================================

// NewWatchtower creates a new Watchtower service
func NewWatchtower(manager *Manager, cfg *WatchtowerConfig) (*Watchtower, error) {
	if manager == nil {
		return nil, errors.New("channel manager is required")
	}
	if cfg == nil {
		cfg = DefaultWatchtowerConfig()
	}
	if cfg.MonitorInterval == 0 {
		cfg.MonitorInterval = 10 * time.Second
	}
	if cfg.StateCheckInterval == 0 {
		cfg.StateCheckInterval = 5 * time.Second
	}
	if cfg.ChallengePeriod == 0 {
		cfg.ChallengePeriod = 24 * time.Hour
	}

	ctx, cancel := context.WithCancel(context.Background())

	wt := &Watchtower{
		config:            cfg,
		manager:           manager,
		monitoredChannels: make(map[[32]byte]*MonitoredChannel),
		alertChan:         make(chan *Alert, cfg.AlertBufferSize),
		fraudChan:         make(chan *FraudEvidence, 100),
		punishChan:        make(chan *PunishmentTask, 100),
		ctx:               ctx,
		cancel:            cancel,
	}

	// Start the fraud detection background routine if we have a dispute resolver
	if wt.disputeResolver != nil {
		wt.wg.Add(1)
		go wt.fraudDetectionLoop()
	}

	return wt, nil
}

// NewWatchtowerWithDisputeResolver creates a new Watchtower with dispute resolution
func NewWatchtowerWithDisputeResolver(manager *Manager, disputeResolver *DisputeResolver, cfg *WatchtowerConfig) (*Watchtower, error) {
	if manager == nil {
		return nil, errors.New("channel manager is required")
	}
	if disputeResolver == nil {
		return nil, errors.New("dispute resolver is required")
	}
	if cfg == nil {
		cfg = DefaultWatchtowerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	wt := &Watchtower{
		config:            cfg,
		manager:           manager,
		disputeResolver:   disputeResolver,
		monitoredChannels: make(map[[32]byte]*MonitoredChannel),
		alertChan:         make(chan *Alert, cfg.AlertBufferSize),
		fraudChan:         make(chan *FraudEvidence, 100),
		punishChan:        make(chan *PunishmentTask, 100),
		ctx:               ctx,
		cancel:            cancel,
	}

	return wt, nil
}

// ============================================================================
// Watchtower Lifecycle Methods
// ============================================================================

// Start starts the Watchtower monitoring service
func (wt *Watchtower) Start() error {
	wt.wg.Add(1)
	go wt.monitoringLoop()

	wt.wg.Add(1)
	go wt.alertProcessingLoop()

	wt.wg.Add(1)
	go wt.punishmentLoop()

	wt.wg.Add(1)
	go wt.statsUpdateLoop()

	return nil
}

// Stop stops the Watchtower service
func (wt *Watchtower) Stop() {
	wt.cancel()
	wt.wg.Wait()
	close(wt.alertChan)
	close(wt.fraudChan)
	close(wt.punishChan)
}

// ============================================================================
// Channel Monitoring Methods
// ============================================================================

// StartMonitoring starts monitoring a channel
func (wt *Watchtower) StartMonitoring(channelID [32]byte) error {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	// Check if already monitored
	if _, exists := wt.monitoredChannels[channelID]; exists {
		return nil // Already monitoring
	}

	// Get channel state from manager
	channel, err := wt.manager.GetChannelState(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}

	// Create monitored channel state
	now := time.Now()
	mc := &MonitoredChannel{
		ChannelID:        channelID,
		PartyA:           channel.PartyA,
		PartyB:           channel.PartyB,
		Status:           StateOpen,
		Sequence:         channel.Sequence,
		BalanceA:         channel.BalanceA,
		BalanceB:         channel.BalanceB,
		StateHash:        channel.StateHash,
		LastUpdate:       now,
		LastChecked:      now,
		KnownStates:      make(map[uint64][32]byte),
		CloseAttempts:    make([]CloseAttempt, 0),
		MonitorStartTime: now,
	}

	// Record initial state
	mc.KnownStates[channel.Sequence] = channel.StateHash

	wt.monitoredChannels[channelID] = mc

	wt.stats.ChannelsMonitored++

	// Send alert
	wt.sendAlert(AlertTypeStateChange, AlertLevelInfo, channelID,
		"Channel monitoring started",
		map[string]interface{}{
			"sequence":  channel.Sequence,
			"balance_a": channel.BalanceA,
			"balance_b": channel.BalanceB,
		})

	return nil
}

// StopMonitoring stops monitoring a channel
func (wt *Watchtower) StopMonitoring(channelID [32]byte) error {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	mc, exists := wt.monitoredChannels[channelID]
	if !exists {
		return ErrChannelNotMonitored
	}

	// Check if channel is frozen
	if mc.IsFrozen {
		return ErrChannelFrozen
	}

	delete(wt.monitoredChannels, channelID)
	wt.stats.ChannelsMonitored--

	wt.sendAlert(AlertTypeStateChange, AlertLevelInfo, channelID,
		"Channel monitoring stopped",
		map[string]interface{}{
			"monitored_duration": time.Since(mc.MonitorStartTime),
			"alert_count":        mc.AlertCount,
		})

	return nil
}

// GetMonitoredChannel returns the monitored channel state
func (wt *Watchtower) GetMonitoredChannel(channelID [32]byte) (*MonitoredChannel, error) {
	wt.mu.RLock()
	defer wt.mu.RUnlock()

	mc, exists := wt.monitoredChannels[channelID]
	if !exists {
		return nil, ErrChannelNotMonitored
	}

	// Return a copy
	mcCopy := *mc
	return &mcCopy, nil
}

// ListMonitoredChannels returns all monitored channel IDs
func (wt *Watchtower) ListMonitoredChannels() [][32]byte {
	wt.mu.RLock()
	defer wt.mu.RUnlock()

	ids := make([][32]byte, 0, len(wt.monitoredChannels))
	for id := range wt.monitoredChannels {
		ids = append(ids, id)
	}

	return ids
}

// ============================================================================
// State Verification Methods
// ============================================================================

// VerifyState verifies if a state is valid for a monitored channel
func (wt *Watchtower) VerifyState(channelID [32]byte, state *interfaces.SignedState) error {
	wt.mu.RLock()
	mc, exists := wt.monitoredChannels[channelID]
	wt.mu.RUnlock()

	if !exists {
		return ErrChannelNotMonitored
	}

	if mc.IsFrozen {
		return ErrChannelFrozen
	}

	// Verify channel ID matches
	if state.ChannelID != channelID {
		return ErrWatchtowerInvalidState
	}

	// Verify sequence number is not too far ahead
	if state.Sequence > mc.Sequence+wt.config.MaxSequenceDiff {
		return fmt.Errorf("sequence number too far ahead: %d > %d + %d",
			state.Sequence, mc.Sequence, wt.config.MaxSequenceDiff)
	}

	// Verify balance conservation
	total := state.BalanceA + state.BalanceB
	oldTotal := mc.BalanceA + mc.BalanceB
	if total != oldTotal {
		return fmt.Errorf("balance conservation violated: %d != %d", total, oldTotal)
	}

	// Verify at least one signature
	if len(state.SigA) == 0 && len(state.SigB) == 0 {
		return errors.New("no signatures provided")
	}

	// Verify signatures if provided
	channel, err := wt.manager.GetChannelState(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}

	stateData := serializeState(state)

	if len(state.SigA) > 0 {
		if len(state.SigA) != ed25519.SignatureSize {
			return errors.New("invalid signature A length")
		}
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyA[:]), stateData, state.SigA) {
			return errors.New("invalid signature A")
		}
	}

	if len(state.SigB) > 0 {
		if len(state.SigB) != ed25519.SignatureSize {
			return errors.New("invalid signature B length")
		}
		if !ed25519.Verify(ed25519.PublicKey(channel.PartyB[:]), stateData, state.SigB) {
			return errors.New("invalid signature B")
		}
	}

	// Update monitored channel state if verification succeeds
	if state.Sequence > mc.Sequence {
		wt.mu.Lock()
		mc.Sequence = state.Sequence
		mc.BalanceA = state.BalanceA
		mc.BalanceB = state.BalanceB
		// Store state hash in KnownStates
		stateHash := sha256.Sum256(stateData)
		mc.KnownStates[state.Sequence] = stateHash
		mc.LatestSignedState = state
		wt.mu.Unlock()
	}

	wt.stats.StateVerifications++

	return nil
}

// ============================================================================
// Fraud Detection Methods
// ============================================================================

// DetectFraud detects fraudulent behavior in a channel
func (wt *Watchtower) DetectFraud(channelID [32]byte, state *interfaces.SignedState, initiator interfaces.Address) *FraudEvidence {
	wt.mu.RLock()
	mc, exists := wt.monitoredChannels[channelID]
	wt.mu.RUnlock()

	if !exists {
		return nil
	}

	var evidence *FraudEvidence
	now := time.Now()

	// Check for sequence rollback
	if state.Sequence < mc.Sequence {
		evidence = &FraudEvidence{
			ChannelID:   channelID,
			FraudType:   FraudTypeSequenceRollback,
			Sequence:    state.Sequence,
			Attacker:    initiator,
			Timestamp:   now,
			Description: fmt.Sprintf("Sequence rollback detected: %d -> %d", mc.Sequence, state.Sequence),
		}
		wt.handleFraudDetected(evidence)
		return evidence
	}

	// Check for double close attempt
	for _, attempt := range mc.CloseAttempts {
		if attempt.Sequence == state.Sequence && attempt.IsOnChain {
			evidence = &FraudEvidence{
				ChannelID:   channelID,
				FraudType:   FraudTypeDoubleClose,
				Sequence:    state.Sequence,
				Attacker:    initiator,
				Timestamp:   now,
				Description: fmt.Sprintf("Double close attempt detected for sequence %d", state.Sequence),
			}
			wt.handleFraudDetected(evidence)
			return evidence
		}
	}

	// Check for invalid state (older than latest known)
	if state.Sequence < mc.Sequence {
		evidence = &FraudEvidence{
			ChannelID:    channelID,
			FraudType:    FraudTypeInvalidClose,
			Sequence:     state.Sequence,
			InvalidState: state,
			Attacker:     initiator,
			Timestamp:    now,
			Description:  fmt.Sprintf("Invalid close: state sequence %d is older than latest %d", state.Sequence, mc.Sequence),
		}
		wt.handleFraudDetected(evidence)
		return evidence
	}

	// Check for balance manipulation
	oldTotal := mc.BalanceA + mc.BalanceB
	newTotal := state.BalanceA + state.BalanceB
	if newTotal != oldTotal {
		evidence = &FraudEvidence{
			ChannelID:    channelID,
			FraudType:    FraudTypeBalanceManipulation,
			Sequence:     state.Sequence,
			InvalidState: state,
			Attacker:     initiator,
			Timestamp:    now,
			Description:  fmt.Sprintf("Balance manipulation detected: %d -> %d", oldTotal, newTotal),
		}
		wt.handleFraudDetected(evidence)
		return evidence
	}

	// Record the close attempt
	wt.mu.Lock()
	if mc.CloseAttempts == nil {
		mc.CloseAttempts = make([]CloseAttempt, 0)
	}
	mc.CloseAttempts = append(mc.CloseAttempts, CloseAttempt{
		Sequence:  state.Sequence,
		BalanceA:  state.BalanceA,
		BalanceB:  state.BalanceB,
		Initiator: initiator,
		Timestamp: now,
		IsOnChain: true, // Assuming it's on chain since we're detecting fraud
	})
	wt.mu.Unlock()

	return nil
}

// CheckStateTransition verifies if a state transition is valid
func (wt *Watchtower) CheckStateTransition(channelID [32]byte, oldState, newState *interfaces.SignedState) error {
	if oldState == nil {
		return nil // No previous state to check
	}

	// Verify sequence incremented
	if newState.Sequence <= oldState.Sequence {
		return fmt.Errorf("sequence must increment: %d <= %d", newState.Sequence, oldState.Sequence)
	}

	// Verify balance conservation
	oldTotal := oldState.BalanceA + oldState.BalanceB
	newTotal := newState.BalanceA + newState.BalanceB
	if newTotal != oldTotal {
		return fmt.Errorf("balance conservation violated: %d != %d", newTotal, oldTotal)
	}

	return nil
}

// ============================================================================
// Punishment Methods
// ============================================================================

// TriggerPunishment triggers punishment for fraudulent behavior
func (wt *Watchtower) TriggerPunishment(evidence *FraudEvidence) (*PunishmentResult, error) {
	if !wt.config.PenaltyEnabled {
		return nil, errors.New("punishment is disabled")
	}

	if wt.disputeResolver == nil {
		return nil, errors.New("dispute resolver not configured")
	}

	// Determine the honest party
	honestParty := evidence.Victim
	if honestParty == (interfaces.Address{}) {
		// If victim is not set, determine based on the valid state
		if evidence.ValidState != nil {
			if len(evidence.ValidState.SigA) > 0 {
				// Party A signed the valid state
				channel, _ := wt.manager.GetChannelState(evidence.ChannelID)
				if channel != nil {
					honestParty = channel.PartyA
				}
			} else if len(evidence.ValidState.SigB) > 0 {
				channel, _ := wt.manager.GetChannelState(evidence.ChannelID)
				if channel != nil {
					honestParty = channel.PartyB
				}
			}
		}
	}

	// Create punishment task
	task := &PunishmentTask{
		ChannelID:   evidence.ChannelID,
		FraudType:   evidence.FraudType,
		Evidence:    evidence,
		HonestParty: honestParty,
		Result:      make(chan *PunishmentResult, 1),
	}

	// Send to punishment channel
	select {
	case wt.punishChan <- task:
		// Wait for result
		select {
		case result := <-task.Result:
			return result, nil
		case <-time.After(wt.config.ChallengePeriod):
			return nil, ErrPunishmentFailed
		}
	default:
		return nil, ErrAlertChannelFull
	}
}

// FreezeChannel freezes a monitored channel
func (wt *Watchtower) FreezeChannel(channelID [32]byte, reason string) error {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	mc, exists := wt.monitoredChannels[channelID]
	if !exists {
		return ErrChannelNotMonitored
	}

	if mc.IsFrozen {
		return nil // Already frozen
	}

	now := time.Now()
	mc.IsFrozen = true
	mc.FrozenAt = &now
	mc.FrozenBy = wt.config.PenaltyRecipient

	wt.sendAlert(AlertTypeChannelFrozen, AlertLevelEmergency, channelID,
		fmt.Sprintf("Channel frozen: %s", reason),
		map[string]interface{}{
			"frozen_at": now,
			"frozen_by": fmt.Sprintf("%x", mc.FrozenBy[:]),
			"reason":    reason,
			"balance_a": mc.BalanceA,
			"balance_b": mc.BalanceB,
		})

	// Freeze the channel in the manager
	channel, err := wt.manager.GetChannelState(channelID)
	if err == nil {
		_ = channel // Would freeze in a full implementation
	}

	return nil
}

// UnfreezeChannel unfreezes a monitored channel
func (wt *Watchtower) UnfreezeChannel(channelID [32]byte) error {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	mc, exists := wt.monitoredChannels[channelID]
	if !exists {
		return ErrChannelNotMonitored
	}

	if !mc.IsFrozen {
		return nil // Not frozen
	}

	mc.IsFrozen = false
	mc.FrozenAt = nil

	wt.sendAlert(AlertTypeChannelFrozen, AlertLevelWarning, channelID,
		"Channel unfrozen",
		map[string]interface{}{
			"channel_id": channelID,
		})

	return nil
}

// ============================================================================
// Alert Methods
// ============================================================================

// SendAlert sends an alert
func (wt *Watchtower) SendAlert(alertType AlertType, level AlertLevel, channelID [32]byte, message string, details map[string]interface{}) {
	wt.sendAlert(alertType, level, channelID, message, details)
}

// GetAlertChannel returns the alert channel
func (wt *Watchtower) GetAlertChannel() <-chan *Alert {
	return wt.alertChan
}

// sendAlert sends an alert to the alert channel
func (wt *Watchtower) sendAlert(alertType AlertType, level AlertLevel, channelID [32]byte, message string, details map[string]interface{}) {
	alert := &Alert{
		ID:        generateAlertID(channelID, time.Now()),
		Type:      alertType,
		Level:     level,
		ChannelID: channelID,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}

	select {
	case wt.alertChan <- alert:
		wt.stats.AlertsGenerated++
	default:
		// Channel full, log error
	}

	// Call callback if set
	if wt.onAlert != nil {
		wt.onAlert(alert)
	}
}

// handleFraudDetected handles detected fraud
func (wt *Watchtower) handleFraudDetected(evidence *FraudEvidence) {
	wt.stats.FraudDetected++

	// Send alert
	wt.sendAlert(AlertTypeFraudAttempt, AlertLevelCritical, evidence.ChannelID,
		fmt.Sprintf("Fraud detected: %s", evidence.Description),
		map[string]interface{}{
			"fraud_type":  evidence.FraudType.String(),
			"sequence":    evidence.Sequence,
			"attacker":    fmt.Sprintf("%x", evidence.Attacker[:]),
			"timestamp":   evidence.Timestamp,
			"description": evidence.Description,
		})

	// Send to fraud channel
	select {
	case wt.fraudChan <- evidence:
	default:
	}

	// Call callback if set
	if wt.onFraudDetected != nil {
		wt.onFraudDetected(evidence)
	}

	// Auto-punish if enabled
	if wt.config.PenaltyEnabled {
		go wt.TriggerPunishment(evidence)
	}
}

// SetAlertCallback sets the alert callback
func (wt *Watchtower) SetAlertCallback(callback func(*Alert)) {
	wt.onAlert = callback
}

// SetFraudDetectionCallback sets the fraud detection callback
func (wt *Watchtower) SetFraudDetectionCallback(callback func(*FraudEvidence)) {
	wt.onFraudDetected = callback
}

// SetPunishmentCallback sets the punishment callback
func (wt *Watchtower) SetPunishmentCallback(callback func(*PunishmentResult)) {
	wt.onPunishment = callback
}

// ============================================================================
// Background Loops
// ============================================================================

// monitoringLoop is the main monitoring loop
func (wt *Watchtower) monitoringLoop() {
	defer wt.wg.Done()

	ticker := time.NewTicker(wt.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wt.ctx.Done():
			return
		case <-ticker.C:
			wt.checkMonitoredChannels()
		}
	}
}

// checkMonitoredChannels checks all monitored channels for anomalies
func (wt *Watchtower) checkMonitoredChannels() {
	wt.mu.RLock()
	channelIDs := make([][32]byte, 0, len(wt.monitoredChannels))
	for id := range wt.monitoredChannels {
		channelIDs = append(channelIDs, id)
	}
	wt.mu.RUnlock()

	for _, channelID := range channelIDs {
		wt.checkChannel(channelID)
	}
}

// checkChannel checks a single channel for anomalies
func (wt *Watchtower) checkChannel(channelID [32]byte) {
	wt.mu.RLock()
	mc, exists := wt.monitoredChannels[channelID]
	wt.mu.RUnlock()

	if !exists {
		return
	}

	// Get latest channel state from manager
	channel, err := wt.manager.GetChannelState(channelID)
	if err != nil {
		wt.sendAlert(AlertTypeChannelAnomaly, AlertLevelWarning, channelID,
			fmt.Sprintf("Failed to get channel state: %v", err),
			map[string]interface{}{"error": err.Error()})
		return
	}

	// Check if channel status changed
	if channel.Sequence != mc.Sequence {
		// Update monitored state
		wt.mu.Lock()
		mc.Sequence = channel.Sequence
		mc.BalanceA = channel.BalanceA
		mc.BalanceB = channel.BalanceB
		mc.StateHash = channel.StateHash
		mc.LastUpdate = time.Now()
		mc.LastChecked = time.Now()

		// Record new state
		mc.KnownStates[channel.Sequence] = channel.StateHash
		wt.mu.Unlock()

		wt.sendAlert(AlertTypeStateChange, AlertLevelInfo, channelID,
			fmt.Sprintf("Channel state updated: sequence %d", channel.Sequence),
			map[string]interface{}{
				"sequence":  channel.Sequence,
				"balance_a": channel.BalanceA,
				"balance_b": channel.BalanceB,
			})
	}

	// Check for stale state
	if time.Since(mc.LastUpdate) > wt.config.AlertThresholds.StateStaleness {
		wt.sendAlert(AlertTypeChannelAnomaly, AlertLevelWarning, channelID,
			"Channel state is stale",
			map[string]interface{}{
				"last_update": mc.LastUpdate,
				"staleness":   time.Since(mc.LastUpdate),
			})
	}

	// Check channel status from manager
	status, err := wt.manager.GetChannelStatus(channelID)
	if err == nil {
		if status == StateClosed || status == StateInDispute {
			// Channel is closed or in dispute, stop monitoring
			wt.mu.Lock()
			delete(wt.monitoredChannels, channelID)
			wt.mu.Unlock()

			wt.sendAlert(AlertTypeStateChange, AlertLevelInfo, channelID,
				fmt.Sprintf("Channel closed or in dispute (status: %d)", status),
				map[string]interface{}{"status": status})
		}
	}
}

// alertProcessingLoop processes alerts
func (wt *Watchtower) alertProcessingLoop() {
	defer wt.wg.Done()

	for {
		select {
		case <-wt.ctx.Done():
			return
		case alert := <-wt.alertChan:
			wt.processAlert(alert)
		}
	}
}

// processAlert processes a single alert
func (wt *Watchtower) processAlert(alert *Alert) {
	// Update alert count for channel
	if alert.ChannelID != [32]byte{} {
		wt.mu.Lock()
		if mc, exists := wt.monitoredChannels[alert.ChannelID]; exists {
			mc.AlertCount++
		}
		wt.mu.Unlock()
	}

	// Log alert
	// In a real implementation, this would send to a logging system
	_ = alert
}

// fraudDetectionLoop handles fraud detection
func (wt *Watchtower) fraudDetectionLoop() {
	defer wt.wg.Done()

	for {
		select {
		case <-wt.ctx.Done():
			return
		case evidence := <-wt.fraudChan:
			wt.processFraudEvidence(evidence)
		}
	}
}

// processFraudEvidence processes fraud evidence
func (wt *Watchtower) processFraudEvidence(evidence *FraudEvidence) {
	// Freeze the channel
	wt.FreezeChannel(evidence.ChannelID, evidence.Description)

	// Trigger punishment
	if wt.config.PenaltyEnabled && wt.disputeResolver != nil {
		result, err := wt.TriggerPunishment(evidence)
		if err != nil {
			wt.sendAlert(AlertTypePunishmentTriggered, AlertLevelCritical, evidence.ChannelID,
				fmt.Sprintf("Punishment failed: %v", err),
				map[string]interface{}{"error": err.Error()})
		}
		if result != nil && wt.onPunishment != nil {
			wt.onPunishment(result)
		}
	}
}

// punishmentLoop handles punishment execution
func (wt *Watchtower) punishmentLoop() {
	defer wt.wg.Done()

	for {
		select {
		case <-wt.ctx.Done():
			return
		case task := <-wt.punishChan:
			wt.executePunishment(task)
		}
	}
}

// executePunishment executes a punishment task
func (wt *Watchtower) executePunishment(task *PunishmentTask) {
	result := &PunishmentResult{
		ChannelID: task.ChannelID,
		Success:   false,
		Timestamp: time.Now(),
	}

	// Get channel
	channel, err := wt.manager.GetChannelState(task.ChannelID)
	if err != nil {
		result.Reason = fmt.Sprintf("Failed to get channel: %v", err)
		task.Result <- result
		return
	}

	// Apply penalty - all funds go to honest party
	var penaltyAmount uint64

	if task.HonestParty == channel.PartyA {
		// Party B is the attacker, all funds go to Party A
		penaltyAmount = channel.BalanceB
		channel.BalanceA = channel.BalanceA + channel.BalanceB
		channel.BalanceB = 0
		result.NewBalanceA = channel.BalanceA
		result.NewBalanceB = 0
	} else if task.HonestParty == channel.PartyB {
		// Party A is the attacker, all funds go to Party B
		penaltyAmount = channel.BalanceA
		channel.BalanceB = channel.BalanceB + channel.BalanceA
		channel.BalanceA = 0
		result.NewBalanceA = 0
		result.NewBalanceB = channel.BalanceB
	} else {
		result.Reason = "Cannot determine honest party"
		task.Result <- result
		return
	}

	result.PenaltyAmount = uint64(float64(penaltyAmount) * wt.config.FraudPenaltyMultiplier)
	result.Success = true
	result.Reason = fmt.Sprintf("Fraud detected: %s", task.FraudType.String())

	// Close the channel
	if err := wt.manager.CloseChannel(wt.ctx, channel, interfaces.SignedState{
		ChannelID: task.ChannelID,
		Sequence:  channel.Sequence,
		BalanceA:  result.NewBalanceA,
		BalanceB:  result.NewBalanceB,
		Timestamp: time.Now(),
	}); err != nil {
		result.Success = false
		result.Reason = fmt.Sprintf("Failed to close channel: %v", err)
	}

	// Update stats
	wt.stats.PunishmentsExecuted++

	// Send alert
	wt.sendAlert(AlertTypePunishmentTriggered, AlertLevelCritical, task.ChannelID,
		fmt.Sprintf("Punishment executed: %s", result.Reason),
		map[string]interface{}{
			"penalty_amount": result.PenaltyAmount,
			"new_balance_a":  result.NewBalanceA,
			"new_balance_b":  result.NewBalanceB,
			"success":        result.Success,
		})

	// Unfreeze channel after punishment
	wt.UnfreezeChannel(task.ChannelID)

	task.Result <- result
}

// statsUpdateLoop updates statistics
func (wt *Watchtower) statsUpdateLoop() {
	defer wt.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-wt.ctx.Done():
			return
		case <-ticker.C:
			wt.mu.Lock()
			wt.stats.LastUpdate = time.Now()
			wt.mu.Unlock()
		}
	}
}

// ============================================================================
// Statistics Methods
// ============================================================================

// GetStats returns the Watchtower statistics
func (wt *Watchtower) GetStats() WatchtowerStats {
	wt.mu.RLock()
	defer wt.mu.RUnlock()

	stats := wt.stats
	stats.ChannelsMonitored = int64(len(wt.monitoredChannels))
	return stats
}

// ============================================================================
// Utility Functions
// ============================================================================

// generateAlertID generates a unique alert ID
func generateAlertID(channelID [32]byte, timestamp time.Time) string {
	h := sha256.New()
	h.Write(channelID[:])
	h.Write([]byte(timestamp.String()))

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(timestamp.UnixNano()))
	h.Write(buf[:])

	return fmt.Sprintf("alert-%x", h.Sum(nil)[:16])
}

// ChannelStatusToString converts channel status to string
func ChannelStatusToString(status int) string {
	switch status {
	case StateOpen:
		return "OPEN"
	case StateClosing:
		return "CLOSING"
	case StateClosed:
		return "CLOSED"
	case StateInDispute:
		return "IN_DISPUTE"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// Additional Fraud Detection Methods
// ============================================================================

// DetectStateReversion detects state reversion fraud
func (wt *Watchtower) DetectStateReversion(channelID [32]byte, newState *interfaces.SignedState) *FraudEvidence {
	wt.mu.RLock()
	mc, exists := wt.monitoredChannels[channelID]
	wt.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check if new state sequence is less than the highest known sequence
	highestSeq := mc.Sequence
	for seq := range mc.KnownStates {
		if seq > highestSeq {
			highestSeq = seq
		}
	}

	if newState.Sequence < highestSeq {
		return &FraudEvidence{
			ChannelID:   channelID,
			FraudType:   FraudTypeStateReversion,
			Sequence:    newState.Sequence,
			Timestamp:   time.Now(),
			Description: fmt.Sprintf("State reversion detected: tried to revert from sequence %d to %d", highestSeq, newState.Sequence),
		}
	}

	return nil
}

// DetectUnauthorizedClose detects unauthorized close attempts
func (wt *Watchtower) DetectUnauthorizedClose(channelID [32]byte, state *interfaces.SignedState, closer interfaces.Address) *FraudEvidence {
	wt.mu.RLock()
	mc, exists := wt.monitoredChannels[channelID]
	wt.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check if closer is one of the channel parties
	if closer == mc.PartyA || closer == mc.PartyB {
		return nil // Authorized party
	}

	// Check if both parties have signed
	hasBothSignatures := len(state.SigA) > 0 && len(state.SigB) > 0
	if hasBothSignatures {
		return nil // Both parties agreed to close
	}

	return &FraudEvidence{
		ChannelID:   channelID,
		FraudType:   FraudTypeUnauthorizedClose,
		Sequence:    state.Sequence,
		Attacker:    closer,
		Timestamp:   time.Now(),
		Description: fmt.Sprintf("Unauthorized close attempt by %v", closer),
	}
}

// ============================================================================
// Health Check Methods
// ============================================================================

// IsHealthy returns true if the Watchtower is healthy
func (wt *Watchtower) IsHealthy() bool {
	// Check if monitoring loop is running
	select {
	case <-wt.ctx.Done():
		return false
	default:
	}

	// Check if alert channel is not full
	select {
	case wt.alertChan <- &Alert{}:
		<-wt.alertChan
		return true
	default:
		return false
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

// UpdateMonitoredChannelSequence updates the sequence number for testing purposes.
// This is intended for use in tests to simulate valid state updates.
func (wt *Watchtower) UpdateMonitoredChannelSequence(channelID [32]byte, sequence uint64) error {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	mc, exists := wt.monitoredChannels[channelID]
	if !exists {
		return ErrChannelNotMonitored
	}

	mc.Sequence = sequence
	return nil
}

// ============================================================================
// Serialization Helpers
// ============================================================================

// addressToHex converts an address to hex string
func addressToHex(a interfaces.Address) string {
	return fmt.Sprintf("%x", a[:])
}
