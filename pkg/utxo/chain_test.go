// Package utxo implements UTXO-based transaction system for AIB blockchain.
package utxo

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"
)

// TestChainState_NewChainState tests creating a new chain state.
func TestChainState_NewChainState(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Check initial state - after creating NewChainState, it's empty (no genesis)
	if cs.GetBestBlockHeight() != 0 {
		t.Errorf("Expected initial height 0, got %d", cs.GetBestBlockHeight())
	}

	if cs.GetBlockCount() != 0 {
		t.Errorf("Expected initial block count 0, got %d", cs.GetBlockCount())
	}
}

// TestChainState_InitGenesis tests initializing the chain with a genesis block.
func TestChainState_InitGenesis(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Create UTXO store for validation
	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	// Generate key pair for genesis
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)

	// Create genesis coinbase transaction - sign first
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	// Create genesis block
	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200 // Fixed timestamp
	genesisBlock.SignBlock(privKey)

	// Initialize with genesis
	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to initialize genesis: %v", err)
	}

	// Verify state
	if cs.GetBestBlockHeight() != 0 {
		t.Errorf("Expected height 0, got %d", cs.GetBestBlockHeight())
	}

	if cs.GetBlockCount() != 1 {
		t.Errorf("Expected block count 1, got %d", cs.GetBlockCount())
	}

	genesisHash := cs.GetGenesisBlockHash()
	if genesisHash != genesisBlock.Hash {
		t.Errorf("Genesis hash mismatch: %x vs %x", genesisHash, genesisBlock.Hash)
	}

	bestHash := cs.GetBestBlockHash()
	if bestHash != genesisBlock.Hash {
		t.Errorf("Best hash mismatch: %x vs %x", bestHash, genesisBlock.Hash)
	}

	// Verify we can retrieve the genesis block
	retrieved, err := cs.GetBlockByHeight(0)
	if err != nil {
		t.Fatalf("Failed to retrieve genesis block: %v", err)
	}

	if retrieved.Hash != genesisBlock.Hash {
		t.Errorf("Retrieved block hash mismatch")
	}
}

// TestChainState_AddBlock tests adding blocks to the chain.
func TestChainState_AddBlock(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Create UTXO store and mempool
	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)

	// Add validator
	err = consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Use current timestamp for genesis
	now := uint64(time.Now().Unix())

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60 // 60 seconds ago
	genesisBlock.Header.VRFSeed = [32]byte{1, 2, 3}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Create a second block - use timestamp close to genesis
	coinbaseTx2 := CreateCoinbaseV2(proposerAddr, 1)
	coinbaseTx2.SignInput(0, privKey)

	block2 := NewBlock([]*Transaction{coinbaseTx2}, genesisBlock.Hash, 1, proposerAddr)
	block2.Header.Timestamp = now // current time
	block2.Header.VRFSeed = [32]byte{4, 5, 6}
	block2.SignBlock(privKey)

	// Add the block
	err = cs.AddBlock(block2)
	if err != nil {
		t.Fatalf("Failed to add block: %v", err)
	}

	// Verify state
	if cs.GetBestBlockHeight() != 1 {
		t.Errorf("Expected height 1, got %d", cs.GetBestBlockHeight())
	}

	if cs.GetBlockCount() != 2 {
		t.Errorf("Expected block count 2, got %d", cs.GetBlockCount())
	}

	// Retrieve the block
	retrieved, err := cs.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("Failed to retrieve block 1: %v", err)
	}

	if retrieved.Hash != block2.Hash {
		t.Errorf("Retrieved block hash mismatch")
	}
}

// TestChainState_GetBlockByHash tests retrieving blocks by hash.
func TestChainState_GetBlockByHash(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Test retrieval by hash
	retrieved, err := cs.GetBlockByHash(genesisBlock.Hash)
	if err != nil {
		t.Fatalf("Failed to get block by hash: %v", err)
	}

	if retrieved.Header.Height != 0 {
		t.Errorf("Expected height 0, got %d", retrieved.Header.Height)
	}

	// Test non-existent block
	nonExistentHash := [32]byte{1, 2, 3}
	_, err = cs.GetBlockByHash(nonExistentHash)
	if err == nil {
		t.Error("Expected error for non-existent block")
	}
}

// TestChainState_HasBlock tests the HasBlock methods.
func TestChainState_HasBlock(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Test HasBlock
	if !cs.HasBlock(0) {
		t.Error("Expected HasBlock(0) to return true")
	}

	if cs.HasBlock(1) {
		t.Error("Expected HasBlock(1) to return false")
	}

	// Test HasBlockByHash
	has, err := cs.HasBlockByHash(genesisBlock.Hash)
	if err != nil {
		t.Fatalf("HasBlockByHash failed: %v", err)
	}
	if !has {
		t.Error("Expected HasBlockByHash to return true for genesis block")
	}

	nonExistentHash := [32]byte{1, 2, 3}
	has, err = cs.HasBlockByHash(nonExistentHash)
	if err != nil {
		t.Fatalf("HasBlockByHash failed: %v", err)
	}
	if has {
		t.Error("Expected HasBlockByHash to return false for non-existent hash")
	}
}

// TestChainState_ValidateBlockTimestamp tests timestamp validation.
func TestChainState_ValidateBlockTimestamp(t *testing.T) {
	now := uint64(time.Now().Unix())

	tests := []struct {
		name          string
		timestamp     uint64
		parentTime    uint64
		expectValid   bool
	}{
		{
			name:        "valid timestamp",
			timestamp:   now,
			parentTime:  now - 60,
			expectValid: true,
		},
		{
			name:        "timestamp too far in future",
			timestamp:   now + 400,
			parentTime:  now - 60,
			expectValid: false,
		},
		{
			name:        "timestamp before parent",
			timestamp:   now - 120,
			parentTime:  now - 60,
			expectValid: false,
		},
		{
			name:        "block time above minimum",
			timestamp:   now,
			parentTime:  now - 15, // 15 seconds is above MinBlockTime (10s)
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := IsValidTimestamp(tt.timestamp, tt.parentTime)
			if valid != tt.expectValid {
				t.Errorf("IsValidTimestamp() = %v, want %v", valid, tt.expectValid)
			}
		})
	}
}

// TestChainState_BlockIterator tests the block iterator.
func TestChainState_BlockIterator(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Use relative timestamps
	now := uint64(time.Now().Unix())

	// Create genesis and a few blocks
	var prevHash [32]byte
	for i := 0; i < 3; i++ {
		coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx.SignInput(0, privKey)

		var block *Block
		if i == 0 {
			block = NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
			block.Header.Timestamp = now - 120 // 2 minutes ago
			block.Header.VRFSeed = [32]byte{}
		} else {
			block = NewBlock([]*Transaction{coinbaseTx}, prevHash, uint64(i), proposerAddr)
			block.Header.Timestamp = now - 120 + uint64(i*30) // 30 second intervals
			block.Header.VRFSeed = [32]byte{byte(i), 0, 0}
		}

		block.SignBlock(privKey)

		if i == 0 {
			cs.InitGenesis(block)
		} else {
			cs.AddBlock(block)
		}

		prevHash = block.Hash
	}

	// Iterate through all blocks
	iter := cs.NewBlockIterator(0)
	count := 0
	for {
		block, ok := iter.Next()
		if !ok {
			break
		}
		count++
		if uint64(count-1) != block.Header.Height {
			t.Errorf("Block height mismatch: expected %d, got %d", count-1, block.Header.Height)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 blocks, got %d", count)
	}
}

// TestChainState_GetChainInfo tests getting chain information.
func TestChainState_GetChainInfo(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Get chain info
	info := cs.GetChainInfo()

	if info.BestBlockHeight != 0 {
		t.Errorf("Expected best height 0, got %d", info.BestBlockHeight)
	}

	if info.TotalBlocks != 1 {
		t.Errorf("Expected total blocks 1, got %d", info.TotalBlocks)
	}

	if info.GenesisBlockHash == "" {
		t.Error("Genesis block hash is empty")
	}
}

// TestChainState_Persistence tests that chain state persists across restarts.
func TestChainState_Persistence(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create and initialize chain state
	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Add a block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Close and reopen
	cs.Close()

	cs2, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen chain state: %v", err)
	}
	defer cs2.Close()

	// Verify state persisted
	if cs2.GetBestBlockHeight() != 0 {
		t.Errorf("Expected height 0 after restart, got %d", cs2.GetBestBlockHeight())
	}

	// Verify we can retrieve the genesis block
	retrieved, err := cs2.GetBlockByHeight(0)
	if err != nil {
		t.Fatalf("Failed to retrieve genesis block after restart: %v", err)
	}

	if retrieved.Hash != genesisBlock.Hash {
		t.Error("Retrieved block hash mismatch after restart")
	}
}

// TestChainState_GetBlockInfo tests the GetBlockInfo API.
func TestChainState_GetBlockInfo(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Get block info
	info, err := cs.GetBlockInfo(0)
	if err != nil {
		t.Fatalf("Failed to get block info: %v", err)
	}

	if info.Height != 0 {
		t.Errorf("Expected height 0, got %d", info.Height)
	}

	if info.TransactionCount != 1 {
		t.Errorf("Expected 1 transaction, got %d", info.TransactionCount)
	}

	if info.Hash == "" {
		t.Error("Block hash is empty")
	}
}

// TestChainState_Rollback tests chain rollback functionality.
func TestChainState_Rollback(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis and two more blocks
	var prevHash [32]byte
	baseTimestamp := uint64(1704067200)
	for i := 0; i < 3; i++ {
		coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx.SignInput(0, privKey)

		var block *Block
		if i == 0 {
			block = NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
			block.Header.Timestamp = baseTimestamp
			block.Header.VRFSeed = [32]byte{}
		} else {
			block = NewBlock([]*Transaction{coinbaseTx}, prevHash, uint64(i), proposerAddr)
			block.Header.Timestamp = baseTimestamp + uint64(i*35) // 35 seconds after previous block
			block.Header.VRFSeed = [32]byte{byte(i), 0, 0}
		}

		block.SignBlock(privKey)

		if i == 0 {
			cs.InitGenesis(block)
		} else {
			cs.AddBlock(block)
		}

		prevHash = block.Hash
	}

	// Verify we have 3 blocks
	if cs.GetBestBlockHeight() != 2 {
		t.Errorf("Expected height 2, got %d", cs.GetBestBlockHeight())
	}

	// Rollback to height 1
	err = cs.RollbackChain(1)
	if err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify height is now 1
	if cs.GetBestBlockHeight() != 1 {
		t.Errorf("Expected height 1 after rollback, got %d", cs.GetBestBlockHeight())
	}

	// Verify block at height 2 no longer exists
	if cs.HasBlock(2) {
		t.Error("Block at height 2 should not exist after rollback")
	}

	// Verify block at height 1 still exists
	if !cs.HasBlock(1) {
		t.Error("Block at height 1 should still exist after rollback")
	}
}

// TestChainState_EvaluateBlockAcceptance tests block acceptance evaluation.
func TestChainState_EvaluateBlockAcceptance(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Test accepting next block in chain
	coinbaseTx2 := CreateCoinbaseV2(proposerAddr, 1)
	coinbaseTx2.SignInput(0, privKey)

	block2 := NewBlock([]*Transaction{coinbaseTx2}, genesisBlock.Hash, 1, proposerAddr)
	block2.Header.Timestamp = uint64(time.Now().Unix())
	block2.Header.VRFSeed = [32]byte{1, 2, 3}
	block2.SignBlock(privKey)

	result := cs.EvaluateBlockAcceptance(block2)
	if !result.CanAccept {
		t.Errorf("Expected block to be accepted, got: %s", result.Reason)
	}

	// Test block that doesn't extend current chain
	block3 := NewBlock([]*Transaction{coinbaseTx2}, [32]byte{1, 2, 3}, 1, proposerAddr)
	block3.Header.Timestamp = uint64(time.Now().Unix())
	block3.SignBlock(privKey)

	result = cs.EvaluateBlockAcceptance(block3)
	if result.CanAccept {
		t.Error("Expected block to be rejected (wrong parent)")
	}

	// Test block too far ahead
	coinbaseTx3 := CreateCoinbaseV2(proposerAddr, 2)
	coinbaseTx3.SignInput(0, privKey)

	block4 := NewBlock([]*Transaction{coinbaseTx3}, genesisBlock.Hash, 5, proposerAddr)
	block4.Header.Timestamp = uint64(time.Now().Unix())
	block4.SignBlock(privKey)

	result = cs.EvaluateBlockAcceptance(block4)
	if result.CanAccept {
		t.Error("Expected block to be rejected (too far ahead)")
	}
}

// TestChainState_GetBestBlock tests getting the best block.
func TestChainState_GetBestBlock(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	mempool := NewMempool(1000, MinTransactionFee)
	cs.SetMempool(mempool)

	consensus := NewConsensusState(DefaultPoSConfig())
	cs.SetConsensus(consensus)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Empty chain should return error
	_, err = cs.GetBestBlock()
	if err == nil {
		t.Error("Expected error for empty chain")
	}

	// Add genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = uint64(time.Now().Unix()) - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	cs.InitGenesis(genesisBlock)

	// Now GetBestBlock should work
	best, err := cs.GetBestBlock()
	if err != nil {
		t.Fatalf("Failed to get best block: %v", err)
	}

	if best.Hash != genesisBlock.Hash {
		t.Error("Best block hash mismatch")
	}
}
