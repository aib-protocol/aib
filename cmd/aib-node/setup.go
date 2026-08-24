package main

// setup: interactive first-run setup, replacing shell logic in install.sh.
// Usage: aib-node setup [--data-dir X] [--api-port N] [--p2p-port N]
// Prompts (via /dev/tty, fallback stdin, final fallback default-Y):
//   1. Create a new wallet now?
//   2. Start CPU mining now (validator mode)?
// All logic in Go: cross-platform, testable, no shell quirks.

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const setupAPIBase = "http://127.0.0.1:8080"

func setupTTYReader() *bufio.Reader {
	if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return bufio.NewReader(f)
	}
	return bufio.NewReader(os.Stdin)
}

func askYesNo(r *bufio.Reader, prompt string, def bool) bool {
	suffix := "[Y/n]"
	if !def {
		suffix = "[y/N]"
	}
	fmt.Printf("\n  %s %s ", prompt, suffix)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return def
	}
	line = strings.ToLower(strings.TrimSpace(line))
	switch line {
	case "":
		return def
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

func setupGet(path string) ([]byte, int, error) {
	resp, err := http.Get(setupAPIBase + path)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func setupPost(path string, body any) ([]byte, int, error) {
	data, _ := json.Marshal(body)
	resp, err := http.Post(setupAPIBase+path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func runSetup(dataDir string, apiPort, p2pPort int, nodeArgs []string) error {
	r := setupTTYReader()

	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║   AIB node setup                     ║")
	fmt.Println("╚══════════════════════════════════════╝")

	// wait for node API
	fmt.Print("  Waiting for node API... ")
	ok := false
	for i := 0; i < 30; i++ {
		if _, code, err := setupGet("/health"); err == nil && code == 200 {
			ok = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ok {
		return fmt.Errorf("node API not reachable at %s", setupAPIBase)
	}
	fmt.Println("✓")

	// 1. wallet
	if askYesNo(r, "Create a new wallet now?", true) {
		body, code, err := setupPost("/v1/wallet/create", map[string]string{"label": "main"})
		if err != nil || code != 200 {
			return fmt.Errorf("wallet create failed (HTTP %d): %s", code, string(body))
		}
		var resp struct {
			Data struct {
				Address     string `json:"address"`
				PrivateKey  string `json:"private_key"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil || resp.Data.Address == "" {
			return fmt.Errorf("wallet create: unexpected response: %s", string(body))
		}
		// save backup (0600)
		backup := filepath.Join(dataDir, "wallet-main.txt")
		_ = os.WriteFile(backup, []byte(fmt.Sprintf("address: %s\nprivate_key: %s\n", resp.Data.Address, resp.Data.PrivateKey)), 0600)
		fmt.Printf("  ✓ Wallet created\n    Address     : %s\n    Private key : %s\n    Backup      : %s\n", resp.Data.Address, resp.Data.PrivateKey, backup)
		fmt.Println("    ⚠ SAVE THE PRIVATE KEY — shown ONCE (also in backup file, keep secret!)")
		if _, err := hex.DecodeString(resp.Data.Address); err != nil {
			// non-fatal, just note it
		}
	}

	// 2. mining
	if askYesNo(r, "Start CPU mining now (validator mode)?", true) {
		// stop current node instance (best-effort, cross-platform: ask user if it fails)
		fmt.Println("  Restarting node in validator mode...")
		if err := stopNodeForRestart(); err != nil {
			fmt.Printf("  ! Could not stop the current node automatically: %v\n", err)
			fmt.Println("    Stop it manually, then run the validator restart command printed below.")
		} else {
			if err := startValidatorNode(dataDir, apiPort, p2pPort); err != nil {
				return fmt.Errorf("starting validator node: %w", err)
			}
			// verify mining
			time.Sleep(3 * time.Second)
			miningOK := false
			for i := 0; i < 10; i++ {
				if body, code, err := setupGet("/v1/mining"); err == nil && code == 200 {
					var m struct {
						Data struct {
							Mining bool `json:"mining"`
						} `json:"data"`
					}
					if json.Unmarshal(body, &m) == nil && m.Data.Mining {
						miningOK = true
						break
					}
				}
				time.Sleep(2 * time.Second)
			}
			if miningOK {
				fmt.Println("  ✓ MINING STARTED")
				fmt.Printf("    Stats  : curl %s/v1/mining\n", setupAPIBase)
				fmt.Printf("    Wallet : curl %s/v1/wallet/info\n", setupAPIBase)
			} else {
				fmt.Println("  ! Node restarted but mining flag not confirmed yet — check /v1/mining")
			}
		}
	} else {
		fmt.Println("  Mining not started (node keeps syncing as follower).")
	}

	fmt.Println("\n  Setup complete. Explorer: https://aib.one/explorer.html")
	return nil
}
