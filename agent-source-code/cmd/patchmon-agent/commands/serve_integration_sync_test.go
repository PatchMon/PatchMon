package commands

import (
	"os"
	"path/filepath"
	"testing"

	"patchmon-agent/internal/config"
	"patchmon-agent/pkg/models"
)

// withTestConfig points the package-level cfgManager at a throwaway config.yml
// holding the given compliance value, and restores the previous manager.
func withTestConfig(t *testing.T, complianceYAML string) *config.Manager {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	contents := `patchmon_server: https://patchmon.example.com
api_version: v1
log_level: info
update_interval: 60
integrations:
  compliance:
    enabled: ` + complianceYAML + `
    openscap_enabled: true
    docker_bench_enabled: false
  docker: false
`
	if err := os.WriteFile(cfgPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	m := config.New()
	m.SetConfigFile(cfgPath)
	// Keep the derived paths inside the temp dir so saving does not try to
	// create /etc/patchmon.
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.LoadConfig(); err != nil {
		t.Fatalf("initial LoadConfig: %v", err)
	}

	previous := cfgManager
	cfgManager = m
	t.Cleanup(func() { cfgManager = previous })
	return m
}

func TestSyncIntegrationStatusCompliance(t *testing.T) {
	tests := []struct {
		name       string
		startMode  string
		response   *models.IntegrationStatusResponse
		wantMode   config.ComplianceMode
		wantDocker bool
	}{
		{
			// The reported bug: the host is on-demand server-side, so the
			// integrations boolean is true. Writing that boolean starts the
			// compliance scheduler and runs full OpenSCAP scans nobody asked
			// for. The mode is what must land in config.yml.
			name:      "enabling an on-demand host does not schedule scans",
			startMode: "false",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": true, "docker": false},
				ComplianceMode: "on-demand",
			},
			wantMode: config.ComplianceOnDemand,
		},
		{
			// The boolean is identical either side of this transition, so it
			// used to be invisible to the sync and the agent stayed on-demand
			// after the operator asked for scheduled scans.
			name:      "on-demand to scheduled propagates",
			startMode: "on-demand",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": true},
				ComplianceMode: "enabled",
			},
			wantMode: config.ComplianceEnabled,
		},
		{
			name:      "scheduled to on-demand propagates",
			startMode: "true",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": true},
				ComplianceMode: "on-demand",
			},
			wantMode: config.ComplianceOnDemand,
		},
		{
			name:      "disabled server-side disables the agent",
			startMode: "on-demand",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": false},
				ComplianceMode: "disabled",
			},
			wantMode: config.ComplianceDisabled,
		},
		{
			// A server predating compliance_mode. The boolean path has to keep
			// working exactly as it did, or upgrading the agent alone breaks
			// compliance for every host.
			name:      "server without compliance_mode falls back to the boolean",
			startMode: "false",
			response: &models.IntegrationStatusResponse{
				Success:      true,
				Integrations: map[string]bool{"compliance": true},
			},
			wantMode: config.ComplianceEnabled,
		},
		{
			// A mode the agent does not understand is not a licence to fall
			// back on the boolean: that fallback writes the scheduled state
			// this whole change exists to stop writing.
			//
			// The fixture has to make the boolean and the local mode disagree
			// to prove it. Starting from on-demand would not: the boolean
			// reads back as true on both sides, so the fallback would write
			// nothing and the assertion would hold for the wrong reason.
			name:      "unknown mode leaves the local configuration alone",
			startMode: "false",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": true},
				ComplianceMode: "sometimes",
			},
			wantMode: config.ComplianceDisabled,
		},
		{
			// The compliance special case must not swallow the other
			// integrations sharing the loop.
			name:      "docker still syncs alongside compliance",
			startMode: "on-demand",
			response: &models.IntegrationStatusResponse{
				Success:        true,
				Integrations:   map[string]bool{"compliance": true, "docker": true},
				ComplianceMode: "on-demand",
			},
			wantMode:   config.ComplianceOnDemand,
			wantDocker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withTestConfig(t, tt.startMode)

			syncIntegrationStatus(tt.response)

			if got := m.GetComplianceMode(); got != tt.wantMode {
				t.Fatalf("compliance mode = %q, want %q", got, tt.wantMode)
			}
			if got := m.IsIntegrationEnabled("docker"); got != tt.wantDocker {
				t.Fatalf("docker enabled = %v, want %v", got, tt.wantDocker)
			}
		})
	}
}

// TestSyncIntegrationStatusKeepsScannerToggles guards the block that follows
// the compliance mode: the scanner toggles are sent independently and must
// still be applied when the mode itself does not move.
func TestSyncIntegrationStatusKeepsScannerToggles(t *testing.T) {
	m := withTestConfig(t, "on-demand")

	syncIntegrationStatus(&models.IntegrationStatusResponse{
		Success:                      true,
		Integrations:                 map[string]bool{"compliance": true},
		ComplianceMode:               "on-demand",
		ComplianceOpenscapEnabled:    boolPtr(false),
		ComplianceDockerBenchEnabled: boolPtr(true),
	})

	if m.GetComplianceOpenscapEnabled() {
		t.Fatal("OpenSCAP toggle was not applied")
	}
	if !m.GetComplianceDockerBenchEnabled() {
		t.Fatal("docker-bench toggle was not applied")
	}
	if got := m.GetComplianceMode(); got != config.ComplianceOnDemand {
		t.Fatalf("compliance mode = %q, want %q", got, config.ComplianceOnDemand)
	}
}
