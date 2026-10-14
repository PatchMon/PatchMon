package commands

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func rdpTestTarget(t *testing.T) (port int, accepted <-chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ch := make(chan net.Conn, 64)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			ch <- c
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, ch
}

func requireClosedByAgent(t *testing.T, c net.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("agent sent data instead of closing the RDP socket")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("agent left the RDP socket open")
	}
	_ = c.Close()
}

func waitRDPConnected(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rdpProxySessionsMu.RLock()
		ps := rdpProxySessions[id]
		rdpProxySessionsMu.RUnlock()
		if ps != nil {
			if ps.tcpConnection() != nil {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %s never connected", id)
}

func rdpSessionCount() int {
	rdpProxySessionsMu.RLock()
	defer rdpProxySessionsMu.RUnlock()
	return len(rdpProxySessions)
}

func TestRDPDisconnectDuringDialDoesNotLeakSocket(t *testing.T) {
	port, accepted := rdpTestTarget(t)
	conn := newWSTestClient(t, func(*websocket.Conn, <-chan struct{}) {})

	for i := 0; i < 30; i++ {
		id := "race-" + strconv.Itoa(i)
		handleRDPProxy(wsMsg{rdpProxySessionID: id, rdpProxyHost: "127.0.0.1", rdpProxyPort: port}, conn)
		handleRDPProxyDisconnect(wsMsg{rdpProxySessionID: id}, conn)

		select {
		case c := <-accepted:
			requireClosedByAgent(t, c)
		case <-time.After(3 * time.Second):
			t.Fatalf("iteration %d: agent never dialled the RDP target", i)
		}
	}
	if n := rdpSessionCount(); n != 0 {
		t.Fatalf("%d RDP sessions left in the map", n)
	}
}

func TestRDPSessionsCloseWhenWebSocketDrops(t *testing.T) {
	port, accepted := rdpTestTarget(t)
	dropped := newWSTestClient(t, func(*websocket.Conn, <-chan struct{}) {})
	live := newWSTestClient(t, func(*websocket.Conn, <-chan struct{}) {})

	handleRDPProxy(wsMsg{rdpProxySessionID: "on-dropped", rdpProxyHost: "127.0.0.1", rdpProxyPort: port}, dropped)
	first := <-accepted
	handleRDPProxy(wsMsg{rdpProxySessionID: "on-live", rdpProxyHost: "127.0.0.1", rdpProxyPort: port}, live)
	second := <-accepted
	waitRDPConnected(t, "on-dropped")
	t.Cleanup(func() { handleRDPProxyDisconnect(wsMsg{rdpProxySessionID: "on-live"}, live) })

	closeRDPProxySessionsForConn(dropped)
	requireClosedByAgent(t, first)

	rdpProxySessionsMu.RLock()
	_, liveKept := rdpProxySessions["on-live"]
	rdpProxySessionsMu.RUnlock()
	if !liveKept {
		t.Fatal("a session on a different WebSocket was closed")
	}
	_ = second.Close()
}

func TestRDPCloseDoesNotWaitForAWriter(t *testing.T) {
	port, accepted := rdpTestTarget(t)
	dropped := newWSTestClient(t, func(*websocket.Conn, <-chan struct{}) {})

	handleRDPProxy(wsMsg{rdpProxySessionID: "writer-held", rdpProxyHost: "127.0.0.1", rdpProxyPort: port}, dropped)
	target := <-accepted
	waitRDPConnected(t, "writer-held")
	rdpProxySessionsMu.RLock()
	ps := rdpProxySessions["writer-held"]
	rdpProxySessionsMu.RUnlock()

	ps.mu.Lock()
	defer ps.mu.Unlock()
	done := make(chan struct{})
	go func() {
		closeRDPProxySessionsForConn(dropped)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closing a dropped connection's sessions waited for the session write lock")
	}
	requireClosedByAgent(t, target)
}
