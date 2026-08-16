// Package wallet provides payment routing and L1/L2 transaction management.
// This file implements refund functionality.
package wallet

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// RefundStatus represents the status of a refund.
type RefundStatus int

const (
	// RefundPending means the refund is waiting to be processed.
	RefundPending RefundStatus = iota
	// RefundProcessing means the refund is being processed.
	RefundProcessing
	// RefundCompleted means the refund was successful.
	RefundCompleted
	// RefundFailed means the refund failed.
	RefundFailed
	// RefundRejected means the refund was rejected.
	RefundRejected
)

// String returns the string representation of refund status.
func (rs RefundStatus) String() string {
	switch rs {
	case RefundPending:
		return "pending"
	case RefundProcessing:
		return "processing"
	case RefundCompleted:
		return "completed"
	case RefundFailed:
		return "failed"
	case RefundRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// RefundRequest represents a request to refund a payment.
type RefundRequest struct {
	OriginalTxHash [32]byte
	RefundTo       [32]byte
	Amount         uint64
	Reason         string
}

// RefundResult represents the result of a refund operation.
type RefundResult struct {
	RefundID       string
	OriginalTxHash [32]byte
	RefundTxHash   [32]byte
	RefundTo       [32]byte
	Amount         uint64
	Fee            uint64
	Status         RefundStatus
	Reason         string
	Method         PaymentMethod
	CreatedAt      time.Time
	CompletedAt    time.Time
	Error          string
}

// String returns a human-readable refund result summary.
func (rr *RefundResult) String() string {
	if rr.Status == RefundCompleted {
		return fmt.Sprintf("Refund %s: Amount=%d, Fee=%d, Method=%s, RefundTx=%x",
			rr.RefundID, rr.Amount, rr.Fee, rr.Method, rr.RefundTxHash[:8])
	}
	return fmt.Sprintf("Refund %s: Status=%s, Error=%s", rr.RefundID, rr.Status, rr.Error)
}

// RefundManager manages refund operations.
type RefundManager struct {
	pm         *PaymentManager
	refunds    map[string]*RefundResult
	timeFunc   func() time.Time
	mu         sync.RWMutex
}

// NewRefundManager creates a new refund manager.
func NewRefundManager(pm *PaymentManager) *RefundManager {
	return &RefundManager{
		pm:       pm,
		refunds:  make(map[string]*RefundResult),
		timeFunc: time.Now,
	}
}

// SetTimeFunc sets a custom time function for testing.
func (rm *RefundManager) SetTimeFunc(fn func() time.Time) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.timeFunc = fn
}

// generateRefundID generates a unique refund ID.
func generateRefundID(originalTxHash [32]byte, amount uint64) string {
	h := sha256.New()
	h.Write(originalTxHash[:])
	buf := make([]byte, 8)
	buf[0] = byte(amount)
	buf[1] = byte(amount >> 8)
	buf[2] = byte(amount >> 16)
	buf[3] = byte(amount >> 24)
	h.Write(buf)
	hash := h.Sum(nil)
	return fmt.Sprintf("refund-%x", hash[:8])
}

// RequestRefund initiates a refund for a previous payment.
// The refund uses L1 or L2 based on the smart routing logic.
func (rm *RefundManager) RequestRefund(req *RefundRequest) (*RefundResult, error) {
	if req == nil {
		return nil, fmt.Errorf("refund request cannot be nil")
	}

	if req.Amount == 0 {
		return nil, fmt.Errorf("refund amount must be greater than zero")
	}

	if req.OriginalTxHash == [32]byte{} {
		return nil, fmt.Errorf("original transaction hash cannot be empty")
	}

	if req.RefundTo == [32]byte{} {
		return nil, fmt.Errorf("refund recipient address cannot be empty")
	}

	refundID := generateRefundID(req.OriginalTxHash, req.Amount)

	rm.mu.Lock()
	// Check for duplicate refund
	if existingRefund, exists := rm.refunds[refundID]; exists {
		rm.mu.Unlock()
		if existingRefund.Status == RefundCompleted {
			return nil, fmt.Errorf("refund already completed: %s", refundID)
		}
		// If previous attempt failed, allow retry
		if existingRefund.Status != RefundFailed {
			return nil, fmt.Errorf("refund already in progress: %s", refundID)
		}
	}

	now := rm.timeFunc()
	result := &RefundResult{
		RefundID:       refundID,
		OriginalTxHash: req.OriginalTxHash,
		RefundTo:       req.RefundTo,
		Amount:         req.Amount,
		Status:         RefundProcessing,
		Reason:         req.Reason,
		CreatedAt:      now,
	}
	rm.refunds[refundID] = result
	rm.mu.Unlock()

	// Execute the refund payment using smart routing
	paymentResult := rm.pm.SmartSend(req.RefundTo, req.Amount)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if paymentResult.Success {
		result.RefundTxHash = paymentResult.TxHash
		result.Fee = paymentResult.Fee
		result.Method = paymentResult.Method
		result.Status = RefundCompleted
		result.CompletedAt = rm.timeFunc()
	} else {
		result.Status = RefundFailed
		result.Error = paymentResult.Error
	}

	return result, nil
}

// RequestRefundL1 forces a refund through L1 (on-chain).
func (rm *RefundManager) RequestRefundL1(req *RefundRequest) (*RefundResult, error) {
	if req == nil {
		return nil, fmt.Errorf("refund request cannot be nil")
	}

	if req.Amount == 0 {
		return nil, fmt.Errorf("refund amount must be greater than zero")
	}

	if req.OriginalTxHash == [32]byte{} {
		return nil, fmt.Errorf("original transaction hash cannot be empty")
	}

	refundID := generateRefundID(req.OriginalTxHash, req.Amount)

	now := rm.timeFunc()
	result := &RefundResult{
		RefundID:       refundID,
		OriginalTxHash: req.OriginalTxHash,
		RefundTo:       req.RefundTo,
		Amount:         req.Amount,
		Status:         RefundProcessing,
		Reason:         req.Reason,
		CreatedAt:      now,
		Method:         PaymentL1,
	}

	rm.mu.Lock()
	rm.refunds[refundID] = result
	rm.mu.Unlock()

	// Execute through L1
	paymentResult := rm.pm.SendL1(req.RefundTo, req.Amount)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if paymentResult.Success {
		result.RefundTxHash = paymentResult.TxHash
		result.Fee = paymentResult.Fee
		result.Status = RefundCompleted
		result.CompletedAt = rm.timeFunc()
	} else {
		result.Status = RefundFailed
		result.Error = paymentResult.Error
	}

	return result, nil
}

// RequestRefundL2 forces a refund through L2 (channel).
func (rm *RefundManager) RequestRefundL2(req *RefundRequest, channelID string) (*RefundResult, error) {
	if req == nil {
		return nil, fmt.Errorf("refund request cannot be nil")
	}

	if req.Amount == 0 {
		return nil, fmt.Errorf("refund amount must be greater than zero")
	}

	if channelID == "" {
		return nil, fmt.Errorf("channel ID cannot be empty")
	}

	refundID := generateRefundID(req.OriginalTxHash, req.Amount)

	now := rm.timeFunc()
	result := &RefundResult{
		RefundID:       refundID,
		OriginalTxHash: req.OriginalTxHash,
		RefundTo:       req.RefundTo,
		Amount:         req.Amount,
		Status:         RefundProcessing,
		Reason:         req.Reason,
		CreatedAt:      now,
		Method:         PaymentL2,
	}

	rm.mu.Lock()
	rm.refunds[refundID] = result
	rm.mu.Unlock()

	// Execute through L2
	paymentResult := rm.pm.SendL2(req.RefundTo, req.Amount, channelID)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if paymentResult.Success {
		result.RefundTxHash = paymentResult.TxHash
		result.Fee = paymentResult.Fee
		result.Status = RefundCompleted
		result.CompletedAt = rm.timeFunc()
	} else {
		result.Status = RefundFailed
		result.Error = paymentResult.Error
	}

	return result, nil
}

// GetRefund retrieves a refund by ID.
func (rm *RefundManager) GetRefund(refundID string) (*RefundResult, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	refund, exists := rm.refunds[refundID]
	if !exists {
		return nil, fmt.Errorf("refund not found: %s", refundID)
	}

	return refund, nil
}

// ListRefunds returns all refunds.
func (rm *RefundManager) ListRefunds() []*RefundResult {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	refunds := make([]*RefundResult, 0, len(rm.refunds))
	for _, r := range rm.refunds {
		refunds = append(refunds, r)
	}

	return refunds
}

// ListRefundsByStatus returns refunds filtered by status.
func (rm *RefundManager) ListRefundsByStatus(status RefundStatus) []*RefundResult {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	refunds := make([]*RefundResult, 0)
	for _, r := range rm.refunds {
		if r.Status == status {
			refunds = append(refunds, r)
		}
	}

	return refunds
}

// GetRefundsByOriginalTx returns all refunds for a given original transaction.
func (rm *RefundManager) GetRefundsByOriginalTx(txHash [32]byte) []*RefundResult {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	refunds := make([]*RefundResult, 0)
	for _, r := range rm.refunds {
		if r.OriginalTxHash == txHash {
			refunds = append(refunds, r)
		}
	}

	return refunds
}

// PartialRefund creates a partial refund for a percentage of the original amount.
// The percentage is in basis points (e.g., 5000 = 50%).
func (rm *RefundManager) PartialRefund(originalTxHash [32]byte, originalAmount uint64, refundTo [32]byte, basisPoints uint64, reason string) (*RefundResult, error) {
	if basisPoints > 10000 {
		return nil, fmt.Errorf("basis points cannot exceed 10000 (100%%)")
	}

	if basisPoints == 0 {
		return nil, fmt.Errorf("basis points must be greater than zero")
	}

	refundAmount := (originalAmount * basisPoints) / 10000

	return rm.RequestRefund(&RefundRequest{
		OriginalTxHash: originalTxHash,
		RefundTo:       refundTo,
		Amount:         refundAmount,
		Reason:         reason,
	})
}
