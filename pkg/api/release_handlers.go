package api

import (
	"net/http"
)

// releaseLatestFn returns the newest on-chain release anchor (or nil).
type releaseProvider interface{}

// handleReleaseLatest returns the most recent release anchor recorded on
// chain (name + artifact SHA256 + height + tx hash).
func (s *Server) handleReleaseLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	if s.releaseLatestFn == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "release index not wired", "")
		return
	}
	rec := s.releaseLatestFn()
	if rec == nil {
		writeError(w, http.StatusNotFound, "not_found", "no release anchor on chain yet", "")
		return
	}
	writeSuccess(w, rec)
}

// handleReleaseHistory returns all release anchors in chain order.
func (s *Server) handleReleaseHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	if s.releaseHistoryFn == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "release index not wired", "")
		return
	}
	writeSuccess(w, map[string]interface{}{"releases": s.releaseHistoryFn()})
}
