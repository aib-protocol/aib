package agentic

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

func TestNodeStatus_String(t *testing.T) {
	tests := []struct {
		status   NodeStatus
		expected string
	}{
		{NodeStatusActive, "active"},
		{NodeStatusInactive, "inactive"},
		{NodeStatusSuspended, "suspended"},
		{NodeStatusSlashed, "slashed"},
		{NodeStatusUnknown, "unknown"},
	}

	for _, test := range tests {
		result := test.status.String()
		if result != test.expected {
			t.Errorf("NodeStatus.String() = %s, expected %s", result, test.expected)
		}
	}
}

func TestServiceType_String(t *testing.T) {
	tests := []struct {
		serviceType ServiceType
		expected    string
	}{
		{ServiceTypeChat, "chat"},
		{ServiceTypeCompletion, "completion"},
		{ServiceTypeEmbedding, "embedding"},
		{ServiceTypeImage, "image"},
		{ServiceTypeUnknown, "unknown"},
	}

	for _, test := range tests {
		result := test.serviceType.String()
		if result != test.expected {
			t.Errorf("ServiceType.String() = %s, expected %s", result, test.expected)
		}
	}
}

func TestSlashReason_String(t *testing.T) {
	tests := []struct {
		reason   SlashReason
		expected string
	}{
		{SlashReasonDowntime, "downtime"},
		{SlashReasonInvalidResponse, "invalid_response"},
		{SlashReasonTimeout, "timeout"},
		{SlashReasonMalicious, "malicious"},
		{SlashReasonConsensusViolation, "consensus_violation"},
		{SlashReasonUnknown, "unknown"},
	}

	for _, test := range tests {
		result := test.reason.String()
		if result != test.expected {
			t.Errorf("SlashReason.String() = %s, expected %s", result, test.expected)
		}
	}
}

func TestGetSlashReasonByCode(t *testing.T) {
	tests := []struct {
		code     string
		expected SlashReason
	}{
		{"downtime", SlashReasonDowntime},
		{"invalid_response", SlashReasonInvalidResponse},
		{"timeout", SlashReasonTimeout},
		{"malicious", SlashReasonMalicious},
		{"consensus_violation", SlashReasonConsensusViolation},
		{"unknown", SlashReasonUnknown},
		{"invalid", SlashReasonUnknown},
	}

	for _, test := range tests {
		result := GetSlashReasonByCode(test.code)
		if result != test.expected {
			t.Errorf("GetSlashReasonByCode(%s) = %v, expected %v", test.code, result, test.expected)
		}
	}
}

func TestDiscoveryMessage_SignVerify(t *testing.T) {
	// Generate Ed25519 keys
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	msg := &DiscoveryMessage{
		NodeID:    interfaces.NodeID{1, 2, 3},
		PublicKey: pubKey,
		Endpoint:  "http://localhost:8080",
		Models:    []string{"gpt-4", "claude-3"},
		Timestamp: time.Now(),
	}

	// Sign the message
	if err := msg.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	// Verify the signature
	if !msg.Verify() {
		t.Error("Signature verification failed")
	}

	// Modify the message and verify fails
	msg.Endpoint = "http://modified:8080"
	if msg.Verify() {
		t.Error("Signature should have failed for modified message")
	}
}

func TestNodeFilter_Matches(t *testing.T) {
	node := &NodeInfo{
		ID:         interfaces.NodeID{1, 2, 3},
		Stake:      1000,
		Reputation: 0.9,
		Models:     []string{"gpt-4", "claude-3"},
		Services:   []ServiceType{ServiceTypeChat, ServiceTypeCompletion},
	}

	tests := []struct {
		name     string
		filter   *NodeFilter
		expected bool
	}{
		{
			name:     "empty filter matches",
			filter:   &NodeFilter{},
			expected: true,
		},
		{
			name:     "min reputation passes",
			filter:   &NodeFilter{MinReputation: 0.8},
			expected: true,
		},
		{
			name:     "min reputation fails",
			filter:   &NodeFilter{MinReputation: 0.95},
			expected: false,
		},
		{
			name:     "min stake passes",
			filter:   &NodeFilter{MinStake: 500},
			expected: true,
		},
		{
			name:     "min stake fails",
			filter:   &NodeFilter{MinStake: 2000},
			expected: false,
		},
		{
			name:     "model match",
			filter:   &NodeFilter{Models: []string{"gpt-4"}},
			expected: true,
		},
		{
			name:     "model no match",
			filter:   &NodeFilter{Models: []string{"llama-2"}},
			expected: false,
		},
		{
			name:     "service match",
			filter:   &NodeFilter{Services: []ServiceType{ServiceTypeChat}},
			expected: true,
		},
		{
			name:     "service no match",
			filter:   &NodeFilter{Services: []ServiceType{ServiceTypeImage}},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.filter.Matches(node)
			if result != test.expected {
				t.Errorf("NodeFilter.Matches() = %v, expected %v", result, test.expected)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Valid config
	cfg := &Config{
		PrivateKey: privKey,
		PublicKey:  privKey.Public().(ed25519.PublicKey),
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}

	// Invalid private key
	cfg = &Config{
		PrivateKey: nil,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Invalid private key should fail validation")
	}

	// Invalid public key
	cfg = &Config{
		PrivateKey: privKey,
		PublicKey:  nil,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Invalid public key should fail validation")
	}
}

func TestNewServiceContext(t *testing.T) {
	ctx := context.Background()
	nodeID := interfaces.NodeID{1, 2, 3}
	requestID := "req-123"

	svcCtx := NewServiceContext(ctx, nodeID, requestID)

	if svcCtx.NodeID != nodeID {
		t.Errorf("NodeID = %v, expected %v", svcCtx.NodeID, nodeID)
	}

	if svcCtx.RequestID != requestID {
		t.Errorf("RequestID = %s, expected %s", svcCtx.RequestID, requestID)
	}

	if svcCtx.Deadline.IsZero() {
		t.Error("Deadline should not be zero")
	}
}
