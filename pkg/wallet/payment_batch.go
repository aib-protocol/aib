// Package wallet provides payment routing and L1/L2 transaction management.
// This file implements batch payment functionality.
package wallet

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// BatchStatus represents the status of a batch payment.
type BatchStatus int

const (
	// BatchPending indicates the batch is pending execution.
	BatchPending BatchStatus = iota
	// BatchInProgress indicates the batch is being processed.
	BatchInProgress
	// BatchCompleted indicates all payments in the batch were successful.
	BatchCompleted
	// BatchPartialFailure indicates some payments failed.
	BatchPartialFailure
	// BatchFailed indicates all payments failed.
	BatchFailed
)

// String returns the string representation of batch status.
func (bs BatchStatus) String() string {
	switch bs {
	case BatchPending:
		return "pending"
	case BatchInProgress:
		return "in_progress"
	case BatchCompleted:
		return "completed"
	case BatchPartialFailure:
		return "partial_failure"
	case BatchFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// PaymentRequest represents a single payment request in a batch.
type PaymentRequest struct {
	To     [32]byte
	Amount uint64
	Memo   string
}

// BatchPayment represents a batch of payments to be executed together.
type BatchPayment struct {
	ID           string
	Payments     []PaymentRequest
	CreatedAt    uint64
	Status       BatchStatus
	TotalAmount  uint64
	TotalFee     uint64
	SuccessCount int
	FailedCount  int
	mu           sync.Mutex
}

// BatchResult represents the result of a batch payment operation.
type BatchResult struct {
	BatchID       string
	Results       []*PaymentResult
	TotalAmount   uint64
	TotalFee      uint64
	SuccessCount  int
	FailedCount   int
	FailedIndices []int // Indices of failed payments
}

// String returns a human-readable summary of batch result.
func (br *BatchResult) String() string {
	return fmt.Sprintf("BatchResult: Total=%d, Success=%d, Failed=%d, Fee=%d",
		br.TotalAmount, br.SuccessCount, br.FailedCount, br.TotalFee)
}

// BatchPaymentManager manages batch payment operations.
type BatchPaymentManager struct {
	pm            *PaymentManager
	batches       map[string]*BatchPayment
	executeQueue  chan *BatchPayment
	maxConcurrent int
	mu            sync.RWMutex
}

// NewBatchPaymentManager creates a new batch payment manager.
func NewBatchPaymentManager(pm *PaymentManager, maxConcurrent int) *BatchPaymentManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	return &BatchPaymentManager{
		pm:            pm,
		batches:       make(map[string]*BatchPayment),
		executeQueue:  make(chan *BatchPayment, 100),
		maxConcurrent: maxConcurrent,
	}
}

// CreateBatchPayment creates a new batch payment with the given requests.
func (bpm *BatchPaymentManager) CreateBatchPayment(requests []PaymentRequest) (*BatchPayment, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("no payment requests provided")
	}

	var totalAmount uint64
	for _, req := range requests {
		if req.Amount == 0 {
			return nil, fmt.Errorf("payment amount must be greater than zero")
		}
		totalAmount += req.Amount
	}

	batchID := generateBatchID(requests)

	batch := &BatchPayment{
		ID:          batchID,
		Payments:    requests,
		CreatedAt:   uint64(time.Now().Unix()),
		Status:      BatchPending,
		TotalAmount: totalAmount,
	}

	bpm.mu.Lock()
	defer bpm.mu.Unlock()
	bpm.batches[batchID] = batch

	return batch, nil
}

// generateBatchID generates a unique batch ID from payment requests.
func generateBatchID(requests []PaymentRequest) string {
	h := sha256.New()
	for _, req := range requests {
		h.Write(req.To[:])
		binary.Write(h, binary.LittleEndian, req.Amount)
	}
	hash := h.Sum(nil)
	return fmt.Sprintf("batch-%x", hash[:8])
}

// ExecuteBatch executes all payments in the batch using smart routing.
// It uses L2 for small amounts and L1 for larger amounts.
func (bpm *BatchPaymentManager) ExecuteBatch(batchID string) (*BatchResult, error) {
	bpm.mu.RLock()
	batch, exists := bpm.batches[batchID]
	bpm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	batch.mu.Lock()
	if batch.Status == BatchInProgress {
		batch.mu.Unlock()
		return nil, fmt.Errorf("batch already in progress")
	}
	batch.Status = BatchInProgress
	batch.mu.Unlock()

	result := &BatchResult{
		BatchID: batchID,
		Results: make([]*PaymentResult, len(batch.Payments)),
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, bpm.maxConcurrent)

	for i, req := range batch.Payments {
		wg.Add(1)
		go func(index int, payment PaymentRequest) {
			defer wg.Done()

			// Acquire semaphore for concurrent execution limit
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Use smart routing for each payment
			paymentResult := bpm.pm.SmartSend(payment.To, payment.Amount)

			result.Results[index] = paymentResult
			result.TotalAmount += payment.Amount

			if paymentResult.Success {
				result.SuccessCount++
				batch.mu.Lock()
				batch.SuccessCount++
				batch.mu.Unlock()
			} else {
				result.FailedCount++
				result.FailedIndices = append(result.FailedIndices, index)
				batch.mu.Lock()
				batch.FailedCount++
				batch.mu.Unlock()
			}

			result.TotalFee += paymentResult.Fee
		}(i, req)
	}

	wg.Wait()

	// Update batch status
	batch.mu.Lock()
	if result.FailedCount == 0 {
		batch.Status = BatchCompleted
	} else if result.SuccessCount == 0 {
		batch.Status = BatchFailed
	} else {
		batch.Status = BatchPartialFailure
	}
	batch.TotalFee = result.TotalFee
	batch.mu.Unlock()

	return result, nil
}

// ExecuteBatchSequential executes payments in sequential order.
func (bpm *BatchPaymentManager) ExecuteBatchSequential(batchID string) (*BatchResult, error) {
	bpm.mu.RLock()
	batch, exists := bpm.batches[batchID]
	bpm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	batch.mu.Lock()
	if batch.Status == BatchInProgress {
		batch.mu.Unlock()
		return nil, fmt.Errorf("batch already in progress")
	}
	batch.Status = BatchInProgress
	batch.mu.Unlock()

	result := &BatchResult{
		BatchID: batchID,
		Results: make([]*PaymentResult, len(batch.Payments)),
	}

	for i, req := range batch.Payments {
		paymentResult := bpm.pm.SmartSend(req.To, req.Amount)
		result.Results[i] = paymentResult
		result.TotalAmount += req.Amount

		if paymentResult.Success {
			result.SuccessCount++
			batch.SuccessCount++
		} else {
			result.FailedCount++
			result.FailedIndices = append(result.FailedIndices, i)
			batch.FailedCount++
		}

		result.TotalFee += paymentResult.Fee
	}

	// Update batch status
	batch.mu.Lock()
	if result.FailedCount == 0 {
		batch.Status = BatchCompleted
	} else if result.SuccessCount == 0 {
		batch.Status = BatchFailed
	} else {
		batch.Status = BatchPartialFailure
	}
	batch.TotalFee = result.TotalFee
	batch.mu.Unlock()

	return result, nil
}

// GetBatch retrieves a batch by ID.
func (bpm *BatchPaymentManager) GetBatch(batchID string) (*BatchPayment, error) {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	batch, exists := bpm.batches[batchID]
	if !exists {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	return batch, nil
}

// GetBatchStatus returns the current status of a batch.
func (bpm *BatchPaymentManager) GetBatchStatus(batchID string) (BatchStatus, error) {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	batch, exists := bpm.batches[batchID]
	if !exists {
		return BatchPending, fmt.Errorf("batch not found: %s", batchID)
	}

	batch.mu.Lock()
	defer batch.mu.Unlock()
	return batch.Status, nil
}

// ListBatches returns all batch payments.
func (bpm *BatchPaymentManager) ListBatches() []*BatchPayment {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	batches := make([]*BatchPayment, 0, len(bpm.batches))
	for _, batch := range bpm.batches {
		batches = append(batches, batch)
	}

	return batches
}

// CancelBatch attempts to cancel a pending batch.
func (bpm *BatchPaymentManager) CancelBatch(batchID string) error {
	bpm.mu.RLock()
	batch, exists := bpm.batches[batchID]
	bpm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("batch not found: %s", batchID)
	}

	batch.mu.Lock()
	defer batch.mu.Unlock()

	if batch.Status == BatchInProgress {
		return fmt.Errorf("cannot cancel batch in progress")
	}

	if batch.Status == BatchCompleted || batch.Status == BatchFailed {
		return fmt.Errorf("cannot cancel completed or failed batch")
	}

	batch.Status = BatchFailed
	return nil
}

// EstimateBatchFee estimates the total fee for a batch of payments.
func (bpm *BatchPaymentManager) EstimateBatchFee(requests []PaymentRequest) (l1Fee, l2Fee uint64, err error) {
	l1Count := 0
	l2Count := 0

	for _, req := range requests {
		if req.Amount < bpm.pm.l2Threshold {
			l2Count++
		} else {
			l1Count++
		}
	}

	// Estimate L1 fee (200 bytes per tx * feePerByte)
	l1Fee = uint64(l1Count) * 200 * bpm.pm.feePerByte

	// Estimate L2 fee (0.1% fee rate)
	for _, req := range requests {
		if req.Amount < bpm.pm.l2Threshold {
			l2Fee += (req.Amount * 10) / 10000 // 0.1%
		}
	}

	return l1Fee, l2Fee, nil
}
