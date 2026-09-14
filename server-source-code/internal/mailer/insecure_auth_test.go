package mailer

import (
	"context"
	"encoding/base64"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

// The whole point of the opt-in: a trusted internal relay that requires AUTH but
// offers no TLS. Without AllowInsecureAuth this combination is refused, which is
// what issue #817 reported.
func TestSend_None_WithCreds_AllowInsecureAuth_Succeeds(t *testing.T) {
	srv := newFakeServer(t, false, false)
	host, port := hostPort(t, srv.addr)

	err := Send(context.Background(), Config{
		Host:              host,
		Port:              port,
		From:              "alerts@example.com",
		Username:          "relay-user",
		Password:          "relay-pass",
		TLSMode:           TLSModeNone,
		AllowInsecureAuth: true,
	}, Message{To: "ops@example.com", Subject: "x", HTMLBody: "<p>x</p>"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.gotStartTLS {
		t.Error("server should not have seen STARTTLS in tls_mode=none")
	}
	if !srv.gotAuth {
		t.Fatal("server never saw AUTH")
	}

	// Verify what actually went on the wire is well-formed SASL PLAIN.
	fields := strings.Fields(srv.authLine)
	if len(fields) != 3 || !strings.EqualFold(fields[0], "AUTH") || !strings.EqualFold(fields[1], "PLAIN") {
		t.Fatalf("unexpected AUTH line %q", srv.authLine)
	}
	decoded, decErr := base64.StdEncoding.DecodeString(fields[2])
	if decErr != nil {
		t.Fatalf("AUTH payload is not base64: %v", decErr)
	}
	if want := "\x00relay-user\x00relay-pass"; string(decoded) != want {
		t.Errorf("PLAIN payload = %q, want %q", decoded, want)
	}
}

// The opt-in must not become a general "skip TLS" switch. It is scoped to
// tls_mode=none, so every other mode keeps the stdlib handle and its refusal to
// send PLAIN over an unencrypted connection.
func TestPlainAuthSelection(t *testing.T) {
	tests := []struct {
		name         string
		mode         TLSMode
		allow        bool
		wantInsecure bool
	}{
		{"none with opt-in uses the insecure handle", TLSModeNone, true, true},
		{"none without opt-in uses the stdlib", TLSModeNone, false, false},
		{"starttls ignores the opt-in", TLSModeStartTLS, true, false},
		{"implicit tls ignores the opt-in", TLSModeTLS, true, false},
		{"auto ignores the opt-in", TLSModeAuto, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plainAuth(Config{
				Host:              "relay.internal",
				Username:          "u",
				Password:          "p",
				TLSMode:           tt.mode,
				AllowInsecureAuth: tt.allow,
			})
			_, isInsecure := got.(insecurePlainAuth)
			if isInsecure != tt.wantInsecure {
				t.Errorf("insecure handle = %v, want %v (%T)", isInsecure, tt.wantInsecure, got)
			}
		})
	}
}

// Guard the mode scoping at the validate layer too, so a caller that sets the
// flag on a TLS mode cannot slip past into an unintended code path.
func TestValidate_AllowInsecureAuthIsScopedToNone(t *testing.T) {
	msg := Message{To: "ops@example.com", Subject: "x", HTMLBody: "x"}
	base := Config{Host: "relay.internal", Port: 25, From: "alerts@example.com", Username: "u", Password: "p"}

	cfg := base
	cfg.TLSMode = TLSModeNone
	if err := validate(cfg, msg); err == nil {
		t.Error("tls_mode=none with creds and no opt-in must be refused")
	}

	cfg.AllowInsecureAuth = true
	if err := validate(cfg, msg); err != nil {
		t.Errorf("tls_mode=none with creds and the opt-in must pass validate: %v", err)
	}

	// TLS modes were never blocked by this rule, and enabling the flag must not
	// change that either way.
	for _, mode := range []TLSMode{TLSModeStartTLS, TLSModeTLS, TLSModeAuto} {
		cfg.TLSMode = mode
		if err := validate(cfg, msg); err != nil {
			t.Errorf("mode %q with the opt-in should still validate: %v", mode, err)
		}
	}
}

func TestInsecurePlainAuth_Start(t *testing.T) {
	a := insecurePlainAuth{username: "user", password: "pass"}
	proto, resp, err := a.Start(&smtp.ServerInfo{Name: "relay.internal", TLS: false})
	if err != nil {
		t.Fatalf("Start on an unencrypted connection must not error: %v", err)
	}
	if proto != "PLAIN" {
		t.Errorf("proto = %q, want PLAIN", proto)
	}
	if want := "\x00user\x00pass"; string(resp) != want {
		t.Errorf("resp = %q, want %q", resp, want)
	}
}

func TestInsecurePlainAuth_NextRejectsChallenge(t *testing.T) {
	a := insecurePlainAuth{username: "user", password: "pass"}

	if _, err := a.Next(nil, false); err != nil {
		t.Errorf("Next without more must succeed: %v", err)
	}

	resp, err := a.Next([]byte("challenge"), true)
	if err == nil {
		t.Fatal("Next must reject an unexpected server challenge")
	}
	if resp != nil {
		t.Errorf("Next must not return a response alongside an error, got %q", resp)
	}
}

// The scenario an operator creates when they enable the opt-in and then "fix"
// the mode: the flag is still set, but the relay offers neither STARTTLS nor
// implicit TLS. Every TLS mode must fail closed, and crucially must not put the
// credentials on the wire on the way out.
func TestSend_TLSModes_WithOptIn_FailClosedWithoutSendingCreds(t *testing.T) {
	for _, mode := range []TLSMode{TLSModeStartTLS, TLSModeTLS, TLSModeAuto} {
		t.Run(string(mode), func(t *testing.T) {
			srv := newFakeServer(t, false, false) // no STARTTLS, no implicit TLS
			host, port := hostPort(t, srv.addr)

			err := Send(context.Background(), Config{
				Host:              host,
				Port:              port,
				From:              "alerts@example.com",
				Username:          "relay-user",
				Password:          "relay-pass",
				TLSMode:           mode,
				AllowInsecureAuth: true,
				DialTimeout:       2 * time.Second,
				SendTimeout:       2 * time.Second,
			}, Message{To: "ops@example.com", Subject: "x", HTMLBody: "<p>x</p>"})
			if err == nil {
				t.Fatalf("mode %q must not deliver against a relay with no TLS", mode)
			}

			srv.mu.Lock()
			defer srv.mu.Unlock()
			if srv.gotAuth {
				t.Errorf("mode %q sent AUTH to a server with no TLS: %q", mode, srv.authLine)
			}
		})
	}
}
