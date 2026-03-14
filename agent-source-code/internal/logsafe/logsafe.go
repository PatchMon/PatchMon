// Package logsafe provides sanitization for user input before logging to prevent log injection.
package logsafe

import (
	"strings"
	"unicode"
)

// MaxLogValueLen limits string length in logs to reduce injection surface.
const MaxLogValueLen = 512

// SanitizeForLog removes or replaces characters that could be used for log injection.
// Newlines, carriage returns, and other control characters are replaced with a space
// so user input cannot forge new log entries.
func SanitizeForLog(s string) string {
	if s == "" {
		return s
	}
	if len(s) > MaxLogValueLen {
		s = s[:MaxLogValueLen]
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1 // drop
		}
		return r
	}, s)
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
			out[k] = SanitizeForLog(val)
		case []string:
			sanitized := make([]string, len(val))
			for i, s := range val {
				sanitized[i] = SanitizeForLog(s)
			}
			out[k] = sanitized
		default:
			out[k] = v
		}
	}
	return out
}
