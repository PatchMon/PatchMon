package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"patchmon-agent/internal/config"
	"patchmon-agent/internal/packages"
	"patchmon-agent/pkg/models"
)

func TestWindowsFinalStageOutlivesALongRun(t *testing.T) {
	var mu sync.Mutex
	var stages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/output") {
			var body struct {
				Stage string `json:"stage"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			stages = append(stages, body.Stage)
			mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	m := config.New()
	cfg := m.GetConfig()
	cfg.PatchmonServer = srv.URL
	cfg.CredentialsFile = filepath.Join(t.TempDir(), "credentials.yml")
	if err := m.SaveCredentials("id", "key"); err != nil {
		t.Fatal(err)
	}
	prevCfg, prevTimeout, prevRun := cfgManager, windowsFinalTimeout, runWindowsPatch
	t.Cleanup(func() { cfgManager, windowsFinalTimeout, runWindowsPatch = prevCfg, prevTimeout, prevRun })
	cfgManager = m
	windowsFinalTimeout = 200 * time.Millisecond
	runWindowsPatch = func(context.Context, context.Context, packages.WindowsPatchRequest, packages.WindowsPatchHooks, func() []models.Package) packages.WindowsPatchResult {
		time.Sleep(3 * windowsFinalTimeout)
		return packages.WindowsPatchResult{Stage: "dry_run_completed"}
	}

	if err := runPatchWindows("11111111-1111-4111-8111-111111111111", packages.WindowsPatchRequest{PatchAll: true, DryRun: true, IncludeUpdates: true}, ""); err != nil {
		t.Fatalf("final stage not delivered after a run longer than its timeout: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(stages) == 0 || stages[len(stages)-1] != "dry_run_completed" {
		t.Fatalf("stages = %v", stages)
	}
}
