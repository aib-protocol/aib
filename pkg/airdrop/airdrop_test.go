package airdrop

import (
	"fmt"
	"testing"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

// TestScorer 测试评分器
func TestScorer(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	// 创建测试用户信息
	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "testuser",
		Name:        "Test User",
		Email:       "test@example.com",
		Bio:         "A test user",
		Location:    "San Francisco",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour), // 1年前
		PublicRepos: 10,
		Followers:   50,
		Following:   30,
	}

	// 测试评分
	score := scorer.ScoreUser(userInfo, nil)

	if score == nil {
		t.Fatal("ScoreUser returned nil")
	}

	t.Logf("Total score: %d", score.Total)
	t.Logf("Account age score: %d", score.AccountAge)
	t.Logf("Social score: %d", score.Social)
	t.Logf("Repo score: %d", score.Repo)
	t.Logf("Contribution score: %d", score.Contribution)
	t.Logf("Is eligible: %v", score.IsEligible)
	t.Logf("Sybil score: %.2f", score.SybilScore)
}

// TestScorer_NewAccount 测试新账户评分
func TestScorer_NewAccount(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	// 创建新账户（7天前创建）
	userInfo := &GitHubUserInfo{
		ID:          12346,
		Login:       "newuser",
		Name:        "",
		Email:       "",
		Bio:         "",
		Location:    "",
		CreatedAt:   time.Now().Add(-7 * 24 * time.Hour),
		PublicRepos: 0,
		Followers:   0,
		Following:   0,
	}

	score := scorer.ScoreUser(userInfo, nil)

	t.Logf("New account - Total score: %d", score.Total)
	t.Logf("Is eligible: %v", score.IsEligible)
	t.Logf("Sybil score: %.2f", score.SybilScore)

	// 新账户应该分数较低或不可资格
	if score.IsEligible && score.SybilScore > 0.5 {
		t.Log("New account with suspicious profile correctly flagged")
	}
}

// TestSybilDetector 测试女巫攻击检测
func TestSybilDetector(t *testing.T) {
	detector := NewSybilDetector(0.7)

	// 创建一组相似账户（疑似女巫）
	baseTime := time.Now().Add(-30 * 24 * time.Hour)

	for i := 0; i < 10; i++ {
		userInfo := &GitHubUserInfo{
			ID:        uint64(1000 + i),
			Login:     "bot" + string(rune('0'+i)),
			CreatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			Followers: 0,
			Following: 0,
			PublicRepos: 0,
		}

		suspicionScore := detector.Detect(userInfo, nil)
		t.Logf("User %d: Suspicion score = %.2f", i, suspicionScore)
	}

	// 查找集群
	clusters := detector.FindClusters()
	t.Logf("Found %d clusters", len(clusters))

	for _, cluster := range clusters {
		t.Logf("Cluster %d: %d members", cluster.ID, cluster.Size)
	}

	suspiciousClusters := detector.GetSuspiciousClusters(3)
	t.Logf("Found %d suspicious clusters (size >= 3)", len(suspiciousClusters))
}

// TestEmailVerifier 测试邮箱验证
func TestEmailVerifier(t *testing.T) {
	verifier := NewEmailVerifier([]string{"example.com", "test.org"}, false)

	tests := []struct {
		email    string
		expected bool
	}{
		{"user@example.com", true},
		{"user@test.org", true},
		{"user@other.com", false},
		{"invalid-email", false},
		{"@example.com", false},
		{"user@", false},
		{"user@tempmail.com", false}, // 黑名单域名
	}

	for _, tt := range tests {
		valid, _ := verifier.VerifyEmail(tt.email)
		if valid != tt.expected {
			t.Errorf("VerifyEmail(%q) = %v, want %v", tt.email, valid, tt.expected)
		}
	}
}

// TestDeviceFingerprint 测试设备指纹
func TestDeviceFingerprint(t *testing.T) {
	df := NewDeviceFingerprint(24 * time.Hour)

	// 生成指纹
	fp1 := df.GenerateFingerprint("Mozilla/5.0", "en-US", "America/New_York")
	fp2 := df.GenerateFingerprint("Mozilla/5.0", "en-US", "America/New_York")
	fp3 := df.GenerateFingerprint("Different/Agent", "fr-FR", "Europe/Paris")

	if fp1 != fp2 {
		t.Error("Same inputs should produce same fingerprint")
	}

	if fp1 == fp3 {
		t.Error("Different inputs should produce different fingerprints")
	}

	// 测试注册
	ok1 := df.CheckAndRegister(fp1)
	if !ok1 {
		t.Error("First registration should succeed")
	}

	ok2 := df.CheckAndRegister(fp1)
	if ok2 {
		t.Error("Duplicate registration should fail")
	}

	ok3 := df.CheckAndRegister(fp3)
	if !ok3 {
		t.Error("New fingerprint registration should succeed")
	}
}

// TestIPLimiter 测试 IP 限制
func TestIPLimiter(t *testing.T) {
	limiter := NewIPLimiter(3, 24*time.Hour)

	// 测试认领
	for i := 0; i < 3; i++ {
		allowed, count := limiter.CheckAndRecord("192.168.1.1")
		if !allowed {
			t.Errorf("Claim %d should be allowed", i+1)
		}
		if count != i+1 {
			t.Errorf("Expected count %d, got %d", i+1, count)
		}
	}

	// 第4次应该被拒绝
	allowed, count := limiter.CheckAndRecord("192.168.1.1")
	if allowed {
		t.Error("4th claim should be denied")
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}

	// 不同 IP 应该可以
	allowed, count = limiter.CheckAndRecord("192.168.1.2")
	if !allowed {
		t.Error("Different IP should be allowed")
	}
	if count != 1 {
		t.Errorf("Expected count 1 for new IP, got %d", count)
	}
}

// TestDistributor 测试分发器
func TestDistributor(t *testing.T) {
	// 生成测试种子
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false, // 禁用签名以便测试
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// 测试金额计算
	amount := distributor.CalculateAmount(80, 100)
	if amount.Total == 0 {
		t.Error("Amount should not be zero for score 80")
	}
	t.Logf("Score 80: Base=%d, Bonus=%d, Total=%d", amount.Base, amount.Bonus, amount.Total)

	// 测试认领
	req := &ClaimRequest{
		Address:     "0x1234567890abcdef1234567890abcdef12345678",
		GitHubID:    12345,
		GitHubLogin: "testuser",
		Score:       80,
		Timestamp:   time.Now().Unix(),
	}

	record, err := distributor.Claim(req)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if record.Amount.Total != amount.Total {
		t.Errorf("Expected amount %d, got %d", amount.Total, record.Amount.Total)
	}

	// 测试重复认领
	_, err = distributor.Claim(req)
	if err != ErrAlreadyClaimed {
		t.Errorf("Expected ErrAlreadyClaimed, got %v", err)
	}

	// 测试统计
	stats := distributor.GetStats()
	t.Logf("Total claims: %d", stats.TotalClaims)
	t.Logf("Distributed: %d", stats.DistributedAmount)
	t.Logf("Remaining: %d", stats.RemainingAmount)
}

// TestDistributor_Signature 测试签名验证
func TestDistributor_Signature(t *testing.T) {
	// 生成测试种子
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   true,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// 创建签名器用于生成签名
	signer, err := NewAirdropSigner(seed)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	address := "0x1234567890abcdef1234567890abcdef12345678"
	amount := uint64(1000000)
	timestamp := time.Now().Unix()

	// 生成有效签名
	signature, err := signer.SignClaim(address, amount, timestamp)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	// 验证签名
	valid := distributor.VerifyClaimSignature(address, amount, timestamp, signature)
	if !valid {
		t.Error("Valid signature should be verified")
	}

	// 测试无效签名
	invalidSig := make([]byte, len(signature))
	copy(invalidSig, signature)
	invalidSig[0] ^= 0xFF

	valid = distributor.VerifyClaimSignature(address, amount, timestamp, invalidSig)
	if valid {
		t.Error("Invalid signature should not be verified")
	}
}

// TestAirdropSigner 测试签名器
func TestAirdropSigner(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	signer, err := NewAirdropSigner(seed)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	// 测试签名
	address := "0x1234567890abcdef1234567890abcdef12345678"
	amount := uint64(1000000)
	timestamp := time.Now().Unix()

	signature, err := signer.SignClaim(address, amount, timestamp)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	// 测试公钥
	pubKey := signer.PublicKey()
	if len(pubKey) != 32 {
		t.Errorf("Expected public key length 32, got %d", len(pubKey))
	}

	// 验证签名
	message := fmt.Sprintf("%s:%d:%d", address, amount, timestamp)
	valid := crypto.Ed25519Verify(pubKey, []byte(message), signature)
	if !valid {
		t.Error("Signature verification failed")
	}
}

// BenchmarkScorer 基准测试评分器
func BenchmarkScorer(b *testing.B) {
	scorer := NewScorer(DefaultScoringConfig())

	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "testuser",
		Name:        "Test User",
		Email:       "test@example.com",
		Bio:         "A test user",
		Location:    "San Francisco",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour),
		PublicRepos: 10,
		Followers:   50,
		Following:   30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.ScoreUser(userInfo, nil)
	}
}

// BenchmarkSybilDetector 基准测试女巫检测
func BenchmarkSybilDetector(b *testing.B) {
	detector := NewSybilDetector(0.7)

	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "testuser",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour),
		Followers:   50,
		Following:   30,
		PublicRepos: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(userInfo, nil)
	}
}
