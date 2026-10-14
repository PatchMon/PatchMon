//go:build windows

package packages

import "os"

func fileOwner(os.FileInfo) (int, bool) {
	return 0, false
}
