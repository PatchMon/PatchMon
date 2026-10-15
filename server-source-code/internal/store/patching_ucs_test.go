package store

import (
	"reflect"
	"testing"
)

const univentionUpgradeOutput = `$ apt-get update -qq
$ univention-upgrade --noninteractive --disable-app-updates --updateto 5.2-7

Starting univention-upgrade. Current UCS version is 5.2-7 errata606

Checking for local repository:                          none
Checking for package updates:                           found

The following packages will be installed:
 apparmor,linux-image-6.1.0-53-amd64
The following packages will be upgraded:
 libperl5.36,perl,python3.11,univention-updater,linux-image-amd64
Starting package upgrade                                done

Starting univention-upgrade. Current UCS version is 5.2-7 errata638

Checking for package updates:                           none
Checking for app updates:                               skipped
Checking for release updates:                           none
Setting update/available
`

func TestParsePackagesAffectedUnivention(t *testing.T) {
	got := parsePackagesAffectedFromRealOutput("Univention Corporate Server", univentionUpgradeOutput)
	want := []string{"apparmor", "linux-image-6.1.0-53-amd64", "libperl5.36", "perl", "python3.11", "univention-updater", "linux-image-amd64"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePackagesAffectedFromRealOutput() = %v, want %v", got, want)
	}
}

func TestParsePackagesAffectedIgnoresUniventionListsElsewhere(t *testing.T) {
	if got := parsePackagesAffectedFromRealOutput("Debian GNU/Linux", univentionUpgradeOutput); len(got) != 0 {
		t.Fatalf("non-UCS host parsed %v", got)
	}
}

func TestParsePackagesAffectedAptOnUCS(t *testing.T) {
	output := `$ apt-get update -qq
$ apt-get -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold --only-upgrade install -y curl libcurl4
The following packages will be upgraded:
  curl libcurl4
2 upgraded, 0 newly installed, 0 to remove and 170 not upgraded.
Unpacking curl (7.64.0-4+deb10u10) over (7.64.0-4+deb10u9) ...
Unpacking libcurl4:amd64 (7.64.0-4+deb10u10) over (7.64.0-4+deb10u9) ...
Setting up libcurl4:amd64 (7.64.0-4+deb10u10) ...
Setting up curl (7.64.0-4+deb10u10) ...
`
	got := parsePackagesAffectedFromRealOutput("Univention Corporate Server", output)
	want := []string{"curl", "libcurl4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePackagesAffectedFromRealOutput() = %v, want %v", got, want)
	}
}
