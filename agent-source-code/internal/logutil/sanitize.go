// Package logutil provides utilities for safe logging, including sanitization
// of user-provided values to prevent log injection.
package logutil

import (
	"fmt"
	"strings"
	"unicode"
)

const hexDigits = "0123456789ABCDEF"

// Sanitize maps control characters to safe, visible escape sequences to prevent
// log injection. Common controls (\n, \r, \t, \v, \f) become literal sequences;
// other Unicode control characters become hex-escaped (\xNN). Use for
// user-provided values (hostnames, IDs, error messages) before logging.
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
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if unicode.IsControl(r) {
				writeHexEscape(&b, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func writeHexEscape(b *strings.Builder, r rune) {
	if r < 0x100 {
		b.WriteString(`\x`)
		b.WriteByte(hexDigits[r>>4])
		b.WriteByte(hexDigits[r&0xF])
	} else if r < 0x10000 {
		b.WriteString(`\u`)
		b.WriteByte(hexDigits[(r>>12)&0xF])
		b.WriteByte(hexDigits[(r>>8)&0xF])
		b.WriteByte(hexDigits[(r>>4)&0xF])
		b.WriteByte(hexDigits[r&0xF])
	} else {
		b.WriteString(`\U`)
		b.WriteByte(hexDigits[(r>>28)&0xF])
		b.WriteByte(hexDigits[(r>>24)&0xF])
		b.WriteByte(hexDigits[(r>>20)&0xF])
		b.WriteByte(hexDigits[(r>>16)&0xF])
		b.WriteByte(hexDigits[(r>>12)&0xF])
		b.WriteByte(hexDigits[(r>>8)&0xF])
		b.WriteByte(hexDigits[(r>>4)&0xF])
		b.WriteByte(hexDigits[r&0xF])
	}
}

// SanitizeMap returns a copy of m with all values sanitized for logging.
// Strings and []string are sanitized element-wise; all other types are
// converted to string via fmt.Sprint and then sanitized, ensuring no
// control characters from untrusted input reach the logger.
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
			out[k] = Sanitize(fmt.Sprint(v))
		}
	}
	return out
}
