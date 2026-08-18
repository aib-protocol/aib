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

// Unified inference interface definitions

// InferenceProviderType defines the supported inference provider types
type InferenceProviderType string

const (
	ProviderTypeOpenAI    InferenceProviderType = "openai-compatible"
	ProviderTypeOllama    InferenceProviderType = "ollama"
	ProviderTypeAnthropic InferenceProviderType = "anthropic"
	ProviderTypeCustom    InferenceProviderType = "custom"
	ProviderTypePlugin    InferenceProviderType = "plugin"
)

// UnifiedConfig generic inference provider configuration
// supports any OpenAI-compatible API
type UnifiedConfig struct {
	Type        InferenceProviderType `json:"type"`        // provider type
	BaseURL     string                `json:"base_url"`    // API base URL
	APIKey      string                `json:"api_key"`     // API key
	Model       string                `json:"model"`       // model name
	Weight      float64               `json:"weight"`      // model weight
	Timeout     time.Duration         `json:"timeout"`     // request timeout
	MaxTokens   int                   `json:"max_tokens"`  // maximum tokens to generate
	Temperature float64               `json:"temperature"` // temperature parameter
	TopP        float64               `json:"top_p"`       // Top-p sampling
}

// DefaultUnifiedConfig returnsdefaultconfig
func DefaultUnifiedConfig() *UnifiedConfig {
	return &UnifiedConfig{
		Type:        ProviderTypeOpenAI,
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-3.5-turbo",
		Weight:      1.0,
		Timeout:     120 * time.Second,
		MaxTokens:   2048,
		Temperature: 0.7,
		TopP:        0.9,
	}
}

// UnifiedProvider is the unified inference provider
// generic provider that supports any OpenAI-compatible API
// no need to write a code adapter for each new model
type UnifiedProvider struct {
	config        *UnifiedConfig
	modelID       []byte
	httpClient    *http.Client
	modelRegistry *ModelRegistry
}

// NewUnifiedProvider creates a new unified inference provider
func NewUnifiedProvider(config *UnifiedConfig) (*UnifiedProvider, error) {
	if config == nil {
		config = DefaultUnifiedConfig()
	}

	// verifyrequiredparameter
	if config.BaseURL == "" {
		return nil, fmt.Errorf("unified: base URL is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("unified: model name is required")
	}

	// compute the model fingerprint ID
	modelData := fmt.Sprintf("%s:%s", config.BaseURL, config.Model)
	modelID := sha256.Sum256([]byte(modelData))

	// create the HTTP client
	client := &http.Client{
		Timeout: config.Timeout,
	}

	return &UnifiedProvider{
		config:     config,
		modelID:    modelID[:],
		httpClient: client,
		modelRegistry: &ModelRegistry{
			models: make(map[string]*ModelInfo),
		},
	}, nil
}

// Infer performs inference (implements the InferenceProvider interface)

// supports the following providers:
// 1. OpenAI-compatible API (default): any OpenAI-format API
// 2. Ollama API (local inference)
// 3. Anthropic Claude API (requires special handling)
func (p *UnifiedProvider) Infer(ctx context.Context, prompt string) (string, error) {
	switch p.config.Type {
	case ProviderTypeOpenAI:
		return p.inferOpenAICompatible(ctx, prompt)
	case ProviderTypeOllama:
		return p.inferOllama(ctx, prompt)
	case ProviderTypeAnthropic:
		return p.inferAnthropic(ctx, prompt)
	case ProviderTypeCustom:
		// custom adapter, loaded via plugin
		return p.inferCustom(ctx, prompt)
	case ProviderTypePlugin:
		// dynamically loaded plugin adapter
		return p.inferPlugin(ctx, prompt)
	default:
		return p.inferOpenAICompatible(ctx, prompt) // default fallback
	}
}

// inferOpenAICompatible calls an OpenAI-compatible API

// supports: Zhipu GLM-4, DeepSeek, Alibaba Tongyi, Tencent Hunyuan, etc.
// any API with an OpenAI-compatible format works
func (p *UnifiedProvider) inferOpenAICompatible(ctx context.Context, prompt string) (string, error) {
	// request body in OpenAI format
	requestBody := map[string]interface{}{
		"model": p.config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  p.config.MaxTokens,
		"temperature": p.config.Temperature,
		"top_p":       p.config.TopP,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unified: failed to marshal request: %w", err)
	}

	// create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/chat/completions", p.config.BaseURL),
		bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("unified: failed to create request: %w", err)
	}

	// set request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	// sendrequest
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unified: request failed: %w", err)
	}
	defer resp.Body.Close()

	// readresponse
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unified: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unified: API returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// parseresponse
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unified: failed to unmarshal response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("unified: no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// inferOllama calls the Ollama API (for local models)

// supports: llama2, qwen2.5, deepseek-coder, mistral, etc.
func (p *UnifiedProvider) inferOllama(ctx context.Context, prompt string) (string, error) {
	// request body in Ollama format
	requestBody := map[string]interface{}{
		"model":  p.config.Model,
		"prompt": prompt,
		"stream": false,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unified: failed to marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/generate", p.config.BaseURL),
		bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("unified: failed to create ollama request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unified: ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unified: failed to read ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unified: ollama returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// parse Ollama response
	var ollamaResp struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("unified: failed to unmarshal ollama response: %w", err)
	}

	return ollamaResp.Response, nil
}

// inferAnthropic calls the Anthropic Claude API

// the Claude API format differs from OpenAI and requires special handling
func (p *UnifiedProvider) inferAnthropic(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model":      p.config.Model,
		"max_tokens": p.config.MaxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unified: failed to marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/messages", p.config.BaseURL),
		bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("unified: failed to create anthropic request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unified: anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unified: failed to read anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unified: anthropic returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// parse Claude response
	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return "", fmt.Errorf("unified: failed to unmarshal claude response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("unified: no content in claude response")
	}

	return claudeResp.Content[0].Text, nil
}

// inferCustom calls a custom adapter

// for special API formats (e.g. Baidu ERNIE, iFlytek Spark, etc.)
func (p *UnifiedProvider) inferCustom(ctx context.Context, prompt string) (string, error) {
	// custom adapters are implemented via plugins or external programs
	// this is just a placeholder interface
	return "", fmt.Errorf("unified: custom adapter not implemented")
}

// inferPlugin calls a dynamically loaded plugin adapter

// supports .so (Linux) or .dll (Windows) plugins
func (p *UnifiedProvider) inferPlugin(ctx context.Context, prompt string) (string, error) {
	// plugin dynamic loading feature
	return "", fmt.Errorf("unified: plugin system not implemented")
}

// ModelID returns the model fingerprint (implements the InferenceProvider interface)

// uniquely identifies the model, based on the SHA-256 hash of baseURL + model
func (p *UnifiedProvider) ModelID() []byte {
	return p.modelID
}

// ModelInfo represents information about a registered model
type ModelInfo struct {
	ModelID      string                `json:"model_id"`      // unique model identifier
	Name         string                `json:"name"`          // model display name
	Type         InferenceProviderType `json:"type"`          // provider type
	BaseURL      string                `json:"base_url"`      // API base URL
	Weight       float64               `json:"weight"`        // weight (1.0 = baseline)
	Performance  *ModelPerformance     `json:"performance"`   // performance metrics
	RegisteredAt time.Time             `json:"registered_at"` // registration time
}

// ModelPerformance represents performance metrics for a model
type ModelPerformance struct {
	TaskCompletionRate float64       `json:"task_completion_rate"` // task completion rate
	UserSatisfaction   float64       `json:"user_satisfaction"`    // user satisfaction
	AvgResponseTime    time.Duration `json:"avg_response_time"`    // average response time
	CostPerTask        float64       `json:"cost_per_task"`        // cost per task
	ReliabilityScore   float64       `json:"reliability_score"`    // reliability score
}

// GetWeight getmodel weight

// weight calculation formula:
// weight = base_weight * (1 + performance_bonus - cost_penalty)
func (p *UnifiedProvider) GetWeight() float64 {
	if p.modelRegistry == nil {
		return p.config.Weight
	}

	// get model performance data
	perf := p.modelRegistry.GetPerformance(p.modelID)
	if perf == nil {
		return p.config.Weight
	}

	// compute the weight adjustment factor
	// performance bonus: high task completion rate, satisfied users
	// cost penalty: higher cost lowers the weight
	performanceBonus := (perf.TaskCompletionRate-0.85)*0.5 + // 0.85 baseline completion rate
		(perf.UserSatisfaction-4.0)*0.2 // 4.0 baseline satisfaction

	costPenalty := (perf.CostPerTask - 0.005) * 100 // 0.005 baseline cost

	// apply the weight adjustment
	adjustedWeight := p.config.Weight * (1 + performanceBonus - costPenalty)

	// ensure the weight stays within a reasonable range

	minWeight := 0.1 // minimum weight
	maxWeight := 3.0 // maximum weight
	if adjustedWeight < minWeight {
		return minWeight
	}
	if adjustedWeight > maxWeight {
		return maxWeight
	}

	return adjustedWeight
}

// WeightedInfer performs weighted inference

// returns the result and the weight factor
func (p *UnifiedProvider) WeightedInfer(ctx context.Context, prompt string) (string, float64, error) {
	result, err := p.Infer(ctx, prompt)
	if err != nil {
		return "", 0, err
	}

	weight := p.GetWeight()
	return result, weight, nil
}

// GovernanceProposal governance proposal struct

// used for governance operations such as weight adjustment and model registration/removal
type GovernanceProposal struct {
	ID          string                   `json:"id"`          // proposal ID
	Type        GovernanceProposalType   `json:"type"`        // proposal type
	Title       string                   `json:"title"`       // proposal title
	Description string                   `json:"description"` // detailed description
	Evidence    map[string]interface{}   `json:"evidence"`    // evidence data
	Proposer    string                   `json:"proposer"`    // proposer
	Deadline    time.Time                `json:"deadline"`    // voting deadline
	Status      GovernanceProposalStatus `json:"status"`      // proposal status
}

type GovernanceProposalType string

const (
	ProposalTypeWeightAdjustment  GovernanceProposalType = "weight_adjustment"
	ProposalTypeModelRegistration GovernanceProposalType = "model_registration"
	ProposalTypeModelRemoval      GovernanceProposalType = "model_removal"
)

type GovernanceProposalStatus string

const (
	ProposalStatusPending  GovernanceProposalStatus = "pending"
	ProposalStatusActive   GovernanceProposalStatus = "active"
	ProposalStatusPassed   GovernanceProposalStatus = "passed"
	ProposalStatusRejected GovernanceProposalStatus = "rejected"
)

// ProposeWeightAdjustment submits a weight adjustment proposal

// anyone can submit one, based on real performance data
func (p *UnifiedProvider) ProposeWeightAdjustment(
	modelID string,
	currentWeight float64,
	proposedWeight float64,
	evidence map[string]interface{},
) (*GovernanceProposal, error) {

	if proposedWeight <= 0 {
		return nil, fmt.Errorf("unified: weight must be positive")
	}

	proposal := &GovernanceProposal{
		ID:    fmt.Sprintf("weight_%s_%d", modelID, time.Now().Unix()),
		Type:  ProposalTypeWeightAdjustment,
		Title: fmt.Sprintf("Adjust weight for model %s", modelID),
		Description: fmt.Sprintf("Proposal to adjust model weight from %.2f to %.2f",
			currentWeight, proposedWeight),
		Evidence: evidence,
		Proposer: "anonymous",                        // should actually use the node ID or wallet address
		Deadline: time.Now().Add(7 * 24 * time.Hour), // 7-day voting period
		Status:   ProposalStatusActive,
	}

	// TODO: submit to the on-chain governance contract
	return proposal, nil
}

// GovernanceVote governancevote

// any token holder can vote
type GovernanceVote struct {
	ProposalID string    `json:"proposal_id"`
	Voter      string    `json:"voter"`
	Vote       bool      `json:"vote"` // true = yes, false = no
	Timestamp  time.Time `json:"timestamp"`
}

// ExecuteProposal executes a passed proposal

// takes effect automatically, no manual intervention required
func (p *UnifiedProvider) ExecuteProposal(proposal *GovernanceProposal) error {
	if proposal.Status != ProposalStatusPassed {
		return fmt.Errorf("unified: proposal not passed")
	}

	switch proposal.Type {
	case ProposalTypeWeightAdjustment:
		// extract modelID and newWeight from evidence
		modelID, ok := proposal.Evidence["model_id"].(string)
		if !ok {
			return fmt.Errorf("unified: missing model_id in evidence")
		}

		newWeight, ok := proposal.Evidence["proposed_weight"].(float64)
		if !ok {
			return fmt.Errorf("unified: missing proposed_weight in evidence")
		}

		// updatemodel weight
		if p.modelRegistry != nil {
			info := p.modelRegistry.GetModelInfo(modelID)
			if info != nil {
				info.Weight = newWeight
			}
		}

	case ProposalTypeModelRegistration:
		// register a new model
	case ProposalTypeModelRemoval:
		// remove the model

	default:
		return fmt.Errorf("unified: unknown proposal type: %s", proposal.Type)
	}

	return nil
}
