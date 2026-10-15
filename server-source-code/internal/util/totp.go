package util

import (
	"crypto/subtle"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// TOTPWindow matches Node speakeasy window: 2 (allow ±2 periods for clock drift).
	TOTPWindow = 2
)

// VerifyTOTP verifies a TOTP token against the base32-encoded secret.
// Uses RFC 6238 (30s period, 6 digits, SHA1) with window 2 for clock drift.
func VerifyTOTP(secret, token string, window int) bool {
	if secret == "" || token == "" {
		return false
	}
	valid, _ := totp.ValidateCustom(token, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      uint(window),
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}

// GenerateTOTPSecret creates a new TOTP secret and otpauth URL for QR code setup.
// Uses 32-byte secret to match Node speakeasy generateSecret({ length: 32 }).
func GenerateTOTPSecret(issuer, accountName string) (base32Secret, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		SecretSize:  32,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// MatchTOTPStep returns the 30-second step the token is valid for, within ±window steps of now.
func MatchTOTPStep(secret, token string, window int, now time.Time) (int64, bool) {
	if secret == "" || len(token) != 6 {
		return 0, false
	}
	opts := totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	for i := -window; i <= window; i++ {
		t := now.UTC().Add(time.Duration(i) * 30 * time.Second)
		code, err := totp.GenerateCodeCustom(secret, t, opts)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(code), []byte(token)) == 1 {
			return t.Unix() / 30, true
		}
	}
	return 0, false
}
