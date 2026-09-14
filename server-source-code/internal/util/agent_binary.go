package util

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// AgentVersionRe parses a version out of an agent binary's own output. Dropping
// the pre-release group makes the server advertise 2.0.3 to an agent reporting
// 2.0.3-rc.61, which then re-downloads itself on every check.
const AgentVersionRe = `(?i)(?:PatchMon Agent v|patchmon-agent v|version )?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`

var (
	agentVersionRe = regexp.MustCompile(AgentVersionRe)
	// The -X value every build path stamps in, as recorded in the binary's build info.
	ldflagsVersionRe = regexp.MustCompile(`pkgversion\.Version=v?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`)
	moduleVersionRe  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

// agentBinaryCacheTTL bounds how long a cached result can outlive an in-place
// replacement that preserved both mtime and size.
const agentBinaryCacheTTL = 60 * time.Second

// agentBinaryNames maps a supported os/arch pair to the binary's name on disk.
// Request values are only ever used as a lookup key here, never interpolated
// into a path, so nothing derived from a request can reach a file operation.
var agentBinaryNames = map[string]string{
	"linux/amd64":   "patchmon-agent-linux-amd64",
	"linux/386":     "patchmon-agent-linux-386",
	"linux/arm64":   "patchmon-agent-linux-arm64",
	"linux/arm":     "patchmon-agent-linux-arm",
	"freebsd/amd64": "patchmon-agent-freebsd-amd64",
	"freebsd/386":   "patchmon-agent-freebsd-386",
	"freebsd/arm64": "patchmon-agent-freebsd-arm64",
	"freebsd/arm":   "patchmon-agent-freebsd-arm",
	"windows/amd64": "patchmon-agent-windows-amd64.exe",
	"windows/arm64": "patchmon-agent-windows-arm64.exe",
}

// AgentBinaryName returns the on-disk binary name for an os/arch pair, and
// whether that pair is supported. The returned name is a constant.
func AgentBinaryName(osParam, arch string) (string, bool) {
	name, ok := agentBinaryNames[osParam+"/"+arch]
	return name, ok
}

// SupportedAgentTargets lists every os-arch pair the server can serve, sorted.
// The frontend splits these on the hyphen to build a download request.
func SupportedAgentTargets() []string {
	targets := make([]string, 0, len(agentBinaryNames))
	for key := range agentBinaryNames {
		targets = append(targets, strings.ReplaceAll(key, "/", "-"))
	}
	slices.Sort(targets)
	return targets
}

// SupportedAgentArches lists the architectures available for an OS, sorted.
func SupportedAgentArches(osParam string) []string {
	prefix := osParam + "/"
	arches := make([]string, 0, len(agentBinaryNames))
	for key := range agentBinaryNames {
		if strings.HasPrefix(key, prefix) {
			arches = append(arches, strings.TrimPrefix(key, prefix))
		}
	}
	slices.Sort(arches)
	return arches
}

// AgentBinaryInfo is the derived metadata for one agent binary on disk.
type AgentBinaryInfo struct {
	Version string
	Hash    string // hex-encoded SHA-256 of the binary
}

type agentBinaryCacheEntry struct {
	info    AgentBinaryInfo
	modTime time.Time
	size    int64
	readAt  time.Time
}

// agentBinaryCache keys derived metadata by path. ServeAgentVersion is called by
// every agent on every report, and each call otherwise re-hashes ~11MB.
var agentBinaryCache sync.Map

// GetAgentsDir returns the agents binary directory from env (AGENT_BINARIES_DIR, AGENTS_DIR) or "agents".
func GetAgentsDir() string {
	if d := os.Getenv("AGENT_BINARIES_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("AGENTS_DIR"); d != "" {
		return d
	}
	return "agents"
}

// getServerGoArch maps runtime.GOARCH to Go binary naming (matches handler).
func getServerGoArch() string {
	archMap := map[string]string{
		"amd64": "amd64",
		"386":   "386",
		"arm64": "arm64",
		"arm":   "arm",
	}
	if a, ok := archMap[runtime.GOARCH]; ok {
		return a
	}
	return runtime.GOARCH
}

// GetCurrentAgentVersionFromBinary finds the Linux agent binary for server arch and returns its version.
// Returns empty string if the binary is missing or its version cannot be determined.
func GetCurrentAgentVersionFromBinary(ctx context.Context, agentsDir string) string {
	candidates := []string{
		"patchmon-agent-linux-" + getServerGoArch(),
		"patchmon-agent-linux-amd64",
		"patchmon-agent",
	}

	for _, name := range candidates {
		path, err := SafePathUnderBase(agentsDir, name)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		return GetVersionFromBinaryPath(ctx, path)
	}
	return ""
}

// GetVersionFromBinaryPath returns the version of the agent binary at the given path,
// or "" if it cannot be determined. Callers must pass binaryPath validated with
// SafePathUnderBase(baseDir, binaryName) to prevent command injection.
func GetVersionFromBinaryPath(ctx context.Context, binaryPath string) string {
	info, err := GetAgentBinaryInfo(ctx, binaryPath)
	if err != nil {
		return ""
	}
	return info.Version
}

// GetAgentBinaryInfo returns the version and SHA-256 of the agent binary at the
// given path, cached until the file changes or the TTL lapses. An undeterminable
// version comes back empty rather than guessed: a wrong version here silently
// disables agent auto-update for every host on that platform.
// Callers must pass binaryPath validated with SafePathUnderBase(baseDir, binaryName).
func GetAgentBinaryInfo(ctx context.Context, binaryPath string) (AgentBinaryInfo, error) {
	// One handle for both the hash and the build info, so a binary replaced
	// mid-read cannot produce a version and hash from different files.
	f, err := os.Open(binaryPath)
	if err != nil {
		return AgentBinaryInfo{}, err
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return AgentBinaryInfo{}, err
	}
	if !stat.Mode().IsRegular() {
		return AgentBinaryInfo{}, fmt.Errorf("not a regular file: %s", binaryPath)
	}

	if cached, ok := agentBinaryCache.Load(binaryPath); ok {
		if entry, ok := cached.(agentBinaryCacheEntry); ok &&
			entry.modTime.Equal(stat.ModTime()) &&
			entry.size == stat.Size() &&
			time.Since(entry.readAt) < agentBinaryCacheTTL {
			return entry.info, nil
		}
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return AgentBinaryInfo{}, err
	}

	version := versionFromBuildInfo(f)
	if version == "" {
		version = versionFromExec(ctx, binaryPath)
	}

	info := AgentBinaryInfo{Version: version, Hash: hex.EncodeToString(hash.Sum(nil))}
	agentBinaryCache.Store(binaryPath, agentBinaryCacheEntry{
		info:    info,
		modTime: stat.ModTime(),
		size:    stat.Size(),
		readAt:  time.Now(),
	})

	return info, nil
}

// versionFromBuildInfo reads the version from the binary's embedded Go build
// info, which works across GOOS/GOARCH without executing anything. The recover
// guards the debug/elf, debug/pe and debug/macho parsers underneath, since one
// caller is a background queue worker rather than an HTTP handler.
func versionFromBuildInfo(r io.ReaderAt) (version string) {
	defer func() {
		if rec := recover(); rec != nil {
			version = ""
		}
	}()

	bi, err := buildinfo.Read(r)
	if err != nil {
		return ""
	}
	for _, setting := range bi.Settings {
		if setting.Key != "-ldflags" {
			continue
		}
		if m := ldflagsVersionRe.FindStringSubmatch(setting.Value); len(m) >= 2 {
			return m[1]
		}
	}
	// Tagged module builds carry the version without any -X flag.
	if v := strings.TrimPrefix(bi.Main.Version, "v"); moduleVersionRe.MatchString(v) {
		return v
	}
	return ""
}

// versionFromExec runs the binary to ask it, which is only possible when the
// binary matches the server platform. Fallback for builds with no usable build info.
func versionFromExec(ctx context.Context, binaryPath string) string {
	name := filepath.Base(binaryPath)
	binaryOS := "linux"
	switch {
	case strings.Contains(name, "freebsd"):
		binaryOS = "freebsd"
	case strings.Contains(name, "windows"), strings.HasSuffix(name, ".exe"):
		binaryOS = "windows"
	}
	if runtime.GOOS != binaryOS {
		return ""
	}

	// There is no "version" subcommand, only cobra's --version flag.
	for _, arg := range []string{"--version", "--help"} {
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := exec.CommandContext(runCtx, binaryPath, arg).CombinedOutput()
		cancel()
		if err != nil {
			continue
		}
		if m := agentVersionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}
