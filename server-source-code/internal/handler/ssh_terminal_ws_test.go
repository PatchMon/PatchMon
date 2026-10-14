package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestSSHUsername(t *testing.T) {
	windows := "windows"
	linux := "linux"
	cases := []struct {
		name      string
		requested string
		host      models.Host
		want      string
	}{
		{"linux default", "", models.Host{OSType: "Ubuntu"}, "root"},
		{"freebsd default", "", models.Host{OSType: "FreeBSD"}, "root"},
		{"unreported host", "", models.Host{}, "root"},
		{"windows reported", "", models.Host{OSType: "Windows Server 2022 Datacenter"}, "Administrator"},
		{"windows expected before first report", "", models.Host{ExpectedPlatform: &windows}, "Administrator"},
		{"linux expected", "", models.Host{ExpectedPlatform: &linux}, "root"},
		{"explicit username wins on windows", "patchadmin", models.Host{OSType: "Windows 11 Pro"}, "patchadmin"},
		{"explicit username wins on linux", "ubuntu", models.Host{OSType: "Ubuntu"}, "ubuntu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshUsername(tc.requested, &tc.host); got != tc.want {
				t.Fatalf("sshUsername(%q) = %q, want %q", tc.requested, got, tc.want)
			}
		})
	}
}
