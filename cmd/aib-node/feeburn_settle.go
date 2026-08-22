package main

// feeburn_settle.go — epoch fee-burn settlement wired into block production.

import (
	"fmt"
	"math/big"

	economy "github.com/aib-protocol/aib/pkg/economy"
	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// EpochLengthBlocks: 1 day at 30s blocks (testnet) = 2880 blocks.
// (RFC-002 decision: Epoch = 1 day.)
const EpochLengthBlocks = 2880

// epochFeeAccumulator collects per-tx fees between settlements.
type epochFeeAccumulator struct {
	feesSat uint64
}

// settleEpochFees runs at every epoch boundary height.
// Testnet simplification: the node operator is the only staker, so the
// payout goes to the node address; burn side is computed identically to
// the multi-staker formula (pkg/economy.BurnSplit) so the economics being
// experienced is the real one.
func (n *Node) settleEpochFees(newHeight uint64) {
	if newHeight%EpochLengthBlocks != 0 {
		return
	}
	fees := n.epochFees.take()
	if fees == 0 {
		return
	}

	epochSeconds := uint64(EpochLengthBlocks * 30)
	stake := n.consensus.GetTotalStake()
	if stake == 0 {
		stake = 1 // avoid div-zero in display math
	}
	supply := utxoPkg.TotalSupply * 1e8

	pay, burn, apr, err := economy.BurnSplit(fees, stake, supply, epochSeconds)
	if err != nil {
		n.logger.Printf("[Epoch @%d] settlement skipped: %v", newHeight, err)
		return
	}
	aprStr := "n/a"
	if apr != nil {
		f64, _ := new(big.Float).SetRat(apr).Float64()
		aprStr = fmt.Sprintf("%.4f%%", f64*100)
	}
	n.miningStats.recordEpochSettlement(fees, pay, burn, stake, aprStr, pay)
	n.logger.Printf("[Epoch @%d] fees=%d pay=%d burn=%d stake=%d realized APR=%s",
		newHeight, fees, pay, burn, stake, aprStr)
}

func (a *epochFeeAccumulator) add(feeSatoshi uint64) {
	a.feesSat += feeSatoshi
}

func (a *epochFeeAccumulator) take() uint64 {
	f := a.feesSat
	a.feesSat = 0
	return f
}
