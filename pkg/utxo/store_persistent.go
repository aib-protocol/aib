// Package utxo provides persistent storage for UTXO blockchain.
// Uses bbolt embedded database for ACID-compliant storage.
package utxo

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.etcd.io/bbolt"
)

// ============================================================================
// Persistent UTXO Store
// ============================================================================

// PersistentUTXOStore implements persistent storage using bbolt.
type PersistentUTXOStore struct {
	db       *bbolt.DB
	mu       sync.RWMutex
	readOnly bool
}

// Bucket names
var (
	BucketUTXO     = []byte("utxo")
	BucketBalances = []byte("balances")
	BucketHeads    = []byte("heads")   // Chain heads (for pruning)
	BucketMeta     = []byte("meta")    // Metadata
	BucketTXIndex  = []byte("txindex") // Transaction index
)

// NewPersistentUTXOStore creates a new persistent UTXO store.
func NewPersistentUTXOStore(dbPath string) (*PersistentUTXOStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := ensureDir(dir); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}
	}

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout:      0,
		NoSync:       true,
		NoGrowSync:   true,
		FreelistType: bbolt.FreelistMapType,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &PersistentUTXOStore{
		db: db,
	}

	// Initialize buckets
	if err := store.initBuckets(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	return store, nil
}

// NewPersistentUTXOStoreWithDB creates a store with existing bbolt.DB.
func NewPersistentUTXOStoreWithDB(db *bbolt.DB) (*PersistentUTXOStore, error) {
	store := &PersistentUTXOStore{
		db: db,
	}

	if err := store.initBuckets(); err != nil {
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	return store, nil
}

// ensureDir creates directory if it doesn't exist.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// initBuckets creates all necessary buckets.
func (s *PersistentUTXOStore) initBuckets() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		buckets := [][]byte{BucketUTXO, BucketBalances, BucketHeads, BucketMeta, BucketTXIndex}
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
}

// Close closes the database.
func (s *PersistentUTXOStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// IsReadOnly returns if store is in read-only mode.
func (s *PersistentUTXOStore) IsReadOnly() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readOnly
}

// SetReadOnly sets read-only mode.
func (s *PersistentUTXOStore) SetReadOnly(readOnly bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readOnly = readOnly
}

// ============================================================================
// UTXO Operations
// ============================================================================

// AddUTXO adds a new UTXO to the store.
func (s *PersistentUTXOStore) AddUTXO(utxo *UTXO) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		key := UTXOKey(utxo.TxHash, utxo.Index)

		// Serialize UTXO
		data, err := serializeUTXO(utxo)
		if err != nil {
			return err
		}

		if err := b.Put([]byte(key), data); err != nil {
			return err
		}

		// Update balance
		return s.updateBalance(tx, utxo.Address, int64(utxo.Value))
	})
}

// GetUTXO retrieves a specific UTXO.
func (s *PersistentUTXOStore) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result *UTXO
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		key := UTXOKey(txHash, index)
		data := b.Get([]byte(key))

		if data == nil {
			return fmt.Errorf("UTXO not found: %s", key)
		}

		var err error
		result, err = deserializeUTXO(data)
		return err
	})

	return result, err
}

// HasUTXO checks if a UTXO exists.
func (s *PersistentUTXOStore) HasUTXO(txHash [32]byte, index uint32) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var exists bool
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		key := UTXOKey(txHash, index)
		exists = b.Get([]byte(key)) != nil
		return nil
	})

	return exists
}

// SpendUTXO marks a UTXO as spent.
func (s *PersistentUTXOStore) SpendUTXO(txHash [32]byte, index uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		key := UTXOKey(txHash, index)
		data := b.Get([]byte(key))

		if data == nil {
			return fmt.Errorf("UTXO not found or already spent")
		}

		utxo, err := deserializeUTXO(data)
		if err != nil {
			return err
		}

		// Delete UTXO
		if err := b.Delete([]byte(key)); err != nil {
			return err
		}

		// Update balance (subtract)
		return s.updateBalance(tx, utxo.Address, -int64(utxo.Value))
	})
}

// GetBalance returns the total balance for an address.
func (s *PersistentUTXOStore) GetBalance(addr [32]byte) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var balance uint64
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketBalances)
		data := b.Get(addr[:])
		if data != nil {
			balance = binary.BigEndian.Uint64(data)
		}
		return nil
	})

	return balance
}

// GetAllUTXOs returns all UTXOs for an address.
func (s *PersistentUTXOStore) GetAllUTXOs(addr [32]byte) []*UTXO {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*UTXO
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		c := b.Cursor()

		prefix := addr[:]
		for k, v := c.Seek(prefix); k != nil && len(k) >= 32 && string(k[:32]) == string(prefix); k, v = c.Next() {
			utxo, err := deserializeUTXO(v)
			if err == nil {
				result = append(result, utxo)
			}
		}
		return nil
	})

	return result
}

// GetUTXOsForAmount selects UTXOs that can cover the requested amount.
func (s *PersistentUTXOStore) GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*UTXO, uint64, error) {
	all := s.GetAllUTXOs(addr)
	var selected []*UTXO
	var total uint64
	for _, u := range all {
		selected = append(selected, u)
		total += u.Value
		if total >= amount {
			break
		}
	}
	if total < amount {
		return nil, 0, fmt.Errorf("insufficient balance: have %d, need %d", total, amount)
	}
	return selected, total, nil
}

// GetUTXOCount returns total UTXO count.
func (s *PersistentUTXOStore) GetUTXOCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		count = b.Stats().KeyN
		return nil
	})

	return count
}

// updateBalance updates balance for an address.
func (s *PersistentUTXOStore) updateBalance(tx *bbolt.Tx, addr [32]byte, delta int64) error {
	b := tx.Bucket(BucketBalances)
	data := b.Get(addr[:])

	var current int64
	if data != nil {
		current = int64(binary.BigEndian.Uint64(data))
	}

	newBalance := current + delta
	if newBalance < 0 {
		return fmt.Errorf("insufficient balance")
	}

	if newBalance == 0 {
		return b.Delete(addr[:])
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(newBalance))
	return b.Put(addr[:], buf[:])
}

// ============================================================================
// Batch Operations
// ============================================================================

// Batch runs a batch of operations atomically.
func (s *PersistentUTXOStore) Batch(fn func(*PersistentUTXOStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	return s.db.Batch(func(tx *bbolt.Tx) error {
		// Create a batch-specific store view
		batchStore := &PersistentUTXOStore{
			db:       s.db,
			readOnly: false,
		}
		return fn(batchStore)
	})
}

// Update runs a write transaction.
func (s *PersistentUTXOStore) Update(fn func(*PersistentUTXOStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		updateStore := &PersistentUTXOStore{
			db:       s.db,
			readOnly: false,
		}
		return fn(updateStore)
	})
}

// View runs a read-only transaction.
func (s *PersistentUTXOStore) View(fn func(*PersistentUTXOStore) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.View(func(tx *bbolt.Tx) error {
		viewStore := &PersistentUTXOStore{
			db:       s.db,
			readOnly: true,
		}
		return fn(viewStore)
	})
}

// ============================================================================
// Transaction Operations
// ============================================================================

// ApplyTransaction applies a transaction to the UTXO set.
func (s *PersistentUTXOStore) ApplyTransaction(tx *Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	return s.db.Update(func(txDB *bbolt.Tx) error {
		// First validate
		utxoBkt := txDB.Bucket(BucketUTXO)
		for _, in := range tx.Inputs {
			key := UTXOKey(in.TxHash, in.Index)
			if utxoBkt.Get([]byte(key)) == nil {
				return fmt.Errorf("UTXO not found: %s", key)
			}
		}

		// Verify signatures
		for i := range tx.Inputs {
			if !tx.VerifyInput(i) {
				return fmt.Errorf("invalid signature for input %d", i)
			}
		}

		// Spend inputs
		for _, in := range tx.Inputs {
			key := UTXOKey(in.TxHash, in.Index)
			data := utxoBkt.Get([]byte(key))

			utxo, err := deserializeUTXO(data)
			if err != nil {
				return err
			}

			// Update balance (subtract)
			if err := s.updateBalance(txDB, utxo.Address, -int64(utxo.Value)); err != nil {
				return err
			}

			if err := utxoBkt.Delete([]byte(key)); err != nil {
				return err
			}
		}

		// Add outputs as new UTXOs
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
			data, err := serializeUTXO(utxo)
			if err != nil {
				return err
			}

			if err := utxoBkt.Put([]byte(key), data); err != nil {
				return err
			}

			// Update balance (add)
			if err := s.updateBalance(txDB, out.Address, int64(out.Value)); err != nil {
				return err
			}
		}

		// Index transaction
		if err := s.indexTransaction(txDB, tx); err != nil {
			return err
		}

		return nil
	})
}

// ValidateTransaction validates a transaction.
func (s *PersistentUTXOStore) ValidateTransaction(tx *Transaction) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.View(func(txDB *bbolt.Tx) error {
		b := txDB.Bucket(BucketUTXO)

		// Check UTXO existence
		for _, in := range tx.Inputs {
			key := UTXOKey(in.TxHash, in.Index)
			if b.Get([]byte(key)) == nil {
				return fmt.Errorf("UTXO not found: %s", key)
			}
		}

		// Verify signatures
		for i := range tx.Inputs {
			if !tx.VerifyInput(i) {
				return fmt.Errorf("invalid signature for input %d", i)
			}
		}

		return nil
	})
}

// ============================================================================
// Snapshot & Restore
// ============================================================================

// Snapshot creates a backup of the current state.
func (s *PersistentUTXOStore) Snapshot(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.View(func(tx *bbolt.Tx) error {
		return tx.CopyFile(path, 0600)
	})
}

// Restore restores state from a backup file.
func (s *PersistentUTXOStore) Restore(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	// Close current DB
	if err := s.db.Close(); err != nil {
		return err
	}

	// Copy backup to current path (assumes same path)
	dbPath := s.db.Path()
	if err := copyFile(path, dbPath); err != nil {
		return err
	}

	// Reopen
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return err
	}
	s.db = db

	return s.initBuckets()
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// ============================================================================
// Statistics
// ============================================================================

// GetStats returns store statistics.
func (s *PersistentUTXOStore) GetStats() (utxoCount, addrCount int, totalValue uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_ = s.db.View(func(tx *bbolt.Tx) error {
		utxoB := tx.Bucket(BucketUTXO)
		balB := tx.Bucket(BucketBalances)

		utxoCount = utxoB.Stats().KeyN
		addrCount = balB.Stats().KeyN

		c := utxoB.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			utxo, err := deserializeUTXO(v)
			if err == nil {
				totalValue += utxo.Value
			}
		}

		return nil
	})

	return
}

// GetDBStats returns low-level bbolt statistics.
func (s *PersistentUTXOStore) GetDBStats() bbolt.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.Stats()
}

// ============================================================================
// Iterator
// ============================================================================

// UTXOIterator iterates over all UTXOs.
type UTXOIterator struct {
	cursor *bbolt.Cursor
}

// NewUTXOIterator creates a new UTXO iterator.
func (s *PersistentUTXOStore) NewUTXOIterator() (*UTXOIterator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var iter *UTXOIterator
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketUTXO)
		iter = &UTXOIterator{cursor: b.Cursor()}
		return nil
	})

	return iter, err
}

// Next returns the next UTXO.
func (i *UTXOIterator) Next() (*UTXO, bool) {
	k, v := i.cursor.Next()
	if k == nil {
		return nil, false
	}

	utxo, err := deserializeUTXO(v)
	if err != nil {
		return nil, false
	}

	return utxo, true
}

// ============================================================================
// Serialization
// ============================================================================

// serializeUTXO serializes a UTXO to bytes.
func serializeUTXO(utxo *UTXO) ([]byte, error) {
	// Format: TxHash(32) + Index(4) + Value(8) + ScriptLen(4) + Script + Address(32)
	size := 32 + 4 + 8 + 4 + len(utxo.Script) + 32
	buf := make([]byte, size)
	offset := 0

	copy(buf[offset:offset+32], utxo.TxHash[:])
	offset += 32

	binary.BigEndian.PutUint32(buf[offset:offset+4], utxo.Index)
	offset += 4

	binary.BigEndian.PutUint64(buf[offset:offset+8], utxo.Value)
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:offset+4], uint32(len(utxo.Script)))
	offset += 4

	copy(buf[offset:offset+len(utxo.Script)], utxo.Script)
	offset += len(utxo.Script)

	copy(buf[offset:offset+32], utxo.Address[:])

	return buf, nil
}

// deserializeUTXO deserializes a UTXO from bytes.
func deserializeUTXO(data []byte) (*UTXO, error) {
	if len(data) < 80 { // Minimum size
		return nil, fmt.Errorf("data too short")
	}

	offset := 0
	var utxo UTXO

	copy(utxo.TxHash[:], data[offset:offset+32])
	offset += 32

	utxo.Index = binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	utxo.Value = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	scriptLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4

	if len(data) < offset+scriptLen+32 {
		return nil, fmt.Errorf("data too short for script")
	}

	utxo.Script = make([]byte, scriptLen)
	copy(utxo.Script, data[offset:offset+scriptLen])
	offset += scriptLen

	copy(utxo.Address[:], data[offset:offset+32])

	return &utxo, nil
}

// indexTransaction adds transaction to index.
func (s *PersistentUTXOStore) indexTransaction(tx *bbolt.Tx, txObj *Transaction) error {
	b := tx.Bucket(BucketTXIndex)
	txHash := txObj.Hash()

	// Store: txHash -> blockHeight (we use 0 for mempool)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], 0) // 0 means unconfirmed
	return b.Put(txHash[:], buf[:])
}

// GetTransactionIndex returns the block height for a transaction.
func (s *PersistentUTXOStore) GetTransactionIndex(txHash [32]byte) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var height uint64
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketTXIndex)
		data := b.Get(txHash[:])
		if data == nil {
			return fmt.Errorf("transaction not found")
		}
		height = binary.BigEndian.Uint64(data)
		return nil
	})

	return height, err
}

// ============================================================================
// Chain Heads (for pruning)
// ============================================================================

// SetChainHead sets the current chain head height.
func (s *PersistentUTXOStore) SetChainHead(height uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketHeads)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], height)
		return b.Put([]byte("head"), buf[:])
	})
}

// GetChainHead returns the current chain head height.
func (s *PersistentUTXOStore) GetChainHead() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var height uint64
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketHeads)
		data := b.Get([]byte("head"))
		if data != nil {
			height = binary.BigEndian.Uint64(data)
		}
		return nil
	})

	return height
}

// ============================================================================
// Metadata
// ============================================================================

// SetMeta stores a metadata key-value pair.
func (s *PersistentUTXOStore) SetMeta(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketMeta)
		return b.Put([]byte(key), []byte(value))
	})
}

// GetMeta retrieves a metadata value.
func (s *PersistentUTXOStore) GetMeta(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketMeta)
		data := b.Get([]byte(key))
		if data != nil {
			value = string(data)
		}
		return nil
	})

	return value
}

// ============================================================================
// Compatibility with In-Memory Store
// ============================================================================

// ToInMemory converts to in-memory store (for testing/migration).
func (s *PersistentUTXOStore) ToInMemory() *UTXOStore {
	store := NewUTXOStore()

	s.mu.RLock()
	_ = s.db.View(func(tx *bbolt.Tx) error {
		// Copy UTXOs
		b := tx.Bucket(BucketUTXO)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			utxo, err := deserializeUTXO(v)
			if err != nil {
				continue
			}
			store.AddUTXO(utxo)
		}
		return nil
	})
	s.mu.RUnlock()

	return store
}

// ============================================================================
// Open DB helpers
// ============================================================================

// OpenDB opens an existing database.
func OpenDB(path string) (*bbolt.DB, error) {
	return bbolt.Open(path, 0600, nil)
}

// CreateDB creates a new database with schema.
func CreateDB(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		buckets := [][]byte{BucketUTXO, BucketBalances, BucketHeads, BucketMeta, BucketTXIndex}
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
