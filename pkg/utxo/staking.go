package utxo

import (
	"fmt"
	"sort"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// True staking model (RFC: pure-stake, no legacy-finance baggage).
//
// A stake is a special UTXO created by a STAKE transaction. The script
// marks it as locked stake. Weight in proposer selection = the value of
// all live stake UTXOs owned by an address — derived purely from the
// chain, identical on every node.
//
// No special genesis stake, no delegation, no slashing-via-committee:
// the only way to gain weight is to lock real AIB.

// StakeScriptTag is written into TXOutput.Script[0] to mark stake outputs.
const StakeScriptTag byte = 0xA1

// UnstakeCooldownBlocks is how long an unstake takes to become spendable.
const UnstakeCooldownBlocks = 500

// IsStakeOutput reports whether a UTXO is stake-locked.
func IsStakeOutput(u *UTXO) bool {
	return len(u.Script) > 0 && u.Script[0] == StakeScriptTag
}

// IsStakeOutputTx is the TXOutput variant.
func IsStakeOutputTx(o TXOutput) bool {
	return len(o.Script) > 0 && o.Script[0] == StakeScriptTag
}

// StakeEntry is one live stake, derived from a stake UTXO.
type StakeEntry struct {
	StakeTxHash [32]byte
	Index       uint32
	Owner       interfaces.Address
	Value       uint64 // raw units (1 AIB = 1e8)
	LockedAt    uint64 // height when staked
}

// BuildStakeIndex walks all UTXOs and collects live stakes.
// Deterministic: derived from chain state only.
func BuildStakeIndex(all []*UTXO) []StakeEntry {
	var out []StakeEntry
	for _, u := range all {
		if IsStakeOutput(u) {
			out = append(out, StakeEntry{
				StakeTxHash: u.TxHash,
				Index:       u.Index,
				Owner:       u.Address,
				Value:       u.Value,
			})
		}
	}
	return out
}

// StakeWeights reduces stake entries to per-address weights, sorted by
// address for deterministic iteration.
func StakeWeights(entries []StakeEntry) ([]interfaces.Address, []uint64) {
	m := map[interfaces.Address]uint64{}
	for _, e := range entries {
		m[e.Owner] += e.Value
	}
	// collect + sort deterministically (by raw bytes)
	addrs := make([]interfaces.Address, 0, len(m))
	for a := range m {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytesLess(addrs[i], addrs[j])
	})
	ws := make([]uint64, len(addrs))
	for i, a := range addrs {
		ws[i] = m[a]
	}
	return addrs, ws
}

// ValidateStakeTx validates a STAKE transaction:
//   - inputs total >= outputs total (fee = difference, standard)
//   - exactly one stake output; its address must equal the input signer
//     (you can only stake your own coins — no staking on behalf)
func ValidateStakeTx(tx *Transaction, inputUTXOs []*UTXO) error {
	stakeOuts := 0
	for _, o := range tx.Outputs {
		if IsStakeOutputTx(o) {
			stakeOuts++
		}
	}
	if stakeOuts != 1 {
		return fmt.Errorf("stake tx must have exactly 1 stake output, got %d", stakeOuts)
	}
	var in, out uint64
	for _, u := range inputUTXOs {
		in += u.Value
	}
	for _, o := range tx.Outputs {
		out += o.Value
	}
	if out > in {
		return fmt.Errorf("stake tx outputs (%d) exceed inputs (%d)", out, in)
	}
	return nil
}

// ValidateUnstakeTx validates an UNSTAKE transaction:
//   - consumes at least one stake UTXO and produces ordinary outputs only
//   - must be at least UnstakeCooldownBlocks after the stake was created
func ValidateUnstakeTx(tx *Transaction, inputUTXOs []*UTXO, currentHeight uint64, stakeCreatedHeight map[[32]byte]uint64) error {
	consumedStake := 0
	for _, u := range inputUTXOs {
		if IsStakeOutput(u) {
			consumedStake++
			created := stakeCreatedHeight[u.TxHash]
			if currentHeight < created+UnstakeCooldownBlocks {
				return fmt.Errorf("unstake too early: stake created at %d, cooldown ends at %d, now %d",
					created, created+UnstakeCooldownBlocks, currentHeight)
			}
		}
	}
	if consumedStake == 0 {
		return fmt.Errorf("unstake tx must consume a stake UTXO")
	}
	for _, o := range tx.Outputs {
		if IsStakeOutputTx(o) {
			return fmt.Errorf("unstake tx cannot create new stake outputs")
		}
	}
	var in, out uint64
	for _, u := range inputUTXOs {
		in += u.Value
	}
	for _, o := range tx.Outputs {
		out += o.Value
	}
	if out > in {
		return fmt.Errorf("unstake tx outputs (%d) exceed inputs (%d)", out, in)
	}
	return nil
}

func bytesLess(a, b interfaces.Address) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
