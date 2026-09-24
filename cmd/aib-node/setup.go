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
	"strconv"
	"time"
)

var setupAPIBase = "http://127.0.0.1:8080"

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



// trimF formats a float without trailing zeros (for JSON amount_aib).
func trimF(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// setupGuideStaking: PoS-era staking guide (user directive 2026-09-22).
// Flexible-staking UX: stake is liquid, no lockup. If the node wallet has a
// spendable balance, offer to stake it right now (one question); otherwise
// print the validator wallet address and tell the user to fund it, then
// re-run setup (or POST /v1/stake) — staking activates within ~1 block.
func setupGuideStaking(r *bufio.Reader, dataDir string) {
	type walletInfo struct {
		Data struct {
			Address    string  `json:"address"`
			BalanceAIB float64 `json:"balance_aib"`
		} `json:"data"`
	}
	var w walletInfo
	haveInfo := false
	if body, code, err := setupGet("/v1/wallet/info"); err == nil && code == 200 {
		if json.Unmarshal(body, &w) == nil && w.Data.Address != "" {
			haveInfo = true
		}
	}
	// Liquid balance + staked amount from the stake info endpoint. NOTE:
	// /v1/wallet/info's balance_aib includes STAKED coins — using it here
	// caused a bogus "Stake 199,990 AIB now?" prompt (and a confusing
	// INSUFFICIENT_BALANCE error) for wallets that were ALREADY staking.
	var liquid, staked float64
	if body, code, err := setupGet("/v1/stake/info/" + w.Data.Address); err == nil && code == 200 {
		var si struct {
			Data struct {
				LiquidAIB float64 `json:"liquid_aib"`
				StakedAIB float64 `json:"staked_aib"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &si) == nil {
			liquid, staked = si.Data.LiquidAIB, si.Data.StakedAIB
		}
	}
	if liquid == 0 && staked == 0 {
		liquid = w.Data.BalanceAIB // endpoint unavailable — fall back
	}
	fmt.Println()
	fmt.Println("  ── PoS STAKING ─────────────────────────────────────")
	fmt.Println("  Flexible staking: AIB staked = mining weight.")
	fmt.Println("  Stake now → mining weight within ~1 block; unstake → coins back in ~2 blocks. No lockup.")
	if !haveInfo {
		fmt.Println("  Could not read wallet info; after sync run: curl " + setupAPIBase + "/v1/wallet/info")
		return
	}
	fmt.Printf("  Validator wallet: %s\n", w.Data.Address)
	if staked > 0 {
		fmt.Printf("  ✓ ALREADY STAKED: %.4f AIB — you are PoS mining right now.\n", staked)
		fmt.Printf("    Liquid (unstaked): %.4f AIB\n", liquid)
		if liquid < 1000 {
			fmt.Println("  Nothing more to do — stake covers all liquid balance.")
			return
		}
		fmt.Println("  Extra liquid balance available — you may stake more:")
	} else if liquid < 1000 {
		fmt.Printf("  Liquid balance : %.4f AIB — below the 1000 AIB minimum stake.\n", liquid)
		fmt.Println("  → Transfer AIB to the address above, then run:  aib-node setup  (it will offer to stake).")
		return
	}
	fmt.Printf("  Liquid balance : %.4f AIB\n", liquid)
	stakeAmt := liquid - 10 // keep a dust buffer for fees
	if !askYesNo(r, fmt.Sprintf("Stake %.0f AIB now and start PoS mining?", stakeAmt), true) {
		fmt.Println("  Skipped — stake any time: POST /v1/stake")
		return
	}
	// The validator wallet key IS the node key (node_key.pem, raw 64-byte
	// ed25519 seed+pub). The setup process can read it from the data dir.
	keyData, err := os.ReadFile(filepath.Join(dataDir, "node_key.pem"))
	if err != nil || len(keyData) < 64 {
		fmt.Println("  ! Could not read node_key.pem — stake manually with your wallet key:")
		fmt.Println("    POST /v1/stake {\"private_key\": \"<hex>\", \"amount_aib\": \"" + trimF(stakeAmt) + "\"}")
		return
	}
	pkHex := hex.EncodeToString(keyData[:64])
	body, code, err := setupPost("/v1/stake", map[string]string{
		"private_key": pkHex,
		"amount_aib":  trimF(stakeAmt),
	})
	if err != nil || code != 200 {
		fmt.Println()
		fmt.Println("  \033[1;37;41m                                                \033[0m")
		fmt.Printf("  \033[1;37;41m  ⚠ STAKE FAILED (HTTP %d) — READ THIS, DO NOT SKIP  \033[0m\n", code)
		fmt.Println("  \033[1;37;41m                                                \033[0m")
		fmt.Printf("  \033[0;31m  Error: %s\033[0m\n", string(body))
		fmt.Println("  \033[0;31m  If this says INSUFFICIENT_BALANCE but you DID stake before,\033[0m")
		fmt.Println("  \033[0;31m  your coins are already staked — nothing was lost. Check:\033[0m")
		fmt.Println("  \033[0;31m  curl " + setupAPIBase + "/v1/stake/info/" + w.Data.Address + "\033[0m")
		fmt.Println()
		fmt.Print("  \033[1;31m  Acknowledge before continuing [press Enter]\033[0m ")
		_, _ = r.ReadString('\n')
		return
	}
	fmt.Println("  \033[1;32m  ✓ STAKED — mining weight active from the next block.\033[0m")
	fmt.Println("    Check : curl " + setupAPIBase + "/v1/wallet/info")
}

func runSetup(dataDir string, apiPort, p2pPort int, nodeArgs []string) error {
	setupAPIBase = fmt.Sprintf("http://127.0.0.1:%d", apiPort)
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

	// 1. wallet — auto-skip if the node already has a key (existing wallet)
	haveWallet := false
	if _, err := os.Stat(filepath.Join(dataDir, "node_key.pem")); err == nil {
		haveWallet = true
	}
	if haveWallet {
		fmt.Println("  ✓ Existing wallet found (node_key.pem) — keeping it, no new wallet created.")
		if body, code, err := setupGet("/v1/wallet/info"); err == nil && code == 200 {
			var w struct {
				Data struct {
					Address string `json:"address"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &w) == nil && w.Data.Address != "" {
				fmt.Printf("    Address: %s\n", w.Data.Address)
			}
		}
	} else if askYesNo(r, "Create a new wallet now?", true) {
		body, code, err := setupPost("/v1/wallet/create", map[string]string{"label": "main"})
		if err != nil || code != 200 {
			return fmt.Errorf("wallet create failed (HTTP %d): %s", code, string(body))
		}
		var resp struct {
			Data struct {
				Address    string `json:"address"`
				PrivateKey string `json:"private_key"`
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

	// 2. mining — auto-skip when the PoW era is already over (chain height > 1000)
	powEraOver := false
	heightProbed := false
	if body, code, err := setupGet("/v1/block/latest"); err == nil && code == 200 {
		heightProbed = true
		var b struct {
			Data struct {
				Height uint64 `json:"height"` // /v1/block/latest: data.height (no header wrapper)
				Header struct {
					Height uint64 `json:"height"`
				} `json:"header"`
			} `json:"data"`
		}
		h := uint64(0)
		if json.Unmarshal(body, &b) == nil {
			h = b.Data.Height
			if h == 0 {
				h = b.Data.Header.Height
			}
		}
		if h > 1000 {
			powEraOver = true
		}
	}
	if powEraOver {
		fmt.Println("  ✓ PoW era is over — this chain is pure PoS now (no CPU mining, ever).")
		setupGuideStaking(r, dataDir)
	} else if !heightProbed {
		// Could not read chain height (fresh node, API not up yet). Assume the
		// PoW era is over (it ended long ago at height 10,000) — asking a new
		// validator to "start CPU mining" is a fossil prompt that confuses
		// PoS users. Skip it and point to staking instead.
		fmt.Println("  ✓ Validator mode — PoS only, CPU mining not needed.")
		fmt.Println("  Node is still syncing; staking will be offered once you have AIB.")
		fmt.Println("  (Re-run setup after sync, or stake any time: POST /v1/stake)")
	} else if askYesNo(r, "Start CPU mining now (validator mode)?", true) {
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
