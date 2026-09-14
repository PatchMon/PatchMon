package notifications

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestScopePackagesToHostGroup(t *testing.T) {
	pkgs := []map[string]interface{}{
		{
			"name":               "openssl",
			"affectedHostsCount": 2,
			"affectedHosts": []map[string]interface{}{
				{"hostId": "h1"},
				{"hostId": "h2"},
			},
		},
		{
			"name":               "curl",
			"affectedHostsCount": 1,
			"affectedHosts": []map[string]interface{}{
				{"hostId": "h3"},
			},
		},
	}
	groupHostIDs := map[string]struct{}{"h1": {}}

	got := scopePackagesToHostGroup(pkgs, groupHostIDs)

	if len(got) != 1 {
		t.Fatalf("expected 1 package to remain in scope, got %d", len(got))
	}
	if got[0]["name"] != "openssl" {
		t.Fatalf("expected openssl to remain (has a host in scope), got %v", got[0]["name"])
	}
	if got[0]["affectedHostsCount"] != 1 {
		t.Fatalf("expected affectedHostsCount recomputed to 1, got %v", got[0]["affectedHostsCount"])
	}
	hosts, _ := got[0]["affectedHosts"].([]map[string]interface{})
	if len(hosts) != 1 || hosts[0]["hostId"] != "h1" {
		t.Fatalf("expected affectedHosts filtered down to just h1, got %v", hosts)
	}
}

func TestScopePackagesToHostGroup_NoMatchDropsPackage(t *testing.T) {
	pkgs := []map[string]interface{}{
		{
			"name":               "curl",
			"affectedHostsCount": 1,
			"affectedHosts":      []map[string]interface{}{{"hostId": "h3"}},
		},
	}
	groupHostIDs := map[string]struct{}{"h1": {}}

	got := scopePackagesToHostGroup(pkgs, groupHostIDs)

	if len(got) != 0 {
		t.Fatalf("expected package with no hosts in scope to be dropped, got %d", len(got))
	}
}

func TestAlertHostID(t *testing.T) {
	cases := []struct {
		name     string
		metadata models.JSON
		want     string
	}{
		{"host-specific alert", models.JSON(`{"host_id":"h1","host_name":"web-01"}`), "h1"},
		{"system-wide alert, no host_id", models.JSON(`{"reason":"smtp down"}`), ""},
		{"nil metadata", nil, ""},
		{"empty metadata", models.JSON(``), ""},
		{"malformed json", models.JSON(`{not valid`), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := store.AlertWithDetails{Alert: models.Alert{Metadata: tc.metadata}}
			got := alertHostID(a)
			if got != tc.want {
				t.Fatalf("alertHostID() = %q, want %q", got, tc.want)
			}
		})
	}
}
