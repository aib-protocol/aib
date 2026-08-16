package airdrop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

var (
	// ErrInvalidRequest 无效请求
	ErrInvalidRequest = errors.New("invalid request")
	// ErrMethodNotAllowed 方法不允许
	ErrMethodNotAllowed = errors.New("method not allowed")
)

// Handler 空投 API 处理器
type Handler struct {
	validator  *Validator
	scorer     *Scorer
	distributor *Distributor
	router     *mux.Router
}

// HandlerConfig 处理器配置
type HandlerConfig struct {
	ValidatorConfig   *ValidatorConfig
	ScoringConfig     *ScoringConfig
	DistributorConfig *DistributorConfig
	SignerSeed        []byte
}

// NewHandler 创建处理器
func NewHandler(config *HandlerConfig) (*Handler, error) {
	// 创建验证器
	validator := NewValidator(config.ValidatorConfig)

	// 创建评分器
	scorer := NewScorer(config.ScoringConfig)

	// 创建分发器
	distributor, err := NewDistributor(config.DistributorConfig, config.SignerSeed)
	if err != nil {
		return nil, fmt.Errorf("create distributor: %w", err)
	}

	h := &Handler{
		validator:   validator,
		scorer:      scorer,
		distributor: distributor,
		router:      mux.NewRouter(),
	}

	h.setupRoutes()

	return h, nil
}

// setupRoutes 设置路由
func (h *Handler) setupRoutes() {
	api := h.router.PathPrefix("/airdrop").Subrouter()

	// 资格检查
	api.HandleFunc("/eligibility", h.handleEligibility).Methods(http.MethodGet, http.MethodOptions)

	// 认领空投
	api.HandleFunc("/claim", h.handleClaim).Methods(http.MethodPost, http.MethodOptions)

	// 查询状态
	api.HandleFunc("/status", h.handleStatus).Methods(http.MethodGet, http.MethodOptions)

	// 统计信息
	api.HandleFunc("/stats", h.handleStats).Methods(http.MethodGet, http.MethodOptions)

	// 公钥查询
	api.HandleFunc("/public-key", h.handlePublicKey).Methods(http.MethodGet, http.MethodOptions)
}

// ServeHTTP 实现 http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// handleEligibility 处理资格检查
func (h *Handler) handleEligibility(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	// 解析查询参数
	githubToken := r.URL.Query().Get("github_token")
	email := r.URL.Query().Get("email")
	address := r.URL.Query().Get("address")

	// 获取客户端信息
	deviceID := h.generateDeviceFingerprint(r)
	ipAddress := h.getClientIP(r)

	// 验证请求
	if githubToken == "" && h.validator.requireGitHub {
		h.writeError(w, http.StatusBadRequest, "MISSING_GITHUB_TOKEN", "GitHub token is required")
		return
	}

	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_ADDRESS", "Wallet address is required")
		return
	}

	// 1. 验证用户
	verifyResult, err := h.validator.ValidateUser(githubToken, email, deviceID, ipAddress)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidGitHubToken):
			h.writeError(w, http.StatusUnauthorized, "INVALID_GITHUB_TOKEN", err.Error())
		case errors.Is(err, ErrIPLimitExceeded):
			h.writeError(w, http.StatusTooManyRequests, "IP_LIMIT_EXCEEDED", err.Error())
		case errors.Is(err, ErrDeviceFingerprintDuplicate):
			h.writeError(w, http.StatusConflict, "DEVICE_REGISTERED", err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, "VERIFICATION_FAILED", err.Error())
		}
		return
	}

	// 2. 评分
	var score *Score
	if verifyResult.UserInfo != nil {
		score = h.scorer.ScoreUser(verifyResult.UserInfo, nil)
	} else {
		score = &Score{
			Total:      verifyResult.Score,
			IsEligible: verifyResult.Success,
		}
	}

	// 3. 检查认领资格
	amount, err := h.distributor.CanClaim(address, verifyResult.UserInfo.ID, score.Total)
	if err != nil {
		h.writeJSON(w, http.StatusOK, &EligibilityResponse{
			Eligible:    false,
			Score:       score,
			Amount:      nil,
			Reason:      err.Error(),
		})
		return
	}

	h.writeJSON(w, http.StatusOK, &EligibilityResponse{
		Eligible: score.IsEligible,
		Score:    score,
		Amount:   amount,
		Reason:   "",
	})
}

// handleClaim 处理认领请求
func (h *Handler) handleClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	// 解析请求
	var req ClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// 验证必填字段
	if req.Address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_ADDRESS", "Address is required")
		return
	}
	if req.GitHubID == 0 {
		h.writeError(w, http.StatusBadRequest, "MISSING_GITHUB_ID", "GitHub ID is required")
		return
	}

	// 添加设备指纹和 IP
	req.DeviceID = h.generateDeviceFingerprint(r)
	req.IPAddress = h.getClientIP(r)
	req.Timestamp = time.Now().Unix()

	// 尝试认领
	record, err := h.distributor.Claim(&req)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyClaimed):
			h.writeError(w, http.StatusConflict, "ALREADY_CLAIMED", err.Error())
		case errors.Is(err, ErrIneligible):
			h.writeError(w, http.StatusForbidden, "INELIGIBLE", err.Error())
		case errors.Is(err, ErrInvalidSignature):
			h.writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", err.Error())
		case errors.Is(err, ErrTeamAddress):
			h.writeError(w, http.StatusForbidden, "TEAM_ADDRESS", err.Error())
		case errors.Is(err, ErrContractAddress):
			h.writeError(w, http.StatusForbidden, "CONTRACT_ADDRESS", err.Error())
		case errors.Is(err, ErrDistributionDisabled):
			h.writeError(w, http.StatusServiceUnavailable, "DISTRIBUTION_DISABLED", err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, "CLAIM_FAILED", err.Error())
		}
		return
	}

	h.writeJSON(w, http.StatusCreated, &ClaimResponse{
		Success:    true,
		Record:     record,
		Message:    "Airdrop claimed successfully",
	})
}

// handleStatus 处理状态查询
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	address := r.URL.Query().Get("address")
	githubID := r.URL.Query().Get("github_id")

	if address == "" && githubID == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAM", "Either address or github_id is required")
		return
	}

	var record *ClaimRecord
	var found bool

	if address != "" {
		record, found = h.distributor.GetClaim(address)
	} else {
		// 解析 GitHub ID
		var id uint64
		if _, err := fmt.Sscanf(githubID, "%d", &id); err == nil {
			record, found = h.distributor.GetClaimByGitHub(id)
		}
	}

	if !found {
		h.writeJSON(w, http.StatusOK, &StatusResponse{
			Claimed: false,
		})
		return
	}

	h.writeJSON(w, http.StatusOK, &StatusResponse{
		Claimed: true,
		Record:  record,
	})
}

// handleStats 处理统计信息
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	stats := h.distributor.GetStats()
	h.writeJSON(w, http.StatusOK, stats)
}

// handlePublicKey 处理公钥查询
func (h *Handler) handlePublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	h.writeJSON(w, http.StatusOK, &PublicKeyResponse{
		PublicKey: h.distributor.GetPublicKeyHex(),
		Algorithm: "ed25519",
	})
}

// ============================================================================
// 请求/响应类型
// ============================================================================

// EligibilityResponse 资格检查响应
type EligibilityResponse struct {
	Eligible bool           `json:"eligible"`
	Score    *Score         `json:"score,omitempty"`
	Amount   *AirdropAmount `json:"amount,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

// ClaimResponse 认领响应
type ClaimResponse struct {
	Success bool         `json:"success"`
	Record  *ClaimRecord `json:"record,omitempty"`
	Message string       `json:"message,omitempty"`
}

// StatusResponse 状态响应
type StatusResponse struct {
	Claimed bool         `json:"claimed"`
	Record  *ClaimRecord `json:"record,omitempty"`
}

// PublicKeyResponse 公钥响应
type PublicKeyResponse struct {
	PublicKey string `json:"public_key"`
	Algorithm string `json:"algorithm"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ============================================================================
// 辅助方法
// ============================================================================

// writeJSON 写入 JSON 响应
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, &ErrorResponse{
		Code:    code,
		Message: message,
	})
}

// writeCORS 写入 CORS 响应
func (h *Handler) writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusOK)
}

// generateDeviceFingerprint 生成设备指纹
func (h *Handler) generateDeviceFingerprint(r *http.Request) string {
	userAgent := r.Header.Get("User-Agent")
	acceptLang := r.Header.Get("Accept-Language")
	timezone := r.URL.Query().Get("timezone")

	if timezone == "" {
		timezone = "UTC"
	}

	return h.validator.deviceFingerprint.GenerateFingerprint(userAgent, acceptLang, timezone)
}

// getClientIP 获取客户端 IP
func (h *Handler) getClientIP(r *http.Request) string {
	// 检查 X-Forwarded-For
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return NormalizeIP(strings.TrimSpace(ips[0]))
		}
	}

	// 检查 X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return NormalizeIP(xri)
	}

	// 使用 RemoteAddr
	return NormalizeIP(r.RemoteAddr)
}

// GetRouter 获取路由器
func (h *Handler) GetRouter() *mux.Router {
	return h.router
}

// GetValidator 获取验证器
func (h *Handler) GetValidator() *Validator {
	return h.validator
}

// GetScorer 获取评分器
func (h *Handler) GetScorer() *Scorer {
	return h.scorer
}

// GetDistributor 获取分发器
func (h *Handler) GetDistributor() *Distributor {
	return h.distributor
}
