package distro

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeSource returns a fixed AdvisorySet for evaluator tests.
type fakeSource struct {
	set *AdvisorySet
	cmp func(a, b string) int
}

func (f *fakeSource) Distro() string                  { return "ubuntu" }
func (f *fakeSource) CompareVersions(a, b string) int { return f.cmp(a, b) }
func (f *fakeSource) ReleaseKey(v string) string      { return "noble" }
func (f *fakeSource) Advisories(context.Context, string) (*AdvisorySet, error) {
	return f.set, nil
}

func TestEvaluatorDecisions(t *testing.T) {
	mk := func(dec Decision, fixed string) *fakeSource {
		return &fakeSource{
			cmp: CompareDpkg,
			set: &AdvisorySet{
				CVEID: "CVE-X", Distro: "ubuntu", Known: true,
				ByRelease: map[string]Advisory{"noble": {Release: "noble", Decision: dec, FixedVersion: fixed}},
			},
		}
	}
	host := func(pkgVer string) Host {
		return Host{OSType: "ubuntu", OSVersion: "24.04", KernelPkgVersion: pkgVer}
	}

	cases := []struct {
		name string
		src  *fakeSource
		host Host
		want Status
	}{
		{"not-affected", mk(DecisionNotAffected, ""), host("6.8.0-40.40"), StatusNotAffected},
		{"affected-no-fix", mk(DecisionAffected, ""), host("6.8.0-40.40"), StatusVulnerable},
		{"fixed-but-old", mk(DecisionFixed, "6.8.0-51.52"), host("6.8.0-40.40"), StatusVulnerable},
		{"fixed-and-current", mk(DecisionFixed, "6.8.0-51.52"), host("6.8.0-51.52"), StatusPatched},
		{"fixed-and-newer", mk(DecisionFixed, "6.8.0-51.52"), host("6.8.0-52.53"), StatusPatched},
		{"fixed-no-pkgversion", mk(DecisionFixed, "6.8.0-51.52"), host(""), StatusUnknown},
	}
	for _, c := range cases {
		e := NewEvaluator(c.src)
		got := e.Evaluate(context.Background(), "CVE-X", c.host)
		if got.Status != c.want {
			t.Errorf("%s: got %s want %s", c.name, got.Status, c.want)
		}
	}
}

func TestEvaluatorUnknownDistro(t *testing.T) {
	e := NewEvaluator(NewUbuntu())
	got := e.Evaluate(context.Background(), "CVE-X", Host{OSType: "arch"})
	if got.Status != StatusUnknown {
		t.Errorf("unknown distro: got %s want unknown", got.Status)
	}
}

const ubuntuFixture = `{
  "packages": [
    {"name": "linux", "statuses": [
      {"status": "released", "release_codename": "jammy", "description": "5.15.0-89.99"},
      {"status": "needed", "release_codename": "noble", "description": ""},
      {"status": "not-affected", "release_codename": "focal", "description": ""}
    ]},
    {"name": "linux-firmware", "statuses": [
      {"status": "not-affected", "release_codename": "jammy", "description": ""}
    ]}
  ]
}`

func TestUbuntuParse(t *testing.T) {
	var data ubuntuCVE
	if err := json.Unmarshal([]byte(ubuntuFixture), &data); err != nil {
		t.Fatal(err)
	}
	set := &AdvisorySet{CVEID: "CVE-X", Distro: "ubuntu", Known: true, ByRelease: map[string]Advisory{}}
	NewUbuntu().parse(&data, set)

	if adv := set.ByRelease["jammy"]; adv.Decision != DecisionFixed || adv.FixedVersion != "5.15.0-89.99" {
		t.Errorf("jammy: %+v", adv)
	}
	if adv := set.ByRelease["noble"]; adv.Decision != DecisionAffected {
		t.Errorf("noble: %+v", adv)
	}
	if adv := set.ByRelease["focal"]; adv.Decision != DecisionNotAffected {
		t.Errorf("focal: %+v", adv)
	}
}

func TestUbuntuReleaseKey(t *testing.T) {
	u := NewUbuntu()
	for in, want := range map[string]string{
		"24.04": "noble", "Ubuntu 22.04.3 LTS": "jammy", "jammy": "jammy", "9.9": "",
	} {
		if got := u.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDebianAdvisoriesFromCache(t *testing.T) {
	d := NewDebian()
	// Inject a cached linux map to avoid the network.
	d.linux = map[string]debianEntry{
		"CVE-A": {Releases: map[string]struct {
			Status       string `json:"status"`
			FixedVersion string `json:"fixed_version"`
		}{
			"bookworm": {Status: "resolved", FixedVersion: "6.1.55-1"},
			"bullseye": {Status: "open", FixedVersion: ""},
			"buster":   {Status: "resolved", FixedVersion: "0"},
		}},
	}
	d.expires = time.Now().Add(time.Hour)

	set, err := d.Advisories(context.Background(), "CVE-A")
	if err != nil {
		t.Fatal(err)
	}
	if !set.Known {
		t.Fatal("expected known")
	}
	if adv := set.ByRelease["bookworm"]; adv.Decision != DecisionFixed || adv.FixedVersion != "6.1.55-1" {
		t.Errorf("bookworm: %+v", adv)
	}
	if adv := set.ByRelease["bullseye"]; adv.Decision != DecisionAffected {
		t.Errorf("bullseye: %+v", adv)
	}
	if adv := set.ByRelease["buster"]; adv.Decision != DecisionNotAffected {
		t.Errorf("buster: %+v", adv)
	}

	// Unknown CVE => Known=false, no error.
	set2, err := d.Advisories(context.Background(), "CVE-MISSING")
	if err != nil || set2.Known {
		t.Errorf("missing CVE: known=%v err=%v", set2.Known, err)
	}

	// End-to-end host evaluation via the evaluator.
	e := NewEvaluator(d)
	got := e.Evaluate(context.Background(), "CVE-A", Host{OSType: "debian", OSVersion: "12", KernelPkgVersion: "6.1.38-1"})
	if got.Status != StatusVulnerable {
		t.Errorf("debian bookworm old kernel: got %s want vulnerable", got.Status)
	}
	got = e.Evaluate(context.Background(), "CVE-A", Host{OSType: "debian", OSVersion: "12", KernelPkgVersion: "6.1.55-1"})
	if got.Status != StatusPatched {
		t.Errorf("debian bookworm patched kernel: got %s want patched", got.Status)
	}
}
