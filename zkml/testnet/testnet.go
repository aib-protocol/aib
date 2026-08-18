package testnet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aib-protocol/aib/zkml/inference"
	"github.com/aib-protocol/aib/zkml/orchestrator"
	"github.com/aib-protocol/aib/zkml/verification"
)

// TestNetConfig configures the testnet simulator
type TestNetConfig struct {
	NodeCount      int           // Number of nodes (default 3)
	MinNodes       int           // Minimum verification nodes
	CommitDuration time.Duration // Commit phase duration
	RevealDuration time.Duration // Reveal phase duration
	TaskTimeout    time.Duration // Task timeout
	AutoSlash      bool          // Auto-slash on disagreement
	HonestRatio    float64       // Honest node ratio (0-1, for simulation)

	// Real AI inference (optional)
	UseRealAI  bool                      // Use real AI API instead of mock
	AIProvider inference.AnthropicConfig // Anthropic API config
}

// DefaultTestNetConfig returns sensible defaults for testing
func DefaultTestNetConfig() *TestNetConfig {
	return &TestNetConfig{
		NodeCount:      3,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		TaskTimeout:    5 * time.Second,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}
}

// SimNode simulates a network node
type SimNode struct {
	ID      string
	Honest  bool          // Honest nodes return correct results
	Model   string        // Model name
	Stake   float64       // Staked amount
	Online  bool          // Whether online
	Latency time.Duration // Simulated network latency
}

// TestNetResult holds the result of a single task submission
type TestNetResult struct {
	TaskID         string
	Prompt         string
	FinalResult    string
	IsValid        bool
	AgreementRate  float64
	Disagreeing    []string
	SlashTriggered int
	Duration       time.Duration
	NodeResults    map[string]string
	Events         []*orchestrator.Event
}

// TestNetStats holds cumulative statistics
type TestNetStats struct {
	TotalTasks   int
	PassedTasks  int
	FailedTasks  int
	TotalSlashes int
	TotalEvents  int
	AvgDuration  time.Duration
	NodeStats    map[string]*NodeStats
}

// NodeStats tracks per-node statistics
type NodeStats struct {
	TasksAssigned  int
	TasksCompleted int
	SlashCount     int
	Online         bool
	Honest         bool
}

// TestNet simulates a multi-node testnet environment
type TestNet struct {
	mu           sync.RWMutex
	orchestrator *orchestrator.Orchestrator
	nodes        map[string]*SimNode
	nodeCount    int
	config       *TestNetConfig
	eventLog     []*orchestrator.Event
	running      bool

	// Real AI inference
	aiProvider *inference.AnthropicProvider

	// Stats tracking
	totalTasks   int
	passedTasks  int
	failedTasks  int
	totalSlashes int
	durations    []time.Duration
}

// NewTestNet creates a new testnet simulator with the given configuration
func NewTestNet(config *TestNetConfig) *TestNet {
	if config == nil {
		config = DefaultTestNetConfig()
	}

	orchConfig := &orchestrator.OrchestratorConfig{
		MinNodes:       config.MinNodes,
		CommitDuration: config.CommitDuration,
		RevealDuration: config.RevealDuration,
		TaskTimeout:    config.TaskTimeout,
		AutoSlash:      config.AutoSlash,
	}

	tn := &TestNet{
		orchestrator: orchestrator.NewOrchestrator(orchConfig),
		nodes:        make(map[string]*SimNode),
		nodeCount:    config.NodeCount,
		config:       config,
		eventLog:     make([]*orchestrator.Event, 0),
	}

	// Initialize real AI provider if enabled
	if config.UseRealAI {
		tn.aiProvider = inference.NewAnthropicProvider(&config.AIProvider)
	}

	return tn
}

// Start starts the testnet by registering all nodes
func (tn *TestNet) Start() error {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	if tn.running {
		return fmt.Errorf("testnet: already running")
	}

	// Subscribe to all events for logging
	tn.orchestrator.EventBus().SubscribeAll(func(e *orchestrator.Event) {
		tn.mu.Lock()
		tn.eventLog = append(tn.eventLog, e)
		tn.mu.Unlock()
	})

	// Create and register nodes
	honestCount := int(float64(tn.nodeCount) * tn.config.HonestRatio)
	for i := 0; i < tn.nodeCount; i++ {
		nodeID := fmt.Sprintf("sim-node-%d", i)
		isHonest := i < honestCount

		node := &SimNode{
			ID:      nodeID,
			Honest:  isHonest,
			Model:   "test-model-v1",
			Stake:   1000.0,
			Online:  true,
			Latency: 10 * time.Millisecond,
		}
		tn.nodes[nodeID] = node

		err := tn.orchestrator.Scheduler().RegisterNode(&orchestrator.NodeInfo{
			ID:     nodeID,
			Active: true,
			Stake:  node.Stake,
		})
		if err != nil {
			return fmt.Errorf("testnet: failed to register node %s: %w", nodeID, err)
		}
	}

	tn.running = true
	return nil
}

// Stop stops the testnet
func (tn *TestNet) Stop() error {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	if !tn.running {
		return fmt.Errorf("testnet: not running")
	}

	tn.running = false
	return nil
}

// IsRunning returns whether the testnet is currently running
func (tn *TestNet) IsRunning() bool {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	return tn.running
}

// Orchestrator returns the underlying orchestrator
func (tn *TestNet) Orchestrator() *orchestrator.Orchestrator {
	return tn.orchestrator
}

// GetNode returns a node by ID
func (tn *TestNet) GetNode(nodeID string) (*SimNode, bool) {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	node, ok := tn.nodes[nodeID]
	return node, ok
}

// GetNodes returns all nodes
func (tn *TestNet) GetNodes() map[string]*SimNode {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	result := make(map[string]*SimNode, len(tn.nodes))
	for k, v := range tn.nodes {
		result[k] = v
	}
	return result
}

// SetNodeOnline simulates a node going online or offline
func (tn *TestNet) SetNodeOnline(nodeID string, online bool) error {
	tn.mu.Lock()
	node, ok := tn.nodes[nodeID]
	if !ok {
		tn.mu.Unlock()
		return fmt.Errorf("testnet: node %s not found", nodeID)
	}
	node.Online = online
	tn.mu.Unlock()

	return tn.orchestrator.Scheduler().SetNodeActive(nodeID, online)
}

// SetNodeHonest changes a node's behavior (honest or dishonest)
func (tn *TestNet) SetNodeHonest(nodeID string, honest bool) error {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	node, ok := tn.nodes[nodeID]
	if !ok {
		return fmt.Errorf("testnet: node %s not found", nodeID)
	}
	node.Honest = honest
	return nil
}

// SubmitTask submits a task and runs the full commit-reveal-verify pipeline
func (tn *TestNet) SubmitTask(prompt string) (*TestNetResult, error) {
	tn.mu.RLock()
	if !tn.running {
		tn.mu.RUnlock()
		return nil, fmt.Errorf("testnet: not running")
	}
	tn.mu.RUnlock()

	startTime := time.Now()
	result := &TestNetResult{
		Prompt:      prompt,
		NodeResults: make(map[string]string),
		Events:      make([]*orchestrator.Event, 0),
	}

	// Record event start index
	tn.mu.RLock()
	eventStartIdx := len(tn.eventLog)
	tn.mu.RUnlock()

	// 1. Submit task to orchestrator
	task, err := tn.orchestrator.SubmitTask(prompt, "testnet-requester")
	if err != nil {
		return nil, fmt.Errorf("testnet: submit task failed: %w", err)
	}
	result.TaskID = task.ID

	// 2. Pre-compute all node results BEFORE starting commit phase
	// This is critical when using real AI (which takes time)
	type nodeCommit struct {
		nodeID string
		result string
		nonce  []byte
	}
	precomputedResults := make(map[string]string)

	for _, nodeID := range task.AssignedTo {
		tn.mu.RLock()
		node, ok := tn.nodes[nodeID]
		tn.mu.RUnlock()

		if !ok || !node.Online {
			continue
		}

		// Determine result based on honesty - do this BEFORE commit phase starts
		var nodeResult string
		if tn.config.UseRealAI && tn.aiProvider != nil && node.Honest {
			// Use real AI inference for honest nodes
			ctx, cancel := context.WithTimeout(context.Background(), tn.config.TaskTimeout)
			result, err := tn.aiProvider.Infer(ctx, prompt)
			cancel()
			if err != nil {
				nodeResult = generateHonestResult(prompt)
			} else {
				// Normalize AI response (trim whitespace) for consistency
				nodeResult = strings.TrimSpace(result)
			}
		} else if node.Honest {
			nodeResult = generateHonestResult(prompt)
		} else {
			nodeResult = generateDishonestResult(prompt, nodeID)
		}

		precomputedResults[nodeID] = nodeResult
	}

	// 3. Start commit phase AFTER all results are computed
	if err := tn.orchestrator.StartCommitPhase(task.ID); err != nil {
		return nil, fmt.Errorf("testnet: start commit phase failed: %w", err)
	}

	// 4. Submit commits using precomputed results
	commits := make([]nodeCommit, 0, len(task.AssignedTo))

	for _, nodeID := range task.AssignedTo {
		tn.mu.RLock()
		node, ok := tn.nodes[nodeID]
		tn.mu.RUnlock()

		if !ok || !node.Online {
			continue
		}

		nodeResult, hasResult := precomputedResults[nodeID]
		if !hasResult {
			continue
		}

		// Simulate latency
		if node.Latency > 0 {
			time.Sleep(node.Latency)
		}

		// Generate nonce
		nonce := make([]byte, 32)
		rand.Read(nonce)

		// Compute commit hash
		commitHash := verification.ComputeCommitHash(nodeResult, nonce)

		// Submit commit
		if err := tn.orchestrator.SubmitCommit(task.ID, nodeID, commitHash); err != nil {
			continue
		}

		commits = append(commits, nodeCommit{
			nodeID: nodeID,
			result: nodeResult,
			nonce:  nonce,
		})
	}

	// 4. Wait for commit phase to end
	time.Sleep(tn.config.CommitDuration + 10*time.Millisecond)

	// 5. Start reveal phase
	if err := tn.orchestrator.StartRevealPhase(task.ID); err != nil {
		return nil, fmt.Errorf("testnet: start reveal phase failed: %w", err)
	}

	// 6. Simulate node reveals
	for _, commit := range commits {
		tn.mu.RLock()
		node, ok := tn.nodes[commit.nodeID]
		tn.mu.RUnlock()

		if !ok || !node.Online {
			continue
		}

		if node.Latency > 0 {
			time.Sleep(node.Latency)
		}

		if err := tn.orchestrator.SubmitReveal(task.ID, commit.nodeID, commit.result, commit.nonce); err != nil {
			continue // Node failed to reveal, skip
		}

		result.NodeResults[commit.nodeID] = commit.result
	}

	// 7. Wait for reveal phase to end
	time.Sleep(tn.config.RevealDuration + 10*time.Millisecond)

	// 8. Verify
	vResult, err := tn.orchestrator.Verify(task.ID)
	if err != nil {
		return nil, fmt.Errorf("testnet: verify failed: %w", err)
	}

	result.IsValid = vResult.IsValid
	result.FinalResult = vResult.MajorityResult
	result.AgreementRate = vResult.AgreementRate
	result.Disagreeing = vResult.Disagreeing

	// 9. Settle task
	if err := tn.orchestrator.SettleTask(task.ID); err != nil {
		return nil, fmt.Errorf("testnet: settle failed: %w", err)
	}

	result.Duration = time.Since(startTime)

	// Count slash events from event log
	tn.mu.Lock()
	for i := eventStartIdx; i < len(tn.eventLog); i++ {
		result.Events = append(result.Events, tn.eventLog[i])
		if tn.eventLog[i].Type == orchestrator.EventSlashTriggered {
			result.SlashTriggered++
		}
	}

	// Update stats
	tn.totalTasks++
	if vResult.IsValid {
		tn.passedTasks++
	} else {
		tn.failedTasks++
	}
	tn.totalSlashes += result.SlashTriggered
	tn.durations = append(tn.durations, result.Duration)
	tn.mu.Unlock()

	return result, nil
}

// RunScenario runs a predefined scenario
func (tn *TestNet) RunScenario(scenario *Scenario) (*ScenarioResult, error) {
	tn.mu.RLock()
	if !tn.running {
		tn.mu.RUnlock()
		return nil, fmt.Errorf("testnet: not running")
	}
	tn.mu.RUnlock()

	sr := &ScenarioResult{
		ScenarioName: scenario.Name,
		TaskResults:  make([]*TestNetResult, 0, len(scenario.Tasks)),
	}

	startTime := time.Now()

	// Run setup if provided
	if scenario.Setup != nil {
		if err := scenario.Setup(tn); err != nil {
			return nil, fmt.Errorf("testnet: scenario setup failed: %w", err)
		}
	}

	// Execute each task in the scenario
	allPassed := true
	for _, st := range scenario.Tasks {
		result, err := tn.SubmitTask(st.Prompt)
		if err != nil {
			// If the task fails to submit (e.g., not enough nodes), record as failure
			sr.TaskResults = append(sr.TaskResults, &TestNetResult{
				Prompt:  st.Prompt,
				IsValid: false,
			})
			if st.ExpectedPass {
				allPassed = false
			}
			continue
		}

		sr.TaskResults = append(sr.TaskResults, result)

		// Check if result matches expectation
		if result.IsValid != st.ExpectedPass {
			allPassed = false
		}
	}

	sr.AllExpectationsMet = allPassed
	sr.Duration = time.Since(startTime)

	return sr, nil
}

// GetStats returns cumulative statistics
func (tn *TestNet) GetStats() *TestNetStats {
	tn.mu.RLock()
	defer tn.mu.RUnlock()

	stats := &TestNetStats{
		TotalTasks:   tn.totalTasks,
		PassedTasks:  tn.passedTasks,
		FailedTasks:  tn.failedTasks,
		TotalSlashes: tn.totalSlashes,
		TotalEvents:  len(tn.eventLog),
		NodeStats:    make(map[string]*NodeStats),
	}

	// Calculate average duration
	if len(tn.durations) > 0 {
		total := time.Duration(0)
		for _, d := range tn.durations {
			total += d
		}
		stats.AvgDuration = total / time.Duration(len(tn.durations))
	}

	// Node stats
	for id, node := range tn.nodes {
		stats.NodeStats[id] = &NodeStats{
			Online: node.Online,
			Honest: node.Honest,
		}
	}

	return stats
}

// GetEventLog returns a copy of the event log
func (tn *TestNet) GetEventLog() []*orchestrator.Event {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	result := make([]*orchestrator.Event, len(tn.eventLog))
	copy(result, tn.eventLog)
	return result
}

// generateHonestResult generates a deterministic "correct" result for a prompt
func generateHonestResult(prompt string) string {
	return fmt.Sprintf("honest_result_for_%x", hashPrompt(prompt))
}

// generateDishonestResult generates a different "incorrect" result for a prompt
// Each nodeID gets a different result to simulate Byzantine behavior
func generateDishonestResult(prompt string, nodeID string) string {
	// Combine prompt and nodeID to create unique hash
	data := fmt.Sprintf("%s:%s", prompt, nodeID)
	return fmt.Sprintf("dishonest_result_for_%x", hashPrompt(data))
}

// hashPrompt creates a short hash of a prompt for deterministic results
func hashPrompt(prompt string) []byte {
	// use SHA-256 for a deterministic hash instead of truncating the first 8 bytes
	h := sha256.Sum256([]byte(prompt))
	return h[:8]
}
