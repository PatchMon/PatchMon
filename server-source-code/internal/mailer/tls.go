package mailer

import (
	"crypto/tls"
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
// which is what we want — TLSModeNone with creds is already rejected upstream
// in validate(), and TLSModeAuto with creds will only authenticate after a
// successful upgrade.
func plainAuth(cfg Config) smtp.Auth {
	return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
}
