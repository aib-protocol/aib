// Package aib provides the AIB 2.0 SDK for Go.
// API client for interacting with the AIB blockchain.
package aib

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientConfig contains configuration for the API client.
type ClientConfig struct {
	// BaseURL is the API endpoint URL
	BaseURL string

	// HTTPClient is the HTTP client to use (optional)
	HTTPClient *http.Client

	// Timeout is the request timeout (default: 30 seconds)
	Timeout time.Duration
}

// DefaultClientConfig returns default client configuration.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		BaseURL: "http://localhost:8080/api/v1",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Timeout: 30 * time.Second,
	}
}

// Client is an API client for interacting with AIB blockchain.
type Client struct {
	config    ClientConfig
	httpClient *http.Client
}

// NewClient creates a new API client with the specified configuration.
func NewClient(config ClientConfig) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}
}

// Balance represents account balance information.
type Balance struct {
	Address   string `json:"address"`
	Confirmed uint64 `json:"confirmed"`
	Pending   uint64 `json:"pending"`
	Locked    uint64 `json:"locked"`
}

// GetBalance retrieves the balance for an address.
func (c *Client) GetBalance(address string) (*Balance, error) {
	url := fmt.Sprintf("%s/balance/%s", c.config.BaseURL, address)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var balance Balance
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &balance, nil
}

// UTXO represents an unspent transaction output.
type UTXO struct {
	TxHash    string `json:"tx_hash"`
	Index     uint32 `json:"index"`
	Address   string `json:"address"`
	Amount    uint64 `json:"amount"`
	AssetID   string `json:"asset_id,omitempty"`
	Confirmations uint64 `json:"confirmations"`
}

// GetUTXOs retrieves unspent outputs for an address.
func (c *Client) GetUTXOs(address string) ([]UTXO, error) {
	url := fmt.Sprintf("%s/utxos/%s", c.config.BaseURL, address)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var utxos []UTXO
	if err := json.NewDecoder(resp.Body).Decode(&utxos); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return utxos, nil
}

// TransactionInfo represents transaction information.
type TransactionInfo struct {
	TxHash        string    `json:"tx_hash"`
	Version       uint32    `json:"version"`
	LockTime      uint32    `json:"lock_time"`
	Confirmations uint64    `json:"confirmations"`
	Timestamp     int64     `json:"timestamp"`
	Inputs        []TXInputInfo  `json:"inputs"`
	Outputs       []TXOutputInfo `json:"outputs"`
	Fee           uint64    `json:"fee"`
}

// TXInputInfo represents input information in a transaction.
type TXInputInfo struct {
	TxHash    string `json:"tx_hash"`
	Index     uint32 `json:"index"`
	Address   string `json:"address"`
	Amount    uint64 `json:"amount"`
}

// TXOutputInfo represents output information in a transaction.
type TXOutputInfo struct {
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`
	AssetID string `json:"asset_id,omitempty"`
}

// GetTransaction retrieves transaction details by hash.
func (c *Client) GetTransaction(txHash string) (*TransactionInfo, error) {
	url := fmt.Sprintf("%s/tx/%s", c.config.BaseURL, txHash)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var txInfo TransactionInfo
	if err := json.NewDecoder(resp.Body).Decode(&txInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &txInfo, nil
}

// SendTransactionRequest represents a request to send a transaction.
type SendTransactionRequest struct {
	Transaction string `json:"tx"` // Hex-encoded transaction
}

// SendTransactionResponse represents the response from sending a transaction.
type SendTransactionResponse struct {
	TxHash string `json:"tx_hash"`
	Code   int    `json:"code"`
}

// SendTransaction submits a signed transaction to the network.
func (c *Client) SendTransaction(tx *Transaction) (string, error) {
	// Serialize transaction to hex
	txHex := hex.EncodeToString(tx.Serialize())

	url := fmt.Sprintf("%s/tx", c.config.BaseURL)

	reqBody := SendTransactionRequest{
		Transaction: txHex,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result SendTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("transaction failed with code: %d", result.Code)
	}

	return result.TxHash, nil
}

// GetTransactionHistory retrieves transaction history for an address.
func (c *Client) GetTransactionHistory(address string, limit, offset int) ([]TransactionInfo, error) {
	url := fmt.Sprintf("%s/address/%s/txs?limit=%d&offset=%d", c.config.BaseURL, address, limit, offset)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var txs []TransactionInfo
	if err := json.NewDecoder(resp.Body).Decode(&txs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return txs, nil
}

// GetBlockInfo retrieves block information.
type BlockInfo struct {
	Hash       string `json:"hash"`
	Height     uint64 `json:"height"`
	Timestamp  int64  `json:"timestamp"`
	TxCount    int    `json:"tx_count"`
	PrevHash   string `json:"prev_hash"`
}

// GetBlock retrieves block information by height or hash.
func (c *Client) GetBlock(blockID string) (*BlockInfo, error) {
	url := fmt.Sprintf("%s/block/%s", c.config.BaseURL, blockID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var block BlockInfo
	if err := json.NewDecoder(resp.Body).Decode(&block); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &block, nil
}

// GetNetworkInfo retrieves network information.
type NetworkInfo struct {
	ChainID        string `json:"chain_id"`
	Version        string `json:"version"`
	Height         uint64 `json:"height"`
	BestBlockHash  string `json:"best_block_hash"`
}

// GetNetworkInfo retrieves current network status.
func (c *Client) GetNetworkInfo() (*NetworkInfo, error) {
	url := fmt.Sprintf("%s/network", c.config.BaseURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get network info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var info NetworkInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// EstimateFee estimates the transaction fee.
func (c *Client) EstimateFee(txSize int) (uint64, error) {
	url := fmt.Sprintf("%s/fee?size=%d", c.config.BaseURL, txSize)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate fee: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var fee struct {
		Fee uint64 `json:"fee"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fee); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return fee.Fee, nil
}
