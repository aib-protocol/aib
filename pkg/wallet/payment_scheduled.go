// Package wallet provides payment routing and L1/L2 transaction management.
// This file implements scheduled (time-based) payment functionality.
package wallet

import (
	"fmt"
	"sync"
	"time"
)

// ScheduledStatus represents the status of a scheduled payment.
type ScheduledStatus int

const (
	// ScheduledPending means the payment is waiting to be executed.
	ScheduledPending ScheduledStatus = iota
	// ScheduledExecuted means the payment has been executed.
	ScheduledExecuted
	// ScheduledFailed means the payment execution failed.
	ScheduledFailed
	// ScheduledCancelled means the payment was cancelled.
	ScheduledCancelled
)

// String returns the string representation of scheduled status.
func (ss ScheduledStatus) String() string {
	switch ss {
	case ScheduledPending:
		return "pending"
	case ScheduledExecuted:
		return "executed"
	case ScheduledFailed:
		return "failed"
	case ScheduledCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// RecurringConfig configures recurring payments.
type RecurringConfig struct {
	Interval time.Duration // Interval between payments
	MaxCount uint64        // Maximum number of executions (0 = unlimited)
	executed uint64        // Number of executions so far
}

// ScheduledPayment represents a payment scheduled for future execution.
type ScheduledPayment struct {
	ID         string
	To         [32]byte
	Amount     uint64
	ExecuteAt  time.Time        // When to execute the payment
	Recurring  *RecurringConfig // Nil if not recurring
	Status     ScheduledStatus
	LastResult *PaymentResult
	Memo       string
	CreatedAt  time.Time
	ExecutedAt time.Time
	mu         sync.Mutex
}

// ScheduleResult represents the result of scheduling a payment.
type ScheduleResult struct {
	ScheduleID string
	Status     ScheduledStatus
	NextExecAt time.Time
}

// ScheduledPaymentManager manages scheduled payments.
type ScheduledPaymentManager struct {
	pm         *PaymentManager
	schedules  map[string]*ScheduledPayment
	stopCh     chan struct{}
	isRunning  bool
	tickerFunc func() time.Time // For testing: override current time
	mu         sync.RWMutex
}

// NewScheduledPaymentManager creates a new scheduled payment manager.
func NewScheduledPaymentManager(pm *PaymentManager) *ScheduledPaymentManager {
	return &ScheduledPaymentManager{
		pm:         pm,
		schedules:  make(map[string]*ScheduledPayment),
		stopCh:     make(chan struct{}),
		tickerFunc: time.Now,
	}
}

// SetTimeFunc sets a custom time function for testing.
func (spm *ScheduledPaymentManager) SetTimeFunc(fn func() time.Time) {
	spm.mu.Lock()
	defer spm.mu.Unlock()
	spm.tickerFunc = fn
}

// SchedulePayment creates a new scheduled payment.
func (spm *ScheduledPaymentManager) SchedulePayment(to [32]byte, amount uint64, executeAt time.Time, memo string) (*ScheduleResult, error) {
	if amount == 0 {
		return nil, fmt.Errorf("payment amount must be greater than zero")
	}

	now := spm.tickerFunc()
	if executeAt.Before(now) {
		return nil, fmt.Errorf("execution time must be in the future")
	}

	scheduleID := fmt.Sprintf("sched-%x-%d", to[:4], executeAt.UnixNano())

	sp := &ScheduledPayment{
		ID:        scheduleID,
		To:        to,
		Amount:    amount,
		ExecuteAt: executeAt,
		Status:    ScheduledPending,
		Memo:      memo,
		CreatedAt: now,
	}

	spm.mu.Lock()
	spm.schedules[scheduleID] = sp
	spm.mu.Unlock()

	return &ScheduleResult{
		ScheduleID: scheduleID,
		Status:     ScheduledPending,
		NextExecAt: executeAt,
	}, nil
}

// ScheduleRecurringPayment creates a recurring scheduled payment.
func (spm *ScheduledPaymentManager) ScheduleRecurringPayment(to [32]byte, amount uint64, startAt time.Time, interval time.Duration, maxCount uint64, memo string) (*ScheduleResult, error) {
	if amount == 0 {
		return nil, fmt.Errorf("payment amount must be greater than zero")
	}

	if interval <= 0 {
		return nil, fmt.Errorf("interval must be positive")
	}

	now := spm.tickerFunc()
	if startAt.Before(now) {
		return nil, fmt.Errorf("start time must be in the future")
	}

	scheduleID := fmt.Sprintf("rsched-%x-%d", to[:4], startAt.UnixNano())

	sp := &ScheduledPayment{
		ID:        scheduleID,
		To:        to,
		Amount:    amount,
		ExecuteAt: startAt,
		Recurring: &RecurringConfig{
			Interval: interval,
			MaxCount: maxCount,
		},
		Status:    ScheduledPending,
		Memo:      memo,
		CreatedAt: now,
	}

	spm.mu.Lock()
	spm.schedules[scheduleID] = sp
	spm.mu.Unlock()

	return &ScheduleResult{
		ScheduleID: scheduleID,
		Status:     ScheduledPending,
		NextExecAt: startAt,
	}, nil
}

// ExecuteDue executes all due scheduled payments.
// Returns the list of payment results for executed payments.
func (spm *ScheduledPaymentManager) ExecuteDue() []*PaymentResult {
	now := spm.tickerFunc()

	spm.mu.RLock()
	duePayments := make([]*ScheduledPayment, 0)
	for _, sp := range spm.schedules {
		sp.mu.Lock()
		if sp.Status == ScheduledPending && !sp.ExecuteAt.After(now) {
			duePayments = append(duePayments, sp)
		}
		sp.mu.Unlock()
	}
	spm.mu.RUnlock()

	results := make([]*PaymentResult, 0, len(duePayments))
	for _, sp := range duePayments {
		result := spm.executePayment(sp)
		results = append(results, result)
	}

	return results
}

// executePayment executes a single scheduled payment.
func (spm *ScheduledPaymentManager) executePayment(sp *ScheduledPayment) *PaymentResult {
	result := spm.pm.SmartSend(sp.To, sp.Amount)

	sp.mu.Lock()
	defer sp.mu.Unlock()

	now := spm.tickerFunc()
	sp.LastResult = result
	sp.ExecutedAt = now

	if result.Success {
		// Check if recurring
		if sp.Recurring != nil {
			sp.Recurring.executed++
			if sp.Recurring.MaxCount > 0 && sp.Recurring.executed >= sp.Recurring.MaxCount {
				sp.Status = ScheduledExecuted
			} else {
				// Schedule next execution
				sp.ExecuteAt = now.Add(sp.Recurring.Interval)
				sp.Status = ScheduledPending
			}
		} else {
			sp.Status = ScheduledExecuted
		}
	} else {
		sp.Status = ScheduledFailed
	}

	return result
}

// CancelSchedule cancels a scheduled payment.
func (spm *ScheduledPaymentManager) CancelSchedule(scheduleID string) error {
	spm.mu.RLock()
	sp, exists := spm.schedules[scheduleID]
	spm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.Status != ScheduledPending {
		return fmt.Errorf("cannot cancel schedule with status: %s", sp.Status)
	}

	sp.Status = ScheduledCancelled
	return nil
}

// GetSchedule retrieves a scheduled payment by ID.
func (spm *ScheduledPaymentManager) GetSchedule(scheduleID string) (*ScheduledPayment, error) {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	sp, exists := spm.schedules[scheduleID]
	if !exists {
		return nil, fmt.Errorf("schedule not found: %s", scheduleID)
	}

	return sp, nil
}

// ListSchedules returns all scheduled payments.
func (spm *ScheduledPaymentManager) ListSchedules() []*ScheduledPayment {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	schedules := make([]*ScheduledPayment, 0, len(spm.schedules))
	for _, sp := range spm.schedules {
		schedules = append(schedules, sp)
	}

	return schedules
}

// ListPending returns all pending scheduled payments.
func (spm *ScheduledPaymentManager) ListPending() []*ScheduledPayment {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	pending := make([]*ScheduledPayment, 0)
	for _, sp := range spm.schedules {
		sp.mu.Lock()
		if sp.Status == ScheduledPending {
			pending = append(pending, sp)
		}
		sp.mu.Unlock()
	}

	return pending
}

// StartAutoExecutor starts a background goroutine that automatically executes
// due payments at the given check interval.
func (spm *ScheduledPaymentManager) StartAutoExecutor(checkInterval time.Duration) error {
	spm.mu.Lock()
	if spm.isRunning {
		spm.mu.Unlock()
		return fmt.Errorf("auto executor is already running")
	}
	spm.isRunning = true
	spm.stopCh = make(chan struct{})
	spm.mu.Unlock()

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				spm.ExecuteDue()
			case <-spm.stopCh:
				return
			}
		}
	}()

	return nil
}

// StopAutoExecutor stops the background auto executor.
func (spm *ScheduledPaymentManager) StopAutoExecutor() {
	spm.mu.Lock()
	defer spm.mu.Unlock()

	if spm.isRunning {
		close(spm.stopCh)
		spm.isRunning = false
	}
}

// IsRunning returns whether the auto executor is running.
func (spm *ScheduledPaymentManager) IsRunning() bool {
	spm.mu.RLock()
	defer spm.mu.RUnlock()
	return spm.isRunning
}
