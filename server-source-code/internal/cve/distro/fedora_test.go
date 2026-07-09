package distro

import (
	"encoding/json"
	"testing"
)

// bodhiFixture is a realistic (trimmed) Bodhi response for
// /updates/?packages=kernel&search=CVE-2024-0000 . It exercises: two stable
// kernel updates for the same release (lowest NVR must win), a stable update for
// another release, a testing update (affected, no fixed version), a subpackage
// build that must not be mistaken for the kernel NVR, and a non-Fedora (EPEL)
// release that must be ignored.
const bodhiFixture = `{
  "updates": [
    {
      "alias": "FEDORA-2024-aaaaaaaaaa",
      "status": "stable",
      "release": {"name": "F39"},
      "builds": [
        {"nvr": "kernel-headers-6.8.11-200.fc39"},
        {"nvr": "kernel-6.8.11-200.fc39"}
      ]
    },
    {
      "alias": "FEDORA-2024-bbbbbbbbbb",
      "status": "stable",
      "release": {"name": "F39"},
      "builds": [{"nvr": "kernel-6.8.9-100.fc39"}]
    },
    {
      "alias": "FEDORA-2024-cccccccccc",
      "status": "stable",
      "release": {"name": "F40"},
      "builds": [{"nvr": "kernel-6.8.9-300.fc40"}]
    },
    {
      "alias": "FEDORA-2024-dddddddddd",
      "status": "testing",
      "release": {"name": "F41"},
      "builds": [{"nvr": "kernel-6.11.3-300.fc41"}]
    },
    {
      "alias": "FEDORA-EPEL-2024-eeeeeeeeee",
      "status": "stable",
      "release": {"name": "EPEL-9"},
      "builds": [{"nvr": "kernel-5.14.0-100.el9"}]
    }
  ],
  "page": 1,
  "pages": 1,
  "total": 5
}`

func TestFedoraParse(t *testing.T) {
	var resp bodhiResp
	if err := json.Unmarshal([]byte(bodhiFixture), &resp); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	f := NewFedora()
	set := &AdvisorySet{CVEID: "CVE-2024-0000", Distro: "fedora", ByRelease: map[string]Advisory{}}
	f.parse(resp.Updates, set)
	set.Known = len(set.ByRelease) > 0

	if !set.Known {
		t.Fatal("expected Known=true")
	}

	tests := []struct {
		release  string
		present  bool
		decision Decision
		fixed    string
	}{
		{"39", true, DecisionFixed, "6.8.9-100.fc39"}, // lowest of two stable NVRs
		{"40", true, DecisionFixed, "6.8.9-300.fc40"},
		{"41", true, DecisionAffected, ""}, // testing => affected, no fix
		{"9", false, "", ""},               // EPEL-9 must be ignored
	}

	for _, tt := range tests {
		adv, ok := set.ByRelease[tt.release]
		if ok != tt.present {
			t.Errorf("release %s: present=%v, want %v (got %+v)", tt.release, ok, tt.present, adv)
			continue
		}
		if !tt.present {
			continue
		}
		if adv.Decision != tt.decision {
			t.Errorf("release %s: decision=%q, want %q", tt.release, adv.Decision, tt.decision)
		}
		if adv.FixedVersion != tt.fixed {
			t.Errorf("release %s: fixed=%q, want %q", tt.release, adv.FixedVersion, tt.fixed)
		}
	}

	if len(set.ByRelease) != 3 {
		t.Errorf("ByRelease size=%d, want 3", len(set.ByRelease))
	}
}

func TestFedoraParseNoUpdates(t *testing.T) {
	f := NewFedora()
	set := &AdvisorySet{CVEID: "CVE-9999-0000", Distro: "fedora", ByRelease: map[string]Advisory{}}
	f.parse(nil, set)
	set.Known = len(set.ByRelease) > 0
	if set.Known {
		t.Error("expected Known=false when no kernel updates exist")
	}
}

func TestFedoraReleaseKey(t *testing.T) {
	f := NewFedora()
	cases := map[string]string{
		"40":           "40",
		"39":           "39",
		"41 (Rawhide)": "41",
		"Fedora 38":    "38",
		"":             "",
		"unknown":      "",
	}
	for in, want := range cases {
		if got := f.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestFedoraCompareVersions(t *testing.T) {
	f := NewFedora()
	if f.CompareVersions("6.8.9-100.fc39", "6.8.11-200.fc39") >= 0 {
		t.Error("6.8.9 should sort below 6.8.11")
	}
	if f.CompareVersions("6.8.9-100.fc39", "6.8.9-100.fc39") != 0 {
		t.Error("equal NVRs should compare equal")
	}
	if f.CompareVersions("6.8.9-200.fc39", "6.8.9-100.fc39") <= 0 {
		t.Error("higher release should sort above")
	}
}
