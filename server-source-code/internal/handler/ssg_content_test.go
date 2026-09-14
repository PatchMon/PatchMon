package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/ssgcontent"
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
		if ssgcontent.FilenameRe.MatchString(name) {
			t.Errorf("ssgcontent.FilenameRe accepted %q, want rejected", name)
		}
	}

	accepted := []string{"ssg-debian13-ds.xml", "ssg-ubuntu2404-ds.xml", "ssg-rhel9-ds.xml"}
	for _, name := range accepted {
		if !ssgcontent.FilenameRe.MatchString(name) {
			t.Errorf("ssgcontent.FilenameRe rejected %q, want accepted", name)
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
		t.Errorf("readSSGVersion() with no content = %q, want empty", got)
	}
	if got := ssgHandler("").readSSGVersion(); got != "" {
		t.Errorf("readSSGVersion() with no dir = %q, want empty", got)
	}
}

// Content laid down from the upstream ComplianceAsCode release has no
// .ssg-version marker, which used to read as an empty content directory: the UI
// said nothing was configured and agents were turned away with a 503 while the
// datastreams sat right there on disk (#1060).
func TestSSGContentWithoutMarkerIsStillServed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<ds:data-stream-collection xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2" xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2">
  <ds:component id="scap_org.open-scap_comp_ssg-debian13-xccdf.xml">
    <xccdf-1.2:Benchmark>
      <xccdf-1.2:version update="https://github.com/ComplianceAsCode/content/releases/latest">0.1.81</xccdf-1.2:version>
    </xccdf-1.2:Benchmark>
  </ds:component>
</ds:data-stream-collection>`
	if err := os.WriteFile(filepath.Join(dir, "ssg-debian13-ds.xml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write datastream: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ssg-version")); !os.IsNotExist(err) {
		t.Fatalf("marker should be absent, stat err = %v", err)
	}

	h := ssgHandler(dir)

	payload, ok := h.agentSSGVersionPayload()
	if !ok {
		t.Fatal("ok = false with unmarked content, want true so agents are served")
	}
	if payload["version"] != "0.1.81" {
		t.Errorf("version = %v, want %q", payload["version"], "0.1.81")
	}

	rec := httptest.NewRecorder()
	h.SSGVersion(rec, httptest.NewRequest(http.MethodGet, "/compliance/ssg-info", nil))
	var got struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != "0.1.81" {
		t.Errorf("version = %q, want %q", got.Version, "0.1.81")
	}
	if len(got.Files) != 1 {
		t.Errorf("files = %v, want one datastream", got.Files)
	}
}
