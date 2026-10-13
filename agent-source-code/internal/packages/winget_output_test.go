package packages

import (
	"io"
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
