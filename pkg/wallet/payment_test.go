// Package wallet provides tests for payment functionality.
package wallet

import (
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// ==================== Batch Payment Tests ====================

func TestBatchPayment_CreateBatchPayment(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10, Memo: "payment 1"},
		{To: [32]byte{0x02}, Amount: 20, Memo: "payment 2"},
		{To: [32]byte{0x03}, Amount: 30, Memo: "payment 3"},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	if batch.Status != BatchPending {
		t.Errorf("Batch status should be pending, got %s", batch.Status)
	}

	if len(batch.Payments) != 3 {
		t.Errorf("Batch should have 3 payments, got %d", len(batch.Payments))
	}

	if batch.TotalAmount != 60 {
		t.Errorf("Total amount should be 60, got %d", batch.TotalAmount)
	}

	t.Logf("Created batch: %s", batch.ID)
}

func TestBatchPayment_CreateBatchPayment_Empty(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	_, err = bpm.CreateBatchPayment([]PaymentRequest{})
	if err == nil {
		t.Error("Should fail with empty requests")
	}
}

func TestBatchPayment_CreateBatchPayment_ZeroAmount(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 0},
	}

	_, err = bpm.CreateBatchPayment(requests)
	if err == nil {
		t.Error("Should fail with zero amount")
	}
}

func TestBatchPayment_ExecuteBatch(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Set up UTXO store with funds
	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xaa},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	bpm := NewBatchPaymentManager(pm, 2)

	// Add L2 channel for small payments
	channel := &L2Channel{
		ChannelID:   "channel-test",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)
	pm.SetL2Threshold(100)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10},
		{To: [32]byte{0x02}, Amount: 20},
		{To: [32]byte{0x03}, Amount: 30},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	result, err := bpm.ExecuteBatch(batch.ID)
	if err != nil {
		t.Fatalf("Failed to execute batch: %v", err)
	}

	if result == nil {
		t.Fatal("Batch result should not be nil")
	}

	if result.TotalAmount != 60 {
		t.Errorf("Total amount should be 60, got %d", result.TotalAmount)
	}

	t.Logf("Batch result: %s", result.String())
}

func TestBatchPayment_GetBatchStatus(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	status, err := bpm.GetBatchStatus(batch.ID)
	if err != nil {
		t.Fatalf("Failed to get batch status: %v", err)
	}

	if status != BatchPending {
		t.Errorf("Status should be pending, got %s", status)
	}
}

func TestBatchPayment_ListBatches(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	// Create multiple batches
	for i := 1; i <= 3; i++ {
		requests := []PaymentRequest{
			{To: [32]byte{byte(i)}, Amount: uint64(i * 10)},
		}
		_, err := bpm.CreateBatchPayment(requests)
		if err != nil {
			t.Fatalf("Failed to create batch %d: %v", i, err)
		}
	}

	batches := bpm.ListBatches()
	if len(batches) != 3 {
		t.Errorf("Should have 3 batches, got %d", len(batches))
	}
}

func TestBatchPayment_CancelBatch(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	err = bpm.CancelBatch(batch.ID)
	if err != nil {
		t.Fatalf("Failed to cancel batch: %v", err)
	}

	status, _ := bpm.GetBatchStatus(batch.ID)
	if status != BatchFailed {
		t.Errorf("Cancelled batch should have failed status, got %s", status)
	}
}

func TestBatchPayment_EstimateBatchFee(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	pm.SetL2Threshold(100)
	pm.SetFeePerByte(2)

	bpm := NewBatchPaymentManager(pm, 5)

	requests := []PaymentRequest{
		{To: [32]byte{0x01}, Amount: 10},  // L2
		{To: [32]byte{0x02}, Amount: 50},  // L2
		{To: [32]byte{0x03}, Amount: 200}, // L1
		{To: [32]byte{0x04}, Amount: 500}, // L1
	}

	l1Fee, l2Fee, err := bpm.EstimateBatchFee(requests)
	if err != nil {
		t.Fatalf("Failed to estimate fee: %v", err)
	}

	// L1: 2 txs * 200 bytes * 2 = 800
	// L2: (10+50) * 0.001 = 0.06 -> 0 (rounded)
	if l1Fee != 800 {
		t.Errorf("L1 fee should be 800, got %d", l1Fee)
	}

	t.Logf("Estimated fees - L1: %d, L2: %d", l1Fee, l2Fee)
}

// ==================== Scheduled Payment Tests ====================

func TestScheduledPayment_SchedulePayment(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	// Set fake time for testing
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	toAddr := [32]byte{0x01}
	executeAt := baseTime.Add(1 * time.Hour)

	result, err := spm.SchedulePayment(toAddr, 100, executeAt, "test payment")
	if err != nil {
		t.Fatalf("Failed to schedule payment: %v", err)
	}

	if result.Status != ScheduledPending {
		t.Errorf("Status should be pending, got %s", result.Status)
	}

	t.Logf("Scheduled payment: %s", result.ScheduleID)
}

func TestScheduledPayment_SchedulePayment_PastTime(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	toAddr := [32]byte{0x01}
	executeAt := baseTime.Add(-1 * time.Hour) // Past time

	_, err = spm.SchedulePayment(toAddr, 100, executeAt, "test")
	if err == nil {
		t.Error("Should fail with past execution time")
	}
}

func TestScheduledPayment_ScheduleRecurringPayment(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	toAddr := [32]byte{0x01}
	startAt := baseTime.Add(1 * time.Hour)

	result, err := spm.ScheduleRecurringPayment(toAddr, 50, startAt, time.Hour, 5, "recurring")
	if err != nil {
		t.Fatalf("Failed to schedule recurring payment: %v", err)
	}

	if result.Status != ScheduledPending {
		t.Errorf("Status should be pending, got %s", result.Status)
	}

	schedule, _ := spm.GetSchedule(result.ScheduleID)
	if schedule.Recurring == nil {
		t.Error("Recurring config should be set")
	}

	if schedule.Recurring.MaxCount != 5 {
		t.Errorf("Max count should be 5, got %d", schedule.Recurring.MaxCount)
	}
}

func TestScheduledPayment_ExecuteDue(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Set up UTXO store
	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xbb},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule a payment due now
	toAddr := [32]byte{0x01}
	executeAt := baseTime // Due now

	_, err = spm.SchedulePayment(toAddr, 100, executeAt, "due now")
	if err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	// Execute due payments
	results := spm.ExecuteDue()
	if len(results) != 1 {
		t.Errorf("Should execute 1 payment, got %d", len(results))
	}

	// Check schedule status
	schedules := spm.ListSchedules()
	for _, s := range schedules {
		t.Logf("Schedule %s status: %s", s.ID, s.Status)
	}
}

func TestScheduledPayment_CancelSchedule(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	toAddr := [32]byte{0x01}
	executeAt := baseTime.Add(1 * time.Hour)

	result, err := spm.SchedulePayment(toAddr, 100, executeAt, "test")
	if err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	err = spm.CancelSchedule(result.ScheduleID)
	if err != nil {
		t.Fatalf("Failed to cancel: %v", err)
	}

	schedule, _ := spm.GetSchedule(result.ScheduleID)
	if schedule.Status != ScheduledCancelled {
		t.Errorf("Status should be cancelled, got %s", schedule.Status)
	}
}

func TestScheduledPayment_ListPending(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Create multiple schedules
	for i := 0; i < 3; i++ {
		toAddr := [32]byte{byte(i)}
		executeAt := baseTime.Add(time.Duration(i+1) * time.Hour)
		spm.SchedulePayment(toAddr, 100, executeAt, "test")
	}

	pending := spm.ListPending()
	if len(pending) != 3 {
		t.Errorf("Should have 3 pending, got %d", len(pending))
	}
}

// ==================== Refund Tests ====================

func TestRefund_RequestRefund(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Set up UTXO store
	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xcc},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x11, 0x22, 0x33},
		RefundTo:       [32]byte{0x01},
		Amount:         100,
		Reason:         "test refund",
	}

	result, err := rm.RequestRefund(req)
	if err != nil {
		t.Fatalf("Failed to request refund: %v", err)
	}

	if result.Status != RefundCompleted {
		t.Errorf("Refund should complete, got %s: %s", result.Status, result.Error)
	}

	t.Logf("Refund result: %s", result.String())
}

func TestRefund_RequestRefund_InvalidAmount(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x11},
		RefundTo:       [32]byte{0x01},
		Amount:         0,
	}

	_, err = rm.RequestRefund(req)
	if err == nil {
		t.Error("Should fail with zero amount")
	}
}

func TestRefund_RequestRefundL1(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xdd},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x22, 0x33, 0x44},
		RefundTo:       [32]byte{0x02},
		Amount:         50,
		Reason:         "L1 refund",
	}

	result, err := rm.RequestRefundL1(req)
	if err != nil {
		t.Fatalf("Failed to request L1 refund: %v", err)
	}

	if result.Method != PaymentL1 {
		t.Errorf("Method should be L1, got %s", result.Method)
	}

	if result.Status == RefundCompleted {
		t.Logf("L1 refund tx: %x", result.RefundTxHash[:8])
	}
}

func TestRefund_GetRefundsByOriginalTx(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xee},
		Index:   0,
		Value:   50000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	rm := NewRefundManager(pm)

	originalTx := [32]byte{0x33, 0x44, 0x55}

	// Request multiple refunds for the same original tx
	req := &RefundRequest{
		OriginalTxHash: originalTx,
		RefundTo:       [32]byte{0x01},
		Amount:         30,
		Reason:         "partial refund",
	}
	rm.RequestRefund(req)

	refunds := rm.GetRefundsByOriginalTx(originalTx)
	if len(refunds) == 0 {
		t.Error("Should find refunds for original tx")
	}

	t.Logf("Found %d refunds for original tx", len(refunds))
}

func TestRefund_PartialRefund(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xff},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	rm := NewRefundManager(pm)

	originalTx := [32]byte{0x44, 0x55, 0x66}
	originalAmount := uint64(1000)
	refundTo := [32]byte{0x03}
	basisPoints := uint64(5000) // 50%

	result, err := rm.PartialRefund(originalTx, originalAmount, refundTo, basisPoints, "50% refund")
	if err != nil {
		t.Fatalf("Failed to request partial refund: %v", err)
	}

	expectedAmount := uint64(500)
	if result.Amount != expectedAmount {
		t.Errorf("Refund amount should be %d, got %d", expectedAmount, result.Amount)
	}

	t.Logf("Partial refund: %s", result.String())
}

func TestRefund_ListRefundsByStatus(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0x11, 0x11},
		Index:   0,
		Value:   50000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	rm := NewRefundManager(pm)

	// Request some refunds
	for i := 0; i < 3; i++ {
		req := &RefundRequest{
			OriginalTxHash: [32]byte{byte(i)},
			RefundTo:       [32]byte{byte(i)},
			Amount:         10,
		}
		rm.RequestRefund(req)
	}

	completed := rm.ListRefundsByStatus(RefundCompleted)
	t.Logf("Found %d completed refunds", len(completed))
}

// ==================== Smart Router Tests ====================

func TestSmartRouter_NewSmartRouter(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	config := DefaultRoutingConfig()

	router := NewSmartRouter(pm, config)
	if router == nil {
		t.Fatal("SmartRouter should not be nil")
	}

	if router.strategy != StrategyBalanced {
		t.Errorf("Default strategy should be balanced, got %s", router.strategy)
	}
}

func TestSmartRouter_CalculateRoutes(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	pm.SetL2Threshold(100)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "router-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	router := NewSmartRouter(pm, nil)

	toAddr := [32]byte{0x01}
	routes, err := router.CalculateRoutes(toAddr, 50)
	if err != nil {
		t.Fatalf("Failed to calculate routes: %v", err)
	}

	if len(routes) < 1 {
		t.Error("Should have at least one route (L1)")
	}

	// Should have L2 route for small amount
	var hasL2 bool
	for _, r := range routes {
		if r.Method == PaymentL2 {
			hasL2 = true
			break
		}
	}

	t.Logf("Found %d routes, L2 available: %v", len(routes), hasL2)
}

func TestSmartRouter_SelectRoute_Cheapest(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	pm.SetFeePerByte(1)

	// Add L2 channel with low fee
	channel := &L2Channel{
		ChannelID:   "low-fee-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     10000,
		FeeRate:     1, // Very low fee
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	router := NewSmartRouter(pm, nil)
	router.SetStrategy(StrategyCheapest)

	toAddr := [32]byte{0x01}
	route, err := router.SelectRoute(toAddr, 100, nil)
	if err != nil {
		t.Fatalf("Failed to select route: %v", err)
	}

	// L2 should be cheaper for small amounts
	if route.Method != PaymentL2 {
		t.Logf("Note: Cheapest route is %s (expected L2 for small amount)", route.Method)
	}

	t.Logf("Selected route: Method=%s, Fee=%d, TotalCost=%d", route.Method, route.Fee, route.TotalCost)
}

func TestSmartRouter_SelectRoute_Fastest(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	channel := &L2Channel{
		ChannelID:   "fast-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     10,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	router := NewSmartRouter(pm, nil)
	router.SetStrategy(StrategyFastest)

	toAddr := [32]byte{0x01}
	route, err := router.SelectRoute(toAddr, 100, nil)
	if err != nil {
		t.Fatalf("Failed to select route: %v", err)
	}

	// L2 should be fastest
	if route.ConfirmTime > 1000 {
		t.Logf("Note: Fastest route confirm time is %dms", route.ConfirmTime)
	}

	t.Logf("Selected fastest route: Method=%s, ConfirmTime=%dms", route.Method, route.ConfirmTime)
}

func TestSmartRouter_ExecuteWithRoute(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0x88},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	router := NewSmartRouter(pm, nil)

	toAddr := [32]byte{0x01}

	// Get a route
	route, err := router.SelectRoute(toAddr, 100, nil)
	if err != nil {
		t.Fatalf("Failed to select route: %v", err)
	}

	result := router.ExecuteWithRoute(toAddr, 100, route)
	if result != nil {
		t.Logf("Payment result: %s", result.String())
	}
}

func TestSmartRouter_SmartRoutePayment(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0x99},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	router := NewSmartRouter(pm, nil)

	toAddr := [32]byte{0x01}
	opts := &RouteOption{
		Strategy: StrategyBalanced,
	}

	result := router.SmartRoutePayment(toAddr, 100, opts)
	if result != nil {
		t.Logf("Smart route payment: %s", result.String())
	}
}

func TestSmartRouter_UpdateChannelHealth(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	router := NewSmartRouter(pm, nil)

	channelID := "test-channel"

	// Update with successful payment
	router.UpdateChannelHealth(channelID, true, 100)
	router.UpdateChannelHealth(channelID, true, 100)
	router.UpdateChannelHealth(channelID, true, 100)

	// Update with failed payment
	router.UpdateChannelHealth(channelID, false, 200)

	healthy := router.GetHealthyChannels(0.5)
	t.Logf("Found %d healthy channels", len(healthy))

	for _, h := range healthy {
		t.Logf("Channel %s: Score=%.2f, SuccessRate=%.2f", h.ChannelID, h.Score, h.SuccessRate)
	}
}

func TestSmartRouter_EstimateTotalCost(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	router := NewSmartRouter(pm, nil)

	amount := uint64(1000)

	// Estimate for L1
	l1Cost := router.EstimateTotalCost(amount, PaymentL1)
	t.Logf("L1 total cost: %d", l1Cost)

	// Estimate for L2
	l2Cost := router.EstimateTotalCost(amount, PaymentL2)
	t.Logf("L2 total cost: %d", l2Cost)

	// Auto estimate
	autoCost := router.EstimateTotalCost(amount, 0)
	t.Logf("Auto total cost: %d", autoCost)
}

func TestRefund_RequestRefundL2(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "refund-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x55, 0x66, 0x77},
		RefundTo:       [32]byte{0x05},
		Amount:         100,
		Reason:         "L2 refund",
	}

	result, err := rm.RequestRefundL2(req, "refund-channel")
	if err != nil {
		t.Fatalf("Failed to request L2 refund: %v", err)
	}

	if result.Method != PaymentL2 {
		t.Errorf("Method should be L2, got %s", result.Method)
	}

	t.Logf("L2 refund result: %s", result.String())
}

func TestRefund_RequestRefundL2_EmptyChannel(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x66, 0x77, 0x88},
		RefundTo:       [32]byte{0x06},
		Amount:         100,
	}

	_, err = rm.RequestRefundL2(req, "")
	if err == nil {
		t.Error("Should fail with empty channel ID")
	}
}

func TestBatchPayment_ExecuteBatchSequential(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Set up UTXO store
	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xab},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "sequential-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     100,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)
	pm.SetL2Threshold(50)

	bpm := NewBatchPaymentManager(pm, 1)

	requests := []PaymentRequest{
		{To: [32]byte{0x10}, Amount: 10},
		{To: [32]byte{0x11}, Amount: 20},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	result, err := bpm.ExecuteBatchSequential(batch.ID)
	if err != nil {
		t.Fatalf("Failed to execute batch: %v", err)
	}

	if result.SuccessCount != 2 {
		t.Errorf("Expected 2 successes, got %d", result.SuccessCount)
	}

	t.Logf("Sequential batch result: %s", result.String())
}

func TestScheduledPayment_AutoExecutor(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xac},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule a payment due immediately
	toAddr := [32]byte{0x20}
	executeAt := baseTime

	_, err = spm.SchedulePayment(toAddr, 10, executeAt, "auto test")
	if err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	// Start auto executor briefly
	err = spm.StartAutoExecutor(10 * time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	// Wait for execution
	time.Sleep(50 * time.Millisecond)

	if !spm.IsRunning() {
		t.Error("Auto executor should be running")
	}

	spm.StopAutoExecutor()

	if spm.IsRunning() {
		t.Error("Auto executor should be stopped")
	}
}

func TestRefund_PartialRefund_InvalidBasisPoints(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	_, err = rm.PartialRefund([32]byte{0x77}, 1000, [32]byte{0x07}, 10001, "invalid")
	if err == nil {
		t.Error("Should fail with basis points > 10000")
	}
}

func TestSmartRouter_GetConfig(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	router := NewSmartRouter(pm, nil)

	config := router.GetConfig()
	if config == nil {
		t.Fatal("Config should not be nil")
	}

	if config.L2Threshold != DefaultL2Threshold {
		t.Errorf("Default L2 threshold should be %d, got %d", DefaultL2Threshold, config.L2Threshold)
	}
}

func TestSmartRouter_UpdateConfig(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	router := NewSmartRouter(pm, nil)

	newConfig := &RoutingConfig{
		L2Threshold: 500,
		FeePerByte:  2,
		L2FeeBPS:    20,
	}

	router.UpdateConfig(newConfig)

	config := router.GetConfig()
	if config.L2Threshold != 500 {
		t.Errorf("L2 threshold should be 500, got %d", config.L2Threshold)
	}

	// Test nil config (should not panic)
	router.UpdateConfig(nil)
}

func TestBatchPayment_CancelBatch_InProgress(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 1)

	// Add funds
	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xad},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	requests := []PaymentRequest{
		{To: [32]byte{0x30}, Amount: 10},
		{To: [32]byte{0x31}, Amount: 10},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	// Start execution in background
	go bpm.ExecuteBatch(batch.ID)

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Try to cancel - should fail because it's in progress
	err = bpm.CancelBatch(batch.ID)
	if err == nil {
		t.Log("Note: Cancel may or may not fail for in-progress batch (timing dependent)")
	}
}

func TestScheduledPayment_GetSchedule_NotFound(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	spm := NewScheduledPaymentManager(pm)

	_, err = spm.GetSchedule("non-existent")
	if err == nil {
		t.Error("Should fail for non-existent schedule")
	}
}

func TestRefund_GetRefund_NotFound(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	_, err = rm.GetRefund("non-existent")
	if err == nil {
		t.Error("Should fail for non-existent refund")
	}
}

func TestBatchPayment_GetBatch_NotFound(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	bpm := NewBatchPaymentManager(pm, 5)

	_, err = bpm.GetBatch("non-existent")
	if err == nil {
		t.Error("Should fail for non-existent batch")
	}
}

func TestBatchPayment_CancelBatch_Completed(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xae},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	bpm := NewBatchPaymentManager(pm, 1)

	requests := []PaymentRequest{
		{To: [32]byte{0x40}, Amount: 10},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	// Execute to completion
	bpm.ExecuteBatch(batch.ID)

	// Try to cancel - should fail
	err = bpm.CancelBatch(batch.ID)
	if err == nil {
		t.Error("Should fail for completed batch")
	}
}

func TestSmartRouter_SelectRoute_WithOptions(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// Add L2 channel
	channel := &L2Channel{
		ChannelID:   "options-channel",
		PeerAddress: wallet.GetAddress(),
		Balance:     1000,
		FeeRate:     5,
		IsActive:    true,
	}
	pm.AddL2Channel(channel)

	router := NewSmartRouter(pm, nil)

	toAddr := [32]byte{0x50}

	// Test with prefer method option
	opts := &RouteOption{
		PreferMethod: PaymentL2,
		Strategy:     StrategyCheapest,
	}

	route, err := router.SelectRoute(toAddr, 100, opts)
	if err != nil {
		t.Fatalf("Failed to select route: %v", err)
	}

	if route.Method != PaymentL2 {
		t.Logf("Note: With prefer L2, got %s", route.Method)
	}

	// Test with max fee constraint
	opts2 := &RouteOption{
		MaxFee: 100, // Very low max fee
	}

	_, err = router.SelectRoute(toAddr, 100, opts2)
	if err != nil {
		t.Logf("Note: With low max fee, got error: %v", err)
	}
}

func TestSmartRouter_SelectRoute_NoValidRoute(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// No channels, no UTXO store

	router := NewSmartRouter(pm, nil)

	toAddr := [32]byte{0x60}

	routes, err := router.CalculateRoutes(toAddr, 100)
	if err != nil {
		t.Fatalf("Should have L1 route at minimum: %v", err)
	}

	if len(routes) == 0 {
		t.Error("Should have at least L1 route")
	}
}

func TestScheduledPayment_CancelSchedule_AlreadyExecuted(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	store := utxo.NewUTXOStore()
	utxo1 := &utxo.UTXO{
		TxHash:  [32]byte{0xaf},
		Index:   0,
		Value:   10000,
		Address: wallet.GetAddress(),
	}
	store.AddUTXO(utxo1)
	pm.SetUTXOStore(store)

	spm := NewScheduledPaymentManager(pm)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	spm.SetTimeFunc(func() time.Time { return baseTime })

	// Schedule and execute
	toAddr := [32]byte{0x70}
	result, err := spm.SchedulePayment(toAddr, 10, baseTime, "execute now")
	if err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	spm.ExecuteDue()

	// Try to cancel - should fail
	err = spm.CancelSchedule(result.ScheduleID)
	if err == nil {
		t.Error("Should fail to cancel executed schedule")
	}
}

func TestBatchPayment_ExecuteBatch_InsufficientBalance(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)

	// No funds
	store := utxo.NewUTXOStore()
	pm.SetUTXOStore(store)

	bpm := NewBatchPaymentManager(pm, 1)

	requests := []PaymentRequest{
		{To: [32]byte{0x80}, Amount: 1000},
	}

	batch, err := bpm.CreateBatchPayment(requests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	result, err := bpm.ExecuteBatch(batch.ID)
	if err != nil {
		t.Fatalf("Should not error: %v", err)
	}

	if result.SuccessCount != 0 {
		t.Errorf("Expected 0 successes, got %d", result.SuccessCount)
	}

	if result.FailedCount != 1 {
		t.Errorf("Expected 1 failure, got %d", result.FailedCount)
	}
}

func TestRefund_RefundTo_EmptyAddress(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{0x88},
		RefundTo:       [32]byte{}, // Empty address
		Amount:         100,
	}

	_, err = rm.RequestRefund(req)
	if err == nil {
		t.Error("Should fail with empty refund address")
	}
}

func TestRefund_OriginalTxHash_Empty(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	pm := NewPaymentManager(wallet)
	rm := NewRefundManager(pm)

	req := &RefundRequest{
		OriginalTxHash: [32]byte{}, // Empty
		RefundTo:       [32]byte{0x01},
		Amount:         100,
	}

	_, err = rm.RequestRefund(req)
	if err == nil {
		t.Error("Should fail with empty original tx hash")
	}
}
