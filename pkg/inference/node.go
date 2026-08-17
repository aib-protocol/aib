// Package inference provides AI inference node services with lightning network payments.
package inference

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"
)

// InferenceNode represents an AI inference service node
type InferenceNode struct {
	PubKey     [32]byte
	Level      uint8   // Interface level 1/2/3
	Stake      uint64  // Staking amount
	Reputation float64 // Reputation score 0-10
	Price      uint64  // Single inference price (satoshi)
	IsOnline   bool
	Stats      NodeStats

	// Internal fields
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	privateKey ed25519.PrivateKey
	requests   chan *InferenceRequest
}

// NodeStats contains node statistics
type NodeStats struct {
	TotalInferences uint64 // Total inference count
	SuccessfulCount uint64 // Successful count
	FailedCount     uint64 // Failed count
	AvgLatencyMs    uint64 // Average latency
	TotalEarnings   uint64 // Total earnings

	// Internal tracking
	latencySum uint64
}

// InferenceRequest represents an inference request
type InferenceRequest struct {
	RequestID   [32]byte
	UserPubKey  [32]byte
	Model       string
	Prompt      string
	MaxTokens   uint32
	Temperature float32
	Timestamp   int64
}

// InferenceResponse represents an inference response
type InferenceResponse struct {
	RequestID [32]byte
	Output    string
	LatencyMs uint64
	Fee       uint64
	Signature []byte // Node signature (proof of execution)
}

// NodeConfig holds configuration for inference node
type NodeConfig struct {
	Level       uint8
	Stake       uint64
	PrivateKey  ed25519.PrivateKey
	Models      []string
	APIEndpoint string
	MaxRequests int
}

// ErrNodeAlreadyRegistered is returned when node is already registered
var ErrNodeAlreadyRegistered = fmt.Errorf("node already registered")

// ErrNodeNotRegistered is returned when node is not registered
var ErrNodeNotRegistered = fmt.Errorf("node not registered")

// ErrInvalidRequest is returned when request is invalid
var ErrInvalidRequest = fmt.Errorf("invalid inference request")

// ErrInferenceFailed is returned when inference execution fails
var ErrInferenceFailed = fmt.Errorf("inference execution failed")

// NewInferenceNode creates a new inference node
func NewInferenceNode(pubKey [32]byte, level uint8, stake uint64) *InferenceNode {
	return &InferenceNode{
		PubKey:     pubKey,
		Level:      level,
		Stake:      stake,
		Reputation: 5.0, // Default reputation
		Price:      0,   // Will be calculated
		IsOnline:   false,
		Stats: NodeStats{
			TotalInferences: 0,
			SuccessfulCount: 0,
			FailedCount:     0,
			AvgLatencyMs:    0,
			TotalEarnings:   0,
		},
		requests: make(chan *InferenceRequest, 100),
	}
}

// NewInferenceNodeWithConfig creates a new inference node with full configuration
func NewInferenceNodeWithConfig(config *NodeConfig) (*InferenceNode, error) {
	if config.Level < 1 || config.Level > 3 {
		return nil, fmt.Errorf("invalid level: %d, must be 1, 2, or 3", config.Level)
	}

	var pubKey [32]byte
	if config.PrivateKey != nil {
		copy(pubKey[:], config.PrivateKey.Public().(ed25519.PublicKey))
	} else {
		// Generate random public key for testing
		rand.Read(pubKey[:])
	}

	node := &InferenceNode{
		PubKey:     pubKey,
		Level:      config.Level,
		Stake:      config.Stake,
		Reputation: 5.0,
		Price:      0,
		IsOnline:   false,
		Stats: NodeStats{
			TotalInferences: 0,
			SuccessfulCount: 0,
			FailedCount:     0,
			AvgLatencyMs:    0,
			TotalEarnings:   0,
		},
		privateKey: config.PrivateKey,
		requests:   make(chan *InferenceRequest, config.MaxRequests),
	}

	// Calculate initial price
	node.Price = node.CalculatePrice()

	return node, nil
}

// Register registers the node to the network
func (n *InferenceNode) Register() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.IsOnline {
		return ErrNodeAlreadyRegistered
	}

	// In production, this would:
	// 1. Verify stake on blockchain
	// 2. Register with discovery service
	// 3. Announce to network

	// Validate level and stake
	switch n.Level {
	case 1:
		if n.Stake < 1000000 {
			return fmt.Errorf("level 1 requires at least 1000000 satoshi stake")
		}
	case 2:
		if n.Stake < 10000000 {
			return fmt.Errorf("level 2 requires at least 10000000 satoshi stake")
		}
	case 3:
		if n.Stake < 100000000 {
			return fmt.Errorf("level 3 requires at least 100000000 satoshi stake")
		}
	default:
		return fmt.Errorf("invalid level: %d", n.Level)
	}

	// Mark as registered
	n.IsOnline = true

	return nil
}

// Unregister removes the node from the network
func (n *InferenceNode) Unregister() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.IsOnline {
		return ErrNodeNotRegistered
	}

	// In production, this would:
	// 1. Unregister from discovery service
	// 2. Withdraw stake if eligible
	// 3. Settle pending payments

	n.IsOnline = false
	return nil
}

// Start starts the inference service
func (n *InferenceNode) Start(ctx context.Context) error {
	n.mu.Lock()
	if !n.IsOnline {
		n.mu.Unlock()
		return ErrNodeNotRegistered
	}

	n.ctx, n.cancel = context.WithCancel(ctx)
	n.mu.Unlock()

	// Start request processor
	go n.processRequests()

	// Start heartbeat
	go n.heartbeat()

	return nil
}

// Stop stops the inference service
func (n *InferenceNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.cancel != nil {
		n.cancel()
	}

	return nil
}

// processRequests processes incoming inference requests
func (n *InferenceNode) processRequests() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case req := <-n.requests:
			if req == nil {
				continue
			}
			// Handle the request asynchronously
			go n.handleRequest(req)
		}
	}
}

// heartbeat sends periodic heartbeats to the network
func (n *InferenceNode) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			// In production, send heartbeat to network
			n.updatePrice()
		}
	}
}

// updatePrice updates the price based on current conditions
func (n *InferenceNode) updatePrice() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Price = n.CalculatePrice()
}

// CalculatePrice calculates the price based on level and reputation
func (n *InferenceNode) CalculatePrice() uint64 {
	n.mu.RLock()
	level := n.Level
	reputation := n.Reputation
	n.mu.RUnlock()

	prices := map[uint8]uint64{
		1: 10000,   // 10k satoshi for level 1
		2: 100000,  // 100k satoshi for level 2
		3: 1000000, // 1M satoshi for level 3
	}

	basePrice := prices[level]
	if basePrice == 0 {
		basePrice = prices[1]
	}

	// Reputation multiplier: 1.0 + (reputation - 5.0) / 20.0
	// This gives a range of 0.75x to 1.25x multiplier for reputation 0-10
	reputationMultiplier := 1.0 + (reputation-5.0)/20.0

	return uint64(float64(basePrice) * reputationMultiplier)
}

// HandleRequest handles an inference request
func (n *InferenceNode) HandleRequest(req *InferenceRequest) (*InferenceResponse, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}

	n.mu.RLock()
	if !n.IsOnline {
		n.mu.RUnlock()
		return nil, ErrNodeNotRegistered
	}
	n.mu.RUnlock()

	// Validate request
	if err := n.validateRequest(req); err != nil {
		return nil, err
	}

	return n.handleRequest(req)
}

// handleRequest internal request handler
func (n *InferenceNode) handleRequest(req *InferenceRequest) (*InferenceResponse, error) {
	startTime := time.Now()

	// Execute inference
	output, err := n.executeInference(req.Prompt)
	if err != nil {
		n.RecordFailure()
		return nil, err
	}

	latencyMs := uint64(time.Since(startTime).Milliseconds())

	// Record success
	n.RecordSuccess(latencyMs)

	// Calculate fee
	fee := n.CalculatePrice()

	// Create response
	resp := &InferenceResponse{
		RequestID: req.RequestID,
		Output:    output,
		LatencyMs: latencyMs,
		Fee:       fee,
	}

	// Sign the response
	if n.privateKey != nil {
		sig, err := n.signResponse(resp)
		if err == nil {
			resp.Signature = sig
		}
	}

	return resp, nil
}

// validateRequest validates an inference request
func (n *InferenceNode) validateRequest(req *InferenceRequest) error {
	// Check request ID
	if req.RequestID == [32]byte{} {
		return fmt.Errorf("invalid request ID")
	}

	// Check prompt
	if len(req.Prompt) == 0 {
		return fmt.Errorf("empty prompt")
	}

	// Check prompt length limit
	if len(req.Prompt) > 10000 {
		return fmt.Errorf("prompt too long, max 10000 characters")
	}

	// Validate temperature
	if req.Temperature < 0 || req.Temperature > 2.0 {
		return fmt.Errorf("invalid temperature, must be between 0 and 2")
	}

	// Validate max tokens
	if req.MaxTokens > 10000 {
		return fmt.Errorf("max tokens too high, max 10000")
	}

	return nil
}

// executeInference executes the AI inference (simulates external API call)
func (n *InferenceNode) executeInference(prompt string) (string, error) {
	// In production, this would:
	// 1. Route to appropriate model
	// 2. Call external AI API (OpenAI, Anthropic, etc.)
	// 3. Process and return result

	// Simulate processing
	time.Sleep(time.Millisecond * time.Duration(50+n.Level*50))

	// Simulate occasional failures (1% failure rate for testing)
	if n.Level == 1 {
		// Level 1 has higher failure rate
		if randInt(100) < 5 {
			return "", ErrInferenceFailed
		}
	}

	// Generate mock response based on prompt
	response := n.generateMockResponse(prompt)

	return response, nil
}

// generateMockResponse generates a mock response for testing
func (n *InferenceNode) generateMockResponse(prompt string) string {
	// Simple mock response generation
	promptHash := sha256.Sum256([]byte(prompt))
	hashStr := hex.EncodeToString(promptHash[:8])

	return fmt.Sprintf("Inference result for prompt '%s...' [hash: %s] - processed by level %d node",
		truncateString(prompt, 20),
		hashStr,
		n.Level)
}

// signResponse signs the inference response
func (n *InferenceNode) signResponse(resp *InferenceResponse) ([]byte, error) {
	if n.privateKey == nil {
		return nil, fmt.Errorf("no private key available")
	}

	// Create signing data
	data := fmt.Sprintf("%x|%s|%d|%d",
		resp.RequestID,
		resp.Output,
		resp.LatencyMs,
		resp.Fee,
	)

	return ed25519.Sign(n.privateKey, []byte(data)), nil
}

// RecordSuccess records a successful inference
func (n *InferenceNode) RecordSuccess(latencyMs uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Stats.TotalInferences++
	n.Stats.SuccessfulCount++
	n.Stats.TotalEarnings += n.Price
	n.Stats.latencySum += latencyMs

	// Update average latency
	if n.Stats.TotalInferences > 0 {
		n.Stats.AvgLatencyMs = n.Stats.latencySum / n.Stats.TotalInferences
	}

	// Improve reputation slightly on success
	if n.Reputation < 10.0 {
		n.Reputation = math.Min(10.0, n.Reputation+0.001)
	}
}

// RecordFailure records a failed inference
func (n *InferenceNode) RecordFailure() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Stats.TotalInferences++
	n.Stats.FailedCount++

	// Penalize reputation on failure
	if n.Reputation > 0.0 {
		n.Reputation = math.Max(0.0, n.Reputation-0.01)
	}
}

// CalculateSuccessRate calculates the success rate
func (n *InferenceNode) CalculateSuccessRate() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.Stats.TotalInferences == 0 {
		return 1.0 // Default to 100% if no inferences
	}

	return float64(n.Stats.SuccessfulCount) / float64(n.Stats.TotalInferences)
}

// GetStats returns a copy of node statistics
func (n *InferenceNode) GetStats() NodeStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return NodeStats{
		TotalInferences: n.Stats.TotalInferences,
		SuccessfulCount: n.Stats.SuccessfulCount,
		FailedCount:     n.Stats.FailedCount,
		AvgLatencyMs:    n.Stats.AvgLatencyMs,
		TotalEarnings:   n.Stats.TotalEarnings,
	}
}

// UpdateReputation updates the node's reputation
func (n *InferenceNode) UpdateReputation(newReputation float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if newReputation < 0 {
		newReputation = 0
	}
	if newReputation > 10 {
		newReputation = 10
	}

	// Use weighted average for reputation updates
	// Old reputation has 90% weight, new has 10%
	n.Reputation = n.Reputation*0.9 + newReputation*0.1
}

// GetInfo returns node information
func (n *InferenceNode) GetInfo() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return map[string]interface{}{
		"pub_key":    hex.EncodeToString(n.PubKey[:]),
		"level":      n.Level,
		"stake":      n.Stake,
		"reputation": n.Reputation,
		"price":      n.Price,
		"is_online":  n.IsOnline,
		"stats":      n.Stats,
	}
}

// Helper functions

func randInt(max int) int {
	b := make([]byte, 4)
	rand.Read(b)
	return int(b[0]) % max
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
