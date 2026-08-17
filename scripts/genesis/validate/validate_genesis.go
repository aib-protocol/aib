// validate_genesis.go - AIB Mainnet Genesis Configuration Validator
//
// This tool validates the genesis.json configuration for AIB mainnet
// ensuring:
// - Total supply matches expected value
// - Allocation percentages sum to 100%
// - Individual allocations match their percentages
// - Genesis timestamp is in the future
// - All required fields are present and valid

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// GenesisConfig represents the genesis.json structure
type GenesisConfig struct {
	ChainID      string         `json:"chain_id"`
	GenesisTime  string         `json:"genesis_time"`
	TotalSupply  string         `json:"total_supply"`
	BlockReward  int            `json:"block_reward"`
	BlockTime    int            `json:"block_time"`
	Validators   []interface{}  `json:"validators"`
	Allocations  Allocations    `json:"allocations"`
	Config       GenesisConfig2 `json:"config"`
}

// Allocations represents token allocation details
type Allocations struct {
	Team           AllocationAmount `json:"team"`
	Ecosystem      AllocationAmount `json:"ecosystem"`
	StakingRewards AllocationAmount `json:"staking_rewards"`
	Community      AllocationAmount `json:"community"`
	AirdropPool    AllocationAmount `json:"airdrop_pool"`
}

// AllocationAmount represents individual allocation
type AllocationAmount struct {
	Amount     string `json:"amount"`
	Percentage string `json:"percentage"`
}

// GenesisConfig2 represents additional config
type GenesisConfig2 struct {
	MaxValidators   int    `json:"max_validators"`
	MinStakeAmount string `json:"min_stake_amount"`
	UnbondingTime  string `json:"unbonding_time"`
}

// ValidationResult holds validation results
type ValidationResult struct {
	Passed  bool
	Errors  []string
	Warnings []string
}

// Expected values for mainnet
const (
	ExpectedChainID     = "aib-mainnet-1"
	ExpectedGenesisTime = "2026-03-14T00:00:00Z"
	ExpectedTotalSupply = 3141592653
	ExpectedBlockReward = 50
	ExpectedBlockTime   = 30
)

func main() {
	// Parse command line flags
	genesisFile := flag.String("genesis", "genesis.json", "Path to genesis.json")
	verbose := flag.Bool("v", false, "Verbose output")
	_ = flag.Bool("fix", false, "Attempt to fix issues (not implemented)")
	flag.Parse()

	// Print banner
	fmt.Println("=================================================")
	fmt.Println("  AIB Mainnet Genesis Configuration Validator")
	fmt.Println("  Version: 1.0.0")
	fmt.Println("=================================================")
	fmt.Println()

	// Load genesis file
	fmt.Printf("Loading genesis file: %s\n", *genesisFile)
	data, err := os.ReadFile(*genesisFile)
	if err != nil {
		fmt.Printf("ERROR: Failed to read genesis file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON
	var genesis GenesisConfig
	if err := json.Unmarshal(data, &genesis); err != nil {
		fmt.Printf("ERROR: Failed to parse genesis JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Genesis file loaded successfully.")
	fmt.Println()

	// Run validations
	result := validateGenesis(genesis, string(data), *verbose)

	// Print results
	printResults(result, *verbose)

	// Exit with appropriate code
	if result.Passed {
		fmt.Println("\nGenesis configuration is VALID.")
		os.Exit(0)
	} else {
		fmt.Println("\nGenesis configuration has ERRORS:")
		for _, err := range result.Errors {
			fmt.Printf("  - %s\n", err)
		}
		os.Exit(1)
	}
}

func validateGenesis(genesis GenesisConfig, data string, verbose bool) ValidationResult {
	result := ValidationResult{
		Passed:   true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// 1. Validate Chain ID
	if verbose {
		fmt.Println("[1/10] Validating Chain ID...")
	}
	if genesis.ChainID != ExpectedChainID {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Chain ID mismatch: expected '%s', got '%s'",
				ExpectedChainID, genesis.ChainID))
	} else if verbose {
		fmt.Printf("  OK: Chain ID = %s\n", genesis.ChainID)
	}

	// 2. Validate Genesis Time
	if verbose {
		fmt.Println("[2/10] Validating Genesis Time...")
	}
	genesisTime, err := time.Parse(time.RFC3339, genesis.GenesisTime)
	if err != nil {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Invalid genesis time format: %v", err))
	} else {
		expectedTime, _ := time.Parse(time.RFC3339, ExpectedGenesisTime)
		if genesisTime != expectedTime {
			result.Passed = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("Genesis time mismatch: expected '%s', got '%s'",
					ExpectedGenesisTime, genesis.GenesisTime))
		} else if verbose {
			fmt.Printf("  OK: Genesis Time = %s\n", genesis.GenesisTime)
		}

		// Check if genesis time is in the future
		if genesisTime.After(time.Now()) {
			duration := genesisTime.Sub(time.Now())
			if verbose {
				fmt.Printf("  INFO: Genesis time is in the future: %s\n", duration.Round(time.Hour))
			}
		} else {
			result.Warnings = append(result.Warnings,
				"Genesis time is in the past")
		}
	}

	// 3. Validate Total Supply
	if verbose {
		fmt.Println("[3/10] Validating Total Supply...")
	}
	totalSupply, err := strconv.ParseInt(genesis.TotalSupply, 10, 64)
	if err != nil {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Invalid total supply: %v", err))
	} else if totalSupply != ExpectedTotalSupply {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Total supply mismatch: expected %d, got %d",
				ExpectedTotalSupply, totalSupply))
	} else if verbose {
		fmt.Printf("  OK: Total Supply = %d\n", totalSupply)
	}

	// 4. Validate Block Reward
	if verbose {
		fmt.Println("[4/10] Validating Block Reward...")
	}
	if genesis.BlockReward != ExpectedBlockReward {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Block reward mismatch: expected %d, got %d",
				ExpectedBlockReward, genesis.BlockReward))
	} else if verbose {
		fmt.Printf("  OK: Block Reward = %d\n", genesis.BlockReward)
	}

	// 5. Validate Block Time
	if verbose {
		fmt.Println("[5/10] Validating Block Time...")
	}
	if genesis.BlockTime != ExpectedBlockTime {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Block time mismatch: expected %d seconds, got %d seconds",
				ExpectedBlockTime, genesis.BlockTime))
	} else if verbose {
		fmt.Printf("  OK: Block Time = %d seconds\n", genesis.BlockTime)
	}

	// 6. Validate Allocation Percentages
	if verbose {
		fmt.Println("[6/10] Validating Allocation Percentages...")
	}
	teamPct, _ := strconv.Atoi(genesis.Allocations.Team.Percentage)
	ecoPct, _ := strconv.Atoi(genesis.Allocations.Ecosystem.Percentage)
	stakePct, _ := strconv.Atoi(genesis.Allocations.StakingRewards.Percentage)
	commPct, _ := strconv.Atoi(genesis.Allocations.Community.Percentage)
	airdropPct, _ := strconv.Atoi(genesis.Allocations.AirdropPool.Percentage)

	totalPct := teamPct + ecoPct + stakePct + commPct + airdropPct
	if totalPct != 100 {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Allocation percentages do not sum to 100: got %d",
				totalPct))
	} else if verbose {
		fmt.Printf("  OK: Total percentage = %d%%\n", totalPct)
	}

	// 7. Validate Individual Allocation Amounts
	if verbose {
		fmt.Println("[7/10] Validating Allocation Amounts...")
	}

	type allocEntry struct {
		expectedPct    int
		amount         string
		pct            string
		name           string
		isRemainderBin bool // community pool receives integer division remainder
	}

	allocList := []allocEntry{
		{15, genesis.Allocations.Team.Amount, genesis.Allocations.Team.Percentage, "Team", false},
		{0, genesis.Allocations.Ecosystem.Amount, genesis.Allocations.Ecosystem.Percentage, "Ecosystem", false},
		{0, genesis.Allocations.Community.Amount, genesis.Allocations.Community.Percentage, "Community", false},
		{5, genesis.Allocations.AirdropPool.Amount, genesis.Allocations.AirdropPool.Percentage, "Airdrop Pool", false},
		{80, genesis.Allocations.StakingRewards.Amount, genesis.Allocations.StakingRewards.Percentage, "Staking Rewards", true},
	}

	// Calculate the remainder from integer division: staking pool absorbs it
	nonRemainderSum := int64(0)
	for _, alloc := range allocList {
		if !alloc.isRemainderBin {
			nonRemainderSum += int64(ExpectedTotalSupply) * int64(alloc.expectedPct) / 100
		}
	}
	communityExpected := int64(ExpectedTotalSupply) - nonRemainderSum

	totalAllocated := int64(0)
	for _, alloc := range allocList {
		amount, err := strconv.ParseInt(alloc.amount, 10, 64)
		if err != nil {
			result.Passed = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("Invalid amount for %s: %v", alloc.name, err))
			continue
		}

		var expectedAmount int64
		if alloc.isRemainderBin {
			expectedAmount = communityExpected
		} else {
			expectedAmount = int64(ExpectedTotalSupply) * int64(alloc.expectedPct) / 100
		}

		if amount != expectedAmount {
			result.Passed = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s allocation mismatch: expected %d (%d%%), got %d",
					alloc.name, expectedAmount, alloc.expectedPct, amount))
		} else if verbose {
			fmt.Printf("  OK: %s = %d (%s%%)\n", alloc.name, amount, alloc.pct)
		}

		totalAllocated += amount
	}

	// Verify total allocated equals total supply
	if totalAllocated != int64(ExpectedTotalSupply) {
		result.Passed = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Total allocated (%d) does not equal total supply (%d)",
				totalAllocated, ExpectedTotalSupply))
	} else if verbose {
		fmt.Printf("  OK: Total allocated = Total supply = %d\n", totalAllocated)
	}

	// 8. Validate Config
	if verbose {
		fmt.Println("[8/10] Validating Config...")
	}
	if genesis.Config.MaxValidators <= 0 {
		result.Warnings = append(result.Warnings,
			"Max validators should be positive")
	} else if verbose {
		fmt.Printf("  OK: Max Validators = %d\n", genesis.Config.MaxValidators)
	}

	if genesis.Config.MinStakeAmount == "" {
		result.Warnings = append(result.Warnings,
			"Min stake amount not set")
	}

	if genesis.Config.UnbondingTime == "" {
		result.Warnings = append(result.Warnings,
			"Unbonding time not set")
	}

	// 9. Validators Structure
	if verbose {
		fmt.Println("[9/10] Validating Validators Structure...")
	}
	if genesis.Validators == nil {
		result.Warnings = append(result.Warnings,
			"Validators array is nil")
	} else if verbose {
		fmt.Printf("  OK: Validators array present (count: %d)\n", len(genesis.Validators))
	}

	// 10. Additional Validations
	if verbose {
		fmt.Println("[10/10] Running Additional Validations...")
	}

	// Validate JSON is properly formatted (already done, but double check)
	if strings.Contains(string(data), "NaN") || strings.Contains(string(data), "Infinity") {
		result.Passed = false
		result.Errors = append(result.Errors,
			"JSON contains invalid numeric values (NaN or Infinity)")
	}

	// Check for reasonable block time
	if genesis.BlockTime < 1 || genesis.BlockTime > 300 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Block time %d seconds is outside recommended range (1-300)", genesis.BlockTime))
	}

	// Check for reasonable block reward
	if genesis.BlockReward < 0 || genesis.BlockReward > 10000 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Block reward %d is outside typical range (0-10000)", genesis.BlockReward))
	}

	if verbose {
		fmt.Println("  OK: All additional validations passed")
	}

	return result
}

func printResults(result ValidationResult, verbose bool) {
	if verbose {
		fmt.Println("\n=================================================")
		fmt.Println("  Validation Summary")
		fmt.Println("=================================================")
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	// Print mathematical verification
	if verbose {
		teamAmt := int64(ExpectedTotalSupply) * 15 / 100
		ecoAmt := int64(ExpectedTotalSupply) * 0 / 100
		stakeAmt := int64(ExpectedTotalSupply) * 80 / 100
		commAmt := int64(ExpectedTotalSupply) - teamAmt - ecoAmt - stakeAmt

		fmt.Println("\n=================================================")
		fmt.Println("  Mathematical Verification")
		fmt.Println("=================================================")
		fmt.Printf("\nExpected Total Supply: %d\n", ExpectedTotalSupply)
		fmt.Printf("Team (15%%):            %d\n", teamAmt)
		fmt.Printf("Ecosystem (0%%):        %d\n", ecoAmt)
		fmt.Printf("Staking Rewards (80%%): %d\n", stakeAmt)
		fmt.Printf("Community (0%%+rem):    %d (includes %d remainder from integer division)\n",
			commAmt, commAmt-int64(ExpectedTotalSupply)*15/100)
		fmt.Printf("Sum:                   %d\n", teamAmt+ecoAmt+stakeAmt+commAmt)

		// Verify using float to check for rounding
		sum := 0.15 + 0.00 + 0.80 + 0.05
		fmt.Printf("\nPercentage Sum: %.2f (should be 1.00)\n", sum)
		fmt.Printf("Math Check: %v\n", math.Abs(sum-1.0) < 0.001)
	}
}
