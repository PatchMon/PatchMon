// Package social exposes community follower counts that are baked into the
// binary at build time.
//
// The server never contacts GitHub, Discord, YouTube or LinkedIn. The build
// reads https://patchmon.net/socialstats/<platform> once and injects the
// numbers through -ldflags, so self-hosted instances make no outbound calls,
// carry no API credentials, and work air-gapped.
package social

import (
	"math"
	"strconv"
	"strings"
)

// Injected at build time via -ldflags -X. See server-source-code/Makefile and
// docker/server.Dockerfile.
//
// Empty means the build supplied nothing (offline build, endpoint down, or a
// plain `go build`), and callers keep their compiled-in default. "0" means the
// endpoint reported an unknown count and the number should be hidden rather
// than shown stale.
var (
	GitHubStars        = ""
	DiscordMembers     = ""
	YouTubeSubscribers = ""
	LinkedInFollowers  = ""
)

// Count parses a raw injected value. ok is false when nothing usable was
// injected, which tells the caller to keep its own default.
func Count(raw string) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Format renders a count for display: 2712 becomes "2.7K", 3400000 becomes
// "3.4M", 612 stays "612", and anything at or below zero becomes "" so the UI
// hides it. Mirrors formatCount in the website's social-stats.ts.
func Format(n int) string {
	if n <= 0 {
		return ""
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}
	// 999500 rather than 1000000: above it the K form would round to "1000K".
	if n < 999500 {
		return unit(float64(n)/1000, "K")
	}
	return unit(float64(n)/1000000, "M")
}

func unit(value float64, suffix string) string {
	if value >= 10 {
		return strconv.Itoa(int(math.Round(value))) + suffix
	}
	return strconv.FormatFloat(math.Round(value*10)/10, 'g', -1, 64) + suffix
}
