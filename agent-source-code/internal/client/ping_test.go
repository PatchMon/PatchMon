package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"patchmon-agent/internal/config"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// newTestClient wires a Client against a test HTTP server, using a throwaway
// config directory so nothing touches /etc/patchmon.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	tmp := t.TempDir()
	mgr := config.New()
	mgr.SetConfigFile(filepath.Join(tmp, "config.yml"))

	cfg := mgr.GetConfig()
	cfg.PatchmonServer = serverURL
	cfg.APIVersion = "v1"
	cfg.CredentialsFile = filepath.Join(tmp, "credentials.yml")
	cfg.LogFile = filepath.Join(tmp, "agent.log")

	if err := mgr.SaveCredentials("test-api-id", "test-api-key"); err != nil {
		t.Fatalf("failed to save test credentials: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	return New(mgr, logger)
}

// TestPingHashGateCapability checks that the agent reads the server's
// hash-gate capability marker off the wire in both directions. The literal
// response bodies are the contract: a pre-2.0.3 server returns 200 with no
// marker and no requestFull, and the agent must be able to tell that apart
// from a v2.0.3 server reporting steady state.
func TestPingHashGateCapability(t *testing.T) {
	tests := []struct {
		name            string
		responseBody    string
		wantHashGate    bool
		wantRequestFull []string
	}{
		{
			name:            "hash_gate_capable_server_requests_sections",
			responseBody:    `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","hashGate":true,"requestFull":["packages","repos"]}`,
			wantHashGate:    true,
			wantRequestFull: []string{"packages", "repos"},
		},
		{
			name:         "hash_gate_capable_server_steady_state",
			responseBody: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","hashGate":true}`,
			wantHashGate: true,
		},
		{
			name:         "legacy_server_omits_hash_gate",
			responseBody: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","friendlyName":"web-01"}`,
			wantHashGate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			var gotPath, gotAPIID string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAPIID = r.Header.Get("X-API-ID")
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read request body: %v", err)
				}
				gotBody = body
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tt.responseBody)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			req := &models.PingRequest{
				AgentVersion: "2.0.3",
				Hashes:       models.PingHashes{PackagesHash: "abc123"},
			}

			resp, err := c.Ping(context.Background(), req)
			if err != nil {
				t.Fatalf("Ping() returned error: %v", err)
			}

			if resp.HashGate != tt.wantHashGate {
				t.Fatalf("PingResponse.HashGate = %v, want %v", resp.HashGate, tt.wantHashGate)
			}
			if len(resp.RequestFull) != len(tt.wantRequestFull) {
				t.Fatalf("PingResponse.RequestFull = %v, want %v", resp.RequestFull, tt.wantRequestFull)
			}
			for i, section := range tt.wantRequestFull {
				if resp.RequestFull[i] != section {
					t.Fatalf("PingResponse.RequestFull[%d] = %q, want %q", i, resp.RequestFull[i], section)
				}
			}

			if gotPath != "/api/v1/hosts/ping" {
				t.Fatalf("ping posted to %q, want /api/v1/hosts/ping", gotPath)
			}
			if gotAPIID != "test-api-id" {
				t.Fatalf("X-API-ID = %q, want test-api-id", gotAPIID)
			}
			if len(gotBody) == 0 {
				t.Fatal("expected the ping request to carry a hash body")
			}
		})
	}
}

// TestPingNilRequestSendsEmptyBody covers the startup heartbeat, which pings
// before any collector has run. Pre-2.0.3 servers reject nothing here because
// they ignore ping bodies entirely.
func TestPingNilRequestSendsEmptyBody(t *testing.T) {
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"message":"pong","timestamp":"2026-07-27T10:00:00Z"}`)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	resp, err := c.Ping(context.Background(), nil)
	if err != nil {
		t.Fatalf("Ping() returned error: %v", err)
	}
	if resp.HashGate {
		t.Fatal("expected HashGate false for a response without the marker")
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected an empty body for a nil ping request, got %q", string(gotBody))
	}
}
