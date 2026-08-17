package mailer

import (
	"crypto/tls"
	"errors"
	"net/smtp"
)

// testTLSOverride is set only by tests to inject InsecureSkipVerify against
// the in-process self-signed SMTP test server. Production code must never set
// this — the package guards against it indirectly by only mutating in _test.go.
var testTLSOverride func(*tls.Config)

// tlsClientConfig builds the *tls.Config used for STARTTLS and implicit TLS
// dials. Pinned to TLS 1.2 minimum; ServerName matches the configured host so
// certificate validation is performed against it rather than the IP.
func tlsClientConfig(host string) *tls.Config {
	c := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	if testTLSOverride != nil {
		testTLSOverride(c)
	}
	return c
}

// plainAuth returns a PLAIN auth handle bound to the configured host. The
// stdlib refuses to send PLAIN to anything that isn't TLS-wrapped or localhost,
// which is what we want by default — TLSModeNone with creds is rejected upstream
// in validate(), and TLSModeAuto with creds will only authenticate after a
// successful upgrade.
//
// The one exception is an operator who has explicitly opted in to cleartext auth
// for a trusted internal relay. The stdlib gives no way to waive its check, so
// that path uses insecurePlainAuth instead.
func plainAuth(cfg Config) smtp.Auth {
	if cfg.TLSMode == TLSModeNone && cfg.AllowInsecureAuth {
		return insecurePlainAuth{username: cfg.Username, password: cfg.Password}
	}
	return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
}

// insecurePlainAuth implements SASL PLAIN (RFC 4616) without the stdlib's
// refusal to run over an unencrypted connection. Reached only when the operator
// has set AllowInsecureAuth on a tls_mode=none destination; validate() rejects
// the combination otherwise.
//
// It also drops the stdlib's server.Name != host check. That check guards
// against sending credentials to a server other than the configured one, and is
// a tautology here: every dial passes cfg.Host to smtp.NewClient, so ServerInfo
// carries the same value it would be compared against.
type insecurePlainAuth struct {
	username string
	password string
}

func (a insecurePlainAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	// authzid NUL authcid NUL passwd, with an empty authzid.
	resp := []byte("\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a insecurePlainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		// PLAIN is a single round trip; a challenge means the server is not
		// speaking the mechanism we agreed.
		return nil, errors.New("unexpected server challenge during PLAIN auth")
	}
	return nil, nil
}
