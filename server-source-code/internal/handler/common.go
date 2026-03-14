package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
)

// hostFromRequest returns the X-Forwarded-Host value for use in job payloads,
// so queue workers can resolve the correct database for the request.
func hostFromRequest(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
}

// ValidatePasswordPolicy checks password against resolved config. Returns descriptive error.
func ValidatePasswordPolicy(resolved *config.ResolvedConfig, password string) error {
	minLen := 8
	needUpper, needLower, needNum, needSpecial := true, true, true, true
	if resolved != nil {
		minLen = resolved.PasswordMinLength
		needUpper = resolved.PasswordRequireUppercase
		needLower = resolved.PasswordRequireLowercase
		needNum = resolved.PasswordRequireNumber
		needSpecial = resolved.PasswordRequireSpecial
	}
	if len(password) < minLen {
		return fmt.Errorf("password must be at least %d characters", minLen)
	}
	var hasUpper, hasLower, hasNum, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsNumber(r):
			hasNum = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	if needUpper && !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if needLower && !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if needNum && !hasNum {
		return fmt.Errorf("password must contain at least one number")
	}
	if needSpecial && !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}
	return nil
}

func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func parseIntQuery(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}
