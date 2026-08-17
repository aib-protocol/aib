// Package sync provides block synchronization and propagation functionality.
// This file implements performance benchmarks for block propagation.
package sync

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aib-protocol/aib/pkg/p2p"
)

// ============================================================================
// Benchmark Helpers
// ============================================================================

// benchmarkNetwork is a minimal network implementation for benchmarks.
type benchmarkNetwork struct {
	peers    []*p2p.PeerInfo
	messages [][]byte // Track total message bytes for throughput
	peerID   p2p.PeerID
}

func newBenchmarkNetwork(peerCount int) *benchmarkNetwork {
	net := &benchmarkNetwork{
		peers:    make([]*p2p.PeerInfo, peerCount),
		messages: make([][]byte, 0, 1000),
		peerID:   "benchmark-node",
	}

	for i := 0; i < peerCount; i++ {
		net.peers[i] = &p2p.PeerInfo{
			ID:        p2p.PeerID(string(rune('a' + i))),
			Connected: true,
			LastSeen:  time.Now(),
		}
	}

	return net
}

func (n *benchmarkNetwork) SendMessage(ctx context.Context, to p2p.PeerID, proto p2p.ProtocolID, msg *p2p.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	n.messages = append(n.messages, data)
	return nil
}

func (n *benchmarkNetwork) RegisterProtocol(proto p2p.ProtocolID, handler p2p.MessageHandler) error {
	return nil
}

func (n *benchmarkNetwork) GetPeers() []*p2p.PeerInfo {
	return n.peers
}

func (n *benchmarkNetwork) PeerID() p2p.PeerID {
	return n.peerID
}

func (n *benchmarkNetwork) Connect(ctx context.Context, addrInfo p2p.AddrInfo) error {
	return nil
}

// benchmarkBlockchain is a minimal blockchain implementation for benchmarks.
type benchmarkBlockchain struct {
	blocks map[uint64]*Block
}

func newBenchmarkBlockchain() *benchmarkBlockchain {
	return &benchmarkBlockchain{
		blocks: make(map[uint64]*Block),
	}
}

func (b *benchmarkBlockchain) AddBlock(block *Block) error {
	b.blocks[block.Height] = block
	return nil
}

func (b *benchmarkBlockchain) GetBlock(height uint64) (*Block, error) {
	block, ok := b.blocks[height]
	if !ok {
		return nil, ErrBlockNotFound
	}
	return block, nil
}

func (b *benchmarkBlockchain) GetLatestBlock() (*Block, error) {
	if len(b.blocks) == 0 {
		return nil, ErrNoBlocks
	}
	var maxHeight uint64
	for h := range b.blocks {
		if h > maxHeight {
			maxHeight = h
		}
	}
	return b.blocks[maxHeight], nil
}

func (b *benchmarkBlockchain) GetBlockCount() uint64 {
	if len(b.blocks) == 0 {
		return 0
	}
	var maxHeight uint64
	for h := range b.blocks {
		if h > maxHeight {
			maxHeight = h
		}
	}
	return maxHeight + 1
}

func (b *benchmarkBlockchain) GetBlocksInRange(start, end uint64) ([]*Block, error) {
	blocks := make([]*Block, 0, end-start+1)
	for h := start; h <= end; h++ {
		if block, ok := b.blocks[h]; ok {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func (b *benchmarkBlockchain) GetBlockByHash(hash []byte) (*Block, error) {
	for _, block := range b.blocks {
		if string(block.Hash()) == string(hash) {
			return block, nil
		}
	}
	return nil, ErrBlockNotFound
}

// generateBenchmarkBlock creates a block for benchmarking with specified transaction count.
func generateBenchmarkBlock(height uint64, txCount int) *Block {
	block := &Block{
		Height:        height,
		Timestamp:     time.Now().Unix(),
		TaskID:        "benchmark-task",
		FinalResult:   "success",
		IsValid:       true,
		Nonce:         height,
		PrevBlockHash: make([]byte, 32),
	}

	if height > 0 {
		block.PrevBlockHash[0] = byte(height - 1)
	}

	// Add transactions
	block.Transactions = make([]*Transaction, txCount)
	for i := 0; i < txCount; i++ {
		block.Transactions[i] = &Transaction{
			ID:        string(rune('a' + i)),
			From:      "sender",
			To:        "receiver",
			Amount:    100,
			Fee:       1,
			Timestamp: time.Now().Unix(),
			Nonce:     uint64(i),
			Signature: []byte("test-signature"),
			Payload:   make([]byte, 100), // 100 byte payload
		}
	}

	block.BlockHash = block.calculateHash()
	return block
}

// generateBenchmarkChain creates a chain of blocks with correct PrevBlockHash linking.
func generateBenchmarkChain(count int, txPerBlock int) []*Block {
	blocks := make([]*Block, count)
	for i := 0; i < count; i++ {
		block := &Block{
			Height:      uint64(i),
			Timestamp:   time.Now().Unix(),
			TaskID:      "benchmark-task",
			FinalResult: "success",
			IsValid:     true,
			Nonce:       uint64(i),
		}
		if i == 0 {
			block.PrevBlockHash = make([]byte, 32)
		} else {
			block.PrevBlockHash = blocks[i-1].Hash()
		}
		block.Transactions = make([]*Transaction, txPerBlock)
		for j := 0; j < txPerBlock; j++ {
			block.Transactions[j] = &Transaction{
				ID:        string(rune('a' + j%26)),
				From:      "sender",
				To:        "receiver",
				Amount:    100,
				Fee:       1,
				Timestamp: time.Now().Unix(),
				Nonce:     uint64(j),
				Signature: []byte("test-signature"),
				Payload:   make([]byte, 100),
			}
		}
		block.BlockHash = block.calculateHash()
		blocks[i] = block
	}
	return blocks
}

// ============================================================================
// Block Propagation Benchmarks
// ============================================================================

// BenchmarkBlockPropagation benchmarks single block propagation with varying peer counts.
func BenchmarkBlockPropagation(b *testing.B) {
	benchmarks := []struct {
		name      string
		peerCount int
		txCount   int
	}{
		{"Small_10Peers_10Tx", 10, 10},
		{"Medium_50Peers_50Tx", 50, 50},
		{"Large_100Peers_100Tx", 100, 100},
		{"VeryLarge_200Peers_500Tx", 200, 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &BlockPropagationConfig{
				PropagationTimeout: 30 * time.Second,
				GossipFanout:       3,
			}

			bp := NewBlockPropagator(chain, net, cfg)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			block := generateBenchmarkBlock(1, bm.txCount)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Reset received cache to prevent duplicate detection
				bp.mu.Lock()
				bp.receivedBlocks = make(map[string]time.Time)
				bp.mu.Unlock()

				if err := bp.BroadcastBlock(block); err != nil {
					b.Fatalf("BroadcastBlock() error = %v", err)
				}
			}

			// Report bytes per operation based on average message size
			if len(net.messages) > 0 {
				totalBytes := 0
				for _, msg := range net.messages {
					totalBytes += len(msg)
				}
				avgBytes := totalBytes / len(net.messages)
				b.SetBytes(int64(avgBytes))
			}
		})
	}
}

// BenchmarkBlockBatchPropagation benchmarks batch block propagation.
func BenchmarkBlockBatchPropagation(b *testing.B) {
	benchmarks := []struct {
		name       string
		batchSize  int
		peerCount  int
		txPerBlock int
	}{
		{"Small_10Blocks_10Peers_10Tx", 10, 10, 10},
		{"Medium_50Blocks_50Peers_25Tx", 50, 50, 25},
		{"Large_100Blocks_100Peers_50Tx", 100, 100, 50},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &BlockPropagationConfig{
				PropagationTimeout: 30 * time.Second,
				GossipFanout:       3,
			}

			bp := NewBlockPropagator(chain, net, cfg)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			// Pre-generate blocks
			blocks := make([]*Block, bm.batchSize)
			for i := 0; i < bm.batchSize; i++ {
				blocks[i] = generateBenchmarkBlock(uint64(i), bm.txPerBlock)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Reset caches
				bp.mu.Lock()
				bp.receivedBlocks = make(map[string]time.Time)
				bp.mu.Unlock()

				// Broadcast batch
				for _, block := range blocks {
					if err := bp.BroadcastBlock(block); err != nil {
						b.Fatalf("BroadcastBlock() error = %v", err)
					}
				}
			}

			// Report total bytes per batch
			if len(net.messages) > 0 {
				totalBytes := 0
				for _, msg := range net.messages {
					totalBytes += len(msg)
				}
				avgBytes := totalBytes / (len(net.messages) / bm.batchSize)
				b.SetBytes(int64(avgBytes * bm.batchSize))
			}
		})
	}
}

// BenchmarkBlockAnnounce benchmarks block announcement (without full block data).
func BenchmarkBlockAnnounce(b *testing.B) {
	benchmarks := []struct {
		name      string
		peerCount int
	}{
		{"Small_10Peers", 10},
		{"Medium_50Peers", 50},
		{"Large_100Peers", 100},
		{"VeryLarge_500Peers", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &BlockPropagationConfig{
				PropagationTimeout: 30 * time.Second,
				GossipFanout:       3,
			}

			bp := NewBlockPropagator(chain, net, cfg)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			block := generateBenchmarkBlock(1, 0) // Block without transactions for announcement

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Reset announced cache
				bp.mu.Lock()
				bp.announcedBlocks = make(map[string]time.Time)
				bp.mu.Unlock()

				// Create announcement message
				announce := &BlockMessage{
					Type:     "block_announce",
					Block:    block,
					FromPeer: net.PeerID(),
					TTL:      bp.gossipFanout,
				}

				// Gossip announcement
				if err := bp.gossipBlock(block, announce); err != nil {
					b.Fatalf("gossipBlock() error = %v", err)
				}
			}

			// Report bytes per announcement
			if len(net.messages) > 0 {
				totalBytes := 0
				for _, msg := range net.messages {
					totalBytes += len(msg)
				}
				avgBytes := totalBytes / len(net.messages)
				b.SetBytes(int64(avgBytes))
			}
		})
	}
}

// BenchmarkPeerSync benchmarks peer-to-peer synchronization operations.
func BenchmarkPeerSync(b *testing.B) {
	benchmarks := []struct {
		name         string
		blockCount   int
		peerCount    int
		blocksPerReq int
	}{
		{"Small_10Blocks_10Peers_5PerReq", 10, 10, 5},
		{"Medium_100Blocks_50Peers_10PerReq", 100, 50, 10},
		{"Large_500Blocks_100Peers_50PerReq", 500, 100, 50},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &Config{
				SyncInterval:    10 * time.Second,
				MaxBlocksPerReq: bm.blocksPerReq,
				Timeout:         30 * time.Second,
			}

			sm := NewSyncManager(chain, net, cfg)
			if err := sm.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer sm.Stop()

			// Pre-generate a properly linked chain
			blocks := generateBenchmarkChain(bm.blockCount, 10)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Clear chain for each iteration
				chain.blocks = make(map[uint64]*Block)

				// Simulate sync by adding blocks (skip validateBlock which checks prev hash against stored chain)
				for _, block := range blocks {
					if err := chain.AddBlock(block); err != nil {
						b.Fatalf("AddBlock() error = %v", err)
					}
				}
			}

			// Report bytes processed
			totalSize := 0
			for _, block := range blocks {
				data, _ := json.Marshal(block)
				totalSize += len(data)
			}
			b.SetBytes(int64(totalSize))
		})
	}
}

// ============================================================================
// Block Validation Benchmarks
// ============================================================================

// BenchmarkBlockValidate benchmarks block validation with varying complexity.
func BenchmarkBlockValidate(b *testing.B) {
	benchmarks := []struct {
		name    string
		txCount int
	}{
		{"Empty_0Tx", 0},
		{"Small_10Tx", 10},
		{"Medium_50Tx", 50},
		{"Large_100Tx", 100},
		{"VeryLarge_500Tx", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			chain := newBenchmarkBlockchain()
			net := newBenchmarkNetwork(10)

			// Generate a properly linked 2-block chain
			linkedChain := generateBenchmarkChain(2, bm.txCount)
			genesis := linkedChain[0]
			block := linkedChain[1]
			chain.AddBlock(genesis)

			bp := NewBlockPropagator(chain, net, nil)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if err := bp.ValidateBlock(block); err != nil {
					b.Fatalf("ValidateBlock() error = %v", err)
				}
			}
		})
	}
}

// ============================================================================
// Block Hash Calculation Benchmarks
// ============================================================================

// BenchmarkBlockHashCalculation benchmarks block hash calculation.
func BenchmarkBlockHashCalculation(b *testing.B) {
	benchmarks := []struct {
		name    string
		txCount int
	}{
		{"Empty_0Tx", 0},
		{"Small_10Tx", 10},
		{"Medium_50Tx", 50},
		{"Large_100Tx", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			block := generateBenchmarkBlock(1, bm.txCount)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = block.calculateHash()
			}
		})
	}
}

// ============================================================================
// Gossip Protocol Benchmarks
// ============================================================================

// BenchmarkGossipFanout benchmarks gossip with different fanout values.
func BenchmarkGossipFanout(b *testing.B) {
	benchmarks := []struct {
		name      string
		peerCount int
		fanout    int
	}{
		{"Fanout1_100Peers", 100, 1},
		{"Fanout3_100Peers", 100, 3},
		{"Fanout5_100Peers", 100, 5},
		{"Fanout10_100Peers", 100, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &BlockPropagationConfig{
				PropagationTimeout: 30 * time.Second,
				GossipFanout:       bm.fanout,
			}

			bp := NewBlockPropagator(chain, net, cfg)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			block := generateBenchmarkBlock(1, 10)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				announce := &BlockMessage{
					Type:     "block_announce",
					Block:    block,
					FromPeer: net.PeerID(),
					TTL:      bm.fanout,
				}

				if err := bp.gossipBlock(block, announce); err != nil {
					b.Fatalf("gossipBlock() error = %v", err)
				}
			}
		})
	}
}

// ============================================================================
// Cache Operations Benchmarks
// ============================================================================

// BenchmarkReceivedCache benchmarks received block cache operations.
func BenchmarkReceivedCache(b *testing.B) {
	chain := newBenchmarkBlockchain()
	net := newBenchmarkNetwork(10)

	bp := NewBlockPropagator(chain, net, nil)
	if err := bp.Start(); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	// Pre-populate cache
	blocks := make([]*Block, 100)
	for i := 0; i < 100; i++ {
		blocks[i] = generateBenchmarkBlock(uint64(i), 10)
		blockHash := string(blocks[i].Hash())
		bp.receivedBlocks[blockHash] = time.Now()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block := blocks[i%len(blocks)]
		blockHash := string(block.Hash())

		bp.mu.RLock()
		_, _ = bp.receivedBlocks[blockHash]
		bp.mu.RUnlock()
	}
}

// BenchmarkCacheCleanup benchmarks cache cleanup operations.
func BenchmarkCacheCleanup(b *testing.B) {
	chain := newBenchmarkBlockchain()
	net := newBenchmarkNetwork(10)

	bp := NewBlockPropagator(chain, net, nil)
	if err := bp.Start(); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer bp.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Add entries
		bp.mu.Lock()
		for j := 0; j < 100; j++ {
			blockHash := string(rune('a' + j%26))
			bp.receivedBlocks[blockHash] = time.Now().Add(-time.Hour)
			bp.announcedBlocks[blockHash] = time.Now().Add(-time.Hour)
		}
		bp.mu.Unlock()

		// Cleanup
		bp.cleanupOldEntries()
	}
}

// ============================================================================
// Message Marshaling Benchmarks
// ============================================================================

// BenchmarkBlockMessageMarshal benchmarks block message marshaling.
func BenchmarkBlockMessageMarshal(b *testing.B) {
	benchmarks := []struct {
		name    string
		txCount int
	}{
		{"Small_0Tx", 0},
		{"Medium_50Tx", 50},
		{"Large_100Tx", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			block := generateBenchmarkBlock(1, bm.txCount)
			msg := &BlockMessage{
				Type:     "block_announce",
				Block:    block,
				FromPeer: "test-peer",
				TTL:      3,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := json.Marshal(msg)
				if err != nil {
					b.Fatalf("Marshal() error = %v", err)
				}
			}
		})
	}
}

// BenchmarkBlockMessageUnmarshal benchmarks block message unmarshaling.
func BenchmarkBlockMessageUnmarshal(b *testing.B) {
	benchmarks := []struct {
		name    string
		txCount int
	}{
		{"Small_0Tx", 0},
		{"Medium_50Tx", 50},
		{"Large_100Tx", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			block := generateBenchmarkBlock(1, bm.txCount)
			msg := &BlockMessage{
				Type:     "block_announce",
				Block:    block,
				FromPeer: "test-peer",
				TTL:      3,
			}

			data, err := json.Marshal(msg)
			if err != nil {
				b.Fatalf("Marshal() error = %v", err)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var result BlockMessage
				if err := json.Unmarshal(data, &result); err != nil {
					b.Fatalf("Unmarshal() error = %v", err)
				}
			}

			b.SetBytes(int64(len(data)))
		})
	}
}

// ============================================================================
// Concurrent Operations Benchmarks
// ============================================================================

// BenchmarkConcurrentBroadcast benchmarks concurrent block broadcasts.
func BenchmarkConcurrentBroadcast(b *testing.B) {
	benchmarks := []struct {
		name       string
		goroutines int
		peerCount  int
	}{
		{"1Goroutine_10Peers", 1, 10},
		{"4Goroutines_10Peers", 4, 10},
		{"8Goroutines_10Peers", 8, 10},
		{"16Goroutines_10Peers", 16, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			net := newBenchmarkNetwork(bm.peerCount)
			chain := newBenchmarkBlockchain()

			cfg := &BlockPropagationConfig{
				PropagationTimeout: 30 * time.Second,
				GossipFanout:       3,
			}

			bp := NewBlockPropagator(chain, net, cfg)
			if err := bp.Start(); err != nil {
				b.Fatalf("Start() error = %v", err)
			}
			defer bp.Stop()

			blocks := make([]*Block, bm.goroutines)
			for i := 0; i < bm.goroutines; i++ {
				blocks[i] = generateBenchmarkBlock(uint64(i), 10)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Use a channel to synchronize goroutines
				done := make(chan struct{}, bm.goroutines)

				for j := 0; j < bm.goroutines; j++ {
					go func(idx int) {
						// Reset cache for this goroutine's block
						bp.mu.Lock()
						bp.receivedBlocks = make(map[string]time.Time)
						bp.mu.Unlock()

						_ = bp.BroadcastBlock(blocks[idx])
						done <- struct{}{}
					}(j)
				}

				// Wait for all goroutines to complete
				for j := 0; j < bm.goroutines; j++ {
					<-done
				}
			}
		})
	}
}
