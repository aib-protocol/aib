package api

import (
	"fmt"
	"net/http"
	"time"
)

// ============================================================================
// channel API handlers
// ============================================================================

func (s *Server) handleChannelOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req OpenChannelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	if req.PartyA == "" || req.PartyB == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing required fields", "party_a and party_b are required")
		return
	}

	channelID := fmt.Sprintf("ch_%d", time.Now().UnixNano())

	writeSuccess(w, ChannelResponse{
		ID:        channelID,
		PartyA:    req.PartyA,
		PartyB:    req.PartyB,
		BalanceA:  req.DepositA,
		BalanceB:  req.DepositB,
		Sequence:  0,
		Status:    "open",
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Server) handleChannelClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req CloseChannelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"channel_id": req.ChannelID,
		"status":     "closing",
		"message":    "Channel close initiated",
	})
}

func (s *Server) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	channelID := parsePathVar(r, "/v1/channel/")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing channel ID", "")
		return
	}

	writeSuccess(w, ChannelResponse{
		ID:       channelID,
		Status:   "open",
		Sequence: 0,
	})
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	writeSuccess(w, ChannelListResponse{
		Channels: []ChannelResponse{},
		Total:    0,
	})
}

func (s *Server) handleChannelPay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req PaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	if req.ChannelID == "" || req.Amount == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing required fields", "channel_id and amount are required")
		return
	}

	writeSuccess(w, map[string]interface{}{
		"channel_id": req.ChannelID,
		"amount":     req.Amount,
		"status":     "completed",
		"timestamp":  time.Now().UTC(),
	})
}
