package evm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/holiman/uint256"
)

// AIBStateDB implements vm.StateDB interface for AIB 2.0
type AIBStateDB struct {
	accounts         map[common.Address]*AccountState
	storage          map[common.Address]map[common.Hash]common.Hash
	transientStorage map[common.Address]map[common.Hash]common.Hash
	journal          *Journal
	snapshots        []int
	refund           uint64
	logs             []*types.Log
	preimages        map[common.Hash][]byte
	mu               sync.RWMutex
	blockNumber      *big.Int
	blockHash        common.Hash
	txHash           common.Hash
	accessList       types.AccessList
}

// AccountState represents an EVM account state
type AccountState struct {
	Nonce    uint64
	Balance  *big.Int
	Code     []byte
	CodeHash common.Hash
}

// NewAIBStateDB creates a new AIB StateDB
func NewAIBStateDB() *AIBStateDB {
	return &AIBStateDB{
		accounts:         make(map[common.Address]*AccountState),
		storage:          make(map[common.Address]map[common.Hash]common.Hash),
		transientStorage: make(map[common.Address]map[common.Hash]common.Hash),
		journal:          NewJournal(),
		preimages:        make(map[common.Hash][]byte),
		accessList:       make(types.AccessList, 0),
		blockNumber:      big.NewInt(0),
	}
}

// SetBlockContext sets the block context for EVM execution
func (s *AIBStateDB) SetBlockContext(blockNumber *big.Int, blockHash common.Hash, txHash common.Hash) {
	s.blockNumber = blockNumber
	s.blockHash = blockHash
	s.txHash = txHash
}

// CreateAccount implements vm.StateDB
func (s *AIBStateDB) CreateAccount(addr common.Address) {
	if s.accounts[addr] == nil {
		s.accounts[addr] = &AccountState{
			Balance:  big.NewInt(0),
			CodeHash: crypto.Keccak256Hash(nil),
		}
	}
}

// CreateContract implements vm.StateDB
func (s *AIBStateDB) CreateContract(addr common.Address) {
	s.CreateAccount(addr)
}

// Exist implements vm.StateDB
func (s *AIBStateDB) Exist(addr common.Address) bool {
	return s.accounts[addr] != nil
}

// Empty implements vm.StateDB
func (s *AIBStateDB) Empty(addr common.Address) bool {
	acc := s.accounts[addr]
	if acc == nil {
		return true
	}
	return acc.Nonce == 0 && acc.Balance.Sign() == 0 && len(acc.Code) == 0
}

// GetBalance implements vm.StateDB
func (s *AIBStateDB) GetBalance(addr common.Address) *uint256.Int {
	acc := s.accounts[addr]
	if acc == nil {
		return uint256.NewInt(0)
	}
	return uint256.NewInt(0).SetBytes(acc.Balance.Bytes())
}

// GetNonce implements vm.StateDB
func (s *AIBStateDB) GetNonce(addr common.Address) uint64 {
	acc := s.accounts[addr]
	if acc == nil {
		return 0
	}
	return acc.Nonce
}

// SetNonce implements vm.StateDB
func (s *AIBStateDB) SetNonce(addr common.Address, nonce uint64) {
	acc := s.getOrNewAccount(addr)
	acc.Nonce = nonce
}

// GetCodeHash implements vm.StateDB
func (s *AIBStateDB) GetCodeHash(addr common.Address) common.Hash {
	acc := s.accounts[addr]
	if acc == nil {
		return common.Hash{}
	}
	return acc.CodeHash
}

// GetCode implements vm.StateDB
func (s *AIBStateDB) GetCode(addr common.Address) []byte {
	acc := s.accounts[addr]
	if acc == nil {
		return nil
	}
	return acc.Code
}

// SetCode implements vm.StateDB
func (s *AIBStateDB) SetCode(addr common.Address, code []byte) {
	acc := s.getOrNewAccount(addr)
	acc.Code = code
	acc.CodeHash = crypto.Keccak256Hash(code)
}

// GetCodeSize implements vm.StateDB
func (s *AIBStateDB) GetCodeSize(addr common.Address) int {
	return len(s.GetCode(addr))
}

// AddRefund implements vm.StateDB
func (s *AIBStateDB) AddRefund(gas uint64) {
	s.refund += gas
}

// SubRefund implements vm.StateDB
func (s *AIBStateDB) SubRefund(gas uint64) {
	if gas > s.refund {
		s.refund = 0
	} else {
		s.refund -= gas
	}
}

// GetRefund implements vm.StateDB
func (s *AIBStateDB) GetRefund() uint64 {
	return s.refund
}

// GetState implements vm.StateDB
func (s *AIBStateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	if s.storage[addr] == nil {
		return common.Hash{}
	}
	return s.storage[addr][key]
}

// SetState implements vm.StateDB
func (s *AIBStateDB) SetState(addr common.Address, key common.Hash, value common.Hash) common.Hash {
	prev := s.GetState(addr, key)
	if s.storage[addr] == nil {
		s.storage[addr] = make(map[common.Hash]common.Hash)
	}
	s.storage[addr][key] = value
	return prev
}

// GetCommittedState implements vm.StateDB
func (s *AIBStateDB) GetCommittedState(addr common.Address, key common.Hash) common.Hash {
	return s.GetState(addr, key)
}

// GetStorageRoot implements vm.StateDB
func (s *AIBStateDB) GetStorageRoot(addr common.Address) common.Hash {
	return common.Hash{}
}

// GetTransientState implements vm.StateDB
func (s *AIBStateDB) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	if s.transientStorage[addr] == nil {
		return common.Hash{}
	}
	return s.transientStorage[addr][key]
}

// SetTransientState implements vm.StateDB
func (s *AIBStateDB) SetTransientState(addr common.Address, key, value common.Hash) {
	if s.transientStorage[addr] == nil {
		s.transientStorage[addr] = make(map[common.Hash]common.Hash)
	}
	s.transientStorage[addr][key] = value
}

// SubBalance implements vm.StateDB
func (s *AIBStateDB) SubBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	if amount.IsZero() {
		return *s.GetBalance(addr)
	}
	acc := s.getOrNewAccount(addr)
	prev := *uint256.NewInt(0).SetBytes(acc.Balance.Bytes())
	acc.Balance.Sub(acc.Balance, amount.ToBig())
	return prev
}

// AddBalance implements vm.StateDB
func (s *AIBStateDB) AddBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	if amount.IsZero() {
		return *s.GetBalance(addr)
	}
	acc := s.getOrNewAccount(addr)
	acc.Balance.Add(acc.Balance, amount.ToBig())
	return *s.GetBalance(addr)
}

// SelfDestruct implements vm.StateDB
func (s *AIBStateDB) SelfDestruct(addr common.Address) uint256.Int {
	var balance uint256.Int
	if acc := s.accounts[addr]; acc != nil {
		balance.SetBytes(acc.Balance.Bytes())
	}
	delete(s.accounts, addr)
	delete(s.storage, addr)
	return balance
}

// HasSelfDestructed implements vm.StateDB
func (s *AIBStateDB) HasSelfDestructed(addr common.Address) bool {
	return s.accounts[addr] == nil
}

// SelfDestruct6780 implements vm.StateDB
func (s *AIBStateDB) SelfDestruct6780(addr common.Address) (uint256.Int, bool) {
	return s.SelfDestruct(addr), true
}

// AddLog implements vm.StateDB
func (s *AIBStateDB) AddLog(log *types.Log) {
	log.TxHash = s.txHash
	log.BlockNumber = s.blockNumber.Uint64()
	log.BlockHash = s.blockHash
	s.logs = append(s.logs, log)
}

// AddPreimage implements vm.StateDB
func (s *AIBStateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := s.preimages[hash]; !ok {
		s.preimages[hash] = preimage
	}
}

// ForEachStorage implements vm.StateDB
func (s *AIBStateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	if s.storage[addr] == nil {
		return
	}
	for key, value := range s.storage[addr] {
		if !cb(key, value) {
			break
		}
	}
}

// Snapshot implements vm.StateDB
func (s *AIBStateDB) Snapshot() int {
	s.snapshots = append(s.snapshots, s.journal.Len())
	return len(s.snapshots) - 1
}

// RevertToSnapshot implements vm.StateDB
func (s *AIBStateDB) RevertToSnapshot(revid int) {
	if revid >= len(s.snapshots) || revid < 0 {
		return
	}
	s.journal.RevertToState(s, s.snapshots[revid])
	s.snapshots = s.snapshots[:revid]
}

// IntermediateRoot implements vm.StateDB
func (s *AIBStateDB) IntermediateRoot(deleteEmptyAccounts bool) common.Hash {
	return s.computeStateRoot()
}

// Finalise implements vm.StateDB
func (s *AIBStateDB) Finalise(deleteEmptyAccounts bool) {
	// Clear transient storage
	s.transientStorage = make(map[common.Address]map[common.Hash]common.Hash)
}

// AddressInAccessList implements vm.StateDB
func (s *AIBStateDB) AddressInAccessList(addr common.Address) bool {
	for _, tuple := range s.accessList {
		if tuple.Address == addr {
			return true
		}
	}
	return false
}

// SlotInAccessList implements vm.StateDB
func (s *AIBStateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressOk bool, slotOk bool) {
	for _, tuple := range s.accessList {
		if tuple.Address == addr {
			addressOk = true
			for _, s := range tuple.StorageKeys {
				if s == slot {
					return true, true
				}
			}
		}
	}
	return false, false
}

// AddAddressToAccessList implements vm.StateDB
func (s *AIBStateDB) AddAddressToAccessList(addr common.Address) {
	if s.AddressInAccessList(addr) {
		return
	}
	s.accessList = append(s.accessList, types.AccessTuple{
		Address:     addr,
		StorageKeys: []common.Hash{},
	})
}

// AddSlotToAccessList implements vm.StateDB
func (s *AIBStateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	for i := range s.accessList {
		if s.accessList[i].Address == addr {
			for _, s := range s.accessList[i].StorageKeys {
				if s == slot {
					return
				}
			}
			s.accessList[i].StorageKeys = append(s.accessList[i].StorageKeys, slot)
			return
		}
	}
	s.accessList = append(s.accessList, types.AccessTuple{
		Address:     addr,
		StorageKeys: []common.Hash{slot},
	})
}

// PointCache implements vm.StateDB - stub
func (s *AIBStateDB) PointCache() *utils.PointCache {
	return utils.NewPointCache(25)
}

// Prepare implements vm.StateDB
func (s *AIBStateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address, precompiles []common.Address, txAccesses types.AccessList) {
	s.accessList = make(types.AccessList, len(txAccesses))
	copy(s.accessList, txAccesses)
}

// Witness implements vm.StateDB - stub
func (s *AIBStateDB) Witness() *stateless.Witness {
	w, _ := stateless.NewWitness(nil, nil)
	return w
}

// getOrNewAccount returns existing account or creates a new one
func (s *AIBStateDB) getOrNewAccount(addr common.Address) *AccountState {
	if s.accounts[addr] == nil {
		s.accounts[addr] = &AccountState{
			Balance:  big.NewInt(0),
			CodeHash: crypto.Keccak256Hash(nil),
		}
	}
	return s.accounts[addr]
}

// computeStateRoot computes the state root hash (simplified)
func (s *AIBStateDB) computeStateRoot() common.Hash {
	var buf bytes.Buffer
	addresses := make([]common.Address, 0, len(s.accounts))
	for addr := range s.accounts {
		addresses = append(addresses, addr)
	}
	sortAddresses(addresses)
	for _, addr := range addresses {
		acc := s.accounts[addr]
		buf.Write(addr.Bytes())
		binary.Write(&buf, binary.BigEndian, acc.Nonce)
		buf.Write(acc.CodeHash.Bytes())
	}
	return crypto.Keccak256Hash(buf.Bytes())
}

// Commit commits state changes
func (s *AIBStateDB) Commit() (common.Hash, error) {
	return s.computeStateRoot(), nil
}

// sortAddresses sorts addresses in ascending order
func sortAddresses(addrs []common.Address) {
	for i := 0; i < len(addrs); i++ {
		for j := i + 1; j < len(addrs); j++ {
			if bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) > 0 {
				addrs[i], addrs[j] = addrs[j], addrs[i]
			}
		}
	}
}

// ConvertAIBToEVMAddress converts an AIB address to EVM address
func ConvertAIBToEVMAddress(addr [20]byte) common.Address {
	return common.BytesToAddress(addr[:])
}

// ConvertEVMToAIBAddress converts an EVM address to AIB address
func ConvertEVMToAIBAddress(addr common.Address) [20]byte {
	var result [20]byte
	copy(result[:], addr.Bytes())
	return result
}

// ConvertHashToEVM converts an AIB hash to EVM hash
func ConvertHashToEVM(hash [32]byte) common.Hash {
	return common.BytesToHash(hash[:])
}

// ConvertEVMToHash converts an EVM hash to AIB hash
func ConvertEVMToHash(hash common.Hash) [32]byte {
	var result [32]byte
	copy(result[:], hash.Bytes())
	return result
}

// String implements fmt.Stringer
func (s *AIBStateDB) String() string {
	var b strings.Builder
	b.WriteString("AIBStateDB {\n")
	b.WriteString(fmt.Sprintf("  Accounts: %d\n", len(s.accounts)))
	b.WriteString(fmt.Sprintf("  Storage: %d\n", len(s.storage)))
	b.WriteString(fmt.Sprintf("  Refund: %d\n", s.refund))
	b.WriteString(fmt.Sprintf("  Logs: %d\n", len(s.logs)))
	b.WriteString("}")
	return b.String()
}

// GetLogs returns execution logs
func (s *AIBStateDB) GetLogs() []*types.Log {
	return s.logs
}

// Ensure AIBStateDB implements vm.StateDB
var _ vm.StateDB = (*AIBStateDB)(nil)
