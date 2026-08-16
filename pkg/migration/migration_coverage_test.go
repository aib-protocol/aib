// Package migration provides additional unit tests for migration coverage.
package migration

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// AIB1 Migration Tests - Additional Coverage
// ============================================================================

func TestSerializeClaimData(t *testing.T) {
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_32_bytes_abc"))

	data := &ClaimData{
		Address: addr,
		Amount:  1000000,
		Nonce:   42,
	}

	serialized := SerializeClaimData(data)

	// Verify length: 32 (address) + 8 (amount) + 8 (nonce) = 48
	if len(serialized) != 48 {
		t.Errorf("expected serialized length 48, got %d", len(serialized))
	}

	// Verify address bytes
	for i := 0; i < 32; i++ {
		if serialized[i] != addr[i] {
			t.Errorf("address byte mismatch at index %d", i)
		}
	}

	// Verify amount (big-endian)
	amount := binary.BigEndian.Uint64(serialized[32:])
	if amount != 1000000 {
		t.Errorf("expected amount 1000000, got %d", amount)
	}

	// Verify nonce (big-endian)
	nonce := binary.BigEndian.Uint64(serialized[40:])
	if nonce != 42 {
		t.Errorf("expected nonce 42, got %d", nonce)
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	// Generate a key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	data := &ClaimData{
		Address: addr,
		Amount:  1000000,
		Nonce:   1,
	}

	message := SerializeClaimData(data)
	signature := ed25519.Sign(privKey, message)

	if !VerifySignature(pubKey, data, signature) {
		t.Error("valid signature should verify")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	// Generate a key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	data := &ClaimData{
		Address: addr,
		Amount:  1000000,
		Nonce:   1,
	}

	message := SerializeClaimData(data)
	signature := ed25519.Sign(privKey, message)

	// Modify the data
	wrongData := &ClaimData{
		Address: addr,
		Amount:  9999999, // Different amount
		Nonce:   1,
	}

	if VerifySignature(pubKey, wrongData, signature) {
		t.Error("signature with modified data should not verify")
	}
}

func TestVerifySignature_WrongPubKey(t *testing.T) {
	pubKey1, privKey1, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey1[:32])

	data := &ClaimData{
		Address: addr,
		Amount:  1000000,
		Nonce:   1,
	}

	message := SerializeClaimData(data)
	signature := ed25519.Sign(privKey1, message)

	// Use different public key
	if VerifySignature(pubKey2, data, signature) {
		t.Error("signature with wrong pubkey should not verify")
	}
}

func TestVerifySignature_InvalidPubKeyLength(t *testing.T) {
	pubKey := []byte("short")

	data := &ClaimData{
		Amount: 1000000,
		Nonce:  1,
	}

	signature := make([]byte, ed25519.SignatureSize)

	if VerifySignature(pubKey, data, signature) {
		t.Error("invalid pubkey length should return false")
	}
}

func TestVerifySignature_InvalidSignatureLength(t *testing.T) {
	pubKey := make([]byte, ed25519.PublicKeySize)

	data := &ClaimData{
		Amount: 1000000,
		Nonce:  1,
	}

	signature := []byte("short")

	if VerifySignature(pubKey, data, signature) {
		t.Error("invalid signature length should return false")
	}
}

func TestAIB1Migration_LoadSnapshot(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	records := []SnapshotRecord{
		{Address: interfaces.Address{1}, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
		{Address: interfaces.Address{3}, Balance: 3000},
	}

	err := migration.LoadSnapshot(records)
	if err != nil {
		t.Errorf("LoadSnapshot failed: %v", err)
	}

	// Verify balances
	balance1, exists := migration.GetSnapshotBalance(interfaces.Address{1})
	if !exists || balance1 != 1000 {
		t.Errorf("expected balance 1000, got %d, exists=%v", balance1, exists)
	}

	balance2, exists := migration.GetSnapshotBalance(interfaces.Address{2})
	if !exists || balance2 != 2000 {
		t.Errorf("expected balance 2000, got %d, exists=%v", balance2, exists)
	}
}

func TestAIB1Migration_HasClaimedWithAmount(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// Load snapshot
	records := []SnapshotRecord{
		{Address: addr, Balance: 5000},
	}
	migration.LoadSnapshot(records)

	// Before claiming
	claimed, amount := migration.HasClaimedWithAmount(addr)
	if claimed {
		t.Error("should not be claimed yet")
	}
	if amount != 0 {
		t.Errorf("expected amount 0, got %d", amount)
	}

	// Claim with valid signature
	claimData := &ClaimData{
		Address: addr,
		Amount:  5000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 5000, pubKey, signature, 1)
	if err != nil {
		t.Errorf("Claim failed: %v", err)
	}

	// After claiming
	claimed, amount = migration.HasClaimedWithAmount(addr)
	if !claimed {
		t.Error("should be claimed")
	}
	if amount != 5000 {
		t.Errorf("expected amount 5000, got %d", amount)
	}
}

func TestAIB1Migration_GetSnapshotRoot(t *testing.T) {
	root := [32]byte{1, 2, 3, 4}
	cfg := &AIB1Config{
		SnapshotRoot:  root,
		SnapshotTime: time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	gotRoot := migration.GetSnapshotRoot()
	if gotRoot != root {
		t.Errorf("expected root %v, got %v", root, gotRoot)
	}
}

func TestAIB1Migration_GetClaimDeadline(t *testing.T) {
	deadline := time.Now().Add(24 * time.Hour)
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: deadline,
	}

	migration := NewAIB1Migration(cfg)

	gotDeadline := migration.GetClaimDeadline()
	if !gotDeadline.Equal(deadline) {
		t.Errorf("expected deadline %v, got %v", deadline, gotDeadline)
	}
}

func TestAIB1Migration_Claim_Success(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// Load snapshot
	records := []SnapshotRecord{
		{Address: addr, Balance: 1000},
	}
	migration.LoadSnapshot(records)

	// Create valid signature
	claimData := &ClaimData{
		Address: addr,
		Amount:  1000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 1000, pubKey, signature, 1)
	if err != nil {
		t.Errorf("Claim failed: %v", err)
	}

	// Verify claimed
	if !migration.IsClaimed(addr) {
		t.Error("should be claimed")
	}

	if migration.GetTotalMigrated() != 1000 {
		t.Errorf("expected total migrated 1000, got %d", migration.GetTotalMigrated())
	}
}

func TestAIB1Migration_Claim_Expired(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now().Add(-48 * time.Hour),
		ClaimDeadline: time.Now().Add(-24 * time.Hour), // Already expired
	}

	migration := NewAIB1Migration(cfg)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	records := []SnapshotRecord{
		{Address: addr, Balance: 1000},
	}
	migration.LoadSnapshot(records)

	claimData := &ClaimData{
		Address: addr,
		Amount:  1000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 1000, pubKey, signature, 1)
	if err != ErrClaimExpired {
		t.Errorf("expected ErrClaimExpired, got %v", err)
	}
}

func TestAIB1Migration_Claim_AlreadyClaimed(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// Load snapshot
	records := []SnapshotRecord{
		{Address: addr, Balance: 1000},
	}
	migration.LoadSnapshot(records)

	// First claim
	claimData := &ClaimData{
		Address: addr,
		Amount:  1000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 1000, pubKey, signature, 1)
	if err != nil {
		t.Errorf("first Claim failed: %v", err)
	}

	// Second claim should fail
	err = migration.Claim(addr, 1000, pubKey, signature, 2)
	if err != ErrAlreadyClaimed {
		t.Errorf("expected ErrAlreadyClaimed, got %v", err)
	}
}

func TestAIB1Migration_Claim_SnapshotNotFound(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// No snapshot loaded

	claimData := &ClaimData{
		Address: addr,
		Amount:  1000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 1000, pubKey, signature, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestAIB1Migration_Claim_AmountMismatch(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// Load snapshot with different balance
	records := []SnapshotRecord{
		{Address: addr, Balance: 1000},
	}
	migration.LoadSnapshot(records)

	// Claim with wrong amount
	claimData := &ClaimData{
		Address: addr,
		Amount:  2000, // Different from snapshot
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(privKey, message)

	err = migration.Claim(addr, 2000, pubKey, signature, 1)
	if err != ErrAmountExceedsBalance {
		t.Errorf("expected ErrAmountExceedsBalance, got %v", err)
	}
}

func TestAIB1Migration_Claim_InvalidSignature(t *testing.T) {
	cfg := &AIB1Config{
		SnapshotRoot:  [32]byte{1},
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)

	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr := interfaces.Address{}
	copy(addr[:], pubKey[:32])

	// Load snapshot
	records := []SnapshotRecord{
		{Address: addr, Balance: 1000},
	}
	migration.LoadSnapshot(records)

	// Sign with different key
	_, wrongPrivKey, _ := ed25519.GenerateKey(rand.Reader)
	claimData := &ClaimData{
		Address: addr,
		Amount:  1000,
		Nonce:   1,
	}
	message := SerializeClaimData(claimData)
	signature := ed25519.Sign(wrongPrivKey, message)

	err = migration.Claim(addr, 1000, pubKey, signature, 1)
	if err != ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

// ============================================================================
// Merkle Tree Tests
// ============================================================================

func TestNewSnapshotMerkleTree(t *testing.T) {
	records := []SnapshotRecord{
		{Address: interfaces.Address{1}, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
		{Address: interfaces.Address{3}, Balance: 3000},
		{Address: interfaces.Address{4}, Balance: 4000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	// Check root exists
	root := tree.Root()
	if len(root) == 0 {
		t.Error("root should not be empty")
	}
}

func TestSnapshotMerkleTree_Root(t *testing.T) {
	records := []SnapshotRecord{
		{Address: interfaces.Address{1}, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	root := tree.Root()
	if root == nil {
		t.Error("root should not be nil")
	}

	// Calling Root again should return same result
	root2 := tree.Root()
	if string(root) != string(root2) {
		t.Error("Root should be deterministic")
	}
}

func TestSnapshotMerkleTree_GetProof(t *testing.T) {
	addr1 := interfaces.Address{1}
	addr2 := interfaces.Address{2}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
		{Address: addr2, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	// Get proof for address 1
	proof, exists := tree.GetProof(addr1)
	if !exists {
		t.Error("proof should exist for addr1")
	}
	if len(proof) == 0 {
		t.Error("proof should not be empty")
	}

	// Get proof for address 2
	proof2, exists := tree.GetProof(addr2)
	if !exists {
		t.Error("proof should exist for addr2")
	}
	if len(proof2) == 0 {
		t.Error("proof should not be empty")
	}

	// Non-existent address
	_, exists = tree.GetProof(interfaces.Address{9})
	if exists {
		t.Error("proof should not exist for unknown address")
	}
}

func TestSnapshotMerkleTree_GetBalance(t *testing.T) {
	addr1 := interfaces.Address{1}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 5000},
		{Address: interfaces.Address{2}, Balance: 3000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	// Get balance for existing address
	balance, exists := tree.GetBalance(addr1)
	if !exists {
		t.Error("balance should exist for addr1")
	}
	if balance != 5000 {
		t.Errorf("expected balance 5000, got %d", balance)
	}

	// Non-existent address
	_, exists = tree.GetBalance(interfaces.Address{9})
	if exists {
		t.Error("balance should not exist for unknown address")
	}
}

func TestAIB1Migration_VerifyMerkleProof(t *testing.T) {
	// Create a merkle tree
	addr1 := interfaces.Address{1}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	// Get the root
	root := tree.Root()
	var rootArray [32]byte
	copy(rootArray[:], root[:32])

	// Create migration with this root
	cfg := &AIB1Config{
		SnapshotRoot:  rootArray,
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)
	migration.LoadSnapshot(records)

	// Get proof
	proof, exists := tree.GetProof(addr1)
	if !exists {
		t.Fatalf("proof should exist")
	}

	// Verify valid proof
	if !migration.VerifyMerkleProof(addr1, 1000, proof) {
		t.Error("valid proof should verify")
	}

	// Verify invalid proof (wrong balance)
	if migration.VerifyMerkleProof(addr1, 999, proof) {
		t.Error("invalid balance should fail verification")
	}
}

func TestAIB1Migration_ClaimWithMerkle(t *testing.T) {
	// Create a merkle tree
	addr1 := interfaces.Address{1}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	// Get the root
	root := tree.Root()
	var rootArray [32]byte
	copy(rootArray[:], root[:32])

	// Create migration with this root
	cfg := &AIB1Config{
		SnapshotRoot:  rootArray,
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)
	migration.LoadSnapshot(records)

	// Get proof
	proof, exists := tree.GetProof(addr1)
	if !exists {
		t.Fatalf("proof should exist")
	}

	// Claim with Merkle proof
	err = migration.ClaimWithMerkle(addr1, 1000, proof)
	if err != nil {
		t.Errorf("ClaimWithMerkle failed: %v", err)
	}

	// Verify claimed
	if !migration.IsClaimed(addr1) {
		t.Error("should be claimed")
	}
}

func TestAIB1Migration_ClaimWithMerkle_Expired(t *testing.T) {
	// Create a merkle tree
	addr1 := interfaces.Address{1}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	root := tree.Root()
	var rootArray [32]byte
	copy(rootArray[:], root[:32])

	// Create migration with expired deadline
	cfg := &AIB1Config{
		SnapshotRoot:  rootArray,
		SnapshotTime:  time.Now().Add(-48 * time.Hour),
		ClaimDeadline: time.Now().Add(-24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)
	migration.LoadSnapshot(records)

	proof, _ := tree.GetProof(addr1)

	err = migration.ClaimWithMerkle(addr1, 1000, proof)
	if err != ErrClaimExpired {
		t.Errorf("expected ErrClaimExpired, got %v", err)
	}
}

func TestAIB1Migration_ClaimWithMerkle_AlreadyClaimed(t *testing.T) {
	addr1 := interfaces.Address{1}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
		{Address: interfaces.Address{2}, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	root := tree.Root()
	var rootArray [32]byte
	copy(rootArray[:], root[:32])

	cfg := &AIB1Config{
		SnapshotRoot:  rootArray,
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)
	migration.LoadSnapshot(records)

	proof, _ := tree.GetProof(addr1)

	// First claim
	err = migration.ClaimWithMerkle(addr1, 1000, proof)
	if err != nil {
		t.Errorf("first ClaimWithMerkle failed: %v", err)
	}

	// Second claim should fail
	err = migration.ClaimWithMerkle(addr1, 1000, proof)
	if err != ErrAlreadyClaimed {
		t.Errorf("expected ErrAlreadyClaimed, got %v", err)
	}
}

func TestAIB1Migration_ClaimWithMerkle_InvalidProof(t *testing.T) {
	addr1 := interfaces.Address{1}
	addr2 := interfaces.Address{2}

	records := []SnapshotRecord{
		{Address: addr1, Balance: 1000},
		{Address: addr2, Balance: 2000},
	}

	tree, err := NewSnapshotMerkleTree(records)
	if err != nil {
		t.Fatalf("NewSnapshotMerkleTree failed: %v", err)
	}

	root := tree.Root()
	var rootArray [32]byte
	copy(rootArray[:], root[:32])

	cfg := &AIB1Config{
		SnapshotRoot:  rootArray,
		SnapshotTime:  time.Now(),
		ClaimDeadline: time.Now().Add(24 * time.Hour),
	}

	migration := NewAIB1Migration(cfg)
	migration.LoadSnapshot(records)

	// Use proof from addr2 for addr1
	proof, _ := tree.GetProof(addr2)

	err = migration.ClaimWithMerkle(addr1, 1000, proof)
	if err != ErrInvalidProof {
		t.Errorf("expected ErrInvalidProof, got %v", err)
	}
}

// ============================================================================
// Cross-Chain Migration Tests - Additional Coverage
// ============================================================================

func TestNewCrossChainMigration(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10, 8, 5},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, err := NewCrossChainMigration(cfg)
	if err != nil {
		t.Fatalf("NewCrossChainMigration failed: %v", err)
	}

	if migration == nil {
		t.Fatal("migration should not be nil")
	}

	if migration.chain != ChainBTC {
		t.Errorf("expected chain BTC, got %s", migration.chain)
	}
}

func TestNewCrossChainMigration_NoRates(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	_, err := NewCrossChainMigration(cfg)
	if err == nil {
		t.Error("should error with no rates")
	}
}

func TestNewCrossChainMigration_TGETooHigh(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     101, // Invalid
		VestingMonths:  3,
	}

	_, err := NewCrossChainMigration(cfg)
	if err == nil {
		t.Error("should error with TGE > 100")
	}
}

func TestNewCrossChainMigration_WindowEndBeforeStart(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now().Add(30 * 24 * time.Hour),
		WindowEnd:      time.Now(), // Before start
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	_, err := NewCrossChainMigration(cfg)
	if err == nil {
		t.Error("should error with window end before start")
	}
}

func TestNewCrossChainMigration_WindowEndEqualsStart(t *testing.T) {
	sameTime := time.Now()
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    sameTime,
		WindowEnd:      sameTime, // Equal
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	_, err := NewCrossChainMigration(cfg)
	if err == nil {
		t.Error("should error with window end equals start")
	}
}

func TestCrossChainMigration_GetCurrentRate(t *testing.T) {
	windowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10, 8, 5}, // Month 1: 10, Month 2: 8, Month 3: 5
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, err := NewCrossChainMigration(cfg)
	if err != nil {
		t.Fatalf("NewCrossChainMigration failed: %v", err)
	}

	// Test rate at different times
	// January (month 1) - rate should be 10
	janTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rate := migration.GetCurrentRate(janTime)
	if rate != 10 {
		t.Errorf("expected rate 10 in January, got %d", rate)
	}

	// February (month 2) - rate should be 8
	febTime := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	rate = migration.GetCurrentRate(febTime)
	if rate != 8 {
		t.Errorf("expected rate 8 in February, got %d", rate)
	}

	// March (month 3) - rate should be 5
	marTime := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	rate = migration.GetCurrentRate(marTime)
	if rate != 5 {
		t.Errorf("expected rate 5 in March, got %d", rate)
	}
}

func TestCrossChainMigration_GetCurrentRate_BeforeWindow(t *testing.T) {
	windowStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10, 8, 5},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	// Before window
	beforeTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rate := migration.GetCurrentRate(beforeTime)
	if rate != 0 {
		t.Errorf("expected rate 0 before window, got %d", rate)
	}
}

func TestCrossChainMigration_GetCurrentRate_AfterWindow(t *testing.T) {
	windowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10, 8, 5},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	// After window (at window end)
	rate := migration.GetCurrentRate(windowEnd)
	if rate != 0 {
		t.Errorf("expected rate 0 at window end, got %d", rate)
	}

	// Well after window
	afterTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rate = migration.GetCurrentRate(afterTime)
	if rate != 0 {
		t.Errorf("expected rate 0 after window, got %d", rate)
	}
}

func TestCrossChainMigration_buildVestingSchedule(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10, 8, 5},
		TGEPercent:     20,
		VestingMonths:  4,
	}

	migration, _ := NewCrossChainMigration(cfg)

	tgeTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vesting := migration.buildVestingSchedule(tgeTime)

	// Should have TGE + vestingMonths entries
	expectedLen := cfg.VestingMonths + 1
	if len(vesting) != int(expectedLen) {
		t.Errorf("expected %d vesting entries, got %d", expectedLen, len(vesting))
	}

	// Check TGE entry
	if vesting[0].UnlockTime != tgeTime {
		t.Error("TGE entry should have correct unlock time")
	}
	if vesting[0].Percent != 20 {
		t.Errorf("expected TGE percent 20, got %d", vesting[0].Percent)
	}

	// Check remaining entries
	var totalPct uint64
	for i := 1; i < len(vesting); i++ {
		totalPct += vesting[i].Percent
	}

	if totalPct != 80 {
		t.Errorf("expected remaining 80%%, got %d%%", totalPct)
	}
}

func TestCrossChainMigration_buildVestingSchedule_ZeroVestingMonths(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     50,
		VestingMonths:  0, // All at TGE
	}

	migration, _ := NewCrossChainMigration(cfg)

	tgeTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vesting := migration.buildVestingSchedule(tgeTime)

	// When vestingMonths is 0, all should be unlocked at TGE
	if len(vesting) != 1 {
		t.Errorf("expected 1 vesting entry, got %d", len(vesting))
	}

	// Should be 100% at TGE
	if vesting[0].Percent != 100 {
		t.Errorf("expected 100%% at TGE, got %d%%", vesting[0].Percent)
	}
}

func TestMinConfirmations(t *testing.T) {
	tests := []struct {
		chain     ChainType
		expected  uint64
	}{
		{ChainBTC, 6},
		{ChainETH, 12},
		{ChainSOL, 32},
		{ChainType("UNKNOWN"), 0},
	}

	for _, tt := range tests {
		result := MinConfirmations(tt.chain)
		if result != tt.expected {
			t.Errorf("expected MinConfirmations(%s) = %d, got %d", tt.chain, tt.expected, result)
		}
	}
}

func TestVerifyCrossChainProof(t *testing.T) {
	// Generate relayer keys
	relayerPubKey1, relayerPrivKey1, _ := ed25519.GenerateKey(rand.Reader)
	relayerPubKey2, relayerPrivKey2, _ := ed25519.GenerateKey(rand.Reader)

	proof := &CrossChainProof{
		Chain:         ChainBTC,
		SourceTxID:    [32]byte{1, 2, 3},
		SourceAddress: []byte("bc1q..."),
		Amount:        1000000,
		BlockHeight:   800000,
		Confirmations: 10, // More than min (6)
		RelayerSigs:    [][]byte{},
		RelayerPubKeys: [][]byte{},
	}

	// No signatures should fail
	err := VerifyCrossChainProof(proof, 1)
	if err == nil {
		t.Error("should error with no signatures")
	}

	// Add one valid signature
	msg := SerializeClaimData(&ClaimData{
		Amount: proof.Amount,
		Nonce:  proof.BlockHeight,
	})
	sig1 := ed25519.Sign(relayerPrivKey1, msg)
	proof.RelayerSigs = append(proof.RelayerSigs, sig1)
	proof.RelayerPubKeys = append(proof.RelayerPubKeys, []byte(relayerPubKey1))

	err = VerifyCrossChainProof(proof, 1)
	if err != nil {
		t.Errorf("should pass with 1 valid signature: %v", err)
	}

	// Require 2 signatures but only have 1
	err = VerifyCrossChainProof(proof, 2)
	if err == nil {
		t.Error("should error requiring 2 signatures but only 1")
	}

	// Add second valid signature
	sig2 := ed25519.Sign(relayerPrivKey2, msg)
	proof.RelayerSigs = append(proof.RelayerSigs, sig2)
	proof.RelayerPubKeys = append(proof.RelayerPubKeys, []byte(relayerPubKey2))

	err = VerifyCrossChainProof(proof, 2)
	if err != nil {
		t.Errorf("should pass with 2 valid signatures: %v", err)
	}
}

func TestVerifyCrossChainProof_InsufficientConfirmations(t *testing.T) {
	proof := &CrossChainProof{
		Chain:         ChainBTC,
		Confirmations: 3, // Less than min (6)
	}

	err := VerifyCrossChainProof(proof, 0)
	if err == nil {
		t.Error("should error with insufficient confirmations")
	}
}

func TestVerifyCrossChainProof_InvalidChain(t *testing.T) {
	proof := &CrossChainProof{
		Chain:         ChainType("INVALID"),
		Confirmations: 10,
	}

	err := VerifyCrossChainProof(proof, 0)
	if err != ErrInvalidChain {
		t.Errorf("expected ErrInvalidChain, got %v", err)
	}
}

func TestVerifyCrossChainProof_NilProof(t *testing.T) {
	err := VerifyCrossChainProof(nil, 0)
	if err == nil {
		t.Error("should error with nil proof")
	}
}

func TestHashCrossChainProof(t *testing.T) {
	proof := &CrossChainProof{
		Chain:         ChainBTC,
		SourceTxID:    [32]byte{1, 2, 3},
		SourceAddress: []byte("test"),
		Amount:        1000,
		BlockHeight:   100,
	}

	hash1 := hashCrossChainProof(proof)
	if len(hash1) != 32 {
		t.Errorf("expected hash length 32, got %d", len(hash1))
	}

	// Same input should produce same hash
	hash2 := hashCrossChainProof(proof)
	if hash1 != hash2 {
		t.Error("hash should be deterministic")
	}

	// Different input should produce different hash
	proof.Amount = 2000
	hash3 := hashCrossChainProof(proof)
	if hash1 == hash3 {
		t.Error("different input should produce different hash")
	}
}

func TestCrossChainMigration_Migrate(t *testing.T) {
	windowStart := time.Now()
	windowEnd := time.Now().Add(30 * 24 * time.Hour)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, err := NewCrossChainMigration(cfg)
	if err != nil {
		t.Fatalf("NewCrossChainMigration failed: %v", err)
	}

	// Generate relayer keys
	relayerPubKey, relayerPrivKey, _ := ed25519.GenerateKey(rand.Reader)

	proof := &CrossChainProof{
		Chain:         ChainBTC,
		SourceTxID:    [32]byte{1, 2, 3},
		SourceAddress: []byte("bc1q..."),
		Amount:        1000000,
		BlockHeight:   800000,
		Confirmations: 10,
	}

	// Sign the proof
	msg := SerializeClaimData(&ClaimData{
		Amount: proof.Amount,
		Nonce:  proof.BlockHeight,
	})
	sig := ed25519.Sign(relayerPrivKey, msg)
	proof.RelayerSigs = append(proof.RelayerSigs, sig)
	proof.RelayerPubKeys = append(proof.RelayerPubKeys, []byte(relayerPubKey))

	userAddr := interfaces.Address{1}

	reward, err := migration.Migrate(userAddr, proof, 1, windowStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if reward == nil {
		t.Fatal("reward should not be nil")
	}

	// Check reward calculation: 1000000 * 10 = 10000000
	if reward.TotalReward != 10000000 {
		t.Errorf("expected total reward 10000000, got %d", reward.TotalReward)
	}

	// Check vesting schedule
	if len(reward.Vesting) == 0 {
		t.Error("vesting schedule should not be empty")
	}

	// Check totals
	if migration.GetTotalMigrated() != 1000000 {
		t.Errorf("expected total migrated 1000000, got %d", migration.GetTotalMigrated())
	}

	if migration.GetTotalRewards() != 10000000 {
		t.Errorf("expected total rewards 10000000, got %d", migration.GetTotalRewards())
	}
}

func TestCrossChainMigration_Migrate_WindowClosed(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now().Add(-60 * 24 * time.Hour),
		WindowEnd:      time.Now().Add(-30 * 24 * time.Hour), // Already closed
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	proof := &CrossChainProof{
		Chain:        ChainBTC,
		Confirmations: 10,
	}

	userAddr := interfaces.Address{1}

	_, err := migration.Migrate(userAddr, proof, 0, time.Now())
	if err != ErrMigrationWindowClosed {
		t.Errorf("expected ErrMigrationWindowClosed, got %v", err)
	}
}

func TestCrossChainMigration_Migrate_WrongChain(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	// Proof for ETH but migration is for BTC
	proof := &CrossChainProof{
		Chain:         ChainETH,
		Confirmations: 20,
	}

	userAddr := interfaces.Address{1}

	_, err := migration.Migrate(userAddr, proof, 0, time.Now())
	if err == nil {
		t.Error("should error with wrong chain")
	}
}

func TestCrossChainMigration_Migrate_DuplicateTx(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	// Generate relayer keys
	relayerPubKey, relayerPrivKey, _ := ed25519.GenerateKey(rand.Reader)

	txID := [32]byte{1, 2, 3}
	proof := &CrossChainProof{
		Chain:         ChainBTC,
		SourceTxID:    txID,
		Amount:        1000000,
		BlockHeight:   800000,
		Confirmations: 10,
	}

	msg := SerializeClaimData(&ClaimData{
		Amount: proof.Amount,
		Nonce:  proof.BlockHeight,
	})
	sig := ed25519.Sign(relayerPrivKey, msg)
	proof.RelayerSigs = append(proof.RelayerSigs, sig)
	proof.RelayerPubKeys = append(proof.RelayerPubKeys, []byte(relayerPubKey))

	userAddr := interfaces.Address{1}

	// First migration should succeed
	_, err := migration.Migrate(userAddr, proof, 1, time.Now())
	if err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}

	// Second migration with same tx should fail
	_, err = migration.Migrate(userAddr, proof, 1, time.Now())
	if err == nil {
		t.Error("should error with duplicate transaction")
	}
}

func TestCrossChainMigration_GetLockedRewards(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	userAddr := interfaces.Address{1}

	// No rewards yet
	rewards := migration.GetLockedRewards(userAddr)
	if rewards != nil {
		t.Error("should return nil for user with no rewards")
	}
}

func TestCrossChainMigration_ClaimUnlocked(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	// Generate relayer keys
	relayerPubKey, relayerPrivKey, _ := ed25519.GenerateKey(rand.Reader)

	proof := &CrossChainProof{
		Chain:         ChainBTC,
		SourceTxID:    [32]byte{1},
		Amount:        1000000, // 10x multiplier = 10000000
		BlockHeight:   800000,
		Confirmations: 10,
	}

	msg := SerializeClaimData(&ClaimData{
		Amount: proof.Amount,
		Nonce:  proof.BlockHeight,
	})
	sig := ed25519.Sign(relayerPrivKey, msg)
	proof.RelayerSigs = append(proof.RelayerSigs, sig)
	proof.RelayerPubKeys = append(proof.RelayerPubKeys, []byte(relayerPubKey))

	userAddr := interfaces.Address{1}

	// Migrate
	_, err := migration.Migrate(userAddr, proof, 1, time.Now())
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Claim at TGE time (should get 20% = 2000000)
	claimTime := time.Now()
	claimed, err := migration.ClaimUnlocked(userAddr, claimTime)
	if err != nil {
		t.Fatalf("ClaimUnlocked failed: %v", err)
	}

	// 20% of 10000000 = 2000000
	if claimed != 2000000 {
		t.Errorf("expected claimed 2000000, got %d", claimed)
	}

	// Try to claim again - nothing new should be unlocked
	_, err = migration.ClaimUnlocked(userAddr, claimTime)
	if err != ErrNothingToClaim {
		t.Errorf("expected ErrNothingToClaim, got %v", err)
	}
}

func TestCrossChainMigration_ClaimUnlocked_NoRewards(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	userAddr := interfaces.Address{1}

	_, err := migration.ClaimUnlocked(userAddr, time.Now())
	if err != ErrNoLockedRewards {
		t.Errorf("expected ErrNoLockedRewards, got %v", err)
	}
}

func TestCrossChainMigration_ChainType(t *testing.T) {
	cfg := &CrossChainConfig{
		Chain:          ChainETH,
		WindowStart:    time.Now(),
		WindowEnd:      time.Now().Add(30 * 24 * time.Hour),
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	if migration.ChainType() != ChainETH {
		t.Errorf("expected ChainType ETH, got %s", migration.ChainType())
	}
}

func TestCrossChainMigration_WindowStart(t *testing.T) {
	windowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	if !migration.WindowStart().Equal(windowStart) {
		t.Errorf("expected WindowStart %v, got %v", windowStart, migration.WindowStart())
	}
}

func TestCrossChainMigration_WindowEnd(t *testing.T) {
	windowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	cfg := &CrossChainConfig{
		Chain:          ChainBTC,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		IncentiveRates: []uint64{10},
		TGEPercent:     20,
		VestingMonths:  3,
	}

	migration, _ := NewCrossChainMigration(cfg)

	if !migration.WindowEnd().Equal(windowEnd) {
		t.Errorf("expected WindowEnd %v, got %v", windowEnd, migration.WindowEnd())
	}
}

// ============================================================================
// LockedReward Methods Tests
// ============================================================================

func TestLockedReward_Claimable(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     0,
		Vesting: []VestingEntry{
			{UnlockTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Percent: 20},  // 2000
			{UnlockTime: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Percent: 30},  // 3000
			{UnlockTime: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Percent: 50},  // 5000
		},
	}

	// At January 15: 20% should be claimable
	claimable := reward.Claimable(now)
	if claimable != 2000 {
		t.Errorf("expected claimable 2000, got %d", claimable)
	}

	// At February 15: 20% + 30% = 50% should be claimable
	febTime := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	claimable = reward.Claimable(febTime)
	if claimable != 5000 {
		t.Errorf("expected claimable 5000, got %d", claimable)
	}

	// At March 15: all 100% should be claimable
	marTime := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	claimable = reward.Claimable(marTime)
	if claimable != 10000 {
		t.Errorf("expected claimable 10000, got %d", claimable)
	}
}

func TestLockedReward_Claimable_AlreadyClaimed(t *testing.T) {
	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     5000, // Already claimed 5000
		Vesting: []VestingEntry{
			{UnlockTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Percent: 100},
		},
	}

	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	claimable := reward.Claimable(now)

	// Total 10000, claimed 5000, should be able to claim 5000 more
	if claimable != 5000 {
		t.Errorf("expected claimable 5000, got %d", claimable)
	}
}

func TestLockedReward_Claimable_AllClaimed(t *testing.T) {
	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     10000, // All claimed
		Vesting: []VestingEntry{
			{UnlockTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Percent: 100},
		},
	}

	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	claimable := reward.Claimable(now)

	if claimable != 0 {
		t.Errorf("expected claimable 0 when all claimed, got %d", claimable)
	}
}

func TestLockedReward_RemainingLocked(t *testing.T) {
	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     2000,
	}

	remaining := reward.RemainingLocked()
	if remaining != 8000 {
		t.Errorf("expected remaining 8000, got %d", remaining)
	}
}

func TestLockedReward_RemainingLocked_AllClaimed(t *testing.T) {
	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     10000,
	}

	remaining := reward.RemainingLocked()
	if remaining != 0 {
		t.Errorf("expected remaining 0, got %d", remaining)
	}
}

func TestLockedReward_RemainingLocked_MoreClaimedThanTotal(t *testing.T) {
	reward := &LockedReward{
		TotalReward: 10000,
		Claimed:     15000, // More than total
	}

	remaining := reward.RemainingLocked()
	if remaining != 0 {
		t.Errorf("expected remaining 0, got %d", remaining)
	}
}
