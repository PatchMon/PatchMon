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

func TestSanitizeDockerPayload_StripsNulsEverywhere(t *testing.T) {
	nul := "\x00"
	p := &DockerReceivePayload{
		Containers: []DockerReceiveContainer{{
			ContainerID: "abc" + nul,
			Name:        "web" + nul,
			ImageName:   "nginx" + nul,
			ImageTag:    "1.27" + nul,
			Status:      "Up 3 hours" + nul,
			State:       "running" + nul,
			Ports:       map[string]string{"80/tcp" + nul: "8080" + nul},
			Labels:      map[string]string{"com.example" + nul: "value" + nul},
		}},
		Images:   []DockerReceiveImage{{Repository: "nginx" + nul, Tag: "latest" + nul, Digest: "sha256:x" + nul}},
		Volumes:  []DockerReceiveVolume{{Name: "data" + nul, Mountpoint: "/var/lib/x" + nul, Options: map[string]string{"o" + nul: "bind" + nul}}},
		Networks: []DockerReceiveNetwork{{Name: "bridge" + nul, Driver: "bridge" + nul, Labels: map[string]string{"k" + nul: "v" + nul}}},
		Updates:  []DockerReceiveImageUpdate{{Repository: "nginx" + nul, AvailableTag: "1.28" + nul}},
	}

	sanitizeDockerPayload(p)

	c := p.Containers[0]
	for name, got := range map[string]string{
		"ContainerID": c.ContainerID, "Name": c.Name, "ImageName": c.ImageName,
		"ImageTag": c.ImageTag, "Status": c.Status, "State": c.State,
		"image.Repository": p.Images[0].Repository, "image.Tag": p.Images[0].Tag,
		"image.Digest": p.Images[0].Digest, "volume.Name": p.Volumes[0].Name,
		"volume.Mountpoint": p.Volumes[0].Mountpoint, "network.Name": p.Networks[0].Name,
		"network.Driver": p.Networks[0].Driver, "update.Repository": p.Updates[0].Repository,
		"update.AvailableTag": p.Updates[0].AvailableTag,
	} {
		if strings.ContainsRune(got, 0) {
			t.Errorf("%s still contains NUL: %q", name, got)
		}
	}

	// Maps must be cleaned on both sides. A NUL in a key survives
	// json.Marshal as an escape sequence and aborts the jsonb write.
	for label, m := range map[string]map[string]string{
		"container.Ports": c.Ports, "container.Labels": c.Labels,
		"volume.Options": p.Volumes[0].Options, "network.Labels": p.Networks[0].Labels,
	} {
		for k, v := range m {
			if strings.ContainsRune(k, 0) {
				t.Errorf("%s key still contains NUL: %q", label, k)
			}
			if strings.ContainsRune(v, 0) {
				t.Errorf("%s value still contains NUL: %q", label, v)
			}
		}
	}
	if got := c.Labels["com.example"]; got != "value" {
		t.Errorf("label not reachable under its cleaned key, got %q", got)
	}
}

func TestSanitizeComplianceScans_StripsNulsEverywhere(t *testing.T) {
	nul := "\x00"
	scans := []SubmittedScan{{
		ProfileName: "CIS" + nul,
		ProfileType: "openscap" + nul,
		Status:      "completed" + nul,
		Error:       "none" + nul,
		Results: []SubmittedScanResult{{
			RuleRef: "r1" + nul, Title: "Ensure X" + nul, Description: "desc" + nul,
			Severity: "high" + nul, Section: "1.1" + nul, Remediation: "do y" + nul,
			Status: "fail" + nul, Finding: "f" + nul, Actual: "a" + nul, Expected: "e" + nul,
		}},
	}}

	sanitizeComplianceScans(scans)

	s := scans[0]
	r := s.Results[0]
	for name, got := range map[string]string{
		"ProfileName": s.ProfileName, "ProfileType": s.ProfileType, "Status": s.Status,
		"Error": s.Error, "RuleRef": r.RuleRef, "Title": r.Title, "Description": r.Description,
		"Severity": r.Severity, "Section": r.Section, "Remediation": r.Remediation,
		"result.Status": r.Status, "Finding": r.Finding, "Actual": r.Actual, "Expected": r.Expected,
	} {
		if strings.ContainsRune(got, 0) {
			t.Errorf("%s still contains NUL: %q", name, got)
		}
	}
	if s.ProfileName != "CIS" || r.Title != "Ensure X" {
		t.Errorf("content altered beyond NUL removal: %q / %q", s.ProfileName, r.Title)
	}
}
