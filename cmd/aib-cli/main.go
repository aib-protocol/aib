// AIB CLI - Command-line interface for AIB 2.0 blockchain
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aib-protocol/aib/pkg/cli"
)

const (
	Version     = "2.0.0"
	VersionInfo = "aib-cli version " + Version
)

var (
	// Global flags
	apiEndpoint string
	outputFormat string
	verbose      bool
)

func main() {
	// Parse global flags first
	flag.StringVar(&apiEndpoint, "api", "http://127.0.0.1:8080", "API 端点地址")
	flag.StringVar(&outputFormat, "output", "text", "输出格式 (json|text|table)")
	flag.BoolVar(&verbose, "v", false, "详细输出")
	flag.BoolVar(&verbose, "verbose", false, "详细输出")

	// Custom usage
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	// Create API client
	client := cli.NewClient(apiEndpoint)

	// Parse output format
	format := cli.OutputFormat(outputFormat)

	// Execute command
	cmd := args[0]
	cmdArgs := args[1:]

	if err := executeCommand(client, cmd, cmdArgs, format); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, VersionInfo+`

用法:
  aib-cli [选项] <命令> [参数]

命令:
  wallet <子命令>      钱包操作
    create              创建新钱包
    restore <助记词>    从助记词恢复钱包
    balance <地址>      查询地址余额
    send <发送方> <接收方> <金额>  发送交易
    stake <地址> <金额> 质押
    unstake <地址> <金额> 解质押

  node <子命令>         节点操作
    status              查询节点状态
    peers               查看对等节点
    block <高度|哈希>   查看区块信息

  tx <哈希>             查询交易

  version              显示版本信息

选项:
  -api <地址>          API 端点 (默认: http://127.0.0.1:8080)
  -output <格式>       输出格式 (json|text|table, 默认: text)
  -v, -verbose         详细输出

示例:
  # 创建钱包
  aib-cli wallet create

  # 查询余额
  aib-cli wallet balance 0x1234...

  # 发送交易 (1.5 AIB)
  aib-cli wallet send 0x sender 0x recipient 1.5

  # 查看节点状态
  aib-cli node status

  # JSON 输出
  aib-cli -output json node status
`)
}

func executeCommand(client *cli.Client, cmd string, args []string, format cli.OutputFormat) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "执行命令: %s %s\n", cmd, strings.Join(args, " "))
		fmt.Fprintf(os.Stderr, "API 端点: %s\n", apiEndpoint)
	}

	switch cmd {
	case "wallet":
		return executeWalletCommand(client, args, format)

	case "node":
		return executeNodeCommand(client, args, format)

	case "tx", "transaction":
		return executeTxCommand(client, args, format)

	case "version", "--version", "-version":
		fmt.Println(VersionInfo)
		return nil

	case "help", "--help", "-h":
		usage()
		return nil

	default:
		return fmt.Errorf("未知命令: %s (使用 'aib-cli help' 查看帮助)", cmd)
	}
}

func executeWalletCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("钱包命令需要子命令 (create|restore|balance|send|stake|unstake)")
	}

	walletCmd := cli.NewWalletCommand(client, format)

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "create":
		savePath := ""
		if len(subArgs) > 0 {
			savePath = subArgs[0]
		}
		return walletCmd.Create(savePath)

	case "restore":
		if len(subArgs) < 1 {
			return fmt.Errorf("restore 命令需要助记词参数")
		}
		mnemonic := subArgs[0]
		savePath := ""
		if len(subArgs) > 1 {
			savePath = subArgs[1]
		}
		return walletCmd.Restore(mnemonic, savePath)

	case "balance":
		if len(subArgs) < 1 {
			return fmt.Errorf("balance 命令需要地址参数")
		}
		return walletCmd.Balance(subArgs[0])

	case "send":
		return handleSend(walletCmd, subArgs)

	case "stake":
		if len(subArgs) < 2 {
			return fmt.Errorf("stake 命令需要地址和金额参数")
		}
		amount, err := cli.ParseAmount(subArgs[1])
		if err != nil {
			return fmt.Errorf("无效的金额: %w", err)
		}
		return walletCmd.Stake(subArgs[0], amount)

	case "unstake":
		if len(subArgs) < 2 {
			return fmt.Errorf("unstake 命令需要地址和金额参数")
		}
		amount, err := cli.ParseAmount(subArgs[1])
		if err != nil {
			return fmt.Errorf("无效的金额: %w", err)
		}
		return walletCmd.Unstake(subArgs[0], amount)

	default:
		return fmt.Errorf("未知钱包子命令: %s", subCmd)
	}
}

func handleSend(walletCmd *cli.WalletCommand, args []string) error {
	// send <from> <to> <amount> [gas-limit] [gas-price]
	if len(args) < 3 {
		return fmt.Errorf("send 命令需要: 发送方地址、接收方地址和金额")
	}

	from := args[0]
	to := args[1]
	amount, err := cli.ParseAmount(args[2])
	if err != nil {
		return fmt.Errorf("无效的金额: %w", err)
	}

	gasLimit := uint64(21000) // Default gas limit
	gasPrice := uint64(1)     // Default gas price (1 wei)

	if len(args) > 3 {
		gl, err := cli.ParseAmount(args[3])
		if err != nil {
			return fmt.Errorf("无效的 gas limit: %w", err)
		}
		gasLimit = gl
	}

	if len(args) > 4 {
		gp, err := cli.ParseAmount(args[4])
		if err != nil {
			return fmt.Errorf("无效的 gas price: %w", err)
		}
		gasPrice = gp
	}

	return walletCmd.Send(from, to, amount, gasLimit, gasPrice)
}

func executeNodeCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("节点命令需要子命令 (status|peers|block)")
	}

	nodeCmd := cli.NewNodeCommand(client, format)

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "status":
		return nodeCmd.Status()

	case "peers":
		return nodeCmd.Peers()

	case "block":
		if len(subArgs) < 1 {
			return fmt.Errorf("block 命令需要区块高度或哈希参数")
		}
		return nodeCmd.Block(subArgs[0])

	default:
		return fmt.Errorf("未知节点子命令: %s", subCmd)
	}
}

func executeTxCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("tx 命令需要交易哈希参数")
	}

	txCmd := cli.NewTxCommand(client, format)
	return txCmd.Query(args[0])
}
