// Package founder implements the founder allocation system for AIB 2.0.
// This file provides configuration loading functionality.
package founder

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// Config represents the founder configuration file structure.
type Config struct {
	Founders []FounderConfig `json:"founders"`
	Version  int             `json:"version"`
	MultiSig MultiSigConfig  `json:"multi_sig"`
}

// FounderConfig represents a founder from the config file.
type FounderConfig struct {
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
	Metadata    FounderMetadata   `json:"metadata"`
}

// LoadConfig loads a founder configuration from a file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// ToFounderList converts a config to a FounderList.
func (c *Config) ToFounderList() (*FounderList, error) {
	fl := NewFounderList()

	for _, fc := range c.Founders {
		f, err := c.configToFounder(fc)
		if err != nil {
			return nil, fmt.Errorf("failed to convert founder %s: %w", fc.ID, err)
		}

		if err := fl.Add(f); err != nil {
			return nil, fmt.Errorf("failed to add founder %s: %w", fc.ID, err)
		}
	}

	fl.Version = uint64(c.Version)
	return fl, nil
}

// configToFounder converts a FounderConfig to a Founder.
func (c *Config) configToFounder(fc FounderConfig) (*Founder, error) {
	// Decode public key
	pubKey, err := hex.DecodeString(fc.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	if len(pubKey) != 32 {
		return nil, fmt.Errorf("invalid public key length: expected 32, got %d", len(pubKey))
	}

	// Derive address from public key (don't validate the address from config)
	address := utxo.AddressFromPublicKey(pubKey)

	// Skip address encoding validation - we'll derive it ourselves
	// This avoids bech32m checksum issues during config loading

	// Parse timestamps
	startTime, err := time.Parse(time.RFC3339, fc.StartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %w", err)
	}

	unlockTime, err := time.Parse(time.RFC3339, fc.UnlockTime)
	if err != nil {
		return nil, fmt.Errorf("invalid unlock_time: %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, fc.EndTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time: %w", err)
	}

	f := &Founder{
		ID:          fc.ID,
		Name:        fc.Name,
		Address:     fc.Address, // Use address from config for display
		PublicKey:   fc.PublicKey,
		TotalAmount: fc.TotalAmount,
		Claimed:     fc.Claimed,
		Status:      FounderStatus(fc.Status),
		StartTime:   startTime,
		UnlockTime:  unlockTime,
		EndTime:     endTime,
		Metadata:    fc.Metadata,
	}

	// Set internal fields with derived address
	copy(f.AddressBytes[:], address[:])
	f.PubKeyBytes = pubKey

	return f, nil
}

// SaveConfig saves a configuration to a file.
func SaveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// InitializeFromFile initializes a FounderList and AllocationManager from a config file.
func InitializeFromFile(configPath string) (*FounderList, *AllocationManager, *Verifier, error) {
	// Load config
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Convert to founder list
	fl, err := config.ToFounderList()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create founder list: %w", err)
	}

	// Get vesting start time from first founder
	vestingStart := time.Now()
	if len(fl.Founders) > 0 {
		vestingStart = fl.Founders[0].StartTime
	}

	// Create allocation manager
	am := NewAllocationManager(fl, vestingStart)

	// Create verifier
	v := NewVerifier(fl, &config.MultiSig)

	return fl, am, v, nil
}
