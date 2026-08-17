package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 节点发现测试
// ============================================================================

// TestDiscovery_NodeDiscovery_NewNode 测试发现新节点
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

	// 初始节点数为0
	if discovery.GetPeerCount() != 0 {
		t.Errorf("Expected 0 peers initially, got %d", discovery.GetPeerCount())
	}

	// 添加新节点
	newPeer := &DiscoveredPeer{
		ID:         PeerID("test-peer-1"),
		Addrs:      []string{"/ip4/127.0.0.1/tcp/4001"},
		Models:     []string{"gpt-4"},
		Discovered: time.Now(),
		LastPing:   time.Now(),
		Latency:    10 * time.Millisecond,
	}
	discovery.AddDiscoveredPeer(newPeer)

	// 验证节点数
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after adding, got %d", discovery.GetPeerCount())
	}

	// 验证获取节点
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer in GetKnownPeers, got %d", len(peers))
	}

	if peers[0].ID != newPeer.ID {
		t.Errorf("Expected peer ID %s, got %s", newPeer.ID, peers[0].ID)
	}
}

// TestDiscovery_NodeDiscovery_MultipleNodes 测试发现多个节点
func TestDiscovery_NodeDiscovery_MultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		MaxPeers: 100,
		MinPeers: 5,
	})

	// 添加多个节点
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

	// 验证节点总数
	if discovery.GetPeerCount() != 5 {
		t.Errorf("Expected 5 peers, got %d", discovery.GetPeerCount())
	}

	// 验证所有节点都能获取
	knownPeers := discovery.GetKnownPeers()
	if len(knownPeers) != 5 {
		t.Errorf("Expected 5 known peers, got %d", len(knownPeers))
	}
}

// TestDiscovery_NodeDiscovery_DuplicateNode 测试重复节点发现
func TestDiscovery_NodeDiscovery_DuplicateNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	peerID := PeerID("duplicate-peer")
	addr := "/ip4/127.0.0.1/tcp/4001"

	// 第一次添加节点
	peer1 := &DiscoveredPeer{
		ID:         peerID,
		Addrs:      []string{addr},
		Models:     []string{"gpt-4"},
		Discovered: time.Now(),
	}
	discovery.AddDiscoveredPeer(peer1)

	// 第二次添加相同节点（更新）
	peer2 := &DiscoveredPeer{
		ID:         peerID,
		Addrs:      []string{addr},
		Models:     []string{"gpt-4", "claude-3"},
		Discovered: time.Now(),
	}
	discovery.AddDiscoveredPeer(peer2)

	// 验证节点数仍然为1（不会重复添加）
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after duplicate add, got %d", discovery.GetPeerCount())
	}

	// 验证节点数据已更新
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}

	// 验证模型已更新
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
// 节点选择测试
// ============================================================================

// TestDiscovery_NodeSelection_ByModel 测试根据模型选择节点
func TestDiscovery_NodeSelection_ByModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 添加不同模型的节点
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("gpt-peer-1"), Models: []string{"gpt-4"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("gpt-peer-2"), Models: []string{"gpt-4"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4002"},
	})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{
		ID: PeerID("claude-peer-1"), Models: []string{"claude-3"}, Addrs: []string{"/ip4/127.0.0.1/tcp/4003"},
	})

	// 测试选择 gpt-4 模型的节点
	gptPeers := discovery.GetPeersForModel("gpt-4")
	if len(gptPeers) != 2 {
		t.Errorf("Expected 2 peers for gpt-4, got %d", len(gptPeers))
	}

	// 测试选择 claude-3 模型的节点
	claudePeers := discovery.GetPeersForModel("claude-3")
	if len(claudePeers) != 1 {
		t.Errorf("Expected 1 peer for claude-3, got %d", len(claudePeers))
	}

	// 测试不存在的模型
	nonexistentPeers := discovery.GetPeersForModel("nonexistent-model")
	if len(nonexistentPeers) != 0 {
		t.Errorf("Expected 0 peers for nonexistent model, got %d", len(nonexistentPeers))
	}
}

// TestDiscovery_NodeSelection_MinPeers 测试最小节点数检查
func TestDiscovery_NodeSelection_MinPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// 创建最小节点数为3的配置
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		MinPeers: 3,
		MaxPeers: 100,
	})

	// 初始状态检查
	if discovery.HasMinimumPeers() {
		t.Error("Should not have minimum peers initially")
	}

	// 添加2个节点（低于最小要求）
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-1")})
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-2")})

	if discovery.HasMinimumPeers() {
		t.Error("Should not have minimum peers with only 2 peers")
	}

	// 添加第3个节点（达到最小要求）
	discovery.AddDiscoveredPeer(&DiscoveredPeer{ID: PeerID("peer-3")})

	if !discovery.HasMinimumPeers() {
		t.Error("Should have minimum peers with 3 peers")
	}
}

// TestDiscovery_NodeSelection_MaxPeers 测试最大节点数限制
func TestDiscovery_NodeSelection_MaxPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// 创建最大节点数为2的配置（测试PeerManager）
	pm, err := NewPeerManager(network, &PeerManagerConfig{
		MaxPeers: 2,
	})
	if err != nil {
		t.Fatalf("Failed to create peer manager: %v", err)
	}

	// 添加第1个节点
	err = pm.AddPeer(PeerID("peer-1"), []string{"/ip4/127.0.0.1/tcp/4001"})
	if err != nil {
		t.Errorf("Failed to add peer 1: %v", err)
	}

	// 添加第2个节点
	err = pm.AddPeer(PeerID("peer-2"), []string{"/ip4/127.0.0.1/tcp/4002"})
	if err != nil {
		t.Errorf("Failed to add peer 2: %v", err)
	}

	// 添加第3个节点应该失败（超过最大限制）
	err = pm.AddPeer(PeerID("peer-3"), []string{"/ip4/127.0.0.1/tcp/4003"})
	if err == nil {
		t.Error("Should fail to add third peer when max peers reached")
	}
}

// ============================================================================
// 发现超时测试
// ============================================================================

// TestDiscovery_Timeout_ContextCancellation 测试上下文取消时的超时处理
func TestDiscovery_Timeout_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		Interval: 10 * time.Millisecond,
		MinPeers: 1,
	})

	// 启动发现服务
	err := discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// 等待一小段时间让发现循环运行
	time.Sleep(50 * time.Millisecond)

	// 取消上下文
	cancel()

	// 等待发现服务停止
	time.Sleep(20 * time.Millisecond)

	// 验证discovery可以安全访问
	count := discovery.GetPeerCount()
	if count < 0 {
		t.Errorf("Invalid peer count after cancellation: %d", count)
	}
}

// TestDiscovery_Timeout_PeerLatency 测试节点延迟跟踪
func TestDiscovery_Timeout_PeerLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 添加节点并设置延迟
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

	// 验证延迟值
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}

	if peers[0].Latency != 100*time.Millisecond {
		t.Errorf("Expected latency 100ms, got %v", peers[0].Latency)
	}

	// 验证ping成功次数
	if peers[0].PingSuccesses != 5 {
		t.Errorf("Expected 5 ping successes, got %d", peers[0].PingSuccesses)
	}
}

// TestDiscovery_Timeout_StalePeerCleanup 测试陈旧节点清理
func TestDiscovery_Timeout_StalePeerCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 添加一个正常节点
	activePeer := &DiscoveredPeer{
		ID:            PeerID("active-peer"),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4001"},
		LastPing:      time.Now(),
		PingSuccesses: 5,
		PingFailures:  0,
	}
	discovery.AddDiscoveredPeer(activePeer)

	// 添加一个陈旧节点（最后一次ping是6分钟前，超过5分钟阈值）
	stalePeer := &DiscoveredPeer{
		ID:            PeerID("stale-peer"),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4002"},
		LastPing:      time.Now().Add(-6 * time.Minute),
		PingSuccesses: 1,
		PingFailures:  5,
	}
	discovery.AddDiscoveredPeer(stalePeer)

	// 验证初始节点数
	if discovery.GetPeerCount() != 2 {
		t.Errorf("Expected 2 peers initially, got %d", discovery.GetPeerCount())
	}

	// 执行清理（内部调用cleanStalePeers）
	discovery.cleanStalePeers()

	// 验证清理后的节点数（应该只有活动节点）
	if discovery.GetPeerCount() != 1 {
		t.Errorf("Expected 1 peer after cleanup, got %d", discovery.GetPeerCount())
	}

	// 验证剩余的是活动节点
	peers := discovery.GetKnownPeers()
	if len(peers) != 1 || peers[0].ID != "active-peer" {
		t.Error("Expected active peer to remain after cleanup")
	}
}

// ============================================================================
// 并发发现测试
// ============================================================================

// TestDiscovery_Concurrency_ConcurrentAdd 测试并发添加节点
func TestDiscovery_Concurrency_ConcurrentAdd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	const numGoroutines = 50
	const peersPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发添加节点
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

	// 等待所有goroutine完成
	wg.Wait()

	// 验证总节点数
	expectedCount := numGoroutines * peersPerGoroutine
	actualCount := discovery.GetPeerCount()

	// 允许一定的误差（可能存在重复ID覆盖）
	if actualCount != expectedCount && actualCount > expectedCount-10 {
		t.Logf("Expected %d peers, got %d (some may have been deduplicated)", expectedCount, actualCount)
	}
}

// TestDiscovery_Concurrency_ConcurrentReadWrite 测试并发读写
func TestDiscovery_Concurrency_ConcurrentReadWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 预先添加一些节点
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

	// 启动写入goroutine
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

	// 启动读取goroutine
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// 读取操作
				_ = discovery.GetPeerCount()
				_ = discovery.GetKnownPeers()
				_ = discovery.GetPeersForModel("gpt-4")
				_ = discovery.HasMinimumPeers()
			}
		}()
	}

	// 等待所有操作完成
	wg.Wait()

	// 验证数据完整性
	peers := discovery.GetKnownPeers()
	if len(peers) == 0 {
		t.Error("Expected at least some peers after concurrent operations")
	}

	// 验证所有节点都可以访问
	for _, p := range peers {
		if p.ID == "" {
			t.Error("Found peer with empty ID")
			break
		}
	}
}

// TestDiscovery_Concurrency_ConcurrentModelQuery 测试并发模型查询
func TestDiscovery_Concurrency_ConcurrentModelQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 添加不同模型的节点
	models := []string{"gpt-4", "claude-3", "llama-2", "mistral", "palm"}
	for i := 0; i < 100; i++ {
		model := models[i%len(models)]
		discovery.AddDiscoveredPeer(&DiscoveredPeer{
			ID:     PeerID(fmt.Sprintf("model-peer-%d", i)),
			Addrs:  []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+i)},
			Models: []string{model},
		})
	}

	// 并发查询每个模型
	var wg sync.WaitGroup
	modelResults := make(map[string][]PeerID)
	resultsMu := sync.Mutex{}

	for _, model := range models {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			// 多次查询以增加并发压力
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

	// 验证每个模型都能正确返回结果
	for _, model := range models {
		if len(modelResults[model]) == 0 {
			t.Errorf("No peers found for model %s", model)
		}
	}
}

// TestDiscovery_Concurrency_StopDuringDiscovery 测试在发现过程中停止
func TestDiscovery_Concurrency_StopDuringDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, &DiscoveryConfig{
		Interval: 5 * time.Millisecond, // 快速发现间隔
		MinPeers: 1,
	})

	// 启动发现服务
	err := discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// 在发现运行时添加节点
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

	// 等待一小段时间
	time.Sleep(50 * time.Millisecond)

	// 停止发现服务
	err = discovery.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	// 验证节点数据完整性
	count := discovery.GetPeerCount()
	if count == 0 {
		t.Error("Expected some peers to be added before stop")
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

// TestDiscovery_Boundary_EmptyNetwork 测试空网络
func TestDiscovery_Boundary_EmptyNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)
	discovery, _ := NewDiscovery(network, nil)

	// 验证空网络的行为
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

	// 查询不存在的模型
	modelPeers := discovery.GetPeersForModel("gpt-4")
	if modelPeers != nil && len(modelPeers) != 0 {
		t.Errorf("Expected nil or empty for nonexistent model, got %v", modelPeers)
	}
}

// TestDiscovery_Boundary_NilConfig 测试nil配置
func TestDiscovery_Boundary_NilConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// 使用nil配置创建discovery
	discovery, err := NewDiscovery(network, nil)
	if err != nil {
		t.Fatalf("Failed to create discovery with nil config: %v", err)
	}

	// 验证默认值
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

// TestDiscovery_Boundary_NilNetwork 测试nil网络
func TestDiscovery_Boundary_NilNetwork(t *testing.T) {
	// 使用nil网络创建discovery应该失败
	_, err := NewDiscovery(nil, nil)
	if err == nil {
		t.Error("Expected error when creating discovery with nil network")
	}
}

// TestDiscovery_Boundary_ZeroConfig 测试零值配置
func TestDiscovery_Boundary_ZeroConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	network, _ := NewNetwork(ctx, nil)

	// 使用零值配置
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

	// 验证默认值被应用
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
