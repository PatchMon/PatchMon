package mailer

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTPServer is a minimal scripted SMTP responder. It is NOT a real SMTP
// implementation; it just speaks enough of the protocol to drive net/smtp
// through HELO/EHLO/STARTTLS/AUTH/MAIL/RCPT/DATA/QUIT for these tests.
type fakeSMTPServer struct {
	advertiseStartTLS bool
	useImplicitTLS    bool
	tlsConfig         *tls.Config

	// recorded after the test exchange completes:
	mu          sync.Mutex
	gotStartTLS bool
	gotAuth     bool
	body        string
	listener    net.Listener
	addr        string
	wg          sync.WaitGroup
}

func newFakeServer(t *testing.T, advertiseStartTLS, useImplicitTLS bool) *fakeSMTPServer {
	t.Helper()
	s := &fakeSMTPServer{
		advertiseStartTLS: advertiseStartTLS,
		useImplicitTLS:    useImplicitTLS,
		tlsConfig:         selfSignedTLS(t),
	}
	var ln net.Listener
	var err error
	if useImplicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", s.tlsConfig)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.listener = ln
	s.addr = ln.Addr().String()
	s.wg.Add(1)
	go s.accept()
	t.Cleanup(func() {
		_ = ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *fakeSMTPServer) accept() {
	defer s.wg.Done()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *fakeSMTPServer) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	write := func(line string) bool {
		_, err := bw.WriteString(line + "\r\n")
		if err != nil {
			return false
		}
		return bw.Flush() == nil
	}
	if !write("220 fake.local ESMTP ready") {
		return
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			if !write("250-fake.local") {
				return
			}
			if s.advertiseStartTLS {
				if !write("250-STARTTLS") {
					return
				}
			}
			if !write("250-AUTH PLAIN") {
				return
			}
			if !write("250 SIZE 10485760") {
				return
			}
		case strings.HasPrefix(upper, "HELO"):
			if !write("250 fake.local") {
				return
			}
		case upper == "STARTTLS":
			if !s.advertiseStartTLS {
				if !write("502 5.5.1 STARTTLS not supported") {
					return
				}
				continue
			}
			s.mu.Lock()
			s.gotStartTLS = true
			s.mu.Unlock()
			if !write("220 Ready to start TLS") {
				return
			}
			tlsConn := tls.Server(c, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			c = tlsConn
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			br = bufio.NewReader(c)
			bw = bufio.NewWriter(c)
		case strings.HasPrefix(upper, "AUTH"):
			s.mu.Lock()
			s.gotAuth = true
			s.mu.Unlock()
			if !write("235 2.7.0 Authentication successful") {
				return
			}
		case strings.HasPrefix(upper, "MAIL FROM"):
			if !write("250 OK") {
				return
			}
		case strings.HasPrefix(upper, "RCPT TO"):
			if !write("250 OK") {
				return
			}
		case upper == "DATA":
			if !write("354 End data with <CR><LF>.<CR><LF>") {
				return
			}
			var sb strings.Builder
			for {
				ln, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if ln == ".\r\n" || ln == ".\n" {
					break
				}
				sb.WriteString(ln)
			}
			s.mu.Lock()
			s.body = sb.String()
			s.mu.Unlock()
			if !write("250 OK queued") {
				return
			}
		case upper == "QUIT":
			_ = write("221 Bye")
			return
		case upper == "RSET":
			if !write("250 OK") {
				return
			}
		case upper == "NOOP":
			if !write("250 OK") {
				return
			}
		default:
			if !write("502 5.5.2 Command not recognised") {
				return
			}
		}
	}
}

func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"fake.local", "localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		// Self-signed; test client trusts it via InsecureSkipVerify in cfgWithSkipVerify.
	}
}

// hostPort splits a 127.0.0.1:NNNNN address into host (always 127.0.0.1) and port.
func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var p int
	for _, r := range port {
		p = p*10 + int(r-'0')
	}
	return host, p
}

// withSkipVerify wraps the public Send to inject InsecureSkipVerify=true on
// the *tls.Config the dialers build. Without this every TLS dial in this test
// would fail certificate validation against the self-signed server cert.
//
// Implementation: we cannot modify Config to skip verify, so we expose a
// package-private knob via an env-style global only used by tests. Cleaner
// option: parameterise tlsClientConfig with an override hook here.
func withSkipVerify(t *testing.T, fn func()) {
	t.Helper()
	prev := testTLSOverride
	testTLSOverride = func(c *tls.Config) {
		c.InsecureSkipVerify = true
	}
	defer func() { testTLSOverride = prev }()
	fn()
}

func TestSend_StartTLS_Success(t *testing.T) {
	srv := newFakeServer(t, true, false)
	host, port := hostPort(t, srv.addr)

	withSkipVerify(t, func() {
		err := Send(context.Background(), Config{
			Host:    host,
			Port:    port,
			From:    "alerts@example.com",
			TLSMode: TLSModeStartTLS,
		}, Message{To: "ops@example.com", Subject: "hi", HTMLBody: "<p>hi</p>"})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	})
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.gotStartTLS {
		t.Fatal("server did not see STARTTLS")
	}
	if !strings.Contains(srv.body, "Subject: hi") {
		t.Fatalf("body missing subject: %q", srv.body)
	}
}

func TestSend_StartTLS_NotAdvertised_FailsClosed(t *testing.T) {
	srv := newFakeServer(t, false, false)
	host, port := hostPort(t, srv.addr)

	err := Send(context.Background(), Config{
		Host:    host,
		Port:    port,
		From:    "alerts@example.com",
		TLSMode: TLSModeStartTLS,
	}, Message{To: "ops@example.com", Subject: "hi", HTMLBody: "<p>hi</p>"})
	if err == nil {
		t.Fatal("expected failure when STARTTLS is not advertised")
	}
	var se *SendError
	if !errors.As(err, &se) {
		t.Fatalf("not a SendError: %T %v", err, err)
	}
	if se.Stage != StageStartTLS {
		t.Fatalf("stage=%q want %q", se.Stage, StageStartTLS)
	}
}

func TestSend_ImplicitTLS_Success(t *testing.T) {
	srv := newFakeServer(t, false, true)
	host, port := hostPort(t, srv.addr)

	withSkipVerify(t, func() {
		err := Send(context.Background(), Config{
			Host:    host,
			Port:    port,
			From:    "alerts@example.com",
			TLSMode: TLSModeTLS,
		}, Message{To: "ops@example.com", Subject: "tls-hi", HTMLBody: "<p>tls-hi</p>"})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	})
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.body, "Subject: tls-hi") {
		t.Fatalf("body missing subject: %q", srv.body)
	}
}

func TestSend_None_NoCreds_Success(t *testing.T) {
	srv := newFakeServer(t, false, false)
	host, port := hostPort(t, srv.addr)

	err := Send(context.Background(), Config{
		Host:    host,
		Port:    port,
		From:    "alerts@example.com",
		TLSMode: TLSModeNone,
	}, Message{To: "ops@example.com", Subject: "plain", HTMLBody: "<p>plain</p>"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.gotStartTLS {
		t.Fatal("server should not have seen STARTTLS")
	}
}

func TestSend_None_WithCreds_RejectedBeforeDial(t *testing.T) {
	t.Parallel()

	err := Send(context.Background(), Config{
		Host:     "127.0.0.1",
		Port:     1, // would fail dial — but we expect to fail validate first
		From:     "alerts@example.com",
		Username: "u",
		Password: "p",
		TLSMode:  TLSModeNone,
	}, Message{To: "ops@example.com", Subject: "x", HTMLBody: "x"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var se *SendError
	if !errors.As(err, &se) {
		t.Fatalf("not a SendError: %T", err)
	}
	if se.Stage != StageValidate {
		t.Fatalf("stage=%q want %q", se.Stage, StageValidate)
	}
}

func TestSend_Auto_WithStartTLS_UsesStartTLS(t *testing.T) {
	srv := newFakeServer(t, true, false)
	host, port := hostPort(t, srv.addr)

	withSkipVerify(t, func() {
		err := Send(context.Background(), Config{
			Host:    host,
			Port:    port,
			From:    "alerts@example.com",
			TLSMode: TLSModeAuto,
		}, Message{To: "ops@example.com", Subject: "auto-starttls", HTMLBody: "<p>x</p>"})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	})
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.gotStartTLS {
		t.Fatal("auto with STARTTLS advertised should upgrade")
	}
}

// TestSend_Auto_NoStartTLS_FallsBackToImplicitTLSAttempt verifies that the
// auto path attempts an implicit-TLS handshake on the same address when
// STARTTLS is not advertised. We point it at a cleartext-only server, so the
// fallback's TLS handshake fails and we expect a *SendError. This preserves
// the legacy behaviour where useTLS=true never silently downgrades to
// cleartext; an operator who picked auto on a non-TLS port gets a clear
// failure rather than a leaky success.
func TestSend_Auto_NoStartTLS_FallsBackToImplicitTLSAttempt(t *testing.T) {
	srv := newFakeServer(t, false, false)
	host, port := hostPort(t, srv.addr)

	err := Send(context.Background(), Config{
		Host:    host,
		Port:    port,
		From:    "alerts@example.com",
		TLSMode: TLSModeAuto,
	}, Message{To: "ops@example.com", Subject: "x", HTMLBody: "<p>x</p>"})
	if err == nil {
		t.Fatal("expected fallback TLS handshake to fail against cleartext-only server")
	}
	var se *SendError
	if !errors.As(err, &se) {
		t.Fatalf("not a SendError: %T %v", err, err)
	}
	if se.Stage != StageDial {
		t.Fatalf("stage=%q want %q", se.Stage, StageDial)
	}
}

// quick sanity that the renderMessage builds valid headers and is io-friendly.
func TestRenderMessage(t *testing.T) {
	t.Parallel()
	out := renderMessage(Config{From: "a@b.com", FromName: "Alerts"}, Message{
		To:       "x@y.com",
		Subject:  "line1\nline2", // CR/LF stripped
		HTMLBody: "<p>body</p>",
	})
	r := strings.NewReader(string(out))
	all, _ := io.ReadAll(r)
	s := string(all)
	if !strings.Contains(s, "Subject: line1line2\r\n") {
		t.Fatalf("subject not stripped: %q", s)
	}
	if !strings.Contains(s, "From: ") {
		t.Fatalf("missing From: %q", s)
	}
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("missing content-type: %q", s)
	}
}
