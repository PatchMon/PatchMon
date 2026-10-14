package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHostPackagesPayloadWindowsPatchFields(t *testing.T) {
	pkgs := []ReportPackage{
		{Name: "linux-pkg", CurrentVersion: "1.0", NeedsUpdate: true},
		{Name: "7-Zip", CurrentVersion: "24.08", NeedsUpdate: true, WingetID: "7zip.7zip"},
		{Name: "Bad App", CurrentVersion: "1.0", NeedsUpdate: true, WingetID: "x';calc;'"},
		{Name: "Driver Update", CurrentVersion: "pending", NeedsUpdate: true, WUAGuid: "7f1b0c5e-1111-2222-3333-444455556666", WUABrowseOnly: true, WUAType: "Driver"},
		{Name: "Odd Type", CurrentVersion: "pending", NeedsUpdate: true, WUAGuid: "7f1b0c5e-1111-2222-3333-444455556667", WUAType: "Firmware"},
	}
	nameToID := map[string]string{"linux-pkg": "p1", "7-Zip": "p2", "Bad App": "p3", "Driver Update": "p4", "Odd Type": "p5"}

	raw, err := buildHostPackagesPayload("h1", pkgs, nameToID, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildHostPackagesPayload: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byPkg := map[string]map[string]any{}
	for _, r := range rows {
		byPkg[r["package_id"].(string)] = r
	}

	if got := byPkg["p2"]["winget_id"]; got != "7zip.7zip" {
		t.Errorf("winget_id = %v, want 7zip.7zip", got)
	}
	if got := byPkg["p3"]["winget_id"]; got != "" {
		t.Errorf("invalid winget_id stored as %v, want empty", got)
	}
	if got := byPkg["p2"]["wua_browse_only"]; got != nil {
		t.Errorf("non-WUA row wua_browse_only = %v, want null", got)
	}
	if got := byPkg["p4"]["wua_browse_only"]; got != true {
		t.Errorf("wua_browse_only = %v, want true", got)
	}
	if got := byPkg["p4"]["wua_update_type"]; got != "Driver" {
		t.Errorf("wua_update_type = %v, want Driver", got)
	}
	if got := byPkg["p5"]["wua_update_type"]; got != "" {
		t.Errorf("unknown wua_update_type stored as %v, want empty", got)
	}
	if got := byPkg["p5"]["wua_browse_only"]; got != false {
		t.Errorf("WUA row without BrowseOnly = %v, want false", got)
	}
}

func TestSanitizeReportPayloadWindowsPatchFields(t *testing.T) {
	p := &ReportPayload{Packages: []ReportPackage{{Name: "a", WingetID: "7zip.\x007zip", WUAType: "Soft\x00ware"}}}
	sanitizeReportPayload(p)
	if p.Packages[0].WingetID != "7zip.7zip" || p.Packages[0].WUAType != "Software" {
		t.Fatalf("NULs not stripped: %+v", p.Packages[0])
	}
}

func TestWindowsPatchIDValidation(t *testing.T) {
	good := []string{"7zip.7zip", "Notepad++.Notepad++", "Microsoft.VCRedist.2015+.x64", "9NBLGGH4NNS1"}
	bad := []string{"", "'", "\"", "a b", "a;b", "$(x)", "`x`", "-x", "..\\x", strings.Repeat("a", 129)}
	for _, id := range good {
		if !IsValidWingetID(id) {
			t.Errorf("IsValidWingetID(%q) = false, want true", id)
		}
	}
	for _, id := range bad {
		if IsValidWingetID(id) {
			t.Errorf("IsValidWingetID(%q) = true, want false", id)
		}
	}
	if !IsValidWUAGuid("7F1B0C5E-1111-2222-3333-444455556666") {
		t.Error("upper-case GUID rejected")
	}
	for _, id := range []string{"{7f1b0c5e-1111-2222-3333-444455556666}", "KB5044284", "7f1b0c5e-1111-2222-3333-44445555666"} {
		if IsValidWUAGuid(id) {
			t.Errorf("IsValidWUAGuid(%q) = true, want false", id)
		}
	}
}

func TestPatchablePredicateIdenticalAcrossQueries(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "packages.sql"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	const predicate = "THEN hp.needs_update AND (hp.wua_guid IS NOT NULL OR hp.winget_id IS NOT NULL) AND NOT COALESCE(hp.winget_pinned, false) AND NOT COALESCE(hp.winget_store_app, false)"
	for _, q := range []string{"GetHostPackagesWithHostsByPackageID", "GetHostRefsForPackageIDs", "ListHostsForPackage"} {
		if !strings.Contains(namedQueryIn(t, src, q), predicate) {
			t.Errorf("%s does not use the shared patchable predicate", q)
		}
	}
	items, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "patch_run_items.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(namedQueryIn(t, string(items), "ResolveWindowsPatchTargets"), "hp.needs_update AND (hp.wua_guid IS NOT NULL OR hp.winget_id IS NOT NULL) AND NOT COALESCE(hp.winget_pinned, false) AND NOT COALESCE(hp.winget_store_app, false)") {
		t.Error("ResolveWindowsPatchTargets does not use the shared patchable predicate")
	}
}

func namedQueryIn(t *testing.T, src, name string) string {
	t.Helper()
	marker := "-- name: " + name + " :"
	i := strings.Index(src, marker)
	if i < 0 {
		t.Fatalf("query %s not found", name)
	}
	rest := src[i+len(marker):]
	if j := strings.Index(rest, "-- name: "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestBuildHostPackagesPayloadFeatureUpgradeType(t *testing.T) {
	raw, err := buildHostPackagesPayload("h1", []ReportPackage{{Name: "24H2", CurrentVersion: "pending", NeedsUpdate: true, WUAGuid: "aaaaaaaa-1111-2222-3333-444455556666", WUAType: "FeatureUpgrade"}}, map[string]string{"24H2": "p1"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"wua_update_type":"FeatureUpgrade"`) {
		t.Fatalf("payload = %s", raw)
	}
}

func TestWithoutWindowsUpdates(t *testing.T) {
	got := withoutWindowsUpdates([]ReportPackage{
		{Name: "7-Zip", Category: "Application"},
		{Name: "CU (KB1)", Category: "Windows Update"},
		{Name: "KB5043080", Category: "Windows Update"},
		{Name: "curl"},
	})
	if len(got) != 2 || got[0].Name != "7-Zip" || got[1].Name != "curl" {
		t.Fatalf("got %+v", got)
	}
}

func TestDeleteExceptWindowsUpdatesKeepsOnlyThatCategory(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "host_report.sql"))
	if err != nil {
		t.Fatal(err)
	}
	q := namedQueryIn(t, string(b), "DeleteHostPackagesExceptWindowsUpdates")
	if !strings.Contains(q, "hp.host_id = $1") || !strings.Contains(q, "p.category = '"+windowsUpdateCategory+"'") {
		t.Fatalf("query does not scope to the host and the Windows Update category:\n%s", q)
	}
}

func TestPendingRestartPredicateInHostListings(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "packages.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"GetHostPackagesWithHostsByPackageID", "GetHostRefsForPackageIDs", "ListHostsForPackage"} {
		body := namedQueryIn(t, string(b), q)
		if !strings.Contains(body, "hp.current_version = 'pending restart'") || !strings.Contains(body, "wp.category = 'Windows Update'") || !strings.Contains(body, "pending_restart") {
			t.Errorf("%s does not return pending_restart for restart-pending Windows Updates", q)
		}
	}
}

func TestWingetPinnedIsStoredAndNotPatchable(t *testing.T) {
	raw, err := buildHostPackagesPayload("h1", []ReportPackage{
		{Name: "Pinned", CurrentVersion: "1", NeedsUpdate: true, WingetID: "a.b", WingetPinned: true},
		{Name: "Free", CurrentVersion: "1", NeedsUpdate: true, WingetID: "c.d"},
	}, map[string]string{"Pinned": "p1", "Free": "p2"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"winget_pinned":true`) || strings.Count(string(raw), `"winget_pinned":null`) != 1 {
		t.Fatalf("payload = %s", raw)
	}
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "packages.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"GetHostPackagesWithHostsByPackageID", "GetHostRefsForPackageIDs", "ListHostsForPackage"} {
		if !strings.Contains(namedQueryIn(t, string(b), q), "COALESCE(hp.winget_pinned, false) THEN 'winget_pinned'") {
			t.Errorf("%s does not give pinned apps the winget_pinned reason", q)
		}
	}
	items, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "patch_run_items.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(namedQueryIn(t, string(items), "ResolveWindowsPatchTargets"), "THEN 'winget_pinned'") {
		t.Error("ResolveWindowsPatchTargets does not reject pinned apps")
	}
}

func TestWingetStoreAppIsStoredAndNotPatchable(t *testing.T) {
	raw, err := buildHostPackagesPayload("h1", []ReportPackage{
		{Name: "Windows Terminal", CurrentVersion: "1.20", NeedsUpdate: true, WingetID: "Microsoft.WindowsTerminal", WingetStoreApp: true},
		{Name: "Free", CurrentVersion: "1", NeedsUpdate: true, WingetID: "c.d"},
	}, map[string]string{"Windows Terminal": "p1", "Free": "p2"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"winget_store_app":true`) || strings.Count(string(raw), `"winget_store_app":null`) != 1 {
		t.Fatalf("payload = %s", raw)
	}
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "packages.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"GetHostPackagesWithHostsByPackageID", "GetHostRefsForPackageIDs", "ListHostsForPackage"} {
		if !strings.Contains(namedQueryIn(t, string(b), q), "COALESCE(hp.winget_store_app, false) THEN 'store_app'") {
			t.Errorf("%s does not give Store apps the store_app reason", q)
		}
	}
	items, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "patch_run_items.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(namedQueryIn(t, string(items), "ResolveWindowsPatchTargets"), "THEN 'store_app'") {
		t.Error("ResolveWindowsPatchTargets does not reject Store apps")
	}
}

func TestReportCountsIncludeKeptWindowsUpdates(t *testing.T) {
	sent := []ReportPackage{
		{Name: "7-Zip", NeedsUpdate: true},
		{Name: "Notepad++"},
	}
	got := countReportPackages(sent).plus(reportCounts{updates: 3, security: 2})
	if got.updates != 4 || got.security != 2 {
		t.Fatalf("counts = %+v, want updates 4 security 2", got)
	}
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "host_report.sql"))
	if err != nil {
		t.Fatal(err)
	}
	q := namedQueryIn(t, string(b), "CountHostWindowsUpdateRows")
	if !strings.Contains(q, "hp.host_id = $1") || !strings.Contains(q, "p.category = '"+windowsUpdateCategory+"'") {
		t.Fatalf("CountHostWindowsUpdateRows must count exactly the rows DeleteHostPackagesExceptWindowsUpdates keeps:\n%s", q)
	}
}
