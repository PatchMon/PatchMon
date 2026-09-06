package packages

import (
	"io"
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// #1081: the severity column of `dnf updateinfo list security` is
// locale-translated (e.g. French renders "Moderate/Sec." as
// "Sécurité/niveau modéré."). The translated severity contains spaces, so the
// fixed-position field reads shift: fields[2] picks up a severity word like
// "modéré." instead of the package NVRA, and no package is ever flagged as a
// security update.
//
// The agent already forces LANG=C for `dnf list installed` and repoquery;
// the fix extends the same guarantee to the remaining dnf/yum invocations
// whose output is parsed positionally: updateinfo (both fallback variants)
// and check-update. The command env is not easily assertable without running
// dnf, so the regression here pins the parsing side: a canonical LANG=C
// updateinfo layout must flag security updates, and the localized severity
// layout must not silently corrupt the package name.

func newSilentManager() *DNFManager {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewDNFManager(log)
}

func TestParseUpgradablePackages_SecurityAdvisoryLayout(t *testing.T) {
	m := newSilentManager()

	// check-update rows (canonical LANG=C layout): name.arch, version, repo.
	// The security flag comes from the updateinfo map built by
	// getSecurityPackages, which parses advisory rows like
	// `ALSA-2025:11140 Moderate/Sec.  kernel-5.14.0-503.el9.x86_64`.
	output := `kernel.x86_64                     5.14.0-503.el9                  baseos
glib2.x86_64                      2.68.4-16.el9_6.2               baseos
`
	security := map[string]bool{"kernel": true, "glib2": true}
	installed := map[string]models.Package{
		"kernel": {Name: "kernel", CurrentVersion: "5.14.0-502.el9"},
		"glib2":  {Name: "glib2", CurrentVersion: "2.68.4-16.el9_6.1"},
	}

	result := m.parseUpgradablePackages(output, "dnf", installed, security)
	if len(result) != 2 {
		t.Fatalf("expected 2 upgradable packages, got %d", len(result))
	}
	for _, pkg := range result {
		if !pkg.IsSecurityUpdate {
			t.Errorf("package %s should be flagged as a security update", pkg.Name)
		}
	}
}

// Without LANG=C the French locale renders the severity with spaces
// ("Sécurité/niveau modéré."), shifting the package NVRA from fields[2] to
// fields[3]. extractBasePackageName must therefore never accept a bare
// severity word as a package; this pins that the shifted layout does not
// produce a bogus package named after a severity fragment.
func TestParseUpgradablePackages_TranslatedSeverityDoesNotCorruptNames(t *testing.T) {
	m := newSilentManager()

	// Simulated fr_FR layout of the same two advisories (no LANG=C):
	// the severity column contains spaces and the NVRA lands in fields[3].
	output := `ALSA-2025:11140 Sécurité/niveau modéré.  kernel-5.14.0-503.el9.x86_64
ALSA-2025:11141 Sécurité/niveau important.  glib2-2.68.4-16.el9_6.2.x86_64
`

	result := m.parseUpgradablePackages(output, "dnf", map[string]models.Package{}, map[string]bool{})
	for _, pkg := range result {
		name := pkg.Name
		for _, bad := range []string{"modéré", "important", "niveau", "Sécurité"} {
			if name == bad || len(name) == 0 {
				t.Errorf("severity fragment %q leaked into package name", name)
			}
		}
	}
}
