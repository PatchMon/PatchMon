package agentregistry

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
)

// seedPresence writes a presence record the way setPresence would.
func seedPresence(t *testing.T, client *redisclient.Client, apiID, pod string, secure bool) {
	t.Helper()
	secureVal := "0"
	if secure {
		secureVal = "1"
	}
	vals := map[string]interface{}{
		"pod":       pod,
		"secure":    secureVal,
		"scope":     "",
		"last_seen": time.Now().UTC().Format(time.RFC3339),
	}
	if err := client.HSet(context.Background(), "agent:meta:"+apiID, vals).Err(); err != nil {
		t.Fatalf("seed %s: %v", apiID, err)
	}
}

// TestSnapshotPresence_OwnPodRecordsAreNotResurrected is the regression guard
// for a restarted server claiming agents are connected when they are not.
//
// snapshotPresence runs at startup, before this process holds any socket. It
// used to import every surviving presence record as Connected=true, including
// the ones the previous process left behind for its own pod. Those sockets
// died with that process, so the registry asserted connections nothing could
// write to: a connected WS pill, an inflated sidebar count, and suppressed
// host_down alerting for the remainder of the key's 5 minute TTL.
func TestSnapshotPresence_OwnPodRecordsAreNotResurrected(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Left behind by the process we are replacing.
	seedPresence(t, client, "api-ours", "pod-a", false)
	// Held by a peer that is still running.
	seedPresence(t, client, "api-theirs", "pod-b", true)
	// Written before pod labelling, or by a pod that could not name itself.
	seedPresence(t, client, "api-nopod", "", false)

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = "pod-a"

	if err := r.snapshotPresence(); err != nil {
		t.Fatalf("snapshotPresence: %v", err)
	}

	ours := r.Get("api-ours")
	if ours.Connected {
		t.Errorf("our own leftover record must not be imported as connected: " +
			"this process holds no sockets at startup")
	}
	if ours.DisconnectedAt == nil {
		t.Errorf("expected DisconnectedAt to be stamped so the pill can show a duration")
	}
	if r.IsConnected("api-ours") {
		t.Errorf("IsConnected must be false for our own leftover record")
	}

	if nopod := r.Get("api-nopod"); nopod.Connected {
		t.Errorf("an unroutable record (no pod) must not be imported as connected")
	}

	theirs := r.Get("api-theirs")
	if !theirs.Connected {
		t.Errorf("a peer pod's record must still be imported as connected: " +
			"that is the whole point of distributed presence")
	}
	if !theirs.Secure {
		t.Errorf("expected the peer record's Secure flag to survive the snapshot")
	}

	if n := r.CountConnected(""); n != 1 {
		t.Errorf("CountConnected = %d, want 1 (only the peer pod's agent)", n)
	}
}

// TestSnapshotPresence_OwnPodRecordsAreNotRouted checks the routing half: a
// leftover of ours must not leave a podMap entry pointing at a pod with no
// socket behind it.
func TestSnapshotPresence_OwnPodRecordsAreNotRouted(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	seedPresence(t, client, "api-ours", "pod-a", false)
	seedPresence(t, client, "api-theirs", "pod-b", false)

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = "pod-a"

	if err := r.snapshotPresence(); err != nil {
		t.Fatalf("snapshotPresence: %v", err)
	}

	r.mu.RLock()
	oursPod, oursOK := r.podMap["api-ours"]
	theirsPod := r.podMap["api-theirs"]
	r.mu.RUnlock()

	if oursOK {
		t.Errorf("podMap[api-ours] = %q, want no entry", oursPod)
	}
	if theirsPod != "pod-b" {
		t.Errorf("podMap[api-theirs] = %q, want pod-b", theirsPod)
	}
}

// TestSnapshotPresence_ReconnectOverwritesStaleEntry confirms the honest
// disconnected entry is not sticky: the agent reconnecting to this process
// puts it back to connected with the real TLS state.
func TestSnapshotPresence_ReconnectOverwritesStaleEntry(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	seedPresence(t, client, "api-1", "pod-a", true)

	r := New()
	r.distCtx = context.Background()
	r.rdb = client
	r.podID = "pod-a"

	if err := r.snapshotPresence(); err != nil {
		t.Fatalf("snapshotPresence: %v", err)
	}
	if r.Get("api-1").Connected {
		t.Fatalf("precondition: stale entry should be disconnected")
	}

	r.Register("api-1", false, "")
	r.SetConnection("api-1", newFakeConn())

	info := r.Get("api-1")
	if !info.Connected {
		t.Errorf("expected the reconnect to mark the agent connected")
	}
	if info.Secure {
		t.Errorf("expected Secure=false from the new plain-HTTP connection, " +
			"not the stale record's true")
	}
	if info.DisconnectedAt != nil {
		t.Errorf("expected DisconnectedAt cleared on reconnect, got %v", info.DisconnectedAt)
	}
}
