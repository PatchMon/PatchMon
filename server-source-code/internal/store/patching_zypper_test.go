package store

import (
	"reflect"
	"testing"
)

func TestParsePackagesAffectedZypper(t *testing.T) {
	dry := `$ zypper --non-interactive --non-interactive-include-reboot-patches --xmlout patch --dry-run --with-update --auto-agree-with-licenses
[patchmon] zypper plans 4 package changes:
  upgrade curl 8.0.1-150400.5.50.1 -> 8.0.1-150400.5.59.1
  upgrade libstdc++6 13.3.0+git8781-150000.1.12.1 -> 14.2.0+git10526-150000.1.6.1
  install kernel-default 6.4.0-150600.23.30.1
  remove python3.11-old 1.0-1.1
[patchmon] Patches: openSUSE-SLE-15.5-2024-3926

--- Dry run completed at 2026-10-14T16:00:00Z ---`

	if got, want := parsePackagesAffectedFromDryRunOutput("openSUSE Leap", dry), []string{"curl", "libstdc++6", "kernel-default", "python3.11-old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("dry run = %v, want %v", got, want)
	}

	real := `$ zypper --non-interactive --non-interactive-include-reboot-patches --xmlout patch --dry-run --with-update --auto-agree-with-licenses
[patchmon] zypper plans 5 package changes:
  upgrade libsolv-tools-base 0.7.30-150400.3.27.2 -> 0.7.31-150500.6.5.1
  upgrade zypper 1.14.76-150500.6.6.15 -> 1.14.78-150500.6.14.1
  upgrade curl 8.0.1-150400.5.50.1 -> 8.0.1-150400.5.59.1
$ zypper --non-interactive --non-interactive-include-reboot-patches patch --with-update --auto-agree-with-licenses
Retrieving: libsolv-tools-base-0.7.31-150500.6.5.1.aarch64 (Update repository with updates from SUSE Linux Enterprise 15) (1/4), 228.8 KiB
Retrieving: libsolv-tools-base-0.7.31-150500.6.5.1.aarch64.rpm [...done (1.4 MiB/s)]

Checking for file conflicts: [..done]
(1/3) Installing: libsolv-tools-base-0.7.31-150500.6.5.1.aarch64 [..done]
(2/3) Installing: libzypp-17.35.16-150500.6.31.1.aarch64 [..
warning: /etc/zypp/zypp.conf created as /etc/zypp/zypp.conf.rpmnew
.done]
(3/3) Removing: openSUSE-build-key-1.0-lp156.8.2.noarch [..done]
Warning: One of the installed patches affects the package manager itself. Run this command once more to install any other needed patches.
$ zypper --non-interactive --non-interactive-include-reboot-patches patch --with-update --auto-agree-with-licenses
( 1/2) Installing: python3.11-base-3.11.10-150600.3.10.1.x86_64 [..done]
( 2/2) Installing: libstdc++6-14.2.0+git10526-150000.1.6.1.x86_64 [..done]

--- Patch run completed at 2026-10-14T16:00:00Z ---`

	want := []string{"libsolv-tools-base", "libzypp", "openSUSE-build-key", "python3.11-base", "libstdc++6"}
	if got := parsePackagesAffectedFromRealOutput("openSUSE Leap", real); !reflect.DeepEqual(got, want) {
		t.Errorf("real run = %v, want %v", got, want)
	}
}

func TestRPMNameFromNVRA(t *testing.T) {
	cases := map[string]string{
		"libsolv-tools-base-0.7.31-150500.6.5.1.aarch64": "libsolv-tools-base",
		"openSUSE-build-key-1.0-lp156.8.2.noarch":        "openSUSE-build-key",
		"which-2.25-1.3": "which",
		"bad":            "",
	}
	for in, want := range cases {
		if got := rpmNameFromNVRA(in); got != want {
			t.Errorf("rpmNameFromNVRA(%q) = %q, want %q", in, got, want)
		}
	}
}
