package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// ssgFilenameRe validates SSG datastream filenames to prevent path traversal.
var ssgFilenameRe = regexp.MustCompile(`^ssg-[a-z0-9]+-ds\.xml$`)

// ComplianceHandler handles compliance endpoints.
type ComplianceHandler struct {
	complianceStore   *store.ComplianceStore
	hostsStore        *store.HostsStore
	registry          *agentregistry.Registry
	queueClient       *asynq.Client
	queueInspector    *asynq.Inspector
	integrationStatus *store.IntegrationStatusStore
	ssgContentDir     string
	notify            *notifications.Emitter
}

// NewComplianceHandler creates a new compliance handler.
func NewComplianceHandler(complianceStore *store.ComplianceStore, hostsStore *store.HostsStore, registry *agentregistry.Registry, queueClient *asynq.Client, queueInspector *asynq.Inspector, integrationStatus *store.IntegrationStatusStore, ssgContentDir string, notify *notifications.Emitter) *ComplianceHandler {
	return &ComplianceHandler{
		complianceStore:   complianceStore,
		hostsStore:        hostsStore,
		registry:          registry,
		queueClient:       queueClient,
		queueInspector:    queueInspector,
		integrationStatus: integrationStatus,
		ssgContentDir:     ssgContentDir,
		notify:            notify,
	}
}

// Scan submission rate limiter: 10/min per agent
type scanRateLimiter struct {
	mu        sync.Mutex
	counts    map[string][]time.Time
	limit     int
	window    time.Duration
	lastClean time.Time
}

func newScanRateLimiter(limit int, window time.Duration) *scanRateLimiter {
	return &scanRateLimiter{
		counts:    make(map[string][]time.Time),
		limit:     limit,
		window:    window,
		lastClean: time.Now(),
	}
}

func (rl *scanRateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Periodic full cleanup: remove stale keys every 5 minutes to prevent
	// unbounded map growth from many unique agent IDs.
	if now.Sub(rl.lastClean) > 5*time.Minute {
		for k, ts := range rl.counts {
			valid := ts[:0]
			for _, t := range ts {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.counts, k)
			} else {
				rl.counts[k] = valid
			}
		}
		rl.lastClean = now
	}

	// Prune old entries for the current key
	valid := rl.counts[key][:0]
	for _, t := range rl.counts[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rl.limit {
		rl.counts[key] = valid
		return false
	}
	valid = append(valid, now)
	rl.counts[key] = valid
	return true
}

var scanSubmitLimiter = newScanRateLimiter(10, time.Minute)

// Agent scan payload (matches agent CompliancePayload / nested scans)
type complianceScanPayload struct {
	Scans        []complianceScanItem `json:"scans"`
	Hostname     string               `json:"hostname"`
	MachineID    string               `json:"machine_id"`
	AgentVersion string               `json:"agent_version"`
	// Legacy flat format
	ProfileName   string                 `json:"profile_name"`
	ProfileType   string                 `json:"profile_type"`
	Results       []complianceResultItem `json:"results"`
	StartedAt     *time.Time             `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at"`
	Status        string                 `json:"status"`
	Score         *float64               `json:"score"`
	TotalRules    *int                   `json:"total_rules"`
	Passed        *int                   `json:"passed"`
	Failed        *int                   `json:"failed"`
	Warnings      *int                   `json:"warnings"`
	Skipped       *int                   `json:"skipped"`
	NotApplicable *int                   `json:"not_applicable"`
	Error         string                 `json:"error"`
}

type complianceScanItem struct {
	ProfileName   string                 `json:"profile_name"`
	ProfileType   string                 `json:"profile_type"`
	Results       []complianceResultItem `json:"results"`
	StartedAt     *time.Time             `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at"`
	Status        string                 `json:"status"`
	Score         *float64               `json:"score"`
	TotalRules    *int                   `json:"total_rules"`
	Passed        *int                   `json:"passed"`
	Failed        *int                   `json:"failed"`
	Warnings      *int                   `json:"warnings"`
	Skipped       *int                   `json:"skipped"`
	NotApplicable *int                   `json:"not_applicable"`
	Error         string                 `json:"error"`
}

type complianceResultItem struct {
	RuleRef     string `json:"rule_ref"`
	RuleID      string `json:"rule_id"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Finding     string `json:"finding"`
	Actual      string `json:"actual"`
	Expected    string `json:"expected"`
	Section     string `json:"section"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Remediation string `json:"remediation"`
}

// ReceiveScans handles POST /api/v1/compliance/scans (agent endpoint, API key auth).
func (h *ComplianceHandler) ReceiveScans(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	key := apiID
	if key == "" {
		key = r.RemoteAddr
	}
	if !scanSubmitLimiter.allow(key) {
		JSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many scan submissions, please try again later"})
		return
	}

	host, err := h.hostsStore.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var payload complianceScanPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	// Normalize to scans array (nested or legacy flat)
	scansToProcess := payload.Scans
	if len(scansToProcess) == 0 && payload.ProfileName != "" {
		scansToProcess = []complianceScanItem{{
			ProfileName:   payload.ProfileName,
			ProfileType:   payload.ProfileType,
			Results:       payload.Results,
			StartedAt:     payload.StartedAt,
			CompletedAt:   payload.CompletedAt,
			Status:        payload.Status,
			Score:         payload.Score,
			TotalRules:    payload.TotalRules,
			Passed:        payload.Passed,
			Failed:        payload.Failed,
			Warnings:      payload.Warnings,
			Skipped:       payload.Skipped,
			NotApplicable: payload.NotApplicable,
			Error:         payload.Error,
		}}
	}

	if len(scansToProcess) == 0 {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload: expected 'scans' array or 'profile_name'"})
		return
	}

	// Convert to store format
	storeScans := make([]store.SubmittedScan, 0, len(scansToProcess))
	for _, s := range scansToProcess {
		results := make([]store.SubmittedScanResult, 0, len(s.Results))
		for _, res := range s.Results {
			ruleRef := res.RuleRef
			if ruleRef == "" {
				ruleRef = res.RuleID
			}
			if ruleRef == "" {
				ruleRef = res.ID
			}
			results = append(results, store.SubmittedScanResult{
				RuleRef:     ruleRef,
				Title:       res.Title,
				Description: res.Description,
				Severity:    res.Severity,
				Section:     res.Section,
				Remediation: res.Remediation,
				Status:      res.Status,
				Finding:     res.Finding,
				Actual:      res.Actual,
				Expected:    res.Expected,
			})
		}
		storeScans = append(storeScans, store.SubmittedScan{
			ProfileName:   s.ProfileName,
			ProfileType:   s.ProfileType,
			Results:       results,
			StartedAt:     s.StartedAt,
			CompletedAt:   s.CompletedAt,
			Status:        s.Status,
			Score:         s.Score,
			TotalRules:    s.TotalRules,
			Passed:        s.Passed,
			Failed:        s.Failed,
			Warnings:      s.Warnings,
			Skipped:       s.Skipped,
			NotApplicable: s.NotApplicable,
			Error:         s.Error,
		})
	}

	openscapEnabled := host.ComplianceOpenscapEnabled
	dockerBenchEnabled := host.ComplianceDockerBenchEnabled

	processed, err := h.complianceStore.SubmitScan(r.Context(), host.ID, openscapEnabled, dockerBenchEnabled, storeScans)
	if err != nil {
		slog.Error("compliance submit scan failed", "error", err, "host_id", host.ID)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save scan results"})
		return
	}

	// Build response
	scanResponses := make([]map[string]interface{}, 0, len(processed))
	failedTotal := 0
	for _, p := range processed {
		if p.Stats != nil {
			failedTotal += p.Stats["failed"]
		}
		scanResponses = append(scanResponses, map[string]interface{}{
			"scan_id":        p.ScanID,
			"profile_name":   p.ProfileName,
			"score":          p.Score,
			"stats":          p.Stats,
			"results_stored": p.ResultsStored,
		})
	}
	if h.notify != nil {
		if d := hostctx.DBFromContext(r.Context()); d != nil {
			sev := "informational"
			if failedTotal > 0 {
				sev = "warning"
			}

			// Resolve host display name
			hostName := host.FriendlyName
			if hostName == "" && host.Hostname != nil {
				hostName = *host.Hostname
			}

			// Build per-profile summaries for NOC readability
			passedTotal := 0
			totalRules := 0
			profileSummaries := make([]map[string]interface{}, 0, len(processed))
			for _, p := range processed {
				ps := map[string]interface{}{
					"profile": p.ProfileName,
				}
				if p.Score != nil {
					ps["score"] = fmt.Sprintf("%.1f%%", *p.Score)
				}
				if p.Stats != nil {
					ps["passed"] = p.Stats["passed"]
					ps["failed"] = p.Stats["failed"]
					ps["warnings"] = p.Stats["warnings"]
					passedTotal += p.Stats["passed"]
					totalRules += p.Stats["passed"] + p.Stats["failed"] + p.Stats["warnings"] + p.Stats["skipped"]
				}
				profileSummaries = append(profileSummaries, ps)
			}

			title := fmt.Sprintf("Compliance Scan - %s", hostName)
			if failedTotal > 0 {
				title = fmt.Sprintf("Compliance Scan - %d Failed Rules - %s", failedTotal, hostName)
			}

			var msgParts []string
			msgParts = append(msgParts, fmt.Sprintf("Compliance scan completed on host %s.", hostName))
			msgParts = append(msgParts, fmt.Sprintf("Profiles scanned: %d", len(processed)))
			if totalRules > 0 {
				msgParts = append(msgParts, fmt.Sprintf("Results: %d passed, %d failed out of %d total rules.", passedTotal, failedTotal, totalRules))
			}
			for _, ps := range profileSummaries {
				line := fmt.Sprintf("  • %s", ps["profile"])
				if score, ok := ps["score"]; ok {
					line += fmt.Sprintf(" - Score: %s", score)
				}
				if passed, ok := ps["passed"]; ok {
					line += fmt.Sprintf(" - Passed: %v, Failed: %v", passed, ps["failed"])
				}
				msgParts = append(msgParts, line)
			}

			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "compliance_scan_completed",
				Severity:      sev,
				Title:         title,
				Message:       strings.Join(msgParts, "\n"),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":           host.ID,
					"host_name":         hostName,
					"failed_count":      failedTotal,
					"passed_count":      passedTotal,
					"total_rules":       totalRules,
					"scans_count":       len(processed),
					"profile_summaries": profileSummaries,
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":        "Scan results saved successfully",
		"scans_received": len(processed),
		"scans":          scanResponses,
	})
}

// ListProfiles returns all compliance profiles.
func (h *ComplianceHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.complianceStore.ListProfiles(r.Context())
	if err != nil {
		slog.Error("compliance list profiles failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch profiles")
		return
	}
	JSON(w, http.StatusOK, profiles)
}

// GetDashboard returns the compliance dashboard.
func (h *ComplianceHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	dash, err := h.complianceStore.GetDashboard(r.Context())
	if err != nil {
		slog.Error("compliance dashboard failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch dashboard data")
		return
	}
	JSON(w, http.StatusOK, dash)
}

// GetActiveScans returns currently running scans from DB and Asynq (queued run_scan jobs).
func (h *ComplianceHandler) GetActiveScans(w http.ResponseWriter, r *http.Request) {
	scans, err := h.complianceStore.ListActiveScans(r.Context())
	if err != nil {
		slog.Error("compliance active scans failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch active scans")
		return
	}
	// Add connection status
	type scanWithStatus struct {
		ID          string  `json:"id"`
		HostID      string  `json:"hostId"`
		HostName    string  `json:"hostName"`
		ApiID       *string `json:"apiId"`
		ProfileName *string `json:"profileName"`
		ProfileType *string `json:"profileType"`
		StartedAt   string  `json:"startedAt"`
		Status      string  `json:"status"`
		Connected   bool    `json:"connected"`
	}
	out := make([]scanWithStatus, 0, len(scans))
	hostIDsInDB := make(map[string]bool)
	for _, sc := range scans {
		hostIDsInDB[sc.HostID] = true
		connected := h.registry.Get(sc.ApiID).Connected
		startedAt := ""
		if sc.StartedAt.Valid {
			startedAt = sc.StartedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		hostName := sc.FriendlyName
		if hostName == "" && sc.Hostname != nil {
			hostName = *sc.Hostname
		}
		apiID := sc.ApiID
		profileName := sc.ProfileName
		profileType := sc.ProfileType
		out = append(out, scanWithStatus{
			ID: sc.ID, HostID: sc.HostID, HostName: hostName, ApiID: &apiID,
			ProfileName: &profileName, ProfileType: &profileType,
			StartedAt: startedAt, Status: sc.Status, Connected: connected,
		})
	}

	// Merge Asynq run_scan jobs (queued, not yet in DB)
	if h.queueInspector != nil {
		queued, _ := queue.ListRunScanTasks(r.Context(), h.queueInspector)
		for _, q := range queued {
			if hostIDsInDB[q.HostID] {
				continue // already have DB record for this host
			}
			hostName := ""
			if host, err := h.hostsStore.GetByID(r.Context(), q.HostID); err == nil && host != nil {
				hostName = host.FriendlyName
				if hostName == "" && host.Hostname != nil && *host.Hostname != "" {
					hostName = *host.Hostname
				}
			}
			if hostName == "" {
				hostName = "Unknown"
			}
			connected := h.registry.Get(q.ApiID).Connected
			startedAt := q.StartedAt.Format("2006-01-02T15:04:05Z07:00")
			profileName := q.ProfileName
			profileType := q.ProfileType
			apiID := q.ApiID
			out = append(out, scanWithStatus{
				ID: q.ID, HostID: q.HostID, HostName: hostName, ApiID: &apiID,
				ProfileName: &profileName, ProfileType: &profileType,
				StartedAt: startedAt, Status: "queued", Connected: connected,
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"activeScans": out,
		"count":       len(out),
	})
}

// GetScanHistory returns paginated scan history.
func (h *ComplianceHandler) GetScanHistory(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25, 1, 100)
	offset := parseIntParam(r, "offset", 0, 0, 100000)
	status := strParam(r, "status")
	profileType := strParam(r, "profile_type")
	hostID := strParam(r, "host_id")

	scans, total, err := h.complianceStore.ListScansHistory(r.Context(), int32(limit), int32(offset), status, hostID, profileType)
	if err != nil {
		slog.Error("compliance scan history failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch scan history")
		return
	}
	rows := make([]map[string]interface{}, 0, len(scans))
	for _, s := range scans {
		durationMs := (*int64)(nil)
		if s.CompletedAt.Valid && s.StartedAt.Valid {
			d := s.CompletedAt.Time.Sub(s.StartedAt.Time).Milliseconds()
			durationMs = &d
		}
		rows = append(rows, map[string]interface{}{
			"id": s.ID, "host_id": s.HostID, "host_name": s.FriendlyName,
			"profile_name": s.ProfileName, "profile_type": s.ProfileType,
			"status": s.Status, "started_at": s.StartedAt, "completed_at": s.CompletedAt,
			"duration_ms": durationMs, "total_rules": s.TotalRules,
			"passed": s.Passed, "failed": s.Failed, "warnings": s.Warnings,
			"skipped": s.Skipped, "not_applicable": s.NotApplicable,
			"score": s.Score, "error_message": s.ErrorMessage,
		})
	}
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"scans": rows,
		"pagination": map[string]interface{}{
			"total": total, "limit": limit, "offset": offset, "total_pages": totalPages,
		},
	})
}

// GetStalledScans returns scans running over 3 hours.
func (h *ComplianceHandler) GetStalledScans(w http.ResponseWriter, r *http.Request) {
	threshold := time.Now().Add(-3 * time.Hour)
	scans, err := h.complianceStore.ListStalledScans(r.Context(), threshold)
	if err != nil {
		slog.Error("compliance stalled scans failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch stalled scans")
		return
	}
	out := make([]map[string]interface{}, 0, len(scans))
	for _, s := range scans {
		runtimeMs := time.Since(s.StartedAt.Time).Milliseconds()
		out = append(out, map[string]interface{}{
			"id": s.ID, "hostId": s.HostID, "hostName": s.FriendlyName, "apiId": s.ApiID,
			"profileName": s.ProfileName, "profileType": s.ProfileType,
			"startedAt": s.StartedAt.Time, "status": s.Status,
			"runtimeMinutes": runtimeMs / 60000,
			"runtimeHours":   float64(runtimeMs) / 3600000,
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"stalledScans": out,
		"count":        len(out),
	})
}

// GetHostScans returns paginated scans for a host.
func (h *ComplianceHandler) GetHostScans(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID format")
		return
	}
	limit := parseIntParam(r, "limit", 20, 1, 100)
	offset := parseIntParam(r, "offset", 0, 0, 10000)

	scans, total, err := h.complianceStore.ListScansByHost(r.Context(), hostID, int32(limit), int32(offset))
	if err != nil {
		slog.Error("compliance host scans failed", "error", err, "host_id", hostID)
		Error(w, http.StatusInternalServerError, "Failed to fetch scans")
		return
	}
	rows := make([]interface{}, 0, len(scans))
	for _, s := range scans {
		rows = append(rows, map[string]interface{}{
			"id": s.ID, "host_id": s.HostID, "profile_id": s.ProfileID,
			"started_at": s.StartedAt, "completed_at": s.CompletedAt, "status": s.Status,
			"total_rules": s.TotalRules, "passed": s.Passed, "failed": s.Failed,
			"warnings": s.Warnings, "skipped": s.Skipped, "not_applicable": s.NotApplicable,
			"score": s.Score, "error_message": s.ErrorMessage,
			"compliance_profiles": map[string]interface{}{"name": s.ProfileName, "type": s.ProfileType},
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"scans":      rows,
		"pagination": map[string]interface{}{"total": total, "limit": limit, "offset": offset},
	})
}

// GetLatestScan returns the latest scan for a host.
func (h *ComplianceHandler) GetLatestScan(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID format")
		return
	}
	profileType := strParam(r, "profile_type")

	scan, err := h.complianceStore.GetLatestScan(r.Context(), hostID, profileType)
	if err != nil {
		slog.Error("compliance latest scan failed", "error", err, "host_id", hostID)
		Error(w, http.StatusInternalServerError, "Failed to fetch latest scan")
		return
	}
	if scan == nil {
		Error(w, http.StatusNotFound, "No scans found for this host")
		return
	}
	// Return full scan with results
	results, _, _, err := h.complianceStore.ListResultsByScan(r.Context(), scan.ID, nil, nil, 1000, 0)
	if err != nil {
		slog.Error("compliance latest scan results failed", "error", err, "host_id", hostID)
	}
	resultsOut := make([]map[string]interface{}, 0, len(results))
	for _, res := range results {
		resultsOut = append(resultsOut, map[string]interface{}{
			"id": res.ID, "scan_id": res.ScanID, "rule_id": res.RuleID,
			"status": res.Status, "finding": res.Finding, "actual": res.Actual,
			"expected": res.Expected, "remediation": res.Remediation,
			"compliance_rules": map[string]interface{}{
				"id": res.RuleID, "rule_ref": res.RuleRef, "title": res.Title,
				"description": res.Description, "severity": res.Severity,
				"section": res.Section, "remediation": res.RuleRemediation,
			},
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"id": scan.ID, "host_id": scan.HostID, "profile_id": scan.ProfileID,
		"started_at": scan.StartedAt, "completed_at": scan.CompletedAt, "status": scan.Status,
		"total_rules": scan.TotalRules, "passed": scan.Passed, "failed": scan.Failed,
		"warnings": scan.Warnings, "skipped": scan.Skipped, "not_applicable": scan.NotApplicable,
		"score": scan.Score, "error_message": scan.ErrorMessage,
		"compliance_profiles": map[string]interface{}{"id": scan.ProfileID, "name": scan.ProfileName, "type": scan.ProfileType},
		"compliance_results":  resultsOut,
	})
}

// GetLatestScansByType returns latest scan per profile type for a host.
func (h *ComplianceHandler) GetLatestScansByType(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID format")
		return
	}
	result, err := h.complianceStore.GetLatestScansByType(r.Context(), hostID)
	if err != nil {
		slog.Error("compliance latest by type failed", "error", err, "host_id", hostID)
		Error(w, http.StatusInternalServerError, "Failed to fetch latest scans by type")
		return
	}
	// Convert to JSON-friendly format
	out := make(map[string]interface{})
	for k, v := range result {
		out[k] = map[string]interface{}{
			"id": v.ID, "profile_name": v.ProfileName, "profile_type": v.ProfileType,
			"score": v.Score, "total_rules": v.TotalRules, "passed": v.Passed,
			"failed": v.Failed, "warnings": v.Warnings, "skipped": v.Skipped,
			"completed_at":       v.CompletedAt,
			"severity_breakdown": v.SeverityBreakdown,
			"section_breakdown":  v.SectionBreakdown,
		}
	}
	JSON(w, http.StatusOK, out)
}

// GetScanResults returns paginated results for a scan.
func (h *ComplianceHandler) GetScanResults(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "scanId")
	if scanID == "" || !isValidUUID(scanID) {
		Error(w, http.StatusBadRequest, "Invalid scan ID format")
		return
	}
	status := strParam(r, "status")
	severity := strParam(r, "severity")
	limit := parseIntParam(r, "limit", 50, 1, 100)
	offset := parseIntParam(r, "offset", 0, 0, 10000)

	results, total, severityBreakdown, err := h.complianceStore.ListResultsByScan(r.Context(), scanID, status, severity, int32(limit), int32(offset))
	if err != nil {
		slog.Error("compliance scan results failed", "error", err, "scan_id", scanID)
		Error(w, http.StatusInternalServerError, "Failed to fetch results")
		return
	}
	rows := make([]map[string]interface{}, 0, len(results))
	for _, res := range results {
		rows = append(rows, map[string]interface{}{
			"id": res.ID, "scan_id": res.ScanID, "rule_id": res.RuleID,
			"status": res.Status, "finding": res.Finding, "actual": res.Actual,
			"expected": res.Expected, "remediation": res.Remediation,
			"compliance_rules": map[string]interface{}{
				"id": res.RuleID, "rule_ref": res.RuleRef, "title": res.Title,
				"description": res.Description, "rationale": res.Rationale,
				"severity": res.Severity, "section": res.Section, "remediation": res.RuleRemediation,
			},
		})
	}
	resp := map[string]interface{}{
		"results":    rows,
		"pagination": map[string]interface{}{"total": total, "limit": limit, "offset": offset},
	}
	if severityBreakdown != nil {
		resp["severity_breakdown"] = severityBreakdown
	}
	JSON(w, http.StatusOK, resp)
}

// GetRules returns rules with aggregated counts.
func (h *ComplianceHandler) GetRules(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50, 1, 200)
	offset := parseIntParam(r, "offset", 0, 0, 100000)
	severity := strParam(r, "severity")
	status := strParam(r, "status")
	search := strParam(r, "search")
	profileType := strParam(r, "profile_type")
	hostID := strParam(r, "host_id")
	sortBy := strParamDefault(r, "sort_by", "status")
	sortDir := strParamDefault(r, "sort_dir", "desc")

	rules, total, err := h.complianceStore.ListRules(r.Context(), severity, status, search, profileType, hostID, int32(limit), int32(offset), sortBy, sortDir)
	if err != nil {
		slog.Error("compliance rules failed", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to fetch rules")
		return
	}
	rows := make([]map[string]interface{}, 0, len(rules))
	for _, r := range rules {
		rows = append(rows, map[string]interface{}{
			"id": r.ID, "rule_ref": r.RuleRef, "title": r.Title,
			"severity": r.Severity, "section": r.Section,
			"profile_id": r.ProfileID, "profile_type": r.ProfileType, "profile_name": r.ProfileName,
			"hosts_passed": r.HostsPassed, "hosts_failed": r.HostsFailed,
			"hosts_warned": r.HostsWarned, "total_hosts": r.TotalHosts,
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"rules":      rows,
		"pagination": map[string]interface{}{"total": total, "limit": limit, "offset": offset},
	})
}

// GetRuleDetail returns rule detail with affected hosts.
func (h *ComplianceHandler) GetRuleDetail(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "ruleId")
	if ruleID == "" || !isValidUUID(ruleID) {
		Error(w, http.StatusBadRequest, "Invalid rule ID format")
		return
	}
	detail, err := h.complianceStore.GetRuleDetail(r.Context(), ruleID)
	if err != nil {
		slog.Error("compliance rule detail failed", "error", err, "rule_id", ruleID)
		Error(w, http.StatusInternalServerError, "Failed to fetch rule detail")
		return
	}
	affected := make([]map[string]interface{}, 0, len(detail.AffectedHosts))
	for _, a := range detail.AffectedHosts {
		affected = append(affected, map[string]interface{}{
			"host_id": a.HostID, "hostname": a.Hostname, "friendly_name": a.FriendlyName,
			"ip": a.IP, "status": a.Status, "finding": a.Finding,
			"actual": a.Actual, "expected": a.Expected, "scan_date": a.ScanDate,
		})
	}
	rule := detail.Rule
	JSON(w, http.StatusOK, map[string]interface{}{
		"rule": map[string]interface{}{
			"id": rule.ID, "rule_ref": rule.RuleRef, "title": rule.Title,
			"description": rule.Description, "rationale": rule.Rationale,
			"severity": rule.Severity, "section": rule.Section, "remediation": rule.Remediation,
			"profile_id": rule.ProfileID, "profile_type": rule.ProfileType, "profile_name": rule.ProfileName,
		},
		"affected_hosts": affected,
	})
}

// TriggerScan handles POST /compliance/trigger/:hostId.
func (h *ComplianceHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	// Run scan now works for both on-demand and scheduled compliance modes.
	// Only reject if compliance is disabled for this host.
	if !host.ComplianceEnabled {
		Error(w, http.StatusBadRequest, "Compliance scanning is disabled for this host")
		return
	}
	var req struct {
		ProfileType          string  `json:"profile_type"`
		ProfileID            *string `json:"profile_id"`
		EnableRemediation    bool    `json:"enable_remediation"`
		FetchRemoteResources bool    `json:"fetch_remote_resources"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	profileType := "all"
	if req.ProfileType != "" {
		profileType = req.ProfileType
	}
	if h.integrationStatus != nil {
		_ = h.integrationStatus.ClearComplianceScanCancel(r.Context(), hostID)
	}
	task, err := queue.NewRunScanTask(queue.RunScanPayload{
		HostID:               hostID,
		ApiID:                host.ApiID,
		Host:                 hostFromRequest(r),
		ProfileType:          profileType,
		ProfileID:            req.ProfileID,
		EnableRemediation:    req.EnableRemediation,
		FetchRemoteResources: req.FetchRemoteResources,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create scan task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to enqueue scan")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Compliance scan triggered",
		"jobId":   info.ID,
		"hostId":  hostID,
	})
}

// TriggerBulkScan handles POST /compliance/trigger/bulk.
func (h *ComplianceHandler) TriggerBulkScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostIDs []string `json:"hostIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.HostIDs) == 0 {
		Error(w, http.StatusBadRequest, "hostIds required")
		return
	}
	hosts, err := h.hostsStore.GetByIDs(r.Context(), req.HostIDs)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	hostByID := make(map[string]*models.Host)
	for i := range hosts {
		hostByID[hosts[i].ID] = &hosts[i]
	}
	enqueued := 0
	for _, hostID := range req.HostIDs {
		host := hostByID[hostID]
		if host == nil {
			continue
		}
		if h.integrationStatus != nil {
			_ = h.integrationStatus.ClearComplianceScanCancel(r.Context(), hostID)
		}
		if h.queueClient == nil {
			continue
		}
		task, err := queue.NewRunScanTask(queue.RunScanPayload{
			HostID: hostID, ApiID: host.ApiID, Host: hostFromRequest(r), ProfileType: "all",
		})
		if err != nil {
			continue
		}
		if _, err := h.queueClient.Enqueue(task); err != nil {
			continue
		}
		enqueued++
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "Bulk scan triggered",
		"enqueued": enqueued,
	})
}

// CancelScan handles POST /compliance/cancel/:hostId.
// Sends cancel to agent (if connected) and removes any queued run_scan job from the queue.
func (h *ComplianceHandler) CancelScan(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	// Remove queued run_scan job so it won't run when agent connects
	if h.queueInspector != nil {
		taskID := "compliance-scan-" + hostID
		_ = h.queueInspector.DeleteTask(queue.QueueCompliance, taskID)
	}
	// Set cancel flag so worker won't re-queue if it runs before DeleteTask propagates
	if h.integrationStatus != nil {
		_ = h.integrationStatus.SetComplianceScanCancel(r.Context(), hostID)
	}
	conn := h.registry.GetConnection(host.ApiID)
	if conn != nil {
		if err := conn.WriteJSON(map[string]interface{}{"type": "compliance_scan_cancel"}); err != nil {
			Error(w, http.StatusServiceUnavailable, "Failed to send cancel to agent")
			return
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Scan cancel sent",
	})
}

// InstallScanner handles POST /compliance/install-scanner/:hostId.
func (h *ComplianceHandler) InstallScanner(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewInstallComplianceToolsTask(hostID, host.ApiID, hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create install task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to enqueue install")
		return
	}
	if h.integrationStatus != nil {
		_ = h.integrationStatus.SetComplianceInstallJob(r.Context(), hostID, info.ID)
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Install scanner queued",
		"jobId":   info.ID,
		"hostId":  hostID,
	})
}

// CancelInstall handles POST /compliance/install-scanner/:hostId/cancel.
func (h *ComplianceHandler) CancelInstall(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	jobID := ""
	if h.integrationStatus != nil {
		jobID, _ = h.integrationStatus.GetComplianceInstallJob(r.Context(), hostID)
	}
	if jobID == "" {
		JSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "No active install job",
		})
		return
	}
	_ = h.integrationStatus.SetComplianceInstallCancel(r.Context(), jobID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Cancel requested",
	})
}

// GetInstallJobStatus handles GET /compliance/install-job/:hostId.
// Returns job status plus install_events/message from integration status (agent reports to Redis).
func (h *ComplianceHandler) GetInstallJobStatus(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	jobID := ""
	if h.integrationStatus != nil {
		jobID, _ = h.integrationStatus.GetComplianceInstallJob(r.Context(), hostID)
	}
	resp := map[string]interface{}{
		"success": true,
		"jobId":   nil,
		"status":  "none",
	}
	if jobID == "" {
		JSON(w, http.StatusOK, resp)
		return
	}
	resp["jobId"] = jobID

	status := "unknown"
	if h.queueInspector != nil {
		info, err := h.queueInspector.GetTaskInfo(queue.QueueCompliance, jobID)
		if err == nil {
			raw := info.State.String()
			switch raw {
			case "pending", "scheduled":
				status = "waiting"
			case "active":
				status = "active"
			case "completed":
				status = "completed"
			case "retry":
				status = "active"
			default:
				status = raw
			}
		}
	}
	resp["status"] = status

	// Merge integration status (install_events, message, progress) from Redis for progress feedback
	if h.integrationStatus != nil {
		host, err := h.hostsStore.GetByID(r.Context(), hostID)
		if err == nil && host != nil {
			live, _ := h.integrationStatus.Get(r.Context(), host.ApiID, "compliance")
			if live != nil {
				if msg, ok := live["message"].(string); ok && msg != "" {
					resp["message"] = msg
				}
				if evts, ok := live["install_events"].([]interface{}); ok && len(evts) > 0 {
					resp["install_events"] = evts
				}
				if prog, ok := live["progress"].(float64); ok {
					resp["progress"] = prog
				}
			}
		}
	}

	JSON(w, http.StatusOK, resp)
}

// UpgradeSSG handles POST /compliance/upgrade-ssg/:hostId.
// Enqueues a per-host ssg_upgrade job so progress is tracked in job_history.
func (h *ComplianceHandler) UpgradeSSG(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	ssgVersion := h.readSSGVersion()
	if ssgVersion == "" {
		Error(w, http.StatusServiceUnavailable, "No SSG content available on server")
		return
	}
	task, err := queue.NewSSGUpgradeTask(queue.SSGUpgradePayload{
		HostID:     hostID,
		ApiID:      host.ApiID,
		Host:       hostFromRequest(r),
		SSGVersion: ssgVersion,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create upgrade task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to enqueue SSG upgrade")
		return
	}
	if h.integrationStatus != nil {
		_ = h.integrationStatus.SetSSGUpgradeJob(r.Context(), hostID, info.ID)
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "SSG upgrade queued",
		"version": ssgVersion,
		"jobId":   info.ID,
	})
}

// GetSSGUpgradeJobStatus handles GET /compliance/ssg-upgrade-job/:hostId.
// Returns the SSG upgrade job status from the queue for progress tracking.
func (h *ComplianceHandler) GetSSGUpgradeJobStatus(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	jobID := ""
	if h.integrationStatus != nil {
		jobID, _ = h.integrationStatus.GetSSGUpgradeJob(r.Context(), hostID)
	}
	if jobID == "" {
		JSON(w, http.StatusOK, map[string]interface{}{"status": "none", "message": "No SSG upgrade job found"})
		return
	}

	resp := map[string]interface{}{
		"jobId":  jobID,
		"status": "unknown",
	}

	if h.queueInspector != nil {
		if info, err := h.queueInspector.GetTaskInfo(queue.QueueCompliance, jobID); err == nil {
			switch info.State {
			case asynq.TaskStatePending:
				resp["status"] = "waiting"
				resp["message"] = "SSG upgrade queued"
			case asynq.TaskStateActive:
				resp["status"] = "active"
				resp["message"] = "SSG upgrade in progress"
			case asynq.TaskStateCompleted:
				resp["status"] = "completed"
				resp["message"] = "SSG upgrade completed"
			case asynq.TaskStateRetry:
				resp["status"] = "delayed"
				resp["message"] = "SSG upgrade will retry"
			default:
				resp["status"] = "completed"
				resp["message"] = "SSG upgrade sent to agent"
			}
		} else {
			// Task no longer in queue - assume completed.
			resp["status"] = "completed"
			resp["message"] = "SSG upgrade completed"
		}
	}

	JSON(w, http.StatusOK, resp)
}

// readSSGVersion reads the embedded SSG version from the .ssg-version marker file.
func (h *ComplianceHandler) readSSGVersion() string {
	if h.ssgContentDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(h.ssgContentDir, ".ssg-version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// listSSGFiles returns the names of all ssg-*-ds.xml files in the content dir.
func (h *ComplianceHandler) listSSGFiles() []string {
	if h.ssgContentDir == "" {
		return nil
	}
	entries, err := os.ReadDir(h.ssgContentDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && ssgFilenameRe.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	return files
}

// SSGVersion handles GET /compliance/ssg-version (agent + session auth).
func (h *ComplianceHandler) SSGVersion(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]interface{}{
		"version": h.readSSGVersion(),
		"files":   h.listSSGFiles(),
	})
}

// SSGContent handles GET /compliance/ssg-content/{filename} (agent auth).
// Serves a specific datastream file from the SSG content directory.
func (h *ComplianceHandler) SSGContent(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if !ssgFilenameRe.MatchString(filename) {
		Error(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	filePath := filepath.Join(h.ssgContentDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		Error(w, http.StatusNotFound, "Content file not found")
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	http.ServeFile(w, r, filePath)
}

// RemediateRule handles POST /compliance/remediate/:hostId.
func (h *ComplianceHandler) RemediateRule(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	var req struct {
		RuleID string `json:"rule_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RuleID == "" {
		Error(w, http.StatusBadRequest, "rule_id required")
		return
	}
	host, err := h.hostsStore.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	conn := h.registry.GetConnection(host.ApiID)
	if conn == nil {
		Error(w, http.StatusServiceUnavailable, "Agent is not connected")
		return
	}
	if err := conn.WriteJSON(map[string]interface{}{"type": "remediate_rule", "rule_id": req.RuleID}); err != nil {
		Error(w, http.StatusServiceUnavailable, "Failed to send remediate command")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Remediation triggered",
	})
}

// GetTrends returns compliance score trends for a host.
func (h *ComplianceHandler) GetTrends(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" || !isValidUUID(hostID) {
		Error(w, http.StatusBadRequest, "Invalid host ID format")
		return
	}
	days := parseIntParam(r, "days", 30, 1, 365)

	rows, err := h.complianceStore.GetTrends(r.Context(), hostID, days)
	if err != nil {
		slog.Error("compliance trends failed", "error", err, "host_id", hostID)
		Error(w, http.StatusInternalServerError, "Failed to fetch trends")
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]interface{}{
			"completed_at":        r.CompletedAt,
			"score":               r.Score,
			"compliance_profiles": map[string]interface{}{"name": r.ProfileName, "type": r.ProfileType},
		})
	}
	JSON(w, http.StatusOK, out)
}

func parseIntParam(r *http.Request, key string, defaultVal, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return defaultVal
		}
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func strParam(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}

func strParamDefault(r *http.Request, key, defaultVal string) string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	return v
}

func isValidUUID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
