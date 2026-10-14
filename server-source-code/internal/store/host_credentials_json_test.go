package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestHostJSONOmitsAPICredentials(t *testing.T) {
	const apiID = "patchmon_0123456789abcdef"
	const apiKeyHash = "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"
	host := models.Host{ID: "h1", FriendlyName: "web-1", ApiID: apiID, ApiKey: apiKeyHash}

	cases := map[string]any{
		"models.Host":      host,
		"*models.Host":     &host,
		"[]models.Host":    []models.Host{host},
		"HostDockerDetail": HostDockerDetail{Host: host},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			out := string(b)
			if !strings.Contains(out, `"FriendlyName":"web-1"`) {
				t.Fatalf("host not serialised: %s", out)
			}
			for _, banned := range []string{"ApiKey", "ApiID", "api_key", "api_id", "$2a$", apiID} {
				if strings.Contains(out, banned) {
					t.Errorf("response contains %q: %s", banned, out)
				}
			}
		})
	}
}
