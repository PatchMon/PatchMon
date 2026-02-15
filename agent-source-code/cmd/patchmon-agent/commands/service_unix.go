//go:build !windows
// +build !windows

package commands

// runAsService on non-Windows just runs the service loop directly
func runAsService() error {
	// On Unix, we don't need Windows Service wrapper
	// Just run the service loop with no stop channel (runs forever)
	return runServiceLoop(nil)
}
