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
	// ErrInvalidGitHubToken invalid GitHub token
	ErrInvalidGitHubToken = errors.New("invalid github token")
	// ErrGitHubVerifyFailed GitHub verifyfailure
	ErrGitHubVerifyFailed = errors.New("github verification failed")
	// ErrEmailVerifyFailed email verification failed
	ErrEmailVerifyFailed = errors.New("email verification failed")
	// ErrIPLimitExceeded IP limit exceeded
	ErrIPLimitExceeded = errors.New("ip limit exceeded")
	// ErrDeviceFingerprintDuplicate device fingerprint duplicate
	ErrDeviceFingerprintDuplicate = errors.New("device fingerprint duplicate")
	// ErrVerificationPending verification pending
	ErrVerificationPending = errors.New("verification pending")
)

// GitHubUserInfo GitHub user info
type GitHubUserInfo struct {
	ID          uint64    `json:"id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	Blog        string    `json:"blog"`
	Location    string    `json:"location"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PublicRepos int       `json:"public_repos"`
	Followers   int       `json:"followers"`
	Following   int       `json:"following"`
}

// VerificationResult verifyresult
type VerificationResult struct {
	Success   bool            `json:"success"`
	UserInfo  *GitHubUserInfo `json:"user_info,omitempty"`
	Email     string          `json:"email,omitempty"`
	DeviceID  string          `json:"device_id,omitempty"`
	IPAddress string          `json:"ip_address,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Score     int             `json:"score"`
	Reasons   []string        `json:"reasons,omitempty"`
}

// GitHubVerifier GitHub OAuth verifier
type GitHubVerifier struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	cache        *VerificationCache
	mu           sync.RWMutex
}

// NewGitHubVerifier create GitHub verify
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

// VerifyToken verify GitHub OAuth token
func (v *GitHubVerifier) VerifyToken(token string) (*GitHubUserInfo, error) {
	if token == "" {
		return nil, ErrInvalidGitHubToken
	}

	// check cache
	if cached, ok := v.cache.Get(token); ok {
		return cached, nil
	}

	// createrequest
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AIB-Airdrop-Verifier/1.0")

	// sendrequest
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrGitHubVerifyFailed
	}

	// parseresponse
	var userInfo GitHubUserInfo
	if err := jsonDecode(resp.Body, &userInfo); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// cache result
	v.cache.Set(token, &userInfo, 5*time.Minute)

	return &userInfo, nil
}

// VerificationCache verifycache
type VerificationCache struct {
	items map[string]*cacheItem
	mu    sync.RWMutex
	ttl   time.Duration
}

type cacheItem struct {
	value      *GitHubUserInfo
	expiration time.Time
}

// NewVerificationCache createverifycache
func NewVerificationCache(size int, ttl time.Duration) *VerificationCache {
	return &VerificationCache{
		items: make(map[string]*cacheItem),
		ttl:   ttl,
	}
}

// Get getcache
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

// Set setcache
func (c *VerificationCache) Set(key string, value *GitHubUserInfo, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(ttl),
	}
}

// EmailVerifier email verifier
type EmailVerifier struct {
	domains            map[string]bool
	allowedDomains     []string
	blacklistedDomains map[string]bool
	verifyMX           bool
	httpClient         *http.Client
}

// NewEmailVerifier creates an email verifier
func NewEmailVerifier(allowedDomains []string, verifyMX bool) *EmailVerifier {
	ev := &EmailVerifier{
		allowedDomains:     allowedDomains,
		domains:            make(map[string]bool),
		blacklistedDomains: make(map[string]bool),
		verifyMX:           verifyMX,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// preload allowed domains
	for _, domain := range allowedDomains {
		ev.domains[strings.ToLower(domain)] = true
	}

	// preload blacklisted domains (disposable email services, etc.)
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

// VerifyEmail verifies an email address
func (ev *EmailVerifier) VerifyEmail(email string) (bool, string) {
	email = strings.ToLower(strings.TrimSpace(email))

	// basic format validation
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false, "invalid email format"
	}

	local, domain := parts[0], parts[1]

	if local == "" || domain == "" {
		return false, "invalid email format"
	}

	// check blacklisted domains
	if ev.blacklistedDomains[domain] {
		return false, "email domain is blacklisted"
	}

	// check allowed domain list
	if len(ev.allowedDomains) > 0 && !ev.domains[domain] {
		return false, "email domain not allowed"
	}

	// check MX records (optional)
	if ev.verifyMX {
		// TODO: implements MX recordcheck
		// this requires DNS query support
	}

	return true, ""
}

// DeviceFingerprint device fingerprint identifier
type DeviceFingerprint struct {
	seen   map[string]time.Time
	mu     sync.RWMutex
	maxAge time.Duration
}

// NewDeviceFingerprint creates a device fingerprint identifier
func NewDeviceFingerprint(maxAge time.Duration) *DeviceFingerprint {
	return &DeviceFingerprint{
		seen:   make(map[string]time.Time),
		maxAge: maxAge,
	}
}

// GenerateFingerprint generates a device fingerprint
// based on User-Agent, Accept-Language, timezone, etc.
func (df *DeviceFingerprint) GenerateFingerprint(userAgent, acceptLang, timezone string) string {
	data := fmt.Sprintf("%s|%s|%s", userAgent, acceptLang, timezone)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// CheckAndRegister checks and registers a device
func (df *DeviceFingerprint) CheckAndRegister(fingerprint string) bool {
	df.mu.Lock()
	defer df.mu.Unlock()

	now := time.Now()

	// clean up expired entries
	for fp, ts := range df.seen {
		if now.Sub(ts) > df.maxAge {
			delete(df.seen, fp)
		}
	}

	// check if it exists
	if _, exists := df.seen[fingerprint]; exists {
		return false
	}

	// register new device
	df.seen[fingerprint] = now
	return true
}

// IPLimiter IP limiter
type IPLimiter struct {
	claims    map[string]*IPRecord
	mu        sync.RWMutex
	maxClaims int
	window    time.Duration
}

// IPRecord IP record
type IPRecord struct {
	Count     int
	LastSeen  time.Time
	FirstSeen time.Time
}

// NewIPLimiter create IP limit
func NewIPLimiter(maxClaims int, window time.Duration) *IPLimiter {
	return &IPLimiter{
		claims:    make(map[string]*IPRecord),
		maxClaims: maxClaims,
		window:    window,
	}
}

// CheckAndRecord checks and records an IP
func (il *IPLimiter) CheckAndRecord(ip string) (bool, int) {
	il.mu.Lock()
	defer il.mu.Unlock()

	now := time.Now()

	// clean up expired records
	for ipAddr, record := range il.claims {
		if now.Sub(record.LastSeen) > il.window {
			delete(il.claims, ipAddr)
		}
	}

	// check IP record
	record, exists := il.claims[ip]
	if !exists {
		il.claims[ip] = &IPRecord{
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
		}
		return true, 1
	}

	// check if limit is exceeded
	if record.Count >= il.maxClaims {
		return false, record.Count
	}

	// increment count
	record.Count++
	record.LastSeen = now

	return true, record.Count
}

// GetRemaining returns the remaining claim count
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

// NormalizeIP normalizes an IP address
func NormalizeIP(ip string) string {
	// remove port
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	// handle IPv6-mapped IPv4
	if strings.HasPrefix(ip, "::ffff:") {
		ip = strings.TrimPrefix(ip, "::ffff:")
	}

	return ip
}

// Validator comprehensive validator
type Validator struct {
	githubVerifier        *GitHubVerifier
	emailVerifier         *EmailVerifier
	deviceFingerprint     *DeviceFingerprint
	ipLimiter             *IPLimiter
	requireEmail          bool
	requireGitHub         bool
	scoreWeightGitHub     int
	scoreWeightEmail      int
	scoreWeightAccountAge int
}

// ValidatorConfig verifyconfig
type ValidatorConfig struct {
	GitHubClientID        string
	GitHubClientSecret    string
	AllowedEmailDomains   []string
	VerifyEmailMX         bool
	MaxClaimsPerIP        int
	IPWindow              time.Duration
	DeviceMaxAge          time.Duration
	RequireEmail          bool
	RequireGitHub         bool
	ScoreWeightGitHub     int
	ScoreWeightEmail      int
	ScoreWeightAccountAge int
}

// NewValidator creates a comprehensive validator
func NewValidator(config *ValidatorConfig) *Validator {
	return &Validator{
		githubVerifier:        NewGitHubVerifier(config.GitHubClientID, config.GitHubClientSecret),
		emailVerifier:         NewEmailVerifier(config.AllowedEmailDomains, config.VerifyEmailMX),
		deviceFingerprint:     NewDeviceFingerprint(config.DeviceMaxAge),
		ipLimiter:             NewIPLimiter(config.MaxClaimsPerIP, config.IPWindow),
		requireEmail:          config.RequireEmail,
		requireGitHub:         config.RequireGitHub,
		scoreWeightGitHub:     config.ScoreWeightGitHub,
		scoreWeightEmail:      config.ScoreWeightEmail,
		scoreWeightAccountAge: config.ScoreWeightAccountAge,
	}
}

// ValidateUser verifyuser
func (v *Validator) ValidateUser(githubToken, email, deviceFingerprint, ipAddress string) (*VerificationResult, error) {
	result := &VerificationResult{
		Timestamp: time.Now(),
		IPAddress: ipAddress,
		DeviceID:  deviceFingerprint,
		Score:     0,
		Reasons:   make([]string, 0),
	}

	// 1. GitHub verification (if required)
	if v.requireGitHub {
		if githubToken == "" {
			return nil, ErrInvalidGitHubToken
		}

		userInfo, err := v.githubVerifier.VerifyToken(githubToken)
		if err != nil {
			return nil, err
		}
		result.UserInfo = userInfo

		// calculate account age score
		accountAge := time.Since(userInfo.CreatedAt)
		ageScore := v.calculateAccountAgeScore(accountAge)
		result.Score += ageScore * v.scoreWeightAccountAge / 100
		result.Reasons = append(result.Reasons, fmt.Sprintf("account age: %d days", int(accountAge.Hours()/24)))

		// GitHub base score
		result.Score += v.scoreWeightGitHub
		result.Reasons = append(result.Reasons, "GitHub verification passed")
	}

	// 2. Email verification (if required)
	if v.requireEmail && email != "" {
		valid, reason := v.emailVerifier.VerifyEmail(email)
		if !valid {
			return nil, errors.New(reason)
		}
		result.Email = email
		result.Score += v.scoreWeightEmail
		result.Reasons = append(result.Reasons, "email verification passed")
	}

	// 3. Device fingerprint check
	isNew := v.deviceFingerprint.CheckAndRegister(deviceFingerprint)
	if !isNew {
		result.Reasons = append(result.Reasons, "device already registered")
		return result, ErrDeviceFingerprintDuplicate
	}
	result.Reasons = append(result.Reasons, "new device")

	// 4. IP limit check
	allowed, count := v.ipLimiter.CheckAndRecord(ipAddress)
	if !allowed {
		result.Reasons = append(result.Reasons, fmt.Sprintf("IP already claimed %d times", count))
		return result, ErrIPLimitExceeded
	}
	result.Reasons = append(result.Reasons, fmt.Sprintf("IP claim count: %d/%d", count, v.ipLimiter.maxClaims))

	result.Success = true
	return result, nil
}

// calculateAccountAgeScore calculates account age score
func (v *Validator) calculateAccountAgeScore(age time.Duration) int {
	days := int(age.Hours() / 24)

	// scoring rules
	switch {
	case days < 30:
		return 10 // new account
	case days < 90:
		return 30 // within 3 months
	case days < 180:
		return 50 // within 6 months
	case days < 365:
		return 70 // within 1 year
	default:
		return 100 // over 1 year
	}
}

// GetIPLimitStatus get IP limitstatus
func (v *Validator) GetIPLimitStatus(ip string) (remaining int, resetTime time.Time) {
	remaining = v.ipLimiter.GetRemaining(ip)
	// TODO: implement reset time calculation
	return remaining, time.Now()
}

// jsonDecode JSON decode helper function
func jsonDecode(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}
