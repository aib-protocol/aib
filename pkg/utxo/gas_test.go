// Package utxo - Gas/Fee Optimization Tests
// Tests for transaction fee calculation accuracy, gas limits, optimization, and cost analysis.
package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Helper: create a signed transaction with real UTXO backing
// ============================================================================

// testUTXOProvider is a minimal UTXOProvider for fee-related tests.
type testUTXOProvider struct {
	utxos map[string]*UTXO
}

func newTestUTXOProvider() *testUTXOProvider {
	return &testUTXOProvider{utxos: make(map[string]*UTXO)}
}

func (p *testUTXOProvider) add(u *UTXO) {
	key := UTXOKey(u.TxHash, u.Index)
	p.utxos[key] = u
}

func (p *testUTXOProvider) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	key := UTXOKey(txHash, index)
	u, ok := p.utxos[key]
	if !ok {
		return nil, fmt.Errorf("UTXO not found: %s", key)
	}
	return u, nil
}

// createSignedTx builds a transaction with properly signed inputs.
// It ensures PublicKey is set BEFORE signing so that SerializeForSigning matches during verification.
func createSignedTx(t *testing.T, inputs []TXInput, outputs []TXOutput, privKeys []ed25519.PrivateKey) *Transaction {
	t.Helper()
	if len(inputs) != len(privKeys) {
		t.Fatalf("inputs length %d != privKeys length %d", len(inputs), len(privKeys))
	}

	// Build inputs with PublicKey set (required for correct serialization before signing)
	signedInputs := make([]TXInput, len(inputs))
	for i, pk := range privKeys {
		signedInputs[i] = TXInput{
			TxHash:    inputs[i].TxHash,
			Index:     inputs[i].Index,
			PublicKey: pk.Public().(ed25519.PublicKey),
		}
	}

	tx := NewTransaction(signedInputs, outputs)

	// Sign each input (signature will be stored)
	for i, pk := range privKeys {
		if err := tx.SignInput(i, pk); err != nil {
			t.Fatalf("SignInput(%d): %v", i, err)
		}
	}

	return tx
}

// makeKeyPair generates a deterministic key pair and returns address, pub, priv.
func makeKeyPair(t *testing.T) ([32]byte, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	addr := sha256.Sum256(pub)
	return addr, pub, priv
}

// ============================================================================
// 1. Gas (Fee) Calculation Accuracy Tests
// ============================================================================

func TestFeeCalculation_BasicAccuracy(t *testing.T) {
	// Test that CalculateFee returns size * feePerByte for a non-coinbase tx.
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("prev-tx"))

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 500, Script: []byte("pay"), Address: addr}}

	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	feePerByte := uint64(10) // 10 satoshi/byte
	calculatedFee := tx.CalculateFee(feePerByte)
	expectedFee := uint64(tx.SerializeSize()) * feePerByte

	if calculatedFee != expectedFee {
		t.Errorf("CalculateFee = %d, expected %d (size=%d, rate=%d)",
			calculatedFee, expectedFee, tx.SerializeSize(), feePerByte)
	}
}

func TestFeeCalculation_MinimumFeeFloor(t *testing.T) {
	// Even with feePerByte=0, minimum fee should be 1 satoshi (not zero).
	inputs := []TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: []byte("s"), PublicKey: []byte("p")}}
	outputs := []TXOutput{{Value: 100, Address: [32]byte{2}}}
	tx := NewTransaction(inputs, outputs)

	fee := tx.CalculateFee(0) // 0 satoshi/byte => size*0 = 0, floor = 1
	if fee != 1 {
		t.Errorf("CalculateFee with rate=0 should be 1 (minimum), got %d", fee)
	}
}

func TestFeeCalculation_CoinbaseAlwaysZero(t *testing.T) {
	// Coinbase transactions must always have 0 fee regardless of fee rate.
	coinbase := CreateCoinbaseTransaction([32]byte{1}, 10*1e8, []byte("block 1"))

	for _, rate := range []uint64{0, 1, 10, 100, 1000} {
		fee := coinbase.CalculateFee(rate)
		if fee != 0 {
			t.Errorf("Coinbase CalculateFee(rate=%d) = %d, expected 0", rate, fee)
		}
	}
}

func TestFeeCalculation_ActualFee_InputsMinusOutputs(t *testing.T) {
	// GetFee should return inputsValue - outputsValue.
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("funding-tx"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9500, Address: [32]byte{9}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	fee, err := tx.GetFee(provider)
	if err != nil {
		t.Fatalf("GetFee: %v", err)
	}

	expectedFee := uint64(10000 - 9500)
	if fee != expectedFee {
		t.Errorf("GetFee = %d, expected %d", fee, expectedFee)
	}
}

func TestFeeCalculation_MultiInput(t *testing.T) {
	// Fee with multiple inputs: sum of all input values - sum of all output values.
	addr, _, priv := makeKeyPair(t)
	hash1 := sha256.Sum256([]byte("utxo-1"))
	hash2 := sha256.Sum256([]byte("utxo-2"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 5000, Address: addr})
	provider.add(&UTXO{TxHash: hash2, Index: 0, Value: 3000, Address: addr})

	inputs := []TXInput{
		{TxHash: hash1, Index: 0},
		{TxHash: hash2, Index: 0},
	}
	outputs := []TXOutput{
		{Value: 6000, Address: [32]byte{10}},
		{Value: 1500, Address: addr}, // change output
	}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv, priv})

	fee, err := tx.GetFee(provider)
	if err != nil {
		t.Fatalf("GetFee: %v", err)
	}
	// 5000+3000 = 8000, 6000+1500 = 7500, fee = 500
	if fee != 500 {
		t.Errorf("GetFee = %d, expected 500", fee)
	}
}

func TestFeeCalculation_MultiOutput(t *testing.T) {
	// Fee with many outputs.
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("big-utxo"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 100000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{
		{Value: 30000, Address: [32]byte{1}},
		{Value: 20000, Address: [32]byte{2}},
		{Value: 10000, Address: [32]byte{3}},
		{Value: 5000, Address: [32]byte{4}},
		{Value: 34000, Address: addr}, // change
	}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	fee, err := tx.GetFee(provider)
	if err != nil {
		t.Fatalf("GetFee: %v", err)
	}
	// 100000 - (30000+20000+10000+5000+34000) = 1000
	if fee != 1000 {
		t.Errorf("GetFee = %d, expected 1000", fee)
	}
}

func TestSerializeSize_Consistency(t *testing.T) {
	// SerializeSize should match the actual length of Serialize().
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("size-test"))

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 1000, Script: []byte("test-script"), Address: addr}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	reportedSize := tx.SerializeSize()
	actualSize := len(tx.Serialize())

	// SerializeSize does NOT count the Sequence field (8 bytes) which Serialize includes.
	// We verify the reported size is consistent with fee calculation purpose.
	if reportedSize <= 0 {
		t.Errorf("SerializeSize = %d, must be positive", reportedSize)
	}

	// The reported size should be close to (within Sequence field difference) the actual serialized data.
	diff := actualSize - reportedSize
	if diff < 0 || diff > 8 {
		t.Errorf("SerializeSize=%d vs Serialize()=%d, unexpected diff=%d", reportedSize, actualSize, diff)
	}
}

func TestSerializeSize_GrowsWithInputsAndOutputs(t *testing.T) {
	// Adding more inputs/outputs should increase the serialized size.
	tx1 := NewTransaction(
		[]TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)}},
		[]TXOutput{{Value: 100, Address: [32]byte{2}}},
	)
	tx2 := NewTransaction(
		[]TXInput{
			{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)},
			{TxHash: [32]byte{2}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)},
		},
		[]TXOutput{
			{Value: 50, Address: [32]byte{3}},
			{Value: 50, Address: [32]byte{4}},
		},
	)

	size1 := tx1.SerializeSize()
	size2 := tx2.SerializeSize()

	if size2 <= size1 {
		t.Errorf("tx with 2 inputs + 2 outputs (size=%d) should be larger than 1+1 (size=%d)",
			size2, size1)
	}
}

// ============================================================================
// 2. Gas (Fee) Limit Tests
// ============================================================================

func TestMinTransactionFee_Constant(t *testing.T) {
	// Verify the constant is set to 100 satoshi.
	if MinTransactionFee != 100 {
		t.Errorf("MinTransactionFee = %d, expected 100", MinTransactionFee)
	}
}

func TestValidateTransactionMinFee_Passes(t *testing.T) {
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("min-fee-pass"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	// Leave 200 satoshi as fee (> MinTransactionFee=100)
	outputs := []TXOutput{{Value: 9800, Address: [32]byte{5}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := ValidateTransactionMinFee(tx, provider)
	if err != nil {
		t.Errorf("ValidateTransactionMinFee should pass with fee=200: %v", err)
	}
}

func TestValidateTransactionMinFee_ExactMinimum(t *testing.T) {
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("exact-min"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	// Exactly MinTransactionFee (100) as fee
	outputs := []TXOutput{{Value: 10000 - MinTransactionFee, Address: [32]byte{6}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := ValidateTransactionMinFee(tx, provider)
	if err != nil {
		t.Errorf("ValidateTransactionMinFee should pass with exact minimum fee: %v", err)
	}
}

func TestValidateTransactionMinFee_BelowMinimum(t *testing.T) {
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("below-min"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	// Only 50 satoshi fee (< MinTransactionFee=100)
	outputs := []TXOutput{{Value: 9950, Address: [32]byte{7}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := ValidateTransactionMinFee(tx, provider)
	if err == nil {
		t.Error("ValidateTransactionMinFee should reject fee below minimum")
	}
}

func TestValidateTransactionMinFee_CoinbaseSkipped(t *testing.T) {
	coinbase := CreateCoinbaseTransaction([32]byte{1}, 10*1e8, nil)

	err := ValidateTransactionMinFee(coinbase, nil) // provider not needed for coinbase
	if err != nil {
		t.Errorf("ValidateTransactionMinFee should skip coinbase: %v", err)
	}
}

func TestGetFee_RejectsNegativeFee(t *testing.T) {
	// When outputs > inputs, GetFee should return an error.
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("neg-fee"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 1000, Address: addr})

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 2000, Address: [32]byte{8}}} // more than input
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	_, err := tx.GetFee(provider)
	if err == nil {
		t.Error("GetFee should reject when outputs > inputs (negative fee)")
	}
}

func TestMaxBlockSize_Constant(t *testing.T) {
	if MaxBlockSize != 1_000_000 {
		t.Errorf("MaxBlockSize = %d, expected 1000000 (1 MB)", MaxBlockSize)
	}
}

func TestValidateTransactionFees_Function(t *testing.T) {
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("validate-fees"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	// Transaction with fee = 500 (above minimum)
	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9500, Address: [32]byte{11}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := ValidateTransactionFees(tx, provider)
	if err != nil {
		t.Errorf("ValidateTransactionFees should pass: %v", err)
	}
}

func TestValidateTransactionFees_TooLow(t *testing.T) {
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("fees-too-low"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	// Fee = 10 (below MinTransactionFee=100)
	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9990, Address: [32]byte{12}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := ValidateTransactionFees(tx, provider)
	if err == nil {
		t.Error("ValidateTransactionFees should reject fee below minimum")
	}
}

// ============================================================================
// 3. Gas (Fee) Optimization Tests
// ============================================================================

func TestFeeRate_Calculation(t *testing.T) {
	// Fee rate = fee / txSize. Verify the mempool calculates it correctly.
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("rate-calc"))

	store := NewUTXOStore()
	store.AddUTXO(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	mempool := NewMempool(100, 0) // minFee=0 to allow any fee

	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9000, Address: [32]byte{13}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := mempool.AddTransaction(tx, store)
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	rates := mempool.GetFeeRates()
	if len(rates) != 1 {
		t.Fatalf("expected 1 rate, got %d", len(rates))
	}

	// fee = 10000 - 9000 = 1000
	// feeRate = 1000 / txSize
	txSize := tx.SerializeSize()
	if txSize == 0 {
		txSize = 1
	}
	expectedRate := float64(1000) / float64(txSize)

	if math.Abs(rates[0]-expectedRate) > 0.001 {
		t.Errorf("FeeRate = %f, expected %f", rates[0], expectedRate)
	}
}

func TestMempoolFeeRateSorting(t *testing.T) {
	// When multiple transactions are in mempool, GetTransactionsForBlock
	// should return them sorted by fee rate (highest first).
	store := NewUTXOStore()

	// Create 3 transactions with different fee rates.
	type txData struct {
		seed      string
		inputVal  uint64
		outputVal uint64
	}

	txSpecs := []txData{
		{"low-fee", 10000, 9900},   // fee=100, low rate
		{"high-fee", 10000, 8000},  // fee=2000, high rate
		{"mid-fee", 10000, 9000},   // fee=1000, mid rate
	}

	mempool := NewMempool(100, 0)

	for _, spec := range txSpecs {
		addr, _, priv := makeKeyPair(t)
		prevHash := sha256.Sum256([]byte(spec.seed))
		store.AddUTXO(&UTXO{TxHash: prevHash, Index: 0, Value: spec.inputVal, Address: addr})

		inputs := []TXInput{{TxHash: prevHash, Index: 0}}
		outputs := []TXOutput{{Value: spec.outputVal, Address: [32]byte{14}}}
		tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

		if err := mempool.AddTransaction(tx, store); err != nil {
			t.Fatalf("AddTransaction(%s): %v", spec.seed, err)
		}
	}

	rates := mempool.GetFeeRates()
	if len(rates) != 3 {
		t.Fatalf("expected 3 rates, got %d", len(rates))
	}

	// Rates should be in descending order.
	if !sort.Float64sAreSorted(reverseFloat64(rates)) {
		// GetFeeRates returns descending, so the slice itself should be sorted descending.
		for i := 1; i < len(rates); i++ {
			if rates[i] > rates[i-1] {
				t.Errorf("rates not sorted descending at index %d: %v", i, rates)
				break
			}
		}
	}
}

// reverseFloat64 returns a copy in ascending order to check if descending sort is correct.
func reverseFloat64(s []float64) []float64 {
	r := make([]float64, len(s))
	for i, v := range s {
		r[len(s)-1-i] = v
	}
	return r
}

func TestBlockTransactionSelection_HighFeeRateFirst(t *testing.T) {
	// GetTransactionsForBlock should select high-fee-rate transactions first
	// when block space is limited.
	store := NewUTXOStore()
	mempool := NewMempool(100, 0)

	var txFees []uint64

	// Add 5 transactions with increasing fees.
	for i := 0; i < 5; i++ {
		addr, _, priv := makeKeyPair(t)
		prevHash := sha256.Sum256([]byte{byte(i), 42})
		inputVal := uint64(50000)
		fee := uint64((i + 1) * 500) // 500, 1000, 1500, 2000, 2500
		outputVal := inputVal - fee
		txFees = append(txFees, fee)

		store.AddUTXO(&UTXO{TxHash: prevHash, Index: 0, Value: inputVal, Address: addr})

		inputs := []TXInput{{TxHash: prevHash, Index: 0}}
		outputs := []TXOutput{{Value: outputVal, Address: [32]byte{15}}}
		tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

		if err := mempool.AddTransaction(tx, store); err != nil {
			t.Fatalf("AddTransaction[%d]: %v", i, err)
		}
	}

	// Get transactions for a small block (only fits ~2 transactions).
	// Estimate each tx is ~200 bytes, so limit to 400.
	selected := mempool.GetTransactionsForBlock(400)

	if len(selected) == 0 {
		t.Fatal("expected at least one transaction selected for block")
	}

	// The selected transactions should have the highest fee rates.
	// Verify the first selected has a high fee.
	firstFee, err := selected[0].GetFee(store)
	if err != nil {
		t.Fatalf("GetFee of selected[0]: %v", err)
	}
	if firstFee < 1500 {
		t.Errorf("first selected tx fee=%d, expected a high-fee tx (>=1500)", firstFee)
	}
}

func TestFeeEfficiency_LargerScriptHigherCost(t *testing.T) {
	// Transactions with larger scripts should cost more in fees.
	config := DefaultPoSConfig()
	feeRate := config.BaseFeePerByte

	tx1 := NewTransaction(
		[]TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)}},
		[]TXOutput{{Value: 100, Script: []byte("short"), Address: [32]byte{2}}},
	)

	longScript := make([]byte, 500)
	for i := range longScript {
		longScript[i] = byte(i % 256)
	}
	tx2 := NewTransaction(
		[]TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)}},
		[]TXOutput{{Value: 100, Script: longScript, Address: [32]byte{2}}},
	)

	fee1 := tx1.CalculateFee(feeRate)
	fee2 := tx2.CalculateFee(feeRate)

	if fee2 <= fee1 {
		t.Errorf("larger script tx fee (%d) should exceed small script fee (%d)", fee2, fee1)
	}
}

func TestFeePerByte_DefaultConfig(t *testing.T) {
	config := DefaultPoSConfig()

	if config.BaseFeePerByte != 10 {
		t.Errorf("BaseFeePerByte = %d, expected 10", config.BaseFeePerByte)
	}
	if config.PriorityFeePerByte != 20 {
		t.Errorf("PriorityFeePerByte = %d, expected 20", config.PriorityFeePerByte)
	}
	if config.PriorityFeePerByte <= config.BaseFeePerByte {
		t.Error("PriorityFeePerByte should be greater than BaseFeePerByte")
	}
}

func TestCalculateFee_DifferentRates(t *testing.T) {
	// Same transaction, different fee rates should yield proportional fees.
	tx := NewTransaction(
		[]TXInput{{TxHash: [32]byte{1}, Index: 0, Signature: make([]byte, 64), PublicKey: make([]byte, 32)}},
		[]TXOutput{{Value: 1000, Address: [32]byte{2}}},
	)

	baseFee := tx.CalculateFee(10)
	priorityFee := tx.CalculateFee(20)

	// Priority fee should be exactly 2x base fee (same tx size, 2x rate).
	if priorityFee != baseFee*2 {
		t.Errorf("priorityFee=%d should be 2x baseFee=%d", priorityFee, baseFee)
	}
}

// ============================================================================
// 4. Gas (Fee) Cost Analysis Tests
// ============================================================================

func TestBlockTotalTransactionFees(t *testing.T) {
	// Verify GetTotalTransactionFees sums fees of all non-coinbase transactions.
	addr, _, priv := makeKeyPair(t)
	proposerAddr := [32]byte{99}

	provider := newTestUTXOProvider()

	// Create 3 funding UTXOs
	hash1 := sha256.Sum256([]byte("funding-1"))
	hash2 := sha256.Sum256([]byte("funding-2"))
	hash3 := sha256.Sum256([]byte("funding-3"))
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 10000, Address: addr})
	provider.add(&UTXO{TxHash: hash2, Index: 0, Value: 20000, Address: addr})
	provider.add(&UTXO{TxHash: hash3, Index: 0, Value: 30000, Address: addr})

	// Create 3 transactions with known fees
	// tx1: 10000 - 9700 = fee 300
	tx1 := createSignedTx(t, []TXInput{{TxHash: hash1, Index: 0}},
		[]TXOutput{{Value: 9700, Address: [32]byte{20}}}, []ed25519.PrivateKey{priv})
	// tx2: 20000 - 19500 = fee 500
	tx2 := createSignedTx(t, []TXInput{{TxHash: hash2, Index: 0}},
		[]TXOutput{{Value: 19500, Address: [32]byte{21}}}, []ed25519.PrivateKey{priv})
	// tx3: 30000 - 29200 = fee 800
	tx3 := createSignedTx(t, []TXInput{{TxHash: hash3, Index: 0}},
		[]TXOutput{{Value: 29200, Address: [32]byte{22}}}, []ed25519.PrivateKey{priv})

	totalFees := uint64(300 + 500 + 800) // 1600

	// Create block with coinbase + 3 transactions
	coinbase := CreateCoinbaseWithFees(proposerAddr, 10*1e8, totalFees, nil)
	block := NewBlock([]*Transaction{coinbase, tx1, tx2, tx3}, [32]byte{}, 1, proposerAddr)

	// Calculate total fees
	fees, err := block.GetTotalTransactionFees(provider)
	if err != nil {
		t.Fatalf("GetTotalTransactionFees: %v", err)
	}

	if fees != totalFees {
		t.Errorf("GetTotalTransactionFees = %d, expected %d", fees, totalFees)
	}
}

func TestBlockReward_SubsidyPlusFees(t *testing.T) {
	// CalculateTotalReward = blockSubsidy + fees.
	addr, _, priv := makeKeyPair(t)
	proposerAddr := [32]byte{99}

	provider := newTestUTXOProvider()
	hash1 := sha256.Sum256([]byte("reward-test"))
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 50000, Address: addr})

	// tx with fee = 1000
	tx := createSignedTx(t, []TXInput{{TxHash: hash1, Index: 0}},
		[]TXOutput{{Value: 49000, Address: [32]byte{30}}}, []ed25519.PrivateKey{priv})

	coinbase := CreateCoinbaseWithFees(proposerAddr, 10*1e8, 1000, nil)
	block := NewBlock([]*Transaction{coinbase, tx}, [32]byte{}, 1, proposerAddr)

	subsidy := uint64(10 * 1e8)
	totalReward, err := block.CalculateTotalReward(provider, subsidy)
	if err != nil {
		t.Fatalf("CalculateTotalReward: %v", err)
	}

	if totalReward != subsidy+1000 {
		t.Errorf("CalculateTotalReward = %d, expected %d", totalReward, subsidy+1000)
	}
}

func TestCoinbaseReward_IncludesFees(t *testing.T) {
	// ValidateCoinbaseReward should pass when coinbase output >= subsidy + fees.
	addr, _, priv := makeKeyPair(t)
	proposerAddr := [32]byte{99}

	provider := newTestUTXOProvider()
	hash1 := sha256.Sum256([]byte("coinbase-reward"))
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 20000, Address: addr})

	// tx with fee = 500
	tx := createSignedTx(t, []TXInput{{TxHash: hash1, Index: 0}},
		[]TXOutput{{Value: 19500, Address: [32]byte{31}}}, []ed25519.PrivateKey{priv})

	subsidy := uint64(10 * 1e8)
	coinbase := CreateCoinbaseWithFees(proposerAddr, subsidy, 500, nil)
	block := NewBlock([]*Transaction{coinbase, tx}, [32]byte{}, 1, proposerAddr)

	err := block.ValidateCoinbaseReward(provider, subsidy)
	if err != nil {
		t.Errorf("ValidateCoinbaseReward should pass: %v", err)
	}
}

func TestCoinbaseReward_InsufficientFails(t *testing.T) {
	// ValidateCoinbaseReward should fail when coinbase output < subsidy + fees.
	addr, _, priv := makeKeyPair(t)
	proposerAddr := [32]byte{99}

	provider := newTestUTXOProvider()
	hash1 := sha256.Sum256([]byte("insufficient-reward"))
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 20000, Address: addr})

	// tx with fee = 5000
	tx := createSignedTx(t, []TXInput{{TxHash: hash1, Index: 0}},
		[]TXOutput{{Value: 15000, Address: [32]byte{32}}}, []ed25519.PrivateKey{priv})

	subsidy := uint64(10 * 1e8)
	// Coinbase only pays subsidy, not including fees.
	coinbase := CreateCoinbaseTransaction(proposerAddr, subsidy, nil)
	block := NewBlock([]*Transaction{coinbase, tx}, [32]byte{}, 1, proposerAddr)

	err := block.ValidateCoinbaseReward(provider, subsidy)
	if err == nil {
		t.Error("ValidateCoinbaseReward should fail when coinbase < subsidy + fees")
	}
}

func TestFeeConfig_DefaultValues(t *testing.T) {
	config := DefaultPoSConfig()

	// Verify all fee-related configuration.
	if config.BaseFeePerByte == 0 {
		t.Error("BaseFeePerByte should not be zero")
	}
	if config.PriorityFeePerByte == 0 {
		t.Error("PriorityFeePerByte should not be zero")
	}
	if config.BlockReward == 0 {
		t.Error("BlockReward should not be zero")
	}

	// BlockReward should be 50 AIB (= 50 * 1e8 satoshi)
	expectedBlockReward := uint64(50 * 1e8)
	if config.BlockReward != expectedBlockReward {
		t.Errorf("BlockReward = %d, expected %d", config.BlockReward, expectedBlockReward)
	}
}

func TestTotalOutputValue(t *testing.T) {
	outputs := []TXOutput{
		{Value: 1000, Address: [32]byte{1}},
		{Value: 2000, Address: [32]byte{2}},
		{Value: 3000, Address: [32]byte{3}},
	}
	tx := NewTransaction([]TXInput{}, outputs)

	total := tx.TotalOutputValue()
	if total != 6000 {
		t.Errorf("TotalOutputValue = %d, expected 6000", total)
	}
}

func TestTotalInputValue(t *testing.T) {
	addr := interfaces.Address{1, 2, 3}
	hash1 := sha256.Sum256([]byte("input-val-1"))
	hash2 := sha256.Sum256([]byte("input-val-2"))

	provider := newTestUTXOProvider()
	provider.add(&UTXO{TxHash: hash1, Index: 0, Value: 5000, Address: addr})
	provider.add(&UTXO{TxHash: hash2, Index: 1, Value: 7000, Address: addr})

	tx := NewTransaction(
		[]TXInput{
			{TxHash: hash1, Index: 0, Signature: []byte("s"), PublicKey: []byte("p")},
			{TxHash: hash2, Index: 1, Signature: []byte("s"), PublicKey: []byte("p")},
		},
		[]TXOutput{},
	)

	total, err := tx.TotalInputValue(provider)
	if err != nil {
		t.Fatalf("TotalInputValue: %v", err)
	}
	if total != 12000 {
		t.Errorf("TotalInputValue = %d, expected 12000", total)
	}
}

func TestFeeScaling_ManyTransactions(t *testing.T) {
	// Analyze fee costs for varying numbers of inputs/outputs.
	config := DefaultPoSConfig()
	feeRate := config.BaseFeePerByte

	type feeResult struct {
		inputs  int
		outputs int
		size    int
		fee     uint64
	}

	var results []feeResult

	scenarios := []struct {
		inputs  int
		outputs int
	}{
		{1, 1},
		{1, 2},
		{2, 1},
		{2, 2},
		{3, 3},
		{5, 5},
		{10, 10},
	}

	for _, s := range scenarios {
		var inputs []TXInput
		for i := 0; i < s.inputs; i++ {
			inputs = append(inputs, TXInput{
				TxHash:    [32]byte{byte(i)},
				Index:     uint32(i),
				Signature: make([]byte, 64),
				PublicKey: make([]byte, 32),
			})
		}

		var outputs []TXOutput
		for i := 0; i < s.outputs; i++ {
			outputs = append(outputs, TXOutput{
				Value:   1000,
				Script:  []byte("pay"),
				Address: [32]byte{byte(i + 100)},
			})
		}

		tx := NewTransaction(inputs, outputs)
		size := tx.SerializeSize()
		fee := tx.CalculateFee(feeRate)

		results = append(results, feeResult{
			inputs:  s.inputs,
			outputs: s.outputs,
			size:    size,
			fee:     fee,
		})
	}

	// Verify monotonic increase: more inputs/outputs should mean higher fees.
	for i := 1; i < len(results); i++ {
		if results[i].inputs+results[i].outputs > results[i-1].inputs+results[i-1].outputs {
			if results[i].fee < results[i-1].fee {
				t.Errorf("fee should increase with more I/O: [%d+%d]fee=%d < [%d+%d]fee=%d",
					results[i].inputs, results[i].outputs, results[i].fee,
					results[i-1].inputs, results[i-1].outputs, results[i-1].fee)
			}
		}
	}

	// Verify fee = size * feeRate for each.
	for _, r := range results {
		expected := uint64(r.size) * feeRate
		if expected < 1 {
			expected = 1
		}
		if r.fee != expected {
			t.Errorf("[%d in, %d out] fee=%d, expected=%d (size=%d, rate=%d)",
				r.inputs, r.outputs, r.fee, expected, r.size, feeRate)
		}
	}
}

func TestBlockSecurityValidation_MinFeeCheck(t *testing.T) {
	// ValidateBlockSecurity should flag transactions with fee below minimum.
	addr, _, priv := makeKeyPair(t)
	proposerAddr := [32]byte{99}

	provider := newTestUTXOProvider()
	prevHash := sha256.Sum256([]byte("security-fee"))
	provider.add(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	// Transaction with fee = 10 (below MinTransactionFee=100)
	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9990, Address: [32]byte{40}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	coinbase := CreateCoinbaseTransaction(proposerAddr, 10*1e8, nil)
	block := NewBlock([]*Transaction{coinbase, tx}, [32]byte{}, 1, proposerAddr)
	block.Header.MerkleRoot = block.CalculateMerkleRoot()
	block.Hash = block.CalculateHash()

	errs := block.ValidateBlockSecurity(provider, 1)
	feeErrorFound := false
	for _, e := range errs {
		if e != nil {
			feeErrorFound = true
		}
	}

	if !feeErrorFound {
		t.Error("ValidateBlockSecurity should detect fee below minimum")
	}
}

func TestMempoolMinFee_Enforcement(t *testing.T) {
	// Mempool with minFee > 0 should reject transactions with insufficient fee.
	store := NewUTXOStore()
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("mempool-min-fee"))

	store.AddUTXO(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	// Mempool requires minFee = 500
	mempool := NewMempool(100, 500)

	// Transaction with fee = 100 (below mempool minFee)
	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9900, Address: [32]byte{50}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := mempool.AddTransaction(tx, store)
	if err == nil {
		t.Error("Mempool should reject tx with fee below minFee")
	}
}

func TestMempoolMinFee_Accepted(t *testing.T) {
	// Transaction meeting minimum fee should be accepted.
	store := NewUTXOStore()
	addr, _, priv := makeKeyPair(t)
	prevHash := sha256.Sum256([]byte("mempool-accept"))

	store.AddUTXO(&UTXO{TxHash: prevHash, Index: 0, Value: 10000, Address: addr})

	mempool := NewMempool(100, 500)

	// Transaction with fee = 1000 (above mempool minFee)
	inputs := []TXInput{{TxHash: prevHash, Index: 0}}
	outputs := []TXOutput{{Value: 9000, Address: [32]byte{51}}}
	tx := createSignedTx(t, inputs, outputs, []ed25519.PrivateKey{priv})

	err := mempool.AddTransaction(tx, store)
	if err != nil {
		t.Errorf("Mempool should accept tx with fee >= minFee: %v", err)
	}

	if mempool.Size() != 1 {
		t.Errorf("Mempool size = %d, expected 1", mempool.Size())
	}
}
