package config

import (
	"testing"
	"time"
)

func TestParseJWTExpiresIn(t *testing.T) {
	valid := map[string]time.Duration{
		"15m": 15 * time.Minute, "30m": 30 * time.Minute, "1h": time.Hour, "2h": 2 * time.Hour,
		"1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, " 1H ": time.Hour,
	}
	for in, want := range valid {
		if got, ok := ParseJWTExpiresIn(in); !ok || got != want {
			t.Errorf("ParseJWTExpiresIn(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "h", "60", "90s", "1hr", "1 h", "0h", "-1h", "+1h", "1.5h", "abc", "99999999999d"} {
		if got, ok := ParseJWTExpiresIn(in); ok {
			t.Errorf("ParseJWTExpiresIn(%q) = %v, want rejected", in, got)
		}
	}
}

func TestJWTExpiresInSecondsFallsBackToOneHour(t *testing.T) {
	for _, in := range []string{"", "60", "90s", "1hr", "1 h", "0h", "-1h"} {
		if got := JWTExpiresInSeconds(in); got != 3600 {
			t.Errorf("JWTExpiresInSeconds(%q) = %d, want 3600", in, got)
		}
	}
	if got := JWTExpiresInSeconds("30m"); got != 1800 {
		t.Errorf("JWTExpiresInSeconds(30m) = %d, want 1800", got)
	}
}

func TestValidateJWTExpiresIn(t *testing.T) {
	if err := ValidateJWTExpiresIn("7d"); err != nil {
		t.Errorf("7d rejected: %v", err)
	}
	if err := ValidateJWTExpiresIn("60"); err == nil {
		t.Error("bare number accepted")
	}
}

func TestEffectiveJWTExpiresIn(t *testing.T) {
	for in, want := range map[string]string{"30m": "30m", " 7d ": "7d", "60": "1h", "1hr": "1h", "": "1h"} {
		if got := EffectiveJWTExpiresIn(in); got != want {
			t.Errorf("EffectiveJWTExpiresIn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultJWTExpiresInSettingMatchesDuration(t *testing.T) {
	if d, ok := ParseJWTExpiresIn(DefaultJWTExpiresInSetting); !ok || d != DefaultJWTExpiresIn {
		t.Errorf("ParseJWTExpiresIn(%q) = %v, %v; want %v", DefaultJWTExpiresInSetting, d, ok, DefaultJWTExpiresIn)
	}
}
