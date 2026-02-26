package consensus

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Blockchain represents the ZKML verification blockchain
type Blockchain struct {
	mu       sync.RWMutex
	blocks   []*Block
	store    Storage
	running  bool
	eventCh  chan *BlockEvent
	producer *BlockProducer
	monitor  *Monitor

	// Configuration
	config *Config

	// Callbacks
	onBlockProduced   func(*Block)
	onBlockVerified  func(*Block)
	onVerificationFailed func(*BlockEvent, error)
}

// Config holds blockchain configuration
type Config struct {
	// Block production interval (for auto-production)
	BlockInterval time.Duration

	// Maximum blocks to keep in memory
	MaxBlocksInMemory int

	// Enable auto block production
	AutoProduce bool

	// Minimum agreement rate for valid verification
	MinAgreementRate float64

	// Genesis block data
	GenesisTaskID    string
	GenesisResult    string
	GenesisTimestamp int64
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		BlockInterval:     5 * time.Second,
		MaxBlocksInMemory: 10000,
		AutoProduce:      true,
		MinAgreementRate: 0.5,
		GenesisTaskID:    "genesis",
		GenesisResult:    "ZKML Blockchain Genesis Block",
		GenesisTimestamp: 1704067200, // 2024-01-01 00:00:00 UTC
	}
}

// NewBlockchain creates a new blockchain with the given storage
func NewBlockchain(store Storage, config *Config) (*Blockchain, error) {
	if store == nil {
		store = NewMemoryStorage()
	}

	if config == nil {
		config = DefaultConfig()
	}

	bc := &Blockchain{
		blocks:  make([]*Block, 0),
		store:   store,
		running: false,
		eventCh: make(chan *BlockEvent, 100),
		config:  config,
	}

	// Load existing blocks from storage
	blocks, err := store.LoadAllBlocks()
	if err != nil {
		return nil, fmt.Errorf("failed to load blocks: %w", err)
	}

	if len(blocks) > 0 {
		bc.blocks = blocks
	}

	// Initialize producer and monitor
	bc.producer = NewBlockProducer(bc, config)
	bc.monitor = NewMonitor(bc, config)

	return bc, nil
}

// Start starts the blockchain services
func (bc *Blockchain) Start() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.running {
		return errors.New("blockchain: already running")
	}

	// Create genesis block if needed
	if len(bc.blocks) == 0 {
		if err := bc.createGenesisBlock(); err != nil {
			return fmt.Errorf("failed to create genesis block: %w", err)
		}
	}

	bc.running = true

	// Start producer and monitor
	if bc.producer != nil {
		bc.producer.Start()
	}

	if bc.monitor != nil {
		bc.monitor.Start()
	}

	return nil
}

// Stop stops the blockchain services
func (bc *Blockchain) Stop() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if !bc.running {
		return errors.New("blockchain: not running")
	}

	bc.running = false

	// Stop producer and monitor
	if bc.producer != nil {
		bc.producer.Stop()
	}

	if bc.monitor != nil {
		bc.monitor.Stop()
	}

	return nil
}

// IsRunning returns whether the blockchain is running
func (bc *Blockchain) IsRunning() bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.running
}

// AddBlock adds a new block to the blockchain
func (bc *Blockchain) AddBlock(block *Block) error {
	if block == nil {
		return errors.New("blockchain: nil block")
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Verify the block
	if err := bc.verifyBlock(block); err != nil {
		return fmt.Errorf("block verification failed: %w", err)
	}

	// Add to chain
	bc.blocks = append(bc.blocks, block)

	// Save to storage
	if err := bc.store.SaveBlock(block); err != nil {
		return fmt.Errorf("failed to save block: %w", err)
	}

	// Trigger callback
	if bc.onBlockProduced != nil {
		go bc.onBlockProduced(block)
	}

	return nil
}

// AddBlockEvent processes a block event and creates a new block
func (bc *Blockchain) AddBlockEvent(event *BlockEvent) error {
	if event == nil {
		return errors.New("blockchain: nil event")
	}

	bc.mu.Lock()
	height := bc.nextHeight()
	prevHash := bc.latestHash()
	bc.mu.Unlock()

	// Create new block
	block := NewBlock(height, prevHash, event)

	// Calculate and set hash
	block.BlockHash = block.CalculateHash()

	// Add to chain
	return bc.AddBlock(block)
}

// GetBlock returns a block by height
func (bc *Blockchain) GetBlock(height uint64) (*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if height >= uint64(len(bc.blocks)) {
		return nil, errors.New("blockchain: block not found")
	}

	return bc.blocks[height], nil
}

// GetBlockByHash returns a block by hash
func (bc *Blockchain) GetBlockByHash(hash []byte) (*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	for _, block := range bc.blocks {
		if bytes.Equal(block.Hash(), hash) {
			return block, nil
		}
	}

	// Try storage
	return bc.store.GetBlockByHash(hash)
}

// GetLatestBlock returns the latest block
func (bc *Blockchain) GetLatestBlock() (*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if len(bc.blocks) == 0 {
		return nil, errors.New("blockchain: no blocks")
	}

	return bc.blocks[len(bc.blocks)-1], nil
}

// GetBlockCount returns the number of blocks
func (bc *Blockchain) GetBlockCount() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return uint64(len(bc.blocks))
}

// GetAllBlocks returns all blocks
func (bc *Blockchain) GetAllBlocks() []*Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	blocks := make([]*Block, len(bc.blocks))
	copy(blocks, bc.blocks)
	return blocks
}

// GetBlocksInRange returns blocks in a height range
func (bc *Blockchain) GetBlocksInRange(start, end uint64) ([]*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if start > end {
		return nil, errors.New("blockchain: invalid range")
	}

	if start >= uint64(len(bc.blocks)) {
		return nil, errors.New("blockchain: start height out of range")
	}

	if end >= uint64(len(bc.blocks)) {
		end = uint64(len(bc.blocks)) - 1
	}

	blocks := make([]*Block, 0, end-start+1)
	for i := start; i <= end; i++ {
		blocks = append(blocks, bc.blocks[i])
	}

	return blocks, nil
}

// VerifyChain verifies the entire blockchain
func (bc *Blockchain) VerifyChain() error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if len(bc.blocks) == 0 {
		return errors.New("blockchain: empty chain")
	}

	// Verify genesis block
	if !bytes.Equal(bc.blocks[0].PreviousBlockHash(), make([]byte, 32)) {
		return errors.New("blockchain: invalid genesis block")
	}

	// Verify each subsequent block
	for i := 1; i < len(bc.blocks); i++ {
		current := bc.blocks[i]
		previous := bc.blocks[i-1]

		// Verify height
		if current.Height != uint64(i) {
			return fmt.Errorf("blockchain: invalid height at block %d", i)
		}

		// Verify previous hash
		if !bytes.Equal(current.PreviousBlockHash(), previous.Hash()) {
			return fmt.Errorf("blockchain: invalid previous hash at block %d", i)
		}

		// Verify hash
		expectedHash := current.CalculateHash()
		if !bytes.Equal(current.Hash(), expectedHash) {
			return fmt.Errorf("blockchain: invalid hash at block %d", i)
		}
	}

	return nil
}

// GetEventChannel returns the event channel for receiving block events
func (bc *Blockchain) GetEventChannel() chan *BlockEvent {
	return bc.eventCh
}

// SendEvent sends a block event to be processed
func (bc *Blockchain) SendEvent(event *BlockEvent) error {
	if event == nil {
		return errors.New("blockchain: nil event")
	}

	select {
	case bc.eventCh <- event:
		return nil
	default:
		return errors.New("blockchain: event channel full")
	}
}

// SetBlockProducedCallback sets the callback for block production
func (bc *Blockchain) SetBlockProducedCallback(fn func(*Block)) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.onBlockProduced = fn
}

// SetBlockVerifiedCallback sets the callback for block verification
func (bc *Blockchain) SetBlockVerifiedCallback(fn func(*Block)) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.onBlockVerified = fn
}

// SetVerificationFailedCallback sets the callback for verification failures
func (bc *Blockchain) SetVerificationFailedCallback(fn func(*BlockEvent, error)) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.onVerificationFailed = fn
}

// GetProducer returns the block producer
func (bc *Blockchain) GetProducer() *BlockProducer {
	return bc.producer
}

// GetMonitor returns the monitor
func (bc *Blockchain) GetMonitor() *Monitor {
	return bc.monitor
}

// GetConfig returns the blockchain configuration
func (bc *Blockchain) GetConfig() *Config {
	return bc.config
}

// nextHeight returns the height for the next block
func (bc *Blockchain) nextHeight() uint64 {
	if len(bc.blocks) == 0 {
		return 0
	}
	return bc.blocks[len(bc.blocks)-1].Height + 1
}

// latestHash returns the hash of the latest block
func (bc *Blockchain) latestHash() []byte {
	if len(bc.blocks) == 0 {
		return make([]byte, 32)
	}
	return bc.blocks[len(bc.blocks)-1].Hash()
}

// verifyBlock verifies a block before adding to the chain
func (bc *Blockchain) verifyBlock(block *Block) error {
	// Check if this is the genesis block
	if block.Height == 0 {
		return nil
	}

	// Get latest block
	latest, err := bc.store.GetLatestBlock()
	if err != nil && len(bc.blocks) > 0 {
		latest = bc.blocks[len(bc.blocks)-1]
	}

	// Verify previous hash
	if latest != nil {
		if !bytes.Equal(block.PreviousBlockHash(), latest.Hash()) {
			return errors.New("blockchain: previous hash mismatch")
		}
	}

	// Verify hash
	expectedHash := block.CalculateHash()
	if !bytes.Equal(block.Hash(), expectedHash) {
		return errors.New("blockchain: hash mismatch")
	}

	// Verify minimum agreement rate for valid blocks
	if block.IsValid && block.AgreementRate < bc.config.MinAgreementRate {
		return fmt.Errorf("blockchain: agreement rate %.2f%% below minimum %.2f%%",
			block.AgreementRate*100, bc.config.MinAgreementRate*100)
	}

	return nil
}

// createGenesisBlock creates the genesis block
func (bc *Blockchain) createGenesisBlock() error {
	genesisEvent := &BlockEvent{
		TaskID:        bc.config.GenesisTaskID,
		FinalResult:   bc.config.GenesisResult,
		IsValid:       true,
		AgreementRate: 1.0,
		NodeResults:   map[string]string{"genesis": "OK"},
		ConsensusNodes: []string{"genesis"},
		Metadata:      map[string]string{"type": "genesis"},
		Timestamp:     bc.config.GenesisTimestamp,
		BlockHeight:   0,
	}

	genesisBlock := NewBlock(0, make([]byte, 32), genesisEvent)
	genesisBlock.BlockHash = genesisBlock.CalculateHash()

	bc.blocks = append(bc.blocks, genesisBlock)

	if err := bc.store.SaveBlock(genesisBlock); err != nil {
		return fmt.Errorf("failed to save genesis block: %w", err)
	}

	return nil
}

// GetBlockchainInfo returns blockchain information for display
func (bc *Blockchain) GetBlockchainInfo() map[string]interface{} {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	latestBlock, _ := bc.GetLatestBlock()

	info := map[string]interface{}{
		"running":           bc.running,
		"block_count":       len(bc.blocks),
		"config":            bc.config,
	}

	if latestBlock != nil {
		info["latest_block"] = latestBlock.ToDisplayMap()
		info["latest_hash"] = hex.EncodeToString(latestBlock.Hash())
		info["latest_height"] = latestBlock.Height
	}

	// Count valid vs invalid blocks
	validBlocks := 0
	invalidBlocks := 0
	for _, b := range bc.blocks {
		if b.IsValid {
			validBlocks++
		} else {
			invalidBlocks++
		}
	}

	info["valid_blocks"] = validBlocks
	info["invalid_blocks"] = invalidBlocks

	return info
}
