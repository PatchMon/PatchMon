package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestStoreAppPatchability(t *testing.T) {
	v := func(s string) *string { return &s }
	cases := []struct {
		name      string
		patchable bool
		reason    string
		source    string
		agent     *string
		wantOK    bool
		wantWhy   string
	}{
		{"old agent keeps the block", false, NotPatchableStoreApp, "winget", v("2.1.8"), false, NotPatchableStoreApp},
		{"unknown agent keeps the block", false, NotPatchableStoreApp, "winget", nil, false, NotPatchableStoreApp},
		{"dev build keeps the block", false, NotPatchableStoreApp, "winget", v("0.0.0"), false, NotPatchableStoreApp},
		{"capable agent lifts it", false, NotPatchableStoreApp, "winget", v("2.1.9"), true, ""},
		{"release candidate lifts it", false, NotPatchableStoreApp, "winget", v("2.1.9-rc.3"), true, ""},
		{"no winget Id stays blocked", false, NotPatchableStoreApp, "", v("2.1.9"), false, "no_winget_id"},
		{"other reasons untouched", false, "winget_pinned", "winget", v("2.1.9"), false, "winget_pinned"},
		{"patchable rows untouched", true, "", "winget", v("2.1.0"), true, ""},
	}
	for _, c := range cases {
		ok, why := StoreAppPatchability(c.patchable, c.reason, c.source, c.agent)
		if ok != c.wantOK || why != c.wantWhy {
			t.Errorf("%s: got %v %q, want %v %q", c.name, ok, why, c.wantOK, c.wantWhy)
		}
	}
}

func TestStoreAppPatchableOnlyOnCapableAgents(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	oldHost, newHost := pendingHost(t, hosts), pendingHost(t, hosts)
	exec(`UPDATE hosts SET os_type = 'Windows', agent_version = '2.1.8' WHERE id = $1`, oldHost.ID)
	exec(`UPDATE hosts SET os_type = 'Windows', agent_version = '2.1.9' WHERE id = $1`, newHost.ID)
	tag := "storeapp-" + uuid.New().String()[:8]
	pkg := uuid.New().String()
	exec(`INSERT INTO packages (id, name, latest_version, updated_at) VALUES ($1, $2, '1.24', NOW())`, pkg, tag)
	exec(`INSERT INTO host_packages (id, host_id, package_id, current_version, available_version, needs_update, is_security_update, winget_id, winget_store_app) VALUES
		($1, $2, $3, '1.18', '1.24', true, false, 'Microsoft.WindowsTerminal', true),
		($4, $5, $3, '1.18', '1.24', true, false, 'Microsoft.WindowsTerminal', true)`,
		uuid.New().String(), oldHost.ID, pkg, uuid.New().String(), newHost.ID)
	t.Cleanup(func() {
		_, _ = d.Exec(context.Background(), `DELETE FROM packages WHERE id = $1`, pkg)
		_, _ = d.Exec(context.Background(), `DELETE FROM hosts WHERE id IN ($1, $2)`, oldHost.ID, newHost.ID)
	})
	exec(`REFRESH MATERIALIZED VIEW mv_package_stats`)
	s := NewPackagesStore(d)
	check := func(where, hostID string, patchable bool, reason string) {
		t.Helper()
		switch {
		case hostID == oldHost.ID && (patchable || reason != NotPatchableStoreApp):
			t.Errorf("%s: old agent got patchable=%v reason=%q, want store_app", where, patchable, reason)
		case hostID == newHost.ID && (!patchable || reason != ""):
			t.Errorf("%s: capable agent got patchable=%v reason=%q, want patchable", where, patchable, reason)
		}
	}

	detail, err := s.GetByID(ctx, pkg)
	if err != nil || detail == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(detail.HostPackages) != 2 {
		t.Fatalf("GetByID rows = %d", len(detail.HostPackages))
	}
	for _, hp := range detail.HostPackages {
		check("GetByID", hp.HostID, hp.Patchable, hp.NotPatchableReason)
	}

	entries, _, err := s.GetHosts(ctx, pkg, GetHostsParams{Page: 1, Limit: 10})
	if err != nil || len(entries) != 2 {
		t.Fatalf("GetHosts: %d rows, %v", len(entries), err)
	}
	for _, e := range entries {
		check("GetHosts", e.HostID, e.Patchable, e.NotPatchableReason)
	}

	pkgs, _, _, err := s.List(ctx, ListParams{Search: tag})
	if err != nil || len(pkgs) != 1 || len(pkgs[0].PackageHosts) != 2 {
		t.Fatalf("List: %+v, %v", pkgs, err)
	}
	for _, ref := range pkgs[0].PackageHosts {
		check("List", ref.HostID, ref.Patchable, ref.NotPatchableReason)
	}
}
