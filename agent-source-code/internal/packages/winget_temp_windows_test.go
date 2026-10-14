//go:build windows

package packages

import (
	"path/filepath"
	"strings"
	"testing"

	"patchmon-agent/internal/winsec"
)

func TestNewRunTempDirIsPrivate(t *testing.T) {
	requireWindowsIntegration(t)
	fresh, err := newRunTempDir(t.TempDir())
	if err != nil {
		t.Fatalf("newRunTempDir: %v (the test must run elevated)", err)
	}
	if !strings.HasPrefix(filepath.Base(fresh), privateDirPrefix) {
		t.Errorf("unexpected name %s", fresh)
	}
	if err := winsec.VerifySecureDir(fresh); err != nil {
		t.Errorf("a fresh private dir fails verification: %v", err)
	}
}
