package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aib-protocol/aib/zkml/testnet"
)

func main() {
	// Parse command line flags
	var (
		nodeCount = flag.Int("nodes", 3, "Number of nodes in testnet")
		duration  = flag.Duration("duration", 5*time.Minute, "Testnet run duration")
		taskCount = flag.Int("tasks", 10, "Number of tasks to submit")
		verbose   = flag.Bool("verbose", false, "Enable verbose output")
	)
	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("  AIB 2.0 ZKML 3-Node Testnet")
	fmt.Println("========================================")
	fmt.Println()

	// Create testnet configuration
	config := &testnet.TestNetConfig{
		NodeCount:      *nodeCount,
		MinNodes:       3,
		CommitDuration: 100 * time.Millisecond,
		RevealDuration: 100 * time.Millisecond,
		TaskTimeout:    30 * time.Second,
		AutoSlash:      true,
		HonestRatio:    1.0,
	}

	// Create and start testnet
	tn := testnet.NewTestNet(config)

	fmt.Printf("Starting %d-node testnet...\n", *nodeCount)
	if err := tn.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start testnet: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Testnet started successfully")
	fmt.Println()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Run tasks
	fmt.Printf("Submitting %d tasks...\n", *taskCount)
	fmt.Println()

	successCount := 0
	failCount := 0
	totalSlashes := 0

	for i := 0; i < *taskCount; i++ {
		select {
		case <-ctx.Done():
			fmt.Println("\n⏱ Duration exceeded, stopping...")
			goto done
		case <-sigChan:
			fmt.Println("\n🛑 Interrupt received, stopping...")
			goto done
		default:
		}

		prompt := fmt.Sprintf("Test task %d: What is the capital of France?", i+1)
		result, err := tn.SubmitTask(prompt)

		if err != nil {
			if *verbose {
				fmt.Printf("[Task %d] ❌ Error: %v\n", i+1, err)
			}
			failCount++
			continue
		}

		if result.IsValid {
			successCount++
			if *verbose {
				fmt.Printf("[Task %d] ✅ Valid (agreement: %.1f%%, duration: %v)\n",
					i+1, result.AgreementRate*100, result.Duration)
			} else {
				fmt.Printf(".")
			}
		} else {
			failCount++
			if *verbose {
				fmt.Printf("[Task %d] ⚠️ Invalid (agreement: %.1f%%)\n",
					i+1, result.AgreementRate*100)
			} else {
				fmt.Printf("x")
			}
		}

		if result.SlashTriggered > 0 {
			totalSlashes += result.SlashTriggered
		}
	}

done:
	fmt.Println()
	fmt.Println()

	// Get final statistics
	stats := tn.GetStats()

	// Stop testnet
	fmt.Println("Stopping testnet...")
	tn.Stop()
	fmt.Println("✓ Testnet stopped")
	fmt.Println()

	// Print summary
	fmt.Println("========================================")
	fmt.Println("  Testnet Run Summary")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Printf("  Total Tasks:     %d\n", stats.TotalTasks)
	fmt.Printf("  Passed Tasks:    %d\n", stats.PassedTasks)
	fmt.Printf("  Failed Tasks:    %d\n", stats.FailedTasks)
	fmt.Printf("  Pass Rate:       %.1f%%\n", float64(stats.PassedTasks)/float64(stats.TotalTasks)*100)
	fmt.Printf("  Total Slashes:   %d\n", totalSlashes)
	fmt.Printf("  Avg Duration:    %v\n", stats.AvgDuration)
	fmt.Println()

	if stats.PassedTasks == stats.TotalTasks {
		fmt.Println("🎉 All tasks passed!")
	} else if stats.PassedTasks > stats.FailedTasks {
		fmt.Println("✅ Testnet run completed with some failures")
	} else {
		fmt.Println("⚠️ Testnet run completed with many failures")
	}
}
