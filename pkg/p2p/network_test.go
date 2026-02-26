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
