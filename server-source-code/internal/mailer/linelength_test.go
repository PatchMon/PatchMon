package mailer

import (
	"io"
	"mime/quotedprintable"
	"strings"
	"testing"
)

// assertSMTPLineLengths is carried across from @veteranbv's #861, which was
// closed only because it patched a file that no longer builds messages.
func assertSMTPLineLengths(t *testing.T, msg string) {
	t.Helper()
	for i, line := range strings.Split(msg, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("SMTP line %d is %d octets, want <= 998", i+1, len(line))
		}
	}
}

// #845: a generated host_down body put more than 998 octets on one line, and
// the body was written raw with no Content-Transfer-Encoding, so a compliant
// MTA rejected the whole message with "maximum allowed line length is 998
// octets". Shorter host_recovered alerts for the same hosts got through, which
// is what pointed at the body rather than the headers.
func TestRenderMessageWrapsLongBodyLines(t *testing.T) {
	t.Parallel()

	// One unbroken line, as the alert templates produce.
	body := "<html><body><p>" + strings.Repeat("host-that-went-down.example.internal ", 80) + "</p></body></html>"
	if len(body) <= 998 {
		t.Fatalf("fixture is only %d octets; it must exceed 998 to exercise the bug", len(body))
	}

	out := string(renderMessage(
		Config{From: "alerts@example.com", FromName: "PatchMon"},
		Message{To: "ops@example.com", Subject: "host down", HTMLBody: body},
	))

	if !strings.Contains(out, "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Error("missing quoted-printable transfer encoding")
	}
	if !strings.Contains(out, "=\r\n") {
		t.Error("expected quoted-printable soft line breaks")
	}
	assertSMTPLineLengths(t, out)
}

// The encoding is only useful if the receiver gets the original bytes back.
func TestRenderMessageBodyRoundTrips(t *testing.T) {
	t.Parallel()

	body := "<html><body><p>" + strings.Repeat("a", 2500) + "</p><p>café = 100% ✓</p></body></html>"

	out := string(renderMessage(
		Config{From: "alerts@example.com"},
		Message{To: "ops@example.com", Subject: "s", HTMLBody: body},
	))

	_, encoded, found := strings.Cut(out, "\r\n\r\n")
	if !found {
		t.Fatal("no header/body separator")
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(encoded)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != body {
		t.Errorf("body did not round-trip through quoted-printable")
	}
}

// A long subject is the other way to breach the limit, and the reporter could
// not tell from the rejection which one they had hit.
func TestRenderMessageFoldsLongSubject(t *testing.T) {
	t.Parallel()

	subject := "[ERROR] " + strings.Repeat("very-long-hostname.example.internal ", 40)

	out := string(renderMessage(
		Config{From: "alerts@example.com"},
		Message{To: "ops@example.com", Subject: subject, HTMLBody: "<p>short</p>"},
	))

	if !strings.Contains(out, "\r\n\t") {
		t.Error("long subject was not folded onto continuation lines")
	}
	assertSMTPLineLengths(t, out)
}

// Folding must not become a header-injection route.
func TestRenderMessageSubjectCannotInjectHeaders(t *testing.T) {
	t.Parallel()

	out := string(renderMessage(
		Config{From: "alerts@example.com"},
		Message{
			To:       "ops@example.com",
			Subject:  "alert\r\nBcc: attacker@example.com",
			HTMLBody: "<p>short</p>",
		},
	))

	if strings.Contains(out, "\r\nBcc:") {
		t.Error("subject newline created an injected Bcc header")
	}
	assertSMTPLineLengths(t, out)
}

// A non-ASCII display name must be RFC 2047 encoded rather than emitted as raw
// 8-bit, and an encoded-word must not sit inside quotes.
func TestRenderMessageEncodesNonASCIIFromName(t *testing.T) {
	t.Parallel()

	out := string(renderMessage(
		Config{From: "alerts@example.com", FromName: "PatchMon Alertes Systèmes"},
		Message{To: "ops@example.com", Subject: "s", HTMLBody: "<p>short</p>"},
	))

	if !strings.Contains(out, "=?utf-8?q?") {
		t.Error("non-ASCII display name was not RFC 2047 encoded")
	}
	if strings.Contains(out, `"=?utf-8?q?`) {
		t.Error("encoded-word must not be wrapped in quotes")
	}
	assertSMTPLineLengths(t, out)
}
