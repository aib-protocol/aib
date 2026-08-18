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
	MaxTokens   int                   `json:"max_tokens"`  // 最大生成 tokens
	Temperature float64               `json:"temperature"` // 温度参数
	TopP        float64               `json:"top_p"`       // Top-p 采样
}

// DefaultUnifiedConfig 返回默认配置
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

// UnifiedProvider 统一推理提供者
// supports any OpenAI-compatible API 的通用提供者
// 无需为每个新模型编写代码适配器
type UnifiedProvider struct {
	config        *UnifiedConfig
	modelID       []byte
	httpClient    *http.Client
	modelRegistry *ModelRegistry
}

// NewUnifiedProvider 创建新的统一推理提供者
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

	// 计算模型指纹 ID
	modelData := fmt.Sprintf("%s:%s", config.BaseURL, config.Model)
	modelID := sha256.Sum256([]byte(modelData))

	// 创建 HTTP 客户端
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

// Infer performs inference（实现 InferenceProvider 接口）

// 支持以下provider：
// 1. OpenAI 兼容 API（默认）：任何 OpenAI 格式 API
// 2. Ollama API（本地推理）
// 3. Anthropic Claude API（需要特殊处理）
func (p *UnifiedProvider) Infer(ctx context.Context, prompt string) (string, error) {
	switch p.config.Type {
	case ProviderTypeOpenAI:
		return p.inferOpenAICompatible(ctx, prompt)
	case ProviderTypeOllama:
		return p.inferOllama(ctx, prompt)
	case ProviderTypeAnthropic:
		return p.inferAnthropic(ctx, prompt)
	case ProviderTypeCustom:
		// 自定义适配器，通过插件加载
		return p.inferCustom(ctx, prompt)
	case ProviderTypePlugin:
		// 动态加载的插件适配器
		return p.inferPlugin(ctx, prompt)
	default:
		return p.inferOpenAICompatible(ctx, prompt) // 默认回退
	}
}

// inferOpenAICompatible 调用 OpenAI 兼容 API

// 支持：智谱 GLM-4、DeepSeek、阿里通义、腾讯混元等
// 只要 API 格式兼容 OpenAI 即可
func (p *UnifiedProvider) inferOpenAICompatible(ctx context.Context, prompt string) (string, error) {
	// OpenAI 格式的请求体
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

	// 设置请求头
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

// inferOllama 调用 Ollama API（用于本地模型）

// 支持：llama2、qwen2.5、deepseek-coder、mistral等
func (p *UnifiedProvider) inferOllama(ctx context.Context, prompt string) (string, error) {
	// Ollama 格式的请求体
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

// inferAnthropic 调用 Anthropic Claude API

// Claude API 格式与 OpenAI 不同，需要特殊处理
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

// inferCustom 调用自定义适配器

// 用于特殊API格式（如百度文心、讯飞星火等）
func (p *UnifiedProvider) inferCustom(ctx context.Context, prompt string) (string, error) {
	// 自定义适配器通过插件或外部程序实现
	// 这里只是一个占位接口
	return "", fmt.Errorf("unified: custom adapter not implemented")
}

// inferPlugin 调用动态加载的插件适配器

// 支持 .so（Linux） 或 .dll（Windows） 插件
func (p *UnifiedProvider) inferPlugin(ctx context.Context, prompt string) (string, error) {
	// 插件动态加载功能
	return "", fmt.Errorf("unified: plugin system not implemented")
}

// ModelID 返回模型指纹（实现 InferenceProvider 接口）

// 用于唯一标识模型，基于 baseURL + model 的 SHA-256 哈希
func (p *UnifiedProvider) ModelID() []byte {
	return p.modelID
}

// ModelInfo represents information about a registered model
type ModelInfo struct {
	ModelID      string                `json:"model_id"`      // 模型唯一标识
	Name         string                `json:"name"`          // 模型显示名称
	Type         InferenceProviderType `json:"type"`          // provider type
	BaseURL      string                `json:"base_url"`      // API base URL
	Weight       float64               `json:"weight"`        // 权重（1.0 = 基准）
	Performance  *ModelPerformance     `json:"performance"`   // 性能指标
	RegisteredAt time.Time             `json:"registered_at"` // 注册时间
}

// ModelPerformance represents performance metrics for a model
type ModelPerformance struct {
	TaskCompletionRate float64       `json:"task_completion_rate"` // 任务完成率
	UserSatisfaction   float64       `json:"user_satisfaction"`    // 用户满意度
	AvgResponseTime    time.Duration `json:"avg_response_time"`    // 平均响应时间
	CostPerTask        float64       `json:"cost_per_task"`        // 每任务成本
	ReliabilityScore   float64       `json:"reliability_score"`    // 可靠性评分
}

// GetWeight getmodel weight

// 权重计算公式：
// weight = base_weight * (1 + performance_bonus - cost_penalty)
func (p *UnifiedProvider) GetWeight() float64 {
	if p.modelRegistry == nil {
		return p.config.Weight
	}

	// 获取模型性能数据
	perf := p.modelRegistry.GetPerformance(p.modelID)
	if perf == nil {
		return p.config.Weight
	}

	// 计算权重调整系数
	// 性能奖励：完成任务率高、用户满意
	// 成本惩罚：成本高则权重降低
	performanceBonus := (perf.TaskCompletionRate-0.85)*0.5 + // 0.85基准完成率
		(perf.UserSatisfaction-4.0)*0.2 // 4.0基准满意度

	costPenalty := (perf.CostPerTask - 0.005) * 100 // 0.005基准成本

	// 应用权重调整
	adjustedWeight := p.config.Weight * (1 + performanceBonus - costPenalty)

	// 确保权重在合理范围内

	minWeight := 0.1 // 最小权重
	maxWeight := 3.0 // 最大权重
	if adjustedWeight < minWeight {
		return minWeight
	}
	if adjustedWeight > maxWeight {
		return maxWeight
	}

	return adjustedWeight
}

// WeightedInfer 执行加权推理

// 返回结果和权重系数
func (p *UnifiedProvider) WeightedInfer(ctx context.Context, prompt string) (string, float64, error) {
	result, err := p.Infer(ctx, prompt)
	if err != nil {
		return "", 0, err
	}

	weight := p.GetWeight()
	return result, weight, nil
}

// GovernanceProposal 治理提案结构

// 用于提出权重调整、模型注册/取消等治理操作
type GovernanceProposal struct {
	ID          string                   `json:"id"`          // 提案ID
	Type        GovernanceProposalType   `json:"type"`        // 提案类型
	Title       string                   `json:"title"`       // 提案标题
	Description string                   `json:"description"` // 详细描述
	Evidence    map[string]interface{}   `json:"evidence"`    // 证据数据
	Proposer    string                   `json:"proposer"`    // 提案人
	Deadline    time.Time                `json:"deadline"`    // 投票截止时间
	Status      GovernanceProposalStatus `json:"status"`      // 提案状态
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

// ProposeWeightAdjustment 提出权重调整提案

// 任何人都可以提出，基于真实性能数据
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
		Proposer: "anonymous",                        // 实际应使用节点ID或钱包地址
		Deadline: time.Now().Add(7 * 24 * time.Hour), // 7天投票期
		Status:   ProposalStatusActive,
	}

	// TODO: 提交到链上治理合约
	return proposal, nil
}

// GovernanceVote governancevote

// 任何持币者可以投票
type GovernanceVote struct {
	ProposalID string    `json:"proposal_id"`
	Voter      string    `json:"voter"`
	Vote       bool      `json:"vote"` // true = yes, false = no
	Timestamp  time.Time `json:"timestamp"`
}

// ExecuteProposal 执行已通过的提案

// 自动生效，无需人工干预
func (p *UnifiedProvider) ExecuteProposal(proposal *GovernanceProposal) error {
	if proposal.Status != ProposalStatusPassed {
		return fmt.Errorf("unified: proposal not passed")
	}

	switch proposal.Type {
	case ProposalTypeWeightAdjustment:
		// 从evidence中提取modelID和newWeight
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
		// 注册新模型
	case ProposalTypeModelRemoval:
		// 移除模型

	default:
		return fmt.Errorf("unified: unknown proposal type: %s", proposal.Type)
	}

	return nil
}
