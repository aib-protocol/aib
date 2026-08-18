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

// Version information
const (
	Version     = "0.1.0"
	VersionInfo = "aib-miner version " + Version
)

func main() {
	// Parse command line arguments
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
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n\n", command)
		usage()
		os.Exit(1)
	}
}

// usage displays help information
func usage() {
	fmt.Fprintf(os.Stderr, `AIB Miner - AIB 2.0 ZKML miner node CLI tool

Usage:
  aib-miner <command> [options]

Commands:
  start      Start the miner node
  status     Show node status
  version    Show version information

Command details:
  aib-miner start --config miner.json      # Start with the specified config file
  aib-miner status                          # Show node running status
  aib-miner version                         # Show version number

`)
}

// startCmd handles the start command
func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Config file path (JSON format)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	// Load config or use default config
	var config *MinerConfig
	var err error

	if *configPath != "" {
		config, err = LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Using config file: %s\n", *configPath)
	} else {
		config = DefaultMinerConfig()
		fmt.Println("Using default config")
		// If the config file does not exist, save the default config
		defaultConfigPath := "miner.json"
		if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
			if err := SaveConfig(defaultConfigPath, config); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save default config: %v\n", err)
			} else {
				fmt.Printf("Default config saved to: %s\n", defaultConfigPath)
			}
		}
	}

	// Create miner instance
	miner, err := NewMiner(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create miner: %v\n", err)
		os.Exit(1)
	}

	// Set up context and signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for system signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the miner
	fmt.Println("Starting miner node...")
	if err := miner.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start miner: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Miner node started\n")
	fmt.Printf("  Node ID: %s\n", config.NodeID)
	fmt.Printf("  Model: %s\n", config.Model)
	fmt.Printf("  Ollama: %s\n", config.OllamaURL)
	fmt.Printf("  Listen address: %s\n", config.ListenAddr)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the node")

	// Wait for signal
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal: %v\n", sig)
	case <-ctx.Done():
		fmt.Println("\nContext cancelled")
	}

	// Stop the miner
	fmt.Println("Stopping miner node...")
	if err := miner.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping miner: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Miner node stopped")

	// Show final status
	status := miner.Status()
	fmt.Printf("  Uptime: %v\n", status.Uptime.Round(time.Second))
	fmt.Printf("  Tasks processed: %d\n", status.TasksProcessed)
}

// statusCmd handles the status command
func statusCmd() {
	// Minimal implementation: show version information
	// The full implementation should connect to a running node to get status
	fmt.Println(VersionInfo)
	fmt.Println()
	fmt.Println("Note: the full implementation of the status command requires connecting to a running node")
	fmt.Println("      Currently showing version information as a reference")
}

// versionCmd handles the version command
func versionCmd() {
	fmt.Println(VersionInfo)
}
