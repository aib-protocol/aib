// Package oracle 实现多源价格预言机系统。
//
// 预言机从多个价格源（DEX、CEX、稳定币锚定）收集价格数据，
// 通过加权平均和离群值过滤计算最终价格，并提供滑点保护功能。
package oracle

import (
	"errors"
	"fmt"
	"time"
)

// 预定义错误
var (
	// ErrNoPriceSources 表示没有可用的价格源
	ErrNoPriceSources = errors.New("oracle: no price sources available")

	// ErrPairNotSupported 表示不支持请求的交易对
	ErrPairNotSupported = errors.New("oracle: trading pair not supported")

	// ErrPriceUnavailable 表示无法获取价格数据
	ErrPriceUnavailable = errors.New("oracle: price data unavailable")

	// ErrPriceStale 表示缓存中的价格数据已过期
	ErrPriceStale = errors.New("oracle: cached price data is stale")

	// ErrSlippageTooHigh 表示滑点超出允许范围
	ErrSlippageTooHigh = errors.New("oracle: slippage exceeds maximum allowed")

	// ErrInsufficientLiquidity 表示流动性不足
	ErrInsufficientLiquidity = errors.New("oracle: insufficient liquidity")

	// ErrOracleNotRunning 表示预言机未启动
	ErrOracleNotRunning = errors.New("oracle: not running")

	// ErrInvalidConfig 表示配置无效
	ErrInvalidConfig = errors.New("oracle: invalid configuration")
)

// SourceType 表示价格源的类型
type SourceType int

const (
	// SourceTypeDEX 去中心化交易所
	SourceTypeDEX SourceType = iota
	// SourceTypeCEX 中心化交易所
	SourceTypeCEX
	// SourceTypeStablecoin 稳定币锚定
	SourceTypeStablecoin
)

// String 返回 SourceType 的字符串表示
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

// TradingPair 表示一个交易对
type TradingPair struct {
	// Base 基础资产（如 AIB）
	Base string
	// Quote 报价资产（如 USD）
	Quote string
}

// String 返回交易对的标准字符串表示（如 "AIB/USD"）
func (tp TradingPair) String() string {
	return tp.Base + "/" + tp.Quote
}

// Equal 比较两个交易对是否相同
func (tp TradingPair) Equal(other TradingPair) bool {
	return tp.Base == other.Base && tp.Quote == other.Quote
}

// 预定义的交易对
var (
	PairAIBUSD  = TradingPair{Base: "AIB", Quote: "USD"}
	PairAIBBTC  = TradingPair{Base: "AIB", Quote: "BTC"}
	PairAIBETH  = TradingPair{Base: "AIB", Quote: "ETH"}
	PairBTCUSD  = TradingPair{Base: "BTC", Quote: "USD"}
	PairETHUSD  = TradingPair{Base: "ETH", Quote: "USD"}
	PairUSDTUSD = TradingPair{Base: "USDT", Quote: "USD"}
	PairUSDCUSD = TradingPair{Base: "USDC", Quote: "USD"}
)

// SupportedPairs 返回所有支持的交易对
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

// PriceData 表示来自单个价格源的价格数据
type PriceData struct {
	// Pair 交易对
	Pair TradingPair

	// Price 价格（以 Quote 资产计价）
	Price float64

	// Volume24h 24小时成交量（以 Base 资产计量）
	Volume24h float64

	// Timestamp 价格数据的时间戳
	Timestamp time.Time

	// Source 价格来源名称
	Source string

	// SourceType 价格源类型
	SourceType SourceType

	// Bid 买一价（可选）
	Bid float64

	// Ask 卖一价（可选）
	Ask float64

	// Liquidity 可用流动性（以 USD 计价，可选）
	Liquidity float64
}

// Spread 返回买卖价差百分比
func (pd PriceData) Spread() float64 {
	if pd.Bid <= 0 || pd.Ask <= 0 {
		return 0
	}
	return (pd.Ask - pd.Bid) / pd.Bid * 100
}

// IsValid 检查价格数据是否有效
func (pd PriceData) IsValid() bool {
	return pd.Price > 0 && !pd.Timestamp.IsZero() && pd.Source != ""
}

// Age 返回价格数据的年龄
func (pd PriceData) Age() time.Duration {
	return time.Since(pd.Timestamp)
}

// AggregatedPrice 表示经过聚合计算的最终价格
type AggregatedPrice struct {
	// Pair 交易对
	Pair TradingPair

	// Price 聚合后的最终价格
	Price float64

	// Sources 参与聚合的数据源数量
	Sources int

	// TotalVolume 所有源的总成交量
	TotalVolume float64

	// Confidence 置信度（0-1之间，基于数据源数量和一致性）
	Confidence float64

	// Timestamp 聚合时间
	Timestamp time.Time

	// MinPrice 各源中的最低价
	MinPrice float64

	// MaxPrice 各源中的最高价
	MaxPrice float64

	// Deviation 各源之间的标准差
	Deviation float64

	// RawPrices 参与聚合的原始价格数据
	RawPrices []PriceData
}

// PriceSource 是价格源的核心接口。
// 每个价格源（DEX、CEX、稳定币锚定）都必须实现此接口。
type PriceSource interface {
	// FetchPrice 从该价格源获取指定交易对的价格数据。
	// 如果交易对不支持或数据不可用，返回相应错误。
	FetchPrice(pair TradingPair) (PriceData, error)

	// IsAvailable 检查该价格源当前是否可用。
	// 可用于健康检查和故障转移判断。
	IsAvailable() bool

	// GetName 返回价格源的名称（如 "Binance"、"Uniswap"）。
	GetName() string

	// GetType 返回价格源的类型。
	GetType() SourceType

	// SupportedPairs 返回该价格源支持的交易对列表。
	SupportedPairs() []TradingPair
}

// OracleConfig 包含价格预言机的配置参数
type OracleConfig struct {
	// RefreshInterval 自动刷新间隔
	RefreshInterval time.Duration

	// CacheTTL 缓存的生存时间
	CacheTTL time.Duration

	// DeviationThreshold 离群值过滤阈值（百分比，如 5.0 表示 5%）
	// 偏离中位数超过此阈值的价格将被剔除
	DeviationThreshold float64

	// MinSources 计算聚合价格所需的最少数据源数量
	MinSources int

	// MaxSlippage 允许的最大滑点百分比
	MaxSlippage float64

	// StaleThreshold 判定价格数据过期的时间阈值
	StaleThreshold time.Duration
}

// DefaultConfig 返回默认的预言机配置
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

// Validate 验证配置是否有效
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

// SlippageResult 表示滑点计算的结果
type SlippageResult struct {
	// InputAmount 输入数量
	InputAmount float64

	// ExpectedOutput 无滑点时的预期输出
	ExpectedOutput float64

	// ActualOutput 考虑滑点后的实际输出
	ActualOutput float64

	// SlippagePercent 滑点百分比
	SlippagePercent float64

	// PriceImpact 价格影响百分比
	PriceImpact float64

	// MinOutput 满足最大滑点限制时的最小输出
	MinOutput float64

	// Pair 交易对
	Pair TradingPair

	// Acceptable 是否在可接受范围内
	Acceptable bool
}

// cacheEntry 价格缓存条目（内部使用）
type cacheEntry struct {
	// price 聚合后的价格
	price AggregatedPrice

	// expiresAt 过期时间
	expiresAt time.Time
}

// isExpired 检查缓存条目是否已过期
func (ce cacheEntry) isExpired() bool {
	return time.Now().After(ce.expiresAt)
}

// priceHistoryEntry 记录历史价格数据（用于偏差检测）
type priceHistoryEntry struct {
	Price     float64
	Timestamp time.Time
}

// AlertHandler 价格偏差告警处理器
type AlertHandler interface {
	// OnPriceDeviation 当价格偏差超过阈值时调用
	OnPriceDeviation(pair TradingPair, oldPrice, newPrice float64, deviation float64)
}
