// Package p2p — replayConn lets http.Server read a connection whose
// first bytes were already consumed for protocol sniffing.
package p2p

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"time"
)

// replayConn wraps a net.Conn, replaying pre-read bytes before the
// underlying stream, so an http.Server can parse the full request.
type replayConn struct {
	net.Conn
	br *bufio.Reader
}

func newReplayConn(c net.Conn, preRead []byte) *replayConn {
	r := &replayConn{Conn: c}
	r.br = bufio.NewReader(io.MultiReader(bytes.NewReader(preRead), c))
	return r
}

// Read serves sniffed bytes first, then the live connection.
func (r *replayConn) Read(p []byte) (int, error) { return r.br.Read(p) }

// SetReadDeadline passes through to the live connection.
func (r *replayConn) SetReadDeadline(t time.Time) error { return r.Conn.SetReadDeadline(t) }
