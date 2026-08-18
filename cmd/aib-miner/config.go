package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// MinerConfig miner configuration
type MinerConfig struct {
	NodeID      string  `json:"node_id"`      // unique node identifier
	OllamaURL   string  `json:"ollama_url"`   // Ollama API address
	Model       string  `json:"model"`        // inference model name
	StakeAmount float64 `json:"stake_amount"` // stake amount
	ListenAddr  string  `json:"listen_addr"`  // listen address
	DataDir     string  `json:"data_dir"`     // data storage directory
	LogLevel    string  `json:"log_level"`    // log level: debug, info, warn, error
}

// DefaultMinerConfig returns a config with sensible defaults
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

// LoadConfig loads config from a JSON file
func LoadConfig(path string) (*MinerConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("config: config file path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: failed to read config file %s: %w", path, err)
	}

	var config MinerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("config: failed to parse config file: %w", err)
	}

	// validate required fields
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves the config to a JSON file
func SaveConfig(path string, config *MinerConfig) error {
	if path == "" {
		return fmt.Errorf("config: save path cannot be empty")
	}
	if config == nil {
		return fmt.Errorf("config: config cannot be nil")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("config: failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: failed to write config file: %w", err)
	}

	return nil
}

// GenerateNodeID generates a random node ID using 16 bytes of cryptographic randomness
func GenerateNodeID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// fall back to timestamp in extreme cases (should not happen)
		return "node_fallback"
	}
	return "miner_" + hex.EncodeToString(b)
}

// Validate validates the config
func (c *MinerConfig) Validate() error {
	if c.NodeID == "" {
		return fmt.Errorf("config: node_id cannot be empty")
	}
	if c.OllamaURL == "" {
		return fmt.Errorf("config: ollama_url cannot be empty")
	}
	if c.Model == "" {
		return fmt.Errorf("config: model cannot be empty")
	}
	if c.StakeAmount < 0 {
		return fmt.Errorf("config: stake_amount cannot be negative")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("config: listen_addr cannot be empty")
	}
	return nil
}
