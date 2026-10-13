package config

import (
	"context"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestParseBodyLimit_UnsuffixedIsMB(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"20", 20 * 1024 * 1024},
		{"20mb", 20 * 1024 * 1024},
		{"8kb", 8 * 1024},
		{"32mb", 32 * 1024 * 1024},
		{"1gb", maxBodyLimitBytes},
		{"512b", 512},
		{"", 7},
		{"garbage", 7},
		{"0", 7},
		{"-3", 7},
	}
	for _, c := range cases {
		if got := parseBodyLimit(c.in, 7); got != c.want {
			t.Errorf("parseBodyLimit(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseBodyLimitKB_UnsuffixedIsKB(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"8", 8 * 1024},
		{"8kb", 8 * 1024},
		{"2mb", 2 * 1024 * 1024},
		{"32mb", 32 * 1024 * 1024},
		{"1gb", maxBodyLimitBytes},
		{"512b", 512},
		{"", 7},
		{"garbage", 7},
		{"0", 7},
	}
	for _, c := range cases {
		if got := parseBodyLimitKB(c.in, 7); got != c.want {
			t.Errorf("parseBodyLimitKB(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestResolveBodyLimitKB_EnvAndDBAgreeOnBareNumbers(t *testing.T) {
	t.Setenv("AGENT_PING_BODY_LIMIT", "16")
	fromEnv := resolveBodyLimitKB("AGENT_PING_BODY_LIMIT", nil, defaultAgentPingBodyLimitKB*1024)

	t.Setenv("AGENT_PING_BODY_LIMIT", "")
	dbVal := "16"
	fromDB := resolveBodyLimitKB("AGENT_PING_BODY_LIMIT", &dbVal, defaultAgentPingBodyLimitKB*1024)

	if fromEnv != fromDB {
		t.Errorf("env = %d, db = %d, want equal", fromEnv, fromDB)
	}
	if fromEnv != 16*1024 {
		t.Errorf("resolved = %d, want %d", fromEnv, 16*1024)
	}
}

func TestResolveBodyLimit_EnvBeatsDB(t *testing.T) {
	t.Setenv("COMPLIANCE_BODY_LIMIT", "32mb")
	dbVal := "50mb"
	if got := resolveBodyLimit("COMPLIANCE_BODY_LIMIT", &dbVal, 20*1024*1024); got != 32*1024*1024 {
		t.Errorf("resolved = %d, want %d", got, 32*1024*1024)
	}
}

func TestResolveBodyLimit_DBBeatsDefault(t *testing.T) {
	t.Setenv("COMPLIANCE_BODY_LIMIT", "")
	dbVal := "25mb"
	if got := resolveBodyLimit("COMPLIANCE_BODY_LIMIT", &dbVal, 20*1024*1024); got != 25*1024*1024 {
		t.Errorf("resolved = %d, want %d", got, 25*1024*1024)
	}
}

// The settings dropdown used to offer 50mb. An install that picked it must land
// on the ceiling after an upgrade, not silently drop to the default.
func TestResolveBodyLimit_StoredValueAboveCeilingClamps(t *testing.T) {
	t.Setenv("COMPLIANCE_BODY_LIMIT", "")
	dbVal := "50mb"
	if got := resolveBodyLimit("COMPLIANCE_BODY_LIMIT", &dbVal, 20*1024*1024); got != maxBodyLimitBytes {
		t.Errorf("resolved = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
}

var bodyLimitKeys = []string{"JSON_BODY_LIMIT", "AGENT_UPDATE_BODY_LIMIT", "COMPLIANCE_BODY_LIMIT", "AGENT_PING_BODY_LIMIT"}

func TestEnvAndDBPathsAgreeOnEverySuffix(t *testing.T) {
	const kb, mb = int64(1 << 10), int64(1 << 20)
	const complianceDefault, pingDefault = 20 * mb, defaultAgentPingBodyLimitKB * kb
	cases := []struct {
		in         string
		general    int64
		compliance int64
		ping       int64
	}{
		{"", 5 * mb, complianceDefault, pingDefault},
		{"0", 5 * mb, complianceDefault, pingDefault},
		{"-5", 5 * mb, complianceDefault, pingDefault},
		{"abc", 5 * mb, complianceDefault, pingDefault},
		{"512b", 512, 512, 512},
		{"8kb", 8 * kb, 8 * kb, 8 * kb},
		{"16", 16 * mb, 16 * mb, 16 * kb},
		{"128kb", 128 * kb, 128 * kb, 128 * kb},
		{"1mb", mb, mb, mb},
		{" 2MB ", 2 * mb, 2 * mb, 2 * mb},
		{"20mb", 20 * mb, 20 * mb, 20 * mb},
		{"32mb", maxBodyLimitBytes, maxBodyLimitBytes, maxBodyLimitBytes},
		{"33mb", maxBodyLimitBytes, maxBodyLimitBytes, maxBodyLimitBytes},
		{"1gb", maxBodyLimitBytes, maxBodyLimitBytes, maxBodyLimitBytes},
	}
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", "postgresql://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")

	for _, c := range cases {
		for _, key := range bodyLimitKeys {
			t.Setenv(key, c.in)
		}
		fromEnv, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for key, got := range map[string]int64{
			"JSON_BODY_LIMIT":         fromEnv.JSONBodyLimitBytes,
			"AGENT_UPDATE_BODY_LIMIT": fromEnv.AgentUpdateBodyLimitBytes,
		} {
			if want := c.general; got != want {
				t.Errorf("env %s=%q = %d, want %d", key, c.in, got, want)
			}
		}
		if fromEnv.ComplianceBodyLimitBytes != c.compliance {
			t.Errorf("env COMPLIANCE_BODY_LIMIT=%q = %d, want %d", c.in, fromEnv.ComplianceBodyLimitBytes, c.compliance)
		}
		if fromEnv.AgentPingBodyLimitBytes != c.ping {
			t.Errorf("env AGENT_PING_BODY_LIMIT=%q = %d, want %d", c.in, fromEnv.AgentPingBodyLimitBytes, c.ping)
		}

		for _, key := range bodyLimitKeys {
			t.Setenv(key, "")
		}
		defaults, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		dbVal := c.in
		fromDB := ResolveConfig(context.Background(), defaults, &models.Settings{
			JSONBodyLimit:        &dbVal,
			AgentUpdateBodyLimit: &dbVal,
			ComplianceBodyLimit:  &dbVal,
			AgentPingBodyLimit:   &dbVal,
		})
		for key, got := range map[string]int64{
			"JSON_BODY_LIMIT":         fromDB.JSONBodyLimitBytes,
			"AGENT_UPDATE_BODY_LIMIT": fromDB.AgentUpdateBodyLimitBytes,
		} {
			if want := c.general; got != want {
				t.Errorf("db %s=%q = %d, want %d", key, c.in, got, want)
			}
		}
		if fromDB.ComplianceBodyLimitBytes != c.compliance {
			t.Errorf("db COMPLIANCE_BODY_LIMIT=%q = %d, want %d", c.in, fromDB.ComplianceBodyLimitBytes, c.compliance)
		}
		if fromDB.AgentPingBodyLimitBytes != c.ping {
			t.Errorf("db AGENT_PING_BODY_LIMIT=%q = %d, want %d", c.in, fromDB.AgentPingBodyLimitBytes, c.ping)
		}
	}
}

// The operator guide states every gb value is reduced to the ceiling.
func TestGigabyteClampsOnBothPaths(t *testing.T) {
	t.Setenv("PROBE_KB", "1gb")
	if got := getEnvBytesKBDefault("PROBE_KB", 64); got != maxBodyLimitBytes {
		t.Errorf("env KB path 1gb = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
	if got := parseBodyLimitKB("1gb", 64*1024); got != maxBodyLimitBytes {
		t.Errorf("db KB path 1gb = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
}

func TestLoad_BodyLimitDefaults(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", "postgresql://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ComplianceBodyLimitBytes != 20*1024*1024 {
		t.Errorf("ComplianceBodyLimitBytes = %d, want %d", cfg.ComplianceBodyLimitBytes, 20*1024*1024)
	}
	if cfg.AgentUpdateBodyLimitBytes != 5*1024*1024 {
		t.Errorf("AgentUpdateBodyLimitBytes = %d, want %d", cfg.AgentUpdateBodyLimitBytes, 5*1024*1024)
	}
	if cfg.AgentPingBodyLimitBytes != 64*1024 {
		t.Errorf("AgentPingBodyLimitBytes = %d, want %d", cfg.AgentPingBodyLimitBytes, 64*1024)
	}
}

func TestParseBodyLimit_OverRangeClamps(t *testing.T) {
	const fallback = 5 * 1024 * 1024
	for _, in := range []string{
		"9223372036854775807",
		"9999999999999gb",
		"9007199254740993gb",
		"8796093022208kb",
		"2gb",
		"512mb",
		"257mb",
	} {
		got := parseBodyLimit(in, fallback)
		if got != maxBodyLimitBytes {
			t.Errorf("parseBodyLimit(%q) = %d, want a clamp to %d", in, got, maxBodyLimitBytes)
		}
	}
}

func TestParseBodyLimitKB_OverRangeClamps(t *testing.T) {
	const fallback = defaultAgentPingBodyLimitKB * 1024
	for _, in := range []string{"9223372036854775807", "9999999999999gb", "2gb", "512mb"} {
		got := parseBodyLimitKB(in, fallback)
		if got != maxBodyLimitBytes {
			t.Errorf("parseBodyLimitKB(%q) = %d, want a clamp to %d", in, got, maxBodyLimitBytes)
		}
	}
}

func TestGetEnvBytes_OverRangeClamps(t *testing.T) {
	t.Setenv("PROBE_LIMIT", "9223372036854775807")
	if got := getEnvBytes("PROBE_LIMIT", 5); got != maxBodyLimitBytes {
		t.Errorf("getEnvBytes = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
	t.Setenv("PROBE_LIMIT", "9999999999999gb")
	if got := getEnvBytes("PROBE_LIMIT", 5); got != maxBodyLimitBytes {
		t.Errorf("getEnvBytes = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
	t.Setenv("PROBE_LIMIT", "not-a-size")
	if got := getEnvBytes("PROBE_LIMIT", 5); got != 5*1024*1024 {
		t.Errorf("getEnvBytes = %d, want the 5mb fallback for garbage", got)
	}
	t.Setenv("PROBE_LIMIT", "9223372036854775807")
	if got := getEnvBytesKBDefault("PROBE_LIMIT", 8); got != maxBodyLimitBytes {
		t.Errorf("getEnvBytesKBDefault = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
}

// Every value the settings dropdown can write must parse to exactly the size
// it names, or saving an unchanged field silently changes the limit.
func TestDropdownOptionsParseExactly(t *testing.T) {
	mb := map[string]int64{
		"1mb": 1 << 20, "2mb": 2 << 20, "5mb": 5 << 20,
		"10mb": 10 << 20, "20mb": 20 << 20, "32mb": 32 << 20,
	}
	for opt, want := range mb {
		if got := parseBodyLimit(opt, -1); got != want {
			t.Errorf("parseBodyLimit(%q) = %d, want %d", opt, got, want)
		}
	}

	kb := map[string]int64{
		"8kb": 8 << 10, "16kb": 16 << 10, "32kb": 32 << 10,
		"64kb": 64 << 10, "128kb": 128 << 10,
	}
	for opt, want := range kb {
		if got := parseBodyLimitKB(opt, -1); got != want {
			t.Errorf("parseBodyLimitKB(%q) = %d, want %d", opt, got, want)
		}
	}
}

func TestScaleBodyLimit_CeilingIsExactlyInclusive(t *testing.T) {
	if got := parseBodyLimit("32mb", -1); got != maxBodyLimitBytes {
		t.Errorf("parseBodyLimit(\"32mb\") = %d, want the %d ceiling", got, maxBodyLimitBytes)
	}
	if got := parseBodyLimit("33mb", 5<<20); got != maxBodyLimitBytes {
		t.Errorf("parseBodyLimit(\"33mb\") = %d, want a clamp to %d", got, maxBodyLimitBytes)
	}
	if got := parseBodyLimit("50mb", 5<<20); got != maxBodyLimitBytes {
		t.Errorf("parseBodyLimit(\"50mb\") = %d, want a clamp to %d, not the default", got, maxBodyLimitBytes)
	}
	if got := parseBodyLimit("garbage", 5<<20); got != 5<<20 {
		t.Errorf("parseBodyLimit(\"garbage\") = %d, want the fallback", got)
	}
	if got := parseBodyLimit("0", 5<<20); got != 5<<20 {
		t.Errorf("parseBodyLimit(\"0\") = %d, want the fallback", got)
	}
}

func TestValidateBodyLimit(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"JSON_BODY_LIMIT", "5mb", true},
		{"JSON_BODY_LIMIT", "5", true},
		{"JSON_BODY_LIMIT", "512kb", true},
		{"JSON_BODY_LIMIT", "1gb", true},
		{"COMPLIANCE_BODY_LIMIT", "50mb", true},
		{"AGENT_UPDATE_BODY_LIMIT", " 2MB ", true},
		{"AGENT_PING_BODY_LIMIT", "64", true},
		{"AGENT_PING_BODY_LIMIT", "128kb", true},
		{"JSON_BODY_LIMIT", "", false},
		{"JSON_BODY_LIMIT", "abc", false},
		{"JSON_BODY_LIMIT", "0", false},
		{"JSON_BODY_LIMIT", "-1mb", false},
		{"JSON_BODY_LIMIT", "5 megabytes", false},
		{"AGENT_PING_BODY_LIMIT", "0kb", false},
		{"AGENT_PING_BODY_LIMIT", "64kib", false},
	}
	for _, c := range cases {
		err := ValidateBodyLimit(c.key, c.value)
		if (err == nil) != c.ok {
			t.Errorf("ValidateBodyLimit(%s, %q) err = %v, want ok=%v", c.key, c.value, err, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), c.key) {
			t.Errorf("error %q does not name %s", err, c.key)
		}
	}
	if err := ValidateBodyLimit("AGENT_PING_BODY_LIMIT", "x"); err == nil || !strings.Contains(err.Error(), "bare number means KB") {
		t.Errorf("ping error = %v, want it to say a bare number means KB", err)
	}
	if err := ValidateBodyLimit("JSON_BODY_LIMIT", "x"); err == nil || !strings.Contains(err.Error(), "bare number means MB") {
		t.Errorf("json error = %v, want it to say a bare number means MB", err)
	}
}
