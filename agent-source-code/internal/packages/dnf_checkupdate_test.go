package packages

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestLooksLikeRPMVersion(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"5.0.1-1.el10", true},
		{"1:3.2.4-4.fc42", true},
		{"9.9p1-11.fc42", true},
		{"20250101-3.el9", true},
		{"*1.2.3-4.el9", true}, // yum marks obsoleting packages with a leading *

		{"Subscription", false}, // #415: the banner line's second column
		{"Management", false},
		{"repositories.", false},
		{"available", false},
		{"5.0.1", false}, // no release component
		{"", false},
	}

	for _, tt := range tests {
		if got := looksLikeRPMVersion(tt.in); got != tt.want {
			t.Errorf("looksLikeRPMVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// #415: on RHEL hosts registered with subscription-manager, check-update
// prints a banner line that survives the field-count guard and was ingested
// as a package called "Updating" at version "Subscription".
func TestParseUpgradablePackages_IgnoresSubscriptionManagerBanner(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	m := &DNFManager{logger: log}

	output := `Updating Subscription Management repositories.
Last metadata expiration check: 0:42:20 ago on Mon 05 Jan 2026 11:52:38 AM CET.

docker-compose-plugin.x86_64      5.0.1-1.el10     docker-ce-stable
openssl-libs.x86_64               1:3.2.4-4.fc42   updates
`

	// The banner appears in "list installed" output too, and that is what made
	// the phantom survive: the installed parser recorded Updating=Subscription,
	// which then satisfied the non-empty currentVersion guard below.
	installedOutput := `Updating Subscription Management repositories.
Installed Packages
docker-compose-plugin.x86_64      5.0.0-1.el10     @docker-ce-stable
openssl-libs.x86_64               1:3.2.3-1.fc42   @updates
`
	installed := m.parseInstalledPackages(installedOutput)
	if _, ok := installed["Updating"]; ok {
		t.Error(`parseInstalledPackages ingested the banner as a package named "Updating"`)
	}

	got := m.parseUpgradablePackages(output, "dnf", installed, map[string]bool{})

	names := make([]string, 0, len(got))
	for _, p := range got {
		names = append(names, p.Name)
	}

	if len(got) != 2 {
		t.Fatalf("parsed %d packages %v, want 2 (banner must not become a package)", len(got), names)
	}
	for _, n := range names {
		if n == "Updating" {
			t.Errorf("banner line was ingested as a package: %v", names)
		}
	}
}
