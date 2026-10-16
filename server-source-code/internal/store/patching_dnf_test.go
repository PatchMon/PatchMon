package store

import (
	"reflect"
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
