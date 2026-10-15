package packages

import (
	"bufio"
	"os"
	"strings"
)

var ucsBaseConf = "/etc/univention/base.conf"

// IsUCS reports whether the host is Univention Corporate Server.
func IsUCS() bool {
	info, err := os.Stat(ucsBaseConf)
	return err == nil && info.Mode().IsRegular()
}

// UCSRelease returns the installed UCS release as "<version>-<patchlevel>", such as "5.2-7",
// or "" when it cannot be read.
func UCSRelease() string {
	f, err := os.Open(ucsBaseConf)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	var version, patchlevel string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ": ")
		if !ok {
			continue
		}
		switch key {
		case "version/version":
			version = strings.TrimSpace(value)
		case "version/patchlevel":
			patchlevel = strings.TrimSpace(value)
		}
	}
	if version == "" || patchlevel == "" {
		return ""
	}
	return version + "-" + patchlevel
}
