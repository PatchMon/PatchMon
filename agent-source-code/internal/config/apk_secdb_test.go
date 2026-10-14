package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPKSecDBConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent block defaults to enabled", "", true},
		{"disabled", "apk_secdb:\n  enabled: false\n", false},
		{"enabled", "apk_secdb:\n  enabled: true\n", true},
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
			if got := m.GetAPKSecDBEnabled(); got != tt.want {
				t.Errorf("GetAPKSecDBEnabled() = %v, want %v", got, tt.want)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), "apk_secdb:") {
				t.Errorf("saved config lacks apk_secdb:\n%s", saved)
			}
			reloaded := New()
			reloaded.SetConfigFile(path)
			if err := reloaded.LoadConfig(); err != nil {
				t.Fatal(err)
			}
			if reloaded.GetAPKSecDBEnabled() != tt.want {
				t.Error("round trip changed the setting")
			}
		})
	}
}
