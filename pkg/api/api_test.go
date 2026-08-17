package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/migration"
)

// ============================================================================
// Mock ChainReader for testing
// ============================================================================

type mockChainReader struct {
	height    uint64
	blockHash [32]byte
}

func newMockChainReader() *mockChainReader {
	return &mockChainReader{
		height:    1000,
		blockHash: [32]byte{1, 2, 3, 4},
	}
}

func (m *mockChainReader) GetHeight() uint64 {
	return m.height
}

func (m *mockChainReader) GetLatestBlock() Block {
	return nil
}

func (m *mockChainReader) GetBestBlockHeight() (uint64, error) {
	return m.height, nil
}

func (m *mockChainReader) GetBlockByHash(hash [32]byte) (Block, error) {
	return nil, nil
}

// mockBlock implements Block interface for testing
type mockBlock struct {
	height    uint64
	timestamp uint64
	hash      [32]byte
}

func (m *mockBlock) GetHeader() BlockHeader {
	return &mockBlockHeader{
		height:    m.height,
		timestamp: m.timestamp,
	}
}

func (m *mockBlock) GetHash() [32]byte {
	return m.hash
}

func (m *mockBlock) GetTransactions() int {
	return 5
}

type mockBlockHeader struct {
	height    uint64
	timestamp uint64
	prevHash  [32]byte
	proposer  [32]byte
}

func (h *mockBlockHeader) GetHeight() uint64 {
	return h.height
}

func (h *mockBlockHeader) GetTimestamp() uint64 {
	return h.timestamp
}

func (h *mockBlockHeader) GetPrevBlockHash() [32]byte {
	return h.prevHash
}

func (h *mockBlockHeader) GetProposer() [32]byte {
	return h.proposer
}

// mockMigrationHub implements MigrationHubAPI for testing
type mockMigrationHub struct {
	status *migration.MigrationStatus
}

func newMockMigrationHub() *mockMigrationHub {
	return &mockMigrationHub{
		status: &migration.MigrationStatus{
			AIB1TotalMigrated:    1000000,
			AIB1ClaimOpen:        true,
			AIB1ClaimDeadline:    time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			BTCWindowOpen:        true,
			BTCCurrentRate:       5,
			ETHWindowOpen:        true,
			ETHCurrentRate:       4,
			SOLWindowOpen:        true,
			SOLCurrentRate:       3,
			MigrationWindowStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			MigrationWindowEnd:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func (m *mockMigrationHub) GetMigrationStatus() *migration.MigrationStatus {
	return m.status
}

func (m *mockMigrationHub) GetUserMigrationInfo(addr interfaces.Address) *migration.UserMigrationInfo {
	return &migration.UserMigrationInfo{
		AIB1SnapshotBalance: 1000,
		AIB1Claimed:         false,
		TotalClaimable:      1000,
	}
}

func (m *mockMigrationHub) ClaimAIB1(targetAddr interfaces.Address, amount uint64, pubKey []byte, signature []byte, nonce uint64) error {
	return nil
}

func (m *mockMigrationHub) ClaimUnlocked(userAddr interfaces.Address) (uint64, error) {
	return 500, nil
}

func (m *mockMigrationHub) GetCrossChainRate(chain string) (uint64, error) {
	switch chain {
	case "BTC":
		return 5, nil
	case "ETH":
		return 4, nil
	case "SOL":
		return 3, nil
	default:
		return 0, nil
	}
}

func (m *mockMigrationHub) GetAIB1Balance(addr interfaces.Address) (uint64, bool) {
	return 1000, true
}

func (m *mockMigrationHub) IsAIB1Claimed(addr interfaces.Address) bool {
	return false
}

// ============================================================================
// Server Tests
// ============================================================================

func TestNewServer(t *testing.T) {
	server := NewServer(8080)

	if server == nil {
		t.Fatal("NewServer should not return nil")
	}

	if server.port != 8080 {
		t.Errorf("expected port 8080, got %d", server.port)
	}

	if server.mux == nil {
		t.Error("mux should be initialized")
	}

	if server.httpServer == nil {
		t.Error("httpServer should be initialized")
	}
}

func TestServer_SetGetChain(t *testing.T) {
	server := NewServer(8080)
	chain := newMockChainReader()

	server.SetChain(chain)

	if server.GetChain() != chain {
		t.Error("GetChain should return the set chain")
	}
}

func TestServer_SetGetMigrationHub(t *testing.T) {
	server := NewServer(8080)
	hub := newMockMigrationHub()

	server.SetMigrationHub(hub)

	// Verify hub was set (compare by checking it's not nil)
	retrieved := server.GetMigrationHub()
	if retrieved == nil {
		t.Error("GetMigrationHub should return non-nil hub")
	}
}

func TestServer_Uptime(t *testing.T) {
	server := NewServer(8080)

	uptime := server.Uptime()

	if uptime < 0 {
		t.Error("Uptime should be non-negative")
	}
}

func TestServer_RegisterRoutes(t *testing.T) {
	server := NewServer(8080)
	server.RegisterRoutes()

	// Test that routes are registered - health check should work
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// We need to call the handler directly
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Middleware Tests
// ============================================================================

func TestLoggingMiddleware(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	config := &Config{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Error("should set Access-Control-Allow-Origin")
	}
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	config := &Config{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("should set wildcard origin")
	}
}

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		origin  string
		allowed []string
		result  bool
	}{
		{"https://example.com", []string{"https://example.com"}, true},
		{"https://evil.com", []string{"https://example.com"}, false},
		{"", []string{"https://example.com"}, false},
		{"https://example.com", []string{}, false},
	}

	for _, tt := range tests {
		result := isOriginAllowed(tt.origin, tt.allowed)
		if result != tt.result {
			t.Errorf("isOriginAllowed(%s, %v) = %v, want %v", tt.origin, tt.allowed, result, tt.result)
		}
	}
}

func TestJoinStrings(t *testing.T) {
	result := joinStrings([]string{"GET", "POST", "PUT"})
	expected := "GET, POST, PUT"
	if result != expected {
		t.Errorf("joinStrings() = %s, want %s", result, expected)
	}
}

func TestRateLimitMiddleware_AllowsRequests(t *testing.T) {
	handler := RateLimitMiddleware(100, 100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTokenBucket_Take(t *testing.T) {
	bucket := NewTokenBucket(10, 5)

	// First 5 requests should succeed
	for i := 0; i < 5; i++ {
		if !bucket.Take("test") {
			t.Errorf("request %d should succeed", i)
		}
	}

	// 6th request should fail (rate limited)
	if bucket.Take("test") {
		t.Error("6th request should be rate limited")
	}

	// The bucket shares capacity across all keys - different key still fails
	if bucket.Take("other") {
		t.Error("other key should also be rate limited (shared bucket)")
	}
}

func TestAuthMiddleware_HealthEndpoint(t *testing.T) {
	handler := AuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Health endpoint should not require auth
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_GETNoAuth(t *testing.T) {
	handler := AuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// GET requests should not require auth
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	handler := AuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/transaction", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	handler := AuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/transaction", nil)
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	handler := AuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/transaction", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestOptionalAuthMiddleware_NoKey(t *testing.T) {
	handler := OptionalAuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/transaction", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// No key should be allowed with optional auth
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestOptionalAuthMiddleware_InvalidKey(t *testing.T) {
	handler := OptionalAuthMiddleware([]string{"test-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/transaction", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	ip := getClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "1.2.3.4")

	ip := getClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"

	ip := getClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", ip)
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("should set Content-Type to application/json")
	}
}

func TestWriteSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	writeSuccess(w, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should have Success = true")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request", "details")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Success {
		t.Error("response should have Success = false")
	}

	if resp.Error == nil {
		t.Error("response should have Error")
	}
}

func TestParsePathVar(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   string
	}{
		{"/v1/block/123", "/v1/block/", "123"},
		{"/v1/block/", "/v1/block/", ""},
		{"/v1/block", "/v1/block/", ""},
		{"/v1/block/abc/def", "/v1/block/", "abc/def"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		result := parsePathVar(req, tt.prefix)
		if result != tt.want {
			t.Errorf("parsePathVar(%s, %s) = %s, want %s", tt.path, tt.prefix, result, tt.want)
		}
	}
}

// ============================================================================
// Type Tests
// ============================================================================

func TestAPIResponse_NewSuccessResponse(t *testing.T) {
	resp := NewSuccessResponse("test data")

	if !resp.Success {
		t.Error("Success should be true")
	}

	if resp.Data != "test data" {
		t.Error("Data should be set")
	}

	if resp.Error != nil {
		t.Error("Error should be nil")
	}
}

func TestAPIResponse_NewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("CODE", "message", "details")

	if resp.Success {
		t.Error("Success should be false")
	}

	if resp.Error == nil {
		t.Error("Error should be set")
	}

	if resp.Error.Code != "CODE" {
		t.Errorf("Error.Code = %s, want CODE", resp.Error.Code)
	}
}

func TestErrorCodes(t *testing.T) {
	if ErrCodeInvalidRequest != "INVALID_REQUEST" {
		t.Errorf("ErrCodeInvalidRequest = %s", ErrCodeInvalidRequest)
	}
	if ErrCodeUnauthorized != "UNAUTHORIZED" {
		t.Errorf("ErrCodeUnauthorized = %s", ErrCodeUnauthorized)
	}
	if ErrCodeNotFound != "NOT_FOUND" {
		t.Errorf("ErrCodeNotFound = %s", ErrCodeNotFound)
	}
	if ErrCodeInternalError != "INTERNAL_ERROR" {
		t.Errorf("ErrCodeInternalError = %s", ErrCodeInternalError)
	}
}

func TestConfig_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}

	if cfg.RateLimitPerSecond != 100 {
		t.Errorf("RateLimitPerSecond = %d, want 100", cfg.RateLimitPerSecond)
	}

	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "*" {
		t.Error("AllowedOrigins should contain *")
	}

	if len(cfg.AllowedMethods) != 5 {
		t.Error("AllowedMethods should have 5 methods")
	}
}

func TestNewPaginationResponse(t *testing.T) {
	// Test case 1: evenly divisible
	p1 := NewPaginationResponse(1, 10, 100)
	if p1.TotalPage != 10 {
		t.Errorf("TotalPage = %d, want 10", p1.TotalPage)
	}

	// Test case 2: not evenly divisible
	p2 := NewPaginationResponse(1, 10, 105)
	if p2.TotalPage != 11 {
		t.Errorf("TotalPage = %d, want 11", p2.TotalPage)
	}

	// Test case 3: zero items
	p3 := NewPaginationResponse(1, 10, 0)
	if p3.TotalPage != 0 {
		t.Errorf("TotalPage = %d, want 0", p3.TotalPage)
	}
}

// ============================================================================
// Migration API Type Tests
// ============================================================================

func TestMigrationErrorCodes(t *testing.T) {
	if ErrCodeMigrationNotFound != "MIGRATION_NOT_FOUND" {
		t.Errorf("ErrCodeMigrationNotFound = %s", ErrCodeMigrationNotFound)
	}
	if ErrCodeMigrationWindowClosed != "MIGRATION_WINDOW_CLOSED" {
		t.Errorf("ErrCodeMigrationWindowClosed = %s", ErrCodeMigrationWindowClosed)
	}
	if ErrCodeAlreadyClaimed != "ALREADY_CLAIMED" {
		t.Errorf("ErrCodeAlreadyClaimed = %s", ErrCodeAlreadyClaimed)
	}
	if ErrCodeInvalidSignature != "INVALID_SIGNATURE" {
		t.Errorf("ErrCodeInvalidSignature = %s", ErrCodeInvalidSignature)
	}
}

// ============================================================================
// HTTP Endpoint Integration Tests
// ============================================================================

func TestHandleHealth(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Error("response should be successful")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", data["status"])
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("POST", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Success {
		t.Error("response should indicate failure")
	}
}

func TestHandleStatus(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())

	req := httptest.NewRequest("GET", "/v1/status", nil)
	w := httptest.NewRecorder()

	server.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["block_height"] != float64(1000) {
		t.Errorf("expected block_height 1000, got %v", data["block_height"])
	}
}

func TestHandleGetBalance(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/balance/0000000000000000000000000000000000000000000000000000000000000001", nil)
	w := httptest.NewRecorder()

	server.handleGetBalance(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["address"] != "0000000000000000000000000000000000000000000000000000000000000001" {
		t.Errorf("expected address, got %v", data["address"])
	}
}

func TestHandleGetBalance_MissingAddress(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/balance/", nil)
	w := httptest.NewRecorder()

	server.handleGetBalance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleSubmitTransaction(t *testing.T) {
	server := NewServer(8080)

	body := `{"from":"0xsender","to":"0xreceiver","amount":100}`
	req := httptest.NewRequest("POST", "/v1/transaction", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSubmitTransaction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["from"] != "0xsender" {
		t.Errorf("expected from, got %v", data["from"])
	}
	if data["to"] != "0xreceiver" {
		t.Errorf("expected to, got %v", data["to"])
	}
	if data["amount"] != float64(100) {
		t.Errorf("expected amount 100, got %v", data["amount"])
	}
}

func TestHandleSubmitTransaction_InvalidJSON(t *testing.T) {
	server := NewServer(8080)

	body := `invalid json`
	req := httptest.NewRequest("POST", "/v1/transaction", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSubmitTransaction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleSubmitTransaction_MissingFields(t *testing.T) {
	server := NewServer(8080)

	body := `{"from":"0xsender"}`
	req := httptest.NewRequest("POST", "/v1/transaction", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSubmitTransaction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleSubmitTransaction_WrongMethod(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/transaction", nil)
	w := httptest.NewRecorder()

	server.handleSubmitTransaction(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleGetLatestBlock(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())

	req := httptest.NewRequest("GET", "/v1/block/latest", nil)
	w := httptest.NewRecorder()

	server.handleGetLatestBlock(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	// Mock chain returns nil for GetLatestBlock, so should return default
	if data["height"] != float64(0) {
		t.Errorf("expected height 0 (default), got %v", data["height"])
	}
}

func TestHandleGetBlock(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/block/123", nil)
	w := httptest.NewRecorder()

	server.handleGetBlock(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["height"] != float64(123) {
		t.Errorf("expected height 123, got %v", data["height"])
	}
}

func TestHandleGetBlock_InvalidHeight(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/block/abc", nil)
	w := httptest.NewRecorder()

	server.handleGetBlock(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleGetBlock_MissingHeight(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/block/", nil)
	w := httptest.NewRecorder()

	server.handleGetBlock(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// Channel API Tests
// ============================================================================

func TestHandleChannelOpen(t *testing.T) {
	server := NewServer(8080)

	body := `{"party_a":"0xa","party_b":"0xb","deposit_a":100,"deposit_b":200}`
	req := httptest.NewRequest("POST", "/v1/channel/open", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelOpen(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["party_a"] != "0xa" {
		t.Errorf("expected party_a, got %v", data["party_a"])
	}
	if data["status"] != "open" {
		t.Errorf("expected status open, got %v", data["status"])
	}
}

func TestHandleChannelOpen_MissingFields(t *testing.T) {
	server := NewServer(8080)

	body := `{"party_a":"0xa"}`
	req := httptest.NewRequest("POST", "/v1/channel/open", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelOpen(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChannelOpen_InvalidJSON(t *testing.T) {
	server := NewServer(8080)

	body := `invalid json`
	req := httptest.NewRequest("POST", "/v1/channel/open", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelOpen(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChannelClose(t *testing.T) {
	server := NewServer(8080)

	body := `{"channel_id":"ch_123","sig_a":"YWJj","sig_b":"ZGVm"}`
	req := httptest.NewRequest("POST", "/v1/channel/close", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelClose(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleGetChannel(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/channel/ch_123", nil)
	w := httptest.NewRecorder()

	server.handleGetChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["id"] != "ch_123" {
		t.Errorf("expected channel id, got %v", data["id"])
	}
}

func TestHandleGetChannel_MissingID(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/channel/", nil)
	w := httptest.NewRecorder()

	server.handleGetChannel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleListChannels(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/channels", nil)
	w := httptest.NewRecorder()

	server.handleListChannels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleChannelPay(t *testing.T) {
	server := NewServer(8080)

	body := `{"channel_id":"ch_123","amount":50,"from_a":true}`
	req := httptest.NewRequest("POST", "/v1/channel/pay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelPay(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["status"] != "completed" {
		t.Errorf("expected status completed, got %v", data["status"])
	}
}

func TestHandleChannelPay_MissingFields(t *testing.T) {
	server := NewServer(8080)

	body := `{"channel_id":"ch_123"}`
	req := httptest.NewRequest("POST", "/v1/channel/pay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleChannelPay(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// AI Service API Tests
// ============================================================================

func TestHandleInference(t *testing.T) {
	server := NewServer(8080)

	body := `{"prompt":"Hello AI","model_id":"test-model","max_tokens":100}`
	req := httptest.NewRequest("POST", "/v1/ai/inference", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleInference(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["model_id"] != "test-model" {
		t.Errorf("expected model_id, got %v", data["model_id"])
	}
}

func TestHandleInference_MissingPrompt(t *testing.T) {
	server := NewServer(8080)

	body := `{"model_id":"test-model"}`
	req := httptest.NewRequest("POST", "/v1/ai/inference", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleInference(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleInference_InvalidJSON(t *testing.T) {
	server := NewServer(8080)

	body := `invalid json`
	req := httptest.NewRequest("POST", "/v1/ai/inference", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleInference(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleListModels(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/ai/models", nil)
	w := httptest.NewRecorder()

	server.handleListModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["total"] != float64(1) {
		t.Errorf("expected total 1, got %v", data["total"])
	}
}

func TestHandleListNodes(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/ai/nodes", nil)
	w := httptest.NewRecorder()

	server.handleListNodes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["total"] != float64(1) {
		t.Errorf("expected total 1, got %v", data["total"])
	}
}

// ============================================================================
// Migration API Tests
// ============================================================================

func TestHandleMigrationSnapshot(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/snapshot", nil)
	w := httptest.NewRecorder()

	server.handleMigrationSnapshot(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["claim_open"] != true {
		t.Errorf("expected claim_open true, got %v", data["claim_open"])
	}
}

func TestHandleMigrationSnapshot_NoHub(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/api/migration/snapshot", nil)
	w := httptest.NewRecorder()

	server.handleMigrationSnapshot(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleMigrationRates(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/rates", nil)
	w := httptest.NewRecorder()

	server.handleMigrationRates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["aib1_rate"] != float64(100) {
		t.Errorf("expected aib1_rate 100, got %v", data["aib1_rate"])
	}
}

func TestHandleMigrationStatus(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/status/0000000000000000000000000000000000000000000000000000000000000001", nil)
	w := httptest.NewRecorder()

	server.handleMigrationStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["aib1_snapshot_balance"] != float64(1000) {
		t.Errorf("expected balance 1000, got %v", data["aib1_snapshot_balance"])
	}
}

func TestHandleMigrationStatus_MissingAddress(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/status/", nil)
	w := httptest.NewRecorder()

	server.handleMigrationStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleMigrationStatus_InvalidAddress(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/status/invalid", nil)
	w := httptest.NewRecorder()

	server.handleMigrationStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleMigrationClaimable(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/claimable/0000000000000000000000000000000000000000000000000000000000000001", nil)
	w := httptest.NewRecorder()

	server.handleMigrationClaimable(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleClaimAIB1(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	body := `{"target_address":"0000000000000000000000000000000000000000000000000000000000000001","amount":100,"public_key":"YWJjZA==","signature":"c2ln","nonce":1}`
	req := httptest.NewRequest("POST", "/api/migration/claim-aib1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimAIB1(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["type"] != "aib1" {
		t.Errorf("expected type aib1, got %v", data["type"])
	}
}

func TestHandleClaimAIB1_MissingFields(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	body := `{"target_address":"0000000000000000000000000000000000000000000000000000000000000001"}`
	req := httptest.NewRequest("POST", "/api/migration/claim-aib1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimAIB1(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleClaimAIB1_InvalidBase64(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	body := `{"target_address":"0000000000000000000000000000000000000000000000000000000000000001","amount":100,"public_key":"!!!invalid!!!","signature":"c2ln","nonce":1}`
	req := httptest.NewRequest("POST", "/api/migration/claim-aib1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimAIB1(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleClaimAIB1_NoHub(t *testing.T) {
	server := NewServer(8080)

	body := `{"target_address":"0000000000000000000000000000000000000000000000000000000000000001","amount":100,"public_key":"YWJjZA==","signature":"c2ln","nonce":1}`
	req := httptest.NewRequest("POST", "/api/migration/claim-aib1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimAIB1(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleClaimUnlocked(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	body := `{"address":"0000000000000000000000000000000000000000000000000000000000000001"}`
	req := httptest.NewRequest("POST", "/api/migration/claim-unlocked", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimUnlocked(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleClaimUnlocked_MissingAddress(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	body := `{}`
	req := httptest.NewRequest("POST", "/api/migration/claim-unlocked", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimUnlocked(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleClaimUnlocked_NoHub(t *testing.T) {
	server := NewServer(8080)

	body := `{"address":"0000000000000000000000000000000000000000000000000000000000000001"}`
	req := httptest.NewRequest("POST", "/api/migration/claim-unlocked", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleClaimUnlocked(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleMigrationEstimate(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/estimate?chain=BTC&amount=1000", nil)
	w := httptest.NewRecorder()

	server.handleMigrationEstimate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data should be a map")
	}

	if data["source_chain"] != "BTC" {
		t.Errorf("expected source_chain BTC, got %v", data["source_chain"])
	}
	if data["source_amount"] != float64(1000) {
		t.Errorf("expected source_amount 1000, got %v", data["source_amount"])
	}
}

func TestHandleMigrationEstimate_MissingParams(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/estimate?chain=BTC", nil)
	w := httptest.NewRecorder()

	server.handleMigrationEstimate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleMigrationEstimate_InvalidChain(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/estimate?chain=INVALID&amount=1000", nil)
	w := httptest.NewRecorder()

	server.handleMigrationEstimate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleMigrationEstimate_InvalidAmount(t *testing.T) {
	server := NewServer(8080)
	server.SetMigrationHub(newMockMigrationHub())

	req := httptest.NewRequest("GET", "/api/migration/estimate?chain=BTC&amount=abc", nil)
	w := httptest.NewRecorder()

	server.handleMigrationEstimate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// JSON Serialization Tests
// ============================================================================

func TestJSONSerialization_HealthResponse(t *testing.T) {
	resp := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:   "2.0.0",
		Uptime:    "1h0m0s",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded HealthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Status != resp.Status {
		t.Errorf("expected status %s, got %s", resp.Status, decoded.Status)
	}
	if decoded.Version != resp.Version {
		t.Errorf("expected version %s, got %s", resp.Version, decoded.Version)
	}
}

func TestJSONSerialization_TransactionRequest(t *testing.T) {
	req := TransactionRequest{
		From:     "0xsender",
		To:       "0xreceiver",
		Amount:   100,
		GasLimit: 21000,
		GasPrice: 1000000000,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded TransactionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.From != req.From || decoded.To != req.To || decoded.Amount != req.Amount {
		t.Errorf("transaction request mismatch")
	}
}

func TestJSONSerialization_BlockResponse(t *testing.T) {
	resp := BlockResponse{
		Height:    100,
		Hash:      "abc123",
		PrevHash:  "def456",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TxCount:   5,
		Validator: "validator1",
		Size:      1024,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded BlockResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Height != resp.Height || decoded.TxCount != resp.TxCount {
		t.Errorf("block response mismatch")
	}
}

func TestJSONSerialization_ChannelResponse(t *testing.T) {
	resp := ChannelResponse{
		ID:        "ch_123",
		PartyA:    "0xa",
		PartyB:    "0xb",
		BalanceA:  100,
		BalanceB:  200,
		Sequence:  1,
		StateHash: "state123",
		Status:    "open",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ChannelResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != resp.ID || decoded.Status != resp.Status {
		t.Errorf("channel response mismatch")
	}
}

func TestJSONSerialization_InferenceRequest(t *testing.T) {
	req := InferenceRequest{
		Prompt:      "Hello AI",
		ModelID:     "test-model",
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded InferenceRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Prompt != req.Prompt || decoded.ModelID != req.ModelID {
		t.Errorf("inference request mismatch")
	}
}

func TestJSONSerialization_PaginationResponse(t *testing.T) {
	resp := NewPaginationResponse(1, 10, 105)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PaginationResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Page != 1 || decoded.PageSize != 10 || decoded.Total != 105 || decoded.TotalPage != 11 {
		t.Errorf("pagination response mismatch: %+v", decoded)
	}
}

func TestJSONSerialization_MigrationRatesResponse(t *testing.T) {
	resp := MigrationRatesResponse{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		AIB1Rate:  100,
		ChainRates: map[string]ChainRateInfo{
			"BTC": {Chain: "BTC", CurrentRate: 5, WindowOpen: true},
			"ETH": {Chain: "ETH", CurrentRate: 4, WindowOpen: true},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MigrationRatesResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.AIB1Rate != 100 || len(decoded.ChainRates) != 2 {
		t.Errorf("migration rates response mismatch")
	}
}

// ============================================================================
// End-to-End Integration Tests
// ============================================================================

func TestServer_EndToEnd(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())
	server.SetMigrationHub(newMockMigrationHub())
	server.RegisterRoutes()

	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test health endpoint
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test status endpoint
	resp, err = http.Get(ts.URL + "/v1/status")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test block endpoint
	resp, err = http.Get(ts.URL + "/v1/block/latest")
	if err != nil {
		t.Fatalf("failed to get block: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test balance endpoint
	resp, err = http.Get(ts.URL + "/v1/balance/0x123")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test models endpoint
	resp, err = http.Get(ts.URL + "/v1/ai/models")
	if err != nil {
		t.Fatalf("failed to get models: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test nodes endpoint
	resp, err = http.Get(ts.URL + "/v1/ai/nodes")
	if err != nil {
		t.Fatalf("failed to get nodes: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test migration snapshot endpoint
	resp, err = http.Get(ts.URL + "/api/migration/snapshot")
	if err != nil {
		t.Fatalf("failed to get snapshot: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test migration rates endpoint
	resp, err = http.Get(ts.URL + "/api/migration/rates")
	if err != nil {
		t.Fatalf("failed to get rates: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServer_EndToEnd_PostRequests(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())
	server.SetMigrationHub(newMockMigrationHub())
	server.RegisterRoutes()

	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test transaction submission
	resp, err := http.Post(ts.URL+"/v1/transaction", "application/json",
		strings.NewReader(`{"from":"0xsender","to":"0xreceiver","amount":100}`))
	if err != nil {
		t.Fatalf("failed to post transaction: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test channel open
	resp, err = http.Post(ts.URL+"/v1/channel/open", "application/json",
		strings.NewReader(`{"party_a":"0xa","party_b":"0xb","deposit_a":100,"deposit_b":200}`))
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test AI inference
	resp, err = http.Post(ts.URL+"/v1/ai/inference", "application/json",
		strings.NewReader(`{"prompt":"Hello AI"}`))
	if err != nil {
		t.Fatalf("failed to post inference: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test channel pay
	resp, err = http.Post(ts.URL+"/v1/channel/pay", "application/json",
		strings.NewReader(`{"channel_id":"ch_123","amount":50,"from_a":true}`))
	if err != nil {
		t.Fatalf("failed to post channel pay: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServer_EndToEnd_ErrorHandling(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())
	server.SetMigrationHub(newMockMigrationHub())
	server.RegisterRoutes()

	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test invalid JSON
	resp, err := http.Post(ts.URL+"/v1/transaction", "application/json",
		strings.NewReader(`invalid json`))
	if err != nil {
		t.Fatalf("failed to post: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test missing required fields
	resp, err = http.Post(ts.URL+"/v1/transaction", "application/json",
		strings.NewReader(`{"from":"0xsender"}`))
	if err != nil {
		t.Fatalf("failed to post: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test wrong method
	req, _ := http.NewRequest("PUT", ts.URL+"/v1/balance/0x123", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test not found
	resp, err = http.Get(ts.URL + "/v1/nonexistent")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================================
// Blockchain API Tests - New Endpoints
// ============================================================================

func TestHandleUTXOByAddress_Success(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/utxo/0000000000000000000000000000000000000000000000000000000000000001", nil)
	w := httptest.NewRecorder()

	server.handleUTXOByAddress(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}
}

func TestHandleUTXOByAddress_InvalidAddress(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/utxo/invalid", nil)
	w := httptest.NewRecorder()

	server.handleUTXOByAddress(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleUTXOByAddress_EmptyAddress(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/utxo/", nil)
	w := httptest.NewRecorder()

	server.handleUTXOByAddress(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleValidators_Success(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/validators", nil)
	w := httptest.NewRecorder()

	server.handleValidators(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}
}

func TestHandleValidators_MethodNotAllowed(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("POST", "/v1/validators", nil)
	w := httptest.NewRecorder()

	server.handleValidators(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleMempool_Success(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/mempool", nil)
	w := httptest.NewRecorder()

	server.handleMempool(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}
}

func TestHandleMempool_WithLimit(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/mempool?limit=50", nil)
	w := httptest.NewRecorder()

	server.handleMempool(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleMempool_MethodNotAllowed(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("POST", "/v1/mempool", nil)
	w := httptest.NewRecorder()

	server.handleMempool(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleStaking_Success(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/staking", nil)
	w := httptest.NewRecorder()

	server.handleStaking(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}
}

func TestHandleStaking_WithStakers(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/staking?include_stakers=true", nil)
	w := httptest.NewRecorder()

	server.handleStaking(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleStaking_MethodNotAllowed(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("POST", "/v1/staking", nil)
	w := httptest.NewRecorder()

	server.handleStaking(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleProposals_Success(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/proposals", nil)
	w := httptest.NewRecorder()

	server.handleProposals(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("response should be successful")
	}
}

func TestHandleProposals_WithStatusFilter(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("GET", "/v1/proposals?status=active", nil)
	w := httptest.NewRecorder()

	server.handleProposals(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleProposals_MethodNotAllowed(t *testing.T) {
	server := NewServer(8080)

	req := httptest.NewRequest("POST", "/v1/proposals", nil)
	w := httptest.NewRecorder()

	server.handleProposals(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// Test new endpoints in end-to-end test
func TestServer_EndToEnd_NewBlockchainEndpoints(t *testing.T) {
	server := NewServer(8080)
	server.SetChain(newMockChainReader())
	server.RegisterRoutes()

	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test UTXO endpoint
	resp, err := http.Get(ts.URL + "/v1/utxo/0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("failed to get UTXO: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test validators endpoint
	resp, err = http.Get(ts.URL + "/v1/validators")
	if err != nil {
		t.Fatalf("failed to get validators: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test mempool endpoint
	resp, err = http.Get(ts.URL + "/v1/mempool")
	if err != nil {
		t.Fatalf("failed to get mempool: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test staking endpoint
	resp, err = http.Get(ts.URL + "/v1/staking")
	if err != nil {
		t.Fatalf("failed to get staking: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test proposals endpoint
	resp, err = http.Get(ts.URL + "/v1/proposals")
	if err != nil {
		t.Fatalf("failed to get proposals: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
