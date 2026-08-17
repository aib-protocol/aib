// Package agentic provides AI service layer with standard API compatibility.
package agentic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicCompat provides Anthropic (Claude) API compatibility.
type AnthropicCompat struct {
	baseURL    string
	apiKey     string
	version    string
	httpClient *http.Client
}

// NewAnthropicCompat creates a new Anthropic compatibility layer.
func NewAnthropicCompat(apiKey string) *AnthropicCompat {
	return &AnthropicCompat{
		baseURL: "https://api.anthropic.com",
		apiKey:  apiKey,
		version: "2023-06-01",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// MessagesRequest represents an Anthropic messages request.
type AnthropicMessagesRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	MaxTokens     int                `json:"max_tokens"`
	System        string             `json:"system,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
	TopP          float64            `json:"top_p,omitempty"`
	TopK          int                `json:"top_k,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

// AnthropicMessage represents a message in the conversation.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MessagesResponse represents an Anthropic messages response.
type AnthropicMessagesResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicContentBlock represents a content block in the response.
type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicUsage represents token usage information.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicErrorResponse represents an error response.
type AnthropicErrorResponse struct {
	Type  string               `json:"type"`
	Error AnthropicErrorDetail `json:"error"`
}

// AnthropicErrorDetail represents error details.
type AnthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Messages sends a messages request.
func (a *AnthropicCompat) Messages(ctx context.Context, req *AnthropicMessagesRequest) (*AnthropicMessagesResponse, error) {
	url := fmt.Sprintf("%s/v1/messages", a.baseURL)

	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for Anthropic API
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", a.version)

	// Send request
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp AnthropicErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result AnthropicMessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// MessagesStream sends a streaming messages request.
func (a *AnthropicCompat) MessagesStream(ctx context.Context, req *AnthropicMessagesRequest) (<-chan *AnthropicMessagesResponse, <-chan error) {
	respChan := make(chan *AnthropicMessagesResponse)
	errChan := make(chan error, 1)

	go func() {
		defer close(respChan)
		defer close(errChan)

		url := fmt.Sprintf("%s/v1/messages", a.baseURL)
		req.Stream = true

		data, err := json.Marshal(req)
		if err != nil {
			errChan <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if err != nil {
			errChan <- fmt.Errorf("failed to create request: %w", err)
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.apiKey)
		httpReq.Header.Set("anthropic-version", a.version)

		resp, err := a.httpClient.Do(httpReq)
		if err != nil {
			errChan <- fmt.Errorf("failed to send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errChan <- fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(body))
			return
		}

		// Handle streaming response (SSE format)
		// This is a simplified implementation
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				errChan <- fmt.Errorf("failed to read stream: %w", err)
				return
			}

			// Parse SSE events
			// In production, use proper SSE parser
			if n > 0 {
				// Simplified: just send what we got
				// Real implementation would parse SSE format
			}
		}
	}()

	return respChan, errChan
}

// CountTokens counts tokens for a message (approximate).
func (a *AnthropicCompat) CountTokens(text string) int {
	// Approximate tokenization
	// Claude uses ~4 chars per token on average
	return len(text) / 4
}

// StreamingText represents a streaming text event.
type StreamingText struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Index int    `json:"index"`
}

// StreamingDelta represents a streaming delta.
type StreamingDelta struct {
	Type  string             `json:"type"`
	Index int                `json:"index"`
	Delta StreamingDeltaText `json:"delta"`
}

// StreamingDeltaText represents delta text.
type StreamingDeltaText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MessageStart represents the start of a message.
type MessageStart struct {
	Type    string                 `json:"type"`
	Message AnthropicMessageHeader `json:"message"`
}

// AnthropicMessageHeader represents message header.
type AnthropicMessageHeader struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []interface{}  `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        AnthropicUsage `json:"usage"`
}

// MessageDelta represents message delta.
type MessageDelta struct {
	Type  string            `json:"type"`
	Delta map[string]string `json:"delta"`
	Usage AnthropicUsage    `json:"usage"`
}

// MessageStop represents message stop event.
type MessageStop struct {
	Type string `json:"type"`
}

// ConvertFromOpenAI converts OpenAI request to Anthropic format.
func ConvertFromOpenAI(openReq interface{}) *AnthropicMessagesRequest {
	// This function handles conversion from OpenAI format to Anthropic
	// In a real implementation, this would handle more complex conversions
	return &AnthropicMessagesRequest{}
}

// ConvertToOpenAI converts Anthropic response to OpenAI format.
func ConvertToOpenAI(anthropicResp *AnthropicMessagesResponse) interface{} {
	// This function handles conversion from Anthropic format to OpenAI
	// In a real implementation, this would handle more complex conversions
	return nil
}

// Models supported by Anthropic.
var AnthropicModels = []string{
	"claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-20240620",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-3-haiku-20240307",
}

// IsModelSupported checks if a model is supported.
func IsAnthropicModelSupported(model string) bool {
	for _, m := range AnthropicModels {
		if m == model {
			return true
		}
	}
	return false
}

// GetMaxTokensForModel returns the maximum tokens for a model.
func GetMaxTokensForModel(model string) int {
	switch {
	case model == "claude-3-5-sonnet-20241022", model == "claude-3-5-sonnet-20240620":
		return 200000
	case model == "claude-3-opus-20240229":
		return 200000
	case model == "claude-3-sonnet-20240229":
		return 200000
	case model == "claude-3-haiku-20240307":
		return 200000
	default:
		return 4096 // Default max tokens
	}
}
