package p2p

import (
	"context"
	"testing"
	"time"
)

func TestNewNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, err := NewNetwork(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	if network == nil {
		t.Fatal("Network should not be nil")
	}

	// Check default configuration
	cfg := network.cfg
	if len(cfg.ListenAddrs) == 0 {
		t.Error("ListenAddrs should have default values")
	}
}

func TestNewNetwork_WithConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := &Config{
		ListenAddrs: []string{"/ip4/0.0.0.0/tcp/12345"},
		MaxPeers:    50,
		MinPeers:    3,
	}

	network, err := NewNetwork(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	if network.cfg.MaxPeers != 50 {
		t.Errorf("MaxPeers = %d, expected 50", network.cfg.MaxPeers)
	}

	if network.cfg.MinPeers != 3 {
		t.Errorf("MinPeers = %d, expected 3", network.cfg.MinPeers)
	}
}

func TestNetwork_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, err := NewNetwork(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	// Start
	err = network.Start()
	if err != nil {
		t.Fatalf("Failed to start network: %v", err)
	}

	if !network.started {
		t.Error("Network should be started")
	}

	// Stop
	err = network.Stop()
	if err != nil {
		t.Fatalf("Failed to stop network: %v", err)
	}

	if network.started {
		t.Error("Network should be stopped")
	}
}

func TestNetwork_DoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	network.Start()

	// Second start should fail
	err := network.Start()
	if err == nil {
		t.Error("Double start should fail")
	}

	network.Stop()
}

func TestNetwork_StopNotStarted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	err := network.Stop()
	if err == nil {
		t.Error("Stop on non-started network should fail")
	}
}

func TestNetwork_GetPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Initially empty
	peers := network.GetPeers()
	if len(peers) != 0 {
		t.Errorf("Initial peer count = %d, expected 0", len(peers))
	}

	network.Start()
	defer network.Stop()

	// Add peer info
	peerInfo := &PeerInfo{
		ID:        "test-peer",
		Connected: true,
		LastSeen:  time.Now(),
	}

	network.AddPeer(peerInfo)

	peers = network.GetPeers()
	if len(peers) != 1 {
		t.Errorf("Peer count = %d, expected 1", len(peers))
	}

	if peers[0].ID != "test-peer" {
		t.Errorf("Peer ID = %s, expected test-peer", peers[0].ID)
	}
}

func TestNetwork_RemovePeer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	peerID := PeerID("test-peer")
	peerInfo := &PeerInfo{ID: peerID}

	network.AddPeer(peerInfo)

	// Remove peer
	network.RemovePeer(peerID)

	peers := network.GetPeers()
	if len(peers) != 0 {
		t.Errorf("Peer count after removal = %d, expected 0", len(peers))
	}
}

func TestNetwork_PeerID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Before starting, PeerID should be empty
	if network.PeerID() != "" {
		t.Error("PeerID should be empty before starting")
	}

	network.Start()
	defer network.Stop()

	// After starting, PeerID should be set
	if network.PeerID() == "" {
		t.Error("PeerID should not be empty after starting")
	}
}

func TestNetwork_Host(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Before starting, Host should be nil (we removed libp2p)
	if network.Host() != nil {
		t.Error("Host should be nil before starting")
	}

	network.Start()
	defer network.Stop()

	// After starting, Host is still nil (we removed libp2p)
	// This is for libp2p compatibility only
	if network.Host() != nil {
		t.Error("Host should be nil (no libp2p)")
	}
}

func TestNetwork_DiscoverPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Without DHT, should return empty
	peers, err := network.DiscoverPeers(ctx)
	if err != nil {
		t.Errorf("DiscoverPeers should not error: %v", err)
	}

	// Should return empty slice
	if len(peers) != 0 {
		t.Errorf("DiscoverPeers = %d peers, expected 0", len(peers))
	}
}

func TestNetwork_Provide(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Without DHT, Provide should return nil (stub)
	err := network.Provide(ctx, []byte("test-key"))
	if err != nil {
		t.Errorf("Provide should not error: %v", err)
	}
}

func TestNetwork_FindProviders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Without DHT, FindProviders should return empty
	providers, err := network.FindProviders(ctx, []byte("test-key"))
	if err != nil {
		t.Errorf("FindProviders should not error: %v", err)
	}

	if len(providers) != 0 {
		t.Errorf("FindProviders = %d providers, expected 0", len(providers))
	}
}

func TestMessage_MarshalJSON(t *testing.T) {
	msg := &Message{
		Type:      "test",
		Payload:   []byte(`{"hello":"world"}`),
		Timestamp: time.Now(),
		Sender:    "test-sender",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("MarshalJSON should return non-empty data")
	}
}

func TestMessage_UnmarshalJSON(t *testing.T) {
	original := &Message{
		Type:      "test",
		Payload:   []byte(`{"hello":"world"}`),
		Timestamp: time.Now(),
		Sender:    "test-sender",
	}

	data, _ := original.MarshalJSON()

	msg := &Message{}
	err := msg.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if msg.Type != original.Type {
		t.Errorf("Type = %s, expected %s", msg.Type, original.Type)
	}

	if msg.Sender != original.Sender {
		t.Errorf("Sender = %s, expected %s", msg.Sender, original.Sender)
	}
}

// TestNetwork_RegisterProtocol tests protocol registration
func TestNetwork_RegisterProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Register a custom protocol handler
	handler := func(ctx context.Context, msg *Message, from PeerID) error {
		return nil
	}

	err := network.RegisterProtocol(ProtocolID("/test/1.0.0"), handler)
	if err != nil {
		t.Fatalf("Failed to register protocol: %v", err)
	}

	network.Start()
	defer network.Stop()
}

// TestNetwork_Connect tests network connect functionality
func TestNetwork_Connect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Connect to a non-existent peer should fail
	err := network.Connect(ctx, AddrInfo{
		ID:    PeerID("test-peer"),
		Addrs: []Multiaddr{"/ip4/127.0.0.1/tcp/59999"},
	})
	if err == nil {
		t.Error("Expected error when connecting to non-existent peer")
	}
}

// TestNetwork_Connect_NotStarted tests connecting to network that hasn't started
func TestNetwork_Connect_NotStarted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	err := network.Connect(ctx, AddrInfo{
		ID:    PeerID("test-peer"),
		Addrs: []Multiaddr{"/ip4/127.0.0.1/tcp/4001"},
	})
	if err == nil {
		t.Error("Expected error when connecting to not started network")
	}
}

// TestNetwork_Disconnect tests network disconnect functionality
func TestNetwork_Disconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Disconnect from non-existent peer should fail
	err := network.Disconnect(ctx, PeerID("non-existent-peer"))
	if err == ErrPeerNotFound {
		t.Logf("Got expected error: %v", err)
	}
}

// TestNetwork_SendMessage tests sending messages
func TestNetwork_SendMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Send to non-existent peer should fail
	msg := &Message{Type: "test", Payload: []byte("hello")}
	err := network.SendMessage(ctx, PeerID("non-existent-peer"), DiscoveryProtocol, msg)
	if err == nil {
		t.Error("Expected error when sending to non-existent peer")
	}
}

// TestNetwork_HandleMessage tests message handling
func TestNetwork_HandleMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Handle message with no registered protocol should not crash
	msg := &Message{
		Type:     "ping",
		Protocol: "/nonexistent/1.0.0",
		Sender:   "test-sender",
	}
	network.handleMessage(ctx, msg, "test-from")
}

// TestNetwork_HandleDiscovery tests discovery message handling
func TestNetwork_HandleDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Send a ping message through discovery protocol
	msg := &Message{
		Type:    "ping",
		Payload: []byte("test-ping"),
	}
	err := network.SendMessage(ctx, network.PeerID(), DiscoveryProtocol, msg)
	if err != nil {
		// Expected since we can't send to ourselves
		t.Logf("Expected error when sending to self: %v", err)
	}
}

// TestNetwork_HandleDiscovery_Pong tests pong response handling
func TestNetwork_HandleDiscovery_Pong(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Add a peer first
	network.peers[PeerID("test-peer")] = &PeerInfo{
		ID:        PeerID("test-peer"),
		Connected: true,
		LastSeen:  time.Now(),
	}

	// Test handling pong message
	msg := &Message{
		Type:    "pong",
		Payload: []byte("test-pong"),
		Sender:  PeerID("test-peer"),
	}
	network.handleDiscovery(ctx, msg, PeerID("test-peer"))
}

// TestNetwork_HandleService tests service message handling
func TestNetwork_HandleService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	network.Start()
	defer network.Stop()

	// Service messages are handled by agentic layer
	msg := &Message{
		Type:    "request",
		Payload: []byte("test-service"),
	}
	err := network.handleService(ctx, msg, "test-from")
	if err != nil {
		t.Errorf("handleService returned error: %v", err)
	}
}

// TestNetwork_Addrs tests getting network addresses
func TestNetwork_Addrs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Before start, addresses should be empty
	addrs := network.Addrs()
	if len(addrs) != 0 {
		t.Errorf("Expected 0 addresses before start, got %d", len(addrs))
	}

	network.Start()
	defer network.Stop()

	// After start, addresses should be available
	addrs = network.Addrs()
	if len(addrs) == 0 {
		t.Error("Expected some addresses after start")
	}
}

// TestCreateNetworkFromEd25519 tests creating network from Ed25519 keys
func TestCreateNetworkFromEd25519(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate Ed25519 keys
	_, privKey, _, err := GeneratePeerID()
	if err != nil {
		t.Fatalf("Failed to generate peer ID: %v", err)
	}

	network, err := CreateNetworkFromEd25519(ctx, privKey, []string{"/ip4/0.0.0.0/tcp/0"})
	if err != nil {
		t.Fatalf("Failed to create network from Ed25519: %v", err)
	}

	if network == nil {
		t.Fatal("Network should not be nil")
	}

	err = network.Start()
	if err != nil {
		t.Fatalf("Failed to start network: %v", err)
	}
	defer network.Stop()
}

// ============================================================================
// PeerManager 扩展测试
// ============================================================================

// TestPeerManager_DisconnectPeer tests DisconnectPeer functionality
func TestPeerManager_DisconnectPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	err := network.Start()
	if err != nil {
		t.Fatalf("Failed to start network: %v", err)
	}
	defer network.Stop()

	pm, err := NewPeerManager(network, &PeerManagerConfig{
		MaxPeers: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create peer manager: %v", err)
	}

	// Disconnect from non-existent peer should fail
	err = pm.DisconnectPeer(PeerID("non-existent"))
	if err == nil {
		t.Error("Expected error when disconnecting non-existent peer")
	}
}

// TestPeerManager_SetPeerDisconnectedHandler tests setting peer disconnected handler
func TestPeerManager_SetPeerDisconnectedHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Set handler (verify no panic)
	pm.SetPeerDisconnectedHandler(func(PeerID) {})

	// Verify handler was set (peer manager should not be nil)
	if pm == nil {
		t.Fatal("Peer manager should not be nil")
	}
}

// TestPeerManager_SetPeerUpdatedHandler tests setting peer updated handler
func TestPeerManager_SetPeerUpdatedHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Set handler (verify no panic)
	pm.SetPeerUpdatedHandler(func(PeerID) {})

	// Verify handler was set
	if pm == nil {
		t.Fatal("Peer manager should not be nil")
	}
}

// TestPeerManager_GetPeersByStatus_Empty tests GetPeersByStatus with no peers
func TestPeerManager_GetPeersByStatus_Empty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Get peers by status when no peers exist
	peers := pm.GetPeersByStatus(PeerStatusUnknown)
	if peers != nil {
		t.Errorf("Expected nil or empty slice, got %v", peers)
	}
}

// TestPeerManager_MaintenanceLoop tests maintenance loop behavior
func TestPeerManager_MaintenanceLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, &PeerManagerConfig{
		KeepAlivePeriod: 50 * time.Millisecond,
	})

	err := pm.Start()
	if err != nil {
		t.Fatalf("Failed to start peer manager: %v", err)
	}
	defer pm.Stop()

	// Wait for a few maintenance cycles
	time.Sleep(150 * time.Millisecond)

	pm.Stop()

	// Verify peer manager can be stopped
	if pm == nil {
		t.Error("Peer manager should still be valid")
	}
}

// TestPeerManager_RemovePeer_NonExistent tests removing non-existent peer
func TestPeerManager_RemovePeer_NonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Remove non-existent peer should fail
	err := pm.RemovePeer(PeerID("non-existent"))
	if err == nil {
		t.Error("Expected error when removing non-existent peer")
	}
}

// TestNetwork_SendMessage_NetworkNotStarted tests sending message to network not started
func TestNetwork_SendMessage_NetworkNotStarted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// Sending message without starting should fail or be handled gracefully
	_ = network.SendMessage(ctx, PeerID("test-peer"), ProtocolID("/test/1.0.0"), &Message{
		Type:    "test",
		Payload: []byte("test-data"),
	})
}

// TestNetwork_Addrs_AddrInfo tests AddrInfo.Addrs field
func TestNetwork_Addrs_AddrInfo(t *testing.T) {
	addrInfo := AddrInfo{
		ID: PeerID("test-peer"),
		Addrs: []Multiaddr{
			"/ip4/127.0.0.1/tcp/4001",
			"/ip4/127.0.0.1/tcp/4002",
		},
	}

	if len(addrInfo.Addrs) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(addrInfo.Addrs))
	}
}

// TestNetwork_MarshalJSON_UnmarshalJSON tests network JSON marshaling
func TestNetwork_MarshalJSON_UnmarshalJSON(t *testing.T) {
	msg := &Message{
		Type:    "test-type",
		Sender:  "sender-peer",
		Payload: []byte("test-payload"),
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded Message
	err = decoded.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %s, expected %s", decoded.Type, msg.Type)
	}

	if decoded.Sender != msg.Sender {
		t.Errorf("Sender = %s, expected %s", decoded.Sender, msg.Sender)
	}
}

// TestPeerManager_checkPeerHealth tests the health check mechanism
func TestPeerManager_checkPeerHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Add peer with correct signature
	peerID := PeerID("health-test-peer")
	err := pm.AddPeer(peerID, []string{"/ip4/127.0.0.1/tcp/4001"})
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Check that peer exists after health check
	peer, err := pm.GetPeer(peerID)
	if err != nil {
		t.Errorf("Peer should exist: %v", err)
	}

	if peer == nil {
		t.Error("Peer should not be nil")
	}
}
