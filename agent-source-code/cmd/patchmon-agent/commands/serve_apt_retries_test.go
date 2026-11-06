package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"patchmon-agent/internal/config"
)

func aptTestManager(t *testing.T, extra string) (*config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	body := "patchmon_server: https://pm.example.com\n" +
		"credentials_file: " + filepath.Join(dir, "credentials.yml") + "\n" +
		"log_file: " + filepath.Join(dir, "agent.log") + "\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := config.New()
	m.SetConfigFile(path)
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	prev := cfgManager
	cfgManager = m
	t.Cleanup(func() { cfgManager = prev })
	return m, path
}

func TestApplyAptDisableRetries(t *testing.T) {
	f, tr := false, true

	t.Run("nil leaves value and file untouched", func(t *testing.T) {
		m, path := aptTestManager(t, "apt:\n  disable_retries: false\n")
		before, _ := os.ReadFile(path)
		applyAptDisableRetries(nil)
		after, _ := os.ReadFile(path)
		if m.GetAptDisableRetries() || string(before) != string(after) {
			t.Error("nil payload changed state")
		}
	})

	t.Run("same value does not rewrite config", func(t *testing.T) {
		_, path := aptTestManager(t, "")
		if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
			t.Fatal(err)
		}
		applyAptDisableRetries(&tr)
		st, _ := os.Stat(path)
		if !st.ModTime().Equal(fixedTime) {
			t.Error("config.yml rewritten for an unchanged value")
		}
	})

	t.Run("changed value persists", func(t *testing.T) {
		m, path := aptTestManager(t, "")
		applyAptDisableRetries(&f)
		if m.GetAptDisableRetries() {
			t.Error("value not applied")
		}
		reloaded := config.New()
		reloaded.SetConfigFile(path)
		if err := reloaded.LoadConfig(); err != nil {
			t.Fatal(err)
		}
		if reloaded.GetAptDisableRetries() {
			t.Error("value not persisted")
		}
	})
}

var fixedTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
