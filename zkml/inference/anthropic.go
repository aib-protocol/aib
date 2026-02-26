package inference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicConfig holds configuration for Anthropic-compatible API
type AnthropicConfig struct {
	BaseURL string        // API base URL
	APIKey  string        // API key (if needed)
	Model   string        // Model name
	Timeout time.Duration // Request timeout
}

// AnthropicProvider implements InferenceProvider for Anthropic-compatible APIs
type AnthropicProvider struct {
	baseURL    string
	apiKey     string
	model      string
	modelID    []byte
	httpClient *http.Client
	timeout    time.Duration
}

// anthropicRequest represents an Anthropic API request
type anthropicRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []anthropicMsg  `json:"messages"`
}

// anthropicMsg represents a message in the request
type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse represents an Anthropic API response
type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewAnthropicProvider creates a new Anthropic-compatible provider
func NewAnthropicProvider(config *AnthropicConfig) *AnthropicProvider {
	baseURL := "https://api.anthropic.com"
	model := "claude-3-sonnet-20240229"
	timeout := 120 * time.Second

	if config != nil {
		if config.BaseURL != "" {
			baseURL = config.BaseURL
		}
		if config.Model != "" {
			model = config.Model
		}
		if config.Timeout > 0 {
			timeout = config.Timeout
		}
	}

	// Calculate model fingerprint
	fingerprint := sha256.Sum256([]byte(baseURL + ":" + model))

	return &AnthropicProvider{
		baseURL: baseURL,
		apiKey:  config.APIKey,
		model:   model,
		modelID: fingerprint[:],
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// Infer performs inference using the Anthropic-compatible API
func (p *AnthropicProvider) Infer(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("anthropic: prompt cannot be empty")
	}

	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: 1024,
		Messages: []anthropicMsg{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	url := p.baseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("anthropic: failed to parse response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("anthropic: API error: %s", result.Error.Message)
	}

	// Extract text from content blocks
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text = block.Text
			break
		}
	}

	if text == "" {
		return "", fmt.Errorf("anthropic: no text content in response")
	}

	return text, nil
}

// ModelID returns the model fingerprint
func (p *AnthropicProvider) ModelID() []byte {
	return p.modelID
}

// Model returns the model name
func (p *AnthropicProvider) Model() string {
	return p.model
}

// BaseURL returns the API base URL
func (p *AnthropicProvider) BaseURL() string {
	return p.baseURL
}

// Ping checks if the API is reachable
func (p *AnthropicProvider) Ping(ctx context.Context) error {
	// Try a minimal request
	_, err := p.Infer(ctx, "ping")
	return err
}
