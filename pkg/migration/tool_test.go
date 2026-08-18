package migration

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createtestaddress
func createTestAddress(i byte) interfaces.Address {
	var addr interfaces.Address
	addr[i%32] = i
	return addr
}

// 创建测试快照记录
func createTestSnapshot(count int) []SnapshotRecord {
	records := make([]SnapshotRecord, count)
	for i := 0; i < count; i++ {
		records[i] = SnapshotRecord{
			Address: createTestAddress(byte(i + 1)),
			Balance: uint64((i + 1) * 1000),
		}
	}
	return records
}

// ============================================================================
// MigrationTool 单元测试
// ============================================================================

func TestNewMigrationTool(t *testing.T) {
	tool := NewMigrationTool(nil)
	if tool == nil {
		t.Fatal("NewMigrationTool should not return nil")
	}
	if tool.hasher == nil {
		t.Error("hasher should be initialized")
	}
}

func TestNewMigrationToolWithSnapshot(t *testing.T) {
	snapshot := createTestSnapshot(5)
	tool := NewMigrationToolWithSnapshot(snapshot)
	if tool == nil {
		t.Fatal("NewMigrationToolWithSnapshot should not return nil")
	}

	count := tool.GetRecordCount()
	if count != 5 {
		t.Errorf("expected 5 records, got %d", count)
	}
}

func TestMigrationTool_ImportSnapshot(t *testing.T) {
	tool := NewMigrationTool(nil)
	snapshot := createTestSnapshot(10)

	err := tool.ImportSnapshot(snapshot)
	if err != nil {
		t.Errorf("ImportSnapshot failed: %v", err)
	}

	if tool.GetRecordCount() != 10 {
		t.Errorf("expected 10 records, got %d", tool.GetRecordCount())
	}
}

func TestMigrationTool_ImportSnapshot_Empty(t *testing.T) {
	tool := NewMigrationTool(nil)
	err := tool.ImportSnapshot([]SnapshotRecord{})
	if err != ErrEmptyData {
		t.Errorf("expected ErrEmptyData, got %v", err)
	}
}

func TestMigrationTool_GetSnapshot(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	result := tool.GetSnapshot()
	if len(result) != 3 {
		t.Errorf("expected 3 records, got %d", len(result))
	}

	// 验证是副本
	result[0].Balance = 99999
	if snapshot[0].Balance != 1000 {
		t.Error("GetSnapshot should return a copy")
	}
}

func TestMigrationTool_GetTotalBalance(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	total := tool.GetTotalBalance()
	// 1000 + 2000 + 3000 = 6000
	expected := uint64(6000)
	if total != expected {
		t.Errorf("expected %d, got %d", expected, total)
	}
}

func TestMigrationTool_GetRecordByAddress(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	// 查找存在的地址
	addr := createTestAddress(2) // 余额 2000
	record, found := tool.GetRecordByAddress(addr)
	if !found {
		t.Error("should find existing address")
	}
	if record.Balance != 2000 {
		t.Errorf("expected balance 2000, got %d", record.Balance)
	}

	// 查找不存在的地址
	emptyAddr := interfaces.Address{}
	_, found = tool.GetRecordByAddress(emptyAddr)
	if found {
		t.Error("should not find empty address")
	}
}

func TestMigrationTool_GetRecordCount(t *testing.T) {
	snapshot := createTestSnapshot(7)
	tool := NewMigrationToolWithSnapshot(snapshot)

	count := tool.GetRecordCount()
	if count != 7 {
		t.Errorf("expected 7, got %d", count)
	}
}

// ============================================================================
// JSON exportimporttest
// ============================================================================

func TestMigrationTool_ExportSnapshotJSON(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	var buf bytes.Buffer
	err := tool.ExportSnapshotJSON(&buf)
	if err != nil {
		t.Errorf("ExportSnapshotJSON failed: %v", err)
	}

	// 验证 JSON 格式
	var export SnapshotExport
	if err := json.Unmarshal(buf.Bytes(), &export); err != nil {
		t.Errorf("failed to parse JSON: %v", err)
	}

	if export.TotalRecords != 3 {
		t.Errorf("expected 3 records, got %d", export.TotalRecords)
	}

	// 验证总余额 1000 + 2000 + 3000 = 6000
	if export.TotalBalance != 6000 {
		t.Errorf("expected total balance 6000, got %d", export.TotalBalance)
	}

	if export.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", export.Version)
	}

	if export.MerkleRoot == "" {
		t.Error("Merkle root should be computed")
	}
}

func TestMigrationTool_ImportSnapshotJSON(t *testing.T) {
	tool := NewMigrationTool(nil)

	// createtestdata
	snapshot := createTestSnapshot(2)
	tool.ImportSnapshot(snapshot)

	// export
	var buf bytes.Buffer
	if err := tool.ExportSnapshotJSON(&buf); err != nil {
		t.Fatalf("ExportSnapshotJSON failed: %v", err)
	}

	// 创建新工具并导入
	tool2 := NewMigrationTool(nil)
	records, err := tool2.ImportSnapshotJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ImportSnapshotJSON failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}

	// 验证数据正确
	if records[0].Balance != 1000 {
		t.Errorf("expected first balance 1000, got %d", records[0].Balance)
	}
}

func TestMigrationTool_ImportSnapshotJSON_Empty(t *testing.T) {
	tool := NewMigrationTool(nil)
	emptyJSON := `{"version":"1.0","timestamp":"2026-01-01T00:00:00Z","total_records":0,"records":[]}`
	_, err := tool.ImportSnapshotJSON(strings.NewReader(emptyJSON))
	if err != ErrEmptyData {
		t.Errorf("expected ErrEmptyData, got %v", err)
	}
}

func TestMigrationTool_ImportSnapshotJSON_Invalid(t *testing.T) {
	tool := NewMigrationTool(nil)
	invalidJSON := `{"invalid": true}`
	_, err := tool.ImportSnapshotJSON(strings.NewReader(invalidJSON))
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}

// ============================================================================
// CSV exportimporttest
// ============================================================================

func TestMigrationTool_ExportSnapshotCSV(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	var buf bytes.Buffer
	err := tool.ExportSnapshotCSV(&buf)
	if err != nil {
		t.Errorf("ExportSnapshotCSV failed: %v", err)
	}

	content := buf.String()
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 4 { // 1 header + 3 data
		t.Errorf("expected 4 lines, got %d", len(lines))
	}

	// 验证表头
	if !strings.Contains(lines[0], "address") || !strings.Contains(lines[0], "balance") {
		t.Error("CSV header should contain address and balance")
	}
}

func TestMigrationTool_ImportSnapshotCSV(t *testing.T) {
	tool := NewMigrationTool(nil)

	// createtestdata
	snapshot := createTestSnapshot(2)
	tool.ImportSnapshot(snapshot)

	// export
	var buf bytes.Buffer
	if err := tool.ExportSnapshotCSV(&buf); err != nil {
		t.Fatalf("ExportSnapshotCSV failed: %v", err)
	}

	// 创建新工具并导入
	tool2 := NewMigrationTool(nil)
	records, err := tool2.ImportSnapshotCSV(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ImportSnapshotCSV failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}

	// 验证数据正确
	if records[0].Balance != 1000 {
		t.Errorf("expected first balance 1000, got %d", records[0].Balance)
	}
}

func TestMigrationTool_ImportSnapshotCSV_Empty(t *testing.T) {
	tool := NewMigrationTool(nil)
	emptyCSV := "address,balance\n"
	_, err := tool.ImportSnapshotCSV(strings.NewReader(emptyCSV))
	if err != ErrEmptyData {
		t.Errorf("expected ErrEmptyData, got %v", err)
	}
}

func TestMigrationTool_ImportSnapshotCSV_Invalid(t *testing.T) {
	tool := NewMigrationTool(nil)
	invalidCSV := "not-valid-csv"
	_, err := tool.ImportSnapshotCSV(strings.NewReader(invalidCSV))
	if err == nil {
		t.Error("should error on invalid CSV")
	}
}

// ============================================================================
// 验证功能测试
// ============================================================================

func TestMigrationTool_ValidateSnapshot(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	report := tool.ValidateSnapshot()
	if report.TotalRecords != 3 {
		t.Errorf("expected 3 total records, got %d", report.TotalRecords)
	}
	if report.ValidRecords != 3 {
		t.Errorf("expected 3 valid records, got %d", report.ValidRecords)
	}
	if report.InvalidRecords != 0 {
		t.Errorf("expected 0 invalid records, got %d", report.InvalidRecords)
	}
}

func TestMigrationTool_ValidateSnapshot_WithInvalid(t *testing.T) {
	snapshot := []SnapshotRecord{
		{Address: createTestAddress(1), Balance: 1000},
		{Address: interfaces.Address{}, Balance: 1000}, // 无效地址
		{Address: createTestAddress(3), Balance: 0},    // 无效余额
	}
	tool := NewMigrationToolWithSnapshot(snapshot)

	report := tool.ValidateSnapshot()
	if report.TotalRecords != 3 {
		t.Errorf("expected 3 total records, got %d", report.TotalRecords)
	}
	if report.ValidRecords != 1 {
		t.Errorf("expected 1 valid record, got %d", report.ValidRecords)
	}
	if report.InvalidRecords != 2 {
		t.Errorf("expected 2 invalid records, got %d", report.InvalidRecords)
	}
	if len(report.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(report.Errors))
	}
}

func TestMigrationTool_ValidateSnapshot_Duplicate(t *testing.T) {
	addr := createTestAddress(1)
	snapshot := []SnapshotRecord{
		{Address: addr, Balance: 1000},
		{Address: addr, Balance: 2000}, // 重复地址
	}
	tool := NewMigrationToolWithSnapshot(snapshot)

	report := tool.ValidateSnapshot()
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning for duplicate, got %d", len(report.Warnings))
	}
}

func TestMigrationTool_ValidateAddress(t *testing.T) {
	tool := NewMigrationTool(nil)

	// 有效地址
	validAddr := createTestAddress(1)
	if !tool.ValidateAddress(validAddr) {
		t.Error("should validate non-empty address")
	}

	// 无效地址
	emptyAddr := interfaces.Address{}
	if tool.ValidateAddress(emptyAddr) {
		t.Error("should not validate empty address")
	}
}

func TestMigrationTool_ValidateBalance(t *testing.T) {
	tool := NewMigrationTool(nil)

	// 有效余额
	if !tool.ValidateBalance(1000) {
		t.Error("should validate positive balance")
	}

	// 无效余额
	if tool.ValidateBalance(0) {
		t.Error("should not validate zero balance")
	}
}

// ============================================================================
// Merkle 根测试
// ============================================================================

func TestMigrationTool_ComputeMerkleRoot(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	root, err := tool.ComputeMerkleRoot()
	if err != nil {
		t.Errorf("ComputeMerkleRoot failed: %v", err)
	}
	if len(root) != 32 {
		t.Errorf("expected 32 bytes root, got %d", len(root))
	}
}

func TestMigrationTool_ComputeMerkleRoot_Empty(t *testing.T) {
	tool := NewMigrationTool(nil)
	_, err := tool.ComputeMerkleRoot()
	if err != ErrEmptyData {
		t.Errorf("expected ErrEmptyData, got %v", err)
	}
}

func TestMigrationTool_GenerateMerkleProof(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	addr := createTestAddress(2)
	proof, ok := tool.GenerateMerkleProof(addr)
	if !ok {
		t.Error("GenerateMerkleProof should succeed for existing address")
	}
	if len(proof) == 0 {
		t.Error("proof should not be empty")
	}
}

func TestMigrationTool_GenerateMerkleProof_NonExistent(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	// 使用不存在的地址
	newAddr := interfaces.Address{0xFF, 0xFF}
	_, ok := tool.GenerateMerkleProof(newAddr)
	if ok {
		t.Error("should fail for non-existent address")
	}
}

func TestMigrationTool_VerifyMerkleProof(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool := NewMigrationToolWithSnapshot(snapshot)

	addr := createTestAddress(2)
	balance := uint64(2000)

	proof, ok := tool.GenerateMerkleProof(addr)
	if !ok {
		t.Fatal("failed to generate proof")
	}

	// verify
	if !tool.VerifyMerkleProof(addr, balance, proof) {
		t.Error("VerifyMerkleProof should return true for valid proof")
	}

	// 验证错误余额应该失败
	if tool.VerifyMerkleProof(addr, 9999, proof) {
		t.Error("VerifyMerkleProof should return false for wrong balance")
	}
}

func TestMigrationTool_MerkleRootConsistency(t *testing.T) {
	snapshot := createTestSnapshot(3)
	tool1 := NewMigrationToolWithSnapshot(snapshot)
	tool2 := NewMigrationToolWithSnapshot(snapshot)

	root1, err := tool1.ComputeMerkleRoot()
	if err != nil {
		t.Fatalf("first ComputeMerkleRoot failed: %v", err)
	}

	root2, err := tool2.ComputeMerkleRoot()
	if err != nil {
		t.Fatalf("second ComputeMerkleRoot failed: %v", err)
	}

	if string(root1) != string(root2) {
		t.Error("Merkle roots should be consistent for same data")
	}
}

// ============================================================================
// exportstatustest
// ============================================================================

func TestMigrationTool_ExportStatusJSON_NoHub(t *testing.T) {
	tool := NewMigrationTool(nil)
	var buf bytes.Buffer
	err := tool.ExportStatusJSON(&buf)
	if err == nil {
		t.Error("should error without migration hub")
	}
}

func TestMigrationTool_ExportStatusCSV_NoHub(t *testing.T) {
	tool := NewMigrationTool(nil)
	var buf bytes.Buffer
	err := tool.ExportStatusCSV(&buf)
	if err == nil {
		t.Error("should error without migration hub")
	}
}

// ============================================================================
// 并发测试
// ============================================================================

func TestMigrationTool_ConcurrentAccess(t *testing.T) {
	snapshot := createTestSnapshot(10)
	tool := NewMigrationToolWithSnapshot(snapshot)

	done := make(chan bool, 2)

	// 并发读
	go func() {
		for i := 0; i < 100; i++ {
			tool.GetSnapshot()
			tool.GetRecordCount()
			tool.GetTotalBalance()
		}
		done <- true
	}()

	// 并发读
	go func() {
		for i := 0; i < 100; i++ {
			tool.ValidateSnapshot()
			tool.ComputeMerkleRoot()
		}
		done <- true
	}()

	<-done
	<-done
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestMigrationTool_EmptySnapshot(t *testing.T) {
	tool := NewMigrationTool(nil)

	// 验证空快照
	report := tool.ValidateSnapshot()
	if report.TotalRecords != 0 {
		t.Errorf("expected 0 records, got %d", report.TotalRecords)
	}

	// Merkle 根应该失败
	_, err := tool.ComputeMerkleRoot()
	if err != ErrEmptyData {
		t.Errorf("expected ErrEmptyData, got %v", err)
	}
}

func TestMigrationTool_ExportToFile(t *testing.T) {
	tool := NewMigrationTool(nil)

	// 使用不存在的路径
	err := tool.ExportToFile("/nonexistent/path/file.json", FormatJSON)
	if err == nil {
		t.Error("should error on invalid path")
	}

	// 使用无效格式
	err = tool.ExportToFile("/tmp/test.json", "invalid")
	if err != ErrInvalidFormat {
		t.Errorf("expected ErrInvalidFormat, got %v", err)
	}
}

func TestMigrationTool_ImportFromFile(t *testing.T) {
	tool := NewMigrationTool(nil)

	// 不存在的文件
	_, err := tool.ImportFromFile("/nonexistent/path/file.csv")
	if err != ErrFileNotFound {
		t.Errorf("expected ErrFileNotFound, got %v", err)
	}
}

// ============================================================================
// 工具函数测试
// ============================================================================

func TestNewSnapshotRecord(t *testing.T) {
	addr := createTestAddress(1)
	record := NewSnapshotRecord(addr, 5000)
	if record.Address != addr {
		t.Error("address mismatch")
	}
	if record.Balance != 5000 {
		t.Error("balance mismatch")
	}
}

func TestHexToAddress(t *testing.T) {
	// 测试有效的十六进制地址
	validHex := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	addr, err := HexToAddress(validHex)
	if err != nil {
		t.Errorf("HexToAddress failed: %v", err)
	}

	// 验证地址字节
	if addr[0] != 0x01 || addr[1] != 0x02 {
		t.Error("address bytes mismatch")
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestMigrationTool_JSONRoundTrip(t *testing.T) {
	// 创建原始快照
	original := createTestSnapshot(5)
	tool1 := NewMigrationToolWithSnapshot(original)

	// 导出到 JSON
	var jsonBuf bytes.Buffer
	if err := tool1.ExportSnapshotJSON(&jsonBuf); err != nil {
		t.Fatalf("ExportSnapshotJSON failed: %v", err)
	}

	// 从 JSON 导入
	tool2 := NewMigrationTool(nil)
	imported, err := tool2.ImportSnapshotJSON(bytes.NewReader(jsonBuf.Bytes()))
	if err != nil {
		t.Fatalf("ImportSnapshotJSON failed: %v", err)
	}

	// 验证记录数量
	if len(imported) != 5 {
		t.Errorf("expected 5 records, got %d", len(imported))
	}

	// 验证 Merkle 根一致
	root1, _ := tool1.ComputeMerkleRoot()
	root2, _ := tool2.ComputeMerkleRoot()
	if string(root1) != string(root2) {
		t.Error("Merkle roots should match after round trip")
	}
}

func TestMigrationTool_CSVRoundTrip(t *testing.T) {
	// 创建原始快照
	original := createTestSnapshot(3)
	tool1 := NewMigrationToolWithSnapshot(original)

	// 导出到 CSV
	var csvBuf bytes.Buffer
	if err := tool1.ExportSnapshotCSV(&csvBuf); err != nil {
		t.Fatalf("ExportSnapshotCSV failed: %v", err)
	}

	// 从 CSV 导入
	tool2 := NewMigrationTool(nil)
	imported, err := tool2.ImportSnapshotCSV(bytes.NewReader(csvBuf.Bytes()))
	if err != nil {
		t.Fatalf("ImportSnapshotCSV failed: %v", err)
	}

	// 验证记录数量
	if len(imported) != 3 {
		t.Errorf("expected 3 records, got %d", len(imported))
	}
}

func TestMigrationTool_ValidationReportTimestamp(t *testing.T) {
	before := time.Now()
	snapshot := createTestSnapshot(1)
	tool := NewMigrationToolWithSnapshot(snapshot)
	report := tool.ValidateSnapshot()
	after := time.Now()

	// 验证时间戳在测试期间
	if report.Timestamp.Before(before) || report.Timestamp.After(after) {
		t.Error("validation report timestamp should be recent")
	}
}

func TestMigrationTool_DifferentToolsSameData(t *testing.T) {
	snapshot := createTestSnapshot(4)

	// 创建两个工具实例
	tool1 := NewMigrationToolWithSnapshot(snapshot)
	tool2 := NewMigrationToolWithSnapshot(snapshot)

	// 验证独立操作不影响彼此
	tool1.ValidateSnapshot()

	// tool2 应该仍然有效
	if tool2.GetRecordCount() != 4 {
		t.Error("second tool should not be affected by first tool")
	}
}
