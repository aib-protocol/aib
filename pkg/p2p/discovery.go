package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ServiceNamespace is the namespace for service discovery.
const ServiceNamespace = "aib-agentic"

// Discovery provides peer discovery using a simple gossip-based protocol.
type Discovery struct {
	mu      sync.RWMutex
	network *Network

	// Discovery state
	knownPeers map[PeerID]*DiscoveredPeer
	services   map[string][]PeerID

	// Configuration
	interval       time.Duration
	maxPeers       int
	minPeers       int
	announceModels []string
	stopCh         chan struct{}
}

// DiscoveredPeer contains information about a discovered peer.
type DiscoveredPeer struct {
	ID            PeerID
	Addrs         []string
	Models        []string
	Discovered    time.Time
	LastPing      time.Time
	Latency       time.Duration
	PingSuccesses int
	PingFailures  int
}

// DiscoveryConfig holds discovery configuration.
type DiscoveryConfig struct {
	Interval       time.Duration
	MaxPeers       int
	MinPeers       int
	AnnounceModels []string
}

// NewDiscovery creates a new discovery instance.
func NewDiscovery(network *Network, cfg *DiscoveryConfig) (*Discovery, error) {
	if network == nil {
		return nil, fmt.Errorf("network is required")
	}

	if cfg == nil {
		cfg = &DiscoveryConfig{
			Interval: 60 * time.Second,
			MaxPeers: 100,
			MinPeers: 5,
		}
	}

	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 100
	}
	if cfg.MinPeers == 0 {
		cfg.MinPeers = 5
	}

	return &Discovery{
		network:        network,
		knownPeers:     make(map[PeerID]*DiscoveredPeer),
		services:       make(map[string][]PeerID),
		interval:       cfg.Interval,
		maxPeers:       cfg.MaxPeers,
		minPeers:       cfg.MinPeers,
		announceModels: cfg.AnnounceModels,
		stopCh:         make(chan struct{}),
	}, nil
}

// Start starts the discovery process.
func (d *Discovery) Start(ctx context.Context) error {
	go d.discoveryLoop(ctx)
	return nil
}

// Stop stops the discovery process.
func (d *Discovery) Stop() error {
	close(d.stopCh)
	return nil
}

// discoveryLoop runs periodic discovery.
func (d *Discovery) discoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	d.discoverOnce(ctx)

	for {
		select {
		case <-ticker.C:
			d.discoverOnce(ctx)
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// discoverOnce performs a single discovery round.
func (d *Discovery) discoverOnce(ctx context.Context) {
	d.announceServices(ctx)
	d.pingKnownPeers(ctx)
	d.cleanStalePeers()
}

// announceServices announces our services to the network.
func (d *Discovery) announceServices(ctx context.Context) {
	if d.network == nil {
		return
	}

	for _, model := range d.announceModels {
		key := fmt.Sprintf("%s/%s", ServiceNamespace, model)
		_ = d.network.Provide(ctx, []byte(key))
	}
}

// pingKnownPeers pings known peers to check availability.
func (d *Discovery) pingKnownPeers(ctx context.Context) {
	d.mu.RLock()
	peers := make([]*DiscoveredPeer, 0, len(d.knownPeers))
	for _, p := range d.knownPeers {
		peers = append(peers, p)
	}
	d.mu.RUnlock()

	for _, p := range peers {
		start := time.Now()

		msg := &Message{
			Type:      "ping",
			Timestamp: time.Now(),
		}

		err := d.network.SendMessage(ctx, p.ID, DiscoveryProtocol, msg)

		d.mu.Lock()
		if err == nil {
			p.LastPing = time.Now()
			p.Latency = time.Since(start)
			p.PingSuccesses++
		} else {
			p.PingFailures++
		}
		d.mu.Unlock()
	}
}

// cleanStalePeers removes peers that haven't been seen for too long.
func (d *Discovery) cleanStalePeers() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)

	for id, p := range d.knownPeers {
		if p.LastPing.Before(cutoff) && p.PingFailures > 3 {
			delete(d.knownPeers, id)
			for model, ids := range d.services {
				d.services[model] = removeID(ids, id)
			}
		}
	}
}

// AddDiscoveredPeer adds a discovered peer.
func (d *Discovery) AddDiscoveredPeer(p *DiscoveredPeer) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.knownPeers[p.ID] = p

	for _, model := range p.Models {
		d.services[model] = append(d.services[model], p.ID)
	}
}

// GetKnownPeers returns all known peers.
func (d *Discovery) GetKnownPeers() []*DiscoveredPeer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*DiscoveredPeer, 0, len(d.knownPeers))
	for _, p := range d.knownPeers {
		result = append(result, p)
	}

	return result
}

// GetPeersForModel returns peers that serve a specific model.
func (d *Discovery) GetPeersForModel(model string) []PeerID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids, ok := d.services[model]
	if !ok {
		return nil
	}

	result := make([]PeerID, len(ids))
	copy(result, ids)
	return result
}

// GetPeerCount returns the number of known peers.
func (d *Discovery) GetPeerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.knownPeers)
}

// HasMinimumPeers checks if we have the minimum number of peers.
func (d *Discovery) HasMinimumPeers() bool {
	return d.GetPeerCount() >= d.minPeers
}

// removeID removes a peer ID from the list.
func removeID(ids []PeerID, id PeerID) []PeerID {
	result := make([]PeerID, 0, len(ids))
	for _, existing := range ids {
		if existing != id {
			result = append(result, existing)
		}
	}
	return result
}
