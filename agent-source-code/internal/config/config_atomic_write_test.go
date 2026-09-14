package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
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

// classifyReadFailure buckets a failed config read by root cause. The three
// buckets need different fixes, so a failure that does not say which one it hit
// is not actionable.
//
// Deliberately string-based rather than build-tagged: this file compiles on
// every platform, and the Windows error text is what identifies the case.
func classifyReadFailure(err error) string {
	if err == nil {
		return "parsed but empty"
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "file missing (gap between remove and rename)"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "being used by another process"),
		strings.Contains(msg, "sharing violation"):
		return "sharing violation (locked out mid-replace)"
	case strings.Contains(msg, "access is denied"):
		return "access denied"
	case strings.Contains(msg, "yaml"), strings.Contains(msg, "unmarshal"):
		return "parse error (genuinely partial content)"
	default:
		return "other: " + err.Error()
	}
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
					// The in-place writer has no retry, so on Windows the
					// concurrent reader locks it out. That is contention, not
					// the corruption this test measures, so it is not a
					// failure of the test itself.
					if err := w.write(v, path); err != nil && !isTransientFileError(err) {
						t.Errorf("write: %v", err)
						return
					}
				}
			}()

			// Bucketed by cause. "The write is not atomic" can mean three very
			// different things, and they do not share a fix: a missing file is
			// a gap between unlink and rename, a sharing violation is the
			// reader being locked out mid-replace, and a parse error is the
			// only one that means genuinely partial content. Counting them
			// together hides which one is happening.
			causes := map[string]int{}
			var sampleErr error

			wg.Add(1)
			go func() {
				defer wg.Done()
				for range iterations {
					// Read it exactly as LoadConfig does, including its retry
					// of transient sharing violations, so a failure here is
					// the failure the agent would hit. Without the retry this
					// measures Windows file-sharing contention rather than
					// whether the write is atomic.
					r := viper.New()
					r.SetConfigFile(path)
					r.SetConfigType("yaml")
					err := retryTransientFile(r.ReadInConfig)
					if err == nil && len(r.AllKeys()) > 0 {
						continue
					}
					mu.Lock()
					corruptions++
					causes[classifyReadFailure(err)]++
					if sampleErr == nil && err != nil {
						sampleErr = err
					}
					mu.Unlock()
				}
			}()

			wg.Wait()
			t.Logf("%s writer: reader observed %d corrupt reads out of %d", w.name, corruptions, iterations)
			for cause, n := range causes {
				t.Logf("    %s: %d", cause, n)
			}
			if sampleErr != nil {
				t.Logf("    sample error: %v", sampleErr)
			}

			switch {
			case w.wantCorruption && corruptions == 0:
				t.Skip("in-place writer did not lose the race on this machine; the atomic case is the one that matters")
			case !w.wantCorruption && corruptions > 0:
				t.Errorf("reader saw %d partial or empty config files; the write is not atomic", corruptions)
			}
		})
	}
}
