// Package channel implements Lightning-style state channels for AIB 2.0.
// It provides order book functionality for L2 trading within channels.
package channel

import (
	"sync"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// TestOrderBookBasicFunctionality 测试订单簿基本功能
func TestOrderBookBasicFunctionality(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	// 创建测试地址
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 测试1：添加买单
	buyOrder := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    100,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	order, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if order.ID == 0 {
		t.Error("Order ID should not be 0")
	}
	if len(trades) != 0 {
		t.Error("Should have no trades initially")
	}

	t.Logf("Created buy order: %s", order.String())
}

// TestOrderMatching 测试订单匹配
func TestOrderMatching(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 添加卖单
	sellOrder := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    50,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	_, _, err := ob.PlaceOrder(sellOrder)
	if err != nil {
		t.Fatalf("PlaceOrder sell failed: %v", err)
	}

	// 添加匹配的买单
	buyOrder := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    50,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	_, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("PlaceOrder buy failed: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	trade := trades[0]
	if trade.Quantity != 50 {
		t.Errorf("Expected trade quantity 50, got %d", trade.Quantity)
	}
	if trade.Price != 1000 {
		t.Errorf("Expected trade price 1000, got %d", trade.Price)
	}

	t.Logf("Trade executed: %s", trade.String())
}

// TestPartialFill 测试部分成交
func TestPartialFill(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 添加卖单 30
	sellOrder1 := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    30,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}
	ob.PlaceOrder(sellOrder1)

	// 添加卖单 30
	sellOrder2 := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    30,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}
	ob.PlaceOrder(sellOrder2)

	// 添加买单 50（应该部分成交）
	buyOrder := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    50,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	_, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// 应该产生2笔成交：第一笔30，第二笔20
	if len(trades) != 2 {
		t.Fatalf("Expected 2 trades, got %d", len(trades))
	}
	if trades[0].Quantity != 30 {
		t.Errorf("Expected first trade quantity 30, got %d", trades[0].Quantity)
	}
	if trades[1].Quantity != 20 {
		t.Errorf("Expected second trade quantity 20, got %d", trades[1].Quantity)
	}
	totalFilled := trades[0].Quantity + trades[1].Quantity
	if totalFilled != 50 {
		t.Errorf("Expected total fill 50, got %d", totalFilled)
	}

	// 订单应该完全成交
	if buyOrder.Status != OrderStatusFilled {
		t.Errorf("Expected order status FILLED, got %s", buyOrder.Status)
	}

	t.Logf("Partial fill test passed")
}

// TestMarketOrder 测试市价单
func TestMarketOrder(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 添加限价卖单
	sellOrder := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    100,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}
	ob.PlaceOrder(sellOrder)

	// 添加市价买单
	marketBuy := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    50,
		OrderType:   OrderTypeMarket,
		Timestamp:   time.Now(),
	}

	_, trades, err := ob.PlaceOrder(marketBuy)
	if err != nil {
		t.Fatalf("PlaceOrder market buy failed: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}
	// 市价单应该以最优价格成交（最低卖价1000）
	if trades[0].Price != 1000 {
		t.Errorf("Expected trade price 1000, got %d", trades[0].Price)
	}

	t.Logf("Market order test passed")
}

// TestCancelOrder 测试取消订单
func TestCancelOrder(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))

	// 添加订单
	order := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    100,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	order, _, err := ob.PlaceOrder(order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// 取消订单
	err = ob.CancelOrder(order.ID, addr1)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	// 验证订单状态
	retrievedOrder, err := ob.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if retrievedOrder.Status != OrderStatusCancelled {
		t.Errorf("Expected status CANCELLED, got %s", retrievedOrder.Status)
	}

	t.Logf("Cancel order test passed")
}

// TestGetOrdersByOwner 测试获取用户订单
func TestGetOrdersByOwner(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 为 addr1 添加订单
	for i := 0; i < 3; i++ {
		order := &Order{
			Owner:       addr1,
			Side:        OrderSideBuy,
			Quantity:    100,
			Price:       uint64(1000 + i*10),
			OrderType:   OrderTypeLimit,
			Timestamp:   time.Now(),
		}
		ob.PlaceOrder(order)
	}

	// 为 addr2 添加订单
	order := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    100,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}
	ob.PlaceOrder(order)

	// 获取 addr1 的订单（应该返回3个，因为它们没有匹配）
	orders := ob.GetOrdersByOwner(addr1)
	if len(orders) != 3 {
		t.Errorf("Expected 3 orders for addr1, got %d", len(orders))
	}

	// 获取 addr2 的订单（应该返回1个）
	orders2 := ob.GetOrdersByOwner(addr2)
	if len(orders2) != 1 {
		t.Errorf("Expected 1 order for addr2, got %d", len(orders2))
	}

	t.Logf("GetOrdersByOwner test passed, found %d orders for addr1, %d for addr2", len(orders), len(orders2))
}

// TestGetDepth 测试订单簿深度
func TestGetDepth(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 添加多个买单价格级别
	for i := 0; i < 5; i++ {
		order := &Order{
			Owner:       addr1,
			Side:        OrderSideBuy,
			Quantity:    uint64((i + 1) * 10),
			Price:       uint64(1000 - i*10),
			OrderType:   OrderTypeLimit,
			Timestamp:   time.Now(),
		}
		ob.PlaceOrder(order)
	}

	// 添加多个卖单价格级别
	for i := 0; i < 5; i++ {
		order := &Order{
			Owner:       addr2,
			Side:        OrderSideSell,
			Quantity:    uint64((i + 1) * 10),
			Price:       uint64(1000 + i*10),
			OrderType:   OrderTypeLimit,
			Timestamp:   time.Now(),
		}
		ob.PlaceOrder(order)
	}

	// 获取深度
	bids, asks := ob.GetDepth(3)

	if len(bids) != 3 {
		t.Errorf("Expected 3 bid levels, got %d", len(bids))
	}
	if len(asks) != 3 {
		t.Errorf("Expected 3 ask levels, got %d", len(asks))
	}

	// 验证买单价格排序（降序）
	if len(bids) > 1 && bids[0].Price < bids[1].Price {
		t.Error("Bids should be in descending order")
	}

	// 验证卖单价格排序（升序）
	if len(asks) > 1 && asks[0].Price > asks[1].Price {
		t.Error("Asks should be in ascending order")
	}

	t.Logf("GetDepth test passed: bids=%v, asks=%v", bids, asks)
}

// TestOrderBookManager 测试订单簿管理器
func TestOrderBookManager(t *testing.T) {
	manager := NewOrderBookManager()

	// 获取或创建订单簿
	ob1 := manager.GetOrCreateOrderBook("AIB/USDT")
	ob2 := manager.GetOrCreateOrderBook("AIB/USDT")

	// 应该是同一个实例
	if ob1 != ob2 {
		t.Error("GetOrCreateOrderBook should return the same instance")
	}

	// 获取不存在的订单簿
	_, err := manager.GetOrderBook("ETH/USDT")
	if err != ErrOrderBookNotFound {
		t.Errorf("Expected ErrOrderBookNotFound, got %v", err)
	}

	// 列出交易对
	pairs := manager.ListTradingPairs()
	if len(pairs) != 1 || pairs[0] != "AIB/USDT" {
		t.Errorf("Expected [AIB/USDT], got %v", pairs)
	}

	t.Logf("OrderBookManager test passed")
}

// TestOrderStatusTransition 测试订单状态转换
func TestOrderStatusTransition(t *testing.T) {
	order := &Order{
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}

	// 测试部分成交
	filled := order.Fill(30)
	if filled != 30 {
		t.Errorf("Expected fill 30, got %d", filled)
	}
	if order.Status != OrderStatusPartialFilled {
		t.Errorf("Expected PARTIAL_FILLED, got %s", order.Status)
	}
	if order.RemainingQuantity() != 70 {
		t.Errorf("Expected remaining 70, got %d", order.RemainingQuantity())
	}

	// 测试完全成交
	filled = order.Fill(70)
	if filled != 70 {
		t.Errorf("Expected fill 70, got %d", filled)
	}
	if order.Status != OrderStatusFilled {
		t.Errorf("Expected FILLED, got %s", order.Status)
	}

	// 测试再次成交（应该返回0）
	filled = order.Fill(10)
	if filled != 0 {
		t.Errorf("Expected fill 0 for filled order, got %d", filled)
	}

	t.Logf("OrderStatusTransition test passed")
}

// TestPricePriority 测试价格优先原则
func TestPricePriority(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 添加多个卖单（不同价格）
	sellPrices := []uint64{1000, 1010, 1020, 1030}
	for _, price := range sellPrices {
		order := &Order{
			Owner:       addr2,
			Side:        OrderSideSell,
			Quantity:    10,
			Price:       price,
			OrderType:   OrderTypeLimit,
			Timestamp:   time.Now(),
		}
		ob.PlaceOrder(order)
	}

	// 添加买单，应该匹配最低卖价1000
	buyOrder := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    10,
		Price:       1050, // 愿意出更高价
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	_, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}
	// 应该以最低卖价成交
	if trades[0].Price != 1000 {
		t.Errorf("Expected trade price 1000 (lowest ask), got %d", trades[0].Price)
	}

	t.Logf("Price priority test passed")
}

// TestTimePriority 测试时间优先原则
func TestTimePriority(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")

	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 先添加两个卖单（相同价格）
	sellOrder1 := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    10,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now().Add(-time.Second), // 较早
	}
	ob.PlaceOrder(sellOrder1)

	sellOrder2 := &Order{
		Owner:       addr2,
		Side:        OrderSideSell,
		Quantity:    10,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(), // 较晚
	}
	ob.PlaceOrder(sellOrder2)

	// 添加买单
	buyOrder := &Order{
		Owner:       addr1,
		Side:        OrderSideBuy,
		Quantity:    10,
		Price:       1000,
		OrderType:   OrderTypeLimit,
		Timestamp:   time.Now(),
	}

	_, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// 成交的应该是较早的订单（sellOrder1是maker）
	if trades[0].MakerOrderID != sellOrder1.ID {
		t.Errorf("Expected maker order ID %d, got %d", sellOrder1.ID, trades[0].MakerOrderID)
	}

	t.Logf("Time priority test passed")
}

// ============================================================================
// 增强测试：边界条件和错误场景
// ============================================================================

// TestPlaceOrder_NilOrder 测试提交空订单
func TestPlaceOrder_NilOrder(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	_, _, err := ob.PlaceOrder(nil)
	if err == nil {
		t.Error("expected error for nil order")
	}
}

// TestPlaceOrder_ZeroQuantity 测试零数量订单
func TestPlaceOrder_ZeroQuantity(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))

	order := &Order{
		Owner:     addr,
		Side:      OrderSideBuy,
		Quantity:  0,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	_, _, err := ob.PlaceOrder(order)
	if err != ErrInvalidQuantity {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}
}

// TestPlaceOrder_LimitOrderZeroPrice 测试限价单零价格
func TestPlaceOrder_LimitOrderZeroPrice(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))

	order := &Order{
		Owner:     addr,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     0,
		OrderType: OrderTypeLimit,
	}
	_, _, err := ob.PlaceOrder(order)
	if err != ErrInvalidPrice {
		t.Errorf("expected ErrInvalidPrice, got %v", err)
	}
}

// TestPlaceOrder_InvalidOwner 测试空所有者
func TestPlaceOrder_InvalidOwner(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	order := &Order{
		Owner:     interfaces.Address{},
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	_, _, err := ob.PlaceOrder(order)
	if err == nil {
		t.Error("expected error for empty owner")
	}
}

// TestCancelOrder_NotFound 测试取消不存在的订单
func TestCancelOrder_NotFound(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))

	err := ob.CancelOrder(99999, addr)
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}
}

// TestCancelOrder_Unauthorized 测试未授权取消订单
func TestCancelOrder_Unauthorized(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	order := &Order{
		Owner:     addr1,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	placed, _, _ := ob.PlaceOrder(order)
	err := ob.CancelOrder(placed.ID, addr2)
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

// TestCancelOrder_AlreadyCancelled 测试取消已取消的订单
func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))

	order := &Order{
		Owner:     addr,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	placed, _, _ := ob.PlaceOrder(order)
	ob.CancelOrder(placed.ID, addr)

	err := ob.CancelOrder(placed.ID, addr)
	if err != ErrOrderNotPending {
		t.Errorf("expected ErrOrderNotPending, got %v", err)
	}
}

// TestGetOrder_NotFound 测试获取不存在的订单
func TestGetOrder_NotFound(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	_, err := ob.GetOrder(99999)
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}
}

// TestOrderBook_SellOrderMatchingBids 测试卖单匹配买单
func TestOrderBook_SellOrderMatchingBids(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 先放买单
	buyOrder := &Order{
		Owner:     addr1,
		Side:      OrderSideBuy,
		Quantity:  100,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	ob.PlaceOrder(buyOrder)

	// 放匹配的卖单
	sellOrder := &Order{
		Owner:     addr2,
		Side:      OrderSideSell,
		Quantity:  50,
		Price:     1000,
		OrderType: OrderTypeLimit,
	}
	_, trades, err := ob.PlaceOrder(sellOrder)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 50 {
		t.Errorf("expected trade qty 50, got %d", trades[0].Quantity)
	}
}

// TestOrderBook_MarketSellMatchingBids 测试市价卖单匹配
func TestOrderBook_MarketSellMatchingBids(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 先放多个买单
	ob.PlaceOrder(&Order{Owner: addr1, Side: OrderSideBuy, Quantity: 30, Price: 1000, OrderType: OrderTypeLimit})
	ob.PlaceOrder(&Order{Owner: addr1, Side: OrderSideBuy, Quantity: 30, Price: 990, OrderType: OrderTypeLimit})

	// 市价卖单
	_, trades, err := ob.PlaceOrder(&Order{
		Owner:     addr2,
		Side:      OrderSideSell,
		Quantity:  50,
		OrderType: OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(trades) < 1 {
		t.Fatalf("expected at least 1 trade, got %d", len(trades))
	}
}

// TestOrderBook_NoMatchWhenPricesDontCross 测试价格不交叉时不成交
func TestOrderBook_NoMatchWhenPricesDontCross(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	// 卖单价格高于买单
	ob.PlaceOrder(&Order{Owner: addr2, Side: OrderSideSell, Quantity: 100, Price: 2000, OrderType: OrderTypeLimit})
	_, trades, err := ob.PlaceOrder(&Order{Owner: addr1, Side: OrderSideBuy, Quantity: 100, Price: 1000, OrderType: OrderTypeLimit})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades when prices don't cross, got %d", len(trades))
	}
}

// TestOrderBook_MultipleMatchesExhaustTaker 测试 taker 被完全成交
func TestOrderBook_MultipleMatchesExhaustTaker(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	ob.PlaceOrder(&Order{Owner: addr2, Side: OrderSideSell, Quantity: 20, Price: 1000, OrderType: OrderTypeLimit})
	ob.PlaceOrder(&Order{Owner: addr2, Side: OrderSideSell, Quantity: 20, Price: 1010, OrderType: OrderTypeLimit})
	ob.PlaceOrder(&Order{Owner: addr2, Side: OrderSideSell, Quantity: 20, Price: 1020, OrderType: OrderTypeLimit})

	buyOrder := &Order{Owner: addr1, Side: OrderSideBuy, Quantity: 35, Price: 1015, OrderType: OrderTypeLimit}
	placed, trades, err := ob.PlaceOrder(buyOrder)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	// 应该匹配 20@1000 + 15@1010 = 35 total
	totalFilled := uint64(0)
	for _, tr := range trades {
		totalFilled += tr.Quantity
	}
	if totalFilled != 35 {
		t.Errorf("expected total filled 35, got %d", totalFilled)
	}
	if placed.Status != OrderStatusFilled {
		t.Errorf("expected FILLED, got %s", placed.Status)
	}
}

// TestOrderBook_GetTrades 测试获取成交记录
func TestOrderBook_GetTrades(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	ob.PlaceOrder(&Order{Owner: addr2, Side: OrderSideSell, Quantity: 100, Price: 1000, OrderType: OrderTypeLimit})
	ob.PlaceOrder(&Order{Owner: addr1, Side: OrderSideBuy, Quantity: 100, Price: 1000, OrderType: OrderTypeLimit})

	allTrades := ob.GetTrades()
	if len(allTrades) != 1 {
		t.Errorf("expected 1 trade in history, got %d", len(allTrades))
	}
}

// TestOrderBook_ConcurrentAccess 测试并发安全性
func TestOrderBook_ConcurrentAccess(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	var wg sync.WaitGroup
	orderCount := 50

	// 并发下买单
	for i := 0; i < orderCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			order := &Order{
				Owner:     addr1,
				Side:      OrderSideBuy,
				Quantity:  10,
				Price:     uint64(1000 + idx),
				OrderType: OrderTypeLimit,
			}
			ob.PlaceOrder(order)
		}(i)
	}

	// 并发下卖单
	for i := 0; i < orderCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			order := &Order{
				Owner:     addr2,
				Side:      OrderSideSell,
				Quantity:  10,
				Price:     uint64(2000 + idx),
				OrderType: OrderTypeLimit,
			}
			ob.PlaceOrder(order)
		}(i)
	}

	wg.Wait()

	// 验证没有 panic，状态一致
	bids := ob.GetBids(0)
	asks := ob.GetAsks(0)
	t.Logf("Concurrent test: %d bids, %d asks", len(bids), len(asks))
}

// TestOrder_RemainingQuantityOverflow 测试 RemainingQuantity 边界
func TestOrder_RemainingQuantityOverflow(t *testing.T) {
	order := &Order{
		Quantity:       50,
		FilledQuantity: 100, // FilledQuantity > Quantity (异常场景)
	}
	remaining := order.RemainingQuantity()
	if remaining != 0 {
		t.Errorf("expected 0, got %d", remaining)
	}
}

// TestOrder_IsActive 测试订单活跃状态
func TestOrder_IsActive(t *testing.T) {
	tests := []struct {
		status   OrderStatus
		expected bool
	}{
		{OrderStatusPending, true},
		{OrderStatusPartialFilled, true},
		{OrderStatusFilled, false},
		{OrderStatusCancelled, false},
		{OrderStatusExpired, false},
	}

	for _, tt := range tests {
		order := &Order{Status: tt.status}
		if order.IsActive() != tt.expected {
			t.Errorf("IsActive for status %s: expected %v, got %v", tt.status, tt.expected, order.IsActive())
		}
	}
}

// TestOrder_IsFilled 测试完全成交判断
func TestOrder_IsFilled(t *testing.T) {
	order1 := &Order{Quantity: 100, FilledQuantity: 100, Status: OrderStatusFilled}
	if !order1.IsFilled() {
		t.Error("order should be filled")
	}

	order2 := &Order{Quantity: 100, FilledQuantity: 50, Status: OrderStatusPartialFilled}
	if order2.IsFilled() {
		t.Error("order should not be filled")
	}
}

// TestOrderStatus_String 测试状态字符串
func TestOrderStatus_String(t *testing.T) {
	tests := []struct {
		status   OrderStatus
		expected string
	}{
		{OrderStatusPending, "PENDING"},
		{OrderStatusPartialFilled, "PARTIAL_FILLED"},
		{OrderStatusFilled, "FILLED"},
		{OrderStatusCancelled, "CANCELLED"},
		{OrderStatusExpired, "EXPIRED"},
		{OrderStatus(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if tt.status.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.status.String())
		}
	}
}

// TestOrderType_String 测试订单类型字符串
func TestOrderType_String(t *testing.T) {
	if OrderTypeLimit.String() != "LIMIT" {
		t.Errorf("expected LIMIT")
	}
	if OrderTypeMarket.String() != "MARKET" {
		t.Errorf("expected MARKET")
	}
	if OrderType(99).String() != "UNKNOWN" {
		t.Errorf("expected UNKNOWN")
	}
}

// TestOrderSide_String 测试订单方向字符串
func TestOrderSide_String(t *testing.T) {
	if OrderSideBuy.String() != "BUY" {
		t.Errorf("expected BUY")
	}
	if OrderSideSell.String() != "SELL" {
		t.Errorf("expected SELL")
	}
	if OrderSide(99).String() != "UNKNOWN" {
		t.Errorf("expected UNKNOWN")
	}
}

// TestOrderSide_IsOpposite 测试对手方向判断
func TestOrderSide_IsOpposite(t *testing.T) {
	if !OrderSideBuy.IsOpposite(OrderSideSell) {
		t.Error("BUY should be opposite to SELL")
	}
	if !OrderSideSell.IsOpposite(OrderSideBuy) {
		t.Error("SELL should be opposite to BUY")
	}
	if OrderSideBuy.IsOpposite(OrderSideBuy) {
		t.Error("BUY should NOT be opposite to BUY")
	}
}

// TestGetDepth_Limit 测试深度限制
func TestGetDepth_Limit(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))

	for i := 1; i <= 10; i++ {
		ob.PlaceOrder(&Order{
			Owner: addr, Side: OrderSideBuy, Quantity: 10,
			Price: uint64(1000 - i*10), OrderType: OrderTypeLimit,
		})
		ob.PlaceOrder(&Order{
			Owner: addr, Side: OrderSideSell, Quantity: 10,
			Price: uint64(2000 + i*10), OrderType: OrderTypeLimit,
		})
	}

	bids, asks := ob.GetDepth(3)
	if len(bids) != 3 {
		t.Errorf("expected 3 bid levels, got %d", len(bids))
	}
	if len(asks) != 3 {
		t.Errorf("expected 3 ask levels, got %d", len(asks))
	}
}

// TestOrderBookManager_MultiPairs 测试多交易对管理
func TestOrderBookManager_MultiPairs(t *testing.T) {
	mgr := NewOrderBookManager()
	pairs := []string{"AIB/USDT", "BTC/USDT", "ETH/USDT", "AIB/BTC"}

	for _, pair := range pairs {
		mgr.GetOrCreateOrderBook(pair)
	}

	listed := mgr.ListTradingPairs()
	if len(listed) != len(pairs) {
		t.Errorf("expected %d pairs, got %d", len(pairs), len(listed))
	}
}

// TestOrder_FillZero 测试 Fill(0) 边界
func TestOrder_FillZero(t *testing.T) {
	order := &Order{
		Quantity:       100,
		FilledQuantity: 0,
		Status:         OrderStatusPending,
	}
	filled := order.Fill(0)
	if filled != 0 {
		t.Errorf("expected 0, got %d", filled)
	}
}

// TestOrder_Expiration 测试过期逻辑
func TestOrder_Expiration(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	expired := &Order{Expiration: &past}
	if !expired.IsExpired() {
		t.Error("should be expired")
	}

	notExpired := &Order{Expiration: &future}
	if notExpired.IsExpired() {
		t.Error("should not be expired")
	}

	noExpiry := &Order{Expiration: nil}
	if noExpiry.IsExpired() {
		t.Error("nil expiration should not be expired")
	}
}

// TestGetBids_Empty 测试空订单簿的 GetBids
func TestGetBids_Empty(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	bids := ob.GetBids(10)
	if len(bids) != 0 {
		t.Errorf("expected 0 bids, got %d", len(bids))
	}
}

// TestGetAsks_Empty 测试空订单簿的 GetAsks
func TestGetAsks_Empty(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	asks := ob.GetAsks(10)
	if len(asks) != 0 {
		t.Errorf("expected 0 asks, got %d", len(asks))
	}
}

// TestOrderString 测试 Order String 方法
func TestOrderString(t *testing.T) {
	addr := interfaces.Address{}
	copy(addr[:], []byte("test_address_1__________"))
	order := &Order{
		ID: 1, Owner: addr, TradingPair: "AIB/USDT", Side: OrderSideBuy,
		Quantity: 100, FilledQuantity: 50, Price: 1000,
		OrderType: OrderTypeLimit, Status: OrderStatusPartialFilled,
	}
	s := order.String()
	if s == "" {
		t.Error("string should not be empty")
	}
}

// TestTradeString 测试 Trade String 方法
func TestTradeString(t *testing.T) {
	addr1 := interfaces.Address{}
	addr2 := interfaces.Address{}
	copy(addr1[:], []byte("test_address_1__________"))
	copy(addr2[:], []byte("test_address_2__________"))

	trade := &Trade{
		ID: 1, TradingPair: "AIB/USDT",
		MakerOrderID: 1, TakerOrderID: 2,
		Maker: addr1, Taker: addr2,
		Side: OrderSideBuy, Quantity: 50, Price: 1000,
	}
	s := trade.String()
	if s == "" {
		t.Error("string should not be empty")
	}
}

// TestOrderBookString 测试 OrderBook String 方法
func TestOrderBookString(t *testing.T) {
	ob := NewOrderBook("AIB/USDT")
	s := ob.String()
	if s == "" {
		t.Error("string should not be empty")
	}
}
