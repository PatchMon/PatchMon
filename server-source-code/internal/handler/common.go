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

// passwordPolicy is the client-side validation shape shared with the login and
// first-time setup screens.
type passwordPolicy struct {
	MinLength        int  `json:"min_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireLowercase bool `json:"require_lowercase"`
	RequireNumber    bool `json:"require_number"`
	RequireSpecial   bool `json:"require_special"`
}

// resolvePasswordPolicy reads the policy from resolved config, falling back to
// the built-in defaults when no config resolves.
func resolvePasswordPolicy(resolved *config.ResolvedConfig) passwordPolicy {
	p := passwordPolicy{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
	}
	if resolved != nil {
		p.MinLength = resolved.PasswordMinLength
		p.RequireUppercase = resolved.PasswordRequireUppercase
		p.RequireLowercase = resolved.PasswordRequireLowercase
		p.RequireNumber = resolved.PasswordRequireNumber
		p.RequireSpecial = resolved.PasswordRequireSpecial
	}
	return p
}

// ValidatePasswordPolicy checks password against resolved config. Returns descriptive error.
func ValidatePasswordPolicy(resolved *config.ResolvedConfig, password string) error {
	policy := resolvePasswordPolicy(resolved)
	minLen := policy.MinLength
	needUpper, needLower, needNum, needSpecial := policy.RequireUppercase, policy.RequireLowercase, policy.RequireNumber, policy.RequireSpecial
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

const maxPaginationOffset = 50000

func clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > maxPaginationOffset {
		return maxPaginationOffset
	}
	return offset
}

func clampPageForLimit(page, limit int) int {
	if page < 1 {
		return 1
	}
	if limit < 1 {
		return page
	}
	maxPage := maxPaginationOffset/limit + 1
	if page > maxPage {
		return maxPage
	}
	return page
}
