package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const (
	agentDNSDomain = "agent.vcheck.patchmon.net"
	agentVersionRe = `(?i)(?:PatchMon Agent v|patchmon-agent v|version )?([0-9]+\.[0-9]+\.[0-9]+)`
)

// AgentVersionHandler handles agent version routes (current from binary, latest from DNS).
type AgentVersionHandler struct {
	agentsDir string
	log       *slog.Logger
	// Cached values (in-memory, same as Node agentVersionService)
	currentVersion string
	latestVersion  string
	lastChecked    *time.Time
}

// NewAgentVersionHandler creates a new agent version handler.
func NewAgentVersionHandler(log *slog.Logger) *AgentVersionHandler {
	agentsDir := os.Getenv("AGENT_BINARIES_DIR")
	if agentsDir == "" {
		agentsDir = os.Getenv("AGENTS_DIR")
	}
	if agentsDir == "" {
		agentsDir = "agents"
	}
	return &AgentVersionHandler{agentsDir: agentsDir, log: log}
}

// getServerGoArch maps runtime.GOARCH to Go binary naming (matches Node os.arch() mapping).
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

// getCurrentAgentVersion finds the Linux agent binary for server arch and executes it to get version.
func (h *AgentVersionHandler) getCurrentAgentVersion(ctx context.Context) string {
	serverGoArch := getServerGoArch()
	h.log.Debug("agent version: detected server arch", "goarch", runtime.GOARCH, "mapped", serverGoArch)

	possiblePaths := []string{
		filepath.Join(h.agentsDir, fmt.Sprintf("patchmon-agent-linux-%s", serverGoArch)),
		filepath.Join(h.agentsDir, "patchmon-agent-linux-amd64"),
		filepath.Join(h.agentsDir, "patchmon-agent"),
	}

	var agentPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			agentPath = p
			h.log.Debug("agent version: found binary", "path", p)
			break
		}
	}
	if agentPath == "" {
		h.log.Debug("agent version: no binary found", "arch", serverGoArch)
		return ""
	}

	versionCommands := []string{"--version", "version", "--help"}
	versionRe := regexp.MustCompile(agentVersionRe)

	for _, cmd := range versionCommands {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := exec.CommandContext(ctx, agentPath, cmd).CombinedOutput()
		cancel()
		if err != nil {
			h.log.Debug("agent version: exec failed", "cmd", cmd, "error", err)
			continue
		}
		output := string(out)
		if m := versionRe.FindStringSubmatch(output); len(m) >= 2 {
			h.log.Info("agent version: current from binary", "version", m[1], "cmd", cmd)
			return m[1]
		}
	}

	h.log.Debug("agent version: could not parse version from binary output")
	return ""
}

// getLatestVersionFromDNS performs DNS TXT lookup for agent version (matches Node checkVersionFromDNS).
func (h *AgentVersionHandler) getLatestVersionFromDNS(ctx context.Context) (string, error) {
	v, err := util.LookupVersionFromDNS(agentDNSDomain)
	if err != nil {
		return "", err
	}
	v = strings.TrimSpace(strings.Trim(v, "\"'"))
	// Validate semver format (x.y.z)
	semverRe := regexp.MustCompile(`^\d+\.\d+\.\d+`)
	if !semverRe.MatchString(v) {
		return "", fmt.Errorf("invalid version format: %s", v)
	}
	return v, nil
}

// GetVersionInfo returns current (from binary cache), latest (from DNS), and updateStatus.
// Use RefreshCurrentVersion to update current from binary, CheckForUpdates to refresh latest from DNS.
func (h *AgentVersionHandler) GetVersionInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Lazy-load current version from binary if not yet cached
	if h.currentVersion == "" {
		h.currentVersion = h.getCurrentAgentVersion(ctx)
	}

	// Latest from DNS - refresh if stale or never checked (matches Node's periodic check)
	now := time.Now()
	if h.latestVersion == "" || h.lastChecked == nil || now.Sub(*h.lastChecked) > 5*time.Minute {
		if v, err := h.getLatestVersionFromDNS(ctx); err == nil {
			h.latestVersion = v
			h.lastChecked = &now
		}
	}

	// Determine updateStatus (matches Node getVersionInfo)
	var hasUpdate bool
	updateStatus := "unknown"
	if h.currentVersion != "" && h.latestVersion != "" {
		cmp := util.CompareVersions(h.currentVersion, h.latestVersion)
		if cmp < 0 {
			hasUpdate = true
			updateStatus = "update-available"
		} else if cmp > 0 {
			updateStatus = "newer-version"
		} else {
			updateStatus = "up-to-date"
		}
	} else if h.latestVersion != "" && h.currentVersion == "" {
		hasUpdate = true
		updateStatus = "no-agent"
	} else if h.currentVersion != "" && h.latestVersion == "" {
		updateStatus = "github-unavailable"
	} else if h.currentVersion == "" && h.latestVersion == "" {
		updateStatus = "no-data"
	}

	resp := map[string]interface{}{
		"currentVersion": h.currentVersion,
		"latestVersion":  h.latestVersion,
		"hasUpdate":      hasUpdate,
		"updateStatus":   updateStatus,
		"lastChecked":    h.lastChecked,
		"supportedArchitectures": []string{
			"linux-amd64", "linux-arm64", "linux-386", "linux-arm",
			"freebsd-amd64", "freebsd-arm64", "freebsd-386", "freebsd-arm",
			"windows-amd64", "windows-arm64",
		},
		"status": "ready",
	}
	if h.latestVersion == "" {
		resp["status"] = "no-version"
	}
	JSON(w, http.StatusOK, resp)
}

// RefreshCurrentVersion re-runs binary version detection.
func (h *AgentVersionHandler) RefreshCurrentVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := h.getCurrentAgentVersion(ctx)
	h.currentVersion = v
	var msg string
	if v != "" {
		msg = fmt.Sprintf("Current version refreshed: %s", v)
	} else {
		msg = "No agent binary found"
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success":        v != "",
		"currentVersion": v,
		"message":        msg,
	})
}

// ServeAgentDownload handles GET /api/v1/agent/download?arch=X&os=Y for authenticated users.
// Serves the agent binary. Requires session auth (route is under auth middleware).
func (h *AgentVersionHandler) ServeAgentDownload(w http.ResponseWriter, r *http.Request) {
	architecture := r.URL.Query().Get("arch")
	if architecture == "" {
		architecture = "amd64"
	}
	osParam := r.URL.Query().Get("os")
	if osParam == "" {
		osParam = "linux"
	}
	validOss := map[string]bool{"linux": true, "freebsd": true, "windows": true}
	if !validOss[osParam] {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid os. Must be one of: linux, freebsd, windows"})
		return
	}
	validArchLinux := map[string]bool{"amd64": true, "386": true, "arm64": true, "arm": true}
	validArchFreebsd := map[string]bool{"amd64": true, "386": true, "arm64": true, "arm": true}
	validArchWindows := map[string]bool{"amd64": true, "arm64": true}
	var validArch map[string]bool
	switch osParam {
	case "freebsd":
		validArch = validArchFreebsd
	case "windows":
		validArch = validArchWindows
	default:
		validArch = validArchLinux
	}
	if !validArch[architecture] {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid architecture for " + osParam})
		return
	}
	binaryName := fmt.Sprintf("patchmon-agent-%s-%s", osParam, architecture)
	if osParam == "windows" {
		binaryName = binaryName + ".exe"
	}
	binaryPath, err := util.SafePathUnderBase(h.agentsDir, binaryName)
	if err != nil {
		h.log.Warn("agent binary path validation failed", "name", binaryName, "error", err)
		JSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Agent binary not found for %s/%s.", osParam, architecture),
		})
		return
	}
	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		h.log.Warn("agent binary not found for user download", "path", binaryPath, "error", err)
		JSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Agent binary not found for %s/%s.", osParam, architecture),
		})
		return
	}
	f, err := os.Open(binaryPath)
	if err != nil {
		h.log.Error("failed to open agent binary", "path", binaryPath, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to serve agent binary"})
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	http.ServeContent(w, r, binaryName, info.ModTime(), f)
}

// CheckForUpdates refreshes latest version from DNS.
func (h *AgentVersionHandler) CheckForUpdates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := h.getLatestVersionFromDNS(ctx)
	now := time.Now()
	if err != nil {
		h.log.Warn("agent version: DNS check failed", "error", err)
		JSON(w, http.StatusOK, map[string]interface{}{
			"latestVersion":  h.latestVersion,
			"currentVersion": h.currentVersion,
			"hasUpdate":      false,
			"lastChecked":    h.lastChecked,
		})
		return
	}
	h.latestVersion = v
	h.lastChecked = &now
	hasUpdate := h.currentVersion != "" && util.CompareVersions(h.currentVersion, v) < 0
	JSON(w, http.StatusOK, map[string]interface{}{
		"latestVersion":  v,
		"currentVersion": h.currentVersion,
		"hasUpdate":      hasUpdate,
		"lastChecked":    now,
	})
}
