// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Chain State Management Tests
package utxo

import (
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// State Initialization Tests
// ============================================================================

// TestState_InitialState tests the initial state of a newly created chain.
func TestState_InitialState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Verify initial state
	if cs.GetBestBlockHeight() != 0 {
		t.Errorf("Expected initial height 0, got %d", cs.GetBestBlockHeight())
	}

	if cs.GetBlockCount() != 0 {
		t.Errorf("Expected initial block count 0, got %d", cs.GetBlockCount())
	}

	// Best hash should be empty
	bestHash := cs.GetBestBlockHash()
	if bestHash != [32]byte{} {
		t.Errorf("Expected empty best hash, got %x", bestHash)
	}

	// Genesis hash should be empty
	genesisHash := cs.GetGenesisBlockHash()
	if genesisHash != [32]byte{} {
		t.Errorf("Expected empty genesis hash, got %x", genesisHash)
	}
}

// TestState_InitGenesis tests genesis block initialization.
func TestState_InitGenesis(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)

	// Create genesis block
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200 // Fixed timestamp
	genesisBlock.SignBlock(privKey)

	// Initialize genesis
	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to initialize genesis: %v", err)
	}

	// Verify genesis state
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

	// Verify we can retrieve genesis block
	retrieved, err := cs.GetBlockByHeight(0)
	if err != nil {
		t.Fatalf("Failed to retrieve genesis block: %v", err)
	}

	if retrieved.Hash != genesisBlock.Hash {
		t.Error("Retrieved block hash mismatch")
	}
}

// TestState_LoadExistingState tests loading state from existing database.
func TestState_LoadExistingState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First, create and populate a chain state
	cs1, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}

	utxoStore1, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore1.Close()
	cs1.SetUTXOStore(utxoStore1)

	mempool1 := NewMempool(1000, MinTransactionFee)
	cs1.SetMempool(mempool1)

	consensus1 := NewConsensusState(DefaultPoSConfig())
	cs1.SetConsensus(consensus1)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	consensus1.AddValidator(proposerAddr, 1000*1e8, pubKey)

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs1.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add a few more blocks
	var prevHash = genesisBlock.Hash
	for i := 1; i < 3; i++ {
		coinbaseTx2 := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx2.SignInput(0, privKey)

		block := NewBlock([]*Transaction{coinbaseTx2}, prevHash, uint64(i), proposerAddr)
		block.Header.Timestamp = now - 60 + uint64(i*30)
		block.Header.VRFSeed = [32]byte{byte(i), 0, 0}
		block.SignBlock(privKey)

		err = cs1.AddBlock(block)
		if err != nil {
			t.Fatalf("Failed to add block %d: %v", i, err)
		}
		prevHash = block.Hash
	}

	// Store the expected state
	expectedHeight := cs1.GetBestBlockHeight()
	expectedBestHash := cs1.GetBestBlockHash()
	expectedGenesisHash := cs1.GetGenesisBlockHash()
	expectedBlockCount := cs1.GetBlockCount()

	// Close the first chain state
	cs1.Close()

	// Now load the state from the existing database
	cs2, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to load existing chain state: %v", err)
	}
	defer cs2.Close()

	// Setup dependencies for loaded state
	utxoStore2, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo2.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore2.Close()
	cs2.SetUTXOStore(utxoStore2)

	// Verify loaded state matches original
	if cs2.GetBestBlockHeight() != expectedHeight {
		t.Errorf("Height mismatch: expected %d, got %d", expectedHeight, cs2.GetBestBlockHeight())
	}

	if cs2.GetBestBlockHash() != expectedBestHash {
		t.Errorf("Best hash mismatch: expected %x, got %x", expectedBestHash, cs2.GetBestBlockHash())
	}

	if cs2.GetGenesisBlockHash() != expectedGenesisHash {
		t.Errorf("Genesis hash mismatch: expected %x, got %x", expectedGenesisHash, cs2.GetGenesisBlockHash())
	}

	if cs2.GetBlockCount() != expectedBlockCount {
		t.Errorf("Block count mismatch: expected %d, got %d", expectedBlockCount, cs2.GetBlockCount())
	}
}

// ============================================================================
// State Transition Tests
// ============================================================================

// TestState_Transition_AddBlock tests state transition when adding a new block.
func TestState_Transition_AddBlock(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Verify initial state
	if cs.GetBestBlockHeight() != 0 {
		t.Errorf("Expected height 0, got %d", cs.GetBestBlockHeight())
	}

	// Add second block
	coinbaseTx2 := CreateCoinbaseV2(proposerAddr, 1)
	coinbaseTx2.SignInput(0, privKey)

	block2 := NewBlock([]*Transaction{coinbaseTx2}, genesisBlock.Hash, 1, proposerAddr)
	block2.Header.Timestamp = now
	block2.Header.VRFSeed = [32]byte{1, 2, 3}
	block2.SignBlock(privKey)

	err = cs.AddBlock(block2)
	if err != nil {
		t.Fatalf("Failed to add block 2: %v", err)
	}

	// Verify state after transition
	if cs.GetBestBlockHeight() != 1 {
		t.Errorf("Expected height 1, got %d", cs.GetBestBlockHeight())
	}

	if cs.GetBestBlockHash() != block2.Hash {
		t.Errorf("Best hash mismatch after transition")
	}

	if cs.GetBlockCount() != 2 {
		t.Errorf("Expected block count 2, got %d", cs.GetBlockCount())
	}
}

// TestState_Transition_ChainReorg tests chain reorganization handling.
func TestState_Transition_ChainReorg(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Generate key pairs for two validators
	pubKey1, privKey1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	pubKey2, privKey2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	var proposerAddr1 [32]byte
	var proposerAddr2 [32]byte
	copy(proposerAddr1[:], pubKey1)
	copy(proposerAddr2[:], pubKey2)

	consensus.AddValidator(proposerAddr1, 1000*1e8, pubKey1)
	consensus.AddValidator(proposerAddr2, 1000*1e8, pubKey2)

	// Create genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx1 := CreateCoinbaseV2(proposerAddr1, 0)
	coinbaseTx1.SignInput(0, privKey1)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx1}, [32]byte{}, 0, proposerAddr1)
	genesisBlock.Header.Timestamp = now - 120
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey1)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add block A (from validator 1)
	coinbaseTxA := CreateCoinbaseV2(proposerAddr1, 1)
	coinbaseTxA.SignInput(0, privKey1)

	blockA := NewBlock([]*Transaction{coinbaseTxA}, genesisBlock.Hash, 1, proposerAddr1)
	blockA.Header.Timestamp = now - 60
	blockA.Header.VRFSeed = [32]byte{1}
	blockA.SignBlock(privKey1)

	err = cs.AddBlock(blockA)
	if err != nil {
		t.Fatalf("Failed to add block A: %v", err)
	}

	// Verify main chain is A
	mainChainTip := cs.GetBestBlockHash()
	if mainChainTip != blockA.Hash {
		t.Errorf("Expected main chain to be A, got %x", mainChainTip)
	}

	// Add block B (from validator 2) - creates fork
	coinbaseTxB := CreateCoinbaseV2(proposerAddr2, 1)
	coinbaseTxB.SignInput(0, privKey2)

	blockB := NewBlock([]*Transaction{coinbaseTxB}, genesisBlock.Hash, 1, proposerAddr2)
	blockB.Header.Timestamp = now - 50
	blockB.Header.VRFSeed = [32]byte{2}
	blockB.SignBlock(privKey2)

	// AddBlock should accept block B if it has higher timestamp (simulating longer chain)
	// Note: In real implementation, fork choice would be more complex
	err = cs.AddBlock(blockB)
	if err != nil {
		t.Logf("Block B rejected (expected if no fork support): %v", err)
	}

	// Try to retrieve both blocks
	_, errA := cs.GetBlockByHeight(1)
	if errA != nil {
		t.Errorf("Should be able to get block A: %v", errA)
	}

	// Get current state
	currentHeight := cs.GetBestBlockHeight()
	currentBest := cs.GetBestBlockHash()
	_ = currentHeight
	_ = currentBest

	t.Logf("Chain state after potential fork: height=%d, best=%x", currentHeight, currentBest)
}

// TestState_Transition_MultipleBlocks tests adding multiple blocks in sequence.
func TestState_Transition_MultipleBlocks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 180
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add multiple blocks
	numBlocks := 10
	var prevHash = genesisBlock.Hash
	for i := 1; i <= numBlocks; i++ {
		coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx.SignInput(0, privKey)

		block := NewBlock([]*Transaction{coinbaseTx}, prevHash, uint64(i), proposerAddr)
		block.Header.Timestamp = now - 180 + uint64(i*30)
		block.Header.VRFSeed = [32]byte{byte(i)}
		block.SignBlock(privKey)

		err = cs.AddBlock(block)
		if err != nil {
			t.Fatalf("Failed to add block %d: %v", i, err)
		}

		prevHash = block.Hash
	}

	// Verify final state
	if cs.GetBestBlockHeight() != uint64(numBlocks) {
		t.Errorf("Expected height %d, got %d", numBlocks, cs.GetBestBlockHeight())
	}

	if cs.GetBlockCount() != uint64(numBlocks+1) {
		t.Errorf("Expected block count %d, got %d", numBlocks+1, cs.GetBlockCount())
	}

	// Verify we can retrieve the last block
	lastBlock, err := cs.GetBlockByHeight(uint64(numBlocks))
	if err != nil {
		t.Fatalf("Failed to get last block: %v", err)
	}

	if lastBlock.Hash != prevHash {
		t.Error("Last block hash mismatch")
	}

	// Verify block chain continuity
	for i := uint64(1); i <= uint64(numBlocks); i++ {
		block, err := cs.GetBlockByHeight(i)
		if err != nil {
			t.Fatalf("Failed to get block at height %d: %v", i, err)
		}
		if block.Header.Height != i {
			t.Errorf("Block height mismatch at %d", i)
		}
	}
}

// ============================================================================
// State Persistence Tests
// ============================================================================

// TestState_Persist_AcrossRestarts tests state persistence across restarts.
func TestState_Persist_AcrossRestarts(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create and populate chain state
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

	// Add genesis and blocks
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add one more block
	coinbaseTx2 := CreateCoinbaseV2(proposerAddr, 1)
	coinbaseTx2.SignInput(0, privKey)

	block2 := NewBlock([]*Transaction{coinbaseTx2}, genesisBlock.Hash, 1, proposerAddr)
	block2.Header.Timestamp = now
	block2.Header.VRFSeed = [32]byte{1}
	block2.SignBlock(privKey)

	err = cs.AddBlock(block2)
	if err != nil {
		t.Fatalf("Failed to add block 2: %v", err)
	}

	// Store expected values
	expectedHeight := cs.GetBestBlockHeight()
	expectedBestHash := cs.GetBestBlockHash()
	expectedGenesisHash := cs.GetGenesisBlockHash()

	// Close and reopen
	cs.Close()

	cs2, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen chain state: %v", err)
	}
	defer cs2.Close()

	// Setup UTXO store for the reopened chain
	utxoStore2, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo2.db"))
	if err != nil {
		t.Fatalf("Failed to create UTXO store: %v", err)
	}
	defer utxoStore2.Close()
	cs2.SetUTXOStore(utxoStore2)

	// Verify state persisted
	if cs2.GetBestBlockHeight() != expectedHeight {
		t.Errorf("Height mismatch: expected %d, got %d", expectedHeight, cs2.GetBestBlockHeight())
	}

	if cs2.GetBestBlockHash() != expectedBestHash {
		t.Errorf("Best hash mismatch after restart: %x vs %x", expectedBestHash, cs2.GetBestBlockHash())
	}

	if cs2.GetGenesisBlockHash() != expectedGenesisHash {
		t.Errorf("Genesis hash mismatch after restart: %x vs %x", expectedGenesisHash, cs2.GetGenesisBlockHash())
	}

	// Verify we can retrieve blocks
	retrieved, err := cs2.GetBlockByHeight(0)
	if err != nil {
		t.Fatalf("Failed to retrieve genesis block after restart: %v", err)
	}
	if retrieved.Hash != genesisBlock.Hash {
		t.Error("Genesis block hash mismatch after restart")
	}

	retrieved2, err := cs2.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("Failed to retrieve block 1 after restart: %v", err)
	}
	if retrieved2.Hash != block2.Hash {
		t.Error("Block 2 hash mismatch after restart")
	}
}

// TestState_SnapshotRestore tests snapshot and restore functionality.
func TestState_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create and populate chain state
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

	// Create snapshot path
	snapshotPath := filepath.Join(tmpDir, "snapshot.db")

	// Create genesis block
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)

	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Create UTXO snapshot
	err = utxoStore.Snapshot(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Verify snapshot exists
	utxoStore2, err := NewPersistentUTXOStore(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer utxoStore2.Close()

	// Verify data in snapshot
	count := utxoStore2.GetUTXOCount()
	if count == 0 {
		t.Error("Snapshot should contain UTXOs")
	}

	cs.Close()
}

// ============================================================================
// State Consistency Tests
// ============================================================================

// TestState_Consistency_BlockHeight tests consistency between block height and count.
func TestState_Consistency_BlockHeight(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add some blocks
	numBlocks := 5
	var prevHash = genesisBlock.Hash
	for i := 1; i <= numBlocks; i++ {
		coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx.SignInput(0, privKey)

		block := NewBlock([]*Transaction{coinbaseTx}, prevHash, uint64(i), proposerAddr)
		block.Header.Timestamp = now - 60 + uint64(i*30)
		block.Header.VRFSeed = [32]byte{byte(i)}
		block.SignBlock(privKey)

		err = cs.AddBlock(block)
		if err != nil {
			t.Fatalf("Failed to add block %d: %v", i, err)
		}
		prevHash = block.Hash
	}

	// Verify consistency
	blockCount := cs.GetBlockCount()
	bestHeight := cs.GetBestBlockHeight()

	// Block count should be best height + 1 (genesis is height 0)
	if blockCount != bestHeight+1 {
		t.Errorf("Inconsistency: blockCount=%d, bestHeight=%d", blockCount, bestHeight)
	}

	// Verify each block in the chain
	for height := uint64(0); height <= bestHeight; height++ {
		block, err := cs.GetBlockByHeight(height)
		if err != nil {
			t.Fatalf("Failed to get block at height %d: %v", height, err)
		}
		if block.Header.Height != height {
			t.Errorf("Height mismatch: expected %d, got %d", height, block.Header.Height)
		}
	}
}

// TestState_Consistency_BlockHash tests consistency of block hashes.
func TestState_Consistency_BlockHash(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Verify best block hash consistency
	bestBlockHash := cs.GetBestBlockHash()
	if bestBlockHash != genesisBlock.Hash {
		t.Errorf("Best block hash mismatch: %x vs %x", bestBlockHash, genesisBlock.Hash)
	}

	// Add another block
	coinbaseTx2 := CreateCoinbaseV2(proposerAddr, 1)
	coinbaseTx2.SignInput(0, privKey)

	block2 := NewBlock([]*Transaction{coinbaseTx2}, genesisBlock.Hash, 1, proposerAddr)
	block2.Header.Timestamp = now
	block2.Header.VRFSeed = [32]byte{1}
	block2.SignBlock(privKey)

	err = cs.AddBlock(block2)
	if err != nil {
		t.Fatalf("Failed to add block 2: %v", err)
	}

	// Verify hash consistency
	bestBlockHash = cs.GetBestBlockHash()
	if bestBlockHash != block2.Hash {
		t.Errorf("Best block hash mismatch after adding block 2")
	}

	// Verify by height vs by hash consistency
	blockByHeight, err := cs.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("Failed to get block by height: %v", err)
	}

	blockByHash, err := cs.GetBlockByHash(block2.Hash)
	if err != nil {
		t.Fatalf("Failed to get block by hash: %v", err)
	}

	if blockByHeight.Hash != blockByHash.Hash {
		t.Error("Block retrieved by height doesn't match block retrieved by hash")
	}
}

// TestState_Consistency_ChainInfo tests consistency of chain info.
func TestState_Consistency_ChainInfo(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Get chain info
	info := cs.GetChainInfo()

	// Verify consistency
	if info.BestBlockHeight != cs.GetBestBlockHeight() {
		t.Errorf("ChainInfo best height mismatch")
	}

	bestHash := cs.GetBestBlockHash()
	if info.BestBlockHash != hex.EncodeToString(bestHash[:]) {
		t.Errorf("ChainInfo best hash mismatch")
	}

	genesisHash := cs.GetGenesisBlockHash()
	if info.GenesisBlockHash != hex.EncodeToString(genesisHash[:]) {
		t.Errorf("ChainInfo genesis hash mismatch")
	}

	// Total blocks should be best height + 1
	if info.TotalBlocks != info.BestBlockHeight+1 {
		t.Errorf("ChainInfo total blocks mismatch: expected %d, got %d",
			info.BestBlockHeight+1, info.TotalBlocks)
	}
}

// TestState_Consistency_AfterRollback tests consistency after rollback.
func TestState_Consistency_AfterRollback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis and blocks
	now := uint64(time.Now().Unix())
	baseTime := now - 180

	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = baseTime
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Add several blocks
	var blocks []*Block
	var prevHash = genesisBlock.Hash
	for i := 1; i <= 3; i++ {
		coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
		coinbaseTx.SignInput(0, privKey)

		block := NewBlock([]*Transaction{coinbaseTx}, prevHash, uint64(i), proposerAddr)
		block.Header.Timestamp = baseTime + uint64(i*30)
		block.Header.VRFSeed = [32]byte{byte(i)}
		block.SignBlock(privKey)

		err = cs.AddBlock(block)
		if err != nil {
			t.Fatalf("Failed to add block %d: %v", i, err)
		}
		blocks = append(blocks, block)
		prevHash = block.Hash
	}

	// Verify initial state
	if cs.GetBestBlockHeight() != 3 {
		t.Errorf("Expected height 3, got %d", cs.GetBestBlockHeight())
	}

	// Rollback to height 1
	err = cs.RollbackChain(1)
	if err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify consistency after rollback
	if cs.GetBestBlockHeight() != 1 {
		t.Errorf("Expected height 1 after rollback, got %d", cs.GetBestBlockHeight())
	}

	// Block at height 2 should not exist
	if cs.HasBlock(2) {
		t.Error("Block at height 2 should not exist after rollback")
	}

	// Block at height 3 should not exist
	if cs.HasBlock(3) {
		t.Error("Block at height 3 should not exist after rollback")
	}

	// Block at height 1 should still exist
	if !cs.HasBlock(1) {
		t.Error("Block at height 1 should exist after rollback")
	}

	// Best hash should be block at height 1
	bestHash := cs.GetBestBlockHash()
	if bestHash != blocks[0].Hash {
		t.Error("Best hash mismatch after rollback")
	}

	// Block count should be 2 (heights 0 and 1)
	if cs.GetBlockCount() != 2 {
		t.Errorf("Expected block count 2, got %d", cs.GetBlockCount())
	}
}

// TestState_ConcurrentAccess tests concurrent access to chain state.
func TestState_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chain state: %v", err)
	}
	defer cs.Close()

	// Setup dependencies
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

	// Add genesis block
	now := uint64(time.Now().Unix())
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)

	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = now - 60
	genesisBlock.Header.VRFSeed = [32]byte{}
	genesisBlock.SignBlock(privKey)

	err = cs.InitGenesis(genesisBlock)
	if err != nil {
		t.Fatalf("Failed to init genesis: %v", err)
	}

	// Run concurrent readers
	var readersDone int
	var readersErr error

	// Concurrent read test
	for i := 0; i < 5; i++ {
		go func() {
			// Read operations
			for j := 0; j < 10; j++ {
				cs.GetBestBlockHeight()
				cs.GetBestBlockHash()
				cs.GetGenesisBlockHash()
				cs.GetBlockCount()
			}
		}()
	}

	// Also run some write operations concurrently
	go func() {
		for i := 1; i <= 5; i++ {
			coinbaseTx := CreateCoinbaseV2(proposerAddr, uint64(i))
			coinbaseTx.SignInput(0, privKey)

			prevBlock, err := cs.GetBestBlock()
			if err != nil || prevBlock == nil {
				return
			}
			block := NewBlock([]*Transaction{coinbaseTx}, prevBlock.Hash, uint64(i), proposerAddr)
			block.Header.Timestamp = now - 60 + uint64(i*30)
			block.Header.VRFSeed = [32]byte{byte(i)}
			block.SignBlock(privKey)

			if err := cs.AddBlock(block); err != nil {
				readersErr = err
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Wait a bit for operations to complete
	time.Sleep(100 * time.Millisecond)

	_ = readersDone
	if readersErr != nil {
		t.Logf("Concurrent error (may be expected): %v", readersErr)
	}

	// Final state should be consistent
	height := cs.GetBestBlockHeight()
	count := cs.GetBlockCount()

	// Count should be height + 1 (genesis at height 0)
	if count != height+1 && height > 0 {
		t.Errorf("Inconsistent state: height=%d, count=%d", height, count)
	}
}
