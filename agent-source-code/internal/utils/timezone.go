package utils

import (
	"os"
	"time"
)

// GetTimezone returns the configured timezone from environment variable
// Defaults to UTC if not set
func GetTimezone() string {
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = os.Getenv("TIMEZONE")
	}
	if tz == "" {
		return "UTC"
	}
	return tz
}

// GetTimezoneLocation returns a time.Location for the configured timezone
// Defaults to UTC if not set or invalid
func GetTimezoneLocation() *time.Location {
	tz := GetTimezone()

	// Handle UTC explicitly
	if tz == "UTC" || tz == "Etc/UTC" {
		return time.UTC
	}

	// Try to load the timezone
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Fallback to UTC if timezone is invalid
		return time.UTC
	}

	return loc
}

// GetCurrentTime returns the current time in the configured timezone
// For database storage, we should use UTC, but this function returns
// the time in the configured timezone for display purposes
func GetCurrentTime() time.Time {
	loc := GetTimezoneLocation()
	return time.Now().In(loc)
}

// GetCurrentTimeUTC returns the current time in UTC
// This should be used for database storage to ensure consistency
func GetCurrentTimeUTC() time.Time {
	return time.Now().UTC()
}

// FormatTimeISO formats a time to ISO 8601 string
func FormatTimeISO(t time.Time) string {
	return t.Format(time.RFC3339)
}
