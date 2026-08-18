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

// PriceOracle is the core struct of the multi-source price oracle.
// It manages multiple price sources, aggregates price data, and provides caching and slippage protection.
type PriceOracle struct {
	mu      sync.RWMutex
	config  OracleConfig
	sources []PriceSource

	// cache stores aggregated prices per trading pair
	cache map[string]cacheEntry

	// cacheHits is the cache hit count (atomic)
	cacheHits int64
	// cacheMisses is the cache miss count (atomic)
	cacheMisses int64

	// priceHistory holds price history records (for deviation detection)
	priceHistory   map[string][]priceHistoryEntry
	priceHistoryMu sync.RWMutex

	// alertHandler handles price deviation alerts
	alertHandler   AlertHandler
	alertHandlerMu sync.RWMutex

	// running indicates whether the oracle is running
	running bool

	// stopCh signals the background refresh goroutine to stop
	stopCh chan struct{}

	// logger for log output
	logger *log.Logger
}

// NewPriceOracle creates a new price oracle instance.
// sources is the list of price sources; config is the configuration.
// Returns an error if config fails validation.
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

// NewDefaultPriceOracle creates an oracle with default sources and default config.
func NewDefaultPriceOracle() (*PriceOracle, error) {
	return NewPriceOracle(DefaultSources(), DefaultConfig())
}

// Start starts the oracle's background auto-refresh.
// Once called, the oracle refreshes prices for all supported pairs at the configured interval.
func (o *PriceOracle) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.mu.Unlock()

	// Refresh immediately on startup
	o.refreshAll()

	// Periodic background refresh
	go o.refreshLoop()
}

// Stop stops the oracle's background auto-refresh
func (o *PriceOracle) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.running {
		return
	}

	o.running = false
	close(o.stopCh)
}

// IsRunning returns whether the oracle is running
func (o *PriceOracle) IsRunning() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.running
}

// refreshLoop is the background refresh loop
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

// refreshAll refreshes prices for all supported pairs
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

// GetPrice returns the aggregated price for the given pair.
// Uses the cache first; refetches when the cache has expired.
func (o *PriceOracle) GetPrice(pair TradingPair) (AggregatedPrice, error) {
	// Check the cache first
	o.mu.RLock()
	entry, exists := o.cache[pair.String()]
	o.mu.RUnlock()

	if exists && !entry.isExpired() {
		// Cache hit - update stats
		atomic.AddInt64(&o.cacheHits, 1)
		return entry.price, nil
	}

	// Cache miss - update stats
	atomic.AddInt64(&o.cacheMisses, 1)

	// Get the old price for deviation detection (before fetching the new price)
	oldPrice := o.getLastPrice(pair)

	// Cache missing or expired; refetch
	aggPrice, err := o.fetchAndAggregate(pair)
	if err != nil {
		return aggPrice, err
	}

	// Check price deviation and trigger alerts
	o.checkPriceDeviationWithOldPrice(aggPrice, oldPrice)

	return aggPrice, nil
}

// GetPriceFromSource fetches the price from a specific source (no aggregation)
func (o *PriceOracle) GetPriceFromSource(pair TradingPair, sourceName string) (PriceData, error) {
	for _, source := range o.sources {
		if source.GetName() == sourceName && source.IsAvailable() {
			return source.FetchPrice(pair)
		}
	}
	return PriceData{}, fmt.Errorf("%w: source %q not found or unavailable", ErrPriceUnavailable, sourceName)
}

// GetAllPrices fetches the given pair's price from all available sources (no aggregation)
func (o *PriceOracle) GetAllPrices(pair TradingPair) []PriceData {
	return o.collectPrices(pair)
}

// fetchAndAggregate collects prices from all available sources and aggregates them
func (o *PriceOracle) fetchAndAggregate(pair TradingPair) (AggregatedPrice, error) {
	prices := o.collectPrices(pair)

	if len(prices) < o.config.MinSources {
		return AggregatedPrice{}, fmt.Errorf(
			"%w: got %d sources, need at least %d for %s",
			ErrPriceUnavailable, len(prices), o.config.MinSources, pair,
		)
	}

	// Aggregate
	aggregated := o.aggregate(pair, prices)

	// Update the cache
	o.mu.Lock()
	o.cache[pair.String()] = cacheEntry{
		price:     aggregated,
		expiresAt: time.Now().Add(o.config.CacheTTL),
	}
	o.mu.Unlock()

	// Save price history
	o.addPriceHistory(pair, aggregated.Price)

	return aggregated, nil
}

// collectPrices collects price data from all available sources in parallel
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

		// Check whether the source supports this pair
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

	// Close the channel after all goroutines finish
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

// aggregate aggregates the collected price data.
// Steps:
//  1. Outlier filtering (drop data deviating from the median beyond the threshold)
//  2. Weighted average (Volume-Weighted Average Price)
//  3. Statistics (confidence, standard deviation, etc.)
func (o *PriceOracle) aggregate(pair TradingPair, prices []PriceData) AggregatedPrice {
	if len(prices) == 0 {
		return AggregatedPrice{Pair: pair, Timestamp: time.Now()}
	}

	// With a single source, return directly
	if len(prices) == 1 {
		return AggregatedPrice{
			Pair:        pair,
			Price:       prices[0].Price,
			Sources:     1,
			TotalVolume: prices[0].Volume24h,
			Confidence:  0.5, // lower confidence for a single source
			Timestamp:   time.Now(),
			MinPrice:    prices[0].Price,
			MaxPrice:    prices[0].Price,
			Deviation:   0,
			RawPrices:   prices,
		}
	}

	// Step 1: compute the median
	median := o.calculateMedian(prices)

	// Step 2: outlier filtering
	filtered := o.filterOutliers(prices, median)

	// If nothing remains after filtering, use the original data
	if len(filtered) == 0 {
		filtered = prices
	}

	// Step 3: volume-weighted average price (VWAP)
	vwap := o.calculateVWAP(filtered)

	// Step 4: statistics
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

// calculateMedian computes the median of the prices
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

// filterOutliers removes price data deviating from the median beyond the threshold.
// The threshold is given by config.DeviationThreshold (percent).
func (o *PriceOracle) filterOutliers(prices []PriceData, median float64) []PriceData {
	if median <= 0 {
		return prices
	}

	threshold := o.config.DeviationThreshold / 100.0 // convert to fraction
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

// calculateVWAP computes the volume-weighted average price (VWAP).
// Falls back to a simple average if all source volumes are zero.
func (o *PriceOracle) calculateVWAP(prices []PriceData) float64 {
	var totalWeightedPrice float64
	var totalVolume float64

	for _, p := range prices {
		volume := p.Volume24h
		if volume <= 0 {
			volume = 1 // minimum weight when no volume data
		}
		totalWeightedPrice += p.Price * volume
		totalVolume += volume
	}

	if totalVolume <= 0 {
		// Fall back to a simple average
		var sum float64
		for _, p := range prices {
			sum += p.Price
		}
		return sum / float64(len(prices))
	}

	return totalWeightedPrice / totalVolume
}

// calculateStdDev computes the standard deviation of the prices
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

// minMax returns the minimum and maximum prices in the list
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

// totalVolume computes the total volume
func (o *PriceOracle) totalVolume(prices []PriceData) float64 {
	var total float64
	for _, p := range prices {
		total += p.Volume24h
	}
	return total
}

// calculateConfidence computes the confidence (0-1).
// Based on source count and price consistency.
func (o *PriceOracle) calculateConfidence(prices []PriceData, stdDev float64) float64 {
	if len(prices) == 0 {
		return 0
	}

	// Source count factor: more sources -> higher confidence
	// Uses a sigmoid-like curve: 1 source = 0.5, 3 sources = 0.75, 5+ sources = ~0.9
	sourceFactor := 1.0 - 1.0/(1.0+float64(len(prices))*0.5)

	// Consistency factor: smaller std dev -> more consistent -> higher confidence
	// Compute the coefficient of variation (CV = stdDev / mean)
	mean := o.calculateVWAP(prices)
	consistencyFactor := 1.0
	if mean > 0 && stdDev > 0 {
		cv := stdDev / mean
		// CV < 0.01 (1%) -> high consistency
		// CV > 0.05 (5%) -> low consistency
		consistencyFactor = math.Max(0.1, 1.0-cv*10)
	}

	// Source-type diversity bonus: mixed source types yield more reliable prices
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
// Slippage protection
// ===========================================================================

// CalculateSlippage computes slippage for a given pair and input amount.
// inputAmount: amount of the base asset
// pair: trading pair
// Returns the slippage analysis result.
func (o *PriceOracle) CalculateSlippage(pair TradingPair, inputAmount float64) (SlippageResult, error) {
	// Get the current aggregated price
	aggPrice, err := o.GetPrice(pair)
	if err != nil {
		return SlippageResult{}, fmt.Errorf("cannot calculate slippage: %w", err)
	}

	if aggPrice.Price <= 0 {
		return SlippageResult{}, fmt.Errorf("%w: aggregated price is zero for %s", ErrPriceUnavailable, pair)
	}

	// Expected output with zero slippage
	expectedOutput := inputAmount * aggPrice.Price

	// Price impact estimation
	// Uses a simplified constant-product AMM model (x * y = k)
	// Price impact ≈ inputAmount / totalLiquidity
	priceImpact := o.estimatePriceImpact(aggPrice, inputAmount)

	// Actual output after slippage
	actualOutput := expectedOutput * (1 - priceImpact/100)

	// Slippage percentage
	slippagePercent := 0.0
	if expectedOutput > 0 {
		slippagePercent = (expectedOutput - actualOutput) / expectedOutput * 100
	}

	// Minimum output satisfying the max slippage limit
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

// ValidateSlippage checks whether a trade's slippage is within the allowed range.
// Returns ErrSlippageTooHigh if slippage exceeds MaxSlippage.
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

// estimatePriceImpact estimates the price impact percentage.
// Estimated using a constant-product AMM model.
func (o *PriceOracle) estimatePriceImpact(aggPrice AggregatedPrice, inputAmount float64) float64 {
	// Get total liquidity from the raw price data
	var totalLiquidity float64
	for _, p := range aggPrice.RawPrices {
		if p.Liquidity > 0 {
			totalLiquidity += p.Liquidity
		}
	}

	// If no liquidity data, approximate with volume
	if totalLiquidity <= 0 {
		totalLiquidity = aggPrice.TotalVolume
	}

	// If still no liquidity data, return a conservative estimate
	if totalLiquidity <= 0 {
		return 0.1 // default 0.1% price impact
	}

	// USD value of the input amount
	inputValueUSD := inputAmount * aggPrice.Price

	// Price impact estimate: impact% ≈ (inputValue / totalLiquidity) * 100
	// Uses a simplified version of the constant-product formula
	impact := (inputValueUSD / totalLiquidity) * 100

	// Non-linear adjustment: impact grows faster for large trades
	if impact > 1.0 {
		impact = impact * (1.0 + impact*0.1)
	}

	return impact
}

// ===========================================================================
// Price cache management
// ===========================================================================

// InvalidateCache invalidates the cache for the given pair
func (o *PriceOracle) InvalidateCache(pair TradingPair) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.cache, pair.String())
}

// InvalidateAllCache invalidates all cached entries
func (o *PriceOracle) InvalidateAllCache() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache = make(map[string]cacheEntry)
}

// GetCachedPrice reads the price from cache only (no refresh).
// Returns ErrPriceStale if the cache is missing or expired.
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
// Price source management
// ===========================================================================

// AddSource adds a new price source
func (o *PriceOracle) AddSource(source PriceSource) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sources = append(o.sources, source)
}

// RemoveSource removes a price source by name
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

// GetSources returns all registered price sources
func (o *PriceOracle) GetSources() []PriceSource {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]PriceSource, len(o.sources))
	copy(result, o.sources)
	return result
}

// GetAvailableSources returns the currently available price sources
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
// Configuration management
// ===========================================================================

// GetConfig returns a copy of the current config
func (o *PriceOracle) GetConfig() OracleConfig {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.config
}

// UpdateConfig updates the oracle configuration.
// The new config must pass validation.
func (o *PriceOracle) UpdateConfig(config OracleConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	o.mu.Lock()
	o.config = config
	o.mu.Unlock()

	return nil
}

// SetLogger sets a custom logger
func (o *PriceOracle) SetLogger(logger *log.Logger) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.logger = logger
}

// ===========================================================================
// Helper query methods
// ===========================================================================

// GetSupportedPairs returns the union of pairs supported by all registered sources
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

// HealthCheck checks the health of the oracle and its price sources.
// Returns a map with each source's status.
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
// Price history and alerts
// ===========================================================================

// checkPriceDeviationWithOldPrice checks price deviation against the given old price and alerts when the threshold is exceeded
func (o *PriceOracle) checkPriceDeviationWithOldPrice(aggPrice AggregatedPrice, oldPrice float64) {
	// No old price (first fetch); skip the check
	if oldPrice <= 0 {
		return
	}

	newPrice := aggPrice.Price

	// Compute deviation percentage
	deviation := math.Abs((newPrice-oldPrice)/oldPrice) * 100

	// Deviation threshold (could be made configurable)
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

// checkPriceDeviation checks price deviation and alerts when the threshold is exceeded
func (o *PriceOracle) checkPriceDeviation(aggPrice AggregatedPrice) {
	o.priceHistoryMu.RLock()
	history, exists := o.priceHistory[aggPrice.Pair.String()]
	o.priceHistoryMu.RUnlock()

	if !exists || len(history) == 0 {
		return
	}

	// Get the most recent historical price
	lastEntry := history[len(history)-1]
	oldPrice := lastEntry.Price
	newPrice := aggPrice.Price

	// Compute deviation percentage
	deviation := math.Abs((newPrice-oldPrice)/oldPrice) * 100

	// Deviation threshold (could be made configurable)
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

// getLastPrice returns the latest historical price for the given pair
func (o *PriceOracle) getLastPrice(pair TradingPair) float64 {
	o.priceHistoryMu.RLock()
	defer o.priceHistoryMu.RUnlock()

	history, exists := o.priceHistory[pair.String()]
	if !exists || len(history) == 0 {
		return 0
	}

	return history[len(history)-1].Price
}

// addPriceHistory appends a price to the history (keeping the most recent 100)
func (o *PriceOracle) addPriceHistory(pair TradingPair, price float64) {
	o.priceHistoryMu.Lock()
	defer o.priceHistoryMu.Unlock()

	const maxHistory = 100

	pairKey := pair.String()
	history := o.priceHistory[pairKey]

	// Append the new entry
	history = append(history, priceHistoryEntry{
		Price:     price,
		Timestamp: time.Now(),
	})

	// Cap the history length
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	o.priceHistory[pairKey] = history
}
