package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

// ReceiveIntegrationStatus preserves stored install_events only when the agent
// omitted the field, and clears them when the agent sent an empty array. That
// distinction rests entirely on encoding/json leaving an absent key as a nil
// slice and an explicit [] as a non-nil empty slice, which len() alone cannot
// tell apart. If that ever stops holding, the compliance install progress
// silently reverts to being wiped by every status report.
func TestIntegrationStatusReq_DistinguishesAbsentFromEmptyInstallEvents(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantNil bool
	}{
		{
			name:    "reportIntegrationStatus omits the field",
			body:    `{"integration":"compliance","enabled":true,"status":"ready","message":"Compliance tools ready"}`,
			wantNil: true,
		},
		{
			name:    "explicit empty array clears",
			body:    `{"integration":"compliance","status":"ready","install_events":[]}`,
			wantNil: false,
		},
		{
			name:    "explicit null is treated as absent",
			body:    `{"integration":"compliance","status":"ready","install_events":null}`,
			wantNil: true,
		},
		{
			name:    "runInstallScanner sends steps",
			body:    `{"integration":"compliance","status":"installing","install_events":[{"step":"detect_os","status":"done"}]}`,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got integrationStatusReq
			if err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if (got.InstallEvents == nil) != tt.wantNil {
				t.Errorf("InstallEvents nil = %v, want %v", got.InstallEvents == nil, tt.wantNil)
			}
		})
	}
}

// The agent's own payload struct tags install_events omitempty, so the caller
// that does not populate it produces the "absent" shape above rather than an
// empty array. This asserts the wire contract from the sending side.
func TestReportIntegrationStatusShapeOmitsInstallEvents(t *testing.T) {
	// Mirrors models.IntegrationSetupStatus as the agent declares it.
	type agentPayload struct {
		Integration   string   `json:"integration"`
		Status        string   `json:"status"`
		InstallEvents []string `json:"install_events,omitempty"`
	}

	b, err := json.Marshal(agentPayload{Integration: "compliance", Status: "ready"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "install_events") {
		t.Errorf("expected install_events to be omitted, got %s", b)
	}
}
