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

type outputPost struct {
	Stage        string `json:"stage"`
	ErrorMessage string `json:"error_message"`
}

func fakeOutputServer(t *testing.T) (*sync.Mutex, *[]outputPost) {
	t.Helper()
	var mu sync.Mutex
	var posts []outputPost
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/output") {
			var body outputPost
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			posts = append(posts, body)
			mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	m := config.New()
	m.SetConfigFile(filepath.Join(dir, "config.yml"))
	cfg := m.GetConfig()
	cfg.PatchmonServer = srv.URL
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "logs", "agent.log")
	if err := m.SaveCredentials("id", "key"); err != nil {
		t.Fatal(err)
	}
	prevCfg, prevTimeout, prevRun := cfgManager, windowsFinalTimeout, runWindowsPatch
	t.Cleanup(func() { cfgManager, windowsFinalTimeout, runWindowsPatch = prevCfg, prevTimeout, prevRun })
	cfgManager = m
	return &mu, &posts
}

func TestWindowsFinalStageOutlivesALongRun(t *testing.T) {
	mu, posts := fakeOutputServer(t)
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
	if len(*posts) == 0 || (*posts)[len(*posts)-1].Stage != "dry_run_completed" {
		t.Fatalf("posts = %+v", *posts)
	}
}

func TestWindowsCompletedRunSendsItsSkipSummary(t *testing.T) {
	mu, posts := fakeOutputServer(t)
	runWindowsPatch = func(context.Context, context.Context, packages.WindowsPatchRequest, packages.WindowsPatchHooks, func() []models.Package) packages.WindowsPatchResult {
		return packages.WindowsPatchResult{Stage: "completed", ErrorMessage: "1 skipped: 7zip.7zip (pinned in winget)"}
	}
	err := runPatchWindows("11111111-1111-4111-8111-111111111111", packages.WindowsPatchRequest{DryRun: true, Targets: []packages.WindowsPatchTarget{{Kind: "winget", ID: "7zip.7zip"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	last := (*posts)[len(*posts)-1]
	if last.Stage != "completed" || last.ErrorMessage != "1 skipped: 7zip.7zip (pinned in winget)" {
		t.Fatalf("final post = %+v", last)
	}
}
