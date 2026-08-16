package agentic_test

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/agentic"
)

// MockChannelManager implements interfaces.ChannelManager for testing
type MockChannelManager struct{}

func (m *MockChannelManager) OpenChannel(ctx context.Context, partyA, partyB interfaces.Address, depositA, depositB uint64) (*interfaces.Channel, error) {
	return &interfaces.Channel{
		ID:           [32]byte{1, 2, 3},
		PartyA:       partyA,
		PartyB:       partyB,
		BalanceA:     depositA,
		BalanceB:     depositB,
		Sequence:     0,
		CreatedAt:    time.Now(),
	}, nil
}

func (m *MockChannelManager) UpdateState(ch *interfaces.Channel, newState interfaces.SignedState) (*interfaces.SignedState, error) {
	return &newState, nil
}

func (m *MockChannelManager) CloseChannel(ctx context.Context, ch *interfaces.Channel, finalState interfaces.SignedState) error {
	return nil
}

func (m *MockChannelManager) Dispute(ctx context.Context, ch *interfaces.Channel, evidence interfaces.SignedState) error {
	return nil
}

func (m *MockChannelManager) GetChannelState(channelID [32]byte) (*interfaces.Channel, error) {
	return nil, nil
}

func newTestConfig(t *testing.T) *agentic.Config {
	t.Helper()
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	return &agentic.Config{
		NodeID:          interfaces.NodeID{1, 2, 3},
		PrivateKey:      privKey,
		PublicKey:       privKey.Public().(ed25519.PublicKey),
		Address:         interfaces.Address{4, 5, 6},
		MinStake:        1000,
		SlashThreshold:  500,
		ReputationDecay: 0.9,
		MaxNodes:        100,
		ServiceTimeout:  30 * time.Second,
	}
}

func TestNewAgenticService(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}

	service, err := agentic.NewAgenticService(channelMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create agentic service: %v", err)
	}

	if service == nil {
		t.Fatal("Service should not be nil")
	}

	// Test configuration
	if service.Config() != cfg {
		t.Error("Config should match")
	}

	// Test node registry
	if service.NodeRegistry() == nil {
		t.Error("Node registry should not be nil")
	}

	// Test staking manager
	if service.StakingManager() == nil {
		t.Error("Staking manager should not be nil")
	}
}

func TestAgenticService_StartStop(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	ctx := context.Background()

	// Start
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Stop
	err = service.Stop()
	if err != nil {
		t.Fatalf("Failed to stop service: %v", err)
	}
}

func TestAgenticService_ChatCompletion(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	ctx := context.Background()

	req := interfaces.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []interfaces.Message{
			{Role: "user", Content: "Hello, world!"},
		},
		MaxTokens:    100,
		Temperature:  0.7,
	}

	// Since we don't have actual providers registered, this will fail
	// But we can test the error handling
	_, err := service.ChatCompletion(ctx, req)
	if err == nil {
		t.Error("Should fail without provider")
	}
}

func TestAgenticService_Messages(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	ctx := context.Background()

	req := interfaces.MessagesRequest{
		Model: "claude-3",
		Messages: []interfaces.Message{
			{Role: "user", Content: "Hello, world!"},
		},
		MaxTokens: 100,
		System:    "You are a helpful assistant.",
	}

	// Since we don't have actual providers registered, this will fail
	_, err := service.Messages(ctx, req)
	if err == nil {
		t.Error("Should fail without provider")
	}
}

func TestAgenticService_GetAvailableNodes(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	ctx := context.Background()

	// Initially empty
	nodes, err := service.GetAvailableNodes(ctx)
	if err != nil {
		t.Fatalf("GetAvailableNodes failed: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("Initial node count = %d, expected 0", len(nodes))
	}
}

func TestAgenticService_OpenServiceChannel(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	ctx := context.Background()

	// Create a test node
	_, privKey, _ := ed25519.GenerateKey(nil)
	nodeID := interfaces.NodeID{9, 9, 9}
	nodeInfo := &agentic.NodeInfo{
		ID:        nodeID,
		Address:   interfaces.Address{8, 8, 8},
		PublicKey: privKey.Public().(ed25519.PublicKey),
		Stake:     5000,
		Models:    []string{"gpt-4"},
		Reputation: 0.8,
		Status:    agentic.NodeStatusActive,
	}

	// Register the node
	err := service.NodeRegistry().RegisterNode(nodeInfo)
	if err != nil {
		t.Fatalf("Failed to register node: %v", err)
	}

	// Open channel
	channel, err := service.OpenServiceChannel(ctx, nodeID, 1000)
	if err != nil {
		t.Fatalf("Failed to open service channel: %v", err)
	}

	if channel == nil {
		t.Fatal("Channel should not be nil")
	}

	if channel.PartyA != cfg.Address {
		t.Error("PartyA should match our address")
	}

	if channel.PartyB != nodeInfo.Address {
		t.Error("PartyB should match node address")
	}

	if channel.BalanceA != 1000 {
		t.Errorf("BalanceA = %d, expected 1000", channel.BalanceA)
	}

	if channel.BalanceB != 0 {
		t.Errorf("BalanceB = %d, expected 0", channel.BalanceB)
	}
}

func TestAgenticService_RegisterProvider(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	// Create provider
	_, privKey, _ := ed25519.GenerateKey(nil)
	nodeInfo := &agentic.NodeInfo{
		ID:        interfaces.NodeID{1},
		PublicKey: privKey.Public().(ed25519.PublicKey),
		Stake:     1000,
		Models:    []string{"gpt-4"},
		Reputation: 0.9,
		Status:    agentic.NodeStatusActive,
	}

	provider := &agentic.ServiceProvider{
		NodeInfo: nodeInfo,
		Pricing:  map[string]uint64{"gpt-4": 10},
		Capacity: 100,
		Load:     0,
		SuccessRate: 1.0,
	}

	// Register provider
	err := service.RegisterProvider(provider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Get provider back
	retrieved, err := service.GetProvider(nodeInfo.ID)
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved != provider {
		t.Error("Retrieved provider should match original")
	}
}

func TestAgenticService_UnregisterProvider(t *testing.T) {
	cfg := newTestConfig(t)
	channelMgr := &MockChannelManager{}
	service, _ := agentic.NewAgenticService(channelMgr, cfg)

	nodeID := interfaces.NodeID{1}
	provider := &agentic.ServiceProvider{
		NodeInfo: &agentic.NodeInfo{ID: nodeID},
	}

	service.RegisterProvider(provider)

	// Unregister
	err := service.UnregisterProvider(nodeID)
	if err != nil {
		t.Fatalf("Failed to unregister provider: %v", err)
	}

	// Should not find after unregister
	_, err = service.GetProvider(nodeID)
	if err == nil {
		t.Error("Should not find unregistered provider")
	}
}

func TestNodeRegistry_RegisterUnregister(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, err := agentic.NewNodeRegistry(stakingMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create node registry: %v", err)
	}

	// Create test node
	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:     nodeID,
		Stake:  1000,
		Models: []string{"gpt-4"},
		Status: agentic.NodeStatusActive,
	}

	// Register
	err = nr.RegisterNode(nodeInfo)
	if err != nil {
		t.Fatalf("Failed to register node: %v", err)
	}

	// Get node
	retrieved, err := nr.GetNode(nodeID)
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if retrieved.ID != nodeID {
		t.Errorf("Node ID = %v, expected %v", retrieved.ID, nodeID)
	}

	// Unregister
	err = nr.UnregisterNode(nodeID)
	if err != nil {
		t.Fatalf("Failed to unregister node: %v", err)
	}

	// Should not find after unregister
	_, err = nr.GetNode(nodeID)
	if err == nil {
		t.Error("Should not find unregistered node")
	}
}

func TestNodeRegistry_FindNodesByModel(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	// Register nodes with different models
	for i := 0; i < 3; i++ {
		nodeID := interfaces.NodeID{}
		nodeID[0] = byte(i)
		nodeInfo := &agentic.NodeInfo{
			ID:     nodeID,
			Models: []string{fmt.Sprintf("model-%d", i%2)},
		}
		nr.RegisterNode(nodeInfo)
	}

	// Find nodes for model-0
	nodes, err := nr.FindNodesByModel("model-0")
	if err != nil {
		t.Fatalf("FindNodesByModel failed: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Found %d nodes for model-0, expected 2", len(nodes))
	}

	// Find nodes for model-1
	nodes, err = nr.FindNodesByModel("model-1")
	if err != nil {
		t.Fatalf("FindNodesByModel failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Found %d nodes for model-1, expected 1", len(nodes))
	}
}

func TestNodeRegistry_FindNodesByFilter(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	// Register test nodes
	node1 := &agentic.NodeInfo{
		ID:         interfaces.NodeID{1},
		Stake:      1000,
		Reputation: 0.8,
		Models:     []string{"gpt-4"},
		Services:   []agentic.ServiceType{agentic.ServiceTypeChat},
	}

	node2 := &agentic.NodeInfo{
		ID:         interfaces.NodeID{2},
		Stake:      500,
		Reputation: 0.6,
		Models:     []string{"claude-3"},
		Services:   []agentic.ServiceType{agentic.ServiceTypeCompletion},
	}

	nr.RegisterNode(node1)
	nr.RegisterNode(node2)

	// Test filter
	filter := &agentic.NodeFilter{
		MinReputation: 0.7,
		MinStake:      800,
		Models:        []string{"gpt-4"},
		Services:      []agentic.ServiceType{agentic.ServiceTypeChat},
	}

	nodes, err := nr.FindNodesByFilter(filter)
	if err != nil {
		t.Fatalf("FindNodesByFilter failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Found %d nodes with filter, expected 1", len(nodes))
	}

	if nodes[0].ID != node1.ID {
		t.Errorf("Found node ID = %v, expected %v", nodes[0].ID, node1.ID)
	}
}

func TestNodeRegistry_UpdateNodeInfo(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:     nodeID,
		Status: agentic.NodeStatusActive,
	}

	nr.RegisterNode(nodeInfo)

	// Update info
	updatedInfo := &agentic.NodeInfo{
		ID:         nodeID,
		Stake:      2000,
		Reputation: 0.9,
		Status:     agentic.NodeStatusActive,
	}
	err := nr.UpdateNodeInfo(updatedInfo)
	if err != nil {
		t.Fatalf("UpdateNodeInfo failed: %v", err)
	}

	// Verify update
	retrieved, _ := nr.GetNode(nodeID)
	if retrieved.Stake != 2000 {
		t.Errorf("Stake = %d, expected 2000", retrieved.Stake)
	}
}

func TestNodeRegistry_UpdateNodeInfo_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:     nodeID,
		Status: agentic.NodeStatusActive,
	}

	// Try to update non-existent node
	err := nr.UpdateNodeInfo(nodeInfo)
	if err == nil {
		t.Error("Should fail for non-existent node")
	}
}

func TestNodeRegistry_CleanInactive(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	// Register inactive node
	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:        nodeID,
		Status:    agentic.NodeStatusInactive,
		LastSeen:  time.Now().Add(-2 * time.Hour), // 2 hours ago
		Models:    []string{"gpt-4"},
	}
	nr.RegisterNode(nodeInfo)

	// Clean with 1 hour timeout
	removed := nr.CleanInactive(1 * time.Hour)
	if removed != 1 {
		t.Errorf("Removed = %d, expected 1", removed)
	}

	// Verify node is removed
	_, err := nr.GetNode(nodeID)
	if err == nil {
		t.Error("Node should be removed")
	}
}

func TestNodeRegistry_CleanInactive_NoRemove(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	// Register active node
	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:        nodeID,
		Status:    agentic.NodeStatusActive,
		LastSeen:  time.Now(),
		Models:    []string{"gpt-4"},
	}
	nr.RegisterNode(nodeInfo)

	// Clean with 1 hour timeout
	removed := nr.CleanInactive(1 * time.Hour)
	if removed != 0 {
		t.Errorf("Removed = %d, expected 0", removed)
	}
}

func TestNodeRegistry_UnregisterNode_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	nodeID := interfaces.NodeID{9, 9, 9}

	err := nr.UnregisterNode(nodeID)
	if err == nil {
		t.Error("Should fail for non-existent node")
	}
}

func TestNodeRegistry_UpdateNodeStatus(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	// Register node
	nodeID := interfaces.NodeID{1}
	nodeInfo := &agentic.NodeInfo{
		ID:     nodeID,
		Status: agentic.NodeStatusActive,
		Models: []string{"gpt-4"},
	}
	nr.RegisterNode(nodeInfo)

	// Update status
	err := nr.UpdateNodeStatus(nodeID, agentic.NodeStatusInactive)
	if err != nil {
		t.Fatalf("UpdateNodeStatus failed: %v", err)
	}

	// Verify update
	retrieved, _ := nr.GetNode(nodeID)
	if retrieved.Status != agentic.NodeStatusInactive {
		t.Errorf("Status = %v, expected NodeStatusInactive", retrieved.Status)
	}
}

func TestNodeRegistry_UpdateNodeStatus_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	nodeID := interfaces.NodeID{9, 9, 9}

	err := nr.UpdateNodeStatus(nodeID, agentic.NodeStatusInactive)
	if err == nil {
		t.Error("Should fail for non-existent node")
	}
}

func TestNodeRegistry_GetStakingManager(t *testing.T) {
	cfg := newTestConfig(t)
	stakingMgr, _ := agentic.NewStakingManager(cfg)
	nr, _ := agentic.NewNodeRegistry(stakingMgr, cfg)

	result := nr.GetStakingManager()
	if result != stakingMgr {
		t.Error("GetStakingManager should return the staking manager")
	}
}

func TestDiscoveryMessage_Unmarshal(t *testing.T) {
	msg := &agentic.DiscoveryMessage{
		NodeID:   interfaces.NodeID{1, 2, 3},
		Endpoint: "http://localhost:8080",
		Models:   []string{"gpt-4"},
		Services: []agentic.ServiceType{agentic.ServiceTypeChat},
		Version:  "1.0",
		Timestamp: time.Now(),
	}

	// Marshal first
	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	newMsg := &agentic.DiscoveryMessage{}
	err = newMsg.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if newMsg.NodeID != msg.NodeID {
		t.Errorf("NodeID = %v, expected %v", newMsg.NodeID, msg.NodeID)
	}

	if newMsg.Endpoint != msg.Endpoint {
		t.Errorf("Endpoint = %s, expected %s", newMsg.Endpoint, msg.Endpoint)
	}
}

func TestDiscoveryMessage_Unmarshal_InvalidJSON(t *testing.T) {
	msg := &agentic.DiscoveryMessage{}
	err := msg.Unmarshal([]byte("invalid json"))
	if err == nil {
		t.Error("Should fail for invalid JSON")
	}
}
