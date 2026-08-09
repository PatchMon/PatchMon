package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// brokenWindowsConfig is what the Windows installer used to write: YAML double
// quotes around Windows paths. A double-quoted scalar processes backslash
// escapes, and "\c" is not one of them, so the document does not parse.
//
// "\P" is a valid escape (U+2029), which is why this looks so arbitrary: the
// same quoting corrupts silently on some paths and fails loudly on others.
const brokenWindowsConfig = `patchmon_server: "https://patchmon.example.net"
api_version: "v1"
credentials_file: "C:\ProgramData\PatchMon\credentials.yml"
log_file: "C:\ProgramData\PatchMon\patchmon-agent.log"
log_level: "info"
skip_ssl_verify: false
`

func newUnreadableManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(brokenWindowsConfig), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	m := New()
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")

	if err := m.LoadConfig(); err == nil {
		t.Fatal("LoadConfig accepted a config file with invalid YAML escapes")
	}
	return m, cfgPath
}

func TestLoadConfig_RecordsFailureAndKeepsDefaults(t *testing.T) {
	t.Parallel()
	m, _ := newUnreadableManager(t)

	if m.LoadError() == nil {
		t.Fatal("LoadError is nil after a failed load")
	}
	if got := m.GetConfig().PatchmonServer; got != "" {
		t.Fatalf("PatchmonServer = %q, want the empty default: the file did not parse, so nothing from it should be in memory", got)
	}
}

// The regression this whole change exists for: the agent fell back to defaults,
// then wrote them back over the operator's file, blanking patchmon_server.
func TestSave_RefusesAfterFailedLoad(t *testing.T) {
	t.Parallel()

	savers := map[string]func(m *Manager) error{
		"SetReportOffset":        func(m *Manager) error { return m.SetReportOffset(42) },
		"SetUpdateInterval":      func(m *Manager) error { return m.SetUpdateInterval(120) },
		"SetPackageCacheRefresh": func(m *Manager) error { return m.SetPackageCacheRefresh("always", 60) },
		"SetIntegrationEnabled":  func(m *Manager) error { return m.SetIntegrationEnabled("docker", true) },
		"SaveConfig":             func(m *Manager) error { return m.SaveConfig() },
	}

	for name, save := range savers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, cfgPath := newUnreadableManager(t)

			err := save(m)
			if err == nil {
				t.Fatal("save succeeded against a config file that could not be read")
			}
			if !errors.Is(err, ErrConfigUnreadable) {
				t.Fatalf("error = %v, want one wrapping ErrConfigUnreadable", err)
			}

			after, readErr := os.ReadFile(cfgPath)
			if readErr != nil {
				t.Fatalf("reading config back: %v", readErr)
			}
			if string(after) != brokenWindowsConfig {
				t.Fatalf("config file was rewritten:\n%s", after)
			}
		})
	}
}

// config set-api is the documented recovery, so it is the one saver allowed to
// replace a file the agent cannot parse.
func TestSetPatchmonServer_RepairsUnreadableConfig(t *testing.T) {
	t.Parallel()
	m, cfgPath := newUnreadableManager(t)

	const serverURL = "https://patchmon.example.net"
	if err := m.SetPatchmonServer(serverURL); err != nil {
		t.Fatalf("SetPatchmonServer on an unreadable config: %v", err)
	}
	if m.LoadError() != nil {
		t.Fatalf("LoadError still set after a repair: %v", m.LoadError())
	}

	fresh := New()
	fresh.SetConfigFile(cfgPath)
	if err := fresh.LoadConfig(); err != nil {
		t.Fatalf("repaired config still does not parse: %v", err)
	}
	if got := fresh.GetConfig().PatchmonServer; got != serverURL {
		t.Fatalf("PatchmonServer = %q, want %q", got, serverURL)
	}
	if fresh.LoadError() != nil {
		t.Fatalf("LoadError set after a successful load: %v", fresh.LoadError())
	}
}

func TestLoadConfig_ClearsFailureOnceTheFileParses(t *testing.T) {
	t.Parallel()
	m, cfgPath := newUnreadableManager(t)

	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0o600); err != nil {
		t.Fatalf("rewriting test config: %v", err)
	}
	if err := m.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig on a valid file: %v", err)
	}
	if m.LoadError() != nil {
		t.Fatalf("LoadError = %v, want nil after a successful load", m.LoadError())
	}
	if err := m.SetReportOffset(42); err != nil {
		t.Fatalf("saving after recovery: %v", err)
	}
}

// A missing file is not a failure: the agent is meant to start on defaults so
// that config set-api can create one.
func TestLoadConfig_MissingFileIsNotAFailure(t *testing.T) {
	t.Parallel()
	m := New()
	m.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	if err := m.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig with no file present: %v", err)
	}
	if m.LoadError() != nil {
		t.Fatalf("LoadError = %v, want nil when the file simply does not exist", m.LoadError())
	}
}
