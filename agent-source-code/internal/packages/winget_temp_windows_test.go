//go:build windows

package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateDirDescriptor(t *testing.T) {
	sd, err := windows.SecurityDescriptorFromString(privateDirSDDL)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateDescriptor(sd); err != nil {
		t.Fatalf("the agent's own descriptor fails its check: %v", err)
	}
	owner, _, _ := sd.Owner()
	if !owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		t.Errorf("owner = %s, want Administrators", owner)
	}
	for _, bad := range []string{
		"O:BAD:(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
		"O:BUD:P(A;OICI;FA;;;SY)",
		"O:BAD:P(A;OICI;FA;;;SY)(A;OICI;0x6;;;BU)",
		"O:BAD:P(A;OICI;FA;;;WD)",
	} {
		sd, err := windows.SecurityDescriptorFromString(bad)
		if err != nil {
			t.Fatal(err)
		}
		if checkPrivateDescriptor(sd) == nil {
			t.Errorf("%s passed the check", bad)
		}
	}
}

func TestCreatePrivateDirRefusesAnExistingDir(t *testing.T) {
	requireWindowsIntegration(t)
	dir := filepath.Join(t.TempDir(), "planted")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := createPrivateDir(dir); err == nil {
		t.Fatal("an existing directory was adopted")
	}
	if err := verifySecureDir(dir); err == nil {
		t.Fatal("a directory with inherited, user-writable permissions passed verification")
	}
	fresh, err := newRunTempDir(t.TempDir())
	if err != nil {
		t.Fatalf("newRunTempDir: %v (the test must run elevated)", err)
	}
	if !strings.HasPrefix(filepath.Base(fresh), privateDirPrefix) {
		t.Errorf("unexpected name %s", fresh)
	}
	if err := verifySecureDir(fresh); err != nil {
		t.Errorf("a fresh private dir fails verification: %v", err)
	}
}
