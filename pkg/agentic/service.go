// Package agentic provides AI service layer with standard API compatibility.
package agentic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// AgenticServiceImpl implements the AgenticService interface.
type AgenticServiceImpl struct {
	mu            sync.RWMutex
	config        *Config
	channelMgr    interfaces.ChannelManager
	nodeRegistry  *NodeRegistry
	stakingMgr    *StakingManager
	providers     map[string]*ServiceProvider
	requests      map[string]*ServiceRequest
	started       bool
}

// NewAgenticService creates a new agentic service implementation.
func NewAgenticService(channelMgr interfaces.ChannelManager, cfg *Config) (*AgenticServiceImpl, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	stakingMgr, err := NewStakingManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking manager: %w", err)
	}

	nodeRegistry, err := NewNodeRegistry(stakingMgr, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create node registry: %w", err)
	}

	return &AgenticServiceImpl{
		config:       cfg,
		channelMgr:   channelMgr,
		nodeRegistry: nodeRegistry,
		stakingMgr:   stakingMgr,
		providers:    make(map[string]*ServiceProvider),
		requests:     make(map[string]*ServiceRequest),
	}, nil
}

// Start starts the agentic service.
func (s *AgenticServiceImpl) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("service already started")
	}

	s.started = true
	return nil
}

// Stop stops the agentic service.
func (s *AgenticServiceImpl) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return fmt.Errorf("service not started")
	}

	s.started = false
	return nil
}

// ChatCompletion provides OpenAI-compatible chat completion.
func (s *AgenticServiceImpl) ChatCompletion(ctx context.Context, req interfaces.ChatCompletionRequest) (*interfaces.ChatCompletionResponse, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}

	// Find a suitable provider for this model
	_, err := s.findProviderForModel(req.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to find provider for model %s: %w", req.Model, err)
	}

	// For now, return a mock response
	// In a real implementation, this would make an actual API call to the provider

	response := &interfaces.ChatCompletionResponse{
		ID:      generateRequestID(),
		Model:   req.Model,
		Choices: []interfaces.Choice{
			{
				Message: interfaces.Message{
					Role:    "assistant",
					Content: "This is a mock response from the agentic service.",
				},
				FinishReason: "stop",
			},
		},
		Usage: interfaces.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	return response, nil
}

// Messages provides Anthropic-compatible message completion.
func (s *AgenticServiceImpl) Messages(ctx context.Context, req interfaces.MessagesRequest) (*interfaces.MessagesResponse, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}

	if req.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required")
	}

	// Find a suitable provider for this model
	_, err := s.findProviderForModel(req.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to find provider for model %s: %w", req.Model, err)
	}

	// For now, return a mock response
	response := &interfaces.MessagesResponse{
		ID:      generateRequestID(),
		Model:   req.Model,
		Content: "This is a mock response from the agentic service.",
		Usage: interfaces.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	return response, nil
}

// GetAvailableNodes returns available AI service nodes.
func (s *AgenticServiceImpl) GetAvailableNodes(ctx context.Context) ([]interfaces.AINode, error) {
	nodes, err := s.nodeRegistry.GetAllNodes()
	if err != nil {
		return nil, err
	}

	result := make([]interfaces.AINode, 0, len(nodes))
	for _, node := range nodes {
		if node.Status == NodeStatusActive {
			result = append(result, interfaces.AINode{
				ID:         node.ID,
				Address:    node.Address,
				Stake:      node.Stake,
				Models:     node.Models,
				Reputation: node.Reputation,
			})
		}
	}

	return result, nil
}

// OpenServiceChannel opens a payment channel with a service provider.
func (s *AgenticServiceImpl) OpenServiceChannel(ctx context.Context, nodeID interfaces.NodeID, maxSpend uint64) (*interfaces.Channel, error) {
	if s.channelMgr == nil {
		return nil, fmt.Errorf("channel manager not configured")
	}

	// Verify the node exists and is active
	node, err := s.nodeRegistry.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	if node.Status != NodeStatusActive {
		return nil, fmt.Errorf("node is not active: %s", node.Status)
	}

	// Open channel with node
	return s.channelMgr.OpenChannel(ctx, s.config.Address, node.Address, maxSpend, 0)
}

// RegisterProvider registers a new service provider.
func (s *AgenticServiceImpl) RegisterProvider(provider *ServiceProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	providerKey := string(provider.NodeInfo.ID[:])
	s.providers[providerKey] = provider

	return nil
}

// UnregisterProvider unregisters a service provider.
func (s *AgenticServiceImpl) UnregisterProvider(nodeID interfaces.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	providerKey := string(nodeID[:])
	delete(s.providers, providerKey)

	return nil
}

// GetProvider returns a provider by node ID.
func (s *AgenticServiceImpl) GetProvider(nodeID interfaces.NodeID) (*ServiceProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providerKey := string(nodeID[:])
	provider, ok := s.providers[providerKey]
	if !ok {
		return nil, fmt.Errorf("provider not found: %x", nodeID)
	}

	return provider, nil
}

// findProviderForModel finds a provider that supports the given model.
func (s *AgenticServiceImpl) findProviderForModel(model string) (*ServiceProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var bestProvider *ServiceProvider
	bestReputation := -1.0

	for _, provider := range s.providers {
		for _, supportedModel := range provider.NodeInfo.Models {
			if supportedModel == model {
				if provider.NodeInfo.Reputation > bestReputation {
					bestProvider = provider
					bestReputation = provider.NodeInfo.Reputation
				}
				break
			}
		}
	}

	if bestProvider == nil {
		return nil, fmt.Errorf("no provider found for model: %s", model)
	}

	return bestProvider, nil
}

// Config returns the service configuration.
func (s *AgenticServiceImpl) Config() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// NodeRegistry returns the node registry.
func (s *AgenticServiceImpl) NodeRegistry() *NodeRegistry {
	return s.nodeRegistry
}

// StakingManager returns the staking manager.
func (s *AgenticServiceImpl) StakingManager() *StakingManager {
	return s.stakingMgr
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// NodeRegistry manages node registration and discovery.
type NodeRegistry struct {
	mu        sync.RWMutex
	nodes     map[interfaces.NodeID]*NodeInfo
	byModel   map[string][]interfaces.NodeID
	stakingMgr *StakingManager
	config    *Config
}

// NewNodeRegistry creates a new node registry.
func NewNodeRegistry(stakingMgr *StakingManager, cfg *Config) (*NodeRegistry, error) {
	return &NodeRegistry{
		nodes:      make(map[interfaces.NodeID]*NodeInfo),
		byModel:    make(map[string][]interfaces.NodeID),
		stakingMgr: stakingMgr,
		config:     cfg,
	}, nil
}

// RegisterNode registers a new node.
func (nr *NodeRegistry) RegisterNode(node *NodeInfo) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	nr.nodes[node.ID] = node

	// Update model index
	for _, model := range node.Models {
		nr.byModel[model] = append(nr.byModel[model], node.ID)
	}

	return nil
}

// UnregisterNode unregisters a node.
func (nr *NodeRegistry) UnregisterNode(nodeID interfaces.NodeID) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, ok := nr.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node not found: %x", nodeID)
	}

	// Remove from model index
	for _, model := range node.Models {
		ids := nr.byModel[model]
		newIDs := make([]interfaces.NodeID, 0, len(ids))
		for _, id := range ids {
			if id != nodeID {
				newIDs = append(newIDs, id)
			}
		}
		nr.byModel[model] = newIDs
	}

	delete(nr.nodes, nodeID)
	return nil
}

// GetNode returns a node by ID.
func (nr *NodeRegistry) GetNode(nodeID interfaces.NodeID) (*NodeInfo, error) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	node, ok := nr.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not found: %x", nodeID)
	}

	return node, nil
}

// GetAllNodes returns all registered nodes.
func (nr *NodeRegistry) GetAllNodes() ([]*NodeInfo, error) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	nodes := make([]*NodeInfo, 0, len(nr.nodes))
	for _, node := range nr.nodes {
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// FindNodesByModel returns nodes that support a given model.
func (nr *NodeRegistry) FindNodesByModel(model string) ([]*NodeInfo, error) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	ids, ok := nr.byModel[model]
	if !ok {
		return []*NodeInfo{}, nil
	}

	nodes := make([]*NodeInfo, 0, len(ids))
	for _, id := range ids {
		if node, ok := nr.nodes[id]; ok {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// FindNodesByFilter returns nodes matching the given filter.
func (nr *NodeRegistry) FindNodesByFilter(filter *NodeFilter) ([]*NodeInfo, error) {
	allNodes, err := nr.GetAllNodes()
	if err != nil {
		return nil, err
	}

	result := make([]*NodeInfo, 0)
	for _, node := range allNodes {
		if filter.Matches(node) {
			result = append(result, node)
		}
	}

	return result, nil
}

// UpdateNodeStatus updates the status of a node.
func (nr *NodeRegistry) UpdateNodeStatus(nodeID interfaces.NodeID, status NodeStatus) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, ok := nr.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node not found: %x", nodeID)
	}

	node.Status = status
	node.LastSeen = time.Now()

	return nil
}

// UpdateNodeInfo updates information for a node.
func (nr *NodeRegistry) UpdateNodeInfo(node *NodeInfo) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	if _, ok := nr.nodes[node.ID]; !ok {
		return fmt.Errorf("node not found: %x", node.ID)
	}

	node.LastSeen = time.Now()
	nr.nodes[node.ID] = node

	return nil
}

// CleanInactive removes nodes that have been inactive for too long.
func (nr *NodeRegistry) CleanInactive(timeout time.Duration) int {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	cutoff := time.Now().Add(-timeout)
	var removed int

	for id, node := range nr.nodes {
		if node.Status == NodeStatusInactive && node.LastSeen.Before(cutoff) {
			// Remove from model index
			for _, model := range node.Models {
				ids := nr.byModel[model]
				newIDs := make([]interfaces.NodeID, 0, len(ids))
				for _, nodeID := range ids {
					if nodeID != id {
						newIDs = append(newIDs, nodeID)
					}
				}
				nr.byModel[model] = newIDs
			}
			delete(nr.nodes, id)
			removed++
		}
	}

	return removed
}

// GetStakingManager returns the staking manager.
func (nr *NodeRegistry) GetStakingManager() *StakingManager {
	return nr.stakingMgr
}
