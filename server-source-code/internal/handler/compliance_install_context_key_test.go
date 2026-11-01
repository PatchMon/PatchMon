package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
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

func TestInstallWorkerSeesAgentReportBehindForwardedHost(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	hosts := store.NewHostsStore(d)

	cases := []struct {
		name          string
		forwardedHost string
		entry         *hostctx.Entry
		wantTaskHost  string
	}{
		{"single context behind a proxy that sets X-Forwarded-Host", "patchmon.example.com", nil, ""},
		{"context entry whose stored host differs in case from the header", "Ctx-A.Example.com", &hostctx.Entry{Host: "ctx-a.example.com"}, "ctx-a.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			opt := asynq.RedisClientOpt{Addr: mr.Addr()}
			client := asynq.NewClient(opt)
			inspector := asynq.NewInspector(opt)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rc.Close() })
			status := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rc})
			reg := agentregistry.New()
			h := &ComplianceHandler{hostsStore: hosts, queueClient: client, queueInspector: inspector, integrationStatus: status, registry: reg}

			host := newPendingHost("key-"+uuid.NewString()[:8], "pending-"+uuid.NewString(), "patchmon_"+uuid.NewString()[:16], "hash")
			host.ID = uuid.NewString()
			if err := hosts.Create(ctx, host); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = d.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, host.ID) })
			connectFakeAgent(t, reg, host.ApiID)

			reqCtx := context.Background()
			if tc.entry != nil {
				reqCtx = hostctx.WithEntry(reqCtx, tc.entry)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/install-scanner/"+host.ID, nil)
			req.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("hostId", host.ID)
			req = req.WithContext(context.WithValue(reqCtx, chi.RouteCtxKey, rctx))
			rec := httptest.NewRecorder()
			h.InstallScanner(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				JobID string `json:"jobId"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			info, err := inspector.GetTaskInfo(queue.QueueCompliance, resp.JobID)
			if err != nil {
				t.Fatal(err)
			}
			var payload queue.InstallComplianceToolsPayload
			if err := json.Unmarshal(info.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Host != tc.wantTaskHost {
				t.Fatalf("task host = %q, want %q", payload.Host, tc.wantTaskHost)
			}

			if err := status.Set(reqCtx, host.ApiID, "compliance", map[string]interface{}{
				"status": "ready",
				"install_events": []interface{}{
					map[string]interface{}{"step": "complete", "status": "done", "message": "Installation complete"},
				},
			}); err != nil {
				t.Fatal(err)
			}

			worker := queue.NewInstallComplianceToolsHandler(reg, nil, nil, rc, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := worker.ProcessTask(runCtx, asynq.NewTask(queue.TypeInstallComplianceTools, info.Payload)); err != nil {
				t.Fatalf("worker did not see the agent's final report: %v", err)
			}
		})
	}
}
