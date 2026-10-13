//go:build windows

package packages

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"patchmon-agent/internal/winexec"
)

func TestWUAScanLive(t *testing.T) {
	requireWindowsIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var log strings.Builder
	scan, err := newWUAScriptBackend().Scan(ctx, func(s string) { log.WriteString(s) })
	assertNoPowerShellParseError(t, log.String())
	if err != nil {
		t.Fatalf("wua.ps1 scan failed: %v\n%s", err, log.String())
	}
	for _, u := range scan.Updates {
		if !wuaGUIDPattern.MatchString(u.ID) || u.Title == "" {
			t.Errorf("malformed update: %+v", u)
		}
	}
	t.Logf("scan: %d updates, reboot required before install: %v", len(scan.Updates), scan.RebootRequiredBeforeInstall)
}

func TestWUAInstallRejectsBadIDLive(t *testing.T) {
	requireWindowsIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, err := newWUAScriptBackend().Install(ctx, "x' OR 1=1", func(string) {})
	if err == nil || !strings.Contains(err.Error(), "invalid update id") {
		t.Fatalf("wua.ps1 must refuse a non-GUID id, got %v", err)
	}
}

func hotfixIDs(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command",
		winexec.Script("(Get-HotFix | Sort-Object HotFixID | ForEach-Object { $_.HotFixID }) -join ','")).Output()
	if err != nil {
		t.Fatalf("Get-HotFix: %v", err)
	}
	return strings.TrimSpace(string(winexec.TrimBOM(out)))
}

func TestWindowsDryRunChangesNothingLive(t *testing.T) {
	requireWindowsIntegration(t)

	before := hotfixIDs(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var out strings.Builder
	res := RunWindowsPatch(ctx, ctx, WindowsPatchRequest{PatchAll: true, DryRun: true, IncludeUpdates: true, IncludeApps: true},
		WindowsPatchHooks{Output: func(s string) { out.WriteString(s) }}, newTestWindowsManager().UpdatableWingetApps)
	if res.Stage != "dry_run_completed" {
		t.Errorf("stage = %q (%s)\n%s", res.Stage, res.ErrorMessage, out.String())
	}
	for _, it := range res.Items {
		if it.Status == itemInstalled || it.Status == itemInstalledWithErrors {
			t.Errorf("dry run installed %+v", it)
		}
	}
	if after := hotfixIDs(t); after != before {
		t.Errorf("Get-HotFix changed during a dry run:\nbefore %s\nafter  %s", before, after)
	}
	t.Logf("dry run items: %d\n%s", len(res.Items), out.String())
}
