// Package api provides REST API handlers for AIB 2.0 wallet operations.
// This file implements handlers for wallet management and transaction queries.
package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
	"github.com/aib-protocol/aib/pkg/wallet"
)

// ============================================================================
// Transaction query handlers
// ============================================================================

// TransactionListRequest transaction list request parameters
type TransactionListRequest struct {
	Address string `json:"address"` // optional: filter by address
	Limit   int    `json:"limit"`   // optional: limit, default 100
	Offset  int    `json:"offset"`  // optional: offset, default 0
}

// TransactionInfo transaction info
type TransactionInfo struct {
	Hash      string     `json:"hash"`
	Version   uint32     `json:"version"`
	Inputs    []TxInput  `json:"inputs"`
	Outputs   []TxOutput `json:"outputs"`
	LockTime  uint32     `json:"lock_time"`
	Sequence  uint64     `json:"sequence"`
	Timestamp *uint64    `json:"timestamp,omitempty"`  // if in a block
	BlockHash *string    `json:"block_hash,omitempty"` // if in a block
	Height    *uint64    `json:"height,omitempty"`     // if in a block
	Fee       *uint64    `json:"fee,omitempty"`        // transaction fee
	Size      int        `json:"size"`                 // transaction size (bytes)
}

// TxInput transaction input
type TxInput struct {
	TxHash      string `json:"tx_hash"`
	OutputIndex uint32 `json:"output_index"`
	Signature   string `json:"signature"`
	PublicKey   string `json:"public_key"`
}

// TxOutput transaction output
type TxOutput struct {
	Value   uint64  `json:"value"`
	Address string  `json:"address"`
	PubKey  string  `json:"pub_key"`
	Index   uint32  `json:"index"`
	IsSpent bool    `json:"is_spent"`
	SpentBy *string `json:"spent_by,omitempty"`
}

// TransactionListResponse transaction list response
type TransactionListResponse struct {
	Transactions []TransactionInfo `json:"transactions"`
	Total        int               `json:"total"`
	Limit        int               `json:"limit"`
	Offset       int               `json:"offset"`
}

// TransactionDetailResponse transaction detail response
type TransactionDetailResponse struct {
	Transaction   TransactionInfo `json:"transaction"`
	Confirmations *uint64         `json:"confirmations,omitempty"`
}

// handleTransactionsList handles transaction list queries
// GET /v1/transactions?address={address}&limit={limit}&offset={offset}
func (s *Server) handleTransactionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// parse query parameters
	address := r.URL.Query().Get("address")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var txInfos []TransactionInfo
	total := 0

	// get pending transactions from the mempool
	if s.mempool != nil {
		entries := s.mempool.GetAllEntries()
		for _, entry := range entries[:min(len(entries), limit)] {
			if address != "" {
				// checks whether a transaction relates to an address
				if !txIsRelatedToAddress(entry.Tx, address) {
					continue
				}
			}

			txInfos = append(txInfos, mempoolEntryToInfo(entry))
		}
	}

	// TODO: fetch transactions from confirmed blocks
	// requires walking the chain to find related transactions

	total = len(txInfos)

	writeSuccess(w, TransactionListResponse{
		Transactions: txInfos,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	})
}

// handleTransactionDetail handles transaction detail queries
// GET /v1/transaction/{hash}
func (s *Server) handleTransactionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// extract the transaction hash from the URL path
	// URL format: /v1/transaction/{hash}
	hashStr := r.URL.Path[len("/v1/transaction/"):]
	if hashStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Transaction hash is required", "")
		return
	}

	txHash, err := hex.DecodeString(hashStr)
	if err != nil || len(txHash) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid transaction hash", "")
		return
	}

	var hashArray [32]byte
	copy(hashArray[:], txHash)

	var txInfo *TransactionInfo
	var confirmations *uint64

	// first look in the mempool
	if s.mempool != nil {
		if tx := s.mempool.GetTransaction(hashArray); tx != nil {
			info := transactionToInfo(tx)
			txInfo = &info
			zero := uint64(0)
			confirmations = &zero
		}
	}

	// if not found in the mempool, search the chain
	if txInfo == nil && s.chain != nil && s.utxoStore != nil {
		if height, err := s.utxoStore.GetTransactionIndex(hashArray); err == nil {
			// find the height of the block containing the transaction
			bestHeight, _ := s.chain.GetBestBlockHeight()
			confirmationsVal := uint64(0)
			if bestHeight >= height {
				confirmationsVal = bestHeight - height + 1
			}

			// get transactions in the block - needs another approach
			// simplified: get transaction info directly from utxoStore
			// for now return basic info without full transaction details
			txInfo = &TransactionInfo{
				Hash:    hashStr,
				Version: 0, // needs to be fetched from storage
				Height:  &height,
			}
			confirmations = &confirmationsVal
		}
	}

	if txInfo == nil {
		writeError(w, http.StatusNotFound, ErrCodeNotFound, "Transaction not found", "")
		return
	}

	writeSuccess(w, TransactionDetailResponse{
		Transaction:   *txInfo,
		Confirmations: confirmations,
	})
}

// ============================================================================
// Wallet management handlers
// ============================================================================

// CreateWalletRequest create wallet request
type CreateWalletRequest struct {
	Password string `json:"password"` // encryption password (optional)
	Label    string `json:"label"`    // wallet label
}

// CreateWalletResponse create wallet response
type CreateWalletResponse struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"` // seed hex — SAVE THIS, shown only once
	Label      string `json:"label,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// RestoreWalletRequest restore wallet request
type RestoreWalletRequest struct {
	PrivateKey string `json:"private_key"` // hex string of the private key
	Password   string `json:"password"`    // encryption password (optional)
	Label      string `json:"label"`       // wallet label
}

// ImportWalletRequest import wallet request (from mnemonic or key file)
type ImportWalletRequest struct {
	Source   string `json:"source"`   // import source: mnemonic, keystore, private_key
	Data     string `json:"data"`     // import data
	Password string `json:"password"` // decryption password (if needed)
	Label    string `json:"label"`    // wallet label
}

// ExportWalletRequest export wallet request
type ExportWalletRequest struct {
	Address  string `json:"address"`  // wallet address
	Format   string `json:"format"`   // export format: private_key, keystore
	Password string `json:"password"` // encryption password (if needed)
}

// ExportWalletResponse export wallet response
type ExportWalletResponse struct {
	Data      string `json:"data"`      // exported data
	Format    string `json:"format"`    // format type
	Timestamp int64  `json:"timestamp"` // export time
}

// WalletBalanceRequest wallet balance request
type WalletBalanceRequest struct {
	Address string `json:"address"` // wallet address
}

// WalletBalanceResponse wallet balance response
type WalletBalanceResponse struct {
	Address          string  `json:"address"`
	Balance          uint64  `json:"balance"`            // available balance (smallest unit)
	BalanceAIB       float64 `json:"balance_aib"`        // available balance (AIB)
	Unconfirmed      uint64  `json:"unconfirmed"`        // unconfirmed balance
	UnconfirmedAIB   float64 `json:"unconfirmed_aib"`    // unconfirmed balance (AIB)
	UTXOCount        int     `json:"utxo_count"`         // UTXO count
	PendingUTXOCount int     `json:"pending_utxo_count"` // pending UTXO count
}

// SendTransactionRequest send transaction request
type SendTransactionRequest struct {
	FromAddress string `json:"from_address"` // sender address
	ToAddress   string `json:"to_address"`   // recipient address
	Amount      uint64 `json:"amount"`       // amount (smallest unit)
	Fee         uint64 `json:"fee"`          // transaction fee (optional, auto-computed)
	PrivateKey  string `json:"private_key"`  // private key (for signing)
	Memo        string `json:"memo"`         // memo (optional)
}

// SendTransactionResponse send transaction response
type SendTransactionResponse struct {
	TxHash      string `json:"tx_hash"`      // transaction hash
	FromAddress string `json:"from_address"` // sender address
	ToAddress   string `json:"to_address"`   // recipient address
	Amount      uint64 `json:"amount"`       // amount
	Fee         uint64 `json:"fee"`          // actual fee
	Timestamp   int64  `json:"timestamp"`    // submission time
}

// handleCreateWallet handles wallet creation
// POST /v1/wallet/create
func (s *Server) handleCreateWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req CreateWalletRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// create a new wallet
	walletInstance, err := wallet.NewWallet()
	if err != nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to create wallet", err.Error())
		return
	}
	address := walletInstance.GetAddress()
	pubKey := walletInstance.GetPublicKey()

	response := CreateWalletResponse{
		Address:    hex.EncodeToString(address[:]),
		PublicKey:  hex.EncodeToString(pubKey),
		PrivateKey: hex.EncodeToString(walletInstance.ExportPrivateKey()),
		Label:      req.Label,
		CreatedAt:  time.Now().Unix(),
	}

	// TODO: if a password is provided, the private key should be stored encrypted
	// this requires implementing wallet storage

	writeSuccess(w, response)
}

// handleRestoreWallet handles wallet restoration
// POST /v1/wallet/restore
func (s *Server) handleRestoreWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req RestoreWalletRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// decode the private key
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// restore the wallet from the private key
	sdk, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to restore wallet", err.Error())
		return
	}

	address := sdk.GetAddress()
	pubKey := sdk.GetPublicKey()

	response := CreateWalletResponse{
		Address:   hex.EncodeToString(address[:]),
		PublicKey: hex.EncodeToString(pubKey),
		Label:     req.Label,
		CreatedAt: time.Now().Unix(),
	}

	writeSuccess(w, response)
}

// handleImportWallet handles wallet import
// POST /v1/wallet/import
func (s *Server) handleImportWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req ImportWalletRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	switch req.Source {
	case "private_key":
		// import the private key directly
		privateKey, err := hex.DecodeString(req.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
			return
		}

		if len(privateKey) != ed25519.PrivateKeySize {
			writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
			return
		}

		walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
			PrivateKey: privateKey,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to import wallet", err.Error())
			return
		}

		address := walletSDK.GetAddress()
		pubKey := walletSDK.GetPublicKey()

		response := CreateWalletResponse{
			Address:   hex.EncodeToString(address[:]),
			PublicKey: hex.EncodeToString(pubKey),
			Label:     req.Label,
			CreatedAt: time.Now().Unix(),
		}

		writeSuccess(w, response)

	case "keystore":
		// TODO: implement keystore import
		writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Keystore import not implemented", "")
		return

	case "mnemonic":
		// TODO: implement mnemonic import
		writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Mnemonic import not implemented", "")
		return

	default:
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid import source", "Supported sources: private_key, keystore, mnemonic")
		return
	}
}

// handleExportWallet handles wallet export
// POST /v1/wallet/export
func (s *Server) handleExportWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req ExportWalletRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// TODO: implement wallet export
	// wallet storage is needed to retrieve the private key

	writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Wallet export not implemented", "Requires wallet storage")
}

// handleWalletBalance handles wallet balance queries
// GET /v1/wallet/balance?address={address}
func (s *Server) handleWalletBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	address := r.URL.Query().Get("address")
	if address == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Address is required", "")
		return
	}

	addrBytes, err := hex.DecodeString(address)
	if err != nil || len(addrBytes) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", "")
		return
	}

	var addrArray [32]byte
	copy(addrArray[:], addrBytes)

	// get confirmed UTXOs
	var balance uint64
	utxoCount := 0

	if s.utxoStore != nil {
		utxos := s.utxoStore.GetAllUTXOs(addrArray)
		utxoCount = len(utxos)
		for _, utxo := range utxos {
			balance += utxo.Value
		}
	}

	// get unconfirmed UTXOs (from the mempool)
	var unconfirmed uint64
	pendingUTXOCount := 0

	if s.mempool != nil {
		// compute the pending balance
		// this requires analyzing mempool transactions
		entries := s.mempool.GetAllEntries()
		for _, entry := range entries {
			// check whether any transaction output is sent to this address
			for _, output := range entry.Tx.Outputs {
				if output.Address == addrArray {
					unconfirmed += output.Value
					pendingUTXOCount++
				}
			}
			// check whether any transaction input spends a UTXO of this address
			for range entry.Tx.Inputs {
				// need to find the address of the UTXO referenced by the input
				// this requires accessing the UTXO store to get the referenced output
			}
		}
	}

	response := WalletBalanceResponse{
		Address:          address,
		Balance:          balance,
		BalanceAIB:       float64(balance) / 1e6, // assume 1 AIB = 1,000,000 smallest units
		Unconfirmed:      unconfirmed,
		UnconfirmedAIB:   float64(unconfirmed) / 1e6,
		UTXOCount:        utxoCount,
		PendingUTXOCount: pendingUTXOCount,
	}

	writeSuccess(w, response)
}

// handleSendTransaction handles sending transactions
// POST /v1/wallet/send
func (s *Server) handleSendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req SendTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// decode the private key
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// create the wallet
	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to create wallet", err.Error())
		return
	}

	// verify the sender address
	fromAddr := walletSDK.GetAddress()
	fromAddrStr := hex.EncodeToString(fromAddr[:])
	if req.FromAddress != "" && req.FromAddress != fromAddrStr {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "From address does not match private key", "")
		return
	}

	// decode the recipient address
	toAddrBytes, err := hex.DecodeString(req.ToAddress)
	if err != nil || len(toAddrBytes) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid to address format", "")
		return
	}

	var toAddr [32]byte
	copy(toAddr[:], toAddrBytes)

	// get the UTXO store and mempool
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// use a type assertion to get the actual UTXO store and mempool
	utxoStore, ok := s.utxoStore.(interface {
		GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error)
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not support GetUTXOsForAmount", "")
		return
	}

	// compute the fee
	feePerByte := uint64(1)        // default fee rate
	estimatedTxSize := uint64(200) // estimated transaction size
	var actualFee uint64
	if req.Fee > 0 {
		actualFee = req.Fee
	} else {
		actualFee = feePerByte * estimatedTxSize
	}

	// get the total amount needed (send amount + fee)
	totalNeeded := req.Amount + actualFee

	// select UTXOs
	selectedUTXOs, totalValue, err := utxoStore.GetUTXOsForAmount(fromAddr, totalNeeded)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Failed to select UTXOs", err.Error())
		return
	}

	// build transaction inputs
	inputs := make([]utxo.TXInput, len(selectedUTXOs))
	for i, u := range selectedUTXOs {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// build transaction outputs
	// output 0: recipient
	// output 1: change (if any)
	outputs := []utxo.TXOutput{
		{
			Value:   req.Amount,
			Address: toAddr,
		},
	}

	// compute the change
	changeAmount := totalValue - req.Amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: fromAddr,
		})
	}

	// create the transaction
	tx := utxo.NewTransaction(inputs, outputs)

	// sign all inputs
	privKey := ed25519.PrivateKey(privateKey)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign transaction", err.Error())
			return
		}
	}

	// submit to the mempool
	if s.mempool == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool not available", "")
		return
	}

	// use a type assertion to get the actual mempool
	actualMempool, ok := s.mempool.(interface {
		AddTransaction(tx *utxo.Transaction, utxoProvider utxo.UTXOProvider) error
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool does not support AddTransaction", "")
		return
	}

	// get the UTXOProvider
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not implement UTXOProvider", "")
		return
	}

	// add the transaction to the mempool
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// gossip the tx to peers (MsgTx)
	if s.txBroadcaster != nil {
		go s.txBroadcaster(tx)
	}

	// compute the transaction hash
	txHash := tx.Hash()

	// return the success response
	response := SendTransactionResponse{
		TxHash:      hex.EncodeToString(txHash[:]),
		FromAddress: req.FromAddress,
		ToAddress:   req.ToAddress,
		Amount:      req.Amount,
		Fee:         actualFee,
		Timestamp:   time.Now().Unix(),
	}

	writeSuccess(w, response)
}

// ============================================================================
// Helper functions
// ============================================================================

// mempoolEntryToInfo converts a mempool entry to transaction info
func mempoolEntryToInfo(entry *utxo.MempoolEntry) TransactionInfo {
	return transactionToInfo(entry.Tx)
}

// transactionToInfo converts a transaction to transaction info
func transactionToInfo(tx *utxo.Transaction) TransactionInfo {
	inputs := make([]TxInput, len(tx.Inputs))
	for i, input := range tx.Inputs {
		inputs[i] = TxInput{
			TxHash:      hex.EncodeToString(input.TxHash[:]),
			OutputIndex: input.Index,
			Signature:   hex.EncodeToString(input.Signature),
			PublicKey:   hex.EncodeToString(input.PublicKey),
		}
	}

	outputs := make([]TxOutput, len(tx.Outputs))
	for i, output := range tx.Outputs {
		outputs[i] = TxOutput{
			Value:   output.Value,
			Address: hex.EncodeToString(output.Address[:]),
			PubKey:  "", // TXOutput has no PubKey field
			Index:   uint32(i),
			IsSpent: false, // needs to be queried from the UTXO store
		}
	}

	hash := tx.Hash()
	return TransactionInfo{
		Hash:     hex.EncodeToString(hash[:]),
		Version:  tx.Version,
		Inputs:   inputs,
		Outputs:  outputs,
		LockTime: tx.LockTime,
		Sequence: tx.Sequence,
		Size:     tx.SerializeSize(),
	}
}

// txIsRelatedToAddress checks whether a transaction relates to an address
func txIsRelatedToAddress(tx *utxo.Transaction, address string) bool {
	addrBytes, err := hex.DecodeString(address)
	if err != nil || len(addrBytes) != 32 {
		return false
	}

	var addrArray [32]byte
	copy(addrArray[:], addrBytes)

	// check inputs
	for _, input := range tx.Inputs {
		if bytesEqual(input.PublicKey, addrBytes) {
			return true
		}
	}

	// check outputs
	for _, output := range tx.Outputs {
		if output.Address == addrArray {
			return true
		}
	}

	return false
}

// calculateTxFee computes the transaction fee
func calculateTxFee(tx *utxo.Transaction, block *utxo.Block) uint64 {
	// compute the sum of inputs
	var inputSum uint64
	for range tx.Inputs {
		// need to get the value of the referenced output from the UTXO store
		// this is a simplified version
		// TODO: the real implementation needs to query the UTXO store
	}

	// compute the sum of outputs
	outputSum := uint64(0)
	for _, output := range tx.Outputs {
		outputSum += output.Value
	}

	// fee = inputs - outputs
	if inputSum > outputSum {
		return inputSum - outputSum
	}
	return 0
}

// bytesEqual compares byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
