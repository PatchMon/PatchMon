package config

import (
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultJWTExpiresIn is the access-token lifetime used when JWT_EXPIRES_IN is unset or unparseable.
const DefaultJWTExpiresIn = time.Hour

// DefaultJWTExpiresInSetting is DefaultJWTExpiresIn written as a setting value.
const DefaultJWTExpiresInSetting = "1h"

var warnedJWTExpiresIn sync.Map

// ParseJWTExpiresIn parses a positive whole number followed by m, h or d (e.g. 30m, 1h, 7d).
func ParseJWTExpiresIn(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 2 {
		return 0, false
	}
	var unit time.Duration
	switch s[len(s)-1] {
	case 'm':
		unit = time.Minute
	case 'h':
		unit = time.Hour
	case 'd':
		unit = 24 * time.Hour
	default:
		return 0, false
	}
	digits := s[:len(s)-1]
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt64/int64(unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

// JWTExpiresInSeconds returns the lifetime for s, falling back to DefaultJWTExpiresIn
// and warning once per distinct unparseable value.
func JWTExpiresInSeconds(s string) int64 {
	if d, ok := ParseJWTExpiresIn(s); ok {
		return int64(d.Seconds())
	}
	warnInvalidJWTExpiresIn(s)
	return int64(DefaultJWTExpiresIn.Seconds())
}

// EffectiveJWTExpiresIn returns s when it parses, otherwise the default it resolves to.
func EffectiveJWTExpiresIn(s string) string {
	if _, ok := ParseJWTExpiresIn(s); ok {
		return strings.TrimSpace(s)
	}
	return DefaultJWTExpiresInSetting
}

func warnInvalidJWTExpiresIn(s string) {
	if _, ok := ParseJWTExpiresIn(s); ok {
		return
	}
	if _, seen := warnedJWTExpiresIn.LoadOrStore(s, struct{}{}); !seen {
		slog.Warn("JWT_EXPIRES_IN is not a valid duration, using the default", "value", s, "default", DefaultJWTExpiresIn.String())
	}
}

// ValidateJWTExpiresIn rejects a value the resolver would discard in favour of the default.
func ValidateJWTExpiresIn(value string) error {
	if _, ok := ParseJWTExpiresIn(value); !ok {
		return errors.New("JWT_EXPIRES_IN must be a positive whole number followed by m, h or d, such as 30m, 1h or 7d")
	}
	return nil
}
