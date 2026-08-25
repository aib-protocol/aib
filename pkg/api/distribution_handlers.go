package api

import (
	"encoding/hex"
	"net/http"
	"sort"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// GET /v1/distribution — AIB distribution across all addresses (pie chart data)
func (s *Server) handleDistribution(w http.ResponseWriter, r *http.Request) {
	allStore, ok := s.utxoStore.(interface {
		GetAllUTXOsAll() []*utxo.UTXO
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}
	all := allStore.GetAllUTXOsAll()
	liquid := map[[32]byte]uint64{}
	staked := map[[32]byte]uint64{}
	for _, u := range all {
		if utxo.IsStakeOutput(u) {
			staked[u.Address] += u.Value
		} else {
			liquid[u.Address] += u.Value
		}
	}
	type entry struct {
		Address  string  `json:"address"`
		Liquid   float64 `json:"liquid_aib"`
		Staked   float64 `json:"staked_aib"`
		Total    float64 `json:"total_aib"`
	}
	out := make([]entry, 0, len(liquid)+len(staked))
	seen := map[[32]byte]bool{}
	for a, v := range liquid {
		out = append(out, entry{hex.EncodeToString(a[:]), float64(v) / 1e8, float64(staked[a]) / 1e8, float64(v+staked[a]) / 1e8})
		seen[a] = true
	}
	for a, v := range staked {
		if !seen[a] {
			out = append(out, entry{hex.EncodeToString(a[:]), 0, float64(v) / 1e8, float64(v) / 1e8})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	var total float64
	for _, e := range out {
		total += e.Total
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"total_supply_aib": total,
			"holders":          out,
		},
	})
}

// GET /v1/validators — current PoS validator set (who is mining, weight, blocks produced)
func (s *Server) handleStakeValidators(w http.ResponseWriter, r *http.Request) {
	allStore, ok := s.utxoStore.(interface {
		GetAllUTXOsAll() []*utxo.UTXO
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}
	all := allStore.GetAllUTXOsAll()
	entries := utxo.BuildStakeIndex(all)
	addrs, weights := utxo.StakeWeights(entries)

	// count blocks produced per proposer in the PoS era from the API's chain reader
	blocksByProposer := map[string]uint64{}
	if s.chain != nil {
		best, err := s.chain.GetBestBlockHeight()
		if err == nil {
			start := uint64(1)
			if best > utxo.PoWEraBlocks+50 {
				// sample the last 50 PoS blocks for responsiveness
				start = best - 50
			}
			if start <= utxo.PoWEraBlocks {
				start = utxo.PoWEraBlocks + 1
			}
			for h := start; h <= best; h++ {
				b, err := s.chain.GetBlockByHeight(h)
				if err != nil || b == nil {
					continue
				}
				blocksByProposer[hex.EncodeToString(b.Header.Proposer[:])]++
			}
		}
	}

	type v struct {
		Address      string  `json:"address"`
		StakedAIB    float64 `json:"staked_aib"`
		WeightPct    float64 `json:"weight_pct"`
		BlocksSample uint64  `json:"blocks_produced_sample"`
	}
	var totalStake uint64
	for _, w := range weights {
		totalStake += w
	}
	out := make([]v, 0, len(addrs))
	for i, a := range addrs {
		ah := hex.EncodeToString(a[:])
		pct := 0.0
		if totalStake > 0 {
			pct = float64(weights[i]) / float64(totalStake) * 100
		}
		out = append(out, v{ah, float64(weights[i]) / 1e8, pct, blocksByProposer[ah]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StakedAIB > out[j].StakedAIB })
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"total_staked_aib": float64(totalStake) / 1e8,
			"validators":       out,
		},
	})
}
