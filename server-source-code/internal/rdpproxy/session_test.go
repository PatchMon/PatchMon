package rdpproxy

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

type recSender struct {
	mu   sync.Mutex
	msgs []map[string]interface{}
}

func (r *recSender) SendJSON(_ string, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := v.(map[string]interface{}); ok {
		r.msgs = append(r.msgs, m)
	}
	return nil
}

func (r *recSender) disconnects() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for _, m := range r.msgs {
		if m["type"] == "rdp_proxy_disconnect" {
			ids = append(ids, m["session_id"].(string))
		}
	}
	return ids
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func create(t *testing.T, s *Sessions, apiID, scope string) (string, int) {
	t.Helper()
	id, port, err := s.Create(context.Background(), Owner{ApiID: apiID, HostID: "h-" + apiID, UserID: "u", Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	return id, port
}

func attach(t *testing.T, port int) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGuacdCloseTellsAgentOnce(t *testing.T) {
	snd := &recSender{}
	s := NewSessions(nil, snd)
	id, port := create(t, s, "api1", "")
	s.OnAgentConnected("api1", id)
	c := attach(t, port)
	_ = c.Close()

	waitFor(t, "session removal", func() bool { return s.Count() == 0 })
	if got := snd.disconnects(); len(got) != 1 || got[0] != id {
		t.Fatalf("disconnects = %v, want exactly [%s]", got, id)
	}
	s.Delete(id)
	if got := snd.disconnects(); len(got) != 1 {
		t.Fatalf("repeat teardown sent another disconnect: %v", got)
	}
}

func TestDeleteTellsAgent(t *testing.T) {
	snd := &recSender{}
	s := NewSessions(nil, snd)
	id, _ := create(t, s, "api1", "")
	s.Delete(id)
	if got := snd.disconnects(); len(got) != 1 || got[0] != id {
		t.Fatalf("disconnects = %v, want [%s]", got, id)
	}
	if _, ok := s.Get(id); ok {
		t.Fatal("session still present after Delete")
	}
}

func TestAgentInitiatedCloseIsNotEchoed(t *testing.T) {
	snd := &recSender{}
	s := NewSessions(nil, snd)
	a, _ := create(t, s, "api1", "")
	b, _ := create(t, s, "api1", "")
	s.OnAgentClosed("api1", a)
	s.OnAgentError("api1", b, "Failed to connect: refused")
	if got := snd.disconnects(); len(got) != 0 {
		t.Fatalf("agent-initiated closes sent disconnects: %v", got)
	}
	if s.Count() != 0 {
		t.Fatalf("Count = %d, want 0", s.Count())
	}
}

func TestOtherAgentCannotCloseSession(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	id, _ := create(t, s, "api1", "")
	s.OnAgentClosed("api2", id)
	if _, ok := s.Get(id); !ok {
		t.Fatal("a different agent closed the session")
	}
}

func TestUnattachedSessionIsReaped(t *testing.T) {
	snd := &recSender{}
	s := NewSessions(nil, snd)
	s.timeouts.unattached, s.timeouts.reap = 50*time.Millisecond, 10*time.Millisecond
	unused, _ := create(t, s, "api1", "")
	used, port := create(t, s, "api1", "")
	c := attach(t, port)
	defer func() { _ = c.Close() }()

	waitFor(t, "unused session reaped", func() bool {
		_, ok := s.Get(unused)
		return !ok
	})
	time.Sleep(100 * time.Millisecond)
	if _, ok := s.Get(used); !ok {
		t.Fatal("attached session was reaped by the unattached timeout")
	}
	if got := snd.disconnects(); len(got) != 1 || got[0] != unused {
		t.Fatalf("disconnects = %v, want [%s]", got, unused)
	}
}

func TestPerScopeCap(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	s.SetMaxSessionsPerScope(2)
	a, _ := create(t, s, "a1", "ctx-a.example")
	create(t, s, "a2", "ctx-a.example")
	if _, _, err := s.Create(context.Background(), Owner{ApiID: "a3", Scope: "ctx-a.example"}); !errors.Is(err, ErrMaxScopeSessionsReached) {
		t.Fatalf("third session in one scope: err = %v, want ErrMaxScopeSessionsReached", err)
	}
	create(t, s, "b1", "ctx-b.example")
	s.Delete(a)
	create(t, s, "a3", "ctx-a.example")
}

func TestGlobalCapStillApplies(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	s.SetMaxSessions(2)
	id, _ := create(t, s, "a", "x")
	create(t, s, "b", "y")
	if _, _, err := s.Create(context.Background(), Owner{ApiID: "c", Scope: "z"}); !errors.Is(err, ErrMaxSessionsReached) {
		t.Fatalf("err = %v, want ErrMaxSessionsReached", err)
	}
	s.Delete(id)
	create(t, s, "c", "z")
}

func TestCloseAgentSessionsRespectsCutoff(t *testing.T) {
	snd := &recSender{}
	s := NewSessions(nil, snd)
	old1, _ := create(t, s, "api1", "")
	old2, port := create(t, s, "api1", "")
	c := attach(t, port)
	defer func() { _ = c.Close() }()
	other, _ := create(t, s, "api2", "")
	time.Sleep(5 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(5 * time.Millisecond)
	fresh, _ := create(t, s, "api1", "")

	ready := make(chan AgentResult, 1)
	go func() {
		r, _ := s.WaitAgentReady(context.Background(), old1, 2*time.Second)
		ready <- r
	}()
	time.Sleep(20 * time.Millisecond)

	if n := s.CloseAgentSessions("api1", cutoff); n != 2 {
		t.Fatalf("closed %d sessions, want 2", n)
	}
	for _, id := range []string{old1, old2} {
		if _, ok := s.Get(id); ok {
			t.Fatalf("session %s survived its agent dropping", id)
		}
	}
	for _, id := range []string{other, fresh} {
		if _, ok := s.Get(id); !ok {
			t.Fatalf("session %s was closed but is not on the dropped connection", id)
		}
	}
	if r := <-ready; r.Connected {
		t.Fatal("pending handshake reported success after the agent dropped")
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("guacd side of a dropped session is still open")
	}
}

func TestOnCloseReportsReason(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	id, _ := create(t, s, "api1", "")
	got := make(chan string, 1)
	if !s.OnClose(id, func(reason string) { got <- reason }) {
		t.Fatal("OnClose refused a live session")
	}
	s.CloseAgentSessions("api1", time.Now().Add(time.Second))
	if r := <-got; r != ReasonAgentGone {
		t.Fatalf("reason = %q, want %q", r, ReasonAgentGone)
	}
	if s.OnClose(id, func(string) {}) {
		t.Fatal("OnClose accepted a closed session")
	}
}

func TestEmptyScopeCapOnlyInMultiContextMode(t *testing.T) {
	single := NewSessions(nil, &recSender{})
	single.SetMaxSessionsPerScope(2)
	single.SetMaxSessions(5)
	for i := 0; i < 5; i++ {
		create(t, single, "solo", "")
	}
	if _, _, err := single.Create(context.Background(), Owner{ApiID: "solo"}); !errors.Is(err, ErrMaxSessionsReached) {
		t.Fatalf("single-context: err = %v, want only the process-wide cap", err)
	}

	multi := NewSessions(nil, &recSender{})
	multi.SetMaxSessionsPerScope(2)
	multi.SetCapEmptyScope(true)
	create(t, multi, "solo", "")
	create(t, multi, "solo", "")
	if _, _, err := multi.Create(context.Background(), Owner{ApiID: "solo"}); !errors.Is(err, ErrMaxScopeSessionsReached) {
		t.Fatalf("multi-context empty scope: err = %v, want ErrMaxScopeSessionsReached", err)
	}
	create(t, multi, "a1", "ctx-a.example")
}

func TestListenerClosesAfterFirstAccept(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	_, port := create(t, s, "api1", "")
	c := attach(t, port)
	defer func() { _ = c.Close() }()
	waitFor(t, "listener closed", func() bool {
		c2, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if err != nil {
			return true
		}
		_ = c2.Close()
		return false
	})
}

func TestCloseAgentSessionsCutoffIgnoresMonotonicClock(t *testing.T) {
	s := NewSessions(nil, &recSender{})
	id, _ := create(t, s, "api1", "")
	cutoff := time.Now().UTC().Round(0).Add(time.Millisecond)
	if n := s.CloseAgentSessions("api1", cutoff); n != 1 {
		t.Fatalf("closed %d, want 1 for a session created before a wall-clock cutoff", n)
	}
	if _, ok := s.Get(id); ok {
		t.Fatal("session survived")
	}
}
