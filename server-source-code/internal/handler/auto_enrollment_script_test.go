package handler

import (
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agents"
)

func testEnv() enrollmentScriptEnv {
	return enrollmentScriptEnv{
		ServerURL:   "http://patchmon.test:3000",
		TokenKey:    "patchmon_ae_key",
		TokenSecret: "secret",
		CurlFlags:   "-s",
	}
}

// The served script must keep the defaults for every variable the injected
// header does not supply.
//
// The header and the configuration block are complementary, not redundant: an
// injected value wins the ${VAR:-default} substitution, an uninjected one keeps
// its default. Stripping the block dropped the latter.
func TestBuildEnrollmentScript_KeepsUninjectedDefaults(t *testing.T) {
	tests := []struct {
		name       string
		scriptType string
		script     []byte
		want       []string
	}{
		{
			name:       "proxmox-lxc",
			scriptType: "proxmox-lxc",
			script:     agents.ProxmoxAutoEnrollScript,
			want: []string{
				`DRY_RUN="${DRY_RUN:-false}"`,
				`HOST_PREFIX="${HOST_PREFIX:-}"`,
				`SKIP_STOPPED="${SKIP_STOPPED:-true}"`,
				`PARALLEL_INSTALL="${PARALLEL_INSTALL:-false}"`,
				`MAX_PARALLEL="${MAX_PARALLEL:-5}"`,
			},
		},
		{
			name:       "direct-host",
			scriptType: "direct-host",
			script:     agents.DirectHostAutoEnrollScript,
			want:       []string{`FRIENDLY_NAME="${FRIENDLY_NAME:-}"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEnrollmentScript(tt.script, tt.scriptType, testEnv())
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("served script lost a default it must keep: %s", want)
				}
			}
		})
	}
}

// An injected value must beat the script's default, which is what makes it safe
// to leave the configuration block in place.
func TestBuildEnrollmentScript_InjectedValuesPrecedeDefaults(t *testing.T) {
	got := buildEnrollmentScript(agents.DirectHostAutoEnrollScript, "direct-host", testEnv())

	header, body, found := strings.Cut(got, "# ===== CONFIGURATION =====")
	if !found {
		t.Fatal("configuration block missing from served script")
	}
	if !strings.Contains(header, `export PATCHMON_URL="http://patchmon.test:3000"`) {
		t.Error("injected server URL not exported ahead of the configuration block")
	}
	if !strings.Contains(body, `PATCHMON_URL="${PATCHMON_URL:-`) {
		t.Error("configuration block does not use :- substitution, so the export would be overwritten")
	}
}

// Only the generated header may carry an interpreter line; the embedded
// script's own shebang is commented out.
func TestBuildEnrollmentScript_SingleShebang(t *testing.T) {
	for _, tt := range []struct {
		scriptType string
		script     []byte
		want       string
	}{
		{"proxmox-lxc", agents.ProxmoxAutoEnrollScript, "#!/bin/bash"},
		{"direct-host", agents.DirectHostAutoEnrollScript, "#!/bin/sh"},
	} {
		t.Run(tt.scriptType, func(t *testing.T) {
			got := buildEnrollmentScript(tt.script, tt.scriptType, testEnv())

			if !strings.HasPrefix(got, tt.want+"\n") {
				t.Errorf("expected leading %q", tt.want)
			}
			if n := strings.Count(got, "\n#!"); n != 0 {
				t.Errorf("found %d further shebang line(s); the embedded one should be commented out", n)
			}
		})
	}
}

// PATCHMON_URL is the one variable whose default is a real placeholder rather
// than empty, so it is the one where losing the ${VAR:-default} form does
// visible damage: every enrolling host would be pointed at
// https://patchmon.example.com. An empty default cannot fail this way, which is
// why this case is called out separately from the table above.
func TestBuildEnrollmentScript_ServerURLPlaceholderNeverUnconditional(t *testing.T) {
	for _, tt := range []struct {
		scriptType string
		script     []byte
	}{
		{"proxmox-lxc", agents.ProxmoxAutoEnrollScript},
		{"direct-host", agents.DirectHostAutoEnrollScript},
	} {
		t.Run(tt.scriptType, func(t *testing.T) {
			got := buildEnrollmentScript(tt.script, tt.scriptType, testEnv())

			if strings.Contains(got, `PATCHMON_URL="https://patchmon.example.com"`) {
				t.Error("placeholder URL assigned unconditionally; the injected server URL would be overwritten")
			}
			if !strings.Contains(got, `export PATCHMON_URL="http://patchmon.test:3000"`) {
				t.Error("injected server URL missing from the header")
			}
		})
	}
}

func TestBuildEnrollmentScript_ForceInstall(t *testing.T) {
	env := testEnv()
	env.ForceInstall = true
	if got := buildEnrollmentScript(agents.DirectHostAutoEnrollScript, "direct-host", env); !strings.Contains(got, `export FORCE_INSTALL="true"`) {
		t.Error("ForceInstall not reflected in the header")
	}
	if got := buildEnrollmentScript(agents.DirectHostAutoEnrollScript, "direct-host", testEnv()); !strings.Contains(got, `export FORCE_INSTALL="false"`) {
		t.Error("default FORCE_INSTALL should be false")
	}
}
