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
	"net/mail"
	"strings"
	"time"
)

// TLSMode selects how the client negotiates TLS with the SMTP server.
type TLSMode string

const (
	// TLSModeNone dials cleartext and never upgrades. Reject if credentials are
	// set (PLAIN over cleartext leaks them on the wire).
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
	if cfg.TLSMode == TLSModeNone &&
		(strings.TrimSpace(cfg.Username) != "" || strings.TrimSpace(cfg.Password) != "") {
		return newSendError(StageValidate,
			errors.New("refusing PLAIN auth over cleartext: tls_mode=none with credentials would leak them on the wire"))
	}
	return nil
}

// renderMessage builds the full RFC 5322 byte stream including headers.
// Subject is stripped of CR/LF to prevent SMTP header injection. From uses the
// FromName when present (encoded as a name-addr pair).
func renderMessage(cfg Config, msg Message) []byte {
	subject := strings.NewReplacer("\r", "", "\n", "").Replace(msg.Subject)
	from := cfg.From
	if strings.TrimSpace(cfg.FromName) != "" {
		cleanName := strings.NewReplacer("\r", "", "\n", "").Replace(cfg.FromName)
		from = fmt.Sprintf("%q <%s>", cleanName, cfg.From)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTMLBody)
	return []byte(b.String())
}
