package handler

import (
	"math"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
)

// TestToWSStatusResponse_Connected_NilDisconnectedSecondsAgo verifies the
// happy-path wire shape: a connected agent reports no `disconnected_seconds_ago`
// regardless of any LastConnectedAt or DisconnectedAt fields the registry may
// carry.
func TestToWSStatusResponse_Connected_NilDisconnectedSecondsAgo(t *testing.T) {
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
			now := time.Now().UTC()
			info := agentregistry.ConnectionInfo{
				Connected:       true,
				Secure:          tc.secure,
				LastConnectedAt: &now,
			}
			got := toWSStatusResponse(info)

			if !got.Connected {
				t.Fatalf("expected Connected=true, got false")
			}
			if got.Secure != tc.secure {
				t.Fatalf("expected Secure=%v, got %v", tc.secure, got.Secure)
			}
			if got.DisconnectedSecondsAgo != nil {
				t.Fatalf("expected DisconnectedSecondsAgo=nil for connected agent, got %v", *got.DisconnectedSecondsAgo)
			}
		})
	}
}

// TestToWSStatusResponse_DisconnectedWithTimestamp_PopulatesField checks that
// `disconnected_seconds_ago` is computed from DisconnectedAt as a non-negative
// integer. We use a 30s offset which mirrors the default host_down threshold.
func TestToWSStatusResponse_DisconnectedWithTimestamp_PopulatesField(t *testing.T) {
	t.Parallel()
	disconnected := time.Now().UTC().Add(-30 * time.Second)
	info := agentregistry.ConnectionInfo{
		Connected:      false,
		Secure:         false,
		DisconnectedAt: &disconnected,
	}

	got := toWSStatusResponse(info)
	if got.Connected {
		t.Fatalf("expected Connected=false, got true")
	}
	if got.DisconnectedSecondsAgo == nil {
		t.Fatalf("expected DisconnectedSecondsAgo to be populated, got nil")
	}
	if *got.DisconnectedSecondsAgo < 29 || *got.DisconnectedSecondsAgo > 31 {
		t.Fatalf("expected DisconnectedSecondsAgo ~30, got %d", *got.DisconnectedSecondsAgo)
	}
}

// TestToWSStatusResponse_DisconnectedNoTimestamp_NilField covers the cold-start
// case where the registry never recorded a DisconnectedAt (e.g. server
// restarted while the agent was already offline). The pill code uses this
// nil to render an "unknown duration" tooltip rather than implying a fresh
// drop.
func TestToWSStatusResponse_DisconnectedNoTimestamp_NilField(t *testing.T) {
	t.Parallel()
	info := agentregistry.ConnectionInfo{
		Connected: false,
		Secure:    false,
	}
	got := toWSStatusResponse(info)
	if got.Connected {
		t.Fatalf("expected Connected=false, got true")
	}
	if got.DisconnectedSecondsAgo != nil {
		t.Fatalf("expected nil DisconnectedSecondsAgo for missing timestamp, got %v", *got.DisconnectedSecondsAgo)
	}
}

// TestToWSStatusResponse_NegativeSecs_ClampedToZero covers clock skew: if
// DisconnectedAt is in the future relative to now (NTP correction, container
// clock jump), we must return 0 rather than a negative value the frontend
// would mis-render as "disconnected -5s ago".
func TestToWSStatusResponse_NegativeSecs_ClampedToZero(t *testing.T) {
	t.Parallel()
	future := time.Now().UTC().Add(10 * time.Second)
	info := agentregistry.ConnectionInfo{
		Connected:      false,
		DisconnectedAt: &future,
	}

	got := toWSStatusResponse(info)
	if got.DisconnectedSecondsAgo == nil {
		t.Fatalf("expected DisconnectedSecondsAgo to be set even for future timestamps, got nil")
	}
	if *got.DisconnectedSecondsAgo != 0 {
		t.Fatalf("expected DisconnectedSecondsAgo=0 for future DisconnectedAt, got %d", *got.DisconnectedSecondsAgo)
	}
}

// TestToWSStatusResponse_HugeSecs_CappedAtMaxInt32 protects the JSON encoder
// from numbers larger than int32 — without the cap, a registry entry with a
// DisconnectedAt from years ago could overflow on 32-bit JS clients (signed
// int boundary in JS is 2^53 but the wire-side cap is more conservative).
func TestToWSStatusResponse_HugeSecs_CappedAtMaxInt32(t *testing.T) {
	t.Parallel()
	veryOld := time.Now().UTC().Add(-time.Duration(math.MaxInt32+1000) * time.Second)
	info := agentregistry.ConnectionInfo{
		Connected:      false,
		DisconnectedAt: &veryOld,
	}

	got := toWSStatusResponse(info)
	if got.DisconnectedSecondsAgo == nil {
		t.Fatalf("expected DisconnectedSecondsAgo to be populated, got nil")
	}
	if *got.DisconnectedSecondsAgo != math.MaxInt32 {
		t.Fatalf("expected DisconnectedSecondsAgo capped at MaxInt32 (%d), got %d", math.MaxInt32, *got.DisconnectedSecondsAgo)
	}
}

// TestToWSStatusResponse_PassesSecureThrough confirms the `Secure` flag is
// propagated unchanged. Combined with the registry-side mask
// (TestGet_DisconnectedMasksSecure), this guarantees the wire response never
// shows Secure=true for a disconnected agent — the regression we just fixed.
func TestToWSStatusResponse_PassesSecureThrough(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    agentregistry.ConnectionInfo
		wantSec  bool
		wantConn bool
	}{
		{
			name:     "connected_secure",
			input:    agentregistry.ConnectionInfo{Connected: true, Secure: true},
			wantSec:  true,
			wantConn: true,
		},
		{
			name:     "connected_insecure",
			input:    agentregistry.ConnectionInfo{Connected: true, Secure: false},
			wantSec:  false,
			wantConn: true,
		},
		{
			name:     "disconnected_secure_input_should_be_masked_upstream",
			input:    agentregistry.ConnectionInfo{Connected: false, Secure: true},
			wantSec:  true, // toWSStatusResponse does NOT mask — the registry does.
			wantConn: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toWSStatusResponse(tc.input)
			if got.Secure != tc.wantSec {
				t.Fatalf("Secure: want %v, got %v", tc.wantSec, got.Secure)
			}
			if got.Connected != tc.wantConn {
				t.Fatalf("Connected: want %v, got %v", tc.wantConn, got.Connected)
			}
		})
	}
}
