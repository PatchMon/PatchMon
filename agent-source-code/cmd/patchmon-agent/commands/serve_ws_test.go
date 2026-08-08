package commands

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// The WebSocket helpers log through the package logger, which is normally
// created by initialiseAgent and is nil under test.
func TestMain(m *testing.M) {
	logger = logrus.New()
	logger.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// TestNewWSDialer guards the dialer defaults. A zero-value gorilla Dialer sets
// no deadline on the socket at all, so a peer that accepts TCP and then stalls
// parks the whole reconnect loop indefinitely.
func TestNewWSDialer(t *testing.T) {
	tests := []struct {
		name              string
		skipSSLVerify     bool
		wantInsecureTLS   bool
		wantTLSConfigured bool
	}{
		{
			name:              "verified TLS keeps the shared defaults",
			skipSSLVerify:     false,
			wantInsecureTLS:   false,
			wantTLSConfigured: false,
		},
		{
			name:              "skip_ssl_verify keeps the defaults and disables verification",
			skipSSLVerify:     true,
			wantInsecureTLS:   true,
			wantTLSConfigured: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newWSDialer(tc.skipSSLVerify)

			if d.HandshakeTimeout == 0 {
				t.Error("HandshakeTimeout is zero: Dial would have no deadline and could block forever")
			}
			if d.HandshakeTimeout != wsHandshakeTimeout {
				t.Errorf("HandshakeTimeout = %v, want %v", d.HandshakeTimeout, wsHandshakeTimeout)
			}
			if d.Proxy == nil {
				t.Error("Proxy is nil: the agent would ignore HTTPS_PROXY on the WebSocket only")
			}
			if got := d.TLSClientConfig != nil; got != tc.wantTLSConfigured {
				t.Errorf("TLSClientConfig set = %v, want %v", got, tc.wantTLSConfigured)
			}
			if tc.wantInsecureTLS && !d.TLSClientConfig.InsecureSkipVerify {
				t.Error("InsecureSkipVerify = false, want true")
			}
			if d == websocket.DefaultDialer {
				t.Error("dialer aliases websocket.DefaultDialer; mutating it would leak into every other caller")
			}
		})
	}
}

// TestNewWSDialerBoundsStalledHandshake dials a listener that completes the TCP
// handshake and then says nothing, which is what a reverse proxy mid-restart or
// a backend with no healthy upstream looks like. Dial must give up.
func TestNewWSDialerBoundsStalledHandshake(t *testing.T) {
	tests := []struct {
		name          string
		skipSSLVerify bool
	}{
		{name: "verified TLS", skipSSLVerify: false},
		{name: "skip_ssl_verify", skipSSLVerify: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restore := wsHandshakeTimeout
			wsHandshakeTimeout = 300 * time.Millisecond
			defer func() { wsHandshakeTimeout = restore }()

			addr := startBlackHoleListener(t)
			dialer := newWSDialer(tc.skipSSLVerify)

			done := make(chan error, 1)
			go func() {
				conn, _, err := dialer.Dial("ws://"+addr+"/api/v1/agents/ws", nil)
				if conn != nil {
					_ = conn.Close()
				}
				done <- err
			}()

			select {
			case err := <-done:
				if err == nil {
					t.Fatal("Dial succeeded against a black-hole listener, want an error")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Dial did not return: the reconnect loop is parked with the socket established")
			}
		})
	}
}

// TestRunWSPingLoop covers the liveness pinger. Pongs are the only thing that
// re-arm the read deadline, so a pinger that stops without tearing the
// connection down leaves a live socket that nothing is watching.
func TestRunWSPingLoop(t *testing.T) {
	tests := []struct {
		name        string
		writeErr    error
		stopEarly   bool
		wantClosed  bool
		wantMinPing int
	}{
		{
			name:        "healthy connection keeps pinging",
			writeErr:    nil,
			wantClosed:  false,
			wantMinPing: 2,
		},
		{
			name:        "transient write timeout tears the connection down",
			writeErr:    errors.New("websocket: write timeout"),
			wantClosed:  true,
			wantMinPing: 1,
		},
		{
			name:        "fatal write error tears the connection down",
			writeErr:    net.ErrClosed,
			wantClosed:  true,
			wantMinPing: 1,
		},
		{
			name:        "cancellation stops the loop without closing",
			writeErr:    nil,
			stopEarly:   true,
			wantClosed:  false,
			wantMinPing: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn := &fakeControlConn{err: tc.writeErr}
			done := make(chan struct{})
			returned := make(chan struct{})

			go func() {
				runWSPingLoop(conn, 10*time.Millisecond, done)
				close(returned)
			}()

			if tc.stopEarly {
				close(done)
			} else if tc.writeErr == nil {
				time.Sleep(60 * time.Millisecond)
				close(done)
			}

			select {
			case <-returned:
			case <-time.After(2 * time.Second):
				close(done)
				t.Fatal("ping loop did not return")
			}

			if got := conn.closeCount(); (got > 0) != tc.wantClosed {
				t.Errorf("connection closed = %v, want %v", got > 0, tc.wantClosed)
			}
			if got := conn.pingCount(); got < tc.wantMinPing {
				t.Errorf("pings = %d, want at least %d", got, tc.wantMinPing)
			}
		})
	}
}

// TestWSReadDeadlineStaysArmed drives a real WebSocket pair. The agent's read
// deadline used to be re-armed only by a pong, so a connection carrying plenty
// of server traffic was still torn down whenever a pong was late.
func TestWSReadDeadlineStaysArmed(t *testing.T) {
	tests := []struct {
		name    string
		serve   func(conn *websocket.Conn, stop <-chan struct{})
		wantErr bool
	}{
		{
			name: "data frames keep the deadline armed",
			serve: func(conn *websocket.Conn, stop <-chan struct{}) {
				sendUntilStop(conn, stop, websocket.TextMessage)
			},
			wantErr: false,
		},
		{
			name: "server pings keep the deadline armed",
			serve: func(conn *websocket.Conn, stop <-chan struct{}) {
				sendUntilStop(conn, stop, websocket.PingMessage)
			},
			wantErr: false,
		},
		{
			name: "silence still fires the watchdog",
			serve: func(_ *websocket.Conn, stop <-chan struct{}) {
				<-stop
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restore := wsPongWait
			wsPongWait = 250 * time.Millisecond
			defer func() { wsPongWait = restore }()

			client := newWSTestClient(t, tc.serve)
			configureWSDeadlines(client)

			errCh := make(chan error, 1)
			readerDone := make(chan struct{})
			go func() {
				defer close(readerDone)
				for {
					if _, err := readWSMessage(client); err != nil {
						errCh <- err
						return
					}
				}
			}()

			// Long enough that a deadline armed once at connect time has
			// certainly expired.
			var gotErr error
			select {
			case gotErr = <-errCh:
			case <-time.After(3 * wsPongWait):
			}

			_ = client.Close()
			<-readerDone

			if tc.wantErr && gotErr == nil {
				t.Error("read never failed: a silent connection is not being watched")
			}
			if !tc.wantErr && gotErr != nil {
				t.Errorf("read failed on a connection the server was actively feeding: %v", gotErr)
			}
		})
	}
}

// TestDispatchWSMessage covers the hand-off from the read loop to the service
// loop. A blocking send parks the reader outside ReadMessage, and a Go read
// deadline only produces an error on an attempted read, so while the reader is
// parked the connection watchdog does not exist.
func TestDispatchWSMessage(t *testing.T) {
	tests := []struct {
		name          string
		prefill       int
		wantDelivered bool
	}{
		{
			name:          "free slot delivers immediately",
			prefill:       0,
			wantDelivered: true,
		},
		{
			name:          "busy service loop drops rather than blocking the reader",
			prefill:       1,
			wantDelivered: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restore := wsDispatchWait
			wsDispatchWait = 50 * time.Millisecond
			defer func() { wsDispatchWait = restore }()

			out := make(chan wsMsg, 1)
			for i := 0; i < tc.prefill; i++ {
				out <- wsMsg{kind: "prefill"}
			}

			returned := make(chan struct{})
			go func() {
				dispatchWSMessage(out, wsMsg{kind: "report_now"})
				close(returned)
			}()

			select {
			case <-returned:
			case <-time.After(2 * time.Second):
				t.Fatal("dispatch blocked: the read loop is parked and the connection is unwatched")
			}

			delivered := false
			for len(out) > 0 {
				if (<-out).kind == "report_now" {
					delivered = true
				}
			}
			if delivered != tc.wantDelivered {
				t.Errorf("delivered = %v, want %v", delivered, tc.wantDelivered)
			}
		})
	}
}

type fakeControlConn struct {
	mu     sync.Mutex
	pings  int
	closes int
	err    error
}

func (f *fakeControlConn) WriteControl(_ int, _ []byte, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings++
	return f.err
}

func (f *fakeControlConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

func (f *fakeControlConn) pingCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pings
}

func (f *fakeControlConn) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// startBlackHoleListener accepts connections and then never writes a byte.
func startBlackHoleListener(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	stop := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-stop
				_ = conn.Close()
			}()
		}
	}()
	t.Cleanup(func() {
		close(stop)
		_ = ln.Close()
	})
	return ln.Addr().String()
}

// newWSTestClient returns a client connection to a test server running serve.
func newWSTestClient(t *testing.T, serve func(conn *websocket.Conn, stop <-chan struct{})) *websocket.Conn {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		serve(conn, stop)
		<-stop
	}))

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial test server: %v", err)
	}
	t.Cleanup(func() {
		close(stop)
		_ = client.Close()
		srv.Close()
	})
	return client
}

// sendUntilStop emits one frame of the given type every 40ms. Nothing it sends
// is a pong, so only a deadline that is re-armed on data or on a ping survives.
func sendUntilStop(conn *websocket.Conn, stop <-chan struct{}, messageType int) {
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			var err error
			if messageType == websocket.PingMessage {
				err = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
			} else {
				err = conn.WriteMessage(messageType, []byte(`{"type":"connected"}`))
			}
			if err != nil {
				return
			}
		}
	}
}
