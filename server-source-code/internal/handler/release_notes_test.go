package handler

import (
	"strings"
	"testing"
)

// Release notes are not authored in this repo. They live in the body of the
// GitHub release, and CI writes RELEASE_NOTES_<version>.md into the tree at
// build time (see .github/actions/release-context). So this cannot assert that
// notes exist for the current version: on main, straight after a release, they
// deliberately do not. What it can assert is that whatever is embedded is
// usable.
func TestReleaseNotesEmbedded(t *testing.T) {
	if len(releaseNotesContent) == 0 {
		t.Fatal("no release notes embedded; check the //go:embed pattern and release_notes_data/")
	}
	for version, content := range releaseNotesContent {
		if strings.TrimSpace(content) == "" {
			t.Errorf("release notes for %s are empty", version)
		}
	}
}

// The map is keyed by the version the server reports, so a malformed filename
// would make the "What's New" modal silently blank for that release.
func TestReleaseNotesKeysAreBareSemver(t *testing.T) {
	for version := range releaseNotesContent {
		parts := strings.Split(version, ".")
		if len(parts) != 3 {
			t.Errorf("release notes key %q is not MAJOR.MINOR.PATCH", version)
			continue
		}
		for _, p := range parts {
			if p == "" || strings.TrimLeft(p, "0123456789") != "" {
				t.Errorf("release notes key %q has a non-numeric component %q", version, p)
				break
			}
		}
	}
}
