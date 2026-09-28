package p2p

// NAT hole punching (v0.11.44) — Bitcoin-style rendezvous with TCP
// simultaneous open.
//
// Roles:
//   - RENDEZVOUS: any node with a public IP that both NATed nodes are already
//     connected to (in AIB's star-ish topology: the bootstrap seeds, but ANY
//     connected public peer can serve — no special authority).
//   - NATed node A / node B: connected to the rendezvous, want a direct link.
//
// Flow:
//   A -> rendezvous: PUNCHREGISTER{target: B}
//   rendezvous -> A: PUNCHINTRO{peer: B, peer_addr: B.ip:B.port, start_at: T}
//   rendezvous -> B: PUNCHINTRO{peer: A, peer_addr: A.ip:A.port, start_at: T}
//   at time T (±slack): A dials B's endpoint WHILE B dials A's endpoint
//     (TCP simultaneous open — both outbound SYNs cross mid-flight and open
//     each other's NAT mapping; the middleboxes see only outbound traffic).
//   The side whose connect() returns first sends PUNCHDIAL; then the normal
//   VERSION/VERACK handshake runs on the punched stream.
//
// Dial scheduling: start_at = now + 2s; both sides retry every 500ms for up
// to 8s (8 attempts). Success = one connect() succeeded AND VERSION/VERACK
// completes. Failure = fall back to relay through existing connections
// (sync still works via the rendezvous/star topology).

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// PunchManager implements the rendezvous + puncher roles.
type PunchManager struct {
	mu sync.Mutex
	// registered endpoints by nodeID: what the rendezvous has OBSERVED
	// (from live connections): nodeID -> "ip:port"
	observed map[string]string
	// pending introductions: target nodeID -> list of waiters
	// (registrant nodeID -> registration)
	pending map[string]map[string]PunchRegisterMsg
	// lastRegister per registrant for rate limiting (anti hammering)
	lastRegister map[string]time.Time
	// pendingSince: when each target's wait-list was created (TTL cleanup)
	pendingSince map[string]time.Time
	// sendIntro: how the rendezvous delivers PUNCHINTRO to a connected node.
	// Injected by ChainPeerManager (writePunchMsg). If a node is not
	// connected, introduction is impossible — registration expires.
	sendIntro func(nodeID string, msg *PunchIntroMsg) bool
	logger    *log.Logger
	now       func() time.Time
}

const (
	punchRegisterMinInterval = 10 * time.Second // per registrant
	punchPendingTTL          = 60 * time.Second
	punchMaxPendingPerTarget = 8
)

// NewPunchManager creates a manager. sendIntro must be safe for concurrent
// use; returning false means "node not currently connected".
func NewPunchManager(logger *log.Logger, sendIntro func(nodeID string, msg *PunchIntroMsg) bool) *PunchManager {
	return &PunchManager{
		observed:     map[string]string{},
		pending:      map[string]map[string]PunchRegisterMsg{},
		lastRegister: map[string]time.Time{},
		pendingSince: map[string]time.Time{},
		sendIntro:    sendIntro,
		logger:       logger,
		now:          time.Now,
	}
}

// Observe records (or refreshes) a node's public endpoint as seen by this
// rendezvous. Called on every connect / handshake.
func (pm *PunchManager) Observe(nodeID, addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.observed[nodeID] = addr
}

// HandleRegister processes PUNCHREGISTER arriving at the rendezvous.
// registrantAddr is the source address of the live connection (observed).
// Returns the session that was created (or nil if impossible: target unknown
// or not connected).
func (pm *PunchManager) HandleRegister(registrant, registrantAddr string, reg PunchRegisterMsg) *PunchIntroMsg {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.cleanupLocked()

	// Anti-hammering: same registrant faster than the interval is dropped.
	if t, ok := pm.lastRegister[registrant]; ok && pm.now().Sub(t) < punchRegisterMinInterval {
		pm.logger.Printf("[PUNCH] register from %s rate-limited", shortID(registrant))
		return nil
	}
	pm.lastRegister[registrant] = pm.now()

	pm.observed[registrant] = registrantAddr

	// Target must be known and currently connected (we can only intro nodes
	// we can deliver PUNCHINTRO to).
	targetAddr, known := pm.observed[reg.TargetNodeID]
	if !known {
		pm.logger.Printf("[PUNCH] %s wants %s but target endpoint unknown", shortID(registrant), shortID(reg.TargetNodeID))
		return nil
	}

	// Record the pending registration (also allows the target to see intent).
	if pm.pending[reg.TargetNodeID] == nil {
		pm.pending[reg.TargetNodeID] = map[string]PunchRegisterMsg{}
		pm.pendingSince[reg.TargetNodeID] = pm.now()
	}
	if len(pm.pending[reg.TargetNodeID]) >= punchMaxPendingPerTarget {
		pm.logger.Printf("[PUNCH] target %s wait-list full — dropping %s", shortID(reg.TargetNodeID), shortID(registrant))
		return nil
	}
	pm.pending[reg.TargetNodeID][registrant] = reg

	// Both sides dial 2.5s from now (gives PUNCHINTRO time to traverse both
	// links; clock skew tolerated by the retry window on both ends).
	startAt := pm.now().Add(2500 * time.Millisecond).Unix()

	introToA := &PunchIntroMsg{
		PeerNodeID:  reg.TargetNodeID,
		PeerAddr:    targetAddr,
		MyAddr:      registrantAddr,
		SessionID:   pm.newSessionID(),
		StartAtUnix: startAt,
	}
	introToB := &PunchIntroMsg{
		PeerNodeID:  registrant,
		PeerAddr:    registrantAddr,
		MyAddr:      targetAddr,
		SessionID:   introToA.SessionID,
		StartAtUnix: startAt,
	}

	okA := pm.sendIntro(registrant, introToA)
	okB := pm.sendIntro(reg.TargetNodeID, introToB)
	if !okA || !okB {
		pm.logger.Printf("[PUNCH] intro delivery failed a=%v b=%v — session %d aborted", okA, okB, introToA.SessionID)
		return nil
	}
	pm.logger.Printf("[PUNCH] introduced %s <-> %s (dial at T+%d)", shortID(registrant), shortID(reg.TargetNodeID), startAt)
	return introToA
}

var sessionCounter uint64
var sessionMu sync.Mutex

func (pm *PunchManager) newSessionID() uint64 {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionCounter++
	return sessionCounter
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// DialPlan is the local schedule computed from a received PUNCHINTRO.
type DialPlan struct {
	PeerAddr    string
	SessionID   uint64
	StartAt     time.Time
	Attempts    int
	Interval    time.Duration
	DialTimeout time.Duration
}

// PlanDial turns a PUNCHINTRO into a local dial schedule. attempt/interval
// defaults tuned for simultaneous open: retries overlap the other side's
// SYNs so at least one pair crosses.
func PlanDial(intro *PunchIntroMsg) *DialPlan {
	return &DialPlan{
		PeerAddr:    intro.PeerAddr,
		SessionID:   intro.SessionID,
		StartAt:     time.Unix(intro.StartAtUnix, 0),
		Attempts:    8,
		Interval:    500 * time.Millisecond,
		DialTimeout: 4 * time.Second,
	}
}

// PunchOutcome reports one punch attempt result.
type PunchOutcome struct {
	SessionID uint64
	Success   bool
	Addr      string
	Err       string
	Elapsed   time.Duration
}

// ExecuteDial runs the simultaneous-open dial loop. onConnected receives the
// established net.Conn for handshake chaining (may be called from a goroutine;
// must take ownership of the conn). It is the caller's job to send PUNCHDIAL
// or VERSION first — ExecuteDial only establishes the TCP stream.
func ExecuteDial(plan *DialPlan, onConnected func(conn net.Conn, outcome PunchOutcome)) {
	start := time.Now()
	// wait until StartAt (bounded: never more than 5s late)
	if wait := plan.StartAt.Sub(time.Now()); wait > 0 {
		if wait > 5*time.Second {
			wait = 5 * time.Second
		}
		time.Sleep(wait)
	}
	var lastErr string
	for i := 0; i < plan.Attempts; i++ {
		conn, err := net.DialTimeout("tcp", plan.PeerAddr, plan.DialTimeout)
		if err == nil {
			onConnected(conn, PunchOutcome{
				SessionID: plan.SessionID,
				Success:   true,
				Addr:      plan.PeerAddr,
				Elapsed:   time.Since(start),
			})
			return
		}
		lastErr = err.Error()
		time.Sleep(plan.Interval)
	}
	onConnected(nil, PunchOutcome{
		SessionID: plan.SessionID,
		Success:   false,
		Addr:      plan.PeerAddr,
		Err:       lastErr,
		Elapsed:   time.Since(start),
	})
}

// cleanupLocked drops stale pending lists (TTL) and rate-limit memory.
// Caller holds pm.mu.
func (pm *PunchManager) cleanupLocked() {
	now := pm.now()
	for target, since := range pm.pendingSince {
		if now.Sub(since) > punchPendingTTL {
			delete(pm.pending, target)
			delete(pm.pendingSince, target)
		}
	}
	if len(pm.lastRegister) > 4096 {
		pm.lastRegister = map[string]time.Time{}
	}
}

// EndpointOf returns the observed public endpoint for a node (rendezvous use).
func (pm *PunchManager) EndpointOf(nodeID string) (string, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	a, ok := pm.observed[nodeID]
	return a, ok
}

// String implements fmt.Stringer for debug.
func (o PunchOutcome) String() string {
	if o.Success {
		return fmt.Sprintf("punch#%d OK %s (%s)", o.SessionID, o.Addr, o.Elapsed.Round(time.Millisecond))
	}
	return fmt.Sprintf("punch#%d FAIL %s: %s", o.SessionID, o.Addr, o.Err)
}
