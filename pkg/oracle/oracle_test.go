// Package oracle provides unit tests for the price oracle functionality.
package oracle

import (
	"math"
	"testing"
	"time"
)

// ============================================================================
// Mock Price Source for Testing
// ============================================================================

// mockPriceSource is a test implementation of PriceSource
type mockPriceSource struct {
	name      string
	srcType   SourceType
	available bool
	prices    map[string]PriceData
	pairs     []TradingPair
}

func newMockSource(name string, srcType SourceType, pairs []TradingPair) *mockPriceSource {
	return &mockPriceSource{
		name:      name,
		srcType:   srcType,
		available: true,
		prices:    make(map[string]PriceData),
		pairs:     pairs,
	}
}

func (m *mockPriceSource) setPrice(pair TradingPair, price float64, volume float64) {
	m.prices[pair.String()] = PriceData{
		Pair:       pair,
		Price:      price,
		Volume24h:  volume,
		Timestamp:  time.Now(),
		Source:     m.name,
		SourceType: m.srcType,
		Bid:        price * 0.999,
		Ask:        price * 1.001,
		Liquidity:  volume * 10,
	}
}

func (m *mockPriceSource) FetchPrice(pair TradingPair) (PriceData, error) {
	pd, ok := m.prices[pair.String()]
	if !ok {
		return PriceData{}, ErrPairNotSupported
	}
	return pd, nil
}

func (m *mockPriceSource) IsAvailable() bool {
	return m.available
}

func (m *mockPriceSource) GetName() string {
	return m.name
}

func (m *mockPriceSource) GetType() SourceType {
	return m.srcType
}

func (m *mockPriceSource) SupportedPairs() []TradingPair {
	return m.pairs
}

// Helper function to create oracle with mock sources
func createTestOracle(config OracleConfig, sources ...PriceSource) (*PriceOracle, error) {
	return NewPriceOracle(sources, config)
}

// ============================================================================
// TradingPair Tests
// ============================================================================

func TestTradingPair_String(t *testing.T) {
	pair := TradingPair{Base: "AIB", Quote: "USD"}
	if pair.String() != "AIB/USD" {
		t.Errorf("expected AIB/USD, got %s", pair.String())
	}
}

func TestTradingPair_Equal(t *testing.T) {
	pair1 := TradingPair{Base: "AIB", Quote: "USD"}
	pair2 := TradingPair{Base: "AIB", Quote: "USD"}
	pair3 := TradingPair{Base: "BTC", Quote: "USD"}

	if !pair1.Equal(pair2) {
		t.Error("identical pairs should be equal")
	}
	if pair1.Equal(pair3) {
		t.Error("different pairs should not be equal")
	}
}

func TestSupportedPairs(t *testing.T) {
	pairs := SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}

	// Verify AIB/USD is supported
	found := false
	for _, p := range pairs {
		if p.Base == "AIB" && p.Quote == "USD" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AIB/USD should be a supported pair")
	}
}

func TestPredefinedPairs(t *testing.T) {
	if PairAIBUSD.Base != "AIB" || PairAIBUSD.Quote != "USD" {
		t.Error("PairAIBUSD should be AIB/USD")
	}
	if PairBTCUSD.Base != "BTC" || PairBTCUSD.Quote != "USD" {
		t.Error("PairBTCUSD should be BTC/USD")
	}
	if PairETHUSD.Base != "ETH" || PairETHUSD.Quote != "USD" {
		t.Error("PairETHUSD should be ETH/USD")
	}
}

// ============================================================================
// PriceData Tests
// ============================================================================

func TestPriceData_IsValid(t *testing.T) {
	// Valid price data
	valid := PriceData{
		Price:     100.0,
		Timestamp: time.Now(),
		Source:    "test",
	}
	if !valid.IsValid() {
		t.Error("should be valid")
	}

	// Invalid: zero price
	zeroPrice := PriceData{
		Price:     0,
		Timestamp: time.Now(),
		Source:    "test",
	}
	if zeroPrice.IsValid() {
		t.Error("zero price should be invalid")
	}

	// Invalid: negative price
	negPrice := PriceData{
		Price:     -1.0,
		Timestamp: time.Now(),
		Source:    "test",
	}
	if negPrice.IsValid() {
		t.Error("negative price should be invalid")
	}

	// Invalid: zero timestamp
	zeroTime := PriceData{
		Price:  100.0,
		Source: "test",
	}
	if zeroTime.IsValid() {
		t.Error("zero timestamp should be invalid")
	}

	// Invalid: empty source
	emptySource := PriceData{
		Price:     100.0,
		Timestamp: time.Now(),
		Source:    "",
	}
	if emptySource.IsValid() {
		t.Error("empty source should be invalid")
	}
}

func TestPriceData_Spread(t *testing.T) {
	// Normal spread
	pd := PriceData{
		Bid: 100.0,
		Ask: 101.0,
	}
	spread := pd.Spread()
	if spread <= 0 || spread > 2 {
		t.Errorf("spread should be positive and reasonable, got %f", spread)
	}

	// Zero bid
	zeroBid := PriceData{Bid: 0, Ask: 101.0}
	if zeroBid.Spread() != 0 {
		t.Error("zero bid should return 0 spread")
	}

	// Zero ask
	zeroAsk := PriceData{Bid: 100.0, Ask: 0}
	if zeroAsk.Spread() != 0 {
		t.Error("zero ask should return 0 spread")
	}
}

func TestPriceData_Age(t *testing.T) {
	pd := PriceData{
		Timestamp: time.Now().Add(-5 * time.Minute),
	}
	age := pd.Age()
	if age < 4*time.Minute || age > 6*time.Minute {
		t.Errorf("age should be around 5 minutes, got %v", age)
	}
}

// ============================================================================
// SourceType Tests
// ============================================================================

func TestSourceType_String(t *testing.T) {
	tests := []struct {
		st       SourceType
		expected string
	}{
		{SourceTypeDEX, "DEX"},
		{SourceTypeCEX, "CEX"},
		{SourceTypeStablecoin, "Stablecoin"},
		{SourceType(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		if tt.st.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.st.String())
		}
	}
}

// ============================================================================
// OracleConfig Tests
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.RefreshInterval <= 0 {
		t.Error("RefreshInterval should be positive")
	}
	if config.CacheTTL <= 0 {
		t.Error("CacheTTL should be positive")
	}
	if config.DeviationThreshold <= 0 || config.DeviationThreshold >= 100 {
		t.Error("DeviationThreshold should be between 0 and 100")
	}
	if config.MinSources < 1 {
		t.Error("MinSources should be at least 1")
	}
	if config.MaxSlippage <= 0 || config.MaxSlippage >= 100 {
		t.Error("MaxSlippage should be between 0 and 100")
	}
}

func TestOracleConfig_Validate(t *testing.T) {
	// Valid config
	validConfig := DefaultConfig()
	if err := validConfig.Validate(); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}

	// Invalid: zero refresh interval
	invalidRefresh := DefaultConfig()
	invalidRefresh.RefreshInterval = 0
	if err := invalidRefresh.Validate(); err == nil {
		t.Error("zero refresh interval should error")
	}

	// Invalid: zero cache TTL
	invalidTTL := DefaultConfig()
	invalidTTL.CacheTTL = 0
	if err := invalidTTL.Validate(); err == nil {
		t.Error("zero cache TTL should error")
	}

	// Invalid: zero deviation threshold
	invalidDev := DefaultConfig()
	invalidDev.DeviationThreshold = 0
	if err := invalidDev.Validate(); err == nil {
		t.Error("zero deviation threshold should error")
	}

	// Invalid: deviation threshold >= 100
	invalidDev2 := DefaultConfig()
	invalidDev2.DeviationThreshold = 100
	if err := invalidDev2.Validate(); err == nil {
		t.Error("deviation threshold >= 100 should error")
	}

	// Invalid: zero min sources
	invalidMin := DefaultConfig()
	invalidMin.MinSources = 0
	if err := invalidMin.Validate(); err == nil {
		t.Error("zero min sources should error")
	}

	// Invalid: zero max slippage
	invalidSlip := DefaultConfig()
	invalidSlip.MaxSlippage = 0
	if err := invalidSlip.Validate(); err == nil {
		t.Error("zero max slippage should error")
	}

	// Invalid: max slippage >= 100
	invalidSlip2 := DefaultConfig()
	invalidSlip2.MaxSlippage = 100
	if err := invalidSlip2.Validate(); err == nil {
		t.Error("max slippage >= 100 should error")
	}

	// Invalid: zero stale threshold
	invalidStale := DefaultConfig()
	invalidStale.StaleThreshold = 0
	if err := invalidStale.Validate(); err == nil {
		t.Error("zero stale threshold should error")
	}
}

// ============================================================================
// PriceOracle Tests
// ============================================================================

func TestNewPriceOracle(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, err := NewPriceOracle([]PriceSource{source}, config)
	if err != nil {
		t.Fatalf("failed to create oracle: %v", err)
	}

	if oracle == nil {
		t.Fatal("oracle should not be nil")
	}
}

func TestNewPriceOracle_InvalidConfig(t *testing.T) {
	invalidConfig := OracleConfig{
		RefreshInterval: 0, // Invalid
	}
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})

	_, err := NewPriceOracle([]PriceSource{source}, invalidConfig)
	if err == nil {
		t.Error("invalid config should error")
	}
}

func TestNewPriceOracle_NoSources(t *testing.T) {
	config := DefaultConfig()
	_, err := NewPriceOracle([]PriceSource{}, config)
	if err == nil {
		t.Error("empty sources should error")
	}
}

func TestPriceOracle_GetPrice(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if price.Price <= 0 {
		t.Error("price should be positive")
	}

	if price.Price != 50000.0 {
		t.Errorf("expected price 50000, got %f", price.Price)
	}
}

func TestPriceOracle_GetPrice_MultiSource(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	source1 := newMockSource("Source1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 2000.0)

	source2 := newMockSource("Source2", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50100.0, 1000.0)

	source3 := newMockSource("Source3", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source3.setPrice(PairBTCUSD, 49900.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2, source3}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if price.Sources < 3 {
		t.Errorf("expected 3 sources, got %d", price.Sources)
	}

	// VWAP should be weighted by volume
	// Expected VWAP = (50000*2000 + 50100*1000 + 49900*500) / (2000+1000+500)
	// = (100000000 + 50100000 + 24950000) / 3500
	// = 175050000 / 3500 = 50014.28...
	if price.Price < 49900 || price.Price > 50100 {
		t.Errorf("VWAP price should be within range, got %f", price.Price)
	}
}

func TestPriceOracle_GetPrice_InsufficientSources(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 3

	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)
	_, err := oracle.GetPrice(PairBTCUSD)
	if err == nil {
		t.Error("should error with insufficient sources")
	}
}

// ============================================================================
// Outlier Filtering Tests
// ============================================================================

func TestPriceOracle_OutlierFiltering(t *testing.T) {
	config := DefaultConfig()
	config.DeviationThreshold = 5.0 // 5% deviation threshold
	config.MinSources = 1

	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50100.0, 1000.0)

	// This source has an outlier price (>5% deviation)
	source3 := newMockSource("Outlier", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source3.setPrice(PairBTCUSD, 60000.0, 1000.0) // 20% deviation

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2, source3}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// The aggregated price should not be heavily influenced by the outlier
	if price.Price > 52000 { // Should be much closer to 50000-50100
		t.Errorf("outlier should be filtered, price too high: %f", price.Price)
	}
}

// ============================================================================
// VWAP Calculation Tests
// ============================================================================

func TestPriceOracle_VWAP(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	// Source with high volume
	source1 := newMockSource("HighVol", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 10000.0)

	// Source with low volume
	source2 := newMockSource("LowVol", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 51000.0, 100.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// VWAP should be closer to 50000 (high volume source)
	if math.Abs(price.Price-50000) > math.Abs(price.Price-51000) {
		t.Error("VWAP should be closer to high-volume source price")
	}
}

// ============================================================================
// Cache Tests
// ============================================================================

func TestPriceOracle_Cache(t *testing.T) {
	config := DefaultConfig()
	config.CacheTTL = 10 * time.Second

	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// First call - fetches from source
	price1, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// Update source price
	source.setPrice(PairBTCUSD, 55000.0, 1000.0)

	// Second call - should use cache
	price2, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// Cached price should be the same as first call
	if price1.Price != price2.Price {
		t.Errorf("cached price should be same: %f vs %f", price1.Price, price2.Price)
	}
}

func TestPriceOracle_InvalidateCache(t *testing.T) {
	config := DefaultConfig()
	config.CacheTTL = 1 * time.Hour

	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// First call
	oracle.GetPrice(PairBTCUSD)

	// Invalidate cache
	oracle.InvalidateCache(PairBTCUSD)

	// Update source
	source.setPrice(PairBTCUSD, 55000.0, 1000.0)

	// Second call should fetch fresh data
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if price.Price != 55000.0 {
		t.Errorf("expected fresh price 55000, got %f", price.Price)
	}
}

func TestPriceOracle_GetPrice_FailoverWithPartialSourceFailures(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 2
	config.CacheTTL = 1 * time.Millisecond

	// Two healthy sources
	healthy1 := newMockSource("Healthy1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	healthy1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	healthy2 := newMockSource("Healthy2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	healthy2.setPrice(PairBTCUSD, 50200.0, 800.0)

	// One unavailable source (simulating a failure)
	down := newMockSource("Down", SourceTypeCEX, []TradingPair{PairBTCUSD})
	down.available = false

	// One source that is available but does not return this pair (FetchPrice failure path)
	unsupported := newMockSource("Unsupported", SourceTypeCEX, []TradingPair{PairETHUSD})
	unsupported.setPrice(PairETHUSD, 3000.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{healthy1, healthy2, down, unsupported}, config)

	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice should succeed with failover sources, got: %v", err)
	}

	if price.Sources != 2 {
		t.Errorf("expected 2 healthy sources in aggregation, got %d", price.Sources)
	}
	if price.Price <= 0 {
		t.Errorf("aggregated price should be positive, got %f", price.Price)
	}
}

func TestPriceOracle_GetPrice_FailoverAllSourcesFailed(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	down1 := newMockSource("Down1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	down1.available = false
	down2 := newMockSource("Down2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	down2.available = false

	oracle, _ := NewPriceOracle([]PriceSource{down1, down2}, config)
	_, err := oracle.GetPrice(PairBTCUSD)
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
}

func TestPriceOracle_OutlierFiltering_AllOutliersFallbackToRaw(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1
	config.DeviationThreshold = 0.0001 // Tiny threshold, forcing most data to be filtered out

	// Use more dispersed prices so only the middle source S2 is not filtered out
	s1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	s2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	s3 := newMockSource("S3", SourceTypeStablecoin, []TradingPair{PairBTCUSD})

	s1.setPrice(PairBTCUSD, 49000.0, 1000.0)
	s2.setPrice(PairBTCUSD, 50000.0, 1000.0)
	s3.setPrice(PairBTCUSD, 51000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{s1, s2, s3}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// The median (50000) source should be kept, others filtered out
	if price.Sources != 1 {
		t.Errorf("expected only median source (1) to remain after strict filtering, got %d", price.Sources)
	}
	// The price should be the median price
	if price.Price != 50000.0 {
		t.Errorf("expected median price 50000.0, got %f", price.Price)
	}
}

func TestPriceOracle_OutlierFiltering_SymmetricOutliers(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1
	config.DeviationThreshold = 5.0 // 5% threshold

	s1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	s2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	s3 := newMockSource("S3", SourceTypeStablecoin, []TradingPair{PairBTCUSD})
	s4 := newMockSource("S4", SourceTypeCEX, []TradingPair{PairBTCUSD})

	// S1 and S4 are symmetric outliers (exceeding the 5% threshold)
	s1.setPrice(PairBTCUSD, 46000.0, 1000.0) // -8% deviation
	s2.setPrice(PairBTCUSD, 49900.0, 1000.0)
	s3.setPrice(PairBTCUSD, 50100.0, 1000.0)
	s4.setPrice(PairBTCUSD, 54000.0, 1000.0) // +8% deviation

	oracle, _ := NewPriceOracle([]PriceSource{s1, s2, s3, s4}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// Outliers should be filtered out, leaving 2 sources
	if price.Sources != 2 {
		t.Errorf("expected 2 sources after filtering outliers, got %d", price.Sources)
	}
	// The price should be close to 50000
	if price.Price < 49900 || price.Price > 50100 {
		t.Errorf("expected price near 50000, got %f", price.Price)
	}
}

func TestPriceOracle_ValidateSlippage_TooHighRejected(t *testing.T) {
	config := DefaultConfig()
	config.MaxSlippage = 1.0 // Strict limit

	// Low liquidity, triggering high slippage
	source := newMockSource("LowLiquidity", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	err := oracle.ValidateSlippage(PairBTCUSD, 1000.0, 1.0)
	if err == nil {
		t.Fatal("expected ErrSlippageTooHigh for excessive slippage")
	}
}

func TestPriceOracle_ValidateSlippage_MinOutputProtection(t *testing.T) {
	config := DefaultConfig()
	config.MaxSlippage = 10.0

	source := newMockSource("Normal", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	result, err := oracle.CalculateSlippage(PairBTCUSD, 1.0)
	if err != nil {
		t.Fatalf("CalculateSlippage failed: %v", err)
	}

	// Set minOutput higher than the actual output to trigger protection
	err = oracle.ValidateSlippage(PairBTCUSD, 1.0, result.ActualOutput+1.0)
	if err == nil {
		t.Fatal("expected slippage validation to fail due to min output protection")
	}
}

func TestPriceOracle_GetCachedPrice_ExpiredAfterTTL(t *testing.T) {
	config := DefaultConfig()
	config.CacheTTL = 5 * time.Millisecond

	source := newMockSource("TTLSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)
	_, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	_, err = oracle.GetCachedPrice(PairBTCUSD)
	if err == nil {
		t.Fatal("expected ErrPriceStale after cache TTL expired")
	}
}

// ============================================================================
// Source Management Tests
// ============================================================================

func TestPriceOracle_AddSource(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1}, config)

	sources := oracle.GetSources()
	if len(sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources))
	}

	// Add new source
	source2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	oracle.AddSource(source2)

	sources = oracle.GetSources()
	if len(sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(sources))
	}
}

func TestPriceOracle_RemoveSource(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50100.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)

	// Remove S2
	oracle.RemoveSource("S2")

	sources := oracle.GetSources()
	if len(sources) != 1 {
		t.Errorf("expected 1 source after removal, got %d", len(sources))
	}
}

func TestPriceOracle_GetAvailableSources(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("Available", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("Unavailable", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.available = false

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)

	available := oracle.GetAvailableSources()
	if len(available) != 1 {
		t.Errorf("expected 1 available source, got %d", len(available))
	}
}

func TestPriceOracle_GetAllPrices(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50100.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)

	allPrices := oracle.GetAllPrices(PairBTCUSD)
	if len(allPrices) != 2 {
		t.Errorf("expected 2 prices, got %d", len(allPrices))
	}
}

func TestPriceOracle_GetPriceFromSource(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	price, err := oracle.GetPriceFromSource(PairBTCUSD, "TestSource")
	if err != nil {
		t.Fatalf("GetPriceFromSource failed: %v", err)
	}
	if price.Price != 50000.0 {
		t.Errorf("expected price 50000, got %f", price.Price)
	}

	// Non-existent source
	_, err = oracle.GetPriceFromSource(PairBTCUSD, "NonExistent")
	if err == nil {
		t.Error("non-existent source should error")
	}
}

// ============================================================================
// Configuration Tests
// ============================================================================

func TestPriceOracle_GetConfig(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	gotConfig := oracle.GetConfig()
	if gotConfig.MinSources != config.MinSources {
		t.Error("config mismatch")
	}
}

func TestPriceOracle_UpdateConfig(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	newConfig := DefaultConfig()
	newConfig.MinSources = 2
	err := oracle.UpdateConfig(newConfig)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	gotConfig := oracle.GetConfig()
	if gotConfig.MinSources != 2 {
		t.Error("config should be updated")
	}
}

func TestPriceOracle_UpdateConfig_Invalid(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	invalidConfig := OracleConfig{
		RefreshInterval: 0, // Invalid
	}
	err := oracle.UpdateConfig(invalidConfig)
	if err == nil {
		t.Error("invalid config update should error")
	}
}

// ============================================================================
// Slippage Calculation Tests
// ============================================================================

func TestPriceOracle_CalculateSlippage(t *testing.T) {
	config := DefaultConfig()
	config.MaxSlippage = 3.0

	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000000.0) // High volume

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	result, err := oracle.CalculateSlippage(PairBTCUSD, 1.0)
	if err != nil {
		t.Fatalf("CalculateSlippage failed: %v", err)
	}

	if result.InputAmount != 1.0 {
		t.Errorf("expected input 1.0, got %f", result.InputAmount)
	}
	if result.ExpectedOutput <= 0 {
		t.Error("expected output should be positive")
	}
	if result.SlippagePercent < 0 {
		t.Error("slippage should not be negative")
	}
}

func TestPriceOracle_CalculateSlippage_HighAmount(t *testing.T) {
	config := DefaultConfig()
	config.MaxSlippage = 3.0

	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 100.0) // Low volume

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Large input amount relative to liquidity
	result, err := oracle.CalculateSlippage(PairBTCUSD, 1000.0)
	if err != nil {
		t.Fatalf("CalculateSlippage failed: %v", err)
	}

	// Slippage should be higher for large amounts
	if result.SlippagePercent <= 0 {
		t.Error("slippage should be positive for large amounts")
	}
}

func TestPriceOracle_ValidateSlippage(t *testing.T) {
	config := DefaultConfig()
	config.MaxSlippage = 3.0

	source := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Small amount should be acceptable
	err := oracle.ValidateSlippage(PairBTCUSD, 0.01, 400.0)
	if err != nil {
		t.Errorf("small amount slippage should be acceptable: %v", err)
	}
}

// ============================================================================
// Health Check Tests
// ============================================================================

func TestPriceOracle_HealthCheck(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("Available", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("Down", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source2.available = false

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)

	status := oracle.HealthCheck()
	if !status["Available"] {
		t.Error("Available source should be healthy")
	}
	if status["Down"] {
		t.Error("Down source should not be healthy")
	}
}

// ============================================================================
// Start/Stop Tests
// ============================================================================

func TestPriceOracle_StartStop(t *testing.T) {
	config := DefaultConfig()
	config.RefreshInterval = 100 * time.Millisecond

	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	if oracle.IsRunning() {
		t.Error("should not be running initially")
	}

	oracle.Start()
	if !oracle.IsRunning() {
		t.Error("should be running after Start")
	}

	oracle.Stop()
	if oracle.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

// ============================================================================
// Supported Pairs Tests
// ============================================================================

func TestPriceOracle_GetSupportedPairs(t *testing.T) {
	config := DefaultConfig()
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD, PairETHUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)
	source1.setPrice(PairETHUSD, 3000.0, 500.0)

	source2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD, PairAIBUSD})
	source2.setPrice(PairBTCUSD, 50100.0, 500.0)
	source2.setPrice(PairAIBUSD, 1.0, 100.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)

	pairs := oracle.GetSupportedPairs()
	if len(pairs) != 3 { // BTC/USD, ETH/USD, AIB/USD (deduplicated)
		t.Errorf("expected 3 unique pairs, got %d", len(pairs))
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestErrors(t *testing.T) {
	errors := []error{
		ErrNoPriceSources,
		ErrPairNotSupported,
		ErrPriceUnavailable,
		ErrPriceStale,
		ErrSlippageTooHigh,
		ErrInsufficientLiquidity,
		ErrOracleNotRunning,
		ErrInvalidConfig,
	}

	for _, e := range errors {
		if e == nil {
			t.Error("error should not be nil")
		}
		if e.Error() == "" {
			t.Error("error message should not be empty")
		}
	}
}

// ============================================================================
// Confidence Calculation Tests
// ============================================================================

func TestPriceOracle_Confidence(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	// Multiple consistent sources should have higher confidence
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeDEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50010.0, 1000.0)

	source3 := newMockSource("S3", SourceTypeStablecoin, []TradingPair{PairBTCUSD})
	source3.setPrice(PairBTCUSD, 49990.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2, source3}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if price.Confidence <= 0 || price.Confidence > 1 {
		t.Errorf("confidence should be between 0 and 1, got %f", price.Confidence)
	}

	// Multiple sources should have higher confidence
	if price.Confidence < 0.5 {
		t.Errorf("3 consistent sources should have high confidence, got %f", price.Confidence)
	}
}

// ============================================================================
// MinMax Tests
// ============================================================================

func TestPriceOracle_MinMaxPrice(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 51000.0, 1000.0)

	source3 := newMockSource("S3", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source3.setPrice(PairBTCUSD, 49000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2, source3}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if price.MinPrice > price.MaxPrice {
		t.Error("min price should be <= max price")
	}
}

// ============================================================================
// Standard Deviation Tests
// ============================================================================

func TestPriceOracle_Deviation(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	// Very consistent sources
	source1 := newMockSource("S1", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source1.setPrice(PairBTCUSD, 50000.0, 1000.0)

	source2 := newMockSource("S2", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source2.setPrice(PairBTCUSD, 50001.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source1, source2}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// Very small deviation expected
	if price.Deviation > 100 {
		t.Errorf("deviation should be very small for consistent sources, got %f", price.Deviation)
	}
}

// ============================================================================
// Single Source Tests
// ============================================================================

func TestPriceOracle_SingleSource(t *testing.T) {
	config := DefaultConfig()
	config.MinSources = 1

	source := newMockSource("SingleSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)
	price, err := oracle.GetPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	// Single source should have lower confidence (0.5)
	if price.Confidence != 0.5 {
		t.Errorf("single source confidence should be 0.5, got %f", price.Confidence)
	}

	// Min and max should be equal
	if price.MinPrice != price.MaxPrice {
		t.Error("single source min and max should be equal")
	}

	if price.Deviation != 0 {
		t.Error("single source deviation should be 0")
	}
}

// ============================================================================
// Cache statistics tests
// ============================================================================

func TestPriceOracle_GetCacheStats(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Initial state
	stats := oracle.GetCacheStats()
	if stats.TotalRequests != 0 {
		t.Errorf("initial total requests should be 0, got %d", stats.TotalRequests)
	}

	// First request - cache miss
	_, _ = oracle.GetPrice(PairBTCUSD)
	stats = oracle.GetCacheStats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// Second request - cache hit
	_, _ = oracle.GetPrice(PairBTCUSD)
	stats = oracle.GetCacheStats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestPriceOracle_ResetCacheStats(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Generate some statistics
	_, _ = oracle.GetPrice(PairBTCUSD)
	_, _ = oracle.GetPrice(PairBTCUSD)

	// Reset statistics
	oracle.ResetCacheStats()

	stats := oracle.GetCacheStats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.TotalRequests != 0 {
		t.Error("stats should be reset to zero")
	}
}

// ============================================================================
// Cache warmup tests
// ============================================================================

func TestPriceOracle_Warmup(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD, PairETHUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)
	source.setPrice(PairETHUSD, 3000.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Warm up the cache
	oracle.Warmup()

	// Subsequent requests should hit the cache
	oracle.ResetCacheStats()
	_, _ = oracle.GetPrice(PairBTCUSD)
	_, _ = oracle.GetPrice(PairETHUSD)

	stats := oracle.GetCacheStats()
	if stats.Hits < 2 {
		t.Errorf("after warmup, both prices should be cached, hits=%d", stats.Hits)
	}
}

func TestPriceOracle_WarmupPairs(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD, PairETHUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)
	source.setPrice(PairETHUSD, 3000.0, 500.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Warm up only BTC/USD
	oracle.WarmupPairs(PairBTCUSD)
	oracle.ResetCacheStats()

	_, _ = oracle.GetPrice(PairBTCUSD)
	stats := oracle.GetCacheStats()
	if stats.Hits == 0 {
		t.Error("BTC/USD should be cached after warmup")
	}
}

// ============================================================================
// Price history tests
// ============================================================================

func TestPriceOracle_GetPriceHistory(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})
	source.setPrice(PairBTCUSD, 50000.0, 1000.0)

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Initial state - no history
	history := oracle.GetPriceHistory(PairBTCUSD)
	if len(history) != 0 {
		t.Errorf("initial history should be empty, got %d entries", len(history))
	}

	// First price fetch -> added to history
	_, _ = oracle.GetPrice(PairBTCUSD)
	history = oracle.GetPriceHistory(PairBTCUSD)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestPriceOracle_GetAveragePrice(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Should return an error when there is no history
	_, err := oracle.GetAveragePrice(PairBTCUSD)
	if err == nil {
		t.Error("should error when no price history")
	}

	// Add price history
	prices := []float64{50000, 50100, 49900}
	for _, p := range prices {
		source.setPrice(PairBTCUSD, p, 1000.0)
		oracle.InvalidateCache(PairBTCUSD)
		_, _ = oracle.GetPrice(PairBTCUSD)
	}

	// Calculate the average price
	avg, err := oracle.GetAveragePrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("GetAveragePrice failed: %v", err)
	}

	expectedAvg := (50000.0 + 50100.0 + 49900.0) / 3.0
	if math.Abs(avg-expectedAvg) > 0.01 {
		t.Errorf("expected average %f, got %f", expectedAvg, avg)
	}
}

// ============================================================================
// Price deviation alert tests
// ============================================================================

func TestPriceOracle_AlertHandler(t *testing.T) {
	config := DefaultConfig()
	source := newMockSource("TestSource", SourceTypeCEX, []TradingPair{PairBTCUSD})

	oracle, _ := NewPriceOracle([]PriceSource{source}, config)

	// Set the alert handler
	alertReceived := false
	oracle.SetDeviationAlertHandler(func(pair TradingPair, oldPrice, newPrice, deviation float64) {
		alertReceived = true
	})

	// Add stable price history
	for i := 0; i < 5; i++ {
		source.setPrice(PairBTCUSD, 50000.0, 1000.0)
		oracle.InvalidateCache(PairBTCUSD)
		_, _ = oracle.GetPrice(PairBTCUSD)
	}

	// Add a sharply deviating price (20% deviation > 10% threshold)
	source.setPrice(PairBTCUSD, 60000.0, 1000.0)
	oracle.InvalidateCache(PairBTCUSD)
	_, _ = oracle.GetPrice(PairBTCUSD)

	if !alertReceived {
		t.Error("alert should have been received for large price deviation")
	}
}

// ============================================================================
// Alert level tests
// ============================================================================

func TestAlertLevel_String(t *testing.T) {
	tests := []struct {
		level    AlertLevel
		expected string
	}{
		{AlertInfo, "INFO"},
		{AlertWarning, "WARNING"},
		{AlertCritical, "CRITICAL"},
		{AlertLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.level.String())
		}
	}
}

// ============================================================================
// CacheStats tests
// ============================================================================

func TestCacheStats_HitRatePercentage(t *testing.T) {
	stats := &CacheStats{
		Hits:          7,
		Misses:        3,
		TotalRequests: 10,
	}

	hitRate := stats.HitRatePercentage()
	if hitRate != 70.0 {
		t.Errorf("expected 70%% hit rate, got %f", hitRate)
	}

	// Test zero requests
	stats2 := &CacheStats{
		Hits:          0,
		Misses:        0,
		TotalRequests: 0,
	}

	hitRate2 := stats2.HitRatePercentage()
	if hitRate2 != 0 {
		t.Errorf("expected 0%% hit rate for zero requests, got %f", hitRate2)
	}
}

// ============================================================================
// DEX Price Source Tests
// ============================================================================

func TestUniswapSource_New(t *testing.T) {
	src := NewUniswapSource()
	if src == nil {
		t.Fatal("NewUniswapSource should not return nil")
	}
	if src.GetName() != "Uniswap V3" {
		t.Errorf("expected name 'Uniswap V3', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeDEX {
		t.Error("GetType should return SourceTypeDEX")
	}
}

func TestUniswapSource_SupportedPairs(t *testing.T) {
	src := NewUniswapSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
	// Verify ETH/USD is supported
	found := false
	for _, p := range pairs {
		if p.Equal(PairETHUSD) {
			found = true
			break
		}
	}
	if !found {
		t.Error("ETH/USD should be a supported pair")
	}
}

func TestUniswapSource_IsAvailable(t *testing.T) {
	src := NewUniswapSource()
	// Initially should be available
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

func TestSushiSwapSource_New(t *testing.T) {
	src := NewSushiSwapSource()
	if src == nil {
		t.Fatal("NewSushiSwapSource should not return nil")
	}
	if src.GetName() != "SushiSwap" {
		t.Errorf("expected name 'SushiSwap', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeDEX {
		t.Error("GetType should return SourceTypeDEX")
	}
}

func TestSushiSwapSource_SupportedPairs(t *testing.T) {
	src := NewSushiSwapSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
	// Verify ETH/USD is supported
	found := false
	for _, p := range pairs {
		if p.Equal(PairETHUSD) {
			found = true
			break
		}
	}
	if !found {
		t.Error("ETH/USD should be a supported pair")
	}
}

func TestSushiSwapSource_IsAvailable(t *testing.T) {
	src := NewSushiSwapSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

func TestCurveSource_New(t *testing.T) {
	src := NewCurveSource()
	if src == nil {
		t.Fatal("NewCurveSource should not return nil")
	}
	if src.GetName() != "Curve" {
		t.Errorf("expected name 'Curve', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeDEX {
		t.Error("GetType should return SourceTypeDEX")
	}
}

func TestCurveSource_SupportedPairs(t *testing.T) {
	src := NewCurveSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
	// Verify USDT/USD is supported
	found := false
	for _, p := range pairs {
		if p.Equal(PairUSDTUSD) {
			found = true
			break
		}
	}
	if !found {
		t.Error("USDT/USD should be a supported pair")
	}
}

func TestCurveSource_IsAvailable(t *testing.T) {
	src := NewCurveSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

// ============================================================================
// CEX Price Source Tests
// ============================================================================

func TestBinanceSource_New(t *testing.T) {
	src := NewBinanceSource()
	if src == nil {
		t.Fatal("NewBinanceSource should not return nil")
	}
	if src.GetName() != "Binance" {
		t.Errorf("expected name 'Binance', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeCEX {
		t.Error("GetType should return SourceTypeCEX")
	}
}

func TestBinanceSource_SupportedPairs(t *testing.T) {
	src := NewBinanceSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
	// Verify BTC/USD is supported
	found := false
	for _, p := range pairs {
		if p.Equal(PairBTCUSD) {
			found = true
			break
		}
	}
	if !found {
		t.Error("BTC/USD should be a supported pair")
	}
}

func TestBinanceSource_IsAvailable(t *testing.T) {
	src := NewBinanceSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

func TestCoinbaseSource_New(t *testing.T) {
	src := NewCoinbaseSource()
	if src == nil {
		t.Fatal("NewCoinbaseSource should not return nil")
	}
	if src.GetName() != "Coinbase" {
		t.Errorf("expected name 'Coinbase', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeCEX {
		t.Error("GetType should return SourceTypeCEX")
	}
}

func TestCoinbaseSource_SupportedPairs(t *testing.T) {
	src := NewCoinbaseSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
}

func TestCoinbaseSource_IsAvailable(t *testing.T) {
	src := NewCoinbaseSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

func TestKrakenSource_New(t *testing.T) {
	src := NewKrakenSource()
	if src == nil {
		t.Fatal("NewKrakenSource should not return nil")
	}
	if src.GetName() != "Kraken" {
		t.Errorf("expected name 'Kraken', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeCEX {
		t.Error("GetType should return SourceTypeCEX")
	}
}

func TestKrakenSource_SupportedPairs(t *testing.T) {
	src := NewKrakenSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
}

func TestKrakenSource_IsAvailable(t *testing.T) {
	src := NewKrakenSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

// ============================================================================
// Stablecoin Source Tests
// ============================================================================

func TestStablecoinSource_New(t *testing.T) {
	src := NewStablecoinSource()
	if src == nil {
		t.Fatal("NewStablecoinSource should not return nil")
	}
	if src.GetName() != "Stablecoin-CoinGecko" {
		t.Errorf("expected name 'Stablecoin-CoinGecko', got %s", src.GetName())
	}
	if src.GetType() != SourceTypeStablecoin {
		t.Error("GetType should return SourceTypeStablecoin")
	}
}

func TestStablecoinSource_SupportedPairs(t *testing.T) {
	src := NewStablecoinSource()
	pairs := src.SupportedPairs()
	if len(pairs) == 0 {
		t.Error("should have supported pairs")
	}
	// Verify USDT/USD is supported
	found := false
	for _, p := range pairs {
		if p.Equal(PairUSDTUSD) {
			found = true
			break
		}
	}
	if !found {
		t.Error("USDT/USD should be a supported pair")
	}
}

func TestStablecoinSource_IsAvailable(t *testing.T) {
	src := NewStablecoinSource()
	if !src.IsAvailable() {
		t.Error("should be available initially")
	}
}

// ============================================================================
// DefaultSources Tests
// ============================================================================

func TestDefaultSources(t *testing.T) {
	sources := DefaultSources()
	if len(sources) == 0 {
		t.Error("should have default sources")
	}

	// Verify we have at least one DEX source
	hasDEX := false
	hasCEX := false
	hasStablecoin := false

	for _, src := range sources {
		switch src.GetType() {
		case SourceTypeDEX:
			hasDEX = true
		case SourceTypeCEX:
			hasCEX = true
		case SourceTypeStablecoin:
			hasStablecoin = true
		}
	}

	if !hasDEX {
		t.Error("should have at least one DEX source")
	}
	if !hasCEX {
		t.Error("should have at least one CEX source")
	}
	if !hasStablecoin {
		t.Error("should have at least one stablecoin source")
	}
}
