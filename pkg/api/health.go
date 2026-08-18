package api

import (
	"encoding/hex"
	"net/http"
	"runtime"
	"time"
)

// ============================================================================
// 增强的健康检查
// ============================================================================

// DetailedHealthResponse 详细的健康检查响应
type DetailedHealthResponse struct {
	Status     string           `json:"status"`
	Timestamp  time.Time        `json:"timestamp"`
	Version    string           `json:"version"`
	Uptime     string           `json:"uptime"`
	Blockchain BlockchainHealth `json:"blockchain"`
	System     SystemHealth     `json:"system"`
	API        APIHealth        `json:"api"`
}

// BlockchainHealth 区块链健康状态
type BlockchainHealth struct {
	Status        string `json:"status"` // "synced", "syncing", "offline"
	ChainID       string `json:"chain_id"`
	Height        uint64 `json:"height"`
	BestBlockHash string `json:"best_block_hash"`
	GenesisHash   string `json:"genesis_hash"`
	Peers         int    `json:"peers"`
	SyncStatus    string `json:"sync_status"`
}

// SystemHealth 系统健康状态
type SystemHealth struct {
	GoVersion       string `json:"go_version"`
	NumGoroutines   int    `json:"num_goroutines"`
	NumCPU          int    `json:"num_cpu"`
	AllocatedMemory uint64 `json:"allocated_memory"`
}

// APIHealth API 健康状态
type APIHealth struct {
	Status          string `json:"status"`
	Uptime          string `json:"uptime"`
	RequestsHandled uint64 `json:"requests_handled"`
}

// handleHealthDetailed 处理详细的健康检查请求
func (s *Server) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// 获取链状态
	var chainHealth = BlockchainHealth{
		Status:     "offline",
		ChainID:    s.GetChainID(),
		Height:     0,
		SyncStatus: "unknown",
	}

	if chain := s.GetChain(); chain != nil {
		chainHealth.Status = "synced"
		chainHealth.Height = chain.GetHeight()
		chainHealth.SyncStatus = "synced"

		block := chain.GetLatestBlock()
		if block != nil {
			blockHash := block.GetHash()
			chainHealth.BestBlockHash = hex.EncodeToString(blockHash[:])
			header := block.GetHeader()
			if header.GetHeight() == 0 {
				chainHealth.GenesisHash = chainHealth.BestBlockHash
			}
		}
	}

	// 获取系统状态
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	systemHealth := SystemHealth{
		GoVersion:       runtime.Version(),
		NumGoroutines:   runtime.NumGoroutine(),
		NumCPU:          runtime.NumCPU(),
		AllocatedMemory: m.Alloc,
	}

	// API status
	apiHealth := APIHealth{
		Status: "healthy",
		Uptime: s.Uptime().String(),
	}

	// buildresponse
	response := DetailedHealthResponse{
		Status:     "healthy",
		Timestamp:  time.Now().UTC(),
		Version:    "2.0.0-mvp",
		Uptime:     s.Uptime().String(),
		Blockchain: chainHealth,
		System:     systemHealth,
		API:        apiHealth,
	}

	writeSuccess(w, response)
}

// handleLiveness handle Kubernetes liveness probe
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleReadiness handle Kubernetes readiness probe
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// 检查链是否就绪
	ready := true
	if chain := s.GetChain(); chain == nil {
		ready = false
	}

	if ready {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Not Ready"))
	}
}
