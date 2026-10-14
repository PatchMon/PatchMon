package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func strp(s string) *string { return &s }

func TestIdentityFingerprintNormalisesMachineID(t *testing.T) {
	a := identityFingerprint("3B0C2F7E-6A1D-4C55-9A0E-2F1F7D9E8C41", "h")
	b := identityFingerprint("3b0c2f7e6a1d4c559a0e2f1f7d9e8c41", "h")
	if a == "" || a != b {
		t.Fatalf("dashes and case must not change the fingerprint: %q vs %q", a, b)
	}
	if identityFingerprint("abc", "h1") == identityFingerprint("abc", "h2") {
		t.Fatal("hostname hash must change the fingerprint")
	}
	if identityFingerprint("", "h") != "" || identityFingerprint("abc", "") != "" {
		t.Fatal("a missing input must give no fingerprint")
	}
}

func TestNextIdentityFingerprint(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour)
	old := now.Add(-25 * time.Hour)
	flagged := now.Add(-time.Minute)

	cases := []struct {
		name        string
		cur         store.IdentityFingerprintState
		f           string
		wantUpdate  bool
		wantPrev    *string
		wantConflct bool
		wantFlagged bool
	}{
		{name: "first fingerprint", f: "A", wantUpdate: true},
		{name: "steady state writes nothing", cur: store.IdentityFingerprintState{Fingerprint: strp("A"), ChangedAt: &recent}, f: "A"},
		{name: "empty fingerprint writes nothing", cur: store.IdentityFingerprintState{Fingerprint: strp("A")}, f: ""},
		{name: "single change is not flagged", cur: store.IdentityFingerprintState{Fingerprint: strp("A"), ChangedAt: &recent}, f: "B", wantUpdate: true, wantPrev: strp("A")},
		{name: "change to a third value is not flagged", cur: store.IdentityFingerprintState{Fingerprint: strp("B"), PrevFingerprint: strp("A"), ChangedAt: &recent}, f: "C", wantUpdate: true, wantPrev: strp("B")},
		{name: "flip back within 24h is flagged", cur: store.IdentityFingerprintState{Fingerprint: strp("B"), PrevFingerprint: strp("A"), ChangedAt: &recent}, f: "A", wantUpdate: true, wantPrev: strp("B"), wantConflct: true, wantFlagged: true},
		{name: "flip back after 24h is a plain change", cur: store.IdentityFingerprintState{Fingerprint: strp("B"), PrevFingerprint: strp("A"), ChangedAt: &old}, f: "A", wantUpdate: true, wantPrev: strp("B")},
		{name: "continued flapping keeps the flag but does not re-log", cur: store.IdentityFingerprintState{Fingerprint: strp("A"), PrevFingerprint: strp("B"), ChangedAt: &recent, ConflictAt: &flagged}, f: "B", wantUpdate: true, wantPrev: strp("A"), wantConflct: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, newly := nextIdentityFingerprint(c.cur, c.f, now)
			if (u != nil) != c.wantUpdate {
				t.Fatalf("update = %+v, want update %v", u, c.wantUpdate)
			}
			if newly != c.wantFlagged {
				t.Errorf("newlyFlagged = %v, want %v", newly, c.wantFlagged)
			}
			if u == nil {
				return
			}
			if u.Fingerprint != c.f || !u.ChangedAt.Equal(now) {
				t.Errorf("update = %+v", u)
			}
			if (u.PrevFingerprint == nil) != (c.wantPrev == nil) || (u.PrevFingerprint != nil && *u.PrevFingerprint != *c.wantPrev) {
				t.Errorf("prev = %v, want %v", u.PrevFingerprint, c.wantPrev)
			}
			if (u.ConflictAt != nil) != c.wantConflct {
				t.Errorf("conflictAt = %v, want set %v", u.ConflictAt, c.wantConflct)
			}
		})
	}
}

func TestDecideReenrol(t *testing.T) {
	now := time.Now()
	justNow := now.Add(-10 * time.Second)
	aWhileAgo := now.Add(-2 * time.Minute)
	reportedAt := now.Add(-time.Hour)
	pending := func(mut func(h *models.Host)) *models.Host {
		h := &models.Host{ID: "h1", Status: "pending", EnrolmentTokenID: strp("tok"), EnrolmentMachineID: strp("Machine-A")}
		if mut != nil {
			mut(h)
		}
		return h
	}
	cases := []struct {
		name      string
		host      *models.Host
		machineID string
		agentSeen bool
		want      reenrolOutcome
	}{
		{"never reported, same token", pending(nil), "", false, reenrolReissue},
		{"different machine_id", pending(nil), "machine-b", false, reenrolNotReenrollable},
		{"same machine_id in another case and dash form", pending(nil), "machinea", false, reenrolReissue},
		{"no machine_id against a stored one", pending(nil), "-", false, reenrolNotReenrollable},
		{"no machine_id against a placeholder", pending(func(h *models.Host) { h.EnrolmentMachineID = nil }), "-", false, reenrolReissue},
		{"junk machine_id against a placeholder", pending(func(h *models.Host) { h.EnrolmentMachineID = nil }), "uninitialized", false, reenrolReissue},
		{"real machine_id against a placeholder", pending(func(h *models.Host) { h.EnrolmentMachineID = nil }), "machine-b", false, reenrolNotReenrollable},
		{"agent seen on the WebSocket", pending(nil), "", true, reenrolHostReporting},
		{"re-issued over a minute ago", pending(func(h *models.Host) { h.CredentialsReissuedAt = &aWhileAgo }), "", false, reenrolReissue},
		{"re-issued seconds ago", pending(func(h *models.Host) { h.CredentialsReissuedAt = &justNow }), "", false, reenrolCooldown},
		{"other token", pending(func(h *models.Host) { h.EnrolmentTokenID = strp("other") }), "", false, reenrolNotReenrollable},
		{"UI-created or pre-2.1.4 row", pending(func(h *models.Host) { h.EnrolmentTokenID = nil }), "", false, reenrolNotReenrollable},
		{"has an agent version", pending(func(h *models.Host) { h.AgentVersion = strp("2.1.4") }), "", false, reenrolHostReporting},
		{"has a full report", pending(func(h *models.Host) { h.LastFullReportAt = &reportedAt }), "", false, reenrolHostReporting},
		{"active", pending(func(h *models.Host) { h.Status = "active" }), "", false, reenrolHostReporting},
		{"reporting beats cooldown", pending(func(h *models.Host) { h.Status = "active"; h.CredentialsReissuedAt = &justNow }), "", false, reenrolHostReporting},
	}
	for _, c := range cases {
		mid := c.machineID
		switch mid {
		case "":
			mid = "machine-a"
		case "-":
			mid = ""
		}
		if got := decideReenrol(c.host, "tok", mid, c.agentSeen, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

type raceEnrollHosts struct {
	fakeEnrollHosts
	byID *models.Host
}

func (f *raceEnrollHosts) GetByID(context.Context, string) (*models.Host, error) { return f.byID, nil }

func TestReenrolExistingHost_ZeroRowsRace(t *testing.T) {
	for _, c := range []struct {
		name     string
		after    *models.Host
		wantCode int
		wantBody string
	}{
		{"a report landed first", &models.Host{ID: "h1", Status: "active"}, http.StatusConflict, `"code":"host_reporting"`},
		{"a concurrent re-issue landed first", &models.Host{ID: "h1", Status: "pending"}, http.StatusTooManyRequests, `"code":"reenrol_cooldown"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			fake := &raceEnrollHosts{byID: c.after}
			h := &AutoEnrollmentHandler{hosts: fake}
			rec := httptest.NewRecorder()
			existing := &models.Host{ID: "h1", Status: "pending", EnrolmentTokenID: strp("tok"), EnrolmentMachineID: strp("m")}
			h.reenrolExistingHost(context.Background(), rec, httptest.NewRequest(http.MethodPost, "/", nil), existing, enrolToken("tok"), "m")
			if rec.Code != c.wantCode || !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body.String(), c.wantCode, c.wantBody)
			}
		})
	}
}

func identityDB(t *testing.T) (*database.DB, *store.HostsStore) {
	t.Helper()
	d := testDatabase(t)
	return d, store.NewHostsStore(d)
}

func newTestHost(t *testing.T, hosts *store.HostsStore, status string) *models.Host {
	t.Helper()
	h := &models.Host{
		FriendlyName: "ident-" + uuid.New().String()[:8],
		OSType:       "linux",
		OSVersion:    "test",
		Status:       status,
		ApiID:        "patchmon_" + uuid.New().String(),
		ApiKey:       "plaintext-" + uuid.New().String(),
	}
	if err := hosts.Create(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return h
}

func ping(t *testing.T, h *InstallHandler, host *models.Host, body string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/ping", strings.NewReader(body))
	if body == "" {
		req.ContentLength = 0
	}
	req.Header.Set("X-API-ID", host.ApiID)
	req.Header.Set("X-API-KEY", host.ApiKey)
	rec := httptest.NewRecorder()
	h.ServePing(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ping: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func identityRow(t *testing.T, d *database.DB, id string) (configVersion, reported *string, conflict bool, fp *string) {
	t.Helper()
	var c *time.Time
	if err := d.RawQueryRow(context.Background(),
		`SELECT config_version, reported_host_id, identity_conflict_at, identity_fingerprint FROM hosts WHERE id = $1`, id).
		Scan(&configVersion, &reported, &c, &fp); err != nil {
		t.Fatal(err)
	}
	return configVersion, reported, c != nil, fp
}

func TestServePing_HostIdentity(t *testing.T) {
	d, hosts := identityDB(t)
	h := NewInstallHandler(hosts, store.NewSettingsStore(d), nil, store.NewReportStore(d), nil)

	t.Run("empty-body heartbeat returns hostId", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		out := ping(t, h, host, "")
		if out["hostId"] != host.ID {
			t.Fatalf("hostId = %v, want %s", out["hostId"], host.ID)
		}
		if _, ok := out["hostIdMismatch"]; ok {
			t.Fatal("no mismatch flag expected on an empty body")
		}
	})

	t.Run("old agent body leaves the new columns NULL", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		out := ping(t, h, host, `{"agentVersion":"2.1.3","hashes":{"packagesHash":"x"}}`)
		if out["hostId"] != host.ID {
			t.Fatalf("hostId = %v", out["hostId"])
		}
		cv, rep, conflict, fp := identityRow(t, d, host.ID)
		if cv != nil || rep != nil || conflict || fp != nil {
			t.Fatalf("columns = %v %v %v %v, want all NULL", cv, rep, conflict, fp)
		}
	})

	t.Run("matching hostId and configVersion are stored", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		out := ping(t, h, host, `{"agentVersion":"2.1.4-rc.1","configVersion":"2.1.4-rc.1","hostId":"`+host.ID+`"}`)
		if _, ok := out["hostIdMismatch"]; ok {
			t.Fatal("unexpected mismatch flag")
		}
		cv, rep, _, _ := identityRow(t, d, host.ID)
		if cv == nil || *cv != "2.1.4-rc.1" || rep == nil || *rep != host.ID {
			t.Fatalf("config_version=%v reported_host_id=%v", cv, rep)
		}
		got, _ := hosts.GetByID(context.Background(), host.ID)
		if got.HostIDMismatch() {
			t.Fatal("HostIDMismatch() true for a matching ID")
		}
	})

	t.Run("dev build config version is stored verbatim", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		ping(t, h, host, `{"configVersion":"2.1.3-main-93c589a4"}`)
		if cv, _, _, _ := identityRow(t, d, host.ID); cv == nil || *cv != "2.1.3-main-93c589a4" {
			t.Fatalf("config_version = %v", cv)
		}
	})

	t.Run("downgrade to an agent without config versioning clears config_version", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		ping(t, h, host, `{"agentVersion":"2.1.4","configVersion":"2.1.4"}`)
		ping(t, h, host, "")
		if cv, _, _, _ := identityRow(t, d, host.ID); cv == nil || *cv != "2.1.4" {
			t.Fatalf("an empty heartbeat changed config_version to %v", cv)
		}
		ping(t, h, host, `{"hashes":{"packagesHash":"x"}}`)
		if cv, _, _, _ := identityRow(t, d, host.ID); cv == nil {
			t.Fatal("a body without an agent version cleared config_version")
		}
		ping(t, h, host, `{"agentVersion":"2.1.3"}`)
		if cv, _, _, _ := identityRow(t, d, host.ID); cv != nil {
			t.Fatalf("config_version = %q after a pre-2.1.4 agent pinged, want NULL", *cv)
		}
	})

	t.Run("invalid configVersion and hostId are ignored", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		ping(t, h, host, `{"configVersion":"<script>","hostId":"../x"}`)
		if cv, rep, _, _ := identityRow(t, d, host.ID); cv != nil || rep != nil {
			t.Fatalf("config_version=%v reported_host_id=%v, want NULL", cv, rep)
		}
	})

	t.Run("different hostId sets the mismatch flag", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		other := uuid.New().String()
		out := ping(t, h, host, `{"hostId":"`+other+`"}`)
		if out["hostIdMismatch"] != true || out["hostId"] != host.ID {
			t.Fatalf("response = %v", out)
		}
		got, _ := hosts.GetByID(context.Background(), host.ID)
		if !got.HostIDMismatch() {
			t.Fatal("stored row should report a mismatch")
		}
		ping(t, h, host, `{"hostId":"`+host.ID+`"}`)
		got, _ = hosts.GetByID(context.Background(), host.ID)
		if got.HostIDMismatch() {
			t.Fatal("a later matching hostId should clear the mismatch")
		}
	})

	t.Run("clone fingerprint flip-flop within 24h flags the host", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		a := `{"machineId":"aaaa","hashes":{"hostnameHash":"h-one"}}`
		b := `{"machineId":"bbbb","hashes":{"hostnameHash":"h-two"}}`
		ping(t, h, host, a)
		ping(t, h, host, a)
		if _, _, conflict, fp := identityRow(t, d, host.ID); conflict || fp == nil {
			t.Fatalf("steady state: conflict=%v fp=%v", conflict, fp)
		}
		ping(t, h, host, b)
		if _, _, conflict, _ := identityRow(t, d, host.ID); conflict {
			t.Fatal("a single change must not flag")
		}
		ping(t, h, host, a)
		if _, _, conflict, _ := identityRow(t, d, host.ID); !conflict {
			t.Fatal("flip back within 24h must flag a possible clone")
		}

		hh := &HostsHandler{hosts: hosts}
		rec := httptest.NewRecorder()
		hh.DismissIdentityConflict(rec, withHostID(httptest.NewRequest(http.MethodPost, "/", nil), host.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("dismiss: %d %s", rec.Code, rec.Body.String())
		}
		if _, _, conflict, _ := identityRow(t, d, host.ID); conflict {
			t.Fatal("dismiss did not clear the flag")
		}

		ping(t, h, host, b)
		if _, _, conflict, _ := identityRow(t, d, host.ID); conflict {
			t.Fatal("one report after dismiss re-flagged the host")
		}
		ping(t, h, host, b)
		if _, _, conflict, _ := identityRow(t, d, host.ID); conflict {
			t.Fatal("the machine that stayed re-flagged the host")
		}
		ping(t, h, host, a)
		if _, _, conflict, _ := identityRow(t, d, host.ID); !conflict {
			t.Fatal("two machines still alternating after dismiss must flag again")
		}
	})

	t.Run("flip-flop older than 24h is not flagged", func(t *testing.T) {
		host := newTestHost(t, hosts, "active")
		ping(t, h, host, `{"machineId":"aaaa","hashes":{"hostnameHash":"h-one"}}`)
		ping(t, h, host, `{"machineId":"bbbb","hashes":{"hostnameHash":"h-two"}}`)
		if _, err := d.Exec(context.Background(), `UPDATE hosts SET identity_fingerprint_changed_at = NOW() - INTERVAL '25 hours' WHERE id = $1`, host.ID); err != nil {
			t.Fatal(err)
		}
		ping(t, h, host, `{"machineId":"aaaa","hashes":{"hostnameHash":"h-one"}}`)
		if _, _, conflict, _ := identityRow(t, d, host.ID); conflict {
			t.Fatal("a change back after 24h must not flag")
		}
	})
}

func withHostID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("hostId", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestDismissIdentityConflict_UnknownHost(t *testing.T) {
	_, hosts := identityDB(t)
	hh := &HostsHandler{hosts: hosts}
	rec := httptest.NewRecorder()
	hh.DismissIdentityConflict(rec, withHostID(httptest.NewRequest(http.MethodPost, "/", nil), uuid.New().String()))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("got %d %s, want 404 with an error body", rec.Code, rec.Body.String())
	}
}

func TestRegenerateCredentialsClearsIdentityFlags(t *testing.T) {
	d, hosts := identityDB(t)
	host := newTestHost(t, hosts, "active")
	if _, err := d.Exec(context.Background(),
		`UPDATE hosts SET reported_host_id = 'someone-else', identity_conflict_at = NOW(), identity_fingerprint = 'a', identity_prev_fingerprint = 'b', identity_fingerprint_changed_at = NOW() WHERE id = $1`, host.ID); err != nil {
		t.Fatal(err)
	}
	hh := &HostsHandler{hosts: hosts}
	rec := httptest.NewRecorder()
	hh.RegenerateCredentials(rec, withHostID(httptest.NewRequest(http.MethodPost, "/", nil), host.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
	}
	_, rep, conflict, fp := identityRow(t, d, host.ID)
	if rep != nil || conflict || fp != nil {
		t.Fatalf("after regenerate: reported_host_id=%v conflict=%v fingerprint=%v, want all cleared", rep, conflict, fp)
	}
}

func TestServeUpdate_ReturnsHostIDAndStoresConfigVersion(t *testing.T) {
	d, hosts := identityDB(t)
	h := NewInstallHandler(hosts, store.NewSettingsStore(d), nil, store.NewReportStore(d), nil)
	host := newTestHost(t, hosts, "pending")

	body := `{"packages":[{"name":"bash","currentVersion":"5.2"}],"repositories":[],"osType":"debian","osVersion":"12",` +
		`"hostname":"web-01","agentVersion":"2.1.4","machineId":"abcd","hostId":"` + host.ID + `","configVersion":"2.1.4"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/update", strings.NewReader(body))
	req.Header.Set("X-API-ID", host.ApiID)
	req.Header.Set("X-API-KEY", host.ApiKey)
	rec := httptest.NewRecorder()
	h.ServeUpdate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["hostId"] != host.ID {
		t.Fatalf("hostId = %v, want %s", out["hostId"], host.ID)
	}
	cv, rep, _, fp := identityRow(t, d, host.ID)
	if cv == nil || *cv != "2.1.4" || rep == nil || *rep != host.ID || fp == nil {
		t.Fatalf("config_version=%v reported_host_id=%v fingerprint=%v", cv, rep, fp)
	}

	old := `{"packages":[{"name":"bash","currentVersion":"5.2"}],"repositories":[],"osType":"debian","osVersion":"12","hostname":"web-01","agentVersion":"2.1.3","machineId":"abcd"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/hosts/update", strings.NewReader(old))
	req.Header.Set("X-API-ID", host.ApiID)
	req.Header.Set("X-API-KEY", host.ApiKey)
	rec = httptest.NewRecorder()
	h.ServeUpdate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("old-agent update: %d %s", rec.Code, rec.Body.String())
	}
	if cv, _, _, _ := identityRow(t, d, host.ID); cv != nil {
		t.Fatalf("config_version = %q after a pre-2.1.4 report, want NULL", *cv)
	}
}

func TestHostIdentityWarningsUseTheInjectedLogger(t *testing.T) {
	d, hosts := identityDB(t)
	var buf bytes.Buffer
	h := NewInstallHandler(hosts, store.NewSettingsStore(d), nil, store.NewReportStore(d), slog.New(slog.NewTextHandler(&buf, nil)))
	host := newTestHost(t, hosts, "active")

	ping(t, h, host, `{"hostId":"`+uuid.New().String()+`"}`)
	a := `{"machineId":"aaaa","hashes":{"hostnameHash":"h-one"}}`
	ping(t, h, host, a)
	ping(t, h, host, `{"machineId":"bbbb","hashes":{"hostnameHash":"h-two"}}`)
	ping(t, h, host, a)

	for _, want := range []string{"agent reported a different host_id", "possible cloned agent"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("injected logger did not receive %q:\n%s", want, buf.String())
		}
	}
}
