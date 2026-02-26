// Package p2p provides P2P networking using Go standard library.
// This is a pure standard library implementation without libp2p dependencies.
package p2p

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/agentic"
)

// Protocol constants
const (
	// DefaultProtocolID is the default protocol identifier for agentic P2P communications.
	DefaultProtocolID ProtocolID = "/aib/agentic/1.0.0"

	// DiscoveryProtocol is the protocol for node discovery.
	DiscoveryProtocol ProtocolID = "/aib/discovery/1.0.0"

	// ServiceProtocol is the protocol for AI service requests.
	ServiceProtocol ProtocolID = "/aib/service/1.0.0"

	// AgenticProtocol is the protocol for agentic communications.
	AgenticProtocol ProtocolID = "/aib/agentic/1.0.0"
)

// Network provides P2P networking capabilities using standard library.
type Network struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Identity
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
	peerID  PeerID

	// Network layer
	tcpNet     *TCPNetwork
	listeners  []net.Listener
	wg         sync.WaitGroup

	// Peers and protocols
	mu           sync.RWMutex
	peers        map[PeerID]*PeerInfo
	connections  map[PeerID]*Conn
	protocols    map[ProtocolID]MessageHandler
	messageChans map[ProtocolID]chan *Message

	// State
	started bool
	cfg     *Config
}

// Config holds P2P network configuration.
type Config struct {
	ListenAddrs    []string
	BootstrapPeers []AddrInfo
	PrivKey        ed25519.PrivateKey
	PubKey         ed25519.PublicKey
	EnableDHT      bool
	MaxPeers       int
	MinPeers       int
}

// PeerInfo holds information about a connected peer.
type PeerInfo struct {
	ID        PeerID
	AddrInfo  *AddrInfo
	NodeInfo  *agentic.NodeInfo
	Connected bool
	LastSeen  time.Time
	Latency   time.Duration
}

// MessageHandler handles messages for a specific protocol.
type MessageHandler func(ctx context.Context, msg *Message, from PeerID) error

// Message represents a P2P message.
type Message struct {
	Type      string          `json:"type"`
	Payload   []byte          `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	Sender    PeerID          `json:"sender"`
	Protocol  ProtocolID      `json:"protocol,omitempty"`
}

// NewNetwork creates a new P2P network instance.
func NewNetwork(ctx context.Context, cfg *Config) (*Network, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	if len(cfg.ListenAddrs) == 0 {
		cfg.ListenAddrs = []string{
			"0.0.0.0:0",
		}
	}

	n := &Network{
		peers:        make(map[PeerID]*PeerInfo),
		connections:  make(map[PeerID]*Conn),
		protocols:    make(map[ProtocolID]MessageHandler),
		messageChans: make(map[ProtocolID]chan *Message),
		cfg:          cfg,
		tcpNet:       NewTCPNetwork(),
	}

	n.ctx, n.cancel = context.WithCancel(ctx)

	return n, nil
}

// Start starts the P2P network.
func (n *Network) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.started {
		return fmt.Errorf("network already started")
	}

	// Initialize keys
	if n.cfg.PrivKey != nil {
		n.privKey = n.cfg.PrivKey
		n.pubKey = n.cfg.PrivKey.Public().(ed25519.PublicKey)
	} else {
		_, privKey, pubKey, err := GeneratePeerID()
		if err != nil {
			return fmt.Errorf("failed to generate peer ID: %w", err)
		}
		n.privKey = privKey
		n.pubKey = pubKey
	}
	n.peerID = peerIDFromPublicKey(n.pubKey)

	// Start listening on configured addresses
	for _, addrStr := range n.cfg.ListenAddrs {
		addr := Multiaddr(addrStr)
		if err := n.tcpNet.Listen(addr); err != nil {
			return fmt.Errorf("failed to listen on %s: %w", addrStr, err)
		}
	}

	// Start accepting connections
	for _, ln := range n.tcpNet.listeners {
		n.wg.Add(1)
		go n.acceptLoop(ln)
	}

	n.registerDefaultProtocols()

	n.started = true
	return nil
}

// Stop stops the P2P network.
func (n *Network) Stop() error {
	n.mu.Lock()
	if !n.started {
		n.mu.Unlock()
		return fmt.Errorf("network not started")
	}
	n.started = false
	n.mu.Unlock()

	n.cancel()

	// Close all connections
	n.mu.Lock()
	for _, conn := range n.connections {
		conn.Close()
	}
	n.connections = make(map[PeerID]*Conn)
	n.mu.Unlock()

	// Close TCP network
	if err := n.tcpNet.Close(); err != nil {
		return fmt.Errorf("error closing network: %w", err)
	}

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished
	case <-time.After(5 * time.Second):
		// Timeout waiting for goroutines
	}

	return nil
}

// acceptLoop accepts incoming connections.
func (n *Network) acceptLoop(ln net.Listener) {
	defer n.wg.Done()

	// Try to cast to TCPListener for deadline support
	tcpLn, ok := ln.(*net.TCPListener)

	for {
		// Check context first
		select {
		case <-n.ctx.Done():
			return
		default:
		}

		var conn net.Conn
		var err error

		// Use Accept with deadline for TCP connections
		if ok && tcpLn != nil {
			// Set a short deadline so we can check context periodically
			tcpLn.SetDeadline(time.Now().Add(100 * time.Millisecond))
			conn, err = tcpLn.Accept()
		} else {
			conn, err = ln.Accept()
		}

		if err != nil {
			// Check if context was cancelled
			select {
			case <-n.ctx.Done():
				return
			default:
				// Timeout error from deadline, continue loop
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				// Other error (like listener closed), check context again
				select {
				case <-n.ctx.Done():
					return
				default:
					continue
				}
			}
		}

		n.wg.Add(1)
		go n.handleConnection(&Conn{
			Conn:        conn,
			Established: time.Now(),
		})
	}
}

// handleConnection handles an incoming connection.
func (n *Network) handleConnection(conn *Conn) {
	defer n.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		select {
		case <-n.ctx.Done():
			return
		default:
		}

		// Read message length (4 bytes)
		lenBuf := make([]byte, 4)
		if _, err := reader.Read(lenBuf); err != nil {
			return
		}

		msgLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		if msgLen > 10*1024*1024 { // Max 10MB
			return
		}

		// Read message data
		data := make([]byte, msgLen)
		if _, err := reader.Read(data); err != nil {
			return
		}

		// Parse message
		msg := &Message{}
		if err := json.Unmarshal(data, msg); err != nil {
			continue
		}

		// Set sender from connection info if not set
		if msg.Sender == "" {
			msg.Sender = conn.RemotePeer
		}

		// Handle message
		n.handleMessage(n.ctx, msg, conn.RemotePeer)
	}
}

// handleMessage routes a message to the appropriate handler.
func (n *Network) handleMessage(ctx context.Context, msg *Message, from PeerID) {
	n.mu.RLock()
	handler, ok := n.protocols[msg.Protocol]
	n.mu.RUnlock()

	if !ok {
		return
	}

	if err := handler(ctx, msg, from); err != nil {
		// Log error but don't stop processing
		_ = err
	}
}

// Host returns nil (for compatibility, no longer returns libp2p host).
func (n *Network) Host() interface{} {
	return nil
}

// PeerID returns the local peer ID.
func (n *Network) PeerID() PeerID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.peerID
}

// Connect connects to a peer.
func (n *Network) Connect(ctx context.Context, addrInfo AddrInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.started {
		return fmt.Errorf("network not started")
	}

	// Try to connect to any of the peer's addresses
	for _, addr := range addrInfo.Addrs {
		conn, err := n.tcpNet.Dial(ctx, addr)
		if err != nil {
			continue
		}

		conn.RemotePeer = addrInfo.ID
		n.connections[addrInfo.ID] = conn

		// Add to peers
		n.peers[addrInfo.ID] = &PeerInfo{
			ID:        addrInfo.ID,
			AddrInfo:  &addrInfo,
			Connected: true,
			LastSeen:  time.Now(),
		}

		// Start handling the connection
		n.wg.Add(1)
		go n.handleConnection(conn)

		return nil
	}

	return fmt.Errorf("failed to connect to peer %s", addrInfo.ID)
}

// Disconnect disconnects from a peer.
func (n *Network) Disconnect(ctx context.Context, p PeerID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.started {
		return fmt.Errorf("network not started")
	}

	conn, ok := n.connections[p]
	if !ok {
		return ErrPeerNotFound
	}

	delete(n.connections, p)
	if peer, ok := n.peers[p]; ok {
		peer.Connected = false
	}

	return conn.Close()
}

// SendMessage sends a message to a peer.
func (n *Network) SendMessage(ctx context.Context, to PeerID, proto ProtocolID, msg *Message) error {
	n.mu.RLock()
	conn, ok := n.connections[to]
	n.mu.RUnlock()

	if !ok {
		return ErrPeerNotFound
	}

	msg.Sender = n.peerID
	msg.Protocol = proto
	msg.Timestamp = time.Now()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write message length (4 bytes) followed by data
	lenBuf := make([]byte, 4)
	msgLen := len(data)
	lenBuf[0] = byte(msgLen >> 24)
	lenBuf[1] = byte(msgLen >> 16)
	lenBuf[2] = byte(msgLen >> 8)
	lenBuf[3] = byte(msgLen)

	if _, err := conn.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// RegisterProtocol registers a message handler for a protocol.
func (n *Network) RegisterProtocol(proto ProtocolID, handler MessageHandler) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.protocols[proto] = handler
	return nil
}

// GetPeers returns all known peers.
func (n *Network) GetPeers() []*PeerInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]*PeerInfo, 0, len(n.peers))
	for _, pi := range n.peers {
		result = append(result, pi)
	}

	return result
}

// AddPeer adds a peer to the network.
func (n *Network) AddPeer(info *PeerInfo) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.peers[info.ID] = info
}

// RemovePeer removes a peer from the network.
func (n *Network) RemovePeer(p PeerID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	delete(n.peers, p)
	if conn, ok := n.connections[p]; ok {
		conn.Close()
		delete(n.connections, p)
	}
}

// DiscoverPeers discovers peers (stub for full DHT implementation).
func (n *Network) DiscoverPeers(ctx context.Context) ([]AddrInfo, error) {
	return []AddrInfo{}, nil
}

// Provide announces that this node can provide a value.
func (n *Network) Provide(ctx context.Context, key []byte) error {
	return nil
}

// FindProviders finds providers for a key.
func (n *Network) FindProviders(ctx context.Context, key []byte) ([]AddrInfo, error) {
	return []AddrInfo{}, nil
}

// registerDefaultProtocols registers default protocols.
// Must be called while n.mu is already held (e.g. from Start()).
func (n *Network) registerDefaultProtocols() {
	n.protocols[DiscoveryProtocol] = n.handleDiscovery
	n.protocols[ServiceProtocol] = n.handleService
}

// handleDiscovery handles discovery messages.
func (n *Network) handleDiscovery(ctx context.Context, msg *Message, from PeerID) error {
	// Handle ping/pong for discovery
	switch msg.Type {
	case "ping":
		// Send pong response
		response := &Message{
			Type:    "pong",
			Payload: msg.Payload,
		}
		return n.SendMessage(ctx, from, DiscoveryProtocol, response)
	case "pong":
		// Update peer latency
		n.mu.Lock()
		if peer, ok := n.peers[from]; ok {
			peer.LastSeen = time.Now()
		}
		n.mu.Unlock()
	}
	return nil
}

// handleService handles service messages.
func (n *Network) handleService(ctx context.Context, msg *Message, from PeerID) error {
	// Service messages are handled by the agentic layer
	return nil
}

// CreateNetworkFromEd25519 creates a P2P network from Ed25519 keys.
func CreateNetworkFromEd25519(ctx context.Context, privKey ed25519.PrivateKey, listenAddrs []string) (*Network, error) {
	cfg := &Config{
		ListenAddrs: listenAddrs,
		PrivKey:     privKey,
		EnableDHT:   true,
	}

	return NewNetwork(ctx, cfg)
}

// MarshalJSON implements json.Marshaler.
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*m = Message(*aux.Alias)
	return nil
}

// Addrs returns the addresses the network is listening on.
func (n *Network) Addrs() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	addrs := n.tcpNet.Addrs()
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = string(addr)
	}
	return result
}
