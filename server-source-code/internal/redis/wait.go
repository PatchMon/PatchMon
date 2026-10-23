package redis

import (
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
)

var authErrorMarkers = []string{"NOAUTH", "WRONGPASS", "NOPERM", "invalid password", "without any password configured"}

// IsPermanentError reports whether a Redis error is an authentication or certificate failure that will recur on retry.
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}
	if database.IsCertificateError(err) {
		return true
	}
	msg := err.Error()
	for _, m := range authErrorMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
