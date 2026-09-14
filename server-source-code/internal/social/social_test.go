package social

import "testing"

func TestCount(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  int
		wanto bool
	}{
		{"empty means not injected", "", 0, false},
		{"whitespace means not injected", "   ", 0, false},
		{"zero is injected and means hide", "0", 0, true},
		{"plain number", "2712", 2712, true},
		{"trailing newline from curl", "612\n", 612, true},
		{"non-numeric falls back", "2.7K", 0, false},
		{"html error page falls back", "<!DOCTYPE html>", 0, false},
		{"negative falls back", "-5", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Count(tt.raw)
			if got != tt.want || ok != tt.wanto {
				t.Errorf("Count(%q) = (%d, %v), want (%d, %v)", tt.raw, got, ok, tt.want, tt.wanto)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "1"},
		{130, "130"},
		{612, "612"},
		{999, "999"},
		{1000, "1K"},
		{1049, "1K"},
		{1050, "1.1K"},
		{2712, "2.7K"},
		{9949, "9.9K"},
		{10000, "10K"},
		{12345, "12K"},
		{999499, "999K"},
		{999500, "1M"},
		{999999, "1M"},
		{1000000, "1M"},
		{3400000, "3.4M"},
		{12345678, "12M"},
	}

	for _, tt := range tests {
		if got := Format(tt.n); got != tt.want {
			t.Errorf("Format(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
