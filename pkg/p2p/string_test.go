// Package p2p provides additional tests for uncovered functions.
package p2p

import (
	"testing"
)

func TestPeerStatus_String(t *testing.T) {
	tests := []struct {
		status   PeerStatus
		expected string
	}{
		{PeerStatusUnknown, "unknown"},
		{PeerStatusConnecting, "connecting"},
		{PeerStatusConnected, "connected"},
		{PeerStatusDisconnected, "disconnected"},
		{PeerStatusError, "error"},
		{PeerStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Errorf("PeerStatus(%d).String() = %s, want %s", tt.status, got, tt.expected)
			}
		})
	}
}

func TestPeerID_String(t *testing.T) {
	id := PeerID("test-peer-id-123")
	got := id.String()
	if got != "test-peer-id-123" {
		t.Errorf("PeerID.String() = %s, want test-peer-id-123", got)
	}

	// Test empty PeerID
	emptyID := PeerID("")
	if emptyID.String() != "" {
		t.Errorf("Empty PeerID.String() = %s, want empty", emptyID.String())
	}
}

func TestProtocolID_String(t *testing.T) {
	id := ProtocolID("/aib/discovery/1.0.0")
	got := id.String()
	if got != "/aib/discovery/1.0.0" {
		t.Errorf("ProtocolID.String() = %s, want /aib/discovery/1.0.0", got)
	}
}

func TestMultiaddr_String(t *testing.T) {
	addr := Multiaddr("/ip4/127.0.0.1/tcp/8080")
	got := addr.String()
	if got != "/ip4/127.0.0.1/tcp/8080" {
		t.Errorf("Multiaddr.String() = %s, want /ip4/127.0.0.1/tcp/8080", got)
	}
}
