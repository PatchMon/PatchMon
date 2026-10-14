package util

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestMatchTOTPStep(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("PatchMon", "t")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_015, 0)
	for _, off := range []int{-2, -1, 0, 1, 2} {
		at := now.Add(time.Duration(off) * 30 * time.Second)
		code, _ := totp.GenerateCode(secret, at)
		step, ok := MatchTOTPStep(secret, code, TOTPWindow, now)
		if !ok || step != at.Unix()/30 {
			t.Fatalf("offset %d: ok=%v step=%d want %d", off, ok, step, at.Unix()/30)
		}
	}
	old, _ := totp.GenerateCode(secret, now.Add(-3*30*time.Second))
	if _, ok := MatchTOTPStep(secret, old, TOTPWindow, now); ok {
		t.Fatal("code outside the window accepted")
	}
	for _, bad := range []string{"", "12345", "abcdef"} {
		if _, ok := MatchTOTPStep(secret, bad, TOTPWindow, now); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}
