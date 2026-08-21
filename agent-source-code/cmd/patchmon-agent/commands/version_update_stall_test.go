package commands

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateAgentAdmitsOneUpdateAtATime(t *testing.T) {
	if !updateInFlight.CompareAndSwap(false, true) {
		t.Fatal("guard was already held before the test started")
	}
	defer updateInFlight.Store(false)

	err := updateAgent()
	if err == nil {
		t.Fatal("expected the second concurrent update to be refused")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("error = %v, want an already-in-progress refusal", err)
	}
}

type slowReader struct {
	remaining int
	tick      time.Duration
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.remaining == 0 {
		return 0, io.EOF
	}
	time.Sleep(s.tick)
	s.remaining--
	p[0] = 'x'
	return 1, nil
}

type deadReader struct {
	release chan struct{}
}

func (d *deadReader) Read(_ []byte) (int, error) {
	<-d.release
	return 0, errors.New("connection aborted")
}

func TestStallReaderAllowsSlowButSteadyProgress(t *testing.T) {
	idle := 200 * time.Millisecond

	var fired atomic.Bool
	timer := time.AfterFunc(idle, func() { fired.Store(true) })
	defer timer.Stop()

	src := &slowReader{remaining: 40, tick: 20 * time.Millisecond}
	got, err := io.ReadAll(&stallReader{r: src, timer: timer, idle: idle})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 40 {
		t.Fatalf("read %d bytes, want 40", len(got))
	}
	if fired.Load() {
		t.Fatal("stall timer fired while the reader was still making progress")
	}
}

func TestStallReaderFiresWhenTheLinkGoesQuiet(t *testing.T) {
	idle := 150 * time.Millisecond

	released := make(chan struct{})
	var fired atomic.Bool
	timer := time.AfterFunc(idle, func() {
		fired.Store(true)
		close(released)
	})
	defer timer.Stop()

	_, err := io.ReadAll(&stallReader{r: &deadReader{release: released}, timer: timer, idle: idle})
	if err == nil {
		t.Fatal("expected an error once the link went quiet")
	}
	if !fired.Load() {
		t.Fatal("stall timer did not fire on a silent link")
	}
}

func TestStallReaderResetsOnEveryChunk(t *testing.T) {
	idle := 120 * time.Millisecond

	var fired atomic.Bool
	timer := time.AfterFunc(idle, func() { fired.Store(true) })
	defer timer.Stop()

	sr := &stallReader{r: &slowReader{remaining: 6, tick: 60 * time.Millisecond}, timer: timer, idle: idle}
	buf := make([]byte, 1)
	for i := 0; i < 6; i++ {
		if _, err := sr.Read(buf); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}

	if fired.Load() {
		t.Fatal("stall timer fired despite continuous progress")
	}
}
