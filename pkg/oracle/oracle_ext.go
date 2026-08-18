// Package oracle provides extended functionality for the price oracle
// including cache statistics, warmup, price deviation detection, and more
package oracle

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// ===========================================================================
// Cache statistics
// ===========================================================================

// GetCacheStats returns cache statistics
func (o *PriceOracle) GetCacheStats() CacheStats {
	hits := atomic.LoadInt64(&o.cacheHits)
	misses := atomic.LoadInt64(&o.cacheMisses)
	total := hits + misses

	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	return CacheStats{
		Hits:          hits,
		Misses:        misses,
		HitRate:       hitRate,
		TotalRequests: total,
	}
}

// ResetCacheStats resets cache statistics
func (o *PriceOracle) ResetCacheStats() {
	atomic.StoreInt64(&o.cacheHits, 0)
	atomic.StoreInt64(&o.cacheMisses, 0)
}

// ===========================================================================
// Cache warmup
// ===========================================================================

// Warmup warms up the cache for all supported trading pairs
func (o *PriceOracle) Warmup() {
	pairs := o.GetSupportedPairs()
	o.warmupPairs(pairs...)
}

// WarmupPairs warms up the cache for the specified trading pairs
func (o *PriceOracle) WarmupPairs(pairs ...TradingPair) {
	o.warmupPairs(pairs...)
}

func (o *PriceOracle) warmupPairs(pairs ...TradingPair) {
	if len(pairs) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, pair := range pairs {
		wg.Add(1)
		go func(p TradingPair) {
			defer wg.Done()
			_, _ = o.GetPrice(p)
		}(pair)
	}
	wg.Wait()
}

// ===========================================================================
// Price history queries
// ===========================================================================

// GetPriceHistory returns the price history for the specified trading pair
func (o *PriceOracle) GetPriceHistory(pair TradingPair) []priceHistoryEntry {
	o.priceHistoryMu.RLock()
	defer o.priceHistoryMu.RUnlock()

	history, exists := o.priceHistory[pair.String()]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]priceHistoryEntry, len(history))
	copy(result, history)
	return result
}

// GetAveragePrice returns the average historical price
func (o *PriceOracle) GetAveragePrice(pair TradingPair) (float64, error) {
	history := o.GetPriceHistory(pair)

	if len(history) == 0 {
		return 0, fmt.Errorf("%w: no price history for %s", ErrPriceUnavailable, pair)
	}

	var sum float64
	for _, entry := range history {
		sum += entry.Price
	}

	return sum / float64(len(history)), nil
}

// ===========================================================================
// Price deviation detection
// ===========================================================================

// CheckPriceDeviation manually checks the price deviation for the specified trading pair
func (o *PriceOracle) CheckPriceDeviation(pair TradingPair) (deviation float64, referencePrice float64, hasAlert bool, alertMessage string) {
	history := o.GetPriceHistory(pair)

	if len(history) < 5 {
		return 0, 0, false, "insufficient historical data"
	}

	currentPrice, err := o.GetCachedPrice(pair)
	if err != nil {
		currentPrice, err = o.GetPrice(pair)
		if err != nil {
			return 0, 0, false, "unable to fetch current price"
		}
	}

	var sum float64
	for _, entry := range history {
		sum += entry.Price
	}
	avgPrice := sum / float64(len(history))

	if avgPrice <= 0 {
		return 0, 0, false, "invalid reference price"
	}

	deviation = math.Abs(currentPrice.Price-avgPrice) / avgPrice * 100
	referencePrice = avgPrice
	threshold := 10.0 // default 10% threshold

	if threshold > 0 && deviation > threshold {
		hasAlert = true
		alertMessage = fmt.Sprintf("price deviation %.2f%% exceeds threshold %.2f%%", deviation, threshold)
	}

	return deviation, referencePrice, hasAlert, alertMessage
}

// SetDeviationAlertHandler sets the price deviation alert handler
func (o *PriceOracle) SetDeviationAlertHandler(handler func(pair TradingPair, oldPrice, newPrice, deviation float64)) {
	o.alertHandlerMu.Lock()
	defer o.alertHandlerMu.Unlock()
	o.alertHandler = &simpleAlertHandler{fn: handler}
}

// simpleAlertHandler is a simple alert handler implementation
type simpleAlertHandler struct {
	fn func(pair TradingPair, oldPrice, newPrice, deviation float64)
}

func (h *simpleAlertHandler) OnPriceDeviation(pair TradingPair, oldPrice, newPrice, deviation float64) {
	if h.fn != nil {
		h.fn(pair, oldPrice, newPrice, deviation)
	}
}
