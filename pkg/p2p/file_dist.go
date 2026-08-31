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
	"bytes"
	"fmt"
	"io"
	"net"
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
// Minimal hand-rolled HTTP/1.1 responder (GET/HEAD only, no keep-alive):
// the request head has already been partially consumed by sniffing, so
// we buffer until "\r\n\r\n", parse the path, and stream the file.
func (pm *ChainPeerManager) serveFileDist(conn net.Conn, firstRead []byte) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	if pm.fileDist == nil || !pm.fileDist.Enabled {
		fmt.Fprintf(conn, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}

	// Buffer the request head (first bytes already read by the sniffer).
	buf := make([]byte, 0, 4096)
	buf = append(buf, firstRead...)
	tmp := make([]byte, 512)
	for i := 0; i < 64 && !bytes.Contains(buf, []byte("\r\n\r\n")); i++ {
		n, err := conn.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil || n == 0 {
			break
		}
	}

	// Parse "GET /path HTTP/1.1"
	line := string(buf)
	space := bytes.IndexByte(buf, ' ')
	if space < 0 || !strings.HasPrefix(line, "GET ") && !strings.HasPrefix(line, "HEAD ") {
		fmt.Fprintf(conn, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	rest2 := buf[space+1:]
	space2 := bytes.IndexByte(rest2, ' ')
	if space2 < 0 {
		space2 = len(rest2)
	}
	rawPath := string(rest2[:space2])
	isHead := strings.HasPrefix(line, "HEAD ")

	// Sanitize: no traversal, no listing.
	clean := filepath.Clean("/" + rawPath)
	if strings.Contains(clean, "..") {
		fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	full := filepath.Join(pm.fileDist.Dir, clean)
	if !strings.HasPrefix(full, filepath.Clean(pm.fileDist.Dir)+string(os.PathSeparator)) {
		fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	fi, err := os.Stat(full)
	if err != nil || fi.IsDir() {
		fmt.Fprintf(conn, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}

	header := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", fi.Size())
	if _, err := conn.Write([]byte(header)); err != nil {
		return
	}
	if isHead {
		return
	}
	f, err := os.Open(full)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(conn, f)
}
