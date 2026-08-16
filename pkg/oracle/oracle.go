package oracle

import (
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// PriceOracle 是多源价格预言机的核心结构体。
// 它管理多个价格源，聚合价格数据，并提供缓存和滑点保护。
type PriceOracle struct {
	mu      sync.RWMutex
	config  OracleConfig
	sources []PriceSource

	// cache 按交易对缓存聚合后的价格
	cache map[string]cacheEntry

	// cacheHits 缓存命中次数（使用原子操作）
	cacheHits int64
	// cacheMisses 缓存未命中次数（使用原子操作）
	cacheMisses int64

	// priceHistory 价格历史记录（用于偏差检测）
	priceHistory map[string][]priceHistoryEntry
	priceHistoryMu sync.RWMutex

	// alertHandler 价格偏差告警处理器
	alertHandler   AlertHandler
	alertHandlerMu sync.RWMutex

	// running 标记预言机是否正在运行
	running bool

	// stopCh 用于通知后台刷新 goroutine 停止
	stopCh chan struct{}

	// logger 日志输出
	logger *log.Logger
}

// NewPriceOracle 创建一个新的价格预言机实例。
// sources 为价格源列表，config 为配置参数。
// 如果 config 未通过验证，返回错误。
func NewPriceOracle(sources []PriceSource, config OracleConfig) (*PriceOracle, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if len(sources) == 0 {
		return nil, ErrNoPriceSources
	}

	return &PriceOracle{
		config:       config,
		sources:      sources,
		cache:        make(map[string]cacheEntry),
		priceHistory: make(map[string][]priceHistoryEntry),
		stopCh:       make(chan struct{}),
		logger:       log.Default(),
	}, nil
}

// NewDefaultPriceOracle 使用默认价格源和默认配置创建预言机。
func NewDefaultPriceOracle() (*PriceOracle, error) {
	return NewPriceOracle(DefaultSources(), DefaultConfig())
}

// Start 启动预言机的后台自动刷新。
// 调用后，预言机会按配置的间隔自动更新所有支持的交易对价格。
func (o *PriceOracle) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.mu.Unlock()

	// 启动时立即刷新一次
	o.refreshAll()

	// 后台定期刷新
	go o.refreshLoop()
}

// Stop 停止预言机的后台自动刷新
func (o *PriceOracle) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.running {
		return
	}

	o.running = false
	close(o.stopCh)
}

// IsRunning 返回预言机是否正在运行
func (o *PriceOracle) IsRunning() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.running
}

// refreshLoop 后台刷新循环
func (o *PriceOracle) refreshLoop() {
	ticker := time.NewTicker(o.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.refreshAll()
		}
	}
}

// refreshAll 刷新所有支持的交易对价格
func (o *PriceOracle) refreshAll() {
	pairs := SupportedPairs()
	var wg sync.WaitGroup

	for _, pair := range pairs {
		wg.Add(1)
		go func(p TradingPair) {
			defer wg.Done()
			_, err := o.fetchAndAggregate(p)
			if err != nil {
				o.logger.Printf("[oracle] refresh %s failed: %v", p, err)
			}
		}(pair)
	}

	wg.Wait()
}

// GetPrice 获取指定交易对的聚合价格。
// 优先使用缓存，缓存过期时重新获取。
func (o *PriceOracle) GetPrice(pair TradingPair) (AggregatedPrice, error) {
	// 先检查缓存
	o.mu.RLock()
	entry, exists := o.cache[pair.String()]
	o.mu.RUnlock()

	if exists && !entry.isExpired() {
		// 缓存命中 - 更新统计
		atomic.AddInt64(&o.cacheHits, 1)
		return entry.price, nil
	}

	// 缓存未命中 - 更新统计
	atomic.AddInt64(&o.cacheMisses, 1)

	// 获取旧价格用于偏差检测（在获取新价格之前）
	oldPrice := o.getLastPrice(pair)

	// 缓存不存在或已过期，重新获取
	aggPrice, err := o.fetchAndAggregate(pair)
	if err != nil {
		return aggPrice, err
	}

	// 检查价格偏差并触发告警
	o.checkPriceDeviationWithOldPrice(aggPrice, oldPrice)

	return aggPrice, nil
}

// GetPriceFromSource 从指定的价格源获取价格（不经过聚合）
func (o *PriceOracle) GetPriceFromSource(pair TradingPair, sourceName string) (PriceData, error) {
	for _, source := range o.sources {
		if source.GetName() == sourceName && source.IsAvailable() {
			return source.FetchPrice(pair)
		}
	}
	return PriceData{}, fmt.Errorf("%w: source %q not found or unavailable", ErrPriceUnavailable, sourceName)
}

// GetAllPrices 从所有可用的价格源获取指定交易对的价格（不聚合）
func (o *PriceOracle) GetAllPrices(pair TradingPair) []PriceData {
	return o.collectPrices(pair)
}

// fetchAndAggregate 从所有可用源收集价格并聚合
func (o *PriceOracle) fetchAndAggregate(pair TradingPair) (AggregatedPrice, error) {
	prices := o.collectPrices(pair)

	if len(prices) < o.config.MinSources {
		return AggregatedPrice{}, fmt.Errorf(
			"%w: got %d sources, need at least %d for %s",
			ErrPriceUnavailable, len(prices), o.config.MinSources, pair,
		)
	}

	// 聚合计算
	aggregated := o.aggregate(pair, prices)

	// 更新缓存
	o.mu.Lock()
	o.cache[pair.String()] = cacheEntry{
		price:     aggregated,
		expiresAt: time.Now().Add(o.config.CacheTTL),
	}
	o.mu.Unlock()

	// 保存价格历史
	o.addPriceHistory(pair, aggregated.Price)

	return aggregated, nil
}

// collectPrices 从所有可用的价格源并行收集价格数据
func (o *PriceOracle) collectPrices(pair TradingPair) []PriceData {
	type result struct {
		data PriceData
		err  error
	}

	results := make(chan result, len(o.sources))
	var wg sync.WaitGroup

	for _, source := range o.sources {
		if !source.IsAvailable() {
			continue
		}

		// 检查该源是否支持此交易对
		supported := false
		for _, sp := range source.SupportedPairs() {
			if sp.Equal(pair) {
				supported = true
				break
			}
		}
		if !supported {
			continue
		}

		wg.Add(1)
		go func(src PriceSource) {
			defer wg.Done()
			data, err := src.FetchPrice(pair)
			results <- result{data: data, err: err}
		}(source)
	}

	// 等待所有 goroutine 完成后关闭通道
	go func() {
		wg.Wait()
		close(results)
	}()

	var prices []PriceData
	for r := range results {
		if r.err != nil {
			o.logger.Printf("[oracle] source error for %s: %v", pair, r.err)
			continue
		}
		if r.data.IsValid() {
			prices = append(prices, r.data)
		}
	}

	return prices
}

// aggregate 对收集到的价格数据进行聚合。
// 步骤:
//  1. 离群值过滤（剔除偏离中位数超过阈值的数据）
//  2. 加权平均计算（Volume-Weighted Average Price）
//  3. 统计计算（置信度、标准差等）
func (o *PriceOracle) aggregate(pair TradingPair, prices []PriceData) AggregatedPrice {
	if len(prices) == 0 {
		return AggregatedPrice{Pair: pair, Timestamp: time.Now()}
	}

	// 只有一个价格源时直接返回
	if len(prices) == 1 {
		return AggregatedPrice{
			Pair:        pair,
			Price:       prices[0].Price,
			Sources:     1,
			TotalVolume: prices[0].Volume24h,
			Confidence:  0.5, // 单源置信度较低
			Timestamp:   time.Now(),
			MinPrice:    prices[0].Price,
			MaxPrice:    prices[0].Price,
			Deviation:   0,
			RawPrices:   prices,
		}
	}

	// 步骤 1: 计算中位数
	median := o.calculateMedian(prices)

	// 步骤 2: 离群值过滤
	filtered := o.filterOutliers(prices, median)

	// 如果过滤后没有剩余数据，使用原始数据
	if len(filtered) == 0 {
		filtered = prices
	}

	// 步骤 3: 加权平均价格计算 (VWAP)
	vwap := o.calculateVWAP(filtered)

	// 步骤 4: 统计信息
	minPrice, maxPrice := o.minMax(filtered)
	deviation := o.calculateStdDev(filtered, vwap)
	totalVolume := o.totalVolume(filtered)
	confidence := o.calculateConfidence(filtered, deviation)

	return AggregatedPrice{
		Pair:        pair,
		Price:       vwap,
		Sources:     len(filtered),
		TotalVolume: totalVolume,
		Confidence:  confidence,
		Timestamp:   time.Now(),
		MinPrice:    minPrice,
		MaxPrice:    maxPrice,
		Deviation:   deviation,
		RawPrices:   filtered,
	}
}

// calculateMedian 计算价格的中位数
func (o *PriceOracle) calculateMedian(prices []PriceData) float64 {
	vals := make([]float64, len(prices))
	for i, p := range prices {
		vals[i] = p.Price
	}
	sort.Float64s(vals)

	n := len(vals)
	if n%2 == 0 {
		return (vals[n/2-1] + vals[n/2]) / 2
	}
	return vals[n/2]
}

// filterOutliers 过滤掉偏离中位数超过阈值的价格数据。
// 阈值由 config.DeviationThreshold 指定（百分比）。
func (o *PriceOracle) filterOutliers(prices []PriceData, median float64) []PriceData {
	if median <= 0 {
		return prices
	}

	threshold := o.config.DeviationThreshold / 100.0 // 转为小数
	var filtered []PriceData

	for _, p := range prices {
		deviation := math.Abs(p.Price-median) / median
		if deviation <= threshold {
			filtered = append(filtered, p)
		} else {
			o.logger.Printf("[oracle] outlier filtered: %s from %s (price=%.8f, median=%.8f, dev=%.2f%%)",
				p.Pair, p.Source, p.Price, median, deviation*100)
		}
	}

	return filtered
}

// calculateVWAP 计算成交量加权平均价格 (Volume-Weighted Average Price)。
// 如果所有数据源的成交量都为零，则回退到简单平均。
func (o *PriceOracle) calculateVWAP(prices []PriceData) float64 {
	var totalWeightedPrice float64
	var totalVolume float64

	for _, p := range prices {
		volume := p.Volume24h
		if volume <= 0 {
			volume = 1 // 无成交量数据时赋予最小权重
		}
		totalWeightedPrice += p.Price * volume
		totalVolume += volume
	}

	if totalVolume <= 0 {
		// 回退到简单平均
		var sum float64
		for _, p := range prices {
			sum += p.Price
		}
		return sum / float64(len(prices))
	}

	return totalWeightedPrice / totalVolume
}

// calculateStdDev 计算价格的标准差
func (o *PriceOracle) calculateStdDev(prices []PriceData, mean float64) float64 {
	if len(prices) <= 1 {
		return 0
	}

	var sumSquares float64
	for _, p := range prices {
		diff := p.Price - mean
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(prices))
	return math.Sqrt(variance)
}

// minMax 返回价格列表中的最小和最大价格
func (o *PriceOracle) minMax(prices []PriceData) (float64, float64) {
	if len(prices) == 0 {
		return 0, 0
	}

	minP := prices[0].Price
	maxP := prices[0].Price

	for _, p := range prices[1:] {
		if p.Price < minP {
			minP = p.Price
		}
		if p.Price > maxP {
			maxP = p.Price
		}
	}

	return minP, maxP
}

// totalVolume 计算总成交量
func (o *PriceOracle) totalVolume(prices []PriceData) float64 {
	var total float64
	for _, p := range prices {
		total += p.Volume24h
	}
	return total
}

// calculateConfidence 计算置信度（0-1之间）。
// 基于数据源数量和价格一致性。
func (o *PriceOracle) calculateConfidence(prices []PriceData, stdDev float64) float64 {
	if len(prices) == 0 {
		return 0
	}

	// 数据源数量因子：更多源 -> 更高置信度
	// 使用 sigmoid-like 曲线：1 源 = 0.5, 3 源 = 0.75, 5+ 源 = ~0.9
	sourceFactor := 1.0 - 1.0/(1.0+float64(len(prices))*0.5)

	// 一致性因子：标准差越小 -> 越一致 -> 越高置信度
	// 计算变异系数 (CV = stdDev / mean)
	mean := o.calculateVWAP(prices)
	consistencyFactor := 1.0
	if mean > 0 && stdDev > 0 {
		cv := stdDev / mean
		// CV < 0.01 (1%) -> 高一致性
		// CV > 0.05 (5%) -> 低一致性
		consistencyFactor = math.Max(0.1, 1.0-cv*10)
	}

	// 源类型多样性加分：不同类型的源提供更可靠的价格
	typeSet := make(map[SourceType]bool)
	for _, p := range prices {
		typeSet[p.SourceType] = true
	}
	diversityBonus := float64(len(typeSet)) * 0.05

	confidence := sourceFactor*0.6 + consistencyFactor*0.4 + diversityBonus
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0 {
		confidence = 0
	}

	return confidence
}

// ===========================================================================
// 滑点保护
// ===========================================================================

// CalculateSlippage 计算给定交易对和输入金额的滑点。
// inputAmount: 输入的基础资产数量
// pair: 交易对
// 返回滑点分析结果。
func (o *PriceOracle) CalculateSlippage(pair TradingPair, inputAmount float64) (SlippageResult, error) {
	// 获取当前聚合价格
	aggPrice, err := o.GetPrice(pair)
	if err != nil {
		return SlippageResult{}, fmt.Errorf("cannot calculate slippage: %w", err)
	}

	if aggPrice.Price <= 0 {
		return SlippageResult{}, fmt.Errorf("%w: aggregated price is zero for %s", ErrPriceUnavailable, pair)
	}

	// 无滑点时的预期输出
	expectedOutput := inputAmount * aggPrice.Price

	// 价格影响估算
	// 使用简化的恒定乘积 AMM 模型 (x * y = k)
	// 价格影响 ≈ inputAmount / totalLiquidity
	priceImpact := o.estimatePriceImpact(aggPrice, inputAmount)

	// 考虑滑点后的实际输出
	actualOutput := expectedOutput * (1 - priceImpact/100)

	// 滑点百分比
	slippagePercent := 0.0
	if expectedOutput > 0 {
		slippagePercent = (expectedOutput - actualOutput) / expectedOutput * 100
	}

	// 满足最大滑点限制的最小输出
	minOutput := expectedOutput * (1 - o.config.MaxSlippage/100)

	return SlippageResult{
		InputAmount:     inputAmount,
		ExpectedOutput:  expectedOutput,
		ActualOutput:    actualOutput,
		SlippagePercent: slippagePercent,
		PriceImpact:     priceImpact,
		MinOutput:       minOutput,
		Pair:            pair,
		Acceptable:      slippagePercent <= o.config.MaxSlippage,
	}, nil
}

// ValidateSlippage 验证交易的滑点是否在允许范围内。
// 如果滑点超出 MaxSlippage，返回 ErrSlippageTooHigh。
func (o *PriceOracle) ValidateSlippage(pair TradingPair, inputAmount float64, minOutputAmount float64) error {
	result, err := o.CalculateSlippage(pair, inputAmount)
	if err != nil {
		return err
	}

	if !result.Acceptable {
		return fmt.Errorf("%w: slippage %.2f%% exceeds max %.2f%% for %s",
			ErrSlippageTooHigh, result.SlippagePercent, o.config.MaxSlippage, pair)
	}

	if result.ActualOutput < minOutputAmount {
		return fmt.Errorf("%w: actual output %.8f < min required %.8f for %s",
			ErrSlippageTooHigh, result.ActualOutput, minOutputAmount, pair)
	}

	return nil
}

// estimatePriceImpact 估算价格影响百分比。
// 使用恒定乘积 AMM 模型进行估算。
func (o *PriceOracle) estimatePriceImpact(aggPrice AggregatedPrice, inputAmount float64) float64 {
	// 从原始价格数据中获取总流动性
	var totalLiquidity float64
	for _, p := range aggPrice.RawPrices {
		if p.Liquidity > 0 {
			totalLiquidity += p.Liquidity
		}
	}

	// 如果没有流动性数据，使用成交量作为近似值
	if totalLiquidity <= 0 {
		totalLiquidity = aggPrice.TotalVolume
	}

	// 如果仍然没有流动性数据，返回一个保守估计
	if totalLiquidity <= 0 {
		return 0.1 // 默认 0.1% 的价格影响
	}

	// 输入金额的 USD 价值
	inputValueUSD := inputAmount * aggPrice.Price

	// 价格影响估算: impact% ≈ (inputValue / totalLiquidity) * 100
	// 使用恒定乘积公式的简化版本
	impact := (inputValueUSD / totalLiquidity) * 100

	// 非线性调整：大额交易的影响增长更快
	if impact > 1.0 {
		impact = impact * (1.0 + impact*0.1)
	}

	return impact
}

// ===========================================================================
// 价格缓存管理
// ===========================================================================

// InvalidateCache 使指定交易对的缓存失效
func (o *PriceOracle) InvalidateCache(pair TradingPair) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.cache, pair.String())
}

// InvalidateAllCache 使所有缓存失效
func (o *PriceOracle) InvalidateAllCache() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache = make(map[string]cacheEntry)
}

// GetCachedPrice 仅从缓存获取价格（不触发刷新）。
// 如果缓存不存在或已过期，返回 ErrPriceStale。
func (o *PriceOracle) GetCachedPrice(pair TradingPair) (AggregatedPrice, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	entry, exists := o.cache[pair.String()]
	if !exists {
		return AggregatedPrice{}, fmt.Errorf("%w: no cached price for %s", ErrPriceUnavailable, pair)
	}

	if entry.isExpired() {
		return entry.price, fmt.Errorf("%w: cached price for %s expired %v ago",
			ErrPriceStale, pair, time.Since(entry.expiresAt))
	}

	return entry.price, nil
}

// ===========================================================================
// 价格源管理
// ===========================================================================

// AddSource 添加一个新的价格源
func (o *PriceOracle) AddSource(source PriceSource) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sources = append(o.sources, source)
}

// RemoveSource 根据名称移除一个价格源
func (o *PriceOracle) RemoveSource(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i, src := range o.sources {
		if src.GetName() == name {
			o.sources = append(o.sources[:i], o.sources[i+1:]...)
			return
		}
	}
}

// GetSources 返回所有已注册的价格源
func (o *PriceOracle) GetSources() []PriceSource {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]PriceSource, len(o.sources))
	copy(result, o.sources)
	return result
}

// GetAvailableSources 返回当前可用的价格源
func (o *PriceOracle) GetAvailableSources() []PriceSource {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var available []PriceSource
	for _, src := range o.sources {
		if src.IsAvailable() {
			available = append(available, src)
		}
	}
	return available
}

// ===========================================================================
// 配置管理
// ===========================================================================

// GetConfig 返回当前配置的副本
func (o *PriceOracle) GetConfig() OracleConfig {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.config
}

// UpdateConfig 更新预言机配置。
// 新配置必须通过验证。
func (o *PriceOracle) UpdateConfig(config OracleConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	o.mu.Lock()
	o.config = config
	o.mu.Unlock()

	return nil
}

// SetLogger 设置自定义日志输出
func (o *PriceOracle) SetLogger(logger *log.Logger) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.logger = logger
}

// ===========================================================================
// 辅助查询方法
// ===========================================================================

// GetSupportedPairs 返回当前所有价格源支持的交易对的并集
func (o *PriceOracle) GetSupportedPairs() []TradingPair {
	o.mu.RLock()
	defer o.mu.RUnlock()

	pairSet := make(map[string]TradingPair)
	for _, src := range o.sources {
		for _, pair := range src.SupportedPairs() {
			pairSet[pair.String()] = pair
		}
	}

	pairs := make([]TradingPair, 0, len(pairSet))
	for _, p := range pairSet {
		pairs = append(pairs, p)
	}
	return pairs
}

// HealthCheck 检查预言机及其价格源的健康状况。
// 返回包含每个源状态的映射。
func (o *PriceOracle) HealthCheck() map[string]bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	status := make(map[string]bool)
	for _, src := range o.sources {
		status[src.GetName()] = src.IsAvailable()
	}
	return status
}

// ===========================================================================
// 价格历史与告警
// ===========================================================================

// checkPriceDeviationWithOldPrice 使用提供的旧价格检查价格偏差并在超过阈值时触发告警
func (o *PriceOracle) checkPriceDeviationWithOldPrice(aggPrice AggregatedPrice, oldPrice float64) {
	// 如果没有旧价格（第一次获取），跳过检查
	if oldPrice <= 0 {
		return
	}

	newPrice := aggPrice.Price

	// 计算偏差百分比
	deviation := math.Abs((newPrice-oldPrice)/oldPrice) * 100

	// 偏差阈值（可在配置中添加）
	const deviationThreshold = 10.0 // 10%

	if deviation > deviationThreshold {
		o.alertHandlerMu.RLock()
		handler := o.alertHandler
		o.alertHandlerMu.RUnlock()

		if handler != nil {
			handler.OnPriceDeviation(aggPrice.Pair, oldPrice, newPrice, deviation)
		}
	}
}

// checkPriceDeviation 检查价格偏差并在超过阈值时触发告警
func (o *PriceOracle) checkPriceDeviation(aggPrice AggregatedPrice) {
	o.priceHistoryMu.RLock()
	history, exists := o.priceHistory[aggPrice.Pair.String()]
	o.priceHistoryMu.RUnlock()

	if !exists || len(history) == 0 {
		return
	}

	// 获取最近的历史价格
	lastEntry := history[len(history)-1]
	oldPrice := lastEntry.Price
	newPrice := aggPrice.Price

	// 计算偏差百分比
	deviation := math.Abs((newPrice-oldPrice)/oldPrice) * 100

	// 偏差阈值（可在配置中添加）
	const deviationThreshold = 10.0 // 10%

	if deviation > deviationThreshold {
		o.alertHandlerMu.RLock()
		handler := o.alertHandler
		o.alertHandlerMu.RUnlock()

		if handler != nil {
			handler.OnPriceDeviation(aggPrice.Pair, oldPrice, newPrice, deviation)
		}
	}
}

// getLastPrice 获取指定交易对的最新历史价格
func (o *PriceOracle) getLastPrice(pair TradingPair) float64 {
	o.priceHistoryMu.RLock()
	defer o.priceHistoryMu.RUnlock()

	history, exists := o.priceHistory[pair.String()]
	if !exists || len(history) == 0 {
		return 0
	}

	return history[len(history)-1].Price
}

// addPriceHistory 添加价格到历史记录（保留最近100条）
func (o *PriceOracle) addPriceHistory(pair TradingPair, price float64) {
	o.priceHistoryMu.Lock()
	defer o.priceHistoryMu.Unlock()

	const maxHistory = 100

	pairKey := pair.String()
	history := o.priceHistory[pairKey]

	// 添加新条目
	history = append(history, priceHistoryEntry{
		Price:     price,
		Timestamp: time.Now(),
	})

	// 限制历史记录数量
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	o.priceHistory[pairKey] = history
}
