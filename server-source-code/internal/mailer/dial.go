package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"time"
)

// dialResult bundles the SMTP client with the underlying net.Conn so the
// caller can close both regardless of which dial path was taken.
type dialResult struct {
	client  *smtp.Client
	netConn net.Conn
}

// dialNone opens a cleartext TCP connection, hands it to net/smtp, and never
// upgrades. Credentials with this mode are rejected in validate() before we
// reach this code path.
func dialNone(ctx context.Context, addr string, cfg Config, dialTimeout time.Duration) (dialResult, *SendError) {
	d := &net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return dialResult{}, newSendError(StageDial, err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return dialResult{}, newSendError(StageDial, err)
	}
	return dialResult{client: client, netConn: conn}, nil
}

// dialStartTLS opens cleartext, then mandates STARTTLS. If the server does not
// advertise STARTTLS we fail closed at StageStartTLS — silently downgrading
// would defeat the purpose of choosing this mode.
func dialStartTLS(ctx context.Context, addr string, cfg Config, dialTimeout time.Duration) (dialResult, *SendError) {
	d := &net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return dialResult{}, newSendError(StageDial, err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return dialResult{}, newSendError(StageDial, err)
	}
	ok, _ := client.Extension("STARTTLS")
	if !ok {
		_ = client.Close()
		return dialResult{}, newSendError(StageStartTLS, errors.New("server does not advertise STARTTLS"))
	}
	if err := client.StartTLS(tlsClientConfig(cfg.Host)); err != nil {
		_ = client.Close()
		return dialResult{}, newSendError(StageStartTLS, err)
	}
	return dialResult{client: client, netConn: conn}, nil
}

// dialImplicitTLS dials with TLS from the first byte (port 465 style).
func dialImplicitTLS(ctx context.Context, addr string, cfg Config, dialTimeout time.Duration) (dialResult, *SendError) {
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: dialTimeout},
		Config:    tlsClientConfig(cfg.Host),
	}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return dialResult{}, newSendError(StageDial, err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return dialResult{}, newSendError(StageDial, err)
	}
	return dialResult{client: client, netConn: conn}, nil
}

// dialAuto preserves PatchMon's pre-tls-mode behaviour: try STARTTLS first,
// fall back to implicit TLS on the same address when STARTTLS is not offered.
// This keeps existing destinations working without requiring an immediate
// migration.
func dialAuto(ctx context.Context, addr string, cfg Config, dialTimeout time.Duration) (dialResult, *SendError) {
	d := &net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return dialResult{}, newSendError(StageDial, err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return dialResult{}, newSendError(StageDial, err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsClientConfig(cfg.Host)); err != nil {
			_ = client.Close()
			return dialResult{}, newSendError(StageStartTLS, err)
		}
		return dialResult{client: client, netConn: conn}, nil
	}
	_ = client.Close()
	return dialImplicitTLS(ctx, addr, cfg, dialTimeout)
}
