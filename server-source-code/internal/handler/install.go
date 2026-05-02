package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agents"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
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
//
// Hash-gated agent check-in. The agent ships its per-section content hashes
// (and volatile metrics like CPU/RAM/uptime) on every cycle; the server
// compares against stored hashes and replies with the list of stale sections
// in `requestFull`. The agent then fires a follow-up POST /hosts/update for
// just those sections.
//
// Empty body is supported as a legacy heartbeat path so old agents keep
// working unmodified — they get the same response shape they always did
// (without `requestFull`), and their next /hosts/update is treated as a
// full report.
func (h *InstallHandler) ServePing(w http.ResponseWriter, r *http.Request) {
	pingStart := time.Now()
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

	// Single SELECT loads identity + hashes + integration toggles. Failure
	// here is the auth failure path — host not found / wrong api_id maps to
	// 401 like before.
	checkin, err := h.hosts.GetCheckin(r.Context(), apiID)
	if err != nil || checkin == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, checkin.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	// Decode body. Empty body (legacy heartbeat, including chunked-encoded
	// empty bodies that arrive with ContentLength=-1) is supported.
	// io.EOF on the first read means there were no bytes to decode — fall
	// through to the legacy heartbeat path. Unknown fields are tolerated
	// so newer agents shipping additional optional fields keep working
	// against older servers (forward compat).
	var req models.PingRequest
	hasBody := false
	if r.ContentLength != 0 {
		err := json.NewDecoder(r.Body).Decode(&req)
		switch {
		case err == nil:
			hasBody = true
		case errors.Is(err, io.EOF):
			// Empty body on a chunked or unspecified-length request.
			// Treat as legacy heartbeat.
			hasBody = false
		default:
			JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
	}

	// Volatile metrics ride on every ping body. Apply them via a dedicated
	// COALESCE-guarded UPDATE so omitted fields don't clobber. Also bumps
	// last_update + status so the legacy heartbeat side-effect is preserved.
	if hasBody {
		mp := buildMetricsParams(&req)
		if err := h.hosts.UpdateMetrics(r.Context(), checkin.ID, mp); err != nil {
			slog.Error("failed to update host metrics", "host_id", checkin.ID, "error", err)
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Ping failed"})
			return
		}
	} else {
		// Legacy heartbeat: just bump last_update.
		if err := h.hosts.UpdatePing(r.Context(), checkin.ID); err != nil {
			slog.Error("failed to update host ping", "host_id", checkin.ID, "error", err)
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Ping failed"})
			return
		}
	}

	// Hash compare. If the agent shipped no body (legacy) we skip this and
	// `requestFull` stays empty — matches previous behaviour exactly.
	var requestFull []string
	if hasBody {
		requestFull = computeRequestFull(checkin, &req.Hashes)
	}

	now := time.Now()
	friendlyName := checkin.FriendlyName
	if friendlyName == "" && checkin.Hostname != nil {
		friendlyName = *checkin.Hostname
	}
	if friendlyName == "" {
		friendlyName = apiID
	}

	resp := map[string]interface{}{
		"message":      "Ping successful",
		"timestamp":    now.Format(time.RFC3339),
		"friendlyName": friendlyName,
		"agentStartup": true,
		"integrations": map[string]bool{
			"docker":     checkin.DockerEnabled,
			"compliance": checkin.ComplianceEnabled,
		},
	}
	if len(requestFull) > 0 {
		resp["requestFull"] = requestFull
	}

	// Record one update_history row per ping cycle for the Agent Activity
	// feed. Best-effort: failure is logged but does not fail the ping — the
	// agent has already done its work and the activity row is a UI nicety.
	// sections_unchanged = enabled hash-gated sections \ requestFull (server
	// is happy with whatever the agent has stored for those). Metrics ride on
	// every ping body, so they are the only "updated" section for a normal
	// body ping; legacy empty-body pings emit no updated sections.
	stale := make(map[string]struct{}, len(requestFull))
	for _, s := range requestFull {
		stale[s] = struct{}{}
	}
	pingSections := []string{
		models.SectionPackages,
		models.SectionRepos,
		models.SectionInterfaces,
		models.SectionHostname,
	}
	if checkin.DockerEnabled {
		pingSections = append(pingSections, models.SectionDocker)
	}
	if checkin.ComplianceEnabled {
		pingSections = append(pingSections, models.SectionCompliance)
	}
	unchanged := []string{}
	if hasBody {
		unchanged = make([]string, 0, len(pingSections))
		for _, s := range pingSections {
			if _, ok := stale[s]; !ok {
				unchanged = append(unchanged, s)
			}
		}
	}
	sectionsSent := []string{}
	if hasBody {
		sectionsSent = append(sectionsSent, models.SectionMetrics)
	}
	procSec := time.Since(pingStart).Seconds()
	var payloadKb *float64
	if r.ContentLength > 0 {
		v := float64(r.ContentLength) / 1024.0
		payloadKb = &v
	}
	var agentExec *int
	if hasBody && req.AgentExecutionMs != nil {
		agentExec = req.AgentExecutionMs
	}
	if err := h.reports.InsertActivityRow(r.Context(), store.AgentActivityInsert{
		HostID:            checkin.ID,
		ReportType:        "ping",
		SectionsSent:      sectionsSent,
		SectionsUnchanged: unchanged,
		PayloadSizeKb:     payloadKb,
		ServerProcessing:  &procSec,
		AgentExecutionMs:  agentExec,
		Status:            "success",
		ErrorMessage:      nil,
	}); err != nil {
		slog.Warn("ping: failed to record agent activity row", "host_id", checkin.ID, "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// buildMetricsParams maps a PingRequest's metrics into the store's update
// param shape. Empty / zero-valued fields stay nil so COALESCE preserves the
// existing column value.
func buildMetricsParams(req *models.PingRequest) store.HostMetricsParams {
	mp := store.HostMetricsParams{}
	m := &req.Metrics
	if m.CPUCores != nil {
		c := int32(*m.CPUCores)
		mp.CPUCores = &c
	}
	if m.CPUModel != nil && *m.CPUModel != "" {
		mp.CPUModel = m.CPUModel
	}
	if m.RAMInstalled != nil {
		mp.RAMInstalled = m.RAMInstalled
	}
	if m.SwapSize != nil {
		mp.SwapSize = m.SwapSize
	}
	if m.SystemUptime != nil && *m.SystemUptime != "" {
		mp.SystemUptime = m.SystemUptime
	}
	if m.NeedsReboot != nil {
		mp.NeedsReboot = m.NeedsReboot
	}
	if m.RebootReason != nil {
		mp.RebootReason = m.RebootReason
	}
	if len(m.DiskDetails) > 0 {
		if b, err := json.Marshal(m.DiskDetails); err == nil {
			mp.DiskDetails = b
		}
	}
	if len(m.LoadAverage) > 0 {
		if b, err := json.Marshal(m.LoadAverage); err == nil {
			mp.LoadAverage = b
		}
	}
	if req.AgentVersion != "" {
		v := req.AgentVersion
		mp.AgentVersion = &v
	}
	return mp
}

// computeRequestFull diffs the agent's claimed hashes against the stored
// hashes and returns the section identifiers the server needs filled in.
// A NULL stored hash, an empty agent hash, or a mismatch all flag the
// section as stale. Docker / compliance entries are suppressed when the
// integration is disabled server-side — the agent should not bother
// shipping data the operator has switched off.
func computeRequestFull(c *store.HostCheckin, h *models.PingHashes) []string {
	out := make([]string, 0, len(models.AllSections))
	stale := func(stored *string, agent string) bool {
		if stored == nil || *stored == "" {
			return true
		}
		if agent == "" {
			return true
		}
		return *stored != agent
	}
	if stale(c.Hashes.PackagesHash, h.PackagesHash) {
		out = append(out, models.SectionPackages)
	}
	if stale(c.Hashes.ReposHash, h.ReposHash) {
		out = append(out, models.SectionRepos)
	}
	if stale(c.Hashes.InterfacesHash, h.InterfacesHash) {
		out = append(out, models.SectionInterfaces)
	}
	if stale(c.Hashes.HostnameHash, h.HostnameHash) {
		out = append(out, models.SectionHostname)
	}
	if c.DockerEnabled && stale(c.Hashes.DockerHash, h.DockerHash) {
		out = append(out, models.SectionDocker)
	}
	if c.ComplianceEnabled && stale(c.Hashes.ComplianceHash, h.ComplianceHash) {
		out = append(out, models.SectionCompliance)
	}
	return out
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

	// Resolve which sections this payload claims to deliver. An absent
	// `sections` field means full report (legacy/old-agent path) — every
	// claimed top-level block is processed.
	var sections store.ReportSections
	if len(payload.Sections) > 0 {
		sections = store.SectionsFromList(payload.Sections)
		// Validate: at least one known section must be claimed. An empty
		// list after parsing means agent sent only unknown section names.
		if !sections.Packages && !sections.Repos && !sections.Interfaces && !sections.Hostname {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "sections must contain at least one of packages, repos, interfaces, hostname"})
			return
		}
		// Section/payload coherence — a claimed section MUST have data, an
		// unclaimed section MUST be empty so we don't accidentally write a
		// section the agent didn't intend to send.
		if sections.Packages && len(payload.Packages) == 0 {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "sections claims 'packages' but packages array is empty"})
			return
		}
		if !sections.Packages && len(payload.Packages) > 0 {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "packages provided but 'packages' not in sections"})
			return
		}
		if sections.Repos && len(payload.Repositories) == 0 {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "sections claims 'repos' but repositories array is empty"})
			return
		}
		if !sections.Repos && len(payload.Repositories) > 0 {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "repositories provided but 'repos' not in sections"})
			return
		}
	} else {
		sections = store.FullReport()
		if len(payload.Packages) == 0 {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "Packages array is required"})
			return
		}
	}

	if len(payload.Packages) > 10000 {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Packages array exceeds maximum size"})
		return
	}

	// Drift check: if the agent supplied a hash for a section, recompute the
	// canonical hash from the data they actually shipped and verify it
	// matches. Mismatch means the agent's canonicalisation diverged from
	// ours — a real bug we want to surface immediately rather than silently
	// store a hash that the next ping will reject.
	if hash := payload.Hashes.PackagesHash; sections.Packages && hash != "" {
		got, err := CanonicalPackagesHash(payload.Packages)
		if err != nil {
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "hash computation failed"})
			return
		}
		if got != hash {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "packages hash mismatch"})
			return
		}
	}
	if hash := payload.Hashes.ReposHash; sections.Repos && hash != "" {
		got, err := CanonicalReposHash(payload.Repositories)
		if err != nil {
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "hash computation failed"})
			return
		}
		if got != hash {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "repos hash mismatch"})
			return
		}
	}
	if hash := payload.Hashes.InterfacesHash; sections.Interfaces && hash != "" {
		ifaces, err := decodeNetworkInterfaces(payload.NetworkInterfaces)
		if err != nil {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid networkInterfaces"})
			return
		}
		got, err := CanonicalInterfacesHash(ifaces)
		if err != nil {
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "hash computation failed"})
			return
		}
		if got != hash {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "interfaces hash mismatch"})
			return
		}
	}
	if hash := payload.Hashes.HostnameHash; sections.Hostname && hash != "" {
		got := CanonicalHostnameHash(payload.Hostname)
		if got != hash {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "hostname hash mismatch"})
			return
		}
	}

	// Wrap ProcessReport in WithRetry so transient 40P01 (deadlock) and 40001
	// (serialization) errors are automatically retried with backoff. Each
	// retry reruns the whole transaction from scratch — see the comment near
	// InsertUpdateHistory for why this is safe.
	var result *store.ProcessReportResult
	err = database.WithRetry(r.Context(), "process_report", database.RetryConfig{}, func(ctx context.Context) error {
		var procErr error
		result, procErr = h.reports.ProcessReport(ctx, host.ID, &payload, sections, payload.Hashes)
		return procErr
	})
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
	validArchWindows := map[string]bool{"amd64": true, "arm64": true}
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
	validArchWindows := map[string]bool{"amd64": true, "arm64": true}
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
