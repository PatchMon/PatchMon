package handler

import (
	"strings"
	"testing"
)

func TestCountJSONObjects_IgnoresBracesInsideStrings(t *testing.T) {
	t.Parallel()

	cases := map[string]int{
		`{}`:                  1,
		`{"a":"{{{{{{"}`:      1,
		`{"a":"\"{"}`:         1,
		`{"a":"\\"}{}`:        2,
		`[{},{},{}]`:          3,
		`{"a":{"b":{"c":1}}}`: 3,
		``:                    0,
		`"{{{"`:               0,
	}
	for in, want := range cases {
		if got := countJSONObjects([]byte(in)); got != want {
			t.Errorf("countJSONObjects(%s) = %d, want %d", in, got, want)
		}
	}
}

// complianceResultItem is 192 bytes of struct per 3 bytes of "{}," on the wire,
// so the body limit alone cannot bound the decode. Cost tracks object count.
func TestCountJSONObjects_AmplificationPayloadExceedsCap(t *testing.T) {
	t.Parallel()

	n := (20 * 1024 * 1024) / 3
	for name, body := range map[string]string{
		"nested results": `{"scans":[{"results":[` + strings.Repeat("{},", n-1) + `{}]}]}`,
		"empty scans":    `{"scans":[` + strings.Repeat("{},", n-1) + `{}]}`,
		"legacy flat":    `{"results":[` + strings.Repeat("{},", n-1) + `{}]}`,
	} {
		got := countJSONObjects([]byte(body))
		if got <= maxComplianceObjects {
			t.Errorf("%s: %d objects in a 20MB body, expected above the %d cap", name, got, maxComplianceObjects)
		}
	}
}

// The cap must not reject a real scan. The largest SSG profiles are ~1500 rules;
// this asserts headroom well past that.
func TestMaxComplianceObjects_LeavesHeadroomForRealProfiles(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	sb.WriteString(`{"scans":[{"profile_name":"cis","results":[`)
	const rules = 5000
	for i := 0; i < rules; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"rule_ref":"xccdf_rule","status":"fail","description":"text","remediation":"text"}`)
	}
	sb.WriteString(`]}]}`)

	got := countJSONObjects([]byte(sb.String()))
	if got > maxComplianceObjects {
		t.Fatalf("a %d-rule profile counts %d objects, above the %d cap", rules, got, maxComplianceObjects)
	}
	if maxComplianceObjects < got*5 {
		t.Errorf("cap %d leaves under 5x headroom over a %d-rule profile (%d objects)", maxComplianceObjects, rules, got)
	}
}
