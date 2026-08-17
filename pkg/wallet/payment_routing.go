// Package wallet provides payment routing and L1/L2 transaction management.
// This file implements enhanced smart routing functionality.
package wallet

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// RoutingStrategy defines the strategy for selecting payment route.
type RoutingStrategy int

const (
	// StrategyCheapest selects the route with lowest total cost (amount + fees).
	StrategyCheapest RoutingStrategy = iota
	// StrategyFastest selects the fastest route.
	StrategyFastest
	// StrategyBalanced selects a balance between cost and speed.
	StrategyBalanced
	// StrategySafe prefers established channels with good history.
	StrategySafe
)

// String returns the string representation of routing strategy.
func (rs RoutingStrategy) String() string {
	switch rs {
	case StrategyCheapest:
		return "cheapest"
	case StrategyFastest:
		return "fastest"
	case StrategyBalanced:
		return "balanced"
	case StrategySafe:
		return "safe"
	default:
		return "unknown"
	}
}

// Route represents a payment route with associated metadata.
type Route struct {
	Method      PaymentMethod
	ChannelID   string // Empty for L1
	TotalCost   uint64 // Amount + fees
	Fee         uint64
	ConfirmTime uint64  // Milliseconds
	SuccessProb float64 // Estimated probability of success (0-1)
}

// RouteOption contains options for route calculation.
type RouteOption struct {
	PreferMethod PaymentMethod // Force a specific method
	MaxFee       uint64        // Maximum fee allowed
	MaxTime      uint64        // Maximum confirmation time (ms)
	Strategy     RoutingStrategy
}

// DefaultRouteOption returns default routing options.
func DefaultRouteOption() *RouteOption {
	return &RouteOption{
		Strategy: StrategyBalanced,
		MaxFee:   math.MaxUint64,
		MaxTime:  math.MaxUint64,
	}
}

// ChannelHealth represents the health status of an L2 channel.
type ChannelHealth struct {
	ChannelID      string
	SuccessRate    float64       // Historical success rate (0-1)
	AvgConfirmTime uint64        // Average confirmation time (ms)
	TotalVolume    uint64        // Total volume through channel
	Age            time.Duration // Age of the channel
	Score          float64       // Composite health score (0-1)
}

// RoutingConfig contains configuration for the routing engine.
type RoutingConfig struct {
	L2Threshold         uint64        // Amount below which L2 is preferred
	FeePerByte          uint64        // L1 fee rate
	L2FeeBPS            uint64        // L2 fee in basis points
	MaxChannelLoad      float64       // Maximum channel utilization (0-1)
	MinChannelAge       time.Duration // Minimum channel age for "safe" strategy
	HealthCheckInterval time.Duration
}

// DefaultRoutingConfig returns default routing configuration.
func DefaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		L2Threshold:         DefaultL2Threshold,
		FeePerByte:          1,
		L2FeeBPS:            10, // 0.1%
		MaxChannelLoad:      0.9,
		MinChannelAge:       24 * time.Hour,
		HealthCheckInterval: 5 * time.Minute,
	}
}

// SmartRouter provides enhanced routing capabilities.
type SmartRouter struct {
	pm            *PaymentManager
	config        *RoutingConfig
	strategy      RoutingStrategy
	channelHealth map[string]*ChannelHealth
	lastCheck     time.Time
	mu            sync.RWMutex
}

// NewSmartRouter creates a new smart router with the given configuration.
func NewSmartRouter(pm *PaymentManager, config *RoutingConfig) *SmartRouter {
	if config == nil {
		config = DefaultRoutingConfig()
	}

	return &SmartRouter{
		pm:            pm,
		config:        config,
		strategy:      StrategyBalanced,
		channelHealth: make(map[string]*ChannelHealth),
		lastCheck:     time.Now(),
	}
}

// SetStrategy sets the routing strategy.
func (sr *SmartRouter) SetStrategy(strategy RoutingStrategy) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.strategy = strategy
}

// CalculateRoutes calculates all available routes for a payment.
func (sr *SmartRouter) CalculateRoutes(to [32]byte, amount uint64) ([]*Route, error) {
	sr.mu.RLock()
	config := sr.config
	pm := sr.pm
	sr.mu.RUnlock()

	routes := make([]*Route, 0)

	// Always add L1 route
	l1Fee := amount*config.FeePerByte + 200 // Base fee estimate
	routes = append(routes, &Route{
		Method:      PaymentL1,
		ChannelID:   "",
		TotalCost:   amount + l1Fee,
		Fee:         l1Fee,
		ConfirmTime: 600000, // ~10 minutes
		SuccessProb: 0.99,   // L1 is very reliable
	})

	// Add L2 routes for each active channel
	pm.mu.RLock()
	for _, ch := range pm.l2Channels {
		if !ch.IsActive || ch.Balance < amount {
			continue
		}

		l2Fee := (amount * ch.FeeRate) / 10000

		// Get channel health
		sr.mu.RLock()
		health, exists := sr.channelHealth[ch.ChannelID]
		if !exists {
			health = &ChannelHealth{
				ChannelID:   ch.ChannelID,
				SuccessRate: 0.95,
				Score:       0.95,
			}
		}
		sr.mu.RUnlock()

		routes = append(routes, &Route{
			Method:      PaymentL2,
			ChannelID:   ch.ChannelID,
			TotalCost:   amount + l2Fee,
			Fee:         l2Fee,
			ConfirmTime: 100, // L2 is fast
			SuccessProb: health.SuccessRate,
		})
	}
	pm.mu.RUnlock()

	if len(routes) == 0 {
		return nil, fmt.Errorf("no available routes for payment of %d", amount)
	}

	return routes, nil
}

// SelectRoute selects the best route based on the configured strategy.
func (sr *SmartRouter) SelectRoute(to [32]byte, amount uint64, opts *RouteOption) (*Route, error) {
	routes, err := sr.CalculateRoutes(to, amount)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = DefaultRouteOption()
	}

	// Filter by constraints
	filtered := make([]*Route, 0)
	for _, r := range routes {
		if r.Fee > opts.MaxFee {
			continue
		}
		if r.ConfirmTime > opts.MaxTime {
			continue
		}
		if opts.PreferMethod != 0 && r.Method != opts.PreferMethod {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) == 0 {
		// If no route matches constraints, return the cheapest
		return sr.selectByStrategy(routes, StrategyCheapest), nil
	}

	return sr.selectByStrategy(filtered, opts.Strategy), nil
}

// selectByStrategy selects a route based on the given strategy.
func (sr *SmartRouter) selectByStrategy(routes []*Route, strategy RoutingStrategy) *Route {
	if len(routes) == 0 {
		return nil
	}

	switch strategy {
	case StrategyCheapest:
		return sr.selectCheapest(routes)
	case StrategyFastest:
		return sr.selectFastest(routes)
	case StrategyBalanced:
		return sr.selectBalanced(routes)
	case StrategySafe:
		return sr.selectSafe(routes)
	default:
		return sr.selectBalanced(routes)
	}
}

// selectCheapest selects the route with lowest total cost.
func (sr *SmartRouter) selectCheapest(routes []*Route) *Route {
	best := routes[0]
	for _, r := range routes[1:] {
		if r.TotalCost < best.TotalCost {
			best = r
		}
	}
	return best
}

// selectFastest selects the route with lowest confirmation time.
func (sr *SmartRouter) selectFastest(routes []*Route) *Route {
	best := routes[0]
	for _, r := range routes[1:] {
		if r.ConfirmTime < best.ConfirmTime {
			best = r
		}
	}
	return best
}

// selectBalanced selects a route balancing cost and speed.
func (sr *SmartRouter) selectBalanced(routes []*Route) *Route {
	// Normalize costs and times
	var maxCost, maxTime uint64
	for _, r := range routes {
		if r.TotalCost > maxCost {
			maxCost = r.TotalCost
		}
		if r.ConfirmTime > maxTime {
			maxTime = r.ConfirmTime
		}
	}

	best := routes[0]
	bestScore := float64(1)

	for _, r := range routes[1:] {
		costScore := 1.0 - float64(r.TotalCost)/float64(maxCost)
		timeScore := 1.0 - float64(r.ConfirmTime)/float64(maxTime)
		probScore := r.SuccessProb

		// Weighted score: 40% cost, 30% speed, 30% reliability
		score := 0.4*costScore + 0.3*timeScore + 0.3*probScore

		if score > bestScore {
			bestScore = score
			best = r
		}
	}

	return best
}

// selectSafe selects the route with highest success probability.
func (sr *SmartRouter) selectSafe(routes []*Route) *Route {
	best := routes[0]
	for _, r := range routes[1:] {
		if r.SuccessProb > best.SuccessProb {
			best = r
		}
	}
	return best
}

// ExecuteWithRoute executes a payment using a specific route.
func (sr *SmartRouter) ExecuteWithRoute(to [32]byte, amount uint64, route *Route) *PaymentResult {
	if route == nil {
		return sr.pm.SmartSend(to, amount)
	}

	switch route.Method {
	case PaymentL1:
		return sr.pm.SendL1(to, amount)
	case PaymentL2:
		if route.ChannelID == "" {
			return &PaymentResult{
				Method:  PaymentL2,
				Success: false,
				Error:   "channel ID required for L2 payment",
			}
		}
		return sr.pm.SendL2(to, amount, route.ChannelID)
	default:
		return sr.pm.SmartSend(to, amount)
	}
}

// SmartRoutePayment executes a payment using intelligent routing.
func (sr *SmartRouter) SmartRoutePayment(to [32]byte, amount uint64, opts *RouteOption) *PaymentResult {
	route, err := sr.SelectRoute(to, amount, opts)
	if err != nil {
		return &PaymentResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	return sr.ExecuteWithRoute(to, amount, route)
}

// UpdateChannelHealth updates the health metrics for a channel.
func (sr *SmartRouter) UpdateChannelHealth(channelID string, success bool, confirmTime uint64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	health, exists := sr.channelHealth[channelID]
	if !exists {
		health = &ChannelHealth{
			ChannelID: channelID,
			Age:       time.Since(sr.lastCheck),
		}
		sr.channelHealth[channelID] = health
	}

	// Update rolling success rate (exponential moving average)
	alpha := 0.1
	if success {
		health.SuccessRate = alpha*1.0 + (1-alpha)*health.SuccessRate
	} else {
		health.SuccessRate = alpha*0.0 + (1-alpha)*health.SuccessRate
	}

	// Update average confirmation time
	health.AvgConfirmTime = uint64(float64(confirmTime)*alpha + float64(health.AvgConfirmTime)*(1-alpha))

	// Calculate composite score
	health.Score = 0.5*health.SuccessRate + 0.3*(1.0-float64(health.AvgConfirmTime)/600000.0) + 0.2*math.Min(float64(health.Age.Hours())/168.0, 1.0)
}

// GetHealthyChannels returns channels sorted by health score.
func (sr *SmartRouter) GetHealthyChannels(minScore float64) []*ChannelHealth {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	healthy := make([]*ChannelHealth, 0)
	for _, h := range sr.channelHealth {
		if h.Score >= minScore {
			healthy = append(healthy, h)
		}
	}

	// Sort by score descending
	for i := 0; i < len(healthy)-1; i++ {
		for j := i + 1; j < len(healthy); j++ {
			if healthy[j].Score > healthy[i].Score {
				healthy[i], healthy[j] = healthy[j], healthy[i]
			}
		}
	}

	return healthy
}

// EstimateTotalCost estimates the total cost (amount + fees) for a payment.
func (sr *SmartRouter) EstimateTotalCost(amount uint64, method PaymentMethod) uint64 {
	sr.mu.RLock()
	config := sr.config
	sr.mu.RUnlock()

	switch method {
	case PaymentL1:
		return amount + amount*config.FeePerByte + 200
	case PaymentL2:
		return amount + (amount*config.L2FeeBPS)/10000
	default:
		// Return cheapest estimate
		l1Cost := amount + amount*config.FeePerByte + 200
		l2Cost := amount + (amount*config.L2FeeBPS)/10000
		if l1Cost < l2Cost {
			return l1Cost
		}
		return l2Cost
	}
}

// GetConfig returns the current routing configuration.
func (sr *SmartRouter) GetConfig() *RoutingConfig {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.config
}

// UpdateConfig updates the routing configuration.
func (sr *SmartRouter) UpdateConfig(config *RoutingConfig) {
	if config == nil {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.config = config
}
