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

// TestDefaultMinerConfig tests default config generation
func TestDefaultMinerConfig(t *testing.T) {
	config := DefaultMinerConfig()

	if config.NodeID == "" {
		t.Error("default config NodeID should not be empty")
	}
	if config.OllamaURL != "http://localhost:11434" {
		t.Errorf("default OllamaURL expected http://localhost:11434，actual %s", config.OllamaURL)
	}
	if config.Model != "llama2" {
		t.Errorf("default Model expected llama2，actual %s", config.Model)
	}
	if config.StakeAmount != 100.0 {
		t.Errorf("default StakeAmount expected 100.0，actual %f", config.StakeAmount)
	}
	if config.ListenAddr != "0.0.0.0:9090" {
		t.Errorf("default ListenAddr expected 0.0.0.0:9090，actual %s", config.ListenAddr)
	}
	if config.DataDir != "./data" {
		t.Errorf("default DataDir expected ./data，actual %s", config.DataDir)
	}
	if config.LogLevel != "info" {
		t.Errorf("default LogLevel expected info，actual %s", config.LogLevel)
	}
}

// TestGenerateNodeID tests node ID generation
func TestGenerateNodeID(t *testing.T) {
	// generate multiple times to ensure uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateNodeID()
		if len(id) < 10 {
			t.Errorf("generated node ID too short: %s", id)
		}
		if ids[id] {
			t.Errorf("duplicate node ID detected: %s", id)
		}
		ids[id] = true
	}
}

// TestConfigSaveLoad tests config save and load
func TestConfigSaveLoad(t *testing.T) {
	// create a temp directory
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	// create test config
	config := &MinerConfig{
		NodeID:      "test_node_123",
		OllamaURL:   "http://test:8080",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  "127.0.0.1:8080",
		DataDir:     "/tmp/test",
		LogLevel:    "debug",
	}

	// test save
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file not created")
	}

	// test load
	loadedConfig, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// verify loaded config matches original
	if loadedConfig.NodeID != config.NodeID {
		t.Errorf("NodeID mismatch: want %s, got %s", config.NodeID, loadedConfig.NodeID)
	}
	if loadedConfig.OllamaURL != config.OllamaURL {
		t.Errorf("OllamaURL mismatch: want %s, got %s", config.OllamaURL, loadedConfig.OllamaURL)
	}
	if loadedConfig.Model != config.Model {
		t.Errorf("Model mismatch: want %s, got %s", config.Model, loadedConfig.Model)
	}
	if loadedConfig.StakeAmount != config.StakeAmount {
		t.Errorf("StakeAmount mismatch: want %f, got %f", config.StakeAmount, loadedConfig.StakeAmount)
	}
	if loadedConfig.ListenAddr != config.ListenAddr {
		t.Errorf("ListenAddr mismatch: want %s, got %s", config.ListenAddr, loadedConfig.ListenAddr)
	}
	if loadedConfig.DataDir != config.DataDir {
		t.Errorf("DataDir mismatch: want %s, got %s", config.DataDir, loadedConfig.DataDir)
	}
	if loadedConfig.LogLevel != config.LogLevel {
		t.Errorf("LogLevel mismatch: want %s, got %s", config.LogLevel, loadedConfig.LogLevel)
	}
}

// TestConfigValidation tests config validation
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   *MinerConfig
		expected string
	}{
		{
			name: "empty NodeID",
			config: &MinerConfig{
				NodeID:      "",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "node_id cannot be empty",
		},
		{
			name: "empty OllamaURL",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "ollama_url cannot be empty",
		},
		{
			name: "empty Model",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "",
				StakeAmount: 10.0,
				ListenAddr:  ":8080",
			},
			expected: "model cannot be empty",
		},
		{
			name: "negative stake amount",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: -1.0,
				ListenAddr:  ":8080",
			},
			expected: "stake_amount cannot be negative",
		},
		{
			name: "empty listen address",
			config: &MinerConfig{
				NodeID:      "test",
				OllamaURL:   "http://test",
				Model:       "test",
				StakeAmount: 10.0,
				ListenAddr:  "",
			},
			expected: "listen_addr cannot be empty",
		},
		{
			name: "valid config",
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
				t.Errorf("want validation to pass, got error: %v", err)
			}
			if tt.expected != "" && (err == nil || err.Error() != "config: "+tt.expected) {
				t.Errorf("want error containing '%s', got error: %v", tt.expected, err)
			}
		})
	}
}

// TestNewMiner tests miner creation
func TestNewMiner(t *testing.T) {
	// test valid config
	config := &MinerConfig{
		NodeID:      "test_node",
		OllamaURL:   "http://test",
		Model:       "test_model",
		StakeAmount: 50.0,
		ListenAddr:  ":8080",
	}

	miner, err := NewMiner(config)
	if err != nil {
		t.Fatalf("failed to create miner: %v", err)
	}
	if miner == nil {
		t.Fatal("miner instance unexpectedly nil")
	}
	if miner.Config().NodeID != config.NodeID {
		t.Errorf("miner config mismatch")
	}

	// test nil config
	_, err = NewMiner(nil)
	if err == nil {
		t.Error("nil config should return error")
	}

	// test invalid config
	invalidConfig := &MinerConfig{
		NodeID: "", // invalid NodeID
	}
	_, err = NewMiner(invalidConfig)
	if err == nil {
		t.Error("invalid config should return error")
	}
}

// TestMinerStartStop tests miner start and stop
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
		t.Fatalf("failed to create miner: %v", err)
	}

	ctx := context.Background()

	// test start
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("failed to start miner: %v", err)
	}

	// verify state
	status := miner.Status()
	if !status.Running {
		t.Error("miner state should be running after start")
	}
	if status.NodeID != config.NodeID {
		t.Errorf("in status: NodeID mismatch")
	}

	// test double start
	if err := miner.Start(ctx); err == nil {
		t.Error("double start should return error")
	}

	// test stop
	if err := miner.Stop(); err != nil {
		t.Fatalf("failed to stop miner: %v", err)
	}

	// verify state
	status = miner.Status()
	if status.Running {
		t.Error("miner state should be stopped after stop")
	}

	// test double stop
	if err := miner.Stop(); err == nil {
		t.Error("double stop should return error")
	}
}

// TestMinerStatus tests status retrieval
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
		t.Fatalf("failed to create miner: %v", err)
	}

	// test stopped state
	status := miner.Status()
	if status.Running {
		t.Error("unstarted miner state expected stopped")
	}
	if status.Uptime != 0 {
		t.Error("unstarted miner uptime expected 0")
	}
	if status.TasksProcessed != 0 {
		t.Error("unstarted miner tasks processed expected 0")
	}
	if status.NodeID != config.NodeID {
		t.Errorf("in status: NodeID mismatch")
	}
	if status.Model != config.Model {
		t.Errorf("in status: Model mismatch")
	}

	// test state after start
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("failed to start miner: %v", err)
	}
	defer miner.Stop()

	time.Sleep(100 * time.Millisecond) // wait a short while

	status = miner.Status()
	if !status.Running {
		t.Error("miner state expected running after start")
	}
	if status.Uptime <= 0 {
		t.Error("miner uptime after start should be > 0")
	}
	if !status.StartTime.After(time.Now().Add(-time.Second)) {
		t.Error("start time should be close to now")
	}
}

// TestProcessTask tests task processing
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
		t.Fatalf("failed to create miner: %v", err)
	}

	// create test task
	task := orchestrator.NewTask("task_123", "test prompt", "requester_123", 3, time.Minute)
	task.AssignNodes([]string{"other_node_1", "test_node", "other_node_2"})

	// test processing task when miner not started
	err = miner.ProcessTask(task)
	if err == nil {
		t.Error("should return error when miner not started")
	}

	// start the miner
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("failed to start miner: %v", err)
	}
	defer miner.Stop()

	// test processing task assigned to current node
	err = miner.ProcessTask(task)
	if err != nil {
		t.Fatalf("failed to process task: %v", err)
	}

	// verify task count
	status := miner.Status()
	if status.TasksProcessed != 1 {
		t.Errorf("task count expected 1, actual %d", status.TasksProcessed)
	}

	// test processing task not assigned to current node
	otherTask := orchestrator.NewTask("task_456", "test prompt", "requester_456", 2, time.Minute)
	otherTask.AssignNodes([]string{"other_node_1", "other_node_2"})

	err = miner.ProcessTask(otherTask)
	if err == nil {
		t.Error("processing task not assigned to current node should return error")
	}

	// test nil task
	err = miner.ProcessTask(nil)
	if err == nil {
		t.Error("processing nil task should return error")
	}
}

// TestConfigJSONMarshaling tests JSON marshal/unmarshal
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

	// test marshal
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// test unmarshal
	var decodedConfig MinerConfig
	if err := json.Unmarshal(data, &decodedConfig); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// verify fields match
	if decodedConfig.NodeID != config.NodeID {
		t.Errorf("NodeID mismatch")
	}
	if decodedConfig.OllamaURL != config.OllamaURL {
		t.Errorf("OllamaURL mismatch")
	}
	if decodedConfig.Model != config.Model {
		t.Errorf("Model mismatch")
	}
	if decodedConfig.StakeAmount != config.StakeAmount {
		t.Errorf("StakeAmount mismatch")
	}
	if decodedConfig.ListenAddr != config.ListenAddr {
		t.Errorf("ListenAddr mismatch")
	}
	if decodedConfig.DataDir != config.DataDir {
		t.Errorf("DataDir mismatch")
	}
	if decodedConfig.LogLevel != config.LogLevel {
		t.Errorf("LogLevel mismatch")
	}
}

// TestLoadConfigInvalidPath test loadinvalid path
func TestLoadConfigInvalidPath(t *testing.T) {
	// test empty path
	_, err := LoadConfig("")
	if err == nil {
		t.Error("empty path should return error")
	}

	// test nonexistent file
	_, err = LoadConfig("/tmp/nonexistent_config_123456.json")
	if err == nil {
		t.Error("nonexistent file should return error")
	}

	// test invalid JSON
	tempDir := t.TempDir()
	invalidJSONPath := filepath.Join(tempDir, "invalid.json")
	if err := os.WriteFile(invalidJSONPath, []byte("{ invalid json"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err = LoadConfig(invalidJSONPath)
	if err == nil {
		t.Error("invalid JSON should return error")
	}
}

// TestSaveConfigInvalidPath tests saving to an invalid path
func TestSaveConfigInvalidPath(t *testing.T) {
	config := &MinerConfig{
		NodeID:      "test",
		OllamaURL:   "http://test",
		Model:       "test",
		StakeAmount: 10.0,
		ListenAddr:  ":8080",
	}

	// test empty path
	err := SaveConfig("", config)
	if err == nil {
		t.Error("empty path should return error")
	}

	// test nil config
	err = SaveConfig("/tmp/test.json", nil)
	if err == nil {
		t.Error("nil config should return error")
	}
}

// TestMinerConcurrentAccess tests concurrent access
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
		t.Fatalf("failed to create miner: %v", err)
	}

	// start the miner
	ctx := context.Background()
	if err := miner.Start(ctx); err != nil {
		t.Fatalf("failed to start miner: %v", err)
	}
	defer miner.Stop()

	// concurrent access test
	done := make(chan bool)
	errors := make(chan error, 10)

	// start multiple goroutines accessing state concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				status := miner.Status()
				if status.NodeID != config.NodeID {
					errors <- fmt.Errorf("goroutine %d got mismatched NodeID", id)
					return
				}
				time.Sleep(time.Millisecond)
			}
			done <- true
		}(i)
	}

	// wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case err := <-errors:
			t.Fatalf("concurrent access error: %v", err)
		}
	}
}
