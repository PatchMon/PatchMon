package textsan

import (
	"testing"

	"patchmon-agent/pkg/models"
)

func TestClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"clean ascii", "Mesh Agent", "Mesh Agent"},
		{"accents preserved", "Mise à jour de la sélection", "Mise à jour de la sélection"},
		{"trailing nuls", "2026-02-16 01:43:44.000+01:00\x00\x00", "2026-02-16 01:43:44.000+01:00"},
		{"embedded nul", "Microsoft\x00", "Microsoft"},
		{"invalid utf8 repaired", "caf\xe9", "caf�"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Clean(tc.in); got != tc.want {
				t.Fatalf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanPackages(t *testing.T) {
	pkgs := CleanPackages([]models.Package{{
		Name:             "Mesh Agent\x00",
		Description:      "3,3 MB",
		CurrentVersion:   "2026-02-16 01:43:44.000+01:00\x00\x00",
		AvailableVersion: "1.2.3\x00",
		WUACategories:    []string{"Security Updates\x00"},
	}})
	p := pkgs[0]
	if p.Name != "Mesh Agent" {
		t.Fatalf("name = %q", p.Name)
	}
	if p.CurrentVersion != "2026-02-16 01:43:44.000+01:00" {
		t.Fatalf("current version = %q", p.CurrentVersion)
	}
	if p.AvailableVersion != "1.2.3" {
		t.Fatalf("available version = %q", p.AvailableVersion)
	}
	if p.WUACategories[0] != "Security Updates" {
		t.Fatalf("wua category = %q", p.WUACategories[0])
	}
}

func TestCleanRepositories(t *testing.T) {
	repos := CleanRepositories([]models.Repository{{
		Name: "Microsoft Update\x00",
		URL:  "https://update.microsoft.com\x00",
	}})
	if repos[0].Name != "Microsoft Update" || repos[0].URL != "https://update.microsoft.com" {
		t.Fatalf("repository not cleaned: %+v", repos[0])
	}
}
