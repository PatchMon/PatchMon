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

func TestScopeAlertsToHostGroup(t *testing.T) {
	alert := func(id, typ, meta string) store.AlertWithDetails {
		a := store.AlertWithDetails{}
		a.ID, a.Type = id, typ
		if meta != "" {
			a.Metadata = models.JSON(meta)
		}
		return a
	}
	alerts := []store.AlertWithDetails{
		alert("in-group", "host_down", `{"host_id":"h1"}`),
		alert("other-group", "host_down", `{"host_id":"h9"}`),
		alert("server-release", "server_update", ""),
		alert("agent-release", "agent_update", `{}`),
		alert("login", "user_login_failed", `{"username":"someone"}`),
	}

	got := scopeAlertsToHostGroup(alerts, map[string]struct{}{"h1": {}})
	var ids []string
	for _, a := range got {
		ids = append(ids, a.ID)
	}
	want := []string{"in-group", "server-release", "agent-release"}
	if len(ids) != len(want) {
		t.Fatalf("scopeAlertsToHostGroup() = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("scopeAlertsToHostGroup() = %v, want %v", ids, want)
		}
	}

	if got := scopeAlertsToHostGroup(alerts[:2], map[string]struct{}{}); len(got) != 0 {
		t.Fatalf("empty group kept host alerts: %v", got)
	}
}

func TestScopePackagesToHostGroupRecomputesSecurityFlag(t *testing.T) {
	pkgs := []map[string]interface{}{{
		"name":             "openssl",
		"isSecurityUpdate": true,
		"affectedHosts": []map[string]interface{}{
			{"hostId": "in", "isSecurityUpdate": false},
			{"hostId": "out", "isSecurityUpdate": true},
		},
	}}
	got := scopePackagesToHostGroup(pkgs, map[string]struct{}{"in": {}})
	if len(got) != 1 || got[0]["isSecurityUpdate"] != false {
		t.Fatalf("security flag not recomputed for the scoped hosts: %v", got)
	}
}
