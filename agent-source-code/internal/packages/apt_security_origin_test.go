package packages

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
)

// The security pocket must be read from the origin list only. The package name
// sits on the same line, and Debian main ships packages whose names contain
// "security" (libapache2-mod-security2, modsecurity-crs, debian-security-support
// and around 40 others), which a whole-line match flags as security updates.
func TestParseAPTUpgrade_SecurityIsReadFromOriginNotPackageName(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	m := &APTManager{logger: log}

	output := `Inst libssl1.1 [1.1.1w-0+deb11u7] (1.1.1w-0+deb11u8 Debian-Security:11/oldoldstable [armhf])
Inst libapache2-mod-security2 [2.9.3-3] (2.9.3-4 Raspbian:11/oldoldstable [armhf])
Inst modsecurity-crs [3.3.2-1] (3.3.3-1 Debian:11/oldoldstable [all])
Inst debian-security-support [1:11+2021.03.15] (1:11+2022.02.02 Debian:11/oldoldstable [all])
Inst curl [7.74.0-1.3+deb11u11] (7.74.0-1.3+deb11u14 Debian-Security:11/oldoldstable [armhf])
`

	got := m.parseAPTUpgrade(output)

	want := map[string]bool{
		"libssl1.1":                true,
		"libapache2-mod-security2": false,
		"modsecurity-crs":          false,
		"debian-security-support":  false,
		"curl":                     true,
	}

	if len(got) != len(want) {
		t.Fatalf("parsed %d packages, want %d", len(got), len(want))
	}
	for _, p := range got {
		expected, ok := want[p.Name]
		if !ok {
			t.Errorf("unexpected package %q", p.Name)
			continue
		}
		if p.IsSecurityUpdate != expected {
			t.Errorf("%s: IsSecurityUpdate = %v, want %v", p.Name, p.IsSecurityUpdate, expected)
		}
	}
}

// Raspbian publishes no security suite at all, so nothing on a Raspberry Pi OS
// host can be classified. Recorded so the behaviour is deliberate rather than
// accidental: see #782.
func TestParseAPTUpgrade_RaspbianHasNoSecurityOrigin(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	m := &APTManager{logger: log}

	output := `Inst openssl [1.1.1w-0+deb11u7] (1.1.1w-0+deb11u8 Raspbian:11/oldoldstable [armhf])
Inst libpng16-16 [1.6.37-3] (1.6.37-3+deb11u4 Raspbian:11/oldoldstable [armhf])
`

	for _, p := range m.parseAPTUpgrade(output) {
		if p.IsSecurityUpdate {
			t.Errorf("%s flagged as a security update, but Raspbian carries no security origin", p.Name)
		}
	}
}
