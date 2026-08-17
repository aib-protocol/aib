// Package eutxo implements Cardano-style Extended UTXO (eUTXO) smart contract model.
// This file contains example contracts and usage patterns.
package eutxo

import (
	"crypto/sha256"
	"fmt"
)

// sha256 import
// ============================================================================
// Example Contracts
// ============================================================================

// Example 1: Simple Payment (P2PKH)
// Basic wallet-style payment using public key hash.

func ExampleSimplePayment() {
	// Generate keys
	privKey, pubKey := generateTestKey()

	// Create transaction
	tx := NewTransaction(1, 100)

	// Add input (from existing UTXO)
	var prevTxID [32]byte
	prevTxID[0] = 1

	// Sign transaction
	redeemer := NewSpendRedeemer(nil, pubKey) // Will sign later

	tx.AddInput(eTXInput{
		TxID:     prevTxID,
		Index:    0,
		Value:    5000000,
		Redeemer: redeemer,
	})

	// Add output (recipient)
	var recipientHash [28]byte
	recipientHash[0] = 0xAA
	tx.AddOutput(eTXOutput{
		Address: NewPubKeyAddress(recipientHash),
		Value:   4000000,
	})

	tx.Fee = 1000000
	tx.ComputeHash()

	// Sign with private key
	tx.Sign(privKey)

	fmt.Printf("Simple Payment TX: %x\n", tx.Hash)
}

// Example 2: Multi-Signature Wallet
// Requires M-of-N signatures to spend.

func ExampleMultiSigWallet() {
	// Generate keys
	_, pubKeysEd := generateTestKeys(5)

	// Convert []ed25519.PublicKey to [][]byte
	pubKeys := make([][]byte, len(pubKeysEd))
	for i, pk := range pubKeysEd {
		pubKeys[i] = pk
	}

	// Create 3-of-5 multi-sig script
	script, _ := CreateMultiSigScript(MultiSigParams{
		Threshold: 3,
		PubKeys:   pubKeys,
	})

	// Create datum with policy
	datum, _ := NewDatumFromJSON(map[string]interface{}{
		"threshold": 3,
		"pubKeys":   pubKeys,
	})

	fmt.Printf("Multi-Sig Script: %x\n", script)
	fmt.Printf("Multi-Sig Datum: %x\n", datum.Data)
}

// Example 3: Time-Locked Contract
// Funds locked until a specific slot number.

func ExampleTimelockContract() {
	// Generate keys
	_, pubKey := generateTestKey()

	// Create time-lock script (lock until slot 10000)
	script, _ := CreateTimelockScript(TimelockParams{
		LockSlot:  10000,
		PublicKey: pubKey,
	})

	// Create datum with lock information
	datum, _ := NewDatumFromJSON(map[string]interface{}{
		"lockSlot":  10000,
		"publicKey": pubKey,
	})

	// Create time-locked UTXO
	var txID [32]byte
	txID[0] = 1

	utxo := &eUTXO{
		TxID:      txID,
		Index:     0,
		Value:     10000000,
		Script:    script,
		Datum:     datum.Data,
		CreatedAt: 5000, // Created at slot 5000
	}

	fmt.Printf("Time-Locked UTXO: locked until slot %d\n", 10000)
	fmt.Printf("Script: %x\n", utxo.Script)
}

// Example 4: Escrow Contract
// Two-party escrow with timeout for seller recovery.

func ExampleEscrowContract() {
	// Generate keys for buyer and seller
	_, buyerPub := generateTestKey()
	_, sellerPub := generateTestKey()

	// Create escrow script
	script, _ := CreateEscrowScript(EscrowParams{
		BuyerPubKey:  buyerPub,
		SellerPubKey: sellerPub,
		TimeoutSlot:  100000,
	})

	// Create datum with escrow terms
	_, _ = NewDatumFromJSON(map[string]interface{}{
		"buyerPubKey":  buyerPub,
		"sellerPubKey": sellerPub,
		"timeoutSlot":  100000,
	})

	fmt.Printf("Escrow Contract: buyer or seller (after timeout)\n")
	fmt.Printf("Buyer: %x\n", buyerPub[:8])
	fmt.Printf("Seller: %x\n", sellerPub[:8])
	fmt.Printf("Timeout Slot: %d\n", 100000)
	fmt.Printf("Script: %x\n", script)
}

// Example 5: State Machine Contract
// Maintains state across transactions using datum.

func ExampleStateMachineContract() {
	// Define state transitions
	_ = map[string][]string{
		"Created":   {"Funded", "Cancelled"},
		"Funded":    {"Active", "Refunded"},
		"Active":    {"Completed", "Cancelled"},
		"Completed": {},
		"Cancelled": {},
	}

	// Create initial state datum
	initialState, _ := NewDatumFromJSON(map[string]interface{}{
		"state":        "Created",
		"step":         0,
		"participants": []string{"Alice", "Bob"},
	})

	fmt.Printf("State Machine Initial State: %s\n", initialState.Data)

	// State validation would check:
	// 1. Current state in datum
	// 2. Redeemer contains new state
	// 3. Transition is valid
	// 4. Update datum in output

	// Example transition: Created -> Funded
	nextState, _ := NewDatumFromJSON(map[string]interface{}{
		"state":        "Funded",
		"step":         1,
		"participants": []string{"Alice", "Bob"},
	})

	fmt.Printf("State Machine Next State: %s\n", nextState.Data)
}

// Example 6: Crowdfunding Contract
// Collects funds until a goal is reached or deadline passes.

type CrowdfundingState struct {
	Creator     []byte
	GoalAmount  uint64
	Deadline    uint64
	TotalRaised uint64
	Backers     [][]byte
	IsActive    bool
}

func ExampleCrowdfundingContract() {
	// Create crowdfunding datum
	cfState := CrowdfundingState{
		GoalAmount:  100000000, // 100 ADA
		Deadline:    50000,
		TotalRaised: 0,
		IsActive:    true,
	}

	datum, _ := NewDatumFromJSON(cfState)

	// Contract logic:
	// - Contributors can add funds while IsActive and Deadline not passed
	// - When GoalAmount is reached, creator can claim
	// - When Deadline passes and goal not reached, contributors can refund
	// - Creator can cancel (refund all) at any time

	fmt.Printf("Crowdfunding Contract: goal %d, deadline %d\n",
		cfState.GoalAmount, cfState.Deadline)
	fmt.Printf("Initial Datum: %s\n", datum.Data)
}

// Example 7: Token Vesting Contract
// Releases tokens according to a schedule.

type VestingSchedule struct {
	TotalAmount   uint64
	StartTime     uint64
	CliffTime     uint64
	VestingPeriod uint64 // in slots
	Interval      uint64 // unlock per interval
	Beneficiary   []byte
}

func ExampleVestingContract() {
	// Create vesting schedule
	schedule := VestingSchedule{
		TotalAmount:   1000000000, // 1000 tokens
		StartTime:     10000,
		CliffTime:     20000,   // 1 year cliff
		VestingPeriod: 1000000, // 1 year total
		Interval:      10000,   // unlock every 10K slots
		Beneficiary:   make([]byte, 32),
	}

	datum, _ := NewDatumFromJSON(schedule)

	// Contract logic:
	// - Calculate vested amount based on current slot
	// - Allow withdrawal of vested amount
	// - Update datum with remaining amount

	fmt.Printf("Vesting Contract: %d tokens with cliff at %d\n",
		schedule.TotalAmount, schedule.CliffTime)
	fmt.Printf("Datum: %s\n", datum.Data)
}

// ============================================================================
// Contract Template Builders
// ============================================================================

// BuildP2PKHContract creates a P2PKH contract.
func BuildP2PKHContract(pubKey []byte, amount uint64) (*eUTXO, error) {
	var txID [32]byte
	txID[0] = 1

	script, _ := CreateP2PKHScript()

	return &eUTXO{
		TxID:      txID,
		Index:     0,
		Value:     amount,
		Address:   NewPubKeyAddress(pubKeyToHash(pubKey)),
		Script:    script,
		CreatedAt: 0,
	}, nil
}

// BuildMultiSigContract creates a multi-sig contract.
func BuildMultiSigContract(
	pubKeys [][]byte,
	threshold uint32,
	amount uint64,
) (*eUTXO, error) {
	var txID [32]byte
	txID[0] = 1

	script, _ := CreateMultiSigScript(MultiSigParams{
		Threshold: threshold,
		PubKeys:   pubKeys,
	})

	// Create script address from script hash
	scriptHash := sha256.Sum256(script)
	var addrHash [28]byte
	copy(addrHash[:], scriptHash[:28])

	datum, _ := NewDatumFromJSON(map[string]interface{}{
		"threshold": threshold,
		"pubKeys":   pubKeys,
	})

	return &eUTXO{
		TxID:      txID,
		Index:     0,
		Value:     amount,
		Address:   NewScriptAddress(addrHash),
		Script:    script,
		Datum:     datum.Data,
		CreatedAt: 0,
	}, nil
}

// BuildTimelockContract creates a time-locked contract.
func BuildTimelockContract(
	pubKey []byte,
	lockSlot uint64,
	amount uint64,
) (*eUTXO, error) {
	var txID [32]byte
	txID[0] = 1

	script, _ := CreateTimelockScript(TimelockParams{
		LockSlot:  lockSlot,
		PublicKey: pubKey,
	})

	// Create script address
	scriptHash := sha256.Sum256(script)
	var addrHash [28]byte
	copy(addrHash[:], scriptHash[:28])

	datum, _ := NewDatumFromJSON(map[string]interface{}{
		"lockSlot":  lockSlot,
		"publicKey": pubKey,
	})

	return &eUTXO{
		TxID:      txID,
		Index:     0,
		Value:     amount,
		Address:   NewScriptAddress(addrHash),
		Script:    script,
		Datum:     datum.Data,
		CreatedAt: 0,
	}, nil
}

// BuildEscrowContract creates an escrow contract.
func BuildEscrowContract(
	buyerPub []byte,
	sellerPub []byte,
	timeoutSlot uint64,
	amount uint64,
) (*eUTXO, error) {
	var txID [32]byte
	txID[0] = 1

	script, _ := CreateEscrowScript(EscrowParams{
		BuyerPubKey:  buyerPub,
		SellerPubKey: sellerPub,
		TimeoutSlot:  timeoutSlot,
	})

	// Create script address
	scriptHash := sha256.Sum256(script)
	var addrHash [28]byte
	copy(addrHash[:], scriptHash[:28])

	datum, _ := NewDatumFromJSON(map[string]interface{}{
		"buyerPubKey":  buyerPub,
		"sellerPubKey": sellerPub,
		"timeoutSlot":  timeoutSlot,
	})

	return &eUTXO{
		TxID:      txID,
		Index:     0,
		Value:     amount,
		Address:   NewScriptAddress(addrHash),
		Script:    script,
		Datum:     datum.Data,
		CreatedAt: 0,
	}, nil
}
