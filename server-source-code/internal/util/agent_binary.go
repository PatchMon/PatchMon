package util

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const agentVersionRe = `(?i)(?:PatchMon Agent v|patchmon-agent v|version )?([0-9]+\.[0-9]+\.[0-9]+)`

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

// GetCurrentAgentVersionFromBinary finds the Linux agent binary for server arch, executes it, and returns the version.
// Returns empty string if binary not found or version cannot be parsed.
func GetCurrentAgentVersionFromBinary(ctx context.Context, agentsDir string) string {
	serverGoArch := getServerGoArch()
	possiblePaths := []string{
		filepath.Join(agentsDir, "patchmon-agent-linux-"+serverGoArch),
		filepath.Join(agentsDir, "patchmon-agent-linux-amd64"),
		filepath.Join(agentsDir, "patchmon-agent"),
	}

	var agentPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			agentPath = p
			break
		}
	}
	if agentPath == "" {
		return ""
	}

	versionRe := regexp.MustCompile(agentVersionRe)
	versionCommands := []string{"--version", "version", "--help"}

	for _, cmd := range versionCommands {
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := exec.CommandContext(runCtx, agentPath, cmd).CombinedOutput()
		cancel()
		if err != nil {
			continue
		}
		if m := versionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

// GetVersionFromBinaryPath gets version from a binary at the given path.
// Tries executing the binary if it matches server platform (linux/linux, freebsd/freebsd), else uses "strings".
func GetVersionFromBinaryPath(ctx context.Context, binaryPath string) string {
	versionRe := regexp.MustCompile(agentVersionRe)
	serverOS := runtime.GOOS
	binaryOS := "linux"
	if strings.Contains(binaryPath, "freebsd") {
		binaryOS = "freebsd"
	} else if strings.Contains(binaryPath, "windows") || strings.HasSuffix(binaryPath, ".exe") {
		binaryOS = "windows"
	}

	// Try exec if same platform (Windows .exe cannot be executed on Linux)
	if (serverOS == "linux" && binaryOS == "linux") || (serverOS == "freebsd" && binaryOS == "freebsd") {
		for _, cmd := range []string{"--version", "version", "--help"} {
			runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			out, err := exec.CommandContext(runCtx, binaryPath, cmd).CombinedOutput()
			cancel()
			if err != nil {
				continue
			}
			if m := versionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
				return m[1]
			}
		}
	}

	// Fallback: use "strings" command (cross-platform)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	out, err := exec.CommandContext(runCtx, "strings", binaryPath).CombinedOutput()
	cancel()
	if err != nil {
		return ""
	}
	if m := versionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
		return m[1]
	}
	return ""
}
