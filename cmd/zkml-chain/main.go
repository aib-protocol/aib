package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aib-protocol/aib/zkml/consensus"
	"github.com/aib-protocol/aib/zkml/inference"
	"github.com/aib-protocol/aib/zkml/orchestrator"
	"github.com/aib-protocol/aib/zkml/verification"
)

// ZKMLBlockchain integrates ZKML verification with blockchain
type ZKMLBlockchain struct {
	mu sync.RWMutex

	// Components
	blockchain   *consensus.Blockchain
	orchestrator *orchestrator.Orchestrator
	aiProvider   *inference.AnthropicProvider

	// Configuration
	config *Config

	// State
	running    bool
	taskCount  int
	blockCount int
	startTime  time.Time

	// Task queue
	taskQueue []string
}

// Config holds the configuration
type Config struct {
	// AI Provider
	AIProviderAIURL  string
	AIProviderAPIKey string
	AIProviderModel  string

	// Network
	NodeID   string
	Port     int
	MinNodes int

	// Timing
	CommitDuration time.Duration
	RevealDuration time.Duration
	TaskInterval   time.Duration
	TaskTimeout    time.Duration

	// Blockchain
	BlockInterval time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		AIProviderAIURL:  "http://217.216.43.45:51201/key/rk-e9412b1f5e955a92bbca9627",
		AIProviderAPIKey: "rk-e9412b1f5e955a92bbca9627",
		AIProviderModel:  "glm-5",
		NodeID:           "zkml-node-1",
		Port:             51200,
		MinNodes:         1,
		CommitDuration:   5 * time.Second,
		RevealDuration:   5 * time.Second,
		TaskInterval:     10 * time.Second,
		TaskTimeout:      30 * time.Second,
		BlockInterval:    10 * time.Second,
	}
}

// NewZKMLBlockchain creates a new ZKML blockchain
func NewZKMLBlockchain(config *Config) (*ZKMLBlockchain, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create AI provider
	aiConfig := &inference.AnthropicConfig{
		BaseURL: config.AIProviderAIURL,
		APIKey:  config.AIProviderAPIKey,
		Model:   config.AIProviderModel,
		Timeout: config.TaskTimeout,
	}
	aiProvider := inference.NewAnthropicProvider(aiConfig)

	// Create orchestrator with custom MinNodes
	orchConfig := &orchestrator.OrchestratorConfig{
		MinNodes:       config.MinNodes,
		CommitDuration: config.CommitDuration,
		RevealDuration: config.RevealDuration,
		TaskTimeout:    config.TaskTimeout,
		AutoSlash:      true,
	}
	orch := orchestrator.NewOrchestrator(orchConfig)

	// Register node
	orch.Scheduler().RegisterNode(&orchestrator.NodeInfo{
		ID:     config.NodeID,
		Active: true,
		Stake:  1000.0,
	})

	// Create blockchain
	bcConfig := &consensus.Config{
		BlockInterval:     config.BlockInterval,
		MaxBlocksInMemory: 10000,
		AutoProduce:       false, // We produce blocks from tasks
		MinAgreementRate:  0.5,
		GenesisTaskID:     "genesis",
		GenesisResult:     "AIB ZKML Blockchain Genesis",
		GenesisTimestamp:  time.Now().Unix(),
	}
	bc, err := consensus.NewBlockchain(nil, bcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	zkb := &ZKMLBlockchain{
		blockchain:   bc,
		orchestrator: orch,
		aiProvider:   aiProvider,
		config:       config,
		taskQueue:    make([]string, 0),
	}

	// Set up block production callback
	bc.SetBlockProducedCallback(func(block *consensus.Block) {
		zkb.mu.Lock()
		zkb.blockCount++
		log.Printf("[BLOCK] New block #%d produced: TaskID=%s, Result=%s, Agreement=%.2f%%",
			block.Height, block.TaskID, block.FinalResult, block.AgreementRate*100)
		zkb.mu.Unlock()
	})

	return zkb, nil
}

// Start starts the ZKML blockchain
func (zkb *ZKMLBlockchain) Start() error {
	zkb.mu.Lock()
	defer zkb.mu.Unlock()

	if zkb.running {
		return fmt.Errorf("already running")
	}

	// Start blockchain
	if err := zkb.blockchain.Start(); err != nil {
		return fmt.Errorf("failed to start blockchain: %w", err)
	}

	zkb.running = true
	zkb.startTime = time.Now()

	// Start task processor
	go zkb.taskLoop()

	log.Printf("[ZKML] Started - Node: %s, Genesis block created", zkb.config.NodeID)
	return nil
}

// Stop stops the ZKML blockchain
func (zkb *ZKMLBlockchain) Stop() error {
	zkb.mu.Lock()
	defer zkb.mu.Unlock()

	if !zkb.running {
		return fmt.Errorf("not running")
	}

	zkb.running = false
	zkb.blockchain.Stop()

	log.Printf("[ZKML] Stopped - Tasks: %d, Blocks: %d", zkb.taskCount, zkb.blockCount)
	return nil
}

// SubmitTask submits a new inference task
func (zkb *ZKMLBlockchain) SubmitTask(prompt string) (*consensus.Block, error) {
	zkb.mu.RLock()
	if !zkb.running {
		zkb.mu.RUnlock()
		return nil, fmt.Errorf("not running")
	}
	zkb.mu.RUnlock()

	log.Printf("[TASK] Submitting task with prompt: %s", prompt)

	// 1. Submit task to orchestrator
	task, err := zkb.orchestrator.SubmitTask(prompt, "requester")
	if err != nil {
		return nil, fmt.Errorf("submit task failed: %w", err)
	}

	// 2. Run AI inference
	ctx, cancel := context.WithTimeout(context.Background(), zkb.config.TaskTimeout)
	result, err := zkb.aiProvider.Infer(ctx, prompt)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("AI inference failed: %w", err)
	}

	log.Printf("[TASK] AI inference result: %s", result)

	// 3. Start commit phase
	if err := zkb.orchestrator.StartCommitPhase(task.ID); err != nil {
		return nil, fmt.Errorf("start commit phase failed: %w", err)
	}

	// 4. Submit commit
	nonce := make([]byte, 32)
	rand.Read(nonce)
	commitHash := verification.ComputeCommitHash(result, nonce)
	if err := zkb.orchestrator.SubmitCommit(task.ID, zkb.config.NodeID, commitHash); err != nil {
		return nil, fmt.Errorf("submit commit failed: %w", err)
	}

	// 5. Wait and start reveal phase
	time.Sleep(zkb.config.CommitDuration + 100*time.Millisecond)
	if err := zkb.orchestrator.StartRevealPhase(task.ID); err != nil {
		return nil, fmt.Errorf("start reveal phase failed: %w", err)
	}

	// 6. Submit reveal
	if err := zkb.orchestrator.SubmitReveal(task.ID, zkb.config.NodeID, result, nonce); err != nil {
		return nil, fmt.Errorf("submit reveal failed: %w", err)
	}

	// 7. Wait and verify
	time.Sleep(zkb.config.RevealDuration + 100*time.Millisecond)
	vResult, err := zkb.orchestrator.Verify(task.ID)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	log.Printf("[TASK] Verification result: Valid=%v, Agreement=%.2f%%", vResult.IsValid, vResult.AgreementRate*100)

	// 8. Create block event
	// Calculate consensus nodes (nodes that agreed with majority)
	consensusNodes := make([]string, 0)
	for nodeID, result := range vResult.NodeResults {
		if result == vResult.MajorityResult {
			consensusNodes = append(consensusNodes, nodeID)
		}
	}

	event := &consensus.BlockEvent{
		TaskID:           task.ID,
		FinalResult:      vResult.MajorityResult,
		IsValid:          vResult.IsValid,
		AgreementRate:    vResult.AgreementRate,
		NodeResults:      vResult.NodeResults,
		ConsensusNodes:   consensusNodes,
		DisagreeingNodes: vResult.Disagreeing,
		Metadata: map[string]string{
			"prompt": prompt,
			"model":  zkb.config.AIProviderModel,
		},
		Timestamp:   time.Now().Unix(),
		BlockHeight: zkb.blockchain.GetBlockCount(),
	}

	// 9. Add block to blockchain
	if err := zkb.blockchain.AddBlockEvent(event); err != nil {
		return nil, fmt.Errorf("add block failed: %w", err)
	}

	// 10. Settle task
	zkb.orchestrator.SettleTask(task.ID)

	zkb.mu.Lock()
	zkb.taskCount++
	zkb.mu.Unlock()

	// Return the new block
	return zkb.blockchain.GetLatestBlock()
}

// taskLoop periodically submits tasks
func (zkb *ZKMLBlockchain) taskLoop() {
	prompts := []string{
		"What is 2+2?",
		"What is the capital of France?",
		"Explain quantum computing in one sentence.",
		"What is the meaning of life?",
		"How does blockchain work?",
	}

	idx := 0
	ticker := time.NewTicker(zkb.config.TaskInterval)
	defer ticker.Stop()

	for {
		zkb.mu.RLock()
		running := zkb.running
		zkb.mu.RUnlock()

		if !running {
			return
		}

		select {
		case <-ticker.C:
			prompt := prompts[idx%len(prompts)]
			idx++

			block, err := zkb.SubmitTask(prompt)
			if err != nil {
				log.Printf("[ERROR] Task failed: %v", err)
				continue
			}
			log.Printf("[SUCCESS] Block #%d created from task", block.Height)
		}
	}
}

// GetStatus returns the current status
func (zkb *ZKMLBlockchain) GetStatus() map[string]interface{} {
	zkb.mu.RLock()
	defer zkb.mu.RUnlock()

	latestBlock, _ := zkb.blockchain.GetLatestBlock()

	status := map[string]interface{}{
		"running":     zkb.running,
		"node_id":     zkb.config.NodeID,
		"task_count":  zkb.taskCount,
		"block_count": zkb.blockchain.GetBlockCount(),
		"uptime":      time.Since(zkb.startTime).String(),
	}

	if latestBlock != nil {
		status["latest_block"] = latestBlock.ToDisplayMap()
	}

	return status
}

// GetBlockchain returns the blockchain
func (zkb *ZKMLBlockchain) GetBlockchain() *consensus.Blockchain {
	return zkb.blockchain
}

// HTTP Handlers

func (zkb *ZKMLBlockchain) handleStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(zkb.GetStatus())
}

func (zkb *ZKMLBlockchain) handleBlocks(w http.ResponseWriter, r *http.Request) {
	blocks := zkb.blockchain.GetAllBlocks()
	result := make([]map[string]interface{}, len(blocks))
	for i, block := range blocks {
		result[i] = block.ToDisplayMap()
	}
	json.NewEncoder(w).Encode(result)
}

func (zkb *ZKMLBlockchain) handleLatestBlock(w http.ResponseWriter, r *http.Request) {
	block, err := zkb.blockchain.GetLatestBlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(block.ToDisplayMap())
}

func (zkb *ZKMLBlockchain) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}

	block, err := zkb.SubmitTask(req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"block":   block.ToDisplayMap(),
	})
}

// StartHTTPServer starts the HTTP API server
func (zkb *ZKMLBlockchain) StartHTTPServer() error {
	http.HandleFunc("/api/status", zkb.handleStatus)
	http.HandleFunc("/api/blocks", zkb.handleBlocks)
	http.HandleFunc("/api/block/latest", zkb.handleLatestBlock)
	http.HandleFunc("/api/task/submit", zkb.handleSubmitTask)

	addr := fmt.Sprintf(":%d", zkb.config.Port)
	log.Printf("[HTTP] Starting server on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func main() {
	config := DefaultConfig()

	flag.StringVar(&config.NodeID, "node", config.NodeID, "Node ID")
	flag.IntVar(&config.Port, "port", config.Port, "HTTP API port")
	flag.StringVar(&config.AIProviderAIURL, "ai-url", config.AIProviderAIURL, "AI Provider URL")
	flag.StringVar(&config.AIProviderAPIKey, "ai-key", config.AIProviderAPIKey, "AI API Key")
	flag.StringVar(&config.AIProviderModel, "ai-model", config.AIProviderModel, "AI Model")
	flag.DurationVar(&config.TaskInterval, "interval", config.TaskInterval, "Task submission interval")
	flag.Parse()

	// Create ZKML blockchain
	zkb, err := NewZKMLBlockchain(config)
	if err != nil {
		log.Fatalf("Failed to create ZKML blockchain: %v", err)
	}

	// Start
	if err := zkb.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in goroutine
	go func() {
		if err := zkb.StartHTTPServer(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown
	<-sigCh
	log.Println("Shutting down...")
	zkb.Stop()
}
