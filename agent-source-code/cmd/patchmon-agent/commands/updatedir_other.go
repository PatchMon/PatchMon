//go:build !windows

package commands

import "errors"

// secureUpdateDir is Windows-only. The other platforms replace the binary in
// place with os.Rename, so nothing is staged outside the install directory.
func secureUpdateDir() (string, error) {
	return "", errors.New("secureUpdateDir is only used on Windows")
}
