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
	m := newSilentManager()
	output := `Last metadata expiration check: 0:12:34 ago on Mon 06 Sep 2026 10:00:00 BST.
ALSA-2025:11140 Moderate/Sec.  kernel-5.14.0-503.el9.x86_64
RLSA-2025:11141 Important/Sec. glib2-2.68.4-16.el9_6.2.x86_64
`
	got := m.parseSecurityPackages([]byte(output))
	if len(got) != 2 || !got["kernel"] || !got["glib2"] {
		t.Fatalf("parseSecurityPackages() = %v, want kernel and glib2", got)
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
			name: "existing LC_CTYPE wins over LC_ALL",
			src:  []string{"LC_ALL=fr_FR.UTF-8", "LC_CTYPE=de_DE.UTF-8"},
			want: []string{"LC_CTYPE=de_DE.UTF-8", "LC_MESSAGES=C"},
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
