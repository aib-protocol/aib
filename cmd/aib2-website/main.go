package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	languages = map[string]bool{
		"en": true, "zh": true, "zh-tw": true,
		"fr": true, "ja": true, "ms": true,
		"th": true, "es": true, "de": true, "ru": true,
	}
	subPages = map[string]bool{
		"quickstart": true, "genesis": true,
		"constitution": true, "memories": true,
		"admin":                     true,
		"human-quickstart-document": true,
		"ai-install":                true,
		"peers":                     true,
	}
	adminPages = map[string]string{
		"":      "index.html",
		"login": "login.html",
	}
)

// ============================================================================
// Peer registry - maintains all known nodes
// ============================================================================

type PeerInfo struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	Version   string    `json:"version"`
	Height    uint64    `json:"block_height"`
	LastSeen  time.Time `json:"last_seen"`
	Connected bool      `json:"connected"`
}

type PeerRegistry struct {
	mu    sync.RWMutex
	peers map[string]*PeerInfo // key = IP:Port
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{peers: make(map[string]*PeerInfo)}
}

// Heartbeat register or update a node
func (r *PeerRegistry) Heartbeat(info *PeerInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := info.Address
	info.LastSeen = time.Now()
	info.Connected = true
	r.peers[key] = info
}

// GetPeers returns all active nodes (heartbeat within 60 seconds)
func (r *PeerRegistry) GetPeers() []*PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cutoff := time.Now().Add(-60 * time.Second)
	var result []*PeerInfo
	for _, p := range r.peers {
		info := *p
		if p.LastSeen.After(cutoff) {
			info.Connected = true
		} else {
			info.Connected = false
		}
		result = append(result, &info)
	}
	return result
}

// Cleanup removes timed-out nodes (no heartbeat for 5 minutes)
func (r *PeerRegistry) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for k, p := range r.peers {
		if p.LastSeen.Before(cutoff) {
			delete(r.peers, k)
		}
	}
}

var registry = NewPeerRegistry()

func main() {
	addr := flag.String("addr", ":51234", "HTTP listen address")
	root := flag.String("root", "./cmd/aib2-portal/new", "Static files root")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(*root))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Periodically clean up timed-out nodes
	go func() {
		for {
			time.Sleep(60 * time.Second)
			registry.Cleanup()
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("aib2-website on %s (root: %s)", *addr, *root)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("stopped")
}

func handler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Heartbeat registration: POST /v1/heartbeat
		if path == "/v1/heartbeat" && r.Method == http.MethodPost {
			handleHeartbeat(w, r)
			return
		}

		// Peer list: GET /v1/peers (local registry)
		if path == "/v1/peers" && r.Method == http.MethodGet {
			handlePeers(w, r)
			return
		}

		// API proxy: /v1/* and /health* forwarded to node API (port 51211)
		if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/health") {
			proxyToNode(w, r)
			return
		}

		// Binary downloads: /downloads/* served directly from files
		if strings.HasPrefix(path, "/downloads/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			serveFile(w, r, filepath.Join(root, filepath.Clean(strings.TrimPrefix(path, "/"))))
			return
		}

		if path == "/" {
			serveFile(w, r, filepath.Join(root, "index.html"))
			return
		}

		clean := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")

		// Static file (has extension)
		if filepath.Ext(path) != "" {
			serveFile(w, r, filepath.Join(root, filepath.Clean(clean)))
			return
		}

		parts := strings.Split(clean, "/")
		switch len(parts) {
		case 1:
			// Admin special case: /admin → index.html
			if parts[0] == "admin" {
				serveFile(w, r, filepath.Join(root, "admin", "index.html"))
				return
			}
			if subPages[parts[0]] || languages[parts[0]] {
				if f := resolve(root, parts[0]); f != "" {
					serveFile(w, r, f)
					return
				}
			}
		case 2:
			// /admin/login, /admin/irc-plan, etc.
			if parts[0] == "admin" {
				if page, ok := adminPages[parts[1]]; ok {
					serveFile(w, r, filepath.Join(root, "admin", page))
					return
				}
			}
			if languages[parts[0]] && subPages[parts[1]] {
				if f := resolve(root, parts[0], parts[1]); f != "" {
					serveFile(w, r, f)
					return
				}
			}
		}
		http.NotFound(w, r)
	}
}

// ============================================================================
// Heartbeat and peer list API
// ============================================================================

func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var info PeerInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid body"})
		return
	}

	// Override reported address with real IP
	ip := getRealIP(r)
	if info.Port == 0 {
		info.Port = 51211
	}
	info.Address = fmt.Sprintf("%s:%d", ip, info.Port)

	registry.Heartbeat(&info)
	log.Printf("[heartbeat] %s (height=%d, version=%s)", info.Address, info.Height, info.Version)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"registered": info.Address,
			"peers":      len(registry.GetPeers()),
		},
	})
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	peers := registry.GetPeers()
	if peers == nil {
		peers = []*PeerInfo{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"total": len(peers),
			"peers": peers,
		},
	})
}

func getRealIP(r *http.Request) string {
	// Cloudflare
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	// Standard proxy headers
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	// Direct connection
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ============================================================================
// File serving and proxying
// ============================================================================

func resolve(root string, segments ...string) string {
	dir := filepath.Join(append([]string{root}, segments...)...)
	if idx := filepath.Join(dir, "index.html"); fileExists(idx) {
		return idx
	}
	if len(segments) == 1 {
		if f := filepath.Join(root, segments[0]+".html"); fileExists(f) {
			return f
		}
	} else if len(segments) == 2 {
		if f := filepath.Join(root, segments[0], segments[1]+".html"); fileExists(f) {
			return f
		}
	}
	return ""
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	if strings.Contains(path, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if filepath.Ext(path) == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.Write(data)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// proxyToNode forwards API requests to the AIB node (port 51211)
func proxyToNode(w http.ResponseWriter, r *http.Request) {
	nodeURL := "http://localhost:51211" + r.URL.Path
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(nodeURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"success":false,"error":{"message":"node unavailable"}}`))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func init() {
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".svg", "image/svg+xml")
}
