package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"patchmon-agent/internal/pkgversion"
	"patchmon-agent/pkg/models"

	"github.com/spf13/viper"
)

const testHostID = "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41"

func TestMigrateConfig(t *testing.T) {
	steps := func(ran *[]string) []configMigration {
		rec := func(v string) func(*models.Config) bool {
			return func(*models.Config) bool { *ran = append(*ran, v); return false }
		}
		return []configMigration{{"2.1.4", rec("2.1.4")}, {"2.2.0", rec("2.2.0")}}
	}

	cases := []struct {
		name          string
		configVersion string
		binary        string
		want          []string
	}{
		{"unstamped pre-2.1.4 config runs every step", "", "2.2.0", []string{"2.1.4", "2.2.0"}},
		{"garbage version ranks lowest", "unknown", "2.2.0", []string{"2.1.4", "2.2.0"}},
		{"2.1.3 config upgraded to 2.1.4", "2.1.3", "2.1.4", []string{"2.1.4", "2.2.0"}},
		{"rc config reruns its release step", "2.1.4-rc.1", "2.1.4", []string{"2.1.4", "2.2.0"}},
		{"rc patch build reruns too", "2.1.4-rc.1.4", "2.1.4-rc.2", []string{"2.1.4", "2.2.0"}},
		{"current config runs nothing", "2.1.4", "2.1.4", nil},
		{"already-applied step does not rerun", "2.1.4", "2.2.0", []string{"2.2.0"}},
		{"downgrade runs nothing and restamps", "2.2.0", "2.1.4", nil},
		{"later rc config skips the 2.1.4 step", "2.1.5-rc.1", "2.1.4", []string{"2.2.0"}},
		{"later rc config upgraded runs only newer steps", "2.1.5-rc.1", "2.2.0", []string{"2.2.0"}},
		{"dev build still applies the steps it carries", "", "0.0.0", []string{"2.1.4", "2.2.0"}},
		{"dev build with a git suffix", "2.1.3", "2.1.3-main-93c589a4", []string{"2.1.4", "2.2.0"}},
		{"dev hop after a final release runs nothing", "2.2.0", "2.1.3-main-93c589a4", nil},
		{"0.0.0 config is lowest", "0.0.0", "2.1.4", []string{"2.1.4", "2.2.0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ran []string
			cfg := &models.Config{ConfigVersion: c.configVersion}
			migrateConfig(cfg, c.binary, steps(&ran))
			if strings.Join(ran, ",") != strings.Join(c.want, ",") {
				t.Errorf("steps run = %v, want %v", ran, c.want)
			}
			if cfg.ConfigVersion != c.binary {
				t.Errorf("stamped %q, want %q", cfg.ConfigVersion, c.binary)
			}

			ran = nil
			migrateConfig(cfg, c.binary, steps(&ran))
			if len(ran) != 0 {
				t.Errorf("second pass ran %v, want nothing", ran)
			}
		})
	}
}

func TestMigrate214(t *testing.T) {
	for in, want := range map[string]string{
		testHostID:              testHostID,
		"  " + testHostID + " ": testHostID,
		"":                      "",
		"not a host id":         "",
		"../../etc":             "",
		strings.Repeat("a", 65): "",
	} {
		c := &models.Config{HostID: in}
		migrate214(c)
		if c.HostID != want {
			t.Errorf("migrate214(%q) host_id = %q, want %q", in, c.HostID, want)
		}
		again := *c
		if migrate214(&again) || again.HostID != c.HostID {
			t.Errorf("migrate214 is not idempotent for %q", in)
		}
	}
}

func newTempManager(t *testing.T, content string) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return m, cfgPath
}

func readKeys(t *testing.T, path string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return v
}

func TestLoadConfig_StampsAndKeepsSettings(t *testing.T) {
	old := "# PatchMon Agent Configuration\n# Generated on Mon\n" + testConfigYAML
	m, cfgPath := newTempManager(t, old)

	v := readKeys(t, cfgPath)
	if got := v.GetString("config_version"); got != pkgversion.Version {
		t.Fatalf("config_version = %q, want %q", got, pkgversion.Version)
	}
	if v.IsSet("host_id") {
		t.Fatal("host_id written although none is known")
	}
	if got := v.GetString("patchmon_server"); got != "https://patchmon.example.com" {
		t.Fatalf("patchmon_server = %q, settings were not preserved", got)
	}
	if m.GetConfig().ConfigVersion != pkgversion.Version {
		t.Fatalf("in-memory config_version = %q", m.GetConfig().ConfigVersion)
	}

	first, _ := os.ReadFile(cfgPath)
	for i := 0; i < 3; i++ {
		if err := m.LoadConfig(); err != nil {
			t.Fatal(err)
		}
	}
	again, _ := os.ReadFile(cfgPath)
	if string(first) != string(again) {
		t.Fatalf("repeated loads changed the file:\n%s\n---\n%s", first, again)
	}
}

func TestLoadConfig_KeepsAndCleansHostID(t *testing.T) {
	_, cfgPath := newTempManager(t, testConfigYAML+"host_id: \" "+testHostID+" \"\nconfig_version: 2.1.4-rc.1\n")
	v := readKeys(t, cfgPath)
	if got := v.GetString("host_id"); got != testHostID {
		t.Fatalf("host_id = %q, want %q", got, testHostID)
	}

	_, cfgPath = newTempManager(t, testConfigYAML+"host_id: \"bad id!\"\n")
	if readKeys(t, cfgPath).IsSet("host_id") {
		t.Fatal("an invalid host_id survived the 2.1.4 migration")
	}
}

func TestLoadConfig_HostIDNotCarriedFromAPreviousLoad(t *testing.T) {
	m, cfgPath := newTempManager(t, testConfigYAML+"host_id: "+testHostID+"\n")
	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := m.GetHostID(); got != "" {
		t.Fatalf("host_id = %q after loading a file without one", got)
	}
}

func TestAdoptHostID(t *testing.T) {
	const other = "11111111-2222-3333-4444-555555555555"
	cases := []struct {
		name     string
		existing string
		server   string
		want     HostIDResult
		wantID   string
	}{
		{"empty config adopts the server's ID", "", testHostID, HostIDAdopted, testHostID},
		{"same ID is a match", testHostID, testHostID, HostIDMatch, testHostID},
		{"different ID is kept", other, testHostID, HostIDConflict, other},
		{"old server sends nothing", other, "", HostIDIgnored, other},
		{"invalid server ID is ignored", "", "bad id", HostIDIgnored, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			content := testConfigYAML
			if c.existing != "" {
				content += "host_id: " + c.existing + "\n"
			}
			m, cfgPath := newTempManager(t, content)
			got, err := m.AdoptHostID(c.server)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("result = %v, want %v", got, c.want)
			}
			if m.GetHostID() != c.wantID {
				t.Fatalf("in-memory host_id = %q, want %q", m.GetHostID(), c.wantID)
			}
			if onDisk := readKeys(t, cfgPath).GetString("host_id"); onDisk != c.wantID {
				t.Fatalf("on-disk host_id = %q, want %q", onDisk, c.wantID)
			}
		})
	}
}

func TestSetAndClearHostID(t *testing.T) {
	m, cfgPath := newTempManager(t, testConfigYAML+"host_id: 11111111-2222-3333-4444-555555555555\n")
	snapshot := m.GetConfig()
	if err := m.SetHostID(testHostID); err != nil {
		t.Fatal(err)
	}
	if snapshot.HostID == testHostID {
		t.Fatal("SetHostID mutated a snapshot returned by GetConfig")
	}
	if got := readKeys(t, cfgPath).GetString("host_id"); got != testHostID {
		t.Fatalf("host_id = %q", got)
	}
	if err := m.SetHostID("bad id"); err == nil {
		t.Fatal("SetHostID accepted an invalid ID")
	}
	if err := m.ClearHostID(); err != nil {
		t.Fatal(err)
	}
	if readKeys(t, cfgPath).IsSet("host_id") {
		t.Fatal("host_id still on disk after ClearHostID")
	}
}

func TestSaveWithoutLoadStampsConfigVersion(t *testing.T) {
	m := New()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.SetPatchmonServer("https://pm.example"); err != nil {
		t.Fatal(err)
	}
	if got := readKeys(t, cfgPath).GetString("config_version"); got != pkgversion.Version {
		t.Fatalf("config_version = %q, want %q", got, pkgversion.Version)
	}
}

func TestLoadConfig_UnsavedStampIsNotReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores Unix directory permissions, so the save cannot be made to fail this way")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(testConfigYAML+"config_version: 2.1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	m := New()
	m.SetConfigFile(cfgPath)
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")
	if err := m.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := m.GetConfig().ConfigVersion; got != "2.1.4" {
		t.Fatalf("config_version = %q, want the on-disk 2.1.4 when the stamp could not be saved", got)
	}
}
