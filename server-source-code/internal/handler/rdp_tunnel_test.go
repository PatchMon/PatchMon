package handler

import (
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/rdpproxy"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

type nopSender struct{}

func (nopSender) SendJSON(string, any) error { return nil }

func TestRDPTunnelInvalidTicketSendsGuacError(t *testing.T) {
	t.Setenv("AI_ENCRYPTION_KEY", "rdp-tunnel-test-key")
	enc, err := util.NewEncryption()
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	h := &RDPHandler{
		rdpTicketStore: store.NewRDPTicketStore(&hostctx.RedisResolver{Default: client}, enc),
		rdpSessions:    rdpproxy.NewSessions(nil, nopSender{}),
		log:            slog.New(slog.DiscardHandler),
	}
	srv := httptest.NewServer(h.WebsocketTunnelHandler())
	defer srv.Close()

	for _, q := range []string{"", "?ticket=deadbeef"} {
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/"+q, nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, msg, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("%q: no Guacamole error instruction before close: %v", q, err)
		}
		if !strings.HasPrefix(string(msg), "5.error,") || !strings.HasSuffix(string(msg), ",3.516;") {
			t.Fatalf("%q: instruction = %q, want an error with status 516", q, msg)
		}
		_, _, err = ws.ReadMessage()
		var ce *websocket.CloseError
		if !errors.As(err, &ce) || ce.Text != "516" {
			t.Fatalf("%q: close = %v, want a close frame with reason 516", q, err)
		}
		_ = ws.Close()
	}
}
