// Package oracle 提供价格预言机的扩展功能
// 包含缓存统计、预热、价格偏差检测等
package oracle

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// ===========================================================================
// 缓存统计功能
// ===========================================================================

// GetCacheStats 返回缓存统计信息
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

// ResetCacheStats 重置缓存统计
func (o *PriceOracle) ResetCacheStats() {
	atomic.StoreInt64(&o.cacheHits, 0)
	atomic.StoreInt64(&o.cacheMisses, 0)
}

// ===========================================================================
// 缓存预热功能
// ===========================================================================

// Warmup 预热所有支持交易对的缓存
func (o *PriceOracle) Warmup() {
	pairs := o.GetSupportedPairs()
	o.warmupPairs(pairs...)
}

// WarmupPairs 预热指定交易对的缓存
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
// 价格历史查询
// ===========================================================================

// GetPriceHistory 获取指定交易对的价格历史
func (o *PriceOracle) GetPriceHistory(pair TradingPair) []priceHistoryEntry {
	o.priceHistoryMu.RLock()
	defer o.priceHistoryMu.RUnlock()

	history, exists := o.priceHistory[pair.String()]
	if !exists {
		return nil
	}

	// 返回副本
	result := make([]priceHistoryEntry, len(history))
	copy(result, history)
	return result
}

// GetAveragePrice 获取历史平均价格
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
// 价格偏差检测
// ===========================================================================

// CheckPriceDeviation 手动检查指定交易对的价格偏差
func (o *PriceOracle) CheckPriceDeviation(pair TradingPair) (deviation float64, referencePrice float64, hasAlert bool, alertMessage string) {
	history := o.GetPriceHistory(pair)

	if len(history) < 5 {
		return 0, 0, false, "历史数据不足"
	}

	currentPrice, err := o.GetCachedPrice(pair)
	if err != nil {
		currentPrice, err = o.GetPrice(pair)
		if err != nil {
			return 0, 0, false, "无法获取当前价格"
		}
	}

	var sum float64
	for _, entry := range history {
		sum += entry.Price
	}
	avgPrice := sum / float64(len(history))

	if avgPrice <= 0 {
		return 0, 0, false, "参考价格无效"
	}

	deviation = math.Abs(currentPrice.Price-avgPrice) / avgPrice * 100
	referencePrice = avgPrice
	threshold := 10.0 // 默认 10% 阈值

	if threshold > 0 && deviation > threshold {
		hasAlert = true
		alertMessage = fmt.Sprintf("价格偏差 %.2f%% 超过阈值 %.2f%%", deviation, threshold)
	}

	return deviation, referencePrice, hasAlert, alertMessage
}

// SetDeviationAlertHandler 设置价格偏差告警处理器
func (o *PriceOracle) SetDeviationAlertHandler(handler func(pair TradingPair, oldPrice, newPrice, deviation float64)) {
	o.alertHandlerMu.Lock()
	defer o.alertHandlerMu.Unlock()
	o.alertHandler = &simpleAlertHandler{fn: handler}
}

// simpleAlertHandler 简单的告警处理器实现
type simpleAlertHandler struct {
	fn func(pair TradingPair, oldPrice, newPrice, deviation float64)
}

func (h *simpleAlertHandler) OnPriceDeviation(pair TradingPair, oldPrice, newPrice, deviation float64) {
	if h.fn != nil {
		h.fn(pair, oldPrice, newPrice, deviation)
	}
}
