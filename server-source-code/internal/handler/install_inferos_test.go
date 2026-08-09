package handler

import "testing"

func TestInferHostOS(t *testing.T) {
	tests := []struct {
		name             string
		expectedPlatform *string
		osType           string
		want             string
	}{
		{name: "expected platform windows", expectedPlatform: strPtr("windows"), want: "windows"},
		{name: "expected platform freebsd", expectedPlatform: strPtr("FreeBSD"), want: "freebsd"},
		{name: "expected platform pfsense", expectedPlatform: strPtr("pfSense"), want: "freebsd"},
		{name: "expected platform linux", expectedPlatform: strPtr("linux"), want: "linux"},
		{name: "expected platform empty falls back to linux", expectedPlatform: strPtr(""), want: "linux"},
		{name: "expected platform wins over os type", expectedPlatform: strPtr("linux"), osType: "FreeBSD 14.4-RELEASE", want: "linux"},

		// The regression case: a FreeBSD host that reported its OS but has no
		// expected platform must not be treated as Linux, or it is served a
		// Linux agent binary it cannot execute.
		{name: "os type freebsd", osType: "FreeBSD 14.4-RELEASE", want: "freebsd"},
		{name: "os type pfsense", osType: "pfSense 2.7", want: "freebsd"},
		{name: "os type windows", osType: "Windows Server 2022", want: "windows"},
		{name: "os type debian", osType: "Debian GNU/Linux", want: "linux"},
		{name: "os type rocky", osType: "Rocky Linux", want: "linux"},

		{name: "nothing known defaults to linux", want: "linux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferHostOS(tt.expectedPlatform, tt.osType)
			if got != tt.want {
				t.Errorf("inferHostOS(%v, %q) = %q, want %q", tt.expectedPlatform, tt.osType, got, tt.want)
			}
		})
	}
}
