// Package p2p implements P2P networking for AIB blockchain.
// ChainBlockSyncer handles initial block download and chain synchronization.
package p2p

import (
	"log"
	"sync"
	"time"
)

// SyncState represents the current sync state.
type SyncState int

const (
	SyncStateIdle SyncState = iota
	SyncStateSyncing
	SyncStateCaughtUp
)

// String returns human-readable sync state.
func (s SyncState) String() string {
	switch s {
	case SyncStateIdle:
		return "idle"
	case SyncStateSyncing:
		return "syncing"
	case SyncStateCaughtUp:
		return "caught_up"
	default:
		return "unknown"
	}
}

// ChainBlockSyncer synchronizes blocks from peers.
type ChainBlockSyncer struct {
	mu sync.RWMutex

	pm           *ChainPeerManager
	state        SyncState
	localHeight  uint64
	targetHeight uint64
	logger       *log.Logger

	// Callbacks
	getLocalHeight func() uint64
	processBlock   func(data BlockData) error

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewChainBlockSyncer creates a new block syncer.
func NewChainBlockSyncer(pm *ChainPeerManager, logger *log.Logger) *ChainBlockSyncer {
	if logger == nil {
		logger = log.Default()
	}
	return &ChainBlockSyncer{
		pm:     pm,
		state:  SyncStateIdle,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// SetHandlers sets the sync callbacks.
func (bs *ChainBlockSyncer) SetHandlers(
	getLocalHeight func() uint64,
	processBlock func(data BlockData) error,
) {
	bs.getLocalHeight = getLocalHeight
	bs.processBlock = processBlock
}

// StartSync begins the sync loop.
func (bs *ChainBlockSyncer) StartSync() {
	bs.wg.Add(1)
	go bs.syncLoop()
}

// StopSync stops the syncer.
func (bs *ChainBlockSyncer) StopSync() {
	close(bs.stopCh)
	bs.wg.Wait()
}

// GetState returns the current sync state.
func (bs *ChainBlockSyncer) GetState() SyncState {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.state
}

// GetProgress returns sync progress info.
func (bs *ChainBlockSyncer) GetProgress() (localHeight, targetHeight uint64, state SyncState) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.localHeight, bs.targetHeight, bs.state
}

func (bs *ChainBlockSyncer) syncLoop() {
	defer bs.wg.Done()

	// Wait for peers to connect
	bs.logger.Println("[SYNC] Waiting for peers...")
	for {
		select {
		case <-bs.stopCh:
			return
		case <-time.After(3 * time.Second):
		}

		if bs.pm.GetPeerCount() > 0 {
			break
		}
	}

	bs.logger.Println("[SYNC] Peers connected, starting sync check...")

	// Initial sync
	bs.checkAndSync()

	// Periodic sync check
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bs.stopCh:
			return
		case <-ticker.C:
			bs.checkAndSync()
		}
	}
}

func (bs *ChainBlockSyncer) checkAndSync() {
	if bs.getLocalHeight == nil {
		return
	}

	localHeight := bs.getLocalHeight()
	peerBestHeight := bs.pm.GetBestPeerHeight()

	bs.mu.Lock()
	bs.localHeight = localHeight
	bs.targetHeight = peerBestHeight
	bs.mu.Unlock()

	if peerBestHeight <= localHeight {
		bs.mu.Lock()
		bs.state = SyncStateCaughtUp
		bs.mu.Unlock()
		return
	}

	bs.mu.Lock()
	bs.state = SyncStateSyncing
	bs.mu.Unlock()

	bs.logger.Printf("[SYNC] Behind: local=%d, best_peer=%d, need %d blocks",
		localHeight, peerBestHeight, peerBestHeight-localHeight)

	// Request blocks in batches
	fromHeight := localHeight + 1
	stallCount := 0
	for fromHeight <= peerBestHeight {
		select {
		case <-bs.stopCh:
			return
		default:
		}

		err := bs.pm.RequestBlocksFromBestPeer(fromHeight)
		if err != nil {
			bs.logger.Printf("[SYNC] Request blocks from %d failed: %v", fromHeight, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Wait for blocks to arrive and be processed
		time.Sleep(2 * time.Second)

		newHeight := bs.getLocalHeight()
		if newHeight <= localHeight {
			// No progress, wait longer
			time.Sleep(5 * time.Second)
			newHeight = bs.getLocalHeight()
		}

		if newHeight > localHeight {
			bs.logger.Printf("[SYNC] Progress: %d -> %d", localHeight, newHeight)
			localHeight = newHeight
			fromHeight = newHeight + 1
			stallCount = 0

			bs.mu.Lock()
			bs.localHeight = localHeight
			bs.mu.Unlock()
		} else {
			stallCount++
			bs.logger.Printf("[SYNC] Stalled at height %d (attempt %d)", localHeight, stallCount)
			// v0.11.33 stall recovery: request headers around the stall point
			// from the best peer. If our chain diverged from the network's,
			// handleHeaders detects it and requests the fork block by hash
			// (fork repair); if our chain matches, the requested blocks will
			// arrive normally. After 3 stalls, rotate to a different peer to
			// rule out a serving-bad-blocks peer.
			if hs := bs.pm.GetHeaderProbeSender(); hs != nil {
				from := uint64(1)
				if localHeight > 20 {
					from = localHeight - 20
				}
				hs(GetHeadersMsg{FromHeight: from, MaxHeaders: 500})
			}
			if stallCount >= 3 {
				stallCount = 0
				bs.logger.Printf("[SYNC] Persistent stall — rotating sync peer")
				if err := bs.pm.RequestBlocksFromBestPeer(localHeight + 1); err == nil {
					time.Sleep(5 * time.Second)
				}
			}
			if stallCount >= 5 {
				break
			}
			continue
		}
	}

	bs.mu.Lock()
	if bs.localHeight >= bs.targetHeight {
		bs.state = SyncStateCaughtUp
		bs.logger.Printf("[SYNC] Caught up at height %d", bs.localHeight)
	}
	bs.mu.Unlock()
}
