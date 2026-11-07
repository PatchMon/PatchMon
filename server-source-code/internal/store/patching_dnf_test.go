package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePackagesAffectedFromDryRunOutputDnf(t *testing.T) {
	cases := []struct {
		name   string
		osType string
		output string
		want   []string
	}{
		{
			name:   "dnf5 on Fedora 43",
			osType: "Fedora Linux",
			output: `$ dnf upgrade --assumeno curl ca-certificates
Updating and loading repositories:
Repositories loaded.
Package                       Arch   Version                     Repository                            Size
Upgrading:
 ca-certificates              noarch 2026.2.90_v9.0.317-1.fc43   updates                            2.7 MiB
   replacing ca-certificates  noarch 2025.2.80_v9.0.304-1.1.fc43 ef307c8a8767464bb2d9389c6dbcba83   2.7 MiB
 curl                         x86_64 8.15.0-10.fc43              updates                          461.4 KiB
   replacing curl             x86_64 8.15.0-4.fc43               2040d210706d45f5b6026645179b0311 461.5 KiB
 openssl-libs                 x86_64 1:3.5.8-1.fc43              updates                            8.9 MiB
   replacing openssl-libs     x86_64 1:3.5.4-1.fc43              2040d210706d45f5b6026645179b0311   8.9 MiB
Installing dependencies:
 duktape                      x86_64 2.7.0-10.fc43               fedora                           615.8 KiB
 gnupg2-g13                   x86_64 2.4.9-5.fc43                updates                          199.7 KiB
   replacing gnupg2           x86_64 2.4.9-1.fc43                2040d210706d45f5b6026645179b0311   6.6 MiB
Installing weak dependencies:
 pcsc-lite-ccid               x86_64 1.7.0-2.fc43                updates                          358.6 KiB

Transaction Summary:
 Installing:        3 packages
 Upgrading:         3 packages
 Replacing:         4 packages

Total size of inbound packages is 19 MiB. Need to download 19 MiB.
Operation aborted by the user.

--- Dry run completed at 2026-10-16T17:37:24Z ---
`,
			want: []string{"ca-certificates", "curl", "openssl-libs", "duktape", "gnupg2-g13", "pcsc-lite-ccid"},
		},
		{
			name:   "dnf5 with its stderr written into the table",
			osType: "Fedora Linux",
			output: `Package                       Arch   Version                     Repository                            Size
Upgrading:
 curl                         x86_64 8.15.0-10.fc43              updates                          461.4 KiB
   replacing curl             x86_64 8.15.0-4.fc43               2040d210706d45f5b6026645179b0311 461.5 KiB
Installing dependencies:
 duktape                      x86_64 2.7.0-10.fc43               fedora                           615.8 KiB
 gnupg2-utils                 x86_64 2.4.9-5.fc43                updates                          342.7 KiB
   replacing gnupg2           x86_64 2.4.9-1.fc43               Total size of inbound packages is 23 MiB. Need to download 23 MiB.
After this operation, 48 MiB extra will be used (install 87 MiB, remove 39 MiB).
Operation aborted by the user.
 2040d210706d45f5b6026645179b0311   6.6 MiB
 gnupg2-wks                   x86_64 2.4.9-5.fc43                updates                          269.4 KiB
 python3-libs                 x86_64 3.14.0-1.fc43               fedora                            43.0 MiB
Installing weak dependencies:
 pcsc-lite-ccid               x86_64 1.7.0-2.fc43                updates                          358.6 KiB

Transaction Summary:
 Installing:        5 packages
 Upgrading:         1 packages
`,
			want: []string{"curl", "duktape", "gnupg2-utils", "gnupg2-wks", "python3-libs", "pcsc-lite-ccid"},
		},
		{
			name:   "dnf4 skipped packages are not counted",
			osType: "Rocky Linux",
			output: `Upgrading:
 curl                     x86_64    7.76.1-40.el9_8.8           baseos    293 k
Skipping packages with conflicts:
(add '--best --allowerasing' to command line to force their upgrade):
 kernel-core              x86_64    5.14.0-570.el9              baseos     17 M

Transaction Summary
`,
			want: []string{"curl"},
		},
		{
			name:   "x86_64_v2 packages on AlmaLinux 10",
			osType: "AlmaLinux",
			output: `Upgrading:
 bash                     x86_64_v2 5.2.26-6.el10_1           baseos    1.8 M
 openssl-libs             x86_64_v2 1:3.5.1-5.el10_1          baseos    2.4 M

Transaction Summary
`,
			want: []string{"bash", "openssl-libs"},
		},
		{
			name:   "dnf4 on Rocky Linux 9",
			osType: "Rocky Linux",
			output: "$ dnf upgrade --assumeno curl\r\n" +
				"Last metadata expiration check: 0:00:10 ago on Fri 16 Oct 2026 06:36:24 PM UTC.\r\n" +
				"Dependencies resolved.\r\n" +
				"================================================================================\r\n" +
				" Package                  Arch      Version                     Repo       Size\r\n" +
				"================================================================================\r\n" +
				"Upgrading:\r\n" +
				" curl                     x86_64    7.76.1-40.el9_8.8           baseos    293 k\r\n" +
				" libcurl                  x86_64    7.76.1-40.el9_8.8           baseos    284 k\r\n" +
				" openssl-libs             x86_64    1:3.5.8-2.el9_8             baseos    2.3 M\r\n" +
				"Installing dependencies:\r\n" +
				" openssl-fips-provider    x86_64    1:3.5.8-2.el9_8             baseos    813 k\r\n" +
				"\r\n" +
				"Transaction Summary\r\n" +
				"================================================================================\r\n" +
				"Install  1 Package\r\n" +
				"Upgrade  3 Packages\r\n" +
				"\r\n" +
				"Total download size: 4 M\r\n" +
				"Operation aborted.\r\n",
			want: []string{"curl", "libcurl", "openssl-libs", "openssl-fips-provider"},
		},
		{
			name:   "yum with a wrapped long package name",
			osType: "CentOS Linux",
			output: `$ yum upgrade --assumeno python-perf
Resolving Dependencies
Dependencies Resolved

================================================================================
 Package                         Arch       Version              Repository  Size
================================================================================
Updating:
 python-perf                     x86_64     3.10.0-1160.119.1.el7 updates    8.2 M
Updating for dependencies:
 kernel-tools-libs-development-headers
                                 x86_64     3.10.0-1160.119.1.el7 updates    8.1 M
Removing:
 kernel-old                      x86_64     3.10.0-1160.el7       @base       64 M

Transaction Summary
================================================================================
Upgrade  2 Packages
Remove   1 Package
Exiting on user command
`,
			want: []string{"python-perf", "kernel-tools-libs-development-headers", "kernel-old"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePackagesAffectedFromDryRunOutput(tc.osType, tc.output)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePackagesAffectedFromDryRunOutput() = %v, want %v", got, tc.want)
			}
		})
	}
}

const dnf5Fedora44Upgrade = `$ dnf -y upgrade NetworkManager
Upgrading:
 NetworkManager                   x86_64 1:1.56.1-2.fc44 updates                            6.1 MiB
   replacing NetworkManager       x86_64 1:1.56.0-1.fc44 4e4a4f70cfdd4047b39087e8606c2591   6.0 MiB
 NetworkManager-libnm             x86_64 1:1.56.1-2.fc44 updates                           10.2 MiB
   replacing NetworkManager-libnm x86_64 1:1.56.0-1.fc44 4e4a4f70cfdd4047b39087e8606c2591  10.2 MiB
 glib2                            x86_64 0:2.88.4-1.fc44 updates                           15.4 MiB
   replacing glib2                x86_64 0:2.88.0-1.fc44 4e4a4f70cfdd4047b39087e8606c2591  15.3 MiB

Transaction Summary:
 Upgrading:          3 packages
 Replacing:          3 packages

Total size of inbound packages is 7 MiB. Need to download 7 MiB.
After this operation, 131 KiB extra will be used (install 32 MiB, remove 32 MiB).
[1/3] NetworkManager-libnm-1:1.56.1-2.f 100% |   4.4 MiB/s |   1.9 MiB |  00m00s
[2/3] NetworkManager-1:1.56.1-2.fc44.x8 100% |   4.8 MiB/s |   2.3 MiB |  00m00s
[3/3] glib2-0:2.88.4-1.fc44.x86_64      100% |   4.8 MiB/s |   3.1 MiB |  00m01s
--------------------------------------------------------------------------------
[3/3] Total                             100% |   4.4 MiB/s |   7.4 MiB |  00m02s
Running transaction
[1/8] Verify package files              100% |  12.0   B/s |   3.0   B |  00m00s
[2/8] Prepare transaction               100% |  26.0   B/s |   6.0   B |  00m00s
[3/8] Upgrading glib2-0:2.88.4-1.fc44.x 100% |  20.1 MiB/s |  15.4 MiB |  00m01s
[4/8] Upgrading NetworkManager-libnm-1: 100% |  23.9 MiB/s |  10.3 MiB |  00m00s
[5/8] Upgrading NetworkManager-1:1.56.1 100% |   9.4 MiB/s |   6.1 MiB |  00m01s
[6/8] Removing NetworkManager-1:1.56.0- 100% | 369.0   B/s |  91.0   B |  00m00s
[7/8] Removing NetworkManager-libnm-1:1 100% |   1.8 KiB/s |  72.0   B |  00m00s
[8/8] Removing glib2-0:2.88.0-1.fc44.x8 100% |  67.0   B/s | 177.0   B |  00m03s
Complete!
`

func TestParsePackagesAffectedFromRealOutputDnf(t *testing.T) {
	cases := []struct {
		name   string
		osType string
		output string
		want   []string
	}{
		{
			name:   "dnf5 on Fedora 44",
			osType: "Fedora Linux",
			output: dnf5Fedora44Upgrade,
			want:   []string{"NetworkManager", "NetworkManager-libnm", "glib2"},
		},
		{
			name:   "dnf5 installing a new dependency",
			osType: "Fedora Linux",
			output: `$ dnf -y upgrade curl
Updating and loading repositories:
Repositories loaded.
Package                     Arch   Version                Repository                            Size
Upgrading:
 curl                       x86_64 8.15.0-10.fc43         updates                          461.4 KiB
   replacing curl           x86_64 8.15.0-4.fc43          2040d210706d45f5b6026645179b0311 461.5 KiB
Installing dependencies:
 duktape                    x86_64 2.7.0-10.fc43          fedora                           615.8 KiB

Transaction Summary:
 Installing:         1 package
 Upgrading:          1 package
 Replacing:          1 package

Total size of inbound packages is 1 MiB. Need to download 1 MiB.
[1/2] duktape-0:2.7.0-10.fc43.x86_64    100% |   2.0 MiB/s | 180.0 KiB |  00m00s
[2/2] curl-0:8.15.0-10.fc43.x86_64      100% |   1.9 MiB/s | 300.0 KiB |  00m00s
Running transaction
[1/5] Verify package files              100% |  50.0   B/s |   2.0   B |  00m00s
[2/5] Prepare transaction               100% |  10.0   B/s |   2.0   B |  00m00s
[3/5] Installing duktape-0:2.7.0-10.fc4 100% |  60.1 MiB/s | 616.0 KiB |  00m00s
[4/5] Upgrading curl-0:8.15.0-10.fc43.x 100% |  10.0 MiB/s | 462.0 KiB |  00m00s
[5/5] Removing curl-0:8.15.0-4.fc43.x86 100% |   5.0 KiB/s |  10.0   B |  00m00s
Complete!
`,
			want: []string{"curl", "duktape"},
		},
		{
			name:   "dnf5 stopped before the transaction ran",
			osType: "Fedora Linux",
			output: dnf5Fedora44Upgrade[:strings.Index(dnf5Fedora44Upgrade, "Running transaction")] +
				"\n--- Patch run stopped by user at 2026-11-07T10:00:00Z ---\n",
			want: nil,
		},
		{
			name:   "dnf5 stopped during the transaction",
			osType: "Fedora Linux",
			output: dnf5Fedora44Upgrade[:strings.Index(dnf5Fedora44Upgrade, "[5/8]")] +
				"\n--- Patch run stopped by user at 2026-11-07T10:00:00Z ---\n",
			want: []string{"NetworkManager-libnm", "glib2"},
		},
		{
			name:   "dnf completed without matchable execution lines",
			osType: "Fedora Linux",
			output: `Upgrading:
 glib2                            x86_64 0:2.88.4-1.fc44 updates                           15.4 MiB

Running transaction
[3/4] Upgrading a-name-cut-before-it-ends 100% |  20.1 MiB/s |  15.4 MiB |  00m01s
Complete!
`,
			want: []string{"glib2"},
		},
		{
			name:   "dnf4 stopped during the transaction",
			osType: "Rocky Linux",
			output: `$ dnf upgrade -y
Upgrading:
 bash                x86_64    5.1.8-9.el9             baseos        1.7 M
 kernel-core         x86_64    5.14.0-611.el9          baseos         18 M
 openssl             x86_64    1:3.5.1-4.el9           baseos        1.2 M

Transaction Summary
Upgrade  3 Packages

Running transaction
  Preparing        :                                                        1/1
  Upgrading        : bash-5.1.8-9.el9.x86_64                                1/6

--- Patch run stopped by user at 2026-11-07T10:00:00Z ---
`,
			want: []string{"bash"},
		},
		{
			name:   "dnf4 prefers the table and skips removals",
			osType: "Rocky Linux",
			output: "$ dnf upgrade -y\r\n" +
				"Dependencies resolved.\r\n" +
				"================================================================================\r\n" +
				" Package             Arch      Version                 Repo           Size\r\n" +
				"================================================================================\r\n" +
				"Installing:\r\n" +
				" kernel              x86_64    5.14.0-611.el9          baseos        2.0 M\r\n" +
				"Upgrading:\r\n" +
				" python3.11-libs     x86_64    3.11.13-2.el9           appstream      10 M\r\n" +
				"Removing:\r\n" +
				" kernel              x86_64    5.14.0-503.el9          @baseos         0  \r\n" +
				" kernel-core         x86_64    5.14.0-503.el9          @baseos        66 M\r\n" +
				"\r\n" +
				"Transaction Summary\r\n" +
				"================================================================================\r\n" +
				"Install  1 Package\r\n" +
				"Upgrade  1 Package\r\n" +
				"Remove   2 Packages\r\n" +
				"\r\n" +
				"Running transaction\r\n" +
				"  Preparing        :                                                        1/1 \r\n" +
				"  Installing       : kernel-5.14.0-611.el9.x86_64                           1/5 \r\n" +
				"  Upgrading        : python3.11-libs-3.11.13-2.el9.x86_64                   2/5 \r\n" +
				"  Erasing          : kernel-core-5.14.0-503.el9.x86_64                      3/5 \r\n" +
				"Complete!\r\n",
			want: []string{"kernel", "python3.11-libs"},
		},
		{
			name:   "dnf4 execution lines without a table",
			osType: "Rocky Linux",
			output: `  Upgrading   : pkg-1.0-1.el9.x86_64   1/4
  Upgrading   : libfoo-devel-2.3-4.el9.x86_64   2/4
  Cleanup     : pkg-0.9-1.el9.x86_64   3/4
`,
			want: []string{"pkg", "libfoo-devel"},
		},
		{
			name:   "dnf removal-only transaction",
			osType: "Fedora Linux",
			output: `$ dnf -y upgrade
Removing:
 kernel-core                x86_64 6.17.1-300.fc43        @System                           66.0 MiB

Transaction Summary:
 Removing:           1 package

Running transaction
[1/2] Prepare transaction               100% |  10.0   B/s |   1.0   B |  00m00s
[2/2] Removing kernel-core-0:6.17.1-300 100% |   5.0 KiB/s | 10.0   B |  00m00s
Complete!
`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePackagesAffectedFromRealOutput(tc.osType, tc.output)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePackagesAffectedFromRealOutput() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParsePackagesAffectedDnf5RealMatchesDryRun(t *testing.T) {
	dry := parsePackagesAffectedFromDryRunOutput("Fedora Linux", dnf5Fedora44Upgrade)
	realRun := parsePackagesAffectedFromRealOutput("Fedora Linux", dnf5Fedora44Upgrade)
	if !reflect.DeepEqual(dry, realRun) {
		t.Fatalf("dry run = %q, real run = %q", dry, realRun)
	}
}

func TestParsePackagesAffectedPfSenseActionLines(t *testing.T) {
	got := parsePackagesAffectedFromRealOutput("pfSense", "[1/1] Upgrading curl from 8.9.1 to 8.10.0...\n")
	if want := []string{"curl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePackagesAffectedFromRealOutput() = %q, want %q", got, want)
	}
}
