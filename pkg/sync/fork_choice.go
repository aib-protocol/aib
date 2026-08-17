// Package sync provides block synchronization and propagation functionality.
// This file implements fork choice logic and chain reorganization.
package sync

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ============================================================================
// Fork Choice Rule
// ============================================================================

// ForkSelector implements the fork choice rule for selecting the best chain.
type ForkSelector struct {
	mu         sync.RWMutex
	localChain Blockchain
	running    bool

	// Fork detection
	reorgThreshold int // blocks behind before considering reorg
}

// NewForkSelector creates a new ForkSelector.
func NewForkSelector(localChain Blockchain, reorgThreshold int) *ForkSelector {
	if reorgThreshold <= 0 {
		reorgThreshold = 6 // Default: 6 blocks confirmation
	}

	return &ForkSelector{
		localChain:     localChain,
		reorgThreshold: reorgThreshold,
	}
}

// SelectChain selects the best chain from multiple competing chains.
// Implements the longest chain rule.
func (fs *ForkSelector) SelectChain(chains [][]*Block) []*Block {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(chains) == 0 {
		return nil
	}

	if len(chains) == 1 {
		return chains[0]
	}

	// Sort chains by length (descending)
	sort.Slice(chains, func(i, j int) bool {
		return len(chains[i]) > len(chains[j])
	})

	// Check if the longest chain is significantly longer
	longest := chains[0]
	secondLongest := chains[1]

	diff := len(longest) - len(secondLongest)

	if diff >= fs.reorgThreshold {
		// Longest chain is significantly longer
		return longest
	}

	// Chains are similar length - use total difficulty or timestamp
	// For simplicity, use the chain with the later timestamp
	return selectChainByTimestamp(chains)
}

// HandleReorg handles a chain reorganization.
// Returns the blocks to be added and the blocks to be removed.
func (fs *ForkSelector) HandleReorg(newChain []*Block, oldChain []*Block) ([]*Block, []*Block, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(newChain) == 0 {
		return nil, nil, errors.New("new chain is empty")
	}

	if len(oldChain) == 0 {
		return newChain, nil, nil
	}

	// Find the common ancestor
	splitPoint := fs.findSplitPoint(newChain, oldChain)

	if splitPoint == -1 {
		return nil, nil, errors.New("chains don't have a common ancestor")
	}

	// Blocks to remove (from split point+1 to end of old chain)
	toRemove := make([]*Block, 0)
	for i := splitPoint + 1; i < len(oldChain); i++ {
		toRemove = append(toRemove, oldChain[i])
	}

	// Blocks to add (from split point+1 to end of new chain)
	toAdd := make([]*Block, 0)
	for i := splitPoint + 1; i < len(newChain); i++ {
		toAdd = append(toAdd, newChain[i])
	}

	return toAdd, toRemove, nil
}

// FindFork finds the fork point between the local chain and a new block.
func (fs *ForkSelector) FindFork(newBlock *Block) (uint64, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Traverse back from the new block until we find a common ancestor
	current := newBlock
	for {
		// Check if this height exists in local chain
		localBlock, err := fs.localChain.GetBlock(current.Height)
		if err == nil {
			// Found block at same height - check if it's the same
			if bytes.Equal(localBlock.Hash(), current.Hash()) {
				// Found common ancestor
				return current.Height, nil
			}
		}

		// Move to previous block
		if current.Height == 0 {
			return 0, errors.New("no common ancestor found")
		}

		// In a real implementation, we would fetch the previous block
		// For now, just move back one height
		prevLocal, err := fs.localChain.GetBlock(current.Height - 1)
		if err != nil {
			return 0, fmt.Errorf("failed to get previous block: %w", err)
		}
		current = prevLocal
	}
}

// ValidateChain validates a chain of blocks.
func (fs *ForkSelector) ValidateChain(blocks []*Block) error {
	if len(blocks) == 0 {
		return errors.New("empty chain")
	}

	// Verify genesis
	if blocks[0].Height != 0 {
		return errors.New("invalid genesis block height")
	}

	if !bytes.Equal(blocks[0].PreviousBlockHash(), make([]byte, 32)) {
		return errors.New("invalid genesis block previous hash")
	}

	// Verify chain linkage
	for i := 1; i < len(blocks); i++ {
		prev := blocks[i-1]
		curr := blocks[i]

		// Verify height
		if curr.Height != prev.Height+1 {
			return fmt.Errorf("height gap at block %d: expected %d, got %d",
				i, prev.Height+1, curr.Height)
		}

		// Verify previous hash
		if !bytes.Equal(curr.PreviousBlockHash(), prev.Hash()) {
			return fmt.Errorf("previous hash mismatch at block %d", i)
		}

		// Verify block hash
		expectedHash := curr.calculateHash()
		if !bytes.Equal(curr.Hash(), expectedHash) {
			return fmt.Errorf("hash mismatch at block %d", i)
		}

		// Verify timestamp (should be after previous block)
		if curr.Timestamp <= prev.Timestamp {
			return fmt.Errorf("timestamp not increasing at block %d", i)
		}
	}

	return nil
}

// IsBetterChain checks if a candidate chain is better than the current chain.
func (fs *ForkSelector) IsBetterChain(candidate []*Block) (bool, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if len(candidate) == 0 {
		return false, errors.New("empty candidate chain")
	}

	// Get current chain height
	currentHeight := fs.localChain.GetBlockCount()

	candidateHeight := candidate[len(candidate)-1].Height

	// Candidate must be valid
	if err := fs.ValidateChain(candidate); err != nil {
		return false, fmt.Errorf("invalid candidate chain: %w", err)
	}

	// Check if candidate is longer
	if candidateHeight > currentHeight {
		// Also verify it connects properly at the fork point
		forkHeight, err := fs.FindFork(candidate[len(candidate)-1])
		if err != nil {
			return false, err
		}

		// Verify candidate has proper connection at fork
		if forkHeight < uint64(len(candidate)) {
			forkBlock := candidate[forkHeight]
			localBlock, err := fs.localChain.GetBlock(forkHeight)
			if err == nil {
				if !bytes.Equal(forkBlock.Hash(), localBlock.Hash()) {
					return false, errors.New("candidate doesn't match at fork point")
				}
			}
		}

		return true, nil
	}

	// Same height but different - use tiebreaker
	if candidateHeight == currentHeight {
		// Compare hash (lexicographically larger wins in tie-breaker)
		candidateHash := candidate[len(candidate)-1].Hash()
		latestBlock, err := fs.localChain.GetLatestBlock()
		if err == nil {
			localHash := latestBlock.Hash()
			if bytes.Compare(candidateHash, localHash) > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

// GetReorgBlocks returns the blocks that would be affected by a reorg.
func (fs *ForkSelector) GetReorgBlocks(newTip *Block) ([]*Block, []*Block, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Find fork point
	forkHeight, err := fs.FindFork(newTip)
	if err != nil {
		return nil, nil, err
	}

	// Blocks to remove (from forkHeight+1 to current tip)
	toRemove := make([]*Block, 0)
	currentTip, err := fs.localChain.GetLatestBlock()
	if err == nil {
		for h := forkHeight + 1; h <= currentTip.Height; h++ {
			block, err := fs.localChain.GetBlock(h)
			if err != nil {
				continue
			}
			toRemove = append(toRemove, block)
		}
	}

	// Blocks to add would be fetched from the peer providing newTip
	// For now, return nil for toAdd
	return nil, toRemove, nil
}

// ============================================================================
// Private Methods
// ============================================================================

func (fs *ForkSelector) findSplitPoint(chain1, chain2 []*Block) int {
	minLen := len(chain1)
	if len(chain2) < minLen {
		minLen = len(chain2)
	}

	// Find the first block that doesn't match
	for i := 0; i < minLen; i++ {
		if !bytes.Equal(chain1[i].Hash(), chain2[i].Hash()) {
			return i - 1
		}
	}

	return minLen - 1
}

// ============================================================================
// Helper Functions
// ============================================================================

// selectChainByTimestamp selects the chain with the most recent work.
// This is a simplified version - in production, use accumulated difficulty.
func selectChainByTimestamp(chains [][]*Block) []*Block {
	if len(chains) == 0 {
		return nil
	}

	bestChain := chains[0]
	bestTimestamp := int64(0)

	for _, chain := range chains {
		if len(chain) == 0 {
			continue
		}

		timestamp := chain[len(chain)-1].Timestamp
		if timestamp > bestTimestamp {
			bestTimestamp = timestamp
			bestChain = chain
		}
	}

	return bestChain
}

// ============================================================================
// Reorg Handler
// ============================================================================

// ReorgHandler handles chain reorganizations.
type ReorgHandler struct {
	mu           sync.RWMutex
	localChain   Blockchain
	onReorgStart func()
	onReorgEnd   func(added, removed int)
	onReorgFail  func(error)
}

// NewReorgHandler creates a new ReorgHandler.
func NewReorgHandler(localChain Blockchain) *ReorgHandler {
	return &ReorgHandler{
		localChain: localChain,
	}
}

// SetOnReorgStart sets the callback called when reorg starts.
func (rh *ReorgHandler) SetOnReorgStart(fn func()) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.onReorgStart = fn
}

// SetOnReorgEnd sets the callback called when reorg completes.
func (rh *ReorgHandler) SetOnReorgEnd(fn func(added, removed int)) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.onReorgEnd = fn
}

// SetOnReorgFail sets the callback called when reorg fails.
func (rh *ReorgHandler) SetOnReorgFail(fn func(error)) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.onReorgFail = fn
}

// ProcessReorg processes a chain reorganization.
func (rh *ReorgHandler) ProcessReorg(newChain []*Block, oldChain []*Block) error {
	rh.mu.Lock()
	defer rh.mu.Unlock()

	// Notify start
	if rh.onReorgStart != nil {
		rh.onReorgStart()
	}

	// Create fork selector
	fs := NewForkSelector(rh.localChain, 6)

	// Get blocks to add and remove
	toAdd, toRemove, err := fs.HandleReorg(newChain, oldChain)
	if err != nil {
		if rh.onReorgFail != nil {
			rh.onReorgFail(err)
		}
		return fmt.Errorf("failed to calculate reorg: %w", err)
	}

	// Remove blocks (in reverse order)
	for i := len(toRemove) - 1; i >= 0; i-- {
		// In a real implementation, we would call RemoveBlock
		// For now, just track it
		_ = toRemove[i]
	}

	// Add new blocks
	for _, block := range toAdd {
		if err := rh.localChain.AddBlock(block); err != nil {
			if rh.onReorgFail != nil {
				rh.onReorgFail(err)
			}
			return fmt.Errorf("failed to add block during reorg: %w", err)
		}
	}

	// Notify completion
	if rh.onReorgEnd != nil {
		rh.onReorgEnd(len(toAdd), len(toRemove))
	}

	return nil
}

// ============================================================================
// Chain Utilities
// ============================================================================

// ChainInfo contains information about a chain.
type ChainInfo struct {
	Height       uint64 `json:"height"`
	TipHash      string `json:"tip_hash"`
	Work         string `json:"work"` // Hex representation of total work
	IsValid      bool   `json:"is_valid"`
	ForkDetected bool   `json:"fork_detected"`
}

// GetChainInfo returns information about a chain.
func GetChainInfo(blocks []*Block) ChainInfo {
	if len(blocks) == 0 {
		return ChainInfo{}
	}

	tip := blocks[len(blocks)-1]

	return ChainInfo{
		Height:       tip.Height,
		TipHash:      hex.EncodeToString(tip.Hash()),
		Work:         calculateChainWork(blocks),
		IsValid:      true, // Assume valid if we can read it
		ForkDetected: false,
	}
}

// calculateChainWork calculates the total work of a chain.
func calculateChainWork(blocks []*Block) string {
	// Simplified - just sum heights
	totalWork := uint64(0)
	for _, block := range blocks {
		totalWork += block.Height + 1
	}
	return fmt.Sprintf("%016x", totalWork)
}

// CompareChains compares two chains and returns which one is better.
// Returns 1 if chain1 is better, -1 if chain2 is better, 0 if equal.
func CompareChains(chain1, chain2 []*Block) int {
	if len(chain1) == 0 && len(chain2) == 0 {
		return 0
	}
	if len(chain1) == 0 {
		return -1
	}
	if len(chain2) == 0 {
		return 1
	}

	// Compare by length
	if len(chain1) > len(chain2) {
		return 1
	}
	if len(chain1) < len(chain2) {
		return -1
	}

	// Same length - compare by work (simplified: compare tip hash)
	tip1 := chain1[len(chain1)-1]
	tip2 := chain2[len(chain2)-1]

	return bytes.Compare(tip1.Hash(), tip2.Hash())
}

// FindCommonAncestor finds the common ancestor of two chains.
func FindCommonAncestor(chain1, chain2 []*Block) *Block {
	minLen := len(chain1)
	if len(chain2) < minLen {
		minLen = len(chain2)
	}

	for i := 0; i < minLen; i++ {
		if !bytes.Equal(chain1[i].Hash(), chain2[i].Hash()) {
			if i == 0 {
				return nil // No common ancestor
			}
			return chain1[i-1]
		}
	}

	if minLen > 0 {
		return chain1[minLen-1]
	}

	return nil
}
