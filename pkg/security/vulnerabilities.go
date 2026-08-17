// Package security provides vulnerability detection for AIB 2.0.
// This package implements static analysis and runtime checks for common vulnerabilities.
package security

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/aib-protocol/aib/pkg/aal"
	"github.com/aib-protocol/aib/pkg/utxo"
)

// ============================================================================
// Vulnerability Types
// ============================================================================

// Severity levels for vulnerabilities
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

// Vulnerability represents a detected security issue
type Vulnerability struct {
	ID             string
	Severity       Severity
	Category       string
	Description    string
	Location       string
	Recommendation string
}

// ============================================================================
// Smart Contract Vulnerability Detection (Solidity)
// ============================================================================

// VulnerabilityPatterns contains regex patterns for common Solidity vulnerabilities
var VulnerabilityPatterns = map[string]*regexp.Regexp{
	// Reentrancy vulnerabilities
	"reentrancy_call":        regexp.MustCompile(`(?i)(call\.value|transfer|send).*\{[^}]*(?<!>)call\{`),
	"reentrancy_unprotected": regexp.MustCompile(`(?i)function\s+\w+\s*\([^)]*\)\s*(public|external)\s*\{[^}](?!.*require\(?!.*\(msg\.sender[^)]*\)))(?!.*ReentrancyGuard)`),

	// Integer overflow/underflow (pre-Solidity 0.8.0)
	"integer_overflow": regexp.MustCompile(`(?i)pragma\s+solidity\s+(\^|>=)\s*0\.[0-7]\.`),
	"unsafe_math":      regexp.MustCompile(`(?i)\.(add|sub|mul|div|mod)\(.*[^safe]`),

	// Access control issues
	"missing_access_control": regexp.MustCompile(`(?i)function\s+\w+\s*\([^)]*\)\s*(public|external)\s*(?!.*(onlyOwner|require\(msg\.sender))\{`),
	"tx_origin_usage":        regexp.MustCompile(`(?i)tx\.origin`),

	// Front-running risks
	"front_running": regexp.MustCompile(`(?i)(block\.timestamp|block\.number).*==.*(now|block\.number)`),

	// Delegatecall risks
	"delegatecall_untrusted": regexp.MustCompile(`(?i)delegatecall.*(address|addr).*\{[^}]*(?!.*(safe|trusted))`),

	// Self-destruct risks
	"unprotected_selfdestruct": regexp.MustCompile(`(?i)selfdestruct.*\(address\(0\)`),
	"suicide_usage":            regexp.MustCompile(`(?i)suicide\(`),
}

// DetectSolidityVulnerabilities scans Solidity code for known vulnerabilities
func DetectSolidityVulnerabilities(code string) []Vulnerability {
	var vulns []Vulnerability

	// Check for outdated Solidity version (before 0.8.0)
	if match := VulnerabilityPatterns["integer_overflow"].FindString(code); match != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-001",
			Severity:       SeverityHigh,
			Category:       "Integer Overflow",
			Description:    "Solidity version below 0.8.0 may have integer overflow issues",
			Location:       "pragma",
			Recommendation: "Upgrade to Solidity 0.8.0 or higher, or use SafeMath library",
		})
	}

	// Check for tx.origin usage (phishing risk)
	if VulnerabilityPatterns["tx_origin_usage"].FindString(code) != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-002",
			Severity:       SeverityMedium,
			Category:       "Phishing",
			Description:    "tx.origin usage can be exploited in phishing attacks",
			Location:       "tx.origin",
			Recommendation: "Use msg.sender instead of tx.origin for authorization",
		})
	}

	// Check for unprotected functions
	if VulnerabilityPatterns["missing_access_control"].FindString(code) != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-003",
			Severity:       SeverityHigh,
			Category:       "Access Control",
			Description:    "Function may lack proper access control",
			Location:       "function definition",
			Recommendation: "Add appropriate access control modifiers (onlyOwner, etc.)",
		})
	}

	// Check for front-running patterns
	if VulnerabilityPatterns["front_running"].FindString(code) != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-004",
			Severity:       SeverityMedium,
			Category:       "Front-Running",
			Description:    "Code may be vulnerable to front-running attacks",
			Location:       "block usage",
			Recommendation: "Use commit-reveal schemes or batch transactions",
		})
	}

	// Check for unsafe delegatecall
	if VulnerabilityPatterns["delegatecall_untrusted"].FindString(code) != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-005",
			Severity:       SeverityCritical,
			Category:       "Delegatecall",
			Description:    "Untrusted delegatecall can lead to code execution",
			Location:       "delegatecall",
			Recommendation: "Validate target address and use trusted contracts only",
		})
	}

	// Check for selfdestruct
	if VulnerabilityPatterns["unprotected_selfdestruct"].FindString(code) != "" {
		vulns = append(vulns, Vulnerability{
			ID:             "SOL-006",
			Severity:       SeverityHigh,
			Category:       "Destructibility",
			Description:    "Unprotected selfdestruct can permanently destroy contract",
			Location:       "selfdestruct",
			Recommendation: "Add access control to selfdestruct functions",
		})
	}

	return vulns
}

// ============================================================================
// EVM Runtime Vulnerability Detection
// ============================================================================

// CheckGasLimit validates gas limit is within safe bounds
func CheckGasLimit(gasLimit, blockGasLimit uint64) Vulnerability {
	if gasLimit > blockGasLimit {
		return Vulnerability{
			ID:             "EVM-001",
			Severity:       SeverityHigh,
			Category:       "Gas",
			Description:    "Transaction gas limit exceeds block gas limit",
			Location:       "gas calculation",
			Recommendation: "Ensure gas limit is <= block gas limit",
		}
	}
	if gasLimit > 10000000 { // EIP-1559 max gas
		return Vulnerability{
			ID:             "EVM-002",
			Severity:       SeverityMedium,
			Category:       "Gas",
			Description:    "Excessive gas limit may cause block validation issues",
			Location:       "gas limit",
			Recommendation: "Consider reducing gas limit for better compatibility",
		}
	}
	return Vulnerability{ID: ""} // No issue
}

// CheckCallDepth verifies call depth is within EVM limits
func CheckCallDepth(depth int) Vulnerability {
	const maxCallDepth = 1024
	if depth > maxCallDepth {
		return Vulnerability{
			ID:             "EVM-003",
			Severity:       SeverityCritical,
			Category:       "Call Depth",
			Description:    "Call depth exceeds EVM maximum (1024)",
			Location:       "call stack",
			Recommendation: "Reduce nested call depth",
		}
	}
	if depth > maxCallDepth-100 {
		return Vulnerability{
			ID:             "EVM-004",
			Severity:       SeverityMedium,
			Category:       "Call Depth",
			Description:    "Call depth approaching EVM limit",
			Location:       "call stack",
			Recommendation: "Consider reducing nested calls",
		}
	}
	return Vulnerability{ID: ""}
}

// CheckMemoryAccess validates memory access bounds
func CheckMemoryAccess(offset, length uint64, memorySize uint64) Vulnerability {
	const maxMemory = 256 * 1024 * 1024 // 256 MB
	if offset+length > maxMemory {
		return Vulnerability{
			ID:             "EVM-005",
			Severity:       SeverityHigh,
			Category:       "Memory",
			Description:    "Memory access exceeds maximum allowed size",
			Location:       "memory access",
			Recommendation: "Ensure memory operations stay within bounds",
		}
	}
	if offset+length > memorySize {
		return Vulnerability{
			ID:             "EVM-006",
			Severity:       SeverityHigh,
			Category:       "Memory",
			Description:    "Memory read/write outside allocated memory",
			Location:       "memory bounds",
			Recommendation: "Expand memory before access or check bounds",
		}
	}
	return Vulnerability{ID: ""}
}

// CheckCREATE2Usage validates CREATE2 parameters
func CheckCREATE2Usage(salt [32]byte, code []byte) Vulnerability {
	if len(code) == 0 {
		return Vulnerability{
			ID:             "EVM-007",
			Severity:       SeverityMedium,
			Category:       "CREATE2",
			Description:    "CREATE2 with empty code creates unusable address",
			Location:       "CREATE2",
			Recommendation: "Ensure non-empty deployment code",
		}
	}
	// Check for deterministic address collision potential
	if bytes.Count(salt[:], []byte{0}) == 32 {
		return Vulnerability{
			ID:             "EVM-008",
			Severity:       SeverityLow,
			Category:       "CREATE2",
			Description:    "Salt is all zeros - potential address collision",
			Location:       "CREATE2 salt",
			Recommendation: "Use cryptographically random salt",
		}
	}
	return Vulnerability{ID: ""}
}

// ============================================================================
// UTXO Vulnerability Detection
// ============================================================================

// CheckDoubleSpend detects potential double-spend attacks
func CheckDoubleSpend(tx1, tx2 *utxo.Transaction) []Vulnerability {
	var vulns []Vulnerability

	// Check if transactions spend the same UTXO
	spentUTXOs := make(map[string]bool)
	for _, input := range tx1.Inputs {
		utxoKey := hex.EncodeToString(input.TxHash[:]) + fmt.Sprintf("%d", input.Index)
		spentUTXOs[utxoKey] = true
	}

	for _, input := range tx2.Inputs {
		utxoKey := hex.EncodeToString(input.TxHash[:]) + fmt.Sprintf("%d", input.Index)
		if spentUTXOs[utxoKey] {
			vulns = append(vulns, Vulnerability{
				ID:             "UTXO-001",
				Severity:       SeverityCritical,
				Category:       "Double-Spend",
				Description:    "Transactions attempt to spend the same UTXO",
				Location:       "transaction inputs",
				Recommendation: "Implement double-spend detection and prevention",
			})
			break
		}
	}

	return vulns
}

// CheckInputOutputConservation validates UTXO conservation (inputs >= outputs + fee)
func CheckInputOutputConservation(tx *utxo.Transaction, minFee uint64) Vulnerability {
	totalIn, _ := tx.TotalInputValue(nil) // TODO: provide proper UTXOProvider
	totalOut := tx.TotalOutputValue()

	if totalOut > totalIn {
		return Vulnerability{
			ID:             "UTXO-002",
			Severity:       SeverityCritical,
			Category:       "Conservation",
			Description:    "Transaction outputs exceed inputs",
			Location:       "value calculation",
			Recommendation: "Ensure inputs >= outputs + fees",
		}
	}

	if totalIn-totalOut < minFee {
		return Vulnerability{
			ID:             "UTXO-003",
			Severity:       SeverityHigh,
			Category:       "Fee",
			Description:    "Transaction fee below minimum",
			Location:       "fee calculation",
			Recommendation: "Increase transaction fee",
		}
	}

	return Vulnerability{ID: ""}
}

// CheckSignatureSecurity validates signature security
func CheckSignatureSecurity(pubKey []byte, msg []byte, sig []byte) Vulnerability {
	// Check for weak key length
	if len(pubKey) != 32 && len(pubKey) != 64 {
		return Vulnerability{
			ID:             "SIG-001",
			Severity:       SeverityCritical,
			Category:       "Signature",
			Description:    "Invalid public key length",
			Location:       "public key",
			Recommendation: "Use valid Ed25519 or Secp256k1 keys",
		}
	}

	// Check signature length
	if len(sig) != 64 && len(sig) != 65 {
		return Vulnerability{
			ID:             "SIG-002",
			Severity:       SeverityCritical,
			Category:       "Signature",
			Description:    "Invalid signature length",
			Location:       "signature",
			Recommendation: "Use proper signature format",
		}
	}

	// Check for signature malleability (low-s value for ECDSA)
	if len(sig) == 65 && sig[64] >= 27 {
		// Could be malleable - suggest low-s value
		return Vulnerability{
			ID:             "SIG-003",
			Severity:       SeverityMedium,
			Category:       "Signature",
			Description:    "Signature may be malleable",
			Location:       "signature s-value",
			Recommendation: "Use low-s values for ECDSA signatures",
		}
	}

	return Vulnerability{ID: ""}
}

// CheckReplayProtection validates replay attack protection
func CheckReplayProtection(tx *utxo.Transaction, chainID []byte) Vulnerability {
	if tx.Sequence == 0 {
		return Vulnerability{
			ID:             "REPLAY-001",
			Severity:       SeverityHigh,
			Category:       "Replay Protection",
			Description:    "Transaction lacks sequence number for replay protection",
			Location:       "sequence",
			Recommendation: "Include valid sequence number",
		}
	}

	// Check for chain ID in signature context (simplified)
	if len(chainID) == 0 {
		return Vulnerability{
			ID:             "REPLAY-002",
			Severity:       SeverityMedium,
			Category:       "Replay Protection",
			Description:    "No chain ID context for cross-chain replay prevention",
			Location:       "signature",
			Recommendation: "Include chain ID in transaction",
		}
	}

	return Vulnerability{ID: ""}
}

// ============================================================================
// AAL (Account Abstraction Layer) Vulnerability Detection
// ============================================================================

// CheckStateConsistency validates AAL state consistency
func CheckStateConsistency(stateManager *aal.StateManager, addr aal.Address) Vulnerability {
	balance, _ := stateManager.GetBalance(addr)
	nonce, _ := stateManager.GetNonce(addr)

	// Check for negative balance
	if balance.Sign() < 0 {
		return Vulnerability{
			ID:             "AAL-001",
			Severity:       SeverityCritical,
			Category:       "State Consistency",
			Description:    "Negative balance detected",
			Location:       "balance",
			Recommendation: "Fix balance calculation to prevent negative values",
		}
	}

	// Check for nonce overflow
	if nonce > 1<<63-1 {
		return Vulnerability{
			ID:             "AAL-002",
			Severity:       SeverityHigh,
			Category:       "State Consistency",
			Description:    "Nonce overflow risk",
			Location:       "nonce",
			Recommendation: "Implement nonce wrapping or reset mechanism",
		}
	}

	return Vulnerability{ID: ""}
}

// CheckCrossLayerAtomicity validates atomic transactions across layers
func CheckCrossLayerAtomicity(txData []byte) Vulnerability {
	// Check for incomplete cross-layer transaction
	if len(txData) < 40 {
		return Vulnerability{
			ID:             "AAL-003",
			Severity:       SeverityMedium,
			Category:       "Atomicity",
			Description:    "Cross-layer transaction may be incomplete",
			Location:       "transaction data",
			Recommendation: "Ensure full transaction data for atomic execution",
		}
	}

	// Check for missing layer indicators
	if !bytes.Contains(txData, []byte("evm")) && !bytes.Contains(txData, []byte("utxo")) {
		return Vulnerability{
			ID:             "AAL-004",
			Severity:       SeverityLow,
			Category:       "Atomicity",
			Description:    "No clear layer indicator in cross-layer transaction",
			Location:       "transaction header",
			Recommendation: "Include explicit layer indicator",
		}
	}

	return Vulnerability{ID: ""}
}

// CheckResourceLock validates resource locking for atomic swaps
func CheckResourceLock(lockDuration uint64, blockTime uint64) Vulnerability {
	if lockDuration == 0 {
		return Vulnerability{
			ID:             "AAL-005",
			Severity:       SeverityHigh,
			Category:       "Resource Lock",
			Description:    "Zero lock duration provides no security",
			Location:       "lock duration",
			Recommendation: "Set appropriate lock duration based on block time",
		}
	}

	// Check if lock is too short relative to block time
	if lockDuration < blockTime*2 {
		return Vulnerability{
			ID:             "AAL-006",
			Severity:       SeverityMedium,
			Category:       "Resource Lock",
			Description:    "Lock duration too short relative to block time",
			Location:       "lock duration",
			Recommendation: "Increase lock duration for better security",
		}
	}

	return Vulnerability{ID: ""}
}

// ============================================================================
// Comprehensive Security Scanner
// ============================================================================

// SecurityReport contains comprehensive security scan results
type SecurityReport struct {
	TotalVulnerabilities int
	CriticalCount        int
	HighCount            int
	MediumCount          int
	LowCount             int
	Vulnerabilities      []Vulnerability
}

// ScanSmartContract performs comprehensive security scan on Solidity code
func ScanSmartContract(code string) SecurityReport {
	report := SecurityReport{
		Vulnerabilities: DetectSolidityVulnerabilities(code),
	}

	for _, v := range report.Vulnerabilities {
		report.TotalVulnerabilities++
		switch v.Severity {
		case SeverityCritical:
			report.CriticalCount++
		case SeverityHigh:
			report.HighCount++
		case SeverityMedium:
			report.MediumCount++
		case SeverityLow:
			report.LowCount++
		}
	}

	return report
}

// ScanTransaction performs security scan on a transaction
func ScanTransaction(tx *utxo.Transaction, chainID []byte, minFee uint64) SecurityReport {
	var vulns []Vulnerability

	// Check input/output conservation
	if v := CheckInputOutputConservation(tx, minFee); v.ID != "" {
		vulns = append(vulns, v)
	}

	// Check replay protection
	if v := CheckReplayProtection(tx, chainID); v.ID != "" {
		vulns = append(vulns, v)
	}

	// Check each input signature
	for i, input := range tx.Inputs {
		txHash := tx.Hash()
		if v := CheckSignatureSecurity(input.Signature[:], txHash[:], input.Signature[:]); v.ID != "" {
			v.Location = fmt.Sprintf("input[%d].signature", i)
			vulns = append(vulns, v)
		}
	}

	report := SecurityReport{
		Vulnerabilities: vulns,
	}

	for _, v := range report.Vulnerabilities {
		report.TotalVulnerabilities++
		switch v.Severity {
		case SeverityCritical:
			report.CriticalCount++
		case SeverityHigh:
			report.HighCount++
		case SeverityMedium:
			report.MediumCount++
		case SeverityLow:
			report.LowCount++
		}
	}

	return report
}

// ============================================================================
// Utility Functions
// ============================================================================

// FormatVulnerabilityReport formats vulnerabilities for display
func FormatVulnerabilityReport(report SecurityReport) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("Security Scan Report\n"))
	buf.WriteString(fmt.Sprintf("=====================\n\n"))
	buf.WriteString(fmt.Sprintf("Total Vulnerabilities: %d\n", report.TotalVulnerabilities))
	buf.WriteString(fmt.Sprintf("  Critical: %d\n", report.CriticalCount))
	buf.WriteString(fmt.Sprintf("  High:     %d\n", report.HighCount))
	buf.WriteString(fmt.Sprintf("  Medium:   %d\n", report.MediumCount))
	buf.WriteString(fmt.Sprintf("  Low:      %d\n\n", report.LowCount))

	if len(report.Vulnerabilities) > 0 {
		buf.WriteString("Vulnerabilities Found:\n")
		buf.WriteString(strings.Repeat("-", 60) + "\n")

		for i, v := range report.Vulnerabilities {
			buf.WriteString(fmt.Sprintf("\n[%d] %s (%s)\n", i+1, v.ID, v.Severity))
			buf.WriteString(fmt.Sprintf("    Category: %s\n", v.Category))
			buf.WriteString(fmt.Sprintf("    Description: %s\n", v.Description))
			buf.WriteString(fmt.Sprintf("    Location: %s\n", v.Location))
			buf.WriteString(fmt.Sprintf("    Recommendation: %s\n", v.Recommendation))
		}
	} else {
		buf.WriteString("No vulnerabilities detected.\n")
	}

	return buf.String()
}

// HexToBytes is a helper to convert hex string to bytes
func HexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}

// MustAddress converts string to AAL Address (panics on error)
func MustAddress(s string) aal.Address {
	// StringToAddress is not available in AAL, use BytesToAddress instead
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		panic(err)
	}
	return aal.BytesToAddress(b)
}
