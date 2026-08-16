package utxo

import (
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// PersistentUTXOStore Tests
// ============================================================================

func createTestPersistentStore(t *testing.T) (*PersistentUTXOStore, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := NewPersistentUTXOStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create persistent store: %v", err)
	}

	return store, dbPath
}

func TestNewPersistentUTXOStore(t *testing.T) {
	t.Parallel()
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	if store == nil {
		t.Fatal("store should not be nil")
	}
}

func TestNewPersistentUTXOStore_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := NewPersistentUTXOStore("/nonexistent/deeply/nested/path/test.db")
	if err == nil {
		t.Error("should error for invalid path")
	}
}

func TestPersistentStore_AddAndGetUTXO(t *testing.T) {
	t.Parallel()
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	utxo := &UTXO{
		TxHash:  [32]byte{1, 2, 3, 4},
		Index:   0,
		Value:   1000,
		Script:  []byte{0xaa, 0xbb},
		Address: [32]byte{10, 20, 30},
	}

	if err := store.AddUTXO(utxo); err != nil {
		t.Fatalf("AddUTXO failed: %v", err)
	}

	got, err := store.GetUTXO([32]byte{1, 2, 3, 4}, 0)
	if err != nil {
		t.Fatalf("GetUTXO failed: %v", err)
	}

	if got.Value != 1000 {
		t.Errorf("expected Value 1000, got %d", got.Value)
	}
	if got.Index != 0 {
		t.Errorf("expected Index 0, got %d", got.Index)
	}
	if got.Address != utxo.Address {
		t.Error("address mismatch")
	}
}

func TestPersistentStore_GetUTXO_NotFound(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	_, err := store.GetUTXO([32]byte{99}, 0)
	if err == nil {
		t.Error("should error for non-existent UTXO")
	}
}

func TestPersistentStore_HasUTXO(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	txHash := [32]byte{5, 6, 7}
	if store.HasUTXO(txHash, 0) {
		t.Error("should not have UTXO before adding")
	}

	utxo := &UTXO{
		TxHash:  txHash,
		Index:   0,
		Value:   500,
		Address: [32]byte{1},
	}
	store.AddUTXO(utxo)

	if !store.HasUTXO(txHash, 0) {
		t.Error("should have UTXO after adding")
	}
}

func TestPersistentStore_SpendUTXO(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	addr := [32]byte{10, 20}
	utxo := &UTXO{
		TxHash:  [32]byte{1, 2},
		Index:   0,
		Value:   1000,
		Address: addr,
	}

	store.AddUTXO(utxo)

	// Verify balance before spend
	bal := store.GetBalance(addr)
	if bal != 1000 {
		t.Errorf("expected balance 1000, got %d", bal)
	}

	// Spend UTXO
	err := store.SpendUTXO([32]byte{1, 2}, 0)
	if err != nil {
		t.Fatalf("SpendUTXO failed: %v", err)
	}

	// UTXO should be gone
	if store.HasUTXO([32]byte{1, 2}, 0) {
		t.Error("UTXO should be removed after spend")
	}

	// Balance should be 0
	bal = store.GetBalance(addr)
	if bal != 0 {
		t.Errorf("expected balance 0 after spend, got %d", bal)
	}
}

func TestPersistentStore_SpendUTXO_NotFound(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	err := store.SpendUTXO([32]byte{99}, 0)
	if err == nil {
		t.Error("should error when spending non-existent UTXO")
	}
}

func TestPersistentStore_GetBalance(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	addr := [32]byte{1, 2, 3}

	// Zero balance initially
	if bal := store.GetBalance(addr); bal != 0 {
		t.Errorf("expected 0, got %d", bal)
	}

	// Add two UTXOs
	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 500, Address: addr})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 300, Address: addr})

	if bal := store.GetBalance(addr); bal != 800 {
		t.Errorf("expected 800, got %d", bal)
	}

	// Spend one
	store.SpendUTXO([32]byte{1}, 0)

	if bal := store.GetBalance(addr); bal != 300 {
		t.Errorf("expected 300, got %d", bal)
	}
}

func TestPersistentStore_GetUTXOCount(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	if store.GetUTXOCount() != 0 {
		t.Errorf("expected 0, got %d", store.GetUTXOCount())
	}

	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 100, Address: [32]byte{1}})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 200, Address: [32]byte{2}})

	if store.GetUTXOCount() != 2 {
		t.Errorf("expected 2, got %d", store.GetUTXOCount())
	}
}

func TestPersistentStore_ReadOnly(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	store.SetReadOnly(true)

	if !store.IsReadOnly() {
		t.Error("should be read-only")
	}

	err := store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 100, Address: [32]byte{1}})
	if err == nil {
		t.Error("should error on write in read-only mode")
	}

	err = store.SpendUTXO([32]byte{1}, 0)
	if err == nil {
		t.Error("should error on spend in read-only mode")
	}
}

func TestPersistentStore_ChainHead(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	if store.GetChainHead() != 0 {
		t.Error("initial chain head should be 0")
	}

	store.SetChainHead(12345)

	if store.GetChainHead() != 12345 {
		t.Errorf("expected chain head 12345, got %d", store.GetChainHead())
	}
}

func TestPersistentStore_Metadata(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	if store.GetMeta("version") != "" {
		t.Error("should be empty for non-existent key")
	}

	store.SetMeta("version", "1.0.0")
	store.SetMeta("network", "mainnet")

	if v := store.GetMeta("version"); v != "1.0.0" {
		t.Errorf("expected '1.0.0', got '%s'", v)
	}

	if v := store.GetMeta("network"); v != "mainnet" {
		t.Errorf("expected 'mainnet', got '%s'", v)
	}
}

func TestPersistentStore_Snapshot(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	// Add data
	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 1000, Address: [32]byte{10}})
	store.SetChainHead(100)

	// Create snapshot
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	err := store.Snapshot(snapshotPath)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	// Verify snapshot file exists
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Error("snapshot file should exist")
	}

	// Open snapshot as new store
	snapshotStore, err := NewPersistentUTXOStore(snapshotPath)
	if err != nil {
		t.Fatalf("failed to open snapshot: %v", err)
	}
	defer snapshotStore.Close()

	// Verify data in snapshot
	if snapshotStore.GetBalance([32]byte{10}) != 1000 {
		t.Error("snapshot should contain original balance")
	}

	if snapshotStore.GetChainHead() != 100 {
		t.Error("snapshot should contain chain head")
	}
}

func TestPersistentStore_GetStats(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	addr1 := [32]byte{1}
	addr2 := [32]byte{2}

	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 500, Address: addr1})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 300, Address: addr1})
	store.AddUTXO(&UTXO{TxHash: [32]byte{3}, Index: 0, Value: 200, Address: addr2})

	utxoCount, addrCount, totalValue := store.GetStats()

	if utxoCount != 3 {
		t.Errorf("expected 3 UTXOs, got %d", utxoCount)
	}

	if addrCount != 2 {
		t.Errorf("expected 2 addresses, got %d", addrCount)
	}

	if totalValue != 1000 {
		t.Errorf("expected total 1000, got %d", totalValue)
	}
}

func TestPersistentStore_ToInMemory(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	addr := [32]byte{1}
	store.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 500, Address: addr})
	store.AddUTXO(&UTXO{TxHash: [32]byte{2}, Index: 0, Value: 300, Address: addr})

	memStore := store.ToInMemory()

	if memStore.GetBalance(addr) != 800 {
		t.Errorf("expected balance 800, got %d", memStore.GetBalance(addr))
	}

	if memStore.GetUTXOCount() != 2 {
		t.Errorf("expected 2 UTXOs, got %d", memStore.GetUTXOCount())
	}
}

func TestPersistentStore_TransactionIndex(t *testing.T) {
	store, _ := createTestPersistentStore(t)
	defer store.Close()

	txHash := [32]byte{1, 2, 3}

	// Not found initially
	_, err := store.GetTransactionIndex(txHash)
	if err == nil {
		t.Error("should error for non-existent tx")
	}
}

func TestSerializeDeserializeUTXO(t *testing.T) {
	original := &UTXO{
		TxHash:  [32]byte{1, 2, 3, 4, 5},
		Index:   42,
		Value:   123456789,
		Script:  []byte{0xaa, 0xbb, 0xcc, 0xdd},
		Address: [32]byte{10, 20, 30, 40, 50},
	}

	data, err := serializeUTXO(original)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	restored, err := deserializeUTXO(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if restored.TxHash != original.TxHash {
		t.Error("TxHash mismatch")
	}
	if restored.Index != original.Index {
		t.Errorf("Index: expected %d, got %d", original.Index, restored.Index)
	}
	if restored.Value != original.Value {
		t.Errorf("Value: expected %d, got %d", original.Value, restored.Value)
	}
	if len(restored.Script) != len(original.Script) {
		t.Errorf("Script length mismatch")
	}
	if restored.Address != original.Address {
		t.Error("Address mismatch")
	}
}

func TestDeserializeUTXO_TooShort(t *testing.T) {
	_, err := deserializeUTXO([]byte{1, 2, 3})
	if err == nil {
		t.Error("should error for too-short data")
	}
}

func TestPersistentStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	// Create and populate store
	store1, err := NewPersistentUTXOStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	store1.AddUTXO(&UTXO{TxHash: [32]byte{1}, Index: 0, Value: 999, Address: [32]byte{10}})
	store1.SetChainHead(500)
	store1.SetMeta("version", "2.0")
	store1.Close()

	// Reopen and verify
	store2, err := NewPersistentUTXOStore(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	if store2.GetBalance([32]byte{10}) != 999 {
		t.Error("balance should persist")
	}

	if store2.GetChainHead() != 500 {
		t.Error("chain head should persist")
	}

	if store2.GetMeta("version") != "2.0" {
		t.Error("metadata should persist")
	}

	if !store2.HasUTXO([32]byte{1}, 0) {
		t.Error("UTXO should persist")
	}
}

func TestCreateDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "create.db")

	db, err := CreateDB(dbPath)
	if err != nil {
		t.Fatalf("CreateDB failed: %v", err)
	}
	db.Close()

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("DB file should exist")
	}
}
