package packages

import "testing"

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
