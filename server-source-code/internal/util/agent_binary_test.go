package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// The bundled-agent version is parsed out of the binary. If the pre-release
// suffix is dropped here, the version the server advertises stops matching the
// version the agent reports, and the agent self-updates on every check forever.
func TestAgentVersionRe_CapturesPreRelease(t *testing.T) {
	re := regexp.MustCompile(AgentVersionRe)

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"bare release", "2.0.2", "2.0.2"},
		{"prefixed release", "PatchMon Agent v2.0.2", "2.0.2"},
		{"lowercase binary name", "patchmon-agent v2.0.2", "2.0.2"},
		{"version word", "version 2.0.2", "2.0.2"},
		{"cobra default output", "patchmon-agent version 2.0.3-rc.137", "2.0.3-rc.137"},
		{"edge build", "PatchMon Agent v2.0.3-rc.61", "2.0.3-rc.61"},
		{"bare edge build", "2.0.3-rc.61", "2.0.3-rc.61"},
		{"edge build with trailing text", "PatchMon Agent v2.0.3-rc.61 (linux/amd64)", "2.0.3-rc.61"},
		{"edge build with newline", "PatchMon Agent v2.0.3-rc.61\n", "2.0.3-rc.61"},
		{"git describe style", "2.0.2-60-gABC123", "2.0.2-60-gABC123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tt.output)
			if len(m) < 2 {
				t.Fatalf("no match for %q", tt.output)
			}
			if m[1] != tt.want {
				t.Errorf("parsed %q from %q, want %q", m[1], tt.output, tt.want)
			}
		})
	}
}

func TestLdflagsVersionRe(t *testing.T) {
	tests := []struct {
		name    string
		ldflags string
		want    string
	}{
		{
			name:    "release build",
			ldflags: `-s -w -X patchmon-agent/internal/pkgversion.Version=2.0.2`,
			want:    "2.0.2",
		},
		{
			name:    "edge build",
			ldflags: `-s -w -X patchmon-agent/internal/pkgversion.Version=2.0.3-rc.137`,
			want:    "2.0.3-rc.137",
		},
		{
			name:    "v prefix",
			ldflags: `-X patchmon-agent/internal/pkgversion.Version=v2.0.3-rc.137`,
			want:    "2.0.3-rc.137",
		},
		{
			name:    "alongside other -X flags",
			ldflags: `-X main.commit=abc123 -X patchmon-agent/internal/pkgversion.Version=2.1.0 -X main.date=2026-08-04`,
			want:    "2.1.0",
		},
		{
			name:    "unrelated ldflags only",
			ldflags: `-s -w -X main.commit=1.2.3`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ldflagsVersionRe.FindStringSubmatch(tt.ldflags)
			got := ""
			if len(m) >= 2 {
				got = m[1]
			}
			if got != tt.want {
				t.Errorf("parsed %q from %q, want %q", got, tt.ldflags, tt.want)
			}
		})
	}
}

// A server can only execute agent binaries built for its own OS and CPU. Every
// other binary it ships (arm64, arm, FreeBSD, Windows) must still report the
// right version, or the agent-facing version endpoint advertises a bogus
// latestVersion and those agents never auto-update.
func TestGetAgentBinaryInfo_ForeignArchitecture(t *testing.T) {
	binaryPath := buildFixtureAgent(t, "linux", foreignArch(), "2.0.3-rc.137")

	info, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo: %v", err)
	}
	if info.Version != "2.0.3-rc.137" {
		t.Errorf("version = %q, want %q", info.Version, "2.0.3-rc.137")
	}

	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	sum := sha256.Sum256(data)
	if want := hex.EncodeToString(sum[:]); info.Hash != want {
		t.Errorf("hash = %q, want %q", info.Hash, want)
	}
}

func TestGetAgentBinaryInfo_WindowsBinary(t *testing.T) {
	binaryPath := buildFixtureAgent(t, "windows", "amd64", "2.0.3-rc.137")

	info, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo: %v", err)
	}
	if info.Version != "2.0.3-rc.137" {
		t.Errorf("version = %q, want %q", info.Version, "2.0.3-rc.137")
	}
}

// A build with no -X flag must report no version rather than a guessed one, so
// the endpoint fails loudly instead of advertising a version that would disable
// auto-update for every host on that platform.
func TestGetAgentBinaryInfo_NoVersionStamp(t *testing.T) {
	binaryPath := buildFixtureAgent(t, "linux", foreignArch(), "")

	// Assert on build info directly: on a host with binfmt registered the exec
	// fallback can run the fixture and read its default version, which says
	// nothing about the path under test.
	f, err := os.Open(binaryPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()
	if v := versionFromBuildInfo(f); v != "" {
		t.Errorf("versionFromBuildInfo = %q, want empty for an unstamped build", v)
	}

	info, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo: %v", err)
	}
	if info.Hash == "" {
		t.Error("hash should still be returned for an unversioned binary")
	}
}

func TestGetAgentBinaryInfo_CacheInvalidatesOnReplace(t *testing.T) {
	binaryPath := buildFixtureAgent(t, "linux", foreignArch(), "2.0.3-rc.137")

	first, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo: %v", err)
	}

	cached, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo (cached): %v", err)
	}
	if cached != first {
		t.Errorf("cached read = %+v, want %+v", cached, first)
	}

	replacement := buildFixtureAgent(t, "linux", foreignArch(), "2.0.4")
	data, err := os.ReadFile(replacement)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if err := os.WriteFile(binaryPath, data, 0o755); err != nil {
		t.Fatalf("replace fixture: %v", err)
	}
	// Guard against a filesystem with coarse mtime resolution.
	if err := os.Chtimes(binaryPath, time.Now().Add(time.Second), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	updated, err := GetAgentBinaryInfo(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("GetAgentBinaryInfo (after replace): %v", err)
	}
	if updated.Version != "2.0.4" {
		t.Errorf("version after replace = %q, want %q", updated.Version, "2.0.4")
	}
	if updated.Hash == first.Hash {
		t.Error("hash did not change after the binary was replaced")
	}
}

func TestGetAgentBinaryInfo_MissingFile(t *testing.T) {
	if _, err := GetAgentBinaryInfo(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected an error for a missing binary")
	}
}

// foreignArch returns an architecture the test host cannot execute, which is
// the case the fix is about.
func foreignArch() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}

// buildFixtureAgent cross-compiles a stand-in agent stamped with the same
// -ldflags the real build uses, so build-info parsing is exercised against a
// genuine binary for the target platform. An empty version omits the -X flag.
func buildFixtureAgent(t *testing.T, goos, goarch, version string) string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module patchmon-agent\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	pkgDir := filepath.Join(srcDir, "internal", "pkgversion")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkgversion: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "version.go"), []byte("package pkgversion\n\nvar Version = \"0.0.0\"\n"), 0o644); err != nil {
		t.Fatalf("write version.go: %v", err)
	}
	main := "package main\n\nimport (\n\t\"fmt\"\n\n\t\"patchmon-agent/internal/pkgversion\"\n)\n\nfunc main() { fmt.Println(\"patchmon-agent version\", pkgversion.Version) }\n"
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	outName := "patchmon-agent-" + goos + "-" + goarch
	if goos == "windows" {
		outName += ".exe"
	}
	outPath := filepath.Join(t.TempDir(), outName)

	ldflags := "-s -w"
	if version != "" {
		ldflags += " -X patchmon-agent/internal/pkgversion.Version=" + version
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", ldflags, "-o", outPath, ".")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(),
		"GOOS="+goos,
		"GOARCH="+goarch,
		"CGO_ENABLED=0",
		"GOFLAGS=",
	)
	// Not a skip: cross-compiling these targets needs nothing beyond the Go
	// toolchain, and a skip would silently retire the regression test.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-compile for %s/%s failed: %v: %s", goos, goarch, err, out)
	}
	return outPath
}
