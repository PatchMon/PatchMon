//go:build windows

package repositories

import (
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

// GetRepositories runs a COM-backed PowerShell script and falls back to a
// hardcoded Microsoft Update entry when it fails, so a broken script is
// invisible from the return value alone. The fallback is what this asserts
// against: the collector must always yield at least one source, and every
// source it yields must be well formed.
func TestGetRepositoriesLive(t *testing.T) {
	if os.Getenv("PATCHMON_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_WINDOWS_INTEGRATION=1 to run the live PowerShell collector")
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	repos, err := NewWindowsManager(logger).GetRepositories()
	if err != nil {
		t.Fatalf("GetRepositories returned an error: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("GetRepositories returned no sources; Microsoft Update should always be present")
	}

	for i, r := range repos {
		if r.Name == "" {
			t.Errorf("source %d has an empty Name: %+v", i, r)
		}
		if r.URL == "" {
			t.Errorf("source %d (%s) has an empty URL", i, r.Name)
		}
		if !utf8.ValidString(r.Name) || !utf8.ValidString(r.URL) {
			t.Errorf("source %d is not valid UTF-8: %+v", i, r)
		}
	}
	t.Logf("collected %d Windows Update sources", len(repos))
}
