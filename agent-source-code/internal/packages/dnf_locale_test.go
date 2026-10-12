package packages

import (
	"io"
	"slices"
	"testing"

	"github.com/sirupsen/logrus"
)

func newSilentManager() *DNFManager {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewDNFManager(log)
}

func TestParseSecurityPackages(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "dnf4 Rocky 9",
			output: `Last metadata expiration check: 0:00:01 ago on Mon Oct 12 06:51:55 2026.
RLSA-2025:23343 Moderate/Sec.  binutils-2.35.2-67.el9_7.1.aarch64
RLSA-2025:0925  Moderate/Sec.  bzip2-libs-1.0.8-10.el9_5.aarch64
`,
			want: []string{"binutils", "bzip2-libs"},
		},
		{
			name: "dnf4 AlmaLinux 10 with epoch",
			output: `ALSA-2026:76781 Moderate/Sec.  glibc-common-2.39-142.el10_2.alma.1.aarch64
ALSA-2026:76918 Important/Sec. openssl-libs-1:3.5.8-2.el10_2.alma.1.aarch64
ALSA-2026:77020 Important/Sec. python3-3.12.15-1.el10_2.aarch64
`,
			want: []string{"glibc-common", "openssl-libs", "python3"},
		},
		{
			name: "dnf5 Fedora 44",
			output: `Name                   Type     Severity                              Package              Issued
FEDORA-2026-044e56d9ff security Moderate            tar-2:1.35-9.fc44.aarch64 2026-09-09 04:47:08
FEDORA-2026-12f3f3d569 security Important openssl-libs-1:3.5.9-1.fc44.aarch64 2026-10-03 01:09:41
FEDORA-2026-7d4732cbab security Important      sudo-1.9.17-10.p2.fc44.aarch64 2026-10-09 01:06:38
`,
			want: []string{"tar", "openssl-libs", "sudo"},
		},
		{
			name: "dnf5 advisory without severity",
			output: `FEDORA-2026-8efcd7a2a0 security None      curl-8.18.0-12.fc44.aarch64 2026-10-08 01:15:47
FEDORA-2026-8efcd7a2a1 security curl-libs-8.18.0-12.fc44.aarch64 2026-10-08 01:15:47
`,
			want: []string{"curl", "curl-libs"},
		},
		{
			name: "yum 3 Amazon Linux 2",
			output: `Loaded plugins: ovl, priorities
ALAS2-2023-1957 important/Sec. ca-certificates-2021.2.50-72.amzn2.0.5.noarch
ALAS2-2023-1972 medium/Sec.    cpio-2.12-11.amzn2.aarch64
ALAS2KERNEL-5.10-2024-055 medium/Sec.    kernel-5.10.29-27.128.amzn2.aarch64
updateinfo list done
`,
			want: []string{"ca-certificates", "cpio", "kernel"},
		},
	}
	m := newSilentManager()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.parseSecurityPackages([]byte(tt.output))
			if len(got) != len(tt.want) {
				t.Fatalf("parseSecurityPackages() = %v, want %v", got, tt.want)
			}
			for _, name := range tt.want {
				if !got[name] {
					t.Errorf("parseSecurityPackages() missing %q, got %v", name, got)
				}
			}
		})
	}
}

func TestMessagesCEnv(t *testing.T) {
	tests := []struct {
		name string
		src  []string
		want []string
	}{
		{
			name: "plain LANG is kept and messages forced to C",
			src:  []string{"PATH=/usr/bin", "LANG=fr_FR.UTF-8"},
			want: []string{"PATH=/usr/bin", "LANG=fr_FR.UTF-8", "LC_MESSAGES=C"},
		},
		{
			name: "LC_ALL is dropped and its charset carried into LC_CTYPE",
			src:  []string{"LC_ALL=fr_FR.UTF-8", "LANG=fr_FR.UTF-8"},
			want: []string{"LANG=fr_FR.UTF-8", "LC_CTYPE=fr_FR.UTF-8", "LC_MESSAGES=C"},
		},
		{
			name: "LC_ALL replaces an existing LC_CTYPE",
			src:  []string{"LC_ALL=fr_FR.UTF-8", "LC_CTYPE=C"},
			want: []string{"LC_CTYPE=fr_FR.UTF-8", "LC_MESSAGES=C"},
		},
		{
			name: "LC_CTYPE kept when LC_ALL is unset",
			src:  []string{"LC_CTYPE=de_DE.UTF-8", "LANG=C"},
			want: []string{"LC_CTYPE=de_DE.UTF-8", "LANG=C", "LC_MESSAGES=C"},
		},
		{
			name: "LANGUAGE and LC_MESSAGES are replaced",
			src:  []string{"LANGUAGE=fr:en", "LC_MESSAGES=fr_FR.UTF-8"},
			want: []string{"LC_MESSAGES=C"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messagesCEnv(tt.src); !slices.Equal(got, tt.want) {
				t.Fatalf("messagesCEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
