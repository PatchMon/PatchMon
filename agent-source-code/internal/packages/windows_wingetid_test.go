package packages

import (
	"io"
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

func TestWingetEntriesToRowsCarriesID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want string
	}{
		{"plain", "7zip.7zip", "7zip.7zip"},
		{"plus signs", "Notepad++.Notepad++", "Notepad++.Notepad++"},
		{"msstore", "9NBLGGH4NNS1", "9NBLGGH4NNS1"},
		{"truncated by winget", "Microsoft.VCRedist.2015+.x6…", ""},
		{"quote", "x';calc;'", ""},
		{"leading dash", "-x", ""},
		{"space", "Foo Bar", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := wingetEntriesToRows([]wingetEntry{{Name: "App", ID: tc.id, Version: "1.0", Available: "2.0"}}, nil)
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1", len(rows))
			}
			if got := rows[0].pkg.WingetID; got != tc.want {
				t.Fatalf("WingetID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergeRegistryAndWingetCopiesWingetID(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)

	reg := []models.Package{
		{Name: "7-Zip 24.08 (x64)", CurrentVersion: "24.08"},
		{Name: "Legacy Tool", CurrentVersion: "1.0"},
	}
	rows := []wingetRow{
		{pkg: models.Package{Name: "7-Zip 24.08 (x64)", CurrentVersion: "24.08", AvailableVersion: "24.09", NeedsUpdate: true, SourceRepository: "winget", WingetID: "7zip.7zip"}},
		{pkg: models.Package{Name: "Notepad++", CurrentVersion: "8.6", AvailableVersion: "8.7", NeedsUpdate: true, SourceRepository: "winget", WingetID: "Notepad++.Notepad++"}},
	}

	merged, _ := m.mergeRegistryAndWinget(reg, rows)
	byName := map[string]models.Package{}
	for _, p := range merged {
		byName[p.Name] = p
	}
	if got := byName["7-Zip 24.08 (x64)"].WingetID; got != "7zip.7zip" {
		t.Errorf("matched registry row WingetID = %q, want 7zip.7zip", got)
	}
	if got := byName["Legacy Tool"].WingetID; got != "" {
		t.Errorf("registry-only row WingetID = %q, want empty", got)
	}
	if got := byName["Notepad++"].WingetID; got != "Notepad++.Notepad++" {
		t.Errorf("winget-only row WingetID = %q, want Notepad++.Notepad++", got)
	}
}
