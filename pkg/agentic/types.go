// Package agentic provides AI service layer with standard API compatibility.
package agentic

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// NodeStatus represents the current status of an AI node.
type NodeStatus int

const (
	NodeStatusUnknown NodeStatus = iota
	NodeStatusActive
	NodeStatusInactive
	NodeStatusSuspended
	NodeStatusSlashed
)

func (s NodeStatus) String() string {
	switch s {
	case NodeStatusActive:
		return "active"
	case NodeStatusInactive:
		return "inactive"
	case NodeStatusSuspended:
		return "suspended"
	case NodeStatusSlashed:
		return "slashed"
	default:
		return "unknown"
	}
}

// ServiceType represents the type of AI service offered.
type ServiceType int

const (
	ServiceTypeUnknown ServiceType = iota
	ServiceTypeChat
	ServiceTypeCompletion
	ServiceTypeEmbedding
	ServiceTypeImage
)

func (t ServiceType) String() string {
	switch t {
	case ServiceTypeChat:
		return "chat"
	case ServiceTypeCompletion:
		return "completion"
	case ServiceTypeEmbedding:
		return "embedding"
	case ServiceTypeImage:
		return "image"
	default:
		return "unknown"
	}
}

// NodeInfo contains detailed information about an AI service node.
type NodeInfo struct {
	ID          interfaces.NodeID
	Address     interfaces.Address
	PublicKey   ed25519.PublicKey
	Stake       uint64
	Models      []string
	Reputation  float64
	Status      NodeStatus
	Services    []ServiceType
	Endpoint    string
	LastSeen    time.Time
	JoinedAt    time.Time
	Version     string
	Metadata    map[string]string
}

// StakeInfo contains staking information for a node.
type StakeInfo struct {
	NodeID         interfaces.NodeID
	Amount         uint64
	LockedUntil    time.Time
	SlashCount     uint32
	TotalSlashed   uint64
	LastSlashTime  *time.Time
}

// SlashReason represents the reason for slashing a node.
type SlashReason int

const (
	SlashReasonUnknown SlashReason = iota
	SlashReasonDowntime
	SlashReasonInvalidResponse
	SlashReasonTimeout
	SlashReasonMalicious
	SlashReasonConsensusViolation
)

func (r SlashReason) String() string {
	switch r {
	case SlashReasonDowntime:
		return "downtime"
	case SlashReasonInvalidResponse:
		return "invalid_response"
	case SlashReasonTimeout:
		return "timeout"
	case SlashReasonMalicious:
		return "malicious"
	case SlashReasonConsensusViolation:
		return "consensus_violation"
	default:
		return "unknown"
	}
}

// SlashRecord records a slashing event.
type SlashRecord struct {
	NodeID      interfaces.NodeID
	Amount      uint64
	Reason      SlashReason
	Evidence    []byte
	Timestamp   time.Time
	BlockHeight uint64
}

// ServiceRequest represents a request for AI service.
type ServiceRequest struct {
	RequestID   string
	NodeID      interfaces.NodeID
	RequestType ServiceType
	Model       string
	Payload     []byte
	MaxCost     uint64
	Deadline    time.Time
}

// ServiceResponse represents a response from AI service.
type ServiceResponse struct {
	RequestID   string
	NodeID      interfaces.NodeID
	Response    []byte
	Cost        uint64
	Latency     time.Duration
	Timestamp   time.Time
	Signature   []byte
}

// ServiceProvider represents a provider of AI services.
type ServiceProvider struct {
	NodeInfo    *NodeInfo
	Pricing     map[string]uint64 // model -> price per token
	Capacity    int
	Load        int
	SuccessRate float64
}

// DiscoveryMessage is used for node discovery.
type DiscoveryMessage struct {
	NodeID    interfaces.NodeID
	Address   interfaces.Address
	PublicKey ed25519.PublicKey
	Endpoint  string
	Models    []string
	Services  []ServiceType
	Version   string
	Timestamp time.Time
	Signature []byte
}

// Marshal serializes the discovery message.
func (m *DiscoveryMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal deserializes the discovery message.
func (m *DiscoveryMessage) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// Verify verifies the signature of the discovery message.
func (m *DiscoveryMessage) Verify() bool {
	if m.PublicKey == nil || m.Signature == nil {
		return false
	}

	// Create a copy without signature for verification
	msgCopy := *m
	msgCopy.Signature = nil

	data, err := msgCopy.Marshal()
	if err != nil {
		return false
	}

	return ed25519.Verify(m.PublicKey, data, m.Signature)
}

// Sign signs the discovery message with the given private key.
func (m *DiscoveryMessage) Sign(privKey ed25519.PrivateKey) error {
	m.Signature = nil
	data, err := m.Marshal()
	if err != nil {
		return err
	}

	m.Signature = ed25519.Sign(privKey, data)
	return nil
}

// NodeFilter is used to filter nodes during discovery.
type NodeFilter struct {
	MinReputation float64
	MinStake      uint64
	Models        []string
	Services      []ServiceType
	MaxLatency    time.Duration
}

// Matches checks if a node matches the filter.
func (f *NodeFilter) Matches(node *NodeInfo) bool {
	if node.Reputation < f.MinReputation {
		return false
	}
	if node.Stake < f.MinStake {
		return false
	}
	if f.MaxLatency > 0 {
		// Would check actual latency here
	}

	// Check models
	if len(f.Models) > 0 {
		modelMap := make(map[string]bool)
		for _, m := range node.Models {
			modelMap[m] = true
		}
		for _, required := range f.Models {
			if !modelMap[required] {
				return false
			}
		}
	}

	// Check services
	if len(f.Services) > 0 {
		serviceMap := make(map[ServiceType]bool)
		for _, s := range node.Services {
			serviceMap[s] = true
		}
		for _, required := range f.Services {
			if !serviceMap[required] {
				return false
			}
		}
	}

	return true
}

// Config holds configuration for the agentic service.
type Config struct {
	NodeID          interfaces.NodeID
	PrivateKey      ed25519.PrivateKey
	PublicKey       ed25519.PublicKey
	Address         interfaces.Address
	MinStake        uint64
	SlashThreshold  uint64
	ReputationDecay float64
	MaxNodes        int
	ServiceTimeout  time.Duration
	DiscoveryInterval time.Duration
	HeartbeatInterval time.Duration
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.PrivateKey == nil || len(c.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key")
	}
	if c.PublicKey == nil || len(c.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key")
	}
	if c.ServiceTimeout == 0 {
		c.ServiceTimeout = 30 * time.Second
	}
	if c.DiscoveryInterval == 0 {
		c.DiscoveryInterval = 60 * time.Second
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 30 * time.Second
	}
	return nil
}

// ServiceContext provides context for service operations.
type ServiceContext struct {
	context.Context
	NodeID    interfaces.NodeID
	RequestID string
	Deadline  time.Time
}

// NewServiceContext creates a new service context.
func NewServiceContext(ctx context.Context, nodeID interfaces.NodeID, requestID string) *ServiceContext {
	return &ServiceContext{
		Context:   ctx,
		NodeID:    nodeID,
		RequestID: requestID,
		Deadline:  time.Now().Add(30 * time.Second),
	}
}

// ErrNodeNotFound is returned when a node is not found.
var ErrNodeNotFound = fmt.Errorf("node not found")

// ErrInsufficientStake is returned when stake is insufficient.
var ErrInsufficientStake = fmt.Errorf("insufficient stake")

// ErrServiceUnavailable is returned when service is unavailable.
var ErrServiceUnavailable = fmt.Errorf("service unavailable")

// ErrInvalidResponse is returned when response is invalid.
var ErrInvalidResponse = fmt.Errorf("invalid response")

// ErrTimeout is returned when operation times out.
var ErrTimeout = fmt.Errorf("operation timeout")
