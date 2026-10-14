package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

func newRDPTicketStore(t *testing.T) *RDPTicketStore {
	t.Helper()
	t.Setenv("AI_ENCRYPTION_KEY", "rdp-ticket-test-key")
	enc, err := util.NewEncryption()
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), PoolSize: 32})
	t.Cleanup(func() { _ = client.Close() })
	return NewRDPTicketStore(&hostctx.RedisResolver{Default: client}, enc)
}

func TestRDPTicketConsumeIsSingleUseUnderConcurrency(t *testing.T) {
	s := newRDPTicketStore(t)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		ticket, err := s.CreateTicket(ctx, "u", "h", "sess", 1234, "user", "pw", 1024, 768, "")
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
				if _, err := s.ConsumeTicket(ctx, ticket); err == nil {
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

func TestRDPTicketConsumeRoundTrip(t *testing.T) {
	s := newRDPTicketStore(t)
	ctx := context.Background()
	ticket, err := s.CreateTicket(ctx, "u1", "h1", "s1", 4321, "admin", "secret", 1280, 800, "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.ConsumeTicket(ctx, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if d.UserID != "u1" || d.HostID != "h1" || d.SessionID != "s1" || d.Password != "secret" || d.Width != 1280 {
		t.Fatalf("unexpected ticket data %+v", d)
	}
	if _, err := s.ConsumeTicket(ctx, ticket); err != ErrInvalidRDPTicket {
		t.Fatalf("second consume err = %v, want ErrInvalidRDPTicket", err)
	}
}

func TestRDPTicketCarriesKeyboardLayout(t *testing.T) {
	s := newRDPTicketStore(t)
	ctx := context.Background()
	ticket, err := s.CreateTicket(ctx, "u1", "h1", "s1", 4321, "admin", "secret", 1280, 800, "de-de-qwertz")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.ConsumeTicket(ctx, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if d.KeyboardLayout != "de-de-qwertz" {
		t.Fatalf("keyboard layout = %q, want de-de-qwertz", d.KeyboardLayout)
	}
}

func TestRDPTicketWithoutLayoutStillDecodes(t *testing.T) {
	s := newRDPTicketStore(t)
	ctx := context.Background()
	legacy := `{"userId":"u1","hostId":"h1","sessionId":"s1","proxyPort":4321,"username":"admin","password":"secret","width":1280,"height":800,"createdAt":1}`
	encrypted, err := s.enc.Encrypt(legacy)
	if err != nil {
		t.Fatal(err)
	}
	const ticket = "legacyticket"
	if err := s.rdb.RDB(ctx).Set(ctx, hostctx.TenantKey(ctx, rdpTicketPrefix+ticket), encrypted, rdpTicketTTL).Err(); err != nil {
		t.Fatal(err)
	}
	d, err := s.ConsumeTicket(ctx, ticket)
	if err != nil {
		t.Fatalf("consume a ticket issued before keyboard layouts: %v", err)
	}
	if d.SessionID != "s1" || d.ProxyPort != 4321 || d.Password != "secret" || d.KeyboardLayout != "" {
		t.Fatalf("unexpected legacy ticket data %+v", d)
	}
}
