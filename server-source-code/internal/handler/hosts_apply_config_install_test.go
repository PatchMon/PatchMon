package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func connectFakeAgent(t *testing.T, reg *agentregistry.Registry, apiID string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	reg.Register(apiID, false, "")
	reg.SetConnection(apiID, conn)
}

func TestApplyPendingConfigQueuesScannerInstall(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rc.Close() })

	hosts := store.NewHostsStore(d)
	pending := store.NewPendingConfigStore(d)
	status := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rc})
	reg := agentregistry.New()
	h := (&HostsHandler{hosts: hosts, pendingConfig: pending, integrationStatus: status, registry: reg, queueClient: client}).
		WithQueueInspector(inspector)

	ready := `{"status":"ready","scanner_info":{"openscap_available":true}}`
	notReady := `{"status":"error","scanner_info":{"openscap_available":false}}`
	partialOpenscapOK := `{"status":"partial","components":{"openscap":"ready","docker-bench":"failed"},"scanner_info":{"openscap_available":true}}`
	on, off := true, false

	cases := []struct {
		name          string
		wasEnabled    bool
		wasOnDemand   bool
		wasDockerB    bool
		scannerStatus string
		pending       store.PendingConfigFields
		installActive bool
		prevFinished  bool
		wantQueued    bool
	}{
		{"disabled to enabled", false, true, false, "", store.PendingConfigFields{ComplianceEnabled: &on, ComplianceOnDemandOnly: &off}, false, false, true},
		{"on-demand to enabled, scanner not ready", true, true, false, notReady, store.PendingConfigFields{ComplianceOnDemandOnly: &off}, false, false, true},
		{"on-demand to enabled, scanner ready", true, true, false, ready, store.PendingConfigFields{ComplianceOnDemandOnly: &off}, false, false, false},
		{"scanner not ready but install already running", true, true, false, notReady, store.PendingConfigFields{ComplianceOnDemandOnly: &off}, true, false, false},
		{"previous job still queued but its final step was reported", true, true, false, notReady, store.PendingConfigFields{ComplianceOnDemandOnly: &off}, true, true, true},
		{"docker bench newly enabled on a ready host", true, false, false, ready, store.PendingConfigFields{ComplianceDockerBenchEnabled: &on}, false, false, false},
		{"partial with openscap working", true, false, false, partialOpenscapOK, store.PendingConfigFields{ComplianceDockerBenchEnabled: &on}, false, false, false},
		{"disabling compliance", true, false, false, notReady, store.PendingConfigFields{ComplianceEnabled: &off}, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := newPendingHost("apply-"+uuid.NewString()[:8], "pending-"+uuid.NewString(), "patchmon_"+uuid.NewString()[:16], "hash")
			host.ComplianceEnabled = tc.wasEnabled
			host.ComplianceOnDemandOnly = tc.wasOnDemand
			host.ComplianceOpenscapEnabled = true
			host.ComplianceDockerBenchEnabled = tc.wasDockerB
			if err := hosts.Create(ctx, host); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = d.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, host.ID) })
			if tc.scannerStatus != "" {
				if _, err := d.Exec(ctx, `UPDATE hosts SET compliance_scanner_status = $2::jsonb WHERE id = $1`, host.ID, tc.scannerStatus); err != nil {
					t.Fatal(err)
				}
			}
			if err := pending.SetPendingConfig(ctx, host.ID, tc.pending); err != nil {
				t.Fatal(err)
			}
			var runningJob string
			if tc.installActive {
				task, err := queue.NewInstallComplianceToolsTask(host.ID, host.ApiID, "")
				if err != nil {
					t.Fatal(err)
				}
				info, err := client.Enqueue(task)
				if err != nil {
					t.Fatal(err)
				}
				runningJob = info.ID
				if err := status.SetComplianceInstallJob(ctx, host.ID, runningJob); err != nil {
					t.Fatal(err)
				}
				if tc.prevFinished {
					if err := status.Set(ctx, host.ApiID, "compliance", map[string]interface{}{
						"status": "ready", "install_job_id": runningJob,
						"install_events": []interface{}{map[string]interface{}{"step": "complete", "status": "done"}},
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
			connectFakeAgent(t, reg, host.ApiID)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/"+host.ID+"/integrations/apply-pending-config", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("hostId", host.ID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			rec := httptest.NewRecorder()
			h.ApplyPendingConfig(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
			}
			var out struct {
				ScannerInstallQueued bool `json:"scannerInstallQueued"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.ScannerInstallQueued != tc.wantQueued {
				t.Fatalf("scannerInstallQueued = %v, want %v", out.ScannerInstallQueued, tc.wantQueued)
			}
			job, _ := status.GetComplianceInstallJob(ctx, host.ID)
			switch {
			case tc.wantQueued && (job == "" || job == runningJob):
				t.Errorf("install job not recorded: %q", job)
			case !tc.wantQueued && job != runningJob:
				t.Errorf("install job changed to %q, want %q", job, runningJob)
			}
		})
	}
}
