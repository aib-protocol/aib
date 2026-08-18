package airdrop

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrScoreBelowThreshold is returned when the score is below the threshold
	ErrScoreBelowThreshold = errors.New("score below threshold")
	// ErrSybilDetected indicates a sybil attack was detected
	ErrSybilDetected = errors.New("sybil attack detected")
)

// ScoringConfig holds the scoring configuration.
type ScoringConfig struct {
	// Minimum qualifying score
	MinThreshold int
	// Maximum score
	MaxScore int

	// Account age weight (0-100)
	AccountAgeWeight int
	// Social activity weight (0-100)
	SocialActivityWeight int
	// Repository activity weight (0-100)
	RepoActivityWeight int
	// Code contribution weight (0-100)
	CodeContributionWeight int

	// Sybil detection threshold
	SybilThreshold float64

	// Minimum account age
	MinAccountAge time.Duration
}

// DefaultScoringConfig returns the default scoring configuration.
func DefaultScoringConfig() *ScoringConfig {
	return &ScoringConfig{
		MinThreshold:           50,
		MaxScore:               100,
		AccountAgeWeight:       20,
		SocialActivityWeight:   25,
		RepoActivityWeight:     25,
		CodeContributionWeight: 30,
		SybilThreshold:         0.7,
		MinAccountAge:          30 * 24 * time.Hour, // 30 days
	}
}

// Score holds the scoring result.
type Score struct {
	Total        int `json:"total"`
	AccountAge   int `json:"account_age"`
	Social       int `json:"social"`
	Repo         int `json:"repo"`
	Contribution int `json:"contribution"`
	Bonus        int `json:"bonus"`

	// Details
	AccountAgeDays int     `json:"account_age_days"`
	SybilScore     float64 `json:"sybil_score"`
	IsEligible     bool    `json:"is_eligible"`
	Reason         string  `json:"reason,omitempty"`
}

// Scorer is the scoring system.
type Scorer struct {
	config        *ScoringConfig
	sybilDetector *SybilDetector
	mu            sync.RWMutex
}

// NewScorer creates a new Scorer.
func NewScorer(config *ScoringConfig) *Scorer {
	if config == nil {
		config = DefaultScoringConfig()
	}
	return &Scorer{
		config:        config,
		sybilDetector: NewSybilDetector(config.SybilThreshold),
	}
}

// ScoreUser scores a user.
func (s *Scorer) ScoreUser(userInfo *GitHubUserInfo, additionalData *AdditionalData) *Score {
	s.mu.Lock()
	defer s.mu.Unlock()

	score := &Score{
		AccountAgeDays: int(time.Since(userInfo.CreatedAt).Hours() / 24),
		SybilScore:     0,
		IsEligible:     false,
	}

	// 1. Account age score
	score.AccountAge = s.scoreAccountAge(userInfo.CreatedAt)

	// 2. Social activity score
	score.Social = s.scoreSocialActivity(userInfo, additionalData)

	// 3. Repository activity score
	score.Repo = s.scoreRepoActivity(userInfo, additionalData)

	// 4. Code contribution score
	score.Contribution = s.scoreCodeContribution(userInfo, additionalData)

	// 5. Calculate the total score
	score.Total = s.calculateTotal(score)

	// 6. Sybil detection
	score.SybilScore = s.sybilDetector.Detect(userInfo, additionalData)

	// 7. Determine eligibility
	score.IsEligible = s.isEligible(score)
	if !score.IsEligible {
		if score.Total < s.config.MinThreshold {
			score.Reason = "score below threshold"
		} else if score.SybilScore > s.config.SybilThreshold {
			score.Reason = "suspected sybil attack"
		} else {
			score.Reason = "eligibility criteria not met"
		}
	}

	return score
}

// scoreAccountAge computes the account age score.
func (s *Scorer) scoreAccountAge(createdAt time.Time) int {
	age := time.Since(createdAt)
	days := int(age.Hours() / 24)

	// Minimum age requirement
	if age < s.config.MinAccountAge {
		return 0
	}

	// Scoring curve (logarithmic growth)
	// 30 days = 20%, 180 days = 60%, 365 days = 80%, 730+ days = 100%
	switch {
	case days < 60:
		return 20
	case days < 120:
		return 40
	case days < 180:
		return 60
	case days < 365:
		return 75
	case days < 730:
		return 90
	default:
		return 100
	}
}

// scoreSocialActivity computes the social activity score.
func (s *Scorer) scoreSocialActivity(userInfo *GitHubUserInfo, data *AdditionalData) int {
	score := 0

	// Followers score (logarithmic)
	followerScore := s.logScore(userInfo.Followers, 1, 1000, 30)
	score += followerScore

	// Following score (moderation suffices)
	followingScore := 0
	switch {
	case userInfo.Following == 0:
		followingScore = 0
	case userInfo.Following <= 50:
		followingScore = 20
	case userInfo.Following <= 200:
		followingScore = 30
	default:
		followingScore = 10 // Following too many accounts may indicate a bot
	}
	score += followingScore

	// Profile completeness bonus for name, bio, location, etc.
	if userInfo.Name != "" {
		score += 10
	}
	if userInfo.Bio != "" {
		score += 10
	}
	if userInfo.Location != "" {
		score += 10
	}
	if userInfo.Email != "" {
		score += 10
	}

	// Cap at the maximum value
	if score > 100 {
		score = 100
	}

	return score
}

// scoreRepoActivity computes the repository activity score.
func (s *Scorer) scoreRepoActivity(userInfo *GitHubUserInfo, data *AdditionalData) int {
	if data == nil {
		return s.baseRepoScore(userInfo.PublicRepos)
	}

	score := s.baseRepoScore(userInfo.PublicRepos)

	// Bonus for organization membership
	if len(data.Organizations) > 0 {
		score += 10
	}

	// Bonus for starred repositories
	if data.StarredCount > 0 {
		score += min(data.StarredCount, 20)
	}

	// Bonus for repositories with forks
	if data.HasForks > 0 {
		score += min(data.HasForks*2, 20)
	}

	// Cap at the maximum value
	if score > 100 {
		score = 100
	}

	return score
}

// baseRepoScore computes the base repository score.
func (s *Scorer) baseRepoScore(publicRepos int) int {
	switch {
	case publicRepos == 0:
		return 0
	case publicRepos <= 3:
		return 30
	case publicRepos <= 10:
		return 60
	case publicRepos <= 30:
		return 80
	default:
		return 100
	}
}

// scoreCodeContribution computes the code contribution score.
func (s *Scorer) scoreCodeContribution(userInfo *GitHubUserInfo, data *AdditionalData) int {
	if data == nil {
		return 0
	}

	score := 0

	// Recent activity bonus
	if data.RecentCommits > 0 {
		score += min(data.RecentCommits*5, 40)
	}

	// Pull request contribution bonus
	if data.PullRequests > 0 {
		score += min(data.PullRequests*10, 30)
	}

	// Issue contribution bonus
	if data.Issues > 0 {
		score += min(data.Issues*5, 15)
	}

	// Gist contribution bonus
	if data.Gists > 0 {
		score += min(data.Gists*2, 15)
	}

	// Cap at the maximum value
	if score > 100 {
		score = 100
	}

	return score
}

// calculateTotal computes the total score.
func (s *Scorer) calculateTotal(score *Score) int {
	total := 0

	total += score.AccountAge * s.config.AccountAgeWeight / 100
	total += score.Social * s.config.SocialActivityWeight / 100
	total += score.Repo * s.config.RepoActivityWeight / 100
	total += score.Contribution * s.config.CodeContributionWeight / 100

	total += score.Bonus

	if total > s.config.MaxScore {
		total = s.config.MaxScore
	}

	return total
}

// isEligible determines eligibility.
func (s *Scorer) isEligible(score *Score) bool {
	// Minimum score check
	if score.Total < s.config.MinThreshold {
		return false
	}

	// Sybil detection check
	if score.SybilScore > s.config.SybilThreshold {
		return false
	}

	return true
}

// logScore computes a logarithmic score.
func (s *Scorer) logScore(value, min, max, maxScore int) int {
	if value <= min {
		return 0
	}
	if value >= max {
		return maxScore
	}

	// Logarithmic curve
	ratio := math.Log(float64(value-min+1)) / math.Log(float64(max-min+1))
	return int(ratio * float64(maxScore))
}

// AdditionalData holds extra data (requires additional API calls to fetch).
type AdditionalData struct {
	// Organization memberships
	Organizations []string
	// Recent commits
	RecentCommits int
	// Number of pull requests
	PullRequests int
	// Number of issues
	Issues int
	// Number of gists
	Gists int
	// Number of starred repositories
	StarredCount int
	// Number of forked repositories
	HasForks int
	// Language usage
	Languages map[string]int
}

// min returns the smaller of two values.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// Sybil Attack Detection
// ============================================================================

// SybilDetector detects sybil attacks.
type SybilDetector struct {
	threshold float64

	// Seen profile features
	seenProfiles map[string]*ProfileFeatures
	mu           sync.RWMutex

	// Cluster analysis
	clusters       []*Cluster
	clusterCounter int
}

// ProfileFeatures holds account features.
type ProfileFeatures struct {
	GitHubID    uint64
	Login       string
	CreatedAt   time.Time
	Followers   int
	Following   int
	PublicRepos int
	BioLength   int
	HasEmail    bool
	HasBlog     bool
	HasLocation bool
	HasName     bool

	// Behavioral features
	AvgCommitsPerWeek float64
	ActiveDays        int

	// Network features
	FollowersMap map[uint64]bool
	FollowingMap map[uint64]bool
}

// Cluster represents a cluster of accounts.
type Cluster struct {
	ID      int
	Members []uint64
	Center  *ProfileFeatures
	Size    int
	Created time.Time
}

// NewSybilDetector creates a new SybilDetector.
func NewSybilDetector(threshold float64) *SybilDetector {
	return &SybilDetector{
		threshold:    threshold,
		seenProfiles: make(map[string]*ProfileFeatures),
		clusters:     make([]*Cluster, 0),
	}
}

// Detect checks for a sybil attack.
func (sd *SybilDetector) Detect(userInfo *GitHubUserInfo, data *AdditionalData) float64 {
	// Extract features
	features := sd.extractFeatures(userInfo, data)

	// Calculate the suspicion score
	suspicionScore := sd.calculateSuspicion(features)

	// Record features
	sd.mu.Lock()
	key := fmt.Sprintf("%d", userInfo.ID)
	sd.seenProfiles[key] = features
	sd.mu.Unlock()

	return suspicionScore
}

// extractFeatures extracts account features.
func (sd *SybilDetector) extractFeatures(userInfo *GitHubUserInfo, data *AdditionalData) *ProfileFeatures {
	features := &ProfileFeatures{
		GitHubID:    userInfo.ID,
		Login:       userInfo.Login,
		CreatedAt:   userInfo.CreatedAt,
		Followers:   userInfo.Followers,
		Following:   userInfo.Following,
		PublicRepos: userInfo.PublicRepos,
		BioLength:   len(userInfo.Bio),
		HasEmail:    userInfo.Email != "",
		HasBlog:     userInfo.Blog != "",
		HasLocation: userInfo.Location != "",
		HasName:     userInfo.Name != "",
	}

	if data != nil {
		features.ActiveDays = data.RecentCommits
	}

	return features
}

// calculateSuspicion computes the suspicion score.
func (sd *SybilDetector) calculateSuspicion(features *ProfileFeatures) float64 {
	score := 0.0

	// 1. Account age check
	age := time.Since(features.CreatedAt).Hours() / 24
	if age < 7 {
		score += 0.3 // Created within a week
	} else if age < 30 {
		score += 0.1 // Created within a month
	}

	// 2. Profile completeness check
	completeness := 0
	if features.HasName {
		completeness++
	}
	if features.HasEmail {
		completeness++
	}
	if features.HasBlog {
		completeness++
	}
	if features.HasLocation {
		completeness++
	}
	if features.BioLength > 10 {
		completeness++
	}

	if completeness <= 1 {
		score += 0.2 // Incomplete profile
	}

	// 3. Social pattern check
	if features.Followers == 0 && features.Following == 0 {
		score += 0.15 // No social activity
	}

	if features.Following > 500 && features.Followers < 10 {
		score += 0.1 // Follows many but has almost no followers
	}

	// 4. Repository pattern check
	if features.PublicRepos == 0 {
		score += 0.15 // No public repositories
	}

	// 5. Name pattern check (simple check)
	if isBotLikeName(features.Login) {
		score += 0.2
	}

	// Clamp to the 0-1 range
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// isBotLikeName checks whether the login looks like a bot.
func isBotLikeName(login string) bool {
	// Check common bot patterns
	botPatterns := []string{
		"bot", "test", "demo", "tmp", "temp",
	}

	lower := strings.ToLower(login)
	for _, pattern := range botPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// FindClusters discovers sybil clusters.
func (sd *SybilDetector) FindClusters() []*Cluster {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Simple clustering algorithm: based on account creation time and behavioral similarity
	profiles := make([]*ProfileFeatures, 0, len(sd.seenProfiles))
	for _, p := range sd.seenProfiles {
		profiles = append(profiles, p)
	}

	// Sort by creation time
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].CreatedAt.Before(profiles[j].CreatedAt)
	})

	clusters := make([]*Cluster, 0)

	// Time-window clustering (similar accounts created within 24 hours)
	window := 24 * time.Hour
	currentCluster := &Cluster{
		ID:      sd.clusterCounter,
		Members: make([]uint64, 0),
		Created: time.Now(),
	}
	sd.clusterCounter++

	if len(profiles) > 0 {
		currentCluster.Center = profiles[0]
		currentCluster.Members = append(currentCluster.Members, profiles[0].GitHubID)
	}

	for i := 1; i < len(profiles); i++ {
		p := profiles[i]
		timeDiff := p.CreatedAt.Sub(currentCluster.Center.CreatedAt)

		if timeDiff < window && sd.similar(currentCluster.Center, p) {
			currentCluster.Members = append(currentCluster.Members, p.GitHubID)
			currentCluster.Size++
		} else {
			if currentCluster.Size > 1 {
				clusters = append(clusters, currentCluster)
			}
			currentCluster = &Cluster{
				ID:      sd.clusterCounter,
				Members: []uint64{p.GitHubID},
				Center:  p,
				Size:    1,
				Created: time.Now(),
			}
			sd.clusterCounter++
		}
	}

	if currentCluster.Size > 1 {
		clusters = append(clusters, currentCluster)
	}

	sd.clusters = clusters
	return clusters
}

// similar checks whether two feature sets are similar.
func (sd *SybilDetector) similar(a, b *ProfileFeatures) bool {
	// Check account creation time difference
	timeDiff := a.CreatedAt.Sub(b.CreatedAt)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	// Check profile similarity
	similarBio := a.BioLength == 0 && b.BioLength == 0
	similarSocial := a.Followers < 10 && b.Followers < 10
	similarRepos := a.PublicRepos == b.PublicRepos

	matchCount := 0
	if similarBio {
		matchCount++
	}
	if similarSocial {
		matchCount++
	}
	if similarRepos {
		matchCount++
	}

	// At least two matching features
	return matchCount >= 2 || timeDiff < time.Hour
}

// GetSuspiciousClusters returns suspicious clusters.
func (sd *SybilDetector) GetSuspiciousClusters(minSize int) []*Cluster {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	suspicious := make([]*Cluster, 0)
	for _, cluster := range sd.clusters {
		if cluster.Size >= minSize {
			suspicious = append(suspicious, cluster)
		}
	}

	return suspicious
}

// IsInCluster checks whether the user is in a suspicious cluster.
func (sd *SybilDetector) IsInCluster(githubID uint64) bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	for _, cluster := range sd.clusters {
		for _, member := range cluster.Members {
			if member == githubID {
				return true
			}
		}
	}

	return false
}

// Clear clears the cache.
func (sd *SybilDetector) Clear() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.seenProfiles = make(map[string]*ProfileFeatures)
	sd.clusters = make([]*Cluster, 0)
	sd.clusterCounter = 0
}

// GetProfileCount returns the number of analyzed profiles.
func (sd *SybilDetector) GetProfileCount() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return len(sd.seenProfiles)
}
