package store

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The host update queries write optional columns supplied by the agent. Under
// hash-gated check-in a partial report carries only the sections that changed,
// so every optional column MUST be guarded: an unguarded assignment receives
// SQL NULL and wipes a value the previous report wrote correctly.
//
// gateway_ip was the one bare assignment among twenty guarded columns, so it
// was blanked by every partial report; reboot_reason had the same shape on the
// ping path while its own header comment claimed otherwise. These tests read
// the sqlc source of truth and fail if a guard is dropped again.

func readQueryFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// extractStatement returns the body of the named sqlc query, from its
// "-- name: <name>" marker up to the next marker or EOF.
func extractStatement(t *testing.T, sql, name string) string {
	t.Helper()
	marker := "-- name: " + name + " "
	start := strings.Index(sql, marker)
	if start < 0 {
		t.Fatalf("query %q not found", name)
	}
	rest := sql[start+len(marker):]
	if next := strings.Index(rest, "\n-- name: "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}

// assignedColumns returns the column names assigned in a SET clause, ignoring
// commented-out lines and continuation lines of multi-line expressions.
var assignRe = regexp.MustCompile(`(?m)^\s{4}([a-z_]+)\s*=\s*(.*)$`)

func TestUpdateHostFromReport_OptionalColumnsAreGuarded(t *testing.T) {
	t.Parallel()

	stmt := extractStatement(t, readQueryFile(t, "../sqlc/queries/host_report.sql"), "UpdateHostFromReport")

	for _, m := range assignRe.FindAllStringSubmatch(stmt, -1) {
		col, expr := m[1], strings.TrimSpace(m[2])

		// Columns the query deliberately sets unconditionally.
		switch col {
		case "last_update", "updated_at", "status", "awaiting_post_patch_report_run_id":
			continue
		}
		// Multi-line CASE expressions open here and are checked separately.
		if strings.HasPrefix(expr, "CASE") {
			continue
		}
		if !strings.Contains(expr, "COALESCE") {
			t.Errorf("column %q is assigned without a COALESCE guard: %s\n"+
				"An unguarded optional column is NULLed by every hash-gated partial report.", col, expr)
		}
	}

	// The specific regression: gateway_ip must be guarded.
	if !strings.Contains(stmt, "gateway_ip = COALESCE(") {
		t.Error("gateway_ip must be COALESCE-guarded; a bare assignment is wiped by every partial report")
	}
}

func TestUpdateHostMetrics_RebootReasonIsPairedWithNeedsReboot(t *testing.T) {
	t.Parallel()

	stmt := extractStatement(t, readQueryFile(t, "../sqlc/queries/hosts.sql"), "UpdateHostMetrics")

	if strings.Contains(stmt, "reboot_reason  = sqlc.narg('reboot_reason'),") ||
		strings.Contains(stmt, "reboot_reason = sqlc.narg('reboot_reason'),") {
		t.Fatal("reboot_reason is assigned unguarded; a ping without a reason blanks the stored one")
	}

	// It must be tied to needs_reboot rather than plain-COALESCE'd, so that a
	// genuine "no longer needs reboot" ping can still clear it.
	if !strings.Contains(stmt, "reboot_reason") || !strings.Contains(stmt, "CASE") {
		t.Fatal("reboot_reason must be written via a CASE keyed on needs_reboot")
	}
	if !strings.Contains(stmt, "WHEN sqlc.narg('needs_reboot')::boolean IS NULL THEN reboot_reason") {
		t.Error("the reboot_reason CASE must preserve the existing value when needs_reboot is absent")
	}
	// The arm that actually covers the reported failure. install.go sets
	// needs_reboot and reboot_reason independently, so a ping can assert the
	// flag with no reason; without this arm that fell through to the ELSE and
	// blanked a stored reason, which is the bug the CASE exists to prevent.
	if !strings.Contains(stmt, "IS TRUE") || !strings.Contains(stmt, "IS NULL THEN reboot_reason") {
		t.Error("the reboot_reason CASE must also preserve the stored value when needs_reboot " +
			"is asserted WITHOUT a reason, not only when needs_reboot is absent")
	}

	// Every other optional metric stays COALESCE-guarded.
	for _, col := range []string{
		"cpu_cores", "cpu_model", "ram_installed", "swap_size", "disk_details",
		"system_uptime", "boot_time", "load_average", "needs_reboot", "agent_version",
	} {
		if !strings.Contains(stmt, col+" ") {
			t.Errorf("expected column %q in UpdateHostMetrics", col)
			continue
		}
		if !regexp.MustCompile(col + `\s*= COALESCE\(`).MatchString(stmt) {
			t.Errorf("column %q must stay COALESCE-guarded on the ping path", col)
		}
	}
}
