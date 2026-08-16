// Package agentic provides AI service layer with reputation scoring system.
package agentic

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================
// Core Types
// ============================================================

// ReputationScore represents a provider's reputation score.
type ReputationScore struct {
	mu sync.RWMutex

	// Node identity
	NodeID string `json:"node_id"`

	// Multi-dimensional scores (0-100 each)
	UserScore    float64 `json:"user_score"`    // From user votes (30% weight)
	TechScore    float64 `json:"tech_score"`    // Technical metrics (25% weight)
	HistoryScore float64 `json:"history_score"` // Historical performance (25% weight)
	StakeScore   float64 `json:"stake_score"`   // Stake-based score (20% weight)

	// Aggregated score
	TotalScore float64 `json:"total_score"`

	// Score components tracking
	PendingPoints []*ReputationPoint `json:"pending_points"` // Time-locked points
	ActivePoints  []*ReputationPoint `json:"active_points"`  // Matured points

	// Statistics
	TotalTasksCompleted  uint64    `json:"total_tasks_completed"`
	TotalTasksAccepted   uint64    `json:"total_tasks_accepted"`
	TotalTasksRejected   uint64    `json:"total_tasks_rejected"`
	TotalBlocksProduced  uint64    `json:"total_blocks_produced"`
	TotalSlashes         uint64    `json:"total_slashes"`
	AvgResponseTimeMs    float64   `json:"avg_response_time_ms"`
	UptimePercentage     float64   `json:"uptime_percentage"`
	LastActiveTime       time.Time `json:"last_active_time"`

	// Suspicion score (0-100, higher = more suspicious)
	SuspicionScore float64 `json:"suspicion_score"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReputationPoint represents a single reputation change event.
type ReputationPoint struct {
	ID        string          `json:"id"`
	NodeID    string `json:"node_id"`
	Amount    float64         `json:"amount"`     // Can be negative
	Source    ReputationSource `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	MaturesAt time.Time       `json:"matures_at"` // When this point becomes active
	Matured   bool            `json:"matured"`

	// Context
	TaskID       string `json:"task_id,omitempty"`
	VoterID      string `json:"voter_id,omitempty"`
	VoterWeight  float64 `json:"voter_weight,omitempty"`
	Evidence     []byte `json:"evidence,omitempty"`
	BlockHeight  uint64 `json:"block_height,omitempty"`

	// Challenge status
	Challenged   bool      `json:"challenged"`
	ChallengeID  string    `json:"challenge_id,omitempty"`
	FrozenUntil  *time.Time `json:"frozen_until,omitempty"`
}

// ReputationSource represents the source of reputation change.
type ReputationSource int

const (
	ReputationSourceUnknown ReputationSource = iota
	ReputationSourceUserAccept      // User accepted result
	ReputationSourceUserReject      // User rejected result
	ReputationSourceUserSkip        // User skipped provider
	ReputationSourceUserReport      // User reported harmful content
	ReputationSourceTaskComplete    // Task completed
	ReputationSourceBlockProduce    // Block produced
	ReputationSourceVerification    // Participated in verification
	ReputationSourceSlash           // Slashed for misbehavior
	ReputationSourceTimeout         // Timeout penalty
	ReputationSourceChallengeWon    // Won a challenge
	ReputationSourceChallengeLost   // Lost a challenge
	ReputationSourceDecay           // Daily decay
)

func (s ReputationSource) String() string {
	switch s {
	case ReputationSourceUserAccept:
		return "user_accept"
	case ReputationSourceUserReject:
		return "user_reject"
	case ReputationSourceUserSkip:
		return "user_skip"
	case ReputationSourceUserReport:
		return "user_report"
	case ReputationSourceTaskComplete:
		return "task_complete"
	case ReputationSourceBlockProduce:
		return "block_produce"
	case ReputationSourceVerification:
		return "verification"
	case ReputationSourceSlash:
		return "slash"
	case ReputationSourceTimeout:
		return "timeout"
	case ReputationSourceChallengeWon:
		return "challenge_won"
	case ReputationSourceChallengeLost:
		return "challenge_lost"
	case ReputationSourceDecay:
		return "decay"
	default:
		return "unknown"
	}
}

// ReputationChange defines how much reputation changes for each source.
type ReputationChange struct {
	Source   ReputationSource
	Amount   float64
	Maturity time.Duration // How long until the point matures
}

// Default reputation changes
var DefaultReputationChanges = map[ReputationSource]ReputationChange{
	ReputationSourceUserAccept:     {ReputationSourceUserAccept, 5.0, 7 * 24 * time.Hour},
	ReputationSourceUserReject:     {ReputationSourceUserReject, -10.0, 0}, // Immediate penalty
	ReputationSourceUserSkip:       {ReputationSourceUserSkip, -1.0, 0},
	ReputationSourceUserReport:     {ReputationSourceUserReport, -50.0, 0},
	ReputationSourceTaskComplete:   {ReputationSourceTaskComplete, 1.0, 3 * 24 * time.Hour},
	ReputationSourceBlockProduce:   {ReputationSourceBlockProduce, 10.0, 14 * 24 * time.Hour},
	ReputationSourceVerification:   {ReputationSourceVerification, 2.0, 7 * 24 * time.Hour},
	ReputationSourceSlash:          {ReputationSourceSlash, -100.0, 0},
	ReputationSourceTimeout:        {ReputationSourceTimeout, -5.0, 0},
	ReputationSourceChallengeWon:   {ReputationSourceChallengeWon, 20.0, 7 * 24 * time.Hour},
	ReputationSourceChallengeLost:  {ReputationSourceChallengeLost, -30.0, 0},
}

// ============================================================
// Reputation Manager
// ============================================================

// ReputationManager manages reputation scores for all providers.
type ReputationManager struct {
	mu sync.RWMutex

	// Configuration
	config *ReputationConfig

	// Scores by node ID
	scores map[string]*ReputationScore

	// All reputation points (for analysis)
	allPoints []*ReputationPoint

	// Voter tracking (for Sybil detection)
	voterHistory map[string]*VoterHistory

	// Challenge tracking
	challenges map[string]*Challenge

	// Stake manager reference
	stakeManager *StakingManager
}

// ReputationConfig holds configuration for reputation scoring.
type ReputationConfig struct {
	// Score weights (should sum to 1.0)
	UserScoreWeight    float64 `json:"user_score_weight"`
	TechScoreWeight    float64 `json:"tech_score_weight"`
	HistoryScoreWeight float64 `json:"history_score_weight"`
	StakeScoreWeight   float64 `json:"stake_score_weight"`

	// Score bounds
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
	BaseScore float64 `json:"base_score"` // Starting score for new nodes

	// Decay parameters
	DailyDecayRate     float64 `json:"daily_decay_rate"`     // 0.99 = 1% decay per day
	DecayHalflifeBlocks uint64  `json:"decay_halflife_blocks"` // Reduce decay after producing blocks

	// Maturity periods
	DefaultMaturityDays int `json:"default_maturity_days"`

	// Suspicion thresholds
	SuspicionThresholdMedium float64 `json:"suspicion_threshold_medium"` // Limit voting weight
	SuspicionThresholdHigh   float64 `json:"suspicion_threshold_high"`   // Auto-challenge
	SuspicionThresholdMax    float64 `json:"suspicion_threshold_max"`    // Freeze

	// Stake-weighted voting
	MinStakeToVote   uint64  `json:"min_stake_to_vote"`
	VoteWeightExponent float64 `json:"vote_weight_exponent"` // 0.5 = sqrt(stake)
}

// DefaultReputationConfig returns default configuration.
func DefaultReputationConfig() *ReputationConfig {
	return &ReputationConfig{
		UserScoreWeight:    0.30,
		TechScoreWeight:    0.25,
		HistoryScoreWeight: 0.25,
		StakeScoreWeight:   0.20,
		MinScore:           -200.0,
		MaxScore:           500.0,
		BaseScore:          100.0,
		DailyDecayRate:     0.99,
		DecayHalflifeBlocks: 10,
		DefaultMaturityDays: 30,
		SuspicionThresholdMedium: 70.0,
		SuspicionThresholdHigh:   90.0,
		SuspicionThresholdMax:    99.0,
		MinStakeToVote:    10,
		VoteWeightExponent: 0.5,
	}
}

// VoterHistory tracks a voter's behavior for Sybil detection.
type VoterHistory struct {
	VoterID         string    `json:"voter_id"`
	TotalVotes      uint64    `json:"total_votes"`
	VotesByProvider map[string]uint64 `json:"votes_by_provider"` // provider_id -> count
	AcceptRate      float64   `json:"accept_rate"` // How often they accept
	AvgResponseTime float64   `json:"avg_response_time"`
	LastVoteTime    time.Time `json:"last_vote_time"`
	FirstVoteTime   time.Time `json:"first_vote_time"`
	IPFingerprints  map[string]int `json:"ip_fingerprints"`
	DeviceFingerprints map[string]int `json:"device_fingerprints"`

	// Behavioral patterns
	VotePatternScore float64 `json:"vote_pattern_score"` // 0-100, higher = more suspicious
}

// Challenge represents a reputation challenge.
type Challenge struct {
	ID           string    `json:"id"`
	ChallengerID string    `json:"challenger_id"`
	TargetNodeID string    `json:"target_node_id"`
	PointID      string    `json:"point_id"` // The reputation point being challenged
	Reason       string    `json:"reason"`
	Evidence     []byte    `json:"evidence"`
	StakeAmount  uint64    `json:"stake_amount"` // Challenger's stake
	Status       ChallengeStatus `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	Resolution   string    `json:"resolution,omitempty"`
	Votes        map[string]bool `json:"votes"` // arbitrator -> vote
}

// ChallengeStatus represents the status of a challenge.
type ChallengeStatus int

const (
	ChallengeStatusPending ChallengeStatus = iota
	ChallengeStatusVoting
	ChallengeStatusResolvedSuccess
	ChallengeStatusResolvedFailed
	ChallengeStatusExpired
)

// NewReputationManager creates a new reputation manager.
func NewReputationManager(config *ReputationConfig, stakeManager *StakingManager) *ReputationManager {
	if config == nil {
		config = DefaultReputationConfig()
	}
	return &ReputationManager{
		config:       config,
		scores:       make(map[string]*ReputationScore),
		allPoints:    make([]*ReputationPoint, 0),
		voterHistory: make(map[string]*VoterHistory),
		challenges:   make(map[string]*Challenge),
		stakeManager: stakeManager,
	}
}

// ============================================================
// Core Reputation Operations
// ============================================================

// GetOrCreateScore gets or creates a reputation score for a node.
func (rm *ReputationManager) GetOrCreateScore(nodeID string) *ReputationScore {
	nodeKey := nodeID

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if score, exists := rm.scores[nodeKey]; exists {
		return score
	}

	score := &ReputationScore{
		NodeID:            nodeID,
		UserScore:         rm.config.BaseScore,
		TechScore:         rm.config.BaseScore,
		HistoryScore:      rm.config.BaseScore,
		StakeScore:        rm.config.BaseScore,
		TotalScore:        rm.config.BaseScore,
		PendingPoints:     make([]*ReputationPoint, 0),
		ActivePoints:      make([]*ReputationPoint, 0),
		UptimePercentage:  100.0, // Start with perfect uptime
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	rm.scores[nodeKey] = score
	return score
}

// GetScore gets the reputation score for a node.
func (rm *ReputationManager) GetScore(nodeID string) (*ReputationScore, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	nodeKey := nodeID
	if score, exists := rm.scores[nodeKey]; exists {
		return score, nil
	}
	return nil, fmt.Errorf("reputation: node not found: %s", nodeID)
}

// AddReputation adds reputation to a node from a specific source.
func (rm *ReputationManager) AddReputation(
	nodeID string,
	source ReputationSource,
	voterID string,
	voterStake uint64,
	context *ReputationContext,
) (*ReputationPoint, error) {

	// Get the default change for this source
	change, exists := DefaultReputationChanges[source]
	if !exists {
		return nil, fmt.Errorf("reputation: unknown source: %v", source)
	}

	// Calculate voter weight (stake-weighted voting)
	voterWeight := rm.calculateVoterWeight(voterStake)

	// Apply voter weight to the amount
	amount := change.Amount * voterWeight

	// Calculate maturity time
	var maturesAt time.Time
	if change.Maturity > 0 {
		maturesAt = time.Now().Add(change.Maturity)
	} else {
		maturesAt = time.Now() // Immediate
	}

	// Create the reputation point
	point := &ReputationPoint{
		ID:          generatePointID(nodeID, source, time.Now()),
		NodeID:      nodeID,
		Amount:      amount,
		Source:      source,
		Timestamp:   time.Now(),
		MaturesAt:   maturesAt,
		Matured:     change.Maturity == 0,
		VoterID:     voterID,
		VoterWeight: voterWeight,
	}

	if context != nil {
		point.TaskID = context.TaskID
		point.BlockHeight = context.BlockHeight
	}

	// Add to the node's score
	score := rm.GetOrCreateScore(nodeID)
	score.mu.Lock()
	defer score.mu.Unlock()

	if point.Matured {
		score.ActivePoints = append(score.ActivePoints, point)
	} else {
		score.PendingPoints = append(score.PendingPoints, point)
	}

	// Update statistics based on source
	rm.updateStatistics(score, source, context)

	// Update suspicion score
	rm.updateSuspicionScore(score, voterID, source)

	// Recalculate total score
	rm.recalculateScore(score)

	// Store point globally
	rm.mu.Lock()
	rm.allPoints = append(rm.allPoints, point)
	rm.mu.Unlock()

	// Update voter history
	rm.updateVoterHistory(voterID, nodeID, source, context)

	score.UpdatedAt = time.Now()

	return point, nil
}

// ReputationContext provides context for a reputation change.
type ReputationContext struct {
	TaskID         string
	BlockHeight    uint64
	ResponseTimeMs float64
	WasTimeout     bool
	Evidence       []byte
	IPFingerprint  string
	DeviceFingerprint string
}

// calculateVoterWeight calculates voting weight based on stake.
func (rm *ReputationManager) calculateVoterWeight(stake uint64) float64 {
	if stake < rm.config.MinStakeToVote {
		return 0.0 // Cannot vote without minimum stake
	}

	// Weight = (stake)^exponent, normalized
	// exponent = 0.5 means sqrt(stake), which reduces the advantage of large stakeholders
	weight := math.Pow(float64(stake), rm.config.VoteWeightExponent)

	// Normalize to 0.01 - 2.0 range
	normalizedWeight := weight / 100.0
	if normalizedWeight < 0.01 {
		normalizedWeight = 0.01
	}
	if normalizedWeight > 2.0 {
		normalizedWeight = 2.0
	}

	return normalizedWeight
}

// updateStatistics updates node statistics based on the reputation source.
func (rm *ReputationManager) updateStatistics(score *ReputationScore, source ReputationSource, context *ReputationContext) {
	switch source {
	case ReputationSourceUserAccept:
		score.TotalTasksAccepted++
		score.TotalTasksCompleted++
	case ReputationSourceUserReject:
		score.TotalTasksRejected++
		score.TotalTasksCompleted++
	case ReputationSourceUserSkip:
		score.TotalTasksCompleted++
	case ReputationSourceTaskComplete:
		score.TotalTasksCompleted++
	case ReputationSourceBlockProduce:
		score.TotalBlocksProduced++
	case ReputationSourceSlash:
		score.TotalSlashes++
	}

	if context != nil && context.ResponseTimeMs > 0 {
		// Update rolling average response time using all events that have response time
		totalWithRT := score.TotalTasksCompleted + score.TotalBlocksProduced
		if totalWithRT > 0 {
			// Rolling average: compute new average from old average and new value
			if score.AvgResponseTimeMs == 0 {
				score.AvgResponseTimeMs = context.ResponseTimeMs
			} else {
				score.AvgResponseTimeMs = (score.AvgResponseTimeMs*float64(totalWithRT-1) + context.ResponseTimeMs) / float64(totalWithRT)
			}
		}
	}

	score.LastActiveTime = time.Now()
}

// updateSuspicionScore updates the suspicion score based on patterns.
func (rm *ReputationManager) updateSuspicionScore(score *ReputationScore, voterID string, source ReputationSource) {
	// Increase suspicion for repeated patterns
	rm.mu.Lock()
	voterHist, exists := rm.voterHistory[voterID]
	rm.mu.Unlock()

	if exists && voterHist.VotePatternScore > rm.config.SuspicionThresholdMedium {
		// High pattern score voter - increase node's suspicion
		score.SuspicionScore += 5.0
	}

	// Decay suspicion over time (good behavior reduces suspicion)
	if source == ReputationSourceUserAccept || source == ReputationSourceBlockProduce {
		score.SuspicionScore *= 0.95
	}

	// Cap suspicion score
	if score.SuspicionScore > 100 {
		score.SuspicionScore = 100
	}
	if score.SuspicionScore < 0 {
		score.SuspicionScore = 0
	}
}

// recalculateScore recalculates the total reputation score.
func (rm *ReputationManager) recalculateScore(score *ReputationScore) {
	// Calculate user score from active points
	var userPoints float64
	for _, p := range score.ActivePoints {
		switch p.Source {
		case ReputationSourceUserAccept, ReputationSourceUserReject,
			ReputationSourceUserSkip, ReputationSourceUserReport:
			userPoints += p.Amount
		}
	}
	score.UserScore = rm.config.BaseScore + userPoints

	// Calculate tech score
	score.TechScore = rm.calculateTechScore(score)

	// Calculate history score
	score.HistoryScore = rm.calculateHistoryScore(score)

	// Calculate stake score
	score.StakeScore = rm.calculateStakeScore(score)

	// Calculate total (weighted average)
	score.TotalScore = (score.UserScore * rm.config.UserScoreWeight +
		score.TechScore * rm.config.TechScoreWeight +
		score.HistoryScore * rm.config.HistoryScoreWeight +
		score.StakeScore * rm.config.StakeScoreWeight)

	// Apply bounds
	if score.TotalScore < rm.config.MinScore {
		score.TotalScore = rm.config.MinScore
	}
	if score.TotalScore > rm.config.MaxScore {
		score.TotalScore = rm.config.MaxScore
	}
}

// calculateTechScore calculates the technical performance score.
func (rm *ReputationManager) calculateTechScore(score *ReputationScore) float64 {
	techScore := rm.config.BaseScore

	// Response time factor (faster = better)
	if score.AvgResponseTimeMs > 0 {
		// Penalty for slow responses (> 5 seconds)
		if score.AvgResponseTimeMs > 5000 {
			techScore -= 10.0
		}
		// Bonus for fast responses (< 1 second)
		if score.AvgResponseTimeMs < 1000 {
			techScore += 5.0
		}
	}

	// Uptime factor
	if score.UptimePercentage < 95 {
		techScore -= (100 - score.UptimePercentage) * 0.5
	}

	return techScore
}

// calculateHistoryScore calculates the historical performance score.
func (rm *ReputationManager) calculateHistoryScore(score *ReputationScore) float64 {
	historyScore := rm.config.BaseScore

	// Block production bonus
	if score.TotalBlocksProduced > 0 {
		historyScore += float64(score.TotalBlocksProduced) * 2.0
	}

	// Task completion rate
	if score.TotalTasksCompleted > 0 {
		acceptRate := float64(score.TotalTasksAccepted) / float64(score.TotalTasksCompleted)
		historyScore += acceptRate * 20.0
	}

	// Slash penalty
	if score.TotalSlashes > 0 {
		historyScore -= float64(score.TotalSlashes) * 30.0
	}

	return historyScore
}

// calculateStakeScore calculates the stake-based score.
func (rm *ReputationManager) calculateStakeScore(score *ReputationScore) float64 {
	if rm.stakeManager == nil {
		return rm.config.BaseScore
	}

	stakeInfo, err := rm.stakeManager.GetStakeInfo(stringToNodeID(score.NodeID))
	if err != nil {
		return rm.config.BaseScore
	}

	// Stake score = base + sqrt(stake/1000) * 10
	stakeScore := rm.config.BaseScore + math.Sqrt(float64(stakeInfo.Amount)/1000.0)*10.0

	return stakeScore
}

// ============================================================
// Time-Locked Reputation
// ============================================================

// ProcessMaturations processes all pending reputation points and matures those that are ready.
func (rm *ReputationManager) ProcessMaturations() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	matured := 0
	now := time.Now()

	for _, score := range rm.scores {
		score.mu.Lock()

		var stillPending []*ReputationPoint
		for _, point := range score.PendingPoints {
			if now.After(point.MaturesAt) || now.Equal(point.MaturesAt) {
				point.Matured = true
				score.ActivePoints = append(score.ActivePoints, point)
				matured++
			} else {
				stillPending = append(stillPending, point)
			}
		}
		score.PendingPoints = stillPending

		if matured > 0 {
			rm.recalculateScore(score)
		}

		score.mu.Unlock()
	}

	return matured
}

// ApplyDecay applies daily decay to all reputation scores.
func (rm *ReputationManager) ApplyDecay() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, score := range rm.scores {
		score.mu.Lock()

		// Apply decay to active points
		var stillActive []*ReputationPoint
		for _, point := range score.ActivePoints {
			if point.Amount > 0 {
				// Decay positive points
				point.Amount *= rm.config.DailyDecayRate

				// Reduce decay for nodes that recently produced blocks
				if score.TotalBlocksProduced > 0 {
					point.Amount /= math.Sqrt(rm.config.DailyDecayRate) // Partial recovery
				}

				// Remove very small points
				if math.Abs(point.Amount) > 0.01 {
					stillActive = append(stillActive, point)
				}
			} else {
				// Keep negative points (penalties don't decay as fast)
				point.Amount *= math.Sqrt(rm.config.DailyDecayRate)
				stillActive = append(stillActive, point)
			}
		}
		score.ActivePoints = stillActive

		rm.recalculateScore(score)

		score.mu.Unlock()
	}
}

// ============================================================
// Voter History & Sybil Detection
// ============================================================

// updateVoterHistory updates the voter's history for pattern analysis.
func (rm *ReputationManager) updateVoterHistory(voterID, providerID string, source ReputationSource, context *ReputationContext) {
	if voterID == "" {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	hist, exists := rm.voterHistory[voterID]
	if !exists {
		hist = &VoterHistory{
			VoterID:          voterID,
			VotesByProvider:  make(map[string]uint64),
			IPFingerprints:   make(map[string]int),
			DeviceFingerprints: make(map[string]int),
			FirstVoteTime:    time.Now(),
		}
		rm.voterHistory[voterID] = hist
	}

	hist.TotalVotes++
	hist.VotesByProvider[providerID]++
	hist.LastVoteTime = time.Now()

	// Update accept rate
	if source == ReputationSourceUserAccept {
		hist.AcceptRate = (hist.AcceptRate*float64(hist.TotalVotes-1) + 1.0) / float64(hist.TotalVotes)
	} else if source == ReputationSourceUserReject {
		hist.AcceptRate = (hist.AcceptRate * float64(hist.TotalVotes-1)) / float64(hist.TotalVotes)
	}

	// Track IP/device fingerprints
	if context != nil {
		if context.IPFingerprint != "" {
			hist.IPFingerprints[context.IPFingerprint]++
		}
		if context.DeviceFingerprint != "" {
			hist.DeviceFingerprints[context.DeviceFingerprint]++
		}
	}

	// Calculate pattern score
	hist.VotePatternScore = rm.calculateVoterPatternScore(hist)
}

// calculateVoterPatternScore calculates how suspicious a voter's pattern is.
func (rm *ReputationManager) calculateVoterPatternScore(hist *VoterHistory) float64 {
	score := 0.0

	// Check if voter only votes for few providers
	if hist.TotalVotes > 10 {
		uniqueProviders := len(hist.VotesByProvider)
		providerRatio := float64(uniqueProviders) / float64(hist.TotalVotes)

		// If voting for very few providers, increase suspicion
		if providerRatio < 0.1 {
			score += 30.0
		} else if providerRatio < 0.3 {
			score += 15.0
		}
	}

	// Check if voter always accepts (never rejects)
	if hist.TotalVotes > 5 && hist.AcceptRate > 0.95 {
		score += 25.0
	}

	// Check for IP/device reuse (multiple accounts from same fingerprint)
	if len(hist.IPFingerprints) > 0 {
		for _, count := range hist.IPFingerprints {
			if count > 50 {
				score += 20.0
			}
		}
	}

	// Check voting velocity (too many votes in short time)
	votingDuration := hist.LastVoteTime.Sub(hist.FirstVoteTime).Hours()
	if votingDuration > 0 {
		votesPerHour := float64(hist.TotalVotes) / votingDuration
		if votesPerHour > 10 {
			score += 25.0
		} else if votesPerHour > 5 {
			score += 10.0
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// GetSuspiciousVoters returns voters with high suspicion scores.
func (rm *ReputationManager) GetSuspiciousVoters(threshold float64) []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var suspicious []string
	for voterID, hist := range rm.voterHistory {
		if hist.VotePatternScore >= threshold {
			suspicious = append(suspicious, voterID)
		}
	}
	return suspicious
}

// ============================================================
// Challenge Mechanism
// ============================================================

// CreateChallenge creates a new challenge against a reputation point.
func (rm *ReputationManager) CreateChallenge(
	challengerID string,
	targetNodeID string,
	pointID string,
	reason string,
	evidence []byte,
	stakeAmount uint64,
) (*Challenge, error) {

	rm.mu.Lock()
	defer rm.mu.Unlock()

	targetNodeKey := targetNodeID

	// Verify challenger has enough stake
	if rm.stakeManager != nil {
		challengerNodeID := interfaces.NodeID{}
		copy(challengerNodeID[:], []byte(challengerID))
		if !rm.stakeManager.HasMinimumStake(challengerNodeID) {
			return nil, fmt.Errorf("reputation: challenger has insufficient stake")
		}
	}

	// Find the point
	var targetPoint *ReputationPoint
	score, exists := rm.scores[targetNodeKey]
	if exists {
		score.mu.RLock()
		for _, p := range score.ActivePoints {
			if p.ID == pointID {
				targetPoint = p
				break
			}
		}
		for _, p := range score.PendingPoints {
			if p.ID == pointID {
				targetPoint = p
				break
			}
		}
		score.mu.RUnlock()
	}

	if targetPoint == nil {
		return nil, fmt.Errorf("reputation: point not found: %s", pointID)
	}

	// Create challenge
	challenge := &Challenge{
		ID:           generateChallengeID(challengerID, targetNodeID, time.Now()),
		ChallengerID: challengerID,
		TargetNodeID: targetNodeKey,
		PointID:      pointID,
		Reason:       reason,
		Evidence:     evidence,
		StakeAmount:  stakeAmount,
		Status:       ChallengeStatusPending,
		CreatedAt:    time.Now(),
		Votes:        make(map[string]bool),
	}

	rm.challenges[challenge.ID] = challenge

	// Mark point as challenged
	targetPoint.Challenged = true
	targetPoint.ChallengeID = challenge.ID

	return challenge, nil
}

// VoteOnChallenge allows an arbitrator to vote on a challenge.
func (rm *ReputationManager) VoteOnChallenge(challengeID string, arbitratorID string, supportTarget bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	challenge, exists := rm.challenges[challengeID]
	if !exists {
		return fmt.Errorf("reputation: challenge not found: %s", challengeID)
	}

	if challenge.Status != ChallengeStatusPending && challenge.Status != ChallengeStatusVoting {
		return fmt.Errorf("reputation: challenge not in voting state")
	}

	challenge.Status = ChallengeStatusVoting
	challenge.Votes[arbitratorID] = supportTarget

	return nil
}

// ResolveChallenge resolves a challenge based on votes.
func (rm *ReputationManager) ResolveChallenge(challengeID string, requiredVotes int) (*Challenge, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	challenge, exists := rm.challenges[challengeID]
	if !exists {
		return nil, fmt.Errorf("reputation: challenge not found: %s", challengeID)
	}

	if len(challenge.Votes) < requiredVotes {
		return nil, fmt.Errorf("reputation: not enough votes: %d < %d", len(challenge.Votes), requiredVotes)
	}

	// Count votes
	supportTarget := 0
	againstTarget := 0
	for _, support := range challenge.Votes {
		if support {
			supportTarget++
		} else {
			againstTarget++
		}
	}

	now := time.Now()
	challenge.ResolvedAt = &now

	// Determine outcome
	if againstTarget > supportTarget {
		// Challenge succeeded - target node was cheating
		challenge.Status = ChallengeStatusResolvedSuccess
		challenge.Resolution = "challenge_succeeded"

		// Slash the target node
		score, exists := rm.scores[challenge.TargetNodeID]
		if exists {
			rm.AddReputation(
				challenge.TargetNodeID,
				ReputationSourceChallengeLost,
				"",
				0,
				&ReputationContext{Evidence: challenge.Evidence},
			)

			// Remove the challenged point
			score.mu.Lock()
			var newActive []*ReputationPoint
			for _, p := range score.ActivePoints {
				if p.ID != challenge.PointID {
					newActive = append(newActive, p)
				}
			}
			score.ActivePoints = newActive
			rm.recalculateScore(score)
			score.mu.Unlock()
		}

	} else {
		// Challenge failed - target node was legitimate
		challenge.Status = ChallengeStatusResolvedFailed
		challenge.Resolution = "challenge_failed"

		// Reward the target
		rm.AddReputation(
			challenge.TargetNodeID,
			ReputationSourceChallengeWon,
			"",
			0,
			nil,
		)
	}

	return challenge, nil
}

// ============================================================
// Block Producer Selection
// ============================================================

// CalculateBlockProbability calculates the probability of a node being selected as block producer.
func (rm *ReputationManager) CalculateBlockProbability(nodeID string) (float64, error) {
	score, err := rm.GetScore(nodeID)
	if err != nil {
		return 0, err
	}

	score.mu.RLock()
	defer score.mu.RUnlock()

	// Base probability from reputation
	// P = (R / 100)^1.5 where R is reputation score
	reputationFactor := math.Pow(score.TotalScore/100.0, 1.5)

	// Stake factor (if available)
	stakeFactor := 1.0
	if rm.stakeManager != nil {
		stakeInfo, err := rm.stakeManager.GetStakeInfo(stringToNodeID(nodeID))
		if err == nil {
			stakeFactor = math.Pow(float64(stakeInfo.Amount)/1000.0, 0.5)
		}
	}

	// History factor
	historyFactor := 1.0
	if score.TotalBlocksProduced > 0 {
		historyFactor = math.Pow(float64(score.TotalBlocksProduced), 0.3)
	}

	// Suspicion penalty
	suspicionPenalty := 1.0 - (score.SuspicionScore / 100.0)

	// Combined probability weight
	weight := reputationFactor * stakeFactor * historyFactor * suspicionPenalty

	// Ensure non-negative
	if weight < 0 {
		weight = 0
	}

	return weight, nil
}

// SelectBlockProducer selects a block producer based on probability weights.
func (rm *ReputationManager) SelectBlockProducer(excludeNodes []string) (string, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Build exclusion map
	excludeMap := make(map[string]bool)
	for _, id := range excludeNodes {
		excludeMap[id] = true
	}

	// Calculate weights for all eligible nodes
	type nodeWeight struct {
		id     string
		weight float64
	}

	var weights []nodeWeight
	totalWeight := 0.0

	for nodeID, score := range rm.scores {
		// Skip excluded nodes
		if excludeMap[nodeID] {
			continue
		}

		// Skip nodes with very low reputation
		if score.TotalScore < 0 {
			continue
		}

		// Skip frozen nodes
		score.mu.RLock()
		if score.SuspicionScore >= rm.config.SuspicionThresholdMax {
			score.mu.RUnlock()
			continue
		}
		score.mu.RUnlock()

		// Calculate weight
		weight, _ := rm.CalculateBlockProbability(nodeID)
		if weight > 0 {
			weights = append(weights, nodeWeight{id: nodeID, weight: weight})
			totalWeight += weight
		}
	}

	if len(weights) == 0 {
		return "", fmt.Errorf("reputation: no eligible block producers")
	}

	// Weighted random selection (lottery mechanism)
	lottery := secureRandomFloat() * totalWeight

	cumulative := 0.0
	for _, nw := range weights {
		cumulative += nw.weight
		if cumulative >= lottery {
			return nw.id, nil
		}
	}

	// Fallback to last node
	return weights[len(weights)-1].id, nil
}

// GetAllScores returns all reputation scores.
func (rm *ReputationManager) GetAllScores() []*ReputationScore {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	scores := make([]*ReputationScore, 0, len(rm.scores))
	for _, score := range rm.scores {
		scores = append(scores, score)
	}

	// Sort by total score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	return scores
}

// GetTopProviders returns the top N providers by reputation.
func (rm *ReputationManager) GetTopProviders(n int) []*ReputationScore {
	scores := rm.GetAllScores()
	if len(scores) < n {
		return scores
	}
	return scores[:n]
}

// ============================================================

// stringToNodeID converts a string to interfaces.NodeID.
func stringToNodeID(s string) interfaces.NodeID {
	var nodeID interfaces.NodeID
	copy(nodeID[:], []byte(s))
	return nodeID
}

func generatePointID(nodeID string, source ReputationSource, timestamp time.Time) string {
	data := fmt.Sprintf("%s-%d-%d", nodeID, source, timestamp.UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16]
}
func generateChallengeID(challengerID string, targetNodeID string, timestamp time.Time) string {
	data := fmt.Sprintf("%s-%s-%d", challengerID, targetNodeID, timestamp.UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16]
}

func secureRandomFloat() float64 {
	// In production, use crypto/rand for secure randomness
	// This is a simplified version
	timestamp := time.Now().UnixNano()
	return float64(timestamp%1000000) / 1000000.0
}

// ToJSON exports the reputation score as JSON.
func (rs *ReputationScore) ToJSON() ([]byte, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return json.MarshalIndent(rs, "", "  ")
}

// Summary returns a summary of the reputation score.
func (rs *ReputationScore) Summary() map[string]interface{} {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return map[string]interface{}{
		"node_id":             fmt.Sprintf("%x", rs.NodeID),
		"total_score":         fmt.Sprintf("%.2f", rs.TotalScore),
		"user_score":          fmt.Sprintf("%.2f", rs.UserScore),
		"tech_score":          fmt.Sprintf("%.2f", rs.TechScore),
		"history_score":       fmt.Sprintf("%.2f", rs.HistoryScore),
		"stake_score":         fmt.Sprintf("%.2f", rs.StakeScore),
		"suspicion_score":     fmt.Sprintf("%.2f", rs.SuspicionScore),
		"tasks_completed":     rs.TotalTasksCompleted,
		"blocks_produced":     rs.TotalBlocksProduced,
		"slashes":             rs.TotalSlashes,
		"pending_points":      len(rs.PendingPoints),
		"active_points":       len(rs.ActivePoints),
		"uptime":              fmt.Sprintf("%.1f%%", rs.UptimePercentage),
		"avg_response_ms":     fmt.Sprintf("%.0f", rs.AvgResponseTimeMs),
	}
}
