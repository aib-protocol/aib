package consensus

import (
	"testing"
)

func TestMemoryStorage_NewMemoryStorage(t *testing.T) {
	store := NewMemoryStorage()

	if store == nil {
		t.Fatal("expected non-nil MemoryStorage")
	}

	if store.count != 0 {
		t.Errorf("expected count 0, got %d", store.count)
	}

	if store.blocks == nil {
		t.Error("expected non-nil blocks map")
	}

	if store.hashMap == nil {
		t.Error("expected non-nil hashMap")
	}
}

func TestMemoryStorage_SaveBlock(t *testing.T) {
	store := NewMemoryStorage()

	block := &Block{
		Height:       0,
		TaskID:       "test-task",
		FinalResult:  "result",
		IsValid:      true,
		Nonce:        123,
	}

	err := store.SaveBlock(block)
	if err != nil {
		t.Fatalf("failed to save block: %v", err)
	}

	if store.count != 1 {
		t.Errorf("expected count 1, got %d", store.count)
	}
}

func TestMemoryStorage_SaveBlockNil(t *testing.T) {
	store := NewMemoryStorage()

	err := store.SaveBlock(nil)
	if err == nil {
		t.Error("expected error for nil block")
	}
}

func TestMemoryStorage_LoadBlock(t *testing.T) {
	store := NewMemoryStorage()

	block := &Block{
		Height:       5,
		TaskID:       "test-task",
		FinalResult:  "result",
		IsValid:      true,
	}

	store.SaveBlock(block)

	loaded, err := store.LoadBlock(5)
	if err != nil {
		t.Fatalf("failed to load block: %v", err)
	}

	if loaded.Height != 5 {
		t.Errorf("expected height 5, got %d", loaded.Height)
	}
	if loaded.TaskID != "test-task" {
		t.Errorf("expected task ID test-task, got %s", loaded.TaskID)
	}
}

func TestMemoryStorage_LoadBlockNotFound(t *testing.T) {
	store := NewMemoryStorage()

	_, err := store.LoadBlock(0)
	if err == nil {
		t.Error("expected error for non-existent block")
	}
}

func TestMemoryStorage_LoadAllBlocks(t *testing.T) {
	store := NewMemoryStorage()

	blocks := []*Block{
		{Height: 0, TaskID: "task0"},
		{Height: 1, TaskID: "task1"},
		{Height: 2, TaskID: "task2"},
	}

	for _, b := range blocks {
		store.SaveBlock(b)
	}

	loaded, err := store.LoadAllBlocks()
	if err != nil {
		t.Fatalf("failed to load all blocks: %v", err)
	}

	if len(loaded) != 3 {
		t.Errorf("expected 3 blocks, got %d", len(loaded))
	}
}

func TestMemoryStorage_GetLatestBlock(t *testing.T) {
	store := NewMemoryStorage()

	_, err := store.GetLatestBlock()
	if err == nil {
		t.Error("expected error when no blocks")
	}

	store.SaveBlock(&Block{Height: 0, TaskID: "task0"})
	store.SaveBlock(&Block{Height: 1, TaskID: "task1"})
	store.SaveBlock(&Block{Height: 2, TaskID: "task2"})

	latest, err := store.GetLatestBlock()
	if err != nil {
		t.Fatalf("failed to get latest block: %v", err)
	}

	if latest.Height != 2 {
		t.Errorf("expected height 2, got %d", latest.Height)
	}
}

func TestMemoryStorage_GetBlockByHash(t *testing.T) {
	store := NewMemoryStorage()

	block := &Block{
		Height:   5,
		TaskID:   "test-task",
		FinalResult: "result",
		Nonce:   100,
	}

	store.SaveBlock(block)

	hash := block.Hash()

	loaded, err := store.GetBlockByHash(hash)
	if err != nil {
		t.Fatalf("failed to get block by hash: %v", err)
	}

	if loaded.Height != 5 {
		t.Errorf("expected height 5, got %d", loaded.Height)
	}
}

func TestMemoryStorage_GetBlockByHashNotFound(t *testing.T) {
	store := NewMemoryStorage()

	_, err := store.GetBlockByHash([]byte("nonexistent"))
	if err == nil {
		t.Error("expected error for non-existent hash")
	}
}

func TestMemoryStorage_GetBlockCount(t *testing.T) {
	store := NewMemoryStorage()

	count := store.GetBlockCount()
	if count != 0 {
		t.Errorf("expected 0 blocks, got %d", count)
	}

	store.SaveBlock(&Block{Height: 0})
	store.SaveBlock(&Block{Height: 1})
	store.SaveBlock(&Block{Height: 2})

	count = store.GetBlockCount()
	if count != 3 {
		t.Errorf("expected 3 blocks, got %d", count)
	}
}

func TestMemoryStorage_Close(t *testing.T) {
	store := NewMemoryStorage()

	err := store.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMemoryStorage_Clear(t *testing.T) {
	store := NewMemoryStorage()

	store.SaveBlock(&Block{Height: 0})
	store.SaveBlock(&Block{Height: 1})
	store.SaveBlock(&Block{Height: 2})

	store.Clear()

	count := store.GetBlockCount()
	if count != 0 {
		t.Errorf("expected 0 blocks after clear, got %d", count)
	}
}

func TestMemoryStorage_SaveBlockOutOfOrder(t *testing.T) {
	store := NewMemoryStorage()

	// Save blocks out of order
	store.SaveBlock(&Block{Height: 2, TaskID: "task2"})
	store.SaveBlock(&Block{Height: 0, TaskID: "task0"})
	store.SaveBlock(&Block{Height: 1, TaskID: "task1"})

	// Should be able to load them all
	for i := uint64(0); i < 3; i++ {
		block, err := store.LoadBlock(i)
		if err != nil {
			t.Errorf("failed to load block %d: %v", i, err)
		}
		if block.Height != i {
			t.Errorf("expected height %d, got %d", i, block.Height)
		}
	}
}

func TestMemoryStorage_Concurrent(t *testing.T) {
	store := NewMemoryStorage()

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				store.SaveBlock(&Block{Height: uint64(n*10 + j), TaskID: "task"})
			}
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// All 100 blocks should be saved
	if store.count != 100 {
		t.Errorf("expected 100 blocks, got %d", store.count)
	}
}

func TestFileStorage_NewFileStorage(t *testing.T) {
	fs := NewFileStorage("/tmp/test_blocks.dat")

	if fs == nil {
		t.Fatal("expected non-nil FileStorage")
	}

	if fs.MemoryStorage == nil {
		t.Error("expected embedded MemoryStorage")
	}

	if fs.filePath != "/tmp/test_blocks.dat" {
		t.Errorf("expected file path /tmp/test_blocks.dat, got %s", fs.filePath)
	}
}

func TestFileStorage_Snapshot(t *testing.T) {
	store := NewFileStorage("/tmp/test_snapshot.dat")

	store.SaveBlock(&Block{Height: 0, TaskID: "task0"})
	store.SaveBlock(&Block{Height: 1, TaskID: "task1"})

	err := store.Snapshot()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileStorage_Restore(t *testing.T) {
	store := NewFileStorage("/tmp/test_restore.dat")

	store.SaveBlock(&Block{Height: 0, TaskID: "task0"})

	err := store.Restore()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileStorage_Close(t *testing.T) {
	store := NewFileStorage("/tmp/test_close.dat")

	err := store.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
