package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PeerManager manages connected peers.
type PeerManager struct {
	mu       sync.RWMutex
	network  *Network
	peers    map[PeerID]*ManagedPeer
	byStatus map[PeerStatus][]PeerID

	// Configuration
	maxPeers        int
	peerTTL         time.Duration
	keepAlivePeriod time.Duration

	// Event handlers
	onPeerConnected    func(PeerID)
	onPeerDisconnected func(PeerID)
	onPeerUpdated      func(PeerID)

	ctx    context.Context
	cancel context.CancelFunc
}

// PeerStatus represents the status of a peer connection.
type PeerStatus int

const (
	PeerStatusUnknown PeerStatus = iota
	PeerStatusConnecting
	PeerStatusConnected
	PeerStatusDisconnected
	PeerStatusError
)

func (s PeerStatus) String() string {
	switch s {
	case PeerStatusConnecting:
		return "connecting"
	case PeerStatusConnected:
		return "connected"
	case PeerStatusDisconnected:
		return "disconnected"
	case PeerStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// ManagedPeer contains information about a managed peer.
type ManagedPeer struct {
	ID              PeerID
	Status          PeerStatus
	ConnectedAt     time.Time
	LastSeen        time.Time
	LastPing        time.Time
	Latency         time.Duration
	FailedPings     int
	SuccessfulPings int
	Metadata        map[string]string
	Streams         map[ProtocolID]*Conn
}

// PeerManagerConfig holds peer manager configuration.
type PeerManagerConfig struct {
	MaxPeers        int
	PeerTTL         time.Duration
	KeepAlivePeriod time.Duration
}

// NewPeerManager creates a new peer manager.
func NewPeerManager(network *Network, cfg *PeerManagerConfig) (*PeerManager, error) {
	if network == nil {
		return nil, fmt.Errorf("network is required")
	}

	if cfg == nil {
		cfg = &PeerManagerConfig{
			MaxPeers:        100,
			PeerTTL:         10 * time.Minute,
			KeepAlivePeriod: 30 * time.Second,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	pm := &PeerManager{
		network:         network,
		peers:           make(map[PeerID]*ManagedPeer),
		byStatus:        make(map[PeerStatus][]PeerID),
		maxPeers:        cfg.MaxPeers,
		peerTTL:         cfg.PeerTTL,
		keepAlivePeriod: cfg.KeepAlivePeriod,
		ctx:             ctx,
		cancel:          cancel,
	}

	return pm, nil
}

// Start starts the peer manager.
func (pm *PeerManager) Start() error {
	go pm.maintenanceLoop()
	return nil
}

// Stop stops the peer manager.
func (pm *PeerManager) Stop() error {
	pm.cancel()
	return nil
}

// maintenanceLoop performs periodic maintenance.
func (pm *PeerManager) maintenanceLoop() {
	ticker := time.NewTicker(pm.keepAlivePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.checkPeerHealth()
			pm.cleanupStalePeers()
		case <-pm.ctx.Done():
			return
		}
	}
}

// AddPeer adds a peer to management.
func (pm *PeerManager) AddPeer(p PeerID, addrs []string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.peers[p]; exists {
		return nil
	}

	if len(pm.peers) >= pm.maxPeers {
		return fmt.Errorf("max peers reached: %d", pm.maxPeers)
	}

	// Create AddrInfo from addrs
	multiaddrs := make([]Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		multiaddrs = append(multiaddrs, Multiaddr(addr))
	}

	addrInfo := &AddrInfo{
		ID:    p,
		Addrs: multiaddrs,
	}

	// Add peer info to network
	pm.network.AddPeer(&PeerInfo{
		ID:       p,
		AddrInfo: addrInfo,
	})

	mp := &ManagedPeer{
		ID:       p,
		Status:   PeerStatusUnknown,
		LastSeen: time.Now(),
		Metadata: make(map[string]string),
		Streams:  make(map[ProtocolID]*Conn),
	}

	pm.peers[p] = mp
	pm.updatePeerStatus(p, PeerStatusUnknown)

	go pm.connectPeer(p, addrInfo)

	return nil
}

// connectPeer connects to a peer.
func (pm *PeerManager) connectPeer(p PeerID, addrInfo *AddrInfo) {
	pm.mu.Lock()
	pm.updatePeerStatus(p, PeerStatusConnecting)
	pm.mu.Unlock()

	ctx, cancel := context.WithTimeout(pm.ctx, 30*time.Second)
	defer cancel()

	err := pm.network.Connect(ctx, *addrInfo)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err != nil {
		pm.updatePeerStatus(p, PeerStatusError)
		return
	}

	pm.updatePeerStatus(p, PeerStatusConnected)

	mp := pm.peers[p]
	mp.ConnectedAt = time.Now()
	mp.LastSeen = time.Now()

	if pm.onPeerConnected != nil {
		go pm.onPeerConnected(p)
	}
}

// DisconnectPeer disconnects from a peer.
func (pm *PeerManager) DisconnectPeer(p PeerID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	mp, exists := pm.peers[p]
	if !exists {
		return fmt.Errorf("peer not found: %s", p)
	}

	for _, conn := range mp.Streams {
		conn.Close()
	}

	pm.network.Disconnect(pm.ctx, p)

	pm.updatePeerStatus(p, PeerStatusDisconnected)

	if pm.onPeerDisconnected != nil {
		go pm.onPeerDisconnected(p)
	}

	return nil
}

// RemovePeer removes a peer from management.
func (pm *PeerManager) RemovePeer(p PeerID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	mp, exists := pm.peers[p]
	if !exists {
		return fmt.Errorf("peer not found: %s", p)
	}

	for _, conn := range mp.Streams {
		conn.Close()
	}

	pm.network.RemovePeer(p)

	for status, ids := range pm.byStatus {
		pm.byStatus[status] = removePeerID(ids, p)
	}

	delete(pm.peers, p)

	if pm.onPeerDisconnected != nil {
		go pm.onPeerDisconnected(p)
	}

	return nil
}

// GetPeer returns a managed peer.
func (pm *PeerManager) GetPeer(p PeerID) (*ManagedPeer, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	mp, exists := pm.peers[p]
	if !exists {
		return nil, fmt.Errorf("peer not found: %s", p)
	}

	return mp, nil
}

// GetAllPeers returns all managed peers.
func (pm *PeerManager) GetAllPeers() []*ManagedPeer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*ManagedPeer, 0, len(pm.peers))
	for _, mp := range pm.peers {
		result = append(result, mp)
	}

	return result
}

// GetPeersByStatus returns peers with a specific status.
func (pm *PeerManager) GetPeersByStatus(status PeerStatus) []*ManagedPeer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	ids, ok := pm.byStatus[status]
	if !ok {
		return nil
	}

	result := make([]*ManagedPeer, 0, len(ids))
	for _, id := range ids {
		if mp, ok := pm.peers[id]; ok {
			result = append(result, mp)
		}
	}

	return result
}

// updatePeerStatus updates a peer's status.
func (pm *PeerManager) updatePeerStatus(p PeerID, status PeerStatus) {
	mp, exists := pm.peers[p]
	if !exists {
		return
	}

	if oldStatus := mp.Status; oldStatus != status {
		pm.byStatus[oldStatus] = removePeerID(pm.byStatus[oldStatus], p)
	}

	mp.Status = status

	pm.byStatus[status] = append(pm.byStatus[status], p)
}

// checkPeerHealth checks the health of all connected peers.
func (pm *PeerManager) checkPeerHealth() {
	pm.mu.RLock()
	peers := make([]*ManagedPeer, 0, len(pm.peers))
	for _, mp := range pm.peers {
		if mp.Status == PeerStatusConnected {
			peers = append(peers, mp)
		}
	}
	pm.mu.RUnlock()

	for _, mp := range peers {
		pm.pingPeer(mp.ID)
	}
}

// pingPeer pings a peer to check connectivity.
func (pm *PeerManager) pingPeer(p PeerID) {
	start := time.Now()

	// Check if peer is still connected
	msg := &Message{
		Type:    "ping",
		Payload: []byte("health-check"),
	}

	err := pm.network.SendMessage(pm.ctx, p, DiscoveryProtocol, msg)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	mp, exists := pm.peers[p]
	if !exists {
		return
	}

	mp.LastPing = time.Now()

	if err != nil {
		mp.Latency = 0
		mp.FailedPings++
		if mp.FailedPings > 3 {
			pm.updatePeerStatus(p, PeerStatusDisconnected)
		}
	} else {
		mp.Latency = time.Since(start)
		mp.SuccessfulPings++
		mp.FailedPings = 0
		mp.LastSeen = time.Now()
	}

	if pm.onPeerUpdated != nil {
		go pm.onPeerUpdated(p)
	}
}

// cleanupStalePeers removes stale peers.
func (pm *PeerManager) cleanupStalePeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	cutoff := time.Now().Add(-pm.peerTTL)

	for id, mp := range pm.peers {
		if mp.LastSeen.Before(cutoff) && mp.Status == PeerStatusDisconnected {
			for status, ids := range pm.byStatus {
				pm.byStatus[status] = removePeerID(ids, id)
			}
			delete(pm.peers, id)
		}
	}
}

// SetPeerConnectedHandler sets the handler for peer connected events.
func (pm *PeerManager) SetPeerConnectedHandler(handler func(PeerID)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onPeerConnected = handler
}

// SetPeerDisconnectedHandler sets the handler for peer disconnected events.
func (pm *PeerManager) SetPeerDisconnectedHandler(handler func(PeerID)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onPeerDisconnected = handler
}

// SetPeerUpdatedHandler sets the handler for peer updated events.
func (pm *PeerManager) SetPeerUpdatedHandler(handler func(PeerID)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onPeerUpdated = handler
}

// removePeerID removes a peer ID from a list.
func removePeerID(ids []PeerID, id PeerID) []PeerID {
	result := make([]PeerID, 0, len(ids))
	for _, existing := range ids {
		if existing != id {
			result = append(result, existing)
		}
	}
	return result
}
