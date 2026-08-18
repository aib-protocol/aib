// Package oracle implements a multi-source price oracle system.
//
// The oracle collects price data from multiple sources (DEX, CEX, stablecoin peg),
// computes a final price via weighted averaging and outlier filtering, and provides slippage protection.
package oracle

import (
	"errors"
	"fmt"
	"time"
)

// Predefined errors
var (
	// ErrNoPriceSources indicates no price sources are available
	ErrNoPriceSources = errors.New("oracle: no price sources available")

	// ErrPairNotSupported indicates the requested pair is not supported
	ErrPairNotSupported = errors.New("oracle: trading pair not supported")

	// ErrPriceUnavailable indicates price data could not be fetched
	ErrPriceUnavailable = errors.New("oracle: price data unavailable")

	// ErrPriceStale indicates cached price data has expired
	ErrPriceStale = errors.New("oracle: cached price data is stale")

	// ErrSlippageTooHigh indicates slippage exceeds the allowed range
	ErrSlippageTooHigh = errors.New("oracle: slippage exceeds maximum allowed")

	// ErrInsufficientLiquidity indicates insufficient liquidity
	ErrInsufficientLiquidity = errors.New("oracle: insufficient liquidity")

	// ErrOracleNotRunning indicates the oracle is not running
	ErrOracleNotRunning = errors.New("oracle: not running")

	// ErrInvalidConfig indicates an invalid configuration
	ErrInvalidConfig = errors.New("oracle: invalid configuration")
)

// SourceType represents the type of a price source
type SourceType int

const (
	// SourceTypeDEX is a decentralized exchange
	SourceTypeDEX SourceType = iota
	// SourceTypeCEX is a centralized exchange
	SourceTypeCEX
	// SourceTypeStablecoin is a stablecoin peg
	SourceTypeStablecoin
)

// String returns the string representation of a SourceType
func (st SourceType) String() string {
	switch st {
	case SourceTypeDEX:
		return "DEX"
	case SourceTypeCEX:
		return "CEX"
	case SourceTypeStablecoin:
		return "Stablecoin"
	default:
		return fmt.Sprintf("Unknown(%d)", int(st))
	}
}

// TradingPair represents a trading pair
type TradingPair struct {
	// Base is the base asset (e.g. AIB)
	Base string
	// Quote is the quote asset (e.g. USD)
	Quote string
}

// String returns the canonical string form of the pair (e.g. "AIB/USD")
func (tp TradingPair) String() string {
	return tp.Base + "/" + tp.Quote
}

// Equal compares two trading pairs for equality
func (tp TradingPair) Equal(other TradingPair) bool {
	return tp.Base == other.Base && tp.Quote == other.Quote
}

// Predefined trading pairs
var (
	PairAIBUSD  = TradingPair{Base: "AIB", Quote: "USD"}
	PairAIBBTC  = TradingPair{Base: "AIB", Quote: "BTC"}
	PairAIBETH  = TradingPair{Base: "AIB", Quote: "ETH"}
	PairBTCUSD  = TradingPair{Base: "BTC", Quote: "USD"}
	PairETHUSD  = TradingPair{Base: "ETH", Quote: "USD"}
	PairUSDTUSD = TradingPair{Base: "USDT", Quote: "USD"}
	PairUSDCUSD = TradingPair{Base: "USDC", Quote: "USD"}
)

// SupportedPairs returns all supported trading pairs
func SupportedPairs() []TradingPair {
	return []TradingPair{
		PairAIBUSD,
		PairAIBBTC,
		PairAIBETH,
		PairBTCUSD,
		PairETHUSD,
		PairUSDTUSD,
		PairUSDCUSD,
	}
}

// PriceData represents price data from a single price source
type PriceData struct {
	// Pair is the trading pair
	Pair TradingPair

	// Price denominated in the Quote asset
	Price float64

	// Volume24h is the 24h volume (in the Base asset)
	Volume24h float64

	// Timestamp of the price data
	Timestamp time.Time

	// Source is the price source name
	Source string

	// SourceType is the price source type
	SourceType SourceType

	// Bid is the best bid (optional)
	Bid float64

	// Ask is the best ask (optional)
	Ask float64

	// Liquidity is available liquidity in USD (optional)
	Liquidity float64
}

// Spread returns the bid-ask spread percentage
func (pd PriceData) Spread() float64 {
	if pd.Bid <= 0 || pd.Ask <= 0 {
		return 0
	}
	return (pd.Ask - pd.Bid) / pd.Bid * 100
}

// IsValid checks whether the price data is valid
func (pd PriceData) IsValid() bool {
	return pd.Price > 0 && !pd.Timestamp.IsZero() && pd.Source != ""
}

// Age returns the age of the price data
func (pd PriceData) Age() time.Duration {
	return time.Since(pd.Timestamp)
}

// AggregatedPrice represents the final aggregated price
type AggregatedPrice struct {
	// Pair is the trading pair
	Pair TradingPair

	// Price is the final aggregated price
	Price float64

	// Sources is the number of sources in the aggregation
	Sources int

	// TotalVolume is the total volume across all sources
	TotalVolume float64

	// Confidence (0-1), based on source count and consistency
	Confidence float64

	// Timestamp of the aggregation
	Timestamp time.Time

	// MinPrice is the lowest price across sources
	MinPrice float64

	// MaxPrice is the highest price across sources
	MaxPrice float64

	// Deviation is the standard deviation across sources
	Deviation float64

	// RawPrices is the raw price data used in the aggregation
	RawPrices []PriceData
}

// PriceSource is the core interface for a price source.
// Every price source (DEX, CEX, stablecoin peg) must implement this interface.
type PriceSource interface {
	// FetchPrice fetches the price data for the given pair from this source.
	// Returns an error if the pair is unsupported or data is unavailable.
	FetchPrice(pair TradingPair) (PriceData, error)

	// IsAvailable checks whether the source is currently available.
	// Useful for health checks and failover decisions.
	IsAvailable() bool

	// GetName returns the source name (e.g. "Binance", "Uniswap").
	GetName() string

	// GetType returns the price source type.
	GetType() SourceType

	// SupportedPairs returns the pairs supported by this source.
	SupportedPairs() []TradingPair
}

// OracleConfig holds the price oracle configuration parameters
type OracleConfig struct {
	// RefreshInterval is the auto-refresh interval
	RefreshInterval time.Duration

	// CacheTTL is the cache time-to-live
	CacheTTL time.Duration

	// DeviationThreshold is the outlier filtering threshold (percent, e.g. 5.0 = 5%)
	// Prices deviating from the median beyond this threshold are dropped
	DeviationThreshold float64

	// MinSources is the minimum number of sources required for aggregation
	MinSources int

	// MaxSlippage is the maximum allowed slippage percentage
	MaxSlippage float64

	// StaleThreshold is the age at which price data is considered stale
	StaleThreshold time.Duration
}

// DefaultConfig returns the default oracle configuration
func DefaultConfig() OracleConfig {
	return OracleConfig{
		RefreshInterval:    30 * time.Second,
		CacheTTL:           60 * time.Second,
		DeviationThreshold: 5.0,
		MinSources:         1,
		MaxSlippage:        3.0,
		StaleThreshold:     5 * time.Minute,
	}
}

// Validate checks whether the configuration is valid
func (c OracleConfig) Validate() error {
	if c.RefreshInterval <= 0 {
		return fmt.Errorf("%w: refresh interval must be positive", ErrInvalidConfig)
	}
	if c.CacheTTL <= 0 {
		return fmt.Errorf("%w: cache TTL must be positive", ErrInvalidConfig)
	}
	if c.DeviationThreshold <= 0 || c.DeviationThreshold >= 100 {
		return fmt.Errorf("%w: deviation threshold must be between 0 and 100", ErrInvalidConfig)
	}
	if c.MinSources < 1 {
		return fmt.Errorf("%w: min sources must be at least 1", ErrInvalidConfig)
	}
	if c.MaxSlippage <= 0 || c.MaxSlippage >= 100 {
		return fmt.Errorf("%w: max slippage must be between 0 and 100", ErrInvalidConfig)
	}
	if c.StaleThreshold <= 0 {
		return fmt.Errorf("%w: stale threshold must be positive", ErrInvalidConfig)
	}
	return nil
}

// SlippageResult represents the result of a slippage calculation
type SlippageResult struct {
	// InputAmount is the input quantity
	InputAmount float64

	// ExpectedOutput is the output with zero slippage
	ExpectedOutput float64

	// ActualOutput is the output after slippage
	ActualOutput float64

	// SlippagePercent is the slippage percentage
	SlippagePercent float64

	// PriceImpact is the price impact percentage
	PriceImpact float64

	// MinOutput is the minimum output satisfying the max slippage limit
	MinOutput float64

	// Pair is the trading pair
	Pair TradingPair

	// Acceptable indicates whether it is within the acceptable range
	Acceptable bool
}

// cacheEntry is a price cache entry (internal use)
type cacheEntry struct {
	// price is the aggregated price
	price AggregatedPrice

	// expiresAt is the expiration time
	expiresAt time.Time
}

// isExpired checks whether the cache entry has expired
func (ce cacheEntry) isExpired() bool {
	return time.Now().After(ce.expiresAt)
}

// priceHistoryEntry records historical price data (for deviation detection)
type priceHistoryEntry struct {
	Price     float64
	Timestamp time.Time
}

// AlertHandler handles price deviation alerts
type AlertHandler interface {
	// OnPriceDeviation is called when price deviation exceeds the threshold
	OnPriceDeviation(pair TradingPair, oldPrice, newPrice float64, deviation float64)
}
