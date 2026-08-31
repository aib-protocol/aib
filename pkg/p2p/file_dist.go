// Package p2p — in-band HTTP file distribution on the P2P port (RFC-distribution-A).
//
// The chain listener sniffs the first bytes of every inbound connection:
// if they look like HTTP ("GET "/"POST "/"HEAD "), the connection is
// served as a read-only file server (install.sh + release binaries),
// otherwise it proceeds through the normal P2P handshake. This lets any
// node double as a distribution mirror with zero extra ports.
//
// Integrity is NOT provided here — install.sh pins SHA256 hashes and
// verifies whatever it downloads (RFC-distribution-B). A malicious
// mirror can only cause a hash-mismatch failure, never code execution.
package p2p

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileDistConfig configures the in-band HTTP distributor.
type FileDistConfig struct {
	// Dir is the directory served (contains install.sh and release files).
	Dir string
	// Enabled turns distribution on/off.
	Enabled bool
}

// looksLikeHTTP reports whether the first bytes are an HTTP request line.
func looksLikeHTTP(b []byte) bool {
	for _, m := range [][]byte{[]byte("GET "), []byte("POST "), []byte("HEAD ")} {
		if len(b) >= len(m) && string(b[:len(m)]) == string(m) {
			return true
		}
	}
	return false
}

// serveFileDist handles one HTTP connection on the P2P listener.
// Connection is closed after the response. Only GET/HEAD, no listings.
func (pm *ChainPeerManager) serveFileDist(conn net.Conn, firstRead []byte) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	if pm.fileDist == nil || !pm.fileDist.Enabled {
		// Minimal 404 without touching the filesystem.
		fmt.Fprintf(conn, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}

	// One-shot serve: wrap the replaying conn in a single-accept listener.
	srv := &http.Server{
		Handler:           pm.fileDistHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	_ = srv.Serve(newOneShotListener(newReplayConn(conn, firstRead)))
}

// fileDistHandler builds the read-only file handler.
func (pm *ChainPeerManager) fileDistHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Sanitize: no traversal, no listing.
		clean := filepath.Clean("/" + r.URL.Path)
		if strings.Contains(clean, "..") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		full := filepath.Join(pm.fileDist.Dir, clean)
		if !strings.HasPrefix(full, filepath.Clean(pm.fileDist.Dir)+string(os.PathSeparator)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fi, err := os.Stat(full)
		if err != nil || fi.IsDir() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, full)
	})
}
