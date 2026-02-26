// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - UTXO Store Module
package utxo

import (
	"fmt"
	"sync"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// UTXOStore implements the UTXOProvider interface.
type UTXOStore struct {
	utxos   map[string]*UTXO    // Key: txHash:index
	balances map[[32]byte]uint64 // Address -> balance
	mu       sync.RWMutex
}

// NewUTXOStore creates a new UTXO store.
func NewUTXOStore() *UTXOStore {
	return &UTXOStore{
		utxos:   make(map[string]*UTXO),
		balances: make(map[[32]byte]uint64),
	}
}

// UTXOKey generates a unique key for a UTXO.
func UTXOKey(txHash [32]byte, index uint32) string {
	return fmt.Sprintf("%x:%d", txHash, index)
}

// AddUTXO adds a new UTXO to the store.
func (s *UTXOStore) AddUTXO(utxo *UTXO) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := UTXOKey(utxo.TxHash, utxo.Index)
	s.utxos[key] = utxo
	s.balances[utxo.Address] += utxo.Value
}

// GetUTXO retrieves a specific UTXO by transaction hash and output index.
func (s *UTXOStore) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := UTXOKey(txHash, index)
	utxo, exists := s.utxos[key]
	if !exists {
		return nil, fmt.Errorf("UTXO not found: %s", key)
	}

	return utxo, nil
}

// HasUTXO checks if a UTXO exists.
func (s *UTXOStore) HasUTXO(txHash [32]byte, index uint32) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := UTXOKey(txHash, index)
	_, exists := s.utxos[key]
	return exists
}

// SpendUTXO marks a UTXO as spent and removes it from the store.
func (s *UTXOStore) SpendUTXO(txHash [32]byte, index uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := UTXOKey(txHash, index)
	utxo, exists := s.utxos[key]
	if !exists {
		return fmt.Errorf("UTXO not found or already spent")
	}

	// Deduct from balance
	s.balances[utxo.Address] -= utxo.Value
	if s.balances[utxo.Address] == 0 {
		delete(s.balances, utxo.Address)
	}

	// Remove UTXO
	delete(s.utxos, key)

	return nil
}

// GetBalance returns the total spendable balance for an address.
func (s *UTXOStore) GetBalance(addr [32]byte) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.balances[addr]
}

// GetAllUTXOs returns all UTXOs for an address.
func (s *UTXOStore) GetAllUTXOs(addr [32]byte) []*UTXO {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*UTXO
	for _, utxo := range s.utxos {
		if utxo.Address == addr {
			result = append(result, utxo)
		}
	}

	return result
}

// GetUTXOCount returns the total number of UTXOs in the store.
func (s *UTXOStore) GetUTXOCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.utxos)
}

// UTXOProviderAdapter implements interfaces.UTXOProvider using UTXOStore.
type UTXOProviderAdapter struct {
	store *UTXOStore
}

// NewUTXOProviderAdapter creates a new UTXO provider adapter.
func NewUTXOProviderAdapter(store *UTXOStore) *UTXOProviderAdapter {
	return &UTXOProviderAdapter{store: store}
}

// GetUTXO implements interfaces.UTXOProvider.
func (p *UTXOProviderAdapter) GetUTXO(txHash [32]byte, index uint32) (*interfaces.UTXO, error) {
	utxo, err := p.store.GetUTXO(txHash, index)
	if err != nil {
		return nil, err
	}
	return utxo.ToInterfacesUTXO(), nil
}

// CreateTXOutput implements interfaces.UTXOProvider.
func (p *UTXOProviderAdapter) CreateTXOutput(addr interfaces.Address, value uint64, script []byte) *interfaces.TXOutput {
	return &interfaces.TXOutput{
		Value:   value,
		Script:  script,
		Address: addr,
	}
}

// SpendUTXO implements interfaces.UTXOProvider.
func (p *UTXOProviderAdapter) SpendUTXO(input interfaces.TXInput, sig []byte) error {
	return p.store.SpendUTXO(input.TxHash, input.Index)
}

// GetBalance implements interfaces.UTXOProvider.
func (p *UTXOProviderAdapter) GetBalance(addr interfaces.Address) (uint64, error) {
	return p.store.GetBalance(addr), nil
}

// CreateTransaction creates a new transaction with inputs and outputs.
func (s *UTXOStore) CreateTransaction(inputs []TXInput, outputs []TXOutput) *Transaction {
	return NewTransaction(inputs, outputs)
}

// ValidateTransaction validates a transaction against the UTXO set.
func (s *UTXOStore) ValidateTransaction(tx *Transaction) error {
	// Check for double spend
	seen := make(map[string]bool)
	for _, in := range tx.Inputs {
		key := UTXOKey(in.TxHash, in.Index)
		if seen[key] {
			return fmt.Errorf("double spend detected: %s", key)
		}
		seen[key] = true

		// Check UTXO exists
		if !s.HasUTXO(in.TxHash, in.Index) {
			return fmt.Errorf("UTXO not found: %s", key)
		}
	}

	// Verify inputs
	for i := range tx.Inputs {
		if !tx.VerifyInput(i) {
			return fmt.Errorf("invalid signature for input %d", i)
		}
	}

	return nil
}

// ApplyTransaction applies a transaction to the UTXO set.
// This creates new UTXOs for the outputs and removes spent inputs.
func (s *UTXOStore) ApplyTransaction(tx *Transaction) error {
	// First validate
	if err := s.ValidateTransaction(tx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark inputs as spent
	for _, in := range tx.Inputs {
		key := UTXOKey(in.TxHash, in.Index)
		utxo := s.utxos[key]

		// Deduct from balance
		s.balances[utxo.Address] -= utxo.Value
		if s.balances[utxo.Address] == 0 {
			delete(s.balances, utxo.Address)
		}

		delete(s.utxos, key)
	}

	// Add new outputs as UTXOs
	txHash := tx.Hash()
	for i, out := range tx.Outputs {
		utxo := &UTXO{
			TxHash:  txHash,
			Index:   uint32(i),
			Value:   out.Value,
			Script:  out.Script,
			Address: out.Address,
		}

		key := UTXOKey(txHash, uint32(i))
		s.utxos[key] = utxo
		s.balances[out.Address] += out.Value
	}

	return nil
}

// GetUTXOsForAmount selects UTXOs that can cover the requested amount.
func (s *UTXOStore) GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*UTXO, uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var selected []*UTXO
	var total uint64

	for _, utxo := range s.utxos {
		if utxo.Address == addr {
			selected = append(selected, utxo)
			total += utxo.Value

			if total >= amount {
				break
			}
		}
	}

	if total < amount {
		return nil, 0, fmt.Errorf("insufficient balance: have %d, need %d", total, amount)
	}

	return selected, total, nil
}

// CreateCoinbaseTransaction creates a coinbase transaction (block reward + fees).
func CreateCoinbaseTransaction(toAddr [32]byte, reward uint64, data []byte) *Transaction {
	// Create input with special coinbase marker
	input := TXInput{
		TxHash: [32]byte{}, // All zeros for coinbase
		Index:  0xffffffff, // Max uint32 for coinbase
	}

	// Create output to the reward recipient (reward includes block subsidy + tx fees)
	output := TXOutput{
		Value:   reward,
		Script:  data,
		Address: toAddr,
	}

	return NewTransaction([]TXInput{input}, []TXOutput{output})
}

// CreateCoinbaseWithFees creates a coinbase transaction where reward = subsidy + fees.
func CreateCoinbaseWithFees(toAddr [32]byte, blockSubsidy uint64, txFees uint64, data []byte) *Transaction {
	return CreateCoinbaseTransaction(toAddr, blockSubsidy+txFees, data)
}

// GetStats returns UTXO store statistics.
func (s *UTXOStore) GetStats() (utxoCount int, addrCount int, totalValue uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	utxoCount = len(s.utxos)
	addrCount = len(s.balances)

	for _, utxo := range s.utxos {
		totalValue += utxo.Value
	}

	return
}
