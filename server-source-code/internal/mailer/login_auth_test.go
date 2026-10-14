package mailer

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedAuthServer advertises a configurable AUTH list and runs the SASL
// LOGIN exchange step by step, recording both sides of the conversation.
type scriptedAuthServer struct {
	mechs      string
	startTLS   bool
	tlsConfig  *tls.Config
	userPrompt string
	passPrompt string
	wantUser   string
	wantPass   string

	mu         sync.Mutex
	transcript []string
	delivered  bool
	addr       string
}

func newScriptedAuthServer(t *testing.T, s *scriptedAuthServer) *scriptedAuthServer {
	t.Helper()
	if s.tlsConfig == nil {
		s.tlsConfig = selfSignedTLS(t)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.addr = ln.Addr().String()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.handle(c)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return s
}

func (s *scriptedAuthServer) log(line string) {
	s.mu.Lock()
	s.transcript = append(s.transcript, line)
	s.mu.Unlock()
}

func (s *scriptedAuthServer) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.transcript...)
}

func (s *scriptedAuthServer) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	write := func(line string) bool {
		s.log("S: " + line)
		_, err := c.Write([]byte(line + "\r\n"))
		return err == nil
	}
	read := func() (string, bool) {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", false
		}
		line = strings.TrimRight(line, "\r\n")
		s.log("C: " + line)
		return line, true
	}
	b64 := func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) }

	if !write("220 scripted.local ESMTP") {
		return
	}
	for {
		line, ok := read()
		if !ok {
			return
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			_ = write("250-scripted.local")
			if s.startTLS {
				_ = write("250-STARTTLS")
			}
			if s.mechs != "" {
				_ = write("250-AUTH " + s.mechs)
			}
			_ = write("250 SIZE 10485760")
		case upper == "STARTTLS":
			_ = write("220 Ready to start TLS")
			tc := tls.Server(c, s.tlsConfig)
			if tc.Handshake() != nil {
				return
			}
			c = tc
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			br = bufio.NewReader(c)
		case upper == "AUTH LOGIN":
			_ = write("334 " + b64(s.userPrompt))
			u, ok := read()
			if !ok {
				return
			}
			_ = write("334 " + b64(s.passPrompt))
			p, ok := read()
			if !ok {
				return
			}
			du, _ := base64.StdEncoding.DecodeString(u)
			dp, _ := base64.StdEncoding.DecodeString(p)
			if string(du) == s.wantUser && string(dp) == s.wantPass {
				_ = write("235 2.7.0 Authentication successful")
			} else {
				_ = write("535 5.7.8 Error: authentication failed")
			}
		case strings.HasPrefix(upper, "AUTH PLAIN ") && strings.Contains(" "+strings.ToUpper(s.mechs)+" ", " PLAIN "):
			_ = write("235 2.7.0 Authentication successful")
		case strings.HasPrefix(upper, "AUTH"):
			_ = write("504 5.5.4 Unrecognized authentication type")
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			_ = write("250 OK")
		case upper == "DATA":
			_ = write("354 End data with <CR><LF>.<CR><LF>")
			for {
				l, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
			}
			s.mu.Lock()
			s.delivered = true
			s.mu.Unlock()
			_ = write("250 OK queued")
		case upper == "QUIT":
			_ = write("221 Bye")
			return
		default:
			_ = write("502 5.5.2 Command not recognised")
		}
	}
}

func scriptedSend(t *testing.T, s *scriptedAuthServer, mode TLSMode, password string) error {
	t.Helper()
	host, port := hostPort(t, s.addr)
	cfg := Config{
		Host:              host,
		Port:              port,
		From:              "alerts@example.com",
		Username:          "relay-user",
		Password:          password,
		TLSMode:           mode,
		AllowInsecureAuth: mode == TLSModeNone,
		DialTimeout:       2 * time.Second,
		SendTimeout:       5 * time.Second,
	}
	var err error
	withSkipVerify(t, func() {
		err = Send(context.Background(), cfg, Message{To: "ops@example.com", Subject: "x", HTMLBody: "<p>x</p>"})
	})
	return err
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

func TestSend_LoginOnlyRelay_StartTLS(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{
		mechs: "LOGIN", startTLS: true,
		userPrompt: "Username:", passPrompt: "Password:",
		wantUser: "relay-user", wantPass: "relay-pass",
	})

	if err := scriptedSend(t, srv, TLSModeStartTLS, "relay-pass"); err != nil {
		t.Fatalf("send: %v", err)
	}

	lines := srv.lines()
	want := []string{
		"C: AUTH LOGIN",
		"S: 334 VXNlcm5hbWU6",
		"C: " + base64.StdEncoding.EncodeToString([]byte("relay-user")),
		"S: 334 UGFzc3dvcmQ6",
		"C: " + base64.StdEncoding.EncodeToString([]byte("relay-pass")),
		"S: 235 2.7.0 Authentication successful",
	}
	start := -1
	for i, l := range lines {
		if l == want[0] {
			start = i
			break
		}
	}
	if start < 0 || start+len(want) > len(lines) {
		t.Fatalf("AUTH LOGIN exchange missing from transcript:\n%s", strings.Join(lines, "\n"))
	}
	for i, w := range want {
		if lines[start+i] != w {
			t.Errorf("transcript[%d] = %q, want %q", start+i, lines[start+i], w)
		}
	}
	if !containsLine(lines[:start], "C: STARTTLS") {
		t.Error("AUTH LOGIN must only happen after STARTTLS")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.delivered {
		t.Error("message was not delivered")
	}
}

func TestSend_LoginPromptHandling(t *testing.T) {
	cases := []struct {
		name       string
		userPrompt string
		passPrompt string
	}{
		{"conventional", "Username:", "Password:"},
		{"lower case", "username:", "password:"},
		{"upper case", "USERNAME:", "PASSWORD:"},
		{"draft wording", "User Name", "Password"},
		{"empty challenges fall back to order", "", ""},
		{"unrecognised challenges fall back to order", "Who?", "Secret?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newScriptedAuthServer(t, &scriptedAuthServer{
				mechs:      "LOGIN",
				userPrompt: tc.userPrompt, passPrompt: tc.passPrompt,
				wantUser: "relay-user", wantPass: "relay-pass",
			})
			if err := scriptedSend(t, srv, TLSModeNone, "relay-pass"); err != nil {
				t.Fatalf("send: %v\n%s", err, strings.Join(srv.lines(), "\n"))
			}
		})
	}
}

func TestSend_PlainPreferredWhenBothOffered(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{mechs: "LOGIN PLAIN", startTLS: true})

	if err := scriptedSend(t, srv, TLSModeStartTLS, "relay-pass"); err != nil {
		t.Fatalf("send: %v", err)
	}
	for _, l := range srv.lines() {
		if l == "C: AUTH LOGIN" {
			t.Fatal("LOGIN used although PLAIN was offered")
		}
		if strings.HasPrefix(l, "C: AUTH PLAIN ") {
			return
		}
	}
	t.Fatal("no AUTH PLAIN in transcript")
}

func TestSend_LoginWrongPassword(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{
		mechs: "LOGIN", startTLS: true,
		userPrompt: "Username:", passPrompt: "Password:",
		wantUser: "relay-user", wantPass: "relay-pass",
	})

	err := scriptedSend(t, srv, TLSModeStartTLS, "wrong")
	var se *SendError
	if !errors.As(err, &se) || se.Stage != StageAuth {
		t.Fatalf("want StageAuth SendError, got %v", err)
	}
	if !strings.Contains(err.Error(), "535") {
		t.Errorf("error should carry the relay's 535 reply: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.delivered {
		t.Error("message delivered despite failed auth")
	}
}

func TestSend_NoSupportedMechanism(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{mechs: "CRAM-MD5 XOAUTH2", startTLS: true})

	err := scriptedSend(t, srv, TLSModeStartTLS, "relay-pass")
	var se *SendError
	if !errors.As(err, &se) || se.Stage != StageAuth {
		t.Fatalf("want StageAuth SendError, got %v", err)
	}
	for _, want := range []string{"no supported AUTH mechanism", "CRAM-MD5 XOAUTH2", "PLAIN, LOGIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err, want)
		}
	}
	for _, l := range srv.lines() {
		if strings.HasPrefix(l, "C: AUTH") {
			t.Errorf("client sent %q although no mechanism was usable", l)
		}
	}
}

func TestSelectAuth(t *testing.T) {
	cfg := Config{Host: "relay.internal", Username: "u", Password: "p", TLSMode: TLSModeStartTLS}

	cases := []struct {
		name      string
		mechs     string
		wantLogin bool
		wantErr   bool
	}{
		{"plain only", "PLAIN", false, false},
		{"plain and login", "PLAIN LOGIN", false, false},
		{"login listed first still prefers plain", "LOGIN PLAIN", false, false},
		{"login only", "LOGIN", true, false},
		{"login only lower case", "login", true, false},
		{"login among unsupported", "XOAUTH2 LOGIN", true, false},
		{"empty list keeps plain", "", false, false},
		{"nothing supported", "CRAM-MD5 XOAUTH2", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectAuth(cfg, tc.mechs)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %T", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectAuth: %v", err)
			}
			_, isLogin := got.(*loginAuth)
			if isLogin != tc.wantLogin {
				t.Errorf("login handle = %v, want %v (%T)", isLogin, tc.wantLogin, got)
			}
		})
	}
}

// LOGIN must obey the same opt-in scoping as PLAIN: only tls_mode=none with the
// flag set may skip the TLS check.
func TestSelectAuth_LoginInsecureScoping(t *testing.T) {
	tests := []struct {
		mode  TLSMode
		allow bool
		want  bool
	}{
		{TLSModeNone, true, true},
		{TLSModeNone, false, false},
		{TLSModeStartTLS, true, false},
		{TLSModeTLS, true, false},
		{TLSModeAuto, true, false},
	}
	for _, tt := range tests {
		got, err := selectAuth(Config{Host: "relay.internal", TLSMode: tt.mode, AllowInsecureAuth: tt.allow}, "LOGIN")
		if err != nil {
			t.Fatalf("selectAuth: %v", err)
		}
		if la := got.(*loginAuth); la.allowInsecureAuth != tt.want {
			t.Errorf("mode %q allow %v: allowInsecureAuth = %v, want %v", tt.mode, tt.allow, la.allowInsecureAuth, tt.want)
		}
	}
}

func TestLoginAuth_Start(t *testing.T) {
	cases := []struct {
		name    string
		auth    loginAuth
		server  smtp.ServerInfo
		wantErr string
	}{
		{"tls and matching host", loginAuth{host: "relay.internal"}, smtp.ServerInfo{Name: "relay.internal", TLS: true}, ""},
		{"cleartext refused", loginAuth{host: "relay.internal"}, smtp.ServerInfo{Name: "relay.internal"}, "unencrypted connection"},
		{"wrong host refused", loginAuth{host: "relay.internal"}, smtp.ServerInfo{Name: "evil.example", TLS: true}, "wrong host name"},
		{"localhost cleartext allowed", loginAuth{host: "localhost"}, smtp.ServerInfo{Name: "localhost"}, ""},
		{"opt-in allows cleartext", loginAuth{host: "relay.internal", allowInsecureAuth: true}, smtp.ServerInfo{Name: "relay.internal"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.auth
			mech, resp, err := a.Start(&tc.server)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			if mech != "LOGIN" || resp != nil {
				t.Errorf("Start = (%q, %q), want (LOGIN, nil)", mech, resp)
			}
		})
	}
}

func TestLoginAuth_Next(t *testing.T) {
	a := &loginAuth{username: "user", password: "pass", host: "h"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "h", TLS: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if resp, err := a.Next([]byte("Password:"), true); err != nil || string(resp) != "pass" {
		t.Fatalf("password prompt first: got (%q, %v)", resp, err)
	}
	if resp, err := a.Next([]byte("Username:"), true); err != nil || string(resp) != "user" {
		t.Fatalf("username prompt second: got (%q, %v)", resp, err)
	}
	if resp, err := a.Next([]byte("Password:"), true); err == nil || resp != nil {
		t.Fatalf("third challenge must be rejected, got (%q, %v)", resp, err)
	}
	if resp, err := a.Next(nil, false); err != nil || resp != nil {
		t.Fatalf("final Next: got (%q, %v)", resp, err)
	}
}

func TestLoginAuth_NextEmptyCredentialIsNonNil(t *testing.T) {
	a := &loginAuth{username: "user", password: "", host: "h"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "h", TLS: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := a.Next([]byte("Username:"), true); err != nil {
		t.Fatalf("username: %v", err)
	}
	resp, err := a.Next([]byte("Password:"), true)
	if err != nil || resp == nil || len(resp) != 0 {
		t.Fatalf("empty password must be a non-nil empty response, got (%#v, %v)", resp, err)
	}
}

func TestSend_Auto_LoginOnlyWithoutTLS_SendsNoAuth(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{
		mechs:      "LOGIN",
		userPrompt: "Username:", passPrompt: "Password:",
		wantUser: "relay-user", wantPass: "relay-pass",
	})

	if err := scriptedSend(t, srv, TLSModeAuto, "relay-pass"); err == nil {
		t.Fatal("auto mode must not deliver to a relay that offers no TLS")
	}
	for _, l := range srv.lines() {
		if strings.HasPrefix(strings.ToUpper(l), "C: AUTH") {
			t.Fatalf("client sent %q to a relay with no TLS", l)
		}
	}
}

func TestSend_Auto_LoginOnlyAuthOnlyAfterSTARTTLS(t *testing.T) {
	srv := newScriptedAuthServer(t, &scriptedAuthServer{
		mechs: "LOGIN", startTLS: true,
		userPrompt: "Username:", passPrompt: "Password:",
		wantUser: "relay-user", wantPass: "relay-pass",
	})

	if err := scriptedSend(t, srv, TLSModeAuto, "relay-pass"); err != nil {
		t.Fatalf("send: %v", err)
	}
	lines := srv.lines()
	tlsAt, authAt := -1, -1
	for i, l := range lines {
		switch {
		case l == "C: STARTTLS" && tlsAt < 0:
			tlsAt = i
		case strings.HasPrefix(l, "C: AUTH") && authAt < 0:
			authAt = i
		}
	}
	if tlsAt < 0 || authAt < 0 || authAt < tlsAt {
		t.Fatalf("AUTH must follow STARTTLS (STARTTLS at %d, AUTH at %d):\n%s", tlsAt, authAt, strings.Join(lines, "\n"))
	}
	if lines[authAt] != "C: AUTH LOGIN" {
		t.Errorf("first AUTH line = %q, want AUTH LOGIN", lines[authAt])
	}
}
