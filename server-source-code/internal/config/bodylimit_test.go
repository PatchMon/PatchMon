package config

import "testing"

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
	fromEnv := resolveBodyLimitKB("AGENT_PING_BODY_LIMIT", nil, 8*1024)

	t.Setenv("AGENT_PING_BODY_LIMIT", "")
	dbVal := "16"
	fromDB := resolveBodyLimitKB("AGENT_PING_BODY_LIMIT", &dbVal, 8*1024)

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

func TestLoad_ComplianceBodyLimitDefault(t *testing.T) {
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
	const fallback = 8 * 1024
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
