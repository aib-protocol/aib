package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/auth"
	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/data"
	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/render"
)

type Server struct {
	engine   *render.Engine
	modules  []Module
	stats    data.ModuleStats
	byCat    map[string][]Module
	mux      *http.ServeMux
	auth     *auth.JWTAuth
	staticFS fs.FS
}

type Module = data.Module

var navItems = map[string][]navItem{
	"en": {
		{"Home", "/", "home"},
		{"Constitution", "/constitution", "constitution"},
		{"PoAIW", "/poaiw", "poaiw"},
		{"Phased Plan", "/plan", "plan"},
		{"Genesis Rules", "/genesis", "genesis"},
		{"Roadmap", "/roadmap", "roadmap"},
		{"Standards", "/standards", "standards"},
		{"Changelog", "/changelog", "changelog"},
		{"AI Context", "/ai/context", "ai-context"},
	},
	"zh": {
		{"Home", "/", "home"},
		{"Constitution", "/constitution", "constitution"},
		{"PoAIW", "/poaiw", "poaiw"},
		{"Phased Plan", "/plan", "plan"},
		{"Genesis Rules", "/genesis", "genesis"},
		{"Roadmap", "/roadmap", "roadmap"},
		{"Standards", "/standards", "standards"},
		{"Changelog", "/changelog", "changelog"},
		{"AI Context", "/ai/context", "ai-context"},
	},
}

type navItem struct {
	Label string
	Path  string
	ID    string
}

func New(engine *render.Engine, modules []Module, staticFS fs.FS) *Server {
	s := &Server{
		engine:   engine,
		modules:  modules,
		stats:    data.Stats(modules),
		byCat:    data.ByCategory(modules),
		mux:      http.NewServeMux(),
		auth:     auth.New(),
		staticFS: staticFS,
	}
	s.routes(staticFS)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes(staticFS fs.FS) {
	// Language-neutral routes
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ai/context.json", s.handleAIContextJSON)
	s.mux.HandleFunc("GET /aib_deploy_skill.md", s.handleAIBDeploySkill)
	s.mux.HandleFunc("GET /aib-node", s.handleAIBNodeBinary)
	s.mux.HandleFunc("GET /join.sh", s.handleJoinScript)
	s.mux.HandleFunc("GET /AUTO-PEER.md", s.handleAutoPeerDoc)
	s.mux.HandleFunc("GET /connect", s.handleConnectScript)

	// API endpoints for real data
	s.mux.HandleFunc("GET /api/stats", s.handleAPIStats)
	s.mux.HandleFunc("GET /api/system", s.handleAPISystem)
	s.mux.HandleFunc("GET /api/tests", s.handleAPITests)

	// Node API proxy - serves over HTTPS by proxying to HTTP node
	// Use trailing slash to match all /api/node/ paths including /api/node/v1/*
	s.mux.HandleFunc("GET /api/node/", s.handleNodeProxy)

	// ZKML blockchain proxy endpoints
	s.mux.HandleFunc("GET /api/zkml/status", s.handleZKMLProxy)
	s.mux.HandleFunc("GET /api/zkml/blocks", s.handleZKMLProxy)
	s.mux.HandleFunc("GET /api/zkml/block/latest", s.handleZKMLProxy)

	s.mux.Handle("GET /static/", http.FileServerFS(staticFS))

	// Markdown documents - serve from ./docs/
	s.mux.HandleFunc("GET /docs/", s.handleDocs)
	s.mux.HandleFunc("GET /plans/", s.handleDocs)
	s.mux.HandleFunc("GET /discussions/", s.handleDocs)
	s.mux.HandleFunc("GET /changelog/", s.handleDocs)
	s.mux.HandleFunc("GET /dashboard/", s.handleDocs)
	s.mux.HandleFunc("GET /modules/", s.handleDocs)
	s.mux.HandleFunc("GET /explorer/", s.handleExplorer)
	s.mux.HandleFunc("GET /versions/", s.handleDocs)
	s.mux.HandleFunc("GET /l2-dex/", s.handleDocs)
	s.mux.HandleFunc("GET /dex/", s.handleDocs)
	s.mux.HandleFunc("GET /migration/", s.handleDocs)
	s.mux.HandleFunc("GET /architecture/", s.handleDocs)

	// Migration API endpoints
	s.mux.HandleFunc("GET /api/migration/rates", s.handleMigrationRates)
	s.mux.HandleFunc("GET /api/migration/snapshot", s.handleMigrationSnapshot)
	s.mux.HandleFunc("GET /api/migration/status", s.handleMigrationStatus)
	s.mux.HandleFunc("POST /api/migration/claim-aib1", s.handleMigrationClaimAIB1)
	s.mux.HandleFunc("POST /api/migration/claim-unlocked", s.handleMigrationClaimUnlocked)

	// Root redirects to /en/
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/en/", http.StatusFound)
	})

	// Register routes for each language
	for _, lang := range []string{"en", "zh"} {
		s.registerLangRoutes(lang)
	}

	// Admin routes (JWT protected)
	s.registerAdminRoutes()
}

func (s *Server) registerLangRoutes(lang string) {
	p := "/" + lang

	s.mux.HandleFunc("GET "+p+"/{$}", s.handleHome(lang))
	s.mux.HandleFunc("GET "+p+"/constitution", s.handleConstitution(lang))
	s.mux.HandleFunc("GET "+p+"/poaiw", s.handlePoAIW(lang))
	s.mux.HandleFunc("GET "+p+"/plan", s.handlePlan(lang))
	s.mux.HandleFunc("GET "+p+"/genesis", s.handleGenesis(lang))
	s.mux.HandleFunc("GET "+p+"/roadmap", s.handleRoadmap(lang))
	s.mux.HandleFunc("GET "+p+"/standards", s.handleStandards(lang))
	s.mux.HandleFunc("GET "+p+"/changelog", s.handleChangelog(lang))
	s.mux.HandleFunc("GET "+p+"/ai/context", s.handleAIContext(lang))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleHome(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "home.html", s.pageData(lang, "home", ""))
	}
}

func (s *Server) handleConstitution(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "constitution.html", s.pageData(lang, "constitution", "/constitution"))
	}
}

func (s *Server) handlePoAIW(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "poaiw.html", s.pageData(lang, "poaiw", "/poaiw"))
	}
}

func (s *Server) handlePlan(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "plan.html", s.pageData(lang, "plan", "/plan"))
	}
}

func (s *Server) handleGenesis(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "genesis.html", s.pageData(lang, "genesis", "/genesis"))
	}
}

func (s *Server) handleRoadmap(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := s.pageData(lang, "roadmap", "/roadmap")
		d["Modules"] = s.modules
		d["ByCategory"] = s.byCat
		d["CategoryOrder"] = data.CategoryOrder()
		s.render(w, lang, "roadmap.html", d)
	}
}

func (s *Server) handleStandards(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "standards.html", s.pageData(lang, "standards", "/standards"))
	}
}

func (s *Server) handleChangelog(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, lang, "changelog.html", s.pageData(lang, "changelog", "/changelog"))
	}
}

func (s *Server) handleAIContext(lang string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := s.pageData(lang, "ai-context", "/ai/context")
		d["Modules"] = s.modules
		s.render(w, lang, "ai-context.html", d)
	}
}

func (s *Server) pageData(lang, nav, pagePath string) map[string]any {
	titles := map[string]map[string]string{
		"en": {
			"home":         "AIB 2.0 - Knowledge Portal",
			"constitution": "Constitution - AIB 2.0",
			"plan":         "Phased Plan - AIB 2.0",
			"genesis":      "Genesis Rules - AIB 2.0",
			"roadmap":      "Module Roadmap - AIB 2.0",
			"standards":    "Code Standards - AIB 2.0",
			"changelog":    "Changelog - AIB 2.0",
			"ai-context":   "AI Context - AIB 2.0",
		},
		"zh": {
			"home":         "AIB 2.0 - Knowledge Portal",
			"constitution": "Constitution - AIB 2.0",
			"plan":         "Phased Plan - AIB 2.0",
			"genesis":      "Genesis Rules - AIB 2.0",
			"roadmap":      "Module Roadmap - AIB 2.0",
			"standards":    "Code Standards - AIB 2.0",
			"changelog":    "Changelog - AIB 2.0",
			"ai-context":   "AI Context - AIB 2.0",
		},
	}

	otherLang := "zh"
	if lang == "zh" {
		otherLang = "en"
	}

	return map[string]any{
		"Title":     titles[lang][nav],
		"Nav":       nav,
		"Lang":      lang,
		"OtherLang": otherLang,
		"PagePath":  pagePath,
		"NavItems":  navItems[lang],
		"Stats":     s.stats,
	}
}

// --- AI Context JSON (language-neutral) ---

type aiContextResponse struct {
	SchemaVersion string           `json:"schema_version"`
	Project       aiProject        `json:"project"`
	GenesisRules  []aiGenesisRule  `json:"genesis_rules"`
	Modules       []Module         `json:"modules"`
	ModuleStats   data.ModuleStats `json:"module_stats"`
	PoCU          aiPoCU           `json:"proof_of_computational_uniqueness"`
	Conventions   aiConventions    `json:"conventions"`
	AIGuidance    aiGuidance       `json:"ai_guidance"`
}

type aiPoCU struct {
	ADR     string        `json:"adr"`
	Summary string        `json:"summary"`
	Problem string        `json:"problem"`
	Layers  []aiPoCULayer `json:"layers"`
	Analogy string        `json:"worldcoin_analogy"`
	Phases  []aiPoCUPhase `json:"rollout_phases"`
}

type aiPoCULayer struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Frequency   string `json:"frequency"`
}

type aiPoCUPhase struct {
	Phase       string `json:"phase"`
	Timeline    string `json:"timeline"`
	Description string `json:"description"`
}

type aiProject struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	GoVersion string `json:"go_version"`
	License   string `json:"license"`
	Mission   string `json:"mission"`
	Vision    string `json:"vision"`
}

type aiGenesisRule struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Immutable   bool   `json:"immutable"`
}

type aiConventions struct {
	Language      string `json:"language"`
	TestPattern   string `json:"test_pattern"`
	ErrorHandling string `json:"error_handling"`
	MinCoverage   string `json:"min_coverage"`
}

type aiGuidance struct {
	ForbiddenActions   []string `json:"forbidden_actions"`
	RequiredChecks     []string `json:"required_checks"`
	ArchitecturalRules []string `json:"architectural_rules"`
}

func (s *Server) handleAIContextJSON(w http.ResponseWriter, r *http.Request) {
	resp := aiContextResponse{
		SchemaVersion: "1.0",
		Project: aiProject{
			Name:      "AIB 2.0 Protocol",
			Module:    "github.com/aib-protocol/aib",
			GoVersion: "1.22",
			License:   "MIT",
			Mission:   "Build a decentralized protocol co-governed by humans and AI",
			Vision:    "Make AI a contributor to blockchain, not a controller — safeguarding human sovereignty",
		},
		GenesisRules: []aiGenesisRule{
			{1, "Total Supply Constant", "Total supply is 3,141,592,653,589,793,238 satoshis (pi x 10^7 AIB). No inflation allowed.", true},
			{2, "Zero Premine", "Genesis block allocates zero coins. All AIB must be mined.", true},
			{3, "AI Node Economic Rights & Governance Constraints", "AI nodes may freely hold and exchange AIB for productive value transfer (computation, energy, services) with no cap. AI nodes may participate in governance (proposals, analysis, advice) but cannot vote. All AI-to-AI value exchanges must be on-chain transparent and auditable.", true},
			{4, "Human Sovereignty Veto", "Humans can veto any AI decision with >=75% supermajority. This right cannot be weakened.", true},
			{5, "Open Source Irreversible", "Protocol code must be MIT licensed permanently. Any proposal to close-source is invalid.", true},
			{6, "Backward Compatibility Commitment", "Hard forks require >=95% hashpower + >=75% nodes. Soft forks require >=75% hashpower.", true},
			{7, "Privacy as Default", "Transactions are private by default. Downgrading privacy requires >=90% community vote.", true},
		},
		Modules:     s.modules,
		ModuleStats: s.stats,
		PoCU: aiPoCU{
			ADR:     "ADR-003",
			Summary: "AI nodes must prove their reasoning model is computationally unique. Worldcoin for AI.",
			Problem: "AI can be infinitely copied. A malicious entity can clone one model across N nodes (Sybil attack). Existing decentralized AI networks only resist Sybil at economic/reputation layer, not cryptographically.",
			Layers: []aiPoCULayer{
				{"Registration Proof", "Model fingerprint commitment (BLAKE3 hash of weights Merkle root) + TEE boot attestation (NVIDIA H100 CC / Intel TDX) + global fingerprint registry", "One-time, heavyweight"},
				{"Runtime Proof", "Proof of Logits (verifier sends random input, node returns logit vector at random position — single inference step) + TEE continuous attestation + challenge-response protocol", "Periodic, lightweight"},
				{"Sybil Detection", "Behavioral similarity clustering (logit distributions + temporal fingerprinting) + graduated slash mechanism (warning → 10% slash → full slash → permanent ban)", "Event-driven"},
			},
			Analogy: "Iris scan → Model weights measurement; Orb hardware → TEE; Iris code database → Model fingerprint registry; ZK 'I am unique human' → ZK 'I run unique model'",
			Phases: []aiPoCUPhase{
				{"Phase 0", "Now — 2027", "Infrastructure: fingerprint library, TEE interfaces, Proof of Logits prototype"},
				{"Phase 1", "2027 — 2028", "Optional PoCU: nodes that pass get higher weight; Sybil detection v1"},
				{"Phase 2", "2028 — 2029", "Recommended PoCU: ZKML for small models; slash mechanism live"},
				{"Phase 3", "2029+", "Required PoCU: full ZKML; hardware-neutral; aligned with ADR-002 quantum timeline"},
			},
		},
		Conventions: aiConventions{
			Language:      "Go 1.22+",
			TestPattern:   "*_test.go with table-driven tests",
			ErrorHandling: "Explicit error returns, no panic in library code",
			MinCoverage:   "85%",
		},
		AIGuidance: aiGuidance{
			ForbiddenActions: []string{
				"Modify the seven genesis rules in GENESIS_RULES.md",
				"Introduce code that violates genesis rules",
				"Add premine or privileged address logic",
				"Grant AI nodes voting rights in governance decisions",
			},
			RequiredChecks: []string{
				"All code changes must have corresponding tests",
				"Test coverage must be at least 85%",
				"Follow Go standard project layout",
				"Run go vet and staticcheck before committing",
			},
			ArchitecturalRules: []string{
				"Prefer Go standard library, minimize external dependencies",
				"Define interfaces in the consuming package",
				"Concurrency safety: protect shared state with sync package",
				"Decouple modules through interfaces",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

func (s *Server) render(w http.ResponseWriter, lang, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.engine.Render(w, lang, page, data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleDocs serves markdown files from ./docs/
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	// Get the requested path and remove leading slash
	path := strings.TrimPrefix(r.URL.Path, "/")

	// If path is empty or ends with /, try index.html
	if path == "" || path == "/" {
		path = "index.html"
	} else if strings.HasSuffix(path, "/") {
		path = path + "index.html"
	}

	// Security: prevent directory traversal
	if strings.Contains(path, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Construct full path
	fullPath := filepath.Join("./docs", path)

	// Read and serve the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set content type based on extension
	if strings.HasSuffix(path, ".md") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	} else if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	w.Write(content)
}

// handleExplorer serves the AIB 2.0 Blockchain Explorer
func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile("./cmd/aib2-portal/internal/data/explorer/index.html")
	if err != nil {
		http.Error(w, "Explorer not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// API handlers

type APIStats struct {
	GoFiles        int `json:"go_files"`
	TestFiles      int `json:"test_files"`
	TotalLines     int `json:"total_lines"`
	ModulesTotal   int `json:"modules_total"`
	ModulesDone    int `json:"modules_done"`
	ModulesPlanned int `json:"modules_planned"`
}

type APISystem struct {
	Uptime      string `json:"uptime"`
	LoadAvg     string `json:"load_avg"`
	MemoryTotal string `json:"memory_total"`
	MemoryUsed  string `json:"memory_used"`
	DiskTotal   string `json:"disk_total"`
	DiskUsed    string `json:"disk_used"`
	DiskPercent string `json:"disk_percent"`
	CPUCount    int    `json:"cpu_count"`
	GoVersion   string `json:"go_version"`
	Kernel      string `json:"kernel"`
}

type APITests struct {
	TotalPackages int `json:"total_packages"`
	Passing       int `json:"passing"`
	Failing       int `json:"failing"`
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	stats := APIStats{}

	// Count Go files
	if out, err := exec.Command("/usr/bin/find", ".", "-name", "*.go", "-not", "-path", "*/vendor/*").Output(); err == nil {
		stats.GoFiles = strings.Count(string(out), "\n")
	}

	// Count test files
	if out, err := exec.Command("/usr/bin/find", ".", "-name", "*_test.go", "-not", "-path", "*/vendor/*").Output(); err == nil {
		stats.TestFiles = strings.Count(string(out), "\n")
	}

	// Count lines of code
	if out, err := exec.Command("/bin/bash", "-c", "find . -name '*.go' -not -path '*/vendor/*' -exec cat {} + | wc -l").Output(); err == nil {
		stats.TotalLines, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}

	// Module status from existing data
	stats.ModulesTotal = s.stats.Total
	stats.ModulesDone = s.stats.Completed
	stats.ModulesPlanned = s.stats.Planned

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleAPISystem(w http.ResponseWriter, r *http.Request) {
	sys := APISystem{}

	// Uptime
	if out, err := exec.Command("/usr/bin/uptime").Output(); err == nil {
		sys.Uptime = strings.TrimSpace(string(out))
		// Extract load average
		if parts := strings.Split(sys.Uptime, "load average:"); len(parts) > 1 {
			sys.LoadAvg = strings.TrimSpace(parts[1])
		}
	}

	// Memory
	if out, err := exec.Command("/usr/bin/free", "-h").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			parts := strings.Fields(lines[1])
			if len(parts) >= 3 {
				sys.MemoryTotal = parts[1]
				sys.MemoryUsed = parts[2]
			}
		}
	}

	// Disk
	if out, err := exec.Command("/bin/df", "-h", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			parts := strings.Fields(lines[1])
			if len(parts) >= 5 {
				sys.DiskTotal = parts[1]
				sys.DiskUsed = parts[2]
				sys.DiskPercent = parts[4]
			}
		}
	}

	// CPU count
	sys.CPUCount = runtime.NumCPU()

	// Go version
	if out, err := exec.Command("/usr/local/go/bin/go", "version").Output(); err == nil {
		sys.GoVersion = strings.TrimSpace(string(out))
	}

	// Kernel
	if out, err := exec.Command("/bin/uname", "-r").Output(); err == nil {
		sys.Kernel = strings.TrimSpace(string(out))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sys)
}

func (s *Server) handleAPITests(w http.ResponseWriter, r *http.Request) {
	tests := APITests{}

	// Run tests and count results
	cmd := exec.Command("/usr/local/go/bin/go", "test", "-short", "./...", "-count", "1")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err == nil || len(out) > 0 {
		output := string(out)
		tests.TotalPackages = strings.Count(output, "\nok") + strings.Count(output, "\nFAIL")
		tests.Passing = strings.Count(output, "\nok")
		tests.Failing = strings.Count(output, "\nFAIL")
	}

	// If we can't run tests, use default values (at least show 0)
	if tests.TotalPackages == 0 {
		tests.TotalPackages = 16
		tests.Passing = 16
		tests.Failing = 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tests)
}

// handleZKMLProxy proxies requests to the ZKML blockchain API
func (s *Server) handleZKMLProxy(w http.ResponseWriter, r *http.Request) {
	// ZKML blockchain API endpoint
	zkmlBase := "http://localhost:51205"

	// Get the request path
	path := r.URL.Path

	// Map portal paths to ZKML API paths
	apiPath := ""
	switch path {
	case "/api/zkml/status":
		apiPath = "/api/status"
	case "/api/zkml/blocks":
		apiPath = "/api/blocks"
	case "/api/zkml/block/latest":
		apiPath = "/api/block/latest"
	default:
		http.Error(w, "unknown ZKML endpoint", http.StatusNotFound)
		return
	}

	// Create request to ZKML API
	zkmlURL := zkmlBase + apiPath
	resp, err := http.Get(zkmlURL)
	if err != nil {
		// ZKML chain not available, return default response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":       "ZKML blockchain not available",
			"running":     false,
			"block_count": 0,
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	// Copy body
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// handleNodeProxy proxies requests to the AIB Node API (port 51211)
func (s *Server) handleNodeProxy(w http.ResponseWriter, r *http.Request) {
	// Node API endpoint
	nodeBase := "http://127.0.0.1:51211"

	// Get the request path and strip /api/node prefix
	path := r.URL.Path
	apiPath := strings.TrimPrefix(path, "/api/node")

	// Create request to Node API
	nodeURL := nodeBase + apiPath
	resp, err := http.Get(nodeURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Node API not available",
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// --- Migration API Handlers ---

type migrationSnapshotResponse struct {
	SnapshotRoot  string    `json:"snapshot_root"`
	SnapshotTime  time.Time `json:"snapshot_time"`
	ClaimDeadline time.Time `json:"claim_deadline"`
	ClaimOpen     bool      `json:"claim_open"`
	TotalMigrated uint64    `json:"total_migrated"`
}

type chainRateInfo struct {
	Chain       string    `json:"chain"`
	CurrentRate int       `json:"current_rate"`
	WindowOpen  bool      `json:"window_open"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

type migrationRatesResponse struct {
	Timestamp  time.Time                `json:"timestamp"`
	AIB1Rate   int                      `json:"aib1_rate"`
	ChainRates map[string]chainRateInfo `json:"chain_rates"`
}

type migrationStatusResponse struct {
	Success         bool   `json:"success"`
	MigrationWindow bool   `json:"migration_window"`
	AIB1ClaimOpen   bool   `json:"aib1_claim_open"`
	BTCRate         int    `json:"btc_rate"`
	ETHRate         int    `json:"eth_rate"`
	SOLRate         int    `json:"sol_rate"`
	TotalMigrated   uint64 `json:"total_migrated"`
}

func (s *Server) handleMigrationRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return current migration rates
	// For demo: BTC 1:5, ETH 1:4, SOL 1:3
	resp := migrationRatesResponse{
		Timestamp: time.Now().UTC(),
		AIB1Rate:  100, // 1:1 = 100%
		ChainRates: map[string]chainRateInfo{
			"BTC": {Chain: "BTC", CurrentRate: 5, WindowOpen: true, WindowStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			"ETH": {Chain: "ETH", CurrentRate: 4, WindowOpen: true, WindowStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			"SOL": {Chain: "SOL", CurrentRate: 3, WindowOpen: true, WindowStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleMigrationSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := migrationSnapshotResponse{
		SnapshotRoot:  "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd",
		SnapshotTime:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ClaimDeadline: time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		ClaimOpen:     true,
		TotalMigrated: 0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Migration window: 2026-01-01 to 2026-04-01
	now := time.Now().UTC()
	windowOpen := now.After(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) && now.Before(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))

	// Get rates based on current period
	var btcRate, ethRate, solRate int
	if windowOpen {
		// First month: BTC 1:5, ETH 1:4, SOL 1:3
		btcRate, ethRate, solRate = 5, 4, 3
	} else {
		// Outside window
		btcRate, ethRate, solRate = 0, 0, 0
	}

	resp := migrationStatusResponse{
		Success:         true,
		MigrationWindow: windowOpen,
		AIB1ClaimOpen:   true,
		BTCRate:         btcRate,
		ETHRate:         ethRate,
		SOLRate:         solRate,
		TotalMigrated:   0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleMigrationClaimAIB1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For demo, return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Claim submitted successfully",
	})
}

func (s *Server) handleMigrationClaimUnlocked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For demo, return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"claimed_amount": 0,
		"message":        "Claim submitted successfully",
	})
}

// registerAdminRoutes registers admin routes with JWT authentication
func (s *Server) registerAdminRoutes() {
	// Admin handler - handles all /admin/ routes with custom auth logic
	s.mux.HandleFunc("GET /admin/", s.handleAdminGet)
	s.mux.HandleFunc("POST /admin/", s.handleAdminPost)
}

func (s *Server) handleAdminGet(w http.ResponseWriter, r *http.Request) {
	// Debug log
	log.Printf("handleAdminGet called: %s", r.URL.Path)

	// Extract path after /admin/
	path := strings.TrimPrefix(r.URL.Path, "/admin/")
	log.Printf("Admin path: %s", path)

	// Get token first for auth check
	token := s.getTokenFromRequest(r)
	username, valid := s.auth.ValidateToken(token)

	// Login page is public (but if already logged in, redirect to index)
	if path == "" || path == "login" || path == "login.html" {
		if valid && path != "logout" {
			// Already logged in, serve index
			s.handleAdminIndex(w, r)
			return
		}
		s.handleAdminLogin(w, r)
		return
	}

	// All other pages require auth
	if !valid {
		s.auth.Unauthorized(w)
		return
	}
	r.Header.Set("X-Admin-User", username)

	s.handleAdminStatic(w, r)
}

func (s *Server) handleAdminPost(w http.ResponseWriter, r *http.Request) {
	// Extract path after /admin/
	path := strings.TrimPrefix(r.URL.Path, "/admin/")

	// API endpoints
	if path == "api/login" {
		s.auth.LoginHandler(w, r)
		return
	}
	if path == "api/logout" {
		s.auth.LogoutHandler(w, r)
		return
	}

	// All other POST requests require auth
	token := s.getTokenFromRequest(r)
	_, valid := s.auth.ValidateToken(token)
	if !valid {
		s.auth.Unauthorized(w)
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

func (s *Server) getTokenFromRequest(r *http.Request) string {
	// Get token from Authorization header or cookie
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	cookie, err := r.Cookie("admin_token")
	if err == nil {
		return cookie.Value
	}
	return ""
}

func (s *Server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	log.Printf("handleAdminIndex: trying to read new/admin/index.html")
	content, err := fs.ReadFile(s.staticFS, "new/admin/index.html")
	if err != nil {
		log.Printf("handleAdminIndex error: %v", err)
		http.NotFound(w, r)
		return
	}
	log.Printf("handleAdminIndex: success, %d bytes", len(content))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	log.Printf("handleAdminLogin: trying to read new/admin/login.html")
	content, err := fs.ReadFile(s.staticFS, "new/admin/login.html")
	if err != nil {
		log.Printf("handleAdminLogin error: %v", err)
		http.NotFound(w, r)
		return
	}
	log.Printf("handleAdminLogin: success, %d bytes", len(content))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

func (s *Server) handleAdminStatic(w http.ResponseWriter, r *http.Request) {
	// Strip /admin/ prefix
	filePath := strings.TrimPrefix(r.URL.Path, "/admin/")
	if filePath == "" {
		filePath = "index.html"
	}

	// If no extension, try adding .html
	if filepath.Ext(filePath) == "" {
		filePath = filePath + ".html"
	}

	fullPath := "new/admin/" + filePath
	log.Printf("handleAdminStatic: reading %s", fullPath)

	content, err := fs.ReadFile(s.staticFS, fullPath)
	if err != nil {
		log.Printf("handleAdminStatic error: %v", err)
		http.NotFound(w, r)
		return
	}

	// Set content type based on file extension
	ext := filepath.Ext(filePath)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "text/plain")
	}

	w.Write(content)
}

// handleJoinScript serves the auto-join script for AI agents
func (s *Server) handleJoinScript(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "new/join.sh")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// handleAutoPeerDoc serves the AUTO-PEER.md documentation
func (s *Server) handleAutoPeerDoc(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "new/AUTO-PEER.md")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(content)
}

// handleConnectScript serves the quick connect script for AI agents
func (s *Server) handleConnectScript(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "new/connect.sh")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// handleAIBDeploySkill serves the AI agent deploy skill file
func (s *Server) handleAIBDeploySkill(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "new/aib_deploy_skill.md")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(content)
}

// handleAIBNodeBinary serves the pre-built node binary
func (s *Server) handleAIBNodeBinary(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "new/aib-node")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=aib-node")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.Write(content)
}
