package inference

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestAnthropicProviderRealAPI tests the Anthropic provider with the real API
// Skip by default - requires real API endpoint
func TestAnthropicProviderRealAPI(t *testing.T) {
	if os.Getenv("RUN_REAL_API_TESTS") != "true" {
		t.Skip("Skipping real API test. Set RUN_REAL_API_TESTS=true to run.")
	}
	config := &AnthropicConfig{
		BaseURL: "http://217.216.43.45:51201/key/rk-e9412b1f5e955a92bbca9627",
		APIKey:  "rk-e9412b1f5e955a92bbca9627",
		Model:   "glm-5",
		Timeout: 30 * time.Second,
	}

	provider := NewAnthropicProvider(config)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test inference
	result, err := provider.Infer(ctx, "What is 2+2?")
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	if result == "" {
		t.Error("Empty response from API")
	}

	t.Logf("Result: %s", result)

	// Test ModelID
	modelID := provider.ModelID()
	if len(modelID) == 0 {
		t.Error("Empty model ID")
	}
	t.Logf("Model ID: %x", modelID)
}
