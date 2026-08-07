//go:build !windows
// +build !windows

package commands

// isWindowsService returns false on non-Windows (stub for cross-platform use)
func isWindowsService() bool {
	return false
}

// runAsService on non-Windows just runs the service loop directly
func runAsService() error {
	// On Unix, we don't need Windows Service wrapper
	// Just run the service loop with no stop channel (runs forever)
	return runServiceLoop(nil)
}
