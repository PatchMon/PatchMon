package store

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestSortReportInputs_PackagesSortedByName verifies that arbitrary input
// ordering of the agent's packages array is sorted by Name in place. This is
// the deterministic-lock-order property that prevents 40P01 deadlocks.
func TestSortReportInputs_PackagesSortedByName(t *testing.T) {
	payload := &ReportPayload{
		Packages: []ReportPackage{
			{Name: "zlib"},
			{Name: "openssl"},
			{Name: "curl"},
			{Name: "apt"},
			{Name: "bash"},
		},
	}

	sortReportInputs(payload)

	got := make([]string, 0, len(payload.Packages))
	for _, p := range payload.Packages {
		got = append(got, p.Name)
	}
	want := []string{"apt", "bash", "curl", "openssl", "zlib"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages not sorted: got %v want %v", got, want)
	}
}

// TestSortReportInputs_StableForEqualNames confirms the sort is *stable*:
// when two packages share a name (which we should never see in practice but
// is theoretically possible from a noisy agent), insertion order is preserved.
// Stability matters because we rely on the SQL `ORDER BY name` to break ties
// the same way pgx-side serialization does.
func TestSortReportInputs_StableForEqualNames(t *testing.T) {
	payload := &ReportPayload{
		Packages: []ReportPackage{
			{Name: "curl", CurrentVersion: "first"},
			{Name: "curl", CurrentVersion: "second"},
			{Name: "apt"},
		},
	}
	sortReportInputs(payload)
	if payload.Packages[0].Name != "apt" {
		t.Fatalf("apt should be first, got %s", payload.Packages[0].Name)
	}
	if payload.Packages[1].CurrentVersion != "first" || payload.Packages[2].CurrentVersion != "second" {
		t.Fatalf("stability broken: got %+v", payload.Packages)
	}
}

// TestSortReportInputs_RepositoriesSorted verifies the (URL, Distribution,
// Components) tuple ordering used by GetRepositoryByURLDistComponents.
func TestSortReportInputs_RepositoriesSorted(t *testing.T) {
	payload := &ReportPayload{
		Repositories: []ReportRepository{
			{URL: "https://b.example", Distribution: "bookworm", Components: "main"},
			{URL: "https://a.example", Distribution: "bookworm", Components: "main"},
			{URL: "https://a.example", Distribution: "bookworm", Components: "contrib"},
			{URL: "https://a.example", Distribution: "bullseye", Components: "main"},
		},
	}
	sortReportInputs(payload)

	want := []ReportRepository{
		{URL: "https://a.example", Distribution: "bookworm", Components: "contrib"},
		{URL: "https://a.example", Distribution: "bookworm", Components: "main"},
		{URL: "https://a.example", Distribution: "bullseye", Components: "main"},
		{URL: "https://b.example", Distribution: "bookworm", Components: "main"},
	}
	if !reflect.DeepEqual(payload.Repositories, want) {
		t.Fatalf("repos not sorted by (URL, Distribution, Components):\n got %+v\nwant %+v", payload.Repositories, want)
	}
}

// TestBuildPackageUpsertPayload_NullHandling exercises the JSON encoding the
// SQL side relies on:
//   - missing description / category / latest_version map to JSON null, which
//     jsonb_to_recordset treats as SQL NULL, which the COALESCE in ON CONFLICT
//     preserves the existing column value for.
//   - present strings round-trip verbatim.
func TestBuildPackageUpsertPayload_NullHandling(t *testing.T) {
	pkgs := []ReportPackage{
		{Name: "with-everything", Description: "desc", Category: "cat", AvailableVersion: strPtr("2.0")},
		{Name: "no-desc", Category: "cat"},
		{Name: "no-cat", Description: "desc"},
		{Name: "no-version", Description: "desc", Category: "cat"},
		{Name: "empty-string-version", Description: "desc", AvailableVersion: strPtr("")},
		{Name: "all-null"},
	}
	raw, err := buildPackageUpsertPayload(pkgs)
	if err != nil {
		t.Fatalf("buildPackageUpsertPayload: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v\npayload=%s", err, raw)
	}
	if len(rows) != len(pkgs) {
		t.Fatalf("row count mismatch: got %d want %d", len(rows), len(pkgs))
	}

	cases := []struct {
		idx                                          int
		wantDesc, wantCategory, wantVersion          any // nil == JSON null / key absent
		descPresent, categoryPresent, versionPresent bool
	}{
		{0, "desc", "cat", "2.0", true, true, true},
		{1, nil, "cat", nil, false, true, false},
		{2, "desc", nil, nil, true, false, false},
		{3, "desc", "cat", nil, true, true, false},
		{4, "desc", nil, nil, true, false, false}, // empty *string treated as absent
		{5, nil, nil, nil, false, false, false},
	}
	for _, tc := range cases {
		row := rows[tc.idx]
		if got, ok := row["description"]; tc.descPresent {
			if !ok || got != tc.wantDesc {
				t.Fatalf("row %d: description = %v (present=%v) want %v", tc.idx, got, ok, tc.wantDesc)
			}
		} else if ok {
			t.Fatalf("row %d: description present but should be absent: %v", tc.idx, got)
		}
		if got, ok := row["category"]; tc.categoryPresent {
			if !ok || got != tc.wantCategory {
				t.Fatalf("row %d: category = %v (present=%v) want %v", tc.idx, got, ok, tc.wantCategory)
			}
		} else if ok {
			t.Fatalf("row %d: category present but should be absent: %v", tc.idx, got)
		}
		if got, ok := row["latest_version"]; tc.versionPresent {
			if !ok || got != tc.wantVersion {
				t.Fatalf("row %d: latest_version = %v (present=%v) want %v", tc.idx, got, ok, tc.wantVersion)
			}
		} else if ok {
			t.Fatalf("row %d: latest_version present but should be absent: %v", tc.idx, got)
		}

		// Every row MUST have a non-empty id and the original name.
		id, _ := row["id"].(string)
		if id == "" {
			t.Fatalf("row %d: missing id", tc.idx)
		}
		name, _ := row["name"].(string)
		if name != pkgs[tc.idx].Name {
			t.Fatalf("row %d: name = %q want %q", tc.idx, name, pkgs[tc.idx].Name)
		}
	}
}

// TestBuildHostPackagesPayload_LinuxAndWindowsMixed asserts the unified
// JSON encoding handles both kinds of agent input in one call:
//   - Linux/FreeBSD entries (no WUAGuid) emit empty WUA fields and no
//     wua_categories key (so the SQL CASE jsonb_typeof = 'array' returns
//     NULL).
//   - Windows entries emit populated wua_* fields and a real JSON array
//     for wua_categories.
func TestBuildHostPackagesPayload_LinuxAndWindowsMixed(t *testing.T) {
	hostID := "host-1"
	pkgs := []ReportPackage{
		// Sorted by name (caller is responsible for sorting).
		{
			Name:           "linux-pkg",
			CurrentVersion: "1.2.3",
			NeedsUpdate:    true,
		},
		{
			Name:              "windows-update",
			CurrentVersion:    "10.0.19041.1",
			AvailableVersion:  strPtr("10.0.19041.2"),
			NeedsUpdate:       true,
			IsSecurityUpdate:  true,
			Description:       "Windows Update KB123",
			WUAGuid:           "{abc-def}",
			WUAKb:             "KB123",
			WUASeverity:       "Critical",
			WUACategories:     []string{"Critical Updates", "Security Updates"},
			WUASupportURL:     "https://support.example/KB123",
			WUARevisionNumber: 5,
		},
	}
	nameToID := map[string]string{
		"linux-pkg":      "pkg-uuid-linux",
		"windows-update": "pkg-uuid-windows",
	}

	raw, err := buildHostPackagesPayload(hostID, pkgs, nameToID, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildHostPackagesPayload: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v\npayload=%s", err, raw)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Row 0: Linux entry. Required fields populated; wua_categories absent
	// (omitempty); wua_* strings empty (NULLIF on SQL side -> NULL).
	r0 := rows[0]
	if r0["host_id"].(string) != hostID || r0["package_id"].(string) != "pkg-uuid-linux" {
		t.Fatalf("row0 host/pkg id wrong: %+v", r0)
	}
	if r0["needs_update"].(bool) != true || r0["is_security_update"].(bool) != false {
		t.Fatalf("row0 boolean flags wrong: %+v", r0)
	}
	if r0["wua_guid"].(string) != "" || r0["wua_kb"].(string) != "" {
		t.Fatalf("row0 expected empty WUA strings, got %+v", r0)
	}
	if _, hasCats := r0["wua_categories"]; hasCats {
		t.Fatalf("row0 should not emit wua_categories key (omitempty), got %+v", r0)
	}
	if r0["wua_revision_number"].(float64) != 0 {
		t.Fatalf("row0 wua_revision_number should be 0, got %v", r0["wua_revision_number"])
	}

	// Row 1: Windows entry. wua_categories is a real JSON array.
	r1 := rows[1]
	if r1["wua_guid"].(string) != "{abc-def}" {
		t.Fatalf("row1 wua_guid wrong: %v", r1["wua_guid"])
	}
	cats, ok := r1["wua_categories"].([]any)
	if !ok {
		t.Fatalf("row1 wua_categories should be a JSON array, got %T %v", r1["wua_categories"], r1["wua_categories"])
	}
	if len(cats) != 2 || cats[0].(string) != "Critical Updates" {
		t.Fatalf("row1 wua_categories wrong: %+v", cats)
	}
	if r1["wua_revision_number"].(float64) != 5 {
		t.Fatalf("row1 wua_revision_number wrong: %v", r1["wua_revision_number"])
	}
	if r1["available_version"].(string) != "10.0.19041.2" {
		t.Fatalf("row1 available_version wrong: %v", r1["available_version"])
	}

	// Each row must have a unique non-empty id.
	id0 := r0["id"].(string)
	id1 := r1["id"].(string)
	if id0 == "" || id1 == "" || id0 == id1 {
		t.Fatalf("row ids invalid or duplicate: %q %q", id0, id1)
	}
}

// TestBuildHostPackagesPayload_SourceRepoResolved checks that the package's
// SourceRepository string is resolved against the lookup maps and emitted as
// the resolved repository ID (not the raw agent string).
func TestBuildHostPackagesPayload_SourceRepoResolved(t *testing.T) {
	pkgs := []ReportPackage{
		{Name: "a-pkg", CurrentVersion: "1", SourceRepository: "baseos"},
		{Name: "b-pkg", CurrentVersion: "1", SourceRepository: "unknown"},   // sentinel -> empty
		{Name: "c-pkg", CurrentVersion: "1", SourceRepository: "non-match"}, // unmatched -> empty
	}
	nameToID := map[string]string{"a-pkg": "p1", "b-pkg": "p2", "c-pkg": "p3"}
	reposByName := map[string]string{"baseos": "repo-uuid-1"}

	raw, err := buildHostPackagesPayload("h1", pkgs, nameToID, reposByName, nil, nil)
	if err != nil {
		t.Fatalf("buildHostPackagesPayload: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rows[0]["source_repository_id"].(string) != "repo-uuid-1" {
		t.Fatalf("row0 source repo not resolved: %v", rows[0]["source_repository_id"])
	}
	if rows[1]["source_repository_id"].(string) != "" {
		t.Fatalf("row1 sentinel should yield empty source_repository_id, got %v", rows[1]["source_repository_id"])
	}
	if rows[2]["source_repository_id"].(string) != "" {
		t.Fatalf("row2 unmatched should yield empty source_repository_id, got %v", rows[2]["source_repository_id"])
	}
}

// TestBuildHostPackagesPayload_MissingPackageIDFails defends against an
// internal bug: BulkUpsertPackages should return one row per input name.
// If the caller assembles a host_packages payload before that result is
// available (or with a stale map), the helper must error rather than emit
// garbage rows.
func TestBuildHostPackagesPayload_MissingPackageIDFails(t *testing.T) {
	pkgs := []ReportPackage{{Name: "missing", CurrentVersion: "1"}}
	_, err := buildHostPackagesPayload("h1", pkgs, map[string]string{}, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error when nameToID is missing the package, got nil")
	}
	if !strings.Contains(err.Error(), "missing") && !strings.Contains(err.Error(), "no upserted ID") {
		t.Fatalf("error message should mention the missing package, got: %v", err)
	}
}

// TestBuildPackageUpsertPayload_PreservesOrder confirms the helper does NOT
// re-sort: it serialises in the order it receives. This is the contract that
// lets ProcessReport sort once (in sortReportInputs) and trust the order
// downstream.
func TestBuildPackageUpsertPayload_PreservesOrder(t *testing.T) {
	pkgs := []ReportPackage{
		{Name: "zzz"},
		{Name: "aaa"},
		{Name: "mmm"},
	}
	raw, err := buildPackageUpsertPayload(pkgs)
	if err != nil {
		t.Fatalf("buildPackageUpsertPayload: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r["name"].(string))
	}
	want := []string{"zzz", "aaa", "mmm"}
	if !slices.Equal(got, want) {
		t.Fatalf("buildPackageUpsertPayload reordered rows: got %v want %v", got, want)
	}
}
