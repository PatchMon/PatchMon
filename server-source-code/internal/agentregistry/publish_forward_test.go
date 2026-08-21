package agentregistry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

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

// TestPublishForward_SelfPodReturnsNotConnected is the direct regression
// guard for the unbounded self-publish loop.
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

// TestPublishForward_RemotePodStillForwards guards against over-correcting: a
// genuinely remote pod must still be published to.
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

// newRecordingRedis returns a client backed by a real in-memory Redis, plus a
// func reporting how many messages have been published to the given channel.
//
// Asserting on the returned error is not enough to discriminate this fix: with
// an unreachable Redis the publish fails on dial, so SendMessage returns
// ErrNotConnected whether or not the self-check exists. The behaviour that
// actually changed is that NO PUBLISH IS ATTEMPTED, so that is what these
// assert.
func newRecordingRedis(t *testing.T) (*redisclient.Client, func(channel string) int) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Subscribe so miniredis records delivery, then count what arrives.
	counts := make(map[string]int)
	var mu sync.Mutex
	subs := make(map[string]*redisclient.PubSub)

	countFor := func(channel string) int {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := subs[channel]; !ok {
			ps := client.Subscribe(context.Background(), channel)
			subs[channel] = ps
			t.Cleanup(func() { _ = ps.Close() })
			// Drain in the background, tallying.
			go func(ch string, ps *redisclient.PubSub) {
				for range ps.Channel() {
					mu.Lock()
					counts[ch]++
					mu.Unlock()
				}
			}(channel, ps)
			// Give the subscription a moment to register.
			time.Sleep(50 * time.Millisecond)
			return 0
		}
		return counts[channel]
	}
	return client, countFor
}

// TestSendMessage_SelfPodPublishesNothing is the behavioural guard.
func TestSendMessage_SelfPodPublishesNothing(t *testing.T) {
	t.Parallel()
	client, countFor := newRecordingRedis(t)

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = "pod-a"

	r.mu.Lock()
	r.meta["api-1"] = ConnectionInfo{Connected: true}
	r.podMap["api-1"] = "pod-a"
	r.mu.Unlock()

	ownChannel := "agent:pod:pod-a"
	_ = countFor(ownChannel) // establish the subscription

	if err := r.SendMessage("api-1", websocket.TextMessage, []byte("report_now")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
	if err := r.SendJSON("api-1", map[string]string{"type": "report_now"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected from SendJSON, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if n := countFor(ownChannel); n != 0 {
		t.Fatalf("nothing may be published to our own channel; got %d message(s). "+
			"Each one is delivered straight back into handlePubSubMessage and re-published, "+
			"which is the unbounded loop this guards.", n)
	}
}

// TestSendMessage_RemotePodStillPublishes is the other half: the guard must
// not suppress a genuinely remote agent.
func TestSendMessage_RemotePodStillPublishes(t *testing.T) {
	t.Parallel()
	client, countFor := newRecordingRedis(t)

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = "pod-a"

	r.mu.Lock()
	r.meta["api-2"] = ConnectionInfo{Connected: true}
	r.podMap["api-2"] = "pod-b" // owned elsewhere
	r.mu.Unlock()

	remoteChannel := "agent:pod:pod-b"
	_ = countFor(remoteChannel)

	if err := r.SendMessage("api-2", websocket.TextMessage, []byte("report_now")); err != nil {
		t.Fatalf("a remote agent must still be forwarded, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if n := countFor(remoteChannel); n != 1 {
		t.Fatalf("expected exactly one publish to the owning pod's channel, got %d", n)
	}
}

// TestSendMessage_RegisterBeforeSetConnectionWindow covers the ordinary
// registration window: Register() writes podMap before SetConnection() stores
// the socket, so podMap naming the local pod with no conn is a normal state
// on a healthy connect, not only after a restart.
func TestSendMessage_RegisterBeforeSetConnectionWindow(t *testing.T) {
	t.Parallel()
	client, countFor := newRecordingRedis(t)

	r := New()
	r.distCtx = context.Background()
	r.podID = "pod-a"
	r.Register("api-1", true, "") // sets meta + podMap; conns still empty
	r.rdb = client

	ownChannel := "agent:pod:pod-a"
	_ = countFor(ownChannel)

	if err := r.SendMessage("api-1", websocket.TextMessage, []byte("x")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected during the Register/SetConnection window, got %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if n := countFor(ownChannel); n != 0 {
		t.Fatalf("the registration window must not publish to our own channel, got %d", n)
	}

	// Once the socket lands, the local write path takes over.
	r.SetConnection("api-1", newFakeConn())
	if r.getEntry("api-1") == nil {
		t.Fatalf("expected a live entry after SetConnection")
	}
}
