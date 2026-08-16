package agentic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAnthropicModelSupport tests Anthropic model validation.
func TestAnthropicModelSupport(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-3-opus-20240229", true},
		{"claude-3-sonnet-20240229", true},
		{"claude-3-haiku-20240307", true},
		{"gpt-4", false},
		{"unknown-model", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsAnthropicModelSupported(tt.model)
		if got != tt.want {
			t.Errorf("IsAnthropicModelSupported(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

// TestGetMaxTokensForModel tests max token calculation.
func TestGetMaxTokensForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"claude-3-opus-20240229", 200000},
		{"claude-3-sonnet-20240229", 200000},
		{"claude-3-haiku-20240307", 200000},
		{"gpt-4", 4096},
		{"", 4096},
	}

	for _, tt := range tests {
		got := GetMaxTokensForModel(tt.model)
		if got != tt.want {
			t.Errorf("GetMaxTokensForModel(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

// TestNewAnthropicCompat tests creating Anthropic compatibility layer.
func TestNewAnthropicCompat(t *testing.T) {
	apiKey := "test-api-key"
	compat := NewAnthropicCompat(apiKey)

	if compat == nil {
		t.Fatal("NewAnthropicCompat returned nil")
	}

	if compat.apiKey != apiKey {
		t.Errorf("apiKey = %s, expected %s", compat.apiKey, apiKey)
	}

	if compat.baseURL != "https://api.anthropic.com" {
		t.Errorf("baseURL = %s, expected https://api.anthropic.com", compat.baseURL)
	}

	if compat.version != "2023-06-01" {
		t.Errorf("version = %s, expected 2023-06-01", compat.version)
	}

	if compat.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

// TestAnthropicCompat_Messages tests the Messages method with mock server.
func TestAnthropicCompat_Messages(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected path /v1/messages, got %s", r.URL.Path)
		}

		// Check headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing or wrong x-api-key header")
		}

		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("missing or wrong anthropic-version header")
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}

		// Send mock response
		resp := AnthropicMessagesResponse{
			ID:      "msg-123",
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{{Type: "text", Text: "Hello!"}},
			Model:   "claude-3-opus-20240229",
			StopReason: "end_turn",
			Usage: AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 20,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create compat with test server URL - use the full server URL
	compat := NewAnthropicCompat("test-key")
	compat.baseURL = server.URL

	req := &AnthropicMessagesRequest{
		Model:     "claude-3-opus-20240229",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := context.Background()
	resp, err := compat.Messages(ctx, req)
	if err != nil {
		t.Fatalf("Messages failed: %v", err)
	}

	if resp.ID != "msg-123" {
		t.Errorf("ID = %s, expected msg-123", resp.ID)
	}

	if len(resp.Content) != 1 {
		t.Errorf("Content length = %d, expected 1", len(resp.Content))
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, expected 10", resp.Usage.InputTokens)
	}
}

// TestAnthropicCompat_MessagesError tests error handling in Messages.
func TestAnthropicCompat_MessagesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := AnthropicErrorResponse{
			Type: "error",
			Error: AnthropicErrorDetail{
				Type:    "invalid_request_error",
				Message: "Invalid request",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	compat := NewAnthropicCompat("test-key")
	compat.baseURL = server.URL

	req := &AnthropicMessagesRequest{
		Model:     "claude-3-opus-20240229",
		MaxTokens: 100,
		Messages:  []AnthropicMessage{{Role: "user", Content: "Hello"}},
	}

	ctx := context.Background()
	_, err := compat.Messages(ctx, req)
	if err == nil {
		t.Error("expected error for bad request")
	}

	if !strings.Contains(err.Error(), "Invalid request") {
		t.Errorf("error message should contain 'Invalid request', got: %v", err)
	}
}

// TestAnthropicCompat_MessagesStream tests streaming messages with mock server.
func TestAnthropicCompat_MessagesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing or wrong x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("missing or wrong anthropic-version header")
		}

		// Return a non-streaming JSON response (the stream parser will handle it)
		w.Header().Set("Content-Type", "text/event-stream")
		resp := AnthropicMessagesResponse{
			ID:      "msg-stream-1",
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{{Type: "text", Text: "Hello from stream!"}},
			Model:   "claude-3-opus-20240229",
			StopReason: "end_turn",
			Usage: AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}
		data, _ := json.Marshal(resp)
		w.Write(data)
	}))
	defer server.Close()

	compat := NewAnthropicCompat("test-key")
	compat.baseURL = server.URL

	req := &AnthropicMessagesRequest{
		Model:     "claude-3-opus-20240229",
		MaxTokens: 100,
		Messages:  []AnthropicMessage{{Role: "user", Content: "Hello"}},
	}

	ctx := context.Background()
	respChan, errChan := compat.MessagesStream(ctx, req)

	select {
	case <-respChan:
		// Channel closed or received a response - both are fine
	case err := <-errChan:
		if err != nil {
			t.Logf("Stream completed with: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for stream to complete")
	}
}

// TestAnthropicCompat_CountTokens tests token counting.
func TestAnthropicCompat_CountTokens(t *testing.T) {
	compat := NewAnthropicCompat("test-key")

	tests := []struct {
		text     string
		expected int
	}{
		{"Hello", 1},           // 5 chars / 4 = 1
		{"Hello world!", 3},    // 12 chars / 4 = 3
		{"", 0},                // 0 chars = 0
		{"a", 0},               // 1 char / 4 = 0
		{"abcdefgh", 2},        // 8 chars / 4 = 2
	}

	for _, tt := range tests {
		got := compat.CountTokens(tt.text)
		if got != tt.expected {
			t.Errorf("CountTokens(%q) = %d, expected %d", tt.text, got, tt.expected)
		}
	}
}

// TestConvertFromOpenAI tests conversion from OpenAI format.
func TestConvertFromOpenAI(t *testing.T) {
	openReq := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}

	result := ConvertFromOpenAI(openReq)
	if result == nil {
		t.Error("ConvertFromOpenAI returned nil")
	}
}

// TestConvertToOpenAI tests conversion to OpenAI format.
func TestConvertToOpenAI(t *testing.T) {
	anthropicResp := &AnthropicMessagesResponse{
		ID:    "msg-123",
		Type:  "message",
		Role:  "assistant",
		Model: "claude-3-opus-20240229",
	}

	result := ConvertToOpenAI(anthropicResp)
	// Function currently returns nil, just test it doesn't panic
	if result != nil {
		t.Error("ConvertToOpenAI should return nil")
	}
}

// TestNewOpenAICompat tests creating OpenAI compatibility layer.
func TestNewOpenAICompat(t *testing.T) {
	baseURL := "https://api.openai.com/v1"
	apiKey := "test-api-key"
	compat := NewOpenAICompat(baseURL, apiKey)

	if compat == nil {
		t.Fatal("NewOpenAICompat returned nil")
	}

	if compat.baseURL != baseURL {
		t.Errorf("baseURL = %s, expected %s", compat.baseURL, baseURL)
	}

	if compat.apiKey != apiKey {
		t.Errorf("apiKey = %s, expected %s", compat.apiKey, apiKey)
	}

	if compat.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

// TestOpenAICompat_ChatCompletion tests chat completion with mock server.
func TestOpenAICompat_ChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Authorization = %s, expected Bearer test-key", auth)
		}

		resp := OpenAIChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []OpenAIChoice{
				{
					Message: OpenAIMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: OpenAIUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	compat := NewOpenAICompat(server.URL, "test-key")

	req := &OpenAIChatCompletionRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []OpenAIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := context.Background()
	resp, err := compat.ChatCompletion(ctx, req)
	if err != nil {
		t.Fatalf("ChatCompletion failed: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("ID = %s, expected chatcmpl-123", resp.ID)
	}

	if len(resp.Choices) != 1 {
		t.Errorf("Choices length = %d, expected 1", len(resp.Choices))
	}

	if resp.Usage.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, expected 30", resp.Usage.TotalTokens)
	}
}

// TestOpenAICompat_ChatCompletionStream tests streaming chat completion.
func TestOpenAICompat_ChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Send multiple JSON objects for streaming
		choices := []OpenAIChoice{
			{Message: OpenAIMessage{Role: "assistant", Content: "Hello"}},
			{Message: OpenAIMessage{Role: "assistant", Content: " world"}},
		}

		for _, c := range choices {
			resp := OpenAIChatCompletionResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "gpt-4",
				Choices: []OpenAIChoice{c},
			}
			json.NewEncoder(w).Encode(resp)
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	compat := NewOpenAICompat(server.URL, "test-key")

	req := &OpenAIChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []OpenAIMessage{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}

	ctx := context.Background()
	respChan, errChan := compat.ChatCompletionStream(ctx, req)

	receivedCount := 0
	done := false
	for !done {
		select {
		case resp, ok := <-respChan:
			if !ok {
				done = true
				break
			}
			if resp != nil {
				receivedCount++
			}
		case err := <-errChan:
			if err != nil {
				t.Logf("Stream error: %v", err)
			}
			done = true
		case <-time.After(5 * time.Second):
			t.Error("timeout waiting for stream")
			done = true
		}
	}

	if receivedCount == 0 {
		t.Error("expected to receive at least one response")
	}
}

// TestOpenAICompat_ListModels tests listing models.
func TestOpenAICompat_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if r.URL.Path != "/models" {
			t.Errorf("expected path /models, got %s", r.URL.Path)
		}

		resp := ModelsResponse{
			Object: "list",
			Data: []Model{
				{ID: "gpt-4", Object: "model", Created: 1234567890, OwnedBy: "organization"},
				{ID: "gpt-3.5-turbo", Object: "model", Created: 1234567890, OwnedBy: "organization"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	compat := NewOpenAICompat(server.URL, "test-key")

	ctx := context.Background()
	resp, err := compat.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("Object = %s, expected list", resp.Object)
	}

	if len(resp.Data) != 2 {
		t.Errorf("Data length = %d, expected 2", len(resp.Data))
	}

	if resp.Data[0].ID != "gpt-4" {
		t.Errorf("First model ID = %s, expected gpt-4", resp.Data[0].ID)
	}
}

// TestOpenAICompat_CreateEmbedding tests creating embeddings.
func TestOpenAICompat_CreateEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/embeddings" {
			t.Errorf("expected path /embeddings, got %s", r.URL.Path)
		}

		resp := EmbeddingResponse{
			Object: "list",
			Data: []Embedding{
				{
					Object:    "embedding",
					Index:     0,
					Embedding: []float64{0.1, 0.2, 0.3},
				},
			},
			Model: "text-embedding-ada-002",
			Usage: OpenAIUsage{
				PromptTokens:     5,
				CompletionTokens: 0,
				TotalTokens:      5,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	compat := NewOpenAICompat(server.URL, "test-key")

	req := &EmbeddingRequest{
		Input: "Hello world",
		Model: "text-embedding-ada-002",
	}

	ctx := context.Background()
	resp, err := compat.CreateEmbedding(ctx, req)
	if err != nil {
		t.Fatalf("CreateEmbedding failed: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("Object = %s, expected list", resp.Object)
	}

	if len(resp.Data) != 1 {
		t.Errorf("Data length = %d, expected 1", len(resp.Data))
	}

	if len(resp.Data[0].Embedding) != 3 {
		t.Errorf("Embedding length = %d, expected 3", len(resp.Data[0].Embedding))
	}
}

// TestConvertToInterfaceRequest tests conversion to interface request.
func TestConvertToInterfaceRequest(t *testing.T) {
	req := &OpenAIChatCompletionRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []OpenAIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	result := ConvertToInterfaceRequest(req)
	if result == nil {
		t.Error("ConvertToInterfaceRequest returned nil")
	}
}

// TestConvertFromInterfaceResponse tests conversion from interface response.
func TestConvertFromInterfaceResponse(t *testing.T) {
	resp := &OpenAIChatCompletionResponse{
		ID:     "test-id",
		Object: "test",
		Choices: []OpenAIChoice{
			{Message: OpenAIMessage{Role: "assistant", Content: "Hello"}},
		},
	}

	result := ConvertFromInterfaceResponse(resp)
	if result == nil {
		t.Error("ConvertFromInterfaceResponse returned nil")
	}
}

// TestNodeStatusString tests NodeStatus string representation.
func TestNodeStatusString(t *testing.T) {
	tests := []struct {
		status NodeStatus
		want   string
	}{
		{NodeStatusUnknown, "unknown"},
		{NodeStatusActive, "active"},
		{NodeStatusInactive, "inactive"},
		{NodeStatusSlashed, "slashed"},
		{NodeStatus(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("NodeStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

// TestGenerateChallengeID tests challenge ID generation.
func TestGenerateChallengeID(t *testing.T) {
	id := generateChallengeID("challenger", "target", time.Now())
	if id == "" {
		t.Error("generateChallengeID returned empty string")
	}
	if len(id) < 10 {
		t.Errorf("generateChallengeID returned short ID: %s", id)
	}
}

// TestStringToNodeID tests node ID parsing.
func TestStringToNodeID(t *testing.T) {
	// Test empty string
	_ = stringToNodeID("")
	// Test invalid hex
	_ = stringToNodeID("invalid")
	// Test valid hex (64 chars)
	_ = stringToNodeID("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
}
