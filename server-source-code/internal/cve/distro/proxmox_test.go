package distro

import "testing"

func TestProxmoxStaticMethods(t *testing.T) {
	p := NewProxmox(nil)
	if p.Distro() != "proxmox" {
		t.Fatalf("Distro() = %q, want proxmox", p.Distro())
	}
	if p.CompareVersions("6.8.0-40.40", "6.8.0-31.31") <= 0 {
		t.Fatalf("CompareVersions should treat higher build as greater")
	}
	cases := map[string]string{"8.2": "8", "8": "8", "7": "7", "pve 9": "9", "": "", "abc": ""}
	for in, want := range cases {
		if got := p.ReleaseKey(in); got != want {
			t.Errorf("ReleaseKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// ubSet builds a hand-made Ubuntu-style AdvisorySet keyed by codename/series.
func ubSet(by map[string]Advisory) *AdvisorySet {
	return &AdvisorySet{CVEID: "CVE-TEST", Distro: "proxmox", Known: true, ByRelease: by}
}

func TestProxmoxSelectBySeries(t *testing.T) {
	p := NewProxmox(nil)
	// VE 8 derives from noble/jammy. A host on the 6.8 kernel matches noble/6.8.
	set := ubSet(map[string]Advisory{
		"noble/6.8":  {Release: "noble/6.8", Decision: DecisionFixed, FixedVersion: "6.8.0-45.45"},
		"jammy/5.15": {Release: "jammy/5.15", Decision: DecisionAffected},
	})
	adv, ok := p.SelectAdvisory(set, Host{OSType: "proxmox", OSVersion: "8.2", KernelVersion: "6.8.12-1-pve", KernelPkgVersion: "6.8.12-1"})
	if !ok || adv.Decision != DecisionFixed || adv.FixedVersion != "6.8.0-45.45" {
		t.Fatalf("VE8 6.8 host: %+v ok=%v", adv, ok)
	}
	// No advisory for the host's series -> no match.
	if _, ok := p.SelectAdvisory(set, Host{OSType: "proxmox", OSVersion: "8", KernelVersion: "6.11.0-1-pve"}); ok {
		t.Fatalf("VE8 6.11 host should not match")
	}
	// Unknown VE major -> no match.
	if _, ok := p.SelectAdvisory(set, Host{OSType: "proxmox", OSVersion: "99", KernelVersion: "6.8.0-1"}); ok {
		t.Fatalf("unknown VE major should not match")
	}
}
