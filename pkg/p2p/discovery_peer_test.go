package p2p

import (
	"context"
	"testing"
	"time"
)

func TestNewDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)

	// Test with nil config
	discovery, err := NewDiscovery(network, nil)
	if err != nil {
		t.Fatalf("Failed to create discovery with nil config: %v", err)
	}

	if discovery == nil {
		t.Fatal("Discovery should not be nil")
	}

	// Test default values
	if discovery.interval != 60*time.Second {
		t.Errorf("Default interval = %v, expected 60s", discovery.interval)
	}

	if discovery.maxPeers != 100 {
		t.Errorf("Default maxPeers = %d, expected 100", discovery.maxPeers)
	}

	if discovery.minPeers != 5 {
		t.Errorf("Default minPeers = %d, expected 5", discovery.minPeers)
	}

	// Test with custom config
	cfg := &DiscoveryConfig{
		Interval:       30 * time.Second,
		MaxPeers:       50,
		MinPeers:       3,
		AnnounceModels: []string{"gpt-4", "claude-3"},
	}

	discovery2, err := NewDiscovery(network, cfg)
	if err != nil {
		t.Fatalf("Failed to create discovery with custom config: %v", err)
	}

	if discovery2.interval != cfg.Interval {
		t.Errorf("Custom interval = %v, expected %v", discovery2.interval, cfg.Interval)
	}

	if discovery2.maxPeers != cfg.MaxPeers {
		t.Errorf("Custom maxPeers = %d, expected %d", discovery2.maxPeers, cfg.MaxPeers)
	}

	if discovery2.minPeers != cfg.MinPeers {
		t.Errorf("Custom minPeers = %d, expected %d", discovery2.minPeers, cfg.MinPeers)
	}
}

func TestDiscovery_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// Start discovery
	err := discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// Stop discovery
	err = discovery.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}
}

func TestDiscovery_GetPeerCount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// Initially empty
	count := discovery.GetPeerCount()
	if count != 0 {
		t.Errorf("Initial peer count = %d, expected 0", count)
	}
}

func TestDiscovery_HasMinimumPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)

	cfg := &DiscoveryConfig{
		MinPeers: 3,
	}
	discovery, _ := NewDiscovery(network, cfg)

	// Initially should not have minimum
	if discovery.HasMinimumPeers() {
		t.Error("Should not have minimum peers initially")
	}
}

func TestDiscovery_GetPeersForModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// Initially empty
	peers := discovery.GetPeersForModel("gpt-4")
	if len(peers) != 0 {
		t.Errorf("Initial peers for model = %d, expected 0", len(peers))
	}
}

func TestNewPeerManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)

	// Test with nil config
	pm, err := NewPeerManager(network, nil)
	if err != nil {
		t.Fatalf("Failed to create peer manager: %v", err)
	}

	if pm == nil {
		t.Fatal("Peer manager should not be nil")
	}

	// Test default values
	if pm.maxPeers != 100 {
		t.Errorf("Default maxPeers = %d, expected 100", pm.maxPeers)
	}

	if pm.peerTTL != 10*time.Minute {
		t.Errorf("Default peerTTL = %v, expected 10m", pm.peerTTL)
	}

	// Test with custom config
	cfg := &PeerManagerConfig{
		MaxPeers:        50,
		PeerTTL:         5 * time.Minute,
		KeepAlivePeriod: 15 * time.Second,
	}

	pm2, err := NewPeerManager(network, cfg)
	if err != nil {
		t.Fatalf("Failed to create peer manager with custom config: %v", err)
	}

	if pm2.maxPeers != cfg.MaxPeers {
		t.Errorf("Custom maxPeers = %d, expected %d", pm2.maxPeers, cfg.MaxPeers)
	}

	if pm2.peerTTL != cfg.PeerTTL {
		t.Errorf("Custom peerTTL = %v, expected %v", pm2.peerTTL, cfg.PeerTTL)
	}
}

func TestPeerManager_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Start peer manager
	err := pm.Start()
	if err != nil {
		t.Fatalf("Failed to start peer manager: %v", err)
	}

	// Stop peer manager
	err = pm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop peer manager: %v", err)
	}
}

func TestPeerManager_AddPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, &PeerManagerConfig{
		MaxPeers: 2,
	})

	// Add first peer
	err := pm.AddPeer(PeerID("peer-1"), []string{"/ip4/127.0.0.1/tcp/1234"})
	if err != nil {
		t.Fatalf("Failed to add first peer: %v", err)
	}

	// Add second peer
	err = pm.AddPeer(PeerID("peer-2"), []string{"/ip4/127.0.0.1/tcp/5678"})
	if err != nil {
		t.Fatalf("Failed to add second peer: %v", err)
	}

	// Add third peer should fail (max peers)
	err = pm.AddPeer(PeerID("peer-3"), []string{"/ip4/127.0.0.1/tcp/9999"})
	if err == nil {
		t.Error("Should fail to add third peer (max peers reached)")
	}
}

func TestPeerManager_GetPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	peerID := PeerID("test-peer")
	pm.AddPeer(peerID, []string{"/ip4/127.0.0.1/tcp/1234"})

	// Get existing peer
	peer, err := pm.GetPeer(peerID)
	if err != nil {
		t.Fatalf("Failed to get peer: %v", err)
	}

	if peer.ID != peerID {
		t.Errorf("Peer ID = %s, expected %s", peer.ID, peerID)
	}

	// Get non-existent peer
	_, err = pm.GetPeer(PeerID("non-existent-peer"))
	if err == nil {
		t.Error("Should fail to get non-existent peer")
	}
}

func TestPeerManager_GetAllPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Initially empty
	peers := pm.GetAllPeers()
	if len(peers) != 0 {
		t.Errorf("Initial peer count = %d, expected 0", len(peers))
	}

	// Add some peers
	pm.AddPeer(PeerID("peer-1"), []string{"/ip4/127.0.0.1/tcp/1234"})
	pm.AddPeer(PeerID("peer-2"), []string{"/ip4/127.0.0.1/tcp/5678"})

	peers = pm.GetAllPeers()
	if len(peers) != 2 {
		t.Errorf("Peer count after adding = %d, expected 2", len(peers))
	}
}

func TestPeerManager_GetPeersByStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Add a peer
	pm.AddPeer(PeerID("test-peer"), []string{"/ip4/127.0.0.1/tcp/1234"})

	// Initially should be in unknown status
	unknownPeers := pm.GetPeersByStatus(PeerStatusUnknown)
	if len(unknownPeers) != 1 {
		t.Errorf("Unknown peers count = %d, expected 1", len(unknownPeers))
	}
}

func TestPeerManager_RemovePeer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	peerID := PeerID("test-peer")
	pm.AddPeer(peerID, []string{"/ip4/127.0.0.1/tcp/1234"})

	// Remove peer
	err := pm.RemovePeer(peerID)
	if err != nil {
		t.Fatalf("Failed to remove peer: %v", err)
	}

	// Should not find after removal
	_, err = pm.GetPeer(peerID)
	if err == nil {
		t.Error("Should not find removed peer")
	}

	// Remove non-existent peer should fail
	err = pm.RemovePeer(PeerID("non-existent-peer"))
	if err == nil {
		t.Error("Should fail to remove non-existent peer")
	}
}

func TestPeerManager_SetPeerConnectedHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, _ := NewNetwork(ctx, nil)
	pm, _ := NewPeerManager(network, nil)

	// Test setting handler
	pm.SetPeerConnectedHandler(func(PeerID) {})

	// Should not crash
	if pm == nil {
		t.Fatal("Peer manager should not be nil")
	}
}
