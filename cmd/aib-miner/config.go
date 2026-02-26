package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// MinerConfig 矿工配置结构
type MinerConfig struct {
	NodeID      string  `json:"node_id"`      // 节点唯一标识
	OllamaURL   string  `json:"ollama_url"`   // Ollama API 地址
	Model       string  `json:"model"`        // 推理模型名称
	StakeAmount float64 `json:"stake_amount"` // 质押数量
	ListenAddr  string  `json:"listen_addr"`  // 监听地址
	DataDir     string  `json:"data_dir"`     // 数据存储目录
	LogLevel    string  `json:"log_level"`    // 日志级别: debug, info, warn, error
}

// DefaultMinerConfig 返回带有合理默认值的配置
func DefaultMinerConfig() *MinerConfig {
	return &MinerConfig{
		NodeID:      GenerateNodeID(),
		OllamaURL:   "http://localhost:11434",
		Model:       "llama2",
		StakeAmount: 100.0,
		ListenAddr:  "0.0.0.0:9090",
		DataDir:     "./data",
		LogLevel:    "info",
	}
}

// LoadConfig 从 JSON 文件加载配置
func LoadConfig(path string) (*MinerConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("config: 配置文件路径不能为空")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: 无法读取配置文件 %s: %w", path, err)
	}

	var config MinerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("config: 解析配置文件失败: %w", err)
	}

	// 验证必要字段
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig 将配置保存到 JSON 文件
func SaveConfig(path string, config *MinerConfig) error {
	if path == "" {
		return fmt.Errorf("config: 保存路径不能为空")
	}
	if config == nil {
		return fmt.Errorf("config: 配置对象不能为 nil")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("config: 序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: 写入配置文件失败: %w", err)
	}

	return nil
}

// GenerateNodeID 生成随机的节点 ID，使用 16 字节加密随机数
func GenerateNodeID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// 极端情况下回退到时间戳（不应该发生）
		return "node_fallback"
	}
	return "miner_" + hex.EncodeToString(b)
}

// Validate 验证配置的合法性
func (c *MinerConfig) Validate() error {
	if c.NodeID == "" {
		return fmt.Errorf("config: node_id 不能为空")
	}
	if c.OllamaURL == "" {
		return fmt.Errorf("config: ollama_url 不能为空")
	}
	if c.Model == "" {
		return fmt.Errorf("config: model 不能为空")
	}
	if c.StakeAmount < 0 {
		return fmt.Errorf("config: stake_amount 不能为负数")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("config: listen_addr 不能为空")
	}
	return nil
}
