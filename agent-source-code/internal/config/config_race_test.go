package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const testConfigYAML = `patchmon_server: https://patchmon.example.com
api_version: v1
log_level: info
update_interval: 60
package_cache_refresh_mode: always
package_cache_refresh_max_age: 60
integrations:
  compliance:
    enabled: on-demand
    openscap_enabled: true
    docker_bench_enabled: false
    scan_interval: 1440
  docker: true
  ssh-proxy-enabled: false
`

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	m := New()
	m.SetConfigFile(cfgPath)
	// Point the derived paths at the temp dir so SaveConfig's setupDirectories
	// does not try to create /etc/patchmon.
	cfg := m.GetConfig()
	cfg.CredentialsFile = filepath.Join(dir, "credentials.yml")
	cfg.LogFile = filepath.Join(dir, "agent.log")

	if err := m.LoadConfig(); err != nil {
		t.Fatalf("initial LoadConfig: %v", err)
	}
	return m
}

// TestManager_ConcurrentLoadAndRead reproduces the crash shape.
func TestManager_ConcurrentLoadAndRead(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)

	const iterations = 60
	var wg sync.WaitGroup

	// Writer: repeated LoadConfig, exactly as sendIntegrationData and
	// runScheduledComplianceScan both do.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := m.LoadConfig(); err != nil {
				t.Errorf("LoadConfig: %v", err)
				return
			}
		}
	}()

	// A second Manager over the same file. getLatestBinaryFromServer builds one
	// of these, which is why a per-Manager lock alone is not enough and
	// LoadConfig had to stop using the package-level viper singleton.
	wg.Add(1)
	go func() {
		defer wg.Done()
		other := New()
		other.SetConfigFile(m.GetConfigFile())
		otherCfg := other.GetConfig()
		otherCfg.CredentialsFile = m.GetConfig().CredentialsFile
		otherCfg.LogFile = m.GetConfig().LogFile
		for i := 0; i < iterations; i++ {
			if err := other.LoadConfig(); err != nil {
				t.Errorf("second manager LoadConfig: %v", err)
				return
			}
		}
	}()

	// Readers, one per real caller site.
	readers := []func(){
		func() { _ = m.IsIntegrationEnabled("docker") },
		func() { _ = m.IsIntegrationEnabled("compliance") },
		func() { _ = m.GetComplianceMode() },
		func() { _ = m.IsComplianceOnDemandOnly() },
		func() { _ = m.GetComplianceOpenscapEnabled() },
		func() { _ = m.GetComplianceDockerBenchEnabled() },
		func() { _ = m.GetComplianceScanInterval() },
		func() { _ = m.GetPackageCacheRefreshMode() },
		func() { _ = m.GetPackageCacheRefreshMaxAge() },
	}
	for _, read := range readers {
		wg.Add(1)
		go func(fn func()) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				fn()
			}
		}(read)
	}

	wg.Wait()
}

// TestManager_ConcurrentSettersAndReaders covers the write path: the setters
// mutate the same nested compliance map the getters read.
func TestManager_ConcurrentSettersAndReaders(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)

	const iterations = 40
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := m.SetComplianceScanners(i%2 == 0, i%3 == 0); err != nil {
				t.Errorf("SetComplianceScanners: %v", err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			mode := ComplianceOnDemand
			if i%2 == 0 {
				mode = ComplianceEnabled
			}
			if err := m.SetComplianceMode(mode); err != nil {
				t.Errorf("SetComplianceMode: %v", err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := m.SetIntegrationEnabled("docker", i%2 == 0); err != nil {
				t.Errorf("SetIntegrationEnabled: %v", err)
				return
			}
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = m.GetComplianceMode()
				_ = m.IsIntegrationEnabled("docker")
				_ = m.GetComplianceOpenscapEnabled()
				_ = m.GetComplianceScanInterval()
			}
		}()
	}

	wg.Wait()
}

// TestLoadConfig_SwapsRatherThanMutates documents why GetConfig can hand out
// a pointer safely: LoadConfig builds a new struct and swaps it in, so a
// caller holding an earlier pointer keeps a stable snapshot instead of
// watching the map be rebuilt underneath it.
func TestLoadConfig_SwapsRatherThanMutates(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)

	before := m.GetConfig()
	if err := m.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	after := m.GetConfig()

	if before == after {
		t.Error("LoadConfig must swap in a new config struct, not mutate the live one in place")
	}
}
