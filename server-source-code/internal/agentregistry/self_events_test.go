package agentregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
)

// TestUpgradeThenImmediateDrop_DoesNotGhost is the regression guard for an
// agent left permanently marked connected despite having no socket.
//
// Register and SetConnection publish their connect event from a goroutine,
// while UnregisterConn publishes its disconnect inline. A socket that dies
// inside that scheduling window therefore delivers connect AFTER disconnect,
// and the pod consumes its own events. The connect case rebuilt the entry
// wholesale, so the agent came back as connected.
//
// The consequence outlives the two bugs this package already guards against:
// in-memory meta has no TTL, so nothing ever flips the ghost back. host_down
// alerting is suppressed for the life of the process, the sidebar count is
// inflated, and every send to the agent fails with ErrNotConnected.
func TestUpgradeThenImmediateDrop_DoesNotGhost(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	r := New()
	if err := r.EnableDistributed(context.Background(), client, "pod-a"); err != nil {
		t.Fatalf("EnableDistributed: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the subscription register

	// A fleet, because the window is a goroutine scheduling race: one agent
	// could win it by luck.
	const fleet = 20
	for i := range fleet {
		apiID := fmt.Sprintf("api-%02d", i)
		conn := newFakeConn()
		r.Register(apiID, false, "")
		r.SetConnection(apiID, conn)
		// The socket dies before the publish goroutines have been scheduled.
		r.UnregisterConn(apiID, conn)
	}

	time.Sleep(time.Second) // let every publish and round trip land

	var ghosts []string
	for i := range fleet {
		apiID := fmt.Sprintf("api-%02d", i)
		if r.Get(apiID).Connected {
			ghosts = append(ghosts, apiID)
		}
	}
	if len(ghosts) > 0 {
		t.Fatalf("%d/%d agents with no socket are still reported connected: %v. "+
			"Nothing clears this: meta has no TTL, so host_down stays suppressed "+
			"for the life of the process", len(ghosts), fleet, ghosts)
	}
}

// TestHandlePubSubMessage_IgnoresOwnPodEvents pins the mechanism directly: an
// event we published ourselves must not be applied, because local state was
// already written synchronously under the lock before the publish went out.
func TestHandlePubSubMessage_IgnoresOwnPodEvents(t *testing.T) {
	t.Parallel()
	r := New()
	r.podID = "pod-a"

	// Local truth: the agent dropped.
	r.Register("api-1", false, "")
	r.Unregister("api-1")

	ev, err := json.Marshal(map[string]any{
		"api_id": "api-1",
		"type":   "connect",
		"pod":    "pod-a", // ours
		"secure": true,
		"scope":  "",
		"ts":     time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r.handlePubSubMessage("agent:events", ev)

	if info := r.Get("api-1"); info.Connected {
		t.Errorf("our own connect event must not resurrect an agent we know is gone")
	}
}

// TestHandlePubSubMessage_AppliesPeerPodEvents is the other half: filtering our
// own events must not deafen us to a peer's, which is the entire purpose of
// distributed presence.
func TestHandlePubSubMessage_AppliesPeerPodEvents(t *testing.T) {
	t.Parallel()
	r := New()
	r.podID = "pod-a"

	ev, err := json.Marshal(map[string]any{
		"api_id": "api-1",
		"type":   "connect",
		"pod":    "pod-b", // a peer
		"secure": true,
		"scope":  "",
		"ts":     time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r.handlePubSubMessage("agent:events", ev)

	info := r.Get("api-1")
	if !info.Connected {
		t.Fatalf("a peer pod's connect event must still be applied")
	}
	if !info.Secure {
		t.Errorf("expected the peer event's Secure flag to be applied")
	}
	r.mu.RLock()
	pod := r.podMap["api-1"]
	r.mu.RUnlock()
	if pod != "pod-b" {
		t.Errorf("podMap = %q, want pod-b so sends can be routed to the owner", pod)
	}
}
