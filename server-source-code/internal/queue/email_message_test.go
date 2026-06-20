package queue

import (
	"strings"
	"testing"
)

func TestBuildEmailMessageWrapsLongHTMLLines(t *testing.T) {
	html := "<html><body><p>" + strings.Repeat("a", 2500) + "</p></body></html>"

	msg := string(buildEmailMessage(
		"patchmon@example.com",
		"ops@example.com",
		"Compliance scan completed",
		html,
	))

	if !strings.Contains(msg, "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Fatal("expected quoted-printable transfer encoding")
	}
	if !strings.Contains(msg, "=\r\n") {
		t.Fatal("expected quoted-printable soft line breaks")
	}
	assertSMTPLineLengths(t, msg)
}

func TestBuildEmailMessageFoldsAndSanitizesSubject(t *testing.T) {
	msg := string(buildEmailMessage(
		"patchmon@example.com",
		"ops@example.com",
		"[ERROR] "+strings.Repeat("very-long-hostname ", 20)+"\r\nBcc: injected@example.com",
		"<html><body>short body</body></html>",
	))

	if strings.Contains(msg, "\r\nBcc:") {
		t.Fatal("subject newline created an injected Bcc header")
	}
	if strings.Count(msg, "\r\n\t") == 0 {
		t.Fatal("expected long subject to be folded onto continuation lines")
	}
	assertSMTPLineLengths(t, msg)
}

func assertSMTPLineLengths(t *testing.T, msg string) {
	t.Helper()
	for i, line := range strings.Split(msg, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("SMTP line %d is %d octets, want <= 998", i+1, len(line))
		}
	}
}
