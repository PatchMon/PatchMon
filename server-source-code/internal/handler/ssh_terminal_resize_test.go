package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

type ptySize struct{ cols, rows uint32 }

func startTestSSHServer(t *testing.T, sizes chan<- ptySize) int {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == "pw" {
				return nil, nil
			}
			return nil, io.EOF
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for nch := range chans {
			ch, chReqs, err := nch.Accept()
			if err != nil {
				return
			}
			go func() {
				for req := range chReqs {
					switch req.Type {
					case "pty-req":
						termLen := binary.BigEndian.Uint32(req.Payload)
						p := req.Payload[4+termLen:]
						sizes <- ptySize{binary.BigEndian.Uint32(p), binary.BigEndian.Uint32(p[4:])}
					case "window-change":
						sizes <- ptySize{binary.BigEndian.Uint32(req.Payload), binary.BigEndian.Uint32(req.Payload[4:])}
					}
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
				}
			}()
			go func() { _, _ = io.Copy(io.Discard, ch) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestDirectModeForwardsResize(t *testing.T) {
	t.Setenv("PATCHMON_SSH_KNOWN_HOSTS", filepath.Join(t.TempDir(), "known_hosts"))
	sizes := make(chan ptySize, 8)
	port := startTestSSHServer(t, sizes)

	ip := "127.0.0.1"
	h := &SshTerminalWSHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil)), pongWait: sshTerminalPongWait, pingPeriod: sshTerminalPingPeriod}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.handleConnection(conn, &models.Host{IP: &ip}, &models.User{}, "")
	}))
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.WriteJSON(map[string]any{
		"type": "connect", "username": "u", "password": "pw",
		"port": port, "cols": 80, "rows": 24,
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var m map[string]any
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatalf("waiting for connected: %v", err)
		}
		if m["type"] == "connected" {
			break
		}
		if m["type"] == "error" {
			t.Fatalf("connect error: %v", m["message"])
		}
	}

	next := func(what string) ptySize {
		t.Helper()
		select {
		case got := <-sizes:
			return got
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
			return ptySize{}
		}
	}

	if got := next("pty-req"); got != (ptySize{80, 24}) {
		t.Fatalf("initial pty size = %v, want 80x24", got)
	}
	for _, tc := range []struct {
		cols, rows int
		want       ptySize
	}{
		{150, 40, ptySize{150, 40}},
		{5000, 0, ptySize{1000, 24}},
	} {
		if err := ws.WriteJSON(map[string]any{"type": "resize", "cols": tc.cols, "rows": tc.rows}); err != nil {
			t.Fatal(err)
		}
		if got := next("window-change"); got != tc.want {
			t.Fatalf("resize %dx%d: window-change = %v, want %v", tc.cols, tc.rows, got, tc.want)
		}
	}
}

func TestTermDim(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 80}, {-3, 80}, {1, 1}, {1000, 1000}, {1001, 1000}, {1 << 30, 1000}} {
		if got := termDim(tc.in, 80); got != tc.want {
			t.Fatalf("termDim(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
