package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// 版本信息
const (
	Version     = "0.1.0"
	VersionInfo = "aib-miner version " + Version
)

func main() {
	// 解析命令行参数
	flag.Usage = usage

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "start":
		startCmd(os.Args[2:])
	case "status":
		statusCmd()
	case "version":
		versionCmd()
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知命令 '%s'\n\n", command)
		usage()
		os.Exit(1)
	}
}

// usage 显示使用帮助
func usage() {
	fmt.Fprintf(os.Stderr, `AIB Miner - AIB 2.0 ZKML 矿工节点 CLI 工具

使用方式:
  aib-miner <命令> [选项]

命令:
  start      启动矿工节点
  status     查看节点状态
  version    显示版本信息

命令详情:
  aib-miner start --config miner.json      # 使用指定配置文件启动
  aib-miner status                          # 查看节点运行状态
  aib-miner version                         # 显示版本号

`)
}

// startCmd 处理 start 命令
func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径 (JSON 格式)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "解析参数失败: %v\n", err)
		os.Exit(1)
	}

	// 加载配置或使用默认配置
	var config *MinerConfig
	var err error

	if *configPath != "" {
		config, err = LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("使用配置文件: %s\n", *configPath)
	} else {
		config = DefaultMinerConfig()
		fmt.Println("使用默认配置")
		// 如果配置文件不存在，保存默认配置
		defaultConfigPath := "miner.json"
		if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
			if err := SaveConfig(defaultConfigPath, config); err != nil {
				fmt.Fprintf(os.Stderr, "警告: 无法保存默认配置: %v\n", err)
			} else {
				fmt.Printf("默认配置已保存到: %s\n", defaultConfigPath)
			}
		}
	}

	// 创建矿工实例
	miner, err := NewMiner(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建矿工失败: %v\n", err)
		os.Exit(1)
	}

	// 设置上下文和信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动矿工
	fmt.Println("正在启动矿工节点...")
	if err := miner.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "启动矿工失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("矿工节点已启动\n")
	fmt.Printf("  节点 ID: %s\n", config.NodeID)
	fmt.Printf("  模型: %s\n", config.Model)
	fmt.Printf("  Ollama: %s\n", config.OllamaURL)
	fmt.Printf("  监听地址: %s\n", config.ListenAddr)
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止节点")

	// 等待信号
	select {
	case sig := <-sigChan:
		fmt.Printf("\n收到信号: %v\n", sig)
	case <-ctx.Done():
		fmt.Println("\n上下文已取消")
	}

	// 停止矿工
	fmt.Println("正在停止矿工节点...")
	if err := miner.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "停止矿工时出错: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("矿工节点已停止")

	// 显示最终状态
	status := miner.Status()
	fmt.Printf("  运行时长: %v\n", status.Uptime.Round(time.Second))
	fmt.Printf("  处理任务数: %d\n", status.TasksProcessed)
}

// statusCmd 处理 status 命令
func statusCmd() {
	// 最小实现：显示版本信息
	// 实际实现应连接到运行中的节点获取状态
	fmt.Println(VersionInfo)
	fmt.Println()
	fmt.Println("注意: status 命令的完整实现需要连接到运行中的节点")
	fmt.Println("      当前显示版本信息作为参考")
}

// versionCmd 处理 version 命令
func versionCmd() {
	fmt.Println(VersionInfo)
}
