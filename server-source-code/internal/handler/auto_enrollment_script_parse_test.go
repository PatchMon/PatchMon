package handler

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agents"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestDirectScriptParsesHostIDFromEnrolResponse(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	script := string(agents.DirectHostAutoEnrollScript)
	start := strings.Index(script, "enrolled_host_id() {")
	end := strings.Index(script[start:], "\n}\n")
	if start < 0 || end < 0 {
		t.Fatal("enrolled_host_id not found in direct_host_auto_enroll.sh")
	}
	fn := script[start : start+end+3]

	host := &models.Host{ID: "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41", FriendlyName: `web "01"`, ApiID: "patchmon_ab", Status: "pending"}
	for name, group := range map[string]*store.HostGroupBrief{
		"no host group":   nil,
		"with host group": {ID: "group-9999", Name: `x"id":"evil`, Color: "#fff"},
	} {
		for _, reenrolled := range []bool{false, true} {
			body, err := json.Marshal(enrolResponse("m", host, "key", group, reenrolled))
			if err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(sh, "-c", fn+`enrolled_host_id "$1"`, "_", string(body)).Output()
			if err != nil {
				t.Fatal(err)
			}
			if got := string(out); got != host.ID+"\n" {
				t.Errorf("%s reenrolled=%v: parsed %q from %s", name, reenrolled, got, body)
			}
		}
	}
}
