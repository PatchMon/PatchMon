package packages

import (
	"io"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestStripTerminalControls(t *testing.T) {
	cases := map[string]string{
		"\x1b[31mred\x1b[0m":                "red",
		"   - \r   \\ \r   | \rFound 7-Zip": "Found 7-Zip",
		"\x1b]0;title\x07after":             "after",
		"tab\tkept":                         "tab\tkept",
		"bell\x07gone\x00":                  "bellgone",
		"line1\nline2":                      "line1\nline2",
		"  ██████████████  1.00 MB / 2.00 MB": "",
		"-\n\\\n|\n/": "",
		"crlf\r\n":    "crlf\n",
	}
	for in, want := range cases {
		if got := stripTerminalControls(in); got != want {
			t.Errorf("stripTerminalControls(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWingetListOutputParsesAfterProgressAndSpinner(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)
	raw := "   - \r   \\ \r\x1b[2K  ███▒▒  1024 KB / 8.30 MB\r\n" +
		"Name               Id                  Version  Available Source\r\n" +
		"-----------------------------------------------------------------\r\n" +
		"7-Zip 24.08 (x64)  7zip.7zip           24.08    24.09     winget\r\n" +
		"Notepad++          Notepad++.Notepad++ 8.6      8.7       winget\r\n"
	entries := m.parseWingetTable(stripTerminalControls(raw))
	if len(entries) != 2 || entries[0].ID != "7zip.7zip" || entries[1].ID != "Notepad++.Notepad++" || entries[1].Available != "8.7" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestClassifyWingetExit(t *testing.T) {
	table := "   - \r   \\ \rName   Id         Version\r\n-----------------------------\r\n7-Zip  7zip.7zip  24.08\r\n"
	noApps := uint32(wingetNoApplicationsFound)
	dllNotFound := uint32(0xC0000135)
	cases := []struct {
		name     string
		code     int
		stdout   string
		class    wingetExitClass
		wantRows bool
	}{
		{"success", 0, table, wingetExitOK, true},
		{"success, nothing listed", 0, "No installed package found matching input criteria.\r\n", wingetExitOK, false},
		{"no applications found", int(noApps), "No installed package found matching input criteria.\r\n", wingetExitNoApps, false},
		{"no applications found, sign-extended", int(int32(noApps)), "", wingetExitNoApps, false},
		{"other error after a table", 1, table, wingetExitPartial, true},
		{"DLL not found, no output", int(dllNotFound), "", wingetExitFailed, false},
		{"error message only", 1, "Failed when searching source: winget\r\nAn unexpected error occurred\r\n", wingetExitFailed, false},
		{"spinner only", 1, "   - \r   \\ \r   | \r", wingetExitFailed, false},
	}
	for _, c := range cases {
		out, class := classifyWingetExit(c.code, c.stdout)
		if class != c.class {
			t.Errorf("%s: class = %d, want %d", c.name, class, c.class)
		}
		if got := strings.Contains(out, "7zip.7zip"); got != c.wantRows {
			t.Errorf("%s: output = %q", c.name, out)
		}
	}
}

func TestWingetListOutputKeepsNonASCIINames(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)
	raw := "Name           Id       Version Available Source\r\n" +
		"-------------------------------------------------\r\n" +
		"Café Ünïcode…  Some.Id  1.0     1.1       winget\r\n"
	entries := m.parseWingetTable(stripTerminalControls(raw))
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	e := entries[0]
	if e.Name != "Café Ünïcode…" || e.ID != "Some.Id" || e.Version != "1.0" || e.Available != "1.1" {
		t.Errorf("row = %+v", e)
	}
	if stripEllipsis(e.Name) != "Café Ünïcode" {
		t.Errorf("stripEllipsis = %q", stripEllipsis(e.Name))
	}
}
