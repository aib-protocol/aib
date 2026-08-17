// Package inference provides AI inference node services with lightning network payments.
package inference

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// TestNewInferenceNode tests creating a new inference node
func TestNewInferenceNode(t *testing.T) {
	// Generate a test public key
	var pubKey [32]byte
	rand.Read(pubKey[:])

	// Test level 1
	node := NewInferenceNode(pubKey, 1, 1000000)
	if node == nil {
		t.Fatal("NewInferenceNode returned nil")
	}

	if node.Level != 1 {
		t.Errorf("expected level 1, got %d", node.Level)
	}

	if node.Stake != 1000000 {
		t.Errorf("expected stake 1000000, got %d", node.Stake)
	}

	if node.Reputation != 5.0 {
		t.Errorf("expected default reputation 5.0, got %f", node.Reputation)
	}

	if node.IsOnline {
		t.Error("expected node to be offline initially")
	}

	if node.Stats.TotalInferences != 0 {
		t.Errorf("expected 0 total inferences, got %d", node.Stats.TotalInferences)
	}

	// Test level 2
	node2 := NewInferenceNode(pubKey, 2, 10000000)
	if node2.Level != 2 {
		t.Errorf("expected level 2, got %d", node2.Level)
	}

	// Test level 3
	node3 := NewInferenceNode(pubKey, 3, 100000000)
	if node3.Level != 3 {
		t.Errorf("expected level 3, got %d", node3.Level)
	}
}

// TestNewInferenceNodeWithConfig tests creating a node with full configuration
func TestNewInferenceNodeWithConfig(t *testing.T) {
	// Generate test key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	config := &NodeConfig{
		Level:       2,
		Stake:       10000000,
		PrivateKey:  privKey,
		MaxRequests: 200,
	}

	node, err := NewInferenceNodeWithConfig(config)
	if err != nil {
		t.Fatalf("NewInferenceNodeWithConfig failed: %v", err)
	}

	if node.Level != 2 {
		t.Errorf("expected level 2, got %d", node.Level)
	}

	if node.Stake != 10000000 {
		t.Errorf("expected stake 10000000, got %d", node.Stake)
	}

	if node.privateKey == nil {
		t.Error("expected private key to be set")
	}

	// Test invalid level
	invalidConfig := &NodeConfig{
		Level: 4,
		Stake: 1000000,
	}
	_, err = NewInferenceNodeWithConfig(invalidConfig)
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

// TestInferenceNodeRegister tests node registration
func TestInferenceNodeRegister(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Should succeed with sufficient stake
	err := node.Register()
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	if !node.IsOnline {
		t.Error("expected node to be online after registration")
	}

	// Should fail if already registered
	err = node.Register()
	if err != ErrNodeAlreadyRegistered {
		t.Errorf("expected ErrNodeAlreadyRegistered, got %v", err)
	}
}

// TestInferenceNodeRegisterInsufficientStake tests registration with insufficient stake
func TestInferenceNodeRegisterInsufficientStake(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	tests := []struct {
		level   uint8
		stake   uint64
		wantErr bool
	}{
		{1, 999999, true},
		{1, 1000000, false},
		{2, 9999999, true},
		{2, 10000000, false},
		{3, 99999999, true},
		{3, 100000000, false},
	}

	for _, tt := range tests {
		node := NewInferenceNode(pubKey, tt.level, tt.stake)
		err := node.Register()
		if tt.wantErr && err == nil {
			t.Errorf("level %d, stake %d: expected error but got none", tt.level, tt.stake)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("level %d, stake %d: unexpected error: %v", tt.level, tt.stake, err)
		}
	}
}

// TestInferenceNodeStart tests starting the inference service
func TestInferenceNodeStart(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Should fail if not registered
	ctx := context.Background()
	err := node.Start(ctx)
	if err != ErrNodeNotRegistered {
		t.Errorf("expected ErrNodeNotRegistered, got %v", err)
	}

	// Register and start
	err = node.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	err = node.Start(ctx)
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Stop node
	err = node.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestCalculatePrice tests price calculation based on level and reputation
func TestCalculatePrice(t *testing.T) {
	tests := []struct {
		level      uint8
		reputation float64
		wantBase   uint64
	}{
		{1, 5.0, 10000},
		{2, 5.0, 100000},
		{3, 5.0, 1000000},
		// Reputation 10 = 1.25x multiplier
		{1, 10.0, 12500},
		{2, 10.0, 125000},
		{3, 10.0, 1250000},
		// Reputation 0 = 0.75x multiplier
		{1, 0.0, 7500},
		{2, 0.0, 75000},
		{3, 0.0, 750000},
	}

	for _, tt := range tests {
		var pubKey [32]byte
		rand.Read(pubKey[:])
		node := NewInferenceNode(pubKey, tt.level, 1000000)
		node.Reputation = tt.reputation

		price := node.CalculatePrice()

		// Allow small floating point variance
		if price < tt.wantBase-100 || price > tt.wantBase+100 {
			t.Errorf("level %d, reputation %.1f: expected price near %d, got %d",
				tt.level, tt.reputation, tt.wantBase, price)
		}
	}
}

// TestHandleRequest tests handling inference requests
func TestHandleRequest(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)
	err := node.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()
	err = node.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer node.Stop()

	// Test valid request
	var reqID [32]byte
	rand.Read(reqID[:])
	var userPubKey [32]byte
	rand.Read(userPubKey[:])

	req := &InferenceRequest{
		RequestID:   reqID,
		UserPubKey:  userPubKey,
		Model:       "gpt-4",
		Prompt:      "Hello, world!",
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := node.HandleRequest(req)
	if err != nil {
		t.Errorf("HandleRequest failed: %v", err)
	}

	if resp.RequestID != reqID {
		t.Errorf("expected request ID %x, got %x", reqID, resp.RequestID)
	}

	if resp.Output == "" {
		t.Error("expected non-empty output")
	}

	if resp.Fee == 0 {
		t.Error("expected non-zero fee")
	}

	if resp.LatencyMs == 0 {
		t.Error("expected non-zero latency")
	}

	// Test nil request
	_, err = node.HandleRequest(nil)
	if err != ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest for nil request, got %v", err)
	}

	// Test empty prompt
	emptyReq := &InferenceRequest{
		RequestID: reqID,
		Prompt:    "",
	}
	_, err = node.HandleRequest(emptyReq)
	if err == nil {
		t.Error("expected error for empty prompt")
	}

	// Test invalid temperature
	invalidTempReq := &InferenceRequest{
		RequestID:   reqID,
		Prompt:      "test",
		Temperature: 3.0, // Too high
	}
	_, err = node.HandleRequest(invalidTempReq)
	if err == nil {
		t.Error("expected error for invalid temperature")
	}

	// Test offline node
	stoppedNode := NewInferenceNode(pubKey, 1, 1000000)
	_, err = stoppedNode.HandleRequest(req)
	if err != ErrNodeNotRegistered {
		t.Errorf("expected ErrNodeNotRegistered for offline node, got %v", err)
	}
}

// TestRecordSuccess tests recording successful inferences
func TestRecordSuccess(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	initialReputation := node.Reputation

	// Record some successes
	for i := 0; i < 10; i++ {
		node.RecordSuccess(uint64(100 + i*10))
	}

	stats := node.GetStats()
	if stats.TotalInferences != 10 {
		t.Errorf("expected 10 total inferences, got %d", stats.TotalInferences)
	}

	if stats.SuccessfulCount != 10 {
		t.Errorf("expected 10 successful, got %d", stats.SuccessfulCount)
	}

	if stats.FailedCount != 0 {
		t.Errorf("expected 0 failed, got %d", stats.FailedCount)
	}

	if stats.AvgLatencyMs == 0 {
		t.Error("expected non-zero average latency")
	}

	if node.Reputation <= initialReputation {
		t.Error("expected reputation to improve after successes")
	}
}

// TestRecordFailure tests recording failed inferences
func TestRecordFailure(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	initialReputation := node.Reputation

	// Record some failures
	for i := 0; i < 10; i++ {
		node.RecordFailure()
	}

	stats := node.GetStats()
	if stats.TotalInferences != 10 {
		t.Errorf("expected 10 total inferences, got %d", stats.TotalInferences)
	}

	if stats.SuccessfulCount != 0 {
		t.Errorf("expected 0 successful, got %d", stats.SuccessfulCount)
	}

	if stats.FailedCount != 10 {
		t.Errorf("expected 10 failed, got %d", stats.FailedCount)
	}

	if node.Reputation >= initialReputation {
		t.Error("expected reputation to decrease after failures")
	}

	// Reputation should not go below 0
	if node.Reputation < 0 {
		t.Error("expected reputation to not go below 0")
	}
}

// TestCalculateSuccessRate tests success rate calculation
func TestCalculateSuccessRate(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Default with no inferences should be 100%
	rate := node.CalculateSuccessRate()
	if rate != 1.0 {
		t.Errorf("expected 1.0 for no inferences, got %f", rate)
	}

	// Add successes and failures
	node.RecordSuccess(100)
	node.RecordSuccess(200)
	node.RecordFailure()
	node.RecordFailure()
	node.RecordFailure()

	rate = node.CalculateSuccessRate()
	expectedRate := 2.0 / 5.0
	if rate != expectedRate {
		t.Errorf("expected %f, got %f", expectedRate, rate)
	}
}

// TestNodeInfo tests getting node info
func TestNodeInfo(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 2, 50000000)
	node.Register()

	info := node.GetInfo()

	if info["level"].(uint8) != 2 {
		t.Errorf("expected level 2, got %v", info["level"])
	}

	if info["stake"].(uint64) != 50000000 {
		t.Errorf("expected stake 50000000, got %v", info["stake"])
	}

	if !info["is_online"].(bool) {
		t.Error("expected node to be online")
	}

	stats := info["stats"].(NodeStats)
	if stats.TotalInferences != 0 {
		t.Errorf("expected 0 inferences, got %d", stats.TotalInferences)
	}
}

// TestUpdateReputation tests reputation updates
func TestUpdateReputation(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)
	initialRep := node.Reputation // 5.0

	// Test weighted average update: old * 0.9 + new * 0.1
	node.UpdateReputation(8.0)
	expected := initialRep*0.9 + 8.0*0.1 // 5.0*0.9 + 8.0*0.1 = 5.3
	if node.Reputation < expected-0.01 || node.Reputation > expected+0.01 {
		t.Errorf("expected reputation ~%f, got %f", expected, node.Reputation)
	}

	// Test second update
	node.UpdateReputation(10.0)
	expected = expected*0.9 + 10.0*0.1 // 5.3*0.9 + 10.0*0.1 = 5.77
	if node.Reputation < expected-0.01 || node.Reputation > expected+0.01 {
		t.Errorf("expected reputation ~%f, got %f", expected, node.Reputation)
	}

	// Test boundary values - negative is clamped to 0
	prevRep := node.Reputation
	node.UpdateReputation(-1.0)
	expected = prevRep*0.9 + 0.0*0.1 // -1 is clamped to 0
	if node.Reputation < expected-0.01 || node.Reputation > expected+0.01 {
		t.Errorf("expected reputation ~%f for clamped negative input, got %f", expected, node.Reputation)
	}

	// Test boundary values - >10 is clamped to 10
	prevRep = node.Reputation
	node.UpdateReputation(15.0)
	expected = prevRep*0.9 + 10.0*0.1 // 15 is clamped to 10
	if node.Reputation < expected-0.01 || node.Reputation > expected+0.01 {
		t.Errorf("expected reputation ~%f for clamped >10 input, got %f", expected, node.Reputation)
	}
}

// TestConcurrentAccess tests concurrent access to node
func TestConcurrentAccess(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)
	node.Register()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := node.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer node.Stop()

	// Concurrent requests
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			var reqID [32]byte
			rand.Read(reqID[:])
			var userPubKey [32]byte
			rand.Read(userPubKey[:])

			req := &InferenceRequest{
				RequestID:   reqID,
				UserPubKey:  userPubKey,
				Prompt:      "test",
				MaxTokens:   100,
				Temperature: 0.7,
			}

			node.HandleRequest(req)
			done <- true
		}()
	}

	// Wait for all to complete
	timeout := time.After(10 * time.Second)
	for i := 0; i < 100; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Error("timeout waiting for requests")
			return
		}
	}

	// Verify stats
	stats := node.GetStats()
	if stats.TotalInferences != 100 {
		t.Errorf("expected 100 inferences, got %d", stats.TotalInferences)
	}
}

// TestUnregister tests node unregistration
func TestUnregister(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Should fail if not registered
	err := node.Unregister()
	if err != ErrNodeNotRegistered {
		t.Errorf("expected ErrNodeNotRegistered, got %v", err)
	}

	// Register then unregister
	err = node.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	err = node.Unregister()
	if err != nil {
		t.Errorf("Unregister failed: %v", err)
	}

	if node.IsOnline {
		t.Error("expected node to be offline after unregister")
	}

	// Double unregister should fail
	err = node.Unregister()
	if err != ErrNodeNotRegistered {
		t.Errorf("expected ErrNodeNotRegistered for double unregister, got %v", err)
	}
}

// TestUpdatePrice tests price update functionality
func TestUpdatePrice(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	// Initial price
	initialPrice := node.CalculatePrice()

	// Update reputation and price
	node.UpdateReputation(10.0)

	// Directly call update price (internal method)
	// We test this indirectly via CalculatePrice
	newPrice := node.CalculatePrice()

	if newPrice <= initialPrice {
		t.Errorf("expected price to increase with higher reputation, got %d <= %d", newPrice, initialPrice)
	}

	// Test price decreases with lower reputation
	node2 := NewInferenceNode(pubKey, 1, 1000000)
	lowRepPrice := node2.CalculatePrice()

	// Create another node with lower reputation
	var pubKey2 [32]byte
	rand.Read(pubKey2[:])
	node3 := NewInferenceNode(pubKey2, 1, 1000000)
	node3.Reputation = 0.0
	lowerRepPrice := node3.CalculatePrice()

	if lowerRepPrice >= lowRepPrice {
		t.Errorf("expected price to be lower with 0 reputation, got %d >= %d", lowerRepPrice, lowRepPrice)
	}
}

// TestSignResponse tests response signing
func TestSignResponse(t *testing.T) {
	// Generate test key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	var pubKey [32]byte
	copy(pubKey[:], privKey.Public().(ed25519.PublicKey))

	node, err := NewInferenceNodeWithConfig(&NodeConfig{
		Level:       1,
		Stake:       1000000,
		PrivateKey:  privKey,
		MaxRequests: 100,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	var reqID [32]byte
	rand.Read(reqID[:])

	resp := &InferenceResponse{
		RequestID: reqID,
		Output:    "test output",
		LatencyMs: 100,
		Fee:       10000,
	}

	// Sign the response
	sig, err := node.signResponse(resp)
	if err != nil {
		t.Fatalf("signResponse failed: %v", err)
	}

	if len(sig) == 0 {
		t.Error("expected non-empty signature")
	}

	// Test signing without private key
	node2 := NewInferenceNode(pubKey, 1, 1000000)
	resp2 := &InferenceResponse{
		RequestID: reqID,
		Output:    "test",
	}
	sig2, err := node2.signResponse(resp2)
	if err == nil {
		t.Error("expected error when signing without private key")
	}
	if sig2 != nil {
		t.Error("expected nil signature when no private key")
	}
}

// TestTruncateString tests string truncation
func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel..."},
		{"hello", 5, "hello"},
		{"hello", 0, "..."},
		{"", 5, ""},
		{"a", 1, "a"},
		{"abc", 2, "ab..."},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q",
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

// TestValidateRequest tests request validation
func TestValidateRequest(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 1, 1000000)

	tests := []struct {
		name    string
		req     *InferenceRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &InferenceRequest{
				RequestID:   [32]byte{1},
				Prompt:      "test prompt",
				MaxTokens:   100,
				Temperature: 0.7,
			},
			wantErr: false,
		},
		{
			name: "empty request ID",
			req: &InferenceRequest{
				RequestID: [32]byte{},
				Prompt:    "test",
			},
			wantErr: true,
		},
		{
			name: "empty prompt",
			req: &InferenceRequest{
				RequestID: [32]byte{1},
				Prompt:    "",
			},
			wantErr: true,
		},
		{
			name: "prompt too long",
			req: &InferenceRequest{
				RequestID: [32]byte{1},
				Prompt:    string(make([]byte, 10001)),
			},
			wantErr: true,
		},
		{
			name: "temperature too high",
			req: &InferenceRequest{
				RequestID:   [32]byte{1},
				Prompt:      "test",
				Temperature: 2.5,
			},
			wantErr: true,
		},
		{
			name: "temperature too low",
			req: &InferenceRequest{
				RequestID:   [32]byte{1},
				Prompt:      "test",
				Temperature: -0.5,
			},
			wantErr: true,
		},
		{
			name: "max tokens too high",
			req: &InferenceRequest{
				RequestID: [32]byte{1},
				Prompt:    "test",
				MaxTokens: 20000,
			},
			wantErr: true,
		},
		{
			name: "valid boundary temperature 0",
			req: &InferenceRequest{
				RequestID:   [32]byte{1},
				Prompt:      "test",
				Temperature: 0.0,
			},
			wantErr: false,
		},
		{
			name: "valid boundary temperature 2.0",
			req: &InferenceRequest{
				RequestID:   [32]byte{1},
				Prompt:      "test",
				Temperature: 2.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := node.validateRequest(tt.req)
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestGenerateMockResponse tests mock response generation
func TestGenerateMockResponse(t *testing.T) {
	var pubKey [32]byte
	rand.Read(pubKey[:])

	node := NewInferenceNode(pubKey, 2, 10000000)

	response := node.generateMockResponse("test prompt")

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Response should contain level info
	if !contains(response, "level 2") {
		t.Error("expected response to contain level info")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
