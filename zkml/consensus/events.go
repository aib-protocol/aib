package consensus

import (
	"time"
)

// BlockEvent represents an event that triggers block creation
type BlockEvent struct {
	TaskID          string            `json:"task_id"`           // ZKML task ID
	FinalResult    string            `json:"final_result"`    // Final verification result
	IsValid        bool              `json:"is_valid"`        // Verification success
	AgreementRate  float64           `json:"agreement_rate"`  // Node consensus rate
	NodeResults    map[string]string `json:"node_results"`    // Node results
	ConsensusNodes []string          `json:"consensus_nodes"`  // Nodes in consensus
	DisagreeingNodes []string        `json:"disagreeing_nodes"` // Disagreeing nodes
	Metadata       map[string]string `json:"metadata"`        // Additional metadata
	Timestamp      int64             `json:"timestamp"`        // Event timestamp
	BlockHeight    uint64            `json:"block_height"`    // Expected block height
}

// NewBlockEvent creates a new block event
func NewBlockEvent(
	taskID string,
	finalResult string,
	isValid bool,
	agreementRate float64,
	nodeResults map[string]string,
	consensusNodes []string,
	disagreeingNodes []string,
	metadata map[string]string,
) *BlockEvent {
	return &BlockEvent{
		TaskID:          taskID,
		FinalResult:    finalResult,
		IsValid:        isValid,
		AgreementRate:  agreementRate,
		NodeResults:    nodeResults,
		ConsensusNodes: consensusNodes,
		DisagreeingNodes: disagreeingNodes,
		Metadata:       metadata,
		Timestamp:      time.Now().Unix(),
	}
}

// TaskVerifiedHandler is a function type that handles verified tasks
type TaskVerifiedHandler func(event *BlockEvent)

// EventChannel returns a channel for receiving block events
type EventChannel chan *BlockEvent

// NewEventChannel creates a new event channel
func NewEventChannel(bufferSize int) EventChannel {
	if bufferSize <= 0 {
		bufferSize = 10
	}
	return make(chan *BlockEvent, bufferSize)
}

// EventType represents the type of consensus event
type EventType string

const (
	EventTaskVerified    EventType = "task_verified"
	EventBlockProduced   EventType = "block_produced"
	EventBlockVerified   EventType = "block_verified"
	EventChainReorganized EventType = "chain_reorganized"
)

// ConsensusEvent represents a consensus-related event
type ConsensusEvent struct {
	Type      EventType     `json:"type"`       // Event type
	BlockHash []byte        `json:"block_hash"` // Related block hash
	Height    uint64        `json:"height"`     // Block height
	Data      interface{}   `json:"data"`       // Event data
	Timestamp time.Time     `json:"timestamp"`  // Event timestamp
}

// NewConsensusEvent creates a new consensus event
func NewConsensusEvent(eventType EventType, blockHash []byte, height uint64, data interface{}) *ConsensusEvent {
	return &ConsensusEvent{
		Type:      eventType,
		BlockHash: blockHash,
		Height:    height,
		Data:      data,
		Timestamp: time.Now(),
	}
}
