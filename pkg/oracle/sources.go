package oracle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ===========================================================================
// 通用 HTTP 价格源基础设施
// ===========================================================================

// httpClient 是所有价格源共享的 HTTP 客户端，带超时配置
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// fetchJSON 从指定 URL 获取 JSON 数据并解码到 target。
// 这是所有 HTTP 价格源的通用方法。
func fetchJSON(url string, target interface{}) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	return nil
}

// ===========================================================================
// Binance 价格源
// ===========================================================================

// BinanceSource 从 Binance API 获取价格数据。
// API 端点: https://api.binance.com/api/v3/ticker/24hr
type BinanceSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping 将标准交易对映射为 Binance 交易对符号
	pairMapping map[string]string
}

// binanceTickerResponse Binance 24小时价格变动统计 API 响应
type binanceTickerResponse struct {
	Symbol    string `json:"symbol"`
	LastPrice string `json:"lastPrice"`
	BidPrice  string `json:"bidPrice"`
	AskPrice  string `json:"askPrice"`
	Volume    string `json:"volume"`
}

// NewBinanceSource 创建 Binance 价格源实例
func NewBinanceSource() *BinanceSource {
	return &BinanceSource{
		available: true,
		baseURL:   "https://api.binance.com",
		pairMapping: map[string]string{
			"BTC/USD":  "BTCUSDT",
			"ETH/USD":  "ETHUSDT",
			"AIB/USD":  "AIBUSDT",
			"AIB/BTC":  "AIBBTC",
			"AIB/ETH":  "AIBETH",
			"USDT/USD": "USDTUSD",
			"USDC/USD": "USDCUSDT",
		},
	}
}

// FetchPrice 从 Binance 获取指定交易对的价格
func (s *BinanceSource) FetchPrice(pair TradingPair) (PriceData, error) {
	symbol, ok := s.pairMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Binance", ErrPairNotSupported, pair)
	}

	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", s.baseURL, symbol)

	var resp binanceTickerResponse
	if err := fetchJSON(url, &resp); err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Binance: %v", ErrPriceUnavailable, err)
	}

	price, _ := strconv.ParseFloat(resp.LastPrice, 64)
	bid, _ := strconv.ParseFloat(resp.BidPrice, 64)
	ask, _ := strconv.ParseFloat(resp.AskPrice, 64)
	volume, _ := strconv.ParseFloat(resp.Volume, 64)

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Volume24h:  volume,
		Timestamp:  time.Now(),
		Source:     "Binance",
		SourceType: SourceTypeCEX,
		Bid:        bid,
		Ask:        ask,
	}, nil
}

// IsAvailable 检查 Binance 源是否可用
func (s *BinanceSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *BinanceSource) GetName() string {
	return "Binance"
}

// GetType 返回价格源类型
func (s *BinanceSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs 返回 Binance 支持的交易对
func (s *BinanceSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD, PairAIBBTC, PairAIBETH,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Coinbase 价格源
// ===========================================================================

// CoinbaseSource 从 Coinbase API 获取价格数据。
// API 端点: https://api.coinbase.com/v2/prices/{pair}/spot
type CoinbaseSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping 将标准交易对映射为 Coinbase API 格式
	pairMapping map[string]string
}

// coinbasePriceResponse Coinbase 价格 API 响应
type coinbasePriceResponse struct {
	Data struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"data"`
}

// NewCoinbaseSource 创建 Coinbase 价格源实例
func NewCoinbaseSource() *CoinbaseSource {
	return &CoinbaseSource{
		available: true,
		baseURL:   "https://api.coinbase.com",
		pairMapping: map[string]string{
			"BTC/USD":  "BTC-USD",
			"ETH/USD":  "ETH-USD",
			"AIB/USD":  "AIB-USD",
			"USDT/USD": "USDT-USD",
			"USDC/USD": "USDC-USD",
		},
	}
}

// FetchPrice 从 Coinbase 获取指定交易对的价格
func (s *CoinbaseSource) FetchPrice(pair TradingPair) (PriceData, error) {
	cbPair, ok := s.pairMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Coinbase", ErrPairNotSupported, pair)
	}

	url := fmt.Sprintf("%s/v2/prices/%s/spot", s.baseURL, cbPair)

	var resp coinbasePriceResponse
	if err := fetchJSON(url, &resp); err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Coinbase: %v", ErrPriceUnavailable, err)
	}

	price, _ := strconv.ParseFloat(resp.Data.Amount, 64)

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Timestamp:  time.Now(),
		Source:     "Coinbase",
		SourceType: SourceTypeCEX,
	}, nil
}

// IsAvailable 检查 Coinbase 源是否可用
func (s *CoinbaseSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *CoinbaseSource) GetName() string {
	return "Coinbase"
}

// GetType 返回价格源类型
func (s *CoinbaseSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs 返回 Coinbase 支持的交易对
func (s *CoinbaseSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Kraken 价格源
// ===========================================================================

// KrakenSource 从 Kraken API 获取价格数据。
// API 端点: https://api.kraken.com/0/public/Ticker
type KrakenSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping 将标准交易对映射为 Kraken API 格式
	pairMapping map[string]string
}

// krakenTickerResponse Kraken Ticker API 响应
type krakenTickerResponse struct {
	Error  []string                          `json:"error"`
	Result map[string]krakenTickerPairResult `json:"result"`
}

// krakenTickerPairResult Kraken 单个交易对的 Ticker 数据
type krakenTickerPairResult struct {
	// Ask [price, wholeLotVolume, lotVolume]
	Ask []string `json:"a"`
	// Bid [price, wholeLotVolume, lotVolume]
	Bid []string `json:"b"`
	// Last trade [price, lotVolume]
	Last []string `json:"c"`
	// Volume [today, last24h]
	Volume []string `json:"v"`
}

// NewKrakenSource 创建 Kraken 价格源实例
func NewKrakenSource() *KrakenSource {
	return &KrakenSource{
		available: true,
		baseURL:   "https://api.kraken.com",
		pairMapping: map[string]string{
			"BTC/USD":  "XXBTZUSD",
			"ETH/USD":  "XETHZUSD",
			"AIB/USD":  "AIBUSD",
			"USDT/USD": "USDTZUSD",
			"USDC/USD": "USDCUSD",
		},
	}
}

// FetchPrice 从 Kraken 获取指定交易对的价格
func (s *KrakenSource) FetchPrice(pair TradingPair) (PriceData, error) {
	krakenPair, ok := s.pairMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Kraken", ErrPairNotSupported, pair)
	}

	url := fmt.Sprintf("%s/0/public/Ticker?pair=%s", s.baseURL, krakenPair)

	var resp krakenTickerResponse
	if err := fetchJSON(url, &resp); err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Kraken: %v", ErrPriceUnavailable, err)
	}

	if len(resp.Error) > 0 {
		return PriceData{}, fmt.Errorf("%w: Kraken API error: %v", ErrPriceUnavailable, resp.Error)
	}

	// 遍历 result 获取第一个交易对数据
	var tickerData krakenTickerPairResult
	for _, v := range resp.Result {
		tickerData = v
		break
	}

	price, _ := strconv.ParseFloat(safeIndex(tickerData.Last, 0), 64)
	ask, _ := strconv.ParseFloat(safeIndex(tickerData.Ask, 0), 64)
	bid, _ := strconv.ParseFloat(safeIndex(tickerData.Bid, 0), 64)
	volume, _ := strconv.ParseFloat(safeIndex(tickerData.Volume, 1), 64) // last 24h

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Volume24h:  volume,
		Timestamp:  time.Now(),
		Source:     "Kraken",
		SourceType: SourceTypeCEX,
		Bid:        bid,
		Ask:        ask,
	}, nil
}

// safeIndex 安全地获取字符串切片中的元素，越界时返回空字符串
func safeIndex(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// IsAvailable 检查 Kraken 源是否可用
func (s *KrakenSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *KrakenSource) GetName() string {
	return "Kraken"
}

// GetType 返回价格源类型
func (s *KrakenSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs 返回 Kraken 支持的交易对
func (s *KrakenSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Uniswap V3 价格源（链上 DEX）
// ===========================================================================

// UniswapSource 从 Uniswap V3 的 The Graph 子图获取价格数据。
// 使用 The Graph 的公开子图 API 查询池子价格。
type UniswapSource struct {
	mu        sync.RWMutex
	available bool
	subgraph  string
	// poolMapping 将交易对映射为 Uniswap 池子地址
	poolMapping map[string]string
}

// uniswapGraphResponse The Graph 子图查询响应
type uniswapGraphResponse struct {
	Data struct {
		Pool *struct {
			Token0Price  string `json:"token0Price"`
			Token1Price  string `json:"token1Price"`
			VolumeUSD    string `json:"volumeUSD"`
			TotalValueLockedUSD string `json:"totalValueLockedUSD"`
		} `json:"pool"`
	} `json:"data"`
}

// NewUniswapSource 创建 Uniswap 价格源实例
func NewUniswapSource() *UniswapSource {
	return &UniswapSource{
		available: true,
		subgraph:  "https://api.thegraph.com/subgraphs/name/uniswap/uniswap-v3",
		poolMapping: map[string]string{
			"ETH/USD":  "0x8ad599c3a0ff1de082011efddc58f1908eb6e6d8", // ETH/USDC 0.3%
			"BTC/USD":  "0x99ac8ca7087fa4a2a1fb6357269965a2014abc35", // WBTC/USDC 0.3%
			"USDT/USD": "0x3416cf6c708da44db2624d63ea0aaef7113527c6", // USDT/USDC 0.01%
			"USDC/USD": "0x3416cf6c708da44db2624d63ea0aaef7113527c6", // USDC reference
		},
	}
}

// FetchPrice 从 Uniswap V3 子图获取指定交易对的价格
func (s *UniswapSource) FetchPrice(pair TradingPair) (PriceData, error) {
	poolAddr, ok := s.poolMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Uniswap", ErrPairNotSupported, pair)
	}

	query := fmt.Sprintf(`{"query":"{ pool(id: \"%s\") { token0Price token1Price volumeUSD totalValueLockedUSD } }"}`, poolAddr)

	req, err := http.NewRequest("POST", s.subgraph, nil)
	if err != nil {
		return PriceData{}, fmt.Errorf("%w: Uniswap: %v", ErrPriceUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 使用 io.NopCloser 将 query 包装为 request body
	req.Body = io.NopCloser(stringReader(query))

	resp, err := httpClient.Do(req)
	if err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Uniswap: %v", ErrPriceUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PriceData{}, fmt.Errorf("%w: Uniswap read error: %v", ErrPriceUnavailable, err)
	}

	var graphResp uniswapGraphResponse
	if err := json.Unmarshal(body, &graphResp); err != nil {
		return PriceData{}, fmt.Errorf("%w: Uniswap JSON error: %v", ErrPriceUnavailable, err)
	}

	if graphResp.Data.Pool == nil {
		return PriceData{}, fmt.Errorf("%w: Uniswap pool not found for %s", ErrPriceUnavailable, pair)
	}

	price, _ := strconv.ParseFloat(graphResp.Data.Pool.Token0Price, 64)
	volume, _ := strconv.ParseFloat(graphResp.Data.Pool.VolumeUSD, 64)
	liquidity, _ := strconv.ParseFloat(graphResp.Data.Pool.TotalValueLockedUSD, 64)

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Volume24h:  volume,
		Timestamp:  time.Now(),
		Source:     "Uniswap V3",
		SourceType: SourceTypeDEX,
		Liquidity:  liquidity,
	}, nil
}

// stringReader 辅助函数，返回字符串的 io.Reader
type stringReaderImpl struct {
	s string
	i int
}

func stringReader(s string) *stringReaderImpl {
	return &stringReaderImpl{s: s}
}

func (r *stringReaderImpl) Read(p []byte) (n int, err error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n = copy(p, r.s[r.i:])
	r.i += n
	return
}

// IsAvailable 检查 Uniswap 源是否可用
func (s *UniswapSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *UniswapSource) GetName() string {
	return "Uniswap V3"
}

// GetType 返回价格源类型
func (s *UniswapSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs 返回 Uniswap 支持的交易对
func (s *UniswapSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairETHUSD, PairBTCUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// SushiSwap 价格源（链上 DEX）
// ===========================================================================

// SushiSwapSource 从 SushiSwap 的 The Graph 子图获取价格数据
type SushiSwapSource struct {
	mu        sync.RWMutex
	available bool
	subgraph  string
	// pairMapping 将交易对映射为 SushiSwap 池子地址
	pairMapping map[string]string
}

// sushiGraphResponse SushiSwap Graph 子图查询响应
type sushiGraphResponse struct {
	Data struct {
		Pair *struct {
			Token0Price string `json:"token0Price"`
			Token1Price string `json:"token1Price"`
			VolumeUSD   string `json:"volumeUSD"`
			ReserveUSD  string `json:"reserveUSD"`
		} `json:"pair"`
	} `json:"data"`
}

// NewSushiSwapSource 创建 SushiSwap 价格源实例
func NewSushiSwapSource() *SushiSwapSource {
	return &SushiSwapSource{
		available: true,
		subgraph:  "https://api.thegraph.com/subgraphs/name/sushiswap/exchange",
		pairMapping: map[string]string{
			"ETH/USD": "0x397ff1542f962076d0bfe58ea045ffa2d347aca0", // ETH/USDC
			"BTC/USD": "0xceff51756c56ceffca006cd410b03ffc46dd3a58", // WBTC/WETH (需要换算)
		},
	}
}

// FetchPrice 从 SushiSwap 子图获取指定交易对的价格
func (s *SushiSwapSource) FetchPrice(pair TradingPair) (PriceData, error) {
	pairAddr, ok := s.pairMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on SushiSwap", ErrPairNotSupported, pair)
	}

	query := fmt.Sprintf(`{"query":"{ pair(id: \"%s\") { token0Price token1Price volumeUSD reserveUSD } }"}`, pairAddr)

	req, err := http.NewRequest("POST", s.subgraph, nil)
	if err != nil {
		return PriceData{}, fmt.Errorf("%w: SushiSwap: %v", ErrPriceUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(stringReader(query))

	resp, err := httpClient.Do(req)
	if err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: SushiSwap: %v", ErrPriceUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PriceData{}, fmt.Errorf("%w: SushiSwap read error: %v", ErrPriceUnavailable, err)
	}

	var graphResp sushiGraphResponse
	if err := json.Unmarshal(body, &graphResp); err != nil {
		return PriceData{}, fmt.Errorf("%w: SushiSwap JSON error: %v", ErrPriceUnavailable, err)
	}

	if graphResp.Data.Pair == nil {
		return PriceData{}, fmt.Errorf("%w: SushiSwap pair not found for %s", ErrPriceUnavailable, pair)
	}

	price, _ := strconv.ParseFloat(graphResp.Data.Pair.Token0Price, 64)
	volume, _ := strconv.ParseFloat(graphResp.Data.Pair.VolumeUSD, 64)
	liquidity, _ := strconv.ParseFloat(graphResp.Data.Pair.ReserveUSD, 64)

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Volume24h:  volume,
		Timestamp:  time.Now(),
		Source:     "SushiSwap",
		SourceType: SourceTypeDEX,
		Liquidity:  liquidity,
	}, nil
}

// IsAvailable 检查 SushiSwap 源是否可用
func (s *SushiSwapSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *SushiSwapSource) GetName() string {
	return "SushiSwap"
}

// GetType 返回价格源类型
func (s *SushiSwapSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs 返回 SushiSwap 支持的交易对
func (s *SushiSwapSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairETHUSD, PairBTCUSD}
}

// ===========================================================================
// Curve Finance 价格源（链上 DEX，专注稳定币）
// ===========================================================================

// CurveSource 从 Curve Finance API 获取价格数据。
// Curve 以稳定币交易对和低滑点闻名。
type CurveSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// poolMapping 将交易对映射为 Curve 池子标识
	poolMapping map[string]string
}

// curvePoolResponse Curve API 响应
type curvePoolResponse struct {
	Data struct {
		PoolData []struct {
			ID               string  `json:"id"`
			VirtualPrice     string  `json:"virtualPrice"`
			TotalLiquidity   string  `json:"totalLiquidity"`
			USDTotal         float64 `json:"usdTotal"`
			Coins            []struct {
				Address  string  `json:"address"`
				Symbol   string  `json:"symbol"`
				USDPrice float64 `json:"usdPrice"`
			} `json:"coins"`
		} `json:"poolData"`
	} `json:"data"`
}

// NewCurveSource 创建 Curve 价格源实例
func NewCurveSource() *CurveSource {
	return &CurveSource{
		available: true,
		baseURL:   "https://api.curve.fi/api",
		poolMapping: map[string]string{
			"USDT/USD": "3pool",
			"USDC/USD": "3pool",
		},
	}
}

// FetchPrice 从 Curve 获取指定交易对的价格
func (s *CurveSource) FetchPrice(pair TradingPair) (PriceData, error) {
	_, ok := s.poolMapping[pair.String()]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Curve", ErrPairNotSupported, pair)
	}

	url := fmt.Sprintf("%s/getPools/ethereum/main", s.baseURL)

	var resp curvePoolResponse
	if err := fetchJSON(url, &resp); err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Curve: %v", ErrPriceUnavailable, err)
	}

	// 在池子的币种中查找目标资产的价格
	targetSymbol := pair.Base
	var price float64
	var liquidity float64

	for _, pool := range resp.Data.PoolData {
		for _, coin := range pool.Coins {
			if coin.Symbol == targetSymbol {
				price = coin.USDPrice
				liquidity = pool.USDTotal
				break
			}
		}
		if price > 0 {
			break
		}
	}

	if price <= 0 {
		// 稳定币默认价格为 1.0（仅在 API 无法返回时使用）
		return PriceData{}, fmt.Errorf("%w: Curve: price not found for %s", ErrPriceUnavailable, pair)
	}

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      price,
		Timestamp:  time.Now(),
		Source:     "Curve",
		SourceType: SourceTypeDEX,
		Liquidity:  liquidity,
	}, nil
}

// IsAvailable 检查 Curve 源是否可用
func (s *CurveSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *CurveSource) GetName() string {
	return "Curve"
}

// GetType 返回价格源类型
func (s *CurveSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs 返回 Curve 支持的交易对
func (s *CurveSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairUSDTUSD, PairUSDCUSD}
}

// ===========================================================================
// 稳定币锚定价格源
// ===========================================================================

// StablecoinSource 提供稳定币（USDT、USDC、DAI）的锚定价格。
// 从 CoinGecko API 获取实际的稳定币价格偏离数据。
type StablecoinSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// coinIDs 将稳定币符号映射为 CoinGecko 的 coin ID
	coinIDs map[string]string
}

// coingeckoResponse CoinGecko 简单价格 API 响应
type coingeckoResponse map[string]struct {
	USD       float64 `json:"usd"`
	Volume24h float64 `json:"usd_24h_vol"`
}

// NewStablecoinSource 创建稳定币锚定价格源实例
func NewStablecoinSource() *StablecoinSource {
	return &StablecoinSource{
		available: true,
		baseURL:   "https://api.coingecko.com/api/v3",
		coinIDs: map[string]string{
			"USDT": "tether",
			"USDC": "usd-coin",
			"DAI":  "dai",
		},
	}
}

// FetchPrice 从 CoinGecko 获取稳定币的实际价格
func (s *StablecoinSource) FetchPrice(pair TradingPair) (PriceData, error) {
	if pair.Quote != "USD" {
		return PriceData{}, fmt.Errorf("%w: %s on Stablecoin source (only USD pairs)", ErrPairNotSupported, pair)
	}

	coinID, ok := s.coinIDs[pair.Base]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: %s on Stablecoin source", ErrPairNotSupported, pair)
	}

	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=usd&include_24hr_vol=true", s.baseURL, coinID)

	var resp coingeckoResponse
	if err := fetchJSON(url, &resp); err != nil {
		s.mu.Lock()
		s.available = false
		s.mu.Unlock()
		return PriceData{}, fmt.Errorf("%w: Stablecoin (CoinGecko): %v", ErrPriceUnavailable, err)
	}

	data, ok := resp[coinID]
	if !ok {
		return PriceData{}, fmt.Errorf("%w: CoinGecko: coin %s not in response", ErrPriceUnavailable, coinID)
	}

	s.mu.Lock()
	s.available = true
	s.mu.Unlock()

	return PriceData{
		Pair:       pair,
		Price:      data.USD,
		Volume24h:  data.Volume24h,
		Timestamp:  time.Now(),
		Source:     "Stablecoin-CoinGecko",
		SourceType: SourceTypeStablecoin,
	}, nil
}

// IsAvailable 检查稳定币价格源是否可用
func (s *StablecoinSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName 返回价格源名称
func (s *StablecoinSource) GetName() string {
	return "Stablecoin-CoinGecko"
}

// GetType 返回价格源类型
func (s *StablecoinSource) GetType() SourceType {
	return SourceTypeStablecoin
}

// SupportedPairs 返回稳定币源支持的交易对
func (s *StablecoinSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairUSDTUSD, PairUSDCUSD}
}

// ===========================================================================
// 工厂函数
// ===========================================================================

// DefaultSources 创建并返回所有默认价格源的列表。
// 包括所有 CEX、DEX 和稳定币锚定源。
func DefaultSources() []PriceSource {
	return []PriceSource{
		// CEX 价格源
		NewBinanceSource(),
		NewCoinbaseSource(),
		NewKrakenSource(),
		// DEX 价格源
		NewUniswapSource(),
		NewSushiSwapSource(),
		NewCurveSource(),
		// 稳定币锚定
		NewStablecoinSource(),
	}
}
