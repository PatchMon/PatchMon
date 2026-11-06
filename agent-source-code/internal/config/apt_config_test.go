package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAptConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent block defaults to retries disabled", "", true},
		{"retries kept", "apt:\n  disable_retries: false\n", false},
		{"retries disabled", "apt:\n  disable_retries: true\n", true},
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
			if got := m.GetAptDisableRetries(); got != tt.want {
				t.Errorf("GetAptDisableRetries() = %v, want %v", got, tt.want)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), "apt:") {
				t.Errorf("saved config lacks apt:\n%s", saved)
			}
			reloaded := New()
			reloaded.SetConfigFile(path)
			if err := reloaded.LoadConfig(); err != nil {
				t.Fatal(err)
			}
			if reloaded.GetAptDisableRetries() != tt.want {
				t.Error("round trip changed the setting")
			}
		})
	}
}

func newAptTestManager(t *testing.T, extra string) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	body := "patchmon_server: https://pm.example.com\n" +
		"credentials_file: " + filepath.Join(dir, "credentials.yml") + "\n" +
		"log_file: " + filepath.Join(dir, "logs", "agent.log") + "\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.SetConfigFile(path)
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	return m, path
}

func TestSetAptDisableRetries(t *testing.T) {
	for _, want := range []bool{false, true} {
		m, path := newAptTestManager(t, "")
		if err := m.SetAptDisableRetries(want); err != nil {
			t.Fatal(err)
		}
		if got := m.GetAptDisableRetries(); got != want {
			t.Errorf("GetAptDisableRetries() = %v, want %v", got, want)
		}
		reloaded := New()
		reloaded.SetConfigFile(path)
		if err := reloaded.LoadConfig(); err != nil {
			t.Fatal(err)
		}
		if reloaded.GetAptDisableRetries() != want {
			t.Errorf("reloaded GetAptDisableRetries() != %v", want)
		}
	}
}
