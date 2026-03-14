// Package migrate runs database migrations at application startup.
// Migrations are embedded in the binary; no separate migrations directory is required.
package migrate

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"strings"

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

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		fmt.Fprintf(os.Stderr, "[migrate] failed: %v\n", err)
		return fmt.Errorf("migration up: %w", err)
	}

	if err == migrate.ErrNoChange {
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
