//go:build windows

package packages

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func x86UninstallLocation(key string) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`+key, registry.QUERY_VALUE|registry.WOW64_32KEY)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	loc, _, err := k.GetStringValue("InstallLocation")
	if err != nil {
		return ""
	}
	loc = strings.Trim(strings.TrimSpace(loc), `"`)
	if pf := os.Getenv("ProgramFiles(x86)"); pf == "" || !pathInside(loc, pf) {
		return ""
	}
	return loc
}
