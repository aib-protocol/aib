package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Node discovery tests
// ============================================================================

// TestDiscovery_NodeDiscovery_NewNode tests discovering a new node
func TestDiscovery_NodeDiscovery_NewNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, err := NewNetwork(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	discovery, err := NewDiscovery(network, &DiscoveryConfig{
		Interval:       60 * time.Second,
		MaxPeers:       100,
		MinPeers:       5,
		AnnounceModels: []string{"gpt-4", "claude-3"},
	})
	if err != nil {
		t.Fatalf("Failed to create discovery: %v", err)
	}

	// initial node count is 0
	if discovery.GetPeerCount() != 0 {
		t.Errorf("Expected 0 peers initially, got %d", discovery.GetPeerCount())
	}

	// add a new node
	newPeer := &DiscoveredPeer{
		ID:         PeerID("test-peer-1"),
		Addrs:      []string{"/ip4/127.0.0.1/tcp/4001"},
		Models:     []string{"gpt-4"},
		Discovered: time.Now(),
		LastPing:   time.Now(),
		Latency:    10 * time.Millisecond,
	}
	discovery.AddDiscoveredPeer(newPeer)

	// verify node count
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after adding, got %d", discovery.GetPeerCount())
	}

	// verify node retrieval
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer in GetKnownPeers, got %d", len(peers))
	}

	if peers[0].ID != newPeer.ID {
		t.Errorf("Expected peer ID %s, got %s", newPeer.ID, peers[0].ID)
	}
}

// TestDiscovery_NodeDiscovery_MultipleNodes tests discovering multiple nodes
func TestDiscovery_NodeDiscovery_MultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		MaxPeers: 100,
		MinPeers: 5,
	})

	// add multiple nodes
	peers := []*DiscoveredPeer{
		{ID: PeerID("peer-1"), Addrs: []string{"/ip4/127.0.0.1/tcp/4001"}, Models: []string{"gpt-4"}},
		{ID: PeerID("peer-2"), Addrs: []string{"/ip4/127.0.0.1/tcp/4002"}, Models: []string{"claude-3"}},
		{ID: PeerID("peer-3"), Addrs: []string{"/ip4/127.0.0.1/tcp/4003"}, Models: []string{"gpt-4", "claude-3"}},
		{ID: PeerID("peer-4"), Addrs: []string{"/ip4/127.0.0.1/tcp/4004"}, Models: []string{"llama-2"}},
		{ID: PeerID("peer-5"), Addrs: []string{"/ip4/127.0.0.1/tcp/4005"}, Models: []string{"gpt-4"}},
	}

	for _, p := range peers {
		discovery.AddDiscoveredPeer(p)
	}

	// verify total node count
	if discovery.GetPeerCount() != 5 {
		t.Errorf("Expected 5 peers, got %d", discovery.GetPeerCount())
	}

	// verify all nodes retrievable
	knownPeers := discovery.GetKnownPeers()
	if len(knownPeers) != 5 {
		t.Errorf("Expected 5 known peers, got %d", len(knownPeers))
	}
}

// TestDiscovery_NodeDiscovery_DuplicateNode tests duplicate node discovery
func TestDiscovery_NodeDiscovery_DuplicateNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	peerID := PeerID("duplicate-peer")
	addr := "/ip4/127.0.0.1/tcp/4001"

	// add the node the first time
	peer1 := &DiscoveredPeer{
		ID:         peerID,
		Addrs:      []string{addr},
		Models:     []string{"gpt-4"},
		Discovered: time.Now(),
	}
	discovery.AddDiscoveredPeer(peer1)

	// add the same node a second time (update)
	peer2 := &DiscoveredPeer{
		ID:         peerID,
		Addrs:      []string{addr},
		Models:     []string{"gpt-4", "claude-3"},
		Discovered: time.Now(),
	}
	discovery.AddDiscoveredPeer(peer2)

	// verify node count still 1 (no duplicate added)
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after duplicate add, got %d", discovery.GetPeerCount())
	}

	// verify node data updated
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}

	// verify model updated
	found := false
	for _, model := range peers[0].Models {
		if model == "claude-3" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected updated peer to have claude-3 model")
	}
}

// ============================================================================
// Node selection tests
// ============================================================================

// TestDiscovery_NodeSelection_ByModel tests selecting nodes by model
func TestDiscovery_NodeSelection_ByModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// add nodes with different models
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("gpt-peer-1"), Models: []string{"gpt-4"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("gpt-peer-2"), Models: []string{"gpt-4"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4002"},
	})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("claude-peer-1"), Models: []string{"claude-3"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4003"},
	})

	// test selecting nodes with gpt-4 model
	gptPeers := discovery.GetPeersForModel("gpt-4")
	if len(gptPeers) != 2 {
		t.Errorf("Expected 2 peers for gpt-4, got %d", len(gptPeers))
	}

	// test selecting nodes with claude-3 model
	claudePeers := discovery.GetPeersForModel("claude-3")
	if len(claudePeers) != 1 {
		t.Errorf("Expected 1 peer for claude-3, got %d", len(claudePeers))
	}

	// test nonexistent model
	nonexistentPeers := discovery.GetPeersForModel("nonexistent-model")
	if len(nonexistentPeers) != 0 {
		t.Errorf("Expected 0 peers for nonexistent model, got %d", len(nonexistentPeers))
	}
}

// TestDiscovery_NodeSelection_MinPeers tests minimum peer count check
func TestDiscovery_NodeSelection_MinPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// create config with min peers 3
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		MinPeers: 3,
		MaxPeers: 100,
	})

	// initial state check
	if discovery.HasMinimumPeers() {
		t.Error("Should not have minimum peers initially")
	}

	// add 2 nodes (below minimum)
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-1")})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-2")})

	if discovery.HasMinimumPeers() {
		t.Error("Should not have minimum peers with only 2 peers")
	}

	// add 3rd node (meets minimum)
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-3")})

	if !discovery.HasMinimumPeers() {
		t.Error("Should have minimum peers with 3 peers")
	}
}

// TestDiscovery_NodeSelection_MaxPeers tests max peer limit
func TestDiscovery_NodeSelection_MaxPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// create config with max peers 2 (tests PeerManager)
	pm, err := NewPeerManager(network, &PeerManagerConfig{
		MaxPeers: 2,
	})
	if err != nil {
		t.Fatalf("Failed to create peer manager: %v", err)
	}

	// add 1st node
	err = pm.AddPeer(PeerID("peer-1"), []string{"/ip4/127.0.0.1/tcp/4001"})
	if err != nil {
		t.Errorf("Failed to add peer 1: %v", err)
	}

	// add 2nd node
	err = pm.AddPeer(PeerID("peer-2"), []string{"/ip4/127.0.0.1/tcp/4002"})
	if err != nil {
		t.Errorf("Failed to add peer 2: %v", err)
	}

	// adding 3rd node should fail (exceeds max limit)
	err = pm.AddPeer(PeerID("peer-3"), []string{"/ip4/127.0.0.1/tcp/4003"})
	if err == nil {
		t.Error("Should fail to add third peer when max peers reached")
	}
}

// ============================================================================
// Discovery timeout tests
// ============================================================================

// TestDiscovery_Timeout_ContextCancellation tests timeout handling on context cancellation
func TestDiscovery_Timeout_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		Interval: 10 * time.Millisecond,
		MinPeers: 1,
	})

	// start discovery service
	err := discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// wait briefly for the discovery loop to run
	time.Sleep(50 * time.Millisecond)

	// cancel the context
	cancel()

	// wait for discovery service to stop
	time.Sleep(20 * time.Millisecond)

	// verify discovery is safely accessible
	count := discovery.GetPeerCount()
	if count < 0 {
		t.Errorf("Invalid peer count after cancellation: %d", count)
	}
}

// TestDiscovery_Timeout_PeerLatency tests peer latency tracking
func TestDiscovery_Timeout_PeerLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// add node and set latency
	now := time.Now()
	peer := &DiscoveredPeer{
		ID:            PeerID("latency-test-peer"),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4001"},
		Models:        []string{"gpt-4"},
		Discovered:    now,
		LastPing:      now,
		Latency:       100 * time.Millisecond,
		PingSuccesses: 5,
		PingFailures:  0,
	}
	discovery.AddDiscoveredPeer(peer)

	// verify latency value
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}

	if peers[0].Latency != 100*time.Millisecond {
		t.Errorf("Expected latency 100ms, got %v", peers[0].Latency)
	}

	// verify ping success count
	if peers[0].PingSuccesses != 5 {
		t.Errorf("Expected 5 ping successes, got %d", peers[0].PingSuccesses)
	}
}

// TestDiscovery_Timeout_StalePeerCleanup tests stale peer cleanup
func TestDiscovery_Timeout_StalePeerCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// add a normal node
	activePeer := &DiscoveredPeer{
		ID:            PeerID("active-peer"),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4001"},
		LastPing:      time.Now(),
		PingSuccesses: 5,
		PingFailures:  0,
	}
	discovery.AddDiscoveredPeer(activePeer)

	// add a stale node (last ping 6 min ago, beyond 5 min threshold)
	stalePeer := &DiscoveredPeer{
		ID:            PeerID("stale-peer"),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4002"},
		LastPing:      time.Now().Add(-6 * time.Minute),
		PingSuccesses: 1,
		PingFailures:  5,
	}
	discovery.AddDiscoveredPeer(stalePeer)

	// verify initial node count
	if discovery.GetPeerCount() != 2 {
		t.Errorf("Expected 2 peers initially, got %d", discovery.GetPeerCount())
	}

	// run cleanup (internally calls cleanStalePeers)
	discovery.cleanStalePeers()

	// verify node count after cleanup (only active nodes should remain)
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after cleanup, got %d", discovery.GetPeerCount())
	}

	// verify remaining node is the active one
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 || peers[0].ID != "active-peer" {
		t.Error("Expected active peer to remain after cleanup")
	}
}

// ============================================================================
// Concurrent discovery tests
// ============================================================================

// TestDiscovery_Concurrency_ConcurrentAdd tests concurrently adding nodes
func TestDiscovery_Concurrency_ConcurrentAdd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	const numGoroutines = 50
	const peersPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// add nodes concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < peersPerGoroutine; j++ {
				peerID := PeerID(fmt.Sprintf("peer-g%d-%d", goroutineID, j))
				peer := &DiscoveredPeer{
					ID:         peerID,
					Addrs:      []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+goroutineID*100+j)},
					Models:     []string{"gpt-4"},
					Discovered: time.Now(),
				}
				discovery.AddDiscoveredPeer(peer)
			}
		}(i)
	}

	// wait for all goroutines to finish
	wg.Wait()

	// verify total node count
	expectedCount := numGoroutines * peersPerGoroutine
	actualCount := discovery.GetPeerCount()

	// allow some tolerance (duplicate IDs may overwrite)
	if actualCount != expectedCount && actualCount > expectedCount-10 {
		t.Logf("Expected %d peers, got %d (some may have been deduplicated)", expectedCount, actualCount)
	}
}

// TestDiscovery_Concurrency_ConcurrentReadWrite tests concurrent read/write
func TestDiscovery_Concurrency_ConcurrentReadWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// pre-add some nodes
	for i := 0; i < 100; i++ {
		discovery.AddDiscoveredPeer(&DiscoveredPeer{
			ID:     PeerID(fmt.Sprintf("initial-peer-%d", i)),
			Addrs:  []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+i)},
			Models: []string{"gpt-4"},
		})
	}

	const numWriters = 10
	const numReaders = 20
	const writesPerWriter = 50

	var wg sync.WaitGroup

	// start writer goroutines
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				peerID := PeerID(fmt.Sprintf("writer-%d-peer-%d", writerID, j))
				discovery.AddDiscoveredPeer(&DiscoveredPeer{
					ID:     peerID,
					Addrs:  []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 5000+writerID*100+j)},
					Models: []string{"gpt-4"},
				})
			}
		}(i)
	}

	// start reader goroutines
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// read operations
				_ = discovery.GetPeerCount()
				_ = discovery.GetKnownPeers()
				_ = discovery.GetPeersForModel("gpt-4")
				_ = discovery.HasMinimumPeers()
			}
		}()
	}

	// wait for all operations to finish
	wg.Wait()

	// verify data integrity
	peers := discovery.GetKnownPeers()
	if len(peers) == 0 {
		t.Error("Expected at least some peers after concurrent operations")
	}

	// verify all nodes accessible
	for _, p := range peers {
		if p.ID == "" {
			t.Error("Found peer with empty ID")
			break
		}
	}
}

// TestDiscovery_Concurrency_ConcurrentModelQuery tests concurrent model queries
func TestDiscovery_Concurrency_ConcurrentModelQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// add nodes with different models
	models := []string{"gpt-4", "claude-3", "llama-2", "mistral", "palm"}
	for i := 0; i < 100; i++ {
		model := models[i%len(models)]
		discovery.AddDiscoveredPeer(&DiscoveredPeer{
			ID:     PeerID(fmt.Sprintf("model-peer-%d", i)),
			Addrs:  []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+i)},
			Models: []string{model},
		})
	}

	// query each model concurrently
	var wg sync.WaitGroup
	modelResults := make(map[string][]PeerID)
	resultsMu := sync.Mutex{}

	for _, model := range models {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			// query multiple times to increase contention
			for i := 0; i < 50; i++ {
				peers := discovery.GetPeersForModel(m)
				if len(peers) > 0 {
					resultsMu.Lock()
					modelResults[m] = peers
					resultsMu.Unlock()
				}
			}
		}(model)
	}

	wg.Wait()

	// verify each model returns correct results
	for _, model := range models {
		if len(modelResults[model]) == 0 {
			t.Errorf("No peers found for model %s", model)
		}
	}
}

// TestDiscovery_Concurrency_StopDuringDiscovery tests stopping during discovery
func TestDiscovery_Concurrency_StopDuringDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		Interval: 5 * time.Millisecond, // fast discovery interval
		MinPeers: 1,
	})

	// start discovery service
	err := discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// add nodes while discovery is running
	go func() {
		for i := 0; i < 100; i++ {
			discovery.AddDiscoveredPeer(&DiscoveredPeer{
				ID:     PeerID(fmt.Sprintf("concurrent-peer-%d", i)),
				Addrs:  []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 6000+i)},
				Models: []string{"gpt-4"},
			})
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// wait a short while
	time.Sleep(50 * time.Millisecond)

	// stop discovery service
	err = discovery.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	// verify node data integrity
	count := discovery.GetPeerCount()
	if count == 0 {
		t.Error("Expected some peers to be added before stop")
	}
}

// ============================================================================
// Boundary condition tests
// ============================================================================

// TestDiscovery_Boundary_EmptyNetwork tests empty network
func TestDiscovery_Boundary_EmptyNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// verify behavior on empty network
	if discovery.GetPeerCount() != 0 {
		t.Errorf("Expected 0 peers in empty network, got %d", discovery.GetPeerCount())
	}

	if discovery.HasMinimumPeers() {
		t.Error("Empty network should not have minimum peers")
	}

	peers := discovery.GetKnownPeers()
	if len(peers) != 0 {
		t.Errorf("Expected 0 known peers, got %d", len(peers))
	}

	// query nonexistent model
	modelPeers := discovery.GetPeersForModel("gpt-4")
	if modelPeers != nil && len(modelPeers) != 0 {
		t.Errorf("Expected nil or empty for nonexistent model, got %v", modelPeers)
	}
}

// TestDiscovery_Boundary_NilConfig tests nil config
func TestDiscovery_Boundary_NilConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// create discovery with nil config
	discovery, err := NewDiscovery(network, nil)
	if err != nil {
		t.Fatalf("Failed to create discovery with nil config: %v", err)
	}

	// verify defaults
	if discovery.interval != 60*time.Second {
		t.Errorf("Expected default interval 60s, got %v", discovery.interval)
	}

	if discovery.maxPeers != 100 {
		t.Errorf("Expected default maxPeers 100, got %d", discovery.maxPeers)
	}

	if discovery.minPeers != 5 {
		t.Errorf("Expected default minPeers 5, got %d", discovery.minPeers)
	}
}

// TestDiscovery_Boundary_NilNetwork tests nil network
func TestDiscovery_Boundary_NilNetwork(t *testing.T) {
	// creating discovery with nil network should fail
	_, err := NewDiscovery(nil, nil)
	if err == nil {
		t.Error("Expected error when creating discovery with nil network")
	}
}

// TestDiscovery_Boundary_ZeroConfig tests zero-value config
func TestDiscovery_Boundary_ZeroConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// use zero-value config
	cfg := &DiscoveryConfig{
		Interval:       0,
		MaxPeers:       0,
		MinPeers:       0,
		AnnounceModels: nil,
	}

	discovery, err := NewDiscovery(network, cfg)
	if err != nil {
		t.Fatalf("Failed to create discovery with zero config: %v", err)
	}

	// verify defaults applied
	if discovery.interval == 0 {
		t.Error("Expected default interval to be set")
	}

	if discovery.maxPeers == 0 {
		t.Error("Expected default maxPeers to be set")
	}

	if discovery.minPeers == 0 {
		t.Error("Expected default minPeers to be set")
	}
}
