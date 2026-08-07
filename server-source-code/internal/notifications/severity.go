package notifications

import "strings"

// SeverityRank maps severities for route filtering (higher = more urgent).
func SeverityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return 4
	case "error":
		return 3
	case "warning", "warn":
		return 2
	case "informational", "info", "":
		return 1
	default:
		return 1
	}
}
