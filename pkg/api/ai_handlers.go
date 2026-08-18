package api

import (
	"net/http"
	"time"
)

// ============================================================================
// AI service API handlers
// ============================================================================

func (s *Server) handleInference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req InferenceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing prompt", "prompt is required")
		return
	}

	// MVP: return a mock response
	writeSuccess(w, InferenceResponse{
		Result:     "MVP inference response",
		ModelID:    req.ModelID,
		ModelName:  "MVP Model",
		Duration:   100,
		TokensUsed: len(req.Prompt) / 4,
		Timestamp:  time.Now().UTC(),
	})
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	writeSuccess(w, ModelListResponse{
		Models: []ModelInfoResponse{
			{
				ModelID:      "mvp-model",
				Name:         "MVP Model",
				Type:         "local",
				Weight:       1.0,
				Available:    true,
				RegisteredAt: time.Now().UTC(),
			},
		},
		Total: 1,
	})
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	writeSuccess(w, AINodeListResponse{
		Nodes: []AINodeInfoResponse{
			{
				NodeID:     "mvp-node-001",
				Address:    "0000000000000000000000000000000000000000000000000000000000000001",
				Stake:      1000000000,
				Models:     []string{"mvp-model"},
				Reputation: 1.0,
				Status:     "active",
				LastSeen:   time.Now().UTC(),
			},
		},
		Total: 1,
	})
}
