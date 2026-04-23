package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agents"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

// InstallHandler handles agent install, bootstrap, ping, update, and agent download endpoints.
type InstallHandler struct {
	hosts      *store.HostsStore
	settings   *store.SettingsStore
	bootstrap  *store.BootstrapStore
	reports    *store.ReportStore
	scriptBase []byte // base script (embedded or from AGENTS_DIR)
}

// NewInstallHandler creates a new install handler.
func NewInstallHandler(hosts *store.HostsStore, settings *store.SettingsStore, bootstrap *store.BootstrapStore, reports *store.ReportStore) *InstallHandler {
	return &InstallHandler{
		hosts:      hosts,
		settings:   settings,
		bootstrap:  bootstrap,
		reports:    reports,
		scriptBase: loadScript(),
	}
}

func loadScript() []byte {
	if dir := os.Getenv("AGENTS_DIR"); dir != "" {
		b, err := os.ReadFile(dir + "/patchmon_install.sh")
		if err == nil {
			return normalizeLineEndings(b)
		}
	}
	return normalizeLineEndings(agents.PatchmonInstallScript)
}

func loadRemoveScript() []byte {
	if dir := os.Getenv("AGENTS_DIR"); dir != "" {
		b, err := os.ReadFile(dir + "/patchmon_remove.sh")
		if err == nil {
			return normalizeLineEndings(b)
		}
	}
	return normalizeLineEndings(agents.PatchmonRemoveScript)
}

func loadWindowsInstallScript() []byte {
	if dir := os.Getenv("AGENTS_DIR"); dir != "" {
		b, err := os.ReadFile(dir + "/patchmon_install_windows.ps1")
		if err == nil {
			return normalizeLineEndings(b)
		}
	}
	return normalizeLineEndings(agents.PatchmonWindowsInstallScript)
}

func loadWindowsRemoveScript() []byte {
	if dir := os.Getenv("AGENTS_DIR"); dir != "" {
		b, err := os.ReadFile(dir + "/patchmon_remove_windows.ps1")
		if err == nil {
			return normalizeLineEndings(b)
		}
	}
	return normalizeLineEndings(agents.PatchmonWindowsRemoveScript)
}

func normalizeLineEndings(b []byte) []byte {
	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}

// ServeInstall handles GET /api/v1/hosts/install.
// Requires X-API-ID and X-API-KEY headers. Serves the install script with env vars and bootstrap token injected.
func (h *InstallHandler) ServeInstall(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("install handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	if len(h.scriptBase) == 0 {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Installation script not found"})
		return
	}

	settings, _ := h.settings.GetFirst(r.Context())
	serverURL := "http://localhost:3001"
	curlFlags := "-s"
	skipSSLVerify := "false"
	if settings != nil {
		if settings.ServerURL != "" {
			serverURL = settings.ServerURL
		}
		if settings.IgnoreSSLSelfSigned {
			curlFlags = "-sk"
			skipSSLVerify = "true"
		}
	}

	forceInstall := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	architecture := r.URL.Query().Get("arch")
	// Allowlist architecture to prevent shell injection in generated install script.
	validArchitectures := map[string]bool{"amd64": true, "386": true, "arm64": true, "arm": true}
	if architecture != "" && !validArchitectures[architecture] {
		architecture = ""
	}
	osParam := r.URL.Query().Get("os")
	if osParam != "linux" && osParam != "freebsd" && osParam != "windows" {
		osParam = "linux"
	}

	// Windows: serve PowerShell script with env vars
	if osParam == "windows" {
		if h.bootstrap == nil {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Bootstrap service unavailable"})
			return
		}
		token, err := h.bootstrap.GenerateToken(r.Context(), host.ApiID, apiKey)
		if err != nil {
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate bootstrap token"})
			return
		}
		script := loadWindowsInstallScript()
		if len(script) == 0 {
			JSON(w, http.StatusNotFound, map[string]string{"error": "Windows installation script not found"})
			return
		}
		envBlock := fmt.Sprintf("$env:PATCHMON_SERVER_URL = \"%s\"\n$env:PATCHMON_BOOTSTRAP_TOKEN = \"%s\"\n$env:PATCHMON_IGNORE_SSL = \"%s\"\n\n", serverURL, token, skipSSLVerify)
		// Remove shebang if present (PowerShell scripts may have # optional)
		if bytes.HasPrefix(script, []byte("#!")) {
			script = append([]byte("#"), script[1:]...)
		}
		script = append([]byte(envBlock), script...)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `inline; filename="patchmon_install_windows.ps1"`)
		_, _ = w.Write(script)
		return
	}

	if h.bootstrap == nil {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Bootstrap service unavailable"})
		return
	}
	token, err := h.bootstrap.GenerateToken(r.Context(), host.ApiID, apiKey)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate bootstrap token"})
		return
	}

	archExport := ""
	if architecture != "" {
		archExport = fmt.Sprintf("export ARCHITECTURE=\"%s\"\n", architecture)
	}
	forceStr := "false"
	if forceInstall {
		forceStr = "true"
	}

	envBlock := fmt.Sprintf(`#!/bin/sh
export PATCHMON_URL="%s"
export PATCHMON_OS="%s"
export BOOTSTRAP_TOKEN="%s"
export CURL_FLAGS="%s"
export SKIP_SSL_VERIFY="%s"
export FORCE_INSTALL="%s"
%s
# Fetch actual credentials using bootstrap token (one-time use, expires in 5 minutes)
fetch_credentials() {
    CREDS=$(curl ${CURL_FLAGS} -X POST "${PATCHMON_URL}/api/v1/hosts/bootstrap/exchange" \
        -H "Content-Type: application/json" \
        -d "{\"token\": \"${BOOTSTRAP_TOKEN}\"}" 2>/dev/null)

    if [ -z "$CREDS" ] || echo "$CREDS" | grep -q '"error"'; then
        echo "ERROR: Failed to fetch credentials. Bootstrap token may have expired."
        echo "Please request a new installation script."
        exit 1
    fi

    export API_ID=$(echo "$CREDS" | grep -o '"apiId":"[^"]*"' | cut -d'"' -f4)
    export API_KEY=$(echo "$CREDS" | grep -o '"apiKey":"[^"]*"' | cut -d'"' -f4)

    if [ -z "$API_ID" ] || [ -z "$API_KEY" ]; then
        echo "ERROR: Invalid credentials received from server."
        exit 1
    fi
}
fetch_credentials
`, serverURL, osParam, token, curlFlags, skipSSLVerify, forceStr, archExport)

	// Remove shebang from original script and prepend env block
	script := h.scriptBase
	if bytes.HasPrefix(script, []byte("#!")) {
		script = append([]byte("#"), script[1:]...)
	}
	script = append([]byte(envBlock), script...)

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", `inline; filename="patchmon_install.sh"`)
	_, _ = w.Write(script)
}

// ServeRemove handles GET /api/v1/hosts/remove.
// Public endpoint (no auth). Serves the agent removal script with CURL_FLAGS from settings.
func (h *InstallHandler) ServeRemove(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("remove handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()
	osParam := r.URL.Query().Get("os")
	if osParam == "windows" {
		script := loadWindowsRemoveScript()
		if len(script) == 0 {
			JSON(w, http.StatusNotFound, map[string]string{"error": "Windows removal script not found"})
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `inline; filename="patchmon_remove_windows.ps1"`)
		_, _ = w.Write(script)
		return
	}
	script := loadRemoveScript()
	if len(script) == 0 {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Removal script not found"})
		return
	}
	curlFlags := "-s"
	if settings, _ := h.settings.GetFirst(r.Context()); settings != nil && settings.IgnoreSSLSelfSigned {
		curlFlags = "-sk"
	}
	envPrefix := []byte("#!/bin/sh\nexport CURL_FLAGS=\"" + curlFlags + "\"\n\n")
	// Replace shebang in script with comment so we can prepend our own
	if bytes.HasPrefix(script, []byte("#!")) {
		script = append([]byte("#"), script[1:]...)
	}
	script = append(envPrefix, script...)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", `inline; filename="patchmon_remove.sh"`)
	_, _ = w.Write(script)
}

// BootstrapExchange handles POST /api/v1/hosts/bootstrap/exchange.
// Consumes a one-time bootstrap token and returns apiId and apiKey.
func (h *InstallHandler) BootstrapExchange(w http.ResponseWriter, r *http.Request) {
	if h.bootstrap == nil {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Bootstrap service unavailable"})
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Token == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Bootstrap token required"})
		return
	}

	apiID, apiKey, ok := h.bootstrap.ConsumeToken(r.Context(), req.Token)
	if !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid or expired bootstrap token"})
		return
	}

	JSON(w, http.StatusOK, map[string]string{
		"apiId":  apiID,
		"apiKey": apiKey,
	})
}

// ServePing handles POST /api/v1/hosts/ping.
// Agent connectivity test and credential validation. Updates host last_update and status.
func (h *InstallHandler) ServePing(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("ping handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()

	if r.Method != http.MethodPost {
		JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	if err := h.hosts.UpdatePing(r.Context(), host.ID); err != nil {
		slog.Error("failed to update host ping", "host_id", host.ID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Ping failed"})
		return
	}

	now := time.Now()
	friendlyName := host.FriendlyName
	if friendlyName == "" && host.Hostname != nil {
		friendlyName = *host.Hostname
	}
	if friendlyName == "" {
		friendlyName = host.ApiID
	}

	resp := map[string]interface{}{
		"message":      "Ping successful",
		"timestamp":    now.Format(time.RFC3339),
		"friendlyName": friendlyName,
		"agentStartup": true, // simplified; Node.js computes from last_update delta
		"integrations": map[string]bool{
			"docker":     host.DockerEnabled,
			"compliance": host.ComplianceEnabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ServeUpdateInterval handles GET /api/v1/settings/update-interval.
// Agent fetches current update interval on startup (API key auth).
func (h *InstallHandler) ServeUpdateInterval(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	settings, err := h.settings.GetFirst(r.Context())
	interval := 60
	if err == nil && settings != nil && settings.UpdateInterval > 0 {
		interval = settings.UpdateInterval
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"updateInterval": interval,
	})
}

// ServeUpdate handles POST /api/v1/hosts/update.
// Agent sends package and system info report.
func (h *InstallHandler) ServeUpdate(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("update handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()

	if r.Method != http.MethodPost {
		JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var payload store.ReportPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if len(payload.Packages) == 0 {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Packages array is required"})
		return
	}

	if len(payload.Packages) > 10000 {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Packages array exceeds maximum size"})
		return
	}

	result, err := h.reports.ProcessReport(r.Context(), host.ID, &payload)
	if err != nil {
		slog.Error("failed to process report", "host_id", host.ID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update host"})
		return
	}

	resp := map[string]interface{}{
		"message":           "Host updated successfully",
		"packagesProcessed": result.PackagesProcessed,
		"updatesAvailable":  result.UpdatesAvailable,
		"securityUpdates":   result.SecurityUpdates,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ServeAgentVersion handles GET /api/v1/hosts/agent/version.
// Requires X-API-ID and X-API-KEY headers. Returns version info for agent auto-update (matches Node hostRoutes).
func (h *InstallHandler) ServeAgentVersion(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("agent version handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()

	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	// Check auto_update settings
	settings, _ := h.settings.GetFirst(r.Context())
	serverAutoUpdate := settings != nil && settings.AutoUpdate
	hostAutoUpdate := host.AutoUpdate
	autoUpdateDisabled := !serverAutoUpdate || !hostAutoUpdate
	var autoUpdateDisabledReason string
	if autoUpdateDisabled {
		if !serverAutoUpdate && !hostAutoUpdate {
			autoUpdateDisabledReason = "Auto-update is disabled in server settings and for this host"
		} else if !serverAutoUpdate {
			autoUpdateDisabledReason = "Auto-update is disabled in server settings"
		} else {
			autoUpdateDisabledReason = "Auto-update is disabled for this host"
		}
	}

	architecture := r.URL.Query().Get("arch")
	if architecture == "" {
		architecture = "amd64"
	}

	osParam := r.URL.Query().Get("os")
	if osParam == "" && host.ExpectedPlatform != nil {
		ep := strings.ToLower(*host.ExpectedPlatform)
		if ep == "windows" {
			osParam = "windows"
		} else if strings.Contains(ep, "freebsd") || strings.Contains(ep, "pfsense") {
			osParam = "freebsd"
		} else {
			osParam = "linux"
		}
	}
	if osParam == "" && host.OSType != "" {
		reported := strings.ToLower(host.OSType)
		if strings.Contains(reported, "windows") {
			osParam = "windows"
		} else if strings.Contains(reported, "freebsd") || strings.Contains(reported, "pfsense") {
			osParam = "freebsd"
		} else {
			osParam = "linux"
		}
	}
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
	validArchWindows := map[string]bool{"amd64": true, "386": true, "arm64": true}
	var validArch map[string]bool
	var archList string
	switch osParam {
	case "freebsd":
		validArch = validArchFreebsd
		archList = "amd64, 386, arm64, arm"
	case "windows":
		validArch = validArchWindows
		archList = "amd64, 386"
	default:
		validArch = validArchLinux
		archList = "amd64, 386, arm64, arm"
	}
	if !validArch[architecture] {
		JSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Invalid architecture for %s. Must be one of: %s", osParam, archList),
		})
		return
	}

	binaryName := fmt.Sprintf("patchmon-agent-%s-%s", osParam, architecture)
	if osParam == "windows" {
		binaryName = binaryName + ".exe"
	}
	binDir := util.GetAgentsDir()
	binaryPath, err := util.SafePathUnderBase(binDir, binaryName)
	if err != nil {
		agentVersion := r.URL.Query().Get("currentVersion")
		if agentVersion == "" {
			agentVersion = "unknown"
		}
		JSON(w, http.StatusNotFound, map[string]interface{}{
			"error":                    fmt.Sprintf("Agent binary not found for %s/%s. Ensure %s exists in the agent binaries directory.", osParam, architecture, binaryName),
			"currentVersion":           agentVersion,
			"latestVersion":            nil,
			"hasUpdate":                false,
			"autoUpdateDisabled":       autoUpdateDisabled,
			"autoUpdateDisabledReason": autoUpdateDisabledReason,
			"architecture":             architecture,
			"agentType":                "go",
		})
		return
	}

	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		agentVersion := r.URL.Query().Get("currentVersion")
		if agentVersion == "" {
			agentVersion = "unknown"
		}
		JSON(w, http.StatusNotFound, map[string]interface{}{
			"error":                    fmt.Sprintf("Agent binary not found for %s/%s. Ensure %s exists in the agent binaries directory.", osParam, architecture, binaryName),
			"currentVersion":           agentVersion,
			"latestVersion":            nil,
			"hasUpdate":                false,
			"autoUpdateDisabled":       autoUpdateDisabled,
			"autoUpdateDisabledReason": autoUpdateDisabledReason,
			"architecture":             architecture,
			"agentType":                "go",
		})
		return
	}

	serverVersion := util.GetVersionFromBinaryPath(r.Context(), binaryPath)
	if serverVersion == "" {
		agentVersion := r.URL.Query().Get("currentVersion")
		if agentVersion == "" {
			agentVersion = "unknown"
		}
		JSON(w, http.StatusNotFound, map[string]interface{}{
			"error":                    fmt.Sprintf("Could not determine version for binary %s", binaryName),
			"currentVersion":           agentVersion,
			"latestVersion":            nil,
			"hasUpdate":                false,
			"autoUpdateDisabled":       autoUpdateDisabled,
			"autoUpdateDisabledReason": autoUpdateDisabledReason,
			"architecture":             architecture,
			"agentType":                "go",
		})
		return
	}

	agentVersion := r.URL.Query().Get("currentVersion")
	if agentVersion == "" {
		agentVersion = serverVersion
	}
	agentVersion = strings.TrimPrefix(agentVersion, "v")
	serverVersionNorm := strings.TrimPrefix(serverVersion, "v")

	hasUpdate := util.CompareVersions(serverVersionNorm, agentVersion) > 0
	if autoUpdateDisabled {
		hasUpdate = false
	}

	var binaryHash string
	if data, err := os.ReadFile(binaryPath); err == nil {
		sum := sha256.Sum256(data)
		binaryHash = hex.EncodeToString(sum[:])
	}

	downloadURL := fmt.Sprintf("/api/v1/hosts/agent/download?arch=%s&os=%s", architecture, osParam)
	JSON(w, http.StatusOK, map[string]interface{}{
		"currentVersion":           agentVersion,
		"latestVersion":            serverVersionNorm,
		"hasUpdate":                hasUpdate,
		"autoUpdateDisabled":       autoUpdateDisabled,
		"autoUpdateDisabledReason": autoUpdateDisabledReason,
		"downloadUrl":              downloadURL,
		"releaseNotes":             fmt.Sprintf("PatchMon Agent v%s", serverVersionNorm),
		"minServerVersion":         nil,
		"architecture":             architecture,
		"agentType":                "go",
		"hash":                     binaryHash,
	})
}

// ServeAgentDownload handles GET /api/v1/hosts/agent/download.
// Requires X-API-ID and X-API-KEY headers. Serves the agent binary for the given arch and os.
func (h *InstallHandler) ServeAgentDownload(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("agent download handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()

	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	architecture := r.URL.Query().Get("arch")
	if architecture == "" {
		architecture = "amd64"
	}

	osParam := r.URL.Query().Get("os")
	if osParam == "" && host.ExpectedPlatform != nil {
		ep := strings.ToLower(*host.ExpectedPlatform)
		if ep == "windows" {
			osParam = "windows"
		} else if strings.Contains(ep, "freebsd") || strings.Contains(ep, "pfsense") {
			osParam = "freebsd"
		} else {
			osParam = "linux"
		}
	}
	if osParam == "" && host.OSType != "" {
		reported := strings.ToLower(host.OSType)
		if strings.Contains(reported, "windows") {
			osParam = "windows"
		} else if strings.Contains(reported, "freebsd") || strings.Contains(reported, "pfsense") {
			osParam = "freebsd"
		} else {
			osParam = "linux"
		}
	}
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
	validArchWindows := map[string]bool{"amd64": true, "386": true, "arm64": true}
	var validArch map[string]bool
	var archList string
	switch osParam {
	case "freebsd":
		validArch = validArchFreebsd
		archList = "amd64, 386, arm64, arm"
	case "windows":
		validArch = validArchWindows
		archList = "amd64, 386"
	default:
		validArch = validArchLinux
		archList = "amd64, 386, arm64, arm"
	}
	if !validArch[architecture] {
		JSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Invalid architecture for %s. Must be one of: %s", osParam, archList),
		})
		return
	}

	binaryName := fmt.Sprintf("patchmon-agent-%s-%s", osParam, architecture)
	if osParam == "windows" {
		binaryName = binaryName + ".exe"
	}

	// Resolve binary directory: AGENT_BINARIES_DIR, then AGENTS_DIR, then "agents" in cwd
	binDir := os.Getenv("AGENT_BINARIES_DIR")
	if binDir == "" {
		binDir = os.Getenv("AGENTS_DIR")
	}
	if binDir == "" {
		binDir = "agents"
	}
	binaryPath, err := util.SafePathUnderBase(binDir, binaryName)
	if err != nil {
		slog.Warn("agent binary path validation failed", "name", binaryName, "error", err)
		JSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Agent binary not found for %s/%s. Ensure %s exists in the agent binaries directory.", osParam, architecture, binaryName),
		})
		return
	}

	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		slog.Warn("agent binary not found", "path", binaryPath, "error", err)
		JSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Agent binary not found for %s/%s. Ensure %s exists in the agent binaries directory.", osParam, architecture, binaryName),
		})
		return
	}

	f, err := os.Open(binaryPath)
	if err != nil {
		slog.Error("failed to open agent binary", "path", binaryPath, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to serve agent binary"})
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	http.ServeContent(w, r, binaryName, info.ModTime(), f)
}
