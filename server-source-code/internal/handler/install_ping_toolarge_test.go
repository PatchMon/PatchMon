package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
)

func TestServePing_OversizedBodyRecordsActivityRow(t *testing.T) {
	d := testDatabase(t)
	ctx := context.Background()
	hosts := store.NewHostsStore(d)
	host := &models.Host{
		FriendlyName: "ping-413-" + uuid.New().String()[:8],
		OSType:       "linux",
		OSVersion:    "test",
		Status:       "active",
		ApiID:        "patchmon_" + uuid.New().String(),
		ApiKey:       "plaintext-" + uuid.New().String(),
	}
	if err := hosts.Create(ctx, host); err != nil {
		t.Fatal(err)
	}
	h := NewInstallHandler(hosts, store.NewSettingsStore(d), nil, store.NewReportStore(d), nil)

	body := `{"metrics":{"diskDetails":[{"mount":"` + strings.Repeat("x", 512) + `"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/ping", strings.NewReader(body))
	req.Header.Set("X-API-ID", host.ApiID)
	req.Header.Set("X-API-KEY", host.ApiKey)
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 64)
	h.ServePing(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "64b AGENT_PING_BODY_LIMIT") {
		t.Errorf("body = %s, want it to name the 64b AGENT_PING_BODY_LIMIT", rec.Body.String())
	}

	var status, reportType, msg string
	err := d.RawQueryRow(ctx,
		`SELECT status, report_type, COALESCE(error_message, '') FROM update_history WHERE host_id = $1`,
		host.ID).Scan(&status, &reportType, &msg)
	if err != nil {
		t.Fatalf("no activity row for the rejected ping: %v", err)
	}
	if status != "error" || reportType != "ping" || !strings.Contains(msg, "AGENT_PING_BODY_LIMIT") {
		t.Errorf("activity row = (%s, %s, %q), want (error, ping, message naming AGENT_PING_BODY_LIMIT)", status, reportType, msg)
	}
}
