package commands

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"patchmon-agent/internal/config"

	"github.com/sirupsen/logrus"
)

func useRemoteActionsConfig(t *testing.T, extra string) {
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
	prevCfg, prevLogger := cfgManager, logger
	cfgManager = m
	logger = logrus.New()
	logger.SetOutput(io.Discard)
	t.Cleanup(func() { cfgManager, logger = prevCfg, prevLogger })
}

func TestAgentUpdateRefusedWhenRemoteActionsDisabled(t *testing.T) {
	calls := stubAgentUpdater(t)
	useRemoteActionsConfig(t, "remote_actions:\n  enabled: false\n")

	deferred, err := updateAgentUnlessPatching()
	if !errors.Is(err, errRemoteActionsDisabled) || deferred {
		t.Fatalf("deferred=%v err=%v, want errRemoteActionsDisabled", deferred, err)
	}
	if *calls != 0 {
		t.Fatalf("agent updater ran %d times with remote actions disabled", *calls)
	}
}

func TestAgentUpdateRunsWhenRemoteActionsEnabled(t *testing.T) {
	calls := stubAgentUpdater(t)
	useRemoteActionsConfig(t, "")

	if _, err := updateAgentUnlessPatching(); err != nil || *calls != 1 {
		t.Fatalf("err=%v calls=%d, want an update by default", err, *calls)
	}
}

func TestRemoteActionRefused(t *testing.T) {
	useRemoteActionsConfig(t, "remote_actions:\n  enabled: false\n")
	if !remoteActionRefused("run_patch") {
		t.Error("run_patch allowed with remote actions disabled")
	}

	useRemoteActionsConfig(t, "")
	if remoteActionRefused("run_patch") {
		t.Error("run_patch refused although remote actions default to enabled")
	}
}
