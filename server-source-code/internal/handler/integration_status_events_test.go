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

// GetInstallJobStatus reports the asynq state of the WebSocket delivery, which
// completes as soon as the agent is told to start, so a scanner install that
// failed on the host surfaced as "completed" or "unknown".
//
// The outcome is taken from install_events and NOT from the stored `status`
// field. That field is shared with the agent's periodic availability report,
// which writes "error" on any host that simply has no scanner yet, from two
// seconds after agent startup. Deriving from it reported every fresh install as
// already failed before the agent had done anything.
func TestInstallJobStatusFromEvents(t *testing.T) {
	event := func(step, status, message string) interface{} {
		return map[string]interface{}{"step": step, "status": status, "message": message}
	}

	tests := []struct {
		name        string
		events      []interface{}
		wantStatus  string
		wantMessage string
		wantOK      bool
	}{
		{
			// The specific reason comes from the step that failed, not from the
			// terminal step's generic label.
			name:        "failed install prefers the failing step's reason",
			events:      []interface{}{event("detect_os", "done", ""), event("install_openscap", "failed", "no package"), event("complete", "failed", "Installation failed")},
			wantStatus:  "failed",
			wantMessage: "no package",
			wantOK:      true,
		},
		{
			name:        "failure with only a terminal message",
			events:      []interface{}{event("detect_os", "done", ""), event("complete", "failed", "timed out after 5 minutes")},
			wantStatus:  "failed",
			wantMessage: "timed out after 5 minutes",
			wantOK:      true,
		},
		{
			// A failed step with no terminal step is not a verdict, so the
			// earlier-failed-step preference must not leak into an in-flight run.
			name:   "failed step without a terminal step",
			events: []interface{}{event("install_openscap", "failed", "no package")},
			wantOK: false,
		},
		{
			name:        "successful install",
			events:      []interface{}{event("detect_os", "done", ""), event("complete", "done", "Installation complete - scanner is ready")},
			wantStatus:  "completed",
			wantMessage: "Installation complete - scanner is ready",
			wantOK:      true,
		},
		{
			// No terminal step yet: the queue state must be left alone rather
			// than reported as permanently active.
			name:   "in flight",
			events: []interface{}{event("detect_os", "done", ""), event("install_openscap", "in_progress", "Installing...")},
			wantOK: false,
		},
		{
			// What InstallScanner leaves behind when it clears the previous
			// run's events, and what a host that has never installed holds.
			name:   "no events",
			events: nil,
			wantOK: false,
		},
		{
			name:   "malformed entries are skipped",
			events: []interface{}{"not-a-map", map[string]interface{}{"step": 42}},
			wantOK: false,
		},
		{
			// A failed step that is not the terminal one is not an outcome:
			// SSG content can fail while the install still completes.
			name:   "non-terminal failure is not the verdict",
			events: []interface{}{event("install_ssg", "failed", "content unavailable")},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message, ok := installJobStatusFromEvents(tt.events)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("= (%q, %q), want (%q, %q)", status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
