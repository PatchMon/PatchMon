package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests assert the SQL semantics that the patch-run lifecycle depends on.
// There is no database-backed test harness in this repo, so they read the sqlc
// source of truth (internal/sqlc/queries/patching.sql) rather than executing
// the statements. That is enough to catch the specific regressions below, all
// of which are one-word edits away from silently returning.

func loadPatchingQueries(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "sqlc", "queries", "patching.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// namedQuery extracts the body of a single `-- name: X :kind` block, with
// comment lines stripped so assertions only see real SQL.
func namedQuery(t *testing.T, src, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^-- name: ` + regexp.QuoteMeta(name) + ` :`)
	loc := re.FindStringIndex(src)
	if loc == nil {
		t.Fatalf("query %q not found in patching.sql", name)
	}
	rest := src[loc[1]:]
	// The block runs until the next `-- name:` marker or end of file.
	if next := regexp.MustCompile(`(?m)^-- name: `).FindStringIndex(rest); next != nil {
		rest = rest[:next[0]]
	}
	var sql []string
	for _, line := range strings.Split(rest, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		sql = append(sql, line)
	}
	return strings.Join(sql, "\n")
}

// TestMarkPatchRunsTimedOutUsesInactivityWindow guards the stall sweep against
// reverting to an elapsed-since-start predicate.
//
// The sweep runs every 10 minutes with a 30-minute default window. Keyed on
// COALESCE(started_at, created_at) it marked every run longer than the window
// timed_out WHILE IT WAS STILL RUNNING (a dist-upgrade on slow disk routinely
// exceeds 30 minutes), and because 'timed_out' then sat in the terminal-state
// guards, the agent's genuine final report was rejected: real shell output and
// packages_affected were discarded and the operator was told the run timed out
// when the patches had actually been applied.
//
// updated_at is bumped by every streamed progress chunk, so it is the only
// column that means "the agent has gone quiet" rather than "this is taking a
// while".
func TestMarkPatchRunsTimedOutUsesInactivityWindow(t *testing.T) {
	q := namedQuery(t, loadPatchingQueries(t), "MarkPatchRunsTimedOut")

	if !strings.Contains(q, "updated_at <") {
		t.Errorf("MarkPatchRunsTimedOut must key its staleness predicate on updated_at, got:\n%s", q)
	}
	if strings.Contains(q, "COALESCE(started_at, created_at) <") || strings.Contains(q, "started_at <") {
		t.Errorf("regression: MarkPatchRunsTimedOut is keyed on run start time again, which times out healthy long-running patches:\n%s", q)
	}
	if !strings.Contains(q, "status = 'running'") {
		t.Errorf("MarkPatchRunsTimedOut must only sweep running rows, got:\n%s", q)
	}
}

// TestLatePatchRunReportsCanUnwedgeTimedOut asserts that 'timed_out' is not in
// the terminal-state guards of the three agent-reported outcomes.
//
// timed_out (like agent_disconnected) is a server-side "we stopped waiting"
// marker, not a real outcome. Leaving it in the guard meant a late but genuine
// completed / failed / cancelled report from the agent matched zero rows, so
// the authoritative output and the packages_affected parse were both thrown
// away and the row stayed permanently wrong.
func TestLatePatchRunReportsCanUnwedgeTimedOut(t *testing.T) {
	src := loadPatchingQueries(t)

	for _, name := range []string{"UpdatePatchRunCompleted", "UpdatePatchRunFailed", "UpdatePatchRunCancelled"} {
		t.Run(name, func(t *testing.T) {
			q := namedQuery(t, src, name)
			if !strings.Contains(q, "status NOT IN") {
				t.Fatalf("%s lost its terminal-state guard entirely:\n%s", name, q)
			}
			if strings.Contains(q, "'timed_out'") {
				t.Errorf("regression: %s still excludes 'timed_out', so a late genuine agent report is discarded:\n%s", name, q)
			}
			if strings.Contains(q, "'agent_disconnected'") {
				t.Errorf("regression: %s excludes 'agent_disconnected', so a recovering agent cannot unwedge the row:\n%s", name, q)
			}
			// The real terminal outcomes must stay protected.
			for _, terminal := range []string{"'completed'", "'failed'", "'validated'", "'dry_run_completed'"} {
				if !strings.Contains(q, terminal) {
					t.Errorf("%s no longer protects %s from being overwritten:\n%s", name, terminal, q)
				}
			}
		})
	}
}

// TestUpdatePatchRunCancelledOutputIsFieldsOnly guards the DB-first StopRun
// repair path.
//
// StopRun sets status='cancelled' before signalling the agent, so on the common
// path the agent's own cancelled-stage report is blocked by the status guard on
// UpdatePatchRunCancelled and its captured output plus the
// packages-actually-applied parse were both dropped. The repair query must
// write the output WITHOUT touching status — the protection that stops a late
// 'completed' from overwriting a cancelled run has to stay intact.
func TestUpdatePatchRunCancelledOutputIsFieldsOnly(t *testing.T) {
	src := loadPatchingQueries(t)
	q := namedQuery(t, src, "UpdatePatchRunCancelledOutput")

	if !strings.Contains(q, "shell_output =") {
		t.Errorf("UpdatePatchRunCancelledOutput must write shell_output:\n%s", q)
	}
	if !strings.Contains(q, "WHERE id = $1") {
		t.Errorf("UpdatePatchRunCancelledOutput must be id-scoped:\n%s", q)
	}
	if !strings.Contains(q, "status = 'cancelled'") {
		t.Errorf("UpdatePatchRunCancelledOutput must only apply to rows already cancelled:\n%s", q)
	}

	// The SET clause specifically must not assign status. Inspecting the whole
	// statement is not good enough: `status = 'cancelled'` legitimately appears
	// in the WHERE clause, so we isolate SET..WHERE and assert status is absent
	// from it.
	setClause := q
	if i := strings.Index(setClause, "SET "); i >= 0 {
		setClause = setClause[i:]
	} else {
		t.Fatalf("UpdatePatchRunCancelledOutput has no SET clause:\n%s", q)
	}
	if i := strings.Index(setClause, "WHERE"); i >= 0 {
		setClause = setClause[:i]
	}
	if strings.Contains(setClause, "status") {
		t.Errorf("UpdatePatchRunCancelledOutput must not assign status; doing so would let it overwrite a genuine terminal state. SET clause:\n%s", setClause)
	}

	// 'cancelled' must REMAIN in UpdatePatchRunCancelled's guard: that is the
	// protection this repair path deliberately routes around rather than
	// weakening.
	cancelled := namedQuery(t, src, "UpdatePatchRunCancelled")
	if !strings.Contains(cancelled, "'cancelled'") {
		t.Errorf("UpdatePatchRunCancelled must keep 'cancelled' in its guard; the fields-only query is the sanctioned way to update an already-cancelled row:\n%s", cancelled)
	}
}

// TestUpdatePatchRunProgressBumpsUpdatedAt is the other half of the stall-sweep
// contract: the inactivity predicate is only meaningful because progress chunks
// touch updated_at.
func TestUpdatePatchRunProgressBumpsUpdatedAt(t *testing.T) {
	q := namedQuery(t, loadPatchingQueries(t), "UpdatePatchRunProgress")
	if !strings.Contains(q, "updated_at = NOW()") {
		t.Errorf("UpdatePatchRunProgress must bump updated_at, otherwise MarkPatchRunsTimedOut's inactivity window sweeps healthy runs:\n%s", q)
	}
}
