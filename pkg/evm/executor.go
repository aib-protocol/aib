package evm

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// EVMExecutor executes EVM code
type EVMExecutor struct {
	stateDB   *AIBStateDB
	gasCalc   *GasCalculator
	preimages map[common.Hash][]byte
	config    *params.ChainConfig
}

// ExecutionResult holds the result of EVM execution
type ExecutionResult struct {
	GasUsed    uint64
	ReturnData []byte
	Logs       []*types.Log
	Error      error
}

// NewEVMExecutor creates a new EVM executor
func NewEVMExecutor(chainID *big.Int) (*EVMExecutor, error) {
	stateDB := NewAIBStateDB()
	gasCalc := NewGasCalculator(&params.ChainConfig{})
	config := &params.ChainConfig{
		ChainID:             chainID,
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
	}

	return &EVMExecutor{
		stateDB:   stateDB,
		gasCalc:   gasCalc,
		preimages: make(map[common.Hash][]byte),
		config:    config,
	}, nil
}

// NewEVM creates a new EVM instance
func (e *EVMExecutor) NewEVM(blockCtx vm.BlockContext, txCtx vm.TxContext) *vm.EVM {
	return vm.NewEVM(blockCtx, txCtx, e.stateDB, e.config, vm.Config{})
}

// ExecuteTransaction executes a transaction
func (e *EVMExecutor) ExecuteTransaction(tx *types.Transaction, blockNumber *big.Int, blockHash common.Hash, from common.Address) (*ExecutionResult, error) {
	e.stateDB.SetBlockContext(blockNumber, blockHash, tx.Hash())

	intrinsicGas, err := e.gasCalc.IntrinsicGas(tx, true, true, true)
	if err != nil {
		return nil, err
	}

	if tx.Gas() < intrinsicGas {
		return nil, ErrIntrinsicGas
	}

	gasPrice := uint256.NewInt(0)
	if tx.GasPrice() != nil {
		gasPrice.SetBytes(tx.GasPrice().Bytes())
	}
	gasFee := new(uint256.Int).Mul(uint256.NewInt(tx.Gas()), gasPrice)
	if e.stateDB.GetBalance(from).Lt(gasFee) {
		return nil, ErrNotEnoughBalance
	}

	e.stateDB.SubBalance(from, gasFee, tracing.BalanceChangeUnspecified)

	var result *ExecutionResult
	var gasUsed uint64

	if tx.To() == nil {
		result, gasUsed, err = e.executeContractCreation(tx, from)
	} else {
		result, gasUsed, err = e.executeContractCall(tx, from)
	}

	if err != nil {
		return nil, err
	}

	gasRefund := gasUsed / 2
	if gasRefund > e.stateDB.GetRefund() {
		gasRefund = e.stateDB.GetRefund()
	}

	remainingGas := tx.Gas() - gasUsed + gasRefund
	refund := new(uint256.Int).Mul(uint256.NewInt(remainingGas), gasPrice)
	e.stateDB.AddBalance(from, refund, tracing.BalanceChangeUnspecified)

	return result, nil
}

// executeContractCreation executes contract creation
func (e *EVMExecutor) executeContractCreation(tx *types.Transaction, from common.Address) (*ExecutionResult, uint64, error) {
	snapshot := e.stateDB.Snapshot()

	e.stateDB.SetNonce(from, tx.Nonce()+1)

	blockCtx := vm.BlockContext{
		CanTransfer: func(db vm.StateDB, addr common.Address, amount *uint256.Int) bool {
			return db.GetBalance(addr).Cmp(amount) >= 0
		},
		Transfer: func(db vm.StateDB, sender, recipient common.Address, amount *uint256.Int) {
			db.SubBalance(sender, amount, tracing.BalanceChangeUnspecified)
			db.AddBalance(recipient, amount, tracing.BalanceChangeUnspecified)
		},
		GetHash: func(n uint64) common.Hash {
			return common.Hash{}
		},
		Coinbase:    common.Address{},
		BlockNumber: e.stateDB.blockNumber,
		Time:        0,
		Difficulty:  big.NewInt(0),
		GasLimit:    30000000,
		BaseFee:     big.NewInt(0),
	}

	txValue := uint256.NewInt(0)
	if tx.Value() != nil {
		txValue.SetBytes(tx.Value().Bytes())
	}

	txCtx := vm.TxContext{
		Origin:     from,
		GasPrice:   tx.GasPrice(),
		BlobHashes: nil,
		BlobFeeCap: big.NewInt(0),
	}

	evm := e.NewEVM(blockCtx, txCtx)

	ret, contractAddr, remainingGas, err := evm.Create(vm.AccountRef(from), tx.Data(), tx.Gas(), txValue)
	gasUsed := tx.Gas() - remainingGas

	if err != nil {
		e.stateDB.RevertToSnapshot(snapshot)
		return nil, 0, err
	}

	if len(ret) > 0 {
		e.stateDB.SetCode(contractAddr, ret)
	} else {
		e.stateDB.RevertToSnapshot(snapshot)
		return nil, 0, errors.New("contract creation failed - no code returned")
	}

	return &ExecutionResult{
		GasUsed:    gasUsed,
		ReturnData: ret,
		Logs:       e.stateDB.GetLogs(),
		Error:      nil,
	}, gasUsed, nil
}

// executeContractCall executes contract call
func (e *EVMExecutor) executeContractCall(tx *types.Transaction, from common.Address) (*ExecutionResult, uint64, error) {
	if IsPrecompiled(*tx.To()) {
		return e.executePrecompiled(tx)
	}

	blockCtx := vm.BlockContext{
		CanTransfer: func(db vm.StateDB, addr common.Address, amount *uint256.Int) bool {
			return db.GetBalance(addr).Cmp(amount) >= 0
		},
		Transfer: func(db vm.StateDB, sender, recipient common.Address, amount *uint256.Int) {
			db.SubBalance(sender, amount, tracing.BalanceChangeUnspecified)
			db.AddBalance(recipient, amount, tracing.BalanceChangeUnspecified)
		},
		GetHash: func(n uint64) common.Hash {
			return common.Hash{}
		},
		Coinbase:    common.Address{},
		BlockNumber: e.stateDB.blockNumber,
		Time:        0,
		Difficulty:  big.NewInt(0),
		GasLimit:    30000000,
		BaseFee:     big.NewInt(0),
	}

	txValue := uint256.NewInt(0)
	if tx.Value() != nil {
		txValue.SetBytes(tx.Value().Bytes())
	}

	txCtx := vm.TxContext{
		Origin:     from,
		GasPrice:   tx.GasPrice(),
		BlobHashes: nil,
		BlobFeeCap: big.NewInt(0),
	}

	evm := e.NewEVM(blockCtx, txCtx)

	ret, remainingGas, err := evm.Call(vm.AccountRef(from), *tx.To(), tx.Data(), tx.Gas(), txValue)
	gasUsed := tx.Gas() - remainingGas

	if err != nil {
		return nil, 0, err
	}

	return &ExecutionResult{
		GasUsed:    gasUsed,
		ReturnData: ret,
		Logs:       e.stateDB.GetLogs(),
		Error:      nil,
	}, gasUsed, nil
}

// executePrecompiled executes a precompiled contract
func (e *EVMExecutor) executePrecompiled(tx *types.Transaction) (*ExecutionResult, uint64, error) {
	contracts := PrecompiledContracts(e.config.ChainID)
	contract, exists := contracts[*tx.To()]
	if !exists {
		return nil, 0, errors.New("precompiled contract not found")
	}

	gas := contract.RequiredGas(tx.Data())

	if tx.Gas() < gas {
		return nil, 0, errors.New("insufficient gas for precompiled contract")
	}

	output, err := contract.Run(tx.Data())
	if err != nil {
		return nil, 0, err
	}

	return &ExecutionResult{
		GasUsed:    gas,
		ReturnData: output,
		Logs:       e.stateDB.GetLogs(),
		Error:      nil,
	}, gas, nil
}

// GetStateDB returns the state database
func (e *EVMExecutor) GetStateDB() *AIBStateDB {
	return e.stateDB
}

// GetStorage returns storage value at a key
func (e *EVMExecutor) GetStorage(addr common.Address, key common.Hash) common.Hash {
	return e.stateDB.GetState(addr, key)
}

// SetStorage sets storage value at a key
func (e *EVMExecutor) SetStorage(addr common.Address, key, value common.Hash) {
	e.stateDB.SetState(addr, key, value)
}

// GetBalance returns account balance
func (e *EVMExecutor) GetBalance(addr common.Address) *big.Int {
	return e.stateDB.GetBalance(addr).ToBig()
}

// SetBalance sets account balance
func (e *EVMExecutor) SetBalance(addr common.Address, balance *big.Int) {
	e.stateDB.AddBalance(addr, uint256.NewInt(0).SetBytes(balance.Bytes()), tracing.BalanceChangeUnspecified)
}

// GetNonce returns account nonce
func (e *EVMExecutor) GetNonce(addr common.Address) uint64 {
	return e.stateDB.GetNonce(addr)
}

// SetNonce sets account nonce
func (e *EVMExecutor) SetNonce(addr common.Address, nonce uint64) {
	e.stateDB.SetNonce(addr, nonce)
}

// CreateAccount creates a new account
func (e *EVMExecutor) CreateAccount(addr common.Address) {
	e.stateDB.CreateAccount(addr)
}

// HasAccount checks if account exists
func (e *EVMExecutor) HasAccount(addr common.Address) bool {
	return e.stateDB.Exist(addr)
}

// GetCode returns contract code
func (e *EVMExecutor) GetCode(addr common.Address) []byte {
	return e.stateDB.GetCode(addr)
}

// SetCode sets contract code
func (e *EVMExecutor) SetCode(addr common.Address, code []byte) {
	e.stateDB.SetCode(addr, code)
}

// Commit commits changes
func (e *EVMExecutor) Commit() (common.Hash, error) {
	return e.stateDB.Commit()
}

// GetLogs returns execution logs
func (e *EVMExecutor) GetLogs() []*types.Log {
	return e.stateDB.GetLogs()
}

// ValidateTransaction validates a transaction before execution
func (e *EVMExecutor) ValidateTransaction(tx *types.Transaction, from common.Address) error {
	intrinsicGas, err := e.gasCalc.IntrinsicGas(tx, true, true, true)
	if err != nil {
		return err
	}

	if tx.Gas() < intrinsicGas {
		return ErrIntrinsicGas
	}

	gasPrice := uint256.NewInt(0)
	if tx.GasPrice() != nil {
		gasPrice.SetBytes(tx.GasPrice().Bytes())
	}
	gasFee := new(uint256.Int).Mul(uint256.NewInt(tx.Gas()), gasPrice)
	if e.stateDB.GetBalance(from).Lt(gasFee) {
		return ErrNotEnoughBalance
	}

	return nil
}

// ExecuteDirect executes code directly (for testing)
func (e *EVMExecutor) ExecuteDirect(caller common.Address, contractAddr common.Address, code []byte, input []byte, value *big.Int, gas uint64) (*ExecutionResult, error) {
	if !e.stateDB.Exist(caller) {
		e.stateDB.CreateAccount(caller)
	}
	if !e.stateDB.Exist(contractAddr) {
		e.stateDB.CreateAccount(contractAddr)
	}

	blockCtx := vm.BlockContext{
		CanTransfer: func(db vm.StateDB, addr common.Address, amount *uint256.Int) bool {
			return db.GetBalance(addr).Cmp(amount) >= 0
		},
		Transfer: func(db vm.StateDB, sender, recipient common.Address, amount *uint256.Int) {
			db.SubBalance(sender, amount, tracing.BalanceChangeUnspecified)
			db.AddBalance(recipient, amount, tracing.BalanceChangeUnspecified)
		},
		GetHash: func(n uint64) common.Hash {
			return common.Hash{}
		},
		Coinbase:    common.Address{},
		BlockNumber: big.NewInt(0),
		Time:        0,
		Difficulty:  big.NewInt(0),
		GasLimit:    30000000,
		BaseFee:     big.NewInt(0),
	}

	txValue := uint256.NewInt(0)
	if value != nil {
		txValue.SetBytes(value.Bytes())
	}

	txCtx := vm.TxContext{
		Origin:     caller,
		GasPrice:   big.NewInt(0),
		BlobHashes: nil,
		BlobFeeCap: big.NewInt(0),
	}

	evm := e.NewEVM(blockCtx, txCtx)

	ret, remainingGas, err := evm.Call(vm.AccountRef(caller), contractAddr, input, gas, txValue)
	if err != nil {
		return nil, err
	}

	return &ExecutionResult{
		GasUsed:    gas - remainingGas,
		ReturnData: ret,
		Logs:       e.stateDB.GetLogs(),
		Error:      nil,
	}, nil
}
