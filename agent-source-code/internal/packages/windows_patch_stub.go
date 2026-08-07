//go:build !windows

package packages

import (
	"context"
	"fmt"
)

// WindowsPatcher is a stub for non-Windows platforms.
type WindowsPatcher struct{}

// NewWindowsPatcher returns a no-op patcher on non-Windows platforms.
func NewWindowsPatcher() *WindowsPatcher { return &WindowsPatcher{} }

// InstallWindowsUpdate is a no-op stub on non-Windows platforms.
func (p *WindowsPatcher) InstallWindowsUpdate(_ context.Context, guid string) (string, error) {
	return "", fmt.Errorf("windows update installation not supported on this platform (guid: %s)", guid)
}

// WinGetUpgradeAll is a no-op stub on non-Windows platforms.
func (p *WindowsPatcher) WinGetUpgradeAll(_ context.Context, _ bool) (string, error) {
	return "", fmt.Errorf("winget not available on this platform")
}

// WinGetUpgradePackage is a no-op stub on non-Windows platforms.
func (p *WindowsPatcher) WinGetUpgradePackage(_ context.Context, packageID string, _ bool) (string, error) {
	return "", fmt.Errorf("winget not available on this platform (package: %s)", packageID)
}

// IsSuperseded always returns false on non-Windows platforms.
func IsSuperseded(_ string) bool {
	return false
}

// RebootRequired always returns false on non-Windows platforms.
func RebootRequired() bool {
	return false
}
