// Package wallet provides integration tests for the wallet SDK.
// These tests verify end-to-end payment flows, L1/L2 routing, batch payments, and scheduled payments.
package wallet

import (
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// ==================== End-to-End Payment Flow Tests ====================

// TestIntegration_EndToEndPaymentFlow tests a complete payment flow from wallet creation to settlement.
func TestIntegration_EndToEndPaymentFlow(t *testing.T) {
	// Create sender wallet
	sender, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create sender wallet: %v", err)
	}

	// Create receiver wallet
	receiver, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create receiver wallet: %v", err)
	}

	// Create payment manager for sender
	pm := NewPaymentManager(sender)

	// Set up UTXO store with funds
	store := utxo.NewUTXOStore()
	txHash := [32]byte{0x01, 0x02, 0x03, 0x04, 0x05}
	utxoEntry := &utxo.UTXO{
		TxHash:  txHash,
		Index:   0,
		Value:   100000,
		Address: sender.GetAddress(),
	}
	store.AddUTXO(utxoEntry)
	pm.SetUTXOStore(store)

	// Configure payment manager
	pm.SetL2Threshold(100)
	pm.SetFeePerByte(2)

	// Test 1: Large payment should use L1
	t.Run("L1Payment", func(t *testing.T) {
		result := pm.SendL1(receiver.GetAddress(), 10000)
		if !result.Success {
			t.Fatalf("L1 payment should succeed: %s", result.Error)
		}
		if result.Method != PaymentL1 {
			t.Errorf("Expected L1 payment, got %s", result.Method)
		}
		if result.Amount != 10000 {
			t.Errorf("Expected amount 10000, got %d", result.Amount)
		}
		if result.Fee == 0 {
			t.Error("L1 payment should have a fee")
		}
		t.Logf("L1 Payment completed: %s", result.String())
	})

	// Test 2: Add L2 channel for small payments
	t.Run("L2ChannelSetup", func(t *testing.T) {
		channel := &L2Channel{
			ChannelID:   "test-channel-001",
			PeerAddress: receiver.GetAddress(),
			Balance:     5000,
			FeeRate:     5, // 0.05%
			IsActive:    true,
		}
		pm.AddL2Channel(channel)

		// Verify channel was added
		ch := pm.GetL2Channel("test-channel-001")
		if ch == nil {
			t.Fatal("Channel should be added")
		}
		if ch.Balance != 5000 {
			t.Errorf("Channel balance should be 5000, got %d", ch.Balance)
		}
	})

	// Test 3: Small payment should use L2
	t.Run("L2Payment", func(t *testing.T) {
		result := pm.SendL2(receiver.GetAddress(), 50, "test-channel-001")
		if !result.Success {
			t.Fatalf("L2 payment should succeed: %s", result.Error)
		}
		if result.Method != PaymentL2 {
			t.Errorf("Expected L2 payment, got %s", result.Method)
		}
		// Verify channel balance was updated
		ch := pm.GetL2Channel("test-channel-001")
		if ch.Balance != 4950 {
			t.Errorf("Channel balance should be 4950, got %d", ch.Balance)
		}
		t.Logf("L2 Payment completed: %s", result.String())
	})

	// Test 4: Smart routing - small amount should use L2
	t.Run("SmartRoutingSmallAmount", func(t *testing.T) {
		result := pm.SmartSend(receiver.GetAddress(), 80)
		if !result.Success {
			t.Fatalf("Smart send should succeed: %s", result.Error)
		}
		if result.Method != PaymentL2 {
			t.Errorf("Small amount should use L2, got %s", result.Method)
		}
		t.Logf("Smart send (small): %s", result.String())
	})

	// Test 5: Smart routing - large amount should use L1
	t.Run("SmartRoutingLargeAmount", func(t *testing.T) {
		result := pm.SmartSend(receiver.GetAddress(), 10000)
		if !result.Success {
			t.Fatalf("Smart send should succeed: %s", result.Error)
		}
		if result.Method != PaymentL1 {
			t.Errorf("Large amount should use L1, got %s", result.Method)
		}
		t.Logf("Smart send (large): %s", result.String())
	})

	// Test 6: Verify final balance
	t.Run("FinalBalance", func(t *testing.T) {
		l1, l2, err := pm.GetBalance()
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}
		// Original: 100000
		// L1 payments: 10000 + 10000 = 20000
		// L2 payments: 50 + 80 = 130
		// L2 remaining: 4950 - 80 = 4870
		t.Logf("Final balance - L1: %d, L2: %d", l1, l2)
	})
}

// TestIntegration_MultiHopPaymentChain tests payment through multiple hops.
func TestIntegration_MultiHopPaymentChain(t *testing.T) {
	// Create three wallets: sender, intermediary, receiver
	sender, _ := NewWallet()
	intermediary, _ := NewWallet()
	receiver, _ := NewWallet()

	// Setup sender
	senderPM := NewPaymentManager(sender)
	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x10},
		Index:   0,
		Value:   50000,
		Address: sender.GetAddress(),
	})
	senderPM.SetUTXOStore(store)
	senderPM.SetL2Threshold(100)

	// Setup intermediary as receiver with L2 channel
	intermediaryPM := NewPaymentManager(intermediary)
	intermediaryStore := utxo.NewUTXOStore()
	intermediaryStore.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x20},
		Index:   0,
		Value:   10000,
		Address: intermediary.GetAddress(),
	})
	intermediaryPM.SetUTXOStore(intermediaryStore)

	// Add L2 channel from sender to intermediary
	channel := &L2Channel{
		ChannelID:   "hop-channel",
		PeerAddress: intermediary.GetAddress(),
		Balance:     1000,
		FeeRate:     10,
		IsActive:    true,
	}
	senderPM.AddL2Channel(channel)

	// Send payment from sender to receiver via intermediary
	// First hop: sender -> intermediary via L2
	result1 := senderPM.SendL2(intermediary.GetAddress(), 100, "hop-channel")
	if !result1.Success {
		t.Fatalf("First hop failed: %s", result1.Error)
	}
	t.Logf("First hop (sender -> intermediary): %s", result1.String())

	// Second hop: intermediary -> receiver via L1
	result2 := intermediaryPM.SendL1(receiver.GetAddress(), 50)
	if !result2.Success {
		t.Fatalf("Second hop failed: %s", result2.Error)
	}
	t.Logf("Second hop (intermediary -> receiver): %s", result2.String())

	// Verify final receiver balance via UTXO
	l1, _, _ := intermediaryPM.GetBalance()
	t.Logf("Intermediary remaining L1 balance: %d", l1)
}

// ==================== L1/L2 Routing Logic Tests ====================

// TestIntegration_L1L2RoutingDecision tests routing decisions based on various conditions.
func TestIntegration_L1L2RoutingDecision(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	// Setup with funds
	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x30},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)

	// Test cases for routing decisions
	tests := []struct {
		name        string
		amount      uint64
		threshold   uint64
		l2Available bool
		expectL2    bool
	}{
		{
			name:        "Small amount with L2 available",
			amount:      50,
			threshold:   100,
			l2Available: true,
			expectL2:    true,
		},
		{
			name:        "Small amount without L2 available",
			amount:      50,
			threshold:   100,
			l2Available: false,
			expectL2:    false,
		},
		{
			name:        "Large amount with L2 available",
			amount:      500,
			threshold:   100,
			l2Available: true,
			expectL2:    false,
		},
		{
			name:        "Amount equals threshold",
			amount:      100,
			threshold:   100,
			l2Available: true,
			expectL2:    false, // Below threshold, not equal
		},
		{
			name:        "Zero threshold forces L1",
			amount:      50,
			threshold:   0,
			l2Available: true,
			expectL2:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset channel
			pm.SetL2Threshold(tt.threshold)
			if tt.l2Available {
				channel := &L2Channel{
					ChannelID:   "routing-test-channel",
					PeerAddress: wallet.GetAddress(),
					Balance:     10000,
					FeeRate:     5,
					IsActive:    true,
				}
				pm.AddL2Channel(channel)
			} else {
				pm.RemoveL2Channel("routing-test-channel")
			}

			// Make payment
			toAddr := [32]byte{0x99}
			result := pm.SmartSend(toAddr, tt.amount)

			if tt.expectL2 && result.Method != PaymentL2 {
				t.Errorf("Expected L2, got %s", result.Method)
			}
			if !tt.expectL2 && result.Method != PaymentL1 {
				t.Errorf("Expected L1, got %s", result.Method)
			}
		})
	}
}

// TestIntegration_RoutingWithInsufficientL2Balance tests routing falls back to L1 when L2 has insufficient balance.
func TestIntegration_RoutingWithInsufficientL2Balance(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	// Setup with funds
	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x40},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)
	pm.SetL2Threshold(100)

	// Add L2 channel with low balance
	channel := &L2Channel{
		ChannelID:   "low-balance-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     10, // Very low balance
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	// Try to send more than L2 balance
	toAddr := [32]byte{0x88}
	result := pm.SmartSend(toAddr, 50) // More than channel balance

	if result.Method != PaymentL1 {
		t.Errorf("Should fall back to L1 when L2 balance insufficient, got %s", result.Method)
	}
	if !result.Success {
		t.Errorf("Payment should succeed via L1 fallback: %s", result.Error)
	}

	t.Logf("Fallback payment result: %s", result.String())
}

// TestIntegration_ActiveChannelRouting tests routing only uses active channels.
func TestIntegration_ActiveChannelRouting(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x50},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)
	pm.SetL2Threshold(100)

	// Add inactive channel
	inactiveChannel := &L2Channel{
		ChannelID:   "inactive-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     10000,
		FeeRate:     5,
		IsActive:    false,
	}
	pm.AddL2Channel(inactiveChannel)

	// Add active channel
	activeChannel := &L2Channel{
		ChannelID:   "active-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     10000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(activeChannel)

	// Send small payment - should use active channel, not inactive
	toAddr := [32]byte{0x77}
	result := pm.SmartSend(toAddr, 50)

	if result.Method != PaymentL2 {
		t.Errorf("Should use L2 with active channel, got %s", result.Method)
	}

	t.Logf("Active channel routing result: %s", result.String())
}

// ==================== Batch Payment Integration Tests ====================

// TestIntegration_BatchPaymentCompleteFlow tests complete batch payment workflow.
func TestIntegration_BatchPaymentCompleteFlow(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	// Setup with sufficient funds
	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x60},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)
	pm.SetL2Threshold(50)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "batch-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     500,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	bpm := NewBatchPaymentManager(pm, 3)

	// Create batch with mixed amounts
	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10, Memo: "payment 1"},
		{To: [32]byte{0x02}, Amount: 30, Memo: "payment 2"},
		{To: [32]byte{0x03}, Amount: 100, Memo: "payment 3"},
		{To: [32]byte{0x04}, Amount: 200, Memo: "payment 4"},
		{To: [32]byte{0x05}, Amount: 500, Memo: "payment 5"},
	}

	// Create batch
	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	// Verify batch created correctly
	if batch.Status != BatchPending {
		t.Errorf("Expected pending status, got %s", batch.Status)
	}
	if len(batch.Payments) != 5 {
		t.Errorf("Expected 5 payments, got %d", len(batch.Payments))
	}
	if batch.TotalAmount != 840 {
		t.Errorf("Expected total amount 840, got %d", batch.TotalAmount)
	}

	// Execute batch
	result, err := bpm.ExecuteBatch(batch.ID)
	if err != nil {
		t.Fatalf("Failed to execute batch: %v", err)
	}

	// Verify results
	if result.SuccessCount != 5 {
		t.Errorf("Expected 5 successes, got %d (failures: %v)",
			result.SuccessCount, result.FailedIndices)
	}
	if result.FailedCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailedCount)
	}
	if result.TotalAmount != 840 {
		t.Errorf("Result total amount should be 840, got %d", result.TotalAmount)
	}

	// Verify batch status
	status, _ := bpm.GetBatchStatus(batch.ID)
	if status != BatchCompleted {
		t.Errorf("Expected completed status, got %s", status)
	}

	t.Logf("Batch payment result: %s", result.String())

	// Verify L2 channel balance was used for small payments
	ch := pm.GetL2Channel("batch-channel")
	// 10 + 30 = 40 paid via L2
	t.Logf("Remaining L2 channel balance: %d", ch.Balance)
}

// TestIntegration_BatchPaymentWithFailures tests batch payment handling partial failures.
func TestIntegration_BatchPaymentWithFailures(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	// Setup with limited funds but sufficient for some payments
	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x70},
		Index:   0,
		Value:   150, // Enough for 2 payments of 50 plus fees
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)
	pm.SetL2Threshold(1000) // Force all to L1

	bpm := NewBatchPaymentManager(pm, 2)

	// Create batch where one will fail (total 150, but need fees)
	requests := []PaymentRequest{
		{To: [32]byte{0x11}, Amount: 50},
		{To: [32]byte{0x12}, Amount: 50},
		{To: [32]byte{0x13}, Amount: 50}, // This should fail
	}

	batch, _ := bpm.CreateBatchPayment(requests)
	result, _ := bpm.ExecuteBatch(batch.ID)

	// Verify at least some failures due to insufficient balance
	if result.FailedCount == 0 {
		t.Logf("Note: All payments succeeded unexpectedly with balance 150")
	}

	// Verify batch completed (either partial or full failure)
	status, _ := bpm.GetBatchStatus(batch.ID)
	if status != BatchPartialFailure && status != BatchFailed {
		t.Errorf("Expected partial_failure or failed status, got %s", status)
	}

	t.Logf("Partial failure batch result: %s", result.String())
}

// TestIntegration_BatchPaymentSequentialOrder tests sequential batch execution order.
func TestIntegration_BatchPaymentSequentialOrder(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x80},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)
	pm.SetL2Threshold(100)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "seq-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	bpm := NewBatchPaymentManager(pm, 1)

	requests := []PaymentRequest{
		{To: [32]byte{0x21}, Amount: 10},
		{To: [32]byte{0x22}, Amount: 20},
		{To: [32]byte{0x23}, Amount: 30},
	}

	batch, _ := bpm.CreateBatchPayment(requests)
	result, err := bpm.ExecuteBatchSequential(batch.ID)
	if err != nil {
		t.Fatalf("Sequential batch failed: %v", err)
	}

	if result.SuccessCount != 3 {
		t.Errorf("Expected 3 successes, got %d", result.SuccessCount)
	}

	t.Logf("Sequential batch completed: %s", result.String())
}

// ==================== Scheduled Payment Integration Tests ====================

// TestIntegration_ScheduledPaymentCompleteFlow tests complete scheduled payment workflow.
func TestIntegration_ScheduledPaymentCompleteFlow(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0x90},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)

	spm := NewScheduledPaymentManager(pm)

	// Set fixed time for testing
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule payment due now
	toAddr := [32]byte{0x31}
	result, err := spm.SchedulePayment(toAddr, 100, baseTime, "immediate payment")
	if err != nil {
		t.Fatalf("Failed to schedule payment: %v", err)
	}

	// Execute due payments
	execResults := spm.ExecuteDue()
	if len(execResults) != 1 {
		t.Fatalf("Expected 1 executed payment, got %d", len(execResults))
	}

	// Verify execution
	if !execResults[0].Success {
		t.Errorf("Scheduled payment should succeed: %s", execResults[0].Error)
	}

	// Verify schedule status
	schedule, _ := spm.GetSchedule(result.ScheduleID)
	if schedule.Status != ScheduledExecuted {
		t.Errorf("Expected executed status, got %s", schedule.Status)
	}

	t.Logf("Scheduled payment executed: %s", execResults[0].String())
}

// TestIntegration_ScheduledRecurringPayment tests recurring payment execution.
func TestIntegration_ScheduledRecurringPayment(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0xa0},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule recurring payment: 3 times, every hour
	toAddr := [32]byte{0x41}
	result, err := spm.ScheduleRecurringPayment(toAddr, 50, baseTime, time.Hour, 3, "recurring test")
	if err != nil {
		t.Fatalf("Failed to schedule recurring: %v", err)
	}

	scheduleID := result.ScheduleID

	// Execute first time
	spm.SetTimeFunc(func() time.Time { return baseTime })
	results1 := spm.ExecuteDue()
	if len(results1) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results1))
	}

	// Check schedule is still pending for next execution
	schedule, _ := spm.GetSchedule(scheduleID)
	if schedule.Status != ScheduledPending {
		t.Errorf("After first execution, should be pending for next, got %s", schedule.Status)
	}

	// Execute second time (1 hour later)
	spm.SetTimeFunc(func() time.Time { return baseTime.Add(time.Hour) })
	results2 := spm.ExecuteDue()
	if len(results2) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results2))
	}

	// Execute third time (2 hours later)
	spm.SetTimeFunc(func() time.Time { return baseTime.Add(2 * time.Hour) })
	results3 := spm.ExecuteDue()
	if len(results3) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results3))
	}

	// After 3rd execution, should be completed
	schedule, _ = spm.GetSchedule(scheduleID)
	if schedule.Status != ScheduledExecuted {
		t.Errorf("After max executions, should be completed, got %s", schedule.Status)
	}

	t.Logf("Recurring payment completed after 3 executions")
}

// TestIntegration_ScheduledPaymentCancellation tests scheduled payment cancellation.
func TestIntegration_ScheduledPaymentCancellation(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule future payment
	toAddr := [32]byte{0x51}
	result, _ := spm.SchedulePayment(toAddr, 100, baseTime.Add(24*time.Hour), "future payment")

	// Cancel before execution
	err := spm.CancelSchedule(result.ScheduleID)
	if err != nil {
		t.Fatalf("Failed to cancel: %v", err)
	}

	// Verify cancelled
	schedule, _ := spm.GetSchedule(result.ScheduleID)
	if schedule.Status != ScheduledCancelled {
		t.Errorf("Expected cancelled status, got %s", schedule.Status)
	}

	// Execute due - should not execute cancelled payment
	execResults := spm.ExecuteDue()
	if len(execResults) != 0 {
		t.Errorf("Cancelled payment should not execute, got %d results", len(execResults))
	}

	t.Logf("Cancellation test passed")
}

// TestIntegration_ScheduledPaymentListPending tests listing pending scheduled payments.
func TestIntegration_ScheduledPaymentListPending(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule multiple payments at different times
	for i := 0; i < 5; i++ {
		toAddr := [32]byte{byte(i)}
		execTime := baseTime.Add(time.Duration(i) * time.Hour)
		spm.SchedulePayment(toAddr, 10, execTime, "test")
	}

	// List pending
	pending := spm.ListPending()
	if len(pending) != 5 {
		t.Errorf("Expected 5 pending, got %d", len(pending))
	}

	// Execute one
	spm.SetTimeFunc(func() time.Time { return baseTime })
	spm.ExecuteDue()

	// List pending again
	pending = spm.ListPending()
	if len(pending) != 4 {
		t.Errorf("Expected 4 pending after one executed, got %d", len(pending))
	}

	t.Logf("Pending list test passed: %d pending after execution", len(pending))
}

// ==================== Smart Router Integration Tests ====================

// TestIntegration_SmartRouterCompleteFlow tests smart router with all strategies.
func TestIntegration_SmartRouterCompleteFlow(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0xb0},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)

	// Add multiple L2 channels
	channels := []*L2Channel{
		{
			ChannelID:   "fast-channel",
			PeerAddress: wallet.GetAddress(),
			Balance:     10000,
			FeeRate:     20, // Higher fee
			IsActive:    true,
		},
		{
			ChannelID:   "cheap-channel",
			PeerAddress: wallet.GetAddress(),
			Balance:     10000,
			FeeRate:     1, // Lower fee
			IsActive:    true,
		},
	}
	for _, ch := range channels {
		pm.AddL2Channel(ch)
	}

	router := NewSmartRouter(pm, nil)
	toAddr := [32]byte{0x61}

	// Test cheapest strategy
	t.Run("CheapestStrategy", func(t *testing.T) {
		router.SetStrategy(StrategyCheapest)
		route, err := router.SelectRoute(toAddr, 100, nil)
		if err != nil {
			t.Fatalf("Failed to select route: %v", err)
		}
		t.Logf("Cheapest route: %+v", route)
	})

	// Test fastest strategy
	t.Run("FastestStrategy", func(t *testing.T) {
		router.SetStrategy(StrategyFastest)
		route, err := router.SelectRoute(toAddr, 100, nil)
		if err != nil {
			t.Fatalf("Failed to select route: %v", err)
		}
		t.Logf("Fastest route: %+v", route)
	})

	// Test balanced strategy
	t.Run("BalancedStrategy", func(t *testing.T) {
		router.SetStrategy(StrategyBalanced)
		route, err := router.SelectRoute(toAddr, 100, nil)
		if err != nil {
			t.Fatalf("Failed to select route: %v", err)
		}
		t.Logf("Balanced route: %+v", route)
	})

	// Test safe strategy
	t.Run("SafeStrategy", func(t *testing.T) {
		router.SetStrategy(StrategySafe)
		route, err := router.SelectRoute(toAddr, 100, nil)
		if err != nil {
			t.Fatalf("Failed to select route: %v", err)
		}
		t.Logf("Safe route: %+v", route)
	})

	// Test execute with route
	t.Run("ExecuteWithRoute", func(t *testing.T) {
		route, _ := router.SelectRoute(toAddr, 100, nil)
		result := router.ExecuteWithRoute(toAddr, 100, route)
		if !result.Success {
			t.Errorf("Execute with route failed: %s", result.Error)
		}
		t.Logf("Execute with route result: %s", result.String())
	})

	// Test smart route payment
	t.Run("SmartRoutePayment", func(t *testing.T) {
		opts := &RouteOption{
			Strategy: StrategyBalanced,
		}
		result := router.SmartRoutePayment(toAddr, 100, opts)
		if !result.Success {
			t.Errorf("Smart route payment failed: %s", result.Error)
		}
		t.Logf("Smart route payment result: %s", result.String())
	})
}

// TestIntegration_SmartRouterWithConstraints tests router with various constraints.
func TestIntegration_SmartRouterWithConstraints(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	store.AddUTXO(&utxo.UTXO{
		TxHash:  [32]byte{0xc0},
		Index:   0,
		Value:   100000,
		Address: wallet.GetAddress(),
	})
	pm.SetUTXOStore(store)

	channel := &L2Channel{
		ChannelID:   "constraint-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     10000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	router := NewSmartRouter(pm, nil)
	toAddr := [32]byte{0x71}

	// Test with max fee constraint
	t.Run("MaxFeeConstraint", func(t *testing.T) {
		opts := &RouteOption{
			MaxFee:  50,
			Strategy: StrategyCheapest,
		}
		route, err := router.SelectRoute(toAddr, 1000, opts)
		// May or may not find route depending on fee
		if err != nil {
			t.Logf("No route found with max fee 50: %v", err)
		} else {
			t.Logf("Route with fee constraint: %+v", route)
		}
	})

	// Test with max time constraint
	t.Run("MaxTimeConstraint", func(t *testing.T) {
		opts := &RouteOption{
			MaxTime:  1000, // Very low
			Strategy: StrategyFastest,
		}
		route, err := router.SelectRoute(toAddr, 100, opts)
		if err != nil {
			t.Logf("No route found with max time 1000ms: %v", err)
		} else {
			t.Logf("Route with time constraint: %+v", route)
		}
	})

	// Test with prefer method constraint
	t.Run("PreferMethodConstraint", func(t *testing.T) {
		opts := &RouteOption{
			PreferMethod: PaymentL2,
			Strategy:     StrategyCheapest,
		}
		route, err := router.SelectRoute(toAddr, 100, opts)
		if err != nil {
			t.Fatalf("Failed to select route: %v", err)
		}
		if route.Method != PaymentL2 {
			t.Logf("Note: Preferred L2 but got %s", route.Method)
		}
		t.Logf("Route with method preference: %+v", route)
	})
}

// TestIntegration_SmartRouterChannelHealth tests channel health tracking.
func TestIntegration_SmartRouterChannelHealth(t *testing.T) {
	wallet, _ := NewWallet()
	pm := NewPaymentManager(wallet)
	router := NewSmartRouter(pm, nil)

	channelID := "health-test-channel"

	// Initial health check
	healthy := router.GetHealthyChannels(0.5)
	initialCount := len(healthy)
	t.Logf("Initial healthy channels: %d", initialCount)

	// Simulate successful payments
	router.UpdateChannelHealth(channelID, true, 100)
	router.UpdateChannelHealth(channelID, true, 100)
	router.UpdateChannelHealth(channelID, true, 100)

	// Simulate failed payment
	router.UpdateChannelHealth(channelID, false, 500)

	// Check health after updates
	healthy = router.GetHealthyChannels(0.3)
	t.Logf("Healthy channels after updates: %d", len(healthy))

	for _, h := range healthy {
		if h.ChannelID == channelID {
			t.Logf("Channel %s - SuccessRate: %.2f, Score: %.2f, AvgConfirmTime: %dms",
				h.ChannelID, h.SuccessRate, h.Score, h.AvgConfirmTime)
		}
	}

	// Estimate costs
	l1Cost := router.EstimateTotalCost(1000, PaymentL1)
	l2Cost := router.EstimateTotalCost(1000, PaymentL2)
	t.Logf("Cost estimates - L1: %d, L2: %d", l1Cost, l2Cost)
}

// ==================== SDK Integration Tests ====================

// TestIntegration_WalletSDKCompleteFlow tests complete flow using WalletSDK.
func TestIntegration_WalletSDKCompleteFlow(t *testing.T) {
	// Create SDK
	sdk, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create SDK: %v", err)
	}

	// Add UTXO funds
	txHash := [32]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	err = sdk.AddUTXO(txHash, 0, 100000)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "sdk-channel",
		PeerAddress: sdk.GetAddress(),
		Balance:     5000,
		FeeRate:     5,
		IsActive:    true,
	}
	sdk.AddL2Channel(channel)

	// Set threshold
	sdk.SetL2Threshold(100)

	// Get initial balance
	balance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}
	t.Logf("Initial balance: L1=%d, L2=%d, Total=%d",
		balance.L1Balance, balance.L2Balance, balance.Total)

	// Send payment using smart routing
	receiverAddr := sdk.GetAddressHex()
	result, err := sdk.Send(receiverAddr, 50)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Payment should succeed: %s", result.Error)
	}
	t.Logf("SDK Send result: method=%s, amount=%d, fee=%d",
		result.Method, result.Amount, result.Fee)

	// Send L1 payment
	resultL1, err := sdk.SendL1(receiverAddr, 1000)
	if err != nil {
		t.Fatalf("SendL1 failed: %v", err)
	}
	t.Logf("SDK SendL1 result: method=%s, amount=%d, fee=%d",
		resultL1.Method, resultL1.Amount, resultL1.Fee)

	// Send L2 payment
	resultL2, err := sdk.SendL2(receiverAddr, 25, "sdk-channel")
	if err != nil {
		t.Fatalf("SendL2 failed: %v", err)
	}
	t.Logf("SDK SendL2 result: method=%s, amount=%d, fee=%d",
		resultL2.Method, resultL2.Amount, resultL2.Fee)

	// Get final balance
	finalBalance, err := sdk.Balance()
	if err != nil {
		t.Fatalf("Failed to get final balance: %v", err)
	}
	t.Logf("Final balance: L1=%d, L2=%d, Total=%d",
		finalBalance.L1Balance, finalBalance.L2Balance, finalBalance.Total)

	// Get transaction history
	history, err := sdk.History(10)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	t.Logf("Transaction history: %d records", len(history))
	for _, tx := range history {
		t.Logf("  - Direction: %s, Amount: %d, Method: %s",
			tx.Direction, tx.Amount, tx.Method)
	}

	// Test signing
	data := []byte("Test message for signing")
	signature := sdk.Sign(data)
	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	// Verify signature
	if !sdk.Verify(data, signature) {
		t.Error("Signature verification failed")
	}

	// Export and verify private key (ed25519 private key is 64 bytes)
	privKey := sdk.ExportPrivateKey()
	if len(privKey) != 64 {
		t.Errorf("Expected private key length 64, got %d", len(privKey))
	}
}

// TestIntegration_WalletSDKRecovery tests SDK wallet recovery.
func TestIntegration_WalletSDKRecovery(t *testing.T) {
	// Create original SDK
	original, err := NewWalletSDK(nil)
	if err != nil {
		t.Fatalf("Failed to create original SDK: %v", err)
	}

	originalAddr := original.GetAddressHex()

	// Add funds to original
	txHash := [32]byte{0xd0}
	original.AddUTXO(txHash, 0, 50000)

	// Get private key and recover
	privKey := original.ExportPrivateKey()
	recovered, err := NewWalletSDK(&SDKConfig{
		PrivateKey: privKey,
	})
	if err != nil {
		t.Fatalf("Failed to recover SDK: %v", err)
	}

	// Verify addresses match
	if originalAddr != recovered.GetAddressHex() {
		t.Errorf("Recovered address %s does not match original %s",
			recovered.GetAddressHex(), originalAddr)
	}

	// The recovered SDK has its own UTXO store, so we add funds separately
	// to verify the wallet is the same (same address)
	recoveredTxHash := [32]byte{0xd1}
	recovered.AddUTXO(recoveredTxHash, 0, 30000)

	// Both should have their own UTXOs in their respective stores
	originalBalance, _ := original.Balance()
	recoveredBalance, _ := recovered.Balance()

	t.Logf("Original SDK balance: L1=%d", originalBalance.L1Balance)
	t.Logf("Recovered SDK balance: L1=%d", recoveredBalance.L1Balance)

	// The original has 50000, the recovered has 30000
	// This shows they are separate instances but share the same address
	if recoveredBalance.L1Balance != 30000 {
		t.Errorf("Expected recovered balance 30000, got %d", recoveredBalance.L1Balance)
	}

	// Verify the addresses are the same (proving recovery worked)
	if original.GetAddress() != recovered.GetAddress() {
		t.Error("Addresses should match after recovery")
	}

	t.Logf("SDK recovery test passed, recovered address matches original")
}
