package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
)

// fakeMarketingList points SubscribeToList at a local server, so a broken
// switch shows up as a failing test rather than a real sign-up.
func fakeMarketingList(t *testing.T) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	prev := marketingSubscribeURL
	marketingSubscribeURL = srv.URL
	t.Cleanup(func() {
		marketingSubscribeURL = prev
		srv.Close()
	})
	return &hits
}

func TestSubscribeToList_Disabled(t *testing.T) {
	hits := fakeMarketingList(t)
	SetNewsletterDisabled(true)
	t.Cleanup(func() { SetNewsletterDisabled(false) })

	if err := SubscribeToList(context.Background(), "n", "a@b.c"); !errors.Is(err, ErrNewsletterDisabled) {
		t.Fatalf("err = %v, want ErrNewsletterDisabled", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("marketing list received %d request(s) while disabled", n)
	}
}

// Control for the test above: the fake server does see a request when the
// switch is off, so a zero count there really means nothing was sent.
func TestSubscribeToList_Enabled(t *testing.T) {
	hits := fakeMarketingList(t)

	if err := SubscribeToList(context.Background(), "n", "a@b.c"); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("marketing list received %d request(s), want 1", n)
	}
}

func TestMetricsHandler_TelemetryDisabled(t *testing.T) {
	h := NewMetricsHandler(nil, nil, &config.Config{DisableTelemetry: true})
	for name, fn := range map[string]http.HandlerFunc{
		"get": h.Get, "update": h.Update, "regenerate": h.RegenerateID, "send-now": h.SendNow,
	} {
		rec := httptest.NewRecorder()
		fn(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, rec.Code)
		}
	}
}

func TestNetworkSwitchHelpers(t *testing.T) {
	if telemetryEnabled(&config.Config{DisableTelemetry: true}) {
		t.Error("telemetryEnabled with DisableTelemetry")
	}
	if githubVersionOnLoginAllowed(&config.Config{DisableGithubVersionOnLogin: true}) {
		t.Error("githubVersionOnLoginAllowed with DisableGithubVersionOnLogin")
	}
	if newsletterEnabled(&config.Config{DisableNewsletter: true}) {
		t.Error("newsletterEnabled with DisableNewsletter")
	}
	if newsletterEnabled(&config.Config{AdminMode: true}) {
		t.Error("newsletterEnabled with AdminMode")
	}
	if !telemetryEnabled(&config.Config{}) || !githubVersionOnLoginAllowed(&config.Config{}) || !newsletterEnabled(&config.Config{}) {
		t.Error("switches must default to enabled")
	}
}
