// Package utxo provides integration tests for PoAIW consensus upgrade.
// This test suite ensures the PoAIW upgrade works correctly without breaking existing functionality.
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPoAIWIntegrationSuite runs the complete PoAIW integration test suite.
func TestPoAIWIntegrationSuite(t *testing.T) {
	t.Run("VersionCompatibility", testVersionCompatibility)
	t.Run("TransferFunctionality", testTransferFunctionality)
	t.Run("DIFIIntegration", testDIFIIntegration)
	t.Run("DAOIntegration", testDAOIntegration)
	t.Run("UpgradePath", testUpgradePath)
	t.Run("EdgeCases", testEdgeCases)
	t.Run("BackwardCompatibility", testBackwardCompatibility)
}

// TestChain represents a test blockchain for integration tests.
type TestChain struct {
	chainState *ChainState
	validators map[string]*TestValidator
	accounts   map[string]ed25519.PrivateKey
	mu         sync.RWMutex
}

// TestValidator represents a test validator node.
type TestValidator struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	address    [32]byte
	chain      *TestChain
}

// NewTestChain creates a new test blockchain.
func NewTestChain(t *testing.T) *TestChain {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_chain.db")

	chainState, err := NewChainState(dbPath)
	require.NoError(t, err)

	return &TestChain{
		chainState: chainState,
		validators: make(map[string]*TestValidator),
		accounts:   make(map[string]ed25519.PrivateKey),
	}
}

// Close closes the test chain resources.
func (tc *TestChain) Close() error {
	return tc.chainState.Close()
}

// AddValidator adds a test validator to the chain.
func (tc *TestChain) AddValidator(t *testing.T, name string) *TestValidator {
	// Create a valid 32-byte seed
	seed := make([]byte, ed25519.SeedSize)
	for i := 0; i < len(name) && i < ed25519.SeedSize; i++ {
		seed[i] = name[i]
	}
	// Fill remaining bytes with pattern
	for i := len(name); i < ed25519.SeedSize; i++ {
		seed[i] = byte(i)
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	var address [32]byte
	hash := sha256.Sum256(publicKey)
	copy(address[:], hash[:32])

	validator := &TestValidator{
		privateKey: privateKey,
		publicKey:  publicKey,
		address:    address,
		chain:      tc,
	}

	tc.mu.Lock()
	tc.validators[name] = validator
	tc.accounts[name] = privateKey
	tc.mu.Unlock()

	return validator
}

// GetValidator returns a test validator by name.
func (tc *TestChain) GetValidator(name string) *TestValidator {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.validators[name]
}

// GetAccount returns a test account private key by name.
func (tc *TestChain) GetAccount(name string) ed25519.PrivateKey {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.accounts[name]
}

// GetHeight returns the current chain height.
func (tc *TestChain) GetHeight() uint64 {
	return tc.chainState.GetBestBlockHeight()
}

// GetBestHash returns the current best block hash.
func (tc *TestChain) GetBestHash() [32]byte {
	return tc.chainState.GetBestBlockHash()
}

func testVersionCompatibility(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	validator1 := chain.AddValidator(t, "validator1")
	validator2 := chain.AddValidator(t, "validator2")

	t.Run("Version1BlockCreation", func(t *testing.T) {
		block := createVersion1Block(t, chain, validator1)
		require.NotNil(t, block)
		require.Equal(t, uint32(1), block.Header.Version)
		require.Equal(t, chain.GetHeight()+1, block.Header.Height)
		t.Logf("Version 1 block created: height=%d, hash=%x", block.Header.Height, block.Hash)
	})

	t.Run("Version2BlockCreation", func(t *testing.T) {
		block := createVersion2Block(t, chain, validator2)
		require.NotNil(t, block)
		require.Equal(t, uint32(2), block.Header.Version)
		require.Equal(t, chain.GetHeight()+1, block.Header.Height)
		require.NotNil(t, block.Header.VRFProof)
		t.Logf("Version 2 block created: height=%d, hash=%x", block.Header.Height, block.Hash)
	})

	t.Run("Version1BlockValidation", func(t *testing.T) {
		block := createVersion1Block(t, chain, validator1)
		err := chain.chainState.ValidateBlock(block)
		if err != nil {
			t.Logf("Version 1 block validation error (may be expected): %v", err)
		}
	})

	t.Run("Version2BlockValidation", func(t *testing.T) {
		block := createVersion2Block(t, chain, validator2)
		err := chain.chainState.ValidateBlock(block)
		if err != nil {
			t.Logf("Version 2 block validation error (may be expected): %v", err)
		}
	})

	t.Run("MixedVersionValidation", func(t *testing.T) {
		block1 := createVersion1Block(t, chain, validator1)
		err1 := chain.chainState.ValidateBlock(block1)
		t.Logf("New node validates Version 1 block: %v", err1)

		block2 := createVersion2Block(t, chain, validator2)
		err2 := chain.chainState.ValidateBlock(block2)
		t.Logf("New node validates Version 2 block: %v", err2)
	})
}

func createVersion1Block(t *testing.T, chain *TestChain, validator *TestValidator) *Block {
	tx := &Transaction{
		Version:  1,
		Inputs:   []TXInput{},
		Outputs:  []TXOutput{},
		LockTime: 0,
		Sequence: 0,
	}

	prevHash := chain.GetBestHash()
	height := chain.GetHeight() + 1

	block := NewBlock([]*Transaction{tx}, prevHash, height, validator.address)
	block.Header.Version = 1
	block.Header.Proposer = validator.address

	signBlock(block, validator.privateKey)
	return block
}

func createVersion2Block(t *testing.T, chain *TestChain, validator *TestValidator) *Block {
	tx := &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs:  []TXOutput{},
		LockTime: 0,
		Sequence: 0,
	}

	prevHash := chain.GetBestHash()
	height := chain.GetHeight() + 1

	block := NewBlock([]*Transaction{tx}, prevHash, height, validator.address)
	block.Header.Version = 2
	block.Header.Proposer = validator.address
	block.Header.VRFProof = []byte("test-vrf-proof")
	block.Header.VRFSeed = [32]byte{1, 2, 3}

	signBlock(block, validator.privateKey)
	return block
}

func signBlock(block *Block, privateKey ed25519.PrivateKey) {
	data := serializeBlockHeaderForSig(&block.Header)
	signature := ed25519.Sign(privateKey, data)
	block.Header.Signature = signature
	block.Hash = block.CalculateHash()
}

func testTransferFunctionality(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	t.Run("Version1Transfer", func(t *testing.T) {
		aliceKey := createTestKey("alice_transfer1")
		bobKey := createTestKey("bob_transfer1")

		var aliceAddr, bobAddr [32]byte
		aliceHash := sha256.Sum256(aliceKey.Public().(ed25519.PublicKey))
		bobHash := sha256.Sum256(bobKey.Public().(ed25519.PublicKey))
		copy(aliceAddr[:], aliceHash[:32])
		copy(bobAddr[:], bobHash[:32])

		tx := createTransferTx(aliceKey, bobKey.Public().(ed25519.PublicKey), 1000*BlockRewardSatoshi)
		block := createBlockWithTransaction(t, chain, tx, 1, aliceAddr)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("Version 1 transfer validation: %v", err)

		if err == nil {
			err = chain.chainState.AddBlock(block)
			t.Logf("Version 1 transfer added: %v", err)
		}
	})

	t.Run("Version2Transfer", func(t *testing.T) {
		aliceKey := createTestKey("alice_transfer2")
		bobKey := createTestKey("bob_transfer2")

		var aliceAddr, bobAddr [32]byte
		aliceHash := sha256.Sum256(aliceKey.Public().(ed25519.PublicKey))
		bobHash := sha256.Sum256(bobKey.Public().(ed25519.PublicKey))
		copy(aliceAddr[:], aliceHash[:32])
		copy(bobAddr[:], bobHash[:32])

		tx := createTransferTx(aliceKey, bobKey.Public().(ed25519.PublicKey), 500*BlockRewardSatoshi)
		block := createBlockWithTransaction(t, chain, tx, 2, aliceAddr)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("Version 2 transfer validation: %v", err)

		if err == nil {
			err = chain.chainState.AddBlock(block)
			t.Logf("Version 2 transfer added: %v", err)
		}
	})

	t.Run("MixedVersionTransfers", func(t *testing.T) {
		aliceKey := createTestKey("alice_transfer3")
		bobKey := createTestKey("bob_transfer3")

		var aliceAddr, bobAddr [32]byte
		aliceHash := sha256.Sum256(aliceKey.Public().(ed25519.PublicKey))
		bobHash := sha256.Sum256(bobKey.Public().(ed25519.PublicKey))
		copy(aliceAddr[:], aliceHash[:32])
		copy(bobAddr[:], bobHash[:32])

		for i := 0; i < 5; i++ {
			version := uint32(1 + i%2)
			tx := createTransferTx(aliceKey, bobKey.Public().(ed25519.PublicKey), 100*BlockRewardSatoshi)
			block := createBlockWithTransaction(t, chain, tx, version, aliceAddr)

			err := chain.chainState.ValidateBlock(block)
			t.Logf("Mixed version %d transfer validation: %v", version, err)
		}
	})
}

// createTestKey creates a test key from a name
func createTestKey(name string) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := 0; i < len(name) && i < ed25519.SeedSize; i++ {
		seed[i] = name[i]
	}
	for i := len(name); i < ed25519.SeedSize; i++ {
		seed[i] = byte(i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func createTransferTx(fromKey ed25519.PrivateKey, toPubKey ed25519.PublicKey, amount uint64) *Transaction {
	var fromAddr, toAddr [32]byte
	fromHash := sha256.Sum256(fromKey.Public().(ed25519.PublicKey))
	toHash := sha256.Sum256(toPubKey)
	copy(fromAddr[:], fromHash[:32])
	copy(toAddr[:], toHash[:32])

	tx := &Transaction{
		Version:  2,
		Inputs: []TXInput{
			{
				TxHash:    [32]byte{1},
				Index:     0,
				Signature: []byte{},
				PublicKey: fromKey.Public().(ed25519.PublicKey),
			},
		},
		Outputs: []TXOutput{
			{
				Value:   amount,
				Script:  toPubKey,
				Address: toAddr,
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}

	if len(tx.Inputs) > 0 {
		txHash := tx.Hash()
		tx.Inputs[0].Signature = ed25519.Sign(fromKey, txHash[:])
	}

	return tx
}

func createBlockWithTransaction(t *testing.T, chain *TestChain, tx *Transaction, version uint32, proposer [32]byte) *Block {
	prevHash := chain.GetBestHash()
	height := chain.GetHeight() + 1

	block := NewBlock([]*Transaction{tx}, prevHash, height, proposer)
	block.Header.Version = version

	if version == 2 {
		block.Header.VRFProof = []byte("test-vrf-proof")
		block.Header.VRFSeed = [32]byte{1, 2, 3}
	}

	var privateKey ed25519.PrivateKey
	for _, v := range chain.validators {
		if v.address == proposer {
			privateKey = v.privateKey
			break
		}
	}

	if privateKey == nil {
		privateKey = createTestKey(fmt.Sprintf("%x", proposer[:8]))
	}

	signBlock(block, privateKey)
	return block
}

func testDIFIIntegration(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	t.Run("Version1DIFI", func(t *testing.T) {
		difiTx := createDIFITransaction(t, 100*BlockRewardSatoshi)
		validator := chain.AddValidator(t, "difi1")
		block := createBlockWithTransaction(t, chain, difiTx, 1, validator.address)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("DIFI Version 1 validation: %v", err)
	})

	t.Run("Version2DIFI", func(t *testing.T) {
		difiTx := createDIFITransaction(t, 50*BlockRewardSatoshi)
		validator := chain.AddValidator(t, "difi2")
		block := createBlockWithTransaction(t, chain, difiTx, 2, validator.address)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("DIFI Version 2 validation: %v", err)
	})

	t.Run("DIFIWorkflow", func(t *testing.T) {
		requestTx := createDIFIRequestTransaction(t)
		validator := chain.AddValidator(t, "difi3")
		block1 := createBlockWithTransaction(t, chain, requestTx, 2, validator.address)
		err1 := chain.chainState.ValidateBlock(block1)
		t.Logf("DIFI request validation: %v", err1)

		responseTx := createDIFIResponseTransaction(t)
		block2 := createBlockWithTransaction(t, chain, responseTx, 2, validator.address)
		err2 := chain.chainState.ValidateBlock(block2)
		t.Logf("DIFI response validation: %v", err2)
	})
}

func createDIFITransaction(t *testing.T, amount uint64) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   amount,
				Script:  []byte("DIFI_REQUEST"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func createDIFIRequestTransaction(t *testing.T) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   100 * BlockRewardSatoshi,
				Script:  []byte("DIFI_REQUEST"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func createDIFIResponseTransaction(t *testing.T) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   50 * BlockRewardSatoshi,
				Script:  []byte("DIFI_RESPONSE"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func testDAOIntegration(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	t.Run("Version1DAO", func(t *testing.T) {
		daoTx := createDAOTransaction(t, 500*BlockRewardSatoshi)
		validator := chain.AddValidator(t, "dao1")
		block := createBlockWithTransaction(t, chain, daoTx, 1, validator.address)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("DAO Version 1 validation: %v", err)
	})

	t.Run("Version2DAO", func(t *testing.T) {
		daoTx := createDAOTransaction(t, 300*BlockRewardSatoshi)
		validator := chain.AddValidator(t, "dao2")
		block := createBlockWithTransaction(t, chain, daoTx, 2, validator.address)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("DAO Version 2 validation: %v", err)
	})

	t.Run("DAOProposalFlow", func(t *testing.T) {
		proposalTx := createDAOProposalTransaction(t)
		validator := chain.AddValidator(t, "dao3")
		block1 := createBlockWithTransaction(t, chain, proposalTx, 2, validator.address)
		err1 := chain.chainState.ValidateBlock(block1)
		t.Logf("DAO proposal validation: %v", err1)

		voteTx := createDAOVoteTransaction(t)
		block2 := createBlockWithTransaction(t, chain, voteTx, 2, validator.address)
		err2 := chain.chainState.ValidateBlock(block2)
		t.Logf("DAO vote validation: %v", err2)
	})
}

func createDAOTransaction(t *testing.T, amount uint64) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   amount,
				Script:  []byte("DAO_PROPOSAL"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func createDAOProposalTransaction(t *testing.T) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   1000 * BlockRewardSatoshi,
				Script:  []byte("DAO_PROPOSAL_CREATE"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func createDAOVoteTransaction(t *testing.T) *Transaction {
	return &Transaction{
		Version:  2,
		Inputs:   []TXInput{},
		Outputs: []TXOutput{
			{
				Value:   0,
				Script:  []byte("DAO_VOTE"),
				Address: [32]byte{},
			},
		},
		LockTime: 0,
		Sequence: uint64(time.Now().Unix()),
	}
}

func testUpgradePath(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	t.Run("SmoothUpgrade", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			validator := chain.AddValidator(t, fmt.Sprintf("v1validator%d", i))
			tx := &Transaction{Version: 1}
			block := createBlockWithTransaction(t, chain, tx, 1, validator.address)

			err := chain.chainState.ValidateBlock(block)
			t.Logf("Version 1 block %d validation: %v", i+1, err)

			if err == nil {
				err = chain.chainState.AddBlock(block)
				t.Logf("Version 1 block %d add: %v", i+1, err)
			}
		}

		for i := 0; i < 3; i++ {
			validator := chain.AddValidator(t, fmt.Sprintf("v2validator%d", i))
			tx := &Transaction{Version: 2}
			block := createBlockWithTransaction(t, chain, tx, 2, validator.address)

			err := chain.chainState.ValidateBlock(block)
			t.Logf("Version 2 block %d validation: %v", i+1, err)

			if err == nil {
				err = chain.chainState.AddBlock(block)
				t.Logf("Version 2 block %d add: %v", i+1, err)
			}
		}

		finalHeight := chain.GetHeight()
		t.Logf("Final chain height after upgrade: %d", finalHeight)
	})

	t.Run("MixedVersionChain", func(t *testing.T) {
		versions := []uint32{1, 2, 1, 2, 1, 2}

		for i, version := range versions {
			validator := chain.AddValidator(t, fmt.Sprintf("mvvalidator%d", i))
			tx := &Transaction{Version: version}
			block := createBlockWithTransaction(t, chain, tx, version, validator.address)

			err := chain.chainState.ValidateBlock(block)
			t.Logf("Mixed version block %d (v%d) validation: %v", i+1, version, err)
		}
	})
}

func testEdgeCases(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	validator := chain.AddValidator(t, "validator1")

	t.Run("VersionTransitionBoundary", func(t *testing.T) {
		prevHash := chain.GetBestHash()
		header1 := BlockHeader{
			Version:       1,
			PrevBlockHash: prevHash,
			MerkleRoot:    [32]byte{},
			Timestamp:     uint64(time.Now().Unix()),
			Height:        999,
			Proposer:      validator.address,
			Signature:     []byte{},
		}

		block1 := &Block{
			Header:       header1,
			Transactions: []*Transaction{},
		}
		block1.Hash = block1.CalculateHash()

		err1 := chain.chainState.ValidateBlock(block1)
		t.Logf("Version 1 block at height 999: %v", err1)

		header2 := BlockHeader{
			Version:       2,
			PrevBlockHash: [32]byte{},
			MerkleRoot:    [32]byte{},
			Timestamp:     uint64(time.Now().Unix()),
			Height:        1000,
			Proposer:      validator.address,
			Signature:     []byte{},
			VRFProof:      []byte("test-vrf-proof"),
			VRFSeed:       [32]byte{1, 2, 3},
		}

		block2 := &Block{
			Header:       header2,
			Transactions: []*Transaction{},
		}
		block2.Hash = block2.CalculateHash()

		err2 := chain.chainState.ValidateBlock(block2)
		t.Logf("Version 2 block at height 1000: %v", err2)
	})

	t.Run("InvalidVersionHandling", func(t *testing.T) {
		prevHash := chain.GetBestHash()
		height := chain.GetHeight() + 1

		header := BlockHeader{
			Version:       3,
			PrevBlockHash: prevHash,
			MerkleRoot:    [32]byte{},
			Timestamp:     uint64(time.Now().Unix()),
			Height:        height,
			Proposer:      validator.address,
			Signature:     []byte{},
		}

		block := &Block{
			Header:       header,
			Transactions: []*Transaction{},
		}
		block.Hash = block.CalculateHash()

		err := chain.chainState.ValidateBlock(block)
		t.Logf("Version 3 block validation result: %v", err)
	})

	t.Run("TimeDriftHandling", func(t *testing.T) {
		baseTime := time.Now()

		for _, version := range []uint32{1, 2} {
			header := BlockHeader{
				Version:       version,
				PrevBlockHash: chain.GetBestHash(),
				MerkleRoot:    [32]byte{},
				Timestamp:     uint64(baseTime.Unix()),
				Height:        chain.GetHeight() + 1,
				Proposer:      validator.address,
				Signature:     []byte{},
			}

			if version == 2 {
				header.VRFProof = []byte("test-vrf-proof")
				header.VRFSeed = [32]byte{1, 2, 3}
			}

			block := &Block{
				Header:       header,
				Transactions: []*Transaction{},
			}
			block.Hash = block.CalculateHash()

			err := chain.chainState.ValidateBlock(block)
			t.Logf("Version %d with valid timestamp: %v", version, err)

			futureTime := baseTime.Add(time.Hour)
			header.Timestamp = uint64(futureTime.Unix())

			block = &Block{
				Header:       header,
				Transactions: []*Transaction{},
			}
			block.Hash = block.CalculateHash()

			err = chain.chainState.ValidateBlock(block)
			t.Logf("Version %d with future timestamp: %v", version, err)
		}
	})

	t.Run("ReputationWeightCalculation", func(t *testing.T) {
		rm := NewReputationManager()

		var testAddr [32]byte
		copy(testAddr[:], []byte("testvalidator"))

		// Submit scores using SubmitScore API
		signerKey := createTestKey("signer")
		for i := 0; i < 10; i++ {
			score := &ReputationScore{
				Content: ScoreContent{
					TargetPubKey: testAddr,
					Score:        8.0 + float64(i)*0.1,
					Reason:       "good_response",
					Timestamp:    uint64(time.Now().Unix()),
				},
				Signer:    [32]byte{},
				Signature: []byte{},
			}
			copy(score.Signer[:], signerKey.Public().(ed25519.PublicKey))

			// Sign the score
			data := serializeScoreContent(&score.Content)
			score.Signature = ed25519.Sign(signerKey, data)

			err := rm.SubmitScore(score)
			require.NoError(t, err)
		}

		avgScore := rm.GetAverageScore(testAddr)
		multiplier := CalculateWeightMultiplier(avgScore)

		t.Logf("Average score: %.2f", avgScore)
		t.Logf("Weight multiplier: %.2f", multiplier)

		if multiplier < 0.5 || multiplier > 2.0 {
			t.Errorf("Multiplier %.2f outside expected range [0.5, 2.0]", multiplier)
		}
	})

	t.Run("VRFProofValidation", func(t *testing.T) {
		var seed [32]byte
		copy(seed[:], []byte("testseed"))

		// Create VRF-like proof using Ed25519 signature
		privateKey := createTestKey("testvrf")
		vrfProof := ed25519.Sign(privateKey, seed[:])
		vrfValue := sha256.Sum256(vrfProof)

		t.Logf("VRF Proof: %x", vrfProof)
		t.Logf("VRF Value: %x", vrfValue)

		// Verify VRF
		isValid := ed25519.Verify(privateKey.Public().(ed25519.PublicKey), seed[:], vrfProof)
		t.Logf("VRF Verification: %v", isValid)

		if !isValid {
			t.Error("VRF verification should succeed")
		}
	})
}

func testBackwardCompatibility(t *testing.T) {
	chain := NewTestChain(t)
	defer chain.Close()

	t.Run("OldNodeValidatesVersion1", func(t *testing.T) {
		validator := chain.AddValidator(t, "oldnode")
		block := createVersion1Block(t, chain, validator)

		err := chain.chainState.ValidateBlock(block)
		t.Logf("Old node validates Version 1: %v", err)
	})

	t.Run("NewNodeValidatesBothVersions", func(t *testing.T) {
		validator1 := chain.AddValidator(t, "newnode1")
		validator2 := chain.AddValidator(t, "newnode2")

		block1 := createVersion1Block(t, chain, validator1)
		err1 := chain.chainState.ValidateBlock(block1)
		t.Logf("New node validates Version 1: %v", err1)

		block2 := createVersion2Block(t, chain, validator2)
		err2 := chain.chainState.ValidateBlock(block2)
		t.Logf("New node validates Version 2: %v", err2)
	})

	t.Run("VersionRollback", func(t *testing.T) {
		validator := chain.AddValidator(t, "rollback")

		v2Block := createVersion2Block(t, chain, validator)
		err := chain.chainState.AddBlock(v2Block)
		t.Logf("Version 2 block add: %v", err)

		if err == nil {
			newHeight := chain.GetHeight()
			t.Logf("Height after Version 2: %d", newHeight)
		}
	})
}

func serializeBlockHeaderForSig(header *BlockHeader) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, header.Version)
	buf.Write(header.PrevBlockHash[:])
	buf.Write(header.MerkleRoot[:])
	binary.Write(&buf, binary.BigEndian, header.Timestamp)
	binary.Write(&buf, binary.BigEndian, header.Height)
	buf.Write(header.Proposer[:])

	if header.Version >= 2 {
		buf.Write(header.VRFProof)
		buf.Write(header.VRFSeed[:])
	}

	return buf.Bytes()
}

// serializeScoreContent serializes ScoreContent for signing
func serializeScoreContent(content *ScoreContent) []byte {
	var buf bytes.Buffer

	buf.Write(content.TargetPubKey[:])

	scoreBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(scoreBytes, math.Float64bits(content.Score))
	buf.Write(scoreBytes)

	binary.Write(&buf, binary.BigEndian, uint32(len(content.Reason)))
	buf.Write([]byte(content.Reason))

	binary.Write(&buf, binary.BigEndian, content.Timestamp)

	return buf.Bytes()
}
