package store

import (
	"reflect"
	"testing"
)

func TestParsePackagesAffectedFromDryRunOutput(t *testing.T) {
	t.Run("parses apt simulate output", func(t *testing.T) {
		output := `Inst openssl [3.0.2] (3.0.3 Debian:stable)
Inst curl [7.88.1] (8.0.0 Debian:stable)`

		got := parsePackagesAffectedFromDryRunOutput(output)
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

		got := parsePackagesAffectedFromDryRunOutput(output)
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

		got := parsePackagesAffectedFromDryRunOutput(output)
		want := []string{freeBSDBasePackageName}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, want)
		}
	})
}

func TestParsePackagesAffectedFromRealOutput(t *testing.T) {
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

	got := parsePackagesAffectedFromRealOutput(output)
	want := []string{freeBSDBasePackageName, "curl", "ca_root_nss"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePackagesAffectedFromRealOutput() = %v, want %v", got, want)
	}
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
