package main

// mining_stats.go — live mining/miner observability for the fee-burn testnet.
// Tracks VRF sortition wins/misses, coinbase earnings, and epoch fee-burn
// settlements, exposed via /v1/mining so a fresh install shows "am I mining,
// am I winning blocks, what did I earn" without any extra tooling.

import (
	"sync"
	"time"
)

// MiningStats is goroutine-safe.
type MiningStats struct {
	mu sync.Mutex

	StartedAt time.Time

	// Sortition
	SlotsWon      uint64 // we were selected and produced
	SlotsMissed   uint64 // someone else won
	LastWinHeight uint64
	LastWinAt     time.Time
	LastWinner    string // who won the last slot we observed

	// Earnings (satoshi)
	CoinbaseEarned uint64 // bootstrap-window coinbase total
	FeePayoutTotal uint64 // epoch fee settlements paid to us
	BurnedTotal    uint64 // fees burned network-wide (observed by us)

	// Epoch economics (last settlement snapshot)
	LastEpochFees   uint64
	LastEpochPayout uint64
	LastEpochBurn   uint64
	LastEpochStake  uint64
	LastEpochAPR    string // realized APR as ratio string

	// peers seen through sortition (participation)
	DistinctWinners map[string]bool
}

func NewMiningStats() *MiningStats {
	return &MiningStats{StartedAt: time.Now(), DistinctWinners: map[string]bool{}}
}

func (m *MiningStats) recordWin(height uint64, stakes map[[32]byte]uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SlotsWon++
	m.LastWinHeight = height
	m.LastWinAt = time.Now()
	m.CoinbaseEarned += 1 * 1e8 // bootstrap window: 1 AIB/block
	for addr := range stakes {
		m.DistinctWinners[hexAddr(addr)] = true
	}
}

func (m *MiningStats) recordMiss(height uint64, winner [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SlotsMissed++
	m.LastWinner = hexAddr(winner)
	m.DistinctWinners[hexAddr(winner)] = true
}

func (m *MiningStats) recordEpochSettlement(fees, payout, burn, stake uint64, apr string, paidToUs uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastEpochFees = fees
	m.LastEpochPayout = payout
	m.LastEpochBurn = burn
	m.LastEpochStake = stake
	m.LastEpochAPR = apr
	m.BurnedTotal += burn
	m.FeePayoutTotal += paidToUs
}

// Snapshot returns a plain copy for JSON.
func (m *MiningStats) Snapshot() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	uptime := time.Since(m.StartedAt).Round(time.Second).String()
	var winRate float64
	total := m.SlotsWon + m.SlotsMissed
	if total > 0 {
		winRate = float64(m.SlotsWon) / float64(total)
	}
	return map[string]interface{}{
		"mining":              true,
		"uptime":              uptime,
		"slots_won":           m.SlotsWon,
		"slots_missed":        m.SlotsMissed,
		"win_rate":            winRate,
		"last_win_height":     m.LastWinHeight,
		"last_win_at":         m.LastWinAt,
		"last_winner":         m.LastWinner,
		"distinct_winners":    len(m.DistinctWinners),
		"coinbase_earned_aib": float64(m.CoinbaseEarned) / 1e8,
		"fee_payout_aib":      float64(m.FeePayoutTotal) / 1e8,
		"burned_total_aib":    float64(m.BurnedTotal) / 1e8,
		"epoch": map[string]interface{}{
			"fees_aib":     float64(m.LastEpochFees) / 1e8,
			"payout_aib":   float64(m.LastEpochPayout) / 1e8,
			"burn_aib":     float64(m.LastEpochBurn) / 1e8,
			"stake_aib":    float64(m.LastEpochStake) / 1e8,
			"realized_apr": m.LastEpochAPR,
		},
	}
}

func hexAddr(a [32]byte) string {
	const hex = "0123456789abcdef"
	buf := make([]byte, 16) // first 8 bytes is plenty for display
	for i := 0; i < 8; i++ {
		buf[i*2] = hex[a[i]>>4]
		buf[i*2+1] = hex[a[i]&0xf]
	}
	return string(buf)
}
