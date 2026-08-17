// snapshot_tool.go - AIB1 Bridge Snapshot Tool
//
// This tool generates Merkle tree snapshots for AIB1 bridge migration.
// It processes account balance data and creates verifiable proofs for
// token holders to claim their tokens on AIB2.

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Account represents a user account with balance
// Structure used for Merkle tree leaves
type Account struct {
	Address   string `json:"address"`
	Balance   string `json:"balance"`   // Amount as string to avoid float precision issues
	Timestamp int64  `json:"timestamp"` // Last activity timestamp
	Nonce     int64  `json:"nonce"`     // For replay protection
}

// SnapshotConfig holds snapshot generation parameters
type SnapshotConfig struct {
	InputFile      string `json:"input_file"`
	OutputFile     string `json:"output_file"`
	SnapshotID     string `json:"snapshot_id"`
	SnapshotTime   string `json:"snapshot_time"`  // RFC3339 format
	ClaimDeadline  string `json:"claim_deadline"` // RFC3339 format
	Network        string `json:"network"`
	MinClaimAmount string `json:"min_claim_amount"`
	HashAlgorithm  string `json:"hash_algorithm"` // "sha256", "sha512", etc.
	TreeType       string `json:"tree_type"`      // "standard", "sparse"
}

// SnapshotResult contains the generated snapshot data
type SnapshotResult struct {
	SnapshotID    string           `json:"snapshot_id"`
	SnapshotTime  string           `json:"snapshot_time"`
	SnapshotRoot  string           `json:"snapshot_root"` // Hex encoded Merkle root
	TotalAccounts int              `json:"total_accounts"`
	TotalAmount   string           `json:"total_amount"`
	ClaimDeadline string           `json:"claim_deadline"`
	Network       string           `json:"network"`
	Accounts      []Account        `json:"accounts"`
	MerkleTree    [][]string       `json:"merkle_tree"` // Each level as hex strings
	Proofs        map[string]Proof `json:"proofs"`      // Address -> proof
	Metadata      SnapshotMetadata `json:"metadata"`
}

// Proof represents a Merkle proof for an account
type Proof struct {
	LeafHash string   `json:"leaf_hash"` // Hex encoded leaf hash
	Path     []string `json:"path"`      // Sibling hashes (hex encoded)
	Indices  []int    `json:"indices"`   // Position indicators (0=left, 1=right)
}

// SnapshotMetadata holds snapshot metadata
type SnapshotMetadata struct {
	Version       string `json:"version"`
	CreatedAt     string `json:"created_at"`
	CreatedBy     string `json:"created_by"`
	TreeType      string `json:"tree_type"`
	HashAlgorithm string `json:"hash_algorithm"`
	InputFile     string `json:"input_file,omitempty"`
}

// Hasher defines hash function interface
type Hasher interface {
	Hash(data []byte) []byte
	Size() int
}

// SHA256Hasher implements SHA-256 hashing
type SHA256Hasher struct{}

func (h *SHA256Hasher) Hash(data []byte) []byte {
	result := sha256.Sum256(data)
	return result[:]
}

func (h *SHA256Hasher) Size() int {
	return sha256.Size
}

// NewHasher creates a hasher based on algorithm name
func NewHasher(algorithm string) Hasher {
	switch strings.ToLower(algorithm) {
	case "sha256":
		return &SHA256Hasher{}
	default:
		return &SHA256Hasher{}
	}
}

func main() {
	// Initialize logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse command line flags
	configPath := flag.String("config", "", "Path to snapshot configuration file")
	inputFile := flag.String("input", "", "Input file with account data (CSV or JSON)")
	outputFile := flag.String("output", "", "Output file for snapshot data")
	snapshotID := flag.String("id", "aib1-snapshot-"+time.Now().Format("20060102"), "Snapshot ID")
	snapshotTime := flag.String("time", time.Now().Format(time.RFC3339), "Snapshot timestamp (RFC3339)")
	deadline := flag.String("deadline", "", "Claim deadline (RFC3339)")
	_ = flag.String("format", "csv", "Input format: csv or json (autodetected from file extension)")
	verbose := flag.Bool("v", false, "Verbose output")
	validateOnly := flag.Bool("validate", false, "Only validate input data without generating snapshot")
	network := flag.String("network", "aib1-mainnet", "Network identifier")
	hashAlgo := flag.String("hash", "sha256", "Hash algorithm (sha256)")
	flag.Parse()

	// Load or create configuration
	var config SnapshotConfig
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			log.Fatalf("Failed to parse configuration: %v", err)
		}
	} else {
		// Create config from flags
		config = SnapshotConfig{
			SnapshotID:    *snapshotID,
			SnapshotTime:  *snapshotTime,
			ClaimDeadline: *deadline,
			Network:       *network,
			HashAlgorithm: *hashAlgo,
			TreeType:      "standard",
		}
	}

	// Override config with command line flags if provided
	if *inputFile != "" {
		config.InputFile = *inputFile
	}
	if *outputFile != "" {
		config.OutputFile = *outputFile
	}
	if *deadline != "" {
		config.ClaimDeadline = *deadline
	}
	if *network != "" {
		config.Network = *network
	}
	if *hashAlgo != "" {
		config.HashAlgorithm = *hashAlgo
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	if *verbose {
		fmt.Println("=================================================")
		fmt.Println("  AIB1 Bridge Snapshot Generator")
		fmt.Println("=================================================")
		fmt.Printf("Snapshot ID: %s\n", config.SnapshotID)
		fmt.Printf("Network: %s\n", config.Network)
		fmt.Printf("Input File: %s\n", config.InputFile)
		fmt.Printf("Output File: %s\n", config.OutputFile)
		fmt.Printf("Hash Algorithm: %s\n", config.HashAlgorithm)
		fmt.Println("=================================================")
		fmt.Println()
	}

	// Read input data
	fmt.Printf("Reading input data from %s...\n", config.InputFile)
	accounts, err := readInputData(config)
	if err != nil {
		log.Fatalf("Failed to read input data: %v", err)
	}

	if *verbose {
		fmt.Printf("Read %d accounts from input\n", len(accounts))
	}

	// Validate accounts
	fmt.Println("Validating account data...")
	if err := validateAccounts(accounts); err != nil {
		log.Fatalf("Account validation failed: %v", err)
	}
	fmt.Printf("Validation passed: %d valid accounts\n", len(accounts))

	if *validateOnly {
		fmt.Printf("\nValidation completed successfully. %d valid accounts found.\n", len(accounts))
		return
	}

	// Generate Merkle tree
	fmt.Println("Generating Merkle tree...")
	tree, rootHash, proofs, err := generateMerkleTree(accounts, config.HashAlgorithm)
	if err != nil {
		log.Fatalf("Failed to generate Merkle tree: %v", err)
	}

	if *verbose {
		fmt.Printf("Merkle tree generated with root: %x\n", rootHash)
		fmt.Printf("Tree depth: %d levels\n", len(tree))
	}

	// Calculate totals
	totalAmount, err := calculateTotalAmount(accounts)
	if err != nil {
		log.Fatalf("Failed to calculate total amount: %v", err)
	}

	// Create snapshot result
	snapshot := SnapshotResult{
		SnapshotID:    config.SnapshotID,
		SnapshotTime:  config.SnapshotTime,
		SnapshotRoot:  fmt.Sprintf("%x", rootHash),
		TotalAccounts: len(accounts),
		TotalAmount:   totalAmount,
		ClaimDeadline: config.ClaimDeadline,
		Network:       config.Network,
		Accounts:      accounts,
		MerkleTree:    tree,
		Proofs:        proofs,
		Metadata: SnapshotMetadata{
			Version:       "1.0.0",
			CreatedAt:     time.Now().Format(time.RFC3339),
			CreatedBy:     "aib-snapshot-tool",
			TreeType:      config.TreeType,
			HashAlgorithm: config.HashAlgorithm,
			InputFile:     filepath.Base(config.InputFile),
		},
	}

	// Save snapshot
	fmt.Printf("Saving snapshot to %s...\n", config.OutputFile)
	if err := saveSnapshot(snapshot, config.OutputFile); err != nil {
		log.Fatalf("Failed to save snapshot: %v", err)
	}

	fmt.Println()
	fmt.Println("=================================================")
	fmt.Println("  Snapshot Generation Summary")
	fmt.Println("=================================================")
	fmt.Printf("  Snapshot ID:      %s\n", snapshot.SnapshotID)
	fmt.Printf("  Total Accounts:   %d\n", snapshot.TotalAccounts)
	fmt.Printf("  Total Amount:     %s\n", snapshot.TotalAmount)
	fmt.Printf("  Merkle Root:      %s\n", snapshot.SnapshotRoot)
	fmt.Printf("  Snapshot Time:    %s\n", snapshot.SnapshotTime)
	fmt.Printf("  Claim Deadline:   %s\n", snapshot.ClaimDeadline)
	fmt.Printf("  Output File:      %s\n", config.OutputFile)
	fmt.Println("=================================================")

	fmt.Println("\nSnapshot generation completed successfully.")
}

// validateConfig validates the snapshot configuration
func validateConfig(config SnapshotConfig) error {
	if config.InputFile == "" {
		return fmt.Errorf("input_file is required")
	}
	if config.OutputFile == "" {
		return fmt.Errorf("output_file is required")
	}
	if config.SnapshotID == "" {
		return fmt.Errorf("snapshot_id is required")
	}
	if config.SnapshotTime == "" {
		return fmt.Errorf("snapshot_time is required")
	}
	if config.ClaimDeadline == "" {
		return fmt.Errorf("claim_deadline is required")
	}
	if config.Network == "" {
		return fmt.Errorf("network is required")
	}

	// Validate timestamp formats
	if _, err := time.Parse(time.RFC3339, config.SnapshotTime); err != nil {
		return fmt.Errorf("invalid snapshot_time format: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, config.ClaimDeadline); err != nil {
		return fmt.Errorf("invalid claim_deadline format: %w", err)
	}

	return nil
}

// readInputData reads account data from input file
func readInputData(config SnapshotConfig) ([]Account, error) {
	file, err := os.Open(config.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	var accounts []Account

	if strings.HasSuffix(strings.ToLower(config.InputFile), ".json") {
		accounts, err = readJSON(file)
	} else {
		// Default to CSV
		accounts, err = readCSV(file)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read input data: %w", err)
	}

	return accounts, nil
}

// readCSV reads account data from CSV file
// Format: address,balance[,timestamp][,nonce]
func readCSV(r io.Reader) ([]Account, error) {
	scanner := bufio.NewScanner(r)
	var accounts []Account
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid CSV format at line %d: expected at least 2 columns (address,balance)", lineNum)
		}

		address := strings.TrimSpace(parts[0])
		balance := strings.TrimSpace(parts[1])
		timestamp := int64(0)
		nonce := int64(0)

		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			timestamp, _ = strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		}
		if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
			nonce, _ = strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		}

		accounts = append(accounts, Account{
			Address:   address,
			Balance:   balance,
			Timestamp: timestamp,
			Nonce:     nonce,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	return accounts, nil
}

// readJSON reads account data from JSON file
func readJSON(r io.Reader) ([]Account, error) {
	decoder := json.NewDecoder(r)
	var accounts []Account
	if err := decoder.Decode(&accounts); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return accounts, nil
}

// validateAccounts validates account data
func validateAccounts(accounts []Account) error {
	if len(accounts) == 0 {
		return fmt.Errorf("no accounts found in input data")
	}

	addressSet := make(map[string]bool)

	for i, account := range accounts {
		// Validate address
		if account.Address == "" {
			return fmt.Errorf("account at index %d has empty address", i)
		}
		if !isValidAddress(account.Address) {
			return fmt.Errorf("account at index %d has invalid address: %s", i, account.Address)
		}

		// Check for duplicate addresses
		if addressSet[account.Address] {
			return fmt.Errorf("duplicate address found at index %d: %s", i, account.Address)
		}
		addressSet[account.Address] = true

		// Validate balance
		if account.Balance == "" {
			return fmt.Errorf("account at index %d has empty balance", i)
		}
		balance, err := strconv.ParseInt(account.Balance, 10, 64)
		if err != nil {
			return fmt.Errorf("account at index %d has invalid balance '%s': %w", i, account.Balance, err)
		}
		if balance <= 0 {
			return fmt.Errorf("account at index %d has non-positive balance: %d", i, balance)
		}

		// Validate timestamp (optional)
		if account.Timestamp < 0 {
			return fmt.Errorf("account at index %d has negative timestamp: %d", i, account.Timestamp)
		}

		// Validate nonce (optional)
		if account.Nonce < 0 {
			return fmt.Errorf("account at index %d has negative nonce: %d", i, account.Nonce)
		}
	}

	return nil
}

// isValidAddress validates an address format
// Basic hex address validation (40-64 hex characters)
func isValidAddress(address string) bool {
	if address == "" {
		return false
	}
	// Allow hex addresses with or without 0x prefix
	testAddr := strings.TrimPrefix(address, "0x")
	if len(testAddr) < 40 || len(testAddr) > 64 {
		return false
	}
	for _, ch := range testAddr {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

// generateMerkleTree generates a Merkle tree from account data
func generateMerkleTree(accounts []Account, hashAlgorithm string) ([][]string, []byte, map[string]Proof, error) {
	hasher := NewHasher(hashAlgorithm)

	// Create leaf hashes
	leafHashes := make([][]byte, len(accounts))
	for i, account := range accounts {
		leafData := fmt.Sprintf("%s:%s:%d:%d", account.Address, account.Balance, account.Timestamp, account.Nonce)
		leafHashes[i] = hasher.Hash([]byte(leafData))
	}

	// Build Merkle tree
	treeData := buildMerkleTree(hasher, leafHashes)
	rootHash := treeData[len(treeData)-1][0]

	// Convert tree to hex strings for JSON serialization
	tree := make([][]string, len(treeData))
	for i, level := range treeData {
		levelHex := make([]string, len(level))
		for j, node := range level {
			levelHex[j] = fmt.Sprintf("%x", node)
		}
		tree[i] = levelHex
	}

	// Generate proofs for each account
	proofs := make(map[string]Proof)
	for i, account := range accounts {
		leafHash := leafHashes[i]
		proof, err := generateProof(treeData, i)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to generate proof for account %s: %w", account.Address, err)
		}
		proofs[account.Address] = Proof{
			LeafHash: fmt.Sprintf("%x", leafHash),
			Path:     proof.path,
			Indices:  proof.indices,
		}
	}

	return tree, rootHash, proofs, nil
}

// proofData holds internal proof generation data
type proofData struct {
	path    []string
	indices []int
}

// buildMerkleTree builds a standard binary Merkle tree
// Returns a slice of levels, where level[0] is leaf hashes and level[len-1] is root
func buildMerkleTree(hasher Hasher, leaves [][]byte) [][][]byte {
	if len(leaves) == 0 {
		return nil
	}

	tree := [][][]byte{}
	level := make([][]byte, len(leaves))
	for i, leaf := range leaves {
		level[i] = make([]byte, len(leaf))
		copy(level[i], leaf)
	}
	tree = append(tree, level)

	for len(level) > 1 {
		// If odd, duplicate last node (Bitcoin convention)
		if len(level)%2 != 0 {
			dup := make([]byte, len(level[len(level)-1]))
			copy(dup, level[len(level)-1])
			level = append(level, dup)
		}

		nextLevel := make([][]byte, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := make([]byte, len(level[i])+len(level[i+1]))
			copy(combined, level[i])
			copy(combined[len(level[i]):], level[i+1])
			nextLevel[i/2] = hasher.Hash(combined)
		}
		tree = append(tree, nextLevel)
		level = nextLevel
	}

	return tree
}

// generateProof generates a Merkle proof for a leaf at the given index
func generateProof(tree [][][]byte, index int) (proofData, error) {
	if len(tree) == 0 {
		return proofData{}, fmt.Errorf("empty tree")
	}
	if index < 0 || index >= len(tree[0]) {
		return proofData{}, fmt.Errorf("index out of range: %d", index)
	}

	proof := proofData{
		path:    make([]string, 0, len(tree)-1),
		indices: make([]int, 0, len(tree)-1),
	}
	idx := index

	for lvl := 0; lvl < len(tree)-1; lvl++ {
		level := tree[lvl]

		// Determine sibling
		var sibling []byte
		var position int // 0 = left (sibling on right), 1 = right (sibling on left)

		if idx%2 == 0 {
			// Current node is left, sibling is right
			if idx+1 < len(level) {
				sibling = level[idx+1]
			} else {
				// Odd case: duplicate self (Bitcoin convention)
				sibling = level[idx]
			}
			position = 0
		} else {
			// Current node is right, sibling is left
			sibling = level[idx-1]
			position = 1
		}

		proof.path = append(proof.path, fmt.Sprintf("%x", sibling))
		proof.indices = append(proof.indices, position)
		idx /= 2
	}

	return proof, nil
}

// calculateTotalAmount calculates the total amount across all accounts
func calculateTotalAmount(accounts []Account) (string, error) {
	var total int64
	for _, account := range accounts {
		amount, err := strconv.ParseInt(account.Balance, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid balance '%s': %w", account.Balance, err)
		}
		total += amount
	}
	return strconv.FormatInt(total, 10), nil
}

// saveSnapshot saves the snapshot result to file
func saveSnapshot(snapshot SnapshotResult, path string) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot file: %w", err)
	}

	return nil
}
