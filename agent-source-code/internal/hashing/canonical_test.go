package hashing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"patchmon-agent/pkg/models"
)

// fixture mirrors testdata/canonical_fixtures.json. Both server and agent tests
// load the same file. Hashes asserted here MUST equal the server's; if either
// side drifts both fail. The fixture file should be byte-identical to the
// server's copy at server-source-code/internal/handler/testdata/.
type fixture struct {
	Packages struct {
		Input        []models.Package `json:"input"`
		ExpectedHash string           `json:"expected_hash"`
	} `json:"packages"`
	Repos struct {
		Input        []models.Repository `json:"input"`
		ExpectedHash string              `json:"expected_hash"`
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
		Input        models.DockerData `json:"input"`
		ExpectedHash string            `json:"expected_hash"`
	} `json:"docker"`
	Compliance struct {
		Input        models.ComplianceData `json:"input"`
		ExpectedHash string                `json:"expected_hash"`
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

// TestCanonicalFixtureHashes asserts the agent's hash output matches the
// server's for the same canonical inputs. Drift here means the canonical
// spec has been broken on one side — fix immediately, do not bump the
// fixture to paper over.
func TestCanonicalFixtureHashes(t *testing.T) {
	f := loadFixture(t)

	gotPackages, err := PackagesHash(f.Packages.Input)
	if err != nil {
		t.Fatalf("packages hash: %v", err)
	}
	gotRepos, err := ReposHash(f.Repos.Input)
	if err != nil {
		t.Fatalf("repos hash: %v", err)
	}
	gotIfaces, err := InterfacesHash(f.Interfaces.Input)
	if err != nil {
		t.Fatalf("interfaces hash: %v", err)
	}
	gotHost := HostnameHash(f.Hostname.Input)
	gotHostNFC := HostnameHash(f.HostnameNFD.InputNFC)
	gotHostNFD := HostnameHash(f.HostnameNFD.InputNFD)
	gotDocker, err := DockerHash(&f.Docker.Input)
	if err != nil {
		t.Fatalf("docker hash: %v", err)
	}
	gotCompliance, err := ComplianceHash(&f.Compliance.Input)
	if err != nil {
		t.Fatalf("compliance hash: %v", err)
	}

	if gotHostNFC != gotHostNFD {
		t.Errorf("NFC normalisation broken: NFC=%s NFD=%s", gotHostNFC, gotHostNFD)
	}

	checks := []struct{ name, got, want string }{
		{"packages", gotPackages, f.Packages.ExpectedHash},
		{"repos", gotRepos, f.Repos.ExpectedHash},
		{"interfaces", gotIfaces, f.Interfaces.ExpectedHash},
		{"hostname", gotHost, f.Hostname.ExpectedHash},
		{"hostname_nfd", gotHostNFC, f.HostnameNFD.ExpectedHash},
		{"docker", gotDocker, f.Docker.ExpectedHash},
		{"compliance", gotCompliance, f.Compliance.ExpectedHash},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s hash drift vs server fixture: agent=%s server=%s", c.name, c.got, c.want)
		}
	}
}

// TestPackagesHashStable mirrors the server test — proves shuffle and dup
// last-wins do not change the hash.
func TestPackagesHashStable(t *testing.T) {
	a := []models.Package{
		{Name: "b", CurrentVersion: "2"},
		{Name: "a", CurrentVersion: "1"},
		{Name: "c", CurrentVersion: "3"},
	}
	b := []models.Package{
		{Name: "a", CurrentVersion: "1"},
		{Name: "b", CurrentVersion: "2"},
		{Name: "c", CurrentVersion: "3"},
	}
	c := []models.Package{
		{Name: "a", CurrentVersion: "OLD"},
		{Name: "b", CurrentVersion: "2"},
		{Name: "a", CurrentVersion: "1"},
		{Name: "c", CurrentVersion: "3"},
	}
	ha, _ := PackagesHash(a)
	hb, _ := PackagesHash(b)
	hc, _ := PackagesHash(c)
	if ha != hb {
		t.Errorf("shuffle changed hash: %s vs %s", ha, hb)
	}
	if ha != hc {
		t.Errorf("dup last-wins broken: %s vs %s", ha, hc)
	}
}

// TestInterfacesHashIncludesAddresses asserts addresses ARE part of the hash
// (per user decision), and link-layer flap fields are NOT.
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

	hBase, _ := InterfacesHash(base)
	hFlap, _ := InterfacesHash(flapping)
	hIP, _ := InterfacesHash(ipChange)

	if hBase != hFlap {
		t.Errorf("link-layer flap changed hash: %s vs %s", hBase, hFlap)
	}
	if hBase == hIP {
		t.Errorf("IP change did not change hash: both %s", hBase)
	}
}
