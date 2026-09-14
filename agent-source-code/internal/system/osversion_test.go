package system

import "testing"

func TestResolveWindowsOSVersion(t *testing.T) {
	cases := []struct {
		name            string
		platformVersion string
		releaseID       string
		kernelVersion   string
		want            string
	}{
		{"server 2022 reports DisplayVersion", "23H2", "2009", "10.0.20348.2762 Build 20348.2762", "23H2"},
		{"server 2016 falls back to ReleaseId", "", "1607", "10.0.14393.9418 Build 14393.9418", "1607"},
		{"server 2019 falls back to ReleaseId", "", "1809", "10.0.17763.6414 Build 17763.6414", "1809"},
		{"registry unreadable falls back to kernel version", "", "", "10.0.14393.9418 Build 14393.9418", "10.0.14393.9418 Build 14393.9418"},
		{"nothing available", "", "", "", "Unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveWindowsOSVersion(tc.platformVersion, tc.releaseID, tc.kernelVersion)
			if got != tc.want {
				t.Errorf("resolveWindowsOSVersion(%q, %q, %q) = %q, want %q",
					tc.platformVersion, tc.releaseID, tc.kernelVersion, got, tc.want)
			}
		})
	}
}
