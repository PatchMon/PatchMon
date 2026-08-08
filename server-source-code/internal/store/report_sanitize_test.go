package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestSanitizeText covers the two things Postgres refuses to store: U+0000 in
// any column, and byte sequences that are not valid UTF-8.
func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"clean ascii", "Mesh Agent", "Mesh Agent"},
		{"clean non-ascii preserved", "Mise à jour de la sélection", "Mise à jour de la sélection"},
		{"trailing nuls", "2026-02-16 01:43:44.000+01:00\x00\x00", "2026-02-16 01:43:44.000+01:00"},
		{"embedded nul", "Microsoft\x00 Visual C++", "Microsoft Visual C++"},
		{"only nuls", "\x00\x00", ""},
		{"invalid utf8 repaired", "caf\xe9", "caf\uFFFD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeText(tc.in); got != tc.want {
				t.Fatalf("sanitizeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSanitizeReportPayload_StripsNulsEverywhere is the regression guard for
// the 22P05 report failure: a single NUL anywhere in an agent payload used to
// abort the whole transaction.
func TestSanitizeReportPayload_StripsNulsEverywhere(t *testing.T) {
	avail := "1.2.3\x00"
	payload := &ReportPayload{
		Packages: []ReportPackage{{
			Name:             "Mesh Agent\x00",
			Description:      "3,3 MB\x00",
			Category:         "Application",
			CurrentVersion:   "2026-02-16 01:43:44.000+01:00\x00\x00",
			AvailableVersion: &avail,
			SourceRepository: "local\x00",
			WUAGuid:          "guid\x00",
			WUAKb:            "KB123\x00",
			WUASeverity:      "Critical\x00",
			WUASupportURL:    "https://example.test\x00",
			WUACategories:    []string{"Security Updates\x00"},
		}},
		Repositories: []ReportRepository{{
			Name:         "Microsoft Update\x00",
			URL:          "https://update.microsoft.com\x00",
			Distribution: "d\x00",
			Components:   "c\x00",
			RepoType:     "wu\x00",
		}},
		OSType:                 "Windows\x00",
		OSVersion:              "25H2\x00",
		Hostname:               "host\x00",
		IP:                     "10.0.0.1\x00",
		Architecture:           "amd64\x00",
		AgentVersion:           "2.0.2\x00",
		MachineID:              "mid\x00",
		KernelVersion:          "kv\x00",
		InstalledKernelVersion: "ikv\x00",
		SELinuxStatus:          "n/a\x00",
		SystemUptime:           "1d\x00",
		CPUModel:               "Intel\x00",
		GatewayIP:              "10.0.0.254\x00",
		RebootReason:           "patching\x00",
		PackageManager:         "windows\x00",
		DNSServers:             []string{"1.1.1.1\x00"},
	}

	sanitizeReportPayload(payload)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The jsonb bind is what actually rejects the escape, so assert on the
	// encoded form rather than field by field.
	if strings.Contains(string(raw), `\u0000`) {
		t.Fatalf("payload still encodes a NUL escape: %s", raw)
	}
	if payload.Packages[0].Name != "Mesh Agent" {
		t.Fatalf("package name = %q", payload.Packages[0].Name)
	}
	if *payload.Packages[0].AvailableVersion != "1.2.3" {
		t.Fatalf("available version = %q", *payload.Packages[0].AvailableVersion)
	}
	if payload.Packages[0].WUACategories[0] != "Security Updates" {
		t.Fatalf("wua category = %q", payload.Packages[0].WUACategories[0])
	}
	if payload.DNSServers[0] != "1.1.1.1" {
		t.Fatalf("dns server = %q", payload.DNSServers[0])
	}
}

// TestSanitizeReportPayload_RunsBeforeDedup documents the ordering contract:
// names that differ only by a NUL collapse to one name, and the dedup in
// ProcessReport (which runs after this) is what stops that reaching
// ON CONFLICT as a 21000 cardinality violation.
func TestSanitizeReportPayload_RunsBeforeDedup(t *testing.T) {
	payload := &ReportPayload{
		Packages: []ReportPackage{
			{Name: "vim"},
			{Name: "vim\x00"},
		},
	}
	sanitizeReportPayload(payload)
	if payload.Packages[0].Name != payload.Packages[1].Name {
		t.Fatalf("expected collapsed names, got %q and %q",
			payload.Packages[0].Name, payload.Packages[1].Name)
	}
}

func TestSanitizeRawJSON(t *testing.T) {
	t.Run("clean blob passes through untouched", func(t *testing.T) {
		in := json.RawMessage(`{"b":1,"a":"x","n":123456789012345}`)
		got := sanitizeRawJSON(in)
		if string(got) != string(in) {
			t.Fatalf("clean blob was rewritten: %s", got)
		}
	})

	t.Run("nul escape removed", func(t *testing.T) {
		in := json.RawMessage(`{"name":"disk\u0000","size":42}`)
		got := sanitizeRawJSON(in)
		if strings.Contains(string(got), `\u0000`) {
			t.Fatalf("nul escape survived: %s", got)
		}
		var out map[string]any
		if err := json.Unmarshal(got, &out); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		if out["name"] != "disk" {
			t.Fatalf("name = %v", out["name"])
		}
	})

	t.Run("large integers keep their exact form", func(t *testing.T) {
		in := json.RawMessage(`{"a":"x\u0000","size":9007199254740993}`)
		got := sanitizeRawJSON(in)
		if !strings.Contains(string(got), "9007199254740993") {
			t.Fatalf("integer precision lost: %s", got)
		}
	})

	t.Run("escaped backslash is not mistaken for an escape", func(t *testing.T) {
		in := json.RawMessage(`{"path":"C:\\u0000dir","x":"y\u0000"}`)
		got := sanitizeRawJSON(in)
		var out map[string]any
		if err := json.Unmarshal(got, &out); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		if out["path"] != `C:\u0000dir` {
			t.Fatalf("literal backslash sequence was corrupted: %v", out["path"])
		}
		if out["x"] != "y" {
			t.Fatalf("x = %v", out["x"])
		}
	})

	t.Run("malformed blob is returned unchanged", func(t *testing.T) {
		in := json.RawMessage(`{"a":"x\u0000"`)
		if got := sanitizeRawJSON(in); !reflect.DeepEqual(got, in) {
			t.Fatalf("malformed blob was rewritten: %s", got)
		}
	})
}
