package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

const itemGUID = "7f1b0c5e-1111-2222-3333-444455556666"

func TestNormaliseAgentItemsRejects(t *testing.T) {
	ok := AgentItemReport{Kind: "wua", ID: itemGUID, Status: "installed"}
	cases := map[string]AgentItemReport{
		"bad kind":         {Kind: "apt", ID: "curl", Status: "installed"},
		"bad status":       {Kind: "wua", ID: itemGUID, Status: "done"},
		"pending is valid": ok,
		"kb as wua id":     {Kind: "wua", ID: "KB5044284", Status: "installed"},
		"quoted winget id": {Kind: "winget", ID: "x';calc;'", Status: "failed"},
		"flag winget id":   {Kind: "winget", ID: "--force", Status: "failed"},
	}
	for name, it := range cases {
		_, err := normaliseAgentItems([]AgentItemReport{ok, it})
		var ie *ItemReportError
		if name == "pending is valid" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if !errors.As(err, &ie) || ie.Index != 1 {
			t.Errorf("%s: err = %v, want ItemReportError at index 1", name, err)
		}
	}
	tooMany := make([]AgentItemReport, MaxItemsPerReport+1)
	for i := range tooMany {
		tooMany[i] = ok
	}
	if _, err := normaliseAgentItems(tooMany); err == nil {
		t.Error("more than MaxItemsPerReport accepted")
	}
}

func TestNormaliseAgentItemsCleansFields(t *testing.T) {
	start := "2026-10-13T13:00:00+01:00"
	bad := "yesterday"
	neg := int64(-5)
	rows, err := normaliseAgentItems([]AgentItemReport{
		{Kind: "wua", ID: strings.ToUpper(itemGUID), Status: "installed_with_errors", Title: strings.Repeat("t", 600) + "\x00", HResult: "0x800F0922", Message: strings.Repeat("m", 5000), StartedAt: &start, CompletedAt: &bad, DownloadBytes: &neg},
		{Kind: "winget", ID: "Notepad++.Notepad++", Status: "failed", HResult: "'; drop"},
		{Kind: "winget", ID: "Notepad++.Notepad++", Status: "installed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("duplicates not collapsed: %d rows", len(rows))
	}
	r := rows[0]
	if r.Identifier != itemGUID {
		t.Errorf("wua id not lower-cased: %s", r.Identifier)
	}
	if len([]rune(r.Title)) != maxItemTitleRunes || strings.Contains(r.Title, "\x00") {
		t.Errorf("title not truncated/sanitised: %d runes", len([]rune(r.Title)))
	}
	if len([]rune(r.Message)) != maxItemMessageRunes {
		t.Errorf("message not truncated: %d", len([]rune(r.Message)))
	}
	if r.HResult != "0x800F0922" {
		t.Errorf("hresult = %q", r.HResult)
	}
	if r.StartedAt == nil || *r.StartedAt != "2026-10-13T12:00:00Z" {
		t.Errorf("started_at = %v", r.StartedAt)
	}
	if r.CompletedAt != nil {
		t.Errorf("unparseable completed_at kept: %v", *r.CompletedAt)
	}
	if r.DownloadBytes != nil {
		t.Error("negative download_bytes kept")
	}
	if rows[1].HResult != "" || rows[1].Status != "installed" {
		t.Errorf("second row = %+v", rows[1])
	}
}

func TestSplitItemRows(t *testing.T) {
	rows := []agentItemRow{{Kind: "wua", Identifier: "a"}, {Kind: "winget", Identifier: "b"}, {Kind: "winget", Identifier: "c"}}
	existing := map[string]bool{"wua\x00a": true}

	known, fresh := splitItemRows(rows, existing, false, 500)
	if len(known) != 1 || len(fresh) != 0 {
		t.Fatalf("patch_package: known=%d fresh=%d", len(known), len(fresh))
	}
	known, fresh = splitItemRows(rows, existing, true, 1)
	if len(known) != 1 || len(fresh) != 1 || fresh[0].Identifier != "b" {
		t.Fatalf("patch_all capped: known=%d fresh=%+v", len(known), fresh)
	}
	_, fresh = splitItemRows(rows, existing, true, 0)
	if len(fresh) != 0 {
		t.Fatal("insert past the per-run cap")
	}
}

func TestWindowsPackagesAffected(t *testing.T) {
	items := []db.PatchRunItem{
		{Title: "A", Status: "installed"},
		{Title: "B", Status: "installed_with_errors"},
		{Title: "C", Status: "failed"},
		{Title: "D", Status: "would_install"},
		{Title: "E", Status: "not_applicable"},
		{Title: "F", Status: "installed_pending_reboot"},
		{Title: "G", Status: "already_installed"},
	}
	for stage, want := range map[string][]string{
		"completed":         {"A", "B"},
		"failed":            {"A", "B"},
		"cancelled":         {"A", "B"},
		"dry_run_completed": {"D"},
		"progress":          nil,
	} {
		if got := windowsPackagesAffected(stage, items); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", stage, got, want)
		}
	}
}

func TestRunAcceptsItemReports(t *testing.T) {
	for status, want := range map[string]bool{
		"queued": true, "running": true, "pending_validation": true,
		"scheduled": false, "pending_approval": false, "approved": false,
		"completed": false, "failed": false, "validated": false, "dry_run_completed": false,
	} {
		for _, grace := range []bool{true, false} {
			if got := RunAcceptsItemReports(status, grace); got != want {
				t.Errorf("%s (grace %v): got %v, want %v", status, grace, got, want)
			}
		}
	}
	for _, status := range []string{"cancelled", "timed_out", "agent_disconnected"} {
		if !RunAcceptsItemReports(status, true) || RunAcceptsItemReports(status, false) {
			t.Errorf("%s must accept items only within the grace window", status)
		}
	}
}

func TestItemsNeedReboot(t *testing.T) {
	if itemsNeedReboot([]db.PatchRunItem{{Status: "installed"}, {Status: "already_installed"}}) {
		t.Error("no restart signal, but reported as needing one")
	}
	if !itemsNeedReboot([]db.PatchRunItem{{Status: "installed"}, {Status: "installed_pending_reboot"}}) {
		t.Error("installed_pending_reboot not treated as needing a restart")
	}
	if !itemsNeedReboot([]db.PatchRunItem{{Status: "installed", RebootRequired: true}}) {
		t.Error("item reboot_required ignored")
	}
}

func TestNormaliseAgentItemsAcceptsRestartStatuses(t *testing.T) {
	for _, st := range []string{"installed_pending_reboot", "already_installed"} {
		if _, err := normaliseAgentItems([]AgentItemReport{{Kind: "wua", ID: itemGUID, Status: st}}); err != nil {
			t.Errorf("%s rejected: %v", st, err)
		}
	}
}

func TestAgentItemReportsNeverRewriteTitles(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", "patch_run_items.sql"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, q := range []string{"UpdatePatchRunItemsFromAgent", "UpsertPatchRunItemsFromAgent"} {
		body := namedQueryIn(t, src, q)
		if regexp.MustCompile(`(?m)^\s*title\s*=`).MatchString(body) {
			t.Errorf("%s rewrites title on an existing item; approval copies titles into package_names", q)
		}
	}
}
