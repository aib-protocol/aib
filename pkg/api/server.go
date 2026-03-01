package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// BlockHeader 定义区块头接口
type BlockHeader interface {
	GetHeight() uint64
	GetTimestamp() uint64
	GetPrevBlockHash() [32]byte
	GetProposer() [32]byte
}

// Block 定义区块接口
type Block interface {
	GetHeader() BlockHeader
	GetHash() [32]byte
	GetTransactions() int
}

// ChainReader 接口用于读取区块链数据
type ChainReader interface {
	GetHeight() uint64
	GetLatestBlock() Block
}

// Server HTTP 服务器
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	port       int
	mu         sync.RWMutex
	startTime  time.Time
	chain      ChainReader
}

// SetChain 设置区块链引用
func (s *Server) SetChain(chain ChainReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chain = chain
}

// GetChain 获取区块链引用
func (s *Server) GetChain() ChainReader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chain
}

// NewServer 创建新的 API 服务器
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	server := &Server{
		mux:       mux,
		port:      port,
		startTime: time.Now(),
	}

	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server
}

// RegisterRoutes 注册所有路由
func (s *Server) RegisterRoutes() {
	// 健康检查
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/status", s.handleStatus)

	// 区块
	s.mux.HandleFunc("/v1/block/latest", s.handleGetLatestBlock)
	s.mux.HandleFunc("/v1/block/", s.handleGetBlock)

	// 交易
	s.mux.HandleFunc("/v1/transaction", s.handleSubmitTransaction)

	// 余额
	s.mux.HandleFunc("/v1/balance/", s.handleGetBalance)

	// 通道
	s.mux.HandleFunc("/v1/channel/open", s.handleChannelOpen)
	s.mux.HandleFunc("/v1/channel/close", s.handleChannelClose)
	s.mux.HandleFunc("/v1/channel/", s.handleGetChannel)
	s.mux.HandleFunc("/v1/channels", s.handleListChannels)
	s.mux.HandleFunc("/v1/channel/pay", s.handleChannelPay)

	// AI服务
	s.mux.HandleFunc("/v1/ai/inference", s.handleInference)
	s.mux.HandleFunc("/v1/ai/models", s.handleListModels)
	s.mux.HandleFunc("/v1/ai/nodes", s.handleListNodes)

	// 应用日志中间件
	s.httpServer.Handler = LoggingMiddleware(s.mux)
}

// Start 启动服务器
func (s *Server) Start() error {
	s.RegisterRoutes()
	log.Printf("Starting API server on port %d", s.port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("Stopping API server on port %d", s.port)
	return s.httpServer.Shutdown(ctx)
}

// Uptime 返回服务器运行时间
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// ============================================================================
// 辅助函数
// ============================================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
	})
}

func writeError(w http.ResponseWriter, status int, code, message, details string) {
	writeJSON(w, status, APIResponse{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func parsePathVar(r *http.Request, prefix string) string {
	path := r.URL.Path
	if len(path) > len(prefix) {
		return path[len(prefix):]
	}
	return ""
}
