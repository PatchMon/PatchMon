package store

import (
	"context"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type capturingDBTX struct{ execArgs [][]any }

func (c *capturingDBTX) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	c.execArgs = append(c.execArgs, args)
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func (c *capturingDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return emptyRows{}, nil
}

func (c *capturingDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row { return nil }

func TestWindowsTerminalStagesKeepTheAgentSummary(t *testing.T) {
	for _, tc := range []struct {
		stage, msg string
		want       *string
	}{
		{"completed", "1 skipped: 7zip.7zip (pinned in winget)", strp("1 skipped: 7zip.7zip (pinned in winget)")},
		{"dry_run_completed", "1 skipped: 7zip.7zip (pinned in winget)", strp("1 skipped: 7zip.7zip (pinned in winget)")},
		{"completed", "", nil},
		{"dry_run_completed", "", nil},
	} {
		c := &capturingDBTX{}
		s := NewPatchRunsStore(staticDBProvider{d: &database.DB{Queries: db.New(c)}})
		if _, err := s.UpdateOutput(context.Background(), "r", "Windows", tc.stage, "out", tc.msg); err != nil {
			t.Fatalf("%s: %v", tc.stage, err)
		}
		if len(c.execArgs) == 0 {
			t.Fatalf("%s: no update issued", tc.stage)
		}
		var got *string
		for _, a := range c.execArgs[0] {
			if p, ok := a.(*string); ok {
				got = p
			}
		}
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s with %q: error_message arg = %v, want %v", tc.stage, tc.msg, deref(got), deref(tc.want))
		}
	}
}

func strp(s string) *string { return &s }
