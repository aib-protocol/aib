// Package utxo — release anchor index (part of RFC-distribution-D).
// Tracks the most recent on-chain release anchor as blocks are added.
package utxo

import (
	"encoding/hex"
	"sync"
)

// AnchorIndex tracks the latest release anchor seen on chain.
type AnchorIndex struct {
	mu      sync.RWMutex
	latest  *ReleaseRecord
	history []*ReleaseRecord
}

// NewAnchorIndex creates an empty index.
func hexIfSet(sha [32]byte) string {
	for _, b := range sha {
		if b != 0 {
			return hex.EncodeToString(sha[:])
		}
	}
	return ""
}

func NewAnchorIndex() *AnchorIndex { return &AnchorIndex{} }

// ScanBlock scans a block for anchor outputs and records any found.
func (ai *AnchorIndex) ScanBlock(b *Block) {
	for ti, tx := range b.Transactions {
		for _, out := range tx.Outputs {
			if !IsAnchorOutput(out) {
				continue
			}
			name, binSHA, insSHA, err := ParseAnchorScript(out.Script)
			if err != nil {
				continue
			}
			txh := tx.Hash()
			rec := &ReleaseRecord{
				Name:            name,
				SHA256:          hex.EncodeToString(binSHA[:]),
				InstallerSHA256: hexIfSet(insSHA),
				Height:          b.Header.Height,
				TxHash:          hex.EncodeToString(txh[:]),
			}
			_ = ti
			ai.mu.Lock()
			ai.history = append(ai.history, rec)
			ai.latest = rec
			ai.mu.Unlock()
		}
	}
}

// Latest returns the newest release anchor (or nil).
func (ai *AnchorIndex) Latest() *ReleaseRecord {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	if ai.latest == nil {
		return nil
	}
	c := *ai.latest
	return &c
}

// History returns all anchors in insertion order.
func (ai *AnchorIndex) History() []ReleaseRecord {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	out := make([]ReleaseRecord, len(ai.history))
	for i, r := range ai.history {
		out[i] = *r
	}
	return out
}
