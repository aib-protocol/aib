package aal

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/aib-protocol/aib/internal/interfaces"
)

var (
	ErrEVMExecutionFailed = errors.New("EVM execution failed")
	ErrOutOfGas          = errors.New("out of gas")
	ErrInvalidTransaction = errors.New("invalid transaction")
	ErrGasLimitExceeded   = errors.New("gas limit exceeded")
)

// ChainConfigAlias is an alias to ChainConfig from types.go

// EVMConfig contains configuration for the EVM executor
type EVMConfig struct {
	ChainID       *big.Int
	BlockNumber   *big.Int
	BlockTime     uint64
	Coinbase      Address
	GasLimit      uint64
	BaseFee       *big.Int
	Difficulty    *big.Int
	EnableTracing bool
}

// EVMExecutor handles EVM contract execution (simplified implementation)
type EVMExecutor struct {
	stateManager *StateManager
	converter    *UTXOToAccountConverter
	chainConfig  *ChainConfig
	gasCalc      *GasCalculator
	blockContext *BlockContext
}

// BlockContext contains block-level information for EVM execution
type BlockContext struct {
	CanTransfer func(db StateDB, addr Address, amount *big.Int) bool
	Transfer   func(db StateDB, sender, recipient Address, amount *big.Int)
	GetHash    func(uint64) Hash
	Coinbase   Address
	GasLimit   uint64
	BlockNumber *big.Int
	Time        uint64
	Difficulty  *big.Int
	BaseFee     *big.Int
	Random      *big.Int
}

// StateDB interface for EVM state operations
type StateDB interface {
	CreateAccount(addr Address)
	SubBalance(addr Address, amount *big.Int)
	AddBalance(addr Address, amount *big.Int)
	GetBalance(addr Address) *big.Int
	GetNonce(addr Address) uint64
	SetNonce(addr Address, nonce uint64)
	GetCodeHash(addr Address) Hash
	GetCode(addr Address) []byte
	SetCode(addr Address, code []byte)
	GetCodeSize(addr Address) int
	AddRefund(gas uint64)
	SubRefund(gas uint64)
	GetRefund() uint64
	GetState(addr Address, key Hash) Hash
	SetState(addr Address, key, value Hash)
	GetTransientState(addr Address, key Hash) Hash
	SetTransientState(addr Address, key, value Hash)
	SelfDestruct(addr Address)
	HasSelfDestructed(addr Address) bool
	Exist(addr Address) bool
	Empty(addr Address) bool
	RevertToSnapshot(revid int)
	Snapshot() int
	AddLog(log *Log)
	GetLogs() []*Log
}

// EVMLog represents an EVM log entry (alias to Log from types.go)
type EVMLog = Log

// NewEVMExecutor creates a new EVM executor instance
func NewEVMExecutor(
	stateManager *StateManager,
	converter *UTXOToAccountConverter,
	config *EVMConfig,
) *EVMExecutor {
	if config.ChainID == nil {
		config.ChainID = big.NewInt(8888) // Default AIB chain ID
	}
	if config.BlockNumber == nil {
		config.BlockNumber = big.NewInt(0)
	}
	if config.GasLimit == 0 {
		config.GasLimit = 30000000 // 30M default
	}
	if config.BaseFee == nil {
		config.BaseFee = big.NewInt(1000000000) // 1 Gwei
	}
	if config.Difficulty == nil {
		config.Difficulty = big.NewInt(1)
	}

	chainConfig := &ChainConfig{
		ChainID: config.ChainID,
	}

	blockContext := &BlockContext{
		CanTransfer: CanTransferSimple,
		Transfer:    TransferSimple,
		GetHash:     func(n uint64) Hash { return Hash{} },
		Coinbase:    config.Coinbase,
		GasLimit:    config.GasLimit,
		BlockNumber: config.BlockNumber,
		Time:        config.BlockTime,
		Difficulty:  config.Difficulty,
		BaseFee:     config.BaseFee,
		Random:      nil,
	}

	return &EVMExecutor{
		stateManager: stateManager,
		converter:    converter,
		chainConfig:  chainConfig,
		gasCalc:      NewGasCalculator(),
		blockContext: blockContext,
	}
}

// UpdateBlockContext updates the block context for execution
func (e *EVMExecutor) UpdateBlockContext(ctx *BlockContext) {
	e.blockContext = ctx
}

// ExecuteTransaction executes a complete transaction
func (e *EVMExecutor) ExecuteTransaction(tx *Transaction) (*TransactionResult, error) {
	// Validate transaction
	if err := e.validateTransaction(tx); err != nil {
		return nil, err
	}

	// Create account state wrapper
	accountState := NewAccountState(e.stateManager, e.converter)

	// Calculate intrinsic gas
	isContractCreation := tx.To == nil
	intrinsicGas := e.gasCalc.CalculateIntrinsicGas(tx.Data, isContractCreation)

	if tx.GasLimit < intrinsicGas {
		return nil, fmt.Errorf("%w: intrinsic gas exceeds gas limit", ErrOutOfGas)
	}

	// Check balance (including gas costs)
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.GasLimit), tx.GasPrice)
	totalCost := new(big.Int).Add(tx.Value, gasCost)

	if accountState.GetBalance(tx.From).Cmp(totalCost) < 0 {
		return nil, ErrInsufficientBalance
	}

	// Deduct gas fee upfront
	accountState.SubBalance(tx.From, gasCost)

	// Execute based on transaction type
	var result *TransactionResult
	if isContractCreation {
		result = e.deployContract(accountState, tx)
	} else {
		result = e.callContract(accountState, tx)
	}

	// Calculate gas refund
	if result.Error == nil {
		// Refund unused gas
		unusedGas := tx.GasLimit - result.GasUsed
		refund := new(big.Int).Mul(new(big.Int).SetUint64(unusedGas), tx.GasPrice)
		accountState.AddBalance(tx.From, refund)

		// Apply gas refund cap (max refund is 1/5th of gas used)
		refundGas := result.GasRefund
		maxRefund := result.GasUsed / 5
		if refundGas > maxRefund {
			refundGas = maxRefund
		}

		if refundGas > 0 {
			refundValue := new(big.Int).Mul(new(big.Int).SetUint64(refundGas), tx.GasPrice)
			accountState.AddBalance(tx.From, refundValue)
		}
	}

	// Get state root
	stateRoot, err := e.stateManager.GetStateRoot()
	if err != nil {
		return nil, err
	}
	result.StateRoot = stateRoot

	return result, nil
}

// validateTransaction validates a transaction before execution
func (e *EVMExecutor) validateTransaction(tx *Transaction) error {
	if tx == nil {
		return ErrInvalidTransaction
	}

	if tx.From == (Address{}) {
		return fmt.Errorf("%w: missing from address", ErrInvalidTransaction)
	}

	if tx.GasLimit == 0 {
		return fmt.Errorf("%w: gas limit is zero", ErrInvalidTransaction)
	}

	if tx.GasLimit > e.blockContext.GasLimit {
		return fmt.Errorf("%w: exceeds block gas limit", ErrGasLimitExceeded)
	}

	if tx.Value == nil {
		tx.Value = big.NewInt(0)
	}

	if tx.GasPrice == nil {
		tx.GasPrice = big.NewInt(0)
	}

	return nil
}

// deployContract handles contract deployment (simplified)
func (e *EVMExecutor) deployContract(state *AccountState, tx *Transaction) *TransactionResult {
	// Calculate contract address from sender and nonce
	contractAddr := CreateAddress(tx.From, state.GetNonce(tx.From))

	// Increment nonce
	state.SetNonce(tx.From, state.GetNonce(tx.From)+1)

	// In a full EVM, this would execute the contract code
	// For simplified version, just store the code
	gasUsed := e.gasCalc.CalculateIntrinsicGas(tx.Data, true)
	if tx.GasLimit > gasUsed {
		gasUsed = tx.GasLimit
	}

	result := &TransactionResult{
		GasUsed:      gasUsed,
		ReturnData:   tx.Data, // In real EVM, this would be the return data
		Error:        nil,
		Logs:         state.GetLogs(),
		ContractAddr: &contractAddr,
	}

	// Store contract code
	state.SetCode(contractAddr, tx.Data)

	return result
}

// callContract handles contract calls (simplified)
func (e *EVMExecutor) callContract(state *AccountState, tx *Transaction) *TransactionResult {
	// Increment nonce
	state.SetNonce(tx.From, state.GetNonce(tx.From)+1)

	// Transfer value if present
	if tx.Value.Sign() > 0 {
		// Check if sender has sufficient balance for value transfer
		// (gas cost was already deducted in ExecuteTransaction)
		if state.GetBalance(tx.From).Cmp(tx.Value) < 0 {
			return &TransactionResult{
				GasUsed: tx.GasLimit,
				Error:   ErrInsufficientBalance,
				Logs:    state.GetLogs(),
			}
		}

		// Transfer value from sender to recipient
		e.blockContext.Transfer(state, tx.From, *tx.To, tx.Value)
	}

	// Get contract code
	code := state.GetCode(*tx.To)

	// In a full EVM, this would execute the contract code
	// For simplified version, we just track gas
	gasUsed := e.gasCalc.CalculateIntrinsicGas(tx.Data, false)
	// Add some gas for code execution
	gasUsed += uint64(len(code)) * 3
	if gasUsed > tx.GasLimit {
		gasUsed = tx.GasLimit
	}

	leftOverGas := tx.GasLimit - gasUsed

	result := &TransactionResult{
		GasUsed:    gasUsed,
		ReturnData: []byte{}, // Empty return in simplified version
		Error:      nil,
		Logs:       state.GetLogs(),
	}

	// Execute any code if present (simplified - just call)
	if len(code) > 0 {
		// In real EVM: execute the code
		// For now: just return success
		result.ReturnData = executeSimpleContract(code, tx.Data)
	}

	_ = leftOverGas // Could be used for refund calculation
	return result
}

// executeSimpleContract executes contract code in a very simplified manner
func executeSimpleContract(code, input []byte) []byte {
	// This is a placeholder for actual EVM execution
	// In a real implementation, this would interpret EVM bytecode
	if len(code) == 0 {
		return nil
	}
	// Return empty for now
	return []byte{}
}

// Call executes a static call (no state changes)
func (e *EVMExecutor) Call(caller, callee Address, input []byte, gas uint64) ([]byte, uint64, error) {
	accountState := NewAccountState(e.stateManager, e.converter)

	// Get contract code
	code := accountState.GetCode(callee)
	if len(code) == 0 {
		return []byte{}, gas, nil
	}

	// Simplified execution
	result := executeSimpleContract(code, input)
	gasUsed := uint64(len(input)) * 3

	return result, gas - gasUsed, nil
}

// StaticCall executes a static call that cannot modify state
func (e *EVMExecutor) StaticCall(caller, callee Address, input []byte, gas uint64) ([]byte, uint64, error) {
	return e.Call(caller, callee, input, gas)
}

// GetCode returns the code at a given address
func (e *EVMExecutor) GetCode(addr Address) []byte {
	accountState := NewAccountState(e.stateManager, e.converter)
	return accountState.GetCode(addr)
}

// GetCodeHash returns the code hash at a given address
func (e *EVMExecutor) GetCodeHash(addr Address) Hash {
	accountState := NewAccountState(e.stateManager, e.converter)
	return accountState.GetCodeHash(addr)
}

// GetBalance returns the balance of an address
func (e *EVMExecutor) GetBalance(addr Address) *big.Int {
	accountState := NewAccountState(e.stateManager, e.converter)
	return accountState.GetBalance(addr)
}

// GetNonce returns the nonce of an address
func (e *EVMExecutor) GetNonce(addr Address) uint64 {
	accountState := NewAccountState(e.stateManager, e.converter)
	return accountState.GetNonce(addr)
}

// EstimateGas estimates the gas required for a transaction
func (e *EVMExecutor) EstimateGas(tx *Transaction) (uint64, error) {
	// Simple estimation based on intrinsic gas
	hi := e.blockContext.GasLimit
	lo := e.gasCalc.CalculateIntrinsicGas(tx.Data, tx.To == nil)

	if tx.GasLimit != 0 && tx.GasLimit < hi {
		hi = tx.GasLimit
	}

	// Binary search (simplified)
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if mid >= lo {
			hi = mid
		} else {
			break
		}
	}

	if hi < lo {
		hi = lo
	}

	return hi, nil
}

// CanTransferSimple checks if an address has sufficient balance
func CanTransferSimple(db StateDB, addr Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// TransferSimple transfers value between addresses
func TransferSimple(db StateDB, sender, recipient Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

// CreateAddress creates a new contract address from sender and nonce
func CreateAddress(sender Address, nonce uint64) Address {
	// RLP encode sender + nonce and hash
	data := append(sender[:], encodeNonce(nonce)...)
	hash := Keccak256Hash(data)
	var addr Address
	copy(addr[:], hash[12:]) // Take last 20 bytes for address
	return addr
}

// UTXOToEVMTransaction converts a UTXO-based transaction to an EVM transaction
func UTXOToEVMTransaction(
	from interfaces.Address,
	to *interfaces.Address,
	value uint64,
	data []byte,
	gasLimit uint64,
	gasPrice *big.Int,
	nonce uint64,
) *Transaction {
	tx := &Transaction{
		From:     ConvertInterfacesAddress(from),
		Value:    new(big.Int).SetUint64(value),
		Data:     data,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
		Nonce:    nonce,
	}

	if to != nil {
		evmAddr := ConvertInterfacesAddress(*to)
		tx.To = &evmAddr
	}

	return tx
}

// ExecuteUTXOTransaction executes a transaction funded by UTXOs
func (e *EVMExecutor) ExecuteUTXOTransaction(
	from interfaces.Address,
	to *interfaces.Address,
	value uint64,
	data []byte,
	gasLimit uint64,
	gasPrice *big.Int,
	utxoInputs []interfaces.TXInput,
) (*TransactionResult, error) {
	// Convert to EVM transaction
	tx := UTXOToEVMTransaction(from, to, value, data, gasLimit, gasPrice, 0)
	tx.UTXOInputs = utxoInputs

	// Validate UTXO inputs have sufficient value
	var totalInput uint64
	for _, input := range utxoInputs {
		utxo, err := e.converter.utxoProvider.GetUTXO(input.TxHash, input.Index)
		if err != nil {
			return nil, fmt.Errorf("invalid UTXO input: %w", err)
		}
		totalInput += utxo.Value
	}

	// Check if UTXO inputs cover value + gas
	totalCost := value + (gasLimit * gasPrice.Uint64())
	if totalInput < totalCost {
		return nil, ErrInsufficientBalance
	}

	// Get or create sender account from UTXO
	account, err := e.converter.GetOrCreateAccount(from)
	if err != nil {
		return nil, err
	}

	// Update nonce
	tx.Nonce = account.Nonce

	// Execute the transaction
	return e.ExecuteTransaction(tx)
}

// PrecompileManager manages AIB-specific precompiled contracts
type PrecompileManager struct {
	precompiles map[Address]PrecompiledContract
}

// PrecompiledContract interface for precompiled contracts
type PrecompiledContract interface {
	RequiredGas(input []byte) uint64
	Run(input []byte) ([]byte, error)
}

// NewPrecompileManager creates a new precompile manager
func NewPrecompileManager() *PrecompileManager {
	return &PrecompileManager{
		precompiles: make(map[Address]PrecompiledContract),
	}
}

// RegisterPrecompile registers a custom precompiled contract
func (pm *PrecompileManager) RegisterPrecompile(addr Address, contract PrecompiledContract) {
	pm.precompiles[addr] = contract
}

// GetPrecompile returns a precompiled contract by address
func (pm *PrecompileManager) GetPrecompile(addr Address) (PrecompiledContract, bool) {
	contract, ok := pm.precompiles[addr]
	return contract, ok
}

// AIBPrecompiledContracts returns AIB-specific precompiled contracts
func AIBPrecompiledContracts() map[Address]PrecompiledContract {
	contracts := make(map[Address]PrecompiledContract)
	// AIB-specific precompiles can be added here
	// Example: UTXO verification precompile
	return contracts
}
