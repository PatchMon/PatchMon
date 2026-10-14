package repositories

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestParseAPKRepositories(t *testing.T) {
	content := `# comment
https://dl-cdn.alpinelinux.org/alpine/v3.21/main
https://dl-cdn.alpinelinux.org/alpine/v3.21/community/

@edge https://dl-cdn.alpinelinux.org/alpine/edge/testing
/media/cdrom/apks
set mirror=https://mirror.example.com/alpine
set -default mirror=https://ignored.example.com
v3 ${mirror}/v3.23 main community
v2 @local https://packages.example.com/apk
ndx https://repo.example.com/x86_64/APKINDEX.tar.gz
`
	got := parseAPKRepositories(content, map[string]string{})
	want := []APKRepoLine{
		{URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/main"},
		{URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/community"},
		{Tag: "@edge", URL: "https://dl-cdn.alpinelinux.org/alpine/edge/testing"},
		{URL: "/media/cdrom/apks"},
		{URL: "https://mirror.example.com/alpine/v3.23/main"},
		{URL: "https://mirror.example.com/alpine/v3.23/community"},
		{Tag: "@local", URL: "https://packages.example.com/apk"},
		{URL: "https://repo.example.com/x86_64/APKINDEX.tar.gz"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseAPKRepositories =\n%+v\nwant\n%+v", got, want)
	}
}

func TestAPKRepository(t *testing.T) {
	cases := []struct {
		line                   APKRepoLine
		name, dist, comp       string
		secure, skippedAsLocal bool
	}{
		{APKRepoLine{URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/main"}, "v3.21-main", "v3.21", "main", true, false},
		{APKRepoLine{URL: "http://dl-cdn.alpinelinux.org/alpine/edge/community"}, "edge-community", "edge", "community", false, false},
		{APKRepoLine{Tag: "@edge", URL: "https://dl-cdn.alpinelinux.org/alpine/edge/testing"}, "edge-edge-testing", "edge", "testing", true, false},
		{APKRepoLine{URL: "https://mirror.example.com/v3.22/main"}, "v3.22-main", "v3.22", "main", true, false},
		{APKRepoLine{URL: "https://mirrors.example.org/pub/linux/alpine/latest-stable/main"}, "latest-stable-main", "latest-stable", "main", true, false},
		{APKRepoLine{URL: "https://packages.example.com/apk/extras"}, "apk-extras", "apk", "extras", true, false},
		{APKRepoLine{URL: "https://packages.example.com/repo"}, "repo", "repo", "", true, false},
		{APKRepoLine{URL: "https://repo.example.com/alpine/v3.23/main/x86_64/APKINDEX.tar.gz"}, "v3.23-main", "v3.23", "main", true, false},
		{APKRepoLine{URL: "/media/cdrom/apks"}, "", "", "", false, true},
		{APKRepoLine{URL: "file:///srv/apks"}, "", "", "", false, true},
	}
	for _, tc := range cases {
		repo := apkRepository(tc.line)
		if tc.skippedAsLocal {
			if repo != nil {
				t.Errorf("%s should be skipped, got %+v", tc.line.URL, repo)
			}
			continue
		}
		if repo == nil {
			t.Errorf("%s was dropped", tc.line.URL)
			continue
		}
		if repo.Name != tc.name || repo.Distribution != tc.dist || repo.Components != tc.comp || repo.IsSecure != tc.secure || repo.RepoType != "apk" {
			t.Errorf("%s = %+v", tc.line.URL, repo)
		}
	}
}

func TestAPKBranchAndComponent(t *testing.T) {
	cases := []struct{ url, branch, comp string }{
		{"https://dl-cdn.alpinelinux.org/alpine/v3.24/community", "v3.24", "community"},
		{"https://mirror.example.com/v3.21/main/", "v3.21", "main"},
		{"https://dl-cdn.alpinelinux.org/alpine/edge/main", "edge", "main"},
		{"https://packages.example.com/apk", "", ""},
	}
	for _, tc := range cases {
		b, c := APKBranchAndComponent(tc.url)
		if b != tc.branch || c != tc.comp {
			t.Errorf("%s = %q %q", tc.url, b, c)
		}
	}
}

func TestReadAPKRepositoryLinesIncludesListDirs(t *testing.T) {
	dir := t.TempDir()
	etcD := filepath.Join(dir, "etc.d")
	libD := filepath.Join(dir, "lib.d")
	for _, d := range []string{etcD, libD} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "repositories"), "https://dl-cdn.alpinelinux.org/alpine/v3.23/main\n")
	write(filepath.Join(libD, "distribution.list"), "set -default distro_mirror=https://dl-cdn.alpinelinux.org/alpine\nv3 ${distro_mirror}/v3.23 community\n")
	write(filepath.Join(etcD, "00-vars.list"), "set distro_mirror=https://mirror.example.com/alpine\n")
	write(filepath.Join(etcD, "shadowed.list"), "https://etc.example.com/v3.23/main\n")
	write(filepath.Join(libD, "shadowed.list"), "https://lib.example.com/v3.23/main\n")

	origFile, origDirs := apkRepositoriesFile, apkRepositoriesDirs
	apkRepositoriesFile, apkRepositoriesDirs = filepath.Join(dir, "repositories"), []string{etcD, libD}
	defer func() { apkRepositoriesFile, apkRepositoriesDirs = origFile, origDirs }()

	var urls []string
	for _, l := range ReadAPKRepositoryLines() {
		urls = append(urls, l.URL)
	}
	want := []string{
		"https://dl-cdn.alpinelinux.org/alpine/v3.23/main",
		"https://mirror.example.com/alpine/v3.23/community",
		"https://etc.example.com/v3.23/main",
	}
	if !reflect.DeepEqual(urls, want) {
		t.Errorf("urls = %q, want %q", urls, want)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	repos, err := NewAPKManager(logger).GetRepositories()
	if err != nil || len(repos) != 3 {
		t.Fatalf("GetRepositories = %+v, %v", repos, err)
	}
}
