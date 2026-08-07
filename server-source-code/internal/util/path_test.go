package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafePathUnderBase_Accepts(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "patchmon-agent-linux-arm64"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := SafePathUnderBase(base, "patchmon-agent-linux-arm64")
	if err != nil {
		t.Fatalf("SafePathUnderBase: %v", err)
	}

	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if want := filepath.Join(resolvedBase, "patchmon-agent-linux-arm64"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("got %q, want an absolute path", got)
	}
}

func TestSafePathUnderBase_Rejects(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "agent"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// A real file outside the base, so rejection cannot be mistaken for "not found".
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	// A real file in a subdirectory of the base. Containment alone would accept
	// it, so only the single-component rule can reject it.
	if err := os.Mkdir(filepath.Join(base, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "sub", "agent"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write nested fixture: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"current dir", "."},
		{"parent dir", ".."},
		{"traversal", "../secret"},
		{"nested traversal", "agent/../../secret"},
		{"backslash traversal", `..\secret`},
		{"absolute path", filepath.Join(outside, "secret")},
		{"absolute unix path", "/etc/passwd"},
		{"forward separator to a real nested file", "sub/agent"},
		{"backslash separator", `sub\agent`},
		{"nested traversal back into base", "sub/../agent"},
		{"dot dot prefix", "..agentrc"},
		{"trailing traversal", "agent/.."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePathUnderBase(base, tt.input)
			if err == nil {
				t.Errorf("SafePathUnderBase(base, %q) = %q, want an error", tt.input, got)
			}
			if got != "" {
				t.Errorf("SafePathUnderBase(base, %q) returned %q alongside an error", tt.input, got)
			}
		})
	}
}

// The separator ban rejects traversal before resolution, but symlinks are the
// case that ban cannot see: a single valid-looking component whose target is
// outside the base.
func TestSafePathUnderBase_SymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(base, "agent")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if got, err := SafePathUnderBase(base, "agent"); err == nil {
		t.Errorf("SafePathUnderBase resolved a symlink out of the base to %q, want an error", got)
	}
}

func TestSafePathUnderBase_SymlinkWithinBaseIsAllowed(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "real-agent")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(base, "agent")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := SafePathUnderBase(base, "agent")
	if err != nil {
		t.Fatalf("SafePathUnderBase: %v", err)
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if want := filepath.Join(resolvedBase, "real-agent"); got != want {
		t.Errorf("got %q, want the resolved target %q", got, want)
	}
}

// Resolution requires the file to exist, so a missing name is an error rather
// than a path the caller might then create.
func TestSafePathUnderBase_MissingFile(t *testing.T) {
	if got, err := SafePathUnderBase(t.TempDir(), "not-there"); err == nil {
		t.Errorf("SafePathUnderBase = %q, want an error for a missing file", got)
	}
}

func TestSafePathUnderBase_MissingBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "no-such-dir")
	if got, err := SafePathUnderBase(base, "agent"); err == nil {
		t.Errorf("SafePathUnderBase = %q, want an error for a missing base", got)
	}
}
