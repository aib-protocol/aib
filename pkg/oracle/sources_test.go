package oracle

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withMockHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()
	oldClient := httpClient
	httpClient = client
	t.Cleanup(func() {
		httpClient = oldClient
	})
}

func TestUniswapSource_FetchPrice_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/subgraph", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		_ = r.Body.Close()
		if !strings.Contains(string(body), "pool") {
			t.Fatalf("expected GraphQL pool query, body=%s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"pool": {
					"token0Price": "3500.12",
					"token1Price": "0.00028570",
					"volumeUSD": "18500000.5",
					"totalValueLockedUSD": "240000000.75"
				}
			}
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	withMockHTTPClient(t, server.Client())

	src := NewUniswapSource()
	src.subgraph = server.URL + "/subgraph"

	price, err := src.FetchPrice(PairETHUSD)
	if err != nil {
		t.Fatalf("FetchPrice failed: %v", err)
	}

	if price.Pair != PairETHUSD {
		t.Fatalf("pair mismatch: %+v", price.Pair)
	}
	if price.Price != 3500.12 {
		t.Fatalf("expected price 3500.12, got %f", price.Price)
	}
	if price.Volume24h != 18500000.5 {
		t.Fatalf("expected volume 18500000.5, got %f", price.Volume24h)
	}
	if price.Liquidity != 240000000.75 {
		t.Fatalf("expected liquidity 240000000.75, got %f", price.Liquidity)
	}
	if price.Source != "Uniswap V3" {
		t.Fatalf("expected source Uniswap V3, got %s", price.Source)
	}
	if price.SourceType != SourceTypeDEX {
		t.Fatalf("expected DEX source type, got %v", price.SourceType)
	}
	if price.Timestamp.IsZero() {
		t.Fatal("timestamp should not be zero")
	}
}

func TestBinanceSource_FetchPrice_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol != "BTCUSDT" {
			t.Fatalf("expected symbol BTCUSDT, got %s", symbol)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"symbol":"BTCUSDT",
			"lastPrice":"68234.56",
			"bidPrice":"68230.10",
			"askPrice":"68238.90",
			"volume":"12450.78"
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	withMockHTTPClient(t, server.Client())

	src := NewBinanceSource()
	src.baseURL = server.URL

	price, err := src.FetchPrice(PairBTCUSD)
	if err != nil {
		t.Fatalf("FetchPrice failed: %v", err)
	}

	if price.Price != 68234.56 {
		t.Fatalf("expected price 68234.56, got %f", price.Price)
	}
	if price.Bid != 68230.10 {
		t.Fatalf("expected bid 68230.10, got %f", price.Bid)
	}
	if price.Ask != 68238.90 {
		t.Fatalf("expected ask 68238.90, got %f", price.Ask)
	}
	if price.Volume24h != 12450.78 {
		t.Fatalf("expected volume 12450.78, got %f", price.Volume24h)
	}
	if price.Source != "Binance" {
		t.Fatalf("expected source Binance, got %s", price.Source)
	}
	if price.SourceType != SourceTypeCEX {
		t.Fatalf("expected CEX source type, got %v", price.SourceType)
	}
	if price.Timestamp.IsZero() {
		t.Fatal("timestamp should not be zero")
	}
}

func TestStablecoinSource_FetchPrice_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/simple/price", func(w http.ResponseWriter, r *http.Request) {
		coinID := r.URL.Query().Get("ids")
		if coinID != "tether" {
			t.Fatalf("expected ids=tether, got %s", coinID)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tether": {
				"usd": 0.9998,
				"usd_24h_vol": 45800000000.25
			}
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	withMockHTTPClient(t, server.Client())

	src := NewStablecoinSource()
	src.baseURL = server.URL + "/api/v3"

	price, err := src.FetchPrice(PairUSDTUSD)
	if err != nil {
		t.Fatalf("FetchPrice failed: %v", err)
	}

	if price.Price != 0.9998 {
		t.Fatalf("expected price 0.9998, got %f", price.Price)
	}
	if price.Volume24h != 45800000000.25 {
		t.Fatalf("expected volume 45800000000.25, got %f", price.Volume24h)
	}
	if price.SourceType != SourceTypeStablecoin {
		t.Fatalf("expected stablecoin source type, got %v", price.SourceType)
	}
	if price.Source != "Stablecoin-CoinGecko" {
		t.Fatalf("expected source Stablecoin-CoinGecko, got %s", price.Source)
	}
	if price.Timestamp.IsZero() {
		t.Fatal("timestamp should not be zero")
	}
}

func TestPriceOracle_MultiSourceAggregation_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol != "USDTUSD" {
			t.Fatalf("expected symbol USDTUSD, got %s", symbol)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"symbol":"USDTUSD",
			"lastPrice":"0.9995",
			"bidPrice":"0.9994",
			"askPrice":"0.9996",
			"volume":"2800000000"
		}`))
	})

	mux.HandleFunc("/subgraph", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST for subgraph, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"pool": {
					"token0Price": "1.0003",
					"token1Price": "0.9997",
					"volumeUSD": "1750000000",
					"totalValueLockedUSD": "900000000"
				}
			}
		}`))
	})

	mux.HandleFunc("/api/v3/simple/price", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ids") != "tether" {
			t.Fatalf("expected ids=tether, got %s", r.URL.Query().Get("ids"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tether": {
				"usd": 1.0001,
				"usd_24h_vol": 46000000000
			}
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	withMockHTTPClient(t, server.Client())

	binance := NewBinanceSource()
	binance.baseURL = server.URL

	uniswap := NewUniswapSource()
	uniswap.subgraph = server.URL + "/subgraph"
	uniswap.poolMapping[PairUSDTUSD.String()] = "0x3416cf6c708da44db2624d63ea0aaef7113527c6"

	stable := NewStablecoinSource()
	stable.baseURL = server.URL + "/api/v3"

	cfg := DefaultConfig()
	cfg.MinSources = 3
	cfg.CacheTTL = 50 * time.Millisecond
	cfg.DeviationThreshold = 5.0

	oracle, err := NewPriceOracle([]PriceSource{binance, uniswap, stable}, cfg)
	if err != nil {
		t.Fatalf("NewPriceOracle failed: %v", err)
	}

	agg, err := oracle.GetPrice(PairUSDTUSD)
	if err != nil {
		t.Fatalf("GetPrice failed: %v", err)
	}

	if agg.Sources < 3 {
		t.Fatalf("expected at least 3 sources, got %d", agg.Sources)
	}
	if agg.Price < 0.9995 || agg.Price > 1.0003 {
		t.Fatalf("aggregated price out of range: %f", agg.Price)
	}
	if agg.Confidence <= 0 {
		t.Fatalf("confidence should be > 0, got %f", agg.Confidence)
	}
	if agg.TotalVolume <= 0 {
		t.Fatalf("total volume should be > 0, got %f", agg.TotalVolume)
	}
	if agg.Timestamp.IsZero() {
		t.Fatal("aggregated timestamp should not be zero")
	}

	for _, pd := range agg.RawPrices {
		if pd.Source == "" {
			t.Fatalf("raw price source should not be empty: %+v", pd)
		}
	}

	_ = fmt.Sprintf("%.6f", agg.Price)
}
