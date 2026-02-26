package aal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/aib-protocol/aib/internal/interfaces"
)

var (
	ErrInvalidAddress     = errors.New("invalid address")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidUTXO        = errors.New("invalid UTXO")
	ErrConversionFailed   = errors.New("UTXO to account conversion failed")
	ErrNonceOverflow      = errors.New("nonce overflow")
)

// UTXOToAccountConverter handles conversion between UTXO and account-based models
type UTXOToAccountConverter struct {
	stateManager *StateManager
	utxoProvider interfaces.UTXOProvider
}

// NewUTXOToAccountConverter creates a new converter instance
func NewUTXOToAccountConverter(stateManager *StateManager, utxoProvider interfaces.UTXOProvider) *UTXOToAccountConverter {
	return &UTXOToAccountConverter{
		stateManager: stateManager,
		utxoProvider: utxoProvider,
	}
}

// GetOrCreateAccount retrieves or creates an account from UTXO data
func (c *UTXOToAccountConverter) GetOrCreateAccount(utxoAddr interfaces.Address) (*Account, error) {
	evmAddr := ConvertInterfacesAddress(utxoAddr)

	// Try to get existing account
	account, err := c.stateManager.GetAccount(evmAddr)
	if err == nil {
		return account, nil
	}

	// Calculate balance from UTXOs
	balance, err := c.utxoProvider.GetBalance(utxoAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConversionFailed, err)
	}

	// Create new account
	account = &Account{
		Address:  evmAddr,
		Balance:  new(big.Int).SetUint64(balance),
		Nonce:    0,
		CodeHash: nil,
		Code:     nil,
		Storage:  make(map[Hash]Hash),
	}

	// Save to state
	if err := c.stateManager.SetAccount(evmAddr, account); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConversionFailed, err)
	}

	return account, nil
}

// UpdateAccountFromUTXO updates account balance based on current UTXO state
func (c *UTXOToAccountConverter) UpdateAccountFromUTXO(utxoAddr interfaces.Address) error {
	evmAddr := ConvertInterfacesAddress(utxoAddr)

	// Get current balance from UTXO
	balance, err := c.utxoProvider.GetBalance(utxoAddr)
	if err != nil {
		return err
	}

	// Get or create account
	account, err := c.stateManager.GetAccount(evmAddr)
	if err != nil {
		account = &Account{
			Address: evmAddr,
			Nonce:   0,
			Storage: make(map[Hash]Hash),
		}
	}

	// Update balance
	account.Balance = new(big.Int).SetUint64(balance)

	return c.stateManager.SetAccount(evmAddr, account)
}

// SpendUTXOAndUpdateAccount spends a UTXO and updates the corresponding account
func (c *UTXOToAccountConverter) SpendUTXOAndUpdateAccount(
	input interfaces.TXInput,
	utxoAddr interfaces.Address,
) error {
	// Mark UTXO as spent (requires signature validation by UTXO provider)
	if err := c.utxoProvider.SpendUTXO(input, input.Signature); err != nil {
		return fmt.Errorf("failed to spend UTXO: %w", err)
	}

	// Update account balance
	return c.UpdateAccountFromUTXO(utxoAddr)
}

// AccountState implements StateDB interface for EVM compatibility
type AccountState struct {
	stateManager *StateManager
	converter    *UTXOToAccountConverter

	// Transient state for current transaction
	transientStorage map[Address]map[Hash]Hash
	logs             []*Log
	refund           uint64
	accessList       *AccessList
}

// NewAccountState creates a new account state wrapper
func NewAccountState(stateManager *StateManager, converter *UTXOToAccountConverter) *AccountState {
	return &AccountState{
		stateManager:     stateManager,
		converter:        converter,
		transientStorage: make(map[Address]map[Hash]Hash),
		logs:             make([]*Log, 0),
		refund:           0,
		accessList:       NewAccessList(),
	}
}

// CreateAccount creates a new account
func (s *AccountState) CreateAccount(addr Address) {
	account := &Account{
		Address: addr,
		Balance: big.NewInt(0),
		Nonce:   0,
		Storage: make(map[Hash]Hash),
	}
	s.stateManager.SetAccount(addr, account)
}

// SubBalance subtracts balance from an account
func (s *AccountState) SubBalance(addr Address, amount *big.Int) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return
	}
	account.Balance = new(big.Int).Sub(account.Balance, amount)
	s.stateManager.SetAccount(addr, account)
}

// AddBalance adds balance to an account
func (s *AccountState) AddBalance(addr Address, amount *big.Int) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		// Create account if it doesn't exist
		account = &Account{
			Address: addr,
			Balance: big.NewInt(0),
			Nonce:   0,
			Storage: make(map[Hash]Hash),
		}
	}
	account.Balance = new(big.Int).Add(account.Balance, amount)
	s.stateManager.SetAccount(addr, account)
}

// GetBalance returns the balance of an account
func (s *AccountState) GetBalance(addr Address) *big.Int {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return big.NewInt(0)
	}
	return account.Balance
}

// GetNonce returns the nonce of an account
func (s *AccountState) GetNonce(addr Address) uint64 {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return 0
	}
	return account.Nonce
}

// SetNonce sets the nonce of an account
func (s *AccountState) SetNonce(addr Address, nonce uint64) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return
	}
	account.Nonce = nonce
	s.stateManager.SetAccount(addr, account)
}

// GetCodeHash returns the code hash of an account
func (s *AccountState) GetCodeHash(addr Address) Hash {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil || account.CodeHash == nil {
		return Hash{}
	}
	return BytesToHash(account.CodeHash)
}

// GetCode returns the code of an account
func (s *AccountState) GetCode(addr Address) []byte {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return nil
	}
	return account.Code
}

// SetCode sets the code of an account
func (s *AccountState) SetCode(addr Address, code []byte) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return
	}
	account.Code = code
	// Calculate code hash
	account.CodeHash = Keccak256Hash(code).Bytes()
	s.stateManager.SetAccount(addr, account)
}

// GetCodeSize returns the size of the code of an account
func (s *AccountState) GetCodeSize(addr Address) int {
	code := s.GetCode(addr)
	return len(code)
}

// AddRefund adds gas refund
func (s *AccountState) AddRefund(gas uint64) {
	s.refund += gas
}

// SubRefund subtracts gas refund
func (s *AccountState) SubRefund(gas uint64) {
	if s.refund >= gas {
		s.refund -= gas
	}
}

// GetRefund returns the current gas refund
func (s *AccountState) GetRefund() uint64 {
	return s.refund
}

// GetState returns a storage value
func (s *AccountState) GetState(addr Address, key Hash) Hash {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return Hash{}
	}
	if val, ok := account.Storage[key]; ok {
		return val
	}
	return Hash{}
}

// SetState sets a storage value
func (s *AccountState) SetState(addr Address, key, value Hash) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return
	}
	account.Storage[key] = value
	s.stateManager.SetAccount(addr, account)
}

// GetTransientState returns a transient storage value
func (s *AccountState) GetTransientState(addr Address, key Hash) Hash {
	if addrMap, ok := s.transientStorage[addr]; ok {
		if val, ok := addrMap[key]; ok {
			return val
		}
	}
	return Hash{}
}

// SetTransientState sets a transient storage value
func (s *AccountState) SetTransientState(addr Address, key, value Hash) {
	if s.transientStorage[addr] == nil {
		s.transientStorage[addr] = make(map[Hash]Hash)
	}
	s.transientStorage[addr][key] = value
}

// SelfDestruct marks an account as self-destructed
func (s *AccountState) SelfDestruct(addr Address) {
	s.stateManager.DeleteAccount(addr)
}

// HasSelfDestructed checks if an account has self-destructed
func (s *AccountState) HasSelfDestructed(addr Address) bool {
	_, err := s.stateManager.GetAccount(addr)
	return err != nil
}

// Exist checks if an account exists
func (s *AccountState) Exist(addr Address) bool {
	_, err := s.stateManager.GetAccount(addr)
	return err == nil
}

// Empty checks if an account is empty
func (s *AccountState) Empty(addr Address) bool {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return true
	}
	return account.Nonce == 0 &&
		account.Balance.Sign() == 0 &&
		len(account.Code) == 0
}

// RevertToSnapshot reverts to a snapshot
func (s *AccountState) RevertToSnapshot(revid int) {
	s.stateManager.RevertToSnapshot(revid)
}

// Snapshot creates a snapshot
func (s *AccountState) Snapshot() int {
	return s.stateManager.Snapshot()
}

// AddLog adds a log
func (s *AccountState) AddLog(log *Log) {
	s.logs = append(s.logs, log)
}

// GetLogs returns all logs
func (s *AccountState) GetLogs() []*Log {
	return s.logs
}

// AddPreimage adds a preimage
func (s *AccountState) AddPreimage(hash Hash, preimage []byte) {
	// Store preimage for debugging
	s.stateManager.AddPreimage(hash, preimage)
}

// ForEachStorage iterates over all storage entries
func (s *AccountState) ForEachStorage(addr Address, cb func(Hash, Hash) bool) {
	account, err := s.stateManager.GetAccount(addr)
	if err != nil {
		return
	}
	for k, v := range account.Storage {
		if !cb(k, v) {
			break
		}
	}
}

// Prepare prepares the access list
func (s *AccountState) Prepare(rules Rules, sender, coinbase Address, dest *Address, precompiles []Address, txAccesses []Address) {
	s.accessList = NewAccessList()

	// Add sender to access list
	s.accessList.AddAddress(sender)

	// Add destination to access list
	if dest != nil {
		s.accessList.AddAddress(*dest)
	}

	// Add coinbase to access list
	s.accessList.AddAddress(coinbase)

	// Add precompiles to access list
	for _, addr := range precompiles {
		s.accessList.AddAddress(addr)
	}

	// Add transaction accesses
	for _, addr := range txAccesses {
		s.accessList.AddAddress(addr)
	}
}

// AddressInAccessList checks if an address is in the access list
func (s *AccountState) AddressInAccessList(addr Address) bool {
	return s.accessList.ContainsAddress(addr)
}

// SlotInAccessList checks if an address and slot are in the access list
func (s *AccountState) SlotInAccessList(addr Address, slot Hash) (addressOk bool, slotOk bool) {
	return s.accessList.Contains(addr, slot)
}

// AddAddressToAccessList adds an address to the access list
func (s *AccountState) AddAddressToAccessList(addr Address) {
	s.accessList.AddAddress(addr)
}

// AddSlotToAccessList adds an address and slot to the access list
func (s *AccountState) AddSlotToAccessList(addr Address, slot Hash) {
	s.accessList.AddSlot(addr, slot)
}

// Selfdestruct6780 marks an account as self-destructed (EIP-6780)
func (s *AccountState) Selfdestruct6780(addr Address) {
	s.SelfDestruct(addr)
}

// AccessList manages accessed addresses and storage slots
type AccessList struct {
	addresses map[Address]struct{}
	slots     map[Address]map[Hash]struct{}
}

// NewAccessList creates a new access list
func NewAccessList() *AccessList {
	return &AccessList{
		addresses: make(map[Address]struct{}),
		slots:     make(map[Address]map[Hash]struct{}),
	}
}

// AddAddress adds an address to the access list
func (al *AccessList) AddAddress(addr Address) {
	al.addresses[addr] = struct{}{}
}

// AddSlot adds a storage slot to the access list
func (al *AccessList) AddSlot(addr Address, slot Hash) {
	al.AddAddress(addr)
	if al.slots[addr] == nil {
		al.slots[addr] = make(map[Hash]struct{})
	}
	al.slots[addr][slot] = struct{}{}
}

// ContainsAddress checks if an address is in the access list
func (al *AccessList) ContainsAddress(addr Address) bool {
	_, ok := al.addresses[addr]
	return ok
}

// Contains checks if an address and slot are in the access list
func (al *AccessList) Contains(addr Address, slot Hash) (bool, bool) {
	_, addressOk := al.addresses[addr]
	if al.slots[addr] != nil {
		_, slotOk := al.slots[addr][slot]
		return addressOk, slotOk
	}
	return addressOk, false
}

// Transaction represents an AAL transaction
type Transaction struct {
	From        Address
	To          *Address // nil for contract creation
	Value       *big.Int
	Data        []byte
	GasLimit    uint64
	GasPrice    *big.Int
	Nonce       uint64
	UTXOInputs  []interfaces.TXInput // Associated UTXO inputs for funding
}

// TransactionResult contains the result of executing a transaction
type TransactionResult struct {
	GasUsed      uint64
	GasRefund    uint64
	ReturnData   []byte
	Error        error
	Logs         []*Log
	ContractAddr *Address // set for contract creation
	StateRoot    Hash
}

// GasCalculator handles gas calculation for AAL operations
type GasCalculator struct {
	baseCost   uint64
	sstoreCost uint64
	callCost   uint64
}

// NewGasCalculator creates a new gas calculator
func NewGasCalculator() *GasCalculator {
	return &GasCalculator{
		baseCost:   21000, // Standard EVM tx cost
		sstoreCost: 20000, // SSTORE cost
		callCost:   700,   // CALL base cost
	}
}

// CalculateIntrinsicGas calculates the intrinsic gas for a transaction
func (gc *GasCalculator) CalculateIntrinsicGas(data []byte, isContractCreation bool) uint64 {
	gas := gc.baseCost

	if isContractCreation {
		gas += 32000 // Contract creation extra cost
	}

	// Calculate data cost
	for _, b := range data {
		if b == 0 {
			gas += 4 // Zero byte cost
		} else {
			gas += 16 // Non-zero byte cost
		}
	}

	return gas
}

// CalculateGasLimit calculates the effective gas limit
func (gc *GasCalculator) CalculateGasLimit(desiredLimit, blockGasLimit uint64) uint64 {
	if desiredLimit > blockGasLimit {
		return blockGasLimit
	}
	return desiredLimit
}

// encodeNonce encodes nonce to bytes for storage
func encodeNonce(nonce uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, nonce)
	return b
}

// decodeNonce decodes nonce from bytes
func decodeNonce(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
