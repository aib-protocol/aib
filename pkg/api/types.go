// Package api provides REST API services for AIB 2.0
package api

import (
	"time"
)

// ============================================================================
// Unified response structures
// ============================================================================

// APIResponse is the unified API response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo holds detailed error information
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// NewSuccessResponse createsuccessresponse
func NewSuccessResponse(data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Data:    data,
	}
}

// NewErrorResponse createerrorresponse
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
// Error code definitions
// ============================================================================

const (
	ErrCodeInvalidRequest = "INVALID_REQUEST"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeRateLimited    = "RATE_LIMITED"
	ErrCodeInternalError  = "INTERNAL_ERROR"
	ErrCodeNotImplemented = "NOT_IMPLEMENTED"
)

// ============================================================================
// Basic API types
// ============================================================================

// HealthResponse health checkresponse
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

// BalanceResponse balancequeryresponse
type BalanceResponse struct {
	Address   string     `json:"address"`
	Balance   uint64     `json:"balance"`
	UTXOCount int        `json:"utxo_count"`
	UTXOs     []UTxOInfo `json:"utxos,omitempty"`
}

// UTxOInfo holds UTXO information
type UTxOInfo struct {
	TxHash string `json:"tx_hash"`
	Index  uint32 `json:"index"`
	Value  uint64 `json:"value"`
	Script string `json:"script,omitempty"`
}

// TransactionRequest submittransactionrequest
type TransactionRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   uint64 `json:"amount"`
	GasLimit uint64 `json:"gas_limit,omitempty"`
	GasPrice uint64 `json:"gas_price,omitempty"`
	Data     []byte `json:"data,omitempty"`
	Nonce    uint64 `json:"nonce,omitempty"`
}

// TransactionResponse transactionresponse
type TransactionResponse struct {
	TxHash    string    `json:"tx_hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    uint64    `json:"amount"`
	GasUsed   uint64    `json:"gas_used"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// BlockResponse blockresponse
type BlockResponse struct {
	Height    uint64    `json:"height"`
	Hash      string    `json:"hash"`
	PrevHash  string    `json:"prev_hash"`
	Timestamp time.Time `json:"timestamp"`
	TxCount   int       `json:"tx_count"`
	Validator string    `json:"validator,omitempty"`
	Size      uint64    `json:"size"`
}

// BlockListResponse blocklistresponse
type BlockListResponse struct {
	Blocks []BlockResponse `json:"blocks"`
	Total  int             `json:"total"`
}

// ============================================================================
// channel API type
// ============================================================================

// OpenChannelRequest is a channel open request
type OpenChannelRequest struct {
	PartyA   string `json:"party_a"`
	PartyB   string `json:"party_b"`
	DepositA uint64 `json:"deposit_a"`
	DepositB uint64 `json:"deposit_b"`
}

// CloseChannelRequest is a channel close request
type CloseChannelRequest struct {
	ChannelID string `json:"channel_id"`
	SigA      []byte `json:"sig_a"`
	SigB      []byte `json:"sig_b"`
}

// PaymentRequest is a channel payment request
type PaymentRequest struct {
	ChannelID string `json:"channel_id"`
	Amount    uint64 `json:"amount"`
	FromA     bool   `json:"from_a"` // true: A pays B, false: B pays A
}

// UpdateChannelRequest updatechannelstatusrequest
type UpdateChannelRequest struct {
	ChannelID string `json:"channel_id"`
	BalanceA  uint64 `json:"balance_a"`
	BalanceB  uint64 `json:"balance_b"`
	Sequence  uint64 `json:"sequence"`
	SigA      []byte `json:"sig_a"`
	SigB      []byte `json:"sig_b"`
}

// ChannelResponse channeldetailsresponse
type ChannelResponse struct {
	ID         string     `json:"id"`
	PartyA     string     `json:"party_a"`
	PartyB     string     `json:"party_b"`
	BalanceA   uint64     `json:"balance_a"`
	BalanceB   uint64     `json:"balance_b"`
	Sequence   uint64     `json:"sequence"`
	StateHash  string     `json:"state_hash"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUpdate time.Time  `json:"last_update,omitempty"`
	DisputeEnd *time.Time `json:"dispute_end,omitempty"`
}

// ChannelListResponse channellistresponse
type ChannelListResponse struct {
	Channels []ChannelResponse `json:"channels"`
	Total    int               `json:"total"`
}

// ChannelStatusResponse channelstatusresponse
type ChannelStatusResponse struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Sequence  uint64 `json:"sequence"`
	BalanceA  uint64 `json:"balance_a"`
	BalanceB  uint64 `json:"balance_b"`
}

// ============================================================================
// AI service API type
// ============================================================================

// InferenceRequest is an AI inference request
type InferenceRequest struct {
	Prompt      string  `json:"prompt"`
	ModelID     string  `json:"model_id,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// InferenceResponse is an AI inference response
type InferenceResponse struct {
	Result     string    `json:"result"`
	ModelID    string    `json:"model_id"`
	ModelName  string    `json:"model_name"`
	Duration   int64     `json:"duration_ms"`
	TokensUsed int       `json:"tokens_used,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ModelInfoResponse modelinforesponse
type ModelInfoResponse struct {
	ModelID      string    `json:"model_id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	BaseURL      string    `json:"base_url,omitempty"`
	Weight       float64   `json:"weight"`
	Available    bool      `json:"available"`
	RegisteredAt time.Time `json:"registered_at"`
}

// ModelListResponse modellistresponse
type ModelListResponse struct {
	Models []ModelInfoResponse `json:"models"`
	Total  int                 `json:"total"`
}

// AINodeInfoResponse is an AI node info response
type AINodeInfoResponse struct {
	NodeID     string    `json:"node_id"`
	Address    string    `json:"address"`
	Stake      uint64    `json:"stake"`
	Models     []string  `json:"models"`
	Reputation float64   `json:"reputation"`
	Status     string    `json:"status"`
	LastSeen   time.Time `json:"last_seen"`
}

// AINodeListResponse AI nodelistresponse
type AINodeListResponse struct {
	Nodes []AINodeInfoResponse `json:"nodes"`
	Total int                  `json:"total"`
}

// ============================================================================
// Pagination types
// ============================================================================

// PaginationRequest is a pagination request
type PaginationRequest struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`
}

// PaginationResponse is pagination response metadata
type PaginationResponse struct {
	Page      int `json:"page"`
	PageSize  int `json:"page_size"`
	Total     int `json:"total"`
	TotalPage int `json:"total_page"`
}

// NewPaginationResponse creates a pagination response
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
// configtype
// ============================================================================

// Config holds the API server configuration
type Config struct {
	// Server configuration
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`

	// CORS config
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`

	// Rate limiting configuration
	RateLimitPerSecond int `json:"rate_limit_per_second"`
	RateLimitBurst     int `json:"rate_limit_burst"`

	// Authentication configuration
	APIKeys []string `json:"api_keys"`

	// logconfig
	EnableRequestLog bool `json:"enable_request_log"`
	EnableErrorLog   bool `json:"enable_error_log"`
	LogRequestBody   bool `json:"log_request_body"`
	LogResponseBody  bool `json:"log_response_body"`
}

// DefaultConfig returnsdefaultconfig
func DefaultConfig() *Config {
	return &Config{
		Port:               8080,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		IdleTimeout:        120 * time.Second,
		AllowedOrigins:     []string{"*"},
		AllowedMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:     []string{"Content-Type", "Authorization"},
		RateLimitPerSecond: 100,
		RateLimitBurst:     200,
		APIKeys:            []string{},
		EnableRequestLog:   true,
		EnableErrorLog:     true,
		LogRequestBody:     false,
		LogResponseBody:    false,
	}
}

// ============================================================================
// P2P API type
// ============================================================================

// PeerResponse noderesponse
type PeerResponse struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LastSeen  time.Time `json:"last_seen"`
	Connected bool      `json:"connected"`
}

// ============================================================================
// Migration API type
// ============================================================================

// MigrationStatus is the migration status (fetched from the migration hub)
// Note: this type must be kept in sync with MigrationStatus in pkg/migration/hub.go
type MigrationStatus struct {
	// AIB1
	AIB1TotalMigrated uint64    `json:"aib1_total_migrated"`
	AIB1ClaimOpen     bool      `json:"aib1_claim_open"`
	AIB1ClaimDeadline time.Time `json:"aib1_claim_deadline"`
	AIB1SnapshotRoot  [32]byte  `json:"aib1_snapshot_root"`
	SnapshotTime      time.Time `json:"snapshot_time"`

	// BTC
	BTCTotalMigrated uint64 `json:"btc_total_migrated"`
	BTCTotalRewards  uint64 `json:"btc_total_rewards"`
	BTCWindowOpen    bool   `json:"btc_window_open"`
	BTCCurrentRate   uint64 `json:"btc_current_rate"`

	// ETH
	ETHTotalMigrated uint64 `json:"eth_total_migrated"`
	ETHTotalRewards  uint64 `json:"eth_total_rewards"`
	ETHWindowOpen    bool   `json:"eth_window_open"`
	ETHCurrentRate   uint64 `json:"eth_current_rate"`

	// SOL
	SOLTotalMigrated uint64 `json:"sol_total_migrated"`
	SOLTotalRewards  uint64 `json:"sol_total_rewards"`
	SOLWindowOpen    bool   `json:"sol_window_open"`
	SOLCurrentRate   uint64 `json:"sol_current_rate"`

	// Time window
	MigrationWindowStart time.Time `json:"migration_window_start"`
	MigrationWindowEnd   time.Time `json:"migration_window_end"`
}

// AIB1SnapshotResponse is the AIB1 snapshot info response
type AIB1SnapshotResponse struct {
	SnapshotRoot  string    `json:"snapshot_root"`
	SnapshotTime  time.Time `json:"snapshot_time"`
	ClaimDeadline time.Time `json:"claim_deadline"`
	ClaimOpen     bool      `json:"claim_open"`
	TotalMigrated uint64    `json:"total_migrated"`
}

// MigrationRatesResponse is the migration rates response
type MigrationRatesResponse struct {
	Timestamp  time.Time                `json:"timestamp"`
	AIB1Rate   uint64                   `json:"aib1_rate"` // fixed 1:1
	ChainRates map[string]ChainRateInfo `json:"chain_rates"`
}

// ChainRateInfo holds per-chain rate information
type ChainRateInfo struct {
	Chain       string    `json:"chain"`
	CurrentRate uint64    `json:"current_rate"` // incentive rate (percentage)
	WindowOpen  bool      `json:"window_open"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// UserMigrationInfoAPI is the user migration info API response
type UserMigrationInfoAPI struct {
	// AIB1
	AIB1SnapshotBalance uint64 `json:"aib1_snapshot_balance"`
	AIB1Claimed         bool   `json:"aib1_claimed"`

	// Cross-chain locked rewards
	LockedRewards LockedRewardsInfo `json:"locked_rewards"`

	// Totals
	TotalClaimable uint64 `json:"total_claimable"`
	TotalLocked    uint64 `json:"total_locked"`
}

// LockedRewardsInfo holds locked rewards information
type LockedRewardsInfo struct {
	BTC []VestingRewardInfo `json:"btc"`
	ETH []VestingRewardInfo `json:"eth"`
	SOL []VestingRewardInfo `json:"sol"`
}

// VestingRewardInfo holds vesting reward information
type VestingRewardInfo struct {
	SourceTxID      string             `json:"source_tx_id"`
	SourceAmount    uint64             `json:"source_amount"`
	TotalReward     uint64             `json:"total_reward"`
	Claimed         uint64             `json:"claimed"`
	Claimable       uint64             `json:"claimable"`
	Locked          uint64             `json:"locked"`
	VestingSchedule []VestingEntryInfo `json:"vesting_schedule"`
}

// VestingEntryInfo holds a vesting unlock entry
type VestingEntryInfo struct {
	UnlockTime time.Time `json:"unlock_time"`
	Percent    uint64    `json:"percent"`
	Amount     uint64    `json:"amount"`
	Status     string    `json:"status"` // "locked" or "unlocked"
}

// ClaimableResponse is the claimable amount response
type ClaimableResponse struct {
	Address             string              `json:"address"`
	TotalClaimable      uint64              `json:"total_claimable"`
	AIB1Claimable       uint64              `json:"aib1_claimable"` // AIB1 pending claim (snapshot balance)
	CrossChainClaimable CrossChainClaimable `json:"cross_chain_claimable"`
}

// CrossChainClaimable holds cross-chain claimable amounts
type CrossChainClaimable struct {
	BTC uint64 `json:"btc"`
	ETH uint64 `json:"eth"`
	SOL uint64 `json:"sol"`
}

// ClaimAIB1Request is an AIB1 claim request
type ClaimAIB1Request struct {
	TargetAddress string `json:"target_address"`
	Amount        uint64 `json:"amount"`
	PublicKey     string `json:"public_key"` // Base64 encoded
	Signature     string `json:"signature"`  // Base64 encoded
	Nonce         uint64 `json:"nonce"`
}

// ClaimUnlockedRequest is an unlocked-token claim request
type ClaimUnlockedRequest struct {
	Address string `json:"address"`
}

// MigrationClaimResponse is a migration operation response
type MigrationClaimResponse struct {
	TxHash    string    `json:"tx_hash"`
	Address   string    `json:"address"`
	Amount    uint64    `json:"amount"`
	Type      string    `json:"type"` // "aib1", "cross_chain"
	Chain     string    `json:"chain,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EstimateRequest is a reward estimation request
type EstimateRequest struct {
	Chain  string `json:"chain"`  // "BTC", "ETH", "SOL"
	Amount uint64 `json:"amount"` // source-chain token amount
}

// EstimateResponse is a reward estimation response
type EstimateResponse struct {
	SourceChain  string             `json:"source_chain"`
	SourceAmount uint64             `json:"source_amount"`
	Reward       EstimateRewardInfo `json:"reward"`
	Vesting      []VestingEntryInfo `json:"vesting"`
}

// EstimateRewardInfo holds estimated reward information
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
