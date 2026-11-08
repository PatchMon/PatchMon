// Package pgtest gives each test binary its own migrated Postgres database,
// created from PATCHMON_TEST_DATABASE_URL and dropped when the binary exits.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/migrate"
	"github.com/jackc/pgx/v5"
)

const EnvURL = "PATCHMON_TEST_DATABASE_URL"

var (
	once     sync.Once
	baseURL  string
	dbURL    string
	dbName   string
	setupErr error

	tplOnce sync.Once
	tplName string
	tplErr  error
)

// URL returns this binary's database, created and migrated on first use.
// Without EnvURL it skips the test, or fails it when CI is set.
func URL(t testing.TB) string {
	t.Helper()
	base := os.Getenv(EnvURL)
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s must be set when CI is set", EnvURL)
		}
		t.Skipf("%s not set", EnvURL)
	}
	once.Do(func() { setupErr = setup(base) })
	if setupErr != nil {
		t.Fatalf("pgtest: %v", setupErr)
	}
	return dbURL
}

// Open returns a pool on this binary's database, closed when the test ends.
func Open(t testing.TB) *database.DB {
	t.Helper()
	u := URL(t)
	d, err := database.NewFromURL(context.Background(), u, 2, 1, &config.Config{DBConnectTimeout: 5})
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// OpenEmpty returns a pool on a freshly migrated database used only by this
// test, for tests that need an empty users table. It is dropped when the test ends.
func OpenEmpty(t testing.TB) *database.DB {
	t.Helper()
	URL(t)
	tplOnce.Do(func() { tplName, tplErr = createMigrated(baseURL) })
	if tplErr != nil {
		t.Fatalf("pgtest: %v", tplErr)
	}
	name, err := randomName()
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	if err := adminExec(baseURL, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{tplName}.Sanitize()); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(name) })
	d, err := database.NewFromURL(context.Background(), withPath(baseURL, name), 10, 1, &config.Config{DBConnectTimeout: 5})
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// Main runs the tests, then drops the databases that URL and OpenEmpty created.
func Main(m *testing.M) int {
	code := m.Run()
	for _, n := range []string{dbName, tplName} {
		if n != "" {
			dropDatabase(n)
		}
	}
	return code
}

func dropDatabase(name string) {
	if err := adminExec(baseURL, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: drop %s: %v\n", name, err)
	}
}

func randomName() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "patchmon_test_" + hex.EncodeToString(b[:]), nil
}

func withPath(base, name string) string {
	u, _ := url.Parse(base)
	u.Path = "/" + name
	return u.String()
}

func createMigrated(base string) (string, error) {
	name, err := randomName()
	if err != nil {
		return "", err
	}
	if err := adminExec(base, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("create database: %w", err)
	}
	if err := migrate.Run(withPath(base, name), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		return name, err
	}
	return name, nil
}

func setup(base string) error {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return errors.New(EnvURL + " must be a postgres:// URL")
	}
	baseURL = base
	name, err := createMigrated(base)
	dbName = name
	if err != nil {
		return err
	}
	dbURL = withPath(base, name)
	return nil
}

func adminExec(connURL, sql string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, sql)
	return err
}
