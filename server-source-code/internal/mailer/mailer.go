// Package mailer is a transport-only SMTP sender. It speaks RFC 5321 via the
// stdlib net/smtp package, supports four explicit TLS modes (none, starttls,
// implicit tls, auto/legacy-opportunistic), and refuses PLAIN auth over
// cleartext. Higher-level concerns (templating, queueing, retry policy,
// logging of delivery outcomes) belong to the caller.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// TLSMode selects how the client negotiates TLS with the SMTP server.
type TLSMode string

const (
	// TLSModeNone dials cleartext and never upgrades. Reject if credentials are
	// set (PLAIN over cleartext leaks them on the wire) unless the operator has
	// explicitly set Config.AllowInsecureAuth.
	TLSModeNone TLSMode = "none"
	// TLSModeStartTLS dials cleartext then mandates STARTTLS. If the server
	// does not advertise STARTTLS the dial fails closed.
	TLSModeStartTLS TLSMode = "starttls"
	// TLSModeTLS dials with implicit TLS from the first byte (e.g. port 465).
	TLSModeTLS TLSMode = "tls"
	// TLSModeAuto preserves PatchMon's historical opportunistic behaviour:
	// dial cleartext, upgrade with STARTTLS when advertised, otherwise retry
	// with implicit TLS on the same host:port.
	TLSModeAuto TLSMode = "auto"
)

// Config holds the transport-level SMTP settings.
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	From        string
	FromName    string
	TLSMode     TLSMode
	DialTimeout time.Duration
	SendTimeout time.Duration
	// AllowInsecureAuth permits PLAIN authentication over an unencrypted
	// connection. It applies only to TLSModeNone and is an explicit per
	// destination opt-in for trusted internal relays that require AUTH but do
	// not offer TLS. It sends the credentials in the clear, so it stays false
	// unless the operator sets it.
	AllowInsecureAuth bool
}

// Message is one outbound email.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
}

// Stage identifies which step of the SMTP exchange failed; surfaced to the UI
// via the test endpoint so the operator can localise the misconfiguration.
type Stage string

const (
	StageValidate Stage = "validate"
	StageDial     Stage = "dial"
	StageStartTLS Stage = "starttls"
	StageAuth     Stage = "auth"
	StageSend     Stage = "send"
)

// SendError wraps a transport-level failure with the stage at which it occurred.
type SendError struct {
	Stage Stage
	Err   error
}

func (e *SendError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("smtp %s: <nil>", e.Stage)
	}
	return fmt.Sprintf("smtp %s: %s", e.Stage, e.Err.Error())
}

func (e *SendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newSendError(stage Stage, err error) *SendError {
	return &SendError{Stage: stage, Err: err}
}

// defaultDialTimeout / defaultSendTimeout match the original inline values in
// notification_worker.go so existing production behaviour is preserved when
// callers leave the timeouts at their zero value.
const (
	defaultDialTimeout = 10 * time.Second
	defaultSendTimeout = 30 * time.Second
)

// Send delivers msg using cfg. Transport errors are returned as *SendError so
// the caller can render the failing stage; argument validation errors come
// back the same way with Stage=StageValidate.
func Send(ctx context.Context, cfg Config, msg Message) error {
	if err := validate(cfg, msg); err != nil {
		return err
	}

	dial := cfg.DialTimeout
	if dial <= 0 {
		dial = defaultDialTimeout
	}
	send := cfg.SendTimeout
	if send <= 0 {
		send = defaultSendTimeout
	}

	deadline := time.Now().Add(dial + send)
	dialCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var (
		conn dialResult
		err  *SendError
	)
	switch cfg.TLSMode {
	case TLSModeNone:
		conn, err = dialNone(dialCtx, addr, cfg, dial)
	case TLSModeStartTLS:
		conn, err = dialStartTLS(dialCtx, addr, cfg, dial)
	case TLSModeTLS:
		conn, err = dialImplicitTLS(dialCtx, addr, cfg, dial)
	case TLSModeAuto:
		conn, err = dialAuto(dialCtx, addr, cfg, dial)
	default:
		return newSendError(StageValidate, fmt.Errorf("unknown TLS mode %q", cfg.TLSMode))
	}
	if err != nil {
		return err
	}

	defer func() { _ = conn.client.Close() }()
	defer func() { _ = conn.netConn.Close() }()

	if cfg.Username != "" || cfg.Password != "" {
		if ok, _ := conn.client.Extension("AUTH"); ok {
			if authErr := conn.client.Auth(plainAuth(cfg)); authErr != nil {
				return newSendError(StageAuth, authErr)
			}
		}
	}

	if mailErr := conn.client.Mail(cfg.From); mailErr != nil {
		return newSendError(StageSend, mailErr)
	}
	if rcptErr := conn.client.Rcpt(msg.To); rcptErr != nil {
		return newSendError(StageSend, rcptErr)
	}
	w, dataErr := conn.client.Data()
	if dataErr != nil {
		return newSendError(StageSend, dataErr)
	}
	// CodeQL flags this write as go/email-injection because untrusted values
	// reach it. Both halves of that rule are already guarded, and the guards
	// live too many frames upstream for its dataflow to see:
	//
	//   Headers — validate() runs mail.ParseAddress over cfg.From and msg.To,
	//   which rejects CR/LF, and renderMessage strips CR/LF from Subject and
	//   FromName. net/smtp's Mail and Rcpt reject CRLF independently.
	//
	//   Body — every value interpolated into msg.HTMLBody is escaped at the
	//   point of construction via notifications.TemplateEscape (see
	//   notifications/report_render.go and queue/notification_worker.go's
	//   buildEmailHTML). That helper escapes quotes as well as & < >, so it is
	//   safe in the attribute contexts those builders use.
	//
	// Anything that adds a new email body builder MUST escape through the same
	// helper, or this write becomes a genuine injection point.
	rendered := renderMessage(cfg, msg)
	if _, writeErr := w.Write(rendered); writeErr != nil {
		_ = w.Close()
		return newSendError(StageSend, writeErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		return newSendError(StageSend, closeErr)
	}
	return nil
}

// ResolveMode picks the effective TLSMode given a stored value, an optional
// legacy boolean, and the SMTP port.
//   - A recognised stored value wins outright.
//   - With no stored value, the legacy bool maps true→auto, false→none.
//   - With neither, port 587→starttls, 465→tls, anything else→auto.
//
// Garbage stored values fall through; validate() will surface the issue at
// send time rather than this function silently lying about the mode.
func ResolveMode(stored string, legacyUseTLS *bool, port int) TLSMode {
	switch TLSMode(strings.ToLower(strings.TrimSpace(stored))) {
	case TLSModeNone, TLSModeStartTLS, TLSModeTLS, TLSModeAuto:
		return TLSMode(strings.ToLower(strings.TrimSpace(stored)))
	}
	if legacyUseTLS != nil {
		if *legacyUseTLS {
			return TLSModeAuto
		}
		return TLSModeNone
	}
	switch port {
	case 587:
		return TLSModeStartTLS
	case 465:
		return TLSModeTLS
	default:
		return TLSModeAuto
	}
}

// validate enforces transport-level invariants before any network I/O.
func validate(cfg Config, msg Message) *SendError {
	if strings.TrimSpace(cfg.Host) == "" {
		return newSendError(StageValidate, errors.New("host is required"))
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return newSendError(StageValidate, fmt.Errorf("port %d out of range", cfg.Port))
	}
	if strings.TrimSpace(cfg.From) == "" {
		return newSendError(StageValidate, errors.New("from is required"))
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return newSendError(StageValidate, fmt.Errorf("invalid from %q: %w", cfg.From, err))
	}
	if strings.TrimSpace(msg.To) == "" {
		return newSendError(StageValidate, errors.New("to is required"))
	}
	if _, err := mail.ParseAddress(msg.To); err != nil {
		return newSendError(StageValidate, fmt.Errorf("invalid to %q: %w", msg.To, err))
	}
	if cfg.TLSMode == TLSModeNone && !cfg.AllowInsecureAuth &&
		(strings.TrimSpace(cfg.Username) != "" || strings.TrimSpace(cfg.Password) != "") {
		return newSendError(StageValidate,
			errors.New("refusing PLAIN auth over cleartext: tls_mode=none with credentials would leak them on the wire. Enable allow_insecure_auth on the destination if the relay is on a trusted network"))
	}
	return nil
}

// headerLineLimit is the soft wrap point for header lines. RFC 5322 recommends
// 78 octets; RFC 5321's hard limit is 998.
const headerLineLimit = 78

// renderMessage builds the full RFC 5322 byte stream including headers.
// Subject is stripped of CR/LF to prevent SMTP header injection. From uses the
// FromName when present (encoded as a name-addr pair).
func renderMessage(cfg Config, msg Message) []byte {
	subject := foldHeader("Subject", stripCRLF(msg.Subject))
	from := cfg.From
	if strings.TrimSpace(cfg.FromName) != "" {
		cleanName := stripCRLF(cfg.FromName)
		if encoded := mime.QEncoding.Encode("utf-8", cleanName); encoded != cleanName {
			// An RFC 2047 encoded-word must not be quoted.
			from = fmt.Sprintf("%s <%s>", encoded, cfg.From)
		} else {
			from = fmt.Sprintf("%q <%s>", cleanName, cfg.From)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	b.WriteString("\r\n")

	// RFC 5321 caps a line at 998 octets excluding CRLF, and a generated alert
	// body routinely puts more than that on one line. Without an encoding a
	// standards-compliant MTA is right to reject the whole message, which is
	// what #845 saw: host_down alerts bounced with "maximum allowed line length
	// is 998 octets" while shorter host_recovered alerts got through.
	// quoted-printable inserts soft breaks, so no line can exceed the limit.
	qp := quotedprintable.NewWriter(&b)
	if _, err := io.WriteString(qp, msg.HTMLBody); err != nil {
		// strings.Builder never fails; fall back to the raw body rather than
		// silently sending an empty one.
		_ = qp.Close()
		return []byte(b.String() + msg.HTMLBody)
	}
	if err := qp.Close(); err != nil {
		return []byte(b.String() + msg.HTMLBody)
	}
	return []byte(b.String())
}

func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// foldHeader keeps a header line inside the RFC 5321 limit by folding it onto
// continuation lines, and RFC 2047 encodes it when it is not plain ASCII. A
// long subject is a second way to breach 998 octets, and the reporter on #845
// could not tell from the rejection whether it was a header or the body.
func foldHeader(name, value string) string {
	if encoded := mime.QEncoding.Encode("utf-8", value); encoded != value {
		// QEncoding already splits into encoded-words short enough to fold.
		return strings.ReplaceAll(encoded, " ", "\r\n\t")
	}
	// +2 for the ": " separator.
	if len(name)+2+len(value) <= headerLineLimit {
		return value
	}

	var out strings.Builder
	lineLen := len(name) + 2
	for i, word := range strings.Fields(value) {
		switch {
		case i == 0:
			out.WriteString(word)
			lineLen += len(word)
		case lineLen+1+len(word) > headerLineLimit:
			out.WriteString("\r\n\t")
			out.WriteString(word)
			lineLen = 1 + len(word)
		default:
			out.WriteString(" ")
			out.WriteString(word)
			lineLen += 1 + len(word)
		}
	}
	return out.String()
}
