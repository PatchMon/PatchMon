package config

import (
	"os"
	"strings"
	"testing"
)

func TestRemoteActionsConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent block defaults to enabled", "", true},
		{"explicitly enabled", "remote_actions:\n  enabled: true\n", true},
		{"disabled", "remote_actions:\n  enabled: false\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, path := newAptTestManager(t, tt.yaml)
			if got := m.GetRemoteActionsEnabled(); got != tt.want {
				t.Errorf("GetRemoteActionsEnabled() = %v, want %v", got, tt.want)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), "remote_actions:") {
				t.Errorf("saved config lacks remote_actions:\n%s", saved)
			}
			reloaded := New()
			reloaded.SetConfigFile(path)
			if err := reloaded.LoadConfig(); err != nil {
				t.Fatal(err)
			}
			if reloaded.GetRemoteActionsEnabled() != tt.want {
				t.Error("round trip changed the setting")
			}
		})
	}
}

// A save triggered by a server-pushed setting must keep remote actions disabled.
func TestRemoteActionsSurviveServerPushedSettings(t *testing.T) {
	m, path := newAptTestManager(t, "remote_actions:\n  enabled: false\n")
	if err := m.SetUpdateInterval(15); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPackageCacheRefresh("never", 60); err != nil {
		t.Fatal(err)
	}
	if err := m.SetIntegrationEnabled("docker", true); err != nil {
		t.Fatal(err)
	}
	reloaded := New()
	reloaded.SetConfigFile(path)
	if err := reloaded.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if reloaded.GetRemoteActionsEnabled() {
		t.Error("a server-pushed setting turned remote actions back on")
	}
}
