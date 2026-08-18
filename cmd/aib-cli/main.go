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
	apiEndpoint  string
	outputFormat string
	verbose      bool
)

func main() {
	// Parse global flags first
	flag.StringVar(&apiEndpoint, "api", "http://127.0.0.1:8080", "API endpoint address")
	flag.StringVar(&outputFormat, "output", "text", "Output format (json|text|table)")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")

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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, VersionInfo+`

Usage:
  aib-cli [options] <command> [args]

Commands:
  wallet <subcommand>  Wallet operations
    create              Create a new wallet
    restore <mnemonic>  Restore wallet from mnemonic
    balance <address>   Query address balance
    send <from> <to> <amount>  Send transaction
    stake <address> <amount> Stake tokens
    unstake <address> <amount> Unstake tokens

  node <subcommand>     Node operations
    status              Query node status
    peers               View peer nodes
    block <height|hash> View block info

  tx <hash>             Query transaction

  version              Show version info

Options:
  -api <address>       API endpoint (default: http://127.0.0.1:8080)
  -output <format>     Output format (json|text|table, default: text)
  -v, -verbose         Verbose output

Examples:
  # Create wallet
  aib-cli wallet create

  # Query balance
  aib-cli wallet balance 0x1234...

  # Send transaction (1.5 AIB)
  aib-cli wallet send 0x sender 0x recipient 1.5

  # View node status
  aib-cli node status

  # JSON output
  aib-cli -output json node status
`)
}

func executeCommand(client *cli.Client, cmd string, args []string, format cli.OutputFormat) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Executing command: %s %s\n", cmd, strings.Join(args, " "))
		fmt.Fprintf(os.Stderr, "API endpoint: %s\n", apiEndpoint)
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
		return fmt.Errorf("Unknown command: %s (use 'aib-cli help' for help)", cmd)
	}
}

func executeWalletCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("wallet command requires a subcommand (create|restore|balance|send|stake|unstake)")
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
			return fmt.Errorf("restore command requires a mnemonic argument")
		}
		mnemonic := subArgs[0]
		savePath := ""
		if len(subArgs) > 1 {
			savePath = subArgs[1]
		}
		return walletCmd.Restore(mnemonic, savePath)

	case "balance":
		if len(subArgs) < 1 {
			return fmt.Errorf("balance command requires an address argument")
		}
		return walletCmd.Balance(subArgs[0])

	case "send":
		return handleSend(walletCmd, subArgs)

	case "stake":
		if len(subArgs) < 2 {
			return fmt.Errorf("stake command requires address and amount arguments")
		}
		amount, err := cli.ParseAmount(subArgs[1])
		if err != nil {
			return fmt.Errorf("Invalid amount: %w", err)
		}
		return walletCmd.Stake(subArgs[0], amount)

	case "unstake":
		if len(subArgs) < 2 {
			return fmt.Errorf("unstake command requires address and amount arguments")
		}
		amount, err := cli.ParseAmount(subArgs[1])
		if err != nil {
			return fmt.Errorf("Invalid amount: %w", err)
		}
		return walletCmd.Unstake(subArgs[0], amount)

	default:
		return fmt.Errorf("Unknown wallet subcommand: %s", subCmd)
	}
}

func handleSend(walletCmd *cli.WalletCommand, args []string) error {
	// send <from> <to> <amount> [gas-limit] [gas-price]
	if len(args) < 3 {
		return fmt.Errorf("send command requires: sender address, recipient address and amount")
	}

	from := args[0]
	to := args[1]
	amount, err := cli.ParseAmount(args[2])
	if err != nil {
		return fmt.Errorf("Invalid amount: %w", err)
	}

	gasLimit := uint64(21000) // Default gas limit
	gasPrice := uint64(1)     // Default gas price (1 wei)

	if len(args) > 3 {
		gl, err := cli.ParseAmount(args[3])
		if err != nil {
			return fmt.Errorf("Invalid gas limit: %w", err)
		}
		gasLimit = gl
	}

	if len(args) > 4 {
		gp, err := cli.ParseAmount(args[4])
		if err != nil {
			return fmt.Errorf("Invalid gas price: %w", err)
		}
		gasPrice = gp
	}

	return walletCmd.Send(from, to, amount, gasLimit, gasPrice)
}

func executeNodeCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("node command requires a subcommand (status|peers|block)")
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
			return fmt.Errorf("block command requires a block height or hash argument")
		}
		return nodeCmd.Block(subArgs[0])

	default:
		return fmt.Errorf("Unknown node subcommand: %s", subCmd)
	}
}

func executeTxCommand(client *cli.Client, args []string, format cli.OutputFormat) error {
	if len(args) < 1 {
		return fmt.Errorf("tx command requires a transaction hash argument")
	}

	txCmd := cli.NewTxCommand(client, format)
	return txCmd.Query(args[0])
}
