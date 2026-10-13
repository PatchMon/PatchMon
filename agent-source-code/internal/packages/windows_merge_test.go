package packages

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

type wingetTestRow struct{ name, id, version, available, source string }

func wingetTable(nameWidth int, rows []wingetTestRow) string {
	const versionWidth, availWidth = 16, 16
	idWidth := 4
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.id) + 1; n > idWidth {
			idWidth = n
		}
	}
	pad := func(s string, width int) string {
		if n := utf8.RuneCountInString(s); n < width {
			return s + strings.Repeat(" ", width-n)
		}
		return s
	}
	var b strings.Builder
	b.WriteString(pad("Name", nameWidth) + pad("Id", idWidth) + pad("Version", versionWidth) + pad("Available", availWidth) + "Source\n")
	b.WriteString(strings.Repeat("-", nameWidth+idWidth+versionWidth+availWidth+6) + "\n")
	for _, r := range rows {
		b.WriteString(pad(r.name, nameWidth) + pad(r.id, idWidth) + pad(r.version, versionWidth) + pad(r.available, availWidth) + r.source + "\n")
	}
	return b.String()
}

func collectWindowsApps(t *testing.T, reg []models.Package, table string, upgradeMap map[string]string) []models.Package {
	t.Helper()
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)
	entries := m.parseWingetTable(table)
	if len(entries) == 0 {
		t.Fatal("fixture produced no winget entries")
	}
	return m.mergeRegistryAndWinget(reg, wingetEntriesToPackages(entries, upgradeMap))
}

func findPackage(t *testing.T, pkgs []models.Package, name string) models.Package {
	t.Helper()
	for _, p := range pkgs {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("package %q missing from merged output: %+v", name, pkgs)
	return models.Package{}
}

// Rows and registry values from issue #1108 (SQL Server 2025 with CU 5005).
func TestSQLServerUnknownVersionIsNotFlagged(t *testing.T) {
	const sqlName = "Microsoft SQL Server 2025 (64-bit)"
	reg := []models.Package{
		{Name: sqlName, CurrentVersion: "unknown"},
		{Name: "Microsoft SQL Server 2025 Setup (English)", CurrentVersion: "17.0.5005.3"},
		{Name: "Enregistreur VSS Microsoft pour SQL Server 2025", CurrentVersion: "17.0.1000.7"},
	}
	table := wingetTable(66, []wingetTestRow{
		{"Correctif logiciel 5005 pour SQL Server 2025 (KB5122048) (64-bit)", `ARP\Machine\X64\KB5122048`, "17.0.5005.3", "", ""},
		{"Enregistreur VSS Microsoft pour SQL Server 2025", `ARP\Machine\X64\{80F7288C-911D-4D81-B493-4D50D4ACF597}`, "17.0.1000.7", "", ""},
		{sqlName, "Microsoft.SQLServer.2025.Developer", "Unknown", "17.0.1000.7", "winget"},
		{"Microsoft SQL Server 2025 Setup (English)", `ARP\Machine\X64\{3A2FB62B-B8E5-4150-9C13-B1DED13E5EE3}`, "17.0.5005.3", "", ""},
	})

	merged := collectWindowsApps(t, reg, table, map[string]string{})

	for _, p := range merged {
		if p.NeedsUpdate {
			t.Errorf("%q flagged for update (installed %q, available %q); winget --upgrade-available reported nothing", p.Name, p.CurrentVersion, p.AvailableVersion)
		}
	}
	if got := findPackage(t, merged, sqlName); got.AvailableVersion != "" {
		t.Errorf("unknown installed version carries available version %q", got.AvailableVersion)
	}
}

func TestSQLServerVersionFromSetupPatchLevel(t *testing.T) {
	setups := parseSQLServerSetups([]byte(`{"Instance":"MSSQL17.MSSQLSERVER","Version":"17.0.1000.7","PatchLevel":"17.0.5005.3"}`))
	if len(setups) != 1 {
		t.Fatalf("single PowerShell object parsed into %d setups", len(setups))
	}

	pkgs := []models.Package{
		{Name: "Microsoft SQL Server 2025 (64-bit)", CurrentVersion: "unknown"},
		{Name: "Microsoft SQL Server 2025 Setup (English)", CurrentVersion: "17.0.5005.3"},
		{Name: "Microsoft SQL Server 2022 (64-bit)", CurrentVersion: "unknown"},
	}
	if !hasUnversionedSQLServer(pkgs) {
		t.Fatal("hasUnversionedSQLServer = false for an unversioned SQL Server product entry")
	}
	applySQLServerPatchLevels(pkgs, setups)

	if got := pkgs[0].CurrentVersion; got != "17.0.5005.3" {
		t.Errorf("SQL Server 2025 version = %q, want PatchLevel 17.0.5005.3", got)
	}
	if got := pkgs[2].CurrentVersion; got != "unknown" {
		t.Errorf("SQL Server 2022 with no MSSQL16 instance = %q, want unknown", got)
	}

	table := wingetTable(66, []wingetTestRow{
		{"Microsoft SQL Server 2025 (64-bit)", "Microsoft.SQLServer.2025.Developer", "Unknown", "17.0.1000.7", "winget"},
	})
	merged := collectWindowsApps(t, pkgs[:1], table, map[string]string{})
	if got := findPackage(t, merged, "Microsoft SQL Server 2025 (64-bit)"); got.NeedsUpdate || got.CurrentVersion != "17.0.5005.3" {
		t.Errorf("merged SQL Server = %+v, want 17.0.5005.3 with no update", got)
	}
}

func TestSQLServerLowestInstancePatchLevelWins(t *testing.T) {
	pkgs := []models.Package{{Name: "Microsoft SQL Server 2022 (64-bit)", CurrentVersion: "unknown"}}
	applySQLServerPatchLevels(pkgs, []sqlServerSetup{
		{Instance: "MSSQL16.MSSQLSERVER", Version: "16.0.1000.6", PatchLevel: "16.0.4185.3"},
		{Instance: "MSSQL16.SQLEXPRESS", Version: "16.0.1000.6", PatchLevel: "16.0.4135.4"},
		{Instance: "MSSQL17.MSSQLSERVER", Version: "17.0.1000.7", PatchLevel: "17.0.5005.3"},
		{Instance: "MSAS16.MSSQLSERVER", Version: "16.0.43.1", PatchLevel: "16.0.1.1"},
	})
	if got := pkgs[0].CurrentVersion; got != "16.0.4135.4" {
		t.Errorf("version = %q, want the lowest MSSQL16 patch level 16.0.4135.4", got)
	}
}

func TestParseSQLServerSetups(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"null", 0},
		{"\xef\xbb\xbf" + `{"Instance":"MSSQL17.MSSQLSERVER","Version":"17.0.1000.7","PatchLevel":"17.0.5005.3"}`, 1},
		{`[{"Instance":"MSSQL16.A","PatchLevel":"16.0.4185.3"},{"Instance":"MSSQL16.B","PatchLevel":"16.0.4135.4"}]`, 2},
		{"not json", 0},
	}
	for _, tc := range cases {
		if got := len(parseSQLServerSetups([]byte(tc.in))); got != tc.want {
			t.Errorf("parseSQLServerSetups(%q) returned %d setups, want %d", tc.in, got, tc.want)
		}
	}
}
