// Package main runs database migrations using golang-migrate with embedded SQL files.
// Usage:
//
//	migrate up   - run all pending migrations
//	migrate down - rollback last migration
//	migrate force V - set migration version (e.g. for baselining)
//
// Requires DATABASE_URL environment variable.
package main

import (
	"flag"
	"fmt"
	"os"

	ourmigrate "github.com/PatchMon/PatchMon/server-source-code/internal/migrate"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

func main() {
	flag.Parse()
	args := flag.Args()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	m, err := ourmigrate.Open(dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrate instance: %v\n", err)
		os.Exit(1)
	}
	defer func() { _, _ = m.Close() }()

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: migrate [up|down|force VERSION|version]")
		os.Exit(1)
	}

	switch args[0] {
	case "up":
		upErr := m.Up()
		if upErr != nil && upErr != migrate.ErrNoChange {
			fmt.Fprintf(os.Stderr, "Migration up failed: %v\n", upErr)
			os.Exit(1)
		}
		if upErr == migrate.ErrNoChange {
			fmt.Println("No migrations to run (already up to date)")
		} else {
			fmt.Println("Migrations completed successfully")
		}
	case "down":
		downErr := m.Steps(-1)
		if downErr != nil && downErr != migrate.ErrNoChange {
			fmt.Fprintf(os.Stderr, "Migration down failed: %v\n", downErr)
			os.Exit(1)
		}
		if downErr == migrate.ErrNoChange {
			fmt.Println("No migrations to roll back")
		} else {
			fmt.Println("Rollback completed successfully")
		}
	case "force":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: migrate force VERSION")
			os.Exit(1)
		}
		var version int
		if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid version: %s\n", args[1])
			os.Exit(1)
		}
		if err := m.Force(version); err != nil {
			fmt.Fprintf(os.Stderr, "Force failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Forced version to %d\n", version)
	case "version":
		version, dirty, err := m.Version()
		if err != nil && err != migrate.ErrNilVersion {
			fmt.Fprintf(os.Stderr, "Version check failed: %v\n", err)
			os.Exit(1)
		}
		if err == migrate.ErrNilVersion {
			fmt.Println("No migrations applied yet")
		} else {
			fmt.Printf("Version: %d (dirty: %v)\n", version, dirty)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: migrate [up|down|force VERSION|version]")
		os.Exit(1)
	}
}
