// Package founder implements the founder allocation system for AIB 2.0.
// This file implements REST API handlers for founder operations.
package founder

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// Handler provides HTTP handlers for founder operations.
type Handler struct {
	manager   *AllocationManager
	verifier  *Verifier
	utxoStore *utxo.UTXOStore
	mu        http.Handler
}

// NewHandler creates a new founder API handler.
func NewHandler(manager *AllocationManager, verifier *Verifier, utxoStore *utxo.UTXOStore) *Handler {
	return &Handler{
		manager:   manager,
		verifier:  verifier,
		utxoStore: utxoStore,
	}
}

// writeJSON writes JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeSuccess writes a success response.
func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, code, message, details string) {
	writeJSON(w, status, map[string]interface{}{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
			"details": details,
		},
	})
}

// RegisterRoutes registers all founder API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Founder list
	mux.HandleFunc("/api/founder/list", h.handleFounderList)

	// Vesting information
	mux.HandleFunc("/api/founder/vesting/", h.handleVestingInfo)

	// Claim operations
	mux.HandleFunc("/api/founder/claim", h.handleClaim)
	mux.HandleFunc("/api/founder/claimable/", h.handleClaimable)

	// Multi-sig operations
	mux.HandleFunc("/api/founder/release/request", h.handleReleaseRequest)
	mux.HandleFunc("/api/founder/release/sign", h.handleReleaseSign)
	mux.HandleFunc("/api/founder/release/status/", h.handleReleaseStatus)

	// Statistics
	mux.HandleFunc("/api/founder/stats", h.handleStats)

	// Multi-sig management
	mux.HandleFunc("/api/founder/multisig/signers", h.handleMultiSigSigners)
}

// FounderListResponse is the response for founder list.
type FounderListResponse struct {
	Founders     []FounderInfo `json:"founders"`
	TotalCount   int           `json:"total_count"`
	TotalAmount  uint64        `json:"total_amount"`
	Version      uint64        `json:"version"`
}

// FounderInfo contains public information about a founder.
type FounderInfo struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Address     string             `json:"address"`
	TotalAmount uint64             `json:"total_amount"`
	Status      string             `json:"status"`
	StartTime   time.Time          `json:"start_time"`
	UnlockTime  time.Time          `json:"unlock_time"`
	EndTime     time.Time          `json:"end_time"`
	Metadata    FounderMetadata    `json:"metadata"`
}

// handleFounderList returns the list of founders.
func (h *Handler) handleFounderList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	founders := h.manager.founders.List()

	response := FounderListResponse{
		Founders:    make([]FounderInfo, len(founders)),
		TotalCount:  len(founders),
		TotalAmount: h.manager.founders.TotalAllocated(),
		Version:     h.manager.founders.Version,
	}

	for i, f := range founders {
		response.Founders[i] = FounderInfo{
			ID:          f.ID,
			Name:        f.Name,
			Address:     f.Address,
			TotalAmount: f.TotalAmount,
			Status:      string(f.Status),
			StartTime:   f.StartTime,
			UnlockTime:  f.UnlockTime,
			EndTime:     f.EndTime,
			Metadata:    f.Metadata,
		}
	}

	writeSuccess(w, response)
}

// handleVestingInfo returns vesting information for a founder.
func (h *Handler) handleVestingInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	// Extract founder ID from path
	founderID := strings.TrimPrefix(r.URL.Path, "/api/founder/vesting/")
	if founderID == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "founder ID required", "")
		return
	}

	info, err := h.manager.GetVestingInfo(founderID)
	if err != nil {
		writeError(w, http.StatusNotFound, ErrCodeNotFound, err.Error(), "")
		return
	}

	writeSuccess(w, info)
}

// handleClaimable returns the claimable amount for a founder.
func (h *Handler) handleClaimable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	// Extract founder ID from path
	founderID := strings.TrimPrefix(r.URL.Path, "/api/founder/claimable/")
	if founderID == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "founder ID required", "")
		return
	}

	amount, err := h.manager.GetClaimableAmount(founderID)
	if err != nil {
		writeError(w, http.StatusNotFound, ErrCodeNotFound, err.Error(), "")
		return
	}

	writeSuccess(w, map[string]interface{}{
		"founder_id":        founderID,
		"claimable_amount":  amount,
	})
}

// ClaimRequest is the request for claiming tokens.
type ClaimRequest struct {
	FounderID  string `json:"founder_id"`
	Amount     uint64 `json:"amount"`
	PublicKey  string `json:"public_key"`  // Base64 encoded
	Signature  string `json:"signature"`  // Base64 encoded
	Nonce      uint64 `json:"nonce"`
}

// ClaimResponse is the response for a claim.
type ClaimResponse struct {
	TxHash     string    `json:"tx_hash"`
	FounderID  string    `json:"founder_id"`
	Amount     uint64    `json:"amount"`
	Timestamp  time.Time `json:"timestamp"`
}

// handleClaim processes a token claim request.
func (h *Handler) handleClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	var req ClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body", err.Error())
		return
	}

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid signature encoding", err.Error())
		return
	}

	// Verify founder signature
	message := fmt.Sprintf("CLAIM:%s:%d:%d", req.FounderID, req.Amount, req.Nonce)
	if err := h.verifier.VerifyFounderIdentity(req.FounderID, []byte(message), signature); err != nil {
		writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "signature verification failed", err.Error())
		return
	}

	// Create transaction
	tx, err := h.manager.CreateClaimTransaction(req.FounderID, req.Amount, h.utxoStore)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "failed to create transaction", err.Error())
		return
	}

	// Process claim
	txHashBytes := tx.Hash()
	txHash := hex.EncodeToString(txHashBytes[:])
	if err := h.manager.ClaimTokens(req.FounderID, req.Amount, txHash, 0); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "claim failed", err.Error())
		return
	}

	response := ClaimResponse{
		TxHash:    txHash,
		FounderID: req.FounderID,
		Amount:    req.Amount,
		Timestamp: time.Now(),
	}

	writeSuccess(w, response)
}

// ReleaseRequestRequest is the request for creating a release request.
type ReleaseRequestRequest struct {
	FounderID string `json:"founder_id"`
	Amount    uint64 `json:"amount"`
	Nonce     uint64 `json:"nonce"`
}

// ReleaseRequestResponse is the response for a release request.
type ReleaseRequestResponse struct {
	RequestID    string    `json:"request_id"` // founderID:nonce
	FounderID    string    `json:"founder_id"`
	Amount       uint64    `json:"amount"`
	Status       string    `json:"status"`
	RequiredSigs int       `json:"required_sigs"`
	CreatedAt    time.Time `json:"created_at"`
}

// handleReleaseRequest creates a new multi-sig release request.
func (h *Handler) handleReleaseRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	var req ReleaseRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body", err.Error())
		return
	}

	record, err := h.verifier.CreateReleaseRequest(req.FounderID, req.Amount, req.Nonce)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "failed to create release request", err.Error())
		return
	}

	response := ReleaseRequestResponse{
		RequestID:    fmt.Sprintf("%s:%d", req.FounderID, req.Nonce),
		FounderID:    req.FounderID,
		Amount:       req.Amount,
		Status:       string(record.Status),
		RequiredSigs: h.verifier.multiSig.RequiredSigs,
		CreatedAt:    record.CreatedAt,
	}

	writeSuccess(w, response)
}

// SignReleaseRequest is the request for signing a release.
type SignReleaseRequest struct {
	FounderID     string `json:"founder_id"`
	Nonce         uint64 `json:"nonce"`
	SignerAddress string `json:"signer_address"`
	Signature     string `json:"signature"` // Hex encoded
}

// SignReleaseResponse is the response for signing a release.
type SignReleaseResponse struct {
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`
	SigsCollected int  `json:"sigs_collected"`
	SigsRequired  int  `json:"sigs_required"`
}

// handleReleaseSign adds a signature to a release request.
func (h *Handler) handleReleaseSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	var req SignReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body", err.Error())
		return
	}

	// Decode signature
	signature, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid signature encoding", err.Error())
		return
	}

	// Add signature
	if err := h.verifier.AddSignature(req.FounderID, req.Nonce, req.SignerAddress, signature); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "failed to add signature", err.Error())
		return
	}

	// Get updated record
	record, err := h.verifier.GetReleaseRequest(req.FounderID, req.Nonce)
	if err != nil {
		writeError(w, http.StatusNotFound, ErrCodeNotFound, "release request not found", err.Error())
		return
	}

	response := SignReleaseResponse{
		RequestID:      fmt.Sprintf("%s:%d", req.FounderID, req.Nonce),
		Status:         string(record.Status),
		SigsCollected:  len(record.Signatures),
		SigsRequired:   h.verifier.multiSig.RequiredSigs,
	}

	writeSuccess(w, response)
}

// handleReleaseStatus returns the status of a release request.
func (h *Handler) handleReleaseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	// Extract request ID from path (format: founderID:nonce)
	requestID := strings.TrimPrefix(r.URL.Path, "/api/founder/release/status/")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "request ID required", "")
		return
	}

	parts := strings.Split(requestID, ":")
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request ID format", "")
		return
	}

	nonce, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid nonce", err.Error())
		return
	}

	record, err := h.verifier.GetReleaseRequest(parts[0], nonce)
	if err != nil {
		writeError(w, http.StatusNotFound, ErrCodeNotFound, err.Error(), "")
		return
	}

	// Build signer info
	signers := make([]map[string]interface{}, len(record.Signatures))
	for i, sig := range record.Signatures {
		addrStr, _ := utxo.AddressToString(sig.SignerAddress)
		signers[i] = map[string]interface{}{
			"address":   addrStr,
			"timestamp": sig.Timestamp,
		}
	}

	writeSuccess(w, map[string]interface{}{
		"request_id":      requestID,
		"founder_id":      record.FounderID,
		"amount":          record.Amount,
		"status":          string(record.Status),
		"signers":         signers,
		"sigs_collected":  len(record.Signatures),
		"sigs_required":   h.verifier.multiSig.RequiredSigs,
		"created_at":      record.CreatedAt,
		"completed_at":    record.CompletedAt,
		"tx_hash":         record.TxHash,
	})
}

// handleStats returns allocation statistics.
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
		return
	}

	stats := h.manager.GetAllocationStats()
	writeSuccess(w, stats)
}

// handleMultiSigSigners manages authorized signers.
func (h *Handler) handleMultiSigSigners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getMultiSigSigners(w, r)
	case http.MethodPost:
		h.addMultiSigSigner(w, r)
	case http.MethodDelete:
		h.removeMultiSigSigner(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "method not allowed", "")
	}
}

// getMultiSigSigners returns the list of authorized signers.
func (h *Handler) getMultiSigSigners(w http.ResponseWriter, r *http.Request) {
	signers := h.verifier.GetAuthorizedSigners()

	writeSuccess(w, map[string]interface{}{
		"signers":       signers,
		"required_sigs": h.verifier.multiSig.RequiredSigs,
	})
}

// AddSignerRequest is the request for adding a signer.
type AddSignerRequest struct {
	Address string `json:"address"`
}

// addMultiSigSigner adds a new authorized signer.
func (h *Handler) addMultiSigSigner(w http.ResponseWriter, r *http.Request) {
	var req AddSignerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body", err.Error())
		return
	}

	if err := h.verifier.AddAuthorizedSigner(req.Address); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "failed to add signer", err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Signer added successfully",
		"address": req.Address,
	})
}

// removeMultiSigSigner removes an authorized signer.
func (h *Handler) removeMultiSigSigner(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "address parameter required", "")
		return
	}

	if err := h.verifier.RemoveAuthorizedSigner(address); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "failed to remove signer", err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Signer removed successfully",
		"address": address,
	})
}

// Error codes for founder API
const (
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeFounderNotFound    = "FOUNDER_NOT_FOUND"
	ErrCodeInvalidClaim       = "INVALID_CLAIM"
	ErrCodeInsufficientVested = "INSUFFICIENT_VESTED"
	ErrCodeReleaseNotFound    = "RELEASE_NOT_FOUND"
	ErrCodeUnauthorizedSigner = "UNAUTHORIZED_SIGNER"
	ErrCodeDuplicateSignature = "DUPLICATE_SIGNATURE"
)
