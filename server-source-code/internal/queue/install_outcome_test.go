package queue

import "testing"

func TestInstallOutcome(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantFinished bool
		wantFailure  string
	}{
		{name: "stale ready status before the agent starts", raw: `{"status":"ready","install_events":[]}`},
		{name: "stale partial status with no events", raw: `{"status":"partial"}`},
		{name: "in progress", raw: `{"status":"installing","install_events":[{"step":"detect_os","status":"done"}]}`},
		{name: "done", raw: `{"status":"ready","install_events":[{"step":"detect_os","status":"done"},{"step":"complete","status":"done"}]}`, wantFinished: true},
		{
			name:         "failed step reason wins",
			raw:          `{"status":"error","install_events":[{"step":"install_openscap","status":"failed","message":"apt-get failed"},{"step":"complete","status":"failed","message":"Installation failed"}]}`,
			wantFinished: true,
			wantFailure:  "apt-get failed",
		},
		{name: "docker bench failure still completes", raw: `{"install_events":[{"step":"docker_bench","status":"failed","message":"pull failed"},{"step":"complete","status":"done"}]}`, wantFinished: true},
		{name: "no content uses complete message", raw: `{"install_events":[{"step":"sync_ssg","status":"done"},{"step":"complete","status":"failed","message":"no SCAP content found"}]}`, wantFinished: true, wantFailure: "no SCAP content found"},
		{name: "failed without messages", raw: `{"install_events":[{"step":"complete","status":"failed"}]}`, wantFinished: true, wantFailure: "Agent reported error"},
		{name: "malformed", raw: `not json`},
		{name: "another job's events are ignored", raw: `{"install_job_id":"job-0","install_events":[{"step":"complete","status":"done"}]}`},
		{name: "this job's events count", raw: `{"install_job_id":"job-1","install_events":[{"step":"complete","status":"done"}]}`, wantFinished: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finished, failure := InstallOutcome(tt.raw, "job-1")
			if finished != tt.wantFinished || failure != tt.wantFailure {
				t.Fatalf("got (%v, %q), want (%v, %q)", finished, failure, tt.wantFinished, tt.wantFailure)
			}
		})
	}
}
