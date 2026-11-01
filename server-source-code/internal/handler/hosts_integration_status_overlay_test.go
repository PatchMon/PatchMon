package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func TestComplianceStatusShowsInstallingWhileJobRuns(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rc.Close() })

	hosts := store.NewHostsStore(d)
	status := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rc})
	h := (&HostsHandler{hosts: hosts, integrationStatus: status}).WithQueueInspector(inspector)

	inProgress := []interface{}{map[string]interface{}{"step": "install_openscap", "status": "in_progress", "message": "Installing OpenSCAP packages..."}}
	finished := []interface{}{map[string]interface{}{"step": "complete", "status": "done", "message": "Installation complete"}}
	agentReport := func(state, jobID string, events []interface{}) map[string]interface{} {
		return map[string]interface{}{
			"status": state, "message": "All compliance tools failed to install",
			"install_job_id": jobID,
			"install_events": events,
		}
	}

	cases := []struct {
		name            string
		report          string
		jobRunning      bool
		eventsFromOther bool
		wantStatus      string
		wantReported    string
		wantEvents      int
		finalStep       bool
	}{
		{"agent reports error while install pending", "error", true, false, "installing", "error", 1, false},
		{"agent reports partial while install pending", "partial", true, false, "installing", "partial", 1, false},
		{"previous run's events hidden while a new install runs", "error", true, true, "installing", "error", 0, true},
		{"no install job: real status passes through", "error", false, false, "error", "", 1, false},
		{"agent's own installing status is untouched", "installing", true, false, "installing", "", 1, false},
		{"final step reported before the worker notices", "ready", true, false, "ready", "", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := newPendingHost("ovl-"+uuid.NewString()[:8], "pending-"+uuid.NewString(), "patchmon_"+uuid.NewString()[:16], "hash")
			if err := hosts.Create(ctx, host); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = d.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, host.ID) })

			jobID := "gone-" + uuid.NewString()
			if tc.jobRunning {
				task, err := queue.NewInstallComplianceToolsTask(host.ID, host.ApiID, "")
				if err != nil {
					t.Fatal(err)
				}
				info, err := client.Enqueue(task)
				if err != nil {
					t.Fatal(err)
				}
				jobID = info.ID
			}
			if err := status.SetComplianceInstallJob(ctx, host.ID, jobID); err != nil {
				t.Fatal(err)
			}
			eventsJob := jobID
			if tc.eventsFromOther {
				eventsJob = "previous-" + uuid.NewString()
			}
			events := inProgress
			if tc.finalStep {
				events = finished
			}
			if err := status.Set(ctx, host.ApiID, "compliance", agentReport(tc.report, eventsJob, events)); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/integrations/compliance/status", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("hostId", host.ID)
			rctx.URLParams.Add("integrationName", "compliance")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			rec := httptest.NewRecorder()
			h.GetIntegrationStatus(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
			}
			var out struct {
				Status map[string]interface{} `json:"status"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Status["status"] != tc.wantStatus {
				t.Errorf("status = %v, want %q", out.Status["status"], tc.wantStatus)
			}
			if got, _ := out.Status["reported_status"].(string); got != tc.wantReported {
				t.Errorf("reported_status = %q, want %q", got, tc.wantReported)
			}
			if evts, _ := out.Status["install_events"].([]interface{}); len(evts) != tc.wantEvents {
				t.Errorf("install_events = %d, want %d", len(evts), tc.wantEvents)
			}
			stored, err := status.Get(ctx, host.ApiID, "compliance")
			if err != nil || stored["status"] != tc.report {
				t.Errorf("stored agent report changed: %v (%v)", stored["status"], err)
			}
		})
	}
}
