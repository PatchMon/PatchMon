//go:build !windows

// Package winsec hardens Windows directories that hold agent secrets or staged
// executables. On other platforms these are no-ops: the file modes the agent
// and installer set already restrict access.
package winsec

import "errors"

// EnsureDataDir is a no-op on platforms other than Windows.
func EnsureDataDir() error { return nil }

// SecureUpdateDir is Windows-only; other platforms replace the binary in place
// with os.Rename and stage nothing outside the install directory.
func SecureUpdateDir() (string, error) {
	return "", errors.New("SecureUpdateDir is only used on Windows")
}
