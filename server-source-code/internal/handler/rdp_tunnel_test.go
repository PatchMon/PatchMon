package handler

import (
	"errors"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/wwt/guac"

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
		if !strings.HasPrefix(string(msg), "5.error,") || !strings.HasSuffix(string(msg), ",3.516;") ||
			!strings.Contains(string(msg), ".rdp_ticket_invalid: ") {
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

type fakeReader struct {
	msgs [][]byte
	err  error
}

func (f *fakeReader) ReadSome() ([]byte, error) {
	if len(f.msgs) == 0 {
		return nil, f.err
	}
	m := f.msgs[0]
	f.msgs = f.msgs[1:]
	return m, nil
}
func (f *fakeReader) Available() bool { return len(f.msgs) > 0 }
func (f *fakeReader) Flush()          {}

func TestAbortableReaderEmitsErrorOnce(t *testing.T) {
	tun := &rdpTunnel{}
	ins := []byte("5.error,3.bye,3.515;")
	tun.abortIns.Store(&ins)
	r := &abortableReader{InstructionReader: &fakeReader{msgs: [][]byte{[]byte("4.sync,1.1;")}, err: errors.New("closed")}, t: tun}
	if m, err := r.ReadSome(); err != nil || string(m) != "4.sync,1.1;" {
		t.Fatalf("first read = %q, %v", m, err)
	}
	if m, err := r.ReadSome(); err != nil || string(m) != string(ins) {
		t.Fatalf("second read = %q, %v; want the abort instruction", m, err)
	}
	if r.Available() {
		t.Fatal("Available after abort")
	}
	if _, err := r.ReadSome(); err == nil {
		t.Fatal("third read succeeded; want the underlying error")
	}
}

func TestRDPAbortMessagesCarryCodePrefix(t *testing.T) {
	cases := map[string]string{
		"agent_disconnected": rdpMsgAgentGone,
		"rdp_session_ended":  rdpMsgSessionEnded,
		"rdp_ticket_invalid": rdpMsgTicketInvalid,
		"server_error":       rdpMsgServerError,
	}
	for code, msg := range cases {
		if !strings.HasPrefix(msg, code+": ") || len(msg) <= len(code)+2 {
			t.Errorf("%q does not start with %q", msg, code+": ")
		}
	}

	a, b := net.Pipe()
	defer func() { _ = b.Close() }()
	tun := &rdpTunnel{guacdConn: a}
	tun.abort(guac.UpstreamError, rdpMsgAgentGone)
	got := string(*tun.abortIns.Load())
	want := guac.NewInstruction("error", rdpMsgAgentGone, "515").String()
	if got != want {
		t.Fatalf("abort instruction = %q, want %q", got, want)
	}
	if _, err := a.Write([]byte("x")); err == nil {
		t.Fatal("abort left the guacd connection open")
	}
}

func TestAbortableWriterDiscardsAfterAbort(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = b.Close() }()
	tun := &rdpTunnel{guacdConn: a}
	w := &abortableWriter{w: a, t: tun}
	tun.abort(guac.UpstreamError, rdpMsgAgentGone)
	if n, err := w.Write([]byte("4.sync,1.1;")); err != nil || n != 11 {
		t.Fatalf("write after abort = %d, %v; want 11, nil", n, err)
	}
}
