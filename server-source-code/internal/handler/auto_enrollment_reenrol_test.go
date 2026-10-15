package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type fakePresence struct{ seen map[string]bool }

func (f *fakePresence) Get(apiID string) agentregistry.ConnectionInfo {
	return agentregistry.ConnectionInfo{Connected: f.seen[apiID]}
}

func enrolToken(id string) db.AutoEnrollmentToken {
	return db.AutoEnrollmentToken{ID: id, TokenName: "test"}
}

type enrolFixture struct {
	t     *testing.T
	d     *database.DB
	hosts *store.HostsStore
	h     *AutoEnrollmentHandler
}

func newEnrolFixture(t *testing.T) *enrolFixture {
	d := testDatabase(t)
	hosts := store.NewHostsStore(d)
	h := NewAutoEnrollmentHandler(store.NewAutoEnrollmentStore(d), store.NewHostGroupsStore(d), hosts, store.NewSettingsStore(d), &fakePresence{seen: map[string]bool{}}, slog.Default(), &config.Config{})
	return &enrolFixture{t: t, d: d, hosts: hosts, h: h}
}

func (f *enrolFixture) token(maxPerDay, createdToday int) (id, key, secret string) {
	f.t.Helper()
	id = uuid.New().String()
	key = "patchmon_ae_" + uuid.New().String()[:12]
	secret = "s-" + uuid.New().String()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.d.Exec(context.Background(),
		`INSERT INTO auto_enrollment_tokens (id, token_name, token_key, token_secret, allowed_ip_ranges, max_hosts_per_day, hosts_created_today, updated_at)
		 VALUES ($1, $2, $3, $4, '{}', $5, $6, NOW())`,
		id, "tok-"+id[:8], key, string(hash), maxPerDay, createdToday); err != nil {
		f.t.Fatal(err)
	}
	return id, key, secret
}

func (f *enrolFixture) enrol(key, secret string, body map[string]interface{}) (int, map[string]interface{}) {
	f.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auto-enrollment/enroll", strings.NewReader(string(b)))
	req.Header.Set("X-Auto-Enrollment-Key", key)
	req.Header.Set("X-Auto-Enrollment-Secret", secret)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.h.Enroll(rec, req)
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (f *enrolFixture) createdToday(tokenID string) int {
	f.t.Helper()
	var n int
	if err := f.d.RawQueryRow(context.Background(), `SELECT hosts_created_today FROM auto_enrollment_tokens WHERE id = $1`, tokenID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func hostField(out map[string]interface{}, k string) string {
	h, _ := out["host"].(map[string]interface{})
	s, _ := h[k].(string)
	return s
}

func TestEnroll_HostIDReissueRules(t *testing.T) {
	f := newEnrolFixture(t)
	ctx := context.Background()
	tokID, key, secret := f.token(100, 0)
	name := "reenrol-" + uuid.New().String()[:8]

	code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name})
	if code != http.StatusCreated {
		t.Fatalf("first enrol: %d %v", code, out)
	}
	hostID := hostField(out, "id")
	apiID := hostField(out, "api_id")
	created, _ := f.hosts.GetByID(ctx, hostID)
	if created.EnrolmentTokenID == nil || *created.EnrolmentTokenID != tokID {
		t.Fatalf("enrolment_token_id = %v, want %s", created.EnrolmentTokenID, tokID)
	}
	if n := f.createdToday(tokID); n != 1 {
		t.Fatalf("hosts_created_today = %d, want 1", n)
	}

	t.Run("never reported and same token re-issues the key", func(t *testing.T) {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
		if code != http.StatusOK || out["reenrolled"] != true {
			t.Fatalf("got %d %v, want 200 reenrolled", code, out)
		}
		if hostField(out, "id") != hostID || hostField(out, "api_id") != apiID {
			t.Fatalf("re-issue must keep the row and API ID: %v", out)
		}
		row, _ := f.hosts.GetByID(ctx, hostID)
		if ok, _ := util.VerifyAPIKey(hostField(out, "api_key"), row.ApiKey); !ok {
			t.Fatal("the re-issued key does not verify")
		}
		if row.CredentialsReissuedAt == nil {
			t.Fatal("credentials_reissued_at not set")
		}
		if n := f.createdToday(tokID); n != 1 {
			t.Fatalf("re-issue consumed the daily cap: hosts_created_today = %d", n)
		}
	})

	t.Run("a second re-issue within 60s is refused", func(t *testing.T) {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
		if code != http.StatusTooManyRequests || out["code"] != "reenrol_cooldown" {
			t.Fatalf("got %d %v, want 429 reenrol_cooldown", code, out)
		}
	})

	if _, err := f.d.Exec(ctx, `UPDATE hosts SET credentials_reissued_at = NOW() - INTERVAL '2 minutes' WHERE id = $1`, hostID); err != nil {
		t.Fatal(err)
	}

	t.Run("another token cannot re-issue and learns no name", func(t *testing.T) {
		_, key2, secret2 := f.token(100, 0)
		code, out := f.enrol(key2, secret2, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
		if code != http.StatusConflict || out["code"] != "host_not_reenrollable" || out["host_id"] != hostID {
			t.Fatalf("got %d %v, want 409 host_not_reenrollable", code, out)
		}
		if _, ok := out["friendly_name"]; ok {
			t.Fatalf("friendly_name leaked to another token: %v", out)
		}
	})

	t.Run("a different machine cannot take the row", func(t *testing.T) {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "sibling", "host_id": hostID})
		if code != http.StatusConflict || out["code"] != "host_not_reenrollable" {
			t.Fatalf("got %d %v, want 409 host_not_reenrollable", code, out)
		}
	})

	t.Run("an agent seen on the WebSocket blocks re-issue", func(t *testing.T) {
		f.h.presence.(*fakePresence).seen[apiID] = true
		defer delete(f.h.presence.(*fakePresence).seen, apiID)
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
		if code != http.StatusConflict || out["code"] != "host_reporting" {
			t.Fatalf("got %d %v, want 409 host_reporting", code, out)
		}
	})

	t.Run("a UI-created host cannot be re-issued", func(t *testing.T) {
		ui := newTestHost(t, f.hosts, "pending")
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": ui.FriendlyName, "host_id": ui.ID})
		if code != http.StatusConflict || out["code"] != "host_not_reenrollable" {
			t.Fatalf("got %d %v, want 409 host_not_reenrollable", code, out)
		}
	})

	t.Run("a host that has reported is never re-keyed", func(t *testing.T) {
		if _, err := f.d.Exec(ctx, `UPDATE hosts SET status = 'active', agent_version = '2.1.4' WHERE id = $1`, hostID); err != nil {
			t.Fatal(err)
		}
		before, _ := f.hosts.GetByID(ctx, hostID)
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
		if code != http.StatusConflict || out["code"] != "host_reporting" || out["friendly_name"] != name {
			t.Fatalf("got %d %v, want 409 host_reporting", code, out)
		}
		after, _ := f.hosts.GetByID(ctx, hostID)
		if after.ApiKey != before.ApiKey {
			t.Fatal("api_key changed on a refused re-issue")
		}
	})

	t.Run("an unknown host_id creates a new host", func(t *testing.T) {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": "new-" + name, "host_id": uuid.New().String()})
		if code != http.StatusCreated || hostField(out, "id") == hostID {
			t.Fatalf("got %d %v, want 201 with a new row", code, out)
		}
	})

	t.Run("an invalid host_id is rejected", func(t *testing.T) {
		code, _ := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "host_id": "bad id;"})
		if code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", code)
		}
	})

	t.Run("no host_id falls back to the machine_id guard", func(t *testing.T) {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name})
		if code != http.StatusConflict || out["code"] != "machine_id_match" {
			t.Fatalf("got %d %v, want 409 machine_id_match", code, out)
		}
	})
}

func TestEnroll_ReissueIsNotBlockedByTheDailyCap(t *testing.T) {
	f := newEnrolFixture(t)
	tokID, key, secret := f.token(1, 0)
	code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": "cap-" + uuid.New().String()[:8]})
	if code != http.StatusCreated {
		t.Fatalf("first enrol: %d %v", code, out)
	}
	hostID := hostField(out, "id")

	if code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": "cap-2"}); code != http.StatusTooManyRequests || out["code"] != nil {
		t.Fatalf("create past the cap: %d %v, want the plain 429", code, out)
	}
	if code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": "cap", "host_id": hostID}); code != http.StatusOK {
		t.Fatalf("re-issue past the cap: %d %v, want 200", code, out)
	}
	if n := f.createdToday(tokID); n != 1 {
		t.Fatalf("hosts_created_today = %d, want 1", n)
	}
}

func TestServeInstall_InjectsHostID(t *testing.T) {
	d := testDatabase(t)
	hosts := store.NewHostsStore(d)
	host := newTestHost(t, hosts, "pending")
	t.Setenv("AI_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	enc, err := util.NewEncryption()
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	h := NewInstallHandler(hosts, store.NewSettingsStore(d), store.NewBootstrapStore(&hostctx.RedisResolver{Default: rdb}, enc), store.NewReportStore(d), nil)

	for _, osParam := range []string{"linux", "windows"} {
		t.Run(osParam, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/install?os="+osParam, nil)
			req.Header.Set("X-API-ID", host.ApiID)
			req.Header.Set("X-API-KEY", host.ApiKey)
			rec := httptest.NewRecorder()
			h.ServeInstall(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("install: %d %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			want := `export PATCHMON_HOST_ID="` + host.ID + `"`
			if osParam == "windows" {
				want = `$env:PATCHMON_HOST_ID = "` + host.ID + `"`
			}
			if !strings.Contains(body, want) {
				t.Fatalf("served script lacks %q", want)
			}
			if osParam != "linux" {
				return
			}
			sh, err := exec.LookPath("sh")
			if err != nil {
				t.Skip("sh not available")
			}
			tmp := t.TempDir() + "/install.sh"
			if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(sh, "-n", tmp).CombinedOutput(); err != nil {
				t.Fatalf("served script fails sh -n: %v\n%s", err, out)
			}
		})
	}
}

func TestEnroll_MachineIDGuardSurvivesReportOverwrite(t *testing.T) {
	f := newEnrolFixture(t)
	ctx := context.Background()
	_, key, secret := f.token(100, 0)
	name := "rebuilt-" + uuid.New().String()[:8]
	raw := strings.ReplaceAll(uuid.New().String(), "-", "")

	code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": raw})
	if code != http.StatusCreated {
		t.Fatalf("first enrol: %d %v", code, out)
	}
	hostID := hostField(out, "id")
	dashed := strings.ToUpper(raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:])
	if _, err := f.d.Exec(ctx, `UPDATE hosts SET machine_id = $2, status = 'active', agent_version = '2.1.4' WHERE id = $1`, hostID, dashed); err != nil {
		t.Fatal(err)
	}
	row, _ := f.hosts.GetByID(ctx, hostID)
	if row.EnrolmentMachineID == nil || *row.EnrolmentMachineID != raw {
		t.Fatalf("enrolment_machine_id = %v, want the enrolment value %q", row.EnrolmentMachineID, raw)
	}

	for _, sent := range []string{raw, dashed, strings.ToLower(dashed), " " + raw + "\n"} {
		code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": strings.ToUpper(name), "machine_id": sent})
		if code != http.StatusConflict || out["code"] != "machine_id_match" {
			t.Fatalf("rerun with %q: %d %v, want 409 machine_id_match", sent, code, out)
		}
	}

	agentOnly := "AB" + strings.Repeat("c", 30)
	if _, err := f.d.Exec(ctx, `UPDATE hosts SET machine_id = $2 WHERE id = $1`, hostID, agentOnly); err != nil {
		t.Fatal(err)
	}
	if code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": strings.ToLower(agentOnly)}); code != http.StatusConflict {
		t.Fatalf("match on the agent-reported value: %d %v, want 409", code, out)
	}
}

func TestEnroll_JunkMachineIDsNeverMatch(t *testing.T) {
	f := newEnrolFixture(t)
	_, key, secret := f.token(100, 0)
	for _, junk := range []string{"uninitialized", "00000000-0000-0000-0000-000000000000", "03000200-0400-0500-0006-000700080009"} {
		name := "junk-" + uuid.New().String()[:8]
		for i := 0; i < 2; i++ {
			if code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": junk}); code != http.StatusCreated {
				t.Fatalf("%s enrol %d: %d %v, want 201 (junk is not an identity)", junk, i, code, out)
			}
		}
	}
}

func TestEnroll_StoresTrimmedEnrolmentMachineID(t *testing.T) {
	f := newEnrolFixture(t)
	_, key, secret := f.token(100, 0)
	name := "trim-" + uuid.New().String()[:8]
	raw := strings.ReplaceAll(uuid.New().String(), "-", "")
	code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "  " + raw + "\n"})
	if code != http.StatusCreated {
		t.Fatalf("enrol: %d %v", code, out)
	}
	row, _ := f.hosts.GetByID(context.Background(), hostField(out, "id"))
	if row.EnrolmentMachineID == nil || *row.EnrolmentMachineID != raw {
		t.Fatalf("enrolment_machine_id = %v, want %q", row.EnrolmentMachineID, raw)
	}
	if code, _ := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": raw}); code != http.StatusConflict {
		t.Fatalf("rerun with the clean ID: %d, want 409", code)
	}
}

func TestEnroll_ReissueKeepsHostGroups(t *testing.T) {
	f := newEnrolFixture(t)
	ctx := context.Background()
	tokID, key, secret := f.token(100, 0)
	groupA, groupB := uuid.New().String(), uuid.New().String()
	for _, g := range []string{groupA, groupB} {
		if _, err := f.d.Exec(ctx, `INSERT INTO host_groups (id, name, updated_at) VALUES ($1, $2, NOW())`, g, "g-"+g[:8]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.d.Exec(ctx, `UPDATE auto_enrollment_tokens SET default_host_group_id = $1 WHERE id = $2`, groupA, tokID); err != nil {
		t.Fatal(err)
	}
	name := "groups-" + uuid.New().String()[:8]
	code, out := f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name})
	if code != http.StatusCreated {
		t.Fatalf("first enrol: %d %v", code, out)
	}
	hostID := hostField(out, "id")
	if err := f.hosts.SetHostGroups(ctx, hostID, []string{groupB}); err != nil {
		t.Fatal(err)
	}

	code, out = f.enrol(key, secret, map[string]interface{}{"friendly_name": name, "machine_id": "m-" + name, "host_id": hostID})
	if code != http.StatusOK || out["reenrolled"] != true {
		t.Fatalf("re-issue: %d %v", code, out)
	}
	var groups []string
	rows, err := f.d.Raw(ctx, `SELECT host_group_id FROM host_group_memberships WHERE host_id = $1 ORDER BY host_group_id`, hostID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, g)
	}
	rows.Close()
	if len(groups) != 1 || groups[0] != groupB {
		t.Fatalf("groups after re-issue = %v, want the admin's choice [%s]", groups, groupB)
	}
}
