package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewOllamaProvider tests creating an OllamaProvider
func TestNewOllamaProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    *OllamaConfig
		wantURL   string
		wantModel string
	}{
		{
			name:      "default config",
			config:    nil,
			wantURL:   "http://localhost:11434",
			wantModel: "llama2",
		},
		{
			name: "custom config",
			config: &OllamaConfig{
				BaseURL: "http://example.com:8080",
				Model:   "mistral",
			},
			wantURL:   "http://example.com:8080",
			wantModel: "mistral",
		},
		{
			name: "automatically removes trailing slash",
			config: &OllamaConfig{
				BaseURL: "http://example.com:8080/",
			},
			wantURL:   "http://example.com:8080",
			wantModel: "llama2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOllamaProvider(tt.config)
			if p.BaseURL() != tt.wantURL {
				t.Errorf("BaseURL() = %v, want %v", p.BaseURL(), tt.wantURL)
			}
			if p.Model() != tt.wantModel {
				t.Errorf("Model() = %v, want %v", p.Model(), tt.wantModel)
			}
			if len(p.ModelID()) != 32 { // SHA-256 output length
				t.Errorf("ModelID() length = %v, want 32", len(p.ModelID()))
			}
		})
	}
}

// TestOllamaProvider_Infer tests the inference functionality
func TestOllamaProvider_Infer(t *testing.T) {
	// Create a mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request method
		if r.Method != http.MethodPost {
			t.Errorf("want POST request, got %s", r.Method)
		}

		// verify request path
		if r.URL.Path != "/api/generate" {
			t.Errorf("want path /api/generate, got %s", r.URL.Path)
		}

		// verify Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("want Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}

		// parse request body
		var reqBody ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to parse request body: %v", err)
			return
		}

		// return mock response
		resp := ollamaGenerateResponse{
			Model:    reqBody.Model,
			Response: "mock inference result: " + reqBody.Prompt,
			Done:     true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create a provider using the test server
	config := &OllamaConfig{
		BaseURL: server.URL,
		Model:   "test-model",
	}
	p := NewOllamaProvider(config)

	// test inference
	ctx := context.Background()
	result, err := p.Infer(ctx, "test prompt")
	if err != nil {
		t.Fatalf("Infer() failed: %v", err)
	}

	expected := "mock inference result: test prompt"
	if result != expected {
		t.Errorf("Infer() = %v, want %v", result, expected)
	}
}

// TestOllamaProvider_Infer_EmptyPrompt tests empty prompt handling
func TestOllamaProvider_Infer_EmptyPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "response",
			Done:     true,
		})
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	_, err := p.Infer(ctx, "")
	if err == nil {
		t.Error("want error for empty prompt")
	}
	if !strings.Contains(err.Error(), "prompt must not be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestOllamaProvider_Infer_ContextTimeout tests context timeout
func TestOllamaProvider_Infer_ContextTimeout(t *testing.T) {
	// Create a server with delayed responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "response",
			Done:     true,
		})
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	// Use a short-timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("want timeout error")
	}
}

// TestOllamaProvider_Infer_HTTPError tests HTTP error handling
func TestOllamaProvider_Infer_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("want HTTP error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error message should contain status code: %v", err)
	}
}

// TestOllamaProvider_Ping tests the health check functionality
func TestOllamaProvider_Ping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "healthy",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "unhealthy",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
			ctx := context.Background()

			err := p.Ping(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestOllamaProvider_ListModels tests listing models
func TestOllamaProvider_ListModels(t *testing.T) {
	expectedModels := []string{
		"llama2:latest",
		"mistral:7b",
		"codellama:instruct",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("want path /api/tags, got %s", r.URL.Path)
		}

		resp := ollamaTagsResponse{
			Models: make([]ollamaModelInfo, len(expectedModels)),
		}
		for i, m := range expectedModels {
			resp.Models[i] = ollamaModelInfo{Name: m}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	models, err := p.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels() failed: %v", err)
	}

	if len(models) != len(expectedModels) {
		t.Errorf("ListModels() returned %d models, want %d", len(models), len(expectedModels))
	}

	for i, m := range models {
		if m != expectedModels[i] {
			t.Errorf("model %d: got %s, want %s", i, m, expectedModels[i])
		}
	}
}

// TestNewMockProvider testcreate MockProvider
func TestNewMockProvider(t *testing.T) {
	config := &MockConfig{
		ModelName: "test-model",
		Delay:     100 * time.Millisecond,
		FailRate:  0, // Set to 0 for deterministic test (preset responses should not fail)
		Responses: map[string]string{
			"hello": "world",
		},
	}

	p := NewMockProvider(config)

	if len(p.ModelID()) != 32 {
		t.Errorf("ModelID() length = %v, want 32", len(p.ModelID()))
	}

	if p.delay != 100*time.Millisecond {
		t.Errorf("delay = %v, want 100ms", p.delay)
	}

	if p.failRate != 0 {
		t.Errorf("failRate = %v, want 0", p.failRate)
	}

	// test preset responses
	result, err := p.Infer(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Infer() failed: %v", err)
	}
	if result != "world" {
		t.Errorf("Infer() = %v, want 'world'", result)
	}
}

// TestMockProvider_Infer tests the MockProvider inference functionality
func TestMockProvider_Infer(t *testing.T) {
	p := NewMockProvider(nil)

	ctx := context.Background()

	// test default response
	result, err := p.Infer(ctx, "any prompt")
	if err != nil {
		t.Fatalf("Infer() failed: %v", err)
	}
	if !strings.Contains(result, "any prompt") {
		t.Errorf("response should contain the prompt: %v", result)
	}

	// test empty prompt
	_, err = p.Infer(ctx, "")
	if err == nil {
		t.Error("want error for empty prompt")
	}

	// test setting a custom response
	p.SetResponse("special", "special response")
	result, err = p.Infer(ctx, "special")
	if err != nil {
		t.Fatalf("Infer() failed: %v", err)
	}
	if result != "special response" {
		t.Errorf("Infer() = %v, want 'special response'", result)
	}
}

// TestMockProvider_Infer_Delay tests the MockProvider delay functionality
func TestMockProvider_Infer_Delay(t *testing.T) {
	p := NewMockProvider(&MockConfig{
		Delay: 100 * time.Millisecond,
	})

	ctx := context.Background()
	start := time.Now()
	p.Infer(ctx, "test")
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("delay = %v, want at least 100ms", elapsed)
	}
}

// TestMockProvider_Infer_FailRate tests the MockProvider failure rate simulation
func TestMockProvider_Infer_FailRate(t *testing.T) {
	// Use a 100% failure rate
	p := NewMockProvider(&MockConfig{
		FailRate: 1.0,
	})

	ctx := context.Background()
	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("want error when failure rate is 100%")
	}

	// test 0% failure rate
	p2 := NewMockProvider(&MockConfig{
		FailRate: 0.0,
	})
	_, err = p2.Infer(ctx, "test")
	if err != nil {
		t.Errorf("0%% failure rate should not return an error: %v", err)
	}
}

// TestMockProvider_Infer_ContextCancel tests MockProvider context cancellation
func TestMockProvider_Infer_ContextCancel(t *testing.T) {
	p := NewMockProvider(&MockConfig{
		Delay: 500 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("want error on context cancellation")
	}
}

// TestOllamaProvider_Concurrent tests concurrent inference
func TestOllamaProvider_Concurrent(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		var reqBody ollamaGenerateRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		resp := ollamaGenerateResponse{
			Response: fmt.Sprintf("response %d", callCount),
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
	ctx := context.Background()

	const concurrency = 10
	var wg sync.WaitGroup
	results := make(chan string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			result, err := p.Infer(ctx, fmt.Sprintf("request %d", n))
			if err == nil {
				results <- result
			}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for range results {
		successCount++
	}

	if successCount != concurrency {
		t.Errorf("succeeded %d/%d", successCount, concurrency)
	}

	if callCount != concurrency {
		t.Errorf("server received %d requests, want %d", callCount, concurrency)
	}
}

// TestMockProvider_InferCount tests the inference count functionality
func TestMockProvider_InferCount(t *testing.T) {
	p := NewMockProvider(nil)
	ctx := context.Background()

	if p.InferCount() != 0 {
		t.Errorf("initial count should be 0, got %d", p.InferCount())
	}

	p.Infer(ctx, "test1")
	p.Infer(ctx, "test2")

	if p.InferCount() != 2 {
		t.Errorf("count should be 2, got %d", p.InferCount())
	}

	p.ResetCount()
	if p.InferCount() != 0 {
		t.Errorf("count after reset should be 0, got %d", p.InferCount())
	}
}

// TestOllamaProvider_ModelIDConsistency tests ModelID consistency
func TestOllamaProvider_ModelIDConsistency(t *testing.T) {
	config := &OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	p1 := NewOllamaProvider(config)
	p2 := NewOllamaProvider(config)

	id1 := p1.ModelID()
	id2 := p2.ModelID()

	if len(id1) != 32 {
		t.Errorf("ModelID length should be 32, got %d", len(id1))
	}

	// identical configs should generate identical ModelIDs
	for i := range id1 {
		if id1[i] != id2[i] {
			t.Errorf("identical configs should generate identical ModelIDs, mismatch at position %d", i)
		}
	}

	// different configs should generate different ModelIDs
	p3 := NewOllamaProvider(&OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "mistral",
	})

	id3 := p3.ModelID()
	match := true
	for i := range id1 {
		if id1[i] != id3[i] {
			match = false
			break
		}
	}
	if match {
		t.Error("different models should generate different ModelIDs")
	}
}

// TestOllamaProvider_Infer_OllamaError tests the case where Ollama returns an error
func TestOllamaProvider_Infer_OllamaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaGenerateResponse{
			Error: "model not found",
			Done:  true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
	ctx := context.Background()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("want Ollama error")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error message should contain 'model not found': %v", err)
	}
}
