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
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
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
		fmt.Println("钱包创建成功")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("地址:      %s\n", v.Address)
		fmt.Printf("公钥:      %s\n", v.PublicKey)
		if v.Mnemonic != "" {
			fmt.Printf("助记词:    %s\n", v.Mnemonic)
			fmt.Println("\n重要: 请妥善保管助记词，它是恢复钱包的唯一方式！")
		}
	case *BalanceResponse:
		fmt.Println("余额信息")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("地址:      %s\n", v.Address)
		fmt.Printf("余额:      %s AIB\n", FormatAmount(v.Balance))
		fmt.Printf("UTXO数量:  %d\n", v.UTXOCount)
		if len(v.UTXOs) > 0 {
			fmt.Println("\nUTXO详情:")
			for i, utxo := range v.UTXOs {
				fmt.Printf("  %d. %s[%d] = %s AIB\n", i+1, shortHash(utxo.TxHash), utxo.Index, FormatAmount(utxo.Value))
			}
		}
	case *SendTransactionResponse:
		fmt.Println("交易已提交")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("交易哈希:  %s\n", v.TxHash)
		fmt.Printf("发送方:    %s\n", v.From)
		fmt.Printf("接收方:    %s\n", v.To)
		fmt.Printf("金额:      %s AIB\n", FormatAmount(v.Amount))
		fmt.Println("\n提示: 交易已进入内存池，等待确认...")
	case *TransactionStatusResponse:
		fmt.Println("交易状态")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("交易哈希:  %s\n", v.TxHash)
		fmt.Printf("状态:      %s\n", v.Status)
		if v.From != "" {
			fmt.Printf("发送方:    %s\n", v.From)
		}
		if v.To != "" {
			fmt.Printf("接收方:    %s\n", v.To)
		}
		if v.Amount > 0 {
			fmt.Printf("金额:      %s AIB\n", FormatAmount(v.Amount))
		}
		fmt.Printf("时间:      %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
	case *StakeResponse:
		fmt.Println("质押操作成功")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("交易哈希:    %s\n", v.TxHash)
		fmt.Printf("质押地址:    %s\n", v.Address)
		fmt.Printf("质押金额:    %s AIB\n", FormatAmount(v.Amount))
		fmt.Printf("当前质押:    %s AIB\n", FormatAmount(v.NewStake))
		fmt.Printf("全网质押:    %s AIB\n", FormatAmount(v.TotalStaked))
	case *NodeStatusResponse:
		fmt.Println("节点状态")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("状态:        %s\n", v.Status)
		fmt.Printf("版本:        %s\n", v.Version)
		fmt.Printf("运行时间:    %s\n", v.Uptime)
		fmt.Printf("当前高度:    %d\n", v.Height)
		fmt.Printf("最新区块:    %s\n", v.Hash)
		if v.LastBlock != "" {
			fmt.Printf("区块时间:    %s\n", v.LastBlock)
		}
		fmt.Printf("对等节点:    %d\n", v.PeerCount)
		if v.Syncing {
			fmt.Printf("同步中:      是 (%.2f%%)\n", v.SyncProgress*100)
		} else {
			fmt.Println("同步中:      否")
		}
	case *PeersResponse:
		fmt.Printf("对等节点列表 (共 %d 个)\n", v.Total)
		fmt.Println(strings.Repeat("-", 80))
		if len(v.Peers) == 0 {
			fmt.Println("暂无对等节点")
		} else {
			fmt.Printf("%-6s %-50s %-10s %-20s\n", "状态", "节点ID", "地址", "最后连接")
			fmt.Println(strings.Repeat("-", 80))
			for _, p := range v.Peers {
				status := "断开"
				if p.Connected {
					status = "连接"
				}
				lastSeen := "从未"
				if !p.LastSeen.IsZero() {
					lastSeen = p.LastSeen.Format("15:04:05")
				}
				fmt.Printf("%-6s %-50s %-10s %-20s\n", status, shortHash(p.ID), p.Address, lastSeen)
			}
		}
	case *BlockResponse:
		fmt.Println("区块信息")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("高度:        %d\n", v.Height)
		fmt.Printf("哈希:        %s\n", v.Hash)
		fmt.Printf("前一块:      %s\n", v.PrevHash)
		fmt.Printf("时间:        %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
		if v.Validator != "" {
			fmt.Printf("验证者:      %s\n", v.Validator)
		}
		if v.Proposer != "" {
			fmt.Printf("提议者:      %s\n", v.Proposer)
		}
		fmt.Printf("交易数量:    %d\n", v.TxCount)
		fmt.Printf("区块大小:    %d 字节\n", v.Size)
	case *HealthResponse:
		fmt.Println("健康检查")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Printf("状态:        %s\n", v.Status)
		fmt.Printf("版本:        %s\n", v.Version)
		fmt.Printf("运行时间:    %s\n", v.Uptime)
		fmt.Printf("检查时间:    %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
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
