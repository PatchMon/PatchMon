package util

import "testing"

func TestCompareKernelVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"6.8.0-51-generic", "6.18.36", -1},
		{"6.18.36", "6.8.0-51-generic", 1},
		{"6.18.36", "6.18.36", 0},
		{"6.18.36-100-generic", "6.18.36", 1},
		{"5.15.0-89", "5.15.0-89-generic", 0},
		{"4.18.0-513.5.1.el8_9.x86_64", "4.18.0-513", 1},
		{"6.1.2", "6.1.10", -1},
	}
	for _, c := range cases {
		if got := CompareKernelVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareKernelVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseKernelExprAndMatch(t *testing.T) {
	cases := []struct {
		expr    string
		kernel  string
		want    bool
		wantErr bool
	}{
		{"<6.18.36", "6.8.0-51-generic", true, false},
		{"<6.18.36", "6.18.36-100-generic", false, false},
		{"<=6.18.36", "6.18.36", true, false},
		{">5.15", "6.8.0-51-generic", true, false},
		{">5.15", "5.4.0-100", false, false},
		{">=5.15", "5.15.0-89-generic", true, false},
		{"6.8", "6.8.0-51-generic", true, false},
		{"6.8", "6.9.0-1-generic", false, false},
		{"=6.8.0", "6.8.0-51-generic", true, false},
		{"5.15..6.1", "5.19.0-3", true, false},
		{"5.15..6.1", "6.2.0-1", false, false},
		{"5.15..6.1", "5.4.0-1", false, false},
		{"", "6.8.0", false, true},
		{"garbage", "6.8.0", false, true},
	}
	for _, c := range cases {
		f, err := ParseKernelExpr(c.expr)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseKernelExpr(%q) expected error, got nil", c.expr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseKernelExpr(%q) unexpected error: %v", c.expr, err)
			continue
		}
		if got := f.Matches(c.kernel); got != c.want {
			t.Errorf("ParseKernelExpr(%q).Matches(%q) = %v, want %v", c.expr, c.kernel, got, c.want)
		}
	}
}

func TestKernelFilterEmptyKernelNeverMatches(t *testing.T) {
	f, err := ParseKernelExpr("<6.18.36")
	if err != nil {
		t.Fatal(err)
	}
	if f.Matches("") {
		t.Error("empty kernel should not match a positive filter")
	}
	if f.Matches("   ") {
		t.Error("blank kernel should not match a positive filter")
	}
}

func TestFromKernelRangesOR(t *testing.T) {
	// Simulates a CVE that affects two disjoint upstream ranges.
	f := FromKernelRanges("CVE-TEST", []KernelRange{
		{Hi: "5.15.90", HiIncl: false},
		{Lo: "6.1", LoIncl: true, Hi: "6.1.20", HiIncl: false},
	})
	if !f.Matches("5.10.0-100") {
		t.Error("5.10.0-100 should match first range")
	}
	if !f.Matches("6.1.5-1") {
		t.Error("6.1.5-1 should match second range")
	}
	if f.Matches("6.2.0-1") {
		t.Error("6.2.0-1 should not match either range")
	}
}
