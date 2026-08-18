package inference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default configuration constants
const (
	defaultBaseURL = "http://localhost:11434"
	defaultTimeout = 120 * time.Second
	defaultModel   = "llama2"
)

// OllamaConfig holds the configuration parameters for the Ollama adapter
type OllamaConfig struct {
	BaseURL string        // Ollama API base URL (default http://localhost:11434)
	Model   string        // model name (e.g. "llama2", "mistral")
	Timeout time.Duration // request timeout
}

// OllamaProvider implements the InferenceProvider interface and communicates with the Ollama HTTP API
type OllamaProvider struct {
	baseURL    string        // Ollama API base URL
	model      string        // model name in use
	modelID    []byte        // model fingerprint hash (SHA-256)
	httpClient *http.Client  // HTTP client
	timeout    time.Duration // request timeout
}

// ollamaGenerateRequest corresponds to the Ollama /api/generate request body
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaGenerateResponse corresponds to the Ollama /api/generate response body
type ollamaGenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ollamaTagsResponse corresponds to the Ollama /api/tags response body
type ollamaTagsResponse struct {
	Models []ollamaModelInfo `json:"models"`
}

// ollamaModelInfo holds basic model information
type ollamaModelInfo struct {
	Name string `json:"name"`
}

// NewOllamaProvider creates a new Ollama inference provider
// It initializes the HTTP client and model fingerprint from the given config
func NewOllamaProvider(config *OllamaConfig) *OllamaProvider {
	// Handle defaults
	baseURL := defaultBaseURL
	model := defaultModel
	timeout := defaultTimeout

	if config != nil {
		if config.BaseURL != "" {
			baseURL = strings.TrimRight(config.BaseURL, "/")
		}
		if config.Model != "" {
			model = config.Model
		}
		if config.Timeout > 0 {
			timeout = config.Timeout
		}
	}

	// Compute the model fingerprint: SHA-256 hash of baseURL + model
	fingerprint := sha256.Sum256([]byte(baseURL + ":" + model))

	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		modelID: fingerprint[:],
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// Infer calls the Ollama /api/generate endpoint to perform inference
// It takes a context and prompt, and returns the inference result string
func (p *OllamaProvider) Infer(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("ollama: prompt must not be empty")
	}

	// Build the request body with stream set to false to get the full response
	reqBody := ollamaGenerateRequest{
		Model:  p.model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama: failed to marshal request: %w", err)
	}

	// create HTTP request
	url := p.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("ollama: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// sendrequest
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama: failed to read response: %w", err)
	}

	// Check the HTTP status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: server returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	// parseresponse
	var result ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("ollama: failed to parse response: %w", err)
	}

	// Check the error message returned by Ollama
	if result.Error != "" {
		return "", fmt.Errorf("ollama: inference error: %s", result.Error)
	}

	return result.Response, nil
}

// ModelID returns the model fingerprint hash (SHA-256)
func (p *OllamaProvider) ModelID() []byte {
	return p.modelID
}

// Ping performs a health check on the Ollama service
// It tries to access the Ollama root path to confirm the service is reachable
func (p *OllamaProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL, nil)
	if err != nil {
		return fmt.Errorf("ollama: failed to create health check request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: health check returned status %d", resp.StatusCode)
	}

	return nil
}

// ListModels lists all available models via the Ollama /api/tags endpoint
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	url := p.baseURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create list models request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: list models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: list models returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to read model list: %w", err)
	}

	var tagsResp ollamaTagsResponse
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("ollama: failed to parse model list: %w", err)
	}

	// withdrawmodel namelist
	models := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		models = append(models, m.Name)
	}

	return models, nil
}

// Model returns the model name currently in use
func (p *OllamaProvider) Model() string {
	return p.model
}

// BaseURL returns the Ollama API base URL currently in use
func (p *OllamaProvider) BaseURL() string {
	return p.baseURL
}
