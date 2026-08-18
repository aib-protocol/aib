package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aib-protocol/aib/pkg/wallet"
)

const (
	Version     = "0.1.0"
	VersionInfo = "aib2-cli version " + Version
)

// Default wallet directory
var defaultWalletDir = filepath.Join(os.Getenv("HOME"), ".aib", "wallets")

// WalletData stores wallet information
type WalletData struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "create":
		createCmd(os.Args[2:])
	case "address":
		addressCmd(os.Args[2:])
	case "public-key":
		publicKeyCmd(os.Args[2:])
	case "private-key":
		privateKeyCmd(os.Args[2:])
	case "sign":
		signCmd(os.Args[2:])
	case "verify":
		verifyCmd(os.Args[2:])
	case "balance":
		balanceCmd(os.Args[2:])
	case "send":
		sendCmd(os.Args[2:])
	case "backup":
		backupCmd(os.Args[2:])
	case "restore":
		restoreCmd(os.Args[2:])
	case "list":
		listCmd()
	case "version":
		versionCmd()
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, VersionInfo)
	fmt.Fprintln(os.Stderr, "\nAIB 2.0 wallet command-line tool")
	fmt.Fprintln(os.Stderr, "\nUsage:")
	fmt.Fprintln(os.Stderr, "  aib2-cli <command> [options]")
	fmt.Fprintln(os.Stderr, "\nAvailable commands:")
	fmt.Fprintln(os.Stderr, "  create [name]           Create a new wallet")
	fmt.Fprintln(os.Stderr, "  list                    List all wallets")
	fmt.Fprintln(os.Stderr, "  address <name>          Show wallet address")
	fmt.Fprintln(os.Stderr, "  public-key <name>       Show public key")
	fmt.Fprintln(os.Stderr, "  private-key <name>      Show private key (use with caution)")
	fmt.Fprintln(os.Stderr, "  sign <name> <message>   Sign a message")
	fmt.Fprintln(os.Stderr, "  verify <name> <message> <signature> Verify a signature")
	fmt.Fprintln(os.Stderr, "  balance <name>          Query balance (requires a running node)")
	fmt.Fprintln(os.Stderr, "  send <from> <to> <amount> Send a transaction")
	fmt.Fprintln(os.Stderr, "  backup <name> <file>    Back up wallet to file")
	fmt.Fprintln(os.Stderr, "  restore <file>          Restore wallet from file")
	fmt.Fprintln(os.Stderr, "  version                 Show version information")
	fmt.Fprintln(os.Stderr, "  help                    Show this help message")
	fmt.Fprintln(os.Stderr, "\nEnvironment variables:")
	fmt.Fprintln(os.Stderr, "  AIB_WALLET_DIR          Wallet storage directory (default ~/.aib/wallets)")
	fmt.Fprintln(os.Stderr, "  AIB_NODE_URL            Node RPC address (default http://localhost:8545)")
	fmt.Fprintln(os.Stderr, "\nExamples:")
	fmt.Fprintln(os.Stderr, "  aib2-cli create my-wallet")
	fmt.Fprintln(os.Stderr, "  aib2-cli address my-wallet")
	fmt.Fprintln(os.Stderr, "  aib2-cli sign my-wallet \"Hello AIB\"")
}

func versionCmd() {
	fmt.Println(VersionInfo)
}

func getWalletDir() string {
	if dir := os.Getenv("AIB_WALLET_DIR"); dir != "" {
		return dir
	}
	return defaultWalletDir
}

func getWalletPath(name string) string {
	return filepath.Join(getWalletDir(), name+".json")
}

func createCmd(args []string) {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	if name == "" {
		// Generate default name
		name = "wallet-" + randomString(8)
	}

	// Check if wallet already exists
	walletPath := getWalletPath(name)
	if _, err := os.Stat(walletPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: wallet '%s' already exists\n", name)
		os.Exit(1)
	}

	// Create new wallet using SDK
	sdk, err := wallet.NewWalletSDK(&wallet.SDKConfig{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create wallet: %v\n", err)
		os.Exit(1)
	}

	w := sdk
	address := w.GetAddress()
	pubKey := w.GetPublicKey()
	privKey := w.ExportPrivateKey()

	// Create wallet data
	data := WalletData{
		Address:    hex.EncodeToString(address[:]),
		PublicKey:  hex.EncodeToString(pubKey),
		PrivateKey: hex.EncodeToString(privKey),
	}

	// Save wallet
	if err := os.MkdirAll(getWalletDir(), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create wallet directory: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to serialize wallet data: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(walletPath, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save wallet: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Wallet created successfully!\n")
	fmt.Printf("  Name: %s\n", name)
	fmt.Printf("  Address: %s\n", data.Address)
	fmt.Printf("  Public key: %s\n", data.PublicKey)
	fmt.Printf("\nWarning: keep your private key safe and never disclose it to anyone!\n")
}

func listCmd() {
	dir := getWalletDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No wallets found")
			return
		}
		fmt.Fprintf(os.Stderr, "Error: failed to read wallet directory: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No wallets found")
		return
	}

	fmt.Printf("Found %d wallets:\n\n", len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		walletPath := getWalletPath(name)

		data, err := os.ReadFile(walletPath)
		if err != nil {
			continue
		}

		var wd WalletData
		if err := json.Unmarshal(data, &wd); err != nil {
			continue
		}

		fmt.Printf("  %s\n", name)
		fmt.Printf("    Address: %s\n", wd.Address)
		fmt.Println()
	}
}

func loadWallet(name string) (*WalletData, error) {
	walletPath := getWalletPath(name)
	data, err := os.ReadFile(walletPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wallet: %w", err)
	}

	var wd WalletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return nil, fmt.Errorf("failed to parse wallet data: %w", err)
	}

	return &wd, nil
}

func addressCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli address <name>")
		os.Exit(1)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.Address)
}

func publicKeyCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli public-key <name>")
		os.Exit(1)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.PublicKey)
}

func privateKeyCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli private-key <name>")
		os.Exit(1)
	}

	// Safety check - require confirmation
	fmt.Fprintln(os.Stderr, "Warning: the private key is sensitive information, make sure you are in a secure environment!")
	fmt.Fprint(os.Stderr, "Confirm displaying the private key? (yes/no): ")

	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Fprintln(os.Stderr, "Operation cancelled")
		os.Exit(0)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.PrivateKey)
}

func signCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli sign <name> <message>")
		os.Exit(1)
	}

	name := args[0]
	message := args[1]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Decode private key
	privKeyBytes, err := hex.DecodeString(wd.PrivateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode private key: %v\n", err)
		os.Exit(1)
	}

	if len(privKeyBytes) != ed25519.PrivateKeySize {
		fmt.Fprintf(os.Stderr, "Error: invalid private key length\n")
		os.Exit(1)
	}

	privKey := ed25519.PrivateKey(privKeyBytes)
	signature := ed25519.Sign(privKey, []byte(message))

	fmt.Printf("Message: %s\n", message)
	fmt.Printf("Signature: %s\n", hex.EncodeToString(signature))
}

func verifyCmd(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli verify <name> <message> <signature>")
		os.Exit(1)
	}

	name := args[0]
	message := args[1]
	signatureHex := args[2]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Decode public key and signature
	pubKeyBytes, err := hex.DecodeString(wd.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode public key: %v\n", err)
		os.Exit(1)
	}

	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode signature: %v\n", err)
		os.Exit(1)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		fmt.Fprintf(os.Stderr, "Error: invalid public key length\n")
		os.Exit(1)
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	valid := ed25519.Verify(pubKey, []byte(message), signature)

	if valid {
		fmt.Println("✓ Signature verified successfully!")
	} else {
		fmt.Println("✗ Signature verification failed!")
		os.Exit(1)
	}
}

func balanceCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli balance <name>")
		os.Exit(1)
	}

	name := args[0]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// TODO: Implement balance query via node RPC
	// This requires connecting to a running node
	fmt.Fprintf(os.Stderr, "Note: the balance query feature requires a connection to an AIB node\n")
	fmt.Printf("Wallet address: %s\n", wd.Address)
	fmt.Println("Balance: requires node support (not yet implemented)")
}

func sendCmd(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli send <from> <to> <amount>")
		os.Exit(1)
	}

	from := args[0]
	to := args[1]
	amount := args[2]

	// Load sender wallet
	wd, err := loadWallet(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// TODO: Implement transaction sending via node RPC
	// This requires connecting to a running node
	fmt.Fprintf(os.Stderr, "Note: the send transaction feature requires a connection to an AIB node\n")
	fmt.Printf("Sender: %s\n", wd.Address)
	fmt.Printf("Recipient: %s\n", to)
	fmt.Printf("Amount: %s\n", amount)
	fmt.Println("Status: requires node support (not yet implemented)")
}

func backupCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli backup <name> <file>")
		os.Exit(1)
	}

	name := args[0]
	file := args[1]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to serialize wallet data: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(file, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write backup file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Wallet '%s' backed up to %s\n", name, file)
}

func restoreCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: aib2-cli restore <file> <name>")
		os.Exit(1)
	}

	file := args[0]
	name := args[1]

	// Read backup file
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read backup file: %v\n", err)
		os.Exit(1)
	}

	var wd WalletData
	if err := json.Unmarshal(data, &wd); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse backup data: %v\n", err)
		os.Exit(1)
	}

	// Save as new wallet
	walletPath := getWalletPath(name)
	if _, err := os.Stat(walletPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: wallet '%s' already exists\n", name)
		os.Exit(1)
	}

	if err := os.MkdirAll(getWalletDir(), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create wallet directory: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to serialize wallet data: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(walletPath, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save wallet: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Wallet restored from %s as '%s'\n", file, name)
	fmt.Printf("  Address: %s\n", wd.Address)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}
