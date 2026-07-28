package commands

import "testing"

// TestValidAptPackagePattern_RejectsRangeArtefacts is the regression guard for
// the character-class range bug.
//
// The pattern was `^[a-zA-Z0-9][a-zA-Z0-9.+-_]*$`. The intended literal set
// ". + - _" was parsed by the regexp engine as a RANGE from '+' (0x2B) to '_'
// (0x5F), which admits every character in between: / \ ; : , < > = ? @ and the
// whole uppercase block plus [ ] ^.
//
// That is not shell injection (nothing is passed through a shell on Unix), it
// is an argument-semantics hole: apt and dnf both treat an argument containing
// a slash as a path to a local package file, resolved against the process CWD
// ("/" under systemd). A run_patch carrying "tmp/evil.deb" therefore became
// "apt-get install -y tmp/evil.deb" as root, giving any local user able to
// write to /tmp a remotely-triggered root install primitive.
func TestValidAptPackagePattern_RejectsRangeArtefacts(t *testing.T) {
	t.Parallel()

	// Every one of these was accepted by the buggy range.
	rejected := []string{
		"tmp/evil.deb", // the actual exploit shape: apt reads it as a local .deb
		"../../tmp/evil.rpm",
		"a;rm",
		`a\b`,
		"a:b",
		"a,b",
		"a<b",
		"a>b",
		"a=b",
		"a@b",
		"a[b]",
		"a^b",
		"a?b",
		"nginx/../../etc",
	}
	for _, name := range rejected {
		if validAptPackagePattern.MatchString(name) {
			t.Errorf("package name %q must be rejected", name)
		}
	}

	// Genuinely valid Debian and RPM package names must still pass.
	accepted := []string{
		"nginx",
		"lib32-glibc",
		"g++",
		"libstdc++6",
		"python3.11",
		"linux-image-6.1.0-13-amd64",
		"ca-certificates",
		"lib_foo",
		"7zip",
		"a",
	}
	for _, name := range accepted {
		if !validAptPackagePattern.MatchString(name) {
			t.Errorf("package name %q must be accepted", name)
		}
	}

	// Structural rules retained from the original pattern.
	mustReject := []string{
		"",           // empty
		"-leading",   // must start alphanumeric
		".leading",   // must start alphanumeric
		"_leading",   // must start alphanumeric
		"with space", // no whitespace
		"a b",
		"a$b",
		"a|b",
		"a*b",
		"a`b",
		`a"b`,
		"a'b",
	}
	for _, name := range mustReject {
		if validAptPackagePattern.MatchString(name) {
			t.Errorf("package name %q must be rejected", name)
		}
	}
}
