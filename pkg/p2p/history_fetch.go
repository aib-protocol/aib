// Package p2p: on-demand historical block refetch (full<->light channel).
// A pruned (light) node can request any range of historical blocks from its
// connected full nodes using GET_BLOCKS_BY_RANGE / BLOCKS_BY_RANGE_RESPONSE,
// mirroring Bitcoin's getheaders/getdata pattern.
package p2p

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// MaxRangeBlocks is the hard cap on blocks per GET_BLOCKS_BY_RANGE request
// (and per response) to bound memory usage.
const MaxRangeBlocks = 100

// GetBlocksByRangeMsg requests historical blocks [HeightFrom, HeightTo].
type GetBlocksByRangeMsg struct {
	RequestID  uint64 `json:"request_id"`
	HeightFrom uint64 `json:"height_from"`
	HeightTo   uint64 `json:"height_to"`
}

// BlocksByRangeRespMsg is the full node's reply. Blocks the serving node does
// not have (pruned / below its horizon) are listed in Missing; the requester
// can try other peers for those.
type BlocksByRangeRespMsg struct {
	RequestID  uint64      `json:"request_id"`
	HeightFrom uint64      `json:"height_from"`
	HeightTo   uint64      `json:"height_to"`
	Missing    []uint64    `json:"missing,omitempty"`
	Blocks     []BlockData `json:"blocks"`
}

// fetch state: exactly one outstanding FetchBlocksByRange at a time.
var rangeFetchReqCounter uint64

// SetBlocksByRangeHandler registers the serving-side handler. The handler
// returns the blocks the node actually has in [from,to] plus the heights it
// knows are missing. Returning (nil, nil) means "I have nothing in this
// range" — the manager silently ignores the request per protocol.
func (pm *ChainPeerManager) SetBlocksByRangeHandler(fn func(from, to uint64) (blocks []BlockData, missing []uint64)) {
	pm.onBlocksByRange = fn
}

// serveGetBlocksByRange handles an inbound GET_BLOCKS_BY_RANGE.
func (pm *ChainPeerManager) serveGetBlocksByRange(peer *ChainPeer, payload []byte) {
	var msg GetBlocksByRangeMsg
	if err := UnmarshalMsg(payload, &msg); err != nil {
		return
	}
	if pm.onBlocksByRange == nil {
		return // not serving historical blocks
	}
	from, to := msg.HeightFrom, msg.HeightTo
	if to < from {
		return
	}
	if to-from+1 > MaxRangeBlocks {
		to = from + MaxRangeBlocks - 1
	}
	blocks, missing := pm.onBlocksByRange(from, to)
	if len(blocks) == 0 && len(missing) == 0 {
		return // nothing known for this range: ignore
	}
	resp := BlocksByRangeRespMsg{
		RequestID:  msg.RequestID,
		HeightFrom: from,
		HeightTo:   to,
		Missing:    missing,
		Blocks:     blocks,
	}
	data, err := MarshalMsg(MsgBlocksByRangeResp, &resp)
	if err != nil {
		pm.logger.Printf("[P2P] Failed to marshal range response: %v", err)
		return
	}
	peer.mu.Lock()
	peer.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, werr := peer.conn.Write(data)
	peer.conn.SetWriteDeadline(time.Time{})
	peer.mu.Unlock()
	if werr != nil {
		pm.logger.Printf("[P2P] Failed to send range response to %s: %v", peer.nodeID, werr)
	}
}

// deliverRangeResponse routes an inbound BLOCKS_BY_RANGE_RESPONSE to a
// waiting FetchBlocksByRange caller (matched by request ID).
func (pm *ChainPeerManager) deliverRangeResponse(payload []byte) {
	var msg BlocksByRangeRespMsg
	if err := UnmarshalMsg(payload, &msg); err != nil {
		return
	}
	pm.fetchChMu.Lock()
	ch, wantID := pm.fetchCh, pm.fetchReqID
	pm.fetchChMu.Unlock()
	if ch == nil || msg.RequestID != wantID {
		return // stale response from an earlier fetch
	}
	select {
	case ch <- msg:
	default: // buffer full; caller already has plenty to work with
	}
}

// FetchBlocksByRange asks ALL connected peers for blocks [from,to], merges
// their responses, verifies block signatures, and returns the blocks sorted
// by height. It tolerates out-of-order blocks, duplicates, gaps and peers
// that do not respond. Range is clamped to MaxRangeBlocks.
func (pm *ChainPeerManager) FetchBlocksByRange(from, to uint64, timeout time.Duration) ([]BlockData, error) {
	if to < from {
		return nil, fmt.Errorf("invalid range: to (%d) < from (%d)", to, from)
	}
	if to-from+1 > MaxRangeBlocks {
		to = from + MaxRangeBlocks - 1
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	// Only one outstanding range fetch at a time.
	pm.fetchMu.Lock()
	defer pm.fetchMu.Unlock()

	reqID := atomic.AddUint64(&rangeFetchReqCounter, 1)
	ch := make(chan BlocksByRangeRespMsg, 64)
	pm.fetchChMu.Lock()
	pm.fetchCh, pm.fetchReqID = ch, reqID
	pm.fetchChMu.Unlock()
	defer func() {
		pm.fetchChMu.Lock()
		pm.fetchCh = nil
		pm.fetchChMu.Unlock()
	}()

	msg := GetBlocksByRangeMsg{RequestID: reqID, HeightFrom: from, HeightTo: to}
	data, err := MarshalMsg(MsgGetBlocksByRange, &msg)
	if err != nil {
		return nil, err
	}

	pm.mu.RLock()
	peers := make([]*ChainPeer, 0, len(pm.peers))
	for _, p := range pm.peers {
		if p.verified {
			peers = append(peers, p)
		}
	}
	pm.mu.RUnlock()
	if len(peers) == 0 {
		return nil, errors.New("no connected peers to fetch historical blocks from")
	}
	for _, p := range peers {
		p.mu.Lock()
		p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		p.conn.Write(data)
		p.conn.SetWriteDeadline(time.Time{})
		p.mu.Unlock()
	}

	// Collect and merge.
	want := to - from + 1
	got := make(map[uint64]BlockData, want)
	deadline := time.After(timeout)
collect:
	for uint64(len(got)) < want {
		select {
		case <-deadline:
			break collect
		case resp := <-ch:
			for _, b := range resp.Blocks {
				if _, dup := got[b.Height]; !dup {
					got[b.Height] = b
				}
			}
		}
	}

	// Signature-verify every block; drop failures (SEC-008 pattern).
	blocks := make([]BlockData, 0, len(got))
	for _, b := range got {
		if err := pm.verifyBlockSignature(b); err != nil {
			pm.logger.Printf("[P2P] Dropped fetched block %d: signature verification failed: %v", b.Height, err)
			continue
		}
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Height < blocks[j].Height })
	return blocks, nil
}

// ValidateFetchedBlocks enforces the prevHash chain over a fetched block set
// and drops anything that does not link to a locally-known hash.
//
//   - anchor is the expected hash of block (from-1); "" means unknown, in
//     which case the first run cannot be externally anchored.
//   - lookup returns the local hash for a height, if the node has it. Runs
//     starting above `from` must link to a local block, otherwise they are
//     discarded (a malicious peer cannot inject a floating sub-chain).
//
// Within a run, block[i].PrevBlockHash must equal block[i-1].Hash; the first
// mismatched block truncates the run. Returns the surviving valid blocks.
func ValidateFetchedBlocks(blocks []BlockData, from, to uint64, anchor string, lookup func(height uint64) (string, bool)) []BlockData {
	// Dedupe by height, drop out-of-range entries and entries without raw data.
	byHeight := make(map[uint64]BlockData, len(blocks))
	for _, b := range blocks {
		if b.Height < from || b.Height > to {
			continue
		}
		if len(b.RawBlock) == 0 {
			continue
		}
		if _, dup := byHeight[b.Height]; !dup {
			byHeight[b.Height] = b
		}
	}

	heights := make([]uint64, 0, len(byHeight))
	for h := range byHeight {
		heights = append(heights, h)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })

	// Verify metadata matches the raw serialized block (height + hash), so a
	// peer cannot serve a real block with forged prevHash metadata.
	type entry struct {
		bd   BlockData
		prev string
		hash string
	}
	valid := make([]entry, 0, len(heights))
	for _, h := range heights {
		bd := byHeight[h]
		blk, err := utxoPkg.DeserializeBlock(bd.RawBlock)
		if err != nil {
			continue
		}
		if blk.Header.Height != bd.Height {
			continue
		}
		if hex.EncodeToString(blk.Hash[:]) != bd.Hash {
			continue
		}
		if hex.EncodeToString(blk.Header.PrevBlockHash[:]) != bd.PrevBlockHash {
			continue
		}
		valid = append(valid, entry{bd: bd, prev: bd.PrevBlockHash, hash: bd.Hash})
	}

	// Walk contiguous runs; anchor each run on a known hash.
	out := make([]BlockData, 0, len(valid))
	var lastHeight uint64
	var lastHash string
	runOpen := false
	for _, e := range valid {
		if runOpen && e.bd.Height == lastHeight+1 {
			if e.prev == lastHash {
				out = append(out, e.bd)
				lastHash, lastHeight = e.hash, e.bd.Height
			} else {
				// prevHash does not link: drop this block and close the run;
				// a later block can only re-enter via a fresh anchor check.
				runOpen = false
			}
			continue
		}
		// New run head: must be anchored on something we already know.
		anchored := false
		switch {
		case e.bd.Height == from && anchor != "" && e.prev == anchor:
			anchored = true // anchored on caller-supplied expected hash
		case lookup != nil:
			if localHash, ok := lookup(e.bd.Height - 1); ok && e.prev == localHash {
				anchored = true // anchored on a locally stored block
			}
		case e.bd.Height == from && anchor == "":
			anchored = true // no anchor reference available: accept the head
		}
		if anchored {
			out = append(out, e.bd)
			lastHash, lastHeight = e.hash, e.bd.Height
			runOpen = true
		}
	}
	return out
}
