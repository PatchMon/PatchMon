//go:build !windows

package packages

import "errors"

var errNotWindows = errors.New("windows patching is not supported on this platform")

func wingetTempDir() (string, error) { return "", errNotWindows }
