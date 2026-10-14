package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

func newSshTicketStore(t *testing.T) *SshTicketStore {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), PoolSize: 32})
	t.Cleanup(func() { _ = client.Close() })
	return NewSshTicketStore(&hostctx.RedisResolver{Default: client})
}

func TestSshTicketConsumeIsSingleUseUnderConcurrency(t *testing.T) {
	s := newSshTicketStore(t)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		ticket, err := s.CreateTicket(ctx, "u", "h")
		if err != nil {
			t.Fatal(err)
		}
		var wins atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if _, err := s.ConsumeTicket(ctx, ticket, "h"); err == nil {
					wins.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		if n := wins.Load(); n != 1 {
			t.Fatalf("iteration %d: %d consumers accepted one ticket, want exactly 1", i, n)
		}
	}
}

func TestSshTicketConsumeRoundTrip(t *testing.T) {
	s := newSshTicketStore(t)
	ctx := context.Background()
	ticket, err := s.CreateTicket(ctx, "u1", "h1")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := s.ConsumeTicket(ctx, ticket, "h1")
	if err != nil || userID != "u1" {
		t.Fatalf("consume = %q, %v; want u1", userID, err)
	}
	if _, err := s.ConsumeTicket(ctx, ticket, "h1"); err != ErrInvalidTicket {
		t.Fatalf("second consume err = %v, want ErrInvalidTicket", err)
	}

	other, err := s.CreateTicket(ctx, "u1", "h1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeTicket(ctx, other, "h2"); err == nil {
		t.Fatal("ticket accepted for a different host")
	}
	if _, err := s.ConsumeTicket(ctx, other, "h1"); err != ErrInvalidTicket {
		t.Fatalf("ticket presented for the wrong host stayed usable: err = %v", err)
	}
}
