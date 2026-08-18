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
// Common HTTP price source infrastructure
// ===========================================================================

// httpClient is shared by all price sources, with a timeout configured
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// fetchJSON fetches JSON from the given URL and decodes it into target.
// This is a common method for all HTTP price sources.
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
// Binance price source
// ===========================================================================

// BinanceSource fetches price data from the Binance API.
// API endpoint: https://api.binance.com/api/v3/ticker/24hr
type BinanceSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping maps standard pairs to Binance symbols
	pairMapping map[string]string
}

// binanceTickerResponse is the Binance 24h ticker statistics API response
type binanceTickerResponse struct {
	Symbol    string `json:"symbol"`
	LastPrice string `json:"lastPrice"`
	BidPrice  string `json:"bidPrice"`
	AskPrice  string `json:"askPrice"`
	Volume    string `json:"volume"`
}

// NewBinanceSource creates a Binance price source instance
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

// FetchPrice fetches the price for the given pair from Binance
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

// IsAvailable checks whether the Binance source is available
func (s *BinanceSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *BinanceSource) GetName() string {
	return "Binance"
}

// GetType returns the price source type
func (s *BinanceSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs returns the pairs supported by Binance
func (s *BinanceSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD, PairAIBBTC, PairAIBETH,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Coinbase price source
// ===========================================================================

// CoinbaseSource fetches price data from the Coinbase API.
// API endpoint: https://api.coinbase.com/v2/prices/{pair}/spot
type CoinbaseSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping maps standard pairs to Coinbase API format
	pairMapping map[string]string
}

// coinbasePriceResponse is the Coinbase price API response
type coinbasePriceResponse struct {
	Data struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"data"`
}

// NewCoinbaseSource creates a Coinbase price source instance
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

// FetchPrice fetches the price for the given pair from Coinbase
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

// IsAvailable checks whether the Coinbase source is available
func (s *CoinbaseSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *CoinbaseSource) GetName() string {
	return "Coinbase"
}

// GetType returns the price source type
func (s *CoinbaseSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs returns the pairs supported by Coinbase
func (s *CoinbaseSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Kraken price source
// ===========================================================================

// KrakenSource fetches price data from the Kraken API.
// API endpoint: https://api.kraken.com/0/public/Ticker
type KrakenSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// pairMapping maps standard pairs to Kraken API format
	pairMapping map[string]string
}

// krakenTickerResponse is the Kraken Ticker API response
type krakenTickerResponse struct {
	Error  []string                          `json:"error"`
	Result map[string]krakenTickerPairResult `json:"result"`
}

// krakenTickerPairResult is the ticker data for a single Kraken pair
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

// NewKrakenSource creates a Kraken price source instance
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

// FetchPrice fetches the price for the given pair from Kraken
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

	// Iterate over result to get the first pair's data
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

// safeIndex safely indexes a string slice, returning "" when out of range
func safeIndex(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// IsAvailable checks whether the Kraken source is available
func (s *KrakenSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *KrakenSource) GetName() string {
	return "Kraken"
}

// GetType returns the price source type
func (s *KrakenSource) GetType() SourceType {
	return SourceTypeCEX
}

// SupportedPairs returns the pairs supported by Kraken
func (s *KrakenSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairBTCUSD, PairETHUSD,
		PairAIBUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// Uniswap V3 price source (on-chain DEX)
// ===========================================================================

// UniswapSource fetches price data from Uniswap V3's The Graph subgraph.
// Queries pool prices via The Graph's public subgraph API.
type UniswapSource struct {
	mu        sync.RWMutex
	available bool
	subgraph  string
	// poolMapping maps pairs to Uniswap pool addresses
	poolMapping map[string]string
}

// uniswapGraphResponse is The Graph subgraph query response
type uniswapGraphResponse struct {
	Data struct {
		Pool *struct {
			Token0Price         string `json:"token0Price"`
			Token1Price         string `json:"token1Price"`
			VolumeUSD           string `json:"volumeUSD"`
			TotalValueLockedUSD string `json:"totalValueLockedUSD"`
		} `json:"pool"`
	} `json:"data"`
}

// NewUniswapSource creates a Uniswap price source instance
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

// FetchPrice fetches the price for the given pair from the Uniswap V3 subgraph
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

	// Wrap the query as the request body using io.NopCloser
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

// stringReader is a helper that returns an io.Reader over a string
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

// IsAvailable checks whether the Uniswap source is available
func (s *UniswapSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *UniswapSource) GetName() string {
	return "Uniswap V3"
}

// GetType returns the price source type
func (s *UniswapSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs returns the pairs supported by Uniswap
func (s *UniswapSource) SupportedPairs() []TradingPair {
	return []TradingPair{
		PairETHUSD, PairBTCUSD,
		PairUSDTUSD, PairUSDCUSD,
	}
}

// ===========================================================================
// SushiSwap price source (on-chain DEX)
// ===========================================================================

// SushiSwapSource fetches price data from SushiSwap's The Graph subgraph
type SushiSwapSource struct {
	mu        sync.RWMutex
	available bool
	subgraph  string
	// pairMapping maps pairs to SushiSwap pool addresses
	pairMapping map[string]string
}

// sushiGraphResponse is the SushiSwap Graph subgraph query response
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

// NewSushiSwapSource creates a SushiSwap price source instance
func NewSushiSwapSource() *SushiSwapSource {
	return &SushiSwapSource{
		available: true,
		subgraph:  "https://api.thegraph.com/subgraphs/name/sushiswap/exchange",
		pairMapping: map[string]string{
			"ETH/USD": "0x397ff1542f962076d0bfe58ea045ffa2d347aca0", // ETH/USDC
			"BTC/USD": "0xceff51756c56ceffca006cd410b03ffc46dd3a58", // WBTC/WETH (needs conversion)
		},
	}
}

// FetchPrice fetches the price for the given pair from the SushiSwap subgraph
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

// IsAvailable checks whether the SushiSwap source is available
func (s *SushiSwapSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *SushiSwapSource) GetName() string {
	return "SushiSwap"
}

// GetType returns the price source type
func (s *SushiSwapSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs returns the pairs supported by SushiSwap
func (s *SushiSwapSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairETHUSD, PairBTCUSD}
}

// ===========================================================================
// Curve Finance price source (on-chain DEX, stablecoin-focused)
// ===========================================================================

// CurveSource fetches price data from the Curve Finance API.
// Curve is known for stablecoin pairs and low slippage.
type CurveSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// poolMapping maps pairs to Curve pool identifiers
	poolMapping map[string]string
}

// curvePoolResponse is the Curve API response
type curvePoolResponse struct {
	Data struct {
		PoolData []struct {
			ID             string  `json:"id"`
			VirtualPrice   string  `json:"virtualPrice"`
			TotalLiquidity string  `json:"totalLiquidity"`
			USDTotal       float64 `json:"usdTotal"`
			Coins          []struct {
				Address  string  `json:"address"`
				Symbol   string  `json:"symbol"`
				USDPrice float64 `json:"usdPrice"`
			} `json:"coins"`
		} `json:"poolData"`
	} `json:"data"`
}

// NewCurveSource creates a Curve price source instance
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

// FetchPrice fetches the price for the given pair from Curve
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

	// Look up the target asset's price among the pool's coins
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
		// Stablecoins default to 1.0 (only when the API cannot return a price)
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

// IsAvailable checks whether the Curve source is available
func (s *CurveSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *CurveSource) GetName() string {
	return "Curve"
}

// GetType returns the price source type
func (s *CurveSource) GetType() SourceType {
	return SourceTypeDEX
}

// SupportedPairs returns the pairs supported by Curve
func (s *CurveSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairUSDTUSD, PairUSDCUSD}
}

// ===========================================================================
// Stablecoin peg price source
// ===========================================================================

// StablecoinSource provides pegged prices for stablecoins (USDT, USDC, DAI).
// Fetches actual stablecoin price deviation data from the CoinGecko API.
type StablecoinSource struct {
	mu        sync.RWMutex
	available bool
	baseURL   string
	// coinIDs maps stablecoin symbols to CoinGecko coin IDs
	coinIDs map[string]string
}

// coingeckoResponse is the CoinGecko simple price API response
type coingeckoResponse map[string]struct {
	USD       float64 `json:"usd"`
	Volume24h float64 `json:"usd_24h_vol"`
}

// NewStablecoinSource creates a stablecoin peg price source instance
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

// FetchPrice fetches the stablecoin's actual price from CoinGecko
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

// IsAvailable checks whether the stablecoin source is available
func (s *StablecoinSource) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// GetName returns the price source name
func (s *StablecoinSource) GetName() string {
	return "Stablecoin-CoinGecko"
}

// GetType returns the price source type
func (s *StablecoinSource) GetType() SourceType {
	return SourceTypeStablecoin
}

// SupportedPairs returns the pairs supported by the stablecoin source
func (s *StablecoinSource) SupportedPairs() []TradingPair {
	return []TradingPair{PairUSDTUSD, PairUSDCUSD}
}

// ===========================================================================
// Factory functions
// ===========================================================================

// DefaultSources creates and returns the list of all default price sources.
// Includes all CEX, DEX, and stablecoin peg sources.
func DefaultSources() []PriceSource {
	return []PriceSource{
		// CEX price sources
		NewBinanceSource(),
		NewCoinbaseSource(),
		NewKrakenSource(),
		// DEX price sources
		NewUniswapSource(),
		NewSushiSwapSource(),
		NewCurveSource(),
		// Stablecoin pegs
		NewStablecoinSource(),
	}
}
