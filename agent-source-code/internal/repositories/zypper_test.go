package repositories

import (
	"os"
	"testing"
)

func TestParseZypperRepos(t *testing.T) {
	cases := []struct {
		fixture string
		count   int
		alias   string
		url     string
		name    string
	}{
		{"zypper-leap155-repos.xml", 7, "repo-sle-update", "http://download.opensuse.org/update/leap/15.5/sle/", "Update repository with updates from SUSE Linux Enterprise 15"},
		{"zypper-leap160-repos.xml", 2, "openSUSE:repo-openh264", "https://codecs.opensuse.org/openh264/openSUSE_Leap_16", "repo-openh264 (16.0)"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			b, err := os.ReadFile("testdata/" + tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			repos, err := parseZypperRepos(b)
			if err != nil {
				t.Fatal(err)
			}
			if len(repos) != tc.count {
				t.Fatalf("got %d enabled repositories, want %d: %+v", len(repos), tc.count, repos)
			}
			for _, r := range repos {
				if r.Name == tc.alias {
					if r.URL != tc.url || r.Distribution != tc.name || !r.IsEnabled || r.IsSecure != (tc.url[:5] == "https") {
						t.Errorf("got %+v", r)
					}
					return
				}
			}
			t.Errorf("%s missing", tc.alias)
		})
	}
}

func TestParseZypperReposDropsSecrets(t *testing.T) {
	xmlOut := `<?xml version='1.0'?>
<stream>
<repo-list>
<repo alias="SLE-Module-Basesystem15-SP6-Updates" name="SLE-Module-Basesystem15-SP6-Updates" type="rpm-md" priority="99" enabled="1" autorefresh="1" gpgcheck="1">
<url>https://updates.suse.com/SUSE/Updates/SLE-Module-Basesystem/15-SP6/x86_64/update?abcDEF123token</url>
</repo>
<repo alias="private" name="Private" enabled="1"><url>https://user:secret@repo.example.com/x/</url></repo>
<repo alias="off" name="Off" enabled="0"><url>https://repo.example.com/off/</url></repo>
<repo alias="cd" name="DVD" enabled="1"><url>cd:/?devices=/dev/sr0</url></repo>
</repo-list>
</stream>`
	repos, err := parseZypperRepos([]byte(xmlOut))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"SLE-Module-Basesystem15-SP6-Updates": "https://updates.suse.com/SUSE/Updates/SLE-Module-Basesystem/15-SP6/x86_64/update",
		"private":                             "https://repo.example.com/x/",
	}
	if len(repos) != len(want) {
		t.Fatalf("got %+v", repos)
	}
	for _, r := range repos {
		if want[r.Name] != r.URL {
			t.Errorf("%s: got %q, want %q", r.Name, r.URL, want[r.Name])
		}
	}
}
