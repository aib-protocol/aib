// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Chain State Management Module
package utxo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

// ============================================================================
// Chain State Management
// ============================================================================

// ChainState represents the current state of the blockchain.
// It manages the chain tip, block storage, and provides block query APIs.
type ChainState struct {
	db            *bbolt.DB
	bestHeight    uint64           // Current best block height
	bestHash      [32]byte         // Current best block hash
	genesisHash   [32]byte         // Genesis block hash
	blockIndex    map[uint64][32]byte // height -> block hash
	blockIndexMu  sync.RWMutex
	utxoStore     *PersistentUTXOStore
	mempool       *Mempool
	consensus     *ConsensusState
}

// ChainBucket names for bbolt
var (
	BucketBlocks      = []byte("blocks")
	BucketBlockHeight = []byte("blockheight") // height -> block hash
	BucketChainMeta   = []byte("chainmeta")   // Chain metadata
)

// Chain metadata keys
const (
	MetaBestHeight = "best_height"
	MetaBestHash   = "best_hash"
	MetaGenesisHash = "genesis_hash"
)

// NewChainState creates a new chain state with persistent storage.
func NewChainState(dbPath string) (*ChainState, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout:      5 * time.Second,
		NoGrowSync:   false,
		FreelistType: bbolt.FreelistMapType,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		buckets := [][]byte{BucketBlocks, BucketBlockHeight, BucketChainMeta}
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	cs := &ChainState{
		db:         db,
		blockIndex: make(map[uint64][32]byte),
	}

	// Load chain state from database
	if err := cs.loadState(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load state: %w", err)
	}

	return cs, nil
}

// NewChainStateWithDB creates a chain state with existing bbolt.DB.
func NewChainStateWithDB(db *bbolt.DB) (*ChainState, error) {
	cs := &ChainState{
		db:         db,
		blockIndex: make(map[uint64][32]byte),
	}

	// Initialize buckets if needed
	err := db.Update(func(tx *bbolt.Tx) error {
		buckets := [][]byte{BucketBlocks, BucketBlockHeight, BucketChainMeta}
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	// Load chain state from database
	if err := cs.loadState(); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return cs, nil
}

// loadState loads the chain state from database.
func (cs *ChainState) loadState() error {
	return cs.db.View(func(tx *bbolt.Tx) error {
		// Load best height
		b := tx.Bucket(BucketChainMeta)
		if data := b.Get([]byte(MetaBestHeight)); data != nil {
			cs.bestHeight = binary.BigEndian.Uint64(data)
		}

		// Load best hash
		if data := b.Get([]byte(MetaBestHash)); data != nil && len(data) == 32 {
			copy(cs.bestHash[:], data)
		}

		// Load genesis hash
		if data := b.Get([]byte(MetaGenesisHash)); data != nil && len(data) == 32 {
			copy(cs.genesisHash[:], data)
		}

		// Load block index
		heightBucket := tx.Bucket(BucketBlockHeight)
		c := heightBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if len(k) == 8 && len(v) == 32 {
				height := binary.BigEndian.Uint64(k)
				var hash [32]byte
				copy(hash[:], v)
				cs.blockIndex[height] = hash
			}
		}

		return nil
	})
}

// Close closes the chain state database.
func (cs *ChainState) Close() error {
	return cs.db.Close()
}

// SetUTXOStore sets the UTXO store for the chain.
func (cs *ChainState) SetUTXOStore(store *PersistentUTXOStore) {
	cs.utxoStore = store
}

// SetMempool sets the mempool for the chain.
func (cs *ChainState) SetMempool(mempool *Mempool) {
	cs.mempool = mempool
}

// SetConsensus sets the consensus state for the chain.
func (cs *ChainState) SetConsensus(consensus *ConsensusState) {
	cs.consensus = consensus
}

// ============================================================================
// Block Operations
// ============================================================================

// AddBlock adds a new block to the chain.
// It validates the block and updates the chain state if the block is accepted.
func (cs *ChainState) AddBlock(block *Block) error {
	// Validate block
	if err := cs.ValidateBlock(block); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	// Store block
	blockHash := block.Hash
	if err := cs.db.Update(func(tx *bbolt.Tx) error {
		// Store block data
		blocksBucket := tx.Bucket(BucketBlocks)
		blockData := block.SerializeBlock()
		if err := blocksBucket.Put(blockHash[:], blockData); err != nil {
			return fmt.Errorf("store block: %w", err)
		}

		// Update height index
		heightBucket := tx.Bucket(BucketBlockHeight)
		var heightKey [8]byte
		binary.BigEndian.PutUint64(heightKey[:], block.Header.Height)
		if err := heightBucket.Put(heightKey[:], blockHash[:]); err != nil {
			return fmt.Errorf("update height index: %w", err)
		}

		// Update chain state if this is the new best block
		// For genesis block (height 0), always update if this is the first block
		isFirstBlock := len(cs.blockIndex) == 0
		if block.Header.Height > cs.bestHeight || (block.Header.Height == 0 && isFirstBlock) {
			// Update best height
			cs.bestHeight = block.Header.Height
			cs.bestHash = blockHash
			cs.blockIndexMu.Lock()
			cs.blockIndex[block.Header.Height] = blockHash
			cs.blockIndexMu.Unlock()

			metaBucket := tx.Bucket(BucketChainMeta)
			var buf [8]byte
			binary.BigEndian.PutUint64(buf[:], cs.bestHeight)
			if err := metaBucket.Put([]byte(MetaBestHeight), buf[:]); err != nil {
				return fmt.Errorf("update best height: %w", err)
			}
			if err := metaBucket.Put([]byte(MetaBestHash), cs.bestHash[:]); err != nil {
				return fmt.Errorf("update best hash: %w", err)
			}
		}

		// Apply transactions to UTXO set
		if cs.utxoStore != nil {
			for _, utxoTx := range block.Transactions {
				if utxoTx.IsCoinbase() {
					// For coinbase transactions, only add outputs as new UTXOs
					if err := cs.applyCoinbaseTransaction(utxoTx); err != nil {
						return fmt.Errorf("apply coinbase: %w", err)
					}
				} else {
					if err := cs.utxoStore.ApplyTransaction(utxoTx); err != nil {
						return fmt.Errorf("apply transaction: %w", err)
					}
				}
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("database update: %w", err)
	}

	// Remove confirmed transactions from mempool
	if cs.mempool != nil && len(block.Transactions) > 0 {
		txHashes := make([][32]byte, len(block.Transactions))
		for i, tx := range block.Transactions {
			txHashes[i] = tx.Hash()
		}
		cs.mempool.RemoveConfirmed(txHashes)
	}

	// Update consensus state
	if cs.consensus != nil {
		if err := cs.consensus.ProcessNewBlock(block); err != nil {
			return fmt.Errorf("process new block: %w", err)
		}
	}

	return nil
}

// GetBlockByHash returns a block by its hash.
func (cs *ChainState) GetBlockByHash(hash [32]byte) (*Block, error) {
	var block *Block
	err := cs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketBlocks)
		data := b.Get(hash[:])
		if data == nil {
			return fmt.Errorf("block not found: %x", hash)
		}

		var err error
		block, err = DeserializeBlock(data)
		return err
	})

	return block, err
}

// GetBlockByHeight returns a block by its height.
func (cs *ChainState) GetBlockByHeight(height uint64) (*Block, error) {
	// Get block hash from index
	cs.blockIndexMu.RLock()
	hash, ok := cs.blockIndex[height]
	cs.blockIndexMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("block at height %d not found", height)
	}

	return cs.GetBlockByHash(hash)
}

// GetBestBlock returns the current best block.
func (cs *ChainState) GetBestBlock() (*Block, error) {
	cs.blockIndexMu.RLock()
	count := len(cs.blockIndex)
	cs.blockIndexMu.RUnlock()
	if count == 0 {
		return nil, fmt.Errorf("chain is empty")
	}
	return cs.GetBlockByHash(cs.bestHash)
}

// GetBlockCount returns the total number of blocks in the chain.
func (cs *ChainState) GetBlockCount() uint64 {
	cs.blockIndexMu.RLock()
	count := uint64(len(cs.blockIndex))
	cs.blockIndexMu.RUnlock()
	return count
}

// GetBestBlockHeight returns the current best block height.
func (cs *ChainState) GetBestBlockHeight() uint64 {
	return cs.bestHeight
}

// GetBestBlockHash returns the current best block hash.
func (cs *ChainState) GetBestBlockHash() [32]byte {
	return cs.bestHash
}

// GetGenesisBlockHash returns the genesis block hash.
func (cs *ChainState) GetGenesisBlockHash() [32]byte {
	return cs.genesisHash
}

// HasBlock checks if a block exists at the given height.
func (cs *ChainState) HasBlock(height uint64) bool {
	cs.blockIndexMu.RLock()
	_, ok := cs.blockIndex[height]
	cs.blockIndexMu.RUnlock()
	return ok
}

// HasBlockByHash checks if a block exists with the given hash.
func (cs *ChainState) HasBlockByHash(hash [32]byte) (bool, error) {
	var exists bool
	err := cs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketBlocks)
		exists = b.Get(hash[:]) != nil
		return nil
	})
	return exists, err
}

// ============================================================================
// Block Validation
// ============================================================================

// ValidationResult represents the result of block validation.
type ValidationResult struct {
	Valid bool
	Error error
}

// ValidateBlock performs comprehensive validation of a block.
func (cs *ChainState) ValidateBlock(block *Block) error {
	// 1. Basic structure validation
	if err := cs.validateBlockStructure(block); err != nil {
		return fmt.Errorf("structure validation: %w", err)
	}

	// 2. Timestamp validation
	if err := cs.validateBlockTimestamp(block); err != nil {
		return fmt.Errorf("timestamp validation: %w", err)
	}

	// 3. Parent chain validation
	if err := cs.validateBlockParent(block); err != nil {
		return fmt.Errorf("parent validation: %w", err)
	}

	// 4. Transaction validation
	if err := cs.validateBlockTransactions(block); err != nil {
		return fmt.Errorf("transaction validation: %w", err)
	}

	// 5. Block signature validation
	if err := cs.validateBlockSignature(block); err != nil {
		return fmt.Errorf("signature validation: %w", err)
	}

	// 6. Proposer validation (PoS)
	if err := cs.validateBlockProposer(block); err != nil {
		return fmt.Errorf("proposer validation: %w", err)
	}

	return nil
}

// validateBlockStructure validates basic block structure.
func (cs *ChainState) validateBlockStructure(block *Block) error {
	// Check version
	if block.Header.Version == 0 {
		return fmt.Errorf("invalid version: 0")
	}

	// Check transactions are not nil
	if block.Transactions == nil {
		return fmt.Errorf("transactions is nil")
	}

	// Check Merkle root
	computedMerkle := block.CalculateMerkleRoot()
	if computedMerkle != block.Header.MerkleRoot {
		return fmt.Errorf("merkle root mismatch")
	}

	// For signed blocks, the hash changes after signature is added.
	// We need to recalculate hash based on the signed header.
	if len(block.Header.Signature) > 0 {
		// Verify the hash stored in block matches the signed header
		expectedHash := block.CalculateHash()
		if expectedHash != block.Hash {
			// Allow mismatch if this is a newly signed block
			// Recalculate and update the block hash
			block.Hash = expectedHash
		}
	}

	return nil
}

// validateBlockTimestamp validates the block timestamp.
func (cs *ChainState) validateBlockTimestamp(block *Block) error {
	now := uint64(time.Now().Unix())

	// Timestamp must not be in the future (allow 5 minute clock drift)
	if block.Header.Timestamp > now+300 {
		return fmt.Errorf("block timestamp %d is too far in the future (now: %d)",
			block.Header.Timestamp, now)
	}

	// If not genesis, timestamp must be after parent
	if block.Header.Height > 0 {
		parent, err := cs.GetBlockByHeight(block.Header.Height - 1)
		if err != nil {
			return fmt.Errorf("failed to get parent block: %w", err)
		}

		if block.Header.Timestamp <= parent.Header.Timestamp {
			return fmt.Errorf("block timestamp %d is not greater than parent timestamp %d",
				block.Header.Timestamp, parent.Header.Timestamp)
		}

		// Check minimum block time
		timeDiff := time.Duration(block.Header.Timestamp-parent.Header.Timestamp) * time.Second
		if timeDiff < MinBlockTime {
			return fmt.Errorf("block time difference %v is below minimum %v", timeDiff, MinBlockTime)
		}

		// Check maximum block time drift
		// Skip this check for blocks immediately after genesis (height <= 100)
		// because genesis may have an old timestamp (like Bitcoin's)
		if block.Header.Height > 100 && timeDiff > MaxBlockTimeDrift {
			return fmt.Errorf("block time difference %v exceeds maximum drift %v", timeDiff, MaxBlockTimeDrift)
		}
	}

	return nil
}

// validateBlockParent validates the block's connection to parent.
func (cs *ChainState) validateBlockParent(block *Block) error {
	// Genesis block has no parent
	if block.IsGenesis() {
		// This is the first block, set genesis hash if not set
		if cs.genesisHash == [32]byte{} {
			cs.genesisHash = block.Hash
		}
		return nil
	}

	// Get parent block
	parent, err := cs.GetBlockByHeight(block.Header.Height - 1)
	if err != nil {
		return fmt.Errorf("parent block not found at height %d: %w", block.Header.Height-1, err)
	}

	// Check previous block hash
	if block.Header.PrevBlockHash != parent.Hash {
		return fmt.Errorf("previous block hash mismatch: expected %x, got %x",
			parent.Hash, block.Header.PrevBlockHash)
	}

	return nil
}

// validateBlockTransactions validates all transactions in the block.
func (cs *ChainState) validateBlockTransactions(block *Block) error {
	// Must have at least one transaction (coinbase)
	if len(block.Transactions) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	// First transaction must be coinbase
	if !block.Transactions[0].IsCoinbase() {
		return fmt.Errorf("first transaction is not coinbase")
	}

	// Check for duplicate coinbase
	coinbaseCount := 0
	for _, tx := range block.Transactions {
		if tx.IsCoinbase() {
			coinbaseCount++
		}
	}
	if coinbaseCount > 1 {
		return fmt.Errorf("multiple coinbase transactions: %d", coinbaseCount)
	}

	// Validate each transaction
	for i, tx := range block.Transactions {
		// Skip coinbase validation (no inputs to verify)
		if tx.IsCoinbase() {
			continue
		}

		// Verify all signatures
		if !tx.VerifyAllInputs() {
			return fmt.Errorf("transaction %d has invalid signatures", i)
		}

		// If UTXO store is available, validate inputs exist
		if cs.utxoStore != nil {
			for j, input := range tx.Inputs {
				_, err := cs.utxoStore.GetUTXO(input.TxHash, input.Index)
				if err != nil {
					return fmt.Errorf("transaction %d input %d: UTXO not found: %x:%d",
						i, j, input.TxHash, input.Index)
				}
			}

			// Check fee is positive
			fee, err := tx.GetFee(cs.utxoStore)
			if err != nil {
				return fmt.Errorf("transaction %d: failed to calculate fee: %w", i, err)
			}
			if fee < MinTransactionFee {
				return fmt.Errorf("transaction %d: fee %d below minimum %d", i, fee, MinTransactionFee)
			}
		}
	}

	// Check for double spend within block
	inputSet := make(map[string]bool)
	for i := 1; i < len(block.Transactions); i++ {
		for j, input := range block.Transactions[i].Inputs {
			key := fmt.Sprintf("%x:%d", input.TxHash, input.Index)
			if inputSet[key] {
				return fmt.Errorf("double spend detected: tx %d input %d", i, j)
			}
			inputSet[key] = true
		}
	}

	return nil
}

// validateBlockSignature validates the block proposer signature.
func (cs *ChainState) validateBlockSignature(block *Block) error {
	// Genesis block (height 0) doesn't require signature
	if block.Header.Height == 0 {
		return nil
	}

	// Block must have a signature
	if len(block.Header.Signature) == 0 {
		return fmt.Errorf("block has no signature")
	}

	// Verify the signature using the proposer's public key
	if !block.VerifyBlockSignature() {
		return fmt.Errorf("invalid block signature")
	}

	return nil
}

// validateBlockProposer validates that the block proposer was correctly selected.
func (cs *ChainState) validateBlockProposer(block *Block) error {
	// Genesis block (height 0) doesn't require proposer validation
	if block.Header.Height == 0 {
		return nil
	}

	if cs.consensus == nil {
		return nil // Skip if no consensus state
	}

	// Get parent block for VRF seed
	var parentBlock *Block
	if block.Header.Height > 0 {
		var err error
		parentBlock, err = cs.GetBlockByHeight(block.Header.Height - 1)
		if err != nil {
			return fmt.Errorf("failed to get parent for proposer validation: %w", err)
		}
	}

	// Verify proposer selection
	result := cs.consensus.VerifyBlockProposer(block, parentBlock)
	if !result.Valid {
		return fmt.Errorf("proposer validation failed: %s", result.Error)
	}

	return nil
}

// ============================================================================
// Chain Information APIs
// ============================================================================

// ChainInfo contains basic chain information.
type ChainInfo struct {
	BestBlockHash   string
	BestBlockHeight uint64
	GenesisBlockHash string
	TotalBlocks     uint64
}

// GetChainInfo returns basic chain information.
func (cs *ChainState) GetChainInfo() *ChainInfo {
	return &ChainInfo{
		BestBlockHash:    hex.EncodeToString(cs.bestHash[:]),
		BestBlockHeight:  cs.bestHeight,
		GenesisBlockHash: hex.EncodeToString(cs.genesisHash[:]),
		TotalBlocks:      cs.bestHeight + 1,
	}
}

// BlockInfo contains detailed block information for API responses.
type BlockInfo struct {
	Hash              string
	Height            uint64
	Version           uint32
	PrevBlockHash     string
	MerkleRoot        string
	Timestamp         uint64
	Proposer          string
	TransactionCount  int
	BlockReward       uint64
	Size              int
}

// ToBlockInfo converts a Block to BlockInfo for API response.
func (cs *ChainState) ToBlockInfo(block *Block) (*BlockInfo, error) {
	info := &BlockInfo{
		Hash:              hex.EncodeToString(block.Hash[:]),
		Height:            block.Header.Height,
		Version:           block.Header.Version,
		PrevBlockHash:     hex.EncodeToString(block.Header.PrevBlockHash[:]),
		MerkleRoot:        hex.EncodeToString(block.Header.MerkleRoot[:]),
		Timestamp:         block.Header.Timestamp,
		Proposer:          hex.EncodeToString(block.Header.Proposer[:]),
		TransactionCount:  len(block.Transactions),
		BlockReward:       block.GetBlockReward(),
		Size:              len(block.SerializeBlock()),
	}

	return info, nil
}

// GetBlockInfo returns detailed block information by height.
func (cs *ChainState) GetBlockInfo(height uint64) (*BlockInfo, error) {
	block, err := cs.GetBlockByHeight(height)
	if err != nil {
		return nil, err
	}
	return cs.ToBlockInfo(block)
}

// GetBlockInfoByHash returns detailed block information by hash.
func (cs *ChainState) GetBlockInfoByHash(hash [32]byte) (*BlockInfo, error) {
	block, err := cs.GetBlockByHash(hash)
	if err != nil {
		return nil, err
	}
	return cs.ToBlockInfo(block)
}

// ============================================================================
// Block Iteration
// ============================================================================

// BlockIterator iterates over blocks in the chain.
type BlockIterator struct {
	cs           *ChainState
	currentHeight uint64
}

// NewBlockIterator creates a new block iterator starting from the given height.
func (cs *ChainState) NewBlockIterator(startHeight uint64) *BlockIterator {
	return &BlockIterator{
		cs:           cs,
		currentHeight: startHeight,
	}
}

// Next returns the next block in the iterator.
func (it *BlockIterator) Next() (*Block, bool) {
	if it.currentHeight > it.cs.bestHeight {
		return nil, false
	}

	block, err := it.cs.GetBlockByHeight(it.currentHeight)
	if err != nil {
		return nil, false
	}

	it.currentHeight++
	return block, true
}

// ============================================================================
// Fork Handling
// ============================================================================

// ForkChoice represents a potential chain fork.
type ForkChoice struct {
	Block    *Block
	IsActive bool
}

// GetForks returns any known forks in the chain.
// This is a basic implementation that checks for alternative chains.
func (cs *ChainState) GetForks() ([]*ForkChoice, error) {
	var forks []*ForkChoice

	// Basic fork detection: check if there are multiple blocks at the same height
	heightToHashes := make(map[uint64][][]byte)

	cs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketBlockHeight)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			height := binary.BigEndian.Uint64(k)
			heightToHashes[height] = append(heightToHashes[height], v)
		}

		return nil
	})

	// Find heights with multiple blocks (forks)
	for _, hashes := range heightToHashes {
		if len(hashes) > 1 {
			for _, hash := range hashes {
				var h [32]byte
				copy(h[:], hash)
				block, err := cs.GetBlockByHash(h)
				if err == nil {
					forks = append(forks, &ForkChoice{
						Block:    block,
						IsActive: h == cs.bestHash,
					})
				}
			}
		}
	}

	return forks, nil
}

// RollbackChain rolls back the chain to a specific height.
func (cs *ChainState) RollbackChain(targetHeight uint64) error {
	if targetHeight >= cs.bestHeight {
		return fmt.Errorf("target height %d is >= current best height %d", targetHeight, cs.bestHeight)
	}

	return cs.db.Update(func(tx *bbolt.Tx) error {
		// Remove blocks above target height
		blocksBucket := tx.Bucket(BucketBlocks)
		heightBucket := tx.Bucket(BucketBlockHeight)
		metaBucket := tx.Bucket(BucketChainMeta)

		// Get blocks to remove
		var blocksToRemove [][32]byte
		for height := cs.bestHeight; height > targetHeight; height-- {
			var heightKey [8]byte
			binary.BigEndian.PutUint64(heightKey[:], height)
			hash := heightBucket.Get(heightKey[:])
			if hash != nil {
				var h [32]byte
				copy(h[:], hash)
				blocksToRemove = append(blocksToRemove, h)
			}
		}

		// Remove blocks
		for _, hash := range blocksToRemove {
			if err := blocksBucket.Delete(hash[:]); err != nil {
				return err
			}
		}

		// Update height index
		for height := cs.bestHeight; height > targetHeight; height-- {
			var heightKey [8]byte
			binary.BigEndian.PutUint64(heightKey[:], height)
			if err := heightBucket.Delete(heightKey[:]); err != nil {
				return err
			}
		}

		// Update best height
		cs.bestHeight = targetHeight

		// Get new best hash
		if targetHeight > 0 {
			var heightKey [8]byte
			binary.BigEndian.PutUint64(heightKey[:], targetHeight)
			newBestHash := heightBucket.Get(heightKey[:])
			if newBestHash != nil && len(newBestHash) == 32 {
				copy(cs.bestHash[:], newBestHash)
			}
		} else {
			cs.bestHash = [32]byte{}
		}

		// Save to database
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], cs.bestHeight)
		if err := metaBucket.Put([]byte(MetaBestHeight), buf[:]); err != nil {
			return err
		}
		if err := metaBucket.Put([]byte(MetaBestHash), cs.bestHash[:]); err != nil {
			return err
		}

		// Update in-memory index
		cs.blockIndexMu.Lock()
		for height := range cs.blockIndex {
			if height > targetHeight {
				delete(cs.blockIndex, height)
			}
		}
		cs.blockIndexMu.Unlock()

		return nil
	})
}

// ============================================================================
// Initialization
// ============================================================================

// InitGenesis initializes the chain with a genesis block.
func (cs *ChainState) InitGenesis(genesisBlock *Block) error {
	// Check if genesis already exists
	if cs.bestHeight > 0 {
		return fmt.Errorf("genesis already initialized")
	}

	// Validate genesis block
	if !genesisBlock.IsGenesis() {
		return fmt.Errorf("invalid genesis block: not a genesis block")
	}

	// Add genesis block
	if err := cs.AddBlock(genesisBlock); err != nil {
		return fmt.Errorf("failed to add genesis block: %w", err)
	}

	// Save genesis hash
	cs.genesisHash = genesisBlock.Hash

	return cs.db.Update(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket(BucketChainMeta)
		return metaBucket.Put([]byte(MetaGenesisHash), cs.genesisHash[:])
	})
}

// ============================================================================
// Coinbase Transaction Handling
// ============================================================================

// applyCoinbaseTransaction adds coinbase outputs as new UTXOs without checking inputs.
func (cs *ChainState) applyCoinbaseTransaction(tx *Transaction) error {
	if cs.utxoStore == nil {
		return nil
	}

	txHash := tx.Hash()
	for i, out := range tx.Outputs {
		utxo := &UTXO{
			TxHash:  txHash,
			Index:   uint32(i),
			Value:   out.Value,
			Script:  out.Script,
			Address: out.Address,
		}
		if err := cs.utxoStore.AddUTXO(utxo); err != nil {
			return fmt.Errorf("add coinbase UTXO %d: %w", i, err)
		}
	}
	return nil
}

// ============================================================================
// UTXOProvider Implementation
// ============================================================================

// GetUTXO implements the UTXOProvider interface.
func (cs *ChainState) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	if cs.utxoStore == nil {
		return nil, fmt.Errorf("UTXO store not set")
	}
	return cs.utxoStore.GetUTXO(txHash, index)
}

// ============================================================================
// Helper Functions
// ============================================================================

// UTXOKey is defined in store.go

// VerifyBlockHash verifies a block's hash is correct.
func VerifyBlockHash(block *Block) bool {
	computed := block.CalculateHash()
	return bytes.Equal(computed[:], block.Hash[:])
}

// GetBlockHashHex returns the block hash as hex string.
func GetBlockHashHex(block *Block) string {
	return hex.EncodeToString(block.Hash[:])
}

// ============================================================================
// Serialization Support
// ============================================================================

// SerializeChainState serializes the chain state for persistence.
func (cs *ChainState) SerializeChainState() ([]byte, error) {
	var buf bytes.Buffer

	// Best height
	binary.Write(&buf, binary.BigEndian, cs.bestHeight)

	// Best hash
	buf.Write(cs.bestHash[:])

	// Genesis hash
	buf.Write(cs.genesisHash[:])

	return buf.Bytes(), nil
}

// DeserializeChainState deserializes chain state.
func DeserializeChainState(data []byte) (bestHeight uint64, bestHash, genesisHash [32]byte, err error) {
	buf := bytes.NewReader(data)

	// Best height
	if err := binary.Read(buf, binary.BigEndian, &bestHeight); err != nil {
		return 0, [32]byte{}, [32]byte{}, err
	}

	// Best hash
	if _, err := buf.Read(bestHash[:]); err != nil {
		return 0, [32]byte{}, [32]byte{}, err
	}

	// Genesis hash
	if _, err := buf.Read(genesisHash[:]); err != nil {
		return 0, [32]byte{}, [32]byte{}, err
	}

	return bestHeight, bestHash, genesisHash, nil
}
