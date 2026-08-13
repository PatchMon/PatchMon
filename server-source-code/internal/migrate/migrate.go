// Package migrate runs database migrations at application startup.
// Migrations are embedded in the binary; no separate migrations directory is required.
package migrate

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run applies all pending migrations using embedded SQL files.
// Logs to the provided logger and always prints migration status to stdout.
// Returns an error if migrations fail.
// ErrNoChange is treated as success (already up to date).
func Run(databaseURL string, log *slog.Logger) error {
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required for migrations")
	}

	databaseURL = ensureSSLMode(databaseURL)

	_, _ = fmt.Fprintln(os.Stdout, "[migrate] running migrations from embedded binary")
	log.Info("running migrations", "path", "embedded")

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create embedded migrate source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	upErr := m.Up()
	if upErr != nil && upErr != migrate.ErrNoChange {
		fmt.Fprintf(os.Stderr, "[migrate] failed: %v\n", upErr)
		var dirty migrate.ErrDirty
		if errors.As(upErr, &dirty) {
			reportDirtyRecovery(os.Stderr, databaseURL, dirty.Version)
		} else {
			// A migration actually ran and failed, so the dirty marker was just
			// set fresh. Forget any earlier report for this database, otherwise
			// the dirty error on the next attempt is silently deduped away at
			// exactly the point the operator needs the guidance repeated.
			forgetDirtyRecovery(databaseURL)
		}
		return fmt.Errorf("migration up: %w", upErr)
	}

	forgetDirtyRecovery(databaseURL)

	if upErr == migrate.ErrNoChange {
		_, _ = fmt.Fprintln(os.Stdout, "[migrate] already up to date")
		log.Info("migrations: already up to date")
		return nil
	}

	version, _, _ := m.Version()
	msg := fmt.Sprintf("[migrate] applied successfully (version %d)", version)
	_, _ = fmt.Fprintln(os.Stdout, msg)
	log.Info("migrations applied successfully", "version", version)
	return nil
}

// Open returns a migrate instance using embedded migrations, for use by the CLI (up/down/force/version).
// Caller must call m.Close() when done.
func Open(databaseURL string) (*migrate.Migrate, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required for migrations")
	}

	databaseURL = ensureSSLMode(databaseURL)

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("create embedded migrate source: %w", err)
	}

	return migrate.NewWithSourceInstance("iofs", source, databaseURL)
}

// dirtyRecoveryReported tracks the dirty version already reported per database,
// so a server that retries migrations per request does not reprint the block on
// every attempt. Keyed by digest, never the DSN itself.
var (
	dirtyRecoveryMu       sync.Mutex
	dirtyRecoveryReported = map[string]int{}
)

func dirtyRecoveryKey(databaseURL string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(databaseURL)))
}

// reportDirtyRecovery turns golang-migrate's "Dirty database version N. Fix and
// force version." into instructions an operator can act on. The migrate CLI is
// not shipped in the server image, so the recovery is given as SQL. It reports
// whether it wrote anything, which is false when the same database is already
// known to be dirty at the same version.
func reportDirtyRecovery(w io.Writer, databaseURL string, version int) bool {
	key := dirtyRecoveryKey(databaseURL)

	dirtyRecoveryMu.Lock()
	last, seen := dirtyRecoveryReported[key]
	if seen && last == version {
		dirtyRecoveryMu.Unlock()
		return false
	}
	dirtyRecoveryReported[key] = version
	dirtyRecoveryMu.Unlock()

	_, _ = fmt.Fprint(w, dirtyRecoveryMessage(databaseURL, version))
	return true
}

// forgetDirtyRecovery drops a database's recorded dirty version once migrations
// get past it, so a later dirty episode reports again.
func forgetDirtyRecovery(databaseURL string) {
	dirtyRecoveryMu.Lock()
	delete(dirtyRecoveryReported, dirtyRecoveryKey(databaseURL))
	dirtyRecoveryMu.Unlock()
}

func dirtyRecoveryMessage(databaseURL string, version int) string {
	// Version 1 has no predecessor to roll back to, and forcing version 0 leaves
	// the marker clean but pointing at a migration that does not exist, which is
	// harder to recover from than the dirty state. Clearing the table is what
	// `migrate force -1` does and is the only way back from a dirty first migration.
	recovery := fmt.Sprintf("UPDATE schema_migrations SET version = %d, dirty = false;", version-1)
	if version <= 1 {
		recovery = "DELETE FROM schema_migrations;"
	}

	return fmt.Sprintf(`
[migrate] Migration %d did not complete on database %q, so it is marked dirty and
[migrate] no further migrations will run against it until that marker is cleared.
[migrate]
[migrate] The error logged above this block, on the run that first failed, is the
[migrate] cause. Fix that first, then confirm migration %d left nothing behind
[migrate] (it is applied as a single transaction, so a failed run rolls back, but
[migrate] a server killed mid-migration can leave partial changes). Then run this
[migrate] against that database:
[migrate]
[migrate]   %s
[migrate]
[migrate] and restart the server so migration %d is retried. Make sure the server
[migrate] is on a build that fixes the cause before retrying, or it will fail again.
`, version, databaseName(databaseURL), version, recovery, version)
}

// databaseName extracts just the database name from a DSN so the operator can
// tell which database is wedged, without putting credentials in the log.
// Anything that is not a URL-form DSN with a path is reported as unknown rather
// than echoed: a keyword/value DSN parses with the whole string as the path,
// which would put the password on stderr.
func databaseName(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil || u.Scheme == "" || !strings.HasPrefix(u.Path, "/") {
		return "unknown"
	}
	if name := strings.TrimPrefix(u.Path, "/"); name != "" {
		return name
	}
	return "unknown"
}

func ensureSSLMode(databaseURL string) string {
	if !strings.Contains(databaseURL, "sslmode=") {
		if strings.Contains(databaseURL, "?") {
			databaseURL += "&sslmode=disable"
		} else {
			databaseURL += "?sslmode=disable"
		}
	}
	return databaseURL
}
