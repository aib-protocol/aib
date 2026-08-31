package utxo

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"
)

// TestPrunerBasics: disabled pruner never prunes; enabled with height
// under keep is a no-op; watermark starts at 0.
func TestPrunerBasics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("NewChainState: %v", err)
	}
	defer cs.Close()

	pr := NewPruner(2)
	if !pr.Enabled() || pr.Keep() != 2 {
		t.Fatal("pruner not configured")
	}
	n, err := pr.Prune(cs, 0)
	if err != nil || n != 0 {
		t.Fatalf("prune empty: n=%d err=%v", n, err)
	}
	off := NewPruner(0)
	if off.Enabled() {
		t.Fatal("keep=0 must disable")
	}
	if _, err := off.Prune(cs, 1000); err != nil {
		t.Fatalf("disabled prune err: %v", err)
	}
	if cs.PruneBelow() != 0 {
		t.Fatalf("watermark = %d, want 0", cs.PruneBelow())
	}
}

// TestPruneGenesisBody: build genesis → prune at best=10 keep=5 →
// genesis body removed, watermark=5, GetBlockByHash returns ErrPruned.
func TestPruneGenesisBody(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cs, err := NewChainState(dbPath)
	if err != nil {
		t.Fatalf("NewChainState: %v", err)
	}
	defer cs.Close()
	utxoStore, err := NewPersistentUTXOStore(filepath.Join(tmpDir, "utxo.db"))
	if err != nil {
		t.Fatalf("UTXO store: %v", err)
	}
	defer utxoStore.Close()
	cs.SetUTXOStore(utxoStore)
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	var proposerAddr [32]byte
	copy(proposerAddr[:], pubKey)
	coinbaseTx := CreateCoinbaseV2(proposerAddr, 0)
	coinbaseTx.SignInput(0, privKey)
	genesisBlock := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposerAddr)
	genesisBlock.Header.Timestamp = 1704067200
	genesisBlock.SignBlock(privKey)
	if err := cs.InitGenesis(genesisBlock); err != nil {
		t.Fatalf("InitGenesis: %v", err)
	}

	pr := NewPruner(5)
	// bestHeight=10, keep=5 → cutoff=5. Only genesis (0) exists, so 1 deleted.
	n, err := pr.Prune(cs, 10)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted (genesis), got %d", n)
	}
	if cs.PruneBelow() != 5 {
		t.Fatalf("watermark = %d, want 5", cs.PruneBelow())
	}
	// Genesis body pruned → GetBlockByHash returns ErrPruned.
	gh := genesisBlock.CalculateHash()
	if _, err := cs.GetBlockByHash(gh); err != ErrPruned {
		t.Fatalf("expected ErrPruned, got %v", err)
	}
	// Running again is a no-op.
	n2, _ := pr.Prune(cs, 10)
	if n2 != 0 {
		t.Fatalf("second prune deleted %d, should be 0", n2)
	}
}
