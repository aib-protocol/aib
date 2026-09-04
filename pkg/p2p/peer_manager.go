// Package p2p implements P2P networking for AIB blockchain.
// ChainPeerManager handles TCP connections, peer discovery, and heartbeat
// for the blockchain layer (distinct from the agentic PeerManager).
package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	utxoPkg "github.com/aib-protocol/aib/pkg/utxo"
)

// BlockVerifier verifies block signatures before accepting them.
type BlockVerifier interface {
	// VerifyBlockSignature checks that the block's signature is valid
	// for the given proposer public key and block hash.
	VerifyBlockSignature(proposerPubKeyHex, blockHash, signature string) error
}

// ChainPeerManager manages chain-level P2P peer connections.
type ChainPeerManager struct {
	mu sync.RWMutex

	// Node identity
	nodeID        string
	nickname      string
	selfValidator bool
	selfStakeAddr string
	advertiseAddr string
	fileDist      *FileDistConfig
	releaseJSON   ReleaseJSONProvider
	onLocalHeight func() uint64
	listenPort    int
	genesisHash   string
	chainID       string
	bestHeight    uint64

	// Network
	listener  net.Listener
	peers     map[string]*ChainPeer // nodeID -> ChainPeer
	bootstrap []string              // bootstrap node addresses

	// Callbacks
	onNewBlock   func(data BlockData) error
	onGetBlocks  func(from, to uint64) ([]BlockData, error)
	onBestHeight func() uint64

	// Block verification
	blockVerifier BlockVerifier

	// Control
	logger *log.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	maxPeers int
	onTx     func(tx *utxoPkg.Transaction)

	// History refetch (GET_BLOCKS_BY_RANGE) state.
	onBlocksByRange func(from, to uint64) (blocks []BlockData, missing []uint64)
	fetchMu         sync.Mutex // one outstanding FetchBlocksByRange
	fetchChMu       sync.Mutex
	fetchCh         chan BlocksByRangeRespMsg
	fetchReqID      uint64
}

// ChainPeer represents a connected blockchain peer.
type ChainPeer struct {
	mu         sync.Mutex
	conn       net.Conn
	nodeID     string
	nickname   string
	address    string // remote ip:port
	listenPort int    // port the peer listens on for P2P
	validator  bool   // peer runs in validator mode
	stakeAddr  string // hex staking address (when staked)
	userAgent  string // peer's reported /aib-node/<version> string
	bestHeight uint64
	lastPing   time.Time
	lastPong   time.Time
	connected  time.Time
	outbound   bool // true if we initiated the connection
	verified   bool // true after VERSION/VERACK exchange
}

// ChainPeerConfig configures ChainPeerManager.
type ChainPeerConfig struct {
	NodeID        string
	Nickname      string
	ListenPort    int
	GenesisHash   string
	ChainID       string // "aib-testnet-1" or "aib-mainnet-1"
	Bootstrap     []string
	MaxPeers      int
	Validator     bool                // this node runs in validator mode
	StakeAddr     string              // hex staking address (when staked)
	AdvertiseAddr string              // "ip:port" to list OURSELF as (external IP; empty = skip self)
	FileDist      *FileDistConfig     // in-band HTTP distribution on the P2P port
	ReleaseJSON   ReleaseJSONProvider // on-chain release record provider
	LocalHeight   func() uint64       // local best height for pongs (peer view)
	Logger        *log.Logger
}

// NewChainPeerManager creates a new ChainPeerManager.
func NewChainPeerManager(cfg ChainPeerConfig) *ChainPeerManager {
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 25
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ChainPeerManager{
		nodeID:        cfg.NodeID,
		nickname:      cfg.Nickname,
		selfValidator: cfg.Validator,
		selfStakeAddr: cfg.StakeAddr,
		advertiseAddr: cfg.AdvertiseAddr,
		fileDist:      cfg.FileDist,
		releaseJSON:   cfg.ReleaseJSON,
		onLocalHeight: cfg.LocalHeight,
		listenPort:    cfg.ListenPort,
		genesisHash:   cfg.GenesisHash,
		chainID:       cfg.ChainID,
		bootstrap:     cfg.Bootstrap,
		peers:         make(map[string]*ChainPeer),
		maxPeers:      cfg.MaxPeers,
		logger:        cfg.Logger,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetHandlers sets callback handlers for block events.
func (pm *ChainPeerManager) SetHandlers(
	onNewBlock func(data BlockData) error,
	onGetBlocks func(from, to uint64) ([]BlockData, error),
	onBestHeight func() uint64,
) {
	pm.onNewBlock = onNewBlock
	pm.onGetBlocks = onGetBlocks
	pm.onBestHeight = onBestHeight
}

// SetBlockVerifier sets the block signature verifier.
// If not set, a default Ed25519BlockVerifier is used.
// StartAutoSync launches a periodic catch-up loop: if any verified peer's
// best height exceeds ours, request missing blocks. This fixes nodes that
// fall behind at runtime (GETBLOCKS used to fire only at startup).
func (pm *ChainPeerManager) StartAutoSync(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			local := uint64(0)
			if pm.onBestHeight != nil {
				local = pm.onBestHeight()
				pm.mu.Lock()
				pm.bestHeight = local
				pm.mu.Unlock()
			}
			pm.mu.RLock()
			var best uint64
			for _, p := range pm.peers {
				if p.verified && p.bestHeight > best {
					best = p.bestHeight
				}
			}
			pm.mu.RUnlock()
			if best > local+1 {
				pm.logger.Printf("[P2P] AutoSync: local height %d < peer best %d, requesting blocks", local, best)
				if err := pm.RequestBlocksFromBestPeer(local + 1); err != nil {
					pm.logger.Printf("[P2P] AutoSync request failed: %v", err)
				}
			}
		}
	}()
}

func (pm *ChainPeerManager) SetBlockVerifier(v BlockVerifier) {
	pm.blockVerifier = v
}

// Ed25519BlockVerifier verifies block signatures using Ed25519.
type Ed25519BlockVerifier struct{}

// VerifyBlockSignature verifies the block hash was signed by the proposer.
func (v *Ed25519BlockVerifier) VerifyBlockSignature(proposerPubKeyHex, blockHash, signature string) error {
	return v.VerifyBlockSignatureFull(proposerPubKeyHex, blockHash, "", signature)
}

// VerifyBlockSignatureFull verifies with explicit signed-hash semantics.
func (v *Ed25519BlockVerifier) VerifyBlockSignatureFull(proposerPubKeyHex, blockHash, signedHashHex, signature string) error {
	if signature == "" {
		return fmt.Errorf("block signature is empty")
	}
	if proposerPubKeyHex == "" {
		return fmt.Errorf("block proposer public key is empty")
	}

	pubKeyBytes, err := hex.DecodeString(proposerPubKeyHex)
	if err != nil {
		return fmt.Errorf("invalid proposer public key hex: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid proposer public key length: got %d, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: got %d, want %d", len(sigBytes), ed25519.SignatureSize)
	}

	// The signature is over the "signed hash" — the header hash WITHOUT the
	// signature field (SignBlock semantics). Fall back to the full block hash
	// for legacy senders that signed the final hash directly.
	targetHex := signedHashHex
	if targetHex == "" {
		targetHex = blockHash
	}
	hashBytes, err := hex.DecodeString(targetHex)
	if err != nil {
		return fmt.Errorf("invalid block hash hex: %w", err)
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	if !ed25519.Verify(pubKey, hashBytes, sigBytes) {
		return fmt.Errorf("block signature verification failed for proposer %s", proposerPubKeyHex)
	}

	return nil
}

// proposerKeyFromRawBlock extracts the embedded proposer public key from a
// serialized block. Kept in p2p (not calling pkg/utxo) to avoid an import
// cycle; parses just the fixed-position header fields before variable data.
func proposerKeyFromRawBlock(raw []byte) string {
	b, err := utxoPkg.DeserializeBlock(raw)
	if err != nil || b == nil {
		return ""
	}
	return hex.EncodeToString(b.Header.ProposerKey[:])
}

// verifyBlockSignature verifies a block's signature using the configured verifier.
func (pm *ChainPeerManager) verifyBlockSignature(block BlockData) error {
	// V3 address model: block.Proposer is the WALLET address (SHA256 of the
	// pubkey), not the pubkey itself. The real public key is carried in the
	// serialized block (Header.ProposerKey). Prefer verifying against the
	// deserialized block; fall back to the legacy Proposer field only when no
	// raw block is available (older peers).
	if len(block.RawBlock) > 0 {
		if pubKeyHex := proposerKeyFromRawBlock(block.RawBlock); pubKeyHex != "" {
			v := pm.blockVerifier
			if v == nil {
				v = &Ed25519BlockVerifier{}
			}
			if fv, ok := v.(interface {
				VerifyBlockSignatureFull(string, string, string, string) error
			}); ok {
				return fv.VerifyBlockSignatureFull(pubKeyHex, block.Hash, block.SignedHash, block.Signature)
			}
		}
	}
	verifier := pm.blockVerifier
	if verifier == nil {
		verifier = &Ed25519BlockVerifier{}
	}
	if fv, ok := verifier.(interface {
		VerifyBlockSignatureFull(string, string, string, string) error
	}); ok {
		return fv.VerifyBlockSignatureFull(block.Proposer, block.Hash, block.SignedHash, block.Signature)
	}
	return verifier.VerifyBlockSignature(block.Proposer, block.Hash, block.Signature)
}

// StartChainP2P starts the chain P2P peer manager.
func (pm *ChainPeerManager) StartChainP2P() error {
	// Start TCP listener
	addr := fmt.Sprintf("0.0.0.0:%d", pm.listenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	pm.listener = ln
	pm.logger.Printf("[P2P] Listening on %s", addr)

	// Accept incoming connections
	pm.wg.Add(1)
	go pm.acceptLoop()

	// Connect to bootstrap nodes
	pm.wg.Add(1)
	go pm.bootstrapConnect()

	// Start heartbeat loop
	pm.wg.Add(1)
	go pm.heartbeatLoop()

	// Start peer discovery loop
	pm.wg.Add(1)
	go pm.discoveryLoop()

	return nil
}

// StopChainP2P stops the chain peer manager gracefully.
func (pm *ChainPeerManager) StopChainP2P() {
	pm.logger.Println("[P2P] Stopping peer manager...")
	pm.cancel()
	if pm.listener != nil {
		pm.listener.Close()
	}
	pm.mu.RLock()
	for _, p := range pm.peers {
		p.conn.Close()
	}
	pm.mu.RUnlock()
	pm.wg.Wait()
	pm.logger.Println("[P2P] Peer manager stopped")
}

// UpdateBestHeight updates the local best block height.
func (pm *ChainPeerManager) UpdateBestHeight(height uint64) {
	pm.mu.Lock()
	pm.bestHeight = height
	pm.mu.Unlock()
}

// GetPeerCount returns number of connected peers.
func (pm *ChainPeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// GetChainID returns the chain ID this peer manager is configured for.
func (pm *ChainPeerManager) GetChainID() string {
	return pm.chainID
}

// GetChainPeers returns info about all connected chain peers.
func (pm *ChainPeerManager) GetChainPeers() []ChainPeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]ChainPeerInfo, 0, len(pm.peers))
	for _, p := range pm.peers {
		result = append(result, ChainPeerInfo{
			NodeID:     p.nodeID,
			Address:    p.address,
			Nickname:   p.nickname,
			Validator:  p.validator,
			StakeAddr:  p.stakeAddr,
			UserAgent:  p.userAgent,
			BestHeight: p.bestHeight,
			LastSeen:   p.lastPong.Unix(),
		})
	}
	return result
}

// SelfInfo returns this node's own peer info (for the peers API), or nil
// when no advertise address is configured.
func (pm *ChainPeerManager) SelfInfo() *ChainPeerInfo {
	if pm.advertiseAddr == "" {
		return nil
	}
	return &ChainPeerInfo{
		NodeID:     pm.nodeID,
		Address:    pm.advertiseAddr,
		Nickname:   pm.nickname,
		Validator:  pm.selfValidator,
		StakeAddr:  pm.selfStakeAddr,
		BestHeight: pm.bestHeight,
		LastSeen:   time.Now().Unix(),
		UserAgent:  UserAgent(),
	}
}

// BroadcastNewBlock sends a new block to all connected peers.
// SetTxCallback registers the handler invoked for gossiped transactions.
func (pm *ChainPeerManager) SetTxCallback(fn func(tx *utxoPkg.Transaction)) {
	pm.onTx = fn
}

// BroadcastTx gossips a signed transaction to all chain peers.
// NOTE: payload is the RAW binary transaction (EncodeMessage), NOT JSON —
// the receiver calls DeserializeTransaction on the payload directly.
func (pm *ChainPeerManager) BroadcastTx(tx *utxoPkg.Transaction) {
	data := tx.Serialize()
	msg := EncodeMessage(MsgTx, data)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.peers {
		p.mu.Lock()
		if p.conn != nil {
			p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			p.conn.Write(msg)
			p.conn.SetWriteDeadline(time.Time{})
		}
		p.mu.Unlock()
	}
}

func (pm *ChainPeerManager) BroadcastNewBlock(block BlockData) {
	msg := NewBlockMsg{Block: block}
	data, err := MarshalMsg(MsgNewBlock, &msg)
	if err != nil {
		pm.logger.Printf("[P2P] Failed to marshal new block: %v", err)
		return
	}

	pm.mu.RLock()
	peers := make([]*ChainPeer, 0, len(pm.peers))
	for _, p := range pm.peers {
		if p.verified {
			peers = append(peers, p)
		}
	}
	pm.mu.RUnlock()

	for _, p := range peers {
		p.mu.Lock()
		_, err := p.conn.Write(data)
		p.mu.Unlock()
		if err != nil {
			pm.logger.Printf("[P2P] Failed to send block to %s: %v", p.nodeID[:8], err)
		}
	}

	pm.logger.Printf("[P2P] Broadcasted block %d to %d peers", block.Height, len(peers))
}

// GetBestPeerHeight returns the highest known height among peers.
func (pm *ChainPeerManager) GetBestPeerHeight() uint64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var best uint64
	for _, p := range pm.peers {
		if p.bestHeight > best {
			best = p.bestHeight
		}
	}
	return best
}

// ======================================================================
// Internal: accept loop
// ======================================================================

func (pm *ChainPeerManager) acceptLoop() {
	defer pm.wg.Done()

	for {
		conn, err := pm.listener.Accept()
		if err != nil {
			select {
			case <-pm.ctx.Done():
				return
			default:
				pm.logger.Printf("[P2P] Accept error: %v", err)
				time.Sleep(time.Second)
				continue
			}
		}

		pm.mu.RLock()
		peerCount := len(pm.peers)
		pm.mu.RUnlock()

		if peerCount >= pm.maxPeers {
			pm.logger.Printf("[P2P] Max peers reached, rejecting %s", conn.RemoteAddr())
			conn.Close()
			continue
		}

		pm.wg.Add(1)
		go pm.handleInbound(conn)
	}
}

// ======================================================================
// Internal: bootstrap connection
// ======================================================================

func (pm *ChainPeerManager) bootstrapConnect() {
	defer pm.wg.Done()

	// Wait a moment for listener to start
	time.Sleep(time.Second)

	// Keep retrying bootstrap until we have at least one peer (seed may come
	// online later than us).
	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
		}
		if pm.GetPeerCount() > 0 {
			// We have peers; still refresh every 5 min in case we drop to zero.
			select {
			case <-pm.ctx.Done():
				return
			case <-time.After(5 * time.Minute):
			}
			if pm.GetPeerCount() > 0 {
				continue
			}
		}
		for _, addr := range pm.bootstrap {
			select {
			case <-pm.ctx.Done():
				return
			default:
			}
			if pm.HasPeerAt(addr) {
				continue // already connected to this bootstrap
			}
			pm.logger.Printf("[P2P] Connecting to bootstrap node %s ...", addr)
			if err := pm.connectToPeer(addr); err != nil {
				pm.logger.Printf("[P2P] Bootstrap %s failed: %v", addr, err)
			}
		}
		select {
		case <-pm.ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// connectToPeer dials and handshakes with a peer.
func (pm *ChainPeerManager) connectToPeer(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	// Send VERSION message
	height := pm.bestHeight
	if pm.onBestHeight != nil {
		height = pm.onBestHeight()
	}

	versionMsg := VersionMsg{
		Version:     ProtocolVersion,
		GenesisHash: pm.genesisHash,
		BestHeight:  height,
		NodeID:      pm.nodeID,
		ListenPort:  pm.listenPort,
		Nickname:    pm.nickname,
		Validator:   pm.selfValidator,
		StakeAddr:   pm.selfStakeAddr,
		Timestamp:   time.Now().Unix(),
		UserAgent:   UserAgent(),
	}

	data, err := MarshalMsg(MsgVersion, &versionMsg)
	if err != nil {
		conn.Close()
		return fmt.Errorf("marshal version: %w", err)
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(data); err != nil {
		conn.Close()
		return fmt.Errorf("send version: %w", err)
	}
	conn.SetWriteDeadline(time.Time{})

	// Read the PEER's VERSION, then VERACK or REJECT.
	// The peer sends its version right after accepting our dial; we must
	// consume it (and capture its UserAgent) before the verack.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var peerVersion VersionMsg
	var verackPayload []byte
	for range 2 {
		msgType, payload, err := ReadMessage(conn)
		if err != nil {
			conn.Close()
			return fmt.Errorf("read verack: %w", err)
		}
		switch msgType {
		case MsgReject:
			var reject RejectMsg
			UnmarshalMsg(payload, &reject)
			conn.Close()
			return fmt.Errorf("rejected: %s", reject.Reason)
		case MsgVersion:
			_ = UnmarshalMsg(payload, &peerVersion)
		case MsgVerack:
			verackPayload = payload
			goto haveVerack
		}
	}
	conn.Close()
	return fmt.Errorf("expected VERACK, got nothing useful")

haveVerack:
	conn.SetReadDeadline(time.Time{})

	var verack VerackMsg
	if err := UnmarshalMsg(verackPayload, &verack); err != nil {
		conn.Close()
		return fmt.Errorf("unmarshal verack: %w", err)
	}

	// Verify genesis hash
	if verack.GenesisHash != pm.genesisHash {
		conn.Close()
		return fmt.Errorf("genesis hash mismatch: %s vs %s", verack.GenesisHash[:16], pm.genesisHash[:16])
	}

	// Register peer
	peer := &ChainPeer{
		conn:       conn,
		nodeID:     verack.NodeID,
		nickname:   verack.Nickname,
		validator:  verack.Validator,
		stakeAddr:  verack.StakeAddr,
		userAgent:  peerVersion.UserAgent,
		address:    addr,
		bestHeight: verack.BestHeight,
		connected:  time.Now(),
		lastPong:   time.Now(),
		outbound:   true,
		verified:   true,
	}
	pm.warnIfOutdated(peer.userAgent, addr)

	pm.mu.Lock()
	if _, exists := pm.peers[peer.nodeID]; exists {
		pm.mu.Unlock()
		conn.Close()
		return fmt.Errorf("already connected to %s", peer.nodeID[:8])
	}
	pm.peers[peer.nodeID] = peer
	pm.mu.Unlock()

	pm.logger.Printf("[P2P] Connected to %s (%s) height=%d",
		peer.nodeID[:8], peer.nickname, peer.bestHeight)

	// Start reading from this peer
	pm.wg.Add(1)
	go pm.readLoop(peer)

	return nil
}

// ======================================================================
// Internal: handle inbound connection
// ======================================================================

func (pm *ChainPeerManager) handleInbound(conn net.Conn) {
	defer pm.wg.Done()

	remoteAddr := conn.RemoteAddr().String()

	// Protocol sniff: HTTP on the P2P port = in-band file distribution
	// (install.sh / binaries). Anything else = normal P2P handshake.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	head := make([]byte, 5)
	n, _ := io.ReadFull(conn, head)
	head = head[:n]
	if looksLikeHTTP(head) {
		pm.serveFileDist(conn, head)
		return
	}
	rest := make([]byte, 0, 64*1024)
	rest = append(rest, head...)
	// fall through to normal handshake with replayed bytes
	msgType, payload, err := ReadMessageOn(conn, rest)
	if err != nil {
		pm.logger.Printf("[P2P] Inbound %s: read version failed: %v", remoteAddr, err)
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	if msgType != MsgVersion {
		pm.logger.Printf("[P2P] Inbound %s: expected VERSION, got %s", remoteAddr, MsgTypeName(msgType))
		conn.Close()
		return
	}

	var version VersionMsg
	if err := UnmarshalMsg(payload, &version); err != nil {
		pm.logger.Printf("[P2P] Inbound %s: unmarshal version failed: %v", remoteAddr, err)
		conn.Close()
		return
	}

	// Verify genesis hash
	if version.GenesisHash != pm.genesisHash {
		pm.logger.Printf("[P2P] Inbound %s: genesis hash mismatch", remoteAddr)
		reject := RejectMsg{
			MessageType: MsgVersion,
			Reason:      "genesis hash mismatch",
			Code:        RejectGenesisMismatch,
		}
		data, _ := MarshalMsg(MsgReject, &reject)
		conn.Write(data)
		conn.Close()
		return
	}

	// Check if already connected
	pm.mu.RLock()
	_, exists := pm.peers[version.NodeID]
	pm.mu.RUnlock()
	if exists {
		reject := RejectMsg{
			MessageType: MsgVersion,
			Reason:      "duplicate connection",
			Code:        RejectDuplicate,
		}
		data, _ := MarshalMsg(MsgReject, &reject)
		conn.Write(data)
		conn.Close()
		return
	}

	// Send our VERSION first (so the dialer can capture our UserAgent),
	// then VERACK.
	height := pm.bestHeight
	if pm.onBestHeight != nil {
		height = pm.onBestHeight()
	}

	ourVersion := VersionMsg{
		Version:     ProtocolVersion,
		GenesisHash: pm.genesisHash,
		BestHeight:  height,
		NodeID:      pm.nodeID,
		ListenPort:  pm.listenPort,
		Nickname:    pm.nickname,
		Validator:   pm.selfValidator,
		StakeAddr:   pm.selfStakeAddr,
		Timestamp:   time.Now().Unix(),
		UserAgent:   UserAgent(),
	}
	if vdata, err := MarshalMsg(MsgVersion, &ourVersion); err == nil {
		conn.Write(vdata)
	}

	verack := VerackMsg{
		GenesisHash: pm.genesisHash,
		BestHeight:  height,
		NodeID:      pm.nodeID,
		Nickname:    pm.nickname,
		Validator:   pm.selfValidator,
		StakeAddr:   pm.selfStakeAddr,
	}
	data, _ := MarshalMsg(MsgVerack, &verack)
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(data); err != nil {
		pm.logger.Printf("[P2P] Inbound %s: send verack failed: %v", remoteAddr, err)
		conn.Close()
		return
	}
	conn.SetWriteDeadline(time.Time{})

	// Determine the peer's listen address
	host, _, _ := net.SplitHostPort(remoteAddr)
	peerListenAddr := fmt.Sprintf("%s:%d", host, version.ListenPort)

	// Register peer
	peer := &ChainPeer{
		conn:       conn,
		nodeID:     version.NodeID,
		nickname:   version.Nickname,
		address:    peerListenAddr,
		listenPort: version.ListenPort,
		validator:  version.Validator,
		stakeAddr:  version.StakeAddr,
		userAgent:  version.UserAgent,
		bestHeight: version.BestHeight,
		connected:  time.Now(),
		lastPong:   time.Now(),
		outbound:   false,
		verified:   true,
	}
	pm.warnIfOutdated(peer.userAgent, remoteAddr)

	if peer.nodeID == pm.nodeID {
		conn.Close()
		pm.logger.Printf("[P2P] Rejected self-connection from %s", remoteAddr)
		return
	}
	pm.mu.Lock()
	pm.peers[peer.nodeID] = peer
	pm.mu.Unlock()

	pm.logger.Printf("[P2P] Inbound peer %s (%s) height=%d from %s",
		peer.nodeID[:8], peer.nickname, peer.bestHeight, remoteAddr)

	// Start reading from this peer
	pm.wg.Add(1)
	go pm.readLoop(peer)
}

// ======================================================================
// Internal: read loop for a connected peer
// ======================================================================

func (pm *ChainPeerManager) readLoop(peer *ChainPeer) {
	defer pm.wg.Done()
	defer pm.removePeer(peer)

	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
		}

		// Set a generous read deadline
		peer.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		msgType, payload, err := ReadMessage(peer.conn)
		if err != nil {
			if err == io.EOF || isClosedError(err) {
				pm.logger.Printf("[P2P] Peer %s disconnected", peer.nodeID[:8])
			} else {
				pm.logger.Printf("[P2P] Peer %s read error: %v", peer.nodeID[:8], err)
			}
			return
		}

		pm.handleChainMessage(peer, msgType, payload)
	}
}

func (pm *ChainPeerManager) handleChainMessage(peer *ChainPeer, msgType uint8, payload []byte) {
	switch msgType {
	case MsgPing:
		var ping PingMsg
		if err := UnmarshalMsg(payload, &ping); err != nil {
			return
		}
		pong := PongMsg{Nonce: ping.Nonce}
		if pm.onLocalHeight != nil {
			pong.Height = pm.onLocalHeight()
		}
		data, _ := MarshalMsg(MsgPong, &pong)
		peer.mu.Lock()
		peer.conn.Write(data)
		peer.mu.Unlock()

	case MsgPong:
		var pong PongMsg
		if err := UnmarshalMsg(payload, &pong); err != nil {
			return
		}
		peer.mu.Lock()
		peer.lastPong = time.Now()
		if pong.Height > peer.bestHeight {
			peer.bestHeight = pong.Height
		}
		peer.mu.Unlock()
	case MsgTx:
		// Transaction gossip: relay inbound transactions to the mempool via
		// the registered callback. Dedup is the mempool's job (tx hash).
		if pm.onTx != nil {
			if tx, err := utxoPkg.DeserializeTransaction(payload); err == nil && tx != nil {
				pm.onTx(tx)
			}
		}
		// Relay to OTHER peers (2-hop reach in a star topology). The
		// originator excluded; mempool dedup on our side prevents loops
		// (a relayed tx we already have is dropped before re-broadcast).
		pm.relayTx(peer, payload)

	case MsgGetPeers:
		peers := pm.GetChainPeers()
		// Add ourselves
		peers = append(peers, ChainPeerInfo{
			NodeID:     pm.nodeID,
			Address:    fmt.Sprintf("0.0.0.0:%d", pm.listenPort),
			Nickname:   pm.nickname,
			BestHeight: pm.bestHeight,
			LastSeen:   time.Now().Unix(),
		})
		msg := PeersMsg{Peers: peers}
		data, _ := MarshalMsg(MsgPeers, &msg)
		peer.mu.Lock()
		peer.conn.Write(data)
		peer.mu.Unlock()

	case MsgPeers:
		var msg PeersMsg
		if err := UnmarshalMsg(payload, &msg); err != nil {
			return
		}
		pm.handlePeersList(msg.Peers)

	case MsgGetBlocks:
		var msg GetBlocksMsg
		if err := UnmarshalMsg(payload, &msg); err != nil {
			return
		}
		if pm.onGetBlocks != nil {
			toHeight := msg.ToHeight
			if toHeight == 0 {
				toHeight = pm.bestHeight
			}
			blocks, err := pm.onGetBlocks(msg.FromHeight, toHeight)
			if err != nil {
				pm.logger.Printf("[P2P] GetBlocks handler error: %v", err)
				return
			}
			resp := BlocksMsg{Blocks: blocks}
			data, _ := MarshalMsg(MsgBlocks, &resp)
			peer.mu.Lock()
			peer.conn.Write(data)
			peer.mu.Unlock()
		}

	case MsgBlocks:
		var msg BlocksMsg
		if err := UnmarshalMsg(payload, &msg); err != nil {
			return
		}
		for _, block := range msg.Blocks {
			// SEC-008: verifyeachblocksignature
			if err := pm.verifyBlockSignature(block); err != nil {
				pm.logger.Printf("[P2P] Rejected synced block %d from peer %s: signature verification failed: %v",
					block.Height, peer.nodeID, err)
				return
			}
			if pm.onNewBlock != nil {
				if err := pm.onNewBlock(block); err != nil {
					pm.logger.Printf("[P2P] ProcessBlock %d error: %v", block.Height, err)
					return
				}
			}
		}
		// Update peer's best height
		if len(msg.Blocks) > 0 {
			last := msg.Blocks[len(msg.Blocks)-1]
			peer.mu.Lock()
			if last.Height > peer.bestHeight {
				peer.bestHeight = last.Height
			}
			peer.mu.Unlock()
		}

	case MsgNewBlock:
		var msg NewBlockMsg
		if err := UnmarshalMsg(payload, &msg); err != nil {
			return
		}
		// SEC-008: verify block signature to prevent forged-block injection
		if err := pm.verifyBlockSignature(msg.Block); err != nil {
			pm.logger.Printf("[P2P] Rejected block %d from peer %s: signature verification failed: %v",
				msg.Block.Height, peer.nodeID, err)
			return
		}
		if pm.onNewBlock != nil {
			if err := pm.onNewBlock(msg.Block); err != nil {
				pm.logger.Printf("[P2P] NewBlock %d error: %v", msg.Block.Height, err)
			}
		}
		// Update peer's height
		peer.mu.Lock()
		if msg.Block.Height > peer.bestHeight {
			peer.bestHeight = msg.Block.Height
		}
		peer.mu.Unlock()

	case MsgGetBlocksByRange:
		pm.serveGetBlocksByRange(peer, payload)

	case MsgBlocksByRangeResp:
		var resp BlocksByRangeRespMsg
		if err := UnmarshalMsg(payload, &resp); err != nil {
			return
		}
		pm.fetchChMu.Lock()
		ch := pm.fetchCh
		want := pm.fetchReqID
		pm.fetchChMu.Unlock()
		if ch != nil && resp.RequestID == want {
			select {
			case ch <- resp:
			default:
			}
		}

	default:
		pm.logger.Printf("[P2P] Unknown message type %s from %s", MsgTypeName(msgType), peer.nodeID[:8])
	}
}

// ======================================================================
// Internal: heartbeat (ping/pong)
// ======================================================================

func (pm *ChainPeerManager) heartbeatLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.sendPings()
			pm.checkTimeouts()
		}
	}
}

func (pm *ChainPeerManager) sendPings() {
	pm.mu.RLock()
	peers := make([]*ChainPeer, 0, len(pm.peers))
	for _, p := range pm.peers {
		peers = append(peers, p)
	}
	pm.mu.RUnlock()

	ping := PingMsg{Nonce: rand.Uint64()}
	data, _ := MarshalMsg(MsgPing, &ping)

	for _, p := range peers {
		p.mu.Lock()
		p.lastPing = time.Now()
		_, err := p.conn.Write(data)
		p.mu.Unlock()
		if err != nil {
			pm.logger.Printf("[P2P] Ping to %s failed: %v", p.nodeID[:8], err)
		}
	}
}

func (pm *ChainPeerManager) checkTimeouts() {
	pm.mu.RLock()
	var stale []*ChainPeer
	for _, p := range pm.peers {
		p.mu.Lock()
		if !p.lastPing.IsZero() && time.Since(p.lastPong) > 120*time.Second {
			stale = append(stale, p)
		}
		p.mu.Unlock()
	}
	pm.mu.RUnlock()

	for _, p := range stale {
		pm.logger.Printf("[P2P] Peer %s timed out, disconnecting", p.nodeID[:8])
		p.conn.Close()
		pm.removePeer(p)
	}
}

// ======================================================================
// Internal: peer discovery
// ======================================================================

func (pm *ChainPeerManager) discoveryLoop() {
	defer pm.wg.Done()

	// Run first discovery after 10 seconds
	select {
	case <-pm.ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}

	pm.requestPeers()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.requestPeers()
		}
	}
}

func (pm *ChainPeerManager) requestPeers() {
	pm.mu.RLock()
	peers := make([]*ChainPeer, 0, len(pm.peers))
	for _, p := range pm.peers {
		if p.verified {
			peers = append(peers, p)
		}
	}
	pm.mu.RUnlock()

	if len(peers) == 0 {
		return
	}

	msg := GetPeersMsg{MaxPeers: 20}
	data, _ := MarshalMsg(MsgGetPeers, &msg)

	for _, p := range peers {
		p.mu.Lock()
		p.conn.Write(data)
		p.mu.Unlock()
	}
}

func (pm *ChainPeerManager) handlePeersList(peers []ChainPeerInfo) {
	pm.mu.RLock()
	currentCount := len(pm.peers)
	pm.mu.RUnlock()

	if currentCount >= pm.maxPeers {
		return
	}

	for _, info := range peers {
		if info.NodeID == pm.nodeID {
			continue // skip self
		}

		pm.mu.RLock()
		_, exists := pm.peers[info.NodeID]
		pm.mu.RUnlock()

		if exists {
			continue
		}

		if info.Address == "" || info.Address == "0.0.0.0:0" {
			continue
		}

		// Try to connect
		go func(addr string) {
			if err := pm.connectToPeer(addr); err != nil {
				// Silent fail for discovery connections
				_ = err
			}
		}(info.Address)
	}
}

// ======================================================================
// Internal: peer removal
// ======================================================================

func (pm *ChainPeerManager) removePeer(peer *ChainPeer) {
	pm.mu.Lock()
	delete(pm.peers, peer.nodeID)
	pm.mu.Unlock()
	peer.conn.Close()
}

// RequestBlocks sends a GETBLOCKS request to a specific peer.
func (pm *ChainPeerManager) RequestBlocks(nodeID string, from, to uint64) error {
	pm.mu.RLock()
	peer, ok := pm.peers[nodeID]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("peer %s not found", nodeID)
	}

	msg := GetBlocksMsg{
		FromHeight: from,
		ToHeight:   to,
		MaxBlocks:  500,
	}
	data, _ := MarshalMsg(MsgGetBlocks, &msg)

	peer.mu.Lock()
	_, err := peer.conn.Write(data)
	peer.mu.Unlock()

	return err
}

// RequestBlocksFromBestPeer sends a GETBLOCKS to the peer with highest height.
func (pm *ChainPeerManager) RequestBlocksFromBestPeer(fromHeight uint64) error {
	pm.mu.RLock()
	var bestPeer *ChainPeer
	var bestHeight uint64
	for _, p := range pm.peers {
		if p.verified && p.bestHeight > bestHeight {
			bestHeight = p.bestHeight
			bestPeer = p
		}
	}
	pm.mu.RUnlock()

	if bestPeer == nil {
		return fmt.Errorf("no peers available")
	}

	msg := GetBlocksMsg{
		FromHeight: fromHeight,
		ToHeight:   0, // up to peer's best
		MaxBlocks:  500,
	}
	data, _ := MarshalMsg(MsgGetBlocks, &msg)

	bestPeer.mu.Lock()
	_, err := bestPeer.conn.Write(data)
	bestPeer.mu.Unlock()

	return err
}

// ======================================================================
// Helpers
// ======================================================================

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "use of closed network connection"
}

// GenerateNodeID generates a node ID from ed25519 public key.
func GenerateNodeID(pubKey []byte) string {
	return hex.EncodeToString(pubKey[:16])
}

// relayTx forwards a raw MsgTx frame to all peers except its origin.
func (pm *ChainPeerManager) relayTx(from *ChainPeer, payload []byte) {
	msg := EncodeMessage(MsgTx, payload)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.peers {
		if p == from || p.conn == nil {
			continue
		}
		p.mu.Lock()
		p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		p.conn.Write(msg)
		p.mu.Unlock()
	}
}

// HasPeerAt reports whether we already maintain a connection to addr.
func (pm *ChainPeerManager) HasPeerAt(addr string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.peers {
		if p.address == addr {
			return true
		}
	}
	return false
}

// warnIfOutdated logs a local, per-peer advisory when a peer runs a
// different node version. Decentralized by design: each node judges for
// itself from the peer's UserAgent; nothing is enforced network-wide.
func (pm *ChainPeerManager) warnIfOutdated(peerUA, addr string) {
	if peerUA == "" {
		return // old node that doesn't send a user agent
	}
	mine := UserAgent()
	if peerUA == mine {
		return
	}
	pm.logger.Printf("[P2P] version advisory: peer %s runs %s (we run %s) — consider aligning node versions", addr, peerUA, mine)
}
