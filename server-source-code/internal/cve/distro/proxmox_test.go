package distro

import "testing"

// ubSet builds a hand-made Ubuntu AdvisorySet keyed by codename for remap tests.
func ubSet(by map[string]Advisory) *AdvisorySet {
	return &AdvisorySet{
		CVEID: "CVE-TEST", Distro: "ubuntu", Known: true, ByRelease: by,
	}
}

func TestProxmoxStaticMethods(t *testing.T) {
	p := NewProxmox(nil)
	if p.Distro() != "proxmox" {
		t.Fatalf("Distro() = %q, want proxmox", p.Distro())
	}
	if p.CompareVersions("6.8.0-40.40", "6.8.0-31.31") <= 0 {
		t.Fatalf("CompareVersions should treat higher build as greater")
	}
	cases := map[string]string{
		"8.2":   "8",
		"8":     "8",
		"7":     "7",
		"pve 9": "9",
		"":      "",
		"abc":   "",
	}
	for in, want := range cases {
		if got := p.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemapFixedSingleBase(t *testing.T) {
	// VE 7 derives from focal only.
	set := ubSet(map[string]Advisory{
		"focal": {Release: "focal", Decision: DecisionFixed, FixedVersion: "5.4.0-90.101"},
		"noble": {Release: "noble", Decision: DecisionAffected},
	})
	got := remapFromUbuntu(set, "7")
	adv, ok := got["7"]
	if !ok {
		t.Fatalf("expected key 7, got %v", got)
	}
	if adv.Decision != DecisionFixed || adv.FixedVersion != "5.4.0-90.101" {
		t.Fatalf("got %+v, want fixed 5.4.0-90.101 from focal", adv)
	}
	if adv.Release != "7" {
		t.Fatalf("Release = %q, want 7", adv.Release)
	}
	// noble should not have leaked into VE 7.
	if len(got) != 1 {
		t.Fatalf("expected single entry, got %v", got)
	}
}

func TestRemapAffectedDominatesFixed(t *testing.T) {
	// VE 8 merges noble + jammy; affected (no fix) must win over a fixed base.
	set := ubSet(map[string]Advisory{
		"noble": {Release: "noble", Decision: DecisionFixed, FixedVersion: "6.8.0-40.40"},
		"jammy": {Release: "jammy", Decision: DecisionAffected},
	})
	adv := remapFromUbuntu(set, "8")["8"]
	if adv.Decision != DecisionAffected {
		t.Fatalf("got %+v, want affected (unfixed base dominates)", adv)
	}
	if adv.FixedVersion != "" {
		t.Fatalf("affected verdict must not carry a fixed version, got %q", adv.FixedVersion)
	}
}

func TestRemapLowestFixedWins(t *testing.T) {
	// Two fixed bases: keep the lower fixed version (dpkg semantics).
	set := ubSet(map[string]Advisory{
		"noble": {Release: "noble", Decision: DecisionFixed, FixedVersion: "6.8.0-40.40"},
		"jammy": {Release: "jammy", Decision: DecisionFixed, FixedVersion: "5.15.0-100.110"},
	})
	adv := remapFromUbuntu(set, "8")["8"]
	if adv.Decision != DecisionFixed || adv.FixedVersion != "5.15.0-100.110" {
		t.Fatalf("got %+v, want fixed 5.15.0-100.110 (lowest)", adv)
	}
}

func TestRemapNotAffected(t *testing.T) {
	set := ubSet(map[string]Advisory{
		"focal": {Release: "focal", Decision: DecisionNotAffected},
	})
	adv := remapFromUbuntu(set, "7")["7"]
	if adv.Decision != DecisionNotAffected {
		t.Fatalf("got %+v, want not_affected", adv)
	}
}

func TestRemapNoBaseData(t *testing.T) {
	// VE 9 derives from noble; if noble is absent, Proxmox has no data.
	set := ubSet(map[string]Advisory{
		"focal": {Release: "focal", Decision: DecisionFixed, FixedVersion: "5.4.0-1.2"},
	})
	if got := remapFromUbuntu(set, "9"); len(got) != 0 {
		t.Fatalf("expected empty remap for VE 9 without noble, got %v", got)
	}
}

func TestRemapUnknownMajor(t *testing.T) {
	set := ubSet(map[string]Advisory{"noble": {Release: "noble", Decision: DecisionFixed, FixedVersion: "6.8.0-1.1"}})
	if got := remapFromUbuntu(set, "99"); len(got) != 0 {
		t.Fatalf("expected empty remap for unknown VE major, got %v", got)
	}
	if got := remapFromUbuntu(nil, "8"); len(got) != 0 {
		t.Fatalf("expected empty remap for nil set, got %v", got)
	}
}
