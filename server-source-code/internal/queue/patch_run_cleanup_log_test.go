package queue

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestPatchRunCleanupLogNamesTheHost(t *testing.T) {
	var buf bytes.Buffer
	h := &PatchRunCleanupHandler{log: slog.New(slog.NewTextHandler(&buf, nil))}
	h.logTimedOut("", "stalled", "run-1", "host-1", "win11", 30)
	line := buf.String()
	for _, want := range []string{"patch_run_id=run-1", "host_id=host-1", "host_name=win11", "reason=stalled"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q lacks %s", line, want)
		}
	}
	if strings.Contains(line, "context=") || strings.Contains(line, `host=""`) {
		t.Errorf("single-context log line carries an empty context: %q", line)
	}
}
