package consensus

import (
	"fmt"
	"sync"
	"time"
)

// BlockProducer handles automatic block production
type BlockProducer struct {
	bc      *Blockchain
	config  *Config
	running bool
	mu      sync.RWMutex
	ticker  *time.Ticker
	stopCh  chan struct{}
}

// NewBlockProducer creates a new block producer
func NewBlockProducer(bc *Blockchain, config *Config) *BlockProducer {
	return &BlockProducer{
		bc:     bc,
		config: config,
		stopCh: make(chan struct{}),
	}
}

// Start starts the block producer
func (bp *BlockProducer) Start() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.running {
		return
	}

	bp.running = true
	bp.ticker = time.NewTicker(bp.config.BlockInterval)

	go bp.run()
}

// Stop stops the block producer
func (bp *BlockProducer) Stop() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if !bp.running {
		return
	}

	bp.running = false
	bp.ticker.Stop()

	select {
	case bp.stopCh <- struct{}{}:
	default:
	}
}

// IsRunning returns whether the producer is running
func (bp *BlockProducer) IsRunning() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.running
}

// run is the main producer loop
func (bp *BlockProducer) run() {
	for {
		select {
		case <-bp.stopCh:
			return
		case <-bp.ticker.C:
			bp.produceBlock()
		}
	}
}

// produceBlock creates a new block if there's pending verification data
func (bp *BlockProducer) produceBlock() {
	// Check if there's pending verification data
	// For now, create a placeholder block if AutoProduce is enabled
	if !bp.config.AutoProduce {
		return
	}

	// Get latest block
	latest, err := bp.bc.GetLatestBlock()
	if err != nil {
		return
	}

	// Create a new block with verification pending status
	event := &BlockEvent{
		TaskID:         fmt.Sprintf("auto-block-%d", latest.Height+1),
		FinalResult:    "Auto-produced block",
		IsValid:        true,
		AgreementRate:  1.0,
		NodeResults:    map[string]string{"producer": "idle"},
		ConsensusNodes: []string{"producer"},
		Metadata:       map[string]string{"type": "auto"},
		Timestamp:      time.Now().Unix(),
		BlockHeight:    latest.Height + 1,
	}

	if err := bp.bc.AddBlockEvent(event); err != nil {
		fmt.Printf("BlockProducer: failed to produce block: %v\n", err)
	}
}

// GetStats returns producer statistics
func (bp *BlockProducer) GetStats() map[string]interface{} {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	return map[string]interface{}{
		"running":        bp.running,
		"block_interval": bp.config.BlockInterval.String(),
		"auto_produce":   bp.config.AutoProduce,
	}
}

// Monitor monitors the blockchain and handles events
type Monitor struct {
	bc      *Blockchain
	config  *Config
	running bool
	mu      sync.RWMutex
	stopCh  chan struct{}
}

// NewMonitor creates a new monitor
func NewMonitor(bc *Blockchain, config *Config) *Monitor {
	return &Monitor{
		bc:     bc,
		config: config,
		stopCh: make(chan struct{}),
	}
}

// Start starts the monitor
func (m *Monitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.running = true

	go m.run()
}

// Stop stops the monitor
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false

	select {
	case m.stopCh <- struct{}{}:
	default:
	}
}

// IsRunning returns whether the monitor is running
func (m *Monitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// run is the main monitor loop
func (m *Monitor) run() {
	eventCh := m.bc.GetEventChannel()

	for {
		select {
		case <-m.stopCh:
			return
		case event := <-eventCh:
			m.handleEvent(event)
		}
	}
}

// handleEvent processes a block event
func (m *Monitor) handleEvent(event *BlockEvent) {
	if event == nil {
		return
	}

	// Validate event
	if event.TaskID == "" {
		return
	}

	// Check agreement rate
	if event.IsValid && event.AgreementRate < m.config.MinAgreementRate {
		fmt.Printf("Monitor: event %s rejected - agreement rate %.2f%% below minimum %.2f%%\n",
			event.TaskID, event.AgreementRate*100, m.config.MinAgreementRate*100)
		return
	}

	// Create block from event
	if err := m.bc.AddBlockEvent(event); err != nil {
		fmt.Printf("Monitor: failed to process event %s: %v\n", event.TaskID, err)
	}
}

// GetStats returns monitor statistics
func (m *Monitor) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"running":            m.running,
		"min_agreement_rate": m.config.MinAgreementRate,
	}
}
