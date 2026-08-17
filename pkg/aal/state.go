package aal

import (
	"encoding/binary"
	"errors"
	"math/big"
	"sync"
)

var (
	ErrAccountNotFound  = errors.New("account not found")
	ErrInvalidKey       = errors.New("invalid key")
	ErrStateCorrupted   = errors.New("state database corrupted")
	ErrSnapshotNotFound = errors.New("snapshot not found")
	ErrInvalidStateRoot = errors.New("invalid state root")
)

// Account represents an EVM-compatible account
type Account struct {
	Address  Address
	Balance  *big.Int
	Nonce    uint64
	CodeHash []byte
	Code     []byte
	Storage  map[Hash]Hash
}

// StateManager manages EVM state in memory
type StateManager struct {
	mu         sync.RWMutex
	accounts   map[Address]*Account
	snapshots  map[int]*Snapshot
	snapshotID int
	stateRoot  Hash
	preimages  map[Hash][]byte
}

// Snapshot represents a point-in-time state snapshot
type Snapshot struct {
	ID        int
	Accounts  map[Address]*Account
	StateRoot Hash
}

// NewStateManager creates a new state manager instance
func NewStateManager() *StateManager {
	return &StateManager{
		accounts:   make(map[Address]*Account),
		snapshots:  make(map[int]*Snapshot),
		snapshotID: 0,
		preimages:  make(map[Hash][]byte),
	}
}

// GetAccount retrieves an account by address
func (sm *StateManager) GetAccount(addr Address) (*Account, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	account, ok := sm.accounts[addr]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return cloneAccount(account), nil
}

// SetAccount saves an account to state
func (sm *StateManager) SetAccount(addr Address, account *Account) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.accounts[addr] = cloneAccount(account)
	return nil
}

// DeleteAccount removes an account from state
func (sm *StateManager) DeleteAccount(addr Address) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.accounts, addr)
	return nil
}

// HasAccount checks if an account exists
func (sm *StateManager) HasAccount(addr Address) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	_, ok := sm.accounts[addr]
	return ok
}

// GetStorage retrieves a storage value for an account
func (sm *StateManager) GetStorage(addr Address, key Hash) (Hash, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	account, ok := sm.accounts[addr]
	if !ok {
		return Hash{}, nil // Return zero hash for non-existent account
	}

	if val, ok := account.Storage[key]; ok {
		return val, nil
	}
	return Hash{}, nil
}

// SetStorage sets a storage value for an account
func (sm *StateManager) SetStorage(addr Address, key, value Hash) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	account, ok := sm.accounts[addr]
	if !ok {
		// Create account if it doesn't exist
		account = &Account{
			Address: addr,
			Balance: big.NewInt(0),
			Nonce:   0,
			Storage: make(map[Hash]Hash),
		}
		sm.accounts[addr] = account
	}

	account.Storage[key] = value
	return nil
}

// GetCode retrieves contract code
func (sm *StateManager) GetCode(addr Address) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	account, ok := sm.accounts[addr]
	if !ok {
		return nil, nil
	}
	return account.Code, nil
}

// SetCode saves contract code
func (sm *StateManager) SetCode(addr Address, code []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	account, ok := sm.accounts[addr]
	if !ok {
		// Create account if it doesn't exist
		account = &Account{
			Address: addr,
			Balance: big.NewInt(0),
			Nonce:   0,
			Storage: make(map[Hash]Hash),
		}
		sm.accounts[addr] = account
	}

	account.Code = code
	// Calculate code hash (simplified)
	if len(code) > 0 {
		account.CodeHash = Keccak256Hash(code).Bytes()
	} else {
		account.CodeHash = nil
	}
	return nil
}

// GetNonce retrieves an account's nonce
func (sm *StateManager) GetNonce(addr Address) (uint64, error) {
	account, err := sm.GetAccount(addr)
	if err != nil {
		return 0, err
	}
	return account.Nonce, nil
}

// SetNonce sets an account's nonce
func (sm *StateManager) SetNonce(addr Address, nonce uint64) error {
	account, err := sm.GetAccount(addr)
	if err != nil {
		return err
	}
	account.Nonce = nonce
	return sm.SetAccount(addr, account)
}

// GetBalance retrieves an account's balance
func (sm *StateManager) GetBalance(addr Address) (*big.Int, error) {
	account, err := sm.GetAccount(addr)
	if err != nil {
		return big.NewInt(0), nil // Return zero balance for non-existent account
	}
	return account.Balance, nil
}

// SetBalance sets an account's balance
func (sm *StateManager) SetBalance(addr Address, balance *big.Int) error {
	account, err := sm.GetAccount(addr)
	if err != nil {
		// Create account if it doesn't exist
		account = &Account{
			Address: addr,
			Balance: balance,
			Nonce:   0,
			Storage: make(map[Hash]Hash),
		}
		return sm.SetAccount(addr, account)
	}
	account.Balance = balance
	return sm.SetAccount(addr, account)
}

// AddBalance adds to an account's balance
func (sm *StateManager) AddBalance(addr Address, amount *big.Int) error {
	balance, err := sm.GetBalance(addr)
	if err != nil {
		return err
	}
	return sm.SetBalance(addr, new(big.Int).Add(balance, amount))
}

// SubBalance subtracts from an account's balance
func (sm *StateManager) SubBalance(addr Address, amount *big.Int) error {
	balance, err := sm.GetBalance(addr)
	if err != nil {
		return err
	}
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient balance")
	}
	return sm.SetBalance(addr, new(big.Int).Sub(balance, amount))
}

// Snapshot creates a snapshot of the current state
func (sm *StateManager) Snapshot() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.snapshotID++
	snapshot := &Snapshot{
		ID:        sm.snapshotID,
		Accounts:  make(map[Address]*Account),
		StateRoot: sm.stateRoot,
	}

	// Copy all accounts
	for addr, account := range sm.accounts {
		snapshot.Accounts[addr] = cloneAccount(account)
	}

	sm.snapshots[snapshot.ID] = snapshot
	return snapshot.ID
}

// RevertToSnapshot reverts state to a snapshot
func (sm *StateManager) RevertToSnapshot(id int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	snapshot, ok := sm.snapshots[id]
	if !ok {
		return
	}

	// Restore accounts from snapshot
	sm.accounts = make(map[Address]*Account)
	for addr, account := range snapshot.Accounts {
		sm.accounts[addr] = cloneAccount(account)
	}

	sm.stateRoot = snapshot.StateRoot

	// Remove all snapshots after this one
	for sid := range sm.snapshots {
		if sid >= id {
			delete(sm.snapshots, sid)
		}
	}
}

// GetStateRoot returns the current state root hash
func (sm *StateManager) GetStateRoot() (Hash, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Calculate state root from all accounts
	return sm.calculateStateRoot()
}

// SetStateRoot sets the state root (for initialization/verification)
func (sm *StateManager) SetStateRoot(root Hash) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.stateRoot = root
	return nil
}

// AddPreimage adds a preimage for debugging
func (sm *StateManager) AddPreimage(hash Hash, preimage []byte) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.preimages[hash] = preimage
}

// GetPreimage retrieves a preimage
func (sm *StateManager) GetPreimage(hash Hash) ([]byte, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	preimage, ok := sm.preimages[hash]
	return preimage, ok
}

// Commit commits all pending changes (no-op for memory state)
func (sm *StateManager) Commit() error {
	return nil
}

// Flush flushes all changes (no-op for memory state)
func (sm *StateManager) Flush() error {
	return nil
}

// Close closes the state manager (no-op for memory state)
func (sm *StateManager) Close() error {
	return nil
}

// ClearCache clears the in-memory cache
func (sm *StateManager) ClearCache() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.accounts = make(map[Address]*Account)
}

// GetAccountCount returns the number of accounts in state
func (sm *StateManager) GetAccountCount() (int, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.accounts), nil
}

// IterateAccounts iterates over all accounts
func (sm *StateManager) IterateAccounts(callback func(addr Address, account *Account) bool) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for addr, account := range sm.accounts {
		if !callback(addr, cloneAccount(account)) {
			break
		}
	}
	return nil
}

// Helper methods

func (sm *StateManager) calculateStateRoot() (Hash, error) {
	// Simple hash of all account addresses and balances
	// In production, this would be a proper Merkle Patricia Trie root
	var data []byte
	for addr, account := range sm.accounts {
		data = append(data, addr[:]...)
		balanceBytes := account.Balance.Bytes()
		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, uint32(len(balanceBytes)))
		data = append(data, lenBytes...)
		data = append(data, balanceBytes...)
	}
	return Keccak256Hash(data), nil
}

func cloneAccount(account *Account) *Account {
	if account == nil {
		return nil
	}

	clone := &Account{
		Address:  account.Address,
		Balance:  new(big.Int).Set(account.Balance),
		Nonce:    account.Nonce,
		CodeHash: make([]byte, len(account.CodeHash)),
		Code:     make([]byte, len(account.Code)),
		Storage:  make(map[Hash]Hash),
	}

	copy(clone.CodeHash, account.CodeHash)
	copy(clone.Code, account.Code)

	for k, v := range account.Storage {
		clone.Storage[k] = v
	}

	return clone
}

// EncodeAccount encodes account data for storage
func EncodeAccount(account *Account) []byte {
	balanceBytes := account.Balance.Bytes()
	data := make([]byte, 12+len(balanceBytes))

	binary.BigEndian.PutUint64(data[:8], account.Nonce)
	binary.BigEndian.PutUint32(data[8:12], uint32(len(balanceBytes)))
	copy(data[12:], balanceBytes)

	return data
}

// DecodeAccount decodes account data from storage
func DecodeAccount(addr Address, data []byte) (*Account, error) {
	if len(data) < 8 {
		return nil, ErrStateCorrupted
	}

	account := &Account{
		Address: addr,
		Balance: big.NewInt(0),
		Nonce:   binary.BigEndian.Uint64(data[:8]),
		Storage: make(map[Hash]Hash),
	}

	if len(data) >= 12 {
		balanceLen := binary.BigEndian.Uint32(data[8:12])
		if len(data) >= 12+int(balanceLen) {
			account.Balance = new(big.Int).SetBytes(data[12 : 12+balanceLen])
		}
	}

	return account, nil
}
