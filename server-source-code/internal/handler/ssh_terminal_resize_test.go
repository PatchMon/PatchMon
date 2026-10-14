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
	t.Setenv("HOME", t.TempDir())
	sizes := make(chan ptySize, 8)
	port := startTestSSHServer(t, sizes)

	ip := "127.0.0.1"
	h := &SshTerminalWSHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.handleConnection(conn, &models.Host{IP: &ip}, &models.User{})
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

	if got := <-sizes; got != (ptySize{80, 24}) {
		t.Fatalf("initial pty size = %v, want 80x24", got)
	}
	if err := ws.WriteJSON(map[string]any{"type": "resize", "cols": 150, "rows": 40}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-sizes:
		if got != (ptySize{150, 40}) {
			t.Fatalf("window-change = %v, want 150x40", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("direct mode did not send window-change after resize")
	}
}
