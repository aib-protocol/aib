package api

import "net/http"

// SetMiningStats plugs the node's live mining snapshot into the API.
func (s *Server) SetMiningStats(fn func() map[string]interface{}) {
	s.mu.Lock()
	s.miningStats = fn
	s.mu.Unlock()
}

func (s *Server) handleMining(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	fn := s.miningStats
	s.mu.RUnlock()
	if fn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "mining stats not available",
		})
		return
	}
	writeJSON(w, http.StatusOK, fn())
}

// SetWalletInfo plugs the node's own address/balance snapshot into the API.
func (s *Server) SetWalletInfo(fn func() map[string]interface{}) {
	s.mu.Lock()
	s.walletInfoFn = fn
	s.mu.Unlock()
}
