// Package api provides REST API handlers for AIB 2.0 staking operations.
// This file implements handlers for staking, unstaking, and stake queries.
package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/utxo"
	"github.com/aib-protocol/aib/pkg/wallet"
)

// ============================================================================
// Staking-related constants
// ============================================================================

const (
	// MinStakeAmount is the minimum staking amount (1000 AIB in smallest units)
	MinStakeAmount = 1000 * 100000000

	// UnstakeUnlockPeriod is the unlock period (approx. 7 days, assuming 60 seconds/block)
	// 7 * 24 * 60 * 60 / 60 = 10080 blocks
	UnstakeUnlockBlocks = 10080

	// StakingScriptType is the staking script type identifier
	StakingScriptType = "staking"
)

// ============================================================================
// Request/response structures
// ============================================================================

// StakeRequest stakerequest
type StakeRequest struct {
	PrivateKey string `json:"private_key"`
	Amount     string `json:"amount"` // Amount as a string (smallest units)
}

// StakeResponse stakeresponse
type StakeResponse struct {
	TxHash       string `json:"tx_hash"`
	StakeID      string `json:"stake_id"`
	Amount       string `json:"amount"`
	UnlockHeight uint64 `json:"unlock_height"`
	Status       string `json:"status"`
	Address      string `json:"address"`
	Timestamp    string `json:"timestamp"`
}

// UnstakeRequest is the unstaking request
type UnstakeRequest struct {
	PrivateKey string `json:"private_key"`
	Amount     string `json:"amount"` // Amount to unstake
}

// UnstakeResponse is the unstaking response
type UnstakeResponse struct {
	TxHash       string `json:"tx_hash"`
	UnstakeID    string `json:"unstake_id"`
	Amount       string `json:"amount"`
	UnlockHeight uint64 `json:"unlock_height"`
	Status       string `json:"status"`
	Address      string `json:"address"`
	Timestamp    string `json:"timestamp"`
}

// StakeInfo stake / stakinginfo
type StakeInfo struct {
	StakeID        string `json:"stake_id"`
	Amount         string `json:"amount"`
	StakedAtHeight uint64 `json:"staked_at_height"`
	Status         string `json:"status"` // "staked", "unstaking", "unlocked"
}

// UnstakeInfo holds unstaking information
type UnstakeInfo struct {
	UnstakeID         string `json:"unstake_id"`
	Amount            string `json:"amount"`
	UnstakingAtHeight uint64 `json:"unstaking_at_height"`
	UnlockHeight      uint64 `json:"unlock_height"`
	Status            string `json:"status"`
}

// WalletStakeResponse is the wallet staking status response
type WalletStakeResponse struct {
	Address        string        `json:"address"`
	TotalStaked    string        `json:"total_staked"`
	TotalUnstaking string        `json:"total_unstaking"`
	PendingRewards string        `json:"pending_rewards"`
	Stakes         []StakeInfo   `json:"stakes"`
	Unstaking      []UnstakeInfo `json:"unstaking"`
}

// ============================================================================
// Staking handlers
// ============================================================================

// handleStake handlestakerequest
// POST /v1/stake
func (s *Server) handleStake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req StakeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// verifyrequestparameter
	if req.PrivateKey == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Private key is required", "")
		return
	}
	if req.Amount == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount is required", "")
		return
	}

	// Parse the private key
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// Create the wallet
	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to create wallet", err.Error())
		return
	}

	// parseamount
	amount, err := strconv.ParseUint(req.Amount, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid amount format", err.Error())
		return
	}

	// Validate the minimum staking amount
	if amount < MinStakeAmount {
		minStakeAIB := MinStakeAmount / uint64(100000000) // 1e8
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount below minimum stake",
			fmt.Sprintf("Minimum stake is %d AIB (%d units)", minStakeAIB, MinStakeAmount))
		return
	}

	// Get the required components
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// Use a type assertion to get the actual UTXO store
	utxoStore, ok := s.utxoStore.(interface {
		GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error)
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not support GetUTXOsForAmount", "")
		return
	}

	// getuseraddress
	address := walletSDK.GetAddress()
	addressHex := hex.EncodeToString(address[:])

	// Calculate the fee
	feePerByte := uint64(1)
	estimatedTxSize := uint64(200)
	actualFee := feePerByte * estimatedTxSize

	// Get the total amount needed (stake amount + fee)
	totalNeeded := amount + actualFee

	// Select UTXOs
	selectedUTXOs, totalValue, err := utxoStore.GetUTXOsForAmount(address, totalNeeded)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Failed to select UTXOs", err.Error())
		return
	}

	// buildtransactioninput
	inputs := make([]utxo.TXInput, len(selectedUTXOs))
	for i, u := range selectedUTXOs {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// buildtransactionoutput
	// Output 0: staking output (special script type)
	// Output 1: change (if any)
	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: address,
		},
	}

	// Set the staking script
	// Note: the staking output requires special handling here
	// Use a normal output for now; a special script type can be implemented later by modifying the Script field

	// Calculate the change
	changeAmount := totalValue - amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: address,
		})
	}

	// createtransaction
	tx := utxo.NewTransaction(inputs, outputs)

	// Set the script of staking output at index 0 to the staking type
	tx.Outputs[0].Script = []byte(StakingScriptType)

	// Sign all inputs
	privKey := ed25519.PrivateKey(privateKey)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign transaction", err.Error())
			return
		}
	}

	// Submit to the mempool
	if s.mempool == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool not available", "")
		return
	}

	// Use a type assertion to get the actual mempool
	actualMempool, ok := s.mempool.(interface {
		AddTransaction(tx *utxo.Transaction, utxoProvider utxo.UTXOProvider) error
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool does not support AddTransaction", "")
		return
	}

	// get UTXOProvider
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not implement UTXOProvider", "")
		return
	}

	// Add the transaction to the mempool
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// Calculate the transaction hash
	txHash := tx.Hash()

	// buildresponse
	response := StakeResponse{
		TxHash:       hex.EncodeToString(txHash[:]),
		StakeID:      hex.EncodeToString(txHash[:]),
		Amount:       req.Amount,
		UnlockHeight: 0,
		Status:       "pending",
		Address:      addressHex,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	writeSuccess(w, response)
}

// handleUnstake handles unstaking requests
// POST /v1/unstake
func (s *Server) handleUnstake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req UnstakeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// verifyrequestparameter
	if req.PrivateKey == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Private key is required", "")
		return
	}
	if req.Amount == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount is required", "")
		return
	}

	// Parse the private key
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// Create the wallet
	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to create wallet", err.Error())
		return
	}

	// parseamount
	amount, err := strconv.ParseUint(req.Amount, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid amount format", err.Error())
		return
	}

	// Get the required components
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// getuseraddress
	address := walletSDK.GetAddress()
	addressHex := hex.EncodeToString(address[:])

	// Get all UTXOs
	allUTXOs := s.utxoStore.GetAllUTXOs(address)

	// Filter UTXOs of the staking type
	var stakedUTXOs []*utxo.UTXO
	for _, u := range allUTXOs {
		if len(u.Script) > 0 && string(u.Script) == StakingScriptType {
			stakedUTXOs = append(stakedUTXOs, u)
		}
	}

	if len(stakedUTXOs) == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "No staked amount found", "No staking UTXOs available")
		return
	}

	// Select staked UTXOs
	var selectedUTXOs []*utxo.UTXO
	var totalStaked uint64

	for _, u := range stakedUTXOs {
		selectedUTXOs = append(selectedUTXOs, u)
		totalStaked += u.Value

		if totalStaked >= amount {
			break
		}
	}

	if totalStaked < amount {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Insufficient staked amount",
			fmt.Sprintf("Requested: %d, Staked: %d", amount, totalStaked))
		return
	}

	// buildtransactioninput
	inputs := make([]utxo.TXInput, len(selectedUTXOs))
	for i, u := range selectedUTXOs {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// Calculate the fee
	feePerByte := uint64(1)
	estimatedTxSize := uint64(200)
	actualFee := feePerByte * estimatedTxSize

	// buildtransactionoutput
	// Output 0: unstaking output
	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: address,
		},
	}

	// Calculate the change
	changeAmount := totalStaked - amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: address,
		})
	}

	// Get the current block height
	currentHeight := uint64(0)
	if chain := s.GetChain(); chain != nil {
		if h, err := chain.GetBestBlockHeight(); err == nil {
			currentHeight = h
		}
	}

	// Calculate the unlock height
	unlockHeight := currentHeight + UnstakeUnlockBlocks

	// createtransaction
	tx := utxo.NewTransaction(inputs, outputs)
	tx.LockTime = uint32(unlockHeight) // Set the lock time

	// Set the unstaking output script
	tx.Outputs[0].Script = []byte("unstake")

	// Sign all inputs
	privKey := ed25519.PrivateKey(privateKey)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign transaction", err.Error())
			return
		}
	}

	// Submit to the mempool
	if s.mempool == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool not available", "")
		return
	}

	// Use a type assertion to get the actual mempool
	actualMempool, ok := s.mempool.(interface {
		AddTransaction(tx *utxo.Transaction, utxoProvider utxo.UTXOProvider) error
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool does not support AddTransaction", "")
		return
	}

	// get UTXOProvider
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not implement UTXOProvider", "")
		return
	}

	// Add the transaction to the mempool
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// Calculate the transaction hash
	txHash := tx.Hash()

	// buildresponse
	response := UnstakeResponse{
		TxHash:       hex.EncodeToString(txHash[:]),
		UnstakeID:    hex.EncodeToString(txHash[:]),
		Amount:       req.Amount,
		UnlockHeight: unlockHeight,
		Status:       "unstaking",
		Address:      addressHex,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	writeSuccess(w, response)
}

// handleGetStake querystakestatus
// GET /v1/wallet/stake?address=0x...
func (s *Server) handleGetStake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// getaddressparameter
	addressStr := r.URL.Query().Get("address")
	if addressStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Address parameter is required", "")
		return
	}

	// decodeaddress
	var address interfaces.Address
	addrBytes, err := hex.DecodeString(addressStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", err.Error())
		return
	}

	if len(addrBytes) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address length", "Expected 32 bytes")
		return
	}
	copy(address[:], addrBytes)

	// Get the UTXO store
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// Get all UTXOs
	utxos := s.utxoStore.GetAllUTXOs(address)

	// Categorize and tally
	var totalStaked uint64
	var totalUnstaking uint64
	var stakes []StakeInfo
	var unstaking []UnstakeInfo

	// Current block height
	currentHeight := uint64(0)
	if chain := s.GetChain(); chain != nil {
		if h, err := chain.GetBestBlockHeight(); err == nil {
			currentHeight = h
		}
	}

	for _, u := range utxos {
		scriptType := string(u.Script)

		switch scriptType {
		case StakingScriptType:
			// Active stakes
			totalStaked += u.Value
			stakes = append(stakes, StakeInfo{
				StakeID:        hex.EncodeToString(u.TxHash[:]) + ":" + strconv.FormatUint(uint64(u.Index), 10),
				Amount:         strconv.FormatUint(u.Value, 10),
				StakedAtHeight: 0,
				Status:         "staked",
			})

		case "unstake":
			// Unstaking
			totalUnstaking += u.Value
			unstaking = append(unstaking, UnstakeInfo{
				UnstakeID:         hex.EncodeToString(u.TxHash[:]) + ":" + strconv.FormatUint(uint64(u.Index), 10),
				Amount:            strconv.FormatUint(u.Value, 10),
				UnstakingAtHeight: 0,
				UnlockHeight:      0,
				Status:            "unstaking",
			})
		}
	}

	// Get pending stakes/unstakes from the mempool
	if s.mempool != nil {
		entries := s.mempool.GetAllEntries()
		for _, entry := range entries {
			txHash := entry.Tx.Hash()
			for i, output := range entry.Tx.Outputs {
				if output.Address == address {
					scriptType := string(output.Script)
					if scriptType == StakingScriptType {
						totalStaked += output.Value
						stakes = append(stakes, StakeInfo{
							StakeID:        "mempool:" + hex.EncodeToString(txHash[:]) + ":" + strconv.Itoa(i),
							Amount:         strconv.FormatUint(output.Value, 10),
							StakedAtHeight: 0,
							Status:         "pending",
						})
					} else if scriptType == "unstake" {
						totalUnstaking += output.Value
						unstaking = append(unstaking, UnstakeInfo{
							UnstakeID:         "mempool:" + hex.EncodeToString(txHash[:]) + ":" + strconv.Itoa(i),
							Amount:            strconv.FormatUint(output.Value, 10),
							UnstakingAtHeight: currentHeight,
							UnlockHeight:      uint64(entry.Tx.LockTime),
							Status:            "unstaking",
						})
					}
				}
			}
		}
	}

	// Get pending rewards (from consensus state)
	// Note: returns 0 for now due to interface limitations
	var pendingRewards uint64

	// buildresponse
	response := WalletStakeResponse{
		Address:        addressStr,
		TotalStaked:    strconv.FormatUint(totalStaked, 10),
		TotalUnstaking: strconv.FormatUint(totalUnstaking, 10),
		PendingRewards: strconv.FormatUint(pendingRewards, 10),
		Stakes:         stakes,
		Unstaking:      unstaking,
	}

	writeSuccess(w, response)
}
