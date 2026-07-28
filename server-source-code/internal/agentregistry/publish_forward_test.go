package agentregistry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	redisclient "github.com/redis/go-redis/v9"
)

// newUnreachableRedis returns a client that is never actually dialled. The
// self-forward guard must return before any network call, so a client pointing
// at a closed port is enough to prove the guard fires rather than publishing.
func newUnreachableRedis() *redisclient.Client {
	return redisclient.NewClient(&redisclient.Options{
		Addr: "127.0.0.1:1",
		// Fail fast and quietly: these tests assert on the guard, not on Redis.
		MaxRetries:  -1,
		DialTimeout: 50 * time.Millisecond,
	})
}

// TestPublishForward_SelfPodReturnsNotConnected is the direct regression guard
// for the unbounded self-publish loop.
//
// The pod subscribes to its own agent:pod:<podID> channel, so publishing a
// forward addressed to ourselves is delivered back into handlePubSubMessage,
// which calls SendMessage, which calls publishForward again. Because
// handlePubSubMessage runs inline in the pubsub consumer's select loop, that
// loop never returns and presence handling freezes fleet-wide.
func TestPublishForward_SelfPodReturnsNotConnected(t *testing.T) {
	t.Parallel()
	r := New()
	r.distCtx = context.Background()
	r.rdb = newUnreachableRedis()
	r.podID = "pod-a"
	r.podMap["api-1"] = "pod-a"

	err := r.publishForward("api-1", websocket.TextMessage, []byte("payload"))
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected when podMap names the local pod, got %v", err)
	}
}

// TestPublishForward_RemotePodStillForwards guards against over-correcting:
// a genuinely remote pod must still be published to.
func TestPublishForward_RemotePodStillForwards(t *testing.T) {
	t.Parallel()
	r := New()
	r.distCtx = context.Background()
	r.rdb = newUnreachableRedis()
	r.podID = "pod-a"
	r.podMap["api-1"] = "pod-b"

	err := r.publishForward("api-1", websocket.TextMessage, []byte("payload"))
	if errors.Is(err, ErrNotConnected) {
		t.Fatalf("remote pod must still be forwarded to, got ErrNotConnected")
	}
	// The publish itself fails (nothing is listening on 127.0.0.1:1); the point
	// is that we got past the guard and attempted it.
	if err == nil {
		t.Fatalf("expected the unreachable-Redis publish to error, got nil")
	}
}

// TestSendMessage_SelfPodWithoutLocalConnIsNotConnected covers the state
// snapshotPresence() leaves behind after a restart: meta says connected and
// podMap names this pod, but conns is empty because the agent has not
// reconnected yet. The send must report ErrNotConnected rather than looping,
// and must not report success for a frame nobody received.
func TestSendMessage_SelfPodWithoutLocalConnIsNotConnected(t *testing.T) {
	t.Parallel()
	r := New()
	r.distCtx = context.Background()
	r.rdb = newUnreachableRedis()
	r.podID = "pod-a"

	// Exactly what snapshotPresence() writes for a surviving agent:meta:* key.
	r.mu.Lock()
	r.meta["api-1"] = ConnectionInfo{Connected: true}
	r.podMap["api-1"] = "pod-a"
	r.mu.Unlock()

	if err := r.SendMessage("api-1", websocket.TextMessage, []byte("report_now")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected for a self-owned apiID with no live conn, got %v", err)
	}
	if err := r.SendJSON("api-1", map[string]string{"type": "report_now"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected from SendJSON for a self-owned apiID with no live conn, got %v", err)
	}
}

// TestSendMessage_RegisterBeforeSetConnectionWindow covers the ordinary
// registration window: Register() writes podMap before SetConnection() stores
// the socket, so podMap naming the local pod with no conn is a normal state on
// a healthy connect, not only after a restart. It must not loop, and it must
// not be mistaken for a delivery.
func TestSendMessage_RegisterBeforeSetConnectionWindow(t *testing.T) {
	t.Parallel()
	r := New()
	r.distCtx = context.Background()
	r.podID = "pod-a"

	r.Register("api-1", true) // sets meta + podMap; conns still empty
	r.rdb = newUnreachableRedis()

	if err := r.SendMessage("api-1", websocket.TextMessage, []byte("x")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected during the Register/SetConnection window, got %v", err)
	}

	// Once the socket lands, the local write path takes over and the send no
	// longer reports ErrNotConnected.
	r.SetConnection("api-1", newFakeConn())
	if r.getEntry("api-1") == nil {
		t.Fatalf("expected a live entry after SetConnection")
	}
}
