package inference

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// MockProvider is a mock inference provider used for testing
// Supports preset response mappings, simulated latency, and failure rate
type MockProvider struct {
	mu        sync.RWMutex
	responses map[string]string // prompt -> response mapping
	modelID   []byte            // simulated model fingerprint
	delay     time.Duration     // simulated inference latency
	failRate  float64           // simulated failure rate (0-1)

	// statistics
	inferCount int // number of inference calls
}

// MockConfig holds the configuration for MockProvider
type MockConfig struct {
	Responses map[string]string // preset prompt -> response mapping
	ModelName string            // simulated model name (used to generate modelID)
	Delay     time.Duration     // simulated inference latency
	FailRate  float64           // simulated failure rate (0-1)
}

// NewMockProvider creates a new mock inference provider
func NewMockProvider(config *MockConfig) *MockProvider {
	modelName := "mock-model"
	responses := make(map[string]string)
	var delay time.Duration
	var failRate float64

	if config != nil {
		if config.ModelName != "" {
			modelName = config.ModelName
		}
		if config.Responses != nil {
			for k, v := range config.Responses {
				responses[k] = v
			}
		}
		delay = config.Delay
		failRate = config.FailRate
	}

	// Generate model fingerprint
	fingerprint := sha256.Sum256([]byte("mock:" + modelName))

	return &MockProvider{
		responses: responses,
		modelID:   fingerprint[:],
		delay:     delay,
		failRate:  failRate,
	}
}

// Infer simulates the inference process
// If the prompt has a preset response in the mapping, return it; otherwise return the default response
func (p *MockProvider) Infer(ctx context.Context, prompt string) (string, error) {
	p.mu.Lock()
	p.inferCount++
	p.mu.Unlock()

	if prompt == "" {
		return "", fmt.Errorf("mock: prompt must not be empty")
	}

	// Simulate inference latency
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.delay):
		}
	}

	// Simulate random failure
	if p.failRate > 0 && rand.Float64() < p.failRate {
		return "", fmt.Errorf("mock: simulated inference failure (fail rate: %.2f)", p.failRate)
	}

	// Check whether the context has been cancelled
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Look up the preset response
	p.mu.RLock()
	resp, ok := p.responses[prompt]
	p.mu.RUnlock()

	if ok {
		return resp, nil
	}

	// Return the default response
	return fmt.Sprintf("mock response for: %s", prompt), nil
}

// ModelID returns the simulated model fingerprint
func (p *MockProvider) ModelID() []byte {
	return p.modelID
}

// SetResponse sets the preset response for the given prompt
func (p *MockProvider) SetResponse(prompt, response string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses[prompt] = response
}

// InferCount returns the number of inference calls (for test verification)
func (p *MockProvider) InferCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.inferCount
}

// ResetCount resets the inference call counter
func (p *MockProvider) ResetCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inferCount = 0
}
