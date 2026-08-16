package aal

import (
	"math/big"
	"testing"
)

// ============================================================================
// Address Tests
// ============================================================================

func TestBytesToAddress(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	addr := BytesToAddress(data)

	if addr[0] != 1 || addr[19] != 20 {
		t.Error("Address bytes don't match")
	}
}

func TestAddressHex(t *testing.T) {
	addr := Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	hex := addr.Hex()

	if len(hex) < 4 || hex[:2] != "0x" {
		t.Errorf("Invalid hex format: %s", hex)
	}
}

func TestParseHexAddress(t *testing.T) {
	hex := "0x0102030405060708090a0b0c0d0e0f1011121314"
	addr, err := ParseHexAddress(hex)
	if err != nil {
		t.Fatalf("ParseHexAddress failed: %v", err)
	}

	if addr[0] != 1 || addr[19] != 20 {
		t.Error("Address bytes don't match")
	}
}

func TestParseHexAddressInvalid(t *testing.T) {
	_, err := ParseHexAddress("invalid")
	if err == nil {
		t.Error("Should fail with invalid hex")
	}
}

// ============================================================================
// Hash Tests
// ============================================================================

func TestBytesToHash(t *testing.T) {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}

	hash := BytesToHash(data)
	if hash[0] != 0 || hash[31] != 31 {
		t.Error("Hash bytes don't match")
	}
}

func TestHashHex(t *testing.T) {
	hash := Hash{}
	for i := range hash {
		hash[i] = byte(i)
	}

	hex := hash.Hex()
	if len(hex) < 4 || hex[:2] != "0x" {
		t.Errorf("Invalid hex format: %s", hex)
	}
}

func TestBigToHash(t *testing.T) {
	bigVal := big.NewInt(255)
	hash := BigToHash(bigVal)

	// 255 = 0xFF, should be last byte
	if hash[31] != 255 {
		t.Errorf("Hash last byte = %d, expected 255", hash[31])
	}
}

func TestHashToBig(t *testing.T) {
	hash := Hash{}
	hash[31] = 255

	bigVal := HashToBig(hash)
	if bigVal.Int64() != 255 {
		t.Errorf("Big value = %d, expected 255", bigVal.Int64())
	}
}

// ============================================================================
// Keccak256Hash Tests
// ============================================================================

func TestKeccak256Hash(t *testing.T) {
	data := []byte("test data")
	hash := Keccak256Hash(data)

	if hash == (Hash{}) {
		t.Error("Hash should not be zero")
	}

	// Same input should produce same hash
	hash2 := Keccak256Hash(data)
	if hash != hash2 {
		t.Error("Same input should produce same hash")
	}

	// Different input should produce different hash
	hash3 := Keccak256Hash([]byte("different data"))
	if hash == hash3 {
		t.Error("Different input should produce different hash")
	}
}

// ============================================================================
// State Manager Tests
// ============================================================================

func TestNewStateManager(t *testing.T) {
	sm := NewStateManager()
	if sm == nil {
		t.Fatal("StateManager should not be nil")
	}
}

func TestStateManagerSetGetAccount(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	account := &Account{
		Address: addr,
		Balance: big.NewInt(1000),
		Nonce:   5,
		Storage: make(map[Hash]Hash),
	}

	err := sm.SetAccount(addr, account)
	if err != nil {
		t.Fatalf("SetAccount failed: %v", err)
	}

	got, err := sm.GetAccount(addr)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	if got.Balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Balance = %s, expected 1000", got.Balance.String())
	}

	if got.Nonce != 5 {
		t.Errorf("Nonce = %d, expected 5", got.Nonce)
	}
}

func TestStateManagerGetNonExistentAccount(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	_, err := sm.GetAccount(addr)
	if err == nil {
		t.Error("Should fail with non-existent account")
	}
}

func TestStateManagerDeleteAccount(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	account := &Account{
		Address: addr,
		Balance: big.NewInt(1000),
		Storage: make(map[Hash]Hash),
	}

	sm.SetAccount(addr, account)
	sm.DeleteAccount(addr)

	if sm.HasAccount(addr) {
		t.Error("Account should be deleted")
	}
}

func TestStateManagerBalance(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	// Set balance
	err := sm.SetBalance(addr, big.NewInt(1000))
	if err != nil {
		t.Fatalf("SetBalance failed: %v", err)
	}

	// Get balance
	balance, err := sm.GetBalance(addr)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Balance = %s, expected 1000", balance.String())
	}

	// Add balance
	err = sm.AddBalance(addr, big.NewInt(500))
	if err != nil {
		t.Fatalf("AddBalance failed: %v", err)
	}

	balance, _ = sm.GetBalance(addr)
	if balance.Cmp(big.NewInt(1500)) != 0 {
		t.Errorf("Balance = %s, expected 1500", balance.String())
	}

	// Subtract balance
	err = sm.SubBalance(addr, big.NewInt(300))
	if err != nil {
		t.Fatalf("SubBalance failed: %v", err)
	}

	balance, _ = sm.GetBalance(addr)
	if balance.Cmp(big.NewInt(1200)) != 0 {
		t.Errorf("Balance = %s, expected 1200", balance.String())
	}
}

func TestStateManagerSubBalanceInsufficient(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	sm.SetBalance(addr, big.NewInt(100))
	err := sm.SubBalance(addr, big.NewInt(200))
	if err == nil {
		t.Error("Should fail with insufficient balance")
	}
}

func TestStateManagerNonce(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	sm.SetBalance(addr, big.NewInt(0)) // Create account

	err := sm.SetNonce(addr, 10)
	if err != nil {
		t.Fatalf("SetNonce failed: %v", err)
	}

	nonce, err := sm.GetNonce(addr)
	if err != nil {
		t.Fatalf("GetNonce failed: %v", err)
	}

	if nonce != 10 {
		t.Errorf("Nonce = %d, expected 10", nonce)
	}
}

func TestStateManagerCode(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	code := []byte{0x60, 0x00, 0x60, 0x00} // Simple bytecode
	err := sm.SetCode(addr, code)
	if err != nil {
		t.Fatalf("SetCode failed: %v", err)
	}

	got, err := sm.GetCode(addr)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}

	if len(got) != 4 {
		t.Errorf("Code length = %d, expected 4", len(got))
	}
}

func TestStateManagerStorage(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}
	key := Hash{1}
	value := Hash{2}

	sm.SetBalance(addr, big.NewInt(0)) // Create account

	err := sm.SetStorage(addr, key, value)
	if err != nil {
		t.Fatalf("SetStorage failed: %v", err)
	}

	got, err := sm.GetStorage(addr, key)
	if err != nil {
		t.Fatalf("GetStorage failed: %v", err)
	}

	if got != value {
		t.Error("Storage value mismatch")
	}
}

func TestStateManagerSnapshot(t *testing.T) {
	sm := NewStateManager()
	addr := Address{1, 2, 3}

	// Set initial state
	sm.SetBalance(addr, big.NewInt(1000))

	// Create snapshot
	snapID := sm.Snapshot()

	// Modify state
	sm.SetBalance(addr, big.NewInt(2000))

	// Verify modified state
	balance, _ := sm.GetBalance(addr)
	if balance.Cmp(big.NewInt(2000)) != 0 {
		t.Errorf("Balance = %s, expected 2000", balance.String())
	}

	// Revert to snapshot
	sm.RevertToSnapshot(snapID)

	// Verify reverted state
	balance, _ = sm.GetBalance(addr)
	if balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Balance after revert = %s, expected 1000", balance.String())
	}
}

func TestStateManagerStateRoot(t *testing.T) {
	sm := NewStateManager()

	root, err := sm.GetStateRoot()
	if err != nil {
		t.Fatalf("GetStateRoot failed: %v", err)
	}

	// Empty state should have some root
	if root == (Hash{}) {
		t.Error("State root should not be zero")
	}

	// Add an account
	addr := Address{1, 2, 3}
	sm.SetBalance(addr, big.NewInt(1000))

	// State root should change
	newRoot, _ := sm.GetStateRoot()
	if root == newRoot {
		t.Error("State root should change after adding account")
	}
}

func TestStateManagerAccountCount(t *testing.T) {
	sm := NewStateManager()

	// Add 3 accounts
	for i := 0; i < 3; i++ {
		addr := Address{byte(i + 1)}
		sm.SetBalance(addr, big.NewInt(1000))
	}

	count, err := sm.GetAccountCount()
	if err != nil {
		t.Fatalf("GetAccountCount failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Account count = %d, expected 3", count)
	}
}

// ============================================================================
// Account State Tests
// ============================================================================

func TestNewAccountState(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)

	if as == nil {
		t.Fatal("AccountState should not be nil")
	}
}

func TestAccountStateBalance(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	// Add balance
	as.AddBalance(addr, big.NewInt(1000))

	// Get balance
	balance := as.GetBalance(addr)
	if balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Balance = %s, expected 1000", balance.String())
	}

	// Subtract balance
	as.SubBalance(addr, big.NewInt(300))

	balance = as.GetBalance(addr)
	if balance.Cmp(big.NewInt(700)) != 0 {
		t.Errorf("Balance = %s, expected 700", balance.String())
	}
}

func TestAccountStateNonce(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	as.AddBalance(addr, big.NewInt(0)) // Create account

	as.SetNonce(addr, 5)
	nonce := as.GetNonce(addr)

	if nonce != 5 {
		t.Errorf("Nonce = %d, expected 5", nonce)
	}
}

func TestAccountStateCode(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	// Create account first
	as.CreateAccount(addr)

	code := []byte{0x60, 0x00}
	as.SetCode(addr, code)

	got := as.GetCode(addr)
	if len(got) != 2 {
		t.Errorf("Code length = %d, expected 2", len(got))
	}

	size := as.GetCodeSize(addr)
	if size != 2 {
		t.Errorf("Code size = %d, expected 2", size)
	}

	codeHash := as.GetCodeHash(addr)
	if codeHash == (Hash{}) {
		t.Error("Code hash should not be zero")
	}
}

func TestAccountStateExist(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	if as.Exist(addr) {
		t.Error("Account should not exist initially")
	}

	as.AddBalance(addr, big.NewInt(100))

	if !as.Exist(addr) {
		t.Error("Account should exist after adding balance")
	}
}

func TestAccountStateEmpty(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	as.CreateAccount(addr)

	if !as.Empty(addr) {
		t.Error("New account should be empty")
	}

	as.AddBalance(addr, big.NewInt(100))

	if as.Empty(addr) {
		t.Error("Account with balance should not be empty")
	}
}

func TestAccountStateStorage(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}
	key := Hash{1}
	value := Hash{2}

	as.AddBalance(addr, big.NewInt(0)) // Create account

	as.SetState(addr, key, value)
	got := as.GetState(addr, key)

	if got != value {
		t.Error("Storage value mismatch")
	}
}

func TestAccountStateTransientStorage(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}
	key := Hash{1}
	value := Hash{2}

	as.SetTransientState(addr, key, value)
	got := as.GetTransientState(addr, key)

	if got != value {
		t.Error("Transient storage value mismatch")
	}
}

func TestAccountStateRefund(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)

	as.AddRefund(1000)
	if as.GetRefund() != 1000 {
		t.Errorf("Refund = %d, expected 1000", as.GetRefund())
	}

	as.SubRefund(300)
	if as.GetRefund() != 700 {
		t.Errorf("Refund = %d, expected 700", as.GetRefund())
	}
}

func TestAccountStateAccessList(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	// Prepare access list
	as.Prepare(Rules{}, addr, Address{4, 5, 6}, nil, nil, nil)

	if !as.AddressInAccessList(addr) {
		t.Error("Sender should be in access list")
	}

	// Add slot to access list
	slot := Hash{1}
	as.AddSlotToAccessList(addr, slot)

	addrOk, slotOk := as.SlotInAccessList(addr, slot)
	if !addrOk || !slotOk {
		t.Error("Slot should be in access list")
	}
}

func TestAccountStateSelfDestruct(t *testing.T) {
	sm := NewStateManager()
	as := NewAccountState(sm, nil)
	addr := Address{1, 2, 3}

	as.CreateAccount(addr)
	as.SelfDestruct(addr)

	if as.Exist(addr) {
		t.Error("Account should be destroyed")
	}
}

// ============================================================================
// Gas Calculator Tests
// ============================================================================

func TestNewGasCalculator(t *testing.T) {
	gc := NewGasCalculator()
	if gc == nil {
		t.Fatal("GasCalculator should not be nil")
	}
}

func TestCalculateIntrinsicGas(t *testing.T) {
	gc := NewGasCalculator()

	// Simple transfer (no data)
	gas := gc.CalculateIntrinsicGas(nil, false)
	if gas != 21000 {
		t.Errorf("Gas = %d, expected 21000", gas)
	}

	// Contract creation
	gas = gc.CalculateIntrinsicGas(nil, true)
	if gas != 53000 { // 21000 + 32000
		t.Errorf("Gas = %d, expected 53000", gas)
	}

	// With data
	data := []byte{0x00, 0x01, 0x00, 0x02} // 2 zero, 2 non-zero
	gas = gc.CalculateIntrinsicGas(data, false)
	expected := uint64(21000 + 2*4 + 2*16) // 21000 + 8 + 32 = 21040
	if gas != expected {
		t.Errorf("Gas = %d, expected %d", gas, expected)
	}
}

func TestCalculateGasLimit(t *testing.T) {
	gc := NewGasCalculator()

	// Within limit
	gas := gc.CalculateGasLimit(50000, 100000)
	if gas != 50000 {
		t.Errorf("Gas limit = %d, expected 50000", gas)
	}

	// Exceeds limit
	gas = gc.CalculateGasLimit(200000, 100000)
	if gas != 100000 {
		t.Errorf("Gas limit = %d, expected 100000 (capped)", gas)
	}
}

// ============================================================================
// Access List Tests
// ============================================================================

func TestNewAccessList(t *testing.T) {
	al := NewAccessList()
	if al == nil {
		t.Fatal("AccessList should not be nil")
	}
}

func TestAccessListAddAddress(t *testing.T) {
	al := NewAccessList()
	addr := Address{1, 2, 3}

	al.AddAddress(addr)

	if !al.ContainsAddress(addr) {
		t.Error("Address should be in access list")
	}
}

func TestAccessListAddSlot(t *testing.T) {
	al := NewAccessList()
	addr := Address{1, 2, 3}
	slot := Hash{1}

	al.AddSlot(addr, slot)

	addrOk, slotOk := al.Contains(addr, slot)
	if !addrOk || !slotOk {
		t.Error("Address and slot should be in access list")
	}
}

// ============================================================================
// Chain Config Tests
// ============================================================================

func TestDefaultChainConfig(t *testing.T) {
	cfg := DefaultChainConfig()

	if cfg.ChainID.Int64() != 8888 {
		t.Errorf("ChainID = %d, expected 8888", cfg.ChainID.Int64())
	}

	if !cfg.Rules.IsLondon {
		t.Error("London rules should be enabled")
	}
}

// ============================================================================
// Encode/Decode Account Tests
// ============================================================================

func TestEncodeDecodeAccount(t *testing.T) {
	addr := Address{1, 2, 3}
	account := &Account{
		Address: addr,
		Balance: big.NewInt(12345),
		Nonce:   42,
		Storage: make(map[Hash]Hash),
	}

	encoded := EncodeAccount(account)
	decoded, err := DecodeAccount(addr, encoded)
	if err != nil {
		t.Fatalf("DecodeAccount failed: %v", err)
	}

	if decoded.Nonce != 42 {
		t.Errorf("Nonce = %d, expected 42", decoded.Nonce)
	}

	if decoded.Balance.Cmp(big.NewInt(12345)) != 0 {
		t.Errorf("Balance = %s, expected 12345", decoded.Balance.String())
	}
}

func TestDecodeAccountCorrupted(t *testing.T) {
	addr := Address{1, 2, 3}

	_, err := DecodeAccount(addr, []byte{1, 2, 3}) // Too short
	if err == nil {
		t.Error("Should fail with corrupted data")
	}
}
