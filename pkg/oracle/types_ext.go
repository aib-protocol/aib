package oracle

import "time"

// CacheStats holds cache statistics
type CacheStats struct {
	// Hits is the number of cache hits
	Hits int64
	// Misses is the number of cache misses
	Misses int64
	// HitRate is the cache hit rate (percentage)
	HitRate float64
	// TotalRequests is the total number of requests
	TotalRequests int64
}

// HitRatePercentage returns the hit rate percentage
func (cs *CacheStats) HitRatePercentage() float64 {
	if cs.TotalRequests == 0 {
		return 0
	}
	return float64(cs.Hits) / float64(cs.TotalRequests) * 100
}

// AlertLevel is the alert level
type AlertLevel int

const (
	// AlertInfo is the info level
	AlertInfo AlertLevel = iota
	// AlertWarning is the warning level
	AlertWarning
	// AlertCritical is the critical level
	AlertCritical
)

// String returns the string representation of the alert level
func (al AlertLevel) String() string {
	switch al {
	case AlertInfo:
		return "INFO"
	case AlertWarning:
		return "WARNING"
	case AlertCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// PriceAlert is a price deviation alert (used for callbacks)
type PriceAlert struct {
	// Pair is the trading pair
	Pair TradingPair
	// Level is the alert level
	Level AlertLevel
	// CurrentPrice is the current price
	CurrentPrice float64
	// ReferencePrice is the reference price (historical average)
	ReferencePrice float64
	// Deviation is the deviation percentage
	Deviation float64
	// Timestamp is the alert timestamp
	Timestamp time.Time
	// Message is the alert message
	Message string
}
