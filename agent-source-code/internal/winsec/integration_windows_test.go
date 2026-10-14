//go:build windows

package winsec

import (
	"os"
	"testing"
)

// requireWindowsIntegration skips tests that need an elevated token to set an
// object's owner to the Administrators group. CI sets the flag on windows-latest.
func requireWindowsIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PATCHMON_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_WINDOWS_INTEGRATION=1 to run the elevated winsec tests")
	}
}
