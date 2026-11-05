package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestSubtractPackageNames(t *testing.T) {
	keep, held := subtractPackageNames([]string{"nginx", "openssl", "curl"}, []string{"nginx", "bash"})
	if len(keep) != 2 || keep[0] != "openssl" || keep[1] != "curl" {
		t.Fatalf("keep = %v", keep)
	}
	if len(held) != 1 || held[0] != "nginx" {
		t.Fatalf("held = %v", held)
	}
}

func TestExcludeAgentRefusal(t *testing.T) {
	ver := func(v string) *models.Host {
		return &models.Host{AgentVersion: &v}
	}
	if excludeAgentRefusal(ver("2.1.7")) == "" {
		t.Fatal("2.1.7 must be refused")
	}
	if excludeAgentRefusal(&models.Host{}) == "" {
		t.Fatal("missing version must be refused")
	}
	if excludeAgentRefusal(ver("2.2.0")) != "" {
		t.Fatal("2.2.0 must be accepted")
	}
	if excludeAgentRefusal(ver("v2.2.0-rc.1")) != "" {
		t.Fatal("2.2.0 pre-release must be accepted")
	}
}
