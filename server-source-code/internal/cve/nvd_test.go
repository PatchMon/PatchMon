package cve

import "testing"

func TestIsCVEID(t *testing.T) {
	valid := []string{"CVE-2026-46331", "cve-2021-1234", "CVE-2019-123456"}
	for _, s := range valid {
		if !IsCVEID(s) {
			t.Errorf("IsCVEID(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "6.8.0", "<6.18.36", "CVE-20-1", "GHSA-xxxx"}
	for _, s := range invalid {
		if IsCVEID(s) {
			t.Errorf("IsCVEID(%q) = true, want false", s)
		}
	}
}

func TestVersionFromCriteria(t *testing.T) {
	cases := map[string]string{
		"cpe:2.3:o:linux:linux_kernel:6.1.2:*:*:*:*:*:*:*": "6.1.2",
		"cpe:2.3:o:linux:linux_kernel:*:*:*:*:*:*:*:*":     "",
		"cpe:2.3:o:linux:linux_kernel:-:*:*:*:*:*:*:*":     "",
		"cpe:2.3:a:other:thing:1.0:*":                      "",
	}
	for in, want := range cases {
		if got := versionFromCriteria(in); got != want {
			t.Errorf("versionFromCriteria(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCPEMatchToRange(t *testing.T) {
	r := cpeMatchToRange(cpeMatch{
		Vulnerable:            true,
		Criteria:              "cpe:2.3:o:linux:linux_kernel:*:*:*:*:*:*:*:*",
		VersionStartIncluding: "5.15",
		VersionEndExcluding:   "6.1.2",
	})
	if r.Lo != "5.15" || !r.LoIncl || r.Hi != "6.1.2" || r.HiIncl {
		t.Errorf("unexpected range: %+v", r)
	}

	exact := cpeMatchToRange(cpeMatch{
		Vulnerable: true,
		Criteria:   "cpe:2.3:o:linux:linux_kernel:6.1.2:*:*:*:*:*:*:*",
	})
	if exact.Exact != "6.1.2" {
		t.Errorf("expected exact 6.1.2, got %+v", exact)
	}
}

func hasLabel(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestDeriveLabels(t *testing.T) {
	l := deriveLabels("CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:N/C:H/I:H/A:H", "local privilege escalation via use-after-free", []string{"CWE-416"})
	if !hasLabel(l, "LPE") || !hasLabel(l, "Local") || !hasLabel(l, "use-after-free") {
		t.Errorf("LPE case: %v", l)
	}
	l = deriveLabels("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "remote code execution overflow", []string{"CWE-787"})
	if !hasLabel(l, "RCE") || !hasLabel(l, "Remote") {
		t.Errorf("RCE case: %v", l)
	}
	l = deriveLabels("CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H", "crash", []string{"CWE-476"})
	if !hasLabel(l, "DoS") {
		t.Errorf("DoS case: %v", l)
	}
}
