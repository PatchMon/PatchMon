package commands

import "testing"

// TestValidAptPackagePattern_RejectsRangeArtefacts is the regression guard
// for the character-class range bug.
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
