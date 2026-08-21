//go:build windows

package system

import "golang.org/x/sys/windows/registry"

// ReleaseId is the only marketing version on Server 2016 and 2019 LTSC, where
// DisplayVersion (gopsutil's PlatformVersion) does not exist.
func windowsReleaseID() string {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()

	v, _, err := k.GetStringValue("ReleaseId")
	if err != nil {
		return ""
	}
	return v
}
