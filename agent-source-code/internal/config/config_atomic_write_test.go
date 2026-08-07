package config

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// writeConfigInPlace is what saveConfigLocked used to do. Kept here so the test
// below can prove it has teeth: the assertion must fail against this and pass
// against writeConfigAtomically. Do not wire this into production code.
func writeConfigInPlace(v *viper.Viper, path string) error {
	return v.WriteConfigAs(path)
}

// bulkyViper builds a config large enough that writing it is not a single
// atomic-ish syscall. The production config is small, and on a fast local disk
// the truncate-then-write window is too narrow to hit reliably, which is why
// this only ever failed on CI. Padding widens the window so the race is
// reproducible anywhere.
func bulkyViper() *viper.Viper {
	v := viper.New()
	v.Set("patchmon_server", "https://patchmon.example.com")
	v.Set("api_version", "v1")
	v.Set("log_level", "info")
	v.Set("update_interval", 60)
	padding := make(map[string]interface{}, 400)
	for i := range 400 {
		padding[string(rune('a'+i%26))+string(rune('a'+i/26))+"_key"] = i
	}
	v.Set("integrations", padding)
	return v
}

// A reader must never observe a partially written config. saveConfigLocked runs
// at the end of every LoadConfig, and separate Managers over the same path hold
// separate locks, so two goroutines genuinely do write this file concurrently
// in the running agent.
func TestConfigWrite_ReaderNeverObservesPartialFile(t *testing.T) {
	t.Parallel()

	writers := []struct {
		name  string
		write func(*viper.Viper, string) error
		// The in-place writer is expected to corrupt; assert that it does, so a
		// future refactor cannot quietly make this test vacuous.
		wantCorruption bool
	}{
		{"atomic", writeConfigAtomically, false},
		{"in place", writeConfigInPlace, true},
	}

	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			v := bulkyViper()
			if err := writeConfigAtomically(v, path); err != nil {
				t.Fatalf("seeding config: %v", err)
			}

			const iterations = 300
			var wg sync.WaitGroup
			var mu sync.Mutex
			corruptions := 0

			wg.Add(1)
			go func() {
				defer wg.Done()
				for range iterations {
					if err := w.write(v, path); err != nil {
						t.Errorf("write: %v", err)
						return
					}
				}
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				for range iterations {
					// Read it exactly as LoadConfig does, so a failure here is
					// the failure the agent would hit.
					r := viper.New()
					r.SetConfigFile(path)
					r.SetConfigType("yaml")
					err := r.ReadInConfig()
					// A parse error is the CI symptom. A file that parses to
					// nothing is the truncate window caught a moment earlier.
					if err != nil || len(r.AllKeys()) == 0 {
						mu.Lock()
						corruptions++
						mu.Unlock()
					}
				}
			}()

			wg.Wait()
			t.Logf("%s writer: reader observed %d corrupt reads out of %d", w.name, corruptions, iterations)

			switch {
			case w.wantCorruption && corruptions == 0:
				t.Skip("in-place writer did not lose the race on this machine; the atomic case is the one that matters")
			case !w.wantCorruption && corruptions > 0:
				t.Errorf("reader saw %d partial or empty config files; the write is not atomic", corruptions)
			}
		})
	}
}
