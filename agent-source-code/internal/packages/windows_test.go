package packages

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

// TestStripEllipsis covers winget's column truncation marker in each of the
// forms it reaches the agent in. The mis-encoded variants appear when the
// PowerShell console encoding is a single-byte code page rather than UTF-8.
func TestStripEllipsis(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"untruncated", "Microsoft Windows Desktop Runtime", "Microsoft Windows Desktop Runtime"},
		{"utf8 ellipsis", "Microsoft Windows Desktop Runtime - 8.0\u2026", "Microsoft Windows Desktop Runtime - 8.0"},
		{"cp850 mangled", "Microsoft Visual C++ 2015-2022 Redistri\u00d4\u00c7\u00aa", "Microsoft Visual C++ 2015-2022 Redistri"},
		{"cp1252 mangled", "Microsoft Visual C++ 2015-2022 Redistri\u00e2\u20ac\u00a6", "Microsoft Visual C++ 2015-2022 Redistri"},
		{"surrounding space trimmed", "  vim\u2026  ", "vim"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripEllipsis(tc.in); got != tc.want {
				t.Fatalf("stripEllipsis(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The winget table is padded to a fixed display width, and a truncated name
// ends in a single ellipsis character that is three bytes of UTF-8. Slicing a
// data line at a byte offset taken from the ASCII header used to cut that
// character in half, which is what the windows-latest run surfaced: names
// arrived ending "\xe2\x80" and versions arrived starting "\xa6".
//
// The fixture only reproduces that when the ellipsis sits hard against the
// column boundary, so the name is built to fill its column exactly. Pad it and
// the byte offset lands harmlessly in the spaces instead, and the test passes
// against the very bug it is meant to catch.
func TestParseWingetTableSplitsOnCharactersNotBytes(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)

	const (
		idColumn      = 40
		versionColumn = 64
	)
	pad := func(s string, width int) string {
		if n := utf8.RuneCountInString(s); n < width {
			return s + strings.Repeat(" ", width-n)
		}
		return s
	}

	// 39 characters plus the ellipsis exactly fills the name column, putting
	// the three ellipsis bytes across the byte offset of the next column.
	const truncatedName = "Microsoft Visual C++ 2015 UWP Desktop R\u2026"
	if got := utf8.RuneCountInString(truncatedName); got != idColumn {
		t.Fatalf("fixture is wrong: name is %d characters, must be exactly %d to straddle the boundary", got, idColumn)
	}

	output := pad("Name", idColumn) + pad("Id", versionColumn-idColumn) + "Version\n" +
		strings.Repeat("-", versionColumn+12) + "\n" +
		truncatedName + pad("Microsoft.VCRedist", versionColumn-idColumn) + "14.0.33728.0\n" +
		pad("Plain ASCII Package", idColumn) + pad("Contoso.Plain", versionColumn-idColumn) + "1.2.3\n"

	entries := m.parseWingetTable(output)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}

	for i, e := range entries {
		for field, v := range map[string]string{"name": e.Name, "id": e.ID, "version": e.Version} {
			if !utf8.ValidString(v) {
				t.Errorf("entry %d %s is not valid UTF-8: %q", i, field, v)
			}
		}
	}

	if got := entries[0].ID; got != "Microsoft.VCRedist" {
		t.Errorf("id = %q, want %q; a split ellipsis leaks its trailing bytes into this field", got, "Microsoft.VCRedist")
	}
	if got := entries[0].Version; got != "14.0.33728.0" {
		t.Errorf("version = %q, want %q", got, "14.0.33728.0")
	}
	// stripEllipsis can only do its job if the ellipsis survived intact.
	if got := stripEllipsis(entries[0].Name); got != "Microsoft Visual C++ 2015 UWP Desktop R" {
		t.Errorf("stripEllipsis(name) = %q, want the name without its truncation marker", got)
	}
	if got := entries[1].Name; got != "Plain ASCII Package" {
		t.Errorf("ascii row regressed: name = %q", got)
	}
}
