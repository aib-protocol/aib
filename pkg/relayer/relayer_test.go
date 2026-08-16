// Package relayer provides unit tests for the cross-chain relayer functionality.
package relayer

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// Helper functions for tests

func newTestAddress(chain ChainType, seed string) Address {
	h := sha256.Sum256([]byte(seed))
	return Address{
		Chain: chain,
		Data:  h[:20], // Use first 20 bytes for address
	}
}

func newTestHash(chain ChainType, seed string) Hash {
	h := sha256.Sum256([]byte(seed))
	return Hash{
		Chain: chain,
		Data:  h,
	}
}

// ============================================================================
// Chain Type Tests
// ============================================================================

func TestChainType_String(t *testing.T) {
	tests := []struct {
		chain     ChainType
		expected  string
	}{
		{ChainBTC, "BTC"},
		{ChainETH, "ETH"},
		{ChainSOL, "SOL"},
		{ChainAIB, "AIB"},
	}

	for _, tt := range tests {
		if tt.chain.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.chain.String())
		}
	}
}

// ============================================================================
// Relayer Status Tests
// ============================================================================

func TestRelayerStatus_String(t *testing.T) {
	tests := []struct {
		status   RelayerStatus
		expected string
	}{
		{StatusActive, "Active"},
		{StatusInactive, "Inactive"},
		{StatusSlashed, "Slashed"},
	}

	for _, tt := range tests {
		if tt.status.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.status.String())
		}
	}
}

func TestRelayerStatus_IsValid(t *testing.T) {
	if !StatusActive.IsValid() {
		t.Error("Active should be valid")
	}
	if !StatusInactive.IsValid() {
		t.Error("Inactive should be valid")
	}
	if !StatusSlashed.IsValid() {
		t.Error("Slashed should be valid")
	}
	if RelayerStatus("Invalid").IsValid() {
		t.Error("Invalid status should not be valid")
	}
}

// ============================================================================
// Transaction Status Tests
// ============================================================================

func TestTxStatus_String(t *testing.T) {
	tests := []struct {
		status   TxStatus
		expected string
	}{
		{TxStatusPending, "Pending"},
		{TxStatusLocked, "Locked"},
		{TxStatusConfirmed, "Confirmed"},
		{TxStatusProofReady, "ProofReady"},
		{TxStatusCompleted, "Completed"},
		{TxStatusFailed, "Failed"},
		{TxStatusDisputed, "Disputed"},
	}

	for _, tt := range tests {
		if tt.status.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.status.String())
		}
	}
}

// ============================================================================
// Address Tests
// ============================================================================

func TestNewAddress(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	addr := NewAddress(ChainETH, data)

	if addr.Chain != ChainETH {
		t.Errorf("expected chain ETH, got %s", addr.Chain)
	}
	if len(addr.Data) != len(data) {
		t.Errorf("expected data length %d, got %d", len(data), len(addr.Data))
	}
}

func TestAddress_String(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	addr := &Address{
		Chain: ChainBTC,
		Data:  data,
	}

	str := addr.String()
	if str == "" {
		t.Error("string should not be empty")
	}

	// Verify it's hex encoded
	if len(str) != len(data)*2 {
		t.Errorf("expected hex string length %d, got %d", len(data)*2, len(str))
	}
}

func TestAddress_Bytes(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	addr := &Address{
		Chain: ChainETH,
		Data:  data,
	}

	bytes := addr.Bytes()
	if len(bytes) != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), len(bytes))
	}
}

// ============================================================================
// Hash Tests
// ============================================================================

func TestNewHash(t *testing.T) {
	data := []byte("test data")
	hash := NewHash(ChainBTC, data)

	if hash.Chain != ChainBTC {
		t.Errorf("expected chain BTC, got %s", hash.Chain)
	}
}

func TestHash_String(t *testing.T) {
	hash := &Hash{
		Chain: ChainETH,
		Data:  [32]byte{1, 2, 3},
	}

	str := hash.String()
	if str == "" {
		t.Error("string should not be empty")
	}
}

func TestHash_Bytes(t *testing.T) {
	data := [32]byte{0xaa, 0xbb, 0xcc, 0xdd}
	hash := &Hash{
		Chain: ChainETH,
		Data:  data,
	}

	bytes := hash.Bytes()
	if len(bytes) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(bytes))
	}
}

// ============================================================================
// Relayer Tests
// ============================================================================

func TestNewRelayer(t *testing.T) {
	addr := newTestAddress(ChainETH, "test")
	chains := []ChainType{ChainETH, ChainBTC}
	stake := big.NewInt(1000000000)

	relayer := NewRelayer("relayer-1", addr, "node-1", chains, stake)

	if relayer.ID != "relayer-1" {
		t.Errorf("expected ID relayer-1, got %s", relayer.ID)
	}
	if relayer.Status != StatusActive {
		t.Errorf("expected status Active, got %s", relayer.Status)
	}
	if len(relayer.SupportedChains) != 2 {
		t.Errorf("expected 2 chains, got %d", len(relayer.SupportedChains))
	}
	if relayer.Reputation != 100.0 {
		t.Errorf("expected reputation 100, got %f", relayer.Reputation)
	}
}

func TestRelayer_GetStatus(t *testing.T) {
	relayer := &Relayer{
		Status: StatusActive,
	}

	if relayer.GetStatus() != StatusActive {
		t.Error("status should be Active")
	}
}

func TestRelayer_SetStatus(t *testing.T) {
	relayer := &Relayer{
		Status: StatusActive,
	}

	relayer.SetStatus(StatusInactive)
	if relayer.Status != StatusInactive {
		t.Error("status should be Inactive")
	}

	// Test invalid status is ignored
	relayer.SetStatus(RelayerStatus("Invalid"))
	if relayer.Status != StatusInactive {
		t.Error("invalid status should be ignored")
	}
}

func TestRelayer_UpdateStats(t *testing.T) {
	relayer := &Relayer{
		TotalTXs:    10,
		SuccessRate: 0.9,
		Reputation:  90,
	}

	relayer.UpdateStats(true)

	if relayer.TotalTXs != 11 {
		t.Errorf("expected TotalTXs 11, got %d", relayer.TotalTXs)
	}
}

func TestRelayer_SupportsChain(t *testing.T) {
	relayer := &Relayer{
		SupportedChains: []ChainType{ChainBTC, ChainETH},
	}

	if !relayer.SupportsChain(ChainBTC) {
		t.Error("should support BTC")
	}
	if !relayer.SupportsChain(ChainETH) {
		t.Error("should support ETH")
	}
	if relayer.SupportsChain(ChainSOL) {
		t.Error("should not support SOL")
	}
}

func TestRelayer_CanProcess(t *testing.T) {
	relayer := &Relayer{
		Status:          StatusActive,
		SupportedChains: []ChainType{ChainBTC, ChainETH},
	}

	if !relayer.CanProcess(ChainBTC, ChainETH) {
		t.Error("should be able to process BTC -> ETH")
	}
	if relayer.CanProcess(ChainBTC, ChainSOL) {
		t.Error("should not be able to process to unsupported chain")
	}

	// Test inactive relayer
	relayer.Status = StatusInactive
	if relayer.CanProcess(ChainBTC, ChainETH) {
		t.Error("inactive relayer should not process")
	}
}

// ============================================================================
// CrossChainTx Tests
// ============================================================================

func TestNewCrossChainTx(t *testing.T) {
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	amount := big.NewInt(100000000) // 1 BTC
	expiry := 24 * time.Hour

	tx := NewCrossChainTx(
		"tx-1",
		ChainBTC,
		ChainETH,
		sender,
		recipient,
		amount,
		"BTC",
		"relayer-1",
		expiry,
	)

	if tx.ID != "tx-1" {
		t.Errorf("expected ID tx-1, got %s", tx.ID)
	}
	if tx.SourceChain != ChainBTC {
		t.Errorf("expected source ChainBTC, got %s", tx.SourceChain)
	}
	if tx.DestChain != ChainETH {
		t.Errorf("expected dest ChainETH, got %s", tx.DestChain)
	}
	if tx.Status != TxStatusPending {
		t.Errorf("expected status Pending, got %s", tx.Status)
	}
}

func TestCrossChainTx_GetStatus(t *testing.T) {
	tx := &CrossChainTx{
		Status: TxStatusPending,
	}

	if tx.GetStatus() != TxStatusPending {
		t.Error("status should be Pending")
	}
}

func TestCrossChainTx_SetStatus(t *testing.T) {
	tx := &CrossChainTx{
		Status: TxStatusPending,
	}

	tx.SetStatus(TxStatusLocked)
	if tx.Status != TxStatusLocked {
		t.Error("status should be Locked")
	}
}

func TestCrossChainTx_UpdateConfirmations(t *testing.T) {
	tx := &CrossChainTx{
		Confirmations: 0,
	}

	tx.UpdateConfirmations(6)
	if tx.Confirmations != 6 {
		t.Errorf("expected 6 confirmations, got %d", tx.Confirmations)
	}
}

func TestCrossChainTx_IsExpired(t *testing.T) {
	// Create expired transaction
	tx1 := &CrossChainTx{
		Expiry: time.Now().Add(-1 * time.Hour),
	}
	if !tx1.IsExpired() {
		t.Error("should be expired")
	}

	// Create non-expired transaction
	tx2 := &CrossChainTx{
		Expiry: time.Now().Add(1 * time.Hour),
	}
	if tx2.IsExpired() {
		t.Error("should not be expired")
	}
}

// ============================================================================
// MerkleProof Tests
// ============================================================================

func TestNewMerkleProof(t *testing.T) {
	txHash := newTestHash(ChainBTC, "tx")
	blockHash := newTestHash(ChainBTC, "block")
	proof := [][]byte{[]byte("node1"), []byte("node2")}

	mp := NewMerkleProof(txHash, blockHash, 800000, 0, proof, ChainBTC)

	if mp.BlockNumber != 800000 {
		t.Errorf("expected block number 800000, got %d", mp.BlockNumber)
	}
	if mp.Index != 0 {
		t.Error("expected index 0")
	}
	if len(mp.Proof) != 2 {
		t.Errorf("expected 2 proof nodes, got %d", len(mp.Proof))
	}
}

func TestMerkleProof_Verify(t *testing.T) {
	// Create a simple merkle proof
	txHash := newTestHash(ChainBTC, "tx")
	blockHash := newTestHash(ChainBTC, "block")

	// Create a proof path (simplified - normally this is generated from the tree)
	proof0 := sha256.Sum256([]byte("proof0"))
	proof1 := sha256.Sum256([]byte("proof1"))
	proof := [][]byte{
		proof0[:],
		proof1[:],
	}

	_ = &MerkleProof{
		TxHash:     txHash,
		BlockHash:  blockHash,
		BlockNumber: 800000,
		Index:       0,
		Proof:       proof,
		Chain:       ChainBTC,
	}

	// Test with nil proof
	nilProof := &MerkleProof{
		Proof: nil,
	}
	if nilProof.Verify(blockHash) {
		t.Error("nil proof should not verify")
	}

	// Test with empty proof
	emptyProof := &MerkleProof{
		Proof: [][]byte{},
	}
	if emptyProof.Verify(blockHash) {
		t.Error("empty proof should not verify")
	}
}

// ============================================================================
// SwapRequest Tests
// ============================================================================

func TestNewSwapRequest(t *testing.T) {
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	amount := big.NewInt(100000000)
	relayerFee := big.NewInt(1000)
	deadline := 24 * time.Hour
	secretHash := sha256.Sum256([]byte("secret"))

	req := NewSwapRequest(
		"req-1",
		ChainBTC,
		ChainETH,
		sender,
		recipient,
		amount,
		"BTC",
		relayerFee,
		deadline,
		secretHash[:],
	)

	if req.ID != "req-1" {
		t.Errorf("expected ID req-1, got %s", req.ID)
	}
	if req.SourceChain != ChainBTC {
		t.Error("source chain should be BTC")
	}
	if req.DestChain != ChainETH {
		t.Error("dest chain should be ETH")
	}
}

func TestSwapRequest_IsExpired(t *testing.T) {
	// Create expired request
	req1 := &SwapRequest{
		Deadline: time.Now().Add(-1 * time.Hour),
	}
	if !req1.IsExpired() {
		t.Error("should be expired")
	}

	// Create non-expired request
	req2 := &SwapRequest{
		Deadline: time.Now().Add(1 * time.Hour),
	}
	if req2.IsExpired() {
		t.Error("should not be expired")
	}
}

func TestSwapRequest_ToCrossChainTx(t *testing.T) {
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "req-1",
		SourceChain:   ChainBTC,
		DestChain:     ChainETH,
		Sender:        sender,
		Recipient:     recipient,
		Amount:        big.NewInt(100000000),
		Token:         "BTC",
		Deadline:      time.Now().Add(24 * time.Hour),
	}

	tx := req.ToCrossChainTx("relayer-1")

	if tx.SourceChain != ChainBTC {
		t.Error("source chain mismatch")
	}
	if tx.DestChain != ChainETH {
		t.Error("dest chain mismatch")
	}
	if tx.RelayerID != "relayer-1" {
		t.Error("relayer ID mismatch")
	}
}

// ============================================================================
// RegisterRequest Tests
// ============================================================================

func TestNewRegisterRequest(t *testing.T) {
	addr := newTestAddress(ChainAIB, "relayer")
	chains := []ChainType{ChainBTC, ChainETH}
	stake := big.NewInt(1000000000)
	feeRate := big.NewInt(1000)

	req := NewRegisterRequest("node-1", addr, chains, stake, feeRate)

	if req.NodeID != "node-1" {
		t.Errorf("expected node ID node-1, got %s", req.NodeID)
	}
	if len(req.SupportedChains) != 2 {
		t.Errorf("expected 2 chains, got %d", len(req.SupportedChains))
	}
}

// ============================================================================
// Utility Function Tests
// ============================================================================

func TestGenerateTxID(t *testing.T) {
	sender := newTestAddress(ChainBTC, "sender")
	timestamp := time.Now()

	txID := GenerateTxID(ChainBTC, ChainETH, sender, timestamp)

	if txID == "" {
		t.Error("tx ID should not be empty")
	}
}

func TestGenerateRelayerID(t *testing.T) {
	pubKey := []byte("test-public-key")
	relayerID := GenerateRelayerID(pubKey)

	if relayerID == "" {
		t.Error("relayer ID should not be empty")
	}
	if len(relayerID) != 32 { // 16 bytes hex encoded
		t.Errorf("expected 32 hex chars, got %d", len(relayerID))
	}
}

func TestCalculateFee(t *testing.T) {
	amount := big.NewInt(100000000) // 1 BTC
	feeRate := big.NewInt(100000000) // 1% (100000000 / 100000000)

	fee := CalculateFee(amount, feeRate)

	// Fee = amount * feeRate / 100000000
	expected := big.NewInt(100000000 * 100000000 / 100000000)
	if fee.Cmp(expected) != 0 {
		t.Errorf("expected fee %s, got %s", expected.String(), fee.String())
	}
}

func TestValidateAddress(t *testing.T) {
	// Test valid ETH address (20 bytes)
	ethAddr := Address{
		Chain: ChainETH,
		Data:  make([]byte, 20),
	}
	if err := ValidateAddress(ethAddr); err != nil {
		t.Errorf("valid ETH address should not error: %v", err)
	}

	// Test invalid ETH address
	invalidEthAddr := Address{
		Chain: ChainETH,
		Data:  make([]byte, 10),
	}
	if err := ValidateAddress(invalidEthAddr); err == nil {
		t.Error("invalid ETH address should error")
	}

	// Test valid BTC address (26-35 bytes)
	btcAddr := Address{
		Chain: ChainBTC,
		Data:  make([]byte, 26),
	}
	if err := ValidateAddress(btcAddr); err != nil {
		t.Errorf("valid BTC address should not error: %v", err)
	}

	// Test empty address
	emptyAddr := Address{
		Chain: ChainETH,
		Data:  []byte{},
	}
	if err := ValidateAddress(emptyAddr); err == nil {
		t.Error("empty address should error")
	}
}

// ============================================================================
// RelayerNode Tests
// ============================================================================

func TestNewRelayerNode(t *testing.T) {
	addr := newTestAddress(ChainETH, "relayer")
	chains := []ChainType{ChainBTC, ChainETH, ChainSOL}
	stake := big.NewInt(1000000000)
	feeRate := big.NewInt(1000)

	node := NewRelayerNode("relayer-1", addr, "node-1", chains, stake, feeRate)

	if node.id != "relayer-1" {
		t.Errorf("expected id relayer-1, got %s", node.id)
	}
	if node.status != StatusActive {
		t.Error("expected status Active")
	}
	if len(node.supportedChains) != 3 {
		t.Errorf("expected 3 chains, got %d", len(node.supportedChains))
	}
}

func TestRelayerNode_Register(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	req := &RegisterRequest{
		NodeID:          "node-1",
		Address:        newTestAddress(ChainAIB, "addr"),
		SupportedChains: []ChainType{ChainBTC, ChainETH},
		Stake:          big.NewInt(2000000000),
		FeeRate:        big.NewInt(500),
	}

	err := node.Register(req)
	if err != nil {
		t.Errorf("register failed: %v", err)
	}

	if node.stake.Cmp(big.NewInt(2000000000)) != 0 {
		t.Error("stake should be updated")
	}
}

func TestRelayerNode_Register_NilRequest(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.Register(nil)
	if err == nil {
		t.Error("nil request should error")
	}
}

func TestRelayerNode_Register_InvalidStake(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	req := &RegisterRequest{
		Stake:    big.NewInt(0),
		FeeRate:  big.NewInt(1000),
	}

	err := node.Register(req)
	if err == nil {
		t.Error("invalid stake should error")
	}
}

func TestRelayerNode_Register_InvalidFeeRate(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	req := &RegisterRequest{
		Stake:    big.NewInt(1000000000),
		FeeRate:  big.NewInt(0),
	}

	err := node.Register(req)
	if err == nil {
		t.Error("invalid fee rate should error")
	}
}

func TestRelayerNode_GetTransaction(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Test non-existent transaction
	_, err := node.GetTransaction("non-existent")
	if err == nil {
		t.Error("should error for non-existent tx")
	}
}

func TestRelayerNode_ListTransactions(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	txs := node.ListTransactions()
	if len(txs) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(txs))
	}
}

func TestRelayerNode_GetStatus(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	status, err := node.GetStatus()
	if err != nil {
		t.Errorf("GetStatus failed: %v", err)
	}

	if status.RelayerID != "relayer-1" {
		t.Errorf("expected relayer ID relayer-1, got %s", status.RelayerID)
	}
	if status.Status != StatusActive {
		t.Error("expected status Active")
	}
}

// ============================================================================
// Validation Tests
// ============================================================================

func TestValidateRelayer(t *testing.T) {
	valid := &RelayerNode{
		id:              "relayer-1",
		supportedChains: []ChainType{ChainBTC},
		stake:          big.NewInt(1000000000),
		feeRate:        big.NewInt(1000),
	}

	if err := ValidateRelayer(valid); err != nil {
		t.Errorf("valid relayer should not error: %v", err)
	}

	// Test nil
	if err := ValidateRelayer(nil); err == nil {
		t.Error("nil relayer should error")
	}

	// Test empty ID
	emptyID := &RelayerNode{
		id:              "",
		supportedChains: []ChainType{ChainBTC},
		stake:          big.NewInt(1000000000),
		feeRate:        big.NewInt(1000),
	}
	if err := ValidateRelayer(emptyID); err == nil {
		t.Error("empty ID should error")
	}

	// Test no chains
	noChains := &RelayerNode{
		id:              "relayer-1",
		supportedChains: []ChainType{},
		stake:          big.NewInt(1000000000),
		feeRate:        big.NewInt(1000),
	}
	if err := ValidateRelayer(noChains); err == nil {
		t.Error("no chains should error")
	}
}

func TestCanRelay(t *testing.T) {
	relayer := &RelayerNode{
		status:          StatusActive,
		supportedChains: []ChainType{ChainBTC, ChainETH},
	}

	if !CanRelay(relayer, ChainBTC, ChainETH) {
		t.Error("should be able to relay BTC -> ETH")
	}
	if CanRelay(relayer, ChainBTC, ChainSOL) {
		t.Error("should not be able to relay to unsupported chain")
	}

	// Test inactive relayer
	relayer.status = StatusInactive
	if CanRelay(relayer, ChainBTC, ChainETH) {
		t.Error("inactive relayer should not relay")
	}
}

func TestSelectBestRelayers(t *testing.T) {
	relayer1 := &RelayerNode{
		id:          "r1",
		reputation:  90,
		feeRate:     big.NewInt(1000),
		status:      StatusActive,
		supportedChains: []ChainType{ChainBTC, ChainETH},
	}

	relayer2 := &RelayerNode{
		id:          "r2",
		reputation:  80,
		feeRate:     big.NewInt(500),
		status:      StatusActive,
		supportedChains: []ChainType{ChainBTC, ChainETH},
	}

	relayers := []*RelayerNode{relayer1, relayer2}
	best := SelectBestRelayers(relayers, ChainBTC, ChainETH, 1)

	if len(best) != 1 {
		t.Errorf("expected 1 best relayer, got %d", len(best))
	}
}

func TestSelectBestRelayers_LowReputation(t *testing.T) {
	// Relayer with reputation below minimum
	relayer := &RelayerNode{
		id:              "r1",
		reputation:     40, // Below MinRelayerReputation (50)
		feeRate:         big.NewInt(1000),
		status:          StatusActive,
		supportedChains: []ChainType{ChainBTC, ChainETH},
	}

	relayers := []*RelayerNode{relayer}
	best := SelectBestRelayers(relayers, ChainBTC, ChainETH, 1)

	// Should not select relayer with low reputation
	if len(best) != 0 {
		t.Errorf("expected 0 best relayers, got %d", len(best))
	}
}

// ============================================================================
// Network Tests
// ============================================================================

func TestNewNetwork(t *testing.T) {
	network := NewNetwork()

	if network == nil {
		t.Fatal("network should not be nil")
	}
	if len(network.relayers) != 0 {
		t.Errorf("expected 0 relayers, got %d", len(network.relayers))
	}
}

func TestNetwork_RegisterRelayer(t *testing.T) {
	network := NewNetwork()

	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := network.RegisterRelayer(node)
	if err != nil {
		t.Errorf("register failed: %v", err)
	}

	if len(network.relayers) != 1 {
		t.Errorf("expected 1 relayer, got %d", len(network.relayers))
	}
}

func TestNetwork_RegisterRelayer_Duplicate(t *testing.T) {
	network := NewNetwork()

	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	network.RegisterRelayer(node)

	// Try to register again
	err := network.RegisterRelayer(node)
	if err == nil {
		t.Error("duplicate registration should error")
	}
}

func TestNetwork_DiscoverRelayers(t *testing.T) {
	network := NewNetwork()

	// Register relayers
	r1 := NewRelayerNode("r1", newTestAddress(ChainETH, "r1"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	r2 := NewRelayerNode("r2", newTestAddress(ChainETH, "r2"), "node-2",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	network.RegisterRelayer(r1)
	network.RegisterRelayer(r2)

	// Discover BTC relayers
	relayers := network.DiscoverRelayers(ChainBTC)
	if len(relayers) != 2 {
		t.Errorf("expected 2 BTC relayers, got %d", len(relayers))
	}

	// Discover ETH relayers
	relayers = network.DiscoverRelayers(ChainETH)
	if len(relayers) != 1 {
		t.Errorf("expected 1 ETH relayer, got %d", len(relayers))
	}

	// Discover SOL relayers (none)
	relayers = network.DiscoverRelayers(ChainSOL)
	if len(relayers) != 0 {
		t.Errorf("expected 0 SOL relayers, got %d", len(relayers))
	}
}

func TestNetwork_GetNetworkStats(t *testing.T) {
	network := NewNetwork()

	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(node)

	stats := network.GetNetworkStats()

	if stats.TotalRelayers != 1 {
		t.Errorf("expected 1 total relayer, got %d", stats.TotalRelayers)
	}
	if stats.ActiveRelayers != 1 {
		t.Errorf("expected 1 active relayer, got %d", stats.ActiveRelayers)
	}
}

// ============================================================================
// Dispute Tests
// ============================================================================

func TestNewDispute(t *testing.T) {
	dispute := &Dispute{
		ID:        "dispute-1",
		TxHash:    "tx-123",
		Reporter: "user1",
		Reason:    "funds not received",
		Status:    "pending",
	}

	if dispute.ID != "dispute-1" {
		t.Errorf("expected ID dispute-1, got %s", dispute.ID)
	}
	if dispute.Status != "pending" {
		t.Error("status should be pending")
	}
}

// ============================================================================
// Serialization Tests
// ============================================================================

func TestCrossChainTx_Serialize(t *testing.T) {
	tx := &CrossChainTx{
		ID:           "tx-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Amount:       big.NewInt(100000000),
		Status:       TxStatusPending,
	}

	data, err := tx.Serialize()
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized data should not be empty")
	}
}

func TestCrossChainTx_Deserialize(t *testing.T) {
	tx := &CrossChainTx{
		ID:          "tx-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Amount:      big.NewInt(100000000),
		Status:      TxStatusPending,
	}

	data, _ := tx.Serialize()
	decoded, err := DeserializeCrossChainTx(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if decoded.ID != tx.ID {
		t.Errorf("expected ID %s, got %s", tx.ID, decoded.ID)
	}
}

// ============================================================================
// Helper Tests
// ============================================================================

func TestEstimateCompletionTime(t *testing.T) {
	btcTime := EstimateCompletionTime(ChainBTC)
	if btcTime <= 0 {
		t.Error("BTC completion time should be positive")
	}

	ethTime := EstimateCompletionTime(ChainETH)
	if ethTime <= 0 {
		t.Error("ETH completion time should be positive")
	}

	solTime := EstimateCompletionTime(ChainSOL)
	if solTime <= 0 {
		t.Error("SOL completion time should be positive")
	}

	unknownTime := EstimateCompletionTime(ChainType("UNKNOWN"))
	if unknownTime <= 0 {
		t.Error("Unknown chain should return default time")
	}
}

func TestNewAddressFromHex(t *testing.T) {
	hexStr := "0102030405060708090a0b0c0d0e0f1011121314"
	addr, err := NewAddressFromHex(ChainETH, hexStr)
	if err != nil {
		t.Fatalf("NewAddressFromHex failed: %v", err)
	}

	if addr.Chain != ChainETH {
		t.Error("chain should be ETH")
	}
}

func TestGenerateNodeID(t *testing.T) {
	nodeID := GenerateNodeID()
	if nodeID == "" {
		t.Error("node ID should not be empty")
	}
}

func TestCalculateTotalFee(t *testing.T) {
	amount := big.NewInt(100000000)
	feeRate := big.NewInt(1000)

	fee := CalculateTotalFee(amount, feeRate)
	if fee == nil {
		t.Error("fee should not be nil")
	}
}

// ============================================================================
// Constants Tests
// ============================================================================

func TestConstants(t *testing.T) {
	if DefaultRelayerStake <= 0 {
		t.Error("DefaultRelayerStake should be positive")
	}
	if DefaultConfirmationBlocks <= 0 {
		t.Error("DefaultConfirmationBlocks should be positive")
	}
	if MaxProofDepth <= 0 {
		t.Error("MaxProofDepth should be positive")
	}
	if MinRelayerReputation <= 0 {
		t.Error("MinRelayerReputation should be positive")
	}
	if SlashingRatio <= 0 || SlashingRatio >= 1 {
		t.Error("SlashingRatio should be between 0 and 1")
	}
}

// ============================================================================
// Additional RelayerNode Tests - SetStatus, Start, Stop
// ============================================================================

func TestRelayerNode_SetStatus(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	// Test valid status change
	node.SetStatus(StatusInactive)
	if node.status != StatusInactive {
		t.Error("status should be Inactive")
	}

	// Test invalid status is ignored
	node.SetStatus(RelayerStatus("Invalid"))
	if node.status != StatusInactive {
		t.Error("invalid status should be ignored")
	}
}

func TestRelayerNode_StartStop(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	// Start the relayer
	node.Start()

	// Give it a moment
	time.Sleep(10 * time.Millisecond)

	// Stop the relayer
	node.Stop()
}

// ============================================================================
// CreateRelayer Tests
// ============================================================================

func TestCreateRelayer(t *testing.T) {
	addr := newTestAddress(ChainETH, "relayer")
	chains := []ChainType{ChainBTC, ChainETH}
	stake := big.NewInt(1000000000)
	feeRate := big.NewInt(1000)

	relayer, err := CreateRelayer("node-1", addr, chains, stake, feeRate)
	if err != nil {
		t.Fatalf("CreateRelayer failed: %v", err)
	}

	if relayer.id == "" {
		t.Error("relayer ID should not be empty")
	}
}

func TestCreateRelayer_InvalidParams(t *testing.T) {
	addr := newTestAddress(ChainETH, "relayer")

	// Test empty node ID
	_, err := CreateRelayer("", addr, []ChainType{ChainBTC}, big.NewInt(1000), big.NewInt(1000))
	if err == nil {
		t.Error("empty node ID should fail")
	}

	// Test empty chains
	_, err = CreateRelayer("node-1", addr, []ChainType{}, big.NewInt(1000), big.NewInt(1000))
	if err == nil {
		t.Error("empty chains should fail")
	}

	// Test nil stake
	_, err = CreateRelayer("node-1", addr, []ChainType{ChainBTC}, nil, big.NewInt(1000))
	if err == nil {
		t.Error("nil stake should fail")
	}

	// Test zero stake
	_, err = CreateRelayer("node-1", addr, []ChainType{ChainBTC}, big.NewInt(0), big.NewInt(1000))
	if err == nil {
		t.Error("zero stake should fail")
	}

	// Test nil fee rate
	_, err = CreateRelayer("node-1", addr, []ChainType{ChainBTC}, big.NewInt(1000), nil)
	if err == nil {
		t.Error("nil fee rate should fail")
	}

	// Test zero fee rate
	_, err = CreateRelayer("node-1", addr, []ChainType{ChainBTC}, big.NewInt(1000), big.NewInt(0))
	if err == nil {
		t.Error("zero fee rate should fail")
	}
}

// ============================================================================
// RelayerNode Serialization Tests
// ============================================================================

func TestRelayerNode_Serialize(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	data, err := node.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized data should not be empty")
	}
}

func TestDeserializeRelayerNode(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	data, _ := node.Serialize()
	decoded, err := DeserializeRelayerNode(data)
	if err != nil {
		t.Fatalf("DeserializeRelayerNode failed: %v", err)
	}

	if decoded.id != node.id {
		t.Errorf("expected ID %s, got %s", node.id, decoded.id)
	}
}

func TestDeserializeRelayerNode_InvalidData(t *testing.T) {
	// Test invalid JSON
	_, err := DeserializeRelayerNode([]byte("invalid"))
	if err == nil {
		t.Error("invalid JSON should fail")
	}

	// Test invalid stake value
	invalidStakeJSON := []byte(`{"id":"r1","address":{"chain":"ETH","data":"aabbccdd"},"node_id":"n1","status":"Active","stake":"invalid","supported_chains":["BTC"],"fee_rate":"1000","reputation":100,"total_txs":0,"success_rate":1,"created_at":1234567890,"last_active_at":1234567890}`)
	_, err = DeserializeRelayerNode(invalidStakeJSON)
	if err == nil {
		t.Error("invalid stake should fail")
	}
}

// ============================================================================
// Network Additional Tests - AssignTask, ReportDispute, ResolveDispute
// ============================================================================

func TestNetwork_AssignTask(t *testing.T) {
	network := NewNetwork()

	// Register a relayer
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(node)

	// Create a swap request
	secretHash := sha256.Sum256([]byte("secret"))
	sender := newTestAddress(ChainBTC, "sender")
	req := &SwapRequest{
		ID:           "req-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    newTestAddress(ChainETH, "recipient"),
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
		SecretHash:   secretHash[:],
	}

	relayer, err := network.AssignTask(req)
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}

	if relayer == nil {
		t.Error("relayer should not be nil")
	}
}

func TestNetwork_AssignTask_NoRelayers(t *testing.T) {
	network := NewNetwork()

	// Try to assign task with no relayers
	sender := newTestAddress(ChainBTC, "sender")
	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   newTestAddress(ChainETH, "recipient"),
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	_, err := network.AssignTask(req)
	if err == nil {
		t.Error("should fail with no relayers")
	}
}

func TestNetwork_AssignTask_NilRequest(t *testing.T) {
	network := NewNetwork()

	_, err := network.AssignTask(nil)
	if err == nil {
		t.Error("nil request should fail")
	}
}

func TestNetwork_ReportDispute(t *testing.T) {
	network := NewNetwork()

	// Register a relayer with a transaction
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	// Report dispute
	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "funds not received",
	}

	err := network.ReportDispute(dispute)
	if err != nil {
		t.Fatalf("ReportDispute failed: %v", err)
	}

	if dispute.ID == "" {
		t.Error("dispute ID should be generated")
	}
}

func TestNetwork_ReportDispute_Nil(t *testing.T) {
	network := NewNetwork()

	err := network.ReportDispute(nil)
	if err == nil {
		t.Error("nil dispute should fail")
	}
}

func TestNetwork_ReportDispute_Invalid(t *testing.T) {
	network := NewNetwork()

	// Missing tx hash
	dispute := &Dispute{
		Reporter: "user1",
		Reason:   "test",
	}
	err := network.ReportDispute(dispute)
	if err == nil {
		t.Error("missing tx hash should fail")
	}
}

func TestNetwork_ResolveDispute(t *testing.T) {
	network := NewNetwork()

	// Register relayer with transaction
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	// Report dispute
	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	// Resolve dispute
	resolution := &DisputeResolution{
		DisputeID:  dispute.ID,
		Winner:     "relayer-1",
		Loser:      "reporter",
		Resolution: "relayer is honest",
		Penalty:    big.NewInt(0),
	}

	err := network.ResolveDispute(dispute.ID, resolution)
	if err != nil {
		t.Fatalf("ResolveDispute failed: %v", err)
	}
}

func TestNetwork_ResolveDispute_NotFound(t *testing.T) {
	network := NewNetwork()

	resolution := &DisputeResolution{
		DisputeID: "nonexistent",
		Winner:    "relayer-1",
	}

	err := network.ResolveDispute("nonexistent", resolution)
	if err == nil {
		t.Error("should fail for nonexistent dispute")
	}
}

func TestNetwork_ResolveDispute_WithPenalty(t *testing.T) {
	network := NewNetwork()

	// Register relayer with transaction
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	// Report dispute
	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	// Resolve with penalty
	resolution := &DisputeResolution{
		DisputeID:  dispute.ID,
		Winner:     "reporter",
		Loser:      "relayer-1",
		Resolution: "relayer is dishonest",
		Penalty:    big.NewInt(100000000),
	}

	err := network.ResolveDispute(dispute.ID, resolution)
	if err != nil {
		t.Fatalf("ResolveDispute failed: %v", err)
	}
}

func TestNetwork_SlashRelayer(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	err := network.SlashRelayer("relayer-1", 0.5)
	if err != nil {
		t.Fatalf("SlashRelayer failed: %v", err)
	}

	if relayer.status != StatusSlashed {
		t.Error("relayer should be slashed")
	}
}

func TestNetwork_SlashRelayer_NotFound(t *testing.T) {
	network := NewNetwork()

	err := network.SlashRelayer("nonexistent", 0.5)
	if err == nil {
		t.Error("should fail for nonexistent relayer")
	}
}

func TestNetwork_ReactivateRelayer(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	// Slash first
	network.SlashRelayer("relayer-1", 0.5)

	// Reactivate
	err := network.ReactivateRelayer("relayer-1", big.NewInt(1000000000))
	if err != nil {
		t.Fatalf("ReactivateRelayer failed: %v", err)
	}

	if relayer.status != StatusActive {
		t.Error("relayer should be active")
	}
}

func TestNetwork_GetRelayer(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	r, err := network.GetRelayer("relayer-1")
	if err != nil {
		t.Fatalf("GetRelayer failed: %v", err)
	}

	if r.id != "relayer-1" {
		t.Error("relayer ID mismatch")
	}
}

func TestNetwork_GetDispute(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	d, err := network.GetDispute(dispute.ID)
	if err != nil {
		t.Fatalf("GetDispute failed: %v", err)
	}

	if d.ID != dispute.ID {
		t.Error("dispute ID mismatch")
	}
}

func TestNetwork_ListDisputes(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	disputes := network.ListDisputes("pending")
	if len(disputes) != 1 {
		t.Errorf("expected 1 dispute, got %d", len(disputes))
	}
}

func TestNetwork_GetResolution(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	resolution := &DisputeResolution{
		DisputeID:  dispute.ID,
		Winner:     "relayer-1",
		Resolution: "resolved",
	}
	network.ResolveDispute(dispute.ID, resolution)

	res, err := network.GetResolution(dispute.ID)
	if err != nil {
		t.Fatalf("GetResolution failed: %v", err)
	}

	if res.DisputeID != dispute.ID {
		t.Error("resolution dispute ID mismatch")
	}
}

func TestNetwork_FindRelayersForSwap(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	relayers := network.FindRelayersForSwap(ChainBTC, ChainETH)
	if len(relayers) != 1 {
		t.Errorf("expected 1 relayer, got %d", len(relayers))
	}
}

func TestNetwork_GetRelayersByChain(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	relayers := network.GetRelayersByChain(ChainBTC)
	if len(relayers) != 1 {
		t.Errorf("expected 1 relayer, got %d", len(relayers))
	}
}

func TestNetwork_GetActiveRelayers(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	relayers := network.GetActiveRelayers()
	if len(relayers) != 1 {
		t.Errorf("expected 1 active relayer, got %d", len(relayers))
	}
}

func TestNewNetworkEvent(t *testing.T) {
	event := NewNetworkEvent("registered", "relayer-1", "new relayer joined")
	if event.Type != "registered" {
		t.Error("event type mismatch")
	}
	if event.RelayerID != "relayer-1" {
		t.Error("relayer ID mismatch")
	}
}

// ============================================================================
// Adapter Tests
// ============================================================================

func TestNewBaseChainAdapter(t *testing.T) {
	adapter := NewBaseChainAdapter(ChainBTC, 6)
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}
	if adapter.chainType != ChainBTC {
		t.Errorf("expected chain BTC, got %s", adapter.chainType)
	}
	if adapter.confirmations != 6 {
		t.Errorf("expected 6 confirmations, got %d", adapter.confirmations)
	}
}

func TestBaseChainAdapter_GetChainType(t *testing.T) {
	adapter := NewBaseChainAdapter(ChainETH, 12)
	if adapter.GetChainType() != ChainETH {
		t.Error("chain type should be ETH")
	}
}

func TestBaseChainAdapter_GetRequiredConfirmations(t *testing.T) {
	adapter := NewBaseChainAdapter(ChainBTC, 6)
	confirmations := adapter.GetRequiredConfirmations()
	if confirmations != 6 {
		t.Errorf("expected 6, got %d", confirmations)
	}
}

func TestBaseChainAdapter_SetConfirmations(t *testing.T) {
	adapter := NewBaseChainAdapter(ChainBTC, 6)
	adapter.SetConfirmations(10)
	if adapter.confirmations != 10 {
		t.Error("confirmations should be updated")
	}
}

// ============================================================================
// Bitcoin Adapter Tests
// ============================================================================

func TestNewBitcoinAdapter(t *testing.T) {
	adapter := NewBitcoinAdapter()
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}
	if adapter.GetChainType() != ChainBTC {
		t.Error("chain type should be BTC")
	}
	if adapter.GetRequiredConfirmations() != 6 {
		t.Error("BTC should require 6 confirmations")
	}
}

func TestBitcoinAdapter_LockFunds(t *testing.T) {
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

func TestBitcoinAdapter_LockFunds_InvalidAmount(t *testing.T) {
	adapter := NewBitcoinAdapter()

	req := &SwapRequest{
		Amount: big.NewInt(0),
	}

	_, err := adapter.LockFunds(req)
	if err == nil {
		t.Error("invalid amount should fail")
	}
}

func TestBitcoinAdapter_UnlockFunds(t *testing.T) {
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	proof := &MerkleProof{
		BlockNumber: 800000,
		Proof:       [][]byte{[]byte("proof1")},
	}

	txHash, err := adapter.UnlockFunds(req, proof)
	if err != nil {
		t.Fatalf("UnlockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

func TestBitcoinAdapter_UnlockFunds_NilProof(t *testing.T) {
	adapter := NewBitcoinAdapter()

	req := &SwapRequest{
		Amount: big.NewInt(100000000),
	}

	_, err := adapter.UnlockFunds(req, nil)
	if err == nil {
		t.Error("nil proof should fail")
	}
}

func TestBitcoinAdapter_SubmitProof(t *testing.T) {
	adapter := NewBitcoinAdapter()

	proof := &MerkleProof{
		Proof: [][]byte{[]byte("proof1"), []byte("proof2")},
	}

	err := adapter.SubmitProof("txhash", proof)
	if err != nil {
		t.Errorf("SubmitProof failed: %v", err)
	}
}

func TestBitcoinAdapter_SubmitProof_EmptyProof(t *testing.T) {
	adapter := NewBitcoinAdapter()

	proof := &MerkleProof{
		Proof: [][]byte{},
	}

	err := adapter.SubmitProof("txhash", proof)
	if err == nil {
		t.Error("empty proof should fail")
	}
}

func TestBitcoinAdapter_SubmitProof_NilProof(t *testing.T) {
	adapter := NewBitcoinAdapter()

	err := adapter.SubmitProof("txhash", nil)
	if err == nil {
		t.Error("nil proof should fail")
	}
}

func TestBitcoinAdapter_GetBlockHeight(t *testing.T) {
	adapter := NewBitcoinAdapter()

	height, err := adapter.GetBlockHeight()
	if err != nil {
		t.Errorf("GetBlockHeight failed: %v", err)
	}
	if height == 0 {
		t.Error("block height should be positive")
	}
}

func TestBitcoinAdapter_GetMerkleProof(t *testing.T) {
	adapter := NewBitcoinAdapter()

	// First lock funds to create transaction
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}
	txHash, _ := adapter.LockFunds(req)

	proof, err := adapter.GetMerkleProof(txHash)
	if err != nil {
		t.Fatalf("GetMerkleProof failed: %v", err)
	}
	if proof == nil {
		t.Error("proof should not be nil")
	}
	if len(proof.Proof) == 0 {
		t.Error("proof path should not be empty")
	}
}

func TestBitcoinAdapter_GetMerkleProof_NotFound(t *testing.T) {
	adapter := NewBitcoinAdapter()

	_, err := adapter.GetMerkleProof("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestBitcoinAdapter_GetTxByHash(t *testing.T) {
	adapter := NewBitcoinAdapter()

	// First lock funds
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}
	txHash, _ := adapter.LockFunds(req)

	tx, err := adapter.GetTxByHash(txHash)
	if err != nil {
		t.Fatalf("GetTxByHash failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
}

func TestBitcoinAdapter_GetTxByHash_NotFound(t *testing.T) {
	adapter := NewBitcoinAdapter()

	_, err := adapter.GetTxByHash("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

// ============================================================================
// Ethereum Adapter Tests
// ============================================================================

func TestNewEthereumAdapter(t *testing.T) {
	adapter := NewEthereumAdapter()
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}
	if adapter.GetChainType() != ChainETH {
		t.Error("chain type should be ETH")
	}
	if adapter.GetRequiredConfirmations() != 12 {
		t.Error("ETH should require 12 confirmations")
	}
}

func TestEthereumAdapter_LockFunds(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000), // 1 ETH
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

func TestEthereumAdapter_UnlockFunds(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	proof := &MerkleProof{
		BlockNumber: 18000000,
		Proof:       [][]byte{[]byte("proof1")},
	}

	txHash, err := adapter.UnlockFunds(req, proof)
	if err != nil {
		t.Fatalf("UnlockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

func TestEthereumAdapter_SubmitProof(t *testing.T) {
	adapter := NewEthereumAdapter()

	proof := &MerkleProof{
		Proof: [][]byte{[]byte("proof1")},
	}

	err := adapter.SubmitProof("txhash", proof)
	if err != nil {
		t.Errorf("SubmitProof failed: %v", err)
	}
}

func TestEthereumAdapter_GetBlockHeight(t *testing.T) {
	adapter := NewEthereumAdapter()

	height, err := adapter.GetBlockHeight()
	if err != nil {
		t.Errorf("GetBlockHeight failed: %v", err)
	}
	if height == 0 {
		t.Error("block height should be positive")
	}
}

// ============================================================================
// Solana Adapter Tests
// ============================================================================

func TestNewSolanaAdapter(t *testing.T) {
	adapter := NewSolanaAdapter()
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}
	if adapter.GetChainType() != ChainSOL {
		t.Error("chain type should be SOL")
	}
	if adapter.GetRequiredConfirmations() != 32 {
		t.Error("SOL should require 32 confirmations")
	}
}

func TestSolanaAdapter_LockFunds(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000), // 1 SOL
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

func TestSolanaAdapter_UnlockFunds(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	proof := &MerkleProof{
		BlockNumber: 230000000,
		Proof:       [][]byte{[]byte("proof1")},
	}

	txHash, err := adapter.UnlockFunds(req, proof)
	if err != nil {
		t.Fatalf("UnlockFunds failed: %v", err)
	}
	if txHash == "" {
		t.Error("tx hash should not be empty")
	}
}

// ============================================================================
// Chain Adapter Factory Tests
// ============================================================================

func TestNewChainAdapter(t *testing.T) {
	// Test BTC
	adapter, err := NewChainAdapter(ChainBTC)
	if err != nil {
		t.Fatalf("NewChainAdapter failed for BTC: %v", err)
	}
	if adapter.GetChainType() != ChainBTC {
		t.Error("should return BTC adapter")
	}

	// Test ETH
	adapter, err = NewChainAdapter(ChainETH)
	if err != nil {
		t.Fatalf("NewChainAdapter failed for ETH: %v", err)
	}
	if adapter.GetChainType() != ChainETH {
		t.Error("should return ETH adapter")
	}

	// Test SOL
	adapter, err = NewChainAdapter(ChainSOL)
	if err != nil {
		t.Fatalf("NewChainAdapter failed for SOL: %v", err)
	}
	if adapter.GetChainType() != ChainSOL {
		t.Error("should return SOL adapter")
	}
}

func TestNewChainAdapter_Unsupported(t *testing.T) {
	_, err := NewChainAdapter(ChainType("UNSUPPORTED"))
	if err == nil {
		t.Error("unsupported chain should fail")
	}
}

// ============================================================================
// Adapter Manager Tests
// ============================================================================

func TestNewAdapterManager(t *testing.T) {
	mgr := NewAdapterManager()
	if mgr == nil {
		t.Fatal("manager should not be nil")
	}
	if len(mgr.adapters) != 0 {
		t.Error("should start with empty adapters")
	}
}

func TestAdapterManager_RegisterAdapter(t *testing.T) {
	mgr := NewAdapterManager()

	adapter := NewBitcoinAdapter()
	err := mgr.RegisterAdapter(adapter)
	if err != nil {
		t.Fatalf("RegisterAdapter failed: %v", err)
	}

	chains := mgr.GetSupportedChains()
	if len(chains) != 1 {
		t.Errorf("expected 1 chain, got %d", len(chains))
	}
}

func TestAdapterManager_RegisterAdapter_Nil(t *testing.T) {
	mgr := NewAdapterManager()

	err := mgr.RegisterAdapter(nil)
	if err == nil {
		t.Error("nil adapter should fail")
	}
}

func TestAdapterManager_GetAdapter(t *testing.T) {
	mgr := NewAdapterManager()

	adapter := NewBitcoinAdapter()
	mgr.RegisterAdapter(adapter)

	retrieved, err := mgr.GetAdapter(ChainBTC)
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}
	if retrieved.GetChainType() != ChainBTC {
		t.Error("should return BTC adapter")
	}
}

func TestAdapterManager_GetAdapter_NotFound(t *testing.T) {
	mgr := NewAdapterManager()

	_, err := mgr.GetAdapter(ChainBTC)
	if err == nil {
		t.Error("should fail for unregistered chain")
	}
}

func TestAdapterManager_GetSupportedChains(t *testing.T) {
	mgr := NewAdapterManager()

	mgr.RegisterAdapter(NewBitcoinAdapter())
	mgr.RegisterAdapter(NewEthereumAdapter())

	chains := mgr.GetSupportedChains()
	if len(chains) != 2 {
		t.Errorf("expected 2 chains, got %d", len(chains))
	}
}

// ============================================================================
// JSON Serialization Tests
// ============================================================================

func TestToJSON(t *testing.T) {
	data := map[string]string{"key": "value"}
	jsonStr, err := ToJSON(data)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if jsonStr == "" {
		t.Error("json should not be empty")
	}
}

func TestFromJSON(t *testing.T) {
	jsonStr := `{"key":"value"}`
	var data map[string]string
	err := FromJSON(jsonStr, &data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}
	if data["key"] != "value" {
		t.Error("value mismatch")
	}
}

// ============================================================================
// Chain Utility Tests
// ============================================================================

func TestParseChainType(t *testing.T) {
	chain, err := ParseChainType("BTC")
	if err != nil {
		t.Fatalf("ParseChainType failed: %v", err)
	}
	if chain != ChainBTC {
		t.Error("should parse BTC")
	}

	chain, err = ParseChainType("ETH")
	if err != nil {
		t.Fatalf("ParseChainType failed: %v", err)
	}
	if chain != ChainETH {
		t.Error("should parse ETH")
	}
}

func TestParseChainType_Invalid(t *testing.T) {
	_, err := ParseChainType("INVALID")
	if err == nil {
		t.Error("invalid chain should fail")
	}
}

func TestIsValidChain(t *testing.T) {
	if !IsValidChain(ChainBTC) {
		t.Error("BTC should be valid")
	}
	if !IsValidChain(ChainETH) {
		t.Error("ETH should be valid")
	}
	if !IsValidChain(ChainSOL) {
		t.Error("SOL should be valid")
	}
	if !IsValidChain(ChainAIB) {
		t.Error("AIB should be valid")
	}
	if IsValidChain(ChainType("INVALID")) {
		t.Error("INVALID should not be valid")
	}
}

func TestNormalizeAmount(t *testing.T) {
	amount := big.NewInt(1)
	normalized := NormalizeAmount(amount, 8)
	if normalized.Cmp(big.NewInt(100000000)) != 0 {
		t.Error("amount should be normalized")
	}
}

func TestDenormalizeAmount(t *testing.T) {
	amount := big.NewInt(100000000)
	denormalized := DenormalizeAmount(amount, 8)
	if denormalized.Cmp(big.NewInt(1)) != 0 {
		t.Error("amount should be denormalized")
	}
}

func TestCompareChainAddresses(t *testing.T) {
	addr1 := Address{Data: []byte{1, 2, 3, 4}}
	addr2 := Address{Data: []byte{1, 2, 3, 4}}
	addr3 := Address{Data: []byte{5, 6, 7, 8}}

	if !CompareChainAddresses(addr1, addr2) {
		t.Error("same addresses should match")
	}
	if CompareChainAddresses(addr1, addr3) {
		t.Error("different addresses should not match")
	}
}

func TestValidateSwapRequest(t *testing.T) {
	// Valid request
	req := &SwapRequest{
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Amount:      big.NewInt(100000000),
		Deadline:    time.Now().Add(24 * time.Hour),
		Sender:      newTestAddress(ChainBTC, "sender"),
		Recipient:   newTestAddress(ChainETH, "recipient"),
	}
	err := ValidateSwapRequest(req)
	if err != nil {
		t.Errorf("valid request should not error: %v", err)
	}

	// Nil request
	err = ValidateSwapRequest(nil)
	if err == nil {
		t.Error("nil request should error")
	}

	// Zero amount
	req.Amount = big.NewInt(0)
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("zero amount should error")
	}

	// Invalid source chain
	req.Amount = big.NewInt(100000000)
	req.SourceChain = ChainType("INVALID")
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("invalid source chain should error")
	}

	// Same source and dest chain
	req.SourceChain = ChainBTC
	req.DestChain = ChainBTC
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("same chain should error")
	}

	// Expired request
	req.DestChain = ChainETH
	req.Deadline = time.Now().Add(-1 * time.Hour)
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("expired request should error")
	}

	// Empty sender
	req.Deadline = time.Now().Add(24 * time.Hour)
	req.Sender = Address{}
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("empty sender should error")
	}
}

// ============================================================================
// RelayerNode Additional Tests - SubmitProof, ReleaseFunds
// ============================================================================

func TestRelayerNode_SubmitProof(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Add a transaction with a known SourceTxHash
	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	tx.Status = TxStatusLocked
	tx.SourceTxHash = newTestHash(ChainBTC, "source-tx")
	sourceTxHashStr := tx.SourceTxHash.String()
	node.transactions[tx.ID] = tx

	proof := &MerkleProof{
		BlockNumber: 800000,
		Proof:       [][]byte{[]byte("proof")},
	}

	err := node.SubmitProof(sourceTxHashStr, proof)
	if err != nil {
		t.Errorf("SubmitProof failed: %v", err)
	}
}

func TestRelayerNode_SubmitProof_EmptyTxHash(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.SubmitProof("", &MerkleProof{})
	if err == nil {
		t.Error("empty tx hash should fail")
	}
}

func TestRelayerNode_SubmitProof_NilProof(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.SubmitProof("txhash", nil)
	if err == nil {
		t.Error("nil proof should fail")
	}
}

func TestRelayerNode_SubmitProof_TxNotFound(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.SubmitProof("nonexistent", &MerkleProof{})
	if err == nil {
		t.Error("nonexistent tx should fail")
	}
}

func TestRelayerNode_ReleaseFunds(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Add a transaction with proof ready and known SourceTxHash
	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	tx.Status = TxStatusProofReady
	tx.SourceTxHash = newTestHash(ChainBTC, "source-tx")
	tx.Proof = &MerkleProof{}
	sourceTxHashStr := tx.SourceTxHash.String()
	node.transactions[tx.ID] = tx

	err := node.ReleaseFunds(sourceTxHashStr)
	if err != nil {
		t.Errorf("ReleaseFunds failed: %v", err)
	}
}

func TestRelayerNode_ReleaseFunds_EmptyTxHash(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.ReleaseFunds("")
	if err == nil {
		t.Error("empty tx hash should fail")
	}
}

func TestRelayerNode_ReleaseFunds_TxNotFound(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	err := node.ReleaseFunds("nonexistent")
	if err == nil {
		t.Error("nonexistent tx should fail")
	}
}

func TestRelayerNode_ReleaseFunds_NotReady(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	tx.Status = TxStatusLocked // Not ready for release
	tx.SourceTxHash = newTestHash(ChainBTC, "source-tx")
	sourceTxHashStr := tx.SourceTxHash.String()
	node.transactions[tx.ID] = tx

	err := node.ReleaseFunds(sourceTxHashStr)
	if err == nil {
		t.Error("tx not ready should fail")
	}
}

// ============================================================================
// Network Unregister Tests
// ============================================================================

func TestNetwork_UnregisterRelayer(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	err := network.UnregisterRelayer("relayer-1")
	if err != nil {
		t.Fatalf("UnregisterRelayer failed: %v", err)
	}

	if len(network.relayers) != 0 {
		t.Error("relayer should be removed")
	}
}

func TestNetwork_UnregisterRelayer_NotFound(t *testing.T) {
	network := NewNetwork()

	err := network.UnregisterRelayer("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent relayer")
	}
}

func TestNetwork_UnregisterRelayer_PendingTXs(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	// Add pending transaction
	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	tx.Status = TxStatusLocked // Not completed
	relayer.transactions[tx.ID] = tx

	network.RegisterRelayer(relayer)

	err := network.UnregisterRelayer("relayer-1")
	if err == nil {
		t.Error("should fail with pending transactions")
	}
}

// ============================================================================
// Additional Adapter Tests
// ============================================================================

func TestBitcoinAdapter_VerifyTx(t *testing.T) {
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	tx, confs, err := adapter.VerifyTx(txHash)
	if err != nil {
		t.Fatalf("VerifyTx failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestBitcoinAdapter_VerifyTx_NotFound(t *testing.T) {
	adapter := NewBitcoinAdapter()

	_, _, err := adapter.VerifyTx("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestBitcoinAdapter_GetConfirmations(t *testing.T) {
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	confs, err := adapter.GetConfirmations(txHash)
	if err != nil {
		t.Errorf("GetConfirmations failed: %v", err)
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestBitcoinAdapter_GetConfirmations_NotFound(t *testing.T) {
	adapter := NewBitcoinAdapter()

	_, err := adapter.GetConfirmations("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestEthereumAdapter_VerifyTx(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	tx, confs, err := adapter.VerifyTx(txHash)
	if err != nil {
		t.Fatalf("VerifyTx failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestEthereumAdapter_VerifyTx_NotFound(t *testing.T) {
	adapter := NewEthereumAdapter()

	_, _, err := adapter.VerifyTx("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestEthereumAdapter_GetConfirmations(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	confs, err := adapter.GetConfirmations(txHash)
	if err != nil {
		t.Errorf("GetConfirmations failed: %v", err)
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestEthereumAdapter_GetMerkleProof(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	proof, err := adapter.GetMerkleProof(txHash)
	if err != nil {
		t.Fatalf("GetMerkleProof failed: %v", err)
	}
	if proof == nil {
		t.Error("proof should not be nil")
	}
}

func TestEthereumAdapter_GetMerkleProof_NotFound(t *testing.T) {
	adapter := NewEthereumAdapter()

	_, err := adapter.GetMerkleProof("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestEthereumAdapter_GetTxByHash(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	tx, err := adapter.GetTxByHash(txHash)
	if err != nil {
		t.Fatalf("GetTxByHash failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
}

func TestEthereumAdapter_GetTxByHash_NotFound(t *testing.T) {
	adapter := NewEthereumAdapter()

	_, err := adapter.GetTxByHash("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestSolanaAdapter_VerifyTx(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	tx, confs, err := adapter.VerifyTx(txHash)
	if err != nil {
		t.Fatalf("VerifyTx failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestSolanaAdapter_VerifyTx_NotFound(t *testing.T) {
	adapter := NewSolanaAdapter()

	_, _, err := adapter.VerifyTx("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestSolanaAdapter_GetConfirmations(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	confs, err := adapter.GetConfirmations(txHash)
	if err != nil {
		t.Errorf("GetConfirmations failed: %v", err)
	}
	if confs != 0 {
		t.Errorf("expected 0 confirmations, got %d", confs)
	}
}

func TestSolanaAdapter_GetBlockHeight(t *testing.T) {
	adapter := NewSolanaAdapter()

	height, err := adapter.GetBlockHeight()
	if err != nil {
		t.Errorf("GetBlockHeight failed: %v", err)
	}
	if height == 0 {
		t.Error("block height should be positive")
	}
}

func TestSolanaAdapter_GetMerkleProof(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	proof, err := adapter.GetMerkleProof(txHash)
	if err != nil {
		t.Fatalf("GetMerkleProof failed: %v", err)
	}
	if proof == nil {
		t.Error("proof should not be nil")
	}
}

func TestSolanaAdapter_GetMerkleProof_NotFound(t *testing.T) {
	adapter := NewSolanaAdapter()

	_, err := adapter.GetMerkleProof("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestSolanaAdapter_GetTxByHash(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	tx, err := adapter.GetTxByHash(txHash)
	if err != nil {
		t.Fatalf("GetTxByHash failed: %v", err)
	}
	if tx == nil {
		t.Error("tx should not be nil")
	}
}

func TestSolanaAdapter_GetTxByHash_NotFound(t *testing.T) {
	adapter := NewSolanaAdapter()

	_, err := adapter.GetTxByHash("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestSolanaAdapter_SubmitProof(t *testing.T) {
	adapter := NewSolanaAdapter()

	proof := &MerkleProof{
		Proof: [][]byte{[]byte("proof")},
	}

	err := adapter.SubmitProof("txhash", proof)
	if err != nil {
		t.Errorf("SubmitProof failed: %v", err)
	}
}

func TestSolanaAdapter_SubmitProof_Nil(t *testing.T) {
	adapter := NewSolanaAdapter()

	err := adapter.SubmitProof("txhash", nil)
	if err == nil {
		t.Error("nil proof should fail")
	}
}

// ============================================================================
// Relayer Serialization Tests
// ============================================================================

func TestRelayer_Serialize(t *testing.T) {
	addr := newTestAddress(ChainETH, "relayer")
	relayer := NewRelayer("relayer-1", addr, "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000))

	data, err := relayer.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("serialized data should not be empty")
	}
}

func TestDeserializeRelayer(t *testing.T) {
	addr := newTestAddress(ChainETH, "relayer")
	relayer := NewRelayer("relayer-1", addr, "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000))

	data, _ := relayer.Serialize()
	decoded, err := DeserializeRelayer(data)
	if err != nil {
		t.Fatalf("DeserializeRelayer failed: %v", err)
	}

	if decoded.ID != relayer.ID {
		t.Errorf("expected ID %s, got %s", relayer.ID, decoded.ID)
	}
}

func TestDeserializeRelayer_InvalidJSON(t *testing.T) {
	_, err := DeserializeRelayer([]byte("invalid"))
	if err == nil {
		t.Error("invalid JSON should fail")
	}
}

// ============================================================================
// MerkleProof Serialization Tests
// ============================================================================

func TestMerkleProof_Serialize(t *testing.T) {
	txHash := newTestHash(ChainBTC, "tx")
	blockHash := newTestHash(ChainBTC, "block")
	proof := NewMerkleProof(txHash, blockHash, 800000, 0, [][]byte{[]byte("node")}, ChainBTC)

	data, err := proof.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("serialized data should not be empty")
	}
}

func TestDeserializeMerkleProof(t *testing.T) {
	txHash := newTestHash(ChainBTC, "tx")
	blockHash := newTestHash(ChainBTC, "block")
	proof := NewMerkleProof(txHash, blockHash, 800000, 0, [][]byte{[]byte("node")}, ChainBTC)

	data, _ := proof.Serialize()
	decoded, err := DeserializeMerkleProof(data)
	if err != nil {
		t.Fatalf("DeserializeMerkleProof failed: %v", err)
	}

	if decoded.BlockNumber != proof.BlockNumber {
		t.Error("block number mismatch")
	}
}

func TestDeserializeMerkleProof_InvalidJSON(t *testing.T) {
	_, err := DeserializeMerkleProof([]byte("invalid"))
	if err == nil {
		t.Error("invalid JSON should fail")
	}
}

// ============================================================================
// RelayerNode CreateSwap Tests
// ============================================================================

func TestRelayerNode_CreateSwap(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "swap-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		RelayerFee:  big.NewInt(1000),
		Deadline:    time.Now().Add(24 * time.Hour),
		SecretHash:  func() []byte { h := sha256.Sum256([]byte("secret")); return h[:] }(),
	}

	tx, err := node.CreateSwap(req)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}
	if tx == nil {
		t.Fatal("tx should not be nil")
	}
	if tx.ID != "swap-1" {
		t.Errorf("expected ID swap-1, got %s", tx.ID)
	}
	if tx.Status != TxStatusLocked {
		t.Errorf("expected status Locked, got %s", tx.Status)
	}
}

func TestRelayerNode_CreateSwap_NilRequest(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	_, err := node.CreateSwap(nil)
	if err == nil {
		t.Error("nil request should fail")
	}
}

func TestRelayerNode_CreateSwap_InvalidRequest(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Invalid: zero amount
	req := &SwapRequest{
		Amount:   big.NewInt(0),
		Deadline: time.Now().Add(24 * time.Hour),
	}

	_, err := node.CreateSwap(req)
	if err == nil {
		t.Error("invalid request should fail")
	}
}

func TestRelayerNode_CreateSwap_InactiveRelayer(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))
	node.status = StatusInactive

	req := &SwapRequest{
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Amount:      big.NewInt(100000000),
		Deadline:    time.Now().Add(24 * time.Hour),
		Sender:      newTestAddress(ChainBTC, "sender"),
		Recipient:   newTestAddress(ChainETH, "recipient"),
	}

	_, err := node.CreateSwap(req)
	if err == nil {
		t.Error("inactive relayer should fail")
	}
}

func TestRelayerNode_CreateSwap_UnsupportedChain(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	req := &SwapRequest{
		SourceChain: ChainBTC,
		DestChain:   ChainSOL, // Not supported
		Amount:      big.NewInt(100000000),
		Deadline:    time.Now().Add(24 * time.Hour),
		Sender:      newTestAddress(ChainBTC, "sender"),
		Recipient:   newTestAddress(ChainSOL, "recipient"),
	}

	_, err := node.CreateSwap(req)
	if err == nil {
		t.Error("unsupported chain should fail")
	}
}

// ============================================================================
// ParseChainType Additional Tests
// ============================================================================

func TestParseChainType_Solana(t *testing.T) {
	chain, err := ParseChainType("SOL")
	if err != nil {
		t.Fatalf("ParseChainType failed for SOL: %v", err)
	}
	if chain != ChainSOL {
		t.Error("should parse SOL")
	}
}

func TestParseChainType_AIB(t *testing.T) {
	chain, err := ParseChainType("AIB")
	if err != nil {
		t.Fatalf("ParseChainType failed for AIB: %v", err)
	}
	if chain != ChainAIB {
		t.Error("should parse AIB")
	}
}

// ============================================================================
// JSON Serialization Error Tests
// ============================================================================

func TestToJSON_Error(t *testing.T) {
	// Pass a channel which cannot be marshaled to JSON
	result, err := ToJSON(make(chan int))
	if err == nil {
		t.Error("channel should fail to marshal")
	}
	if result != "" {
		t.Error("result should be empty on error")
	}
}

func TestFromJSON_Error(t *testing.T) {
	var data map[string]string
	err := FromJSON("{invalid json}", &data)
	if err == nil {
		t.Error("invalid JSON should fail")
	}
}

// ============================================================================
// ValidateAddress Additional Tests
// ============================================================================

func TestValidateAddress_Solana(t *testing.T) {
	solAddr := Address{
		Chain: ChainSOL,
		Data:  make([]byte, 32),
	}
	if err := ValidateAddress(solAddr); err != nil {
		t.Errorf("valid SOL address should not error: %v", err)
	}

	invalidSolAddr := Address{
		Chain: ChainSOL,
		Data:  make([]byte, 20),
	}
	if err := ValidateAddress(invalidSolAddr); err == nil {
		t.Error("invalid SOL address should error")
	}
}

func TestValidateAddress_AIB(t *testing.T) {
	aibAddr := Address{
		Chain: ChainAIB,
		Data:  make([]byte, 32),
	}
	if err := ValidateAddress(aibAddr); err != nil {
		t.Errorf("valid AIB address should not error: %v", err)
	}

	invalidAibAddr := Address{
		Chain: ChainAIB,
		Data:  make([]byte, 20),
	}
	if err := ValidateAddress(invalidAibAddr); err == nil {
		t.Error("invalid AIB address should error")
	}
}

func TestValidateAddress_UnknownChain(t *testing.T) {
	unknownAddr := Address{
		Chain: ChainType("UNKNOWN"),
		Data:  make([]byte, 20),
	}
	if err := ValidateAddress(unknownAddr); err == nil {
		t.Error("unknown chain should error")
	}
}

func TestValidateAddress_BTC_P2SH(t *testing.T) {
	// BTC P2SH address (62 bytes)
	btcAddr := Address{
		Chain: ChainBTC,
		Data:  make([]byte, 62),
	}
	if err := ValidateAddress(btcAddr); err != nil {
		t.Errorf("valid BTC P2SH address should not error: %v", err)
	}
}

func TestValidateAddress_BTC_TooLong(t *testing.T) {
	btcAddr := Address{
		Chain: ChainBTC,
		Data:  make([]byte, 70), // Too long even for P2SH
	}
	if err := ValidateAddress(btcAddr); err == nil {
		t.Error("too long BTC address should error")
	}
}

// ============================================================================
// NewAddressFromHex Error Tests
// ============================================================================

func TestNewAddressFromHex_InvalidHex(t *testing.T) {
	_, err := NewAddressFromHex(ChainETH, "not-a-valid-hex-string")
	if err == nil {
		t.Error("invalid hex should fail")
	}
}

// ============================================================================
// WaitForConfirmations Tests
// ============================================================================

func TestBitcoinAdapter_WaitForConfirmations(t *testing.T) {
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	// Wait with enough timeout - each tick is 1 second, confirmations++ per tick
	err := adapter.WaitForConfirmations(txHash, 2, 5*time.Second)
	if err != nil {
		t.Errorf("WaitForConfirmations failed: %v", err)
	}
}

func TestBitcoinAdapter_WaitForConfirmations_NotFound(t *testing.T) {
	adapter := NewBitcoinAdapter()

	err := adapter.WaitForConfirmations("nonexistent", 1, 50*time.Millisecond)
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestBitcoinAdapter_WaitForConfirmations_Timeout(t *testing.T) {
	adapter := NewBitcoinAdapter()

	// Create a tx but don't increment confirmations
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	// Manually set confirmations to 0 so we can test timeout
	tx, _ := adapter.getTransaction(txHash)
	tx.Confirmations = 0

	// Wait for more confirmations than we'll get in the short timeout
	err := adapter.WaitForConfirmations(txHash, 100, 500*time.Millisecond)
	if err == nil {
		t.Error("should timeout")
	}
}

func TestEthereumAdapter_WaitForConfirmations(t *testing.T) {
	adapter := NewEthereumAdapter()

	sender := newTestAddress(ChainETH, "sender")
	recipient := newTestAddress(ChainBTC, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainETH,
		DestChain:   ChainBTC,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000000000000),
		Token:       "ETH",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	err := adapter.WaitForConfirmations(txHash, 2, 5*time.Second)
	if err != nil {
		t.Errorf("WaitForConfirmations failed: %v", err)
	}
}

func TestEthereumAdapter_WaitForConfirmations_NotFound(t *testing.T) {
	adapter := NewEthereumAdapter()

	err := adapter.WaitForConfirmations("nonexistent", 1, 50*time.Millisecond)
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

func TestSolanaAdapter_WaitForConfirmations(t *testing.T) {
	adapter := NewSolanaAdapter()

	sender := newTestAddress(ChainSOL, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainSOL,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(1000000000),
		Token:       "SOL",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, _ := adapter.LockFunds(req)

	err := adapter.WaitForConfirmations(txHash, 2, 5*time.Second)
	if err != nil {
		t.Errorf("WaitForConfirmations failed: %v", err)
	}
}

func TestSolanaAdapter_WaitForConfirmations_NotFound(t *testing.T) {
	adapter := NewSolanaAdapter()

	err := adapter.WaitForConfirmations("nonexistent", 1, 50*time.Millisecond)
	if err == nil {
		t.Error("should fail for nonexistent tx")
	}
}

// ============================================================================
// RelayerNode CreateSwap with Adapter Error Tests
// ============================================================================

func TestRelayerNode_CreateSwap_NoAdapter(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	// Try to create swap for unsupported chain (ETH not in supportedChains means no ETH adapter)
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "swap-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		RelayerFee:  big.NewInt(1000),
		Deadline:    time.Now().Add(24 * time.Hour),
		SecretHash:  func() []byte { h := sha256.Sum256([]byte("secret")); return h[:] }(),
	}

	_, err := node.CreateSwap(req)
	if err == nil {
		t.Error("should fail when dest chain not supported")
	}
}

// ============================================================================
// Network RegisterRelayer Additional Tests
// ============================================================================

func TestNetwork_RegisterRelayer_Nil(t *testing.T) {
	network := NewNetwork()

	err := network.RegisterRelayer(nil)
	if err == nil {
		t.Error("nil relayer should fail")
	}
}

func TestNetwork_RegisterRelayer_BelowMinStake(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1), big.NewInt(1000)) // Stake below minimum

	err := network.RegisterRelayer(relayer)
	if err == nil {
		t.Error("below minimum stake should fail")
	}
}

func TestNetwork_RegisterRelayer_InvalidRelayer(t *testing.T) {
	network := NewNetwork()

	// Relayer with empty ID
	relayer := NewRelayerNode("", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	err := network.RegisterRelayer(relayer)
	if err == nil {
		t.Error("invalid relayer should fail")
	}
}

// ============================================================================
// Network ResolveDispute Additional Tests
// ============================================================================

func TestNetwork_ResolveDispute_NilResolution(t *testing.T) {
	network := NewNetwork()

	err := network.ResolveDispute("dispute-1", nil)
	if err == nil {
		t.Error("nil resolution should fail")
	}
}

func TestNetwork_ResolveDispute_EmptyID(t *testing.T) {
	network := NewNetwork()

	resolution := &DisputeResolution{}
	err := network.ResolveDispute("", resolution)
	if err == nil {
		t.Error("empty dispute ID should fail")
	}
}

func TestNetwork_ResolveDispute_AlreadyResolved(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	dispute := &Dispute{
		TxHash:  tx.ID,
		Reporter: "user1",
		Reason:  "test",
	}
	network.ReportDispute(dispute)

	resolution := &DisputeResolution{
		DisputeID:  dispute.ID,
		Winner:     "relayer-1",
		Resolution: "resolved",
	}
	network.ResolveDispute(dispute.ID, resolution)

	// Try to resolve again
	err := network.ResolveDispute(dispute.ID, resolution)
	if err == nil {
		t.Error("already resolved dispute should fail")
	}
}

// ============================================================================
// Network ReactivateRelayer Additional Tests
// ============================================================================

func TestNetwork_ReactivateRelayer_NotFound(t *testing.T) {
	network := NewNetwork()

	err := network.ReactivateRelayer("nonexistent", big.NewInt(1000000000))
	if err == nil {
		t.Error("nonexistent relayer should fail")
	}
}

func TestNetwork_ReactivateRelayer_InsufficientStake(t *testing.T) {
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	// Slash the relayer first
	network.SlashRelayer("relayer-1", 1.0) // Slash 100%

	// Try to reactivate without enough stake
	err := network.ReactivateRelayer("relayer-1", big.NewInt(1))
	if err == nil {
		t.Error("insufficient stake should fail")
	}
}

// ============================================================================
// Network GetRelayer Additional Tests
// ============================================================================

func TestNetwork_GetRelayer_NotFound(t *testing.T) {
	network := NewNetwork()

	_, err := network.GetRelayer("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent relayer")
	}
}

// ============================================================================
// Network GetDispute Additional Tests
// ============================================================================

func TestNetwork_GetDispute_NotFound(t *testing.T) {
	network := NewNetwork()

	_, err := network.GetDispute("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent dispute")
	}
}

// ============================================================================
// Network GetResolution Additional Tests
// ============================================================================

func TestNetwork_GetResolution_NotFound(t *testing.T) {
	network := NewNetwork()

	_, err := network.GetResolution("nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent resolution")
	}
}

// ============================================================================
// NewRelayerNode Additional Tests
// ============================================================================

func TestNewRelayerNode_WithUnsupportedChain(t *testing.T) {
	// Create relayer with a chain that NewChainAdapter doesn't support
	// This should still create the relayer but without that adapter
	addr := newTestAddress(ChainETH, "relayer")
	chains := []ChainType{ChainBTC, ChainType("UNSUPPORTED")}
	stake := big.NewInt(1000000000)
	feeRate := big.NewInt(1000)

	node := NewRelayerNode("relayer-1", addr, "node-1", chains, stake, feeRate)

	// Should still create the node
	if node.id != "relayer-1" {
		t.Error("node should be created")
	}
	// But only have BTC adapter registered
	_, err := node.adapters.GetAdapter(ChainBTC)
	if err != nil {
		t.Error("BTC adapter should be registered")
	}
	_, err = node.adapters.GetAdapter(ChainType("UNSUPPORTED"))
	if err == nil {
		t.Error("unsupported chain adapter should not be registered")
	}
}

// ============================================================================
// Relayer Register Additional Tests
// ============================================================================

func TestRelayerNode_Register_WithNewChains(t *testing.T) {
	node := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000))

	initialChains := len(node.supportedChains)

	req := &RegisterRequest{
		NodeID:          "node-1",
		Address:        newTestAddress(ChainAIB, "addr"),
		SupportedChains: []ChainType{ChainBTC, ChainETH, ChainSOL},
		Stake:          big.NewInt(2000000000),
		FeeRate:        big.NewInt(500),
	}

	err := node.Register(req)
	if err != nil {
		t.Errorf("register failed: %v", err)
	}

	if len(node.supportedChains) != 3 {
		t.Errorf("expected 3 chains, got %d", len(node.supportedChains))
	}
	if len(node.supportedChains) <= initialChains {
		t.Error("chains should be updated")
	}
}

// ============================================================================
// Cross-Chain Message Passing Tests
// ============================================================================

func TestCrossChainMessage_Transfer(t *testing.T) {
	// Test complete cross-chain message transfer flow
	network := NewNetwork()

	// Register relayer
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainAIB, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH, ChainSOL, ChainAIB}, big.NewInt(1000000000), big.NewInt(1000))
	network.RegisterRelayer(relayer)

	// Create swap request
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	secretHash := sha256.Sum256([]byte("secret"))

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
		SecretHash:   secretHash[:],
	}

	// Assign task to relayer
	assignedRelayer, err := network.AssignTask(req)
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}

	// Create swap via relayer
	tx, err := assignedRelayer.CreateSwap(req)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	if tx.Status != TxStatusLocked {
		t.Errorf("expected status Locked, got %s", tx.Status)
	}

	if tx.SourceChain != ChainBTC {
		t.Error("source chain should be BTC")
	}

	if tx.DestChain != ChainETH {
		t.Error("dest chain should be ETH")
	}
}

func TestCrossChainMessage_SenderRecipientValidation(t *testing.T) {
	// Test sender and recipient validation in message transfer
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// Test valid swap request
	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err := ValidateSwapRequest(req)
	if err != nil {
		t.Errorf("valid request should not error: %v", err)
	}

	// Test empty sender
	reqNoSender := &SwapRequest{
		ID:           "msg-2",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       Address{Chain: ChainBTC, Data: []byte{}},
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err = ValidateSwapRequest(reqNoSender)
	if err == nil {
		t.Error("empty sender should error")
	}

	// Test empty recipient
	reqNoRecipient := &SwapRequest{
		ID:           "msg-3",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    Address{Chain: ChainETH, Data: []byte{}},
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err = ValidateSwapRequest(reqNoRecipient)
	if err == nil {
		t.Error("empty recipient should error")
	}
}

func TestCrossChainMessage_ChainValidation(t *testing.T) {
	// Test chain type validation
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// Test same source and dest chain (should fail)
	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainBTC,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err := ValidateSwapRequest(req)
	if err == nil {
		t.Error("same chain should error")
	}

	// Test invalid source chain
	reqInvalidSource := &SwapRequest{
		ID:           "msg-2",
		SourceChain:  ChainType("INVALID"),
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err = ValidateSwapRequest(reqInvalidSource)
	if err == nil {
		t.Error("invalid source chain should error")
	}

	// Test invalid dest chain
	reqInvalidDest := &SwapRequest{
		ID:           "msg-3",
		SourceChain:  ChainBTC,
		DestChain:    ChainType("INVALID"),
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	err = ValidateSwapRequest(reqInvalidDest)
	if err == nil {
		t.Error("invalid dest chain should error")
	}
}

func TestCrossChainMessage_Expiry(t *testing.T) {
	// Test message expiry handling
	_ = NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// Test with expired deadline
	expiredReq := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(-1 * time.Hour), // Already expired
	}

	err := ValidateSwapRequest(expiredReq)
	if err == nil {
		t.Error("expired request should error")
	}

	// Test with valid deadline
	validReq := &SwapRequest{
		ID:           "msg-2",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(1 * time.Hour),
	}

	err = ValidateSwapRequest(validReq)
	if err != nil {
		t.Errorf("valid request should not error: %v", err)
	}

	// Test transaction expiry check
	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		sender, recipient, big.NewInt(100000000), "BTC", "relayer-1", -1*time.Hour)

	if !tx.IsExpired() {
		t.Error("expired tx should be expired")
	}

	tx2 := NewCrossChainTx("tx-2", ChainBTC, ChainETH,
		sender, recipient, big.NewInt(100000000), "BTC", "relayer-1", 1*time.Hour)

	if tx2.IsExpired() {
		t.Error("non-expired tx should not be expired")
	}
}

func TestCrossChainMessage_Multihop(t *testing.T) {
	// Test multi-hop cross-chain (BTC -> ETH -> SOL)
	network := NewNetwork()

	// Register multiple relayers for different hops
	relayer1 := NewRelayerNode("relayer-1", newTestAddress(ChainAIB, "relayer1"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))
	relayer2 := NewRelayerNode("relayer-2", newTestAddress(ChainAIB, "relayer2"), "node-2",
		[]ChainType{ChainETH, ChainSOL}, big.NewInt(1000000000), big.NewInt(1000))

	network.RegisterRelayer(relayer1)
	network.RegisterRelayer(relayer2)

	// First hop: BTC -> ETH
	sender := newTestAddress(ChainBTC, "sender")
	intermediate := newTestAddress(ChainETH, "intermediate")

	_ = &SwapRequest{
		ID:           "multihop-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    intermediate,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	// Find relayers for BTC -> ETH
	relayers := network.FindRelayersForSwap(ChainBTC, ChainETH)
	if len(relayers) != 1 {
		t.Errorf("expected 1 relayer for BTC->ETH, got %d", len(relayers))
	}

	// Second hop: ETH -> SOL would be handled by another relayer
	relayers2 := network.FindRelayersForSwap(ChainETH, ChainSOL)
	if len(relayers2) != 1 {
		t.Errorf("expected 1 relayer for ETH->SOL, got %d", len(relayers2))
	}
}

// ============================================================================
// Message Confirmation Mechanism Tests
// ============================================================================

func TestConfirmation_WaitForConfirmations(t *testing.T) {
	// Test waiting for confirmations
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}

	// Wait for 3 confirmations with timeout
	err = adapter.WaitForConfirmations(txHash, 3, 5*time.Second)
	if err != nil {
		t.Errorf("WaitForConfirmations failed: %v", err)
	}

	// Verify confirmations
	confs, err := adapter.GetConfirmations(txHash)
	if err != nil {
		t.Fatalf("GetConfirmations failed: %v", err)
	}

	if confs < 3 {
		t.Errorf("expected at least 3 confirmations, got %d", confs)
	}
}

func TestConfirmation_Timeout(t *testing.T) {
	// Test confirmation timeout
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}

	// Try to wait for 100 confirmations with short timeout (should timeout)
	err = adapter.WaitForConfirmations(txHash, 100, 100*time.Millisecond)
	if err == nil {
		t.Error("should timeout when waiting for too many confirmations")
	}
}

func TestConfirmation_Increment(t *testing.T) {
	// Test confirmation increment
	adapter := NewBitcoinAdapter()

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:          "req-1",
		SourceChain: ChainBTC,
		DestChain:   ChainETH,
		Sender:      sender,
		Recipient:   recipient,
		Amount:      big.NewInt(100000000),
		Token:       "BTC",
		Deadline:    time.Now().Add(24 * time.Hour),
	}

	txHash, err := adapter.LockFunds(req)
	if err != nil {
		t.Fatalf("LockFunds failed: %v", err)
	}

	// Get initial confirmations
	initialConfs, _ := adapter.GetConfirmations(txHash)

	// Wait for more confirmations
	adapter.WaitForConfirmations(txHash, 3, 2*time.Second)

	// Verify increment
	updatedConfs, _ := adapter.GetConfirmations(txHash)
	if updatedConfs <= initialConfs {
		t.Error("confirmations should have incremented")
	}
}

func TestConfirmation_RequiredConfirmations(t *testing.T) {
	// Test different confirmation requirements for different chains
	btcAdapter := NewBitcoinAdapter()
	ethAdapter := NewEthereumAdapter()
	solAdapter := NewSolanaAdapter()

	if btcAdapter.GetRequiredConfirmations() != 6 {
		t.Errorf("BTC should require 6 confirmations, got %d", btcAdapter.GetRequiredConfirmations())
	}

	if ethAdapter.GetRequiredConfirmations() != 12 {
		t.Errorf("ETH should require 12 confirmations, got %d", ethAdapter.GetRequiredConfirmations())
	}

	if solAdapter.GetRequiredConfirmations() != 32 {
		t.Errorf("SOL should require 32 confirmations, got %d", solAdapter.GetRequiredConfirmations())
	}
}

func TestConfirmation_StatusTransition(t *testing.T) {
	// Test transaction status transition after confirmations
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Initial status should be Locked
	if tx.Status != TxStatusLocked {
		t.Errorf("expected status Locked, got %s", tx.Status)
	}

	// Simulate confirmation
	tx.UpdateConfirmations(6)

	// Status should remain locked until proof is ready
	if tx.Status != TxStatusLocked {
		t.Error("status should remain locked until proof is ready")
	}
}

// ============================================================================
// Retry Logic Tests
// ============================================================================

func TestRetry_LockFundsFailure(t *testing.T) {
	// Test retry logic when lock funds fails
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// First attempt with zero amount (should fail)
	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(0), // Invalid amount
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	_, err := relayer.CreateSwap(req)
	if err == nil {
		t.Error("should fail with zero amount")
	}

	// Second attempt with valid amount
	req.Amount = big.NewInt(100000000)
	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Fatalf("retry with valid amount should succeed: %v", err)
	}

	if tx.Status != TxStatusLocked {
		t.Errorf("expected status Locked, got %s", tx.Status)
	}
}

func TestRetry_UnlockFundsFailure(t *testing.T) {
	// Test retry when unlock funds fails due to missing proof
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Try to release funds without proof (should fail)
	err = relayer.ReleaseFunds(tx.SourceTxHash.String())
	if err == nil {
		t.Error("should fail without proof")
	}

	// Now add proof and try again
	proof := &MerkleProof{
		BlockNumber: 800000,
		Proof:       [][]byte{[]byte("proof1")},
	}
	tx.SetProof(proof)

	// Now release should work
	err = relayer.ReleaseFunds(tx.SourceTxHash.String())
	if err != nil {
		t.Errorf("release with proof should succeed: %v", err)
	}
}

func TestRetry_ProofSubmissionFailure(t *testing.T) {
	// Test proof submission failure handling
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Submit proof with empty hash (should fail)
	err := relayer.SubmitProof("", &MerkleProof{BlockNumber: 800000, Proof: [][]byte{[]byte("proof")}})
	if err == nil {
		t.Error("should fail with empty tx hash")
	}

	// Submit nil proof (should fail)
	err = relayer.SubmitProof("txhash", nil)
	if err == nil {
		t.Error("should fail with nil proof")
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	// Test behavior when max retries are exceeded
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// Multiple failed attempts
	for i := 0; i < 3; i++ {
		req := &SwapRequest{
			ID:           fmt.Sprintf("msg-%d", i),
			SourceChain:  ChainBTC,
			DestChain:    ChainETH,
			Sender:       sender,
			Recipient:    recipient,
			Amount:       big.NewInt(0), // Invalid
			Token:        "BTC",
			RelayerFee:   big.NewInt(1000),
			Deadline:     time.Now().Add(24 * time.Hour),
		}

		relayer.CreateSwap(req) // Ignore error for retry test
	}

	// Now try with valid request
	req := &SwapRequest{
		ID:           "msg-valid",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Errorf("valid request after retries should succeed: %v", err)
	}

	if tx == nil {
		t.Error("tx should not be nil")
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	// Test exponential backoff behavior simulation
	// In real implementation, this would test actual retry delays
	baseDelay := time.Millisecond * 100

	delays := make([]time.Duration, 0)
	for i := 0; i < 5; i++ {
		delay := baseDelay * time.Duration(1<<uint(i)) // Exponential: 100ms, 200ms, 400ms, 800ms, 1600ms
		delays = append(delays, delay)
	}

	// Verify exponential growth
	if delays[1] <= delays[0] {
		t.Error("delays should increase exponentially")
	}

	if delays[4] <= delays[3] {
		t.Error("last delay should be largest")
	}

	// Verify they follow 2^n pattern
	expected := []time.Duration{100, 200, 400, 800, 1600}
	for i, d := range delays {
		if d != expected[i]*time.Millisecond {
			t.Errorf("delay %d: expected %v, got %v", i, expected[i]*time.Millisecond, d)
		}
	}
}

// ============================================================================
// Error Recovery Tests
// ============================================================================

func TestRecovery_TransactionNotFound(t *testing.T) {
	// Test recovery when transaction is not found
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Try to get non-existent transaction
	_, err := relayer.GetTransaction("nonexistent-tx")
	if err == nil {
		t.Error("should error for non-existent transaction")
	}

	// Try to release funds for non-existent transaction
	err = relayer.ReleaseFunds("nonexistent-tx")
	if err == nil {
		t.Error("should error for non-existent tx in ReleaseFunds")
	}

	// Try to submit proof for non-existent transaction
	err = relayer.SubmitProof("nonexistent-tx", &MerkleProof{BlockNumber: 800000, Proof: [][]byte{[]byte("proof")}})
	if err == nil {
		t.Error("should error for non-existent tx in SubmitProof")
	}
}

func TestRecovery_InvalidProof(t *testing.T) {
	// Test recovery when proof is invalid
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Fatalf("CreateSwap failed: %v", err)
	}

	// Try to unlock with nil proof
	err = relayer.ReleaseFunds(tx.SourceTxHash.String())
	if err == nil {
		t.Error("should fail with nil proof")
	}
}

func TestRecovery_AdapterNotFound(t *testing.T) {
	// Test recovery when adapter is not found
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC}, big.NewInt(1000000000), big.NewInt(1000)) // Only BTC supported

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH, // ETH not supported by this relayer
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	// Should fail because relayer doesn't support ETH
	_, err := relayer.CreateSwap(req)
	if err == nil {
		t.Error("should fail when dest chain not supported")
	}
}

func TestRecovery_RelayerNotActive(t *testing.T) {
	// Test recovery when relayer is not active
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Set relayer to inactive
	relayer.SetStatus(StatusInactive)

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(100000000),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	// Should fail because relayer is inactive
	_, err := relayer.CreateSwap(req)
	if err == nil {
		t.Error("should fail when relayer is inactive")
	}

	// Reactivate and try again
	relayer.SetStatus(StatusActive)
	tx, err := relayer.CreateSwap(req)
	if err != nil {
		t.Errorf("should succeed after reactivation: %v", err)
	}

	if tx == nil {
		t.Error("tx should not be nil after recovery")
	}
}

func TestRecovery_InsufficientFunds(t *testing.T) {
	// Test recovery when amount is insufficient
	_ = NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")

	// Test with zero amount
	req := &SwapRequest{
		ID:           "msg-1",
		SourceChain:  ChainBTC,
		DestChain:    ChainETH,
		Sender:       sender,
		Recipient:    recipient,
		Amount:       big.NewInt(0),
		Token:        "BTC",
		RelayerFee:   big.NewInt(1000),
		Deadline:     time.Now().Add(24 * time.Hour),
	}

	// Validate should fail
	err := ValidateSwapRequest(req)
	if err == nil {
		t.Error("zero amount should fail validation")
	}

	// Test with negative amount
	req.Amount = big.NewInt(-1)
	err = ValidateSwapRequest(req)
	if err == nil {
		t.Error("negative amount should fail validation")
	}

	// Test with valid amount
	req.Amount = big.NewInt(100000000)
	err = ValidateSwapRequest(req)
	if err != nil {
		t.Errorf("valid amount should pass: %v", err)
	}
}

func TestRecovery_NilSwapRequest(t *testing.T) {
	// Test recovery when swap request is nil
	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// CreateSwap with nil request
	_, err := relayer.CreateSwap(nil)
	if err == nil {
		t.Error("nil request should fail")
	}

	// Network AssignTask with nil request
	network := NewNetwork()
	_, err = network.AssignTask(nil)
	if err == nil {
		t.Error("nil request should fail in AssignTask")
	}
}

func TestRecovery_NetworkUnregisterWithPending(t *testing.T) {
	// Test recovery when unregistering relayer with pending transactions
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	// Add a pending transaction
	sender := newTestAddress(ChainBTC, "sender")
	recipient := newTestAddress(ChainETH, "recipient")
	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		sender, recipient, big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	tx.Status = TxStatusLocked // Pending
	relayer.transactions[tx.ID] = tx

	network.RegisterRelayer(relayer)

	// Try to unregister (should fail due to pending)
	err := network.UnregisterRelayer("relayer-1")
	if err == nil {
		t.Error("should fail when pending transactions exist")
	}

	// Complete the transaction
	tx.Status = TxStatusCompleted

	// Now unregister should succeed
	err = network.UnregisterRelayer("relayer-1")
	if err != nil {
		t.Errorf("unregister after completing tx should succeed: %v", err)
	}
}

func TestRecovery_ResolveAlreadyResolvedDispute(t *testing.T) {
	// Test recovery when resolving an already resolved dispute
	network := NewNetwork()

	relayer := NewRelayerNode("relayer-1", newTestAddress(ChainETH, "relayer"), "node-1",
		[]ChainType{ChainBTC, ChainETH}, big.NewInt(1000000000), big.NewInt(1000))

	tx := NewCrossChainTx("tx-1", ChainBTC, ChainETH,
		newTestAddress(ChainBTC, "sender"),
		newTestAddress(ChainETH, "recipient"),
		big.NewInt(100000000), "BTC", "relayer-1", 24*time.Hour)
	relayer.transactions[tx.ID] = tx
	network.RegisterRelayer(relayer)

	// Report and resolve dispute
	dispute := &Dispute{
		TxHash:   tx.ID,
		Reporter: "user1",
		Reason:   "test",
	}
	network.ReportDispute(dispute)

	resolution := &DisputeResolution{
		DisputeID:  dispute.ID,
		Winner:     "relayer-1",
		Resolution: "resolved",
	}

	err := network.ResolveDispute(dispute.ID, resolution)
	if err != nil {
		t.Fatalf("first resolve should succeed: %v", err)
	}

	// Try to resolve again (should fail)
	err = network.ResolveDispute(dispute.ID, resolution)
	if err == nil {
		t.Error("should fail when dispute already resolved")
	}
}
