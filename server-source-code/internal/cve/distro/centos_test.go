package distro

import (
	"encoding/json"
	"testing"
)

// almaErrataFixture is a realistic (trimmed) AlmaLinux errata.full.json document
// for major 9. It exercises:
//   - a normal kernel ALSA (prefer the main `kernel` package for the NVR),
//   - two kernel errata for the same CVE where the earliest-issued (lower NVR)
//     fix must win,
//   - a kernel-rt-only erratum that must be ignored,
//   - a kpatch erratum (title mentions kernel) that must be ignored,
//   - a non-kernel (httpd) erratum that must be ignored,
//   - a kernel erratum with no issued_date that must be skipped,
//   - a kernel erratum carrying a non-zero epoch (epoch prefix in the NVR).
const almaErrataFixture = `{
  "data": [
    {
      "id": "ALSA-2024:1234",
      "title": "Important: kernel security update",
      "issued_date": 1710000000,
      "references": [
        {"type": "self", "id": "ALSA-2024:1234"},
        {"type": "cve", "id": "CVE-2024-1086"}
      ],
      "packages": [
        {"name": "kernel-core", "epoch": "0", "version": "5.14.0", "release": "427.13.1.el9_4", "arch": "x86_64", "filename": "kernel-core-5.14.0-427.13.1.el9_4.x86_64.rpm"},
        {"name": "kernel", "epoch": "0", "version": "5.14.0", "release": "427.13.1.el9_4", "arch": "x86_64", "filename": "kernel-5.14.0-427.13.1.el9_4.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2023:0950",
      "title": "Important: kernel security update",
      "issued_date": 1690000000,
      "references": [
        {"type": "cve", "id": "CVE-2023-1111"}
      ],
      "packages": [
        {"name": "kernel", "epoch": "0", "version": "5.14.0", "release": "284.30.1.el9_2", "arch": "x86_64", "filename": "kernel-5.14.0-284.30.1.el9_2.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2023:0900",
      "title": "Important: kernel security update",
      "issued_date": 1680000000,
      "references": [
        {"type": "cve", "id": "CVE-2023-1111"}
      ],
      "packages": [
        {"name": "kernel", "epoch": "0", "version": "5.14.0", "release": "284.11.1.el9_2", "arch": "x86_64", "filename": "kernel-5.14.0-284.11.1.el9_2.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2024:2000",
      "title": "Important: kernel security update",
      "issued_date": 1712000000,
      "references": [
        {"type": "cve", "id": "CVE-2024-5555"}
      ],
      "packages": [
        {"name": "kernel", "epoch": "2", "version": "5.14.0", "release": "300.el9", "arch": "x86_64", "filename": "kernel-5.14.0-300.el9.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2024:3000",
      "title": "Important: kernel-rt security update",
      "issued_date": 1712500000,
      "references": [
        {"type": "cve", "id": "CVE-2024-9999"}
      ],
      "packages": [
        {"name": "kernel-rt", "epoch": "0", "version": "5.14.0", "release": "427.13.1.rt7.el9_4", "arch": "x86_64", "filename": "kernel-rt-5.14.0-427.13.1.rt7.el9_4.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2024:3100",
      "title": "Important: kernel live patch (kpatch) update",
      "issued_date": 1712600000,
      "references": [
        {"type": "cve", "id": "CVE-2024-8888"}
      ],
      "packages": [
        {"name": "kpatch-patch-5_14_0-427", "epoch": "0", "version": "1", "release": "1.el9_4", "arch": "x86_64", "filename": "kpatch-patch-5_14_0-427-1-1.el9_4.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2024:4000",
      "title": "Important: httpd security update",
      "issued_date": 1712700000,
      "references": [
        {"type": "cve", "id": "CVE-2024-2222"}
      ],
      "packages": [
        {"name": "httpd", "epoch": "0", "version": "2.4.57", "release": "5.el9", "arch": "x86_64", "filename": "httpd-2.4.57-5.el9.x86_64.rpm"}
      ]
    },
    {
      "id": "ALSA-2024:5000",
      "title": "Important: kernel security update",
      "references": [
        {"type": "cve", "id": "CVE-2024-3333"}
      ],
      "packages": [
        {"name": "kernel", "epoch": "0", "version": "5.14.0", "release": "500.el9", "arch": "x86_64", "filename": "kernel-5.14.0-500.el9.x86_64.rpm"}
      ]
    }
  ]
}`

func TestCentOSParse(t *testing.T) {
	var file almaErrataFile
	if err := json.Unmarshal([]byte(almaErrataFixture), &file); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	c := NewCentOS()
	got := c.parse(&file, "9")

	tests := []struct {
		cve      string
		present  bool
		decision Decision
		fixed    string
	}{
		{"CVE-2024-1086", true, DecisionFixed, "5.14.0-427.13.1.el9_4"}, // main kernel package NVR
		{"CVE-2023-1111", true, DecisionFixed, "5.14.0-284.11.1.el9_2"}, // earliest-issued fix wins
		{"CVE-2024-5555", true, DecisionFixed, "2:5.14.0-300.el9"},      // non-zero epoch prefix
		{"CVE-2024-9999", false, "", ""},                                // kernel-rt only: ignored
		{"CVE-2024-8888", false, "", ""},                                // kpatch: ignored
		{"CVE-2024-2222", false, "", ""},                                // httpd: not a kernel erratum
		{"CVE-2024-3333", false, "", ""},                                // no issued_date: skipped
	}

	for _, tt := range tests {
		adv, ok := got[tt.cve]
		if ok != tt.present {
			t.Errorf("%s: present=%v, want %v (got %+v)", tt.cve, ok, tt.present, adv)
			continue
		}
		if !tt.present {
			continue
		}
		if adv.Release != "9" {
			t.Errorf("%s: release=%q, want %q", tt.cve, adv.Release, "9")
		}
		if adv.Decision != tt.decision {
			t.Errorf("%s: decision=%q, want %q", tt.cve, adv.Decision, tt.decision)
		}
		if adv.FixedVersion != tt.fixed {
			t.Errorf("%s: fixed=%q, want %q", tt.cve, adv.FixedVersion, tt.fixed)
		}
	}

	if len(got) != 3 {
		t.Errorf("parse produced %d CVEs, want 3", len(got))
	}
}

func TestCentOSReleaseKey(t *testing.T) {
	c := NewCentOS()
	cases := map[string]string{
		"9.4":           "9",
		"9":             "9",
		"8.10":          "8",
		"AlmaLinux 9.4": "9",
		"":              "",
		"stream":        "",
	}
	for in, want := range cases {
		if got := c.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestCentOSCompareVersions(t *testing.T) {
	c := NewCentOS()
	// A host at or above the fixed NVR is patched; below it is vulnerable.
	if c.CompareVersions("5.14.0-284.11.1.el9_2", "5.14.0-284.30.1.el9_2") >= 0 {
		t.Error("284.11.1 should sort below 284.30.1")
	}
	if c.CompareVersions("5.14.0-427.13.1.el9_4", "5.14.0-427.13.1.el9_4") != 0 {
		t.Error("equal NVRs should compare equal")
	}
	if c.CompareVersions("5.14.0-500.el9", "5.14.0-427.13.1.el9_4") <= 0 {
		t.Error("500.el9 should sort above 427.13.1.el9_4")
	}
	// Epoch dominates version-release.
	if c.CompareVersions("2:5.14.0-300.el9", "5.14.0-999.el9") <= 0 {
		t.Error("epoch 2 should outrank epoch 0 regardless of version-release")
	}
}

func TestCentOSDistro(t *testing.T) {
	if got := NewCentOS().Distro(); got != "centos" {
		t.Errorf("Distro()=%q, want %q", got, "centos")
	}
}

func TestCentOSParseEmpty(t *testing.T) {
	c := NewCentOS()
	got := c.parse(&almaErrataFile{}, "9")
	if len(got) != 0 {
		t.Errorf("empty document should yield no advisories, got %d", len(got))
	}
}
