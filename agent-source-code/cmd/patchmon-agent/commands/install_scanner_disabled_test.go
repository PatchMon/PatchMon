package commands

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"patchmon-agent/internal/config"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

func TestRunInstallScannerRefusesWhenComplianceDisabled(t *testing.T) {
	var mu sync.Mutex
	var reports []models.IntegrationSetupStatus
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/hosts/integration-status" {
			_, _ = io.Copy(io.Discard, r.Body)
			http.NotFound(w, r)
			return
		}
		var p models.IntegrationSetupStatus
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		reports = append(reports, p)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	contents := "patchmon_server: " + srv.URL + "\napi_version: v1\nintegrations:\n  compliance:\n    enabled: false\n    openscap_enabled: true\n"
	if err := os.WriteFile(cfgPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte("api_id: patchmon_test\napi_key: testkey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := config.New()
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadCredentials(); err != nil {
		t.Fatal(err)
	}
	prevCfg, prevLogger := cfgManager, logger
	cfgManager = m
	logger = logrus.New()
	logger.SetOutput(io.Discard)
	t.Cleanup(func() {
		cfgManager, logger = prevCfg, prevLogger
		installScannerMu.Lock()
		installScannerRunning, installScannerResend, installScannerJobID = false, false, ""
		installScannerMu.Unlock()
		lastInstallOutcome.Store(nil)
	})

	if !startInstallScanner("stale-job") {
		t.Fatal("install slot should be free")
	}
	err := runInstallScanner()
	finishInstallScanner()
	if !errors.Is(err, errComplianceDisabled) {
		t.Fatalf("err = %v, want errComplianceDisabled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 1 {
		t.Fatalf("got %d status reports, want 1", len(reports))
	}
	got := reports[0]
	if got.Status != "disabled" || got.Enabled || got.InstallJobID != "stale-job" {
		t.Fatalf("report = status %q enabled %v job %q", got.Status, got.Enabled, got.InstallJobID)
	}
	if len(got.InstallEvents) != 1 || got.InstallEvents[0].Step != "complete" || got.InstallEvents[0].Status != "failed" {
		t.Fatalf("install events = %+v, want a single failed complete step", got.InstallEvents)
	}
	for _, e := range got.InstallEvents {
		if e.Step == "install_openscap" || e.Step == "detect_os" {
			t.Fatalf("install work started despite compliance being disabled: %+v", e)
		}
	}
}
