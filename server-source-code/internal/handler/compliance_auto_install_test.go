package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"golang.org/x/crypto/bcrypt"
)

func TestComplianceAvailabilityReportAutoInstallsOnce(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rc.Close() })

	const apiKey = "test-api-key"
	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hosts := store.NewHostsStore(d)
	status := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rc})
	h := NewIntegrationsHandler(hosts, nil, status, d, nil, nil, store.NewJobHistoryStore(d)).WithScannerAutoInstall(client, inspector)

	report := func(t *testing.T, apiID, state, osFamily string, openscap bool) {
		t.Helper()
		component := "failed"
		if openscap {
			component = "ready"
		}
		body, _ := json.Marshal(map[string]interface{}{
			"integration": "compliance", "enabled": true, "status": state,
			"message":      "All compliance tools failed to install",
			"components":   map[string]string{"openscap": component, "docker-bench": "failed"},
			"scanner_info": map[string]interface{}{"os_family": osFamily, "openscap_available": openscap},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/integration-status", strings.NewReader(string(body)))
		req.Header.Set("X-API-ID", apiID)
		req.Header.Set("X-API-KEY", apiKey)
		rec := httptest.NewRecorder()
		h.ReceiveIntegrationStatus(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
		}
	}

	cases := []struct {
		name       string
		onDemand   bool
		osFamily   string
		state      string
		openscap   bool
		hadHistory bool
		wantQueued bool
		dockerOnly bool
	}{
		{"enabled, scanner missing, debian", false, "debian", "error", false, false, true, false},
		{"enabled, scanner missing, rhel", false, "rhel", "partial", false, false, true, false},
		{"on-demand, scanner missing", true, "debian", "error", false, false, false, false},
		{"enabled, unsupported os family", false, "alpine", "error", false, false, false, false},
		{"enabled, an install has run before", false, "debian", "error", false, true, false, false},
		{"enabled, scanner ready", false, "debian", "ready", true, false, false, false},
		{"enabled, partial with openscap working", false, "debian", "partial", true, false, false, false},
		{"enabled, docker bench only", false, "debian", "error", false, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := newPendingHost("auto-"+uuid.NewString()[:8], "pending-"+uuid.NewString(), "patchmon_"+uuid.NewString()[:16], string(hash))
			host.ComplianceEnabled = true
			host.ComplianceOnDemandOnly = tc.onDemand
			host.ComplianceOpenscapEnabled = true
			if err := hosts.Create(ctx, host); err != nil {
				t.Fatal(err)
			}
			if tc.dockerOnly {
				if err := hosts.UpdateComplianceScanners(ctx, host.ID, false, true); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				_, _ = d.Exec(ctx, `DELETE FROM job_history WHERE host_id = $1`, host.ID)
				_, _ = d.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, host.ID)
			})
			if tc.hadHistory {
				if _, err := d.Exec(ctx, `INSERT INTO job_history (id, job_id, queue_name, job_name, host_id, api_id, status, attempt_number, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, 'failed', 1, NOW(), NOW())`,
					uuid.NewString(), uuid.NewString(), queue.QueueCompliance, queue.TypeInstallComplianceTools, host.ID, host.ApiID); err != nil {
					t.Fatal(err)
				}
			}

			report(t, host.ApiID, tc.state, tc.osFamily, tc.openscap)
			_, err := inspector.GetTaskInfo(queue.QueueCompliance, autoInstallTaskID(host.ID))
			if queued := err == nil; queued != tc.wantQueued {
				t.Fatalf("queued = %v, want %v (%v)", queued, tc.wantQueued, err)
			}
			if !tc.wantQueued {
				return
			}
			job, _ := status.GetComplianceInstallJob(ctx, host.ID)
			if job != autoInstallTaskID(host.ID) {
				t.Errorf("install job = %q", job)
			}

			report(t, host.ApiID, tc.state, tc.osFamily, tc.openscap)
			pending, err := inspector.ListPendingTasks(queue.QueueCompliance)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, p := range pending {
				var payload queue.InstallComplianceToolsPayload
				if json.Unmarshal(p.Payload, &payload) == nil && payload.HostID == host.ID {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%d install tasks queued after a second report, want 1", n)
			}
		})
	}
}

func TestCancelledAutoInstallIsNotQueuedAgain(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rc.Close() })

	const apiKey = "test-api-key"
	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hosts := store.NewHostsStore(d)
	jobHistory := store.NewJobHistoryStore(d)
	status := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rc})
	integrations := NewIntegrationsHandler(hosts, nil, status, d, nil, nil, jobHistory).WithScannerAutoInstall(client, inspector)
	compliance := (&ComplianceHandler{hostsStore: hosts, queueClient: client, queueInspector: inspector, integrationStatus: status}).WithJobHistory(jobHistory)

	host := newPendingHost("cancel-"+uuid.NewString()[:8], "pending-"+uuid.NewString(), "patchmon_"+uuid.NewString()[:16], string(hash))
	host.ID = uuid.NewString()
	host.ComplianceEnabled = true
	host.ComplianceOpenscapEnabled = true
	if err := hosts.Create(ctx, host); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(ctx, `DELETE FROM job_history WHERE host_id = $1`, host.ID)
		_, _ = d.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, host.ID)
	})

	report := func() {
		t.Helper()
		body := `{"integration":"compliance","enabled":true,"status":"error","components":{"openscap":"failed"},"scanner_info":{"os_family":"debian","openscap_available":false}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/integration-status", strings.NewReader(body))
		req.Header.Set("X-API-ID", host.ApiID)
		req.Header.Set("X-API-KEY", apiKey)
		rec := httptest.NewRecorder()
		integrations.ReceiveIntegrationStatus(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
		}
	}

	report()
	if _, err := inspector.GetTaskInfo(queue.QueueCompliance, autoInstallTaskID(host.ID)); err != nil {
		t.Fatalf("auto-install not queued: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/install-scanner/"+host.ID+"/cancel", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("hostId", host.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	compliance.CancelInstall(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := inspector.GetTaskInfo(queue.QueueCompliance, autoInstallTaskID(host.ID)); err == nil {
		t.Fatal("cancelled task still queued")
	}
	if has, err := jobHistory.HostHasJob(ctx, host.ID, queue.TypeInstallComplianceTools); err != nil || !has {
		t.Fatalf("cancel left no job history (has=%v, err=%v)", has, err)
	}

	report()
	if _, err := inspector.GetTaskInfo(queue.QueueCompliance, autoInstallTaskID(host.ID)); err == nil {
		t.Fatal("auto-install queued again after being cancelled")
	}
}
