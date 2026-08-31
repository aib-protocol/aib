package p2p

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestFileDistServesReleaseJSON(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			head := make([]byte, 5)
			n, _ := c.Read(head)
			head = head[:n]
			if looksLikeHTTP(head) {
				pm := &ChainPeerManager{
					fileDist:    &FileDistConfig{Dir: t.TempDir(), Enabled: true},
					releaseJSON: func() []byte { return []byte(`{"name":"vX","sha256":"ab"}`) },
				}
				pm.serveFileDist(c, head)
			}
		}(conn)
	}()
	time.Sleep(50 * time.Millisecond)
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("GET /release.json HTTP/1.1\r\nHost: x\r\n\r\n"))
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 512)
	n, _ := c.Read(buf)
	resp := string(buf[:n])
	want := `{"name":"vX","sha256":"ab"}`
	if len(resp) < 12 || resp[:12] != "HTTP/1.1 200" || !strings.Contains(resp, want) {
		t.Fatalf("bad response: %q", resp)
	}
}
