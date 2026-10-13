package docker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Debouncer coalesces bursts of Trigger calls into one call of fn per delay window.
type Debouncer struct {
	delay time.Duration
	fn    func()

	mu      sync.Mutex
	timer   *time.Timer
	armed   bool
	running bool
	pending bool
	stopped bool
}

// NewDebouncer returns a Debouncer that runs fn delay after the first Trigger of a burst.
func NewDebouncer(delay time.Duration, fn func()) *Debouncer {
	return &Debouncer{delay: delay, fn: fn}
}

// Trigger schedules fn unless a call is already scheduled.
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case d.stopped, d.armed:
	case d.running:
		d.pending = true
	default:
		d.armLocked()
	}
}

// Stop cancels any scheduled call.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	d.pending = false
	if d.timer != nil {
		d.timer.Stop()
	}
	d.armed = false
}

func (d *Debouncer) armLocked() {
	d.armed = true
	d.timer = time.AfterFunc(d.delay, d.fire)
}

func (d *Debouncer) fire() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.armed = false
	d.running = true
	d.mu.Unlock()

	d.fn()

	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = false
	if d.pending && !d.stopped {
		d.pending = false
		d.armLocked()
	}
}

// SingleFlight runs fn at most once at a time.
type SingleFlight struct {
	fn       func(ctx context.Context)
	running  atomic.Bool
	finished atomic.Int64
}

// NewSingleFlight wraps fn.
func NewSingleFlight(fn func(ctx context.Context)) *SingleFlight {
	return &SingleFlight{fn: fn}
}

// Run calls fn and reports true, or reports false when a call is already in progress.
func (s *SingleFlight) Run(ctx context.Context) bool {
	if !s.running.CompareAndSwap(false, true) {
		return false
	}
	defer s.running.Store(false)
	s.fn(ctx)
	s.finished.Store(time.Now().UnixNano())
	return true
}

// LastFinished returns when the last call completed, or the zero time.
func (s *SingleFlight) LastFinished() time.Time {
	ns := s.finished.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// RunEvery calls s.Run after first and then after each interval() until ctx is done.
func (s *SingleFlight) RunEvery(ctx context.Context, first time.Duration, interval func() time.Duration) {
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Run(ctx)
			t.Reset(interval())
		}
	}
}
