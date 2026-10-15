package system

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

func alpineTestDetector(t *testing.T, container bool, kernelRelease map[string]string, modules []string) *Detector {
	t.Helper()
	root := t.TempDir()
	share := filepath.Join(root, "share")
	mods := filepath.Join(root, "modules")
	for flavour, release := range kernelRelease {
		dir := filepath.Join(share, flavour)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "kernel.release"), []byte(release+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range modules {
		if err := os.MkdirAll(filepath.Join(mods, m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	origShare, origMods, origContainer := alpineKernelShareDir, kernelModulesDir, inContainer
	alpineKernelShareDir, kernelModulesDir = share, mods
	inContainer = func() bool { return container }
	t.Cleanup(func() { alpineKernelShareDir, kernelModulesDir, inContainer = origShare, origMods, origContainer })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return New(logger)
}

func TestAlpineKernelFlavour(t *testing.T) {
	cases := map[string]string{
		"6.18.56-0-lts":    "lts",
		"6.18.56-0-virt":   "virt",
		"6.12.47-0-rpi":    "rpi",
		"6.12.76-linuxkit": "",
		"":                 "",
	}
	for in, want := range cases {
		if got := alpineKernelFlavour(in); got != want {
			t.Errorf("alpineKernelFlavour(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAlpineKernelStatus(t *testing.T) {
	cases := []struct {
		name          string
		running       string
		kernelRelease map[string]string
		modules       []string
		wantInstalled string
		wantStale     bool
	}{
		{"same kernel", "6.18.56-0-lts", map[string]string{"lts": "6.18.56-0-lts"}, nil, "6.18.56-0-lts", false},
		{"newer lts installed", "6.18.56-0-lts", map[string]string{"lts": "6.18.58-0-lts"}, nil, "6.18.58-0-lts", true},
		{"pkgrel bump", "6.18.56-0-virt", map[string]string{"virt": "6.18.56-1-virt"}, nil, "6.18.56-1-virt", true},
		{"only another flavour upgraded", "6.18.56-0-virt", map[string]string{"virt": "6.18.56-0-virt", "lts": "6.18.58-0-lts"}, nil, "6.18.56-0-virt", false},
		{"falls back to modules", "6.12.112-0-lts", nil, []string{"6.12.112-0-lts", "6.12.120-0-lts", "6.18.56-0-virt"}, "6.12.120-0-lts", true},
		{"old module dirs linger", "6.12.120-0-lts", nil, []string{"6.12.112-0-lts", "6.12.120-0-lts"}, "6.12.120-0-lts", false},
		{"no kernel package", "6.12.76-linuxkit", nil, nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := alpineTestDetector(t, false, tc.kernelRelease, tc.modules)
			installed, stale := d.alpineKernelStatus(tc.running)
			if installed != tc.wantInstalled || stale != tc.wantStale {
				t.Errorf("alpineKernelStatus(%q) = %q, %v; want %q, %v", tc.running, installed, stale, tc.wantInstalled, tc.wantStale)
			}
		})
	}
}

func TestAlpineRebootNotApplicableInContainer(t *testing.T) {
	d := alpineTestDetector(t, true, map[string]string{"lts": "6.18.58-0-lts"}, nil)
	if needed, reason := d.checkAlpineRebootRequired(); needed || reason != "" {
		t.Errorf("container reported reboot required: %q", reason)
	}
	if got := d.alpineLatestInstalledKernel(); got != "" {
		t.Errorf("container reported installed kernel %q", got)
	}
}
