// Package relayer provides unit tests for the cross-chain relayer functionality.
package relayer

import (
	"crypto/sha256"
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
