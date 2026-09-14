//go:build windows

package system

import (
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

// checkWindowsRebootRequired returns (false, "") both when no reboot is
// pending and when the PowerShell fails outright. The distinguishing signal is
// the reason string: whenever the boolean is true the reason must be populated,
// because that text goes to the server and into the UI.
func TestCheckRebootRequiredLive(t *testing.T) {
	if os.Getenv("PATCHMON_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_WINDOWS_INTEGRATION=1 to run the live PowerShell collector")
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	required, reason := New(logger).CheckRebootRequired()
	if required && reason == "" {
		t.Error("reboot reported as required with no reason; the UI has nothing to show")
	}
	if !utf8.ValidString(reason) {
		t.Errorf("reboot reason is not valid UTF-8: %q", reason)
	}
	t.Logf("CheckRebootRequired() = %v, %q", required, reason)
}
