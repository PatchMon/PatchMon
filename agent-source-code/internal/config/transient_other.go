//go:build !windows

package config

// isTransientFileError reports whether a file operation failed for a reason
// that will clear on its own.
//
// POSIX has no equivalent: rename(2) does not fail because a reader holds the
// destination open, and opening a file does not block a concurrent rename. So
// nothing here is retryable and every error is real.
func isTransientFileError(error) bool {
	return false
}
