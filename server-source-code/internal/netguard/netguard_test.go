package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateOutboundURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"public ipv4", "https://93.184.216.34/webhook", false},
		{"public ipv6", "https://[2606:2800:220:1:248:1893:25c8:1946]/x", false},
		{"rfc1918 10", "http://10.0.0.5:8080/hook", false},
		{"rfc1918 172", "https://172.16.3.4/hook", false},
		{"rfc1918 192", "http://192.168.1.10/hook", false},
		{"ula ipv6", "https://[fd00::1]/hook", false},
		{"ntfy default host", "https://ntfy.sh/mytopic", false},
		{"loopback v4", "http://127.0.0.1/hook", true},
		{"loopback v4 range", "http://127.5.6.7/hook", true},
		{"loopback v6", "http://[::1]/hook", true},
		{"link-local metadata", "http://169.254.169.254/latest/meta-data/", true},
		{"link-local range", "http://169.254.1.1/x", true},
		{"link-local v6", "http://[fe80::1]/x", true},
		{"aws metadata v6", "http://[fd00:ec2::254]/latest/meta-data/", true},
		{"unspecified v4", "http://0.0.0.0/x", true},
		{"unspecified v6", "http://[::]/x", true},
		{"gcp metadata name", "http://metadata.google.internal/computeMetadata/v1/", true},
		{"gcp metadata name trailing dot", "http://metadata.google.internal./x", true},
		{"ipv4-mapped loopback", "http://[::ffff:127.0.0.1]/x", true},
		{"ipv4-mapped metadata", "http://[::ffff:169.254.169.254]/x", true},
		{"userinfo rejected", "http://user:pass@93.184.216.34/x", true},
		{"userinfo only user", "http://admin@93.184.216.34/x", true},
		{"bad scheme ftp", "ftp://93.184.216.34/x", true},
		{"bad scheme file", "file:///etc/passwd", true},
		{"bad scheme gopher", "gopher://93.184.216.34/x", true},
		{"empty host", "http:///path", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutboundURL(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateOutboundURL(%q) = nil, want error", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateOutboundURL(%q) = %v, want nil", tt.raw, err)
			}
		})
	}
}

func TestValidateOutboundURLBlockedSentinel(t *testing.T) {
	err := ValidateOutboundURL("http://169.254.169.254/latest/meta-data/")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}
}

func TestGuardedClientBlocksLoopbackDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := GuardedClient(5 * time.Second).Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("guarded client dialled a loopback server, want refusal")
	}
}

func TestGuardedClientConfigured(t *testing.T) {
	c := GuardedClient(30 * time.Second)
	if c.Timeout != 30*time.Second {
		t.Fatalf("timeout = %v, want 30s", c.Timeout)
	}
	if c.Transport == nil {
		t.Fatal("transport is nil")
	}
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}
}
