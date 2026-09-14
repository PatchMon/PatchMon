package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newPendingLoginStore(t *testing.T) (*PendingLoginStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewPendingLoginStore(&hostctx.RedisResolver{Default: client}), mr
}

// TestPendingLogin_TicketIsSingleUse is the property the entire TFA fix rests
// on, and the one thing the handler tests could not cover without a Redis.
func TestPendingLogin_TicketIsSingleUse(t *testing.T) {
	t.Parallel()
	s, _ := newPendingLoginStore(t)
	ctx := context.Background()

	ticket, err := s.Create(ctx, "user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	userID, err := s.Consume(ctx, ticket)
	if err != nil {
		t.Fatalf("first Consume must succeed: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("expected user-1, got %q", userID)
	}

	// Second use must fail. This is the regression that matters.
	if _, err := s.Consume(ctx, ticket); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("a ticket must not be usable twice, got %v", err)
	}
	// And a third, in case the first failure left it in a resurrectable state.
	if _, err := s.Consume(ctx, ticket); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("a spent ticket must stay spent, got %v", err)
	}
}

// TestPendingLogin_ResolvesTheIssuedUser confirms the happy path the handler
// depends on: the user comes from the ticket, not from the request body.
func TestPendingLogin_ResolvesTheIssuedUser(t *testing.T) {
	t.Parallel()
	s, _ := newPendingLoginStore(t)
	ctx := context.Background()

	ticketA, err := s.Create(ctx, "user-a")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	ticketB, err := s.Create(ctx, "user-b")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if ticketA == ticketB {
		t.Fatal("tickets must be unique")
	}

	gotA, err := s.Consume(ctx, ticketA)
	if err != nil || gotA != "user-a" {
		t.Fatalf("ticket A must resolve to user-a, got %q %v", gotA, err)
	}
	gotB, err := s.Consume(ctx, ticketB)
	if err != nil || gotB != "user-b" {
		t.Fatalf("ticket B must resolve to user-b, got %q %v", gotB, err)
	}
}

// TestPendingLogin_TicketExpires bounds the window in which a leaked ticket
// is worth anything.
func TestPendingLogin_TicketExpires(t *testing.T) {
	t.Parallel()
	s, mr := newPendingLoginStore(t)
	ctx := context.Background()

	ticket, err := s.Create(ctx, "user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Just inside the TTL.
	mr.FastForward(pendingLoginTTL - time.Second)
	if _, err := s.Consume(ctx, ticket); err != nil {
		t.Fatalf("ticket must still be valid just inside its TTL: %v", err)
	}

	// A fresh one, then past the TTL.
	ticket2, err := s.Create(ctx, "user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mr.FastForward(pendingLoginTTL + time.Second)
	if _, err := s.Consume(ctx, ticket2); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("an expired ticket must be refused, got %v", err)
	}
}

// TestPendingLogin_RejectsUnknownTickets covers the shapes an attacker
// supplies.
func TestPendingLogin_RejectsUnknownTickets(t *testing.T) {
	t.Parallel()
	s, _ := newPendingLoginStore(t)
	ctx := context.Background()

	for _, ticket := range []string{
		"",
		"deadbeef",
		"../../etc/passwd",
		"auth:pending_tfa:forged",
	} {
		if _, err := s.Consume(ctx, ticket); !errors.Is(err, ErrInvalidPendingLogin) {
			t.Errorf("ticket %q must be refused, got %v", ticket, err)
		}
	}
}

// TestPendingLogin_ConcurrentConsumeYieldsExactlyOneWinner is a smoke test,
// NOT a proof of atomicity.
func TestPendingLogin_ConcurrentConsumeYieldsExactlyOneWinner(t *testing.T) {
	t.Parallel()
	s, _ := newPendingLoginStore(t)
	ctx := context.Background()

	ticket, err := s.Create(ctx, "user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const racers = 16
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, err := s.Consume(ctx, ticket)
			results <- err
		}()
	}
	close(start)

	var wins int
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one concurrent Consume may succeed, got %d", wins)
	}
}

// TestPendingLogin_TicketsAreHighEntropy guards the token size.
func TestPendingLogin_TicketsAreHighEntropy(t *testing.T) {
	t.Parallel()
	s, _ := newPendingLoginStore(t)
	ctx := context.Background()

	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		ticket, err := s.Create(ctx, "user-1")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(ticket) != 64 { // 32 random bytes, hex-encoded
			t.Fatalf("expected a 64-char hex ticket, got %d chars", len(ticket))
		}
		if _, dup := seen[ticket]; dup {
			t.Fatal("ticket collision")
		}
		seen[ticket] = struct{}{}
	}
}

// TestPendingLogin_ConsumeUsesASingleAtomicCommand pins the property that
// actually makes the ticket single-use under concurrency.
func TestPendingLogin_ConsumeUsesASingleAtomicCommand(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("pending_login.go")
	if err != nil {
		t.Fatalf("reading source: %v", err)
	}
	body := string(src)

	start := strings.Index(body, "func (s *PendingLoginStore) Consume(")
	if start < 0 {
		t.Fatal("Consume not found")
	}
	consume := body[start:]
	if end := strings.Index(consume, "\n}\n"); end > 0 {
		consume = consume[:end]
	}

	if !strings.Contains(consume, "GetDel(") {
		t.Error("Consume must use GetDel: the read and delete have to be one command, " +
			"or two requests arriving together both get a guess at the code")
	}
	if strings.Contains(consume, "rdb.Get(") && strings.Contains(consume, "rdb.Del(") {
		t.Error("Consume must not read-then-delete; that is not atomic across clients")
	}
}
