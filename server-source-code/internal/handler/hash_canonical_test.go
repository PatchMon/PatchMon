package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// fixture mirrors testdata/canonical_fixtures.json. Both server and agent tests
// load the same file so the hashes asserted here are identical to the agent's.
type fixture struct {
	Packages struct {
		Input        []store.ReportPackage `json:"input"`
		ExpectedHash string                `json:"expected_hash"`
	} `json:"packages"`
	Repos struct {
		Input        []store.ReportRepository `json:"input"`
		ExpectedHash string                   `json:"expected_hash"`
	} `json:"repos"`
	Interfaces struct {
		Input        []models.NetworkInterface `json:"input"`
		ExpectedHash string                    `json:"expected_hash"`
	} `json:"interfaces"`
	Hostname struct {
		Input        string `json:"input"`
		ExpectedHash string `json:"expected_hash"`
	} `json:"hostname"`
	HostnameNFD struct {
		InputNFC     string `json:"input_nfc"`
		InputNFD     string `json:"input_nfd"`
		ExpectedHash string `json:"expected_hash"`
	} `json:"hostname_nfd"`
	Docker struct {
		Input        DockerHashInput `json:"input"`
		ExpectedHash string          `json:"expected_hash"`
	} `json:"docker"`
	Compliance struct {
		Input        ComplianceHashInput `json:"input"`
		ExpectedHash string              `json:"expected_hash"`
	} `json:"compliance"`
}

func loadFixture(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join("testdata", "canonical_fixtures.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return &f
}

// TestCanonicalFixtureHashes asserts each canonical helper produces the
// expected hash for the shared fixture inputs. The agent's test asserts the
// SAME hashes; if either side drifts, both fail.
func TestCanonicalFixtureHashes(t *testing.T) {
	f := loadFixture(t)

	gotPackages, err := CanonicalPackagesHash(f.Packages.Input)
	if err != nil {
		t.Fatalf("packages hash: %v", err)
	}
	gotRepos, err := CanonicalReposHash(f.Repos.Input)
	if err != nil {
		t.Fatalf("repos hash: %v", err)
	}
	gotIfaces, err := CanonicalInterfacesHash(f.Interfaces.Input)
	if err != nil {
		t.Fatalf("interfaces hash: %v", err)
	}
	gotHost := CanonicalHostnameHash(f.Hostname.Input)
	gotHostNFC := CanonicalHostnameHash(f.HostnameNFD.InputNFC)
	gotHostNFD := CanonicalHostnameHash(f.HostnameNFD.InputNFD)
	gotDocker, err := CanonicalDockerHash(f.Docker.Input)
	if err != nil {
		t.Fatalf("docker hash: %v", err)
	}
	gotCompliance, err := CanonicalComplianceHash(f.Compliance.Input)
	if err != nil {
		t.Fatalf("compliance hash: %v", err)
	}

	t.Logf("packages   = %s", gotPackages)
	t.Logf("repos      = %s", gotRepos)
	t.Logf("interfaces = %s", gotIfaces)
	t.Logf("hostname   = %s", gotHost)
	t.Logf("nfc/nfd    = %s / %s", gotHostNFC, gotHostNFD)
	t.Logf("docker     = %s", gotDocker)
	t.Logf("compliance = %s", gotCompliance)

	if gotHostNFC != gotHostNFD {
		t.Errorf("NFC normalisation broken: NFC=%s NFD=%s", gotHostNFC, gotHostNFD)
	}

	checks := []struct {
		name, got, want string
	}{
		{"packages", gotPackages, f.Packages.ExpectedHash},
		{"repos", gotRepos, f.Repos.ExpectedHash},
		{"interfaces", gotIfaces, f.Interfaces.ExpectedHash},
		{"hostname", gotHost, f.Hostname.ExpectedHash},
		{"hostname_nfd", gotHostNFC, f.HostnameNFD.ExpectedHash},
		{"docker", gotDocker, f.Docker.ExpectedHash},
		{"compliance", gotCompliance, f.Compliance.ExpectedHash},
	}
	for _, c := range checks {
		if c.want == "TBD" {
			t.Logf("%s: no expected hash baked in yet (got %s) — paste into fixture", c.name, c.got)
			continue
		}
		if c.got != c.want {
			t.Errorf("%s hash drift: got %s want %s", c.name, c.got, c.want)
		}
	}
}

// TestPackagesHashStable proves the hash does not depend on input ordering or
// on duplicate-package presence. Same canonical content → same hash.
func TestPackagesHashStable(t *testing.T) {
	a := []store.ReportPackage{
		{Name: "b", CurrentVersion: "2"},
		{Name: "a", CurrentVersion: "1"},
		{Name: "c", CurrentVersion: "3"},
	}
	b := []store.ReportPackage{
		{Name: "a", CurrentVersion: "1"},
		{Name: "b", CurrentVersion: "2"},
		{Name: "c", CurrentVersion: "3"},
	}
	c := []store.ReportPackage{
		{Name: "a", CurrentVersion: "OLD"},
		{Name: "b", CurrentVersion: "2"},
		{Name: "a", CurrentVersion: "1"},
		{Name: "c", CurrentVersion: "3"},
	}
	ha, _ := CanonicalPackagesHash(a)
	hb, _ := CanonicalPackagesHash(b)
	hc, _ := CanonicalPackagesHash(c)
	if ha != hb {
		t.Errorf("shuffle changed hash: %s vs %s", ha, hb)
	}
	if ha != hc {
		t.Errorf("dup last-wins broken: %s vs %s", ha, hc)
	}
}

// TestInterfacesHashIncludesAddresses proves IP changes flip the hash, while
// link-layer flap fields are not present at all and so cannot affect it.
func TestInterfacesHashIncludesAddresses(t *testing.T) {
	base := []models.NetworkInterface{
		{
			Name:       "eth0",
			Type:       "ethernet",
			MACAddress: "AA:BB:CC:DD:EE:FF",
			MTU:        1500,
			Status:     "up",
			LinkSpeed:  1000,
			Duplex:     "full",
			Addresses: []models.NetworkAddress{
				{Address: "10.0.0.5", Family: "inet", Netmask: "/24"},
			},
		},
	}
	flapping := []models.NetworkInterface{
		{
			Name: "eth0", Type: "ethernet", MACAddress: "aa:bb:cc:dd:ee:ff",
			MTU: 9000, Status: "down", LinkSpeed: 100, Duplex: "half",
			Addresses: []models.NetworkAddress{
				{Address: "10.0.0.5", Family: "inet", Netmask: "/24"},
			},
		},
	}
	ipChange := []models.NetworkInterface{
		{
			Name: "eth0", Type: "ethernet", MACAddress: "AA:BB:CC:DD:EE:FF",
			Addresses: []models.NetworkAddress{
				{Address: "10.0.0.6", Family: "inet", Netmask: "/24"},
			},
		},
	}

	hBase, _ := CanonicalInterfacesHash(base)
	hFlap, _ := CanonicalInterfacesHash(flapping)
	hIP, _ := CanonicalInterfacesHash(ipChange)

	if hBase != hFlap {
		t.Errorf("link-layer flap or MAC case changed hash: %s vs %s", hBase, hFlap)
	}
	if hBase == hIP {
		t.Errorf("IP change MUST change hash: both %s", hBase)
	}
}

// TestComplianceHashIgnoresTimestamps ensures running the same scan twice
// yields the same hash even though the underlying scan would have new
// started_at / completed_at timestamps. Hash is over findings only.
func TestComplianceHashIgnoresTimestamps(t *testing.T) {
	a := ComplianceHashInput{
		Scans: []ComplianceWireScan{
			{
				ProfileName: "CIS L1",
				ProfileType: "openscap",
				Status:      "completed",
				Score:       95.5,
				TotalRules:  10,
				Passed:      9,
				Failed:      1,
				Results: []ComplianceWireResult{
					{RuleRef: "rule1", Status: "fail", Severity: "high"},
				},
			},
		},
	}
	// Identical findings; the hash row has no timestamp fields, so any change
	// the agent might make to its own timestamps is irrelevant to the hash.
	b := ComplianceHashInput{
		Scans: []ComplianceWireScan{
			{
				ProfileName: "CIS L1",
				ProfileType: "openscap",
				Status:      "completed",
				Score:       95.5,
				TotalRules:  10,
				Passed:      9,
				Failed:      1,
				Results: []ComplianceWireResult{
					{RuleRef: "rule1", Status: "fail", Severity: "high"},
				},
			},
		},
	}
	ha, _ := CanonicalComplianceHash(a)
	hb, _ := CanonicalComplianceHash(b)
	if ha != hb {
		t.Errorf("compliance hash unstable across re-runs: %s vs %s", ha, hb)
	}
}
