package evm

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

var (
	ErrGasLimitReached  = errors.New("gas limit reached")
	ErrIntrinsicGas     = errors.New("intrinsic gas insufficient")
	ErrNotEnoughBalance = errors.New("not enough balance")
)

// GasCalculator calculates gas costs for EVM operations
type GasCalculator struct {
	config *params.ChainConfig
}

// NewGasCalculator creates a new gas calculator
func NewGasCalculator(config *params.ChainConfig) *GasCalculator {
	return &GasCalculator{config: config}
}

// IntrinsicGas calculates the intrinsic gas cost for a transaction
func (g *GasCalculator) IntrinsicGas(tx *types.Transaction, isHomestead, isEIP155, isEIP2028 bool) (uint64, error) {
	var gas uint64

	data := tx.Data()

	if len(data) > 0 {
		nonZero := 0
		for _, b := range data {
			if b != 0 {
				nonZero++
			}
		}

		zeroBytes := len(data) - nonZero

		if isEIP2028 {
			gas += uint64(zeroBytes)*params.TxDataZeroGas + uint64(nonZero)*params.TxDataNonZeroGasEIP2028
		} else {
			gas += uint64(zeroBytes)*params.TxDataZeroGas + uint64(nonZero)*params.TxDataNonZeroGasFrontier
		}
	}

	if tx.To() == nil {
		gas += params.CreateGas
		if isHomestead {
			gas += params.CreateDataGas * uint64(len(data))
		}
	}

	if isEIP155 && len(tx.AccessList()) > 0 {
		// Access list gas cost
		gas += 20 * uint64(len(tx.AccessList()))
	}

	return gas, nil
}

// GasCost constants
const (
	MemoryGas uint64 = 3
	StackMin  int    = 0
	StackMax  int    = 1024
)

// OpCode represents an EVM opcode
type OpCode uint8

// EVM opcodes
const (
	STOP OpCode = iota
	ADD
	MUL
	SUB
	DIV
	SDIV
	MOD
	SMOD
	ADDMOD
	MULMOD
	EXP
	SIGNEXTEND
	LT
	GT
	SLT
	SGT
	EQ
	ISZERO
	AND
	OR
	XOR
	NOT
	BYTE
	SHL
	SHR
	SAR
	SHA3
	ADDRESS
	BALANCE
	ORIGIN
	CALLER
	CALLVALUE
	CALLDATALOAD
	CALLDATASIZE
	CALLDATACOPY
	CODESIZE
	CODECOPY
	GASPRICE
	EXTCODESIZE
	EXTCODECOPY
	EXTCODEHASH
	BLOCKHASH
	COINBASE
	TIMESTAMP
	NUMBER
	DIFFICULTY
	GASLIMIT
	POP
	MLOAD
	MSTORE
	MSTORE8
	SLOAD
	SSTORE
	JUMP
	JUMPI
	PC
	MSIZE
	GAS
	JUMPDEST
	PUSH1
	PUSH2
	PUSH3
	PUSH4
	PUSH5
	PUSH6
	PUSH7
	PUSH8
	PUSH9
	PUSH10
	PUSH11
	PUSH12
	PUSH13
	PUSH14
	PUSH15
	PUSH16
	PUSH17
	PUSH18
	PUSH19
	PUSH20
	PUSH21
	PUSH22
	PUSH23
	PUSH24
	PUSH25
	PUSH26
	PUSH27
	PUSH28
	PUSH29
	PUSH30
	PUSH31
	PUSH32
	DUP1
	DUP2
	DUP3
	DUP4
	DUP5
	DUP6
	DUP7
	DUP8
	DUP9
	DUP10
	DUP11
	DUP12
	DUP13
	DUP14
	DUP15
	DUP16
	SWAP1
	SWAP2
	SWAP3
	SWAP4
	SWAP5
	SWAP6
	SWAP7
	SWAP8
	SWAP9
	SWAP10
	SWAP11
	SWAP12
	SWAP13
	SWAP14
	SWAP15
	SWAP16
	LOG0
	LOG1
	LOG2
	LOG3
	LOG4
	CREATE
	CALL
	CALLCODE
	RETURN
	DELEGATECALL
	CREATE2
	STATICCALL
	REVERT
	INVALID
	SELFDESTRUCT
)

// GasCost returns the gas cost for an opcode
func (op OpCode) GasCost(isEIP150 bool) uint64 {
	switch op {
	case STOP:
		return 0
	case ADD, SUB, LT, GT, SLT, SGT, EQ, ISZERO, AND, OR, XOR, NOT, BYTE:
		return 3
	case MUL, DIV, SDIV, MOD, SMOD, ADDMOD, MULMOD, SIGNEXTEND:
		return 5
	case SHL, SHR, SAR:
		return 3
	case SHA3:
		return 30
	case ADDRESS, ORIGIN, CALLER, CALLVALUE, CALLDATASIZE, GASPRICE, COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT:
		return 2
	case BALANCE:
		return 20
	case SLOAD:
		return 50
	case JUMPDEST:
		return 1
	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return 3
	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return 3
	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return 3
	case POP:
		return 2
	case MLOAD, MSTORE, MSTORE8:
		return 3
	case JUMP, JUMPI:
		return 8
	case PC, MSIZE, GAS:
		return 2
	case EXTCODESIZE:
		return 20
	case EXTCODECOPY, EXTCODEHASH:
		return 20
	case CODESIZE, CODECOPY:
		return 2
	case CALLDATACOPY:
		return 3
	case LOG0, LOG1, LOG2, LOG3, LOG4:
		return 375
	case CREATE:
		return 32000
	case CREATE2:
		return 32000
	case CALL, CALLCODE, STATICCALL, DELEGATECALL:
		return 0
	case RETURN, REVERT:
		return 0
	case SELFDESTRUCT:
		return 0
	case INVALID:
		return 0
	default:
		return 0
	}
}

// MemoryExpansionCost calculates the cost of expanding memory
func MemoryExpansionCost(oldSize, newSize uint64) (uint64, error) {
	if newSize < oldSize {
		return 0, nil
	}
	if newSize > 0x1fffffffffffff {
		return 0, errors.New("memory overflow")
	}
	oldWords := (oldSize + 31) / 32
	newWords := (newSize + 31) / 32

	if newWords > oldWords {
		cost := (newWords - oldWords) * MemoryGas
		cost += ((newWords * newWords) - (oldWords * oldWords)) / 512
		return cost, nil
	}
	return 0, nil
}

// CalcGasFee calculates total gas fee
func CalcGasFee(gas uint64, gasPrice *big.Int) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
}

// GetGasForCall calculates gas for a call operation
func GetGasForCall(op OpCode, callerParams *CallParams) uint64 {
	switch op {
	case CALL, CALLCODE, DELEGATECALL, STATICCALL:
		return callerParams.Gas
	default:
		return 0
	}
}

// CallParams holds parameters for call operations
type CallParams struct {
	Gas     uint64
	Address common.Address
	In      []byte
	Caller  common.Address
	Value   *big.Int
}
