// Package cli provides shared functionality for AIB command-line tools
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	FormatJSON  OutputFormat = "json"
	FormatText  OutputFormat = "text"
	FormatTable OutputFormat = "table"
)

// FormatOutput formats and prints output based on the specified format
func FormatOutput(data interface{}, format OutputFormat, err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	switch format {
	case FormatJSON:
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if e := encoder.Encode(data); e != nil {
			return e
		}
	case FormatTable, FormatText:
		// Default formatting
		formatAsText(data)
	}

	return nil
}

// formatAsText formats data as human-readable text
func formatAsText(data interface{}) {
	switch v := data.(type) {
	case *WalletCreateResponse:
		fmt.Println("Wallet created successfully")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Address:   %s\n", v.Address)
		fmt.Printf("Public key: %s\n", v.PublicKey)
		if v.Mnemonic != "" {
			fmt.Printf("Mnemonic:  %s\n", v.Mnemonic)
			fmt.Println("\nImportant: keep the mnemonic safe, it is the only way to recover the wallet!")
		}
	case *BalanceResponse:
		fmt.Println("Balance Information")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Address:   %s\n", v.Address)
		fmt.Printf("Balance:   %s AIB\n", FormatAmount(v.Balance))
		fmt.Printf("UTXO count: %d\n", v.UTXOCount)
		if len(v.UTXOs) > 0 {
			fmt.Println("\nUTXO details:")
			for i, utxo := range v.UTXOs {
				fmt.Printf("  %d. %s[%d] = %s AIB\n", i+1, shortHash(utxo.TxHash), utxo.Index, FormatAmount(utxo.Value))
			}
		}
	case *SendTransactionResponse:
		fmt.Println("Transaction submitted")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Tx hash:   %s\n", v.TxHash)
		fmt.Printf("From:      %s\n", v.From)
		fmt.Printf("To:        %s\n", v.To)
		fmt.Printf("Amount:    %s AIB\n", FormatAmount(v.Amount))
		fmt.Println("\nNote: the transaction has entered the mempool, waiting for confirmation...")
	case *TransactionStatusResponse:
		fmt.Println("Transaction Status")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Tx hash:   %s\n", v.TxHash)
		fmt.Printf("Status:    %s\n", v.Status)
		if v.From != "" {
			fmt.Printf("From:      %s\n", v.From)
		}
		if v.To != "" {
			fmt.Printf("To:        %s\n", v.To)
		}
		if v.Amount > 0 {
			fmt.Printf("Amount:    %s AIB\n", FormatAmount(v.Amount))
		}
		fmt.Printf("Time:      %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
	case *StakeResponse:
		fmt.Println("Stake operation successful")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Tx hash:      %s\n", v.TxHash)
		fmt.Printf("Address:      %s\n", v.Address)
		fmt.Printf("Amount:       %s AIB\n", FormatAmount(v.Amount))
		fmt.Printf("Current stake: %s AIB\n", FormatAmount(v.NewStake))
		fmt.Printf("Total staked: %s AIB\n", FormatAmount(v.TotalStaked))
	case *NodeStatusResponse:
		fmt.Println("Node Status")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Status:        %s\n", v.Status)
		fmt.Printf("Version:       %s\n", v.Version)
		fmt.Printf("Uptime:        %s\n", v.Uptime)
		fmt.Printf("Height:        %d\n", v.Height)
		fmt.Printf("Latest block:  %s\n", v.Hash)
		if v.LastBlock != "" {
			fmt.Printf("Block time:    %s\n", v.LastBlock)
		}
		fmt.Printf("Peers:         %d\n", v.PeerCount)
		if v.Syncing {
			fmt.Printf("Syncing:       yes (%.2f%%)\n", v.SyncProgress*100)
		} else {
			fmt.Println("Syncing:       no")
		}
	case *PeersResponse:
		fmt.Printf("Peer list (total %d)\n", v.Total)
		fmt.Println(strings.Repeat("-", 80))
		if len(v.Peers) == 0 {
			fmt.Println("No peers")
		} else {
			fmt.Printf("%-8s %-50s %-10s %-20s\n", "Status", "Node ID", "Address", "Last seen")
			fmt.Println(strings.Repeat("-", 80))
			for _, p := range v.Peers {
				status := "offline"
				if p.Connected {
					status = "online"
				}
				lastSeen := "never"
				if !p.LastSeen.IsZero() {
					lastSeen = p.LastSeen.Format("15:04:05")
				}
				fmt.Printf("%-6s %-50s %-10s %-20s\n", status, shortHash(p.ID), p.Address, lastSeen)
			}
		}
	case *BlockResponse:
		fmt.Println("Block Information")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Height:        %d\n", v.Height)
		fmt.Printf("Hash:          %s\n", v.Hash)
		fmt.Printf("Previous:      %s\n", v.PrevHash)
		fmt.Printf("Time:          %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
		if v.Validator != "" {
			fmt.Printf("Validator:     %s\n", v.Validator)
		}
		if v.Proposer != "" {
			fmt.Printf("Proposer:      %s\n", v.Proposer)
		}
		fmt.Printf("Tx count:      %d\n", v.TxCount)
		fmt.Printf("Block size:    %d bytes\n", v.Size)
	case *HealthResponse:
		fmt.Println("Health Check")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Printf("Status:        %s\n", v.Status)
		fmt.Printf("Version:       %s\n", v.Version)
		fmt.Printf("Uptime:        %s\n", v.Uptime)
		fmt.Printf("Checked at:    %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
	default:
		// Default JSON formatting
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(data)
	}
}

// shortHash returns a shortened version of a hash string
func shortHash(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return hash[:8] + "..." + hash[len(hash)-8:]
}
