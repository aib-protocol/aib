package inference

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// MockProvider 用于测试的模拟推理提供者
// 支持预设响应映射、模拟延迟和失败率
type MockProvider struct {
	mu        sync.RWMutex
	responses map[string]string // prompt -> response 映射
	modelID   []byte           // 模拟的模型指纹
	delay     time.Duration    // 模拟推理延迟
	failRate  float64          // 模拟失败率 (0-1)

	// 统计数据
	inferCount int // 推理调用次数
}

// MockConfig 保存 MockProvider 的配置
type MockConfig struct {
	Responses map[string]string // 预设的 prompt -> response 映射
	ModelName string            // 模拟的模型名称（用于生成 modelID）
	Delay     time.Duration    // 模拟推理延迟
	FailRate  float64          // 模拟失败率 (0-1)
}

// NewMockProvider 创建一个新的模拟推理提供者
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

	// 生成模型指纹
	fingerprint := sha256.Sum256([]byte("mock:" + modelName))

	return &MockProvider{
		responses: responses,
		modelID:   fingerprint[:],
		delay:     delay,
		failRate:  failRate,
	}
}

// Infer 模拟推理过程
// 如果 prompt 在预设映射中有对应响应则返回，否则返回默认响应
func (p *MockProvider) Infer(ctx context.Context, prompt string) (string, error) {
	p.mu.Lock()
	p.inferCount++
	p.mu.Unlock()

	if prompt == "" {
		return "", fmt.Errorf("mock: 提示词不能为空")
	}

	// 模拟推理延迟
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.delay):
		}
	}

	// 模拟随机失败
	if p.failRate > 0 && rand.Float64() < p.failRate {
		return "", fmt.Errorf("mock: 模拟推理失败（失败率: %.2f）", p.failRate)
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 查找预设响应
	p.mu.RLock()
	resp, ok := p.responses[prompt]
	p.mu.RUnlock()

	if ok {
		return resp, nil
	}

	// 返回默认响应
	return fmt.Sprintf("mock response for: %s", prompt), nil
}

// ModelID 返回模拟的模型指纹
func (p *MockProvider) ModelID() []byte {
	return p.modelID
}

// SetResponse 设置指定 prompt 的预设响应
func (p *MockProvider) SetResponse(prompt, response string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses[prompt] = response
}

// InferCount 返回推理调用次数（用于测试验证）
func (p *MockProvider) InferCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.inferCount
}

// ResetCount 重置推理调用计数
func (p *MockProvider) ResetCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inferCount = 0
}
