package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapActiveRunConflict(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"active run index", &pgconn.PgError{Code: "23505", ConstraintName: activePatchRunIndex}, ErrHostHasActivePatchRun},
		{"wrapped", fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: activePatchRunIndex}), ErrHostHasActivePatchRun},
		{"primary key", &pgconn.PgError{Code: "23505", ConstraintName: "patch_runs_pkey"}, nil},
		{"foreign key", &pgconn.PgError{Code: "23503", ConstraintName: activePatchRunIndex}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapActiveRunConflict(tc.err)
			if tc.want != nil {
				if !errors.Is(got, tc.want) {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				return
			}
			if errors.Is(got, ErrHostHasActivePatchRun) || got != tc.err {
				t.Fatalf("non-matching error must pass through unchanged, got %v", got)
			}
		})
	}
}

func activeIndexStatuses(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(`(?s)CREATE UNIQUE INDEX IF NOT EXISTS ` + activePatchRunIndex + `\s+ON patch_runs \(host_id\)\s+WHERE status IN \(([^)]*)\)`)
	m := re.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("%s does not define %s as a partial unique index on patch_runs(host_id)", path, activePatchRunIndex)
	}
	var out []string
	for _, s := range strings.Split(m[1], ",") {
		out = append(out, strings.Trim(strings.TrimSpace(s), "'"))
	}
	return out
}

func TestActivePatchRunIndexCoversEveryDispatchableStatus(t *testing.T) {
	want := []string{"queued", "pending_validation", "running"}
	paths := []string{
		filepath.Join("..", "sqlc", "schema", "schema.sql"),
		filepath.Join("..", "migrate", "migrations", "000050_v2-1-4_patch_runs_one_active_per_host.up.sql"),
	}
	for _, p := range paths {
		got := activeIndexStatuses(t, p)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: index statuses = %v, want %v", p, got, want)
		}
	}
}
