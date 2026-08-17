// Package channel implements Lightning-style state channels for AIB 2.0.
// It provides order book functionality for L2 trading within channels.
package channel

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// 订单状态常量
// ============================================================================

// OrderStatus 表示订单的当前状态
type OrderStatus int

const (
	OrderStatusPending       OrderStatus = iota // 待成交
	OrderStatusPartialFilled                    // 部分成交
	OrderStatusFilled                           // 完全成交
	OrderStatusCancelled                        // 已取消
	OrderStatusExpired                          // 已过期
)

// String 返回订单状态的字符串表示
func (s OrderStatus) String() string {
	switch s {
	case OrderStatusPending:
		return "PENDING"
	case OrderStatusPartialFilled:
		return "PARTIAL_FILLED"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusCancelled:
		return "CANCELLED"
	case OrderStatusExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// 订单类型常量
// ============================================================================

// OrderType 表示订单类型
type OrderType int

const (
	OrderTypeLimit  OrderType = iota // 限价单
	OrderTypeMarket                  // 市价单
)

// String 返回订单类型的字符串表示
func (t OrderType) String() string {
	switch t {
	case OrderTypeLimit:
		return "LIMIT"
	case OrderTypeMarket:
		return "MARKET"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// 订单方向常量
// ============================================================================

// OrderSide 表示订单方向（买单或卖单）
type OrderSide int

const (
	OrderSideBuy  OrderSide = iota // 买单
	OrderSideSell                  // 卖单
)

// String 返回订单方向的字符串表示
func (s OrderSide) String() string {
	switch s {
	case OrderSideBuy:
		return "BUY"
	case OrderSideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

// IsOpposite 返回两个订单方向是否相反
func (s OrderSide) IsOpposite(other OrderSide) bool {
	return s != other
}

// ============================================================================
// 错误定义
// ============================================================================

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrInvalidQuantity     = errors.New("invalid quantity")
	ErrInvalidPrice        = errors.New("invalid price for limit order")
	ErrOrderNotPending     = errors.New("order is not in pending status")
	ErrUnauthorized        = errors.New("unauthorized operation")
	ErrTradingPairMismatch = errors.New("trading pair mismatch")
	ErrOrderBookNotFound   = errors.New("order book not found")
	ErrDuplicateOrder      = errors.New("duplicate order ID")
)

// ============================================================================
// Order 结构体
// ============================================================================

// Order 代表一个订单
type Order struct {
	ID             uint64             // 唯一订单ID
	Owner          interfaces.Address // 订单所有者
	TradingPair    string             // 交易对 (如 "AIB/USDT")
	Side           OrderSide          // 订单方向 (BUY/SELL)
	Quantity       uint64             // 订单总数量
	FilledQuantity uint64             // 已成交数量
	Price          uint64             // 订单价格 (0 表示市价单)
	OrderType      OrderType          // 订单类型 (限价单/市价单)
	Status         OrderStatus        // 订单状态
	Timestamp      time.Time          // 创建时间
	Expiration     *time.Time         // 过期时间 (可选)
}

// RemainingQuantity 返回订单的剩余未成交数量
func (o *Order) RemainingQuantity() uint64 {
	if o.Quantity >= o.FilledQuantity {
		return o.Quantity - o.FilledQuantity
	}
	return 0
}

// IsFilled 检查订单是否完全成交
func (o *Order) IsFilled() bool {
	return o.Status == OrderStatusFilled || o.FilledQuantity >= o.Quantity
}

// IsActive 检查订单是否处于活跃状态（可成交）
func (o *Order) IsActive() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusPartialFilled
}

// IsExpired 检查订单是否已过期
func (o *Order) IsExpired() bool {
	if o.Expiration == nil {
		return false
	}
	return time.Now().After(*o.Expiration)
}

// Fill 成交指定数量的订单
func (o *Order) Fill(quantity uint64) uint64 {
	remaining := o.RemainingQuantity()
	if quantity >= remaining {
		// 完全成交
		o.FilledQuantity = o.Quantity
		o.Status = OrderStatusFilled
		return remaining
	}
	// 部分成交
	o.FilledQuantity += quantity
	o.Status = OrderStatusPartialFilled
	return quantity
}

// ============================================================================
// Trade 结构体
// ============================================================================

// Trade 代表一次成交记录
type Trade struct {
	ID           uint64             // 唯一成交ID
	TradingPair  string             // 交易对
	MakerOrderID uint64             // Maker订单ID
	TakerOrderID uint64             // Taker订单ID
	Maker        interfaces.Address // Maker地址
	Taker        interfaces.Address // Taker地址
	Side         OrderSide          // 成交方向 (买单/卖单)
	Quantity     uint64             // 成交数量
	Price        uint64             // 成交价格
	Timestamp    time.Time          // 成交时间
}

// ============================================================================
// OrderBook 结构体
// ============================================================================

// priceLevel 表示同一价格的订单级别
type priceLevel struct {
	price    uint64     // 价格
	orders   *list.List // 订单列表 (按时间顺序)
	totalQty uint64     // 该价格级别的总数量
}

// OrderBook 代表一个交易对的订单簿
type OrderBook struct {
	tradingPair string                   // 交易对
	bids        map[uint64]*priceLevel   // 买单按价格索引 (价格 -> priceLevel)
	asks        map[uint64]*priceLevel   // 卖单按价格索引 (价格 -> priceLevel)
	bidPrices   []uint64                 // 买单价格排序 (降序)
	askPrices   []uint64                 // 卖单价格排序 (升序)
	orders      map[uint64]*list.Element // 活跃订单ID -> 订单列表中的元素 (仅包含在订单簿中的活跃订单)
	allOrders   map[uint64]*Order        // 所有订单ID -> 订单 (包含已取消、已成交等所有状态)
	trades      []*Trade                 // 成交记录
	orderIDSeq  uint64                   // 订单ID序列号
	tradeIDSeq  uint64                   // 成交ID序列号
	mu          sync.RWMutex             // 读写锁
}

// NewOrderBook 创建一个新的订单簿
func NewOrderBook(tradingPair string) *OrderBook {
	return &OrderBook{
		tradingPair: tradingPair,
		bids:        make(map[uint64]*priceLevel),
		asks:        make(map[uint64]*priceLevel),
		bidPrices:   make([]uint64, 0),
		askPrices:   make([]uint64, 0),
		orders:      make(map[uint64]*list.Element),
		allOrders:   make(map[uint64]*Order),
		trades:      make([]*Trade, 0),
		orderIDSeq:  0,
		tradeIDSeq:  0,
	}
}

// generateOrderID 生成唯一的订单ID
func (ob *OrderBook) generateOrderID() uint64 {
	ob.orderIDSeq++
	// 使用时间戳和序列号生成唯一ID
	ts := uint64(time.Now().UnixNano())
	return ts<<16 | ob.orderIDSeq
}

// generateTradeID 生成唯一的成交ID
func (ob *OrderBook) generateTradeID() uint64 {
	ob.tradeIDSeq++
	ts := uint64(time.Now().UnixNano())
	return ts<<16 | ob.tradeIDSeq
}

// generateOrderHash 生成订单的唯一哈希
func generateOrderHash(owner interfaces.Address, tradingPair string, side OrderSide, quantity, price uint64, timestamp time.Time) [32]byte {
	h := sha256.New()
	h.Write(owner[:])
	h.Write([]byte(tradingPair))
	h.Write([]byte{byte(side)})
	h.Write(binary.BigEndian.AppendUint64(nil, quantity))
	h.Write(binary.BigEndian.AppendUint64(nil, price))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(timestamp.UnixNano())))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// ============================================================================
// 订单操作方法
// ============================================================================

// PlaceOrder 将订单添加到订单簿并尝试匹配
// 返回更新后的订单和可能产生的成交记录
func (ob *OrderBook) PlaceOrder(order *Order) (*Order, []*Trade, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// 验证订单
	if err := ob.validateOrder(order); err != nil {
		return nil, nil, err
	}

	// 如果没有指定订单ID，则生成一个
	if order.ID == 0 {
		order.ID = ob.generateOrderID()
	}

	// 如果没有指定时间戳，则使用当前时间
	if order.Timestamp.IsZero() {
		order.Timestamp = time.Now()
	}

	// 设置初始状态
	if order.Status == 0 {
		order.Status = OrderStatusPending
	}

	// 设置交易对
	order.TradingPair = ob.tradingPair

	// 尝试匹配订单
	newTrades := ob.matchOrder(order)

	// 如果订单还有剩余未成交，且状态为活跃，则添加到订单簿
	if order.IsActive() && order.RemainingQuantity() > 0 {
		ob.addOrderToBook(order)
	}

	// 保存订单到allOrders
	ob.allOrders[order.ID] = order

	return order, newTrades, nil
}

// validateOrder 验证订单的有效性
func (ob *OrderBook) validateOrder(order *Order) error {
	if order == nil {
		return errors.New("order is nil")
	}
	if order.Quantity == 0 {
		return ErrInvalidQuantity
	}
	if order.Side != OrderSideBuy && order.Side != OrderSideSell {
		return errors.New("invalid order side")
	}
	if order.OrderType != OrderTypeLimit && order.OrderType != OrderTypeMarket {
		return errors.New("invalid order type")
	}
	// 限价单必须有有效价格
	if order.OrderType == OrderTypeLimit && order.Price == 0 {
		return ErrInvalidPrice
	}
	// 检查订单所有者
	if order.Owner == (interfaces.Address{}) {
		return errors.New("invalid order owner")
	}
	return nil
}

// addOrderToBook 将订单添加到订单簿
func (ob *OrderBook) addOrderToBook(order *Order) {
	var (
		priceMap  map[uint64]*priceLevel
		priceList *[]uint64
	)

	if order.Side == OrderSideBuy {
		priceMap = ob.bids
		priceList = &ob.bidPrices
	} else {
		priceMap = ob.asks
		priceList = &ob.askPrices
	}

	// 获取或创建价格级别
	level, exists := priceMap[order.Price]
	if !exists {
		level = &priceLevel{
			price:  order.Price,
			orders: list.New(),
		}
		priceMap[order.Price] = level
		// 添加到价格列表并排序
		*priceList = append(*priceList, order.Price)
		ob.sortPriceList(priceList, order.Side == OrderSideBuy)
	}

	// 添加订单到价格级别
	elem := level.orders.PushBack(order)
	ob.orders[order.ID] = elem
	level.totalQty += order.RemainingQuantity()
}

// sortPriceList 对价格列表进行排序
// isDescending: 买单为降序，卖单为升序
func (ob *OrderBook) sortPriceList(prices *[]uint64, isDescending bool) {
	if isDescending {
		// 买单：降序（高价在前）
		for i := 0; i < len(*prices)-1; i++ {
			for j := i + 1; j < len(*prices); j++ {
				if (*prices)[i] < (*prices)[j] {
					(*prices)[i], (*prices)[j] = (*prices)[j], (*prices)[i]
				}
			}
		}
	} else {
		// 卖单：升序（低价在前）
		for i := 0; i < len(*prices)-1; i++ {
			for j := i + 1; j < len(*prices); j++ {
				if (*prices)[i] > (*prices)[j] {
					(*prices)[i], (*prices)[j] = (*prices)[j], (*prices)[i]
				}
			}
		}
	}
}

// CancelOrder 取消指定订单
func (ob *OrderBook) CancelOrder(orderID uint64, owner interfaces.Address) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// 先从allOrders查找订单
	order, exists := ob.allOrders[orderID]
	if !exists {
		return ErrOrderNotFound
	}

	// 验证订单所有者
	if order.Owner != owner {
		return ErrUnauthorized
	}

	// 检查订单状态
	if !order.IsActive() {
		return ErrOrderNotPending
	}

	// 更新订单状态
	order.Status = OrderStatusCancelled

	// 从活跃订单簿中移除
	if elem, ok := ob.orders[orderID]; ok {
		ob.removeOrderFromBook(order, elem)
	}

	return nil
}

// removeOrderFromBook 从订单簿中移除订单
func (ob *OrderBook) removeOrderFromBook(order *Order, elem *list.Element) {
	var priceMap map[uint64]*priceLevel
	if order.Side == OrderSideBuy {
		priceMap = ob.bids
	} else {
		priceMap = ob.asks
	}

	level, exists := priceMap[order.Price]
	if !exists {
		return
	}

	// 从订单列表中移除
	level.orders.Remove(elem)
	level.totalQty -= order.RemainingQuantity()

	// 如果该价格级别没有订单了，删除该级别
	if level.orders.Len() == 0 {
		delete(priceMap, order.Price)
		// 从价格列表中移除
		ob.removePriceFromList(order.Price, order.Side == OrderSideBuy)
	}

	// 从活跃订单映射中删除（但allOrders中仍然保留）
	delete(ob.orders, order.ID)
}

// removePriceFromList 从价格列表中移除指定价格
func (ob *OrderBook) removePriceFromList(price uint64, isBid bool) {
	var priceList *[]uint64
	if isBid {
		priceList = &ob.bidPrices
	} else {
		priceList = &ob.askPrices
	}

	for i, p := range *priceList {
		if p == price {
			*priceList = append((*priceList)[:i], (*priceList)[i+1:]...)
			return
		}
	}
}

// GetOrder 根据订单ID获取订单（包括所有状态的订单）
func (ob *OrderBook) GetOrder(orderID uint64) (*Order, error) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	order, exists := ob.allOrders[orderID]
	if !exists {
		return nil, ErrOrderNotFound
	}

	// 返回订单的副本
	orderCopy := *order
	return &orderCopy, nil
}

// GetOrdersByOwner 获取指定用户的所有订单（包括所有状态的订单）
func (ob *OrderBook) GetOrdersByOwner(owner interfaces.Address) []*Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	result := make([]*Order, 0)
	for _, order := range ob.allOrders {
		if order.Owner == owner {
			orderCopy := *order
			result = append(result, &orderCopy)
		}
	}
	return result
}

// GetBids 获取买单列表（按价格降序）
func (ob *OrderBook) GetBids(limit int) []*Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	result := make([]*Order, 0)
	for _, price := range ob.bidPrices {
		level := ob.bids[price]
		for elem := level.orders.Front(); elem != nil; elem = elem.Next() {
			order := elem.Value.(*Order)
			if order.IsActive() {
				orderCopy := *order
				result = append(result, &orderCopy)
				if limit > 0 && len(result) >= limit {
					return result
				}
			}
		}
	}
	return result
}

// GetAsks 获取卖单列表（按价格升序）
func (ob *OrderBook) GetAsks(limit int) []*Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	result := make([]*Order, 0)
	for _, price := range ob.askPrices {
		level := ob.asks[price]
		for elem := level.orders.Front(); elem != nil; elem = elem.Next() {
			order := elem.Value.(*Order)
			if order.IsActive() {
				orderCopy := *order
				result = append(result, &orderCopy)
				if limit > 0 && len(result) >= limit {
					return result
				}
			}
		}
	}
	return result
}

// GetTrades 获取成交记录
func (ob *OrderBook) GetTrades() []*Trade {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	result := make([]*Trade, len(ob.trades))
	copy(result, ob.trades)
	return result
}

// GetDepth 获取订单簿深度
func (ob *OrderBook) GetDepth(levels int) (bids []struct {
	Price    uint64
	Quantity uint64
}, asks []struct {
	Price    uint64
	Quantity uint64
}) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids = make([]struct {
		Price    uint64
		Quantity uint64
	}, 0, levels)
	asks = make([]struct {
		Price    uint64
		Quantity uint64
	}, 0, levels)

	// 获取买单深度（按价格降序）
	for i := 0; i < len(ob.bidPrices) && i < levels; i++ {
		price := ob.bidPrices[i]
		if level, exists := ob.bids[price]; exists {
			bids = append(bids, struct {
				Price    uint64
				Quantity uint64
			}{Price: price, Quantity: level.totalQty})
		}
	}

	// 获取卖单深度（按价格升序）
	for i := 0; i < len(ob.askPrices) && i < levels; i++ {
		price := ob.askPrices[i]
		if level, exists := ob.asks[price]; exists {
			asks = append(asks, struct {
				Price    uint64
				Quantity uint64
			}{Price: price, Quantity: level.totalQty})
		}
	}

	return
}

// ============================================================================
// 订单匹配引擎
// ============================================================================

// matchOrder 尝试匹配订单
// 返回成交记录列表
func (ob *OrderBook) matchOrder(order *Order) []*Trade {
	var newTrades []*Trade

	// 确定对手方订单簿
	var oppositeBook map[uint64]*priceLevel
	var oppositePrices *[]uint64

	if order.Side == OrderSideBuy {
		// 买单寻找卖单（asks - 升序，最低卖价优先）
		oppositeBook = ob.asks
		oppositePrices = &ob.askPrices
	} else {
		// 卖单寻找买单（bids - 降序，最高买价优先）
		oppositeBook = ob.bids
		oppositePrices = &ob.bidPrices
	}

	// 遍历对手方价格列表
	for order.IsActive() && order.RemainingQuantity() > 0 {
		// 找到最优价格
		bestPrice := ob.findBestPrice(order, oppositeBook, oppositePrices)
		if bestPrice == 0 {
			// 没有可匹配的对手单
			break
		}

		// 检查价格是否可以成交
		if !ob.canMatch(order, bestPrice) {
			break
		}

		// 获取该价格级别的订单
		level, exists := oppositeBook[bestPrice]
		if !exists || level.orders.Len() == 0 {
			break
		}

		// 从最老的订单开始匹配（时间优先）
		elem := level.orders.Front()
		makerOrder := elem.Value.(*Order)

		// 执行成交
		trade := ob.executeTrade(order, makerOrder, bestPrice)
		newTrades = append(newTrades, trade)

		// 更新订单簿
		if makerOrder.IsFilled() || makerOrder.Status == OrderStatusCancelled {
			ob.removeOrderFromBook(makerOrder, elem)
		}
	}

	return newTrades
}

// findBestPrice 找到最优可匹配价格
func (ob *OrderBook) findBestPrice(order *Order, oppositeBook map[uint64]*priceLevel, oppositePrices *[]uint64) uint64 {
	if len(*oppositePrices) == 0 {
		return 0
	}

	if order.OrderType == OrderTypeMarket {
		// 市价单：以对手最优价格成交
		// 买单找最低卖价，卖单找最高买价
		if order.Side == OrderSideBuy {
			// 买单：找最低卖价
			return (*oppositePrices)[0]
		} else {
			// 卖单：找最高买价
			prices := *oppositePrices
			return prices[len(prices)-1]
		}
	}

	// 限价单：按价格规则匹配
	if order.Side == OrderSideBuy {
		// 买单：找价格 <= 订单价格的最低卖单
		for _, price := range *oppositePrices {
			if price <= order.Price {
				return price
			}
		}
	} else {
		// 卖单：找价格 >= 订单价格的最高买单
		prices := *oppositePrices
		for i := len(prices) - 1; i >= 0; i-- {
			if prices[i] >= order.Price {
				return prices[i]
			}
		}
	}

	return 0
}

// canMatch 检查价格是否允许成交
func (ob *OrderBook) canMatch(order *Order, price uint64) bool {
	if order.OrderType == OrderTypeMarket {
		return true
	}

	if order.Side == OrderSideBuy {
		// 买单：成交价不能高于订单价格
		return price <= order.Price
	} else {
		// 卖单：成交价不能低于订单价格
		return price >= order.Price
	}
}

// executeTrade 执行一次成交
func (ob *OrderBook) executeTrade(takerOrder, makerOrder *Order, price uint64) *Trade {
	// 计算成交数量
	quantity := takerOrder.RemainingQuantity()
	makerRemaining := makerOrder.RemainingQuantity()

	if quantity > makerRemaining {
		quantity = makerRemaining
	}

	// 成交
	takerFilled := takerOrder.Fill(quantity)
	makerOrder.Fill(quantity)

	// 确定谁是maker谁是taker
	var maker, taker interfaces.Address
	var makerOrderID, takerOrderID uint64

	// 按时间优先，先提交的订单是maker
	if makerOrder.Timestamp.Before(takerOrder.Timestamp) ||
		(makerOrder.Timestamp.Equal(takerOrder.Timestamp) && makerOrder.ID < takerOrder.ID) {
		maker = makerOrder.Owner
		taker = takerOrder.Owner
		makerOrderID = makerOrder.ID
		takerOrderID = takerOrder.ID
	} else {
		maker = takerOrder.Owner
		taker = makerOrder.Owner
		makerOrderID = takerOrder.ID
		takerOrderID = makerOrder.ID
	}

	trade := &Trade{
		ID:           ob.generateTradeID(),
		TradingPair:  ob.tradingPair,
		MakerOrderID: makerOrderID,
		TakerOrderID: takerOrderID,
		Maker:        maker,
		Taker:        taker,
		Side:         takerOrder.Side, // Taker的方向
		Quantity:     takerFilled,
		Price:        price,
		Timestamp:    time.Now(),
	}

	ob.trades = append(ob.trades, trade)

	return trade
}

// ============================================================================
// OrderBookManager - 订单簿管理器
// ============================================================================

// OrderBookManager 管理多个交易对的订单簿
type OrderBookManager struct {
	orderBooks map[string]*OrderBook // tradingPair -> OrderBook
	mu         sync.RWMutex
}

// NewOrderBookManager 创建一个新的订单簿管理器
func NewOrderBookManager() *OrderBookManager {
	return &OrderBookManager{
		orderBooks: make(map[string]*OrderBook),
	}
}

// GetOrCreateOrderBook 获取或创建指定交易对的订单簿
func (m *OrderBookManager) GetOrCreateOrderBook(tradingPair string) *OrderBook {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ob, exists := m.orderBooks[tradingPair]; exists {
		return ob
	}

	ob := NewOrderBook(tradingPair)
	m.orderBooks[tradingPair] = ob
	return ob
}

// GetOrderBook 获取指定交易对的订单簿
func (m *OrderBookManager) GetOrderBook(tradingPair string) (*OrderBook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ob, exists := m.orderBooks[tradingPair]; exists {
		return ob, nil
	}
	return nil, ErrOrderBookNotFound
}

// ListTradingPairs 列出所有支持的交易对
func (m *OrderBookManager) ListTradingPairs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, 0, len(m.orderBooks))
	for pair := range m.orderBooks {
		result = append(result, pair)
	}
	return result
}

// ============================================================================
// 辅助方法
// ============================================================================

// String 返回订单的字符串表示
func (o *Order) String() string {
	return fmt.Sprintf("Order{id=%d, owner=%s, pair=%s, side=%s, qty=%d, filled=%d, price=%d, type=%s, status=%s}",
		o.ID, o.Owner[:8], o.TradingPair, o.Side, o.Quantity, o.FilledQuantity, o.Price, o.OrderType, o.Status)
}

// String 返回成交记录的字符串表示
func (t *Trade) String() string {
	return fmt.Sprintf("Trade{id=%d, pair=%s, maker=%s, taker=%s, side=%s, qty=%d, price=%d}",
		t.ID, t.TradingPair, t.Maker[:8], t.Taker[:8], t.Side, t.Quantity, t.Price)
}

// String 返回订单簿的字符串表示
func (ob *OrderBook) String() string {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return fmt.Sprintf("OrderBook{pair=%s, bids=%d, asks=%d, orders=%d, trades=%d}",
		ob.tradingPair, len(ob.bidPrices), len(ob.askPrices), len(ob.orders), len(ob.trades))
}
