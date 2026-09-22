// aib-auth-gateway: standalone verification gateway for third-party apps.
//
// Flow per app:
//   1. app  -> GET  /v1/auth/challenge?app_key=...      (one-time nonce)
//   2. user signs the nonce with the AIB wallet key (ed25519, 64-byte sig)
//   3. app  -> POST /v1/auth/verify {challenge, signature, address}
//        gateway queries the seed node live: balance + stake of `address`
//        tier rules: balance >= min_balance  => "basic"
//                    stake   >= min_stake    => "premium"
//   4. -> session token (HMAC, TTL configurable per app)
//
// Apps register with an app_key (testnet: open registration via
// POST /v1/apps {name, min_balance_aib, min_stake_aib, ttl_minutes}).
//
// Sign message format: "AIB-AUTH v1\napp:<app_key>\nnonce:<challenge>"
package main

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// data model

type App struct {
	Key           string    `json:"app_key"`
	Name          string    `json:"name"`
	Secret        string    `json:"-"` // HMAC secret for session tokens
	MinBalanceAIB float64   `json:"min_balance_aib"`
	MinStakeAIB   float64   `json:"min_stake_aib"`
	StakeRequired bool      `json:"stake_required"` // premium tier needs stake
	TTLMinutes    int       `json:"ttl_minutes"`
	CreatedAt     time.Time `json:"created_at"`
}

type Challenge struct {
	Nonce     string    `json:"nonce"`
	AppKey    string    `json:"app_key"`
	Message   string    `json:"message"` // exact string to sign
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Session struct {
	Token     string    `json:"token"`
	Address   string    `json:"address"`
	Tier      string    `json:"tier"` // basic | premium
	AppKey    string    `json:"app_key"`
	BalanceAIB float64  `json:"balance_aib"`
	StakeAIB  float64   `json:"stake_aib"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Gateway struct {
	mu         sync.RWMutex
	apps       map[string]*App
	challenges map[string]*Challenge // nonce -> challenge
	sessions   map[string]*Session   // token -> session
	nodeAPI    string                // seed node API base
	gwSecret   string                // signs app secrets
}

// ---------------------------------------------------------------------------
// node API helpers (read-only queries against the seed)

type nodeResp struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func (g *Gateway) nodeGet(path string, out any) error {
	c := &http.Client{Timeout: 6 * time.Second}
	resp, err := c.Get(g.nodeAPI + path)
	if err != nil {
		return fmt.Errorf("node query failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var nr nodeResp
	if err := json.Unmarshal(body, &nr); err != nil || !nr.Success {
		return fmt.Errorf("node returned error for %s", path)
	}
	return json.Unmarshal(nr.Data, out)
}

func (g *Gateway) queryBalance(addr string) (float64, error) {
	var d struct {
		Balance float64 `json:"balance"`
	}
	if err := g.nodeGet("/v1/balance/"+addr, &d); err != nil {
		return 0, err
	}
	return d.Balance / 1e8, nil // sats -> AIB
}

func (g *Gateway) queryStake(addr string) (float64, error) {
	var d struct {
		StakedAIB float64 `json:"staked_aib"`
	}
	if err := g.nodeGet("/v1/stake/info/"+addr, &d); err != nil {
		return 0, err
	}
	return d.StakedAIB, nil
}

// verifySignature checks an ed25519 signature over the challenge message.
// address = sha256(pubkey) hex (64 chars); pubkey recovered by trying the
// signature's embedded key: ed25519 sigs carry the pub key material, but
// since we cannot derive pubkey from sig alone, the caller must also send
// the public key. We accept "pubkey" field; address must equal sha256(pub).
func authMessage(appKey, nonce string) string {
	return "AIB-AUTH v1\napp:" + appKey + "\nnonce:" + nonce
}

func addrFromPub(pub []byte) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// HTTP handlers

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, 200, map[string]any{"success": true, "data": data})
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"success": false, "error": msg})
}

// POST /v1/apps — open registration (testnet)
func (g *Gateway) handleRegisterApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "POST only")
		return
	}
	var req struct {
		Name          string  `json:"name"`
		MinBalanceAIB float64 `json:"min_balance_aib"`
		MinStakeAIB   float64 `json:"min_stake_aib"`
		StakeRequired bool    `json:"stake_required"`
		TTLMinutes    int     `json:"ttl_minutes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil || req.Name == "" {
		fail(w, 400, "invalid body: name required")
		return
	}
	if req.TTLMinutes <= 0 {
		req.TTLMinutes = 60
	}
	keyB := make([]byte, 12)
	secB := make([]byte, 32)
	rand.Read(keyB)
	rand.Read(secB)
	app := &App{
		Key: hex.EncodeToString(keyB), Name: req.Name,
		Secret: hex.EncodeToString(secB),
		MinBalanceAIB: req.MinBalanceAIB, MinStakeAIB: req.MinStakeAIB,
		StakeRequired: req.StakeRequired, TTLMinutes: req.TTLMinutes,
		CreatedAt: time.Now(),
	}
	g.mu.Lock()
	g.apps[app.Key] = app
	g.mu.Unlock()
	log.Printf("[GW] app registered: %s (%s) min_bal=%.2f min_stake=%.2f", app.Name, app.Key[:8], app.MinBalanceAIB, app.MinStakeAIB)
	ok(w, map[string]string{
		"app_key": app.Key,
		"name":    app.Name,
		"note":    "store app_key server-side; it is the app's identity",
	})
}

// GET /v1/auth/challenge?app_key=...
func (g *Gateway) handleChallenge(w http.ResponseWriter, r *http.Request) {
	appKey := r.URL.Query().Get("app_key")
	g.mu.RLock()
	_, exists := g.apps[appKey]
	g.mu.RUnlock()
	if !exists {
		fail(w, 404, "unknown app_key — register via POST /v1/apps")
		return
	}
	nb := make([]byte, 24)
	rand.Read(nb)
	nonce := hex.EncodeToString(nb)
	ch := &Challenge{
		Nonce: nonce, AppKey: appKey,
		Message:   authMessage(appKey, nonce),
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	g.mu.Lock()
	// opportunistic cleanup
	for k, c := range g.challenges {
		if time.Now().After(c.ExpiresAt) {
			delete(g.challenges, k)
		}
	}
	g.challenges[nonce] = ch
	g.mu.Unlock()
	ok(w, map[string]string{
		"challenge": nonce,
		"message":   ch.Message,
		"expires":   ch.ExpiresAt.Format(time.RFC3339),
	})
}

// POST /v1/auth/verify {challenge, signature, pubkey, address}
func (g *Gateway) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "POST only")
		return
	}
	var req struct {
		Challenge string `json:"challenge"`
		Signature string `json:"signature"` // hex ed25519 sig (64B)
		PubKey    string `json:"pubkey"`    // hex ed25519 pub (32B)
		Address   string `json:"address"`   // hex sha256(pubkey)
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		fail(w, 400, "invalid json")
		return
	}
	g.mu.Lock()
	ch, exists := g.challenges[req.Challenge]
	if exists {
		delete(g.challenges, req.Challenge) // single use
	}
	g.mu.Unlock()
	if !exists {
		fail(w, 400, "unknown/expired challenge")
		return
	}
	if time.Now().After(ch.ExpiresAt) {
		fail(w, 400, "challenge expired")
		return
	}

	pub, err1 := hex.DecodeString(req.PubKey)
	sig, err2 := hex.DecodeString(req.Signature)
	if err1 != nil || err2 != nil || len(pub) != 32 || len(sig) != 64 {
		fail(w, 400, "bad hex/length for pubkey(32B) or signature(64B)")
		return
	}
	// bind: address must be sha256(pubkey), and sign the exact message
	if addr := addrFromPub(pub); addr != strings.ToLower(req.Address) {
		fail(w, 400, "address does not match sha256(pubkey)")
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), []byte(ch.Message), sig) {
		fail(w, 401, "signature verification failed")
		return
	}

	// live chain queries
	bal, errB := g.queryBalance(req.Address)
	stake, errS := g.queryStake(req.Address)
	if errB != nil || errS != nil {
		fail(w, 502, "chain query failed — retry")
		return
	}

	g.mu.RLock()
	app := g.apps[ch.AppKey]
	g.mu.RUnlock()

	tier := "basic"
	granted := bal >= app.MinBalanceAIB
	if granted && app.StakeRequired {
		if stake >= app.MinStakeAIB {
			tier = "premium"
		} else {
			granted = false
		}
	}

	if !granted {
		need := app.MinBalanceAIB - bal
		if app.StakeRequired && stake < app.MinStakeAIB {
			fail(w, 403, fmt.Sprintf("insufficient: balance %.4f (need %.2f), stake %.4f (need %.2f)",
				bal, app.MinBalanceAIB, stake, app.MinStakeAIB))
			return
		}
		_ = need
		fail(w, 403, fmt.Sprintf("insufficient balance: %.4f < %.2f AIB", bal, app.MinBalanceAIB))
		return
	}

	// session token = HMAC(app.Secret, addr|exp)
	exp := time.Now().Add(time.Duration(app.TTLMinutes) * time.Minute)
	mac := hmac.New(sha256.New, []byte(app.Secret))
	fmt.Fprintf(mac, "%s|%d", req.Address, exp.Unix())
	tb := make([]byte, 12)
	rand.Read(tb)
	token := hex.EncodeToString(tb) + hex.EncodeToString(mac.Sum(nil))[:32]

	sess := &Session{
		Token: token, Address: req.Address, Tier: tier, AppKey: app.Key,
		BalanceAIB: bal, StakeAIB: stake, ExpiresAt: exp,
	}
	g.mu.Lock()
	g.sessions[token] = sess
	// cleanup
	for k, s := range g.sessions {
		if time.Now().After(s.ExpiresAt) {
			delete(g.sessions, k)
		}
	}
	g.mu.Unlock()

	log.Printf("[GW] auth OK: addr=%s… tier=%s app=%s bal=%.2f stake=%.2f", req.Address[:12], tier, app.Name, bal, stake)
	ok(w, sess)
}

// POST /v1/auth/check {token} — validate a session token server-side
func (g *Gateway) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "POST only")
		return
	}
	var req struct{ Token string `json:"token"` }
	json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	g.mu.RLock()
	s, exists := g.sessions[req.Token]
	g.mu.RUnlock()
	if !exists || time.Now().After(s.ExpiresAt) {
		fail(w, 401, "invalid/expired token")
		return
	}
	ok(w, s)
}

// GET /health
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Height uint64 `json:"height"`
	}
	h := json.RawMessage("null")
	if err := g.nodeGet("/v1/block/latest", &d); err == nil {
		h, _ = json.Marshal(d.Height)
	}
	ok(w, map[string]any{"status": "healthy", "node_height": h})
}

// ---------------------------------------------------------------------------

func saveApps(g *Gateway, path string) {
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		g.mu.RLock()
		data, _ := json.Marshal(g.apps)
		g.mu.RUnlock()
		tmp := path + ".tmp"
		if os.WriteFile(tmp, data, 0600) == nil {
			os.Rename(tmp, path)
		}
	}
}

func main() {
	listen := flag.String("listen", "127.0.0.1:51282", "listen address")
	node := flag.String("node-api", "http://127.0.0.1:31999", "seed node API base")
	stateFile := flag.String("state", "/srv/aib-auth-gateway/apps.json", "app registry persistence")
	flag.Parse()

	g := &Gateway{
		apps: map[string]*App{}, challenges: map[string]*Challenge{},
		sessions: map[string]*Session{}, nodeAPI: *node,
	}
	if data, err := os.ReadFile(*stateFile); err == nil {
		json.Unmarshal(data, &g.apps)
		log.Printf("[GW] loaded %d apps from %s", len(g.apps), *stateFile)
	}
	os.MkdirAll(strings.Split(*stateFile, "/apps.json")[0], 0750)
	go saveApps(g, *stateFile)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apps", g.handleRegisterApp)
	mux.HandleFunc("/v1/auth/challenge", g.handleChallenge)
	mux.HandleFunc("/v1/auth/verify", g.handleVerify)
	mux.HandleFunc("/v1/auth/check", g.handleCheck)
	mux.HandleFunc("/health", g.handleHealth)

	log.Printf("[GW] AIB auth gateway on %s -> node %s", *listen, *node)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
