package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aib-protocol/aib/zkml/orchestrator"
)

// TestDefaultMinerConfig 测试默认配置生成
func TestDefaultMinerConfig(t *testing.T) {
	config := DefaultMinerConfig()

	if config.NodeID == "" {
		t.Error("默认配置的 NodeID 不应为空")
	}
	if config.OllamaURL != "http://localhost:11434" {
		t.Errorf("默认 OllamaURL 应为 http://localhost:11434，实际为 %s", config.OllamaURL)
	}
	if config.Model != "llama2" {
		t.Errorf("默认 Model 应为 llama2，实际为 %s", config.Model)
	}
	if config.StakeAmount != 100.0 {
		t.Errorf("默认 StakeAmount 应为 100.0，实际为 %f", config.StakeAmount)
	}
	if config.ListenAddr != "0.0.0.0:9090" {
		t.Errorf("默认 ListenAddr 应为 0.0.0.0:9090，实际为 %s", config.ListenAddr)
	}
	if config.DataDir != "./data" {
		t.Errorf("默认 DataDir 应为 ./data，实际为 %s", config.DataDir)
	}
	if config.LogLevel != "info" {
		t.Errorf("默认 LogLevel 应为 info，实际为 %s", config.LogLevel)
	}
}

// TestGenerateNodeID 测试节点 ID 生成
func TestGenerateNodeID(t *testing.T) {
	// 测试多次生成，确保唯一性
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateNodeID()
		if len(id) < 10 {
			t.Errorf("生成的节点 ID 过短: %s", id)
		}
		if ids[id] {
			t.Errorf("检测到重复的节点 ID: %s", id)
		}
		ids[id] = true
	}
}

// TestConfigSaveLoad 测试配置保存和加载
func TestConfigSaveLoad(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	// 创建测试配置
	config := &MinerConfig{
		NodeID:      "test_node_123",
		OllamaURL:   "http://test:8080",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  "127.0.0.1:8080",
		DataDir:     "/tmp/test",
		LogLevel:    "debug",
	}

	// 测试保存
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("配置文件未创建")
	}

	// 测试加载
	loadedConfig, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证加载的配置与原始配置一致
	if loadedConfig.NodeID != config.NodeID {
		t.Errorf("NodeID 不匹配: 期望 %s, 实际 %s", config.NodeID, loadedConfig.NodeID)
	}
	if loadedConfig.OllamaURL != config.OllamaURL {
		t.Errorf("OllamaURL 不匹配: 期望 %s, 实际 %s", config.OllamaURL, loadedConfig.OllamaURL)
	}
	if loadedConfig.Model != config.Model {
		t.Errorf("Model 不匹配: 期望 %s, 实际 %s", config.Model, loadedConfig.Model)
	}
	if loadedConfig.StakeAmount != config.StakeAmount {
		t.Errorf("StakeAmount 不匹配: 期望 %f, 实际 %f", config.StakeAmount, loadedConfig.StakeAmount)
	}
	if loadedConfig.ListenAddr != config.ListenAddr {
		t.Errorf("ListenAddr 不匹配: 期望 %s, 实际 %s", config.ListenAddr, loadedConfig.ListenAddr)
	}
	if loadedConfig.DataDir != config.DataDir {
		t.Errorf("DataDir 不匹配: 期望 %s, 实际 %s", config.DataDir, loadedConfig.DataDir)
	}
	if loadedConfig.LogLevel != config.LogLevel {
		t.Errorf("LogLevel 不匹配: 期望 %s, 实际 %s", config.LogLevel, loadedConfig.LogLevel)
	}
}

// TestConfigValidation 测试配置验证
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   *MinerConfig
		expected string
	}{
		{
			name: "空 NodeID",
			config: &MinerConfig{
				NodeID:      "",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "node_id 不能为空",
		},
		{
			name: "空 OllamaURL",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "ollama_url 不能为空",
		},
		{
			name: "空 Model",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "model 不能为空",
		},
		{
			name: "负质押数量",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: -1.0,
				ListenAddr:  ":8080",
			},
			expected: "stake_amount 不能为负数",
		},
		{
			name: "空监听地址",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  "",
			},
			expected: "listen_addr 不能为空",
		},
		{
			name: "有效配置",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expected == "" && err != nil {
				t.Errorf("期望验证通过，实际错误: %v", err)
			}
			if tt.expected != "" && (err == nil || err.Error() != "config: "+tt.expected) {
				t.Errorf("期望错误包含 '%s'，实际错误: %v", tt.expected, err)
			}
		})
	}
}

// TestNewMiner 测试矿工创建
func TestNewMiner(t *testing.T) {
	// 测试有效配置
	config := &MinerConfig{
		NodeID:      "test_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("创建矿工失败: %v", err)
	}
	if miner == nil {
		t.Fatal("矿工实例不应为 nil")
	}
	if miner.Config().NodeID != config.NodeID {
		t.Errorf("矿工配置不匹配")
	}

	// 测试 nil 配置
	_, err = NewMiner(nil)
	if err == nil {
		t.Error("nil 配置应返回错误")
	}

	// 测试无效配置
	invalidConfig := &MinerConfig{
		NodeID: "", // 无效的 NodeID
	}
	_, err = NewMiner(invalidConfig)
	if err == nil {
		t.Error("无效配置应返回错误")
	}
}

// TestMinerStartStop 测试矿工启动和停止
func TestMinerStartStop(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "test_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("创建矿工失败: %v", err)
	}

	ctx := context.Background()

	// 测试启动
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("启动矿工失败: %v", err)
	}

	// 验证状态
	status := miner.Status()
	if !status.Running {
		t.Error("矿工启动后状态应为运行中")
	}
	if status.NodeID != config.NodeID {
		t.Errorf("状态中的 NodeID 不匹配")
	}

	// 测试重复启动
	if err := miner.Start(ctx); err == nil {
		t.Error("重复启动应返回错误")
	}

	// 测试停止
	if err := miner.Stop(); err != nil {
		t.Fatalf("停止矿工失败: %v", err)
	}

	// 验证状态
	status = miner.Status()
	if status.Running {
		t.Error("矿工停止后状态应为停止")
	}

	// 测试重复停止
	if err := miner.Stop(); err == nil {
		t.Error("重复停止应返回错误")
	}
}

// TestMinerStatus 测试状态获取
func TestMinerStatus(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "test_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("创建矿工失败: %v", err)
	}

	// 测试停止状态
	status := miner.Status()
	if status.Running {
		t.Error("未启动的矿工状态应为停止")
	}
	if status.Uptime != 0 {
		t.Error("未启动的矿工运行时长应为 0")
	}
	if status.TasksProcessed != 0 {
		t.Error("未启动的矿工任务数应为 0")
	}
	if status.NodeID != config.NodeID {
		t.Errorf("状态中的 NodeID 不匹配")
	}
	if status.Model != config.Model {
		t.Errorf("状态中的 Model 不匹配")
	}

	// 测试启动后状态
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("启动矿工失败: %v", err)
	}
	defer miner.Stop()

	time.Sleep(100 * time.Millisecond) // 等待一小段时间

	status = miner.Status()
	if !status.Running {
		t.Error("启动后的矿工状态应为运行中")
	}
	if status.Uptime <= 0 {
		t.Error("启动后的矿工运行时长应大于 0")
	}
	if !status.StartTime.After(time.Now().Add(-time.Second)) {
		t.Error("启动时间应接近当前时间")
	}
}

// TestProcessTask 测试任务处理
func TestProcessTask(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "test_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("创建矿工失败: %v", err)
	}

	// 创建测试任务
	task := orchestrator.NewTask("task_123", "test prompt", "requester_123", 3, time.Minute)
	task.AssignNodes([]string{"other_node_1", "test_node", "other_node_2"})

	// 测试矿工未启动时处理任务
	err = miner.ProcessTask(task)
	if err == nil {
		t.Error("矿工未启动时应返回错误")
	}

	// 启动矿工
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("启动矿工失败: %v", err)
	}
	defer miner.Stop()

	// 测试处理分配给当前节点的任务
	err = miner.ProcessTask(task)
	if err != nil {
		t.Fatalf("处理任务失败: %v", err)
	}

	// 验证任务计数
	status := miner.Status()
	if status.TasksProcessed != 1 {
		t.Errorf("任务计数应为 1，实际为 %d", status.TasksProcessed)
	}

	// 测试处理未分配给当前节点的任务
	otherTask := orchestrator.NewTask("task_456", "test prompt", "requester_456", 2, time.Minute)
	otherTask.AssignNodes([]string{"other_node_1", "other_node_2"})

	err = miner.ProcessTask(otherTask)
	if err == nil {
		t.Error("处理未分配给当前节点的任务时应返回错误")
	}

	// 测试 nil 任务
	err = miner.ProcessTask(nil)
	if err == nil {
		t.Error("处理 nil 任务时应返回错误")
	}
}

// TestConfigJSONMarshaling 测试 JSON 序列化和反序列化
func TestConfigJSONMarshaling(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "json_test_node",
		OllamaURL:   "http://json-test",
		Model:       "json_model",
		StakeAmount: 75.5,
		ListenAddr:  "0.0.0.0:9999",
		DataDir:     "/tmp/json",
		LogLevel:    "warn",
	}

	// 测试序列化
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	// 测试反序列化
	var decodedConfig MinerConfig
	if err := json.Unmarshal(data, &decodedConfig); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	// 验证字段匹配
	if decodedConfig.NodeID != config.NodeID {
		t.Errorf("NodeID 不匹配")
	}
	if decodedConfig.OllamaURL != config.OllamaURL {
		t.Errorf("OllamaURL 不匹配")
	}
	if decodedConfig.Model != config.Model {
		t.Errorf("Model 不匹配")
	}
	if decodedConfig.StakeAmount != config.StakeAmount {
		t.Errorf("StakeAmount 不匹配")
	}
	if decodedConfig.ListenAddr != config.ListenAddr {
		t.Errorf("ListenAddr 不匹配")
	}
	if decodedConfig.DataDir != config.DataDir {
		t.Errorf("DataDir 不匹配")
	}
	if decodedConfig.LogLevel != config.LogLevel {
		t.Errorf("LogLevel 不匹配")
	}
}

// TestLoadConfigInvalidPath 测试加载无效路径
func TestLoadConfigInvalidPath(t *testing.T) {
	// 测试空路径
	_, err := LoadConfig("")
	if err == nil {
		t.Error("空路径应返回错误")
	}

	// 测试不存在的文件
	_, err = LoadConfig("/tmp/nonexistent_config_123456.json")
	if err == nil {
		t.Error("不存在的文件应返回错误")
	}

	// 测试无效 JSON
	tempDir := t.TempDir()
	invalidJSONPath := filepath.Join(tempDir, "invalid.json")
	if err := os.WriteFile(invalidJSONPath, []byte("{ invalid json"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	_, err = LoadConfig(invalidJSONPath)
	if err == nil {
		t.Error("无效 JSON 应返回错误")
	}
}

// TestSaveConfigInvalidPath 测试保存到无效路径
func TestSaveConfigInvalidPath(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "test",
		OllamaURL:   "http://test",
		Model:       "test",
		StakeAmount: 10.0,
		ListenAddr:  ":8080",
	}

	// 测试空路径
	err := SaveConfig("", config)
	if err == nil {
		t.Error("空路径应返回错误")
	}

	// 测试 nil 配置
	err = SaveConfig("/tmp/test.json", nil)
	if err == nil {
		t.Error("nil 配置应返回错误")
	}
}

// TestMinerConcurrentAccess 测试并发访问
func TestMinerConcurrentAccess(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "concurrent_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("创建矿工失败: %v", err)
	}

	// 启动矿工
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("启动矿工失败: %v", err)
	}
	defer miner.Stop()

	// 并发访问测试
	done := make(chan bool)
	errors := make(chan error, 10)

	// 启动多个 goroutine 并发访问状态
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				status := miner.Status()
				if status.NodeID != config.NodeID {
					errors <- fmt.Errorf("goroutine %d 获取的 NodeID 不匹配", id)
					return
				}
				time.Sleep(time.Millisecond)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case err := <-errors:
			t.Fatalf("并发访问出错: %v", err)
		}
	}
}
