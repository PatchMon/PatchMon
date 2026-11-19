package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIsEnrollmentToken(t *testing.T) {
	for _, test := range []struct {
		metadata string
		allowed  bool
	}{
		{"", true}, {"null", true}, {`{}`, true},
		{`{"integration_type":"proxmox-lxc"}`, true},
		{`{"integration_type":"direct-host"}`, true},
		{`{"integration_type":"api"}`, false},
		{`{"integration_type":"gethomepage"}`, false},
		{`{"INTEGRATION_TYPE":"api"}`, false},
		{`{"integration_type":"direct-host","Integration_Type":"api"}`, false},
		{`{"integration_type":"unknown"}`, false},
		{`{"integration_type":null}`, false},
		{`{"integration_type":["proxmox-lxc"]}`, false},
		{`{"integration_type":{"name":"direct-host"}}`, false},
		{`[]`, false}, {`"api"`, false}, {`invalid`, false},
	} {
		t.Run(test.metadata, func(t *testing.T) {
			if got := isEnrollmentToken([]byte(test.metadata)); got != test.allowed {
				t.Fatalf("isEnrollmentToken = %v, want %v", got, test.allowed)
			}
		})
	}
}

func TestMergeEnrollmentMetadata(t *testing.T) {
	base := json.RawMessage(`{"integration_type":"direct-host","source":"netbox","tenant_id":9007199254740993}`)
	merged, err := mergeEnrollmentMetadata(base, json.RawMessage(`{"tenant_slug":"customer","source":"updated"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), `"tenant_id":9007199254740993`) || !strings.Contains(string(merged), `"source":"updated"`) || !isEnrollmentToken(merged) {
		t.Fatalf("metadata was not preserved: %s", merged)
	}
	for _, patch := range []string{
		`{"integration_type":"api"}`, `{"integration_type":"gethomepage"}`,
		`{"INTEGRATION_TYPE":"api"}`, `{"Integration_Type":"gethomepage"}`,
		`{"integration_type":null}`, `{"integration_type":[]}`, `{"integration_type":{}}`,
		`{"integration_type":5}`, `[]`, `"api"`,
	} {
		t.Run(patch, func(t *testing.T) {
			if _, err := mergeEnrollmentMetadata(base, json.RawMessage(patch)); err == nil {
				t.Fatal("accepted invalid metadata or integration type change")
			}
		})
	}
	if _, err := mergeEnrollmentMetadata(base, json.RawMessage(`{"integration_type":"direct-host"}`)); err != nil {
		t.Fatalf("unchanged integration type: %v", err)
	}
}

func TestValidateEnrollmentConfig(t *testing.T) {
	validRate := int32(100)
	if err := validateEnrollmentConfig(&validRate, []string{"127.0.0.1", "192.0.2.0/24", "2001:db8::/32", "::1"}); err != nil {
		t.Fatal(err)
	}
	for _, rate := range []int32{-1, 0, 1001} {
		if err := validateEnrollmentConfig(&rate, nil); err == nil {
			t.Fatalf("accepted rate %d", rate)
		}
	}
	for _, ip := range []string{"", "example.com", "192.0.2.1/33", "2001:db8::/129", "*"} {
		if err := validateEnrollmentConfig(nil, []string{ip}); err == nil {
			t.Fatalf("accepted IP range %q", ip)
		}
	}
}

func TestParseEnrollmentExpiration(t *testing.T) {
	for _, value := range []string{"2026-11-19T14:00:00Z", "2026-11-19T15:00:00+01:00", "2026-11-19T14:00"} {
		stamp, err := parseEnrollmentExpiration(&value)
		if err != nil || !stamp.Valid || !stamp.Time.Equal(time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)) {
			t.Fatalf("parse %q = %v, %v", value, stamp, err)
		}
	}
	for _, value := range []string{"invalid", "2026-11-19", "2026-02-30T14:00:00Z"} {
		if _, err := parseEnrollmentExpiration(&value); err == nil {
			t.Fatalf("accepted invalid expiration %q", value)
		}
	}
	empty := ""
	for _, value := range []*string{nil, &empty} {
		if stamp, err := parseEnrollmentExpiration(value); err != nil || stamp.Valid {
			t.Fatalf("cleared expiration = %v, %v", stamp, err)
		}
	}
}
