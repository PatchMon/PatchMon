package handler

import "testing"

func TestJWTExpiresInSourceReportsDefaultForUnparseableValue(t *testing.T) {
	if got := jwtExpiresInSource("90s", "", "90s"); got != "default" {
		t.Errorf("source for junk db value = %q, want default", got)
	}
	if got := jwtExpiresInSource("2h", "", "2h"); got != "db" {
		t.Errorf("source for valid db value = %q, want db", got)
	}
	if got := jwtExpiresInSource("30m", "30m", ""); got != "env" {
		t.Errorf("source for valid env value = %q, want env", got)
	}
}
