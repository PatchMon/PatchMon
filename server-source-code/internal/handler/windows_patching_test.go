package handler

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func boolp(b bool) *bool  { return &b }
func i32p(i int32) *int32 { return &i }

const testGUID = "7F1B0C5E-1111-2222-3333-444455556666"

func fixedResolver(rows ...db.ResolveWindowsPatchTargetsRow) windowsTargetResolver {
	return func(names []string) ([]db.ResolveWindowsPatchTargetsRow, error) {
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		var out []db.ResolveWindowsPatchTargetsRow
		for _, r := range rows {
			if want[r.Name] {
				out = append(out, r)
			}
		}
		return out, nil
	}
}

func wuRow(name string) db.ResolveWindowsPatchTargetsRow {
	return db.ResolveWindowsPatchTargetsRow{Name: name, NeedsUpdate: true, WuaGuid: strp(testGUID), WuaRevisionNumber: i32p(201), WuaKb: strp("KB5044284"), Patchable: true}
}

func appRow(name, id string) db.ResolveWindowsPatchTargetsRow {
	return db.ResolveWindowsPatchTargetsRow{Name: name, NeedsUpdate: true, WingetID: strp(id), Patchable: true}
}

func TestPlanWindowsRunAgentVersionGate(t *testing.T) {
	cases := []struct {
		name    string
		version *string
		wantErr bool
	}{
		{"missing", nil, true},
		{"empty", strp(""), true},
		{"older", strp("2.1.3"), true},
		{"pre-release of older", strp("2.1.3-rc.9"), true},
		{"pre-release of minimum", strp("2.1.4-rc.1"), false},
		{"numeric pre-release of minimum", strp("2.1.4-1"), false},
		{"minimum", strp(MinWindowsPatchAgentVersion), false},
		{"newer", strp("2.2.0"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, reqErr, err := planWindowsRun(tc.version, "patch_all", nil, nil, fixedResolver())
			if err != nil {
				t.Fatal(err)
			}
			if (reqErr != nil) != tc.wantErr {
				t.Fatalf("reqErr = %v, wantErr %v", reqErr, tc.wantErr)
			}
			if reqErr != nil && !strings.Contains(reqErr.Body["error"].(string), "v"+MinWindowsPatchAgentVersion) {
				t.Fatalf("message does not name the minimum version: %v", reqErr.Body["error"])
			}
		})
	}
}

func TestPlanWindowsRunPatchAllScope(t *testing.T) {
	v := strp("2.1.4")
	cases := []struct {
		name        string
		scope       *windowsScope
		wantUpdates bool
		wantApps    bool
		wantErr     bool
	}{
		{"nil defaults both on", nil, true, true, false},
		{"empty defaults both on", &windowsScope{}, true, true, false},
		{"updates only", &windowsScope{Updates: boolp(true), Apps: boolp(false)}, true, false, false},
		{"apps only", &windowsScope{Updates: boolp(false)}, false, true, false},
		{"both off", &windowsScope{Updates: boolp(false), Apps: boolp(false)}, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, reqErr, err := planWindowsRun(v, "patch_all", []string{"ignored"}, tc.scope, fixedResolver())
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantErr {
				if reqErr == nil || reqErr.Status != http.StatusBadRequest {
					t.Fatalf("want 400, got %v", reqErr)
				}
				return
			}
			if reqErr != nil {
				t.Fatalf("unexpected reject: %v", reqErr.Body)
			}
			if *plan.IncludeUpdates != tc.wantUpdates || *plan.IncludeApps != tc.wantApps {
				t.Fatalf("scope = %v/%v, want %v/%v", *plan.IncludeUpdates, *plan.IncludeApps, tc.wantUpdates, tc.wantApps)
			}
			if len(plan.Items) != 0 {
				t.Fatalf("patch_all must not pre-create items, got %d", len(plan.Items))
			}
		})
	}
}

func TestPlanWindowsRunPatchPackage(t *testing.T) {
	v := strp("2.1.4")
	upgrade := wuRow("Windows 11, version 24H2 (KB5044285)")
	upgrade.WuaGuid = strp("aaaaaaaa-1111-2222-3333-444455556666")
	upgrade.WuaCategories = []byte(`["Upgrades"]`)
	resolver := fixedResolver(
		wuRow("2026-10 Cumulative Update (KB5044284)"),
		appRow("7-Zip 24.08 (x64)", "7zip.7zip"),
		appRow("Notepad++", "Notepad++.Notepad++"),
		upgrade,
		db.ResolveWindowsPatchTargetsRow{Name: "Legacy App", NeedsUpdate: true, NotPatchableReason: "no_winget_id"},
		db.ResolveWindowsPatchTargetsRow{Name: "Up To Date", NeedsUpdate: false, WingetID: strp("x.y"), NotPatchableReason: "not_pending"},
		appRow("Tampered", "x';calc;'"),
		db.ResolveWindowsPatchTargetsRow{Name: "Pinned App", NeedsUpdate: true, WingetID: strp("a.b"), NotPatchableReason: "winget_pinned"},
		db.ResolveWindowsPatchTargetsRow{Name: "Store App", NeedsUpdate: true, WingetID: strp("Microsoft.WindowsTerminal"), NotPatchableReason: "store_app"},
	)

	t.Run("resolves updates and apps to identifiers", func(t *testing.T) {
		plan, reqErr, err := planWindowsRun(v, "patch_package", []string{"2026-10 Cumulative Update (KB5044284)", "7-Zip 24.08 (x64)", "Notepad++", "7-Zip 24.08 (x64)"}, nil, resolver)
		if err != nil || reqErr != nil {
			t.Fatalf("err=%v reqErr=%v", err, reqErr)
		}
		if len(plan.Items) != 3 {
			t.Fatalf("got %d items, want 3 (duplicates removed): %+v", len(plan.Items), plan.Items)
		}
		wu := plan.Items[0]
		if wu.Kind != store.PatchItemKindWUA || wu.Identifier != strings.ToLower(testGUID) || wu.RevisionNumber != 201 || wu.KB != "KB5044284" {
			t.Fatalf("WUA item wrong: %+v", wu)
		}
		if plan.Items[1].Kind != store.PatchItemKindWinget || plan.Items[1].Identifier != "7zip.7zip" {
			t.Fatalf("winget item wrong: %+v", plan.Items[1])
		}
		if plan.IncludeUpdates != nil || plan.IncludeApps != nil {
			t.Fatal("patch_package must store NULL scope")
		}
	})

	t.Run("feature upgrade accepted when named", func(t *testing.T) {
		plan, reqErr, err := planWindowsRun(v, "patch_package", []string{upgrade.Name}, nil, resolver)
		if err != nil || reqErr != nil {
			t.Fatalf("err=%v reqErr=%v", err, reqErr)
		}
		if len(plan.Items) != 1 || plan.Items[0].Kind != store.PatchItemKindWUA {
			t.Fatalf("items = %+v", plan.Items)
		}
	})

	t.Run("rejects list every unpatchable name with its reason", func(t *testing.T) {
		_, reqErr, err := planWindowsRun(v, "patch_package", []string{"7-Zip 24.08 (x64)", "Legacy App", "Up To Date", "Missing", "Tampered", "Pinned App", "Store App"}, nil, resolver)
		if err != nil {
			t.Fatal(err)
		}
		if reqErr == nil {
			t.Fatal("want 400")
		}
		rejects := reqErr.Body["rejected"].([]windowsTargetReject)
		want := map[string]string{"Legacy App": rejectNoWingetID, "Up To Date": rejectNotPending, "Missing": rejectNotFound, "Tampered": rejectInvalidIdentifier, "Pinned App": rejectWingetPinned, "Store App": rejectStoreApp}
		if len(rejects) != len(want) {
			t.Fatalf("rejects = %+v", rejects)
		}
		for _, r := range rejects {
			if want[r.Name] != r.Reason {
				t.Errorf("%s: reason %q, want %q", r.Name, r.Reason, want[r.Name])
			}
		}
	})

	t.Run("name validation", func(t *testing.T) {
		over := make([]string, maxPatchTargets+1)
		for i := range over {
			over[i] = "n"
		}
		for name, names := range map[string][]string{
			"empty":        nil,
			"too many":     over,
			"control char": {"bad\x07name"},
			"newline":      {"bad\nname"},
			"too long":     {strings.Repeat("a", maxWindowsPackageNameLen+1)},
			"empty name":   {""},
		} {
			_, reqErr, err := planWindowsRun(v, "patch_package", names, nil, resolver)
			if err != nil || reqErr == nil || reqErr.Status != http.StatusBadRequest {
				t.Errorf("%s: want 400, got reqErr=%v err=%v", name, reqErr, err)
			}
		}
		_, reqErr, _ := planWindowsRun(v, "patch_package", []string{"Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810"}, nil, fixedResolver(appRow("Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810", "Microsoft.VCRedist.2015+.x64")))
		if reqErr != nil {
			t.Errorf("display name with spaces and parentheses rejected: %v", reqErr.Body)
		}
	})

	t.Run("resolver error is returned", func(t *testing.T) {
		boom := errors.New("db down")
		_, _, err := planWindowsRun(v, "patch_package", []string{"x"}, nil, func([]string) ([]db.ResolveWindowsPatchTargetsRow, error) { return nil, boom })
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestItemsForApproval(t *testing.T) {
	items := []db.PatchRunItem{
		{Kind: "wua", Identifier: "a", Title: "A", Status: "would_install", RevisionNumber: i32p(3), Kb: strp("KB1")},
		{Kind: "wua", Identifier: "b", Title: "B", Status: "not_applicable"},
		{Kind: "winget", Identifier: "c", Title: "C", Status: "pending"},
	}
	got := itemsForApproval("validated", items)
	if len(got) != 1 || got[0].Identifier != "a" || got[0].RevisionNumber != 3 || got[0].KB != "KB1" {
		t.Fatalf("validated: %+v", got)
	}
	got = itemsForApproval("pending_approval", items)
	if len(got) != 2 || got[0].Identifier != "a" || got[1].Identifier != "c" {
		t.Fatalf("pending_approval: %+v", got)
	}
}

func TestPackageNameRegexIsNotACharacterRange(t *testing.T) {
	for _, ok := range []string{"libssl3", "g++", "python3.12", "lib_foo-dev", "Foo-1.0"} {
		if !isValidPackageName(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"a,b", "a/b", "a:b", "a;b", "a<b", "a=b", "a>b", "a?b", "a@b", "a[b", "a\\b", "a]b", "a^b", "-a", "_a"} {
		if isValidPackageName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestWindowsScopeResponse(t *testing.T) {
	if windowsScopeResponse(nil, nil) != nil {
		t.Error("non-Windows run got a scope")
	}
	got := windowsScopeResponse(boolp(false), nil)
	if got["updates"] || !got["apps"] {
		t.Errorf("got %v", got)
	}
}

func TestPatchRunListRowsCarryHostOSType(t *testing.T) {
	rows := patchRunsListToResponse([]db.ListPatchRunsRow{{ID: "r", HostOsType: strp("Windows")}})
	hosts := rows[0]["hosts"].(map[string]interface{})
	if got := hosts["os_type"].(*string); got == nil || *got != "Windows" {
		t.Fatalf("hosts.os_type = %v", hosts["os_type"])
	}
}

func TestNotifiesRunEnd(t *testing.T) {
	cases := []struct {
		stage string
		ended bool
		want  bool
	}{
		{"completed", true, true},
		{"failed", true, true},
		{"completed", false, false},
		{"failed", false, false},
		{"cancelled", true, false},
		{"progress", true, false},
	}
	for _, tc := range cases {
		if got := notifiesRunEnd(tc.stage, tc.ended); got != tc.want {
			t.Errorf("notifiesRunEnd(%q, %v) = %v, want %v", tc.stage, tc.ended, got, tc.want)
		}
	}
}

func TestRevalidationItemsResetPatchAll(t *testing.T) {
	plan := &windowsRunPlan{Items: []store.NewPatchRunItem{{Kind: "wua", Identifier: "x"}}}
	if got := revalidationItems("patch_all", plan); got != nil {
		t.Fatalf("patch_all retry must clear the earlier dry run's items, got %+v", got)
	}
	if got := revalidationItems("patch_package", plan); len(got) != 1 {
		t.Fatalf("patch_package retry must re-create its targets, got %+v", got)
	}
}
