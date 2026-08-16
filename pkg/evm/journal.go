package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// JournalEntry is an interface for journal entries
type JournalEntry interface {
	revert(s *AIBStateDB)
}

// Journal tracks state changes for snapshot/revert functionality
type Journal struct {
	entries []JournalEntry
	length  int
}

// NewJournal creates a new journal
func NewJournal() *Journal {
	return &Journal{
		entries: make([]JournalEntry, 0),
		length:  0,
	}
}

// append adds a new entry to the journal
func (j *Journal) append(entry JournalEntry) {
	j.entries = append(j.entries, entry)
	j.length++
}

// Len returns the current journal size
func (j *Journal) Len() int {
	return j.length
}

// RevertToState reverts state changes to a specific size
func (j *Journal) RevertToState(s *AIBStateDB, targetSize int) {
	for i := len(j.entries) - 1; i >= targetSize; i-- {
		if j.entries[i] != nil {
			j.entries[i].revert(s)
		}
	}
	j.entries = j.entries[:targetSize]
	j.length = targetSize
}

// Journal entry types

type (
	createAccountChange struct {
		prev *AccountState
	}
	balanceChange struct {
		prev *big.Int
		addr common.Address
	}
	nonceChange struct {
		prev uint64
		addr common.Address
	}
	codeChange struct {
		prev []byte
		addr common.Address
	}
	refundChange struct {
		prev uint64
	}
	storageChange struct {
		addr  common.Address
		key   common.Hash
		prev  common.Hash
	}
	selfDestructChange struct {
		addr common.Address
		prev *AccountState
	}
	logChange struct {
		// tracks log index
	}
)

// revert implementations

func (c createAccountChange) revert(s *AIBStateDB) {
	if c.prev == nil {
		delete(s.accounts, common.Address{})
	} else {
		s.accounts[common.Address{}] = c.prev
	}
}

func (c balanceChange) revert(s *AIBStateDB) {
	acc := s.accounts[c.addr]
	if acc != nil {
		acc.Balance = c.prev
	}
}

func (c nonceChange) revert(s *AIBStateDB) {
	acc := s.accounts[c.addr]
	if acc != nil {
		acc.Nonce = c.prev
	}
}

func (c codeChange) revert(s *AIBStateDB) {
	acc := s.accounts[c.addr]
	if acc != nil {
		acc.Code = c.prev
		acc.CodeHash = common.BytesToHash(acc.Code)
	}
}

func (c refundChange) revert(s *AIBStateDB) {
	s.refund = c.prev
}

func (c storageChange) revert(s *AIBStateDB) {
	if s.storage[c.addr] == nil {
		return
	}
	if c.prev == (common.Hash{}) {
		delete(s.storage[c.addr], c.key)
	} else {
		s.storage[c.addr][c.key] = c.prev
	}
}

func (c selfDestructChange) revert(s *AIBStateDB) {
	if c.prev == nil {
		delete(s.accounts, c.addr)
		delete(s.storage, c.addr)
	} else {
		s.accounts[c.addr] = c.prev
	}
}

func (c logChange) revert(s *AIBStateDB) {
	if len(s.logs) > 0 {
		s.logs = s.logs[:len(s.logs)-1]
	}
}

// Undo removes the last journal entry
func (j *Journal) Undo(s *AIBStateDB) {
	if len(j.entries) == 0 {
		return
	}
	last := j.entries[len(j.entries)-1]
	last.revert(s)
	j.entries = j.entries[:len(j.entries)-1]
	j.length--
}
