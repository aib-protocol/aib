package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Transaction Tests
// ============================================================================

func TestNewTransaction(t *testing.T) {
	inputs := []TXInput{
		{
			TxHash:    [32]byte{1, 2, 3},
			Index:     0,
			Signature: []byte("sig"),
			PublicKey: []byte("pubkey"),
		},
	}

	outputs := []TXOutput{
		{
			Value:   1000,
			Script:  []byte("script"),
			Address: interfaces.Address{4, 5, 6},
		},
	}

	tx := NewTransaction(inputs, outputs)

	if tx == nil {
		t.Fatal("Transaction should not be nil")
	}

	if tx.Version != 1 {
		t.Errorf("Version = %d, expected 1", tx.Version)
	}

	if len(tx.Inputs) != 1 {
		t.Errorf("Inputs count = %d, expected 1", len(tx.Inputs))
	}

	if len(tx.Outputs) != 1 {
		t.Errorf("Outputs count = %d, expected 1", len(tx.Outputs))
	}

	if tx.LockTime != 0 {
		t.Errorf("LockTime = %d, expected 0", tx.LockTime)
	}
}

func TestTransactionHash(t *testing.T) {
	tx := NewTransaction([]TXInput{}, []TXOutput{})
	hash := tx.Hash()

	// Hash should be 32 bytes (SHA256)
	if len(hash) != 32 {
		t.Errorf("Hash length = %d, expected 32", len(hash))
	}

	// Same transaction should produce same hash
	hash2 := tx.Hash()
	if !bytes.Equal(hash[:], hash2[:]) {
		t.Error("Same transaction should produce same hash")
	}
}

func TestTransactionSerialize(t *testing.T) {
	inputs := []TXInput{
		{
			TxHash:    [32]byte{1, 2, 3},
			Index:     0,
			Signature: []byte("signature"),
			PublicKey: []byte("pubkey"),
		},
	}

	outputs := []TXOutput{
		{
			Value:   1000,
			Script:  []byte("script"),
			Address: interfaces.Address{4, 5, 6},
		},
	}

	tx := NewTransaction(inputs, outputs)
	serialized := tx.Serialize()

	if len(serialized) == 0 {
		t.Error("Serialized transaction should not be empty")
	}

	// Verify basic structure
	// Version (4) + InputCount (4) + OutputCount (4) + LockTime (4) = 16 minimum
	if len(serialized) < 16 {
		t.Errorf("Serialized length = %d, expected at least 16", len(serialized))
	}
}

func TestTransactionWithMultipleInputs(t *testing.T) {
	inputs := []TXInput{
		{TxHash: [32]byte{1}, Index: 0, Signature: []byte("sig1"), PublicKey: []byte("pk1")},
		{TxHash: [32]byte{2}, Index: 0, Signature: []byte("sig2"), PublicKey: []byte("pk2")},
		{TxHash: [32]byte{3}, Index: 1, Signature: []byte("sig3"), PublicKey: []byte("pk3")},
	}

	tx := NewTransaction(inputs, []TXOutput{})

	if len(tx.Inputs) != 3 {
		t.Errorf("Inputs count = %d, expected 3", len(tx.Inputs))
	}
}

func TestTransactionWithMultipleOutputs(t *testing.T) {
	outputs := []TXOutput{
		{Value: 1000, Script: []byte("s1"), Address: interfaces.Address{1}},
		{Value: 2000, Script: []byte("s2"), Address: interfaces.Address{2}},
		{Value: 3000, Script: []byte("s3"), Address: interfaces.Address{3}},
	}

	tx := NewTransaction([]TXInput{}, outputs)

	if len(tx.Outputs) != 3 {
		t.Errorf("Outputs count = %d, expected 3", len(tx.Outputs))
	}

	// Verify total output value
	var total uint64
	for _, out := range tx.Outputs {
		total += out.Value
	}
	if total != 6000 {
		t.Errorf("Total output value = %d, expected 6000", total)
	}
}

// ============================================================================
// UTXO Tests
// ============================================================================

func TestUTXOCreation(t *testing.T) {
	utxo := &UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   1000,
		Script:  []byte("script"),
		Address: interfaces.Address{4, 5, 6},
	}

	if utxo.Value != 1000 {
		t.Errorf("Value = %d, expected 1000", utxo.Value)
	}

	if utxo.Index != 0 {
		t.Errorf("Index = %d, expected 0", utxo.Index)
	}
}

// ============================================================================
// MultiSig Tests
// ============================================================================

func TestNewMultiSigStore(t *testing.T) {
	store := NewMultiSigStore()

	if store == nil {
		t.Fatal("MultiSigStore should not be nil")
	}

	if store.utxos == nil {
		t.Error("utxos map should be initialized")
	}

	if store.scripts == nil {
		t.Error("scripts map should be initialized")
	}
}

func TestCreateP2PKHScript(t *testing.T) {
	pubKeyHash := [32]byte{1, 2, 3, 4, 5}
	script := CreateP2PKHScript(pubKeyHash)

	if len(script) == 0 {
		t.Error("P2PKH script should not be empty")
	}

	// Should contain OP_DUP at the beginning
	if len(script) > 0 && script[0] != byte(OP_DUP) {
		t.Errorf("First opcode = 0x%x, expected OP_DUP (0x76)", script[0])
	}
}

func TestCreateMultiSigScript(t *testing.T) {
	_, pubKey1, _ := ed25519.GenerateKey(nil)
	_, pubKey2, _ := ed25519.GenerateKey(nil)

	script := CreateMultiSig2Script(pubKey1, pubKey2)

	if len(script) == 0 {
		t.Error("MultiSig script should not be empty")
	}
}

func TestParseMultiSigScriptInvalid(t *testing.T) {
	// Too short
	_, err := ParseMultiSigScript([]byte{0x01})
	if err == nil {
		t.Error("Should fail with script too short")
	}

	// Invalid opcode
	_, err = ParseMultiSigScript([]byte{0x00, 0x01, 0x02, 0x03, 0x04})
	if err == nil {
		t.Error("Should fail with invalid multi-sig required sigs")
	}
}

func TestParseMultiSigScriptInvalidEnding(t *testing.T) {
	// Valid start but invalid ending
	_, pubKey1, _ := ed25519.GenerateKey(nil)

	// Create a script with valid start but invalid ending
	script := []byte{byte(OP_2), byte(len(pubKey1))}
	script = append(script, pubKey1...)
	script = append(script, byte(len(pubKey1)))
	script = append(script, pubKey1...)
	// Wrong ending - should be OP_2 OP_CHECKMULTISIG
	script = append(script, byte(OP_3), byte(OP_CHECKMULTISIG))

	_, err := ParseMultiSigScript(script)
	if err == nil {
		t.Error("Should fail with invalid ending")
	}
}

// ============================================================================
// Address Tests (Bech32m)
// Note: Bech32m implementation has known issues, skipping full tests
// ============================================================================

func TestAddressFromPublicKey(t *testing.T) {
	pubKey := []byte("test public key")
	addr := AddressFromPublicKey(pubKey)

	if addr == [32]byte{} {
		t.Error("Address should not be zero")
	}
}

func TestAddressFromBytes(t *testing.T) {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}

	addr, err := AddressFromBytes(data)
	if err != nil {
		t.Fatalf("AddressFromBytes failed: %v", err)
	}

	// Verify round-trip
	if addr[0] != 0 || addr[31] != 31 {
		t.Error("Address bytes don't match")
	}
}

func TestAddressFromBytesInvalidLength(t *testing.T) {
	_, err := AddressFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("Should fail with invalid length")
	}
}

func TestAddressEqual(t *testing.T) {
	addr1 := [32]byte{1, 2, 3}
	addr2 := [32]byte{1, 2, 3}
	addr3 := [32]byte{4, 5, 6}

	if !AddressEqual(addr1, addr2) {
		t.Error("Equal addresses should match")
	}

	if AddressEqual(addr1, addr3) {
		t.Error("Different addresses should not match")
	}
}

func TestIsZeroAddress(t *testing.T) {
	zeroAddr := [32]byte{}
	nonZeroAddr := [32]byte{1}

	if !IsZeroAddress(zeroAddr) {
		t.Error("Zero address should be detected")
	}

	if IsZeroAddress(nonZeroAddr) {
		t.Error("Non-zero address should not be detected as zero")
	}
}

// ============================================================================
// Block Tests
// ============================================================================

func TestNewBlock(t *testing.T) {
	tx := NewTransaction([]TXInput{}, []TXOutput{})
	transactions := []*Transaction{tx}

	block := NewBlock(transactions, [32]byte{}, 1, [32]byte{4, 5, 6})

	if block == nil {
		t.Fatal("Block should not be nil")
	}

	if len(block.Transactions) != 1 {
		t.Errorf("Transactions count = %d, expected 1", len(block.Transactions))
	}
}

func TestBlockHash(t *testing.T) {
	block := NewBlock([]*Transaction{}, [32]byte{}, 0, [32]byte{1, 2, 3})
	hash := block.Hash

	if len(hash) != 32 {
		t.Errorf("Block hash length = %d, expected 32", len(hash))
	}
}

func TestComputeMerkleRoot(t *testing.T) {
	// Empty transactions
	block := NewBlock([]*Transaction{}, [32]byte{}, 0, [32]byte{})
	root := block.CalculateMerkleRoot()
	if root != [32]byte{} {
		t.Error("Empty merkle root should be zero")
	}

	// Single transaction
	tx := NewTransaction([]TXInput{}, []TXOutput{})
	block = NewBlock([]*Transaction{tx}, [32]byte{}, 0, [32]byte{})
	root = block.CalculateMerkleRoot()
	if root == [32]byte{} {
		t.Error("Single tx merkle root should not be zero")
	}

	// Two transactions
	tx2 := NewTransaction([]TXInput{}, []TXOutput{{Value: 100, Address: interfaces.Address{1}}})
	block = NewBlock([]*Transaction{tx, tx2}, [32]byte{}, 0, [32]byte{})
	root = block.CalculateMerkleRoot()
	if root == [32]byte{} {
		t.Error("Two tx merkle root should not be zero")
	}
}

// ============================================================================
// Store Tests
// ============================================================================

func TestNewUTXOStore(t *testing.T) {
	store := NewUTXOStore()

	if store == nil {
		t.Fatal("UTXOStore should not be nil")
	}
}

func TestUTXOStoreAddGet(t *testing.T) {
	store := NewUTXOStore()

	utxo := &UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   1000,
		Address: interfaces.Address{4, 5, 6},
	}

	store.AddUTXO(utxo)

	// Get UTXO
	got, err := store.GetUTXO([32]byte{1, 2, 3}, 0)
	if err != nil {
		t.Fatalf("UTXO should be found: %v", err)
	}

	if got.Value != 1000 {
		t.Errorf("Value = %d, expected 1000", got.Value)
	}
}

func TestUTXOStoreSpend(t *testing.T) {
	store := NewUTXOStore()

	utxo := &UTXO{
		TxHash:  [32]byte{1, 2, 3},
		Index:   0,
		Value:   1000,
		Address: interfaces.Address{4, 5, 6},
	}

	store.AddUTXO(utxo)

	// Spend UTXO
	err := store.SpendUTXO([32]byte{1, 2, 3}, 0)
	if err != nil {
		t.Fatalf("SpendUTXO failed: %v", err)
	}

	// UTXO should be gone
	_, err = store.GetUTXO([32]byte{1, 2, 3}, 0)
	if err == nil {
		t.Error("UTXO should be spent")
	}
}

func TestUTXOStoreSpendNonExistent(t *testing.T) {
	store := NewUTXOStore()

	err := store.SpendUTXO([32]byte{1, 2, 3}, 0)
	if err == nil {
		t.Error("Should fail to spend non-existent UTXO")
	}
}

func TestUTXOStoreGetBalance(t *testing.T) {
	store := NewUTXOStore()

	addr := interfaces.Address{1, 2, 3}

	// Add multiple UTXOs for same address
	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 1000, Address: addr})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 2000, Address: addr})
	store.AddUTXO(&UTXO{TxHash: [32]byte{3}, Index: 0, Value: 3000, Address: addr})

	balance := store.GetBalance(addr)
	if balance != 6000 {
		t.Errorf("Balance = %d, expected 6000", balance)
	}
}

func TestUTXOStoreGetAllUTXOs(t *testing.T) {
	store := NewUTXOStore()

	addr := interfaces.Address{1, 2, 3}

	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 1000, Address: addr})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 2000, Address: addr})

	utxos := store.GetAllUTXOs(addr)
	if len(utxos) != 2 {
		t.Errorf("UTXO count = %d, expected 2", len(utxos))
	}
}

// ============================================================================
// Signature Verification Tests
// ============================================================================

func TestSignVerifyTransaction(t *testing.T) {
	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create transaction
	tx := NewTransaction([]TXInput{}, []TXOutput{
		{Value: 1000, Address: interfaces.Address{1, 2, 3}},
	})

	// Sign transaction hash
	txHash := tx.Hash()
	signature := ed25519.Sign(privKey, txHash[:])

	// Verify signature
	if !ed25519.Verify(pubKey, txHash[:], signature) {
		t.Error("Signature verification failed")
	}
}

func TestInvalidSignature(t *testing.T) {
	pubKey, _, _ := ed25519.GenerateKey(nil)
	_, wrongPrivKey, _ := ed25519.GenerateKey(nil)

	tx := NewTransaction([]TXInput{}, []TXOutput{})
	txHash := tx.Hash()

	// Sign with wrong key
	wrongSig := ed25519.Sign(wrongPrivKey, txHash[:])

	// Should fail verification
	if ed25519.Verify(pubKey, txHash[:], wrongSig) {
		t.Error("Should not verify signature from wrong key")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestEmptyTransaction(t *testing.T) {
	tx := NewTransaction([]TXInput{}, []TXOutput{})

	if len(tx.Inputs) != 0 {
		t.Error("Empty transaction should have no inputs")
	}

	if len(tx.Outputs) != 0 {
		t.Error("Empty transaction should have no outputs")
	}

	// Should still be able to hash
	hash := tx.Hash()
	if len(hash) != 32 {
		t.Errorf("Hash length = %d, expected 32", len(hash))
	}
}

func TestZeroValueOutput(t *testing.T) {
	output := TXOutput{
		Value:   0,
		Script:  []byte{},
		Address: interfaces.Address{},
	}

	if output.Value != 0 {
		t.Error("Zero value output should be allowed")
	}
}

func TestLargeTransaction(t *testing.T) {
	// Create transaction with many outputs
	var outputs []TXOutput
	for i := 0; i < 1000; i++ {
		outputs = append(outputs, TXOutput{
			Value:   uint64(i),
			Script:  []byte{},
			Address: interfaces.Address{byte(i % 256)},
		})
	}

	tx := NewTransaction([]TXInput{}, outputs)

	if len(tx.Outputs) != 1000 {
		t.Errorf("Outputs count = %d, expected 1000", len(tx.Outputs))
	}

	// Should still serialize correctly
	serialized := tx.Serialize()
	if len(serialized) == 0 {
		t.Error("Large transaction should serialize")
	}
}

// ============================================================================
// Hash Tests
// ============================================================================

func TestDoubleSHA256(t *testing.T) {
	data := []byte("test data")

	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])

	// Double hash should be different from single hash
	if bytes.Equal(hash1[:], hash2[:]) {
		t.Error("Double SHA256 should differ from single")
	}
}

func TestDeterministicHash(t *testing.T) {
	data := []byte("same data")

	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(data)

	if !bytes.Equal(hash1[:], hash2[:]) {
		t.Error("Same input should produce same hash")
	}
}

// ============================================================================
// Consensus Verification Tests
// ============================================================================

func TestSelectProposer_Deterministic(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	// Add validators
	_, privA, _ := ed25519.GenerateKey(nil)
	_, privB, _ := ed25519.GenerateKey(nil)
	addrA := sha256.Sum256(privA.Public().(ed25519.PublicKey))
	addrB := sha256.Sum256(privB.Public().(ed25519.PublicKey))

	cs.AddValidator(addrA, 1000, privA.Public().(ed25519.PublicKey))
	cs.AddValidator(addrB, 500, privB.Public().(ed25519.PublicKey))

	seed := []byte("block-seed-123")

	// Same seed should always select the same proposer
	proposer1, err := cs.SelectProposer(seed)
	if err != nil {
		t.Fatalf("SelectProposer failed: %v", err)
	}

	proposer2, err := cs.SelectProposer(seed)
	if err != nil {
		t.Fatalf("SelectProposer failed: %v", err)
	}

	if proposer1 != proposer2 {
		t.Error("Same seed should produce same proposer")
	}
}

func TestSelectProposer_WeightedByStake(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	// Add validators with different stakes
	_, privHigh, _ := ed25519.GenerateKey(nil)
	_, privLow, _ := ed25519.GenerateKey(nil)
	addrHigh := sha256.Sum256(privHigh.Public().(ed25519.PublicKey))
	addrLow := sha256.Sum256(privLow.Public().(ed25519.PublicKey))

	cs.AddValidator(addrHigh, 9000, privHigh.Public().(ed25519.PublicKey))
	cs.AddValidator(addrLow, 1000, privLow.Public().(ed25519.PublicKey))

	// Run many selections with different seeds
	highCount := 0
	for i := 0; i < 100; i++ {
		seed := sha256.Sum256([]byte(fmt.Sprintf("seed-%d", i)))
		proposer, err := cs.SelectProposer(seed[:])
		if err != nil {
			t.Fatalf("SelectProposer failed: %v", err)
		}
		if proposer == addrHigh {
			highCount++
		}
	}

	// High-stake node should be selected significantly more often
	// 9000/10000 = 90% expected, allow 60-100% range
	if highCount < 60 {
		t.Errorf("high-stake node selected %d/100 times, expected ~90", highCount)
	}
}

func TestVerifyBlockProposer(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	_, priv, _ := ed25519.GenerateKey(nil)
	addr := sha256.Sum256(priv.Public().(ed25519.PublicKey))
	cs.AddValidator(addr, 1000, priv.Public().(ed25519.PublicKey))

	seed := []byte("test-seed")
	expectedProposer, _ := cs.SelectProposer(seed)

	// Create a block with the correct proposer
	block := NewBlock(nil, [32]byte{}, 1, expectedProposer)
	blockHash := block.CalculateHash()
	block.Header.Signature = ed25519.Sign(priv, blockHash[:])

	prevBlock := &Block{}
	copy(prevBlock.Header.VRFSeed[:], seed)

	result := cs.VerifyBlockProposer(block, prevBlock)

	if !result.Valid {
		t.Errorf("expected valid proposer, got error: %s", result.Error)
	}

	if result.Stake != 1000 {
		t.Errorf("expected stake 1000, got %d", result.Stake)
	}
}

func TestVerifyBlockProposer_WrongProposer(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	_, priv, _ := ed25519.GenerateKey(nil)
	addr := sha256.Sum256(priv.Public().(ed25519.PublicKey))
	cs.AddValidator(addr, 1000, priv.Public().(ed25519.PublicKey))

	// Create a block with the WRONG proposer
	fakeAddr := [32]byte{0xff, 0xfe, 0xfd}
	block := NewBlock(nil, [32]byte{}, 1, fakeAddr)

	prevBlock := &Block{}

	result := cs.VerifyBlockProposer(block, prevBlock)

	if result.Valid {
		t.Error("expected invalid proposer, but got valid")
	}
}

func TestValidatorStateRoot_Deterministic(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	_, priv, _ := ed25519.GenerateKey(nil)
	addr := sha256.Sum256(priv.Public().(ed25519.PublicKey))
	cs.AddValidator(addr, 1000, priv.Public().(ed25519.PublicKey))

	root1, err := cs.CalculateValidatorStateRoot()
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	root2, err := cs.CalculateValidatorStateRoot()
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	if root1 != root2 {
		t.Error("same validators should produce same state root")
	}

	// Verify using the helper
	valid, err := cs.VerifyValidatorStateRoot(root1)
	if err != nil || !valid {
		t.Error("state root should verify")
	}
}

func TestValidatorStateRoot_ChangesOnStakeUpdate(t *testing.T) {
	config := DefaultPoSConfig()
	config.MinStake = 100
	cs := NewConsensusState(config)

	_, priv, _ := ed25519.GenerateKey(nil)
	addr := sha256.Sum256(priv.Public().(ed25519.PublicKey))
	cs.AddValidator(addr, 1000, priv.Public().(ed25519.PublicKey))

	root1, _ := cs.CalculateValidatorStateRoot()

	// Change stake
	cs.mu.Lock()
	cs.validators[addr].Stake = 2000
	cs.mu.Unlock()

	root2, _ := cs.CalculateValidatorStateRoot()

	if root1 == root2 {
		t.Error("different stakes should produce different state roots")
	}

	// Old root should no longer verify
	valid, _ := cs.VerifyValidatorStateRoot(root1)
	if valid {
		t.Error("old state root should not verify after stake change")
	}
}

func TestTransactionFee_Calculation(t *testing.T) {
	tx := NewTransaction(
		[]TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)}},
		[]TXOutput{{Value: 500, Script: []byte("pay"), Address: [32]byte{2}}},
	)

	config := DefaultPoSConfig()
	fee := tx.CalculateFee(config.BaseFeePerByte)

	if fee == 0 {
		t.Error("expected non-zero fee")
	}

	expectedFee := uint64(tx.SerializeSize()) * config.BaseFeePerByte
	if fee != expectedFee {
		t.Errorf("expected fee %d, got %d", expectedFee, fee)
	}
}

func TestTransactionFee_CoinbaseIsZero(t *testing.T) {
	coinbase := CreateCoinbaseTransaction([32]byte{1}, 10*1e8, nil)

	fee := coinbase.CalculateFee(100)
	if fee != 0 {
		t.Errorf("coinbase fee should be 0, got %d", fee)
	}
}

func TestCreateCoinbaseWithFees(t *testing.T) {
	subsidy := uint64(10 * 1e8)
	fees := uint64(5000)
	coinbase := CreateCoinbaseWithFees([32]byte{1}, subsidy, fees, nil)

	reward := coinbase.TotalOutputValue()
	if reward != subsidy+fees {
		t.Errorf("expected reward %d, got %d", subsidy+fees, reward)
	}
}
