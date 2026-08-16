package consensus

import (
	"errors"
	"sync"
)

// Storage defines the interface for blockchain storage
type Storage interface {
	// SaveBlock saves a block to storage
	SaveBlock(block *Block) error

	// LoadBlock loads a block by height
	LoadBlock(height uint64) (*Block, error)

	// LoadAllBlocks loads all blocks from storage
	LoadAllBlocks() ([]*Block, error)

	// GetLatestBlock gets the latest block
	GetLatestBlock() (*Block, error)

	// GetBlockByHash loads a block by hash
	GetBlockByHash(hash []byte) (*Block, error)

	// GetBlockCount returns the number of blocks
	GetBlockCount() uint64

	// Close closes the storage
	Close() error
}

// MemoryStorage implements in-memory storage for blocks
type MemoryStorage struct {
	mu       sync.RWMutex
	blocks   map[uint64]*Block
	hashMap  map[string]uint64 // hash -> height mapping
	count    uint64
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		blocks:  make(map[uint64]*Block),
		hashMap: make(map[string]uint64),
		count:   0,
	}
}

// SaveBlock saves a block to memory storage
func (s *MemoryStorage) SaveBlock(block *Block) error {
	if block == nil {
		return errors.New("storage: nil block")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute hash if not set
	hash := block.Hash()
	hashStr := string(hash)

	s.blocks[block.Height] = block
	s.hashMap[hashStr] = block.Height

	if block.Height >= s.count {
		s.count = block.Height + 1
	}

	return nil
}

// LoadBlock loads a block by height
func (s *MemoryStorage) LoadBlock(height uint64) (*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blocks[height]
	if !ok {
		return nil, errors.New("storage: block not found")
	}

	return block, nil
}

// LoadAllBlocks loads all blocks
func (s *MemoryStorage) LoadAllBlocks() ([]*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	blocks := make([]*Block, 0, len(s.blocks))
	for i := uint64(0); i < s.count; i++ {
		if block, ok := s.blocks[i]; ok {
			blocks = append(blocks, block)
		}
	}

	return blocks, nil
}

// GetLatestBlock gets the latest block
func (s *MemoryStorage) GetLatestBlock() (*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.count == 0 {
		return nil, errors.New("storage: no blocks")
	}

	return s.blocks[s.count-1], nil
}

// GetBlockByHash loads a block by hash
func (s *MemoryStorage) GetBlockByHash(hash []byte) (*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	height, ok := s.hashMap[string(hash)]
	if !ok {
		return nil, errors.New("storage: block not found by hash")
	}

	return s.blocks[height], nil
}

// GetBlockCount returns the number of blocks
func (s *MemoryStorage) GetBlockCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// Close closes the storage (no-op for memory storage)
func (s *MemoryStorage) Close() error {
	return nil
}

// Clear clears all blocks from storage
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blocks = make(map[uint64]*Block)
	s.hashMap = make(map[string]uint64)
	s.count = 0
}

// FileStorage implements file-based storage for blocks
// This is a simple implementation that stores blocks in memory
// with periodic snapshots. For production use, consider using
// a proper database like LevelDB or BadgerDB.
type FileStorage struct {
	*MemoryStorage
	filePath string
	mu       sync.Mutex
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		MemoryStorage: NewMemoryStorage(),
		filePath:      filePath,
	}
}

// Snapshot saves all blocks to a snapshot
func (s *FileStorage) Snapshot() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blocks, err := s.MemoryStorage.LoadAllBlocks()
	if err != nil {
		return err
	}

	// For now, we just keep in memory
	// In production, serialize to file
	_ = blocks

	return nil
}

// Restore loads blocks from a snapshot
func (s *FileStorage) Restore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// For now, this is a no-op
	// In production, deserialize from file
	return nil
}
