//go:build !linux

package system

import "time"

// containerUptime is Linux-only; every other platform the agent supports
// reports its own uptime correctly without help.
func containerUptime() (time.Duration, bool) {
	return 0, false
}
