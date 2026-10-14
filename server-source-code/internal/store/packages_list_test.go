package store

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestListPackagesSQLHostScopedSource(t *testing.T) {
	host := "h1"
	scoped, _ := listPackagesSQL(packagesListFilters{HostID: &host}, "status", "asc", 50, 0)
	if strings.Contains(scoped, "mv_package_stats") {
		t.Fatal("host-filtered list must not read fleet-wide mv_package_stats")
	}
	if !strings.Contains(scoped, "JOIN host_packages hph") {
		t.Fatal("host-filtered list must read the host's own host_packages row")
	}
	countSQL, _ := countPackagesSQL(packagesListFilters{HostID: &host})
	if strings.Contains(countSQL, "mv_package_stats") {
		t.Fatal("host-filtered count must not read mv_package_stats")
	}

	fleet, _ := listPackagesSQL(packagesListFilters{}, "status", "asc", 50, 0)
	if !strings.Contains(fleet, "LEFT JOIN mv_package_stats") || strings.Contains(fleet, "host_packages") {
		t.Fatal("unfiltered list must stay on mv_package_stats with no host_packages scan")
	}

	sec := "false"
	where, _ := packagesListWhereSQL(packagesListFilters{HostID: &host, IsSecurityUpdate: &sec}, "p")
	if strings.Contains(where, "EXISTS") {
		t.Fatal("host-filtered predicates must apply to the joined host row, not an EXISTS over every host")
	}
}

func TestListPackagesHostScopedStatus(t *testing.T) {
	d, hosts := identityTestDB(t)
	ctx := context.Background()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	a := pendingHost(t, hosts)
	b := pendingHost(t, hosts)
	tag := "pkgscope-" + uuid.New().String()[:8]
	pkgX, pkgY, pkgZ := uuid.New().String(), uuid.New().String(), uuid.New().String()
	exec(`INSERT INTO packages (id, name, latest_version, updated_at) VALUES ($1, $2, '2.0', NOW()), ($3, $4, '5.0', NOW()), ($5, $6, '9.0', NOW())`,
		pkgX, tag+"-x", pkgY, tag+"-y", pkgZ, tag+"-z")
	exec(`INSERT INTO host_packages (id, host_id, package_id, current_version, available_version, needs_update, is_security_update) VALUES
		($1, $2, $3, '1.0', '2.0', true, true),
		($4, $5, $3, '2.0', NULL, false, false),
		($6, $2, $7, '5.0', NULL, false, false),
		($8, $5, $7, '4.0', '5.0', true, false),
		($9, $2, $10, '8.0', '9.0', true, true),
		($11, $5, $10, '8.0', '8.5', true, false)`,
		uuid.New().String(), a.ID, pkgX, uuid.New().String(), b.ID, uuid.New().String(), pkgY, uuid.New().String(),
		uuid.New().String(), pkgZ, uuid.New().String())
	t.Cleanup(func() {
		_, _ = d.Exec(context.Background(), `DELETE FROM packages WHERE id IN ($1, $2, $3)`, pkgX, pkgY, pkgZ)
		_, _ = d.Exec(context.Background(), `DELETE FROM hosts WHERE id IN ($1, $2)`, a.ID, b.ID)
	})
	exec(`REFRESH MATERIALIZED VIEW mv_package_stats`)

	store := NewPackagesStore(d)
	list := func(host, sort, order, sec string) (map[string]PackageWithStats, []string) {
		t.Helper()
		pkgs, total, _, err := store.List(ctx, ListParams{Search: tag, Host: host, Sort: sort, Order: order, IsSecurityUpdate: sec})
		if err != nil {
			t.Fatal(err)
		}
		if total != len(pkgs) {
			t.Fatalf("count %d disagrees with %d rows", total, len(pkgs))
		}
		m := map[string]PackageWithStats{}
		var names []string
		for _, p := range pkgs {
			m[strings.TrimPrefix(p.Name, tag+"-")] = p
			names = append(names, strings.TrimPrefix(p.Name, tag+"-"))
		}
		return m, names
	}
	want := func(p PackageWithStats, installs, updates, security int, latest string) {
		t.Helper()
		if p.Stats.TotalInstalls != installs || p.Stats.UpdatesNeeded != updates || p.Stats.SecurityUpdates != security {
			t.Fatalf("%s stats = %+v, want %d/%d/%d", p.Name, p.Stats, installs, updates, security)
		}
		if p.LatestVersion == nil || *p.LatestVersion != latest {
			t.Fatalf("%s latest_version = %v, want %s", p.Name, p.LatestVersion, latest)
		}
	}

	fleet, _ := list("", "", "", "")
	want(fleet["x"], 2, 1, 1, "2.0")
	want(fleet["y"], 2, 1, 0, "5.0")
	want(fleet["z"], 2, 2, 1, "9.0")

	onB, order := list(b.ID, "status", "asc", "")
	want(onB["x"], 1, 0, 0, "2.0")
	want(onB["y"], 1, 1, 0, "5.0")
	want(onB["z"], 1, 1, 0, "8.5")
	if strings.Join(order, ",") != "y,z,x" {
		t.Fatalf("status sort on host B = %v, want y,z (update) before x (up to date)", order)
	}

	onA, order := list(a.ID, "status", "asc", "")
	want(onA["x"], 1, 1, 1, "2.0")
	want(onA["y"], 1, 0, 0, "5.0")
	want(onA["z"], 1, 1, 1, "9.0")
	if strings.Join(order, ",") != "x,z,y" {
		t.Fatalf("status sort on host A = %v, want x,z (security) before y", order)
	}

	if _, order := list("", "status", "asc", ""); strings.Join(order, ",") != "x,z,y" {
		t.Fatalf("fleet status sort = %v, want x,z (security) before y", order)
	}

	if _, names := list(b.ID, "", "", "false"); strings.Join(names, ",") != "y,z" {
		t.Fatalf("regular updates on host B = %v, want y,z", names)
	}

	if _, names := list("", "", "", "false"); strings.Join(names, ",") != "y" {
		t.Fatalf("fleet regular updates = %v, want y (x and z have a security update somewhere)", names)
	}
}
