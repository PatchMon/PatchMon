package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerUpdateCheckConfig(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantEnabled  bool
		wantInterval time.Duration
	}{
		{"absent block uses defaults", "", true, 6 * time.Hour},
		{"disabled", "docker_update_check:\n  enabled: false\n", false, 6 * time.Hour},
		{"custom interval", "docker_update_check:\n  interval_hours: 12\n", true, 12 * time.Hour},
		{"interval below minimum", "docker_update_check:\n  enabled: true\n  interval_hours: 0\n", true, 6 * time.Hour},
		{"negative interval", "docker_update_check:\n  interval_hours: -3\n", true, 6 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yml")
			body := "patchmon_server: https://pm.example.com\n" +
				"credentials_file: " + filepath.Join(dir, "credentials.yml") + "\n" +
				"log_file: " + filepath.Join(dir, "logs", "agent.log") + "\n" + tt.yaml
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			m := New()
			m.SetConfigFile(path)
			if err := m.LoadConfig(); err != nil {
				t.Fatal(err)
			}
			enabled, interval := m.GetDockerUpdateCheck()
			if enabled != tt.wantEnabled || interval != tt.wantInterval {
				t.Errorf("got enabled=%v interval=%v", enabled, interval)
			}

			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), "docker_update_check:") {
				t.Errorf("saved config lacks docker_update_check:\n%s", saved)
			}
			reloaded := New()
			reloaded.SetConfigFile(path)
			if err := reloaded.LoadConfig(); err != nil {
				t.Fatal(err)
			}
			if e, i := reloaded.GetDockerUpdateCheck(); e != enabled || i != interval {
				t.Errorf("round trip changed settings: %v %v", e, i)
			}
		})
	}
}
