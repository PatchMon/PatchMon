package util

import (
	"crypto/rand"
	"encoding/json"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// TokenRegex validates TFA token: 6 chars A-Z0-9.
var TokenRegex = regexp.MustCompile(`^[A-Z0-9]{6}$`)

const (
	// BcryptCost matches Node bcrypt.hash(code, 10).
	BcryptCost = 10
	// BackupCodeLength matches Node: 6 chars A-Z0-9.
	BackupCodeLength = 6
	// BackupCodeCount matches Node: 10 codes.
	BackupCodeCount = 10
)

const backupCodeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// HashBackupCodes hashes backup codes with bcrypt cost 10 (matches Node).
func HashBackupCodes(codes []string) ([]string, error) {
	hashed := make([]string, len(codes))
	for i, code := range codes {
		h, err := bcrypt.GenerateFromPassword([]byte(code), BcryptCost)
		if err != nil {
			return nil, err
		}
		hashed[i] = string(h)
	}
	return hashed, nil
}

// VerifyBackupCode checks a code against hashed codes. Input is normalized to uppercase.
// Returns (valid, index). Index is for removal when used.
func VerifyBackupCode(code string, hashed []string) (valid bool, index int) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || len(hashed) == 0 {
		return false, -1
	}
	for i, h := range hashed {
		if err := bcrypt.CompareHashAndPassword([]byte(h), []byte(code)); err == nil {
			return true, i
		}
	}
	return false, -1
}

// GenerateBackupCodes creates n codes of 6 chars A-Z0-9 (matches Node format).
func GenerateBackupCodes(n int) []string {
	codes := make([]string, n)
	buf := make([]byte, BackupCodeLength)
	for i := 0; i < n; i++ {
		if _, err := rand.Read(buf); err != nil {
			// Fallback: minimal entropy (should not happen)
			for j := range buf {
				buf[j] = backupCodeChars[(i*BackupCodeLength+j)%len(backupCodeChars)]
			}
		} else {
			for j := range buf {
				buf[j] = backupCodeChars[int(buf[j])%len(backupCodeChars)]
			}
		}
		codes[i] = string(buf)
	}
	return codes
}

// ParseBackupCodesJSON parses tfa_backup_codes from DB (JSON array of bcrypt hashes).
func ParseBackupCodesJSON(jsonStr *string) []string {
	if jsonStr == nil || *jsonStr == "" {
		return nil
	}
	var hashed []string
	if err := json.Unmarshal([]byte(*jsonStr), &hashed); err != nil {
		return nil
	}
	return hashed
}

// EncodeBackupCodesJSON encodes hashed codes for DB storage.
func EncodeBackupCodesJSON(hashed []string) string {
	if len(hashed) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(hashed)
	return string(b)
}
