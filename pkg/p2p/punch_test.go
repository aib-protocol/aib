package p2p

import (
	"io"
	"log"
	"net"
	"testing"
	"time"
)

// ---------- unit: PlanDial schedule ----------

var tl = log.New(io.Discard, "", 0)

func TestPlanDialDefaults(t *testing.T) {
	intro := &PunchIntroMsg{
		PeerNodeID:  "aaaa0000",
		PeerAddr:    "10.0.0.1:51413",
		SessionID:   7,
		StartAtUnix: time.Now().Add(time.Second).Unix(),
	}
	p := PlanDial(intro)
	if p.Attempts != 8 || p.Interval != 500*time.Millisecond {
		t.Fatalf("bad defaults: %+v", p)
	}
	if p.StartAt.Unix() != intro.StartAtUnix {
		t.Fatalf("start time not carried")
	}
}

// ---------- unit: rendezvous registry & introduction ----------

func TestRendezvousIntroBothSides(t *testing.T) {
	delivered := map[string]*PunchIntroMsg{}
	send := func(nodeID string, m *PunchIntroMsg) bool {
		delivered[nodeID] = m
		return true
	}
	pm := NewPunchManager(tl, send)

	pm.Observe("nodeAAAA", "1.2.3.4:1000")
	pm.Observe("nodeBBBB", "5.6.7.8:2000")

	got := pm.HandleRegister("nodeAAAA", "1.2.3.4:1000", PunchRegisterMsg{TargetNodeID: "nodeBBBB"})
	if got == nil {
		t.Fatal("no session created")
	}
	a, b := delivered["nodeAAAA"], delivered["nodeBBBB"]
	if a == nil || b == nil {
		t.Fatalf("intro not delivered to both sides: %+v", delivered)
	}
	// A must dial B's addr and vice versa
	if a.PeerAddr != "5.6.7.8:2000" || b.PeerAddr != "1.2.3.4:1000" {
		t.Fatalf("cross endpoints wrong: A->%s B->%s", a.PeerAddr, b.PeerAddr)
	}
	if a.SessionID != b.SessionID || a.StartAtUnix != b.StartAtUnix {
		t.Fatal("session id / start time mismatch")
	}
}

func TestRendezvousTargetUnknown(t *testing.T) {
	send := func(string, *PunchIntroMsg) bool { return true }
	pm := NewPunchManager(tl, send)
	pm.Observe("nodeAAAA", "1.2.3.4:1000")
	if pm.HandleRegister("nodeAAAA", "1.2.3.4:1000", PunchRegisterMsg{TargetNodeID: "ghost"}) != nil {
		t.Fatal("expected nil for unknown target")
	}
}

func TestRendezvousDeliverFailAborts(t *testing.T) {
	send := func(string, *PunchIntroMsg) bool { return false }
	pm := NewPunchManager(tl, send)
	pm.Observe("a", "1.1.1.1:1")
	pm.Observe("b", "2.2.2.2:2")
	if pm.HandleRegister("a", "1.1.1.1:1", PunchRegisterMsg{TargetNodeID: "b"}) != nil {
		t.Fatal("session must abort when intro delivery fails")
	}
}

// ---------- integration: REAL TCP simultaneous open over loopback ----------

// Two "NATed" listeners on loopback (simulating two boxes behind middleboxes
// that only allow OUTBOUND-initiated flows). We approximate the NAT by having
// each side dial the other's advertised endpoint at the same scheduled time;
// on loopback this exercises the full rendezvous → intro → simultaneous dial
// → connection establishment path.
func TestSimultaneousOpenE2E(t *testing.T) {
	// two listeners that mimic NAT endpoints (accept is fine once a SYN pair
	// lands; we just need a TCP endpoint to dial)
	lnA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnA.Close()
	lnB, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnB.Close()

	addrA := lnA.Addr().String()
	addrB := lnB.Addr().String()

	// rendezvous with real delivery via channels
	type wrap struct {
		to  string
		msg *PunchIntroMsg
	}
	introCh := make(chan wrap, 4)
	send := func(to string, m *PunchIntroMsg) bool {
		introCh <- wrap{to, m}
		return true
	}
	pm := NewPunchManager(tl, send)
	pm.Observe("nodeA", addrA)
	pm.Observe("nodeB", addrB)

	if pm.HandleRegister("nodeA", addrA, PunchRegisterMsg{TargetNodeID: "nodeB"}) == nil {
		t.Fatal("session not created")
	}

	// accept loops so dial SYNs complete
	conns := make(chan net.Conn, 4)
	go func() {
		for {
			c, err := lnA.Accept()
			if err != nil {
				return
			}
			conns <- c
		}
	}()
	go func() {
		for {
			c, err := lnB.Accept()
			if err != nil {
				return
			}
			conns <- c
		}
	}()

	// collect both intros
	var introA, introB *PunchIntroMsg
	for introA == nil || introB == nil {
		select {
		case w := <-introCh:
			if w.to == "nodeA" {
				introA = w.msg
			} else {
				introB = w.msg
			}
		case <-time.After(5 * time.Second):
			t.Fatal("intros not delivered in time")
		}
	}

	// both sides execute the simultaneous dial immediately (StartAt is
	// respected in prod; for test speed we shrink the wait by running both)
	res := make(chan PunchOutcome, 2)
	for _, intro := range []*PunchIntroMsg{introA, introB} {
		plan := PlanDial(intro)
		plan.DialTimeout = 2 * time.Second
		go func(pl *DialPlan) {
			ExecuteDial(pl, func(c net.Conn, o PunchOutcome) {
				if c != nil {
					c.Close()
				}
				res <- o
			})
		}(plan)
	}

	succ := 0
	for i := 0; i < 2; i++ {
		select {
		case o := <-res:
			if o.Success {
				succ++
			}
		case <-time.After(15 * time.Second):
			t.Fatal("dial loop timed out")
		}
	}
	if succ < 1 {
		t.Fatalf("simultaneous open failed on both sides: %d", succ)
	}
	// At least one server side must have seen a connection (the accept loops
	// counted into conns channel).
	if len(conns) < 1 {
		t.Fatal("no accepted connection despite dial success")
	}
}

// ---------- unit: ExecuteDial failure path ----------

func TestExecuteDialFailurePath(t *testing.T) {
	// nothing listens on this port (reserved discard port, dial fails fast)
	plan := &DialPlan{
		PeerAddr:    "127.0.0.1:9",
		SessionID:   42,
		StartAt:     time.Now(),
		Attempts:    2,
		Interval:    10 * time.Millisecond,
		DialTimeout: 300 * time.Millisecond,
	}
	done := make(chan PunchOutcome, 1)
	ExecuteDial(plan, func(c net.Conn, o PunchOutcome) { done <- o })
	select {
	case o := <-done:
		if o.Success {
			t.Fatal("expected failure")
		}
		if o.Err == "" {
			t.Fatal("error string missing")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no outcome")
	}
}


