// Package logutil provides utilities for safe logging, including sanitization
// of user-provided values to prevent log injection.
package logutil

import (
	"strings"
	"unicode"
)

// Sanitize replaces newlines, carriage returns, and other control characters
// in s with safe placeholders to prevent log injection. Use for user-provided
// values (hostnames, IDs, error messages) before logging.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if unicode.IsControl(r) {
				b.WriteString("?")
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// SanitizeMap returns a copy of m with string values sanitized for logging.
func SanitizeMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			out[k] = Sanitize(val)
		case []string:
			sanitized := make([]string, len(val))
			for i, s := range val {
				sanitized[i] = Sanitize(s)
			}
			out[k] = sanitized
		default:
			out[k] = v
		}
	}
	return out
}
