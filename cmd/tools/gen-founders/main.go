//go:build ignore
// +build ignore

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

func main() {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	unlockTime := startTime.AddDate(1, 0, 0)
	endTime := unlockTime.AddDate(1, 0, 0)

	type Founder struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Address     string            `json:"address"`
		PublicKey   string            `json:"public_key"`
		TotalAmount uint64            `json:"total_amount"`
		Claimed     uint64            `json:"claimed"`
		Status      string            `json:"status"`
		StartTime   string            `json:"start_time"`
		UnlockTime  string            `json:"unlock_time"`
		EndTime     string            `json:"end_time"`
		Metadata    map[string]string `json:"metadata"`
	}

	founderData := []struct {
		id          string
		name        string
		role        string
		description string
	}{
		{"f001", "Alice", "Core Developer", "Lead blockchain architect"},
		{"f002", "Bob", "Protocol Designer", "Consensus mechanism designer"},
		{"f003", "Charlie", "Security Lead", "Smart contract security expert"},
		{"f004", "Diana", "Ecosystem Growth", "Business development lead"},
		{"f005", "Eve", "Community Manager", "Community and operations"},
	}

	founders := make([]Founder, 0, len(founderData))

	for _, fd := range founderData {
		pubKey, _, _ := ed25519.GenerateKey(rand.Reader)
		pubKeyHex := hex.EncodeToString(pubKey)

		address := utxo.AddressFromPublicKey(pubKey)
		addrStr, err := utxo.AddressToString(address)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding address: %v\n", err)
			os.Exit(1)
		}

		f := Founder{
			ID:          fd.id,
			Name:        fd.name,
			Address:     addrStr,
			PublicKey:   pubKeyHex,
			TotalAmount: 3141,
			Claimed:     0,
			Status:      "locked",
			StartTime:   startTime.Format(time.RFC3339),
			UnlockTime:  unlockTime.Format(time.RFC3339),
			EndTime:     endTime.Format(time.RFC3339),
			Metadata: map[string]string{
				"description": fd.description,
				"role":        fd.role,
				"joined_at":   startTime.Format(time.RFC3339),
			},
		}
		founders = append(founders, f)
	}

	config := map[string]interface{}{
		"founders": founders,
		"version":  1,
		"multi_sig": map[string]interface{}{
			"required_sigs":    3,
			"signer_addresses": []string{},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
