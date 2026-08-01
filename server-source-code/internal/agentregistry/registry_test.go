package agentregistry

import (
	"testing"
	"time"
)

// TestRegister_SetsConnectedAndSecure verifies that a fresh Register call lands
// the agent as Connected with the right Secure flag, stamps LastConnectedAt,
// and leaves DisconnectedAt nil.
func TestRegister_SetsConnectedAndSecure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		secure bool
	}{
		{"secure", true},
		{"insecure", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := New()
			before := time.Now().UTC()
			r.Register("api-1", tc.secure)
			after := time.Now().UTC()

			info := r.Get("api-1")
			if !info.Connected {
				t.Fatalf("expected Connected=true, got false")
			}
			if info.Secure != tc.secure {
				t.Fatalf("expected Secure=%v, got %v", tc.secure, info.Secure)
			}
			if info.DisconnectedAt != nil {
				t.Fatalf("expected DisconnectedAt=nil, got %v", info.DisconnectedAt)
			}
			if info.LastConnectedAt == nil {
				t.Fatalf("expected LastConnectedAt to be set, got nil")
			}
			if info.LastConnectedAt.Before(before) || info.LastConnectedAt.After(after) {
				t.Fatalf("LastConnectedAt %v not within [%v, %v]", info.LastConnectedAt, before, after)
			}
		})
	}
}

// TestGet_DisconnectedMasksSecure is the direct regression guard for the bug
// where a prior WSS connection's Secure=true survived a disconnect, so the WS
// pill briefly showed "WSS" for an agent that was reconnecting via plain WS.
// Get must return Secure=false whenever Connected=false, regardless of what
// was last set in the meta map.
func TestGet_DisconnectedMasksSecure(t *testing.T) {
	t.Parallel()
	r := New()
	r.Register("api-1", true) // simulate prior WSS session
	r.Unregister("api-1")

	info := r.Get("api-1")
	if info.Connected {
		t.Fatalf("expected Connected=false after Unregister, got true")
	}
	if info.Secure {
		t.Fatalf("regression: Get returned Secure=true for a disconnected agent — the WS pill would falsely render as WSS")
	}
	if info.DisconnectedAt == nil {
		t.Fatalf("expected DisconnectedAt to be preserved across the disconnect, got nil")
	}
	if info.LastConnectedAt == nil {
		t.Fatalf("expected LastConnectedAt to be preserved across the disconnect, got nil")
	}
}

// TestGet_UnknownApiID_ReturnsZeroValue ensures querying an api_id we've never
// seen returns the zero value rather than a partially-populated entry.
func TestGet_UnknownApiID_ReturnsZeroValue(t *testing.T) {
	t.Parallel()
	r := New()
	info := r.Get("never-registered")
	if info.Connected || info.Secure {
		t.Fatalf("expected zero ConnectionInfo for unknown apiID, got %+v", info)
	}
	if info.LastConnectedAt != nil || info.DisconnectedAt != nil {
		t.Fatalf("expected nil timestamps for unknown apiID, got %+v", info)
	}
}

// TestRegister_Reconnect_OverwritesSecureAndClearsDisconnect verifies that a
// reconnect after Unregister cleanly transitions the agent back to Connected
// with the new Secure value and a cleared DisconnectedAt.
func TestRegister_Reconnect_OverwritesSecureAndClearsDisconnect(t *testing.T) {
	t.Parallel()
	r := New()

	r.Register("api-1", true)
	r.Unregister("api-1")

	// Confirm the disconnected snapshot before the reconnect.
	before := r.Get("api-1")
	if before.Connected || before.DisconnectedAt == nil {
		t.Fatalf("precondition failed: expected disconnected with timestamp, got %+v", before)
	}

	// Reconnect via plain WS (secure=false) — the new value must win.
	r.Register("api-1", false)
	after := r.Get("api-1")

	if !after.Connected {
		t.Fatalf("expected Connected=true after reconnect, got false")
	}
	if after.Secure {
		t.Fatalf("expected Secure=false after Register(false), got true — stale value leaked through")
	}
	if after.DisconnectedAt != nil {
		t.Fatalf("expected DisconnectedAt cleared on reconnect, got %v", after.DisconnectedAt)
	}
}

// TestUnregister_StampsDisconnectedAt ensures the registry records a fresh
// DisconnectedAt timestamp the first time an agent's WS drops.
func TestUnregister_StampsDisconnectedAt(t *testing.T) {
	t.Parallel()
	r := New()
	r.Register("api-1", false)

	before := time.Now().UTC()
	r.Unregister("api-1")
	after := time.Now().UTC()

	info := r.Get("api-1")
	if info.DisconnectedAt == nil {
		t.Fatalf("expected DisconnectedAt to be stamped, got nil")
	}
	if info.DisconnectedAt.Before(before) || info.DisconnectedAt.After(after) {
		t.Fatalf("DisconnectedAt %v not within [%v, %v]", info.DisconnectedAt, before, after)
	}
	if info.LastConnectedAt == nil {
		t.Fatalf("expected LastConnectedAt to be preserved, got nil")
	}
}

// TestUnregister_DoubleCallDoesNotBumpDisconnectedAt guards against a flap
// where Unregister fires twice (e.g. close handler + readPump exit). The
// timestamp must reflect the original drop, not the latest call.
func TestUnregister_DoubleCallDoesNotBumpDisconnectedAt(t *testing.T) {
	t.Parallel()
	r := New()
	r.Register("api-1", false)

	r.Unregister("api-1")
	first := r.Get("api-1").DisconnectedAt
	if first == nil {
		t.Fatalf("expected DisconnectedAt after first Unregister, got nil")
	}

	// Sleep a tick so a buggy implementation that re-stamps would produce a
	// detectably different timestamp.
	time.Sleep(2 * time.Millisecond)
	r.Unregister("api-1")
	second := r.Get("api-1").DisconnectedAt
	if second == nil {
		t.Fatalf("expected DisconnectedAt to remain after second Unregister, got nil")
	}
	if !second.Equal(*first) {
		t.Fatalf("DisconnectedAt was bumped by the second Unregister: %v -> %v", first, second)
	}
}

// TestUnregister_NoPriorRegister_StampsDisconnectedAt covers the cold-start
// path where Unregister fires for an apiID we've never seen Register'd —
// e.g. a stale close fired after a server restart wiped the in-memory map.
func TestUnregister_NoPriorRegister_StampsDisconnectedAt(t *testing.T) {
	t.Parallel()
	r := New()

	before := time.Now().UTC()
	r.Unregister("api-1")
	after := time.Now().UTC()

	info := r.Get("api-1")
	if info.Connected || info.Secure {
		t.Fatalf("expected disconnected zero-secure entry, got %+v", info)
	}
	if info.DisconnectedAt == nil {
		t.Fatalf("expected DisconnectedAt to be stamped on cold-Unregister, got nil")
	}
	if info.DisconnectedAt.Before(before) || info.DisconnectedAt.After(after) {
		t.Fatalf("DisconnectedAt %v not within [%v, %v]", info.DisconnectedAt, before, after)
	}
}

// TestGetBulk_AppliesSameMaskAsGet exercises the bulk path with a mix of
// connected/disconnected/unknown api_ids and verifies the Secure-on-disconnect
// mask is applied consistently with Get.
func TestGetBulk_AppliesSameMaskAsGet(t *testing.T) {
	t.Parallel()
	r := New()

	// Connected via WSS.
	r.Register("connected-secure", true)
	// Connected via plain WS.
	r.Register("connected-insecure", false)
	// Previously WSS, now disconnected — the regression case.
	r.Register("disconnected-was-secure", true)
	r.Unregister("disconnected-was-secure")
	// Previously plain WS, now disconnected.
	r.Register("disconnected-was-insecure", false)
	r.Unregister("disconnected-was-insecure")

	bulk := r.GetBulk([]string{
		"connected-secure",
		"connected-insecure",
		"disconnected-was-secure",
		"disconnected-was-insecure",
		"never-seen",
	})

	if got := bulk["connected-secure"]; !got.Connected || !got.Secure {
		t.Fatalf("connected-secure: want Connected=true Secure=true, got %+v", got)
	}
	if got := bulk["connected-insecure"]; !got.Connected || got.Secure {
		t.Fatalf("connected-insecure: want Connected=true Secure=false, got %+v", got)
	}
	if got := bulk["disconnected-was-secure"]; got.Connected || got.Secure {
		t.Fatalf("disconnected-was-secure: regression — want Connected=false Secure=false, got %+v", got)
	} else if got.DisconnectedAt == nil {
		t.Fatalf("disconnected-was-secure: expected DisconnectedAt preserved, got nil")
	}
	if got := bulk["disconnected-was-insecure"]; got.Connected || got.Secure {
		t.Fatalf("disconnected-was-insecure: want Connected=false Secure=false, got %+v", got)
	}
	if got := bulk["never-seen"]; got.Connected || got.Secure || got.DisconnectedAt != nil || got.LastConnectedAt != nil {
		t.Fatalf("never-seen: want zero ConnectionInfo, got %+v", got)
	}
}

// TestGetBulk_PreservesUnderlyingMetaMap is a defensive test to ensure GetBulk
// returns a copy/value, not a pointer back into the registry's internal map.
// A future refactor that returns shared references would let callers mutate
// internal state through the wire layer — which would be very bad.
func TestGetBulk_PreservesUnderlyingMetaMap(t *testing.T) {
	t.Parallel()
	r := New()
	r.Register("api-1", true)

	bulk := r.GetBulk([]string{"api-1"})
	mutated := bulk["api-1"]
	mutated.Secure = false
	bulk["api-1"] = mutated

	internal := r.Get("api-1")
	if !internal.Secure {
		t.Fatalf("Get returned a stale/mutated value — bulk map appears to share memory with internal meta")
	}
}

// TestIsConnected covers the live boolean check used by the SendJSON /
// SendMessage hot paths. It must reflect the real connection state, not the
// preserved meta entry.
func TestIsConnected(t *testing.T) {
	t.Parallel()
	r := New()

	if r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=false for unknown apiID")
	}

	r.Register("api-1", false)
	if !r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=true after Register")
	}

	r.Unregister("api-1")
	if r.IsConnected("api-1") {
		t.Fatalf("expected IsConnected=false after Unregister, got true — stale meta leaked through")
	}
}
