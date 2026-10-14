package util

import (
	"regexp"
	"strings"
)

var (
	versionPrefixRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)
	versionStringRe = regexp.MustCompile(`^\d+\.\d+\.\d+[0-9A-Za-z.+-]*$`)
	hostIDRe        = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

// HasVersionPrefix reports whether v starts with a numeric N.N.N core. It is a
// prefix match, so pre-release and build suffixes pass.
func HasVersionPrefix(v string) bool {
	return versionPrefixRe.MatchString(v)
}

// ValidConfigVersion reports whether v is an acceptable agent config_version:
// an N.N.N core, an optional pre-release or build suffix, at most 64 bytes.
func ValidConfigVersion(v string) bool {
	return len(v) <= 64 && versionStringRe.MatchString(v)
}

// ValidHostID reports whether id has the shape of a host ID an agent may
// claim: 1 to 64 letters, digits or hyphens.
func ValidHostID(id string) bool {
	return hostIDRe.MatchString(id)
}

var junkMachineIDs = map[string]bool{
	"uninitialized":                    true,
	"03000200040005000006000700080009": true,
}

// NormaliseMachineID lowercases id and drops dashes and surrounding space. It
// returns "" for values that do not identify a machine: empty, "uninitialized",
// all zeros or all Fs, and vendor default SMBIOS UUIDs shared by many machines.
func NormaliseMachineID(id string) string {
	n := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(id), "-", ""))
	if n == "" || junkMachineIDs[n] || strings.Trim(n, "0") == "" || strings.Trim(n, "f") == "" {
		return ""
	}
	return n
}
