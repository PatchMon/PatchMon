package util

import (
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
