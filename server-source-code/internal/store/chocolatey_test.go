package store

import (
	"context"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
)

func TestChocolateyInventoryAndPatchTargetsDB(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	host := pendingHost(t, hosts)
	available := "2.0"
	pkgs := []ReportPackage{
		{Name: "chocolatey:app", ChocolateyID: "app", CurrentVersion: "1.0", AvailableVersion: &available, NeedsUpdate: true},
		{Name: "chocolatey:pinned", ChocolateyID: "pinned", ChocolateyPinned: true, CurrentVersion: "1.0", AvailableVersion: &available, NeedsUpdate: true},
		{Name: "chocolatey:legacy", CurrentVersion: "1.0", AvailableVersion: &available, NeedsUpdate: true},
		{Name: "chocolatey:all", ChocolateyID: "all", CurrentVersion: "1.0", AvailableVersion: &available, NeedsUpdate: true},
	}
	payload := &ReportPayload{OSType: "Windows Server 2025", OSVersion: "2025", Hostname: "choco-test", Packages: pkgs}
	reports := NewReportStore(d)
	if _, err := reports.ProcessReport(ctx, host.ID, payload, FullReport(), ReportHashes{}); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range pkgs {
		names = append(names, p.Name)
	}
	rows, err := d.Queries.ResolveWindowsPatchTargets(ctx, db.ResolveWindowsPatchTargetsParams{HostID: host.ID, Names: names})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows=%+v", rows)
	}
	for _, r := range rows {
		if r.Patchable != (r.Name == "chocolatey:app") {
			t.Fatalf("unsafe patchability: %+v", r)
		}
		if r.Name == "chocolatey:pinned" && r.NotPatchableReason != "chocolatey_pinned" {
			t.Fatalf("pin not honoured: %+v", r)
		}
	}
	// A reporting-only agent must clear capability metadata from a previous report.
	for i := range payload.Packages {
		if payload.Packages[i].Name == "chocolatey:app" {
			payload.Packages[i].ChocolateyID = ""
		}
	}
	if _, err := reports.ProcessReport(ctx, host.ID, payload, FullReport(), ReportHashes{}); err != nil {
		t.Fatal(err)
	}
	rows, err = d.Queries.ResolveWindowsPatchTargets(ctx, db.ResolveWindowsPatchTargetsParams{HostID: host.ID, Names: []string{"chocolatey:app"}})
	if err != nil || len(rows) != 1 || rows[0].Patchable {
		t.Fatalf("stale capability retained: %+v %v", rows, err)
	}
}

func TestChocolateyItemValidation(t *testing.T) {
	rows, err := normaliseAgentItems([]AgentItemReport{{Kind: PatchItemKindChocolatey, ID: "App.Install", Status: "installed"}})
	if err != nil || len(rows) != 1 || rows[0].Identifier != "app.install" {
		t.Fatalf("item rejected: %+v %v", rows, err)
	}
	for _, id := range []string{"all", "ALL", "--force", "a;b", "a b"} {
		if _, err := normaliseAgentItems([]AgentItemReport{{Kind: PatchItemKindChocolatey, ID: id, Status: "installed"}}); err == nil {
			t.Fatalf("unsafe id accepted: %q", id)
		}
	}
}

func TestChocolateyRunItemRoundTripDB(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	host := pendingHost(t, hosts)
	runID := uuid.NewString()
	_, err := d.Exec(ctx, `INSERT INTO patch_runs (id, host_id, job_id, patch_type, status, updated_at) VALUES ($1, $2, $3, 'patch_package', 'running', NOW())`, runID, host.ID, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if err := insertPatchRunItems(ctx, d.Queries, runID, host.ID, []NewPatchRunItem{{Kind: PatchItemKindChocolatey, Identifier: "app", Title: "chocolatey:app"}}); err != nil {
		t.Fatal(err)
	}
	runs := NewPatchRunsStore(d)
	code := int64(0)
	result, err := runs.RecordAgentItems(ctx, runID, host.ID, false, []AgentItemReport{{Kind: PatchItemKindChocolatey, ID: "app", Status: "installed", ExitCode: &code}})
	if err != nil || result.Updated != 1 {
		t.Fatalf("report=%+v err=%v", result, err)
	}
	items, err := runs.ListItems(ctx, runID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].Kind != PatchItemKindChocolatey || items[0].Identifier != "app" || items[0].Status != "installed" || items[0].ExitCode == nil || *items[0].ExitCode != 0 {
		t.Fatalf("item=%+v", items[0])
	}
}
