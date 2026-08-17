// Package interfaces defines the contracts between all development teams.
// This file is the single source of truth for team-to-team communication.
// DO NOT MODIFY without consent from all team leads.

package interfaces

import (
	"context"
	"crypto/ed25519"
	"time"
)

// ============================================================================
// Team Alpha → Team Bravo (UTXO → AAL)
// ============================================================================

// UTXOProvider provides UTXO operations for Account Abstraction Layer.
type UTXOProvider interface {
	// GetUTXO retrieves a specific UTXO by transaction hash and output index.
	GetUTXO(txHash [32]byte, index uint32) (*UTXO, error)

	// CreateTXOutput creates a new transaction output.
	CreateTXOutput(addr Address, value uint64, script []byte) *TXOutput

	// SpendUTXO marks a UTXO as spent with the provided signature.
	SpendUTXO(input TXInput, sig []byte) error

	// GetBalance returns the total spendable balance for an address.
	GetBalance(addr Address) (uint64, error)
}

// UTXO represents an unspent transaction output.
type UTXO struct {
	TxHash  [32]byte
	Index   uint32
	Value   uint64
	Script  []byte
	Address Address
}

// TXOutput represents a transaction output.
type TXOutput struct {
	Value   uint64
	Script  []byte
	Address Address
}

// TXInput represents a transaction input.
type TXInput struct {
	TxHash    [32]byte
	Index     uint32
	Signature []byte
	PublicKey []byte
}

// Address represents a blockchain address.
type Address [32]byte

// ============================================================================
// Team Alpha → Team Charlie (UTXO → Channel)
// ============================================================================

// MultiSigLocker provides multi-signature locking for state channels.
type MultiSigLocker interface {
	// CreateMultiSigOutput creates a 2-of-2 multi-sig output.
	CreateMultiSigOutput(partyA, partyB Address, amount uint64) (*UTXO, error)

	// SpendMultiSig spends a multi-sig output with both signatures.
	SpendMultiSig(utxo *UTXO, sigA, sigB []byte, outputs []TXOutput) error

	// VerifyMultiSig verifies both signatures for a multi-sig output.
	VerifyMultiSig(utxo *UTXO, sigA, sigB []byte) bool
}

// ============================================================================
// Team Charlie → Team Echo (Channel → Agentic)
// ============================================================================

// ChannelManager provides state channel management for AI services.
type ChannelManager interface {
	// OpenChannel opens a new state channel with initial deposits.
	OpenChannel(ctx context.Context, partyA, partyB Address, depositA, depositB uint64) (*Channel, error)

	// UpdateState updates the channel state with a new signed state.
	UpdateState(ch *Channel, newState SignedState) (*SignedState, error)

	// CloseChannel closes the channel with the final state.
	CloseChannel(ctx context.Context, ch *Channel, finalState SignedState) error

	// Dispute initiates a dispute with evidence.
	Dispute(ctx context.Context, ch *Channel, evidence SignedState) error

	// GetChannelState returns the current channel state.
	GetChannelState(channelID [32]byte) (*Channel, error)
}

// Channel represents a state channel.
type Channel struct {
	ID         [32]byte
	PartyA     Address
	PartyB     Address
	BalanceA   uint64
	BalanceB   uint64
	Sequence   uint64
	StateHash  [32]byte
	CreatedAt  time.Time
	DisputeEnd *time.Time // nil if no dispute
}

// SignedState represents a signed channel state.
type SignedState struct {
	ChannelID [32]byte
	Sequence  uint64
	BalanceA  uint64
	BalanceB  uint64
	SigA      []byte
	SigB      []byte
	Timestamp time.Time
}

// ============================================================================
// Team Delta → Team Alpha (ZK → Block)
// ============================================================================

// ZKBatchVerifier provides ZK proof verification for rollup batches.
type ZKBatchVerifier interface {
	// VerifyBatch verifies a ZK proof for a batch of state transitions.
	VerifyBatch(batch *ZKProofBatch) (bool, error)

	// GetCurrentStateRoot returns the current verified state root.
	GetCurrentStateRoot() [32]byte

	// UpdateStateRoot updates the state root after batch verification.
	UpdateStateRoot(newRoot [32]byte) error
}

// ZKProofBatch represents a batch of ZK-proven state transitions.
type ZKProofBatch struct {
	PrevStateRoot [32]byte
	NewStateRoot  [32]byte
	Proof         []byte
	PublicInputs  []byte
	TxCount       uint64
	ChannelOps    []ChannelOp
	Timestamp     time.Time
}

// ChannelOp represents a channel operation in a batch.
type ChannelOp struct {
	Type      uint8 // 0=Open, 1=Update, 2=Close, 3=Dispute
	ChannelID [32]byte
	Sequence  uint64
	FinalA    uint64
	FinalB    uint64
}

// ============================================================================
// Team Echo → All Teams (Agentic Service)
// ============================================================================

// AgenticService provides LLM service with standard API compatibility.
type AgenticService interface {
	// ChatCompletion provides OpenAI-compatible chat completion.
	ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)

	// Messages provides Anthropic-compatible message completion.
	Messages(ctx context.Context, req MessagesRequest) (*MessagesResponse, error)

	// GetAvailableNodes returns available AI service nodes.
	GetAvailableNodes(ctx context.Context) ([]AINode, error)

	// OpenServiceChannel opens a payment channel with a service provider.
	OpenServiceChannel(ctx context.Context, nodeID NodeID, maxSpend uint64) (*Channel, error)
}

// ChatCompletionRequest represents an OpenAI-compatible request.
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// MessagesRequest represents an Anthropic-compatible request.
type MessagesRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
}

type MessagesResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

type AINode struct {
	ID         NodeID
	Address    Address
	Stake      uint64
	Models     []string // supported models
	Reputation float64
}

type NodeID [32]byte

// ============================================================================
// Common Types (used across all interfaces)
// ============================================================================

// PublicKey is a 32-byte Ed25519 public key.
type PublicKey = ed25519.PublicKey

// PrivateKey is a 64-byte Ed25519 private key.
type PrivateKey = ed25519.PrivateKey
