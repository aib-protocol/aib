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
	// ErrScoreBelowThreshold 分数低于阈值
	ErrScoreBelowThreshold = errors.New("score below threshold")
	// ErrSybilDetected 检测到女巫攻击
	ErrSybilDetected = errors.New("sybil attack detected")
)

// ScoringConfig 评分配置
type ScoringConfig struct {
	// 最低合格分数
	MinThreshold int
	// 最大分数
	MaxScore int

	// 账户年龄权重 (0-100)
	AccountAgeWeight int
	// 社交活动权重 (0-100)
	SocialActivityWeight int
	// 仓库活跃度权重 (0-100)
	RepoActivityWeight int
	// 代码贡献权重 (0-100)
	CodeContributionWeight int

	// 女巫检测阈值
	SybilThreshold float64

	// 最小账户年龄
	MinAccountAge time.Duration
}

// DefaultScoringConfig 默认评分配置
func DefaultScoringConfig() *ScoringConfig {
	return &ScoringConfig{
		MinThreshold:           50,
		MaxScore:               100,
		AccountAgeWeight:       20,
		SocialActivityWeight:   25,
		RepoActivityWeight:     25,
		CodeContributionWeight: 30,
		SybilThreshold:         0.7,
		MinAccountAge:          30 * 24 * time.Hour, // 30天
	}
}

// Score 评分结果
type Score struct {
	Total        int `json:"total"`
	AccountAge   int `json:"account_age"`
	Social       int `json:"social"`
	Repo         int `json:"repo"`
	Contribution int `json:"contribution"`
	Bonus        int `json:"bonus"`

	// 细节
	AccountAgeDays int     `json:"account_age_days"`
	SybilScore     float64 `json:"sybil_score"`
	IsEligible     bool    `json:"is_eligible"`
	Reason         string  `json:"reason,omitempty"`
}

// Scorer 评分系统
type Scorer struct {
	config        *ScoringConfig
	sybilDetector *SybilDetector
	mu            sync.RWMutex
}

// NewScorer 创建评分器
func NewScorer(config *ScoringConfig) *Scorer {
	if config == nil {
		config = DefaultScoringConfig()
	}
	return &Scorer{
		config:        config,
		sybilDetector: NewSybilDetector(config.SybilThreshold),
	}
}

// ScoreUser 对用户评分
func (s *Scorer) ScoreUser(userInfo *GitHubUserInfo, additionalData *AdditionalData) *Score {
	s.mu.Lock()
	defer s.mu.Unlock()

	score := &Score{
		AccountAgeDays: int(time.Since(userInfo.CreatedAt).Hours() / 24),
		SybilScore:     0,
		IsEligible:     false,
	}

	// 1. 账户年龄评分
	score.AccountAge = s.scoreAccountAge(userInfo.CreatedAt)

	// 2. 社交活动评分
	score.Social = s.scoreSocialActivity(userInfo, additionalData)

	// 3. 仓库活动评分
	score.Repo = s.scoreRepoActivity(userInfo, additionalData)

	// 4. 代码贡献评分
	score.Contribution = s.scoreCodeContribution(userInfo, additionalData)

	// 5. 计算总分
	score.Total = s.calculateTotal(score)

	// 6. 女巫检测
	score.SybilScore = s.sybilDetector.Detect(userInfo, additionalData)

	// 7. 判断是否合格
	score.IsEligible = s.isEligible(score)
	if !score.IsEligible {
		if score.Total < s.config.MinThreshold {
			score.Reason = "分数低于阈值"
		} else if score.SybilScore > s.config.SybilThreshold {
			score.Reason = "疑似女巫攻击"
		} else {
			score.Reason = "不满足资格条件"
		}
	}

	return score
}

// scoreAccountAge 账户年龄评分
func (s *Scorer) scoreAccountAge(createdAt time.Time) int {
	age := time.Since(createdAt)
	days := int(age.Hours() / 24)

	// 最低年龄要求
	if age < s.config.MinAccountAge {
		return 0
	}

	// 评分曲线（对数增长）
	// 30天 = 20%, 180天 = 60%, 365天 = 80%, 730天+ = 100%
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

// scoreSocialActivity 社交活动评分
func (s *Scorer) scoreSocialActivity(userInfo *GitHubUserInfo, data *AdditionalData) int {
	score := 0

	// Followers 评分（对数）
	followerScore := s.logScore(userInfo.Followers, 1, 1000, 30)
	score += followerScore

	// Following 评分（适度即可）
	followingScore := 0
	switch {
	case userInfo.Following == 0:
		followingScore = 0
	case userInfo.Following <= 50:
		followingScore = 20
	case userInfo.Following <= 200:
		followingScore = 30
	default:
		followingScore = 10 // 关注太多可能是机器人
	}
	score += followingScore

	// 有名字、bio、location 等完整性加分
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

	// 限制最大值
	if score > 100 {
		score = 100
	}

	return score
}

// scoreRepoActivity 仓库活动评分
func (s *Scorer) scoreRepoActivity(userInfo *GitHubUserInfo, data *AdditionalData) int {
	if data == nil {
		return s.baseRepoScore(userInfo.PublicRepos)
	}

	score := s.baseRepoScore(userInfo.PublicRepos)

	// 有组织成员加分
	if len(data.Organizations) > 0 {
		score += 10
	}

	// 有星标仓库加分
	if data.StarredCount > 0 {
		score += min(data.StarredCount, 20)
	}

	// 有受关注仓库加分
	if data.HasForks > 0 {
		score += min(data.HasForks*2, 20)
	}

	// 限制最大值
	if score > 100 {
		score = 100
	}

	return score
}

// baseRepoScore 基础仓库评分
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

// scoreCodeContribution 代码贡献评分
func (s *Scorer) scoreCodeContribution(userInfo *GitHubUserInfo, data *AdditionalData) int {
	if data == nil {
		return 0
	}

	score := 0

	// 最近活跃度加分
	if data.RecentCommits > 0 {
		score += min(data.RecentCommits*5, 40)
	}

	// PR 贡献加分
	if data.PullRequests > 0 {
		score += min(data.PullRequests*10, 30)
	}

	// Issue 贡献加分
	if data.Issues > 0 {
		score += min(data.Issues*5, 15)
	}

	// Gist 贡献加分
	if data.Gists > 0 {
		score += min(data.Gists*2, 15)
	}

	// 限制最大值
	if score > 100 {
		score = 100
	}

	return score
}

// calculateTotal 计算总分
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

// isEligible 判断是否合格
func (s *Scorer) isEligible(score *Score) bool {
	// 最低分数检查
	if score.Total < s.config.MinThreshold {
		return false
	}

	// 女巫检测检查
	if score.SybilScore > s.config.SybilThreshold {
		return false
	}

	return true
}

// logScore 对数评分
func (s *Scorer) logScore(value, min, max, maxScore int) int {
	if value <= min {
		return 0
	}
	if value >= max {
		return maxScore
	}

	// 对数曲线
	ratio := math.Log(float64(value-min+1)) / math.Log(float64(max-min+1))
	return int(ratio * float64(maxScore))
}

// AdditionalData 额外数据（需要额外 API 调用获取）
type AdditionalData struct {
	// 组织成员
	Organizations []string
	// 最近活跃提交
	RecentCommits int
	// PR 数量
	PullRequests int
	// Issue 数量
	Issues int
	// Gist 数量
	Gists int
	// 星标数量
	StarredCount int
	// 被复刻仓库数
	HasForks int
	// 语言使用
	Languages map[string]int
}

// min 返回最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// Sybil 女巫攻击检测
// ============================================================================

// SybilDetector 女巫攻击检测器
type SybilDetector struct {
	threshold float64

	// 已见账户特征
	seenProfiles map[string]*ProfileFeatures
	mu           sync.RWMutex

	// 聚类分析
	clusters       []*Cluster
	clusterCounter int
}

// ProfileFeatures 账户特征
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

	// 行为特征
	AvgCommitsPerWeek float64
	ActiveDays        int

	// 网络特征
	FollowersMap map[uint64]bool
	FollowingMap map[uint64]bool
}

// Cluster 聚类
type Cluster struct {
	ID      int
	Members []uint64
	Center  *ProfileFeatures
	Size    int
	Created time.Time
}

// NewSybilDetector 创建女巫检测器
func NewSybilDetector(threshold float64) *SybilDetector {
	return &SybilDetector{
		threshold:    threshold,
		seenProfiles: make(map[string]*ProfileFeatures),
		clusters:     make([]*Cluster, 0),
	}
}

// Detect 检测女巫攻击
func (sd *SybilDetector) Detect(userInfo *GitHubUserInfo, data *AdditionalData) float64 {
	// 提取特征
	features := sd.extractFeatures(userInfo, data)

	// 计算可疑度分数
	suspicionScore := sd.calculateSuspicion(features)

	// 记录特征
	sd.mu.Lock()
	key := fmt.Sprintf("%d", userInfo.ID)
	sd.seenProfiles[key] = features
	sd.mu.Unlock()

	return suspicionScore
}

// extractFeatures 提取账户特征
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

// calculateSuspicion 计算可疑度分数
func (sd *SybilDetector) calculateSuspicion(features *ProfileFeatures) float64 {
	score := 0.0

	// 1. 账户年龄检查
	age := time.Since(features.CreatedAt).Hours() / 24
	if age < 7 {
		score += 0.3 // 一周内创建
	} else if age < 30 {
		score += 0.1 // 一月内创建
	}

	// 2. 资料完整性检查
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
		score += 0.2 // 资料不完整
	}

	// 3. 社交模式检查
	if features.Followers == 0 && features.Following == 0 {
		score += 0.15 // 无社交活动
	}

	if features.Following > 500 && features.Followers < 10 {
		score += 0.1 // 关注很多人但几乎没有粉丝
	}

	// 4. 仓库模式检查
	if features.PublicRepos == 0 {
		score += 0.15 // 无公开仓库
	}

	// 5. 名字模式检查（简单检查）
	if isBotLikeName(features.Login) {
		score += 0.2
	}

	// 限制在 0-1 范围内
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// isBotLikeName 检查是否像机器人名字
func isBotLikeName(login string) bool {
	// 检查常见机器人模式
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

// FindClusters 发现女巫集群
func (sd *SybilDetector) FindClusters() []*Cluster {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// 简单聚类算法：基于账户创建时间和行为相似度
	profiles := make([]*ProfileFeatures, 0, len(sd.seenProfiles))
	for _, p := range sd.seenProfiles {
		profiles = append(profiles, p)
	}

	// 按创建时间排序
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].CreatedAt.Before(profiles[j].CreatedAt)
	})

	clusters := make([]*Cluster, 0)

	// 时间窗口聚类（24小时内创建的相似账户）
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

// similar 检查两个特征是否相似
func (sd *SybilDetector) similar(a, b *ProfileFeatures) bool {
	// 检查账户创建时间差
	timeDiff := a.CreatedAt.Sub(b.CreatedAt)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	// 检查资料相似度
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

	// 至少两个特征相似
	return matchCount >= 2 || timeDiff < time.Hour
}

// GetSuspiciousClusters 获取可疑集群
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

// IsInCluster 检查用户是否在可疑集群中
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

// Clear 清除缓存
func (sd *SybilDetector) Clear() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.seenProfiles = make(map[string]*ProfileFeatures)
	sd.clusters = make([]*Cluster, 0)
	sd.clusterCounter = 0
}

// GetProfileCount 获取已分析的配置文件数量
func (sd *SybilDetector) GetProfileCount() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return len(sd.seenProfiles)
}
