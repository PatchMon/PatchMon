package store

import (
	"context"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
)

func identityTestDB(t *testing.T) (*database.DB, *HostsStore) {
	t.Helper()
	d := pgtest.Open(t)
	return d, NewHostsStore(d)
}

func pendingHost(t *testing.T, hosts *HostsStore) *models.Host {
	t.Helper()
	h := &models.Host{
		FriendlyName: "store-ident-" + uuid.New().String()[:8],
		OSType:       "unknown",
		OSVersion:    "unknown",
		Status:       "pending",
		ApiID:        "patchmon_" + uuid.New().String(),
		ApiKey:       "k-" + uuid.New().String(),
	}
	if err := hosts.Create(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestReissueApiKeyGuards(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	h := pendingHost(t, hosts)
	if ok, err := hosts.ReissueApiKey(ctx, h.ID, "new-1-"+uuid.New().String()); err != nil || !ok {
		t.Fatalf("first re-issue: ok=%v err=%v", ok, err)
	}
	if ok, _ := hosts.ReissueApiKey(ctx, h.ID, "new-2-"+uuid.New().String()); ok {
		t.Fatal("a second re-issue inside 60 s must match no row")
	}
	exec(`UPDATE hosts SET credentials_reissued_at = NOW() - INTERVAL '61 seconds' WHERE id = $1`, h.ID)
	if ok, _ := hosts.ReissueApiKey(ctx, h.ID, "new-3-"+uuid.New().String()); !ok {
		t.Fatal("re-issue after the cooldown should succeed")
	}

	for name, sql := range map[string]string{
		"active status":       `UPDATE hosts SET status = 'active' WHERE id = $1`,
		"agent version":       `UPDATE hosts SET agent_version = '2.1.4' WHERE id = $1`,
		"full report arrived": `UPDATE hosts SET last_full_report_at = NOW() WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			r := pendingHost(t, hosts)
			exec(sql, r.ID)
			if ok, _ := hosts.ReissueApiKey(ctx, r.ID, "x-"+uuid.New().String()); ok {
				t.Fatal("a host that has reported must never be re-keyed")
			}
			got, _ := hosts.GetByID(ctx, r.ID)
			if got.ApiKey != r.ApiKey {
				t.Fatal("api_key changed")
			}
		})
	}
}

func TestUpdateIdentityFingerprintKeepsConflict(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	h := pendingHost(t, hosts)
	first := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	if err := hosts.UpdateIdentityFingerprint(ctx, h.ID, IdentityFingerprintUpdate{Fingerprint: "a", ChangedAt: first, ConflictAt: &first}); err != nil {
		t.Fatal(err)
	}
	later := time.Now().UTC()
	prev := "a"
	if err := hosts.UpdateIdentityFingerprint(ctx, h.ID, IdentityFingerprintUpdate{Fingerprint: "b", PrevFingerprint: &prev, ChangedAt: later, ConflictAt: &later}); err != nil {
		t.Fatal(err)
	}
	got, _ := hosts.GetByID(ctx, h.ID)
	if got.IdentityConflictAt == nil || !got.IdentityConflictAt.Equal(first) {
		t.Fatalf("identity_conflict_at = %v, want the first flag %v kept", got.IdentityConflictAt, first)
	}
	if got.IdentityFingerprint == nil || *got.IdentityFingerprint != "b" || got.IdentityPrevFingerprint == nil || *got.IdentityPrevFingerprint != "a" {
		t.Fatalf("fingerprints = %v / %v", got.IdentityFingerprint, got.IdentityPrevFingerprint)
	}

	if found, err := hosts.DismissIdentityConflict(ctx, h.ID); err != nil || !found {
		t.Fatalf("dismiss: %v %v", found, err)
	}
	if err := hosts.UpdateApiCredentials(ctx, h.ID, "patchmon_"+uuid.New().String(), "hash-"+uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	var fp, conflict *string
	if err := d.RawQueryRow(ctx, `SELECT identity_fingerprint, identity_conflict_at::text FROM hosts WHERE id = $1`, h.ID).Scan(&fp, &conflict); err != nil {
		t.Fatal(err)
	}
	if fp != nil || conflict != nil {
		t.Fatalf("regenerated credentials kept fingerprint %v / conflict %v", fp, conflict)
	}
}
