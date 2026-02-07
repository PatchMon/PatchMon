package utils

import (
	"hash/fnv"
	"time"
)

// CalculateReportOffset calculates a unique, deterministic offset for report timing
// based on the agent's api_id and the reporting interval. This ensures different
// agents report at staggered times to prevent overwhelming the server.
//
// For intervals >= 60 minutes: returns offset in minutes (0-59)
// For intervals < 60 minutes: returns offset in seconds (0 to interval*60-1)
//
// The same api_id will always produce the same offset, ensuring consistency
// across service restarts.
func CalculateReportOffset(apiId string, intervalMinutes int) time.Duration {
	// Hash the api_id to get a consistent numeric value
	hash := hashString(apiId)

	if intervalMinutes >= 60 {
		// For hourly or longer intervals, offset in minutes (0-59)
		// Example: api_id hash % 60 = 10 → reports at :10 past each hour
		offsetMinutes := hash % 60
		return time.Duration(offsetMinutes) * time.Minute
	} else {
		// For sub-hourly intervals, offset in seconds
		// Example: 5-minute interval, hash % 300 = 7 → reports at :07, :12, :17, etc.
		maxOffsetSeconds := intervalMinutes * 60
		offsetSeconds := hash % uint64(maxOffsetSeconds)
		return time.Duration(offsetSeconds) * time.Second
	}
}

// hashString creates a deterministic hash from a string using FNV-1a algorithm
// This ensures the same input always produces the same hash value
func hashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
