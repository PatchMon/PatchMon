package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5"
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

type execOnlyDBTX struct {
	execTag pgconn.CommandTag
	execErr error
}

func (f execOnlyDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return f.execTag, f.execErr
}

func (f execOnlyDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f execOnlyDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return nil
}

type staticDBProvider struct{ d *database.DB }

func (p staticDBProvider) DB(context.Context) *database.DB { return p.d }

func runsStoreWith(f execOnlyDBTX) *PatchRunsStore {
	return NewPatchRunsStore(staticDBProvider{d: &database.DB{Queries: db.New(f)}})
}

func TestActivateScheduledRun(t *testing.T) {
	ctx := context.Background()
	conflict := &pgconn.PgError{Code: "23505", ConstraintName: activePatchRunIndex}

	ok, err := runsStoreWith(execOnlyDBTX{execTag: pgconn.NewCommandTag("UPDATE 1")}).ActivateScheduledRun(ctx, "r")
	if err != nil || !ok {
		t.Fatalf("idle host: got ok=%v err=%v", ok, err)
	}
	ok, err = runsStoreWith(execOnlyDBTX{execTag: pgconn.NewCommandTag("UPDATE 0")}).ActivateScheduledRun(ctx, "r")
	if err != nil || ok {
		t.Fatalf("no longer scheduled: got ok=%v err=%v", ok, err)
	}
	_, err = runsStoreWith(execOnlyDBTX{execErr: conflict}).ActivateScheduledRun(ctx, "r")
	if !errors.Is(err, ErrHostHasActivePatchRun) {
		t.Fatalf("busy host: got %v, want ErrHostHasActivePatchRun", err)
	}
}

func TestCreateRunMapsActiveRunConflict(t *testing.T) {
	s := runsStoreWith(execOnlyDBTX{execErr: &pgconn.PgError{Code: "23505", ConstraintName: activePatchRunIndex}})
	_, err := s.CreateRun(context.Background(), "", "h", "j", "patch_all", nil, nil, nil, false, nil, nil, nil, nil, nil)
	if !errors.Is(err, ErrHostHasActivePatchRun) {
		t.Fatalf("got %v, want ErrHostHasActivePatchRun", err)
	}
}

func TestCreateRunStatusDerivation(t *testing.T) {
	future := time.Now().Add(time.Hour)
	cases := []struct {
		name      string
		dryRun    bool
		scheduled *time.Time
		opts      *CreateRunOpts
		want      string
	}{
		{"immediate", false, nil, nil, "queued"},
		{"policy delayed", false, &future, nil, "scheduled"},
		{"dry run", true, nil, nil, "pending_validation"},
		{"submit for approval", false, nil, &CreateRunOpts{InitialStatus: "pending_approval"}, "pending_approval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := newCreatePatchRunParams("", "h", "j", "patch_all", nil, nil, nil, tc.dryRun, tc.scheduled, nil, nil, nil, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if p.Status != tc.want {
				t.Errorf("status = %q, want %q", p.Status, tc.want)
			}
		})
	}
}

func TestScheduledRunQueriesAreGuarded(t *testing.T) {
	src := loadPatchingQueries(t)
	activate := namedQuery(t, src, "ActivateScheduledPatchRun")
	if !strings.Contains(activate, "status = 'queued'") || !strings.Contains(activate, "AND status = 'scheduled'") {
		t.Errorf("ActivateScheduledPatchRun must move only scheduled rows into queued, got:\n%s", activate)
	}
	skip := namedQuery(t, src, "SkipScheduledPatchRun")
	if !strings.Contains(skip, "AND status = 'scheduled'") {
		t.Errorf("SkipScheduledPatchRun must only touch scheduled rows, got:\n%s", skip)
	}
	expire := namedQuery(t, src, "MarkUndispatchedPatchRunsTimedOut")
	if !strings.Contains(expire, "COALESCE(scheduled_at, created_at) <") || strings.Contains(expire, "updated_at <") {
		t.Errorf("MarkUndispatchedPatchRunsTimedOut must key on when the run became due, got:\n%s", expire)
	}
	if strings.Contains(expire, "'running'") {
		t.Errorf("MarkUndispatchedPatchRunsTimedOut must not touch running rows, got:\n%s", expire)
	}
}

func TestActiveRunMigrationPreparesDataBeforeIndex(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "migrate", "migrations", "000050_v2-1-4_patch_runs_one_active_per_host.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up := string(b)
	if !strings.HasPrefix(up, "LOCK TABLE patch_runs IN SHARE ROW EXCLUSIVE MODE;") {
		t.Error("up migration must lock patch_runs first so an older instance cannot insert a duplicate before the index exists")
	}
	idx := strings.Index(up, "CREATE UNIQUE INDEX")
	for _, step := range []string{
		"SET status = 'scheduled'",
		"COALESCE(scheduled_at, created_at) < (NOW() AT TIME ZONE 'UTC') - INTERVAL '24 hours'",
		"ranked.rn > 1",
	} {
		at := strings.Index(up, step)
		if at < 0 || at > idx {
			t.Errorf("up migration must run %q before creating the index", step)
		}
	}
}

func TestAgentAndRequeueTransitionsAreAllowListed(t *testing.T) {
	src := loadPatchingQueries(t)
	started := namedQuery(t, src, "UpdatePatchRunStarted")
	if !strings.Contains(started, "status IN ('queued','pending_validation','running','agent_disconnected')") {
		t.Errorf("UpdatePatchRunStarted must allow-list the statuses an agent may start, got:\n%s", started)
	}
	requeue := namedQuery(t, src, "RequeuePatchRun")
	if !strings.Contains(requeue, "status IN ('queued', 'pending_validation')") {
		t.Errorf("RequeuePatchRun must only touch runs still awaiting dispatch, got:\n%s", requeue)
	}
}

func TestUpdateOutputReportsWhetherItEndedTheRun(t *testing.T) {
	ctx := context.Background()
	for _, osType := range []string{"ubuntu", "Windows"} {
		for _, stage := range []string{"completed", "failed"} {
			ended, err := runsStoreWith(execOnlyDBTX{execTag: pgconn.NewCommandTag("UPDATE 0")}).UpdateOutput(ctx, "r", osType, stage, "out", "")
			if err != nil || ended {
				t.Errorf("%s %s replay on a finished run: ended=%v err=%v", osType, stage, ended, err)
			}
		}
	}
	if ended, err := runsStoreWith(execOnlyDBTX{execTag: pgconn.NewCommandTag("UPDATE 1")}).UpdateOutput(ctx, "r", "ubuntu", "dry_run_completed", "out", ""); err != nil || ended {
		t.Errorf("dry_run_completed: ended=%v err=%v", ended, err)
	}
	ended, err := runsStoreWith(execOnlyDBTX{execTag: pgconn.NewCommandTag("UPDATE 1")}).UpdateOutput(ctx, "r", "ubuntu", "completed", "", "")
	if err != nil || !ended {
		t.Errorf("first completed report: ended=%v err=%v", ended, err)
	}
}
