package agentregistry

import (
	"testing"

	"github.com/gorilla/websocket"
)

// newFakeConn returns a distinct, non-nil *websocket.Conn usable purely as an
// identity token. The registry never reads or writes it on these paths — it
// only compares pointers — so a zero-valued Conn is sufficient and avoids
// standing up a real WebSocket server for an identity test.
func newFakeConn() *websocket.Conn {
	return &websocket.Conn{}
}

// TestUnregisterConn_OwnConnectionTearsDown covers the ordinary teardown: the
// connection being torn down is the one the registry holds, so the agent is
// marked disconnected and the caller is told it owns the teardown.
func TestUnregisterConn_OwnConnectionTearsDown(t *testing.T) {
	t.Parallel()
	r := New()
	connA := newFakeConn()

	r.Register("api-1", true)
	r.SetConnection("api-1", connA)

	if !r.UnregisterConn("api-1", connA) {
		t.Fatalf("expected UnregisterConn to report ownership of the teardown for its own conn")
	}
	info := r.Get("api-1")
	if info.Connected {
		t.Fatalf("expected Connected=false after tearing down the live conn, got true")
	}
	if info.DisconnectedAt == nil {
		t.Fatalf("expected DisconnectedAt to be stamped, got nil")
	}
	if r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=false after teardown")
	}
}

// TestUnregisterConn_DoesNotClobberReconnect is the direct regression guard for
// the reconnect-clobber race.
//
// Timeline: conn A drops, the disconnect callback starts its (multi-second) DB
// work, the agent's 1s reconnect backoff fires and registers conn B, and only
// then does the OLD teardown reach the registry. The identity check must leave
// conn B completely untouched. Before this existed, the plain Unregister
// deleted conn B, so the agent held a live WebSocket while the registry
// reported it disconnected: every server-to-agent send failed with
// ErrNotConnected, the WS pill showed down, and patch-run stop commands could
// not be delivered — for the whole fleet at once after a proxy restart.
func TestUnregisterConn_DoesNotClobberReconnect(t *testing.T) {
	t.Parallel()
	r := New()
	connA := newFakeConn()
	connB := newFakeConn()

	// Original connection.
	r.Register("api-1", true)
	r.SetConnection("api-1", connA)

	// Agent reconnects before the old teardown lands.
	r.Register("api-1", true)
	r.SetConnection("api-1", connB)

	if r.UnregisterConn("api-1", connA) {
		t.Fatalf("regression: stale teardown for conn A claimed ownership while conn B is live")
	}

	if !r.IsConnected("api-1") {
		t.Fatalf("regression: stale teardown for conn A marked the agent disconnected while conn B is live")
	}
	info := r.Get("api-1")
	if !info.Connected {
		t.Fatalf("regression: Get reports Connected=false while conn B is live: %+v", info)
	}
	if info.DisconnectedAt != nil {
		t.Fatalf("regression: stale teardown stamped DisconnectedAt on a live agent: %v", info.DisconnectedAt)
	}
	if e := r.getEntry("api-1"); e == nil || e.ws != connB {
		t.Fatalf("regression: stale teardown removed or replaced the live conn B entry")
	}

	// The owning teardown still works afterwards.
	if !r.UnregisterConn("api-1", connB) {
		t.Fatalf("expected conn B's own teardown to claim ownership")
	}
	if r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=false after conn B tore itself down")
	}
}

// TestUnregisterConn_NoStoredConnection covers the cold path: the registry has
// no connection for this api_id (already torn down, or a stale close after a
// restart wiped the in-memory map). There is nothing newer to protect, so the
// caller owns the teardown and the disconnect is recorded.
func TestUnregisterConn_NoStoredConnection(t *testing.T) {
	t.Parallel()
	r := New()

	if !r.UnregisterConn("api-1", newFakeConn()) {
		t.Fatalf("expected ownership when the registry holds no connection for the api_id")
	}
	info := r.Get("api-1")
	if info.Connected {
		t.Fatalf("expected Connected=false, got true")
	}
	if info.DisconnectedAt == nil {
		t.Fatalf("expected DisconnectedAt to be stamped on the cold path, got nil")
	}
}

// TestUnregisterConn_NilConnFallsBackToUnconditional documents the nil-conn
// contract: callers that cannot supply an identity get the old unconditional
// behaviour rather than a silent no-op.
func TestUnregisterConn_NilConnFallsBackToUnconditional(t *testing.T) {
	t.Parallel()
	r := New()
	r.Register("api-1", false)
	r.SetConnection("api-1", newFakeConn())

	if !r.UnregisterConn("api-1", nil) {
		t.Fatalf("expected a nil conn to tear down unconditionally")
	}
	if r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=false after unconditional teardown")
	}
}

// TestUnregisterConn_DoubleCallDoesNotBumpDisconnectedAt mirrors the existing
// Unregister guard: teardown can fire twice (close handler + readPump exit) and
// the timestamp must reflect the original drop.
func TestUnregisterConn_DoubleCallDoesNotBumpDisconnectedAt(t *testing.T) {
	t.Parallel()
	r := New()
	conn := newFakeConn()
	r.Register("api-1", false)
	r.SetConnection("api-1", conn)

	r.UnregisterConn("api-1", conn)
	first := r.Get("api-1").DisconnectedAt
	if first == nil {
		t.Fatalf("expected DisconnectedAt after the first teardown, got nil")
	}

	r.UnregisterConn("api-1", conn)
	second := r.Get("api-1").DisconnectedAt
	if second == nil {
		t.Fatalf("expected DisconnectedAt to survive the second teardown, got nil")
	}
	if !second.Equal(*first) {
		t.Fatalf("DisconnectedAt was bumped by the second teardown: %v -> %v", first, second)
	}
}
