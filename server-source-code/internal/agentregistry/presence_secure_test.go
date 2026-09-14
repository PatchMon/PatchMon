package agentregistry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
)

// newPresenceRedis returns a registry wired to a real in-memory Redis, with
// distributed presence active but no pubsub consumer running, so tests can
// assert on the presence hash without racing the subscriber.
func newPresenceRedis(t *testing.T, podID string) (*Registry, *redisclient.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = podID
	return r, client
}

// awaitPresenceField polls the presence hash until the field appears, so the
// async setPresence goroutine has a bounded window to land.
func awaitPresenceField(t *testing.T, client *redisclient.Client, apiID, field string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		v, err := client.HGet(context.Background(), "agent:meta:"+apiID, field).Result()
		if err == nil {
			last = v
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("presence field %q for %s never appeared", field, apiID)
	return ""
}

// TestSetConnection_PublishesRealSecureFlag is the direct regression guard for
// the WS/WSS mislabel. SetConnection republishes presence to carry the context
// label forward, and used to hardcode secure=true while doing so. That value
// raced Register's correct one through the agent:events channel, so roughly
// half of a plain-HTTP fleet ended up labelled WSS in the UI.
//
// The published flag must mirror whatever Register already stored, never a
// constant.
func TestSetConnection_PublishesRealSecureFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		secure bool
		want   string
	}{
		{"plain ws stays insecure", false, "0"},
		{"wss stays secure", true, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, client := newPresenceRedis(t, "pod-a")

			// Seed exactly what Register leaves behind on the upgrade path,
			// without letting its own async publish race this assertion.
			r.mu.Lock()
			r.meta["api-1"] = ConnectionInfo{Connected: true, Secure: tc.secure}
			r.mu.Unlock()

			r.SetConnection("api-1", newFakeConn())

			if got := awaitPresenceField(t, client, "api-1", "secure"); got != tc.want {
				t.Fatalf("presence secure=%q, want %q: SetConnection must publish the "+
					"connection's real TLS state, not a constant", got, tc.want)
			}
			if info := r.Get("api-1"); info.Secure != tc.secure {
				t.Fatalf("local meta Secure=%v, want %v", info.Secure, tc.secure)
			}
		})
	}
}

// TestUpgradePath_PlainHTTPFleetNeverReportsSecure exercises the real upgrade
// sequence from handler.AgentWSHandler.ServeWS (Register then SetConnection)
// with the pubsub consumer live, which is where the race actually bit: the pod
// receives its own connect events and handlePubSubMessage overwrites local meta
// from them. Every agent here connected over plain HTTP, so not one may end up
// reported as secure.
func TestUpgradePath_PlainHTTPFleetNeverReportsSecure(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	r := New()
	if err := r.EnableDistributed(context.Background(), client, "pod-a"); err != nil {
		t.Fatalf("EnableDistributed: %v", err)
	}
	// Let the subscription register before anything publishes.
	time.Sleep(200 * time.Millisecond)

	// A fleet rather than a single agent: the losing side of the race was
	// per-agent and roughly a coin flip, so one agent would pass by luck half
	// the time.
	const fleet = 40
	for i := range fleet {
		apiID := fmt.Sprintf("api-%02d", i)
		r.Register(apiID, false /* plain HTTP, no X-Forwarded-Proto: https */, "")
		r.SetConnection(apiID, newFakeConn())
	}

	// Give every async publish and its pubsub round trip room to land.
	time.Sleep(time.Second)

	// Assert on the durable presence record as well as local meta. Local meta
	// alone would pass vacuously on a runner slow enough that no event is
	// consumed within the settle window, since Register's own correct value
	// would simply still be sitting there — the guard would silently disarm
	// rather than fail. The hash is also what a peer's snapshotPresence reads
	// back, so a wrong value there mislabels the pill on another pod too.
	var mislabelled, badRecord []string
	for i := range fleet {
		apiID := fmt.Sprintf("api-%02d", i)
		if r.Get(apiID).Secure {
			mislabelled = append(mislabelled, apiID)
		}
		if got := awaitPresenceField(t, client, apiID, "secure"); got != "0" {
			badRecord = append(badRecord, fmt.Sprintf("%s=%s", apiID, got))
		}
	}
	if len(badRecord) > 0 {
		t.Errorf("%d/%d plain-HTTP agents have a secure presence record: %v",
			len(badRecord), fleet, badRecord)
	}
	if len(mislabelled) > 0 {
		t.Fatalf("%d/%d plain-HTTP agents reported Secure=true (UI renders a WSS pill): %v",
			len(mislabelled), fleet, mislabelled)
	}
}
