package api

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// health check
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	writeSuccess(w, HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Version:   "2.0.0-mvp",
		Uptime:    s.Uptime().String(),
	})
}

// ============================================================================
// nodestatus
// ============================================================================

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	blockHeight := uint64(0)
	if chain := s.GetChain(); chain != nil {
		blockHeight = chain.GetHeight()
	}

	// get P2P network info
	peerCount := 0
	if network := s.GetP2PNetwork(); network != nil {
		peerCount = len(network.GetPeerList())
	}

	writeSuccess(w, map[string]interface{}{
		"node_id":      "aib-node-mvp",
		"version":      "2.0.0-mvp",
		"network":      "testnet",
		"block_height": blockHeight,
		"peers":        peerCount,
		"sync_status":  "synced",
		"block_time":   "60s",
		"uptime":       s.Uptime().String(),
	})
}

// ============================================================================
// balancequery
// ============================================================================

func (s *Server) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	address := parsePathVar(r, "/v1/balance/")
	if address == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing address", "")
		return
	}

	// MVP: return address info (real query pending integration)
	writeSuccess(w, BalanceResponse{
		Address:   address,
		Balance:   0,
		UTXOCount: 0,
	})
}

// ============================================================================
// transaction
// ============================================================================

func (s *Server) handleSubmitTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req TransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	if req.To == "" || req.Amount == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing required fields", "to and amount are required")
		return
	}

	// MVP: generate transaction hash
	txHash := fmt.Sprintf("%x", hex.EncodeToString([]byte(fmt.Sprintf("%s-%s-%d-%d", req.From, req.To, req.Amount, time.Now().UnixNano()))))

	writeSuccess(w, TransactionResponse{
		TxHash:    txHash[:64],
		From:      req.From,
		To:        req.To,
		Amount:    req.Amount,
		Status:    "pending",
		Timestamp: time.Now().UTC(),
	})
}

// ============================================================================
// block
// ============================================================================

func (s *Server) handleGetLatestBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// try to fetch real block data
	chain := s.GetChain()
	if chain != nil {
		block := chain.GetLatestBlock()
		if block != nil {
			header := block.GetHeader()
			proposer := header.GetProposer()
			// strip null bytes
			proposerStr := strings.TrimRight(string(proposer[:]), "\x00")
			if proposerStr == "" {
				proposerStr = "validator"
			}
			writeSuccess(w, BlockResponse{
				Height:    header.GetHeight(),
				Hash:      fmt.Sprintf("%x", block.GetHash()),
				PrevHash:  fmt.Sprintf("%x", header.GetPrevBlockHash()),
				Timestamp: time.Unix(int64(header.GetTimestamp()), 0).UTC(),
				TxCount:   block.GetTransactions(),
				Validator: proposerStr,
			})
			return
		}
	}

	// default return
	writeSuccess(w, BlockResponse{
		Height:    0,
		Hash:      "0000000000000000000000000000000000000000000000000000000000000000",
		PrevHash:  "0000000000000000000000000000000000000000000000000000000000000000",
		Timestamp: time.Now().UTC(),
		TxCount:   0,
		Validator: "genesis",
	})
}

func (s *Server) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	heightStr := parsePathVar(r, "/v1/block/")
	if heightStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing block height", "")
		return
	}

	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid block height", err.Error())
		return
	}

	writeSuccess(w, BlockResponse{
		Height:    height,
		Hash:      "0000000000000000000000000000000000000000000000000000000000000000",
		PrevHash:  "0000000000000000000000000000000000000000000000000000000000000000",
		Timestamp: time.Now().UTC(),
		TxCount:   0,
		Validator: "genesis",
	})
}

// ============================================================================
// P2P nodelist
// ============================================================================

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var peers []PeerResponse
	if network := s.GetP2PNetwork(); network != nil {
		for _, entry := range network.GetPeerList() {
			peers = append(peers, PeerResponse{
				ID:        entry.ID,
				Address:   entry.Address,
				LastSeen:  entry.LastSeen,
				Connected: entry.Connected,
			})
		}
	}

	if peers == nil {
		peers = []PeerResponse{}
	}

	writeSuccess(w, map[string]interface{}{
		"total": len(peers),
		"peers": peers,
	})
}

// ============================================================================
// wallet management
// ============================================================================
