package handler

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wwt/guac"

	"github.com/PatchMon/PatchMon/server-source-code/internal/rdpproxy"
)

func TestAbortReachesBrowser(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		for {
			if _, err := c.Write([]byte("4.sync,1.1;")); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	tun := &rdpTunnel{Tunnel: guac.NewSimpleTunnel(guac.NewStream(conn, guac.SocketTimeout)), guacdConn: conn}
	h := &RDPHandler{rdpSessions: rdpproxy.NewSessions(nil, nopSender{})}
	srv := h.tunnelServer(func(*websocket.Conn, *http.Request) (guac.Tunnel, error) { return tun, nil })
	hs := httptest.NewServer(srv)
	defer hs.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(hs.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	_, _, _ = ws.ReadMessage()
	tun.abort(guac.UpstreamError, rdpMsgAgentGone)
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, m, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("connection ended without the abort error: %v", err)
		}
		if strings.Contains(string(m), "agent_disconnected: ") {
			break
		}
	}
	for {
		_, _, err := ws.ReadMessage()
		var ce *websocket.CloseError
		if err == nil {
			continue
		}
		if !errors.As(err, &ce) || ce.Text != "515" {
			t.Fatalf("close = %v, want a close frame with reason 515", err)
		}
		return
	}
}
