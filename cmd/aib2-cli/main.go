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
	fmt.Fprintln(os.Stderr, "\nAIB 2.0 钱包命令行工具")
	fmt.Fprintln(os.Stderr, "\n用法:")
	fmt.Fprintln(os.Stderr, "  aib2-cli <command> [options]")
	fmt.Fprintln(os.Stderr, "\n可用命令:")
	fmt.Fprintln(os.Stderr, "  create [name]           创建新钱包")
	fmt.Fprintln(os.Stderr, "  list                    列出所有钱包")
	fmt.Fprintln(os.Stderr, "  address <name>          显示钱包地址")
	fmt.Fprintln(os.Stderr, "  public-key <name>       显示公钥")
	fmt.Fprintln(os.Stderr, "  private-key <name>      显示私钥 (谨慎使用)")
	fmt.Fprintln(os.Stderr, "  sign <name> <message>   签名消息")
	fmt.Fprintln(os.Stderr, "  verify <name> <message> <signature> 验证签名")
	fmt.Fprintln(os.Stderr, "  balance <name>          查询余额 (需要运行节点)")
	fmt.Fprintln(os.Stderr, "  send <from> <to> <amount> 发送交易")
	fmt.Fprintln(os.Stderr, "  backup <name> <file>    备份钱包到文件")
	fmt.Fprintln(os.Stderr, "  restore <file>          从文件恢复钱包")
	fmt.Fprintln(os.Stderr, "  version                 显示版本信息")
	fmt.Fprintln(os.Stderr, "  help                    显示此帮助信息")
	fmt.Fprintln(os.Stderr, "\n环境变量:")
	fmt.Fprintln(os.Stderr, "  AIB_WALLET_DIR          钱包存储目录 (默认 ~/.aib/wallets)")
	fmt.Fprintln(os.Stderr, "  AIB_NODE_URL            节点 RPC 地址 (默认 http://localhost:8545)")
	fmt.Fprintln(os.Stderr, "\n示例:")
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
		fmt.Fprintf(os.Stderr, "错误: 钱包 '%s' 已存在\n", name)
		os.Exit(1)
	}

	// Create new wallet using SDK
	sdk, err := wallet.NewWalletSDK(&wallet.SDKConfig{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建钱包失败: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "错误: 创建钱包目录失败: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 序列化钱包数据失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(walletPath, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存钱包失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 钱包创建成功!\n")
	fmt.Printf("  名称: %s\n", name)
	fmt.Printf("  地址: %s\n", data.Address)
	fmt.Printf("  公钥: %s\n", data.PublicKey)
	fmt.Printf("\n警告: 请妥善保管您的私钥，不要泄露给任何人!\n")
}

func listCmd() {
	dir := getWalletDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("没有找到钱包")
			return
		}
		fmt.Fprintf(os.Stderr, "错误: 读取钱包目录失败: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("没有找到钱包")
		return
	}

	fmt.Printf("找到 %d 个钱包:\n\n", len(entries))

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
		fmt.Printf("    地址: %s\n", wd.Address)
		fmt.Println()
	}
}

func loadWallet(name string) (*WalletData, error) {
	walletPath := getWalletPath(name)
	data, err := os.ReadFile(walletPath)
	if err != nil {
		return nil, fmt.Errorf("读取钱包失败: %w", err)
	}

	var wd WalletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return nil, fmt.Errorf("解析钱包数据失败: %w", err)
	}

	return &wd, nil
}

func addressCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli address <name>")
		os.Exit(1)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.Address)
}

func publicKeyCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli public-key <name>")
		os.Exit(1)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.PublicKey)
}

func privateKeyCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli private-key <name>")
		os.Exit(1)
	}

	// Safety check - require confirmation
	fmt.Fprintln(os.Stderr, "警告: 私钥是敏感信息，请确保您在安全的环境中使用!")
	fmt.Fprint(os.Stderr, "确认显示私钥? (yes/no): ")

	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Fprintln(os.Stderr, "操作已取消")
		os.Exit(0)
	}

	name := args[0]
	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(wd.PrivateKey)
}

func signCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli sign <name> <message>")
		os.Exit(1)
	}

	name := args[0]
	message := args[1]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// Decode private key
	privKeyBytes, err := hex.DecodeString(wd.PrivateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 解码私钥失败: %v\n", err)
		os.Exit(1)
	}

	if len(privKeyBytes) != ed25519.PrivateKeySize {
		fmt.Fprintf(os.Stderr, "错误: 私钥长度无效\n")
		os.Exit(1)
	}

	privKey := ed25519.PrivateKey(privKeyBytes)
	signature := ed25519.Sign(privKey, []byte(message))

	fmt.Printf("消息: %s\n", message)
	fmt.Printf("签名: %s\n", hex.EncodeToString(signature))
}

func verifyCmd(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli verify <name> <message> <signature>")
		os.Exit(1)
	}

	name := args[0]
	message := args[1]
	signatureHex := args[2]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// Decode public key and signature
	pubKeyBytes, err := hex.DecodeString(wd.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 解码公钥失败: %v\n", err)
		os.Exit(1)
	}

	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 解码签名失败: %v\n", err)
		os.Exit(1)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		fmt.Fprintf(os.Stderr, "错误: 公钥长度无效\n")
		os.Exit(1)
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	valid := ed25519.Verify(pubKey, []byte(message), signature)

	if valid {
		fmt.Println("✓ 签名验证成功!")
	} else {
		fmt.Println("✗ 签名验证失败!")
		os.Exit(1)
	}
}

func balanceCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli balance <name>")
		os.Exit(1)
	}

	name := args[0]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// TODO: Implement balance query via node RPC
	// This requires connecting to a running node
	fmt.Fprintf(os.Stderr, "注意: 余额查询功能需要连接到 AIB 节点\n")
	fmt.Printf("钱包地址: %s\n", wd.Address)
	fmt.Println("余额: 需要节点支持 (待实现)")
}

func sendCmd(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli send <from> <to> <amount>")
		os.Exit(1)
	}

	from := args[0]
	to := args[1]
	amount := args[2]

	// Load sender wallet
	wd, err := loadWallet(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// TODO: Implement transaction sending via node RPC
	// This requires connecting to a running node
	fmt.Fprintf(os.Stderr, "注意: 发送交易功能需要连接到 AIB 节点\n")
	fmt.Printf("发送方: %s\n", wd.Address)
	fmt.Printf("接收方: %s\n", to)
	fmt.Printf("金额: %s\n", amount)
	fmt.Println("状态: 需要节点支持 (待实现)")
}

func backupCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli backup <name> <file>")
		os.Exit(1)
	}

	name := args[0]
	file := args[1]

	wd, err := loadWallet(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 序列化钱包数据失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(file, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 写入备份文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 钱包 '%s' 已备份到 %s\n", name, file)
}

func restoreCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: aib2-cli restore <file> <name>")
		os.Exit(1)
	}

	file := args[0]
	name := args[1]

	// Read backup file
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取备份文件失败: %v\n", err)
		os.Exit(1)
	}

	var wd WalletData
	if err := json.Unmarshal(data, &wd); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 解析备份数据失败: %v\n", err)
		os.Exit(1)
	}

	// Save as new wallet
	walletPath := getWalletPath(name)
	if _, err := os.Stat(walletPath); err == nil {
		fmt.Fprintf(os.Stderr, "错误: 钱包 '%s' 已存在\n", name)
		os.Exit(1)
	}

	if err := os.MkdirAll(getWalletDir(), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建钱包目录失败: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 序列化钱包数据失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(walletPath, jsonData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存钱包失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 钱包已从 %s 恢复为 '%s'\n", file, name)
	fmt.Printf("  地址: %s\n", wd.Address)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}
