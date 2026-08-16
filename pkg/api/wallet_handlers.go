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
// 交易查询处理器
// ============================================================================

// TransactionListRequest 交易列表请求参数
type TransactionListRequest struct {
	Address string `json:"address"`    // 可选：按地址过滤
	Limit   int    `json:"limit"`      // 可选：限制数量，默认 100
	Offset  int    `json:"offset"`     // 可选：偏移量，默认 0
}

// TransactionInfo 交易信息
type TransactionInfo struct {
	Hash      string       `json:"hash"`
	Version   uint32       `json:"version"`
	Inputs    []TxInput    `json:"inputs"`
	Outputs   []TxOutput   `json:"outputs"`
	LockTime  uint32       `json:"lock_time"`
	Sequence  uint64       `json:"sequence"`
	Timestamp *uint64      `json:"timestamp,omitempty"` // 如果在区块中
	BlockHash *string      `json:"block_hash,omitempty"` // 如果在区块中
	Height    *uint64      `json:"height,omitempty"`     // 如果在区块中
	Fee       *uint64      `json:"fee,omitempty"`        // 交易费用
	Size      int          `json:"size"`                 // 交易大小（字节）
}

// TxInput 交易输入
type TxInput struct {
	TxHash      string `json:"tx_hash"`
	OutputIndex uint32 `json:"output_index"`
	Signature   string `json:"signature"`
	PublicKey   string `json:"public_key"`
}

// TxOutput 交易输出
type TxOutput struct {
	Value     uint64 `json:"value"`
	Address   string `json:"address"`
	PubKey    string `json:"pub_key"`
	Index     uint32 `json:"index"`
	IsSpent   bool   `json:"is_spent"`
	SpentBy   *string `json:"spent_by,omitempty"`
}

// TransactionListResponse 交易列表响应
type TransactionListResponse struct {
	Transactions []TransactionInfo `json:"transactions"`
	Total        int              `json:"total"`
	Limit        int              `json:"limit"`
	Offset       int              `json:"offset"`
}

// TransactionDetailResponse 交易详情响应
type TransactionDetailResponse struct {
	Transaction TransactionInfo `json:"transaction"`
	Confirmations *uint64      `json:"confirmations,omitempty"`
}

// handleTransactionsList 处理交易列表查询
// GET /v1/transactions?address={address}&limit={limit}&offset={offset}
func (s *Server) handleTransactionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// 解析查询参数
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

	// 从内存池获取待确认交易
	if s.mempool != nil {
		entries := s.mempool.GetAllEntries()
		for _, entry := range entries[:min(len(entries), limit)] {
			if address != "" {
				// 检查交易是否与地址相关
				if !txIsRelatedToAddress(entry.Tx, address) {
					continue
				}
			}

			txInfos = append(txInfos, mempoolEntryToInfo(entry))
		}
	}

	// TODO: 从已确认的区块中获取交易
	// 需要遍历区块链查找相关交易

	total = len(txInfos)

	writeSuccess(w, TransactionListResponse{
		Transactions: txInfos,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	})
}

// handleTransactionDetail 处理交易详情查询
// GET /v1/transaction/{hash}
func (s *Server) handleTransactionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// 从 URL 路径中提取交易哈希
	// URL 格式: /v1/transaction/{hash}
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

	// 首先在内存池中查找
	if s.mempool != nil {
		if tx := s.mempool.GetTransaction(hashArray); tx != nil {
			info := transactionToInfo(tx)
			txInfo = &info
			zero := uint64(0)
			confirmations = &zero
		}
	}

	// 如果在内存池中未找到，在区块链中查找
	if txInfo == nil && s.chain != nil && s.utxoStore != nil {
		if height, err := s.utxoStore.GetTransactionIndex(hashArray); err == nil {
			// 找到交易所在区块的高度
			bestHeight, _ := s.chain.GetBestBlockHeight()
			confirmationsVal := uint64(0)
			if bestHeight >= height {
				confirmationsVal = bestHeight - height + 1
			}

			// 获取区块中的交易 - 需要通过其他方式
			// 简化：直接从 utxoStore 获取交易信息
			// 暂时返回基本信息，不包含完整交易详情
			txInfo = &TransactionInfo{
				Hash:    hashStr,
				Version: 0, // 需要从存储获取
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
		Transaction: *txInfo,
		Confirmations: confirmations,
	})
}

// ============================================================================
// 钱包管理处理器
// ============================================================================

// CreateWalletRequest 创建钱包请求
type CreateWalletRequest struct {
	Password string `json:"password"` // 加密密码（可选）
	Label    string `json:"label"`    // 钱包标签
}

// CreateWalletResponse 创建钱包响应
type CreateWalletResponse struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	Label      string `json:"label,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// RestoreWalletRequest 恢复钱包请求
type RestoreWalletRequest struct {
	PrivateKey string `json:"private_key"` // 私钥的十六进制字符串
	Password   string `json:"password"`    // 加密密码（可选）
	Label      string `json:"label"`       // 钱包标签
}

// ImportWalletRequest 导入钱包请求（从助记词或密钥文件）
type ImportWalletRequest struct {
	Source   string `json:"source"`   // 导入源：mnemonic, keystore, private_key
	Data     string `json:"data"`     // 导入数据
	Password string `json:"password"` // 解密密码（如需要）
	Label    string `json:"label"`    // 钱包标签
}

// ExportWalletRequest 导出钱包请求
type ExportWalletRequest struct {
	Address  string `json:"address"`  // 钱包地址
	Format   string `json:"format"`   // 导出格式：private_key, keystore
	Password string `json:"password"` // 加密密码（如需要）
}

// ExportWalletResponse 导出钱包响应
type ExportWalletResponse struct {
	Data      string `json:"data"`      // 导出的数据
	Format    string `json:"format"`    // 格式类型
	Timestamp int64  `json:"timestamp"` // 导出时间
}

// WalletBalanceRequest 钱包余额请求
type WalletBalanceRequest struct {
	Address string `json:"address"` // 钱包地址
}

// WalletBalanceResponse 钱包余额响应
type WalletBalanceResponse struct {
	Address           string  `json:"address"`
	Balance           uint64  `json:"balance"`            // 可用余额（最小单位）
	BalanceAIB        float64 `json:"balance_aib"`        // 可用余额（AIB）
	Unconfirmed       uint64  `json:"unconfirmed"`        // 未确认余额
	UnconfirmedAIB    float64 `json:"unconfirmed_aib"`    // 未确认余额（AIB）
	UTXOCount         int     `json:"utxo_count"`         // UTXO 数量
	PendingUTXOCount  int     `json:"pending_utxo_count"` // 待确认 UTXO 数量
}

// SendTransactionRequest 发送交易请求
type SendTransactionRequest struct {
	FromAddress string `json:"from_address"` // 发送方地址
	ToAddress   string `json:"to_address"`   // 接收方地址
	Amount      uint64 `json:"amount"`       // 金额（最小单位）
	Fee         uint64 `json:"fee"`          // 交易费用（可选，自动计算）
	PrivateKey  string `json:"private_key"`  // 私钥（用于签名）
	Memo        string `json:"memo"`        // 备注（可选）
}

// SendTransactionResponse 发送交易响应
type SendTransactionResponse struct {
	TxHash      string `json:"tx_hash"`       // 交易哈希
	FromAddress string `json:"from_address"`  // 发送方地址
	ToAddress   string `json:"to_address"`    // 接收方地址
	Amount      uint64 `json:"amount"`        // 金额
	Fee         uint64 `json:"fee"`           // 实际费用
	Timestamp   int64  `json:"timestamp"`     // 提交时间
}

// handleCreateWallet 处理创建钱包
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

	// 创建新钱包
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
		Label:      req.Label,
		CreatedAt:  time.Now().Unix(),
	}

	// TODO: 如果提供了密码，应该加密存储私钥
	// 这需要实现钱包存储功能

	writeSuccess(w, response)
}

// handleRestoreWallet 处理恢复钱包
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

	// 解码私钥
	privateKey, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key format", "")
		return
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key length", "")
		return
	}

	// 从私钥恢复钱包
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
		Address:    hex.EncodeToString(address[:]),
		PublicKey:  hex.EncodeToString(pubKey),
		Label:      req.Label,
		CreatedAt:  time.Now().Unix(),
	}

	writeSuccess(w, response)
}

// handleImportWallet 处理导入钱包
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
		// 直接导入私钥
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
		// TODO: 实现 keystore 格式导入
		writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Keystore import not implemented", "")
		return

	case "mnemonic":
		// TODO: 实现助记词导入
		writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Mnemonic import not implemented", "")
		return

	default:
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid import source", "Supported sources: private_key, keystore, mnemonic")
		return
	}
}

// handleExportWallet 处理导出钱包
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

	// TODO: 实现钱包导出功能
	// 需要钱包存储功能来检索私钥

	writeError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "Wallet export not implemented", "Requires wallet storage")
}

// handleWalletBalance 处理钱包余额查询
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

	// 获取已确认的 UTXO
	var balance uint64
	utxoCount := 0

	if s.utxoStore != nil {
		utxos := s.utxoStore.GetAllUTXOs(addrArray)
		utxoCount = len(utxos)
		for _, utxo := range utxos {
			balance += utxo.Value
		}
	}

	// 获取未确认的 UTXO（从内存池）
	var unconfirmed uint64
	pendingUTXOCount := 0

	if s.mempool != nil {
		// 计算待确认的余额
		// 这需要分析内存池中的交易
		entries := s.mempool.GetAllEntries()
		for _, entry := range entries {
			// 检查交易的输出是否发送到该地址
			for _, output := range entry.Tx.Outputs {
				if output.Address == addrArray {
					unconfirmed += output.Value
					pendingUTXOCount++
				}
			}
			// 检查交易的输入是否花费了该地址的 UTXO
			for range entry.Tx.Inputs {
				// 需要查找输入引用的 UTXO 的地址
				// 这需要访问 UTXO 存储来获取引用的输出
			}
		}
	}

	response := WalletBalanceResponse{
		Address:           address,
		Balance:           balance,
		BalanceAIB:        float64(balance) / 1e6, // 假设 1 AIB = 1,000,000 最小单位
		Unconfirmed:       unconfirmed,
		UnconfirmedAIB:    float64(unconfirmed) / 1e6,
		UTXOCount:         utxoCount,
		PendingUTXOCount:  pendingUTXOCount,
	}

	writeSuccess(w, response)
}

// handleSendTransaction 处理发送交易
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

	// 解码私钥
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

	// 验证发送地址
	fromAddr := walletSDK.GetAddress()
	fromAddrStr := hex.EncodeToString(fromAddr[:])
	if req.FromAddress != "" && req.FromAddress != fromAddrStr {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "From address does not match private key", "")
		return
	}

	// 解码接收地址
	toAddrBytes, err := hex.DecodeString(req.ToAddress)
	if err != nil || len(toAddrBytes) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid to address format", "")
		return
	}

	var toAddr [32]byte
	copy(toAddr[:], toAddrBytes)

	// 获取 UTXO Store 和 Mempool
	if s.utxoStore == nil {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// 使用类型断言获取实际的 UTXO Store 和 Mempool
	utxoStore, ok := s.utxoStore.(interface {
		GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error)
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store does not support GetUTXOsForAmount", "")
		return
	}

	// 计算费用
	feePerByte := uint64(1) // 默认费用率
	estimatedTxSize := uint64(200) // 估算交易大小
	var actualFee uint64
	if req.Fee > 0 {
		actualFee = req.Fee
	} else {
		actualFee = feePerByte * estimatedTxSize
	}

	// 获取需要的总金额（发送金额 + 手续费）
	totalNeeded := req.Amount + actualFee

	// 选择 UTXO
	selectedUTXOs, totalValue, err := utxoStore.GetUTXOsForAmount(fromAddr, totalNeeded)
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
	// 输出 0: 接收方
	// 输出 1: 找零（如果有）
	outputs := []utxo.TXOutput{
		{
			Value:   req.Amount,
			Address: toAddr,
		},
	}

	// 计算找零
	changeAmount := totalValue - req.Amount - actualFee
	if changeAmount > 0 {
		outputs = append(outputs, utxo.TXOutput{
			Value:   changeAmount,
			Address: fromAddr,
		})
	}

	// 创建交易
	tx := utxo.NewTransaction(inputs, outputs)

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

	// 添加交易到内存池
	if err := actualMempool.AddTransaction(tx, utxoProvider); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to add transaction to mempool", err.Error())
		return
	}

	// 计算交易哈希
	txHash := tx.Hash()

	// 返回成功响应
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
// 辅助函数
// ============================================================================

// mempoolEntryToInfo 将内存池条目转换为交易信息
func mempoolEntryToInfo(entry *utxo.MempoolEntry) TransactionInfo {
	return transactionToInfo(entry.Tx)
}

// transactionToInfo 将交易转换为交易信息
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
			PubKey:  "", // TXOutput 没有 PubKey 字段
			Index:   uint32(i),
			IsSpent: false, // 需要从 UTXO 存储查询
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

// txIsRelatedToAddress 检查交易是否与地址相关
func txIsRelatedToAddress(tx *utxo.Transaction, address string) bool {
	addrBytes, err := hex.DecodeString(address)
	if err != nil || len(addrBytes) != 32 {
		return false
	}

	var addrArray [32]byte
	copy(addrArray[:], addrBytes)

	// 检查输入
	for _, input := range tx.Inputs {
		if bytesEqual(input.PublicKey, addrBytes) {
			return true
		}
	}

	// 检查输出
	for _, output := range tx.Outputs {
		if output.Address == addrArray {
			return true
		}
	}

	return false
}

// calculateTxFee 计算交易费用
func calculateTxFee(tx *utxo.Transaction, block *utxo.Block) uint64 {
	// 计算输入总和
	var inputSum uint64
	for range tx.Inputs {
		// 需要从 UTXO 存储中获取引用的输出的值
		// 这是一个简化版本
		// TODO: 实际实现需要查询 UTXO 存储
	}

	// 计算输出总和
	outputSum := uint64(0)
	for _, output := range tx.Outputs {
		outputSum += output.Value
	}

	// 费用 = 输入 - 输出
	if inputSum > outputSum {
		return inputSum - outputSum
	}
	return 0
}

// bytesEqual 比较字节切片是否相等
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

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
