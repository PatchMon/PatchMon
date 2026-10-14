package system

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	alpineReleaseFile    = "/etc/alpine-release"
	alpineKernelShareDir = "/usr/share/kernel"
	kernelModulesDir     = "/lib/modules"
	inContainer          = runningInContainer
)

func isAlpine() bool {
	_, err := os.Stat(alpineReleaseFile)
	return err == nil
}

// alpineKernelFlavour returns lts from 6.18.56-0-lts: Alpine releases are <ver>-<pkgrel>-<flavour>.
func alpineKernelFlavour(release string) string {
	parts := strings.SplitN(release, "-", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

func alpineInstalledKernel(running string) string {
	flavour := alpineKernelFlavour(running)
	if flavour == "" || strings.ContainsAny(flavour, `/\`) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(alpineKernelShareDir, flavour, "kernel.release"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// A container runs the host's kernel, so there is nothing to compare.
func (d *Detector) checkAlpineRebootRequired() (bool, string) {
	if inContainer() {
		d.logger.Debug("Container: kernel reboot check not applicable")
		return false, ""
	}
	running := d.getRunningKernel()
	installed, stale := d.alpineKernelStatus(running)
	if !stale {
		return false, ""
	}
	return true, "Kernel version mismatch | Running kernel: " + running + ", Installed kernel: " + installed
}

func (d *Detector) alpineKernelStatus(running string) (installed string, stale bool) {
	if running == "" {
		return "", false
	}
	if installed = alpineInstalledKernel(running); installed != "" {
		return installed, installed != running
	}
	installed = d.selectLatestKernel(d.collectKernelsFromModules(), kernelFlavour(running))
	return installed, kernelIsOutdated(running, installed)
}

func (d *Detector) alpineLatestInstalledKernel() string {
	if inContainer() {
		return ""
	}
	installed, _ := d.alpineKernelStatus(d.getRunningKernel())
	return installed
}
