package system

import (
	"bufio"
	"os"
	"strings"
)

var lsbReleaseFile = "/etc/lsb-release"

const ucsOSType = "Univention Corporate Server"

// ucsRelease returns the UCS release from lsb-release, such as "5.2-7 errata606", or "" when
// the host is not UCS. UCS 5.0 names itself Debian in os-release, so os-release cannot tell.
func ucsRelease(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	var id, release string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "DISTRIB_ID":
			id = value
		case "DISTRIB_RELEASE":
			release = value
		}
	}
	if id != "Univention" || release == "" {
		return ""
	}
	return release
}
