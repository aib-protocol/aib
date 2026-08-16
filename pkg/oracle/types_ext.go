package oracle

import "time"

// CacheStats 缓存统计信息
type CacheStats struct {
	// Hits 缓存命中次数
	Hits int64
	// Misses 缓存未命中次数
	Misses int64
	// HitRate 缓存命中率（百分比）
	HitRate float64
	// TotalRequests 总请求数
	TotalRequests int64
}

// HitRatePercentage 返回命中率百分比
func (cs *CacheStats) HitRatePercentage() float64 {
	if cs.TotalRequests == 0 {
		return 0
	}
	return float64(cs.Hits) / float64(cs.TotalRequests) * 100
}

// AlertLevel 告警级别
type AlertLevel int

const (
	// AlertInfo 信息级别
	AlertInfo AlertLevel = iota
	// AlertWarning 警告级别
	AlertWarning
	// AlertCritical 严重级别
	AlertCritical
)

// String 返回告警级别的字符串表示
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

// PriceAlert 价格偏差告警（用于回调）
type PriceAlert struct {
	// Pair 交易对
	Pair TradingPair
	// Level 告警级别
	Level AlertLevel
	// CurrentPrice 当前价格
	CurrentPrice float64
	// ReferencePrice 参考价格（历史平均）
	ReferencePrice float64
	// Deviation 偏差百分比
	Deviation float64
	// Timestamp 告警时间戳
	Timestamp time.Time
	// Message 告警消息
	Message string
}
