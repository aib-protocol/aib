package slashing

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ViolationType represents the type of violation
type ViolationType string

// Violation types
const (
	FraudProof   ViolationType = "fraud_proof"    // 伪造证明
	SybilAttack  ViolationType = "sybil_attack"   // 女巫攻击
	CopyResult   ViolationType = "copy_result"    // 抄袭结果
	NoWork       ViolationType = "no_work"        // 没干活
	Misbehavior  ViolationType = "misbehavior"    // 恶意行为
)

// SlashConfig defines the slash ratios for different violations
type SlashConfig struct {
	FraudProof          float64 // 100% 质押
	SybilAttack         float64 // 100% 质押
	CopyResult          float64 // 50% 质押
	NoWork              float64 // 25% 质押
	Misbehavior         float64 // 可配置
	ReporterRewardRatio float64 // 举报奖励比例 (默认 0.2 = 20%)
}

// DefaultSlashConfig returns the default slash configuration
func DefaultSlashConfig() *SlashConfig {
	return &SlashConfig{
		FraudProof:          1.0,  // 100%
		SybilAttack:         1.0,  // 100%
		CopyResult:          0.5,  // 50%
		NoWork:              0.25, // 25%
		Misbehavior:         0.1,  // 10%
		ReporterRewardRatio: 0.2,  // 20% 举报奖励
	}
}

// Evidence represents evidence of a violation
type Evidence struct {
	ID            string        // Unique evidence ID
	Type          ViolationType // Type of violation
	Offender      []byte        // Node ID of the offender
	Reporter      []byte        // Node ID of the reporter
	Timestamp     int64         // When the violation was detected
	Description   string        // Human-readable description
	ProofData     []byte        // Cryptographic proof
	TaskID        string        // Associated task ID (if applicable)
	Severity      int           // Severity level (1-10)
	Witnesses     [][]byte      // Additional witnesses
}

// SlashEvent represents a slash event
type SlashEvent struct {
	ID             string        // Unique slash event ID
	Offender       []byte        // Node ID of the offender
	Amount         float64       // Amount slashed
	Reason         ViolationType // Reason for slash
	Timestamp      int64         // When the slash occurred
	Evidence       *Evidence     // Associated evidence
	BlockHeight    int64         // Block height when slash occurred
	TransactionID  string        // Transaction ID of the slash
	Reporter       []byte        // Node ID of the reporter who submitted evidence
	ReporterReward float64       // Reward amount for the reporter
}

// SlashEngine handles slash detection and execution
type SlashEngine struct {
	mu              sync.RWMutex
	config          *SlashConfig
	evidencePool    map[string]*Evidence
	slashHistory    map[string][]*SlashEvent
	bannedNodes     map[string]struct{}
	reporterRewards map[string]float64 // nodeID -> total rewards
	appealWindow    time.Duration
	maxSlashCount   int
}

// NewSlashEngine creates a new slash engine
func NewSlashEngine(config *SlashConfig) *SlashEngine {
	if config == nil {
		config = DefaultSlashConfig()
	}
	return &SlashEngine{
		config:          config,
		evidencePool:    make(map[string]*Evidence),
		slashHistory:    make(map[string][]*SlashEvent),
		bannedNodes:     make(map[string]struct{}),
		reporterRewards: make(map[string]float64),
		appealWindow:    24 * time.Hour, // 24 hours to appeal
		maxSlashCount:   3,              // Ban after 3 slashes
	}
}

// determineSeverity calculates the severity based on violation type and evidence
// (protocol-determined, not reporter-set, to prevent manipulation)
func determineSeverity(evidence *Evidence) int {
	baseSeverity := map[ViolationType]int{
		FraudProof:  9,  // Almost always critical
		SybilAttack: 9,  // Almost always critical
		CopyResult:  5,  // Medium severity
		NoWork:      3,  // Low severity
		Misbehavior: 4,  // Low-medium severity
	}

	severity, ok := baseSeverity[evidence.Type]
	if !ok {
		severity = 5 // Default medium
	}

	// Adjust based on witness count (more witnesses = higher confidence)
	if len(evidence.Witnesses) >= 3 {
		severity = severity + 1
		if severity > 10 {
			severity = 10
		}
	}

	return severity
}

// SubmitEvidence submits evidence of a violation
func (e *SlashEngine) SubmitEvidence(evidence *Evidence) error {
	if evidence == nil {
		return errors.New("slash: nil evidence")
	}
	if len(evidence.Offender) == 0 {
		return errors.New("slash: empty offender ID")
	}
	if evidence.Type == "" {
		return errors.New("slash: empty violation type")
	}
	if len(evidence.ProofData) == 0 {
		return errors.New("slash: empty proof data")
	}

	// Override reporter-set severity with protocol-determined severity
	evidence.Severity = determineSeverity(evidence)

	// Verify evidence signature and integrity
	if err := e.verifyEvidence(evidence); err != nil {
		return fmt.Errorf("slash: invalid evidence: %w", err)
	}

	// Check if node is already banned
	if e.IsBanned(evidence.Offender) {
		return errors.New("slash: offender already banned")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Store evidence
	evidenceID := e.generateEvidenceID(evidence)
	evidence.ID = evidenceID
	e.evidencePool[evidenceID] = evidence

	// Auto-slash for severe violations
	if evidence.Severity >= 8 {
		if _, err := e.executeSlash(evidence); err != nil {
			return fmt.Errorf("slash: failed to execute slash: %w", err)
		}
	}

	return nil
}

// ShouldSlash determines if a node should be slashed based on evidence
func (e *SlashEngine) ShouldSlash(evidence *Evidence) (bool, float64) {
	if evidence == nil {
		return false, 0
	}

	// Check evidence validity
	if err := e.verifyEvidence(evidence); err != nil {
		return false, 0
	}

	// Determine slash ratio based on violation type
	var slashRatio float64
	switch evidence.Type {
	case FraudProof:
		slashRatio = e.config.FraudProof
	case SybilAttack:
		slashRatio = e.config.SybilAttack
	case CopyResult:
		slashRatio = e.config.CopyResult
	case NoWork:
		slashRatio = e.config.NoWork
	case Misbehavior:
		slashRatio = e.config.Misbehavior
	default:
		return false, 0
	}

	// Adjust based on severity
	if evidence.Severity >= 9 {
		slashRatio = 1.0 // Maximum slash for critical violations
	}

	return true, slashRatio
}

// ExecuteSlash executes a slash against a node
func (e *SlashEngine) ExecuteSlash(evidence *Evidence) (*SlashEvent, error) {
	if evidence == nil {
		return nil, errors.New("slash: nil evidence")
	}

	shouldSlash, _ := e.ShouldSlash(evidence)
	if !shouldSlash {
		return nil, errors.New("slash: evidence does not warrant slash")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.executeSlash(evidence)
}

// executeSlash is the internal slash execution (must be called with lock held)
func (e *SlashEngine) executeSlash(evidence *Evidence) (*SlashEvent, error) {
	offenderStr := string(evidence.Offender)

	// Get current slash count
	slashCount := len(e.slashHistory[offenderStr])

	// Check if already at max slashes
	if slashCount >= e.maxSlashCount {
		e.banNode(evidence.Offender)
		return nil, errors.New("slash: node already at max slash count")
	}

	// Calculate slash amount (this would normally query stake)
	// For now, use placeholder stake amount
	stakeAmount := 1000.0 // Placeholder stake
	slashAmount := stakeAmount * e.getSlashRatio(evidence.Type)

	// Calculate reporter reward
	reporterReward := slashAmount * e.config.ReporterRewardRatio

	// Create slash event
	event := &SlashEvent{
		ID:             e.generateSlashEventID(evidence),
		Offender:       evidence.Offender,
		Amount:         slashAmount,
		Reason:         evidence.Type,
		Timestamp:      time.Now().Unix(),
		Evidence:       evidence,
		BlockHeight:    0,  // Would be set by blockchain
		TransactionID:  "", // Would be set by blockchain
		Reporter:       evidence.Reporter,
		ReporterReward: reporterReward,
	}

	// Record reporter reward
	if len(evidence.Reporter) > 0 {
		reporterStr := string(evidence.Reporter)
		e.reporterRewards[reporterStr] += reporterReward
	}

	// Record slash
	if e.slashHistory[offenderStr] == nil {
		e.slashHistory[offenderStr] = make([]*SlashEvent, 0)
	}
	e.slashHistory[offenderStr] = append(e.slashHistory[offenderStr], event)

	// Check if should ban after this slash
	if len(e.slashHistory[offenderStr]) >= e.maxSlashCount {
		e.banNode(evidence.Offender)
	}

	return event, nil
}

// getSlashRatio returns the slash ratio for a violation type
func (e *SlashEngine) getSlashRatio(violationType ViolationType) float64 {
	switch violationType {
	case FraudProof:
		return e.config.FraudProof
	case SybilAttack:
		return e.config.SybilAttack
	case CopyResult:
		return e.config.CopyResult
	case NoWork:
		return e.config.NoWork
	case Misbehavior:
		return e.config.Misbehavior
	default:
		return 0
	}
}

// banNode bans a node permanently
func (e *SlashEngine) banNode(nodeID []byte) {
	e.bannedNodes[string(nodeID)] = struct{}{}
}

// IsBanned checks if a node is banned
func (e *SlashEngine) IsBanned(nodeID []byte) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, banned := e.bannedNodes[string(nodeID)]
	return banned
}

// GetSlashHistory returns the slash history for a node
func (e *SlashEngine) GetSlashHistory(nodeID []byte) []*SlashEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.slashHistory[string(nodeID)]
}

// GetReporterReward returns the total reporter rewards for a node
func (e *SlashEngine) GetReporterReward(nodeID []byte) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.reporterRewards[string(nodeID)]
}

// GetTotalSlashed returns the total amount slashed for a node
func (e *SlashEngine) GetTotalSlashed(nodeID []byte) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	history := e.slashHistory[string(nodeID)]
	total := 0.0
	for _, event := range history {
		total += event.Amount
	}
	return total
}

// CanAppeal checks if a slash event can be appealed
func (e *SlashEngine) CanAppeal(event *SlashEvent) bool {
	if event == nil {
		return false
	}
	// Can appeal within 24 hours
	return time.Now().Unix() < event.Timestamp+int64(e.appealWindow.Seconds())
}

// Appeal submits an appeal for a slash event
func (e *SlashEngine) Appeal(eventID string, appealEvidence *Evidence) error {
	if appealEvidence == nil {
		return errors.New("slash: nil appeal evidence")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the slash event
	var event *SlashEvent
	for _, events := range e.slashHistory {
		for _, ev := range events {
			if ev.ID == eventID {
				event = ev
				break
			}
		}
		if event != nil {
			break
		}
	}

	if event == nil {
		return errors.New("slash: event not found")
	}

	// Check if can appeal
	if !e.CanAppeal(event) {
		return errors.New("slash: appeal window closed")
	}

	// Verify appeal evidence
	if err := e.verifyEvidence(appealEvidence); err != nil {
		return fmt.Errorf("slash: invalid appeal evidence: %w", err)
	}

	// Process appeal (in production, this would go to governance)
	// For now, just mark as appealed
	event.Evidence = appealEvidence

	return nil
}

// verifyEvidence verifies the integrity of evidence
func (e *SlashEngine) verifyEvidence(evidence *Evidence) error {
	if evidence.Timestamp > time.Now().Unix()+300 {
		return errors.New("evidence timestamp in future")
	}

	// Verify proof data is not empty
	if len(evidence.ProofData) == 0 {
		return errors.New("empty proof data")
	}

	// In production, verify cryptographic signatures
	// For now, just check basic validity

	return nil
}

// generateEvidenceID generates a unique ID for evidence
func (e *SlashEngine) generateEvidenceID(evidence *Evidence) string {
	data := fmt.Sprintf("%s:%s:%d:%x",
		evidence.Type,
		evidence.Offender,
		evidence.Timestamp,
		evidence.ProofData[:min(32, len(evidence.ProofData))])
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

// generateSlashEventID generates a unique ID for a slash event
func (e *SlashEngine) generateSlashEventID(evidence *Evidence) string {
	data := fmt.Sprintf("slash:%s:%d:%s",
		evidence.Offender,
		time.Now().Unix(),
		evidence.ID)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

// GetStatistics returns slash statistics
func (e *SlashEngine) GetStatistics() *SlashStatistics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := &SlashStatistics{
		TotalSlashes:    0,
		TotalAmount:     0,
		BannedCount:     len(e.bannedNodes),
		ViolationCounts: make(map[ViolationType]int),
	}

	for _, events := range e.slashHistory {
		stats.TotalSlashes += len(events)
		for _, event := range events {
			stats.TotalAmount += event.Amount
			stats.ViolationCounts[event.Reason]++
			stats.TotalReporterRewards += event.ReporterReward
		}
	}

	return stats
}

// SlashStatistics contains slash statistics
type SlashStatistics struct {
	TotalSlashes         int
	TotalAmount          float64
	BannedCount          int
	ViolationCounts      map[ViolationType]int
	TotalReporterRewards float64
}

// Export exports the slash engine state
func (e *SlashEngine) Export() ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state := struct {
		Config          *SlashConfig
		EvidencePool    map[string]*Evidence
		SlashHistory    map[string][]*SlashEvent
		BannedNodes     []string
		ReporterRewards map[string]float64
	}{
		Config:          e.config,
		EvidencePool:    e.evidencePool,
		SlashHistory:    e.slashHistory,
		BannedNodes:     make([]string, 0, len(e.bannedNodes)),
		ReporterRewards: e.reporterRewards,
	}

	for nodeID := range e.bannedNodes {
		state.BannedNodes = append(state.BannedNodes, nodeID)
	}

	return json.Marshal(state)
}

// Import imports the slash engine state
func (e *SlashEngine) Import(data []byte) error {
	var state struct {
		Config          *SlashConfig
		EvidencePool    map[string]*Evidence
		SlashHistory    map[string][]*SlashEvent
		BannedNodes     []string
		ReporterRewards map[string]float64
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = state.Config
	e.evidencePool = state.EvidencePool
	e.slashHistory = state.SlashHistory
	e.bannedNodes = make(map[string]struct{})
	for _, nodeID := range state.BannedNodes {
		e.bannedNodes[nodeID] = struct{}{}
	}
	e.reporterRewards = state.ReporterRewards
	if e.reporterRewards == nil {
		e.reporterRewards = make(map[string]float64)
	}

	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}