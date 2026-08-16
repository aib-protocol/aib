// Package cli provides shared functionality for AIB command-line tools
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// File operations utilities

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 {
			return filepath.Join(home, path[1:])
		}
		return home
	}
	return path
}

// Wallet file operations

type WalletFile struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Mnemonic   string `json:"mnemonic,omitempty"`
}

func LoadWalletFile(path string) (*WalletFile, error) {
	path = expandPath(path)
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	var wf WalletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	return &wf, nil
}

func SaveWalletFile(path string, wf *WalletFile) error {
	path = expandPath(path)
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		_ = mkdirAll(dir)
	}

	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}

	return writeFile(path, data)
}
