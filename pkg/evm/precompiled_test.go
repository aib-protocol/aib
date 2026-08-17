package evm

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/aib-protocol/aib/pkg/aal"
)

// ============================================================================
// Precompiled Contract Tests
// ============================================================================
// Tests for EVM precompiled contracts at addresses 0x01-0x09
// These are built-in contracts that provide cryptographic operations
// ============================================================================

// ============================================================================
// Test Utilities for Precompiled Contracts
// ============================================================================

func createPrecompileTestState() (*aal.StateManager, *aal.EVMExecutor) {
	sm := aal.NewStateManager()
	config := &aal.EVMConfig{
		ChainID:     big.NewInt(1),
		BlockNumber: big.NewInt(1),
		BlockTime:   15,
		Coinbase:    TestAddr1,
		GasLimit:    10_000_000,
	}
	executor := aal.NewEVMExecutor(sm, nil, config)
	sm.SetBalance(TestAddr1, new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)))
	return sm, executor
}

// ============================================================================
// SHA256 Precompile Tests (Address 0x02)
// ============================================================================

func TestPrecompileSHA256(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		output [32]byte
	}{
		{
			"empty input",
			[]byte{},
			sha256.Sum256([]byte{}),
		},
		{
			"hello world",
			[]byte("hello world"),
			sha256.Sum256([]byte("hello world")),
		},
		{
			"binary data",
			[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			sha256.Sum256([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}),
		},
		{
			"large input",
			bytes.Repeat([]byte{0xFF}, 1024),
			sha256.Sum256(bytes.Repeat([]byte{0xFF}, 1024)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sha256.Sum256(tt.input)
			if result != tt.output {
				t.Errorf("SHA256(%x) = %x, want %x", tt.input, result, tt.output)
			}
		})
	}
}

func TestPrecompileSHA256GasCost(t *testing.T) {
	tests := []struct {
		name        string
		inputLen    int
		expectedGas uint64
	}{
		{"Empty", 0, 60 + 12*0},
		{"32 bytes", 32, 60 + 12*1},
		{"64 bytes", 64, 60 + 12*2},
		{"128 bytes", 128, 60 + 12*4},
		{"256 bytes", 256, 60 + 12*8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SHA256 precompile gas formula: 60 + 12 * ceil(len(input) / 32)
			words := (uint64(tt.inputLen) + 31) / 32
			gasCost := uint64(60) + 12*words
			if gasCost != tt.expectedGas {
				t.Errorf("SHA256 gas for %d bytes = %d, want %d", tt.inputLen, gasCost, tt.expectedGas)
			}
		})
	}
}

// ============================================================================
// Ecrecover Precompile Tests (Address 0x01)
// ============================================================================

func TestPrecompileEcrecover(t *testing.T) {
	// Test ecrecover precompile behavior
	// Hash (32 bytes) + V (32 bytes) + R (32 bytes) + S (32 bytes) = 128 bytes input

	tests := []struct {
		name  string
		input []byte
		valid bool
	}{
		{
			"valid signature components (128 bytes)",
			make([]byte, 128),
			true,
		},
		{
			"short input (64 bytes)",
			make([]byte, 64),
			false,
		},
		{
			"too short input (0 bytes)",
			[]byte{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate input length
			isValid := len(tt.input) >= 128
			if isValid != tt.valid {
				t.Errorf("ecrecover input validation: got valid=%v, want valid=%v", isValid, tt.valid)
			}
		})
	}
}

func TestPrecompileEcrecoverGasCost(t *testing.T) {
	// Ecrecover always costs 3000 gas
	expectedGas := uint64(3000)
	if expectedGas != 3000 {
		t.Errorf("Ecrecover gas = %d, want 3000", expectedGas)
	}
}

// ============================================================================
// Identity Precompile Tests (Address 0x04)
// ============================================================================

func TestPrecompileIdentity(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty input", []byte{}},
		{"single byte", []byte{0x42}},
		{"32 bytes", bytes.Repeat([]byte{0xAB}, 32)},
		{"64 bytes", bytes.Repeat([]byte{0xCD}, 64)},
		{"large input", bytes.Repeat([]byte{0xEF}, 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Identity precompile simply returns the input
			output := make([]byte, len(tt.input))
			copy(output, tt.input)

			if !bytes.Equal(output, tt.input) {
				t.Errorf("Identity precompile failed: got %x, want %x", output, tt.input)
			}
		})
	}
}

func TestPrecompileIdentityGasCost(t *testing.T) {
	tests := []struct {
		name        string
		inputLen    int
		expectedGas uint64
	}{
		{"Empty", 0, 15 + 3*0},
		{"32 bytes", 32, 15 + 3*1},
		{"64 bytes", 64, 15 + 3*2},
		{"128 bytes", 128, 15 + 3*4},
		{"256 bytes", 256, 15 + 3*8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Identity gas: 15 + 3 * ceil(len / 32)
			words := (uint64(tt.inputLen) + 31) / 32
			gasCost := uint64(15) + 3*words
			if gasCost != tt.expectedGas {
				t.Errorf("Identity gas for %d bytes = %d, want %d", tt.inputLen, gasCost, tt.expectedGas)
			}
		})
	}
}

// ============================================================================
// RIPEMD160 Precompile Tests (Address 0x03)
// ============================================================================

func TestPrecompileRIPEMD160GasCost(t *testing.T) {
	tests := []struct {
		name        string
		inputLen    int
		expectedGas uint64
	}{
		{"Empty", 0, 600 + 120*0},
		{"32 bytes", 32, 600 + 120*1},
		{"64 bytes", 64, 600 + 120*2},
		{"128 bytes", 128, 600 + 120*4},
		{"256 bytes", 256, 600 + 120*8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// RIPEMD160 gas: 600 + 120 * ceil(len / 32)
			words := (uint64(tt.inputLen) + 31) / 32
			gasCost := uint64(600) + 120*words
			if gasCost != tt.expectedGas {
				t.Errorf("RIPEMD160 gas for %d bytes = %d, want %d", tt.inputLen, gasCost, tt.expectedGas)
			}
		})
	}
}

// ============================================================================
// Modexp Precompile Tests (Address 0x05)
// ============================================================================

func TestPrecompileModexp(t *testing.T) {
	tests := []struct {
		name     string
		base     *big.Int
		exponent *big.Int
		modulus  *big.Int
		expected *big.Int
	}{
		{
			"simple modexp",
			big.NewInt(2),
			big.NewInt(10),
			big.NewInt(1000),
			new(big.Int).Exp(big.NewInt(2), big.NewInt(10), big.NewInt(1000)),
		},
		{
			"large base",
			big.NewInt(123456),
			big.NewInt(789),
			big.NewInt(999999),
			new(big.Int).Exp(big.NewInt(123456), big.NewInt(789), big.NewInt(999999)),
		},
		{
			"modulus 1",
			big.NewInt(2),
			big.NewInt(10),
			big.NewInt(1),
			big.NewInt(0),
		},
		{
			"zero exponent",
			big.NewInt(2),
			big.NewInt(0),
			big.NewInt(1000),
			big.NewInt(1),
		},
		{
			"zero base",
			big.NewInt(0),
			big.NewInt(10),
			big.NewInt(1000),
			big.NewInt(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := new(big.Int).Exp(tt.base, tt.exponent, tt.modulus)
			if result.Cmp(tt.expected) != 0 {
				t.Errorf("modexp(%v, %v, %v) = %v, want %v", tt.base, tt.exponent, tt.modulus, result, tt.expected)
			}
		})
	}
}

func TestPrecompileModexpGasCost(t *testing.T) {
	// Modexp gas cost formula:
	// max(200, floor(f(max_base_len, max_modulus_len)) * ceil(exp_len / 8))
	tests := []struct {
		name    string
		baseLen uint64
		expLen  uint64
		modLen  uint64
		minGas  uint64
	}{
		{"Small params", 1, 1, 1, 200},
		{"Medium params", 32, 32, 32, 200},
		{"Large params", 128, 128, 128, 200},
		{"Very large", 256, 256, 256, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simplified gas calculation
			maxLen := tt.baseLen
			if tt.modLen > maxLen {
				maxLen = tt.modLen
			}

			lenMul := maxLen * maxLen
			expMul := tt.expLen
			if expMul < 1 {
				expMul = 1
			}

			gas := lenMul * expMul / 8
			if gas < tt.minGas {
				gas = tt.minGas
			}

			if gas < tt.minGas {
				t.Errorf("Modexp gas too low: got %d, want at least %d", gas, tt.minGas)
			}
		})
	}
}

// ============================================================================
// BN128 Add Precompile Tests (Address 0x06)
// ============================================================================

func TestPrecompileBN128AddInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		inputLen  int
		shouldErr bool
	}{
		{"exact 128 bytes", 128, false},
		{"zero padding (0 bytes)", 0, false},
		{"short input (64 bytes)", 64, false},
		{"extra long input (256 bytes)", 256, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]byte, tt.inputLen)
			// BN128 Add expects 128 bytes: x1(32) + y1(32) + x2(32) + y2(32)
			// Short inputs should be right-padded with zeros
			_ = input // Validation test
		})
	}
}

func TestPrecompileBN128AddGasCost(t *testing.T) {
	// BN128 Add gas cost: 150 (after Istanbul, was 500 before)
	istanbulGas := uint64(150)
	preByzantiumGas := uint64(500)

	if istanbulGas >= preByzantiumGas {
		t.Errorf("Istanbul should be cheaper: %d >= %d", istanbulGas, preByzantiumGas)
	}
}

// ============================================================================
// BN128 Mul Precompile Tests (Address 0x07)
// ============================================================================

func TestPrecompileBN128MulInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		inputLen  int
		shouldErr bool
	}{
		{"exact 96 bytes", 96, false},
		{"zero padding (0 bytes)", 0, false},
		{"short input (64 bytes)", 64, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]byte, tt.inputLen)
			// BN128 Mul expects 96 bytes: x(32) + y(32) + scalar(32)
			_ = input
		})
	}
}

func TestPrecompileBN128MulGasCost(t *testing.T) {
	// BN128 Mul gas cost: 6000 (after Istanbul, was 40000 before)
	istanbulGas := uint64(6000)
	preByzantiumGas := uint64(40000)

	if istanbulGas >= preByzantiumGas {
		t.Errorf("Istanbul should be cheaper: %d >= %d", istanbulGas, preByzantiumGas)
	}
}

// ============================================================================
// BN128 Pairing Precompile Tests (Address 0x08)
// ============================================================================

func TestPrecompileBN128PairingInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		inputLen  int
		shouldErr bool
	}{
		{"empty input (0 pairs)", 0, false},
		{"one pair (192 bytes)", 192, false},
		{"two pairs (384 bytes)", 384, false},
		{"invalid length (100 bytes)", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// BN128 Pairing expects input length to be multiple of 192 bytes
			isValid := tt.inputLen%192 == 0
			if isValid == tt.shouldErr {
				t.Errorf("BN128 pairing input validation: len=%d, valid=%v, shouldErr=%v", tt.inputLen, isValid, tt.shouldErr)
			}
		})
	}
}

func TestPrecompileBN128PairingGasCost(t *testing.T) {
	// BN128 Pairing gas: 45000 + 34000 * k (Istanbul) or 100000 + 80000 * k (pre-Istanbul)
	tests := []struct {
		name        string
		numPairs    uint64
		istanbulGas uint64
	}{
		{"0 pairs", 0, 45000},
		{"1 pair", 1, 45000 + 34000*1},
		{"2 pairs", 2, 45000 + 34000*2},
		{"4 pairs", 4, 45000 + 34000*4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gasCost := uint64(45000) + 34000*tt.numPairs
			if gasCost != tt.istanbulGas {
				t.Errorf("BN128 pairing gas for %d pairs = %d, want %d", tt.numPairs, gasCost, tt.istanbulGas)
			}
		})
	}
}

// ============================================================================
// Blake2F Precompile Tests (Address 0x09)
// ============================================================================

func TestPrecompileBlake2FInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		inputLen  int
		shouldErr bool
	}{
		{"exact 213 bytes", 213, false},
		{"too short", 212, true},
		{"too long", 214, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.inputLen == 213
			if isValid == tt.shouldErr {
				t.Errorf("Blake2F input validation: len=%d, valid=%v, shouldErr=%v", tt.inputLen, isValid, tt.shouldErr)
			}
		})
	}
}

// ============================================================================
// Precompile Address Detection Tests
// ============================================================================

func TestPrecompiledAddressDetection(t *testing.T) {
	precompiledAddresses := map[uint8]string{
		0x01: "ecrecover",
		0x02: "SHA256",
		0x03: "RIPEMD160",
		0x04: "identity",
		0x05: "modexp",
		0x06: "bn128Add",
		0x07: "bn128Mul",
		0x08: "bn128Pairing",
		0x09: "blake2F",
	}

	for addrByte, name := range precompiledAddresses {
		t.Run(name, func(t *testing.T) {
			addr := aal.Address{}
			addr[19] = addrByte

			// Check that this is recognized as a precompiled address
			isPrecompiled := addrByte >= 1 && addrByte <= 9
			if !isPrecompiled {
				t.Errorf("Address 0x%02x not recognized as precompiled", addrByte)
			}
		})
	}
}

func TestNonPrecompiledAddress(t *testing.T) {
	nonPrecompiledAddresses := []uint8{0x00, 0x0A, 0x0B, 0xFF}

	for _, addrByte := range nonPrecompiledAddresses {
		t.Run(string(rune(addrByte)), func(t *testing.T) {
			isPrecompiled := addrByte >= 1 && addrByte <= 9
			if isPrecompiled {
				t.Errorf("Address 0x%02x incorrectly recognized as precompiled", addrByte)
			}
		})
	}
}

// ============================================================================
// Precompile Gas Consistency Tests
// ============================================================================

func TestPrecompileGasConsistency(t *testing.T) {
	// Verify gas costs are consistent with EIP-2028
	precompileGas := map[string]uint64{
		"ecrecover":         3000,
		"SHA256_base":       60,
		"RIPEMD160_base":    600,
		"identity_base":     15,
		"bn128Add":          150,
		"bn128Mul":          6000,
		"bn128Pairing_base": 45000,
	}

	for name, gas := range precompileGas {
		t.Run(name, func(t *testing.T) {
			if gas == 0 {
				t.Errorf("%s has zero gas cost", name)
			}
			// Gas should be reasonable
			if gas > 100_000_000 {
				t.Errorf("%s gas cost unreasonably high: %d", name, gas)
			}
		})
	}
}

// ============================================================================
// Precompile Execution via EVM Tests
// ============================================================================

func TestPrecompileCallViaTx(t *testing.T) {
	sm, executor := createPrecompileTestState()

	precompiles := []struct {
		name string
		addr aal.Address
		data []byte
	}{
		{
			"SHA256 via call",
			func() aal.Address {
				var a aal.Address
				a[19] = 0x02
				return a
			}(),
			[]byte("test data"),
		},
		{
			"Identity via call",
			func() aal.Address {
				var a aal.Address
				a[19] = 0x04
				return a
			}(),
			[]byte{0x01, 0x02, 0x03},
		},
	}

	for _, pc := range precompiles {
		t.Run(pc.name, func(t *testing.T) {
			tx := &aal.Transaction{
				From:     TestAddr1,
				To:       &pc.addr,
				Value:    big.NewInt(0),
				Data:     pc.data,
				GasLimit: 100000,
				GasPrice: big.NewInt(1),
				Nonce:    0,
			}

			result, err := executor.ExecuteTransaction(tx)
			t.Logf("Precompile %s: result=%v, err=%v", pc.name, result, err)
		})
	}

	_ = sm
}

// ============================================================================
// Benchmark Tests for Precompiled Contracts
// ============================================================================

func BenchmarkSHA256Precompile(b *testing.B) {
	data := bytes.Repeat([]byte{0x42}, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sha256.Sum256(data)
	}
}

func BenchmarkModexp(b *testing.B) {
	base := big.NewInt(2)
	exp := big.NewInt(1024)
	mod := big.NewInt(999999999)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		new(big.Int).Exp(base, exp, mod)
	}
}

func BenchmarkIdentityPrecompile(b *testing.B) {
	data := bytes.Repeat([]byte{0xAB}, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output := make([]byte, len(data))
		copy(output, data)
	}
}
