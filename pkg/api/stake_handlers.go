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
// 质押相关常量
// ============================================================================

const (
	// MinStakeAmount 最小质押金额 (1000 AIB in smallest units)
	MinStakeAmount = 1000 * 100000000

	// UnstakeUnlockPeriod 解锁期（约 7 天，假设 60 秒/区块）
	// 7 * 24 * 60 * 60 / 60 = 10080 个区块
	UnstakeUnlockBlocks = 10080

	// StakingScriptType 质押脚本类型标识
	StakingScriptType = "staking"
)

// ============================================================================
// 请求/响应结构
// ============================================================================

// StakeRequest 质押请求
type StakeRequest struct {
	PrivateKey string `json:"private_key"`
	Amount     string `json:"amount"` // 字符串格式的金额（最小单位）
}

// StakeResponse 质押响应
type StakeResponse struct {
	TxHash       string `json:"tx_hash"`
	StakeID      string `json:"stake_id"`
	Amount       string `json:"amount"`
	UnlockHeight uint64 `json:"unlock_height"`
	Status       string `json:"status"`
	Address      string `json:"address"`
	Timestamp    string `json:"timestamp"`
}

// UnstakeRequest 解质押请求
type UnstakeRequest struct {
	PrivateKey string `json:"private_key"`
	Amount     string `json:"amount"` // 解质押金额
}

// UnstakeResponse 解质押响应
type UnstakeResponse struct {
	TxHash       string `json:"tx_hash"`
	UnstakeID    string `json:"unstake_id"`
	Amount       string `json:"amount"`
	UnlockHeight uint64 `json:"unlock_height"`
	Status       string `json:"status"`
	Address      string `json:"address"`
	Timestamp    string `json:"timestamp"`
}

// StakeInfo 质押信息
type StakeInfo struct {
	StakeID        string `json:"stake_id"`
	Amount         string `json:"amount"`
	StakedAtHeight uint64 `json:"staked_at_height"`
	Status         string `json:"status"` // "staked", "unstaking", "unlocked"
}

// UnstakeInfo 解质押信息
type UnstakeInfo struct {
	UnstakeID         string `json:"unstake_id"`
	Amount            string `json:"amount"`
	UnstakingAtHeight uint64 `json:"unstaking_at_height"`
	UnlockHeight      uint64 `json:"unlock_height"`
	Status            string `json:"status"`
}

// WalletStakeResponse 钱包质押状态响应
type WalletStakeResponse struct {
	Address        string       `json:"address"`
	TotalStaked    string       `json:"total_staked"`
	TotalUnstaking string       `json:"total_unstaking"`
	PendingRewards string       `json:"pending_rewards"`
	Stakes         []StakeInfo  `json:"stakes"`
	Unstaking      []UnstakeInfo `json:"unstaking"`
}

// ============================================================================
// 质押处理器
// ============================================================================

// handleStake 处理质押请求
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

	// 验证请求参数
	if req.PrivateKey == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Private key is required", "")
		return
	}
	if req.Amount == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount is required", "")
		return
	}

	// 解析私钥
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// 创建钱包
	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to create wallet", err.Error())
		return
	}

	// 解析金额
	amount, err := strconv.ParseUint(req.Amount, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid amount format", err.Error())
		return
	}

	// 验证最小质押金额
	if amount < MinStakeAmount {
		minStakeAIB := MinStakeAmount / uint64(100000000) // 1e8
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount below minimum stake",
			fmt.Sprintf("Minimum stake is %d AIB (%d units)", minStakeAIB, MinStakeAmount))
		return
	}

	// 获取必要的组件
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// 使用类型断言获取实际的 UTXO Store
	utxoStore, ok := s.utxoStore.(interface {
		GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error)
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not support GetUTXOsForAmount", "")
		return
	}

	// 获取用户地址
	address := walletSDK.GetAddress()
	addressHex := hex.EncodeToString(address[:])

	// 计算费用
	feePerByte := uint64(1)
	estimatedTxSize := uint64(200)
	actualFee := feePerByte * estimatedTxSize

	// 获取需要的总金额（质押金额 + 手续费）
	totalNeeded := amount + actualFee

	// 选择 UTXO
	selectedUTXOs, totalValue, err := utxoStore.GetUTXOsForAmount(address, totalNeeded)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Failed to select UTXOs", err.Error())
		return
	}

	// 构建交易输入
	inputs := make([]utxo.TXInput, len(selectedUTXOs))
	for i, u := range selectedUTXOs {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// 构建交易输出
	// 输出 0: 质押输出（特殊脚本类型）
	// 输出 1: 找零（如果有）
	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: address,
		},
	}

	// 设置质押脚本
	// 注意：这里需要特殊处理质押输出
	// 暂时使用普通输出，后续可以通过修改 Script 字段来实现特殊脚本类型

	// 计算找零
	changeAmount := totalValue - amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: address,
		})
	}

	// 创建交易
	tx := utxo.NewTransaction(inputs, outputs)

	// 设置质押输出索引 0 的脚本为质押类型
	tx.Outputs[0].Script = []byte(StakingScriptType)

	// 签名所有输入
	privKey := ed25519.PrivateKey(privateKey)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign transaction", err.Error())
			return
		}
	}

	// 提交到内存池
	if s.mempool == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool not available", "")
		return
	}

	// 使用类型断言获取实际的 Mempool
	actualMempool, ok := s.mempool.(interface {
		AddTransaction(tx *utxo.Transaction, utxoProvider utxo.UTXOProvider) error
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool does not support AddTransaction", "")
		return
	}

	// 获取 UTXOProvider
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not implement UTXOProvider", "")
		return
	}

	// 添加交易到 mempool
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// 计算交易哈希
	txHash := tx.Hash()

	// 构建响应
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

// handleUnstake 处理解质押请求
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

	// 验证请求参数
	if req.PrivateKey == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Private key is required", "")
		return
	}
	if req.Amount == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount is required", "")
		return
	}

	// 解析私钥
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// 创建钱包
	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to create wallet", err.Error())
		return
	}

	// 解析金额
	amount, err := strconv.ParseUint(req.Amount, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid amount format", err.Error())
		return
	}

	// 获取必要的组件
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// 获取用户地址
	address := walletSDK.GetAddress()
	addressHex := hex.EncodeToString(address[:])

	// 获取所有 UTXO
	allUTXOs := s.utxoStore.GetAllUTXOs(address)

	// 筛选质押类型的 UTXO
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

	// 选择质押 UTXO
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

	// 构建交易输入
	inputs := make([]utxo.TXInput, len(selectedUTXOs))
	for i, u := range selectedUTXOs {
		inputs[i] = utxo.TXInput{
			TxHash: u.TxHash,
			Index:  u.Index,
		}
	}

	// 计算费用
	feePerByte := uint64(1)
	estimatedTxSize := uint64(200)
	actualFee := feePerByte * estimatedTxSize

	// 构建交易输出
	// 输出 0: 解质押输出
	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: address,
		},
	}

	// 计算找零
	changeAmount := totalStaked - amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: address,
		})
	}

	// 获取当前区块高度
	currentHeight := uint64(0)
	if chain := s.GetChain(); chain != nil {
		if h, err := chain.GetBestBlockHeight(); err == nil {
			currentHeight = h
		}
	}

	// 计算解锁高度
	unlockHeight := currentHeight + UnstakeUnlockBlocks

	// 创建交易
	tx := utxo.NewTransaction(inputs, outputs)
	tx.LockTime = uint32(unlockHeight) // 设置解锁时间

	// 设置解质押输出脚本
	tx.Outputs[0].Script = []byte("unstake")

	// 签名所有输入
	privKey := ed25519.PrivateKey(privateKey)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign transaction", err.Error())
			return
		}
	}

	// 提交到内存池
	if s.mempool == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool not available", "")
		return
	}

	// 使用类型断言获取实际的 Mempool
	actualMempool, ok := s.mempool.(interface {
		AddTransaction(tx *utxo.Transaction, utxoProvider utxo.UTXOProvider) error
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Mempool does not support AddTransaction", "")
		return
	}

	// 获取 UTXOProvider
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not implement UTXOProvider", "")
		return
	}

	// 添加交易到 mempool
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// 计算交易哈希
	txHash := tx.Hash()

	// 构建响应
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

// handleGetStake 查询质押状态
// GET /v1/wallet/stake?address=0x...
func (s *Server) handleGetStake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// 获取地址参数
	addressStr := r.URL.Query().Get("address")
	if addressStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Address parameter is required", "")
		return
	}

	// 解码地址
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

	// 获取 UTXO 存储
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// 获取所有 UTXO
	utxos := s.utxoStore.GetAllUTXOs(address)

	// 分类统计
	var totalStaked uint64
	var totalUnstaking uint64
	var stakes []StakeInfo
	var unstaking []UnstakeInfo

	// 当前区块高度
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
			// 活跃质押
			totalStaked += u.Value
			stakes = append(stakes, StakeInfo{
				StakeID:        hex.EncodeToString(u.TxHash[:]) + ":" + strconv.FormatUint(uint64(u.Index), 10),
				Amount:         strconv.FormatUint(u.Value, 10),
				StakedAtHeight: 0,
				Status:         "staked",
			})

		case "unstake":
			// 解质押中
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

	// 从 mempool 获取待处理的质押/解质押
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

	// 获取待领取奖励（从共识状态）
	// 注意：由于接口限制，这里暂时返回 0
	var pendingRewards uint64

	// 构建响应
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
