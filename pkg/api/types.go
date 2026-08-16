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
	ErrCodeInvalidRequest  = "INVALID_REQUEST"
	ErrCodeUnauthorized    = "UNAUTHORIZED"
	ErrCodeForbidden       = "FORBIDDEN"
	ErrCodeNotFound        = "NOT_FOUND"
	ErrCodeConflict        = "CONFLICT"
	ErrCodeRateLimited     = "RATE_LIMITED"
	ErrCodeInternalError   = "INTERNAL_ERROR"
	ErrCodeNotImplemented  = "NOT_IMPLEMENTED"
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

// ============================================================================
// P2P API 类型
// ============================================================================

// PeerResponse 节点响应
type PeerResponse struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LastSeen  time.Time `json:"last_seen"`
	Connected bool      `json:"connected"`
}

// ============================================================================
// Migration API 类型
// ============================================================================

// MigrationStatus 迁移状态（从 migration hub 获取）
// 注意：此类型需要与 pkg/migration/hub.go 中的 MigrationStatus 保持同步
type MigrationStatus struct {
	// AIB1
	AIB1TotalMigrated  uint64    `json:"aib1_total_migrated"`
	AIB1ClaimOpen      bool      `json:"aib1_claim_open"`
	AIB1ClaimDeadline  time.Time `json:"aib1_claim_deadline"`
	AIB1SnapshotRoot   [32]byte  `json:"aib1_snapshot_root"`
	SnapshotTime       time.Time `json:"snapshot_time"`

	// BTC
	BTCTotalMigrated uint64 `json:"btc_total_migrated"`
	BTCTotalRewards  uint64 `json:"btc_total_rewards"`
	BTCWindowOpen    bool   `json:"btc_window_open"`
	BTCCurrentRate  uint64 `json:"btc_current_rate"`

	// ETH
	ETHTotalMigrated uint64 `json:"eth_total_migrated"`
	ETHTotalRewards  uint64 `json:"eth_total_rewards"`
	ETHWindowOpen    bool   `json:"eth_window_open"`
	ETHCurrentRate  uint64 `json:"eth_current_rate"`

	// SOL
	SOLTotalMigrated uint64 `json:"sol_total_migrated"`
	SOLTotalRewards  uint64 `json:"sol_total_rewards"`
	SOLWindowOpen    bool   `json:"sol_window_open"`
	SOLCurrentRate  uint64 `json:"sol_current_rate"`

	// 时间窗口
	MigrationWindowStart time.Time `json:"migration_window_start"`
	MigrationWindowEnd   time.Time `json:"migration_window_end"`
}

// AIB1SnapshotResponse AIB1 快照信息响应
type AIB1SnapshotResponse struct {
	SnapshotRoot    string    `json:"snapshot_root"`
	SnapshotTime    time.Time `json:"snapshot_time"`
	ClaimDeadline  time.Time `json:"claim_deadline"`
	ClaimOpen       bool      `json:"claim_open"`
	TotalMigrated   uint64    `json:"total_migrated"`
}

// MigrationRatesResponse 迁移汇率响应
type MigrationRatesResponse struct {
	Timestamp time.Time         `json:"timestamp"`
	AIB1Rate  uint64            `json:"aib1_rate"` // 1:1 固定
	ChainRates map[string]ChainRateInfo `json:"chain_rates"`
}

// ChainRateInfo 链汇率信息
type ChainRateInfo struct {
	Chain         string    `json:"chain"`
	CurrentRate   uint64    `json:"current_rate"`   // 激励比率（百分比）
	WindowOpen    bool      `json:"window_open"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
}

// UserMigrationInfoAPI 用户迁移信息 API 响应
type UserMigrationInfoAPI struct {
	// AIB1
	AIB1SnapshotBalance uint64 `json:"aib1_snapshot_balance"`
	AIB1Claimed        bool   `json:"aib1_claimed"`

	// 跨链锁定奖励
	LockedRewards LockedRewardsInfo `json:"locked_rewards"`

	// 总计
	TotalClaimable uint64 `json:"total_claimable"`
	TotalLocked    uint64 `json:"total_locked"`
}

// LockedRewardsInfo 锁定奖励信息
type LockedRewardsInfo struct {
	BTC []VestingRewardInfo `json:"btc"`
	ETH []VestingRewardInfo `json:"eth"`
	SOL []VestingRewardInfo `json:"sol"`
}

// VestingRewardInfo Vesting 奖励信息
type VestingRewardInfo struct {
	SourceTxID    string          `json:"source_tx_id"`
	SourceAmount  uint64          `json:"source_amount"`
	TotalReward   uint64          `json:"total_reward"`
	Claimed       uint64          `json:"claimed"`
	Claimable     uint64          `json:"claimable"`
	Locked        uint64          `json:"locked"`
	VestingSchedule []VestingEntryInfo `json:"vesting_schedule"`
}

// VestingEntryInfo Vesting 解锁条目信息
type VestingEntryInfo struct {
	UnlockTime time.Time `json:"unlock_time"`
	Percent    uint64    `json:"percent"`
	Amount     uint64    `json:"amount"`
	Status     string    `json:"status"` // "locked" 或 "unlocked"
}

// ClaimableResponse 可领取金额响应
type ClaimableResponse struct {
	Address       string    `json:"address"`
	TotalClaimable uint64   `json:"total_claimable"`
	AIB1Claimable uint64    `json:"aib1_claimable"` // AIB1 待认领（快照余额）
	CrossChainClaimable CrossChainClaimable `json:"cross_chain_claimable"`
}

// CrossChainClaimable 跨链可领取金额
type CrossChainClaimable struct {
	BTC uint64 `json:"btc"`
	ETH uint64 `json:"eth"`
	SOL uint64 `json:"sol"`
}

// ClaimAIB1Request AIB1 认领请求
type ClaimAIB1Request struct {
	TargetAddress string `json:"target_address"`
	Amount        uint64 `json:"amount"`
	PublicKey     string `json:"public_key"`     // Base64 编码
	Signature     string `json:"signature"`      // Base64 编码
	Nonce         uint64 `json:"nonce"`
}

// ClaimUnlockedRequest 解锁代币认领请求
type ClaimUnlockedRequest struct {
	Address string `json:"address"`
}

// MigrationClaimResponse 迁移操作响应
type MigrationClaimResponse struct {
	TxHash     string    `json:"tx_hash"`
	Address    string    `json:"address"`
	Amount     uint64    `json:"amount"`
	Type       string    `json:"type"` // "aib1", "cross_chain"
	Chain      string    `json:"chain,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// EstimateRequest 收益估算请求
type EstimateRequest struct {
	Chain   string `json:"chain"`   // "BTC", "ETH", "SOL"
	Amount  uint64 `json:"amount"`  // 源链代币数量
}

// EstimateResponse 收益估算响应
type EstimateResponse struct {
	SourceChain   string             `json:"source_chain"`
	SourceAmount  uint64             `json:"source_amount"`
	Reward        EstimateRewardInfo `json:"reward"`
	Vesting       []VestingEntryInfo `json:"vesting"`
}

// EstimateRewardInfo 估算奖励信息
type EstimateRewardInfo struct {
	TotalReward   uint64 `json:"total_reward"`
	CurrentRate   uint64 `json:"current_rate"`
	TGEPercent    uint64 `json:"tge_percent"`
	TGEAmount     uint64 `json:"tge_amount"`
	VestingMonths uint64 `json:"vesting_months"`
}

// Error codes for migration API
const (
	ErrCodeMigrationNotFound     = "MIGRATION_NOT_FOUND"
	ErrCodeMigrationWindowClosed = "MIGRATION_WINDOW_CLOSED"
	ErrCodeAlreadyClaimed        = "ALREADY_CLAIMED"
	ErrCodeInvalidSignature      = "INVALID_SIGNATURE"
	ErrCodeInsufficientBalance   = "INSUFFICIENT_BALANCE"
)
