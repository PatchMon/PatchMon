package system

import "testing"

func TestResolveWindowsOSVersion(t *testing.T) {
	cases := []struct {
		name            string
		platformFamily  string
		platformVersion string
		releaseID       string
		kernelVersion   string
		want            string
	}{
		{"server 2016", "Server", "", "1607", "10.0.14393.693 Build 14393.693", "Server 2016 (1607)"},
		{"server 2019", "Server", "", "1809", "10.0.17763.3650 Build 17763.3650", "Server 2019 (1809)"},
		{"server 2022", "Server", "21H2", "2009", "10.0.20348.587 Build 20348.587", "Server 2022 (21H2)"},
		{"server 2025", "Server", "24H2", "2009", "10.0.26100.1742 Build 26100.1742", "Server 2025 (24H2)"},
		{"domain controller", "Server (Domain Controller)", "", "1809", "10.0.17763.6414 Build 17763.6414", "Server 2019 (1809)"},
		{"server 2016 registry unreadable", "Server", "", "", "10.0.14393.9418 Build 14393.9418", "Server 2016"},
		{"unmapped server build", "Server", "23H2", "2009", "10.0.25398.1189 Build 25398.1189", "Server 23H2"},
		{"windows 11 24H2 shares the 2025 build", "Standalone Workstation", "24H2", "2009", "10.0.26100.4652 Build 26100.4652", "24H2"},
		{"windows 10 LTSC 2019 falls back to ReleaseId", "Standalone Workstation", "", "1809", "10.0.17763.6414 Build 17763.6414", "1809"},
		{"registry unreadable falls back to kernel version", "", "", "", "10.0.14393.9418 Build 14393.9418", "10.0.14393.9418 Build 14393.9418"},
		{"nothing available", "", "", "", "", "Unknown"},
		{"server with nothing available", "Server", "", "", "", "Unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveWindowsOSVersion(tc.platformFamily, tc.platformVersion, tc.releaseID, tc.kernelVersion)
			if got != tc.want {
				t.Errorf("resolveWindowsOSVersion(%q, %q, %q, %q) = %q, want %q",
					tc.platformFamily, tc.platformVersion, tc.releaseID, tc.kernelVersion, got, tc.want)
			}
		})
	}
}

func TestWindowsBuild(t *testing.T) {
	cases := map[string]int{
		"10.0.20348.587 Build 20348.587": 20348,
		"10.0.26100.1742":                26100,
		"10.0":                           0,
		"10.0.abc.1 Build x":             0,
		"":                               0,
	}
	for in, want := range cases {
		if got := windowsBuild(in); got != want {
			t.Errorf("windowsBuild(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestResolveLinuxOSVersion(t *testing.T) {
	cases := []struct {
		name          string
		info          OSReleaseInfo
		alpineRelease string
		want          string
	}{
		{"debian VERSION wins", OSReleaseInfo{Version: "12 (bookworm)", VersionID: "12"}, "", "12 (bookworm)"},
		{"ubuntu", OSReleaseInfo{Version: "24.04.1 LTS (Noble Numbat)", VersionID: "24.04"}, "", "24.04.1 LTS (Noble Numbat)"},
		{"alpine has only VERSION_ID", OSReleaseInfo{Name: "Alpine Linux", ID: "alpine", VersionID: "3.23.0", PrettyName: "Alpine Linux v3.23"}, "", "3.23.0"},
		{"alpine edge", OSReleaseInfo{ID: "alpine", VersionID: "3.25.0_alpha20260805"}, "", "3.25.0_alpha20260805"},
		{"alpine without VERSION_ID reads alpine-release", OSReleaseInfo{ID: "alpine"}, "3.21.8", "3.21.8"},
		{"arch host", OSReleaseInfo{Name: "Arch Linux", ID: "arch", BuildID: "rolling"}, "", "rolling"},
		{"arch container image", OSReleaseInfo{ID: "arch", BuildID: "rolling", VersionID: "20261011.0.609700"}, "", "rolling"},
		{"build id only", OSReleaseInfo{BuildID: "2026.10"}, "", "2026.10"},
		{"nothing", OSReleaseInfo{}, "", "Unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLinuxOSVersion(&tc.info, tc.alpineRelease); got != tc.want {
				t.Errorf("resolveLinuxOSVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}
