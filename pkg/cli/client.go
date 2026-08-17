// Package cli provides shared functionality for AIB command-line tools
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultAPIEndpoint is the default API endpoint
	DefaultAPIEndpoint = "http://127.0.0.1:8080"
	// APITimeout is the default HTTP timeout for API requests
	APITimeout = 30 * time.Second
)

// Client represents an API client for communicating with the AIB node
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new API client
func NewClient(endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultAPIEndpoint
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	return &Client{
		BaseURL: strings.TrimSuffix(endpoint, "/"),
		HTTPClient: &http.Client{
			Timeout: APITimeout,
		},
	}
}

// APIResponse represents the standard API response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents error details in API responses
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// doRequest performs an HTTP request with proper error handling
func (c *Client) doRequest(method, path string, body []byte) (*APIResponse, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !apiResp.Success && apiResp.Error != nil {
		return &apiResp, fmt.Errorf("%s: %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	return &apiResp, nil
}

// Get performs a GET request
func (c *Client) Get(path string) (*APIResponse, error) {
	return c.doRequest("GET", path, nil)
}

// Post performs a POST request
func (c *Client) Post(path string, body interface{}) (*APIResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("编码请求体失败: %w", err)
	}
	return c.doRequest("POST", path, jsonBody)
}

// ============================================================================
// API Response Types
// ============================================================================

// WalletCreateResponse represents wallet creation response
type WalletCreateResponse struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Mnemonic   string `json:"mnemonic,omitempty"`
}

// BalanceResponse represents balance query response
type BalanceResponse struct {
	Address   string     `json:"address"`
	Balance   uint64     `json:"balance"`
	UTXOCount int        `json:"utxo_count"`
	UTXOs     []UTxOInfo `json:"utxos,omitempty"`
}

// UTxOInfo represents UTXO information
type UTxOInfo struct {
	TxHash string `json:"tx_hash"`
	Index  uint32 `json:"index"`
	Value  uint64 `json:"value"`
}

// SendTransactionRequest represents send transaction request
type SendTransactionRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   uint64 `json:"amount"`
	GasLimit uint64 `json:"gas_limit,omitempty"`
	GasPrice uint64 `json:"gas_price,omitempty"`
}

// SendTransactionResponse represents send transaction response
type SendTransactionResponse struct {
	TxHash string `json:"tx_hash"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount uint64 `json:"amount"`
}

// TransactionStatusResponse represents transaction status response
type TransactionStatusResponse struct {
	TxHash    string    `json:"tx_hash"`
	Status    string    `json:"status"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Amount    uint64    `json:"amount,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// StakeRequest represents staking request
type StakeRequest struct {
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`
}

// StakeResponse represents staking response
type StakeResponse struct {
	TxHash      string `json:"tx_hash"`
	Address     string `json:"address"`
	Amount      uint64 `json:"amount"`
	NewStake    uint64 `json:"new_stake"`
	TotalStaked uint64 `json:"total_staked"`
}

// NodeStatusResponse represents node status response
type NodeStatusResponse struct {
	Status       string  `json:"status"`
	Version      string  `json:"version"`
	Uptime       string  `json:"uptime"`
	Height       uint64  `json:"height"`
	Hash         string  `json:"hash"`
	LastBlock    string  `json:"last_block_time"`
	PeerCount    int     `json:"peer_count"`
	Syncing      bool    `json:"syncing"`
	SyncProgress float64 `json:"sync_progress"`
}

// PeerInfo represents peer information
type PeerInfo struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LastSeen  time.Time `json:"last_seen"`
	Connected bool      `json:"connected"`
}

// PeersResponse represents peers list response
type PeersResponse struct {
	Peers []PeerInfo `json:"peers"`
	Total int        `json:"total"`
}

// BlockResponse represents block information
type BlockResponse struct {
	Height    uint64    `json:"height"`
	Hash      string    `json:"hash"`
	PrevHash  string    `json:"prev_hash"`
	Timestamp time.Time `json:"timestamp"`
	TxCount   int       `json:"tx_count"`
	Validator string    `json:"validator,omitempty"`
	Size      uint64    `json:"size"`
	Proposer  string    `json:"proposer,omitempty"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

// FormatAmount formats an amount in human-readable form (assuming 18 decimals)
func FormatAmount(amount uint64) string {
	if amount == 0 {
		return "0"
	}
	// Convert to float with 18 decimal places
	divisor := uint64(1e18)
	whole := amount / divisor
	frac := amount % divisor
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	// Remove trailing zeros from fractional part
	fracStr := fmt.Sprintf("%018d", frac)
	fracStr = strings.TrimRight(fracStr, "0")
	if fracStr == "" {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.%s", whole, fracStr)
}

// ParseAmount parses a human-readable amount to uint64 (18 decimals)
func ParseAmount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("金额不能为空")
	}

	// Check if there's a decimal point
	if idx := strings.Index(s, "."); idx != -1 {
		whole := s[:idx]
		frac := s[idx+1:]

		// Pad or truncate fractional part to 18 digits
		if len(frac) > 18 {
			frac = frac[:18]
		} else {
			frac = frac + strings.Repeat("0", 18-len(frac))
		}

		var wholePart, fracPart uint64
		if whole != "" {
			_, err := fmt.Sscanf(whole, "%d", &wholePart)
			if err != nil {
				return 0, fmt.Errorf("无效的金额格式: %w", err)
			}
		}
		if frac != "" {
			_, err := fmt.Sscanf(frac, "%d", &fracPart)
			if err != nil {
				return 0, fmt.Errorf("无效的金额格式: %w", err)
			}
		}

		return wholePart*1e18 + fracPart, nil
	}

	// No decimal point
	var amount uint64
	_, err := fmt.Sscanf(s, "%d", &amount)
	if err != nil {
		return 0, fmt.Errorf("无效的金额格式: %w", err)
	}
	return amount * 1e18, nil
}
