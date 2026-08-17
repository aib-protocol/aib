package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Block represents a ZKML verification result block
type Block struct {
	Height           uint64            `json:"height"`            // Block height
	PrevBlockHash    []byte            `json:"prev_block_hash"`   // Previous block hash
	Timestamp        int64             `json:"timestamp"`         // Block creation timestamp
	TaskID           string            `json:"task_id"`           // ZKML task ID
	FinalResult      string            `json:"final_result"`      // Final verification result
	IsValid          bool              `json:"is_valid"`          // Verification success
	AgreementRate    float64           `json:"agreement_rate"`    // Node consensus rate
	NodeResults      map[string]string `json:"node_results"`      // Node results
	ConsensusNodes   []string          `json:"consensus_nodes"`   // Nodes in consensus
	DisagreeingNodes []string          `json:"disagreeing_nodes"` // Disagreeing nodes
	Metadata         map[string]string `json:"metadata"`          // Additional metadata
	BlockHash        []byte            `json:"hash"`              // Block hash
	Nonce            uint64            `json:"nonce"`             // Nonce for PoW
}

// NewBlock creates a new block from a verification event
func NewBlock(height uint64, prevHash []byte, event *BlockEvent) *Block {
	return &Block{
		Height:           height,
		PrevBlockHash:    prevHash,
		Timestamp:        time.Now().Unix(),
		TaskID:           event.TaskID,
		FinalResult:      event.FinalResult,
		IsValid:          event.IsValid,
		AgreementRate:    event.AgreementRate,
		NodeResults:      event.NodeResults,
		ConsensusNodes:   event.ConsensusNodes,
		DisagreeingNodes: event.DisagreeingNodes,
		Metadata:         event.Metadata,
		Nonce:            0,
	}
}

// CalculateHash computes the block hash
func (b *Block) CalculateHash() []byte {
	data := bytes.Buffer{}
	data.Write(b.PreviousBlockHash())
	data.Write([]byte(fmt.Sprintf("%d", b.Height)))
	data.Write([]byte(fmt.Sprintf("%d", b.Timestamp)))
	data.Write([]byte(b.TaskID))
	data.Write([]byte(b.FinalResult))
	data.Write([]byte(fmt.Sprintf("%v", b.IsValid)))
	data.Write([]byte(fmt.Sprintf("%f", b.AgreementRate)))
	data.Write([]byte(fmt.Sprintf("%d", b.Nonce)))

	// Sort node IDs for deterministic hash
	if len(b.NodeResults) > 0 {
		keys := make([]string, 0, len(b.NodeResults))
		for nodeID := range b.NodeResults {
			keys = append(keys, nodeID)
		}
		sort.Strings(keys)

		// Include sorted node results in hash
		for _, nodeID := range keys {
			data.Write([]byte(nodeID))
			data.Write([]byte(b.NodeResults[nodeID]))
		}
	}

	// Use SHA256 from standard library
	hash := sha256.Sum256(data.Bytes())
	return hash[:]
}

// Hash returns the block hash, computing it if necessary
func (b *Block) Hash() []byte {
	if b.BlockHash == nil {
		b.BlockHash = b.CalculateHash()
	}
	return b.BlockHash
}

// PreviousBlockHash returns the previous block hash
func (b *Block) PreviousBlockHash() []byte {
	if b.PrevBlockHash == nil {
		return make([]byte, 32)
	}
	return b.PrevBlockHash
}

// String returns a human-readable representation of the block
func (b *Block) String() string {
	return fmt.Sprintf("Block{Hash: %s, Height: %d, TaskID: %s, Valid: %v, Agreement: %.2f%%}",
		hex.EncodeToString(b.Hash())[:16]+"...",
		b.Height,
		b.TaskID,
		b.IsValid,
		b.AgreementRate*100,
	)
}

// ToJSON returns JSON representation of the block
func (b *Block) ToJSON() ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

// ToDisplayMap returns a map suitable for display
func (b *Block) ToDisplayMap() map[string]interface{} {
	result := map[string]interface{}{
		"height":            b.Height,
		"hash":              hex.EncodeToString(b.Hash()),
		"prev_block_hash":   hex.EncodeToString(b.PrevBlockHash),
		"timestamp":         time.Unix(b.Timestamp, 0).Format(time.RFC3339),
		"task_id":           b.TaskID,
		"final_result":      b.FinalResult,
		"is_valid":          b.IsValid,
		"agreement_rate":    fmt.Sprintf("%.2f%%", b.AgreementRate*100),
		"consensus_nodes":   len(b.ConsensusNodes),
		"disagreeing_nodes": len(b.DisagreeingNodes),
		"nonce":             b.Nonce,
	}

	if len(b.ConsensusNodes) > 0 {
		result["consensus_nodes_list"] = b.ConsensusNodes
	}

	if len(b.DisagreeingNodes) > 0 {
		result["disagreeing_nodes_list"] = b.DisagreeingNodes
	}

	return result
}

// Serialize serializes the block to bytes
func (b *Block) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(b)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DeserializeBlock deserializes a block from bytes
func DeserializeBlock(data []byte) (*Block, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var block Block
	err := dec.Decode(&block)
	if err != nil {
		return nil, err
	}
	return &block, nil
}
