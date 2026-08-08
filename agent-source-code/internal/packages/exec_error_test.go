package packages

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestIsExitCode guards the dnf/yum check-update contract.
func TestIsExitCode(t *testing.T) {
	t.Parallel()

	updatesAvailable := exec.Command("sh", "-c", "exit 100").Run()
	if !isExitCode(updatesAvailable, 100) {
		t.Fatalf("expected exit 100 to be recognised, got %v", updatesAvailable)
	}
	if isExitCode(updatesAvailable, 1) {
		t.Fatalf("exit 100 must not be mistaken for exit 1")
	}

	realFailure := exec.Command("sh", "-c", "exit 1").Run()
	if isExitCode(realFailure, 100) {
		t.Fatalf("exit 1 must not be mistaken for the updates-available signal")
	}

	if isExitCode(nil, 100) {
		t.Fatalf("a nil error carries no exit code")
	}
	if isExitCode(errors.New("not an exec error"), 100) {
		t.Fatalf("a non-exec error carries no exit code")
	}
}

// TestCommandError_IncludesStderr covers the diagnostic that was previously
// lost.
func TestCommandError_IncludesStderr(t *testing.T) {
	t.Parallel()

	const diagnostic = "E: Unable to correct problems, you have held broken packages"
	_, err := exec.Command("sh", "-c", "echo '"+diagnostic+"' >&2; exit 100").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("apt upgrade simulation", err).Error()
	if !strings.Contains(got, "apt upgrade simulation") {
		t.Errorf("expected the command name in %q", got)
	}
	if !strings.Contains(got, "exit status 100") {
		t.Errorf("expected the wrapped exec error in %q", got)
	}
	if !strings.Contains(got, "held broken packages") {
		t.Errorf("expected stderr to be folded in, got %q", got)
	}
}

// TestCommandError_TruncatesLargeStderr keeps a pathological dependency-
// resolution trace out of the logs while retaining the leading diagnostic.
func TestCommandError_TruncatesLargeStderr(t *testing.T) {
	t.Parallel()

	_, err := exec.Command("sh", "-c", "head -c 4000 /dev/zero | tr '\\0' 'x' >&2; exit 1").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("dnf check-update", err).Error()
	if !strings.Contains(got, "(truncated)") {
		t.Errorf("expected oversized stderr to be truncated, got %d chars", len(got))
	}
	if len(got) > maxStderrInError+256 {
		t.Errorf("truncated error is still too large: %d chars", len(got))
	}
}

// TestCommandError_NoStderr keeps the message clean when the command produced
// no diagnostic at all.
func TestCommandError_NoStderr(t *testing.T) {
	t.Parallel()

	_, err := exec.Command("sh", "-c", "exit 3").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("apk list --installed", err).Error()
	if strings.HasSuffix(got, ": ") || strings.Contains(got, ":  ") {
		t.Errorf("expected no dangling separator when stderr is empty, got %q", got)
	}
	if !strings.Contains(got, "exit status 3") {
		t.Errorf("expected the exec error in %q", got)
	}
}

// TestCommandError_RedactsRepositoryCredentials keeps secrets out of the
// agent log.
func TestCommandError_RedactsRepositoryCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		stderr      string
		mustNotHave []string
		mustHave    string
	}{
		{
			name:        "https basic auth",
			stderr:      "Failed to download metadata for repo 'internal': https://svc-patching:s3cr3t-token@repo.example.com/rhel9/os",
			mustNotHave: []string{"s3cr3t-token", "svc-patching"},
			mustHave:    "repo.example.com",
		},
		{
			name:        "token only",
			stderr:      "curl error on https://abcdef1234567890@satellite.example.com/pulp/repos/x",
			mustNotHave: []string{"abcdef1234567890"},
			mustHave:    "satellite.example.com",
		},
		{
			name:        "no credentials is left alone",
			stderr:      "Failed to download metadata for repo 'baseos': https://mirror.example.com/rhel9/os",
			mustNotHave: []string{"[redacted]"},
			mustHave:    "mirror.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := exec.Command("sh", "-c", "printf '%s' "+shellQuote(tc.stderr)+" >&2; exit 1").Output()
			if err == nil {
				t.Fatal("expected the command to fail")
			}
			got := commandError("dnf check-update", err).Error()

			for _, secret := range tc.mustNotHave {
				if strings.Contains(got, secret) {
					t.Errorf("error must not carry %q: %s", secret, got)
				}
			}
			if !strings.Contains(got, tc.mustHave) {
				t.Errorf("error should retain the host for diagnosis, got: %s", got)
			}
		})
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
