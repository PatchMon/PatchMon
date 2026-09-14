package queue

import (
	"encoding/json"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/mailer"
)

// The stored config is JSON, so an absent "use_tls" key must stay absent all
// the way through to ResolveMode. Decoding it into a plain bool turns "not
// configured" into an explicit false, which ResolveMode reads as "the operator
// chose no encryption" and returns TLSModeNone, skipping the port heuristic.
//
// The test-email handler decodes into *bool and therefore got the heuristic,
// while both send workers decoded into bool and did not. A destination could
// pass "send test email" over STARTTLS and then deliver real alerts in the
// clear.
func TestEmailConfig_AbsentUseTLSStaysNil(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want mailer.TLSMode
		port int
	}{
		{
			name: "absent key on 587 falls through to the port heuristic",
			raw:  `{"smtp_host":"mail.example.com","smtp_port":587,"from":"a@b.c","to":"d@e.f"}`,
			port: 587,
			want: mailer.TLSModeStartTLS,
		},
		{
			name: "absent key on 465 falls through to the port heuristic",
			raw:  `{"smtp_host":"mail.example.com","smtp_port":465,"from":"a@b.c","to":"d@e.f"}`,
			port: 465,
			want: mailer.TLSModeTLS,
		},
		{
			name: "explicit false is still honoured as a deliberate choice",
			raw:  `{"smtp_host":"mail.example.com","smtp_port":587,"from":"a@b.c","to":"d@e.f","use_tls":false}`,
			port: 587,
			want: mailer.TLSModeNone,
		},
		{
			name: "explicit true maps to auto",
			raw:  `{"smtp_host":"mail.example.com","smtp_port":25,"from":"a@b.c","to":"d@e.f","use_tls":true}`,
			port: 25,
			want: mailer.TLSModeAuto,
		},
		{
			name: "an explicit mode wins over everything",
			raw:  `{"smtp_host":"mail.example.com","smtp_port":587,"from":"a@b.c","to":"d@e.f","use_tls":false,"tls_mode":"tls"}`,
			port: 587,
			want: mailer.TLSModeTLS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var alert emailConfig
			if err := json.Unmarshal([]byte(tt.raw), &alert); err != nil {
				t.Fatalf("emailConfig: %v", err)
			}
			if got := mailer.ResolveMode(alert.TLSMode, alert.UseTLS, tt.port); got != tt.want {
				t.Errorf("emailConfig resolved %q, want %q", got, tt.want)
			}

			var report scheduledEmailConfig
			if err := json.Unmarshal([]byte(tt.raw), &report); err != nil {
				t.Fatalf("scheduledEmailConfig: %v", err)
			}
			if got := mailer.ResolveMode(report.TLSMode, report.UseTLS, tt.port); got != tt.want {
				t.Errorf("scheduledEmailConfig resolved %q, want %q", got, tt.want)
			}
		})
	}
}

// All three call sites must agree. The two workers previously disagreed with
// the test-email path for exactly the configs an operator is most likely to
// have: saved through the current UI, which writes tls_mode and no use_tls.
func TestEmailConfig_SendPathsAgreeWithTestPath(t *testing.T) {
	const savedByCurrentUI = `{"smtp_host":"mail.example.com","smtp_port":587,"from":"a@b.c","to":"d@e.f"}`

	// The handler's shape: *bool, absent stays nil.
	var handlerSide struct {
		UseTLS  *bool  `json:"use_tls"`
		TLSMode string `json:"tls_mode"`
	}
	if err := json.Unmarshal([]byte(savedByCurrentUI), &handlerSide); err != nil {
		t.Fatal(err)
	}
	want := mailer.ResolveMode(handlerSide.TLSMode, handlerSide.UseTLS, 587)

	var alert emailConfig
	if err := json.Unmarshal([]byte(savedByCurrentUI), &alert); err != nil {
		t.Fatal(err)
	}
	var report scheduledEmailConfig
	if err := json.Unmarshal([]byte(savedByCurrentUI), &report); err != nil {
		t.Fatal(err)
	}

	if got := mailer.ResolveMode(alert.TLSMode, alert.UseTLS, 587); got != want {
		t.Errorf("alert worker resolved %q but the test-email path resolves %q", got, want)
	}
	if got := mailer.ResolveMode(report.TLSMode, report.UseTLS, 587); got != want {
		t.Errorf("scheduled report worker resolved %q but the test-email path resolves %q", got, want)
	}
}

// allow_insecure_auth is the fourth key to live in this blob, and it is decoded
// by the same three structs. A tag typo in any one of them compiles, passes
// every other test, and fails silently in production: the operator ticks the
// box, "send test email" succeeds via the handler path, and scheduled reports
// still refuse to authenticate. That is the same class of bug this file exists
// to catch, so pin all three against one raw string.
func TestEmailConfig_AllowInsecureAuthAgreesAcrossPaths(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "absent key means no consent",
			raw:  `{"smtp_host":"relay.internal","smtp_port":25,"from":"a@b.c","to":"d@e.f","tls_mode":"none"}`,
			want: false,
		},
		{
			name: "explicit false means no consent",
			raw:  `{"smtp_host":"relay.internal","smtp_port":25,"from":"a@b.c","to":"d@e.f","tls_mode":"none","allow_insecure_auth":false}`,
			want: false,
		},
		{
			name: "explicit true is carried on every path",
			raw:  `{"smtp_host":"relay.internal","smtp_port":25,"from":"a@b.c","to":"d@e.f","tls_mode":"none","username":"u","password":"p","allow_insecure_auth":true}`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mirrors testSMTPRequest in internal/handler/notifications.go, which
			// lives in another package and cannot be referenced from here.
			var handlerSide struct {
				TLSMode           string `json:"tls_mode"`
				AllowInsecureAuth bool   `json:"allow_insecure_auth"`
			}
			if err := json.Unmarshal([]byte(tt.raw), &handlerSide); err != nil {
				t.Fatal(err)
			}
			var alert emailConfig
			if err := json.Unmarshal([]byte(tt.raw), &alert); err != nil {
				t.Fatal(err)
			}
			var report scheduledEmailConfig
			if err := json.Unmarshal([]byte(tt.raw), &report); err != nil {
				t.Fatal(err)
			}

			if handlerSide.AllowInsecureAuth != tt.want {
				t.Errorf("test-email path decoded %v, want %v", handlerSide.AllowInsecureAuth, tt.want)
			}
			if alert.AllowInsecureAuth != tt.want {
				t.Errorf("alert worker decoded %v, want %v", alert.AllowInsecureAuth, tt.want)
			}
			if report.AllowInsecureAuth != tt.want {
				t.Errorf("scheduled report worker decoded %v, want %v", report.AllowInsecureAuth, tt.want)
			}
		})
	}
}
