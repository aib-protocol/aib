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
// Order status constants
// ============================================================================

// OrderStatus represents the current status of an order
type OrderStatus int

const (
	OrderStatusPending       OrderStatus = iota // pending
	OrderStatusPartialFilled                    // partially filled
	OrderStatusFilled                           // fully filled
	OrderStatusCancelled                        // cancelled
	OrderStatusExpired                          // expired
)

// String returns the string representation of the order status
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
// Order type constants
// ============================================================================

// OrderType represents the order type
type OrderType int

const (
	OrderTypeLimit  OrderType = iota // limit order
	OrderTypeMarket                  // market order
)

// String returns the string representation of the order type
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
// Order side constants
// ============================================================================

// OrderSide represents the order side (buy or sell)
type OrderSide int

const (
	OrderSideBuy  OrderSide = iota // buy order
	OrderSideSell                  // sell order
)

// String returns the string representation of the order side
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

// IsOpposite returns whether two order sides are opposite
func (s OrderSide) IsOpposite(other OrderSide) bool {
	return s != other
}

// ============================================================================
// Error definitions
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
// Order struct
// ============================================================================

// Order represents an order
type Order struct {
	ID             uint64             // unique order ID
	Owner          interfaces.Address // order owner
	TradingPair    string             // trading pair (e.g. "AIB/USDT")
	Side           OrderSide          // order side (BUY/SELL)
	Quantity       uint64             // total order quantity
	FilledQuantity uint64             // filled quantity
	Price          uint64             // order price (0 means market order)
	OrderType      OrderType          // order type (limit/market)
	Status         OrderStatus        // order status
	Timestamp      time.Time          // creation time
	Expiration     *time.Time         // expiration time (optional)
}

// RemainingQuantity returns the remaining unfilled quantity of the order
func (o *Order) RemainingQuantity() uint64 {
	if o.Quantity >= o.FilledQuantity {
		return o.Quantity - o.FilledQuantity
	}
	return 0
}

// IsFilled checks whether the order is fully filled
func (o *Order) IsFilled() bool {
	return o.Status == OrderStatusFilled || o.FilledQuantity >= o.Quantity
}

// IsActive checks whether the order is active (fillable)
func (o *Order) IsActive() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusPartialFilled
}

// IsExpired checks whether the order has expired
func (o *Order) IsExpired() bool {
	if o.Expiration == nil {
		return false
	}
	return time.Now().After(*o.Expiration)
}

// Fill fills the order with the given quantity
func (o *Order) Fill(quantity uint64) uint64 {
	remaining := o.RemainingQuantity()
	if quantity >= remaining {
		// fully filled
		o.FilledQuantity = o.Quantity
		o.Status = OrderStatusFilled
		return remaining
	}
	// partially filled
	o.FilledQuantity += quantity
	o.Status = OrderStatusPartialFilled
	return quantity
}

// ============================================================================
// Trade struct
// ============================================================================

// Trade represents a single trade record
type Trade struct {
	ID           uint64             // unique trade ID
	TradingPair  string             // trading pair
	MakerOrderID uint64             // maker order ID
	TakerOrderID uint64             // taker order ID
	Maker        interfaces.Address // maker address
	Taker        interfaces.Address // taker address
	Side         OrderSide          // trade side (buy/sell)
	Quantity     uint64             // trade quantity
	Price        uint64             // trade price
	Timestamp    time.Time          // trade time
}

// ============================================================================
// OrderBook struct
// ============================================================================

// priceLevel represents the order level at the same price
type priceLevel struct {
	price    uint64     // price
	orders   *list.List // order list (in time order)
	totalQty uint64     // total quantity at this price level
}

// OrderBook represents the order book of a trading pair
type OrderBook struct {
	tradingPair string                   // trading pair
	bids        map[uint64]*priceLevel   // bids indexed by price (price -> priceLevel)
	asks        map[uint64]*priceLevel   // asks indexed by price (price -> priceLevel)
	bidPrices   []uint64                 // sorted bid prices (descending)
	askPrices   []uint64                 // sorted ask prices (ascending)
	orders      map[uint64]*list.Element // active order ID -> list element (active orders in the book only)
	allOrders   map[uint64]*Order        // all order IDs -> orders (including cancelled, filled, etc.)
	trades      []*Trade                 // trade records
	orderIDSeq  uint64                   // order ID sequence
	tradeIDSeq  uint64                   // trade ID sequence
	mu          sync.RWMutex             // read-write lock
}

// NewOrderBook creates a new order book
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

// generateOrderID generates a unique order ID
func (ob *OrderBook) generateOrderID() uint64 {
	ob.orderIDSeq++
	// generate a unique ID from timestamp and sequence number
	ts := uint64(time.Now().UnixNano())
	return ts<<16 | ob.orderIDSeq
}

// generateTradeID generates a unique trade ID
func (ob *OrderBook) generateTradeID() uint64 {
	ob.tradeIDSeq++
	ts := uint64(time.Now().UnixNano())
	return ts<<16 | ob.tradeIDSeq
}

// generateOrderHash generates the unique hash of the order
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
// Order operation methods
// ============================================================================

// PlaceOrder adds an order to the book and tries to match it
// returns the updated order and any resulting trades
func (ob *OrderBook) PlaceOrder(order *Order) (*Order, []*Trade, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// validate the order
	if err := ob.validateOrder(order); err != nil {
		return nil, nil, err
	}

	// if no order ID is specified, generate one
	if order.ID == 0 {
		order.ID = ob.generateOrderID()
	}

	// if no timestamp is specified, use the current time
	if order.Timestamp.IsZero() {
		order.Timestamp = time.Now()
	}

	// set the initial status
	if order.Status == 0 {
		order.Status = OrderStatusPending
	}

	// set the trading pair
	order.TradingPair = ob.tradingPair

	// tries to match an order
	newTrades := ob.matchOrder(order)

	// if the order has remaining quantity and is active, add it to the book
	if order.IsActive() && order.RemainingQuantity() > 0 {
		ob.addOrderToBook(order)
	}

	// save the order to allOrders
	ob.allOrders[order.ID] = order

	return order, newTrades, nil
}

// validateOrder validates the order
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
	// a limit order must have a valid price
	if order.OrderType == OrderTypeLimit && order.Price == 0 {
		return ErrInvalidPrice
	}
	// check the order owner
	if order.Owner == (interfaces.Address{}) {
		return errors.New("invalid order owner")
	}
	return nil
}

// addOrderToBook adds an order to the book
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

	// get or create the price level
	level, exists := priceMap[order.Price]
	if !exists {
		level = &priceLevel{
			price:  order.Price,
			orders: list.New(),
		}
		priceMap[order.Price] = level
		// add to the price list and sort
		*priceList = append(*priceList, order.Price)
		ob.sortPriceList(priceList, order.Side == OrderSideBuy)
	}

	// add the order to the price level
	elem := level.orders.PushBack(order)
	ob.orders[order.ID] = elem
	level.totalQty += order.RemainingQuantity()
}

// sortPriceList sorts the price list
// isDescending: descending for bids, ascending for asks
func (ob *OrderBook) sortPriceList(prices *[]uint64, isDescending bool) {
	if isDescending {
		// bids: descending (highest first)
		for i := 0; i < len(*prices)-1; i++ {
			for j := i + 1; j < len(*prices); j++ {
				if (*prices)[i] < (*prices)[j] {
					(*prices)[i], (*prices)[j] = (*prices)[j], (*prices)[i]
				}
			}
		}
	} else {
		// asks: ascending (lowest first)
		for i := 0; i < len(*prices)-1; i++ {
			for j := i + 1; j < len(*prices); j++ {
				if (*prices)[i] > (*prices)[j] {
					(*prices)[i], (*prices)[j] = (*prices)[j], (*prices)[i]
				}
			}
		}
	}
}

// CancelOrder cancels the specified order
func (ob *OrderBook) CancelOrder(orderID uint64, owner interfaces.Address) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// look up the order in allOrders first
	order, exists := ob.allOrders[orderID]
	if !exists {
		return ErrOrderNotFound
	}

	// verify the order owner
	if order.Owner != owner {
		return ErrUnauthorized
	}

	// check the order status
	if !order.IsActive() {
		return ErrOrderNotPending
	}

	// update the order status
	order.Status = OrderStatusCancelled

	// remove from the active book
	if elem, ok := ob.orders[orderID]; ok {
		ob.removeOrderFromBook(order, elem)
	}

	return nil
}

// removeOrderFromBook removes an order from the book
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

	// remove from the order list
	level.orders.Remove(elem)
	level.totalQty -= order.RemainingQuantity()

	// if the price level has no orders left, delete the level
	if level.orders.Len() == 0 {
		delete(priceMap, order.Price)
		// remove from the price list
		ob.removePriceFromList(order.Price, order.Side == OrderSideBuy)
	}

	// remove from the active order map (allOrders keeps it)
	delete(ob.orders, order.ID)
}

// removePriceFromList removes the given price from the price list
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

// GetOrder returns the order by ID (any status)
func (ob *OrderBook) GetOrder(orderID uint64) (*Order, error) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	order, exists := ob.allOrders[orderID]
	if !exists {
		return nil, ErrOrderNotFound
	}

	// return a copy of the order
	orderCopy := *order
	return &orderCopy, nil
}

// GetOrdersByOwner returns all orders of the given owner (any status)
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

// GetBids returns the bid list (descending by price)
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

// GetAsks returns the ask list (ascending by price)
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

// GetTrades returns the trade records
func (ob *OrderBook) GetTrades() []*Trade {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	result := make([]*Trade, len(ob.trades))
	copy(result, ob.trades)
	return result
}

// GetDepth returns the order book depth
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

	// get bid depth (descending by price)
	for i := 0; i < len(ob.bidPrices) && i < levels; i++ {
		price := ob.bidPrices[i]
		if level, exists := ob.bids[price]; exists {
			bids = append(bids, struct {
				Price    uint64
				Quantity uint64
			}{Price: price, Quantity: level.totalQty})
		}
	}

	// get ask depth (ascending by price)
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
// Order matching engine
// ============================================================================

// matchOrder tries to match an order
// returns the list of trades
func (ob *OrderBook) matchOrder(order *Order) []*Trade {
	var newTrades []*Trade

	// determine the opposite-side book
	var oppositeBook map[uint64]*priceLevel
	var oppositePrices *[]uint64

	if order.Side == OrderSideBuy {
		// a buy order looks for asks (asks - ascending, lowest first)
		oppositeBook = ob.asks
		oppositePrices = &ob.askPrices
	} else {
		// a sell order looks for bids (bids - descending, highest first)
		oppositeBook = ob.bids
		oppositePrices = &ob.bidPrices
	}

	// iterate the opposite-side price list
	for order.IsActive() && order.RemainingQuantity() > 0 {
		// find the best price
		bestPrice := ob.findBestPrice(order, oppositeBook, oppositePrices)
		if bestPrice == 0 {
			// no matching opposite order
			break
		}

		// check whether the price allows a fill
		if !ob.canMatch(order, bestPrice) {
			break
		}

		// get orders at this price level
		level, exists := oppositeBook[bestPrice]
		if !exists || level.orders.Len() == 0 {
			break
		}

		// match starting from the oldest order (time priority)
		elem := level.orders.Front()
		makerOrder := elem.Value.(*Order)

		// execute the fill
		trade := ob.executeTrade(order, makerOrder, bestPrice)
		newTrades = append(newTrades, trade)

		// update the order book
		if makerOrder.IsFilled() || makerOrder.Status == OrderStatusCancelled {
			ob.removeOrderFromBook(makerOrder, elem)
		}
	}

	return newTrades
}

// findBestPrice finds the best matchable price
func (ob *OrderBook) findBestPrice(order *Order, oppositeBook map[uint64]*priceLevel, oppositePrices *[]uint64) uint64 {
	if len(*oppositePrices) == 0 {
		return 0
	}

	if order.OrderType == OrderTypeMarket {
		// market order: fill at the opposite best price
		// buy order takes the lowest ask, sell order takes the highest bid
		if order.Side == OrderSideBuy {
			// buy order: find the lowest ask
			return (*oppositePrices)[0]
		} else {
			// sell order: find the highest bid
			prices := *oppositePrices
			return prices[len(prices)-1]
		}
	}

	// limit order: match by price rules
	if order.Side == OrderSideBuy {
		// buy order: find the lowest ask with price <= order price
		for _, price := range *oppositePrices {
			if price <= order.Price {
				return price
			}
		}
	} else {
		// sell order: find the highest bid with price >= order price
		prices := *oppositePrices
		for i := len(prices) - 1; i >= 0; i-- {
			if prices[i] >= order.Price {
				return prices[i]
			}
		}
	}

	return 0
}

// canMatch checks whether the price allows a fill
func (ob *OrderBook) canMatch(order *Order, price uint64) bool {
	if order.OrderType == OrderTypeMarket {
		return true
	}

	if order.Side == OrderSideBuy {
		// buy order: fill price must not exceed the order price
		return price <= order.Price
	} else {
		// sell order: fill price must not be below the order price
		return price >= order.Price
	}
}

// executeTrade executes one fill
func (ob *OrderBook) executeTrade(takerOrder, makerOrder *Order, price uint64) *Trade {
	// compute the fill quantity
	quantity := takerOrder.RemainingQuantity()
	makerRemaining := makerOrder.RemainingQuantity()

	if quantity > makerRemaining {
		quantity = makerRemaining
	}

	// fill
	takerFilled := takerOrder.Fill(quantity)
	makerOrder.Fill(quantity)

	// determine who is maker and who is taker
	var maker, taker interfaces.Address
	var makerOrderID, takerOrderID uint64

	// by time priority, the earlier order is the maker
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
		Side:         takerOrder.Side, // taker's side
		Quantity:     takerFilled,
		Price:        price,
		Timestamp:    time.Now(),
	}

	ob.trades = append(ob.trades, trade)

	return trade
}

// ============================================================================
// OrderBookManager - Order book manager
// ============================================================================

// OrderBookManager manages order books for multiple trading pairs
type OrderBookManager struct {
	orderBooks map[string]*OrderBook // tradingPair -> OrderBook
	mu         sync.RWMutex
}

// NewOrderBookManager creates a new order book manager
func NewOrderBookManager() *OrderBookManager {
	return &OrderBookManager{
		orderBooks: make(map[string]*OrderBook),
	}
}

// GetOrCreateOrderBook gets or creates the order book for the given trading pair
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

// GetOrderBook gets the order book for the given trading pair
func (m *OrderBookManager) GetOrderBook(tradingPair string) (*OrderBook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ob, exists := m.orderBooks[tradingPair]; exists {
		return ob, nil
	}
	return nil, ErrOrderBookNotFound
}

// ListTradingPairs lists all supported trading pairs
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
// Helper methods
// ============================================================================

// String returns the string representation of the order
func (o *Order) String() string {
	return fmt.Sprintf("Order{id=%d, owner=%s, pair=%s, side=%s, qty=%d, filled=%d, price=%d, type=%s, status=%s}",
		o.ID, o.Owner[:8], o.TradingPair, o.Side, o.Quantity, o.FilledQuantity, o.Price, o.OrderType, o.Status)
}

// String returns the string representation of the trade
func (t *Trade) String() string {
	return fmt.Sprintf("Trade{id=%d, pair=%s, maker=%s, taker=%s, side=%s, qty=%d, price=%d}",
		t.ID, t.TradingPair, t.Maker[:8], t.Taker[:8], t.Side, t.Quantity, t.Price)
}

// String returns the string representation of the order book
func (ob *OrderBook) String() string {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return fmt.Sprintf("OrderBook{pair=%s, bids=%d, asks=%d, orders=%d, trades=%d}",
		ob.tradingPair, len(ob.bidPrices), len(ob.askPrices), len(ob.orders), len(ob.trades))
}
