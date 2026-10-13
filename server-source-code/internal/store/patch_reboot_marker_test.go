package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportWritersHonourPatchRebootMarker(t *testing.T) {
	for file, query := range map[string]string{"hosts.sql": "UpdateHostMetrics", "host_report.sql": "UpdateHostFromReport"} {
		b, err := os.ReadFile(filepath.Join("..", "sqlc", "queries", file))
		if err != nil {
			t.Fatal(err)
		}
		q := namedQueryIn(t, string(b), query)
		for _, want := range []string{
			"needs_reboot = COALESCE(sqlc.narg('needs_reboot')::boolean, needs_reboot) OR (patch_reboot_pending_boot_time IS NOT NULL",
			"THEN 'Restart required to finish a patch run'",
			"patch_reboot_pending_boot_time = CASE WHEN (patch_reboot_pending_boot_time IS NOT NULL",
			"COALESCE(sqlc.narg('boot_time')::timestamptz, boot_time) - patch_reboot_pending_boot_time",
		} {
			if !strings.Contains(strings.Join(strings.Fields(q), " "), strings.Join(strings.Fields(want), " ")) {
				t.Errorf("%s no longer keeps a patch run's restart flag until the boot time changes (missing %q)", query, want)
			}
		}
	}
}
