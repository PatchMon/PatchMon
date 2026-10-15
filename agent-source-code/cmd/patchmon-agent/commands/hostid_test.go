package commands

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/config"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

const (
	serverHostID = "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41"
	otherHostID  = "11111111-2222-3333-4444-555555555555"
)

func newHostIDFixture(t *testing.T, hostIDLine string, serverResp map[string]interface{}) (*config.Manager, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.URL.Path != "/api/v1/hosts/ping" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serverResp)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	content := "patchmon_server: " + srv.URL + "\napi_version: v1\n" + hostIDLine
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte("api_id: patchmon_old\napi_key: oldkey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := config.New()
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	prevCfg, prevLogger := cfgManager, logger
	cfgManager = m
	logger = logrus.New()
	logger.SetOutput(io.Discard)
	hostIDConflictOnce = sync.Once{}
	t.Cleanup(func() { cfgManager, logger = prevCfg, prevLogger })
	return m, srv.URL
}

func TestReconcileHostID(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		server   string
		want     string
	}{
		{"learns the server's ID", "", serverHostID, serverHostID},
		{"keeps a matching ID", serverHostID, serverHostID, serverHostID},
		{"never overwrites a different ID", otherHostID, serverHostID, otherHostID},
		{"old server sends nothing", otherHostID, "", otherHostID},
		{"invalid server value is ignored", "", "not a host id", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line := ""
			if c.existing != "" {
				line = "host_id: " + c.existing + "\n"
			}
			m, _ := newHostIDFixture(t, line, nil)
			reconcileHostID(c.server)
			if got := m.GetHostID(); got != c.want {
				t.Fatalf("host_id = %q, want %q", got, c.want)
			}
		})
	}
}

func TestConfigureCredsHostID(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		apiID    string
		flag     string
		want     string
	}{
		{"--host-id is written", otherHostID, "patchmon_old", serverHostID, serverHostID},
		{"same API ID keeps the host ID", otherHostID, "patchmon_old", "", otherHostID},
		{"new API ID clears then relearns from the server", otherHostID, "patchmon_new", "", serverHostID},
		{"empty host ID is learned", "", "patchmon_new", "", serverHostID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line := ""
			if c.existing != "" {
				line = "host_id: " + c.existing + "\n"
			}
			m, url := newHostIDFixture(t, line, map[string]interface{}{"message": "Ping successful", "hostId": serverHostID})
			if err := configureCreds(c.apiID, "newkey", url, c.flag); err != nil {
				t.Fatal(err)
			}
			if got := m.GetHostID(); got != c.want {
				t.Fatalf("host_id = %q, want %q", got, c.want)
			}
		})
	}

	t.Run("an invalid --host-id is refused before anything is written", func(t *testing.T) {
		m, url := newHostIDFixture(t, "host_id: "+otherHostID+"\n", map[string]interface{}{"hostId": serverHostID})
		if err := configureCreds("patchmon_new", "newkey", url, "bad id"); err == nil {
			t.Fatal("expected an error")
		}
		if got := m.GetHostID(); got != otherHostID {
			t.Fatalf("host_id = %q, want it untouched", got)
		}
	})
}

func TestSyncHostID(t *testing.T) {
	t.Run("adopts a different server ID", func(t *testing.T) {
		m, _ := newHostIDFixture(t, "host_id: "+otherHostID+"\n", map[string]interface{}{"hostId": serverHostID})
		if err := syncHostID(); err != nil {
			t.Fatal(err)
		}
		if got := m.GetHostID(); got != serverHostID {
			t.Fatalf("host_id = %q, want %q", got, serverHostID)
		}
	})
	t.Run("old server is an error and changes nothing", func(t *testing.T) {
		m, _ := newHostIDFixture(t, "host_id: "+otherHostID+"\n", map[string]interface{}{"message": "Ping successful"})
		if err := syncHostID(); err == nil {
			t.Fatal("expected an error from a server that sends no hostId")
		}
		if got := m.GetHostID(); got != otherHostID {
			t.Fatalf("host_id = %q, want it untouched", got)
		}
	})
}

func TestPartialReportCarriesIdentityFields(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"message":"ok","hostId":"` + serverHostID + `"}`))
	}))
	defer srv.Close()

	m, _ := newHostIDFixture(t, "", nil)
	m.GetConfig().PatchmonServer = srv.URL
	if err := m.LoadCredentials(); err != nil {
		t.Fatal(err)
	}
	full := &models.ReportPayload{
		AgentVersion:  "2.1.4",
		MachineID:     "abcd",
		HostID:        serverHostID,
		ConfigVersion: "2.1.4",
		Hostname:      "web-01",
	}
	if err := sendPartialReport(context.Background(), client.New(m, logger), full, []string{models.SectionHostname}); err != nil {
		t.Fatal(err)
	}
	if got["hostId"] != serverHostID || got["configVersion"] != "2.1.4" || got["machineId"] != "abcd" {
		t.Fatalf("partial payload identity = hostId %v configVersion %v machineId %v", got["hostId"], got["configVersion"], got["machineId"])
	}
}

func TestOldServerPayloadsStillDecode(t *testing.T) {
	var ping models.PingResponse
	if err := json.Unmarshal([]byte(`{"message":"Ping successful","hashGate":true}`), &ping); err != nil || ping.HostID != "" {
		t.Fatalf("pre-2.1.4 ping response: %v %q", err, ping.HostID)
	}
	var upd models.UpdateResponse
	if err := json.Unmarshal([]byte(`{"message":"ok","packagesProcessed":1,"hostId":"`+serverHostID+`"}`), &upd); err != nil || upd.HostID != serverHostID {
		t.Fatalf("update response: %v %q", err, upd.HostID)
	}
	b, _ := json.Marshal(models.PingRequest{AgentVersion: "2.1.4"})
	for _, k := range []string{"hostId", "configVersion", "machineId"} {
		if strings.Contains(string(b), k) {
			t.Fatalf("empty %s must be omitted from the ping body: %s", k, b)
		}
	}
}
