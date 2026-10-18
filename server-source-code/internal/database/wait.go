package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DefaultAttemptTimeout limits each startup connection attempt when no connect timeout is configured.
const DefaultAttemptTimeout = 5 * time.Second

// WaitOptions controls WaitUntilReady.
type WaitOptions struct {
	Name           string
	Attempts       int
	Interval       time.Duration
	AttemptTimeout time.Duration
	Log            *slog.Logger
	Stderr         io.Writer
}

// WaitUntilReady calls probe until it succeeds, permanent reports true for its error,
// the attempts run out or ctx is cancelled.
func WaitUntilReady(ctx context.Context, opts WaitOptions, probe func(context.Context) error, permanent func(error) bool) error {
	attempts := max(opts.Attempts, 1)
	for attempt := 1; ; attempt++ {
		err := runProbe(ctx, opts.AttemptTimeout, probe)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%s: %w", opts.Name, ctxErr)
		}
		if permanent != nil && permanent(err) {
			return fmt.Errorf("%s: %w", opts.Name, err)
		}
		if attempt >= attempts {
			return fmt.Errorf("%s unavailable after %d attempts: %w", opts.Name, attempts, err)
		}
		if opts.Stderr != nil {
			_, _ = fmt.Fprintf(opts.Stderr, "[startup] %s not ready (attempt %d/%d): %v\n", opts.Name, attempt, attempts, err)
		}
		if opts.Log != nil {
			opts.Log.Warn(opts.Name+" not ready", "attempt", attempt, "max_attempts", attempts, "error", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", opts.Name, ctx.Err())
		case <-time.After(opts.Interval):
		}
	}
}

func runProbe(ctx context.Context, timeout time.Duration, probe func(context.Context) error) error {
	if timeout <= 0 {
		return probe(ctx)
	}
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return probe(actx)
}

// WaitForDatabase waits until a connection to databaseURL succeeds. A zero
// opts.AttemptTimeout uses the connect timeout from the URL, or 5s.
func WaitForDatabase(ctx context.Context, databaseURL string, opts WaitOptions) error {
	connCfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("%s: parse DATABASE_URL: %w", opts.Name, err)
	}
	if opts.AttemptTimeout <= 0 {
		opts.AttemptTimeout = DefaultAttemptTimeout
		if connCfg.ConnectTimeout > 0 {
			opts.AttemptTimeout = connCfg.ConnectTimeout
		}
	}
	probe := func(ctx context.Context) error {
		conn, err := pgx.ConnectConfig(ctx, connCfg.Copy())
		if err != nil {
			return err
		}
		defer func() {
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			_ = conn.Close(cctx)
		}()
		return conn.Ping(ctx)
	}
	return WaitUntilReady(ctx, opts, probe, IsPermanentConnectError)
}

// IsPermanentConnectError reports whether a connection error will recur on retry.
func IsPermanentConnectError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code != "57P03" && pgErr.Code != "53300"
	}
	return IsCertificateError(err)
}

// IsCertificateError reports whether err is a TLS certificate verification failure.
func IsCertificateError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var certInvalid x509.CertificateInvalidError
	var verify *tls.CertificateVerificationError
	return errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostname) ||
		errors.As(err, &certInvalid) ||
		errors.As(err, &verify)
}
