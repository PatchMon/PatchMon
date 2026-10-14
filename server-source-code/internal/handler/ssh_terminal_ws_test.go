package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshproxy"
	"github.com/gorilla/websocket"
)

func startTerminalWS(t *testing.T, pongWait, pingPeriod time.Duration) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	h := &SshTerminalWSHandler{
		proxySess:  sshproxy.NewSessions(),
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		pongWait:   pongWait,
		pingPeriod: pingPeriod,
	}
	done := make(chan struct{})
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		h.handleConnection(conn, &models.Host{}, &models.User{}, "")
		close(done)
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, done
}

func TestSshTerminalWS_UnresponsiveClientIsReaped(t *testing.T) {
	t.Parallel()

	// The client never reads, so it never answers a ping: a vanished browser.
	_, done := startTerminalWS(t, 300*time.Millisecond, 100*time.Millisecond)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler still running after the client stopped answering pings")
	}
}

func TestSshTerminalWS_IdleResponsiveClientStaysOpen(t *testing.T) {
	t.Parallel()

	client, done := startTerminalWS(t, 500*time.Millisecond, 100*time.Millisecond)

	// Reading lets gorilla's default ping handler reply with pongs, as a browser does.
	go func() {
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
		t.Fatal("idle terminal that answers pings was torn down")
	case <-time.After(1500 * time.Millisecond):
	}

	if err := client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not exit after a close frame")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

type fakeControlWriter struct {
	calls atomic.Int32
	err   func(call int32) error
}

func (f *fakeControlWriter) WriteControl(int, []byte, time.Time) error {
	return f.err(f.calls.Add(1))
}

func TestRunTerminalPings_TimeoutKeepsPinging(t *testing.T) {
	t.Parallel()

	w := &fakeControlWriter{err: func(int32) error { return timeoutErr{} }}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runTerminalPings(w, 10*time.Millisecond, stop)
		close(done)
	}()
	time.Sleep(200 * time.Millisecond)
	close(stop)
	<-done
	if n := w.calls.Load(); n < 3 {
		t.Fatalf("pinger gave up after a write timeout: %d pings", n)
	}
}

func TestRunTerminalPings_OtherErrorStops(t *testing.T) {
	t.Parallel()

	w := &fakeControlWriter{err: func(int32) error { return errors.New("broken pipe") }}
	done := make(chan struct{})
	go func() {
		runTerminalPings(w, 10*time.Millisecond, make(chan struct{}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pinger kept running after a non-timeout error")
	}
	if n := w.calls.Load(); n != 1 {
		t.Fatalf("expected exactly one ping before stopping, got %d", n)
	}
}

func TestSSHUsername(t *testing.T) {
	windows := "windows"
	linux := "linux"
	cases := []struct {
		name      string
		requested string
		host      models.Host
		want      string
	}{
		{"linux default", "", models.Host{OSType: "Ubuntu"}, "root"},
		{"freebsd default", "", models.Host{OSType: "FreeBSD"}, "root"},
		{"unreported host", "", models.Host{}, "root"},
		{"windows reported", "", models.Host{OSType: "Windows Server 2022 Datacenter"}, "Administrator"},
		{"windows expected before first report", "", models.Host{ExpectedPlatform: &windows}, "Administrator"},
		{"linux expected", "", models.Host{ExpectedPlatform: &linux}, "root"},
		{"explicit username wins on windows", "patchadmin", models.Host{OSType: "Windows 11 Pro"}, "patchadmin"},
		{"explicit username wins on linux", "ubuntu", models.Host{OSType: "Ubuntu"}, "ubuntu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshUsername(tc.requested, &tc.host); got != tc.want {
				t.Fatalf("sshUsername(%q) = %q, want %q", tc.requested, got, tc.want)
			}
		})
	}
}

func TestProxyHostBoundToHost(t *testing.T) {
	t.Parallel()

	ip := "203.0.113.5"
	hostname := "web01.example.com"
	ifaces := []models.NetworkInterface{{
		Name:      "eth0",
		Addresses: []models.NetworkAddress{{Address: "10.1.2.3/24", Family: "inet"}},
	}}
	raw, err := json.Marshal(ifaces)
	if err != nil {
		t.Fatalf("marshal interfaces: %v", err)
	}
	host := &models.Host{IP: &ip, Hostname: &hostname, NetworkInterfaces: models.JSON(raw)}

	tests := []struct {
		name      string
		proxyHost string
		want      bool
	}{
		{"registered ip", "203.0.113.5", true},
		{"registered hostname", "web01.example.com", true},
		{"hostname case insensitive", "WEB01.EXAMPLE.COM", true},
		{"interface address with mask", "10.1.2.3", true},
		{"localhost name", "localhost", true},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unrelated internal host", "10.9.9.9", false},
		{"cloud metadata", "169.254.169.254", false},
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := proxyHostBoundToHost(host, tc.proxyHost); got != tc.want {
				t.Fatalf("proxyHostBoundToHost(%q) = %v, want %v", tc.proxyHost, got, tc.want)
			}
		})
	}
}
