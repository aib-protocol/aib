// Package api provides REST API services for AIB 2.0
package api

import (
	"time"
)

// ============================================================================
// 统一响应结构
// ============================================================================

// APIResponse 统一 API 响应结构
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo 错误详细信息
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Data:    data,
	}
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code, message, details string) APIResponse {
	return APIResponse{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

// ============================================================================
// 错误码定义
// ============================================================================

const (
	ErrCodeInvalidRequest = "INVALID_REQUEST"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeRateLimited    = "RATE_LIMITED"
	ErrCodeInternalError  = "INTERNAL_ERROR"
)

// ============================================================================
// 基础 API 类型
// ============================================================================

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

// BalanceResponse 余额查询响应
type BalanceResponse struct {
	Address  string     `json:"address"`
	Balance  uint64     `json:"balance"`
	UTXOCount int       `json:"utxo_count"`
	UTXOs    []UTxOInfo `json:"utxos,omitempty"`
}

// UTxOInfo UTXO 信息
type UTxOInfo struct {
	TxHash  string `json:"tx_hash"`
	Index   uint32 `json:"index"`
	Value   uint64 `json:"value"`
	Script  string `json:"script,omitempty"`
}

// TransactionRequest 提交交易请求
type TransactionRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   uint64 `json:"amount"`
	GasLimit uint64 `json:"gas_limit,omitempty"`
	GasPrice uint64 `json:"gas_price,omitempty"`
	Data     []byte `json:"data,omitempty"`
	Nonce    uint64 `json:"nonce,omitempty"`
}

// TransactionResponse 交易响应
type TransactionResponse struct {
	TxHash    string    `json:"tx_hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    uint64    `json:"amount"`
	GasUsed   uint64    `json:"gas_used"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// BlockResponse 区块响应
type BlockResponse struct {
	Height     uint64    `json:"height"`
	Hash       string    `json:"hash"`
	PrevHash   string    `json:"prev_hash"`
	Timestamp  time.Time `json:"timestamp"`
	TxCount    int       `json:"tx_count"`
	Validator  string    `json:"validator,omitempty"`
	Size       uint64    `json:"size"`
}

// BlockListResponse 区块列表响应
type BlockListResponse struct {
	Blocks []BlockResponse `json:"blocks"`
	Total  int             `json:"total"`
}

// ============================================================================
// 通道 API 类型
// ============================================================================

// OpenChannelRequest 开启通道请求
type OpenChannelRequest struct {
	PartyA   string `json:"party_a"`
	PartyB   string `json:"party_b"`
	DepositA uint64 `json:"deposit_a"`
	DepositB uint64 `json:"deposit_b"`
}

// CloseChannelRequest 关闭通道请求
type CloseChannelRequest struct {
	ChannelID string      `json:"channel_id"`
	SigA      []byte      `json:"sig_a"`
	SigB      []byte      `json:"sig_b"`
}

// PaymentRequest 通道支付请求
type PaymentRequest struct {
	ChannelID string `json:"channel_id"`
	Amount    uint64 `json:"amount"`
	FromA     bool   `json:"from_a"` // true: A支付给B, false: B支付给A
}

// UpdateChannelRequest 更新通道状态请求
type UpdateChannelRequest struct {
	ChannelID string `json:"channel_id"`
	BalanceA  uint64 `json:"balance_a"`
	BalanceB  uint64 `json:"balance_b"`
	Sequence  uint64 `json:"sequence"`
	SigA      []byte `json:"sig_a"`
	SigB      []byte `json:"sig_b"`
}

// ChannelResponse 通道详情响应
type ChannelResponse struct {
	ID           string     `json:"id"`
	PartyA       string     `json:"party_a"`
	PartyB       string     `json:"party_b"`
	BalanceA     uint64     `json:"balance_a"`
	BalanceB     uint64     `json:"balance_b"`
	Sequence     uint64     `json:"sequence"`
	StateHash    string     `json:"state_hash"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUpdate   time.Time  `json:"last_update,omitempty"`
	DisputeEnd   *time.Time `json:"dispute_end,omitempty"`
}

// ChannelListResponse 通道列表响应
type ChannelListResponse struct {
	Channels []ChannelResponse `json:"channels"`
	Total    int               `json:"total"`
}

// ChannelStatusResponse 通道状态响应
type ChannelStatusResponse struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Sequence  uint64 `json:"sequence"`
	BalanceA  uint64 `json:"balance_a"`
	BalanceB  uint64 `json:"balance_b"`
}

// ============================================================================
// AI 服务 API 类型
// ============================================================================

// InferenceRequest AI 推理请求
type InferenceRequest struct {
	Prompt      string  `json:"prompt"`
	ModelID     string  `json:"model_id,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// InferenceResponse AI 推理响应
type InferenceResponse struct {
	Result      string        `json:"result"`
	ModelID     string        `json:"model_id"`
	ModelName   string        `json:"model_name"`
	Duration    int64         `json:"duration_ms"`
	TokensUsed  int           `json:"tokens_used,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
}

// ModelInfoResponse 模型信息响应
type ModelInfoResponse struct {
	ModelID       string    `json:"model_id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	BaseURL       string    `json:"base_url,omitempty"`
	Weight        float64   `json:"weight"`
	Available     bool      `json:"available"`
	RegisteredAt  time.Time `json:"registered_at"`
}

// ModelListResponse 模型列表响应
type ModelListResponse struct {
	Models []ModelInfoResponse `json:"models"`
	Total  int                 `json:"total"`
}

// AINodeInfoResponse AI 节点信息响应
type AINodeInfoResponse struct {
	NodeID      string   `json:"node_id"`
	Address     string   `json:"address"`
	Stake       uint64   `json:"stake"`
	Models      []string `json:"models"`
	Reputation  float64  `json:"reputation"`
	Status      string   `json:"status"`
	LastSeen    time.Time `json:"last_seen"`
}

// AINodeListResponse AI 节点列表响应
type AINodeListResponse struct {
	Nodes []AINodeInfoResponse `json:"nodes"`
	Total int                  `json:"total"`
}

// ============================================================================
// 分页类型
// ============================================================================

// PaginationRequest 分页请求
type PaginationRequest struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`
}

// PaginationResponse 分页响应元数据
type PaginationResponse struct {
	Page      int `json:"page"`
	PageSize  int `json:"page_size"`
	Total     int `json:"total"`
	TotalPage int `json:"total_page"`
}

// NewPaginationResponse 创建分页响应
func NewPaginationResponse(page, pageSize, total int) PaginationResponse {
	totalPage := total / pageSize
	if total%pageSize != 0 {
		totalPage++
	}
	return PaginationResponse{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: totalPage,
	}
}

// ============================================================================
// 配置类型
// ============================================================================

// Config API 服务器配置
type Config struct {
	// 服务器配置
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`

	// CORS 配置
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`

	// 限流配置
	RateLimitPerSecond int `json:"rate_limit_per_second"`
	RateLimitBurst     int `json:"rate_limit_burst"`

	// 认证配置
	APIKeys []string `json:"api_keys"`

	// 日志配置
	EnableRequestLog  bool `json:"enable_request_log"`
	EnableErrorLog    bool `json:"enable_error_log"`
	LogRequestBody    bool `json:"log_request_body"`
	LogResponseBody   bool `json:"log_response_body"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Port:              8080,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		AllowedOrigins:    []string{"*"},
		AllowedMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:    []string{"Content-Type", "Authorization"},
		RateLimitPerSecond: 100,
		RateLimitBurst:     200,
		APIKeys:           []string{},
		EnableRequestLog:  true,
		EnableErrorLog:    true,
		LogRequestBody:    false,
		LogResponseBody:   false,
	}
}
