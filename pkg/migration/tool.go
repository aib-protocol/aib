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
// Tool error definitions
// ============================================================================

var (
	ErrInvalidFormat       = errors.New("invalid format")
	ErrEmptyData           = errors.New("empty data")
	ErrInvalidAddress      = errors.New("invalid address")
	ErrInvalidBalance      = errors.New("invalid balance")
	ErrRecordCountMismatch = errors.New("record count mismatch")
	ErrValidationFailed    = errors.New("validation failed")
	ErrFileNotFound        = errors.New("file not found")
	ErrInvalidJSON         = errors.New("invalid JSON")
	ErrInvalidCSV          = errors.New("invalid CSV")
)

// ExportFormat defines the export format
type ExportFormat string

const (
	FormatJSON ExportFormat = "json"
	FormatCSV  ExportFormat = "csv"
)

// ============================================================================
// Snapshot export structures
// ============================================================================

// SnapshotExport snapshot export format
type SnapshotExport struct {
	Version      string           `json:"version"`
	Timestamp    time.Time        `json:"timestamp"`
	TotalRecords int              `json:"total_records"`
	TotalBalance uint64           `json:"total_balance"`
	Records      []SnapshotRecord `json:"records"`
	MerkleRoot   string           `json:"merkle_root,omitempty"`
}

// StatusExport status export format
type StatusExport struct {
	Version    string           `json:"version"`
	Timestamp  time.Time        `json:"timestamp"`
	AIB1       AIB1Status       `json:"aib1"`
	CrossChain CrossChainStatus `json:"cross_chain"`
}

// AIB1Status AIB1 migration status
type AIB1Status struct {
	TotalMigrated uint64    `json:"total_migrated"`
	ClaimOpen     bool      `json:"claim_open"`
	ClaimDeadline time.Time `json:"claim_deadline"`
	TotalAccounts int       `json:"total_accounts"`
}

// CrossChainStatus cross-chain migration status
type CrossChainStatus struct {
	BTC BTCStatus `json:"btc"`
	ETH ETHStatus `json:"eth"`
	SOL SOLStatus `json:"sol"`
}

// BTCStatus BTC migration status
type BTCStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// ETHStatus ETH migration status
type ETHStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// SOLStatus SOL migration status
type SOLStatus struct {
	TotalMigrated uint64 `json:"total_migrated"`
	TotalRewards  uint64 `json:"total_rewards"`
	WindowOpen    bool   `json:"window_open"`
	CurrentRate   uint64 `json:"current_rate"`
}

// ============================================================================
// Validation report
// ============================================================================

// ValidationReport validation report
type ValidationReport struct {
	Timestamp      time.Time           `json:"timestamp"`
	TotalRecords   int                 `json:"total_records"`
	ValidRecords   int                 `json:"valid_records"`
	InvalidRecords int                 `json:"invalid_records"`
	Errors         []ValidationError   `json:"errors"`
	Warnings       []ValidationWarning `json:"warnings"`
}

// ValidationError validation error
type ValidationError struct {
	Index   int    `json:"index"`
	Address string `json:"address"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ValidationWarning validation warning
type ValidationWarning struct {
	Index   int    `json:"index"`
	Address string `json:"address"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ============================================================================
// MigrationTool main tool structure
// ============================================================================

// MigrationTool data migration tool
// Provides snapshot export, import, and validation functionality
type MigrationTool struct {
	mu        sync.RWMutex
	migration *MigrationHub
	aib1      *AIB1Migration
	snapshot  []SnapshotRecord
	hasher    crypto.Hasher
}

// NewMigrationTool creates a new migration tool
func NewMigrationTool(hub *MigrationHub) *MigrationTool {
	return &MigrationTool{
		migration: hub,
		hasher:    crypto.NewSHA256d(),
	}
}

// NewMigrationToolWithSnapshot creates a migration tool from snapshot records
func NewMigrationToolWithSnapshot(records []SnapshotRecord) *MigrationTool {
	return &MigrationTool{
		snapshot: records,
		hasher:   crypto.NewSHA256d(),
	}
}

// ============================================================================
// Export functionality
// ============================================================================

// ExportSnapshotJSON exports the snapshot in JSON format
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

	// Calculate total balance
	var totalBalance uint64
	for _, r := range records {
		totalBalance += r.Balance
	}
	export.TotalBalance = totalBalance

	// Compute the Merkle root
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

// ExportSnapshotCSV exports the snapshot in CSV format
func (t *MigrationTool) ExportSnapshotCSV(w io.Writer) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write the header
	if err := writer.Write([]string{"address", "balance"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write the data
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

// ExportStatusJSON exports migration status in JSON format
func (t *MigrationTool) ExportStatusJSON(w io.Writer) error {
	if t.migration == nil {
		return errors.New("migration hub not initialized")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	status := t.migration.GetMigrationStatus()
	export := StatusExport{
		Version:   "1.0",
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

// ExportStatusCSV exports migration status in CSV format
func (t *MigrationTool) ExportStatusCSV(w io.Writer) error {
	if t.migration == nil {
		return errors.New("migration hub not initialized")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write the header
	if err := writer.Write([]string{"chain", "total_migrated", "total_rewards", "window_open", "current_rate"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	status := t.migration.GetMigrationStatus()

	// Write BTC
	if err := writer.Write([]string{
		"BTC",
		fmt.Sprintf("%d", status.BTCTotalMigrated),
		fmt.Sprintf("%d", status.BTCTotalRewards),
		fmt.Sprintf("%t", status.BTCWindowOpen),
		fmt.Sprintf("%d", status.BTCCurrentRate),
	}); err != nil {
		return fmt.Errorf("failed to write BTC record: %w", err)
	}

	// Write ETH
	if err := writer.Write([]string{
		"ETH",
		fmt.Sprintf("%d", status.ETHTotalMigrated),
		fmt.Sprintf("%d", status.ETHTotalRewards),
		fmt.Sprintf("%t", status.ETHWindowOpen),
		fmt.Sprintf("%d", status.ETHCurrentRate),
	}); err != nil {
		return fmt.Errorf("failed to write ETH record: %w", err)
	}

	// Write SOL
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

// ExportToFile exports to a file (format auto-detected)
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
// Import functionality
// ============================================================================

// ImportSnapshotJSON imports a snapshot from JSON
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

// ImportSnapshotCSV imports a snapshot from CSV
func (t *MigrationTool) ImportSnapshotCSV(r io.Reader) ([]SnapshotRecord, error) {
	reader := csv.NewReader(r)

	// Read the header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}

	// Validate the header
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

		// Parse the address
		addr, err := hexToAddress(row[0])
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: invalid address: %v", ErrInvalidCSV, lineNum, err)
		}

		// Parse the balance
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

// ImportSnapshot loads snapshot records
func (t *MigrationTool) ImportSnapshot(records []SnapshotRecord) error {
	if len(records) == 0 {
		return ErrEmptyData
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = records

	return nil
}

// ImportFromFile imports from a file (format auto-detected)
func (t *MigrationTool) ImportFromFile(filename string) ([]SnapshotRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Try to detect the format
	// by reading the first few bytes
	header := make([]byte, 4)
	n, err := file.Read(header)
	if err != nil {
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}
	if n < 4 {
		return nil, ErrInvalidFormat
	}

	// Seek back to the start of the file
	file.Seek(0, io.SeekStart)

	// JSON typically starts with {
	if header[0] == '{' {
		return t.ImportSnapshotJSON(file)
	}

	// CSV starts with a letter
	if (header[0] >= 'a' && header[0] <= 'z') || (header[0] >= 'A' && header[0] <= 'Z') {
		return t.ImportSnapshotCSV(file)
	}

	return nil, ErrInvalidFormat
}

// ============================================================================
// Validation functionality
// ============================================================================

// ValidateSnapshot validates snapshot data integrity
func (t *MigrationTool) ValidateSnapshot() *ValidationReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	report := &ValidationReport{
		Timestamp:    time.Now(),
		TotalRecords: len(t.snapshot),
	}

	// Use a map to detect duplicate addresses
	seen := make(map[string]bool)

	for i, record := range t.snapshot {
		valid := true
		addrStr := addressToHex(record.Address)

		// Check the address
		if !t.ValidateAddress(record.Address) {
			report.Errors = append(report.Errors, ValidationError{
				Index:   i,
				Address: addrStr,
				Type:    "invalid_address",
				Message: "invalid address format",
			})
			valid = false
		}

		// Check the balance
		if !t.ValidateBalance(record.Balance) {
			report.Errors = append(report.Errors, ValidationError{
				Index:   i,
				Address: addrStr,
				Type:    "invalid_balance",
				Message: "balance must be greater than 0",
			})
			valid = false
		}

		// Check for duplicates
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

// ValidateAddress validates the address format
func (t *MigrationTool) ValidateAddress(addr interfaces.Address) bool {
	// Check whether the address is empty (all zeros)
	empty := true
	for _, b := range addr {
		if b != 0 {
			empty = false
			break
		}
	}
	return !empty
}

// ValidateBalance validates the balance
func (t *MigrationTool) ValidateBalance(balance uint64) bool {
	return balance > 0
}

// ComputeMerkleRoot computes the Merkle root of the snapshot
func (t *MigrationTool) ComputeMerkleRoot() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.computeMerkleRoot(t.snapshot)
}

// computeMerkleRoot internally computes the Merkle root
func (t *MigrationTool) computeMerkleRoot(records []SnapshotRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, ErrEmptyData
	}

	// Create leaf nodes
	leaves := make([][]byte, len(records))
	for i, record := range records {
		leaf := make([]byte, 40)
		copy(leaf[:32], record.Address[:])
		// Store the balance in big-endian order
		for j := 0; j < 8; j++ {
			leaf[32+j] = byte(record.Balance >> (56 - j*8))
		}
		leaves[i] = leaf
	}

	// Build the Merkle tree
	tree, err := crypto.NewStandardMerkleTree(t.hasher, leaves)
	if err != nil {
		return nil, fmt.Errorf("failed to build Merkle tree: %w", err)
	}

	return tree.Root(), nil
}

// ============================================================================
// Snapshot data queries
// ============================================================================

// GetSnapshot returns the current snapshot data
func (t *MigrationTool) GetSnapshot() []SnapshotRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]SnapshotRecord, len(t.snapshot))
	copy(result, t.snapshot)
	return result
}

// GetTotalBalance returns the total balance of the snapshot
func (t *MigrationTool) GetTotalBalance() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total uint64
	for _, r := range t.snapshot {
		total += r.Balance
	}
	return total
}

// GetRecordCount returns the number of records
func (t *MigrationTool) GetRecordCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.snapshot)
}

// GetRecordByAddress finds a record by address
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
// Merkle proofs
// ============================================================================

// GenerateMerkleProof generates a Merkle proof for an address
func (t *MigrationTool) GenerateMerkleProof(addr interfaces.Address) ([][]byte, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.snapshot) == 0 {
		return nil, false
	}

	// Create leaf nodes
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

	// Build the tree
	tree, err := crypto.NewStandardMerkleTree(t.hasher, leaves)
	if err != nil {
		return nil, false
	}

	// Find the index
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

// VerifyMerkleProof verifies a Merkle proof
func (t *MigrationTool) VerifyMerkleProof(addr interfaces.Address, balance uint64, proof [][]byte) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Find the index
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

	// Compute the Merkle root
	root, err := t.computeMerkleRoot(t.snapshot)
	if err != nil {
		return false
	}

	// Create leaf nodes
	leaf := make([]byte, 40)
	copy(leaf[:32], addr[:])
	for i := 0; i < 8; i++ {
		leaf[32+i] = byte(balance >> (56 - i*8))
	}

	return crypto.VerifyMerkleProof(t.hasher, leaf, root, proof, index)
}

// ============================================================================
// Helper functions
// ============================================================================

// NewSnapshotRecord creates a snapshot record
func NewSnapshotRecord(addr interfaces.Address, balance uint64) SnapshotRecord {
	return SnapshotRecord{
		Address: addr,
		Balance: balance,
	}
}

// HexToAddress converts a hex string to an address
func HexToAddress(hexStr string) (interfaces.Address, error) {
	return hexToAddress(hexStr)
}

// hexToAddress internal function: converts a hex string to an address
func hexToAddress(hexStr string) (interfaces.Address, error) {
	// Strip the 0x prefix
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

// addressToHex converts an address to a hex string
func addressToHex(addr interfaces.Address) string {
	return hex.EncodeToString(addr[:])
}
