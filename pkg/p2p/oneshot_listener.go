package p2p

import "net"

// oneShotListener serves exactly one connection then blocks forever
// (connection is closed by the caller after Serve returns).
type oneShotListener struct {
	conn net.Conn
	done chan struct{}
}

func newOneShotListener(c net.Conn) *oneShotListener {
	return &oneShotListener{conn: c, done: make(chan struct{})}
}

func (l *oneShotListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
		close(l.done)
		return l.conn, nil
	}
}

func (l *oneShotListener) Close() error   { return nil }
func (l *oneShotListener) Addr() net.Addr { return l.conn.LocalAddr() }
