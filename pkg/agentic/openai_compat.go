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

// OpenAICompat provides OpenAI API compatibility.
type OpenAICompat struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompat creates a new OpenAI compatibility layer.
func NewOpenAICompat(baseURL, apiKey string) *OpenAICompat {
	return &OpenAICompat{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ChatCompletionRequest represents an OpenAI chat completion request.
type OpenAIChatCompletionRequest struct {
	Model       string         `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	TopP        float64        `json:"top_p,omitempty"`
	N           int            `json:"n,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Stop        []string       `json:"stop,omitempty"`
}

// OpenAIMessage represents a message in the chat.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents an OpenAI chat completion response.
type OpenAIChatCompletionResponse struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []OpenAIChoice            `json:"choices"`
	Usage   OpenAIUsage               `json:"usage"`
}

// OpenAIChoice represents a choice in the response.
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIUsage represents token usage information.
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an error response.
type OpenAIErrorResponse struct {
	Error OpenAIError `json:"error"`
}

// OpenAIError represents an error.
type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ChatCompletion sends a chat completion request.
func (o *OpenAICompat) ChatCompletion(ctx context.Context, req *OpenAIChatCompletionRequest) (*OpenAIChatCompletionResponse, error) {
	url := fmt.Sprintf("%s/chat/completions", o.baseURL)

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

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))
	}

	// Send request
	resp, err := o.httpClient.Do(httpReq)
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
		var errResp OpenAIErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse response
	var result OpenAIChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// ChatCompletionStream sends a streaming chat completion request.
func (o *OpenAICompat) ChatCompletionStream(ctx context.Context, req *OpenAIChatCompletionRequest) (<-chan *OpenAIChatCompletionResponse, <-chan error) {
	respChan := make(chan *OpenAIChatCompletionResponse)
	errChan := make(chan error, 1)

	go func() {
		defer close(respChan)
		defer close(errChan)

		url := fmt.Sprintf("%s/chat/completions", o.baseURL)
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
		if o.apiKey != "" {
			httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))
		}

		resp, err := o.httpClient.Do(httpReq)
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

		// Handle streaming response
		decoder := json.NewDecoder(resp.Body)
		for {
			var result OpenAIChatCompletionResponse
			if err := decoder.Decode(&result); err != nil {
				if err == io.EOF {
					break
				}
				errChan <- fmt.Errorf("failed to decode response: %w", err)
				return
			}

			select {
			case respChan <- &result:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return respChan, errChan
}

// ModelsResponse represents the models API response.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents a model.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ListModels lists available models.
func (o *OpenAICompat) ListModels(ctx context.Context) (*ModelsResponse, error) {
	url := fmt.Sprintf("%s/models", o.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))
	}

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var result ModelsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// EmbeddingRequest represents an embedding request.
type EmbeddingRequest struct {
	Input  interface{} `json:"input"` // string, []string, []int, [][]int
	Model  string      `json:"model"`
	Format string      `json:"format,omitempty"` // "float" or "base64"
}

// EmbeddingResponse represents an embedding response.
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  OpenAIUsage `json:"usage"`
}

// Embedding represents an embedding vector.
type Embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// CreateEmbedding creates an embedding.
func (o *OpenAICompat) CreateEmbedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	url := fmt.Sprintf("%s/embeddings", o.baseURL)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))
	}

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var result EmbeddingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// ConvertToInterfaceRequest converts an interfaces.ChatCompletionRequest to OpenAI format.
func ConvertToInterfaceRequest(req *OpenAIChatCompletionRequest) interface{} {
	// This function handles conversion between our internal types and OpenAI format
	// In a real implementation, this would handle more complex conversions
	return req
}

// ConvertFromInterfaceResponse converts an OpenAI response to our internal format.
func ConvertFromInterfaceResponse(resp *OpenAIChatCompletionResponse) interface{} {
	// This function handles conversion from OpenAI format to our internal types
	return resp
}
