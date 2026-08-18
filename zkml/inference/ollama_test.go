package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewOllamaProvider testcreates an OllamaProvider
func TestNewOllamaProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    *OllamaConfig
		wantURL   string
		wantModel string
	}{
		{
			name:      "默认配置",
			config:    nil,
			wantURL:   "http://localhost:11434",
			wantModel: "llama2",
		},
		{
			name: "自定义配置",
			config: &OllamaConfig{
				BaseURL: "http://example.com:8080",
				Model:   "mistral",
			},
			wantURL:   "http://example.com:8080",
			wantModel: "mistral",
		},
		{
			name: "自动移除尾部斜杠",
			config: &OllamaConfig{
				BaseURL: "http://example.com:8080/",
			},
			wantURL:   "http://example.com:8080",
			wantModel: "llama2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOllamaProvider(tt.config)
			if p.BaseURL() != tt.wantURL {
				t.Errorf("BaseURL() = %v, want %v", p.BaseURL(), tt.wantURL)
			}
			if p.Model() != tt.wantModel {
				t.Errorf("Model() = %v, want %v", p.Model(), tt.wantModel)
			}
			if len(p.ModelID()) != 32 { // SHA-256 输出长度
				t.Errorf("ModelID() length = %v, want 32", len(p.ModelID()))
			}
		})
	}
}

// TestOllamaProvider_Infer test推理功能
func TestOllamaProvider_Infer(t *testing.T) {
	// 创建模拟 Ollama 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST 请求，得到 %s", r.Method)
		}

		// 验证请求路径
		if r.URL.Path != "/api/generate" {
			t.Errorf("期望路径 /api/generate，得到 %s", r.URL.Path)
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("期望 Content-Type: application/json，得到 %s", r.Header.Get("Content-Type"))
		}

		// 解析请求体
		var reqBody ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("解析请求体失败: %v", err)
			return
		}

		// 返回模拟响应
		resp := ollamaGenerateResponse{
			Model:    reqBody.Model,
			Response: "模拟inference result: " + reqBody.Prompt,
			Done:     true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 使用test服务器创建 provider
	config := &OllamaConfig{
		BaseURL: server.URL,
		Model:   "test-model",
	}
	p := NewOllamaProvider(config)

	// test推理
	ctx := context.Background()
	result, err := p.Infer(ctx, "test提示词")
	if err != nil {
		t.Fatalf("Infer() 失败: %v", err)
	}

	expected := "模拟inference result: test提示词"
	if result != expected {
		t.Errorf("Infer() = %v, want %v", result, expected)
	}
}

// TestOllamaProvider_Infer_EmptyPrompt test空提示词处理
func TestOllamaProvider_Infer_EmptyPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "响应",
			Done:     true,
		})
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	_, err := p.Infer(ctx, "")
	if err == nil {
		t.Error("期望空提示词返回error")
	}
	if !strings.Contains(err.Error(), "提示词不能为空") {
		t.Errorf("error信息不正确: %v", err)
	}
}

// TestOllamaProvider_Infer_ContextTimeout test上下文超时
func TestOllamaProvider_Infer_ContextTimeout(t *testing.T) {
	// 创建延迟响应的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "响应",
			Done:     true,
		})
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	// 使用短超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("期望超时error")
	}
}

// TestOllamaProvider_Infer_HTTPError test HTTP error处理
func TestOllamaProvider_Infer_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("内部服务器error"))
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("期望 HTTP error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error信息应包含状态码: %v", err)
	}
}

// TestOllamaProvider_Ping test健康检查功能
func TestOllamaProvider_Ping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "健康",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "不健康",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
			ctx := context.Background()

			err := p.Ping(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestOllamaProvider_ListModels test列出模型功能
func TestOllamaProvider_ListModels(t *testing.T) {
	expectedModels := []string{
		"llama2:latest",
		"mistral:7b",
		"codellama:instruct",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("期望路径 /api/tags，得到 %s", r.URL.Path)
		}

		resp := ollamaTagsResponse{
			Models: make([]ollamaModelInfo, len(expectedModels)),
		}
		for i, m := range expectedModels {
			resp.Models[i] = ollamaModelInfo{Name: m}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})

	ctx := context.Background()
	models, err := p.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels() 失败: %v", err)
	}

	if len(models) != len(expectedModels) {
		t.Errorf("ListModels() 返回 %d 个模型，期望 %d", len(models), len(expectedModels))
	}

	for i, m := range models {
		if m != expectedModels[i] {
			t.Errorf("模型 %d: 得到 %s，期望 %s", i, m, expectedModels[i])
		}
	}
}

// TestNewMockProvider test创建 MockProvider
func TestNewMockProvider(t *testing.T) {
	config := &MockConfig{
		ModelName: "test-model",
		Delay:     100 * time.Millisecond,
		FailRate:  0, // Set to 0 for deterministic test (preset responses should not fail)
		Responses: map[string]string{
			"hello": "world",
		},
	}

	p := NewMockProvider(config)

	if len(p.ModelID()) != 32 {
		t.Errorf("ModelID() 长度 = %v，期望 32", len(p.ModelID()))
	}

	if p.delay != 100*time.Millisecond {
		t.Errorf("delay = %v，期望 100ms", p.delay)
	}

	if p.failRate != 0 {
		t.Errorf("failRate = %v，期望 0", p.failRate)
	}

	// test预设响应
	result, err := p.Infer(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Infer() 失败: %v", err)
	}
	if result != "world" {
		t.Errorf("Infer() = %v，期望 'world'", result)
	}
}

// TestMockProvider_Infer test MockProvider 推理功能
func TestMockProvider_Infer(t *testing.T) {
	p := NewMockProvider(nil)

	ctx := context.Background()

	// test默认响应
	result, err := p.Infer(ctx, "任意提示词")
	if err != nil {
		t.Fatalf("Infer() 失败: %v", err)
	}
	if !strings.Contains(result, "任意提示词") {
		t.Errorf("响应应包含提示词: %v", result)
	}

	// test空提示词
	_, err = p.Infer(ctx, "")
	if err == nil {
		t.Error("期望空提示词返回error")
	}

	// test设置自定义响应
	p.SetResponse("special", "特殊响应")
	result, err = p.Infer(ctx, "special")
	if err != nil {
		t.Fatalf("Infer() 失败: %v", err)
	}
	if result != "特殊响应" {
		t.Errorf("Infer() = %v，期望 '特殊响应'", result)
	}
}

// TestMockProvider_Infer_Delay test MockProvider 延迟功能
func TestMockProvider_Infer_Delay(t *testing.T) {
	p := NewMockProvider(&MockConfig{
		Delay: 100 * time.Millisecond,
	})

	ctx := context.Background()
	start := time.Now()
	p.Infer(ctx, "test")
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("延迟 = %v，期望至少 100ms", elapsed)
	}
}

// TestMockProvider_Infer_FailRate test MockProvider 失败率模拟
func TestMockProvider_Infer_FailRate(t *testing.T) {
	// 使用 100% 失败率
	p := NewMockProvider(&MockConfig{
		FailRate: 1.0,
	})

	ctx := context.Background()
	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("期望失败率 100% 时返回error")
	}

	// test 0% 失败率
	p2 := NewMockProvider(&MockConfig{
		FailRate: 0.0,
	})
	_, err = p2.Infer(ctx, "test")
	if err != nil {
		t.Errorf("失败率 0%% 不应返回error: %v", err)
	}
}

// TestMockProvider_Infer_ContextCancel test MockProvider 上下文取消
func TestMockProvider_Infer_ContextCancel(t *testing.T) {
	p := NewMockProvider(&MockConfig{
		Delay: 500 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("期望上下文取消返回error")
	}
}

// TestOllamaProvider_Concurrent test并发推理
func TestOllamaProvider_Concurrent(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		var reqBody ollamaGenerateRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		resp := ollamaGenerateResponse{
			Response: fmt.Sprintf("响应 %d", callCount),
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
	ctx := context.Background()

	const concurrency = 10
	var wg sync.WaitGroup
	results := make(chan string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			result, err := p.Infer(ctx, fmt.Sprintf("请求 %d", n))
			if err == nil {
				results <- result
			}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for range results {
		successCount++
	}

	if successCount != concurrency {
		t.Errorf("成功 %d/%d", successCount, concurrency)
	}

	if callCount != concurrency {
		t.Errorf("服务器收到 %d 请求，期望 %d", callCount, concurrency)
	}
}

// TestMockProvider_InferCount test推理计数功能
func TestMockProvider_InferCount(t *testing.T) {
	p := NewMockProvider(nil)
	ctx := context.Background()

	if p.InferCount() != 0 {
		t.Errorf("初始计数应为 0，得到 %d", p.InferCount())
	}

	p.Infer(ctx, "test1")
	p.Infer(ctx, "test2")

	if p.InferCount() != 2 {
		t.Errorf("计数应为 2，得到 %d", p.InferCount())
	}

	p.ResetCount()
	if p.InferCount() != 0 {
		t.Errorf("重置后计数应为 0，得到 %d", p.InferCount())
	}
}

// TestOllamaProvider_ModelIDConsistency test ModelID 一致性
func TestOllamaProvider_ModelIDConsistency(t *testing.T) {
	config := &OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	p1 := NewOllamaProvider(config)
	p2 := NewOllamaProvider(config)

	id1 := p1.ModelID()
	id2 := p2.ModelID()

	if len(id1) != 32 {
		t.Errorf("ModelID 长度应为 32，得到 %d", len(id1))
	}

	// 相同配置应生成相同的 ModelID
	for i := range id1 {
		if id1[i] != id2[i] {
			t.Errorf("相同配置应生成相同的 ModelID，位置 %d 不匹配", i)
		}
	}

	// 不同配置应生成不同的 ModelID
	p3 := NewOllamaProvider(&OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "mistral",
	})

	id3 := p3.ModelID()
	match := true
	for i := range id1 {
		if id1[i] != id3[i] {
			match = false
			break
		}
	}
	if match {
		t.Error("不同模型应生成不同的 ModelID")
	}
}

// TestOllamaProvider_Infer_OllamaError test Ollama 返回error的情况
func TestOllamaProvider_Infer_OllamaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaGenerateResponse{
			Error: "model not found",
			Done:  true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(&OllamaConfig{BaseURL: server.URL})
	ctx := context.Background()

	_, err := p.Infer(ctx, "test")
	if err == nil {
		t.Error("期望 Ollama error")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error信息应包含 'model not found': %v", err)
	}
}
