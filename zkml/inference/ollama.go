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

// 默认配置常量
const (
	defaultBaseURL = "http://localhost:11434"
	defaultTimeout = 120 * time.Second
	defaultModel   = "llama2"
)

// OllamaConfig 保存 Ollama 适配器的配置参数
type OllamaConfig struct {
	BaseURL string        // Ollama API base URL（默认 http://localhost:11434）
	Model   string        // model name（如 "llama2", "mistral"）
	Timeout time.Duration // request timeout时间
}

// OllamaProvider 实现 InferenceProvider 接口，负责与 Ollama HTTP API 通信
type OllamaProvider struct {
	baseURL    string        // Ollama API base URL
	model      string        // 使用的model name
	modelID    []byte        // 模型指纹哈希（SHA-256）
	httpClient *http.Client  // HTTP 客户端
	timeout    time.Duration // request timeout
}

// ollamaGenerateRequest 对应 Ollama /api/generate 请求体
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaGenerateResponse 对应 Ollama /api/generate 响应体
type ollamaGenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ollamaTagsResponse 对应 Ollama /api/tags 响应体
type ollamaTagsResponse struct {
	Models []ollamaModelInfo `json:"models"`
}

// ollamaModelInfo 保存模型的基本信息
type ollamaModelInfo struct {
	Name string `json:"name"`
}

// NewOllamaProvider 创建一个新的 Ollama 推理提供者
// 根据传入的配置初始化 HTTP 客户端和模型指纹
func NewOllamaProvider(config *OllamaConfig) *OllamaProvider {
	// 处理默认值
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

	// 计算模型指纹：使用 baseURL + model 的 SHA-256 哈希
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

// Infer 调用 Ollama /api/generate 接口进行推理
// 传入上下文和提示词，返回inference result字符串
func (p *OllamaProvider) Infer(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("ollama: 提示词不能为空")
	}

	// 构建请求体，stream 设为 false 以获取完整响应
	reqBody := ollamaGenerateRequest{
		Model:  p.model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama: 序列化请求失败: %w", err)
	}

	// 创建 HTTP 请求
	url := p.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("ollama: 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama: 读取响应失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: 服务器返回error状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("ollama: 解析响应失败: %w", err)
	}

	// 检查 Ollama 返回的error信息
	if result.Error != "" {
		return "", fmt.Errorf("ollama: 推理error: %s", result.Error)
	}

	return result.Response, nil
}

// ModelID 返回模型指纹哈希（SHA-256）
func (p *OllamaProvider) ModelID() []byte {
	return p.modelID
}

// Ping 对 Ollama 服务进行健康检查
// 尝试访问 Ollama 根路径，确认服务可达
func (p *OllamaProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL, nil)
	if err != nil {
		return fmt.Errorf("ollama: 创建健康检查请求失败: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: 健康检查失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: 健康检查返回状态码 %d", resp.StatusCode)
	}

	return nil
}

// ListModels 通过 Ollama /api/tags 接口列出所有可用模型
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	url := p.baseURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: 创建列出模型请求失败: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: 列出模型请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: 列出模型返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: 读取模型列表失败: %w", err)
	}

	var tagsResp ollamaTagsResponse
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("ollama: 解析模型列表失败: %w", err)
	}

	// 提取model name列表
	models := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		models = append(models, m.Name)
	}

	return models, nil
}

// Model 返回当前使用的model name
func (p *OllamaProvider) Model() string {
	return p.model
}

// BaseURL 返回当前使用的 Ollama API base URL
func (p *OllamaProvider) BaseURL() string {
	return p.baseURL
}
