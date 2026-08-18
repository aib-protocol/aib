// Package cli provides shared functionality for AIB command-line tools
package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// WalletCommand handles wallet-related operations
type WalletCommand struct {
	client *Client
	format OutputFormat
}

// NewWalletCommand creates a new wallet command handler
func NewWalletCommand(client *Client, format OutputFormat) *WalletCommand {
	return &WalletCommand{
		client: client,
		format: format,
	}
}

// Create creates a new wallet
func (w *WalletCommand) Create(savePath string) error {
	resp, err := w.client.Post("/v1/wallet/create", nil)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	// Extract data from response
	dataBytes, _ := json.Marshal(resp.Data)
	var result WalletCreateResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Save to file if path specified
	if savePath != "" {
		if err := w.saveWallet(&result, savePath); err != nil {
			return fmt.Errorf("failed to save wallet: %w", err)
		}
		fmt.Printf("Wallet saved to: %s\n", savePath)
	}

	return FormatOutput(&result, w.format, nil)
}

// Restore restores a wallet from mnemonic
func (w *WalletCommand) Restore(mnemonic, savePath string) error {
	mnemonic = strings.TrimSpace(mnemonic)
	if mnemonic == "" {
		return fmt.Errorf("mnemonic cannot be empty")
	}

	reqBody := map[string]string{"mnemonic": mnemonic}
	resp, err := w.client.Post("/v1/wallet/restore", reqBody)
	if err != nil {
		return fmt.Errorf("failed to restore wallet: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result WalletCreateResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Save to file if path specified
	if savePath != "" {
		if err := w.saveWallet(&result, savePath); err != nil {
			return fmt.Errorf("failed to save wallet: %w", err)
		}
		fmt.Printf("Wallet saved to: %s\n", savePath)
	}

	return FormatOutput(&result, w.format, nil)
}

// Balance queries the balance of an address
func (w *WalletCommand) Balance(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}

	resp, err := w.client.Get("/v1/wallet/balance?address=" + address)
	if err != nil {
		return fmt.Errorf("failed to query balance: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result BalanceResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, w.format, nil)
}

// Send sends a transaction
func (w *WalletCommand) Send(from, to string, amount uint64, gasLimit, gasPrice uint64) error {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return fmt.Errorf("sender and receiver addresses cannot be empty")
	}
	if amount == 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	req := SendTransactionRequest{
		From:     from,
		To:       to,
		Amount:   amount,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
	}

	resp, err := w.client.Post("/v1/wallet/send", req)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result SendTransactionResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, w.format, nil)
}

// Stake stakes tokens
func (w *WalletCommand) Stake(address string, amount uint64) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if amount == 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	req := StakeRequest{
		Address: address,
		Amount:  amount,
	}

	resp, err := w.client.Post("/v1/wallet/stake", req)
	if err != nil {
		return fmt.Errorf("failed to stake: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result StakeResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, w.format, nil)
}

// Unstake unstakes tokens
func (w *WalletCommand) Unstake(address string, amount uint64) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if amount == 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	req := StakeRequest{
		Address: address,
		Amount:  amount,
	}

	resp, err := w.client.Post("/v1/wallet/unstake", req)
	if err != nil {
		return fmt.Errorf("failed to unstake: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result StakeResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, w.format, nil)
}

// saveWallet saves wallet data to a file
func (w *WalletCommand) saveWallet(data *WalletCreateResponse, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		// Create directory if needed
		_ = mkdirAll(dir)
	}

	walletData := map[string]string{
		"address":     data.Address,
		"public_key":  data.PublicKey,
		"private_key": data.PrivateKey,
	}
	if data.Mnemonic != "" {
		walletData["mnemonic"] = data.Mnemonic
	}

	jsonData, err := json.MarshalIndent(walletData, "", "  ")
	if err != nil {
		return err
	}

	return writeFile(path, jsonData)
}

// NodeCommand handles node-related operations
type NodeCommand struct {
	client *Client
	format OutputFormat
}

// NewNodeCommand creates a new node command handler
func NewNodeCommand(client *Client, format OutputFormat) *NodeCommand {
	return &NodeCommand{
		client: client,
		format: format,
	}
}

// Status queries the node status
func (n *NodeCommand) Status() error {
	resp, err := n.client.Get("/v1/status")
	if err != nil {
		return fmt.Errorf("failed to query node status: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result NodeStatusResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, n.format, nil)
}

// Peers queries the connected peers
func (n *NodeCommand) Peers() error {
	resp, err := n.client.Get("/v1/peers")
	if err != nil {
		return fmt.Errorf("failed to query peers: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result PeersResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, n.format, nil)
}

// Block queries a block by height or hash
func (n *NodeCommand) Block(heightOrHash string) error {
	heightOrHash = strings.TrimSpace(heightOrHash)
	if heightOrHash == "" {
		return fmt.Errorf("block height or hash cannot be empty")
	}

	// API uses /v1/block/ for both height and hash
	path := "/v1/block/" + heightOrHash

	resp, err := n.client.Get(path)
	if err != nil {
		return fmt.Errorf("failed to query block: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result BlockResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, n.format, nil)
}

// TxCommand handles transaction-related operations
type TxCommand struct {
	client *Client
	format OutputFormat
}

// NewTxCommand creates a new transaction command handler
func NewTxCommand(client *Client, format OutputFormat) *TxCommand {
	return &TxCommand{
		client: client,
		format: format,
	}
}

// Query queries a transaction by hash
func (t *TxCommand) Query(hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("transaction hash cannot be empty")
	}

	resp, err := t.client.Get("/v1/transaction/" + hash)
	if err != nil {
		return fmt.Errorf("failed to query transaction: %w", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var result TransactionStatusResponse
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return FormatOutput(&result, t.format, nil)
}

// Helper functions
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
