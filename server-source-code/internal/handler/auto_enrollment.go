package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agents"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type AutoEnrollmentHandler struct {
	tokens     *store.AutoEnrollmentStore
	hostGroups *store.HostGroupsStore
	hosts      *store.HostsStore
	settings   *store.SettingsStore
	log        *slog.Logger
	cfg        *config.Config
}

func NewAutoEnrollmentHandler(tokens *store.AutoEnrollmentStore, hostGroups *store.HostGroupsStore, hosts *store.HostsStore, settings *store.SettingsStore, log *slog.Logger, cfg *config.Config) *AutoEnrollmentHandler {
	return &AutoEnrollmentHandler{tokens: tokens, hostGroups: hostGroups, hosts: hosts, settings: settings, log: log, cfg: cfg}
}

// List handles GET /auto-enrollment/tokens
func (h *AutoEnrollmentHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.tokens.List(r.Context())
	if err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment list failed", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to list tokens")
		return
	}
	items := make([]store.TokenListItem, len(rows))
	for i, row := range rows {
		items[i] = store.RowToTokenListItem(row)
	}
	JSON(w, http.StatusOK, items)
}

// GetByID handles GET /auto-enrollment/tokens/{tokenId}
func (h *AutoEnrollmentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	row, err := h.tokens.GetByID(r.Context(), tokenID)
	if err != nil {
		Error(w, http.StatusNotFound, "Token not found")
		return
	}
	JSON(w, http.StatusOK, store.IDRowToTokenListItem(row))
}

// Create handles POST /auto-enrollment/tokens
func (h *AutoEnrollmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TokenName          string          `json:"token_name"`
		AllowedIPRanges    []string        `json:"allowed_ip_ranges"`
		MaxHostsPerDay     *int32          `json:"max_hosts_per_day"`
		DefaultHostGroupID *string         `json:"default_host_group_id"`
		ExpiresAt          *string         `json:"expires_at"`
		Metadata           json.RawMessage `json:"metadata"`
		Scopes             json.RawMessage `json:"scopes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.TokenName == "" || len(req.TokenName) > 255 {
		Error(w, http.StatusBadRequest, "Token name is required (max 255 characters)")
		return
	}

	if req.DefaultHostGroupID != nil && *req.DefaultHostGroupID != "" {
		if _, err := h.hostGroups.GetByID(r.Context(), *req.DefaultHostGroupID); err != nil {
			Error(w, http.StatusBadRequest, "Host group not found")
			return
		}
	}

	// Parse metadata to check integration_type for scope validation
	var metaMap map[string]interface{}
	if len(req.Metadata) > 0 {
		_ = json.Unmarshal(req.Metadata, &metaMap)
	}
	if metaMap == nil {
		metaMap = map[string]interface{}{}
	}
	if _, ok := metaMap["integration_type"]; !ok {
		metaMap["integration_type"] = "proxmox-lxc"
	}
	metadataBytes, _ := json.Marshal(metaMap)

	integrationType, _ := metaMap["integration_type"].(string)

	// Validate scopes for API tokens
	var scopesBytes []byte
	if integrationType == "api" && len(req.Scopes) > 0 && string(req.Scopes) != "null" {
		if err := validateScopes(req.Scopes); err != nil {
			Error(w, http.StatusBadRequest, err.Error())
			return
		}
		scopesBytes = req.Scopes
	}

	tokenKey, tokenSecret, hashedSecret, err := generateEnrollmentCredentials()
	if err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment generate credentials failed", "error", err)
		}
		msg := "Failed to generate token credentials"
		if h.cfg != nil && h.cfg.Env == "development" {
			msg = "Failed to generate token credentials: " + err.Error()
		}
		Error(w, http.StatusInternalServerError, msg)
		return
	}

	maxHostsPerDay := int32(100)
	if req.MaxHostsPerDay != nil && *req.MaxHostsPerDay >= 1 && *req.MaxHostsPerDay <= 1000 {
		maxHostsPerDay = *req.MaxHostsPerDay
	}

	var expiresAt pgtype.Timestamp
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		// Support both RFC3339 and datetime-local format (e.g. "2026-03-15T10:00")
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			expiresAt = pgtype.Timestamp{Time: t, Valid: true}
		} else if t, err := time.Parse("2006-01-02T15:04", *req.ExpiresAt); err == nil {
			expiresAt = pgtype.Timestamp{Time: t, Valid: true}
		}
	}

	allowedIPRanges := req.AllowedIPRanges
	if allowedIPRanges == nil {
		allowedIPRanges = []string{}
	}

	var hostGroupID *string
	if req.DefaultHostGroupID != nil && *req.DefaultHostGroupID != "" {
		hostGroupID = req.DefaultHostGroupID
	}

	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	var createdByUserID *string
	if userID != "" {
		createdByUserID = &userID
	}

	now := pgtype.Timestamp{Time: time.Now(), Valid: true}
	id := uuid.New().String()

	err = h.tokens.Create(r.Context(), db.CreateAutoEnrollmentTokenParams{
		ID:                 id,
		TokenName:          req.TokenName,
		TokenKey:           tokenKey,
		TokenSecret:        hashedSecret,
		CreatedByUserID:    createdByUserID,
		IsActive:           true,
		AllowedIpRanges:    allowedIPRanges,
		MaxHostsPerDay:     maxHostsPerDay,
		DefaultHostGroupID: hostGroupID,
		ExpiresAt:          expiresAt,
		Metadata:           metadataBytes,
		Scopes:             scopesBytes,
		UpdatedAt:          now,
	})
	if err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment create failed", "error", err)
		}
		msg := "Failed to create token"
		if h.cfg != nil && h.cfg.Env == "development" {
			msg = "Failed to create token: " + err.Error()
		}
		Error(w, http.StatusInternalServerError, msg)
		return
	}

	// Fetch back with joins so we have host_groups and users in response
	row, err := h.tokens.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Token created but failed to retrieve")
		return
	}
	item := store.IDRowToTokenListItem(row)

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Auto-enrollment token created successfully",
		"token": map[string]interface{}{
			"id":                 item.ID,
			"token_name":         item.TokenName,
			"token_key":          tokenKey,
			"token_secret":       tokenSecret,
			"max_hosts_per_day":  item.MaxHostsPerDay,
			"default_host_group": item.HostGroups,
			"created_by":         item.Users,
			"expires_at":         item.ExpiresAt,
			"scopes":             item.Scopes,
		},
		"warning": "Save the token_secret now - it cannot be retrieved later!",
	})
}

// Update handles PATCH /auto-enrollment/tokens/{tokenId}
func (h *AutoEnrollmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	existing, err := h.tokens.GetRaw(r.Context(), tokenID)
	if err != nil {
		Error(w, http.StatusNotFound, "Token not found")
		return
	}

	var req struct {
		TokenName          *string         `json:"token_name"`
		IsActive           *bool           `json:"is_active"`
		MaxHostsPerDay     *int32          `json:"max_hosts_per_day"`
		AllowedIPRanges    *[]string       `json:"allowed_ip_ranges"`
		ExpiresAt          *string         `json:"expires_at"`
		DefaultHostGroupID *string         `json:"default_host_group_id"`
		Scopes             json.RawMessage `json:"scopes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Start with existing values
	tokenName := existing.TokenName
	isActive := existing.IsActive
	maxHostsPerDay := existing.MaxHostsPerDay
	allowedIPRanges := existing.AllowedIpRanges
	expiresAt := existing.ExpiresAt
	hostGroupID := existing.DefaultHostGroupID
	scopes := existing.Scopes

	if req.TokenName != nil {
		if *req.TokenName == "" || len(*req.TokenName) > 255 {
			Error(w, http.StatusBadRequest, "Token name must be between 1 and 255 characters")
			return
		}
		tokenName = *req.TokenName
	}
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if req.MaxHostsPerDay != nil {
		if *req.MaxHostsPerDay < 1 || *req.MaxHostsPerDay > 1000 {
			Error(w, http.StatusBadRequest, "Max hosts per day must be between 1 and 1000")
			return
		}
		maxHostsPerDay = *req.MaxHostsPerDay
	}
	if req.AllowedIPRanges != nil {
		allowedIPRanges = *req.AllowedIPRanges
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			expiresAt = pgtype.Timestamp{}
		} else if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			expiresAt = pgtype.Timestamp{Time: t, Valid: true}
		}
	}
	if req.DefaultHostGroupID != nil {
		if *req.DefaultHostGroupID == "" {
			hostGroupID = nil
		} else {
			if _, err := h.hostGroups.GetByID(r.Context(), *req.DefaultHostGroupID); err != nil {
				Error(w, http.StatusBadRequest, "Host group not found")
				return
			}
			hostGroupID = req.DefaultHostGroupID
		}
	}

	// Handle scopes - only for API integration tokens
	if len(req.Scopes) > 0 && string(req.Scopes) != "null" {
		var metaMap map[string]interface{}
		_ = json.Unmarshal(existing.Metadata, &metaMap)
		integrationType, _ := metaMap["integration_type"].(string)

		if integrationType != "api" {
			Error(w, http.StatusBadRequest, "Scopes can only be updated for API integration tokens")
			return
		}
		if err := validateScopes(req.Scopes); err != nil {
			Error(w, http.StatusBadRequest, err.Error())
			return
		}
		scopes = req.Scopes
	}

	if allowedIPRanges == nil {
		allowedIPRanges = []string{}
	}

	now := pgtype.Timestamp{Time: time.Now(), Valid: true}
	err = h.tokens.Update(r.Context(), db.UpdateAutoEnrollmentTokenParams{
		TokenName:          tokenName,
		IsActive:           isActive,
		MaxHostsPerDay:     maxHostsPerDay,
		AllowedIpRanges:    allowedIPRanges,
		ExpiresAt:          expiresAt,
		DefaultHostGroupID: hostGroupID,
		Scopes:             scopes,
		UpdatedAt:          now,
		ID:                 tokenID,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update token")
		return
	}

	row, err := h.tokens.GetByID(r.Context(), tokenID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Token updated but failed to retrieve")
		return
	}
	item := store.IDRowToTokenListItem(row)

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Token updated successfully",
		"token":   item,
	})
}

// Delete handles DELETE /auto-enrollment/tokens/{tokenId}
func (h *AutoEnrollmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	existing, err := h.tokens.GetRaw(r.Context(), tokenID)
	if err != nil {
		Error(w, http.StatusNotFound, "Token not found")
		return
	}

	if err := h.tokens.Delete(r.Context(), tokenID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete token")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Auto-enrollment token deleted successfully",
		"deleted_token": map[string]string{
			"id":         existing.ID,
			"token_name": existing.TokenName,
		},
	})
}

// Enroll handles POST /auto-enrollment/enroll.
// Auth via X-Auto-Enrollment-Key and X-Auto-Enrollment-Secret headers.
// Body: { "friendly_name": "...", "machine_id": "...", "metadata": {...} }
func (h *AutoEnrollmentHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tokenKey := r.Header.Get("X-Auto-Enrollment-Key")
	tokenSecret := r.Header.Get("X-Auto-Enrollment-Secret")
	if tokenKey == "" || tokenSecret == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Auto-enrollment credentials required"})
		return
	}

	ctx := r.Context()
	token, err := h.tokens.GetByKey(ctx, tokenKey)
	if err != nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid or inactive token"})
		return
	}
	if !token.IsActive {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid or inactive token"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(token.TokenSecret), []byte(tokenSecret)); err != nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token secret"})
		return
	}
	if token.ExpiresAt.Valid && token.ExpiresAt.Time.Before(time.Now()) {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Token expired"})
		return
	}

	if len(token.AllowedIpRanges) > 0 {
		clientIP := clientIPFromRequest(r)
		if !ipAllowed(clientIP, token.AllowedIpRanges) {
			if h.log != nil {
				h.log.Warn("Auto-enrollment attempt from unauthorized IP", "ip", clientIP)
			}
			JSON(w, http.StatusForbidden, map[string]string{"error": "IP address not authorized for this token"})
			return
		}
	}

	// Rate limit: effective hosts_created_today (reset if new day)
	today := time.Now().Format("2006-01-02")
	tokenResetDate := ""
	if token.LastResetDate.Valid {
		tokenResetDate = token.LastResetDate.Time.Format("2006-01-02")
	}
	effectiveCreated := token.HostsCreatedToday
	if tokenResetDate != today {
		effectiveCreated = 0
	}
	if token.MaxHostsPerDay > 0 && effectiveCreated >= token.MaxHostsPerDay {
		JSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":   "Rate limit exceeded",
			"message": "Maximum hosts per day allowed for this token",
		})
		return
	}

	var req struct {
		FriendlyName string          `json:"friendly_name"`
		MachineID    string          `json:"machine_id"`
		Metadata     json.RawMessage `json:"metadata"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.FriendlyName == "" || len(req.FriendlyName) > 255 {
		Error(w, http.StatusBadRequest, "Friendly name is required (max 255 characters)")
		return
	}
	if req.MachineID != "" && len(req.MachineID) > 255 {
		Error(w, http.StatusBadRequest, "Machine ID must be between 1 and 255 characters if provided")
		return
	}

	apiID := "patchmon_" + hex.EncodeToString(mustRand(8))
	apiKey := hex.EncodeToString(mustRand(32))
	apiKeyHash, err := bcrypt.GenerateFromPassword([]byte(apiKey), 10)
	if err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment bcrypt failed", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to generate credentials")
		return
	}

	complianceEnabled := true
	complianceOnDemandOnly := true
	if h.settings != nil {
		if s, _ := h.settings.GetFirst(ctx); s != nil {
			switch s.DefaultComplianceMode {
			case "disabled":
				complianceEnabled = false
			case "enabled":
				complianceOnDemandOnly = false
			default:
				complianceOnDemandOnly = true
			}
		}
	}

	machineID := req.MachineID
	if machineID == "" {
		machineID = "pending-" + uuid.New().String()
	}

	// Enforce host limit if a package is applied.
	if entry := hostctx.EntryFromContext(ctx); entry != nil && entry.MaxHosts != nil {
		count, countErr := h.hosts.Count(ctx)
		if countErr == nil && count >= *entry.MaxHosts {
			Error(w, http.StatusForbidden, "Host limit reached for this host's package")
			return
		}
	}

	host := &models.Host{
		MachineID:              &machineID,
		FriendlyName:           req.FriendlyName,
		OSType:                 "unknown",
		OSVersion:              "unknown",
		Status:                 "pending",
		ApiID:                  apiID,
		ApiKey:                 string(apiKeyHash),
		DockerEnabled:          false,
		ComplianceEnabled:      complianceEnabled,
		ComplianceOnDemandOnly: complianceOnDemandOnly,
	}
	if err := h.hosts.Create(ctx, host); err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment create host failed", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to enroll host")
		return
	}

	if token.DefaultHostGroupID != nil && *token.DefaultHostGroupID != "" {
		_ = h.hosts.SetHostGroups(ctx, host.ID, []string{*token.DefaultHostGroupID})
	}

	if err := h.tokens.IncrementHostsCreated(ctx, token.ID); err != nil {
		if h.log != nil {
			h.log.Error("auto-enrollment increment hosts failed", "error", err)
		}
	}

	if h.log != nil {
		h.log.Info("Auto-enrolled host", "friendly_name", req.FriendlyName, "host_id", host.ID, "token", token.TokenName)
	}

	var hostGroup *store.HostGroupBrief
	if token.DefaultHostGroupID != nil && *token.DefaultHostGroupID != "" {
		if hg, _ := h.hostGroups.GetByID(ctx, *token.DefaultHostGroupID); hg != nil {
			color := ""
			if hg.Color != nil {
				color = *hg.Color
			}
			hostGroup = &store.HostGroupBrief{ID: hg.ID, Name: hg.Name, Color: color}
		}
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Host enrolled successfully",
		"host": map[string]interface{}{
			"id":            host.ID,
			"friendly_name": host.FriendlyName,
			"api_id":        apiID,
			"api_key":       apiKey,
			"host_group":    hostGroup,
			"status":        host.Status,
		},
	})
}

func mustRand(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			xff = strings.TrimSpace(xff[:i])
		} else {
			xff = strings.TrimSpace(xff)
		}
		if xff != "" {
			if host, _, err := net.SplitHostPort(xff); err == nil {
				return host
			}
			return xff
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" {
		return host
	}
	return r.RemoteAddr
}

func ipAllowed(clientIP string, allowedRanges []string) bool {
	if clientIP == "" {
		return false
	}
	client := net.ParseIP(clientIP)
	if client == nil {
		return false
	}
	clientV4 := client.To4()
	if clientV4 == nil {
		clientV4 = client
	}
	for _, rng := range allowedRanges {
		rng = strings.TrimSpace(rng)
		if rng == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(rng); err == nil {
			if network.Contains(client) {
				return true
			}
			continue
		}
		if allowed := net.ParseIP(rng); allowed != nil {
			allowedV4 := allowed.To4()
			if allowedV4 == nil {
				allowedV4 = allowed
			}
			if clientV4 != nil && allowedV4 != nil && clientV4.Equal(allowedV4) {
				return true
			}
			if client.Equal(allowed) {
				return true
			}
		}
	}
	return false
}

// ServeScript handles GET /auto-enrollment/script?type=direct-host|proxmox-lxc&token_key=...&token_secret=...
// Public endpoint (no JWT). Validates token and serves the enrollment script with credentials injected.
func (h *AutoEnrollmentHandler) ServeScript(w http.ResponseWriter, r *http.Request) {
	tokenKey := r.URL.Query().Get("token_key")
	tokenSecret := r.URL.Query().Get("token_secret")
	scriptType := r.URL.Query().Get("type")

	if tokenKey == "" || tokenSecret == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Token key and secret required as query parameters"})
		return
	}
	if scriptType == "" {
		JSON(w, http.StatusBadRequest, map[string]string{
			"error": "Script type required as query parameter (e.g., ?type=proxmox-lxc or ?type=direct-host)",
		})
		return
	}

	var script []byte
	switch scriptType {
	case "proxmox-lxc":
		script = agents.ProxmoxAutoEnrollScript
	case "direct-host":
		script = agents.DirectHostAutoEnrollScript
	default:
		JSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid script type: " + scriptType + ". Supported types: proxmox-lxc, direct-host",
		})
		return
	}

	if len(script) == 0 {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Enrollment script not found: " + scriptType})
		return
	}

	ctx := r.Context()
	token, err := h.tokens.GetByKey(ctx, tokenKey)
	if err != nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid or inactive token"})
		return
	}
	if !token.IsActive {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid or inactive token"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(token.TokenSecret), []byte(tokenSecret)); err != nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token secret"})
		return
	}
	if token.ExpiresAt.Valid && token.ExpiresAt.Time.Before(time.Now()) {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Token expired"})
		return
	}

	serverURL := "http://localhost:3001"
	curlFlags := "-s"
	forceInstall := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	if h.settings != nil {
		if s, _ := h.settings.GetFirst(ctx); s != nil {
			if s.ServerURL != "" {
				serverURL = s.ServerURL
			}
			if s.IgnoreSSLSelfSigned {
				curlFlags = "-sk"
			}
		}
	}

	shebang := "#!/bin/sh"
	if scriptType == "proxmox-lxc" {
		shebang = "#!/bin/bash"
	}
	envBlock := shebang + "\n# PatchMon Auto-Enrollment Configuration (Auto-generated)\n" +
		"export PATCHMON_URL=\"" + serverURL + "\"\n" +
		"export AUTO_ENROLLMENT_KEY=\"" + token.TokenKey + "\"\n" +
		"export AUTO_ENROLLMENT_SECRET=\"" + tokenSecret + "\"\n" +
		"export CURL_FLAGS=\"" + curlFlags + "\"\n" +
		"export FORCE_INSTALL=\"" + map[bool]string{true: "true", false: "false"}[forceInstall] + "\"\n\n"

	scriptStr := string(script)
	scriptStr = strings.ReplaceAll(scriptStr, "\r\n", "\n")
	scriptStr = strings.ReplaceAll(scriptStr, "\r", "\n")
	if strings.HasPrefix(scriptStr, "#!") {
		scriptStr = "#" + scriptStr[1:]
	}
	// Remove the configuration section (between # ===== CONFIGURATION ===== and # ===== COLOR OUTPUT =====)
	configStart := strings.Index(scriptStr, "# ===== CONFIGURATION =====")
	colorStart := strings.Index(scriptStr, "# ===== COLOR OUTPUT =====")
	if configStart >= 0 && colorStart > configStart {
		scriptStr = scriptStr[:configStart] + scriptStr[colorStart:]
	}

	out := bytes.Buffer{}
	out.WriteString(envBlock)
	out.WriteString(scriptStr)

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", "inline; filename=\""+scriptType+"_auto_enroll.sh\"")
	// Prevent caching of URLs containing token credentials in query parameters.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}

func generateEnrollmentCredentials() (tokenKey, tokenSecret, hashedSecret string, err error) {
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	tokenKey = "patchmon_ae_" + hex.EncodeToString(b)

	// Use 32 bytes (64 hex chars) so the secret is under bcrypt's 72-byte limit
	b = make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	tokenSecret = hex.EncodeToString(b)

	hash, err := bcrypt.GenerateFromPassword([]byte(tokenSecret), 10)
	if err != nil {
		return "", "", "", err
	}
	return tokenKey, tokenSecret, string(hash), nil
}

func validateScopes(raw json.RawMessage) error {
	var scopes map[string]interface{}
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return &scopeError{"Scopes must be an object"}
	}
	for resource, actions := range scopes {
		arr, ok := actions.([]interface{})
		if !ok {
			return &scopeError{"Scopes for resource \"" + resource + "\" must be an array of actions"}
		}
		for _, a := range arr {
			if _, ok := a.(string); !ok {
				return &scopeError{"All actions in scopes must be strings"}
			}
		}
	}
	return nil
}

type scopeError struct{ msg string }

func (e *scopeError) Error() string { return e.msg }
