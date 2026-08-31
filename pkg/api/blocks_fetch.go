package api

import (
	"encoding/json"
	"errors"
	"net/http"

	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// POST /v1/blocks/fetch {"from":100,"to":200}
//
// On-demand history refetch for pruned (light) nodes: asks every
// connected peer for the block range, validates the returned blocks
// against the local header chain (prevHash linkage), and persists the
// bodies into the local store so subsequent /v1/block/<h> reads work.
func (s *Server) handleBlocksFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	var req struct {
		From uint64 `json:"from"`
		To   uint64 `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON", err.Error())
		return
	}
	if req.To < req.From {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "to < from", "")
		return
	}
	fetcher := s.blocksFetcher
	if fetcher == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "history fetch not configured on this node", "")
		return
	}
	result, err := fetcher(req.From, req.To)
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch_failed", err.Error(), "")
		return
	}
	writeSuccess(w, map[string]interface{}{
		"fetched":  result.Fetched,
		"stored":   result.Stored,
		"missing":  result.Missing,
		"rejected": result.Rejected,
	})
}

// BlocksFetchResult reports a refetch outcome.
type BlocksFetchResult struct {
	Fetched  int      `json:"fetched"`
	Stored   int      `json:"stored"`
	Missing  []uint64 `json:"missing"`
	Rejected int      `json:"rejected"`
}

var _ = errors.New
var _ = utxoPkg.ErrPruned
