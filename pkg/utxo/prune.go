// Package utxo — block pruning (light node support).
//
// Pruning lets a sync node keep only the last N blocks in the local
// bbolt database, dramatically reducing storage on long-running nodes.
// Headers + UTXO state + block-index are NEVER pruned, so:
//
//   - Best-block, validator set, balance queries keep working
//   - The node can still produce / validate new blocks (UTXO replay
//     needs only the most recent UTXO snapshot, which we keep live)
//   - Historical block bodies can be re-fetched on demand from peers
//     (handled by feat/history-refetch)
//
// Validators MUST NOT prune - they need the full UTXO history to
// validate reorgs beyond the snapshot point. main.go enforces this.
package utxo

import (
	"encoding/binary"
	"fmt"
	"sync"

	"go.etcd.io/bbolt"
)

// ErrPruned is returned by GetBlockBy* when the requested block was
// pruned from the local database.
var ErrPruned = fmt.Errorf("block body pruned")

// Pruner owns the pruning policy.
type Pruner struct {
	keepBlocks uint64 // 0 = disabled (full history)
	mu         sync.Mutex
}

// NewPruner creates a pruner that keeps the most recent keepBlocks
// block bodies. keepBlocks==0 disables pruning entirely.
func NewPruner(keepBlocks uint64) *Pruner {
	return &Pruner{keepBlocks: keepBlocks}
}

// Enabled reports whether pruning is active.
func (p *Pruner) Enabled() bool { return p != nil && p.keepBlocks > 0 }

// Keep returns the configured keep-blocks count (0 = disabled).
func (p *Pruner) Keep() uint64 {
	if p == nil {
		return 0
	}
	return p.keepBlocks
}

// Prune removes block bodies with height <= (bestHeight - keepBlocks).
// Returns the number of block bodies deleted. Safe to call repeatedly.
func (p *Pruner) Prune(cs *ChainState, bestHeight uint64) (int, error) {
	if !p.Enabled() || bestHeight <= p.keepBlocks {
		return 0, nil
	}
	cutoff := bestHeight - p.keepBlocks
	p.mu.Lock()
	defer p.mu.Unlock()

	deleted := 0
	err := cs.db.Update(func(tx *bbolt.Tx) error {
		blocksBucket := tx.Bucket(BucketBlocks)
		heightBucket := tx.Bucket(BucketBlockHeight)

		var h uint64
		for h = 0; h <= cutoff; h++ {
			heightKey := make([]byte, 8)
			binary.BigEndian.PutUint64(heightKey, h)
			hashBytes := heightBucket.Get(heightKey)
			if hashBytes == nil {
				continue
			}
			var hash [32]byte
			copy(hash[:], hashBytes)
			if blocksBucket.Get(hash[:]) == nil {
				continue
			}
			if err := blocksBucket.Delete(hash[:]); err != nil {
				return err
			}
			deleted++
		}
		meta := tx.Bucket(BucketChainMeta)
		if meta != nil {
			watermark := make([]byte, 8)
			binary.BigEndian.PutUint64(watermark, cutoff)
			if err := meta.Put([]byte("prune_below"), watermark); err != nil {
				return err
			}
		}
		return nil
	})
	return deleted, err
}

// PruneBelow returns the highest pruned height + 1 (i.e. the lowest
// height whose body may have been deleted). Returns 0 if never pruned.
func (cs *ChainState) PruneBelow() uint64 {
	var v uint64
	_ = cs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BucketChainMeta)
		if b == nil {
			return nil
		}
		data := b.Get([]byte("prune_below"))
		if len(data) == 8 {
			v = binary.BigEndian.Uint64(data)
		}
		return nil
	})
	return v
}

// StoreFetchedBlock persists a re-fetched historical block body back
// into the blocks bucket (light-node history refetch). The block index
// (height -> hash) is untouched: it was never pruned.
func (cs *ChainState) StoreFetchedBlock(block *Block) error {
	hash := block.CalculateHash()
	return cs.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(BucketBlocks).Put(hash[:], block.SerializeBlock())
	})
}

// GetBlockHashByHeight returns the block hash for a height from the
// in-memory index (never pruned). ok=false when unknown.
func (cs *ChainState) GetBlockHashByHeight(height uint64) ([32]byte, bool) {
	cs.blockIndexMu.RLock()
	hash, ok := cs.blockIndex[height]
	cs.blockIndexMu.RUnlock()
	return hash, ok
}
