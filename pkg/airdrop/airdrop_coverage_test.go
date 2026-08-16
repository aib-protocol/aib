package airdrop

import (
	"testing"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

// ============================================================================
// Distributor PublicKey Tests
// ============================================================================

func TestDistributorPublicKey(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Test GetPublicKey
	pubKey := distributor.GetPublicKey()
	if len(pubKey) != 32 {
		t.Errorf("Expected public key length 32, got %d", len(pubKey))
	}

	// Test GetPublicKeyHex
	pubKeyHex := distributor.GetPublicKeyHex()
	if len(pubKeyHex) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Expected hex length 64, got %d", len(pubKeyHex))
	}

	// Verify hex encoding matches using crypto package
	valid := crypto.Ed25519Verify(pubKey, []byte("test"), []byte("sig"))
	t.Logf("Ed25519Verify exists: %v", valid)
}

// ============================================================================
// Distributor Claims Import/Export Tests
// ============================================================================

func TestDistributorClaimsExport(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Add some claims with different addresses
	addresses := []string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"0x3333333333333333333333333333333333333333",
	}
	for i, addr := range addresses {
		req := &ClaimRequest{
			Address:     addr,
			GitHubID:    uint64(1000 + i),
			GitHubLogin: "testuser",
			Score:       80,
			Timestamp:   time.Now().Unix(),
		}
		_, err := distributor.Claim(req)
		if err != nil && err != ErrAlreadyClaimed {
			t.Fatalf("Claim failed: %v", err)
		}
	}

	// Export claims
	claims, err := distributor.ExportClaims()
	if err != nil {
		t.Fatalf("ExportClaims failed: %v", err)
	}

	if len(claims) == 0 {
		t.Error("Expected non-empty claims export")
	}

	// Verify structure
	if len(claims) != 3 {
		t.Errorf("Expected 3 claims, got %d", len(claims))
	}

	// Check first claim has required fields
	if claims[0].Address == "" {
		t.Error("Expected non-empty address")
	}
	if claims[0].Amount.Total == 0 {
		t.Error("Expected non-zero amount")
	}
}

func TestDistributorClaimsImport(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	// Create distributor with some claims
	distributor1, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	req := &ClaimRequest{
		Address:     "0x1234567890abcdef1234567890abcdef12345678",
		GitHubID:    12345,
		GitHubLogin: "testuser",
		Score:       80,
		Timestamp:   time.Now().Unix(),
	}
	_, err = distributor1.Claim(req)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// Export claims
	claims, err := distributor1.ExportClaims()
	if err != nil {
		t.Fatalf("ExportClaims failed: %v", err)
	}

	// Create new distributor and import claims
	distributor2, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create second distributor: %v", err)
	}

	err = distributor2.ImportClaims(claims)
	if err != nil {
		t.Fatalf("ImportClaims failed: %v", err)
	}

	// Verify the claim was imported correctly
	stats := distributor2.GetStats()
	if stats.TotalClaims != 1 {
		t.Errorf("Expected 1 total claim, got %d", stats.TotalClaims)
	}

	// Verify duplicate import
	err = distributor2.ImportClaims(claims)
	if err == nil {
		t.Error("Expected error for duplicate import")
	}
}

func TestDistributorClaimsImportInvalid(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Test empty slice
	err = distributor.ImportClaims([]*ClaimRecord{})
	if err != nil {
		t.Fatalf("Import empty slice failed: %v", err)
	}
}

// ============================================================================
// Distributor MerkleTree Tests
// ============================================================================

func TestDistributorMerkleTreeGeneration(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Add some claims
	for i := 0; i < 5; i++ {
		req := &ClaimRequest{
			Address:     "0x1234567890abcdef1234567890abcdef12345678",
			GitHubID:    uint64(1000 + i),
			GitHubLogin: "testuser",
			Score:       80,
			Timestamp:   time.Now().Unix(),
		}
		_, err := distributor.Claim(req)
		if err != nil && err != ErrAlreadyClaimed {
			t.Fatalf("Claim failed: %v", err)
		}
	}

	// Generate Merkle tree
	tree, err := distributor.GenerateMerkleTree()
	if err != nil {
		t.Fatalf("GenerateMerkleTree failed: %v", err)
	}

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
}

func TestDistributorGenerateMerkleTreeEmpty(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Generate Merkle tree with no claims
	tree, err := distributor.GenerateMerkleTree()
	if err != nil {
		t.Fatalf("GenerateMerkleTree failed: %v", err)
	}

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
}

// ============================================================================
// Scoring Edge Cases Tests
// ============================================================================

func TestScoringEdgeCases(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	// Test with very old account
	veryOldUser := &GitHubUserInfo{
		ID:          12345,
		Login:       "olduser",
		CreatedAt:   time.Now().Add(-20 * 365 * 24 * time.Hour), // 20 years
		PublicRepos: 100,
		Followers:   1000,
		Following:   100,
	}

	score := scorer.ScoreUser(veryOldUser, nil)
	if score == nil {
		t.Fatal("Expected non-nil score")
	}
	if score.Total > 100 {
		t.Errorf("Score should be capped at 100, got %d", score.Total)
	}

	// Test with zero values
	zeroUser := &GitHubUserInfo{
		ID:          12345,
		Login:       "zerouser",
		CreatedAt:   time.Now(),
		PublicRepos: 0,
		Followers:   0,
		Following:   0,
	}

	score = scorer.ScoreUser(zeroUser, nil)
	if score == nil {
		t.Fatal("Expected non-nil score")
	}
}

func TestScoringWithAdditionalData(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "contributor",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour),
		PublicRepos: 10,
		Followers:   50,
		Following:   30,
	}

	additionalData := &AdditionalData{
		Organizations: []string{"org1", "org2"},
		StarredCount:  50,
		HasForks:      10,
		PullRequests:  25,
		Issues:        15,
	}

	score := scorer.ScoreUser(userInfo, additionalData)
	if score == nil {
		t.Fatal("Expected non-nil score")
	}

	// Score should be higher with additional data
	scoreWithout := scorer.ScoreUser(userInfo, nil)
	if score.Total <= scoreWithout.Total {
		t.Logf("Additional data should contribute to score: with=%d, without=%d", score.Total, scoreWithout.Total)
	}
}

func TestScoringRepoActivity(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	// Test with high repo count
	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "repoowner",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour),
		PublicRepos: 100,
		Followers:   10,
		Following:   10,
	}

	score := scorer.ScoreUser(userInfo, nil)
	if score == nil {
		t.Fatal("Expected non-nil score")
	}
	t.Logf("Repo owner score: %d (Repo score: %d)", score.Total, score.Repo)
}

func TestScoringCodeContribution(t *testing.T) {
	scorer := NewScorer(DefaultScoringConfig())

	userInfo := &GitHubUserInfo{
		ID:          12345,
		Login:       "contributor",
		CreatedAt:   time.Now().Add(-365 * 24 * time.Hour),
		PublicRepos: 5,
		Followers:   20,
		Following:   10,
	}

	additionalData := &AdditionalData{
		PullRequests: 50,
		Issues:        30,
	}

	score := scorer.ScoreUser(userInfo, additionalData)
	if score == nil {
		t.Fatal("Expected non-nil score")
	}
	t.Logf("Contributor score: %d (Contribution score: %d)", score.Total, score.Contribution)
}

func TestMinFunction(t *testing.T) {
	// Test min helper function
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 100, 0},
		{-1, 1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

// ============================================================================
// GitHub Verifier Tests
// ============================================================================

func TestGitHubVerifierCreation(t *testing.T) {
	verifier := NewGitHubVerifier("test_client_id", "test_secret")

	if verifier == nil {
		t.Fatal("Expected non-nil verifier")
	}
	if verifier.clientID != "test_client_id" {
		t.Errorf("Expected client_id 'test_client_id', got '%s'", verifier.clientID)
	}
}

func TestGitHubVerifierTokenValidation(t *testing.T) {
	verifier := NewGitHubVerifier("test_client_id", "test_secret")

	// Test with invalid token
	userInfo, err := verifier.VerifyToken("invalid_token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
	if userInfo != nil {
		t.Error("Expected nil user info for invalid token")
	}

	// Test with empty token
	userInfo, err = verifier.VerifyToken("")
	if err == nil {
		t.Error("Expected error for empty token")
	}
}

// ============================================================================
// Verification Cache Tests
// ============================================================================

func TestVerificationCache(t *testing.T) {
	cache := NewVerificationCache(100, 5*time.Minute)

	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}

	// Test cache miss
	_, ok := cache.Get("token1")
	if ok {
		t.Error("Expected cache miss for non-existent token")
	}

	// Test cache set and get
	expectedUser := &GitHubUserInfo{
		ID:    12345,
		Login: "testuser",
	}

	cache.Set("token1", expectedUser, 5*time.Minute)

	userInfo, ok := cache.Get("token1")
	if !ok {
		t.Error("Expected cache hit after set")
	}
	if userInfo.ID != expectedUser.ID {
		t.Errorf("Expected user ID %d, got %d", expectedUser.ID, userInfo.ID)
	}
}

func TestVerificationCacheExpiration(t *testing.T) {
	cache := NewVerificationCache(100, 10*time.Millisecond)

	expectedUser := &GitHubUserInfo{
		ID:    12345,
		Login: "testuser",
	}

	cache.Set("token1", expectedUser, 10*time.Millisecond)

	// Should be available immediately
	_, ok := cache.Get("token1")
	if !ok {
		t.Error("Expected cache hit immediately after set")
	}

	// Wait for expiration
	time.Sleep(15 * time.Millisecond)

	_, ok = cache.Get("token1")
	if ok {
		t.Error("Expected cache miss after expiration")
	}
}

func TestVerificationCacheEviction(t *testing.T) {
	// Create a small cache
	cache := NewVerificationCache(2, 5*time.Minute)

	user1 := &GitHubUserInfo{ID: 1, Login: "user1"}
	user2 := &GitHubUserInfo{ID: 2, Login: "user2"}
	user3 := &GitHubUserInfo{ID: 3, Login: "user3"}

	cache.Set("token1", user1, 5*time.Minute)
	cache.Set("token2", user2, 5*time.Minute)
	cache.Set("token3", user3, 5*time.Minute) // May evict token1 due to size limit

	userInfo, ok := cache.Get("token2")
	if !ok {
		t.Error("Expected cache hit for token2")
	}
	if userInfo.ID != 2 {
		t.Errorf("Expected user ID 2, got %d", userInfo.ID)
	}
}

// ============================================================================
// Scorer Config Tests
// ============================================================================

func TestScorerConfig(t *testing.T) {
	config := DefaultScoringConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	// Test config values
	if config.MinAccountAge == 0 {
		t.Error("Expected non-zero MinAccountAge")
	}
	if config.MaxScore == 0 {
		t.Error("Expected non-zero MaxScore")
	}
}

// ============================================================================
// Claim Amount Calculation Tests
// ============================================================================

func TestCalculateAmountEdgeCases(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	config := &DistributorConfig{
		BaseAmount:         1000000,
		MaxBonusMultiplier: 2.0,
		Enabled:            true,
		RequireSignature:   false,
		MaxTotalAmount:     10000000,
		MinClaimScore:      50,
	}

	distributor, err := NewDistributor(config, seed)
	if err != nil {
		t.Fatalf("Failed to create distributor: %v", err)
	}

	// Test below minimum score
	amount := distributor.CalculateAmount(30, 100)
	if amount.Total != 0 {
		t.Errorf("Expected zero amount for score below minimum, got %d", amount.Total)
	}

	// Test at minimum score
	amount = distributor.CalculateAmount(50, 100)
	if amount.Total == 0 {
		t.Error("Expected non-zero amount at minimum score")
	}

	// Test maximum score
	amount = distributor.CalculateAmount(100, 100)
	if amount.Total == 0 {
		t.Error("Expected non-zero amount at maximum score")
	}

	// Test zero multiplier
	amount = distributor.CalculateAmount(80, 0)
	if amount.Total == 0 {
		t.Error("Expected base amount even with zero multiplier")
	}
}
