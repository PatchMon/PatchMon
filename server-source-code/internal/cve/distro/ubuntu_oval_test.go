package distro

import "testing"

// Real "Update Instructions" description shape from Canonical's jammy OVAL for
// CVE-2024-1086 (trimmed). Only generic kernels must be extracted, keyed by
// series; cloud/lowlatency/64k variants ignored.
const ovalDesc = `A use-after-free in netfilter ... We recommend upgrading.

    Update Instructions:

    Run ` + "`sudo pro fix CVE-2024-1086`" + ` to fix the vulnerability. The problem can be corrected
    by updating your system to the following package versions:

linux-image-5.15.0-101-generic - 5.15.0-101.111
linux-image-unsigned-5.15.0-101-generic - 5.15.0-101.111
linux-image-unsigned-5.15.0-101-generic-64k - 5.15.0-101.111
No subscription required

linux-image-6.5.0-26-generic - 6.5.0-26.26~22.04.1
linux-image-unsigned-6.5.0-26-generic - 6.5.0-26.26~22.04.1
No subscription required

linux-image-unsigned-5.15.0-1056-aws - 5.15.0-1056.61
No subscription required
`

func TestParseOVALFixed(t *testing.T) {
	got := parseOVALFixed(ovalDesc)
	want := map[string]string{
		"5.15": "5.15.0-101.111",
		"6.5":  "6.5.0-26.26~22.04.1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d series %v, want %d %v", len(got), got, len(want), want)
	}
	for s, v := range want {
		if got[s] != v {
			t.Errorf("series %s: got %q want %q", s, got[s], v)
		}
	}
	// The 64k and aws variants must NOT create series entries of their own here.
	if _, ok := got["5.15.0"]; ok {
		t.Error("unexpected malformed series key")
	}
}
