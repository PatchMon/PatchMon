package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ssgHandler builds a ComplianceHandler wired to nothing but a content
// directory. Every assertion below is reached before any store is touched.
func ssgHandler(dir string) *ComplianceHandler {
	return NewComplianceHandler(nil, nil, nil, nil, nil, nil, dir, nil, nil)
}

func populatedSSGDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".ssg-version"), []byte("0.1.81\n"), 0o600); err != nil {
		t.Fatalf("write version marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ssg-debian13-ds.xml"), []byte("<xml/>"), 0o600); err != nil {
		t.Fatalf("write datastream: %v", err)
	}

	return dir
}

// The session-facing handler stays a 200 either way so Compliance Settings
// still renders; the UI branches on an empty version.
func TestSSGVersionAlwaysSucceeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dir         func(*testing.T) string
		wantVersion string
	}{
		{name: "content present", dir: populatedSSGDir, wantVersion: "0.1.81"},
		{name: "content missing", dir: func(t *testing.T) string { t.Helper(); return t.TempDir() }, wantVersion: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			ssgHandler(tt.dir(t)).SSGVersion(rec, httptest.NewRequest(http.MethodGet, "/compliance/ssg-info", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var body struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", body.Version, tt.wantVersion)
			}
		})
	}
}

// The agent-facing decision, isolated from the credential check so it can be
// asserted directly: a server with no bundled content must not be able to
// answer an agent with a success. Treating that as success is what sent agents
// looking elsewhere for content (#842).
func TestAgentSSGVersionPayloadRequiresContent(t *testing.T) {
	t.Parallel()

	payload, ok := ssgHandler(populatedSSGDir(t)).agentSSGVersionPayload()
	if !ok {
		t.Fatal("ok = false with content present, want true")
	}
	if payload["version"] != "0.1.81" {
		t.Errorf("version = %v, want %q", payload["version"], "0.1.81")
	}

	if _, ok := ssgHandler(t.TempDir()).agentSSGVersionPayload(); ok {
		t.Error("ok = true with no content, want false so the handler 503s")
	}
	if _, ok := ssgHandler("").agentSSGVersionPayload(); ok {
		t.Error("ok = true with no content dir, want false so the handler 503s")
	}
}

// SSGContent joins a caller-supplied name onto a filesystem path, so the regex
// guarding it is load-bearing.
func TestSSGFilenameRegexRejectsTraversal(t *testing.T) {
	t.Parallel()

	rejected := []string{
		"../../etc/passwd",
		"../.ssg-version",
		"ssg-debian13-ds.xml/../../etc/passwd",
		"/etc/passwd",
		"ssg-../-ds.xml",
		"ssg-debian13-ds.xml\n",
		"SSG-DEBIAN13-DS.XML",
		"ssg--ds.xml",
		"",
	}
	for _, name := range rejected {
		if ssgFilenameRe.MatchString(name) {
			t.Errorf("ssgFilenameRe accepted %q, want rejected", name)
		}
	}

	accepted := []string{"ssg-debian13-ds.xml", "ssg-ubuntu2404-ds.xml", "ssg-rhel9-ds.xml"}
	for _, name := range accepted {
		if !ssgFilenameRe.MatchString(name) {
			t.Errorf("ssgFilenameRe rejected %q, want accepted", name)
		}
	}
}

// Both agent-facing SSG endpoints were previously unauthenticated despite their
// doc comments claiming otherwise.
func TestAgentSSGEndpointsRequireCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		handler func(*ComplianceHandler) http.HandlerFunc
		headers map[string]string
	}{
		{
			name:    "version without credentials",
			target:  "/compliance/ssg-version",
			handler: func(h *ComplianceHandler) http.HandlerFunc { return h.AgentSSGVersion },
		},
		{
			name:    "content without credentials",
			target:  "/compliance/ssg-content/ssg-debian13-ds.xml",
			handler: func(h *ComplianceHandler) http.HandlerFunc { return h.SSGContent },
		},
		{
			name:    "version with a blank api key",
			target:  "/compliance/ssg-version",
			handler: func(h *ComplianceHandler) http.HandlerFunc { return h.AgentSSGVersion },
			headers: map[string]string{"X-API-ID": "some-agent"},
		},
		{
			name:    "content with a blank api id",
			target:  "/compliance/ssg-content/ssg-debian13-ds.xml",
			handler: func(h *ComplianceHandler) http.HandlerFunc { return h.SSGContent },
			headers: map[string]string{"X-API-KEY": "some-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A fully populated directory, so a 401 cannot be mistaken for
			// "there was nothing to serve anyway".
			h := ssgHandler(populatedSSGDir(t))

			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			tt.handler(h)(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestReadSSGVersion(t *testing.T) {
	t.Parallel()

	if got := ssgHandler(populatedSSGDir(t)).readSSGVersion(); got != "0.1.81" {
		t.Errorf("readSSGVersion() = %q, want %q", got, "0.1.81")
	}
	if got := ssgHandler(t.TempDir()).readSSGVersion(); got != "" {
		t.Errorf("readSSGVersion() with no marker = %q, want empty", got)
	}
	if got := ssgHandler("").readSSGVersion(); got != "" {
		t.Errorf("readSSGVersion() with no dir = %q, want empty", got)
	}
}
