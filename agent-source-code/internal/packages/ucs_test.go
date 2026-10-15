package packages

import (
	"os"
	"path/filepath"
	"testing"
)

func withUCSBaseConf(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "base.conf")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := ucsBaseConf
	ucsBaseConf = path
	t.Cleanup(func() { ucsBaseConf = old })
}

func TestUCSRelease(t *testing.T) {
	withUCSBaseConf(t, "# univention_ base.conf\n\nversion/erratalevel: 606\nversion/patchlevel: 7\nversion/version: 5.2\nupdate/available: yes\n")
	if !IsUCS() {
		t.Fatal("IsUCS() = false with a base.conf present")
	}
	if got := UCSRelease(); got != "5.2-7" {
		t.Fatalf("UCSRelease() = %q, want 5.2-7", got)
	}
}

func TestUCSReleaseIncomplete(t *testing.T) {
	withUCSBaseConf(t, "version/version: 5.2\n")
	if got := UCSRelease(); got != "" {
		t.Fatalf("UCSRelease() = %q, want empty without a patchlevel", got)
	}
}

func TestIsUCSWithoutBaseConf(t *testing.T) {
	withUCSBaseConf(t, "")
	if IsUCS() {
		t.Fatal("IsUCS() = true without a base.conf")
	}
	if got := UCSRelease(); got != "" {
		t.Fatalf("UCSRelease() = %q, want empty", got)
	}
}
