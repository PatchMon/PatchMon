package store

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
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

func TestParseHHMM(t *testing.T) {
	cases := []struct {
		in      string
		wantH   int
		wantM   int
		wantS   int
		wantErr bool
	}{
		{"15:00", 15, 0, 0, false},
		{"01:30:45", 1, 30, 45, false},
		{" 09:05 ", 9, 5, 0, false},
		{"00:00", 0, 0, 0, false},
		{"23:59:59", 23, 59, 59, false},
		{"", 0, 0, 0, true},
		{"abc", 0, 0, 0, true},
		{"25:00", 0, 0, 0, true},
		{"12:60", 0, 0, 0, true},
		{"12:30:60", 0, 0, 0, true},
		{"12", 0, 0, 0, true},
		{"1:2:3:4", 0, 0, 0, true},
		{"-1:00", 0, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			h, m, s, err := ParseHHMM(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseHHMM(%q) = (%d,%d,%d, nil), want error", c.in, h, m, s)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHHMM(%q) unexpected error: %v", c.in, err)
			}
			if h != c.wantH || m != c.wantM || s != c.wantS {
				t.Errorf("ParseHHMM(%q) = (%d,%d,%d), want (%d,%d,%d)", c.in, h, m, s, c.wantH, c.wantM, c.wantS)
			}
		})
	}
}

func TestNextFixedWallClockUTC(t *testing.T) {
	mustLoad := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("LoadLocation(%q): %v", name, err)
		}
		return loc
	}
	berlin := mustLoad("Europe/Berlin")

	cases := []struct {
		name    string
		now     time.Time
		hhmm    string
		loc     *time.Location
		wantUTC time.Time
		wantErr bool
	}{
		{
			name:    "same-day-future-utc",
			now:     time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			hhmm:    "15:00",
			loc:     time.UTC,
			wantUTC: time.Date(2025, 6, 1, 15, 0, 0, 0, time.UTC),
		},
		{
			name:    "same-day-past-rolls-tomorrow",
			now:     time.Date(2025, 6, 1, 18, 0, 0, 0, time.UTC),
			hhmm:    "15:00",
			loc:     time.UTC,
			wantUTC: time.Date(2025, 6, 2, 15, 0, 0, 0, time.UTC),
		},
		{
			// Berlin in summer is UTC+2 (CEST). 14:00 Berlin = 12:00 UTC.
			name:    "berlin-summer-future",
			now:     time.Date(2025, 7, 15, 10, 0, 0, 0, time.UTC),
			hhmm:    "14:00",
			loc:     berlin,
			wantUTC: time.Date(2025, 7, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			// Berlin in winter is UTC+1 (CET). It is 23:00 UTC = 00:00 Berlin
			// next day, so policy 01:00 Berlin should fire at 00:00 UTC the
			// same Berlin day (which is 16 Jan in UTC because the local day
			// already rolled over).
			name:    "berlin-winter-rollover",
			now:     time.Date(2025, 1, 15, 23, 0, 0, 0, time.UTC),
			hhmm:    "01:00",
			loc:     berlin,
			wantUTC: time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			// Spring-forward: 02:30 local does not exist on the transition
			// day in Europe/Berlin (clock jumps 02:00 -> 03:00). Go's
			// time.Date normalises forward, so 02:30 local becomes 03:30
			// local, which is 01:30 UTC.
			name:    "dst-spring-forward-gap",
			now:     time.Date(2025, 3, 30, 0, 30, 0, 0, time.UTC),
			hhmm:    "02:30",
			loc:     berlin,
			wantUTC: time.Date(2025, 3, 30, 1, 30, 0, 0, time.UTC),
		},
		{
			// Fall-back: 02:30 local occurs twice on the transition day in
			// Europe/Berlin. Go's time.Date selects the standard-time (CET,
			// post-shift) occurrence at 01:30 UTC.
			name:    "dst-fall-back-overlap",
			now:     time.Date(2025, 10, 26, 0, 0, 0, 0, time.UTC),
			hhmm:    "02:30",
			loc:     berlin,
			wantUTC: time.Date(2025, 10, 26, 1, 30, 0, 0, time.UTC),
		},
		{
			name:    "hh-mm-ss-format",
			now:     time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			hhmm:    "15:00:30",
			loc:     time.UTC,
			wantUTC: time.Date(2025, 6, 1, 15, 0, 30, 0, time.UTC),
		},
		{
			name:    "nil-loc-falls-back-to-utc",
			now:     time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			hhmm:    "15:00",
			loc:     nil,
			wantUTC: time.Date(2025, 6, 1, 15, 0, 0, 0, time.UTC),
		},
		{name: "invalid-malformed", now: time.Now(), hhmm: "abc", loc: time.UTC, wantErr: true},
		{name: "invalid-out-of-range-hour", now: time.Now(), hhmm: "25:00", loc: time.UTC, wantErr: true},
		{name: "invalid-empty", now: time.Now(), hhmm: "", loc: time.UTC, wantErr: true},
		{name: "invalid-too-many-parts", now: time.Now(), hhmm: "1:2:3:4", loc: time.UTC, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := nextFixedWallClockUTC(c.now, c.hhmm, c.loc)
			if c.wantErr {
				if err == nil {
					t.Fatalf("nextFixedWallClockUTC: want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("nextFixedWallClockUTC: unexpected error: %v", err)
			}
			if !got.Equal(c.wantUTC) {
				t.Errorf("nextFixedWallClockUTC: got %s, want %s", got.UTC().Format(time.RFC3339), c.wantUTC.Format(time.RFC3339))
			}
			if !got.After(c.now) && !got.Equal(c.now) {
				t.Errorf("nextFixedWallClockUTC: got %s should be >= now %s", got.Format(time.RFC3339), c.now.Format(time.RFC3339))
			}
		})
	}
}

func TestComputeRunAtWithMeta(t *testing.T) {
	st := &PatchPoliciesStore{}

	t.Run("nil-policy-returns-immediate", func(t *testing.T) {
		got, meta := st.ComputeRunAtWithMeta(nil, "Europe/Berlin")
		if time.Since(got) > time.Second {
			t.Errorf("nil policy should return now-ish, got %s", got)
		}
		if meta.Source != "n/a" {
			t.Errorf("nil policy meta.Source = %q, want n/a", meta.Source)
		}
	})

	t.Run("immediate-policy-ignores-tz", func(t *testing.T) {
		policy := &db.PatchPolicy{PatchDelayType: "immediate"}
		_, meta := st.ComputeRunAtWithMeta(policy, "Europe/Berlin")
		if meta.Source != "n/a" {
			t.Errorf("immediate policy meta.Source = %q, want n/a", meta.Source)
		}
	})

	t.Run("delayed-policy-adds-minutes", func(t *testing.T) {
		mins := int32(10)
		policy := &db.PatchPolicy{PatchDelayType: "delayed", DelayMinutes: &mins}
		got, meta := st.ComputeRunAtWithMeta(policy, "Europe/Berlin")
		if meta.Source != "n/a" {
			t.Errorf("delayed policy meta.Source = %q, want n/a", meta.Source)
		}
		// Allow a generous skew (CI scheduling jitter).
		if got.Before(time.Now().Add(9*time.Minute)) || got.After(time.Now().Add(11*time.Minute)) {
			t.Errorf("delayed policy returned %s, expected ~10 minutes from now", got)
		}
	})

	t.Run("fixed-time-uses-org-tz", func(t *testing.T) {
		hhmm := "02:00"
		policy := &db.PatchPolicy{PatchDelayType: "fixed_time", FixedTimeUtc: &hhmm}
		got, meta := st.ComputeRunAtWithMeta(policy, "Europe/Berlin")
		if meta.Source != "org" {
			t.Errorf("fixed_time policy meta.Source = %q, want org", meta.Source)
		}
		if meta.Timezone != "Europe/Berlin" {
			t.Errorf("fixed_time policy meta.Timezone = %q, want Europe/Berlin", meta.Timezone)
		}
		if !got.After(time.Now()) {
			t.Errorf("fixed_time policy result %s should be in the future", got)
		}
		if time.Until(got) > 25*time.Hour {
			t.Errorf("fixed_time policy result %s too far in the future", got)
		}
	})

	t.Run("fixed-time-invalid-org-tz-falls-back-to-utc", func(t *testing.T) {
		hhmm := "02:00"
		policy := &db.PatchPolicy{PatchDelayType: "fixed_time", FixedTimeUtc: &hhmm}
		_, meta := st.ComputeRunAtWithMeta(policy, "Not/A/Real/Zone")
		if meta.Source != "utc-fallback" {
			t.Errorf("invalid org tz meta.Source = %q, want utc-fallback", meta.Source)
		}
		if meta.Timezone != "UTC" {
			t.Errorf("invalid org tz meta.Timezone = %q, want UTC", meta.Timezone)
		}
	})

	t.Run("fixed-time-empty-org-tz-falls-back-to-utc", func(t *testing.T) {
		hhmm := "02:00"
		policy := &db.PatchPolicy{PatchDelayType: "fixed_time", FixedTimeUtc: &hhmm}
		_, meta := st.ComputeRunAtWithMeta(policy, "")
		if meta.Source != "utc-fallback" {
			t.Errorf("empty org tz meta.Source = %q, want utc-fallback", meta.Source)
		}
	})

	t.Run("fixed-time-malformed-returns-immediate", func(t *testing.T) {
		hhmm := "abc"
		policy := &db.PatchPolicy{PatchDelayType: "fixed_time", FixedTimeUtc: &hhmm}
		got, meta := st.ComputeRunAtWithMeta(policy, "Europe/Berlin")
		if meta.Err == nil {
			t.Errorf("malformed fixed_time should set meta.Err")
		}
		if time.Since(got) > time.Second {
			t.Errorf("malformed fixed_time should fall back to now-ish, got %s", got)
		}
	})

	t.Run("policy-timezone-column-is-ignored", func(t *testing.T) {
		hhmm := "02:00"
		nyc := "America/New_York"
		policy := &db.PatchPolicy{
			PatchDelayType: "fixed_time",
			FixedTimeUtc:   &hhmm,
			Timezone:       &nyc, // legacy column - must NOT win over org tz
		}
		_, meta := st.ComputeRunAtWithMeta(policy, "Europe/Berlin")
		if meta.Timezone != "Europe/Berlin" {
			t.Errorf("legacy policy.Timezone leaked into scheduler; meta.Timezone = %q, want Europe/Berlin", meta.Timezone)
		}
	})
}
