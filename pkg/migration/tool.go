package migration

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// 工具错误定义
// ============================================================================

var (
	ErrInvalidFormat       = errors.New("invalid format")
	ErrEmptyData           = errors.New("empty data")
	ErrInvalidAddress      = errors.New("invalid address")
	ErrInvalidBalance      = errors.New("invalid balance")
	ErrRecordCountMismatch = errors.New("record count mismatch")
	ErrValidationFailed   = errors.New("validation failed")
	ErrFileNotFound        = errors.New("file not found")
	ErrInvalidJSON         = errors.New("invalid JSON")
	ErrInvalidCSV          = errors.New("invalid CSV")
)

// ExportFormat 定义导出格式
type ExportFormat string

const (
	FormatJSON ExportFormat = "json"
	FormatCSV  ExportFormat = "csv"
)

// ============================================================================
// 快照导出结构
// ============================================================================

// SnapshotExport 格式快照导出
type SnapshotExport struct {
	Version      string           `json:"version"`
	Timestamp    time.Time        `json:"timestamp"`
	TotalRecords int              `json:"total_records"`
	TotalBalance uint64           `json:"total_balance"`
	Records      []SnapshotRecord `json:"records"`
	MerkleRoot   string           `json:"merkle_root,omitempty"`
}

// StatusExport 状态导出格式
type StatusExport struct {
	Version                string    `json:"version"`
	Timestamp             time.Time `json:"timestamp"`
	AIB1                  AIB1Status `json:"aib1"`
	CrossChain            CrossChainStatus `json:"cross_chain"`
}

// AIB1Status AIB1 迁移状态
type AIB1Status struct {
	TotalMigrated uint64 `json:"total_migrated"`
	ClaimOpen     bool   `json:"claim_open"`
	ClaimDeadline time.Time `json:"claim_deadline"`
	TotalAccounts int    `json:"total_accounts"`
}

// CrossChainStatus 跨链迁移状态
type CrossChainStatus struct {
	BTC BTCStatus `json:"btc"`
	ETH ETHStatus `json:"eth"`
	SOL SOLStatus `json:"sol"`
}

// BTCStatus BTC 迁移状态
type BTCStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// ETHStatus ETH 迁移状态
type ETHStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// SOLStatus SOL 迁移状态
type SOLStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// ============================================================================
// 验证报告
// ============================================================================

// ValidationReport 验证报告
type ValidationReport struct {
	Timestamp      time.Time          `json:"timestamp"`
	TotalRecords   int                `json:"total_records"`
	ValidRecords   int                `json:"valid_records"`
	InvalidRecords int                `json:"invalid_records"`
	Errors         []ValidationError  `json:"errors"`
	Warnings       []ValidationWarning `json:"warnings"`
}

// ValidationError 验证错误
type ValidationError struct {
	Index   int    `json:"index"`
	Address string `json:"address"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ValidationWarning 验证警告
type ValidationWarning struct {
	Index   int    `json:"index"`
	Address string `json:"address"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ============================================================================
// MigrationTool 主工具结构
// ============================================================================

// MigrationTool 数据迁移工具
// 提供快照导出、导入、验证功能
type MigrationTool struct {
	mu         sync.RWMutex
	migration  *MigrationHub
	aib1       *AIB1Migration
	snapshot   []SnapshotRecord
	hasher     crypto.Hasher
}

// NewMigrationTool 创建新的迁移工具
func NewMigrationTool(hub *MigrationHub) *MigrationTool {
	return &MigrationTool{
		migration: hub,
		hasher:   crypto.NewSHA256d(),
	}
}

// NewMigrationToolWithSnapshot 从快照记录创建迁移工具
func NewMigrationToolWithSnapshot(records []SnapshotRecord) *MigrationTool {
	return &MigrationTool{
		snapshot: records,
		hasher:   crypto.NewSHA256d(),
	}
}

// ============================================================================
// 导出功能
// ============================================================================

// ExportSnapshotJSON 导出快照为 JSON 格式
func (t *MigrationTool) ExportSnapshotJSON(w io.Writer) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	records := t.snapshot
	export := SnapshotExport{
		Version:      "1.0",
		Timestamp:    time.Now(),
		TotalRecords: len(records),
		Records:      records,
	}

	// 计算总余额
	var totalBalance uint64
	for _, r := range records {
		totalBalance += r.Balance
	}
	export.TotalBalance = totalBalance

	// 计算 Merkle 根
	if len(records) > 0 {
		merkleRoot, err := t.computeMerkleRoot(records)
		if err == nil {
			export.MerkleRoot = hex.EncodeToString(merkleRoot)
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(export)
}

// ExportSnapshotCSV 导出快照为 CSV 格式
func (t *MigrationTool) ExportSnapshotCSV(w io.Writer) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// 写入表头
	if err := writer.Write([]string{"address", "balance"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// 写入数据
	for _, record := range t.snapshot {
		row := []string{
			addressToHex(record.Address),
			fmt.Sprintf("%d", record.Balance),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	return nil
}

// ExportStatusJSON 导出迁移状态为 JSON 格式
func (t *MigrationTool) ExportStatusJSON(w io.Writer) error {
	if t.migration == nil {
		return errors.New("migration hub not initialized")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	status := t.migration.GetMigrationStatus()
	export := StatusExport{
		Version:  "1.0",
		Timestamp: time.Now(),
		AIB1: AIB1Status{
			TotalMigrated: status.AIB1TotalMigrated,
			ClaimOpen:     status.AIB1ClaimOpen,
			ClaimDeadline: status.AIB1ClaimDeadline,
			TotalAccounts: len(t.snapshot),
		},
		CrossChain: CrossChainStatus{
			BTC: BTCStatus{
				TotalMigrated: status.BTCTotalMigrated,
				TotalRewards:  status.BTCTotalRewards,
				WindowOpen:    status.BTCWindowOpen,
				CurrentRate:   status.BTCCurrentRate,
			},
			ETH: ETHStatus{
				TotalMigrated: status.ETHTotalMigrated,
				TotalRewards:  status.ETHTotalRewards,
				WindowOpen:    status.ETHWindowOpen,
				CurrentRate:   status.ETHCurrentRate,
			},
			SOL: SOLStatus{
				TotalMigrated: status.SOLTotalMigrated,
				TotalRewards:  status.SOLTotalRewards,
				WindowOpen:    status.SOLWindowOpen,
				CurrentRate:   status.SOLCurrentRate,
			},
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(export)
}

// ExportStatusCSV 导出迁移状态为 CSV 格式
func (t *MigrationTool) ExportStatusCSV(w io.Writer) error {
	if t.migration == nil {
		return errors.New("migration hub not initialized")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// 写入表头
	if err := writer.Write([]string{"chain", "total_migrated", "total_rewards", "window_open", "current_rate"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	status := t.migration.GetMigrationStatus()

	// 写入 BTC
	if err := writer.Write([]string{
		"BTC",
		fmt.Sprintf("%d", status.BTCTotalMigrated),
		fmt.Sprintf("%d", status.BTCTotalRewards),
		fmt.Sprintf("%t", status.BTCWindowOpen),
		fmt.Sprintf("%d", status.BTCCurrentRate),
	}); err != nil {
		return fmt.Errorf("failed to write BTC record: %w", err)
	}

	// 写入 ETH
	if err := writer.Write([]string{
		"ETH",
		fmt.Sprintf("%d", status.ETHTotalMigrated),
		fmt.Sprintf("%d", status.ETHTotalRewards),
		fmt.Sprintf("%t", status.ETHWindowOpen),
		fmt.Sprintf("%d", status.ETHCurrentRate),
	}); err != nil {
		return fmt.Errorf("failed to write ETH record: %w", err)
	}

	// 写入 SOL
	if err := writer.Write([]string{
		"SOL",
		fmt.Sprintf("%d", status.SOLTotalMigrated),
		fmt.Sprintf("%d", status.SOLTotalRewards),
		fmt.Sprintf("%t", status.SOLWindowOpen),
		fmt.Sprintf("%d", status.SOLCurrentRate),
	}); err != nil {
		return fmt.Errorf("failed to write SOL record: %w", err)
	}

	return nil
}

// ExportToFile 导出到文件（自动识别格式）
func (t *MigrationTool) ExportToFile(filename string, format ExportFormat) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	switch format {
	case FormatJSON:
		return t.ExportSnapshotJSON(file)
	case FormatCSV:
		return t.ExportSnapshotCSV(file)
	default:
		return ErrInvalidFormat
	}
}

// ============================================================================
// 导入功能
// ============================================================================

// ImportSnapshotJSON 从 JSON 导入快照
func (t *MigrationTool) ImportSnapshotJSON(r io.Reader) ([]SnapshotRecord, error) {
	decoder := json.NewDecoder(r)
	var export SnapshotExport
	if err := decoder.Decode(&export); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if len(export.Records) == 0 {
		return nil, ErrEmptyData
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = export.Records

	return export.Records, nil
}

// ImportSnapshotCSV 从 CSV 导入快照
func (t *MigrationTool) ImportSnapshotCSV(r io.Reader) ([]SnapshotRecord, error) {
	reader := csv.NewReader(r)

	// 读取表头
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}

	// 验证表头
	if len(header) < 2 {
		return nil, fmt.Errorf("%w: missing required columns", ErrInvalidCSV)
	}

	var records []SnapshotRecord
	lineNum := 1

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", ErrInvalidCSV, lineNum, err)
		}
		lineNum++

		if len(row) < 2 {
			continue
		}

		// 解析地址
		addr, err := hexToAddress(row[0])
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: invalid address: %v", ErrInvalidCSV, lineNum, err)
		}

		// 解析余额
		var balance uint64
		if _, err := fmt.Sscanf(row[1], "%d", &balance); err != nil {
			return nil, fmt.Errorf("%w: line %d: invalid balance: %v", ErrInvalidCSV, lineNum, err)
		}

		records = append(records, SnapshotRecord{
			Address: addr,
			Balance: balance,
		})
	}

	if len(records) == 0 {
		return nil, ErrEmptyData
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = records

	return records, nil
}

// ImportSnapshot 加载快照记录
func (t *MigrationTool) ImportSnapshot(records []SnapshotRecord) error {
	if len(records) == 0 {
		return ErrEmptyData
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = records

	return nil
}

// ImportFromFile 从文件导入（自动识别格式）
func (t *MigrationTool) ImportFromFile(filename string) ([]SnapshotRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 尝试检测格式
	// 读取前几个字节来判断
	header := make([]byte, 4)
	n, err := file.Read(header)
	if err != nil {
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}
	if n < 4 {
		return nil, ErrInvalidFormat
	}

	// 回到文件开头
	file.Seek(0, io.SeekStart)

	// JSON 通常以 { 开始
	if header[0] == '{' {
		return t.ImportSnapshotJSON(file)
	}

	// CSV 以字母开始
	if (header[0] >= 'a' && header[0] <= 'z') || (header[0] >= 'A' && header[0] <= 'Z') {
		return t.ImportSnapshotCSV(file)
	}

	return nil, ErrInvalidFormat
}

// ============================================================================
// 验证功能
// ============================================================================

// ValidateSnapshot 验证快照数据完整性
func (t *MigrationTool) ValidateSnapshot() *ValidationReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	report := &ValidationReport{
		Timestamp:    time.Now(),
		TotalRecords: len(t.snapshot),
	}

	// 使用 map 检测重复地址
	seen := make(map[string]bool)

	for i, record := range t.snapshot {
		valid := true
		addrStr := addressToHex(record.Address)

		// 检查地址
		if !t.ValidateAddress(record.Address) {
			report.Errors = append(report.Errors, ValidationError{
				Index:   i,
				Address: addrStr,
				Type:    "invalid_address",
				Message: "invalid address format",
			})
			valid = false
		}

		// 检查余额
		if !t.ValidateBalance(record.Balance) {
			report.Errors = append(report.Errors, ValidationError{
				Index:   i,
				Address: addrStr,
				Type:    "invalid_balance",
				Message: "balance must be greater than 0",
			})
			valid = false
		}

		// 检查重复
		if seen[addrStr] {
			report.Warnings = append(report.Warnings, ValidationWarning{
				Index:   i,
				Address: addrStr,
				Type:    "duplicate_address",
				Message: "duplicate address found",
			})
		}
		seen[addrStr] = true

		if valid {
			report.ValidRecords++
		} else {
			report.InvalidRecords++
		}
	}

	return report
}

// ValidateAddress 验证地址格式
func (t *MigrationTool) ValidateAddress(addr interfaces.Address) bool {
	// 检查地址是否为空（全零）
	empty := true
	for _, b := range addr {
		if b != 0 {
			empty = false
			break
		}
	}
	return !empty
}

// ValidateBalance 验证余额
func (t *MigrationTool) ValidateBalance(balance uint64) bool {
	return balance > 0
}

// ComputeMerkleRoot 计算快照的 Merkle 根
func (t *MigrationTool) ComputeMerkleRoot() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.computeMerkleRoot(t.snapshot)
}

// computeMerkleRoot 内部计算 Merkle 根
func (t *MigrationTool) computeMerkleRoot(records []SnapshotRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, ErrEmptyData
	}

	// 创建叶子节点
	leaves := make([][]byte, len(records))
	for i, record := range records {
		leaf := make([]byte, 40)
		copy(leaf[:32], record.Address[:])
		// 使用大端序存储余额
		for j := 0; j < 8; j++ {
			leaf[32+j] = byte(record.Balance >> (56 - j*8))
		}
		leaves[i] = leaf
	}

	// 构建 Merkle 树
	tree, err := crypto.NewStandardMerkleTree(t.hasher, leaves)
	if err != nil {
		return nil, fmt.Errorf("failed to build Merkle tree: %w", err)
	}

	return tree.Root(), nil
}

// ============================================================================
// 快照数据查询
// ============================================================================

// GetSnapshot 返回当前快照数据
func (t *MigrationTool) GetSnapshot() []SnapshotRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]SnapshotRecord, len(t.snapshot))
	copy(result, t.snapshot)
	return result
}

// GetTotalBalance 返回快照总余额
func (t *MigrationTool) GetTotalBalance() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total uint64
	for _, r := range t.snapshot {
		total += r.Balance
	}
	return total
}

// GetRecordCount 返回记录数量
func (t *MigrationTool) GetRecordCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.snapshot)
}

// GetRecordByAddress 根据地址查找记录
func (t *MigrationTool) GetRecordByAddress(addr interfaces.Address) (*SnapshotRecord, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, record := range t.snapshot {
		if record.Address == addr {
			return &record, true
		}
	}
	return nil, false
}

// ============================================================================
// Merkle 证明
// ============================================================================

// GenerateMerkleProof 生成地址的 Merkle 证明
func (t *MigrationTool) GenerateMerkleProof(addr interfaces.Address) ([][]byte, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.snapshot) == 0 {
		return nil, false
	}

	// 创建叶子节点
	leaves := make([][]byte, len(t.snapshot))
	leafMap := make(map[string]int)
	for i, record := range t.snapshot {
		leaf := make([]byte, 40)
		copy(leaf[:32], record.Address[:])
		for j := 0; j < 8; j++ {
			leaf[32+j] = byte(record.Balance >> (56 - j*8))
		}
		leaves[i] = leaf
		leafMap[addressToHex(record.Address)] = i
	}

	// 构建树
	tree, err := crypto.NewStandardMerkleTree(t.hasher, leaves)
	if err != nil {
		return nil, false
	}

	// 查找索引
	index, exists := leafMap[addressToHex(addr)]
	if !exists {
		return nil, false
	}

	proof, err := tree.Proof(index)
	if err != nil {
		return nil, false
	}
	return proof, true
}

// VerifyMerkleProof 验证 Merkle 证明
func (t *MigrationTool) VerifyMerkleProof(addr interfaces.Address, balance uint64, proof [][]byte) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// 查找索引
	index := -1
	for i, record := range t.snapshot {
		if record.Address == addr && record.Balance == balance {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}

	// 计算 Merkle 根
	root, err := t.computeMerkleRoot(t.snapshot)
	if err != nil {
		return false
	}

	// 创建叶子节点
	leaf := make([]byte, 40)
	copy(leaf[:32], addr[:])
	for i := 0; i < 8; i++ {
		leaf[32+i] = byte(balance >> (56 - i*8))
	}

	return crypto.VerifyMerkleProof(t.hasher, leaf, root, proof, index)
}

// ============================================================================
// 辅助函数
// ============================================================================

// NewSnapshotRecord 创建快照记录
func NewSnapshotRecord(addr interfaces.Address, balance uint64) SnapshotRecord {
	return SnapshotRecord{
		Address: addr,
		Balance: balance,
	}
}

// HexToAddress 将十六进制字符串转换为地址
func HexToAddress(hexStr string) (interfaces.Address, error) {
	return hexToAddress(hexStr)
}

// hexToAddress 内部函数：将十六进制字符串转换为地址
func hexToAddress(hexStr string) (interfaces.Address, error) {
	// 移除 0x 前缀
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}

	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return interfaces.Address{}, fmt.Errorf("invalid hex string: %w", err)
	}

	if len(bytes) != 32 {
		return interfaces.Address{}, fmt.Errorf("address must be 32 bytes, got %d", len(bytes))
	}

	var addr interfaces.Address
	copy(addr[:], bytes)
	return addr, nil
}

// addressToHex 将地址转换为十六进制字符串
func addressToHex(addr interfaces.Address) string {
	return hex.EncodeToString(addr[:])
}
