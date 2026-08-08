//go:build windows

package config

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isTransientFileError reports whether a file operation failed for a reason
// that will clear on its own.
//
// Windows file sharing is mandatory rather than advisory, so a reader and a
// writer of the same path lock each other out in both directions: an open can
// fail with ERROR_SHARING_VIOLATION while a replace is in flight, and the
// replace itself can fail with ERROR_ACCESS_DENIED while a reader holds the
// destination. Neither means the file is damaged. The agent writes config.yml
// at the end of every LoadConfig, and separate Managers over the same path hold
// separate locks, so both directions happen in the running agent.
//
// Antivirus and the search indexer produce the same errors by briefly opening
// files they have just seen change.
func isTransientFileError(err error) bool {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case windows.ERROR_SHARING_VIOLATION,
		windows.ERROR_LOCK_VIOLATION,
		windows.ERROR_ACCESS_DENIED,
		windows.ERROR_USER_MAPPED_FILE:
		return true
	default:
		return false
	}
}
