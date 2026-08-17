// Package p2p provides P2P networking using Go standard library.
// This is a pure standard library implementation without libp2p dependencies.
package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Type Aliases (replacing libp2p types)
// ============================================================================

// PeerID is a unique identifier for a peer (replaces peer.ID).
// It is the base58 or hex encoded public key hash.
type PeerID string

// ProtocolID identifies a protocol (replaces protocol.ID).
type ProtocolID string

// Multiaddr represents a multi-address (simplified version).
// Format: /ip4/<ip>/tcp/<port> or /ip6/<ip>/tcp/<port>
type Multiaddr string

// AddrInfo holds peer address information (replaces peer.AddrInfo).
type AddrInfo struct {
	ID    PeerID
	Addrs []Multiaddr
}

// ============================================================================
// Network Types
// ============================================================================

// Conn represents a connection to a peer.
type Conn struct {
	net.Conn
	LocalPeer   PeerID
	RemotePeer  PeerID
	Established time.Time
}

// Stream represents a bidirectional stream over a connection.
type Stream struct {
	id       string
	conn     *Conn
	protocol ProtocolID
	reader   *streamReader
	writer   *streamWriter
	closed   bool
	mu       sync.Mutex
}

// Note: Network interface is defined in network.go to avoid circular import

// TCPNetwork is a TCP-based network implementation using standard library.
type TCPNetwork struct {
	listeners []net.Listener
	addrs     []Multiaddr
	mu        sync.RWMutex
	closed    bool
}

// NewTCPNetwork creates a new TCP network.
func NewTCPNetwork() *TCPNetwork {
	return &TCPNetwork{
		listeners: make([]net.Listener, 0),
		addrs:     make([]Multiaddr, 0),
	}
}

// Listen implements Network.Listen.
func (n *TCPNetwork) Listen(addr Multiaddr) error {
	host, port, err := decodeTCPAddr(addr)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", addr, err)
	}

	listenAddr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		ln.Close()
		return errors.New("network is closed")
	}

	n.listeners = append(n.listeners, ln)

	// Get actual address (in case port was 0)
	actualAddr := ln.Addr().(*net.TCPAddr)
	actualMultiaddr := encodeTCPAddr(actualAddr.IP.String(), actualAddr.Port)
	n.addrs = append(n.addrs, actualMultiaddr)

	return nil
}

// Dial implements Network.Dial.
func (n *TCPNetwork) Dial(ctx context.Context, addr Multiaddr) (*Conn, error) {
	host, port, err := decodeTCPAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", addr, err)
	}

	dialAddr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{}
	netConn, err := dialer.DialContext(ctx, "tcp", dialAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", dialAddr, err)
	}

	return &Conn{
		Conn:        netConn,
		Established: time.Now(),
	}, nil
}

// Close implements Network.Close.
func (n *TCPNetwork) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}

	n.closed = true

	var lastErr error
	for _, ln := range n.listeners {
		if err := ln.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Addrs implements Network.Addrs.
func (n *TCPNetwork) Addrs() []Multiaddr {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]Multiaddr, len(n.addrs))
	copy(result, n.addrs)
	return result
}

// ============================================================================
// Address Utilities
// ============================================================================

// encodeTCPAddr encodes an IP and port into a multiaddr format.
func encodeTCPAddr(ip string, port int) Multiaddr {
	// Check if it's IPv6
	if strings.Contains(ip, ":") {
		return Multiaddr(fmt.Sprintf("/ip6/%s/tcp/%d", ip, port))
	}
	return Multiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip, port))
}

// decodeTCPAddr decodes a multiaddr into host and port.
func decodeTCPAddr(addr Multiaddr) (host string, port int, err error) {
	s := string(addr)

	// Handle /ip4/<ip>/tcp/<port> format
	if strings.HasPrefix(s, "/ip4/") {
		parts := strings.Split(s, "/")
		if len(parts) >= 5 && parts[3] == "tcp" {
			host = parts[2]
			port, err = strconv.Atoi(parts[4])
			return
		}
	}

	// Handle /ip6/<ip>/tcp/<port> format
	if strings.HasPrefix(s, "/ip6/") {
		parts := strings.Split(s, "/")
		if len(parts) >= 5 && parts[3] == "tcp" {
			host = parts[2]
			port, err = strconv.Atoi(parts[4])
			return
		}
	}

	// Try to parse as host:port
	host, portStr, err := net.SplitHostPort(s)
	if err == nil {
		port, err = strconv.Atoi(portStr)
		return
	}

	return "", 0, fmt.Errorf("invalid address format: %s", s)
}

// peerIDFromPublicKey creates a PeerID from a public key.
func peerIDFromPublicKey(pubKey ed25519.PublicKey) PeerID {
	// Use the first 16 bytes of the public key hash as the peer ID
	if len(pubKey) == 0 {
		return ""
	}
	return PeerID(hex.EncodeToString(pubKey[:16]))
}

// ============================================================================
// Stream Implementation
// ============================================================================

// streamReader wraps a net.Conn for reading.
type streamReader struct {
	conn   net.Conn
	closed bool
	mu     sync.Mutex
}

func (r *streamReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return 0, errors.New("stream closed")
	}
	r.mu.Unlock()
	return r.conn.Read(p)
}

func (r *streamReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// streamWriter wraps a net.Conn for writing.
type streamWriter struct {
	conn   net.Conn
	closed bool
	mu     sync.Mutex
}

func (w *streamWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, errors.New("stream closed")
	}
	w.mu.Unlock()
	return w.conn.Write(p)
}

func (w *streamWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// ============================================================================
// Common Errors
// ============================================================================

var (
	// ErrNetworkClosed is returned when the network is closed.
	ErrNetworkClosed = errors.New("network is closed")

	// ErrPeerNotFound is returned when a peer is not found.
	ErrPeerNotFound = errors.New("peer not found")

	// ErrProtocolNotSupported is returned when a protocol is not supported.
	ErrProtocolNotSupported = errors.New("protocol not supported")

	// ErrStreamClosed is returned when a stream is closed.
	ErrStreamClosed = errors.New("stream is closed")

	// ErrInvalidAddress is returned when an address is invalid.
	ErrInvalidAddress = errors.New("invalid address")
)

// ============================================================================
// Helper Functions
// ============================================================================

// GeneratePeerID generates a random peer ID.
func GeneratePeerID() (PeerID, ed25519.PrivateKey, ed25519.PublicKey, error) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate key: %w", err)
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	peerID := peerIDFromPublicKey(pubKey)

	return peerID, privKey, pubKey, nil
}

// String returns the string representation of a PeerID.
func (id PeerID) String() string {
	return string(id)
}

// String returns the string representation of a ProtocolID.
func (id ProtocolID) String() string {
	return string(id)
}

// String returns the string representation of a Multiaddr.
func (m Multiaddr) String() string {
	return string(m)
}
