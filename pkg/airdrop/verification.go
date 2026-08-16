package airdrop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidGitHubToken 无效的 GitHub token
	ErrInvalidGitHubToken = errors.New("invalid github token")
	// ErrGitHubVerifyFailed GitHub 验证失败
	ErrGitHubVerifyFailed = errors.New("github verification failed")
	// ErrEmailVerifyFailed 邮箱验证失败
	ErrEmailVerifyFailed = errors.New("email verification failed")
	// ErrIPLimitExceeded IP 限制超出
	ErrIPLimitExceeded = errors.New("ip limit exceeded")
	// ErrDeviceFingerprintDuplicate 设备指纹重复
	ErrDeviceFingerprintDuplicate = errors.New("device fingerprint duplicate")
	// ErrVerificationPending 验证待处理
	ErrVerificationPending = errors.New("verification pending")
)

// GitHubUserInfo GitHub 用户信息
type GitHubUserInfo struct {
	ID        uint64    `json:"id"`
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
	Bio       string    `json:"bio"`
	Blog      string    `json:"blog"`
	Location  string    `json:"location"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	PublicRepos int     `json:"public_repos"`
	Followers   int     `json:"followers"`
	Following   int     `json:"following"`
}

// VerificationResult 验证结果
type VerificationResult struct {
	Success     bool           `json:"success"`
	UserInfo    *GitHubUserInfo `json:"user_info,omitempty"`
	Email       string         `json:"email,omitempty"`
	DeviceID    string         `json:"device_id,omitempty"`
	IPAddress   string         `json:"ip_address,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Score       int            `json:"score"`
	Reasons     []string       `json:"reasons,omitempty"`
}

// GitHubVerifier GitHub OAuth 验证器
type GitHubVerifier struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	cache        *VerificationCache
	mu           sync.RWMutex
}

// NewGitHubVerifier 创建 GitHub 验证器
func NewGitHubVerifier(clientID, clientSecret string) *GitHubVerifier {
	return &GitHubVerifier{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: NewVerificationCache(1000, 5*time.Minute),
	}
}

// VerifyToken 验证 GitHub OAuth token
func (v *GitHubVerifier) VerifyToken(token string) (*GitHubUserInfo, error) {
	if token == "" {
		return nil, ErrInvalidGitHubToken
	}

	// 检查缓存
	if cached, ok := v.cache.Get(token); ok {
		return cached, nil
	}

	// 创建请求
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AIB-Airdrop-Verifier/1.0")

	// 发送请求
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrGitHubVerifyFailed
	}

	// 解析响应
	var userInfo GitHubUserInfo
	if err := jsonDecode(resp.Body, &userInfo); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// 缓存结果
	v.cache.Set(token, &userInfo, 5*time.Minute)

	return &userInfo, nil
}

// VerificationCache 验证缓存
type VerificationCache struct {
	items map[string]*cacheItem
	mu    sync.RWMutex
	ttl   time.Duration
}

type cacheItem struct {
	value      *GitHubUserInfo
	expiration time.Time
}

// NewVerificationCache 创建验证缓存
func NewVerificationCache(size int, ttl time.Duration) *VerificationCache {
	return &VerificationCache{
		items: make(map[string]*cacheItem),
		ttl:   ttl,
	}
}

// Get 获取缓存
func (c *VerificationCache) Get(key string) (*GitHubUserInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(item.expiration) {
		return nil, false
	}

	return item.value, true
}

// Set 设置缓存
func (c *VerificationCache) Set(key string, value *GitHubUserInfo, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(ttl),
	}
}

// EmailVerifier 邮箱验证器
type EmailVerifier struct {
	domains          map[string]bool
	allowedDomains   []string
	blacklistedDomains map[string]bool
	verifyMX         bool
	httpClient       *http.Client
}

// NewEmailVerifier 创建邮箱验证器
func NewEmailVerifier(allowedDomains []string, verifyMX bool) *EmailVerifier {
	ev := &EmailVerifier{
		allowedDomains:     allowedDomains,
		domains:           make(map[string]bool),
		blacklistedDomains: make(map[string]bool),
		verifyMX:          verifyMX,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// 预加载允许的域名
	for _, domain := range allowedDomains {
		ev.domains[strings.ToLower(domain)] = true
	}

	// 预加载黑名单域名（一次性邮箱等）
	blacklist := []string{
		"tempmail.com", "guerrillamail.com", "mailinator.com",
		"10minutemail.com", "yopmail.com", "maildrop.cc",
		"throwaway.email", "getairmail.com", "fakeinbox.com",
	}
	for _, domain := range blacklist {
		ev.blacklistedDomains[strings.ToLower(domain)] = true
	}

	return ev
}

// VerifyEmail 验证邮箱
func (ev *EmailVerifier) VerifyEmail(email string) (bool, string) {
	email = strings.ToLower(strings.TrimSpace(email))

	// 基本格式验证
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false, "invalid email format"
	}

	local, domain := parts[0], parts[1]

	if local == "" || domain == "" {
		return false, "invalid email format"
	}

	// 检查黑名单域名
	if ev.blacklistedDomains[domain] {
		return false, "email domain is blacklisted"
	}

	// 检查允许的域名列表
	if len(ev.allowedDomains) > 0 && !ev.domains[domain] {
		return false, "email domain not allowed"
	}

	// 检查 MX 记录（可选）
	if ev.verifyMX {
		// TODO: 实现 MX 记录检查
		// 这需要 DNS 查询功能
	}

	return true, ""
}

// DeviceFingerprint 设备指纹识别器
type DeviceFingerprint struct {
	seen    map[string]time.Time
	mu      sync.RWMutex
	maxAge  time.Duration
}

// NewDeviceFingerprint 创建设备指纹识别器
func NewDeviceFingerprint(maxAge time.Duration) *DeviceFingerprint {
	return &DeviceFingerprint{
		seen:   make(map[string]time.Time),
		maxAge: maxAge,
	}
}

// GenerateFingerprint 生成设备指纹
// 基于 User-Agent, Accept-Language, 时区等
func (df *DeviceFingerprint) GenerateFingerprint(userAgent, acceptLang, timezone string) string {
	data := fmt.Sprintf("%s|%s|%s", userAgent, acceptLang, timezone)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// CheckAndRegister 检查并注册设备
func (df *DeviceFingerprint) CheckAndRegister(fingerprint string) bool {
	df.mu.Lock()
	defer df.mu.Unlock()

	now := time.Now()

	// 清理过期条目
	for fp, ts := range df.seen {
		if now.Sub(ts) > df.maxAge {
			delete(df.seen, fp)
		}
	}

	// 检查是否存在
	if _, exists := df.seen[fingerprint]; exists {
		return false
	}

	// 注册新设备
	df.seen[fingerprint] = now
	return true
}

// IPLimiter IP 限制器
type IPLimiter struct {
	claims    map[string]*IPRecord
	mu        sync.RWMutex
	maxClaims int
	window    time.Duration
}

// IPRecord IP 记录
type IPRecord struct {
	Count    int
	LastSeen time.Time
	FirstSeen time.Time
}

// NewIPLimiter 创建 IP 限制器
func NewIPLimiter(maxClaims int, window time.Duration) *IPLimiter {
	return &IPLimiter{
		claims:    make(map[string]*IPRecord),
		maxClaims: maxClaims,
		window:    window,
	}
}

// CheckAndRecord 检查并记录 IP
func (il *IPLimiter) CheckAndRecord(ip string) (bool, int) {
	il.mu.Lock()
	defer il.mu.Unlock()

	now := time.Now()

	// 清理过期记录
	for ipAddr, record := range il.claims {
		if now.Sub(record.LastSeen) > il.window {
			delete(il.claims, ipAddr)
		}
	}

	// 检查 IP 记录
	record, exists := il.claims[ip]
	if !exists {
		il.claims[ip] = &IPRecord{
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
		}
		return true, 1
	}

	// 检查是否超出限制
	if record.Count >= il.maxClaims {
		return false, record.Count
	}

	// 增加计数
	record.Count++
	record.LastSeen = now

	return true, record.Count
}

// GetRemaining 获取剩余认领次数
func (il *IPLimiter) GetRemaining(ip string) int {
	il.mu.RLock()
	defer il.mu.RUnlock()

	record, exists := il.claims[ip]
	if !exists {
		return il.maxClaims
	}

	remaining := il.maxClaims - record.Count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// NormalizeIP 规范化 IP 地址
func NormalizeIP(ip string) string {
	// 移除端口
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	// 处理 IPv6 映射的 IPv4
	if strings.HasPrefix(ip, "::ffff:") {
		ip = strings.TrimPrefix(ip, "::ffff:")
	}

	return ip
}

// Validator 综合验证器
type Validator struct {
	githubVerifier       *GitHubVerifier
	emailVerifier        *EmailVerifier
	deviceFingerprint    *DeviceFingerprint
	ipLimiter            *IPLimiter
	requireEmail         bool
	requireGitHub        bool
	scoreWeightGitHub    int
	scoreWeightEmail     int
	scoreWeightAccountAge int
}

// ValidatorConfig 验证器配置
type ValidatorConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	AllowedEmailDomains []string
	VerifyEmailMX      bool
	MaxClaimsPerIP     int
	IPWindow           time.Duration
	DeviceMaxAge       time.Duration
	RequireEmail       bool
	RequireGitHub      bool
	ScoreWeightGitHub    int
	ScoreWeightEmail     int
	ScoreWeightAccountAge int
}

// NewValidator 创建综合验证器
func NewValidator(config *ValidatorConfig) *Validator {
	return &Validator{
		githubVerifier:    NewGitHubVerifier(config.GitHubClientID, config.GitHubClientSecret),
		emailVerifier:     NewEmailVerifier(config.AllowedEmailDomains, config.VerifyEmailMX),
		deviceFingerprint: NewDeviceFingerprint(config.DeviceMaxAge),
		ipLimiter:         NewIPLimiter(config.MaxClaimsPerIP, config.IPWindow),
		requireEmail:      config.RequireEmail,
		requireGitHub:     config.RequireGitHub,
		scoreWeightGitHub:    config.ScoreWeightGitHub,
		scoreWeightEmail:     config.ScoreWeightEmail,
		scoreWeightAccountAge: config.ScoreWeightAccountAge,
	}
}

// ValidateUser 验证用户
func (v *Validator) ValidateUser(githubToken, email, deviceFingerprint, ipAddress string) (*VerificationResult, error) {
	result := &VerificationResult{
		Timestamp: time.Now(),
		IPAddress: ipAddress,
		DeviceID:  deviceFingerprint,
		Score:     0,
		Reasons:   make([]string, 0),
	}

	// 1. GitHub 验证（如果需要）
	if v.requireGitHub {
		if githubToken == "" {
			return nil, ErrInvalidGitHubToken
		}

		userInfo, err := v.githubVerifier.VerifyToken(githubToken)
		if err != nil {
			return nil, err
		}
		result.UserInfo = userInfo

		// 计算账户年龄分数
		accountAge := time.Since(userInfo.CreatedAt)
		ageScore := v.calculateAccountAgeScore(accountAge)
		result.Score += ageScore * v.scoreWeightAccountAge / 100
		result.Reasons = append(result.Reasons, fmt.Sprintf("账户年龄: %d 天", int(accountAge.Hours()/24)))

		// GitHub 基础分数
		result.Score += v.scoreWeightGitHub
		result.Reasons = append(result.Reasons, "GitHub 验证通过")
	}

	// 2. 邮箱验证（如果需要）
	if v.requireEmail && email != "" {
		valid, reason := v.emailVerifier.VerifyEmail(email)
		if !valid {
			return nil, errors.New(reason)
		}
		result.Email = email
		result.Score += v.scoreWeightEmail
		result.Reasons = append(result.Reasons, "邮箱验证通过")
	}

	// 3. 设备指纹检查
	isNew := v.deviceFingerprint.CheckAndRegister(deviceFingerprint)
	if !isNew {
		result.Reasons = append(result.Reasons, "设备已注册")
		return result, ErrDeviceFingerprintDuplicate
	}
	result.Reasons = append(result.Reasons, "新设备")

	// 4. IP 限制检查
	allowed, count := v.ipLimiter.CheckAndRecord(ipAddress)
	if !allowed {
		result.Reasons = append(result.Reasons, fmt.Sprintf("IP 已认领 %d 次", count))
		return result, ErrIPLimitExceeded
	}
	result.Reasons = append(result.Reasons, fmt.Sprintf("IP 认领次数: %d/%d", count, v.ipLimiter.maxClaims))

	result.Success = true
	return result, nil
}

// calculateAccountAgeScore 计算账户年龄分数
func (v *Validator) calculateAccountAgeScore(age time.Duration) int {
	days := int(age.Hours() / 24)

	// 评分规则
	switch {
	case days < 30:
		return 10 // 新账户
	case days < 90:
		return 30 // 3个月内
	case days < 180:
		return 50 // 6个月内
	case days < 365:
		return 70 // 1年内
	default:
		return 100 // 1年以上
	}
}

// GetIPLimitStatus 获取 IP 限制状态
func (v *Validator) GetIPLimitStatus(ip string) (remaining int, resetTime time.Time) {
	remaining = v.ipLimiter.GetRemaining(ip)
	// TODO: 实现重置时间计算
	return remaining, time.Now()
}

// jsonDecode JSON 解码辅助函数
func jsonDecode(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}
