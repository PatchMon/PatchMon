package sshproxy

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// TestConnWriter_ConcurrentWrites reproduces the shape of the original
// defect: two unrelated goroutines writing the same frontend connection.
func TestConnWriter_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- c
		// Hold the handler open for the duration of the test.
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	dialURL := "ws" + srv.URL[len("http"):]
	client, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	serverConn := <-connCh
	defer func() { _ = serverConn.Close() }()

	// Drain on the client side so writes do not block on flow control.
	go func() {
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	writer := NewConnWriter(serverConn)

	const goroutines = 8
	const writes = 40

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < writes; i++ {
				// Mixed payload shapes, matching the four message types
				// HandleAgentMessage emits plus the handler's own sends.
				if err := writer.WriteJSON(map[string]interface{}{
					"type": "data",
					"g":    g,
					"i":    i,
				}); err != nil {
					return // connection closed; not what this test is asserting
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestConnWriter_NilSafety covers the paths the handler relies on: a nil
// writer (no proxy session registered) and a released connection.
func TestConnWriter_NilSafety(t *testing.T) {
	t.Parallel()

	var nilWriter *ConnWriter
	if err := nilWriter.WriteJSON(map[string]string{"type": "data"}); err != nil {
		t.Errorf("nil writer must be a no-op, got %v", err)
	}

	released := &ConnWriter{}
	if err := released.WriteJSON(map[string]string{"type": "data"}); err != nil {
		t.Errorf("writer with no connection must be a no-op, got %v", err)
	}
}

// TestSessionCarriesWriter documents the invariant that makes the fix work:
// the session stores the serialising writer, not a bare connection, so there
// is no way for a caller to obtain the raw conn and write it unlocked.
func TestSessionCarriesWriter(t *testing.T) {
	t.Parallel()

	s := NewSessions()
	w := NewConnWriter(nil)
	s.Set("sess-1", &Session{Frontend: w, HostID: "h1", ApiID: "api1"})

	got := s.Get("sess-1")
	if got == nil {
		t.Fatal("expected the session back")
	}
	if got.Frontend != w {
		t.Error("session must carry the shared writer")
	}
}
