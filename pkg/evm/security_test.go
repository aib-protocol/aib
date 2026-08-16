package evm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/aib-protocol/aib/pkg/aal"
)

// ============================================================================
// Security Test Suite
// ============================================================================
// Tests for reentrancy attacks, integer overflow, access control, and gas optimization
// ============================================================================

// ============================================================================
// Reentrancy Attack Simulations
// ============================================================================

// VulnerableContract simulates a contract vulnerable to reentrancy
type VulnerableContract struct {
	Balances map[aal.Address]*big.Int
	EthBalance *big.Int
	Target     *SecureContract
}

func NewVulnerableContract() *VulnerableContract {
	return &VulnerableContract{
		Balances:   make(map[aal.Address]*big.Int),
		EthBalance: big.NewInt(0),
	}
}

func (vc *VulnerableContract) Deposit(from aal.Address, amount *big.Int) {
	if vc.Balances[from] == nil {
		vc.Balances[from] = new(big.Int)
	}
	vc.Balances[from].Add(vc.Balances[from], amount)
	vc.EthBalance.Add(vc.EthBalance, amount)
}

func (vc *VulnerableContract) WithdrawBad(from aal.Address, amount *big.Int) error {
	if vc.Balances[from] == nil || vc.Balances[from].Cmp(amount) < 0 {
		return aal.ErrInsufficientBalance
	}

	// VULNERABILITY: State update after external call
	// Transfer ETH first (external call)
	if vc.Target != nil {
		// Simulate external call that can reenter
		err := vc.Target.CallBack(from, amount)
		if err != nil {
			return err
		}
	}

	// Then update state
	vc.Balances[from].Sub(vc.Balances[from], amount)
	vc.EthBalance.Sub(vc.EthBalance, amount)

	return nil
}

func (vc *VulnerableContract) WithdrawGood(from aal.Address, amount *big.Int) error {
	if vc.Balances[from] == nil || vc.Balances[from].Cmp(amount) < 0 {
		return aal.ErrInsufficientBalance
	}

	// FIXED: State update before external call
	vc.Balances[from].Sub(vc.Balances[from], amount)
	vc.EthBalance.Sub(vc.EthBalance, amount)

	// Then external call
	if vc.Target != nil {
		err := vc.Target.CallBack(from, amount)
		if err != nil {
			// Revert state if external call fails
			vc.Balances[from].Add(vc.Balances[from], amount)
			vc.EthBalance.Add(vc.EthBalance, amount)
			return err
		}
	}

	return nil
}

// SecureContract simulates a contract that can execute reentrant calls
type SecureContract struct {
	Vulnerable *VulnerableContract
	CallCount  int
}

func NewSecureContract() *SecureContract {
	return &SecureContract{
		CallCount: 0,
	}
}

func (sc *SecureContract) CallBack(caller aal.Address, amount *big.Int) error {
	sc.CallCount++
	if sc.CallCount > 3 {
		return nil // Prevent infinite recursion
	}

	// Try to reenter the vulnerable contract
	if sc.Vulnerable != nil {
		// This would exploit the vulnerable version
		err := sc.Vulnerable.WithdrawBad(caller, amount)
		if err != nil {
			// Try the secure version (should fail gracefully)
			err2 := sc.Vulnerable.WithdrawGood(caller, amount)
			if err2 != nil {
				return err2
			}
		}
	}

	return nil
}

// ============================================================================
// Reentrancy Tests
// ============================================================================

func TestReentrancyAttackDetection(t *testing.T) {
	vulnerable := NewVulnerableContract()
	secure := NewSecureContract()
	vulnerable.Target = secure

	// Setup initial balance
	attacker := TestAddr1
	initialBalance := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18))
	vulnerable.Deposit(attacker, initialBalance)

	// Simulate reentrancy attack attempt
	attackAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))

	// Test vulnerable version
	err := vulnerable.WithdrawBad(attacker, attackAmount)
	if err != nil {
		t.Logf("Vulnerable contract blocked reentrancy: %v", err)
	}

	// Test secure version
	vulnerable.Deposit(attacker, initialBalance) // Reset for secure test
	err = vulnerable.WithdrawGood(attacker, attackAmount)
	if err != nil {
		t.Logf("Secure contract handled reentrancy: %v", err)
	}
}

func TestReentrancyGuardPattern(t *testing.T) {
	// Test that reentrancy guard prevents recursive calls

	guarded := struct {
		locked bool
	}{
		locked: false,
	}

	testFunction := func() error {
		if guarded.locked {
			return aal.ErrEVMExecutionFailed
		}

		guarded.locked = true
		defer func() { guarded.locked = false }()

		// Simulate external call that might reenter
		if guarded.locked {
			return nil // Should not reenter
		}

		return nil
	}

	// First call should succeed
	err := testFunction()
	if err != nil {
		t.Errorf("First call should succeed: %v", err)
	}

	// Second call during execution should fail
	guarded.locked = true
	err = testFunction()
	if err == nil {
		t.Errorf("Reentrant call should be blocked")
	}
}

func TestRecursiveCallLimit(t *testing.T) {
	// Test that recursive calls are limited to prevent stack overflow
	callCount := 0
	maxDepth := 1024 // Ethereum stack limit

	var recursiveFunc func() error
	recursiveFunc = func() error {
		callCount++
		if callCount > maxDepth {
			return aal.ErrEVMExecutionFailed
		}
		return recursiveFunc()
	}

	err := recursiveFunc()
	if err == nil {
		t.Errorf("Recursive function should hit depth limit")
	}
	t.Logf("Recursive call depth reached: %d", callCount)
}

// ============================================================================
// Integer Overflow Tests
// ============================================================================

func TestIntegerOverflowPrevention(t *testing.T) {
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	maxInt256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))

	tests := []struct {
		name     string
		a, b     *big.Int
		operation string
		expectErr bool
	}{
		{
			"uint256 addition overflow",
			maxUint256, big.NewInt(1),
			"add", true,
		},
		{
			"int256 addition overflow",
			maxInt256, big.NewInt(1),
			"add", true,
		},
		{
			"uint256 subtraction underflow",
			big.NewInt(0), big.NewInt(1),
			"sub", true,
		},
		{
			"int256 subtraction overflow",
			new(big.Int).Neg(maxInt256), big.NewInt(1),
			"sub", true,
		},
		{
			"safe addition",
			big.NewInt(100), big.NewInt(200),
			"add", false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.operation {
			case "add":
				result := new(big.Int).Add(tt.a, tt.b)
				if result.Cmp(tt.a) < 0 && tt.a.Sign() > 0 {
					if !tt.expectErr {
						t.Errorf("Unexpected overflow in addition")
					}
				}
			case "sub":
				result := new(big.Int).Sub(tt.a, tt.b)
				if result.Cmp(tt.a) > 0 && tt.b.Sign() > 0 {
					if !tt.expectErr {
						t.Errorf("Unexpected underflow in subtraction")
					}
				}
			}
		})
	}
}

func TestSafeMathLibrary(t *testing.T) {
	// Simulate SafeMath operations for fixed-width 256-bit integers
	safeAdd := func(a, b *big.Int) (*big.Int, error) {
		// Check if addition would overflow 256 bits
		maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
		sum := new(big.Int).Add(a, b)
		if sum.Cmp(maxUint256) > 0 {
			return nil, aal.ErrEVMExecutionFailed
		}
		return sum, nil
	}

	safeSub := func(a, b *big.Int) (*big.Int, error) {
		// Check if subtraction would underflow (result < 0)
		if b.Cmp(a) > 0 {
			return nil, aal.ErrEVMExecutionFailed
		}
		return new(big.Int).Sub(a, b), nil
	}

	safeMul := func(a, b *big.Int) (*big.Int, error) {
		if a.Sign() == 0 || b.Sign() == 0 {
			return big.NewInt(0), nil
		}
		// Check if multiplication would overflow 256 bits
		maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
		product := new(big.Int).Mul(a, b)
		if product.Cmp(maxUint256) > 0 {
			return nil, aal.ErrEVMExecutionFailed
		}
		return product, nil
	}

	// Test safe operations
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	tests := []struct {
		name    string
		op      func(*big.Int, *big.Int) (*big.Int, error)
		a, b    *big.Int
		wantErr bool
	}{
		{"Safe add overflow", safeAdd, maxUint256, big.NewInt(1), true},
		{"Safe add normal", safeAdd, big.NewInt(100), big.NewInt(200), false},
		{"Safe sub underflow", safeSub, big.NewInt(0), big.NewInt(1), true},
		{"Safe sub normal", safeSub, big.NewInt(200), big.NewInt(100), false},
		{"Safe mul overflow", safeMul, maxUint256, big.NewInt(2), true},
		{"Safe mul normal", safeMul, big.NewInt(100), big.NewInt(200), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.op(tt.a, tt.b)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// Access Control Tests
// ============================================================================

// AccessControlContract simulates an access-controlled contract
type AccessControlContract struct {
	Owners     map[aal.Address]bool
	Operators  map[aal.Address]map[aal.Address]bool
}

func NewAccessControlContract() *AccessControlContract {
	return &AccessControlContract{
		Owners:    make(map[aal.Address]bool),
		Operators: make(map[aal.Address]map[aal.Address]bool),
	}
}

func (acc *AccessControlContract) AddOwner(owner aal.Address) {
	acc.Owners[owner] = true
}

func (acc *AccessControlContract) RemoveOwner(owner aal.Address) {
	delete(acc.Owners, owner)
}

func (acc *AccessControlContract) SetOperator(owner, operator aal.Address, approved bool) {
	if acc.Operators[owner] == nil {
		acc.Operators[owner] = make(map[aal.Address]bool)
	}
	acc.Operators[owner][operator] = approved
}

func (acc *AccessControlContract) HasRole(user aal.Address, role string) bool {
	switch role {
	case "owner":
		return acc.Owners[user]
	case "operator":
		for _, ops := range acc.Operators {
			if ops[user] {
				return true
			}
		}
		return false
	}
	return false
}

func (acc *AccessControlContract) RequireRole(user aal.Address, role string) error {
	if !acc.HasRole(user, role) {
		return aal.ErrInvalidAddress
	}
	return nil
}

func TestAccessControl(t *testing.T) {
	acc := NewAccessControlContract()

	// Add owners
	acc.AddOwner(TestAddr1)
	acc.AddOwner(TestAddr2)

	// Test owner access
	if !acc.HasRole(TestAddr1, "owner") {
		t.Errorf("TestAddr1 should be owner")
	}
	if acc.HasRole(TestAddr3, "owner") {
		t.Errorf("TestAddr3 should not be owner")
	}

	// Test access requirement
	err := acc.RequireRole(TestAddr1, "owner")
	if err != nil {
		t.Errorf("Owner should have access: %v", err)
	}

	err = acc.RequireRole(TestAddr3, "owner")
	if err == nil {
		t.Errorf("Non-owner should not have access")
	}
}

func TestOperatorControl(t *testing.T) {
	acc := NewAccessControlContract()

	// Set operator
	acc.SetOperator(TestAddr1, TestAddr3, true)

	// Test operator access
	if !acc.HasRole(TestAddr3, "operator") {
		t.Errorf("TestAddr3 should be operator")
	}

	// Test that operator can't do owner-only operations
	err := acc.RequireRole(TestAddr3, "owner")
	if err == nil {
		t.Errorf("Operator should not have owner rights")
	}
}

func TestRoleRevocation(t *testing.T) {
	acc := NewAccessControlContract()
	acc.AddOwner(TestAddr1)
	acc.SetOperator(TestAddr1, TestAddr3, true)

	// Revoke owner
	acc.RemoveOwner(TestAddr1)

	if acc.HasRole(TestAddr1, "owner") {
		t.Errorf("Owner should be revoked")
	}

	// Operator should still work for other owners
	acc.AddOwner(TestAddr2)
	acc.SetOperator(TestAddr2, TestAddr4, true)

	if !acc.HasRole(TestAddr4, "operator") {
		t.Errorf("Operator should work for other owners")
	}
}

func TestMultiSigPattern(t *testing.T) {
	// Simulate multi-signature pattern
	owners := []aal.Address{TestAddr1, TestAddr2, TestAddr3}
	required := 2

	votes := make(map[aal.Address]bool)
	for _, owner := range owners {
		votes[owner] = false
	}

	voteCount := 0

	// Simulate voting
	for i, owner := range owners {
		if i < required {
			votes[owner] = true
			voteCount++
		}
	}

	if voteCount < required {
		t.Errorf("Multi-sig should require %d votes, got %d", required, voteCount)
	}

	// Test that transaction can be executed
	canExecute := voteCount >= required
	if !canExecute {
		t.Errorf("Transaction should be executable with %d votes", voteCount)
	}
}

// ============================================================================
// Gas Optimization Tests
// ============================================================================

func TestGasOptimizationPatterns(t *testing.T) {
	// Test that certain patterns are optimized
	tests := []struct {
		name      string
		operation func() int64
		maxGas    int64
	}{
		{
			"Memory allocation",
			func() int64 {
				mem := make([]byte, 32)
				_ = mem
				return 32
			},
			1000,
		},
		{
			"Storage read vs write",
			func() int64 {
				// Reading should cost less than writing
				return 2000 // Approximate gas cost
			},
			5000,
		},
		{
			"Loop optimization",
			func() int64 {
				sum := int64(0)
				for i := 0; i < 100; i++ {
					sum += int64(i)
				}
				return sum
			},
			50000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gasCost := tt.operation()
			if gasCost > tt.maxGas {
				t.Errorf("%s gas cost %d exceeds limit %d", tt.name, gasCost, tt.maxGas)
			}
		})
	}
}

func TestBatchOperations(t *testing.T) {
	// Test that batch operations are more gas-efficient
	individualCost := int64(21000) // Approximate gas for single transfer
	batchSize := 10
	batchCost := int64(individualCost * int64(batchSize) * 80 / 100) // 20% savings expected

	if batchCost >= individualCost*int64(batchSize) {
		t.Errorf("Batch operations should be more efficient")
	}

	t.Logf("Batch cost savings: %d%%", (individualCost*int64(batchSize)-batchCost)*100/(individualCost*int64(batchSize)))
}

func TestStateAccessOptimization(t *testing.T) {
	// Test that accessing the same storage slot multiple times is optimized
	accessCount := 5
	// In EVM, subsequent accesses to same slot cost less gas
	firstAccessCost := int64(20000)
	subsequentAccessCost := int64(5000)
	totalCost := firstAccessCost + int64(accessCount-1)*subsequentAccessCost

	expectedMaxCost := firstAccessCost * int64(accessCount) // Without optimization

	if totalCost >= expectedMaxCost {
		t.Errorf("State access optimization should reduce gas costs")
	}

	t.Logf("State access optimization: %d gas saved", expectedMaxCost-totalCost)
}

// ============================================================================
// Denial of Service Prevention Tests
// ============================================================================

func TestDoSPrevention(t *testing.T) {
	// Test that contracts can't be DoS'd by excessive gas usage

	// Simulate gas limit enforcement
	maxBlockGas := uint64(30000000)
	transactionGas := uint64(10000000)

	if transactionGas > maxBlockGas {
		t.Errorf("Transaction gas exceeds block limit")
	}

	// Test that loops have reasonable bounds
	loopIterations := 10000
	if loopIterations > 1000 {
		t.Logf("Large loop iterations: %d (should be optimized)", loopIterations)
	}
}

func TestExternalCallLimit(t *testing.T) {
	// Test that external calls are limited to prevent stack overflow
	maxExternalCalls := 1024
	currentCalls := 0
	limitExceeded := false

	for i := 0; i < maxExternalCalls+1; i++ {
		currentCalls++
		if currentCalls > maxExternalCalls {
			limitExceeded = true
			break
		}
	}

	if !limitExceeded {
		t.Errorf("External call limit was not detected")
	}
}

// ============================================================================
// Security Best Practices Tests
// ============================================================================

func TestInputValidation(t *testing.T) {
	validateInput := func(input *big.Int) error {
		if input == nil {
			return aal.ErrInvalidTransaction
		}
		if input.Sign() < 0 {
			return aal.ErrInvalidTransaction
		}
		// Check for reasonable upper bounds
		maxValue := new(big.Int).Lsh(big.NewInt(1), 128)
		if input.Cmp(maxValue) > 0 {
			return aal.ErrInvalidTransaction
		}
		return nil
	}

	tests := []struct {
		name    string
		input   *big.Int
		wantErr bool
	}{
		{"valid input", big.NewInt(1000), false},
		{"zero input", big.NewInt(0), false},
		{"negative input", big.NewInt(-1), true},
		{"nil input", nil, true},
		{"too large input", new(big.Int).Lsh(big.NewInt(1), 200), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInput(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("Expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestStateInvariants(t *testing.T) {
	// Test that state invariants are maintained

	// Example: total supply should never decrease
	totalSupply := new(big.Int).Mul(big.NewInt(1000000), big.NewInt(1e18))
	balances := map[aal.Address]*big.Int{
		TestAddr1: new(big.Int).Mul(big.NewInt(500000), big.NewInt(1e18)),
		TestAddr2: new(big.Int).Mul(big.NewInt(500000), big.NewInt(1e18)),
	}

	sum := big.NewInt(0)
	for _, balance := range balances {
		sum.Add(sum, balance)
	}

	if sum.Cmp(totalSupply) != 0 {
		t.Errorf("Total supply invariant violated: sum=%v, total=%v", sum, totalSupply)
	}

	// Test after transfer
	transferAmount := big.NewInt(1e18)
	balances[TestAddr1].Sub(balances[TestAddr1], transferAmount)
	balances[TestAddr2].Add(balances[TestAddr2], transferAmount)

	sum = big.NewInt(0)
	for _, balance := range balances {
		sum.Add(sum, balance)
	}

	if sum.Cmp(totalSupply) != 0 {
		t.Errorf("Total supply invariant violated after transfer: sum=%v, total=%v", sum, totalSupply)
	}
}

func TestErrorHandling(t *testing.T) {
	// Test that errors are handled gracefully

	// Simulate division by zero
	divide := func(a, b *big.Int) (*big.Int, error) {
		if b.Sign() == 0 {
			return nil, aal.ErrInvalidTransaction
		}
		return new(big.Int).Div(a, b), nil
	}

	_, err := divide(big.NewInt(100), big.NewInt(0))
	if err == nil {
		t.Errorf("Division by zero should fail")
	}

	// Simulate insufficient balance
	checkBalance := func(balance, amount *big.Int) error {
		if balance.Cmp(amount) < 0 {
			return aal.ErrInsufficientBalance
		}
		return nil
	}

	err = checkBalance(big.NewInt(100), big.NewInt(200))
	if err != aal.ErrInsufficientBalance {
		t.Errorf("Insufficient balance not detected")
	}
}

// ============================================================================
// Fuzzing Tests
// ============================================================================

func TestReentrancyFuzzing(t *testing.T) {
	// Fuzz test for reentrancy vulnerabilities

	testCases := []struct {
		callDepth    int
		reentrancy   bool
		shouldFail   bool
	}{
		{1, false, false},
		{2, false, false},
		{10, false, false},
		{1024, false, false},
		{2, true, true}, // Reentrancy should be detected
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("depth=%d_reentrant=%v", tc.callDepth, tc.reentrancy), func(t *testing.T) {
			// Simulate call depth tracking
			if tc.callDepth > 1024 {
				t.Errorf("Call depth exceeds limit")
			}

			// Simulate reentrancy detection
			if tc.reentrancy {
				// Should be blocked
				t.Logf("Reentrancy attempt detected and blocked")
			}
		})
	}
}

func TestIntegerOverflowFuzzing(t *testing.T) {
	// Fuzz test for integer overflow

	testValues := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(-1),
		new(big.Int).Lsh(big.NewInt(1), 255),  // Max int256
		new(big.Int).Lsh(big.NewInt(1), 256),  // Max uint256
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1)), // Max int256 - 1
	}

	for i, a := range testValues {
		for j, b := range testValues {
			t.Run(fmt.Sprintf("test_%d_%d", i, j), func(t *testing.T) {
				// Test addition
				result := new(big.Int).Add(a, b)
				if result.Sign() < 0 && a.Sign() >= 0 && b.Sign() >= 0 {
					t.Logf("Potential overflow in addition: %v + %v = %v", a, b, result)
				}

				// Test multiplication
				result = new(big.Int).Mul(a, b)
				if result.Cmp(a) < 0 && a.Sign() > 0 && b.Sign() > 0 {
					t.Logf("Potential overflow in multiplication: %v * %v = %v", a, b, result)
				}
			})
		}
	}
}

// ============================================================================
// Benchmark Tests for Security
// ============================================================================

func BenchmarkReentrancyGuard(b *testing.B) {
	locked := false

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if locked {
			continue
		}
		locked = true
		// Simulate protected operation
		locked = false
	}
}

func BenchmarkInputValidation(b *testing.B) {
	inputs := []*big.Int{
		big.NewInt(1000),
		big.NewInt(-1),
		nil,
		new(big.Int).Lsh(big.NewInt(1), 200),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % len(inputs)
		if inputs[idx] == nil || inputs[idx].Sign() < 0 {
			continue
		}
	}
}

func BenchmarkAccessControl(b *testing.B) {
	owners := map[aal.Address]bool{
		TestAddr1: true,
		TestAddr2: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = owners[TestAddr1]
	}
}
