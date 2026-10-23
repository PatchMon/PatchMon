package database

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsPermanentConnectError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, false},
		{"connection reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, false},
		{"dns not found", &net.DNSError{Err: "no such host", Name: "database", IsNotFound: true}, false},
		{"attempt timeout", fmt.Errorf("connect: %w", context.DeadlineExceeded), false},
		{"starting up", &pgconn.PgError{Code: "57P03", Message: "the database system is starting up"}, false},
		{"wrapped starting up", fmt.Errorf("connect: %w", &pgconn.PgError{Code: "57P03"}), false},
		{"too many connections", &pgconn.PgError{Code: "53300", Message: "sorry, too many clients already"}, false},
		{"bad password", &pgconn.PgError{Code: "28P01"}, true},
		{"no pg_hba entry", &pgconn.PgError{Code: "28000"}, true},
		{"database missing", fmt.Errorf("connect: %w", &pgconn.PgError{Code: "3D000"}), true},
		{"unknown authority", x509.UnknownAuthorityError{}, true},
		{"hostname mismatch", fmt.Errorf("tls: %w", x509.HostnameError{Host: "db"}), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsPermanentConnectError(c.err); got != c.want {
				t.Errorf("IsPermanentConnectError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func waitOpts(attempts int, stderr *bytes.Buffer) WaitOptions {
	opts := WaitOptions{Name: "database", Attempts: attempts, Interval: time.Millisecond}
	if stderr != nil {
		opts.Stderr = stderr
	}
	return opts
}

func TestWaitUntilReadySucceedsAfterTransientFailures(t *testing.T) {
	var stderr bytes.Buffer
	calls := 0
	err := WaitUntilReady(context.Background(), waitOpts(5, &stderr), func(context.Context) error {
		calls++
		if calls < 3 {
			return syscall.ECONNREFUSED
		}
		return nil
	}, IsPermanentConnectError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if !strings.Contains(stderr.String(), "attempt 1/5") || !strings.Contains(stderr.String(), "attempt 2/5") {
		t.Fatalf("stderr missing attempt lines: %q", stderr.String())
	}
}

func TestWaitUntilReadyStopsOnPermanentError(t *testing.T) {
	calls := 0
	authErr := &pgconn.PgError{Code: "28P01"}
	err := WaitUntilReady(context.Background(), waitOpts(5, nil), func(context.Context) error {
		calls++
		return authErr
	}, IsPermanentConnectError)
	if !errors.Is(err, authErr) {
		t.Fatalf("err = %v, want the auth error", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestWaitUntilReadyGivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	err := WaitUntilReady(context.Background(), waitOpts(4, nil), func(context.Context) error {
		calls++
		return syscall.ECONNREFUSED
	}, IsPermanentConnectError)
	if err == nil || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("err = %v, want wrapped ECONNREFUSED", err)
	}
	if !strings.Contains(err.Error(), "after 4 attempts") {
		t.Fatalf("err = %v, want attempt count", err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
}

func TestWaitUntilReadyHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := WaitOptions{Name: "database", Attempts: 100, Interval: time.Hour}
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- WaitUntilReady(ctx, opts, func(context.Context) error {
			calls++
			return syscall.ECONNREFUSED
		}, IsPermanentConnectError)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitUntilReady did not return after cancellation")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestWaitUntilReadyAppliesAttemptTimeout(t *testing.T) {
	opts := WaitOptions{Name: "database", Attempts: 2, Interval: time.Millisecond, AttemptTimeout: 10 * time.Millisecond}
	calls := 0
	err := WaitUntilReady(context.Background(), opts, func(ctx context.Context) error {
		calls++
		<-ctx.Done()
		return ctx.Err()
	}, IsPermanentConnectError)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (an attempt timeout is transient)", calls)
	}
}

func TestWaitForDatabaseInvalidURLIsPermanentAndRedacted(t *testing.T) {
	err := WaitForDatabase(context.Background(), "postgres://user:s3cretpw@localhost:notaport/db", waitOpts(5, nil))
	if err == nil {
		t.Fatal("want an error for an invalid URL")
	}
	if strings.Contains(err.Error(), "s3cretpw") {
		t.Fatalf("error leaks the password: %v", err)
	}
}

func TestWaitForDatabaseRefusedDoesNotLeakPassword(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var stderr bytes.Buffer
	err = WaitForDatabase(context.Background(), "postgres://user:s3cretpw@"+addr+"/db?sslmode=disable", waitOpts(2, &stderr))
	if err == nil {
		t.Fatal("want an error when nothing is listening")
	}
	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Fatalf("err = %v, want retries", err)
	}
	if strings.Contains(err.Error(), "s3cretpw") || strings.Contains(stderr.String(), "s3cretpw") {
		t.Fatalf("output leaks the password: %v / %q", err, stderr.String())
	}
}

func TestWaitForDatabaseIntegration(t *testing.T) {
	url := os.Getenv("PATCHMON_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PATCHMON_TEST_DATABASE_URL not set")
	}
	if err := WaitForDatabase(context.Background(), url, waitOpts(3, nil)); err != nil {
		t.Fatalf("WaitForDatabase: %v", err)
	}
}
