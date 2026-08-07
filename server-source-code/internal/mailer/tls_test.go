package mailer

import (
	"errors"
	"testing"
)

func TestResolveMode(t *testing.T) {
	t.Parallel()

	truthy := true
	falsy := false

	cases := []struct {
		name      string
		stored    string
		legacyUse *bool
		port      int
		expected  TLSMode
	}{
		{"stored none wins", "none", &truthy, 587, TLSModeNone},
		{"stored starttls wins", "starttls", &falsy, 465, TLSModeStartTLS},
		{"stored tls wins", "tls", &falsy, 587, TLSModeTLS},
		{"stored auto wins", "auto", &falsy, 25, TLSModeAuto},
		{"stored is uppercased and trimmed", "  STARTTLS  ", nil, 25, TLSModeStartTLS},

		{"legacy true with empty stored maps to auto", "", &truthy, 25, TLSModeAuto},
		{"legacy false with empty stored maps to none", "", &falsy, 25, TLSModeNone},
		{"legacy true beats port heuristic", "", &truthy, 587, TLSModeAuto},

		{"empty + nil legacy + port 587 → starttls", "", nil, 587, TLSModeStartTLS},
		{"empty + nil legacy + port 465 → tls", "", nil, 465, TLSModeTLS},
		{"empty + nil legacy + port 25 → auto", "", nil, 25, TLSModeAuto},
		{"empty + nil legacy + port 0 → auto", "", nil, 0, TLSModeAuto},

		{"garbage stored falls through to legacy", "yes", &truthy, 25, TLSModeAuto},
		{"garbage stored falls through to port heuristic", "wat", nil, 587, TLSModeStartTLS},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveMode(tc.stored, tc.legacyUse, tc.port)
			if got != tc.expected {
				t.Fatalf("ResolveMode(%q, %v, %d) = %q, want %q",
					tc.stored, tc.legacyUse, tc.port, got, tc.expected)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	good := Config{
		Host:    "smtp.example.com",
		Port:    587,
		From:    "alerts@example.com",
		TLSMode: TLSModeStartTLS,
	}
	goodMsg := Message{To: "ops@example.com", Subject: "x", HTMLBody: "<p>x</p>"}

	cases := []struct {
		name      string
		mutate    func(*Config, *Message)
		wantStage Stage
		wantErr   bool
	}{
		{
			name:    "happy path",
			mutate:  func(_ *Config, _ *Message) {},
			wantErr: false,
		},
		{
			name:      "empty host",
			mutate:    func(c *Config, _ *Message) { c.Host = "  " },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "port zero",
			mutate:    func(c *Config, _ *Message) { c.Port = 0 },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "port too high",
			mutate:    func(c *Config, _ *Message) { c.Port = 70000 },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "empty from",
			mutate:    func(c *Config, _ *Message) { c.From = "" },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "malformed from",
			mutate:    func(c *Config, _ *Message) { c.From = "not-an-address" },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "empty to",
			mutate:    func(_ *Config, m *Message) { m.To = "" },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name:      "malformed to",
			mutate:    func(_ *Config, m *Message) { m.To = "ops@" },
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name: "tls=none with username rejected",
			mutate: func(c *Config, _ *Message) {
				c.TLSMode = TLSModeNone
				c.Username = "user"
			},
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name: "tls=none with password rejected",
			mutate: func(c *Config, _ *Message) {
				c.TLSMode = TLSModeNone
				c.Password = "secret"
			},
			wantStage: StageValidate,
			wantErr:   true,
		},
		{
			name: "tls=none without creds is fine",
			mutate: func(c *Config, _ *Message) {
				c.TLSMode = TLSModeNone
			},
			wantErr: false,
		},
		{
			name: "tls=starttls with creds is fine",
			mutate: func(c *Config, _ *Message) {
				c.Username = "user"
				c.Password = "pw"
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := good
			msg := goodMsg
			tc.mutate(&cfg, &msg)
			err := validate(cfg, msg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Stage != tc.wantStage {
					t.Fatalf("stage = %q, want %q (err=%v)", err.Stage, tc.wantStage, err)
				}
				var se *SendError
				if !errors.As(err, &se) {
					t.Fatalf("validate returned non-SendError: %T", err)
				}
				if se.Unwrap() == nil {
					t.Fatal("SendError.Err is nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSendErrorFormat(t *testing.T) {
	t.Parallel()

	se := newSendError(StageStartTLS, errors.New("boom"))
	if got := se.Error(); got != "smtp starttls: boom" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(se, se.Err) {
		t.Fatal("errors.Is should match wrapped err via Unwrap")
	}
}
