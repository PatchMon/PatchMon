package distro

import (
	"encoding/json"
	"testing"
)

// rhelVEXFixture is a realistic (trimmed) Red Hat CSAF VEX document. It exercises
// the parse paths we care about:
//   - RHEL 9: mainline `kernel` fixed in two streams (el9_2, el9_4) plus a
//     kernel-rt fix, and a kernel-debug listed as known_affected. Because
//     "fixed" outranks "known_affected", major 9 is DecisionFixed and the fixed
//     version is the LOWEST mainline kernel NVR (el9_2), arch suffix stripped.
//   - RHEL 8: known_not_affected -> DecisionNotAffected.
//   - RHEL 7: known_affected -> DecisionAffected, no fixed version.
//
// Major is resolved via the product_tree CPEs (product refs also match the
// fetch_rhel_vex.py regexes, so both paths agree).
const rhelVEXFixture = `{
  "document": {
    "tracking": {
      "id": "CVE-2024-1086",
      "initial_release_date": "2024-01-31T00:00:00Z"
    }
  },
  "product_tree": {
    "branches": [
      {
        "category": "vendor",
        "name": "Red Hat",
        "branches": [
          {
            "category": "product_name",
            "name": "Red Hat Enterprise Linux 9",
            "product": {
              "product_id": "red_hat_enterprise_linux_9",
              "product_identification_helper": { "cpe": "cpe:/o:redhat:enterprise_linux:9" }
            }
          },
          {
            "category": "product_name",
            "name": "Red Hat Enterprise Linux 8",
            "product": {
              "product_id": "red_hat_enterprise_linux_8",
              "product_identification_helper": { "cpe": "cpe:/o:redhat:enterprise_linux:8" }
            }
          },
          {
            "category": "product_name",
            "name": "Red Hat Enterprise Linux 7",
            "product": {
              "product_id": "red_hat_enterprise_linux_7",
              "product_identification_helper": { "cpe": "cpe:/o:redhat:enterprise_linux:7" }
            }
          }
        ]
      }
    ]
  },
  "vulnerabilities": [
    {
      "cve": "CVE-2024-1086",
      "product_status": {
        "fixed": [
          "red_hat_enterprise_linux_9:kernel-0:5.14.0-427.13.1.el9_4.x86_64",
          "red_hat_enterprise_linux_9:kernel-0:5.14.0-284.30.1.el9_2.x86_64",
          "red_hat_enterprise_linux_9:kernel-rt-0:5.14.0-427.13.1.rt21.el9_4.x86_64"
        ],
        "known_affected": [
          "red_hat_enterprise_linux_9:kernel-debug-0:5.14.0-70.el9_0.x86_64",
          "red_hat_enterprise_linux_7:kernel-0:3.10.0-1160.el7.x86_64"
        ],
        "known_not_affected": [
          "red_hat_enterprise_linux_8:kernel-0:4.18.0-513.el8.x86_64"
        ]
      }
    }
  ]
}`

// rhelRTOnlyFixture has a major whose only fixed kernel is kernel-rt, verifying
// the kernel-rt fallback for FixedVersion.
const rhelRTOnlyFixture = `{
  "document": { "tracking": { "id": "CVE-2023-0001" } },
  "product_tree": { "branches": [
    { "category": "product_name", "name": "Red Hat Enterprise Linux 9",
      "product": { "product_id": "red_hat_enterprise_linux_9",
        "product_identification_helper": { "cpe": "cpe:/o:redhat:enterprise_linux:9" } } }
  ] },
  "vulnerabilities": [
    { "cve": "CVE-2023-0001", "product_status": {
        "fixed": [ "red_hat_enterprise_linux_9:kernel-rt-0:5.14.0-100.rt21.el9_0.aarch64" ]
    } }
  ]
}`

func parseFixture(t *testing.T, fixture string) *AdvisorySet {
	t.Helper()
	var data csafVEX
	if err := json.Unmarshal([]byte(fixture), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	r := NewRHEL()
	set := &AdvisorySet{CVEID: "CVE-TEST", Distro: "rhel", ByRelease: map[string]Advisory{}}
	r.parse(&data, set)
	return set
}

func TestRHELParse(t *testing.T) {
	set := parseFixture(t, rhelVEXFixture)

	cases := []struct {
		major    string
		decision Decision
		fixed    string
	}{
		{"9", DecisionFixed, "5.14.0-284.30.1.el9_2"}, // lowest mainline NVR, arch stripped
		{"8", DecisionNotAffected, ""},
		{"7", DecisionAffected, ""},
	}
	for _, c := range cases {
		adv, ok := set.ByRelease[c.major]
		if !ok {
			t.Errorf("major %s: missing from ByRelease", c.major)
			continue
		}
		if adv.Decision != c.decision {
			t.Errorf("major %s: decision = %q, want %q", c.major, adv.Decision, c.decision)
		}
		if adv.FixedVersion != c.fixed {
			t.Errorf("major %s: fixed = %q, want %q", c.major, adv.FixedVersion, c.fixed)
		}
		if adv.Release != c.major {
			t.Errorf("major %s: release = %q, want %q", c.major, adv.Release, c.major)
		}
	}
	if len(set.ByRelease) != len(cases) {
		t.Errorf("ByRelease has %d majors, want %d", len(set.ByRelease), len(cases))
	}
}

func TestRHELParseRTFallback(t *testing.T) {
	set := parseFixture(t, rhelRTOnlyFixture)
	adv, ok := set.ByRelease["9"]
	if !ok {
		t.Fatal("major 9 missing")
	}
	if adv.Decision != DecisionFixed {
		t.Errorf("decision = %q, want %q", adv.Decision, DecisionFixed)
	}
	if adv.FixedVersion != "5.14.0-100.rt21.el9_0" {
		t.Errorf("fixed = %q, want %q", adv.FixedVersion, "5.14.0-100.rt21.el9_0")
	}
}

func TestRHELReleaseKey(t *testing.T) {
	r := NewRHEL()
	cases := map[string]string{
		"9.4":                          "9",
		"8":                            "8",
		"9":                            "9",
		"Red Hat Enterprise Linux 9.4": "9",
		"":                             "",
		"unknown":                      "",
	}
	for in, want := range cases {
		if got := r.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRHELParseKernelNVR(t *testing.T) {
	cases := []struct {
		component   string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{"kernel-0:5.14.0-427.13.1.el9_4.x86_64", "kernel", "5.14.0-427.13.1.el9_4", true},
		{"kernel-rt-0:5.14.0-427.13.1.rt21.el9_4.x86_64", "kernel-rt", "5.14.0-427.13.1.rt21.el9_4", true},
		{"kernel-debug-0:5.14.0-70.el9_0.noarch", "kernel-debug", "5.14.0-70.el9_0", true},
		{"kernel-0:4.18.0-513.el8.src", "kernel", "4.18.0-513.el8", true},
		{"no-colon-here", "", "", false},
	}
	for _, c := range cases {
		name, ver, ok := rhelKernelNVR(c.component)
		if name != c.wantName || ver != c.wantVersion || ok != c.wantOK {
			t.Errorf("rhelKernelNVR(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.component, name, ver, ok, c.wantName, c.wantVersion, c.wantOK)
		}
	}
}

func TestRHELMajorFromRef(t *testing.T) {
	cases := map[string]string{
		"red_hat_enterprise_linux_10": "10",
		"BaseOS-9.4.0.GA":             "9",
		"AppStream-9.4.0.GA":          "9",
		"8Base-RHOSE-4.12":            "8",
		"red_hat_products":            "",
	}
	for ref, want := range cases {
		if got := rhelMajor(ref, nil); got != want {
			t.Errorf("rhelMajor(%q) = %q, want %q", ref, got, want)
		}
	}
}
