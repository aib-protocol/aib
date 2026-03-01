// Package relayer provides cross-chain relayer functionality.
// This file implements the RelayerNetwork for relayer registration,
// discovery, task assignment, and dispute resolution.
package relayer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ============================================================================
// Relayer Network Interface
// ============================================================================

// RelayerNetwork defines the interface for the relayer network.
type RelayerNetwork interface {
	// RegisterRelayer registers a new relayer on the network.
	RegisterRelayer(relayer *RelayerNode) error

	// UnregisterRelayer removes a relayer from the network.
	UnregisterRelayer(relayerID string) error

	// DiscoverRelayers discovers relayers that support a specific chain.
	DiscoverRelayers(chain ChainType) []*RelayerNode

	// AssignTask assigns a swap task to the best available relayer.
	AssignTask(req *SwapRequest) (*RelayerNode, error)

	// ReportDispute reports a dispute against a relayer.
	ReportDispute(dispute *Dispute) error

	// ResolveDispute resolves a pending dispute.
	ResolveDispute(disputeID string, resolution *DisputeResolution) error

	// GetRelayer retrieves a relayer by ID.
	GetRelayer(relayerID string) (*RelayerNode, error)

	// GetNetworkStats returns the network statistics.
	GetNetworkStats() *NetworkStats
}

// ============================================================================
// Network Statistics
// ============================================================================

// NetworkStats contains network-wide statistics.
type NetworkStats struct {
	TotalRelayers    int       `json:"total_relayers"`
	ActiveRelayers   int       `json:"active_relayers"`
	InactiveRelayers int       `json:"inactive_relayers"`
	SlashedRelayers  int       `json:"slashed_relayers"`
	TotalStake       string    `json:"total_stake"`
	TotalTXs         uint64    `json:"total_txs"`
	PendingDisputes  int       `json:"pending_disputes"`
	SupportedChains  []ChainType `json:"supported_chains"`
}

// ============================================================================
// Relayer Network Implementation
// ============================================================================

// Network implements the RelayerNetwork interface.
type Network struct {
	relayers      map[string]*RelayerNode   // relayerID -> RelayerNode
	chainIndex    map[ChainType][]string    // chain -> list of relayer IDs
	disputes      map[string]*Dispute       // disputeID -> Dispute
	resolutions   map[string]*DisputeResolution // disputeID -> Resolution
	totalStake    *big.Int                  // Total stake in the network
	mu            sync.RWMutex
}

// NewNetwork creates a new relayer network.
func NewNetwork() *Network {
	return &Network{
		relayers:    make(map[string]*RelayerNode),
		chainIndex:  make(map[ChainType][]string),
		disputes:    make(map[string]*Dispute),
		resolutions: make(map[string]*DisputeResolution),
		totalStake:  big.NewInt(0),
	}
}

// RegisterRelayer registers a new relayer on the network.
func (n *Network) RegisterRelayer(relayer *RelayerNode) error {
	if relayer == nil {
		return fmt.Errorf("relayer is nil")
	}

	// Validate relayer configuration
	if err := ValidateRelayer(relayer); err != nil {
		return fmt.Errorf("invalid relayer: %w", err)
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if relayer already exists
	if _, exists := n.relayers[relayer.id]; exists {
		return fmt.Errorf("relayer already registered: %s", relayer.id)
	}

	// Validate minimum stake
	minStake := big.NewInt(DefaultRelayerStake)
	if relayer.stake.Cmp(minStake) < 0 {
		return fmt.Errorf("stake below minimum requirement: %s < %s",
			relayer.stake.String(), minStake.String())
	}

	// Register relayer
	n.relayers[relayer.id] = relayer

	// Update chain index
	for _, chain := range relayer.supportedChains {
		n.chainIndex[chain] = append(n.chainIndex[chain], relayer.id)
	}

	// Update total stake
	n.totalStake.Add(n.totalStake, relayer.stake)

	return nil
}

// UnregisterRelayer removes a relayer from the network.
func (n *Network) UnregisterRelayer(relayerID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	relayer, exists := n.relayers[relayerID]
	if !exists {
		return fmt.Errorf("relayer not found: %s", relayerID)
	}

	// Check for pending transactions
	pendingTXs := 0
	for _, tx := range relayer.transactions {
		if tx.Status != TxStatusCompleted && tx.Status != TxStatusFailed {
			pendingTXs++
		}
	}
	if pendingTXs > 0 {
		return fmt.Errorf("relayer has %d pending transactions", pendingTXs)
	}

	// Remove from chain index
	for _, chain := range relayer.supportedChains {
		ids := n.chainIndex[chain]
		for i, id := range ids {
			if id == relayerID {
				n.chainIndex[chain] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}

	// Update total stake
	n.totalStake.Sub(n.totalStake, relayer.stake)

	// Remove relayer
	delete(n.relayers, relayerID)

	return nil
}

// DiscoverRelayers discovers relayers that support a specific chain.
func (n *Network) DiscoverRelayers(chain ChainType) []*RelayerNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ids, ok := n.chainIndex[chain]
	if !ok {
		return nil
	}

	relayers := make([]*RelayerNode, 0)
	for _, id := range ids {
		if relayer, exists := n.relayers[id]; exists {
			if relayer.status == StatusActive {
				relayers = append(relayers, relayer)
			}
		}
	}

	return relayers
}

// AssignTask assigns a swap task to the best available relayer.
func (n *Network) AssignTask(req *SwapRequest) (*RelayerNode, error) {
	if req == nil {
		return nil, fmt.Errorf("swap request is nil")
	}

	if err := ValidateSwapRequest(req); err != nil {
		return nil, fmt.Errorf("invalid swap request: %w", err)
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	// Find relayers that support both chains
	var candidates []*RelayerNode
	for _, relayer := range n.relayers {
		if CanRelay(relayer, req.SourceChain, req.DestChain) {
			candidates = append(candidates, relayer)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available relayers for %s -> %s",
			req.SourceChain, req.DestChain)
	}

	// Select the best relayer based on reputation and fee rate
	best := SelectBestRelayers(candidates, req.SourceChain, req.DestChain, 1)
	if len(best) == 0 {
		return nil, fmt.Errorf("no suitable relayers found")
	}

	return best[0], nil
}

// ReportDispute reports a dispute against a relayer.
func (n *Network) ReportDispute(dispute *Dispute) error {
	if dispute == nil {
		return fmt.Errorf("dispute is nil")
	}

	if dispute.TxHash == "" {
		return fmt.Errorf("transaction hash is required")
	}

	if dispute.Reporter == "" {
		return fmt.Errorf("reporter is required")
	}

	if dispute.Reason == "" {
		return fmt.Errorf("reason is required")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Generate dispute ID
	if dispute.ID == "" {
		data := fmt.Sprintf("%s-%s-%d", dispute.TxHash, dispute.Reporter, time.Now().UnixNano())
		hash := sha256.Sum256([]byte(data))
		dispute.ID = hex.EncodeToString(hash[:16])
	}

	dispute.Status = "pending"
	dispute.CreatedAt = time.Now()

	// Verify the transaction exists
	found := false
	for _, relayer := range n.relayers {
		for _, tx := range relayer.transactions {
			if tx.ID == dispute.TxHash {
				// Mark transaction as disputed
				tx.SetStatus(TxStatusDisputed)
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("transaction not found: %s", dispute.TxHash)
	}

	n.disputes[dispute.ID] = dispute

	return nil
}

// ResolveDispute resolves a pending dispute.
func (n *Network) ResolveDispute(disputeID string, resolution *DisputeResolution) error {
	if disputeID == "" {
		return fmt.Errorf("dispute ID is required")
	}

	if resolution == nil {
		return fmt.Errorf("resolution is required")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	dispute, exists := n.disputes[disputeID]
	if !exists {
		return fmt.Errorf("dispute not found: %s", disputeID)
	}

	if dispute.Status != "pending" {
		return fmt.Errorf("dispute already resolved: %s", dispute.Status)
	}

	// Apply penalty if any
	if resolution.Penalty != nil && resolution.Penalty.Sign() > 0 {
		loserRelayer, exists := n.relayers[resolution.Loser]
		if exists {
			// Slash the loser's stake
			slashAmount := new(big.Int).Set(resolution.Penalty)
			if loserRelayer.stake.Cmp(slashAmount) < 0 {
				slashAmount = new(big.Int).Set(loserRelayer.stake)
			}

			loserRelayer.mu.Lock()
			loserRelayer.stake.Sub(loserRelayer.stake, slashAmount)
			loserRelayer.reputation -= 10 // Deduct reputation
			if loserRelayer.reputation < 0 {
				loserRelayer.reputation = 0
			}
			// If reputation drops below minimum, slash the relayer
			if loserRelayer.reputation < MinRelayerReputation {
				loserRelayer.status = StatusSlashed
			}
			loserRelayer.mu.Unlock()

			// Update total stake
			n.totalStake.Sub(n.totalStake, slashAmount)
		}
	}

	// Update dispute status
	now := time.Now()
	dispute.Status = "resolved"
	dispute.ResolvedAt = &now

	// Store resolution
	n.resolutions[disputeID] = resolution

	return nil
}

// GetRelayer retrieves a relayer by ID.
func (n *Network) GetRelayer(relayerID string) (*RelayerNode, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	relayer, exists := n.relayers[relayerID]
	if !exists {
		return nil, fmt.Errorf("relayer not found: %s", relayerID)
	}

	return relayer, nil
}

// GetNetworkStats returns the network statistics.
func (n *Network) GetNetworkStats() *NetworkStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	active := 0
	inactive := 0
	slashed := 0
	totalTXs := uint64(0)

	for _, relayer := range n.relayers {
		switch relayer.status {
		case StatusActive:
			active++
		case StatusInactive:
			inactive++
		case StatusSlashed:
			slashed++
		}
		totalTXs += relayer.totalTXs
	}

	// Collect unique supported chains
	chainSet := make(map[ChainType]bool)
	for chain := range n.chainIndex {
		chainSet[chain] = true
	}
	chains := make([]ChainType, 0, len(chainSet))
	for chain := range chainSet {
		chains = append(chains, chain)
	}

	pendingDisputes := 0
	for _, dispute := range n.disputes {
		if dispute.Status == "pending" {
			pendingDisputes++
		}
	}

	return &NetworkStats{
		TotalRelayers:    len(n.relayers),
		ActiveRelayers:   active,
		InactiveRelayers: inactive,
		SlashedRelayers:  slashed,
		TotalStake:       n.totalStake.String(),
		TotalTXs:         totalTXs,
		PendingDisputes:  pendingDisputes,
		SupportedChains:  chains,
	}
}

// ============================================================================
// Network Management Functions
// ============================================================================

// SlashRelayer slashes a relayer by reducing its stake.
func (n *Network) SlashRelayer(relayerID string, ratio float64) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	relayer, exists := n.relayers[relayerID]
	if !exists {
		return fmt.Errorf("relayer not found: %s", relayerID)
	}

	// Calculate slash amount
	slashFloat := new(big.Float).Mul(
		new(big.Float).SetInt(relayer.stake),
		big.NewFloat(ratio),
	)
	slashAmount, _ := slashFloat.Int(nil)

	// Apply slash
	relayer.mu.Lock()
	relayer.stake.Sub(relayer.stake, slashAmount)
	relayer.status = StatusSlashed
	relayer.reputation = 0
	relayer.mu.Unlock()

	// Update total stake
	n.totalStake.Sub(n.totalStake, slashAmount)

	return nil
}

// ReactivateRelayer reactivates a slashed or inactive relayer.
func (n *Network) ReactivateRelayer(relayerID string, additionalStake *big.Int) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	relayer, exists := n.relayers[relayerID]
	if !exists {
		return fmt.Errorf("relayer not found: %s", relayerID)
	}

	// Add additional stake
	relayer.mu.Lock()
	defer relayer.mu.Unlock()

	if additionalStake != nil && additionalStake.Sign() > 0 {
		relayer.stake.Add(relayer.stake, additionalStake)
		n.totalStake.Add(n.totalStake, additionalStake)
	}

	// Check if stake meets minimum requirement
	minStake := big.NewInt(DefaultRelayerStake)
	if relayer.stake.Cmp(minStake) < 0 {
		return fmt.Errorf("insufficient stake after reactivation: %s < %s",
			relayer.stake.String(), minStake.String())
	}

	relayer.status = StatusActive
	relayer.reputation = MinRelayerReputation // Reset reputation to minimum
	relayer.lastActiveAt = time.Now()

	return nil
}

// GetDispute retrieves a dispute by ID.
func (n *Network) GetDispute(disputeID string) (*Dispute, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	dispute, exists := n.disputes[disputeID]
	if !exists {
		return nil, fmt.Errorf("dispute not found: %s", disputeID)
	}

	return dispute, nil
}

// ListDisputes returns all disputes with the given status.
func (n *Network) ListDisputes(status string) []*Dispute {
	n.mu.RLock()
	defer n.mu.RUnlock()

	disputes := make([]*Dispute, 0)
	for _, dispute := range n.disputes {
		if status == "" || dispute.Status == status {
			disputes = append(disputes, dispute)
		}
	}

	return disputes
}

// GetResolution retrieves a dispute resolution by dispute ID.
func (n *Network) GetResolution(disputeID string) (*DisputeResolution, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	resolution, exists := n.resolutions[disputeID]
	if !exists {
		return nil, fmt.Errorf("resolution not found for dispute: %s", disputeID)
	}

	return resolution, nil
}

// ============================================================================
// Network Discovery Functions
// ============================================================================

// FindRelayersForSwap finds all relayers capable of performing a specific swap.
func (n *Network) FindRelayersForSwap(sourceChain, destChain ChainType) []*RelayerNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var result []*RelayerNode
	for _, relayer := range n.relayers {
		if CanRelay(relayer, sourceChain, destChain) {
			result = append(result, relayer)
		}
	}

	return result
}

// GetRelayersByChain returns all relayers that support a specific chain.
func (n *Network) GetRelayersByChain(chain ChainType) []*RelayerNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ids, ok := n.chainIndex[chain]
	if !ok {
		return nil
	}

	result := make([]*RelayerNode, 0, len(ids))
	for _, id := range ids {
		if relayer, exists := n.relayers[id]; exists {
			result = append(result, relayer)
		}
	}

	return result
}

// GetActiveRelayers returns all active relayers.
func (n *Network) GetActiveRelayers() []*RelayerNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]*RelayerNode, 0)
	for _, relayer := range n.relayers {
		if relayer.status == StatusActive {
			result = append(result, relayer)
		}
	}

	return result
}

// ============================================================================
// Network Event Types
// ============================================================================

// NetworkEvent represents an event in the relayer network.
type NetworkEvent struct {
	Type      string    `json:"type"`
	RelayerID string    `json:"relayer_id"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// NewNetworkEvent creates a new network event.
func NewNetworkEvent(eventType, relayerID, details string) *NetworkEvent {
	return &NetworkEvent{
		Type:      eventType,
		RelayerID: relayerID,
		Details:   details,
		Timestamp: time.Now(),
	}
}
