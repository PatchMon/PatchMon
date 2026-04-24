package store

import (
	"reflect"
	"slices"
	"testing"
)

func TestParsePackagesAffectedFromDryRunOutput(t *testing.T) {
	t.Run("parses apt simulate output", func(t *testing.T) {
		output := `Inst openssl [3.0.2] (3.0.3 Debian:stable)
Inst curl [7.88.1] (8.0.0 Debian:stable)`

		got := parsePackagesAffectedFromDryRunOutput("ubuntu", output)
		want := []string{"openssl", "curl"}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, want)
		}
	})

	t.Run("parses freebsd pkg summary output", func(t *testing.T) {
		output := `Checking integrity... done (0 conflicting)
The following 3 package(s) will be affected (of 0 checked):

Installed packages to be UPGRADED:
	curl: 8.9.1 -> 8.10.0

Installed packages to be INSTALLED:
	libnghttp2: 1.63.0
	ca_root_nss: 3.104

Number of packages to be upgraded: 1
Number of packages to be installed: 2`

		got := parsePackagesAffectedFromDryRunOutput("freebsd", output)
		want := []string{"curl", "libnghttp2", "ca_root_nss"}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, want)
		}
	})

	t.Run("includes freebsd base when fetch reports updates", func(t *testing.T) {
		output := `$ /usr/sbin/freebsd-update fetch --not-running-from-cron
Looking up update.FreeBSD.org mirrors... 3 mirrors found.
Fetching metadata signature for 14.2-RELEASE from update2.freebsd.org... done.
The following files will be updated as part of updating to 14.2-RELEASE-p3:
/bin/freebsd-version
/usr/lib/libc.so.7`

		got := parsePackagesAffectedFromDryRunOutput("freebsd", output)
		want := []string{freeBSDBasePackageName}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, want)
		}
	})

	// Regression: apt dry-run output contains the phrase
	// "The following additional packages will be installed:" which previously
	// tripped the freeBSDUpdateOutputHasPendingUpdates heuristic and injected
	// "freebsd-base" into Linux validation runs. Gating on osType eliminates
	// this class of false positive entirely.
	t.Run("apt single package dry run does not emit freebsd-base on linux", func(t *testing.T) {
		output := `$ apt-get update -qq
$ apt-get -s install libcap2
Reading package lists...
Building dependency tree...
Reading state information...
The following packages were automatically installed and are no longer required:
  linux-headers-6.8.0-101 linux-headers-6.8.0-101-generic
  linux-image-6.8.0-101-generic linux-modules-6.8.0-101-generic
  linux-modules-extra-6.8.0-101-generic linux-tools-6.8.0-101
  linux-tools-6.8.0-101-generic
Use 'apt autoremove' to remove them.
The following additional packages will be installed:
  libcap2-bin libpam-cap
The following packages will be upgraded:
  libcap2 libcap2-bin libpam-cap
3 upgraded, 0 newly installed, 0 to remove and 12 not upgraded.
Inst libcap2 [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Conf libcap2 (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst libpam-cap [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst libcap2-bin [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Conf libpam-cap (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Conf libcap2-bin (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])

--- Dry run completed at 2026-04-23T06:15:05Z ---`

		got := parsePackagesAffectedFromDryRunOutput("ubuntu", output)
		want := []string{"libcap2", "libpam-cap", "libcap2-bin"}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, want)
		}
		if slices.Contains(got, freeBSDBasePackageName) {
			t.Fatalf("freebsd-base must not appear in Linux dry-run output: %v", got)
		}
	})

	t.Run("apt bulk dry run does not emit freebsd-base on linux", func(t *testing.T) {
		output := `$ apt-get update -qq
$ apt-get -s install libcap2 libcap2-bin libntfs-3g89t64 libpam-cap libpython3.12-minimal libpython3.12-stdlib libpython3.12t64 ntfs-3g python3.12 python3.12-minimal
Reading package lists...
Building dependency tree...
Reading state information...
The following packages were automatically installed and are no longer required:
  linux-headers-6.8.0-101 linux-headers-6.8.0-101-generic
Use 'apt autoremove' to remove them.
Suggested packages:
  python3.12-venv python3.12-doc binutils binfmt-support
The following packages will be upgraded:
  libcap2 libcap2-bin libntfs-3g89t64 libpam-cap libpython3.12-minimal
  libpython3.12-stdlib libpython3.12t64 ntfs-3g python3.12 python3.12-minimal
10 upgraded, 0 newly installed, 0 to remove and 5 not upgraded.
Inst libpython3.12t64 [3.12.3-1ubuntu0.12] (3.12.3-1ubuntu0.13 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64]) []
Inst python3.12 [3.12.3-1ubuntu0.12] (3.12.3-1ubuntu0.13 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64]) []
Inst libpython3.12-stdlib [3.12.3-1ubuntu0.12] (3.12.3-1ubuntu0.13 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64]) []
Inst python3.12-minimal [3.12.3-1ubuntu0.12] (3.12.3-1ubuntu0.13 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64]) []
Inst libpython3.12-minimal [3.12.3-1ubuntu0.12] (3.12.3-1ubuntu0.13 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst ntfs-3g [1:2022.10.3-1.2ubuntu3] (1:2022.10.3-1.2ubuntu3.1 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64]) []
Inst libntfs-3g89t64 [1:2022.10.3-1.2ubuntu3] (1:2022.10.3-1.2ubuntu3.1 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst libcap2 [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Conf libcap2 (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst libpam-cap [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst libcap2-bin [1:2.66-5ubuntu2.2] (1:2.66-5ubuntu2.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])

--- Dry run completed at 2026-04-23T06:12:34Z ---`

		got := parsePackagesAffectedFromDryRunOutput("ubuntu", output)
		if slices.Contains(got, freeBSDBasePackageName) {
			t.Fatalf("freebsd-base must not appear in Linux dry-run output: %v", got)
		}
		// Sanity: some of the actual packages must still be parsed.
		for _, want := range []string{"libcap2", "python3.12", "ntfs-3g"} {
			if !slices.Contains(got, want) {
				t.Fatalf("expected %q in parsed packages, got %v", want, got)
			}
		}
	})
}

func TestParsePackagesAffectedFromRealOutput(t *testing.T) {
	t.Run("freebsd combined real output includes freebsd-base", func(t *testing.T) {
		output := `$ /usr/sbin/freebsd-update fetch --not-running-from-cron
The following files will be updated as part of updating to 14.2-RELEASE-p3:
/bin/freebsd-version
$ /usr/sbin/freebsd-update install
Installing updates... done.
$ /usr/local/sbin/pkg upgrade -y
The following 2 package(s) will be affected (of 0 checked):

Installed packages to be UPGRADED:
	curl: 8.9.1 -> 8.10.0

Installed packages to be INSTALLED:
	ca_root_nss: 3.104

Number of packages to be upgraded: 1
Number of packages to be installed: 1
[1/2] Upgrading curl from 8.9.1 to 8.10.0...
[2/2] Installing ca_root_nss-3.104...`

		got := parsePackagesAffectedFromRealOutput("freebsd", output)
		want := []string{freeBSDBasePackageName, "curl", "ca_root_nss"}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromRealOutput() = %v, want %v", got, want)
		}
	})

	// Regression: apt real output contains "will be installed" lines that
	// previously falsely emitted freebsd-base on Linux hosts.
	t.Run("apt real output does not emit freebsd-base on linux", func(t *testing.T) {
		output := `Reading package lists...
Building dependency tree...
The following additional packages will be installed:
  libcap2-bin libpam-cap
The following packages will be upgraded:
  libcap2 libcap2-bin libpam-cap
Get:1 http://archive.ubuntu.com/ubuntu noble-updates/main amd64 libcap2 amd64 1:2.66-5ubuntu2.4 [14.1 kB]
Fetched 14.1 kB in 0s (42.0 kB/s)
(Reading database ... 12345 files and directories currently installed.)
Preparing to unpack .../libcap2_1%3a2.66-5ubuntu2.4_amd64.deb ...
Unpacking libcap2:amd64 (1:2.66-5ubuntu2.4) over (1:2.66-5ubuntu2.2) ...
Setting up libcap2:amd64 (1:2.66-5ubuntu2.4) ...
Unpacking libpam-cap:amd64 (1:2.66-5ubuntu2.4) over (1:2.66-5ubuntu2.2) ...
Setting up libpam-cap:amd64 (1:2.66-5ubuntu2.4) ...
Unpacking libcap2-bin (1:2.66-5ubuntu2.4) over (1:2.66-5ubuntu2.2) ...
Setting up libcap2-bin (1:2.66-5ubuntu2.4) ...`

		got := parsePackagesAffectedFromRealOutput("ubuntu", output)
		if slices.Contains(got, freeBSDBasePackageName) {
			t.Fatalf("freebsd-base must not appear in Linux real output: %v", got)
		}
		for _, want := range []string{"libcap2", "libpam-cap", "libcap2-bin"} {
			if !slices.Contains(got, want) {
				t.Fatalf("expected %q in parsed packages, got %v", want, got)
			}
		}
	})
}

func TestFreeBSDUpdateOutputHasPendingUpdates(t *testing.T) {
	t.Run("no updates", func(t *testing.T) {
		output := `No updates needed to update system to 14.2-RELEASE-p3.`
		if freeBSDUpdateOutputHasPendingUpdates(output) {
			t.Fatal("expected no pending updates")
		}
	})

	t.Run("pending updates", func(t *testing.T) {
		output := `The following files will be installed as part of updating to 14.2-RELEASE-p3:
/usr/lib/libfoo.so`
		if !freeBSDUpdateOutputHasPendingUpdates(output) {
			t.Fatal("expected pending updates")
		}
	})
}

func TestIsFreeBSD(t *testing.T) {
	cases := []struct {
		osType string
		want   bool
	}{
		{"freebsd", true},
		{"FreeBSD", true},
		{"  freebsd  ", true},
		{"ubuntu", false},
		{"debian", false},
		{"rhel", false},
		{"", false},
		{"freebsdish", false},
	}
	for _, c := range cases {
		if got := isFreeBSD(c.osType); got != c.want {
			t.Errorf("isFreeBSD(%q) = %v, want %v", c.osType, got, c.want)
		}
	}
}
