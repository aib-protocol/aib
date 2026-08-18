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
	// ErrInvalidRequest invalidrequest
	ErrInvalidRequest = errors.New("invalid request")
	// ErrMethodNotAllowed method not allowed
	ErrMethodNotAllowed = errors.New("method not allowed")
)

// Handler airdrop API handler
type Handler struct {
	validator   *Validator
	scorer      *Scorer
	distributor *Distributor
	router      *mux.Router
}

// HandlerConfig handlerconfig
type HandlerConfig struct {
	ValidatorConfig   *ValidatorConfig
	ScoringConfig     *ScoringConfig
	DistributorConfig *DistributorConfig
	SignerSeed        []byte
}

// NewHandler createhandler
func NewHandler(config *HandlerConfig) (*Handler, error) {
	// create validator
	validator := NewValidator(config.ValidatorConfig)

	// createscorer
	scorer := NewScorer(config.ScoringConfig)

	// create distributor
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

// setupRoutes setrouting
func (h *Handler) setupRoutes() {
	api := h.router.PathPrefix("/airdrop").Subrouter()

	// eligibility check
	api.HandleFunc("/eligibility", h.handleEligibility).Methods(http.MethodGet, http.MethodOptions)

	// claim airdrop
	api.HandleFunc("/claim", h.handleClaim).Methods(http.MethodPost, http.MethodOptions)

	// querystatus
	api.HandleFunc("/status", h.handleStatus).Methods(http.MethodGet, http.MethodOptions)

	// stats
	api.HandleFunc("/stats", h.handleStats).Methods(http.MethodGet, http.MethodOptions)

	// public key query
	api.HandleFunc("/public-key", h.handlePublicKey).Methods(http.MethodGet, http.MethodOptions)
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// handleEligibility handles eligibility checks
func (h *Handler) handleEligibility(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	// parsequeryparameter
	githubToken := r.URL.Query().Get("github_token")
	email := r.URL.Query().Get("email")
	address := r.URL.Query().Get("address")

	// get client info
	deviceID := h.generateDeviceFingerprint(r)
	ipAddress := h.getClientIP(r)

	// verifyrequest
	if githubToken == "" && h.validator.requireGitHub {
		h.writeError(w, http.StatusBadRequest, "MISSING_GITHUB_TOKEN", "GitHub token is required")
		return
	}

	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_ADDRESS", "Wallet address is required")
		return
	}

	// 1. verifyuser
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

	// 2. scoring
	var score *Score
	if verifyResult.UserInfo != nil {
		score = h.scorer.ScoreUser(verifyResult.UserInfo, nil)
	} else {
		score = &Score{
			Total:      verifyResult.Score,
			IsEligible: verifyResult.Success,
		}
	}

	// 3. check claim eligibility
	amount, err := h.distributor.CanClaim(address, verifyResult.UserInfo.ID, score.Total)
	if err != nil {
		h.writeJSON(w, http.StatusOK, &EligibilityResponse{
			Eligible: false,
			Score:    score,
			Amount:   nil,
			Reason:   err.Error(),
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

// handleClaim handles claim requests
func (h *Handler) handleClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	// parserequest
	var req ClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// validate required fields
	if req.Address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_ADDRESS", "Address is required")
		return
	}
	if req.GitHubID == 0 {
		h.writeError(w, http.StatusBadRequest, "MISSING_GITHUB_ID", "GitHub ID is required")
		return
	}

	// add device fingerprint and IP
	req.DeviceID = h.generateDeviceFingerprint(r)
	req.IPAddress = h.getClientIP(r)
	req.Timestamp = time.Now().Unix()

	// attempt claim
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
		Success: true,
		Record:  record,
		Message: "Airdrop claimed successfully",
	})
}

// handleStatus handlestatusquery
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
		// parse GitHub ID
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

// handleStats handles stats
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.writeCORS(w)
		return
	}

	stats := h.distributor.GetStats()
	h.writeJSON(w, http.StatusOK, stats)
}

// handlePublicKey handles public key queries
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
// request/responsetype
// ============================================================================

// EligibilityResponse eligibility check response
type EligibilityResponse struct {
	Eligible bool           `json:"eligible"`
	Score    *Score         `json:"score,omitempty"`
	Amount   *AirdropAmount `json:"amount,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

// ClaimResponse claim response
type ClaimResponse struct {
	Success bool         `json:"success"`
	Record  *ClaimRecord `json:"record,omitempty"`
	Message string       `json:"message,omitempty"`
}

// StatusResponse statusresponse
type StatusResponse struct {
	Claimed bool         `json:"claimed"`
	Record  *ClaimRecord `json:"record,omitempty"`
}

// PublicKeyResponse public key response
type PublicKeyResponse struct {
	PublicKey string `json:"public_key"`
	Algorithm string `json:"algorithm"`
}

// ErrorResponse errorresponse
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ============================================================================
// helper methods
// ============================================================================

// writeJSON write JSON response
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(data)
}

// writeError writeerrorresponse
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, &ErrorResponse{
		Code:    code,
		Message: message,
	})
}

// writeCORS write CORS response
func (h *Handler) writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusOK)
}

// generateDeviceFingerprint generates a device fingerprint
func (h *Handler) generateDeviceFingerprint(r *http.Request) string {
	userAgent := r.Header.Get("User-Agent")
	acceptLang := r.Header.Get("Accept-Language")
	timezone := r.URL.Query().Get("timezone")

	if timezone == "" {
		timezone = "UTC"
	}

	return h.validator.deviceFingerprint.GenerateFingerprint(userAgent, acceptLang, timezone)
}

// getClientIP getclient IP
func (h *Handler) getClientIP(r *http.Request) string {
	// check X-Forwarded-For
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return NormalizeIP(strings.TrimSpace(ips[0]))
		}
	}

	// check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return NormalizeIP(xri)
	}

	// use RemoteAddr
	return NormalizeIP(r.RemoteAddr)
}

// GetRouter getrouting
func (h *Handler) GetRouter() *mux.Router {
	return h.router
}

// GetValidator getverify
func (h *Handler) GetValidator() *Validator {
	return h.validator
}

// GetScorer getscorer
func (h *Handler) GetScorer() *Scorer {
	return h.scorer
}

// GetDistributor returns the distributor
func (h *Handler) GetDistributor() *Distributor {
	return h.distributor
}
