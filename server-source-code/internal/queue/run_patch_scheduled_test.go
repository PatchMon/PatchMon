package queue

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

type fakeScheduledRuns struct {
	run         *db.GetPatchRunByIDRow
	activateOK  bool
	activateErr error
	activated   int
	skipped     string
}

func (f *fakeScheduledRuns) GetByID(context.Context, string) (*db.GetPatchRunByIDRow, error) {
	return f.run, nil
}

func (f *fakeScheduledRuns) ActivateScheduledRun(context.Context, string) (bool, error) {
	f.activated++
	return f.activateOK, f.activateErr
}

func (f *fakeScheduledRuns) SkipScheduledRun(_ context.Context, _ string, reason string) error {
	f.skipped = reason
	return nil
}

func TestActivateIfScheduled(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := RunPatchPayload{PatchRunID: "run-1", ApiID: "api-1"}

	cases := []struct {
		name         string
		runs         *fakeScheduledRuns
		wantProceed  bool
		wantActivate int
		wantSkipped  bool
	}{
		{"immediate run passes through", &fakeScheduledRuns{run: &db.GetPatchRunByIDRow{Status: "queued"}}, true, 0, false},
		{"missing run left to dispatch guard", &fakeScheduledRuns{}, true, 0, false},
		{"idle host activates", &fakeScheduledRuns{run: &db.GetPatchRunByIDRow{Status: "scheduled"}, activateOK: true}, true, 1, false},
		{"busy host skips", &fakeScheduledRuns{run: &db.GetPatchRunByIDRow{Status: "scheduled"}, activateErr: store.ErrHostHasActivePatchRun}, false, 1, true},
		{"cancelled before window drops", &fakeScheduledRuns{run: &db.GetPatchRunByIDRow{Status: "scheduled"}}, false, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proceed, err := activateIfScheduled(context.Background(), tc.runs, log, p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proceed != tc.wantProceed {
				t.Errorf("proceed = %v, want %v", proceed, tc.wantProceed)
			}
			if tc.runs.activated != tc.wantActivate {
				t.Errorf("activate calls = %d, want %d", tc.runs.activated, tc.wantActivate)
			}
			if got := tc.runs.skipped != ""; got != tc.wantSkipped {
				t.Errorf("skipped = %v, want %v", got, tc.wantSkipped)
			}
			if tc.wantSkipped && tc.runs.skipped != scheduledRunSkippedReason {
				t.Errorf("skip reason = %q", tc.runs.skipped)
			}
		})
	}
}

func TestActivateIfScheduledPropagatesOtherErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	boom := errors.New("connection reset")
	runs := &fakeScheduledRuns{run: &db.GetPatchRunByIDRow{Status: "scheduled"}, activateErr: boom}
	proceed, err := activateIfScheduled(context.Background(), runs, log, RunPatchPayload{PatchRunID: "run-1"})
	if !errors.Is(err, boom) || proceed {
		t.Fatalf("got proceed=%v err=%v, want the error returned so asynq retries", proceed, err)
	}
	if runs.skipped != "" {
		t.Fatal("a transient error must not skip the run")
	}
}
