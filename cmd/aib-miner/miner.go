package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/zkml/orchestrator"
)

// MinerStatus holds miner status info
type MinerStatus struct {
	NodeID         string        `json:"node_id"`         // node ID
	Running        bool          `json:"running"`         // whether running
	Uptime         time.Duration `json:"uptime"`          // uptime duration
	TasksProcessed int           `json:"tasks_processed"` // number of processed tasks
	Model          string        `json:"model"`           // model in use
	StartTime      time.Time     `json:"start_time"`      // start time
}

// Miner is the core miner node, managing inference task execution
type Miner struct {
	config     *MinerConfig               // miner config
	orch       *orchestrator.Orchestrator // orchestrator instance
	scheduler  *orchestrator.Scheduler    // scheduler
	running    bool                       // running flag
	startedAt  time.Time                  // start time
	tasksCount int                        // processed task count
	mu         sync.RWMutex               // read/write lock, protects concurrent access
	cancel     context.CancelFunc         // cancel function, used to stop the main loop
}

// NewMiner creates a new miner instance
func NewMiner(config *MinerConfig) (*Miner, error) {
	if config == nil {
		return nil, fmt.Errorf("miner: config must not be nil")
	}

	// validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("miner: config validation failed: %w", err)
	}

	// create orchestrator with default config
	orchConfig := orchestrator.DefaultOrchestratorConfig()
	orch := orchestrator.NewOrchestrator(orchConfig)

	return &Miner{
		config:    config,
		orch:      orch,
		scheduler: orch.Scheduler(),
		running:   false,
	}, nil
}

// Start launches the miner node
func (m *Miner) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("miner: already running")
	}

	// register node to scheduler
	nodeInfo := &orchestrator.NodeInfo{
		ID:      m.config.NodeID,
		ModelID: []byte(m.config.Model), // use model name as ModelID
		Stake:   m.config.StakeAmount,
		Active:  true,
	}

	if err := m.scheduler.RegisterNode(nodeInfo); err != nil {
		return fmt.Errorf("miner: failed to register node: %w", err)
	}

	// create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.running = true
	m.startedAt = time.Now()

	// subscribe to events, listen for task assignment
	m.orch.EventBus().Subscribe(orchestrator.EventTaskAssigned, m.handleTaskAssigned)

	// start main loop (background goroutine)
	go m.runLoop(ctx)

	return nil
}

// Stop stops the miner node
func (m *Miner) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner: not running")
	}

	// cancel context to stop main loop
	if m.cancel != nil {
		m.cancel()
	}

	// unregister node from scheduler
	m.scheduler.UnregisterNode(m.config.NodeID)

	m.running = false
	return nil
}

// Status returns the current miner status
func (m *Miner) Status() *MinerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime time.Duration
	if m.running {
		uptime = time.Since(m.startedAt)
	}

	return &MinerStatus{
		NodeID:         m.config.NodeID,
		Running:        m.running,
		Uptime:         uptime,
		TasksProcessed: m.tasksCount,
		Model:          m.config.Model,
		StartTime:      m.startedAt,
	}
}

// ProcessTask processes a single inference task
func (m *Miner) ProcessTask(task *orchestrator.Task) error {
	if task == nil {
		return fmt.Errorf("miner: task must not be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner: not running, cannot process task")
	}

	// check whether task is assigned to current node
	assignedToMe := false
	for _, nodeID := range task.AssignedTo {
		if nodeID == m.config.NodeID {
			assignedToMe = true
			break
		}
	}

	if !assignedToMe {
		return fmt.Errorf("miner: task %s not assigned to current node", task.ID)
	}

	// simulate inference (should call Ollama API in production)
	// demonstration only; real inference is implemented by inference module
	result := fmt.Sprintf("inference_result_for_%s", task.ID)

	// set task result
	task.SetResult(m.config.NodeID, result)

	// increment task count
	m.tasksCount++

	return nil
}

// handleTaskAssigned handles task assignment events
func (m *Miner) handleTaskAssigned(event *orchestrator.Event) {
	if event == nil {
		return
	}

	// fetch task details
	task, err := m.orch.GetTask(event.TaskID)
	if err != nil {
		return
	}

	// check whether assigned to current node
	for _, nodeID := range task.AssignedTo {
		if nodeID == m.config.NodeID {
			// process task asynchronously to avoid blocking event bus
			go func(t *orchestrator.Task) {
				if err := m.ProcessTask(t); err != nil {
					// log error but do not abort
					fmt.Printf("failed to process task: %v\n", err)
				}
			}(task)
			break
		}
	}
}

// runLoop is the main loop, listening for and processing tasks
func (m *Miner) runLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// context cancelled, exit loop
			return
		case <-ticker.C:
			// periodically check task status (extensible: heartbeat, health checks, etc.)
			// currently minimal implementation
		}
	}
}

// Orchestrator returns the orchestrator instance (for testing and external access)
func (m *Miner) Orchestrator() *orchestrator.Orchestrator {
	return m.orch
}

// Config returns miner config (read-only access)
func (m *Miner) Config() *MinerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}
