package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"

	p2p "github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Finality gadget (v0.11.33) — BFT-style vote accounting on top of VRF PoS.
//
// Validators gossip FinalityVoteMsg for the tip they have validated. Votes
// are weighted by the voter's stake. When votes for a height reach 2/3 of
// active validator stake, the height is FINAL: reorgs that would roll the
// chain back below it are refused. Non-invasive: no block format change.
// ============================================================================

type FinalityTracker struct {
	mu sync.Mutex

	// votes[hash] -> set of voters (stake address) with cumulative stake
	votes     map[string]map[string]uint64 // blockHash -> voter -> stake(sats)
	voteHt    map[string]uint64            // blockHash -> height
	finalized uint64                       // highest finalized height
	finalHash string                       // hash at finalized height

	consensus  *utxoPkg.ConsensusState
	chainState *utxoPkg.ChainState
	pm         *p2p.ChainPeerManager
	privKey    ed25519.PrivateKey
	stakeAddr  [32]byte
	logger     func(string, ...interface{})
}

func NewFinalityTracker(consensus *utxoPkg.ConsensusState, chainState *utxoPkg.ChainState,
	pm *p2p.ChainPeerManager, privKey ed25519.PrivateKey, stakeAddr [32]byte,
	logger func(string, ...interface{})) *FinalityTracker {
	return &FinalityTracker{
		votes:      make(map[string]map[string]uint64),
		voteHt:     make(map[string]uint64),
		consensus:  consensus,
		chainState: chainState,
		pm:         pm,
		privKey:    privKey,
		stakeAddr:  stakeAddr,
		logger:     logger,
	}
}

// FinalitySigPayload is the canonical bytes signed by a finality vote.
func FinalitySigPayload(height uint64, hashHex string) []byte {
	return []byte(fmt.Sprintf("AIB-FINALITY:%d:%s", height, hashHex))
}

// VoteFor broadcasts our vote for the current tip.
func (ft *FinalityTracker) VoteFor(height uint64, hashHex string) {
	if ft.privKey == nil || ft.pm == nil {
		return
	}
	pub := ft.privKey.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(ft.privKey, FinalitySigPayload(height, hashHex))
	vote := p2p.FinalityVoteMsg{
		Height: height,
		Hash:   hashHex,
		Voter:  hex.EncodeToString(ft.stakeAddr[:]),
		PubKey: hex.EncodeToString(pub),
		Sig:    hex.EncodeToString(sig),
	}
	ft.pm.BroadcastFinalityVote(vote)
	ft.HandleVote(vote) // count our own
}

// HandleVote verifies + tallies a gossiped vote.
func (ft *FinalityTracker) HandleVote(v p2p.FinalityVoteMsg) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	// verify signature
	pub, err := hex.DecodeString(v.PubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return
	}
	sig, err := hex.DecodeString(v.Sig)
	if err != nil {
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), FinalitySigPayload(v.Height, v.Hash), sig) {
		ft.logger("[FINALITY] invalid vote signature from %s", v.Voter[:min(12, len(v.Voter))])
		return
	}
	// voter address must equal sha256(pubkey)
	vsum := sha256Sum(pub)
	if v.Voter != hex.EncodeToString(vsum[:]) {
		return
	}
	// voter must hold stake
	stake := ft.stakeOf(v.Voter)
	if stake == 0 {
		return
	}

	if ft.votes[v.Hash] == nil {
		ft.votes[v.Hash] = make(map[string]uint64)
	}
	if _, dup := ft.votes[v.Hash][v.Voter]; dup {
		return // one vote per validator per block
	}
	ft.votes[v.Hash][v.Voter] = stake
	ft.voteHt[v.Hash] = v.Height

	// tally
	var got uint64
	for _, s := range ft.votes[v.Hash] {
		got += s
	}
	total := ft.totalStake()
	if total == 0 {
		return
	}
	if got*3 >= total*2 && v.Height > ft.finalized {
		ft.finalized = v.Height
		ft.finalHash = v.Hash
		ft.logger("[FINALITY] ⚓ height %d finalized (%d/%d stake voted)", v.Height, got, total)
	}
}

// IsFinalized reports whether rolling back below `height` is forbidden.
func (ft *FinalityTracker) IsFinalized(height uint64) bool {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return height <= ft.finalized && ft.finalized > 0
}

// FinalizedHeight returns the highest finalized height.
func (ft *FinalityTracker) FinalizedHeight() uint64 {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.finalized
}

func (ft *FinalityTracker) stakeOf(addrHex string) uint64 {
	raw, err := hex.DecodeString(addrHex)
	if err != nil || len(raw) != 32 {
		return 0
	}
	for _, v := range ft.consensus.GetActiveValidators() {
		if v.Address == *(*[32]byte)(raw) {
			return v.Stake
		}
	}
	return 0
}

func (ft *FinalityTracker) totalStake() uint64 {
	var t uint64
	for _, v := range ft.consensus.GetActiveValidators() {
		t += v.Stake
	}
	return t
}

func sha256Sum(b []byte) [32]byte {
	return sha256.Sum256(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StartVoteLoop periodically votes for our current tip (validators only).
func (ft *FinalityTracker) StartVoteLoop(isValidator func() bool) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !isValidator() {
				continue
			}
			h := ft.chainState.GetBestBlockHeight()
			b, err := ft.chainState.GetBlockByHeight(h)
			if err != nil || b == nil {
				continue
			}
			ft.VoteFor(h, hex.EncodeToString(b.Hash[:]))
		}
	}()
}

// RejectsIfFinalized returns an error if a reorg below finality is attempted.
func (ft *FinalityTracker) RejectsIfFinalized(newTipHeight uint64) error {
	if ft.IsFinalized(newTipHeight + 1) {
		return fmt.Errorf("reorg below finalized height %d refused", ft.FinalizedHeight())
	}
	return nil
}

var _ = strings.TrimSpace // keep strings import if unused paths change

// CanonicalGuard refuses an incoming block whose height is at or below the
// finalized height but whose hash differs from our canonical chain's block
// at that height (alternative history below finality is forbidden).
func (ft *FinalityTracker) CanonicalGuard(height uint64, blockHash [32]byte) error {
	ft.mu.Lock()
	fin := ft.finalized
	ft.mu.Unlock()
	if fin == 0 || height > fin {
		return nil
	}
	canonical, err := ft.chainState.GetBlockByHeight(height)
	if err != nil || canonical == nil {
		return nil // can't judge without canonical; other validation will handle
	}
	if canonical.Hash != blockHash {
		return fmt.Errorf("block h%d conflicts with finalized chain (have %x, got %x)",
			height, canonical.Hash[:8], blockHash[:8])
	}
	return nil
}
