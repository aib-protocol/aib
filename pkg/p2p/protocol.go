// Package p2p implements P2P networking for AIB blockchain.
// Protocol message definitions for node-to-node communication.
package p2p

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Protocol version
const (
	ProtocolVersion uint32 = 1
	MagicBytes      uint32 = 0x41494232       // "AIB2" in hex
	MaxMessageSize         = 10 * 1024 * 1024 // 10 MB max message
)

// Message types
const (
	MsgVersion   uint8 = 1
	MsgVerack    uint8 = 2
	MsgPing      uint8 = 3
	MsgPong      uint8 = 4
	MsgGetBlocks uint8 = 5
	MsgBlocks    uint8 = 6
	MsgNewBlock  uint8 = 7
	MsgGetPeers  uint8 = 8
	MsgPeers     uint8 = 9
	MsgTx        uint8 = 10
	MsgReject    uint8 = 11

	// History refetch channel (full<->light): on-demand retrieval of pruned
	// historical blocks from full nodes.
	MsgGetBlocksByRange  uint8 = 12
	MsgBlocksByRangeResp uint8 = 13
)

// MsgTypeName returns human-readable name for message type
func MsgTypeName(t uint8) string {
	switch t {
	case MsgVersion:
		return "VERSION"
	case MsgVerack:
		return "VERACK"
	case MsgPing:
		return "PING"
	case MsgPong:
		return "PONG"
	case MsgGetBlocks:
		return "GETBLOCKS"
	case MsgBlocks:
		return "BLOCKS"
	case MsgNewBlock:
		return "NEWBLOCK"
	case MsgGetPeers:
		return "GETPEERS"
	case MsgPeers:
		return "PEERS"
	case MsgTx:
		return "TX"
	case MsgReject:
		return "REJECT"
	case MsgGetBlocksByRange:
		return "GETBLOCKSBYRANGE"
	case MsgBlocksByRangeResp:
		return "BLOCKSBYRANGERESP"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", t)
	}
}

// ======================================================================
// Wire Protocol: [4 bytes magic][1 byte type][4 bytes payload length][payload]
// ======================================================================

// MessageHeader is the wire header for all P2P messages.
type MessageHeader struct {
	Magic  uint32
	Type   uint8
	Length uint32
}

const MessageHeaderSize = 9 // 4 + 1 + 4

// EncodeMessage encodes a message to wire format.
func EncodeMessage(msgType uint8, payload []byte) []byte {
	buf := make([]byte, MessageHeaderSize+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], MagicBytes)
	buf[4] = msgType
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(payload)))
	copy(buf[9:], payload)
	return buf
}

// ReadMessage reads a single message from reader.
// ReadMessageOn reads one message from r, replaying preRead bytes first
// (used after protocol sniffing consumed the first bytes).
func ReadMessageOn(r io.Reader, preRead []byte) (uint8, []byte, error) {
	if len(preRead) > 0 {
		return ReadMessage(io.MultiReader(bytes.NewReader(preRead), r))
	}
	return ReadMessage(r)
}

func ReadMessage(r io.Reader) (uint8, []byte, error) {
	header := make([]byte, MessageHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != MagicBytes {
		return 0, nil, fmt.Errorf("invalid magic: 0x%08x (expected 0x%08x)", magic, MagicBytes)
	}

	msgType := header[4]
	length := binary.BigEndian.Uint32(header[5:9])

	if length > MaxMessageSize {
		return 0, nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}

	if length == 0 {
		return msgType, nil, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload: %w", err)
	}

	return msgType, payload, nil
}

// ======================================================================
// Message Payloads
// ======================================================================

// VersionMsg is sent when first connecting to a peer.
type VersionMsg struct {
	Version     uint32 `json:"version"`
	GenesisHash string `json:"genesis_hash"` // hex-encoded genesis block hash
	BestHeight  uint64 `json:"best_height"`
	NodeID      string `json:"node_id"`
	ListenPort  int    `json:"listen_port"`
	Nickname    string `json:"nickname,omitempty"`
	Validator   bool   `json:"validator,omitempty"`  // node runs in validator mode
	StakeAddr   string `json:"stake_addr,omitempty"` // hex staking address (when staked)
	Timestamp   int64  `json:"timestamp"`
	UserAgent   string `json:"user_agent"`
}

// VerackMsg acknowledges a version message.
type VerackMsg struct {
	GenesisHash string `json:"genesis_hash"`
	BestHeight  uint64 `json:"best_height"`
	NodeID      string `json:"node_id"`
	Nickname    string `json:"nickname,omitempty"`
	Validator   bool   `json:"validator,omitempty"`
	StakeAddr   string `json:"stake_addr,omitempty"`
}

// PingMsg is a keepalive message.
type PingMsg struct {
	Nonce uint64 `json:"nonce"`
}

// PongMsg responds to a ping.
type PongMsg struct {
	Nonce uint64 `json:"nonce"`
}

// GetBlocksMsg requests blocks from a peer.
type GetBlocksMsg struct {
	FromHeight uint64 `json:"from_height"`
	ToHeight   uint64 `json:"to_height"`  // 0 = up to peer's best
	MaxBlocks  int    `json:"max_blocks"` // max blocks to return (default 500)
}

// BlockData is a serialized block for wire transfer.
type BlockData struct {
	Height        uint64 `json:"height"`
	Hash          string `json:"hash"`
	PrevBlockHash string `json:"prev_block_hash"`
	MerkleRoot    string `json:"merkle_root"`
	Timestamp     uint64 `json:"timestamp"`
	Proposer      string `json:"proposer"`
	Signature     string `json:"signature"`   // ed25519 signature of block hash by proposer
	SignedHash    string `json:"signed_hash"` // hash that was signed (header without signature)
	TxCount       int    `json:"tx_count"`
	RawBlock      []byte `json:"raw_block"` // full serialized block
}

// BlocksMsg contains a batch of blocks.
type BlocksMsg struct {
	Blocks []BlockData `json:"blocks"`
}

// NewBlockMsg announces a new block to peers.
type NewBlockMsg struct {
	Block BlockData `json:"block"`
}

// ChainPeerInfo describes a known peer in the chain P2P network.
type ChainPeerInfo struct {
	NodeID     string `json:"node_id"`
	Address    string `json:"address"` // ip:port
	Nickname   string `json:"nickname,omitempty"`
	Validator  bool   `json:"validator"`
	StakeAddr  string `json:"stake_addr,omitempty"`
	BestHeight uint64 `json:"best_height"`
	LastSeen   int64  `json:"last_seen"`
}

// GetPeersMsg requests peer list.
type GetPeersMsg struct {
	MaxPeers int `json:"max_peers"`
}

// PeersMsg contains a list of known peers.
type PeersMsg struct {
	Peers []ChainPeerInfo `json:"peers"`
}

// RejectMsg signals rejection of a message.
type RejectMsg struct {
	MessageType uint8  `json:"message_type"`
	Reason      string `json:"reason"`
	Code        uint8  `json:"code"`
}

// Rejection codes
const (
	RejectGenesisMismatch uint8 = 1
	RejectInvalidVersion  uint8 = 2
	RejectDuplicate       uint8 = 3
	RejectInvalidBlock    uint8 = 4
)

// ======================================================================
// Encoding helpers
// ======================================================================

// MarshalMsg encodes a message payload to JSON bytes wrapped in wire format.
func MarshalMsg(msgType uint8, v interface{}) ([]byte, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", MsgTypeName(msgType), err)
	}
	return EncodeMessage(msgType, payload), nil
}

// UnmarshalMsg decodes a JSON message payload.
func UnmarshalMsg(payload []byte, v interface{}) error {
	return json.NewDecoder(bytes.NewReader(payload)).Decode(v)
}
