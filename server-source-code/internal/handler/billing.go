// Package handler - billing.go implements the two thin proxy endpoints that back
// the PatchMon-native Billing page (/api/v1/me/billing and /api/v1/me/billing/portal).
//
// Both endpoints are double-gated:
//  1. config.AdminMode must be true (ADMIN_MODE=on). Self-hosted installs always
//     get a 404 so the Billing surface area is effectively invisible.
//  2. The caller's role must have can_manage_billing = true.
//
// If either gate fails we return 404 (not 403) so the endpoint is indistinguishable
// from "does not exist". Gating is done inside the handler rather than as middleware
// so both failure modes collapse to the same status code.
//
// Task 12 landed the tier-change flow: /me/billing/tier-change (mutating) and
// /me/billing/tier-change/preview (read-only upcoming-invoice preview). Both
// inject the caller's context id server-side so a user cannot target another
// workspace's subscription by passing a different tenant_id.
package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// BillingHandler proxies the PatchMon-native Billing page to the internal billing service.
type BillingHandler struct {
	cfg   *config.Config
	http  *http.Client
	log   *slog.Logger
	hosts *store.HostsStore // live active-host count for the Billing page
}

// NewBillingHandler creates a new BillingHandler.
func NewBillingHandler(cfg *config.Config, log *slog.Logger, hosts *store.HostsStore) *BillingHandler {
	return &BillingHandler{
		cfg:   cfg,
		http:  &http.Client{Timeout: 10 * time.Second},
		log:   log,
		hosts: hosts,
	}
}

// gateOrNotFound checks AdminMode + can_manage_billing permission. Returns true if both pass.
// On failure writes a 404 so the endpoint is indistinguishable from "not found".
// Also returns 503 if AdminMode is on but BILLING_SERVICE_URL is not configured,
// because that's an operator error worth surfacing rather than silently 404'ing.
func (h *BillingHandler) gateOrNotFound(w http.ResponseWriter, r *http.Request, permissions *store.PermissionsStore) bool {
	if h.cfg == nil || !h.cfg.AdminMode {
		Error(w, http.StatusNotFound, "Not found")
		return false
	}
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if role == "" {
		// No authenticated role - surface as 404 (not 401) to stay consistent.
		Error(w, http.StatusNotFound, "Not found")
		return false
	}
	p, err := permissions.GetByRole(r.Context(), role)
	if err != nil || p == nil || !p.CanManageBilling {
		Error(w, http.StatusNotFound, "Not found")
		return false
	}
	if strings.TrimSpace(h.cfg.BillingServiceURL) == "" {
		h.log.Warn("billing endpoint called but BILLING_SERVICE_URL is not configured")
		Error(w, http.StatusServiceUnavailable, "Billing service is not configured")
		return false
	}
	return true
}

// currentTenantID returns the context ("tenant") id from the multi-context registry.
// In managed/cloud mode (AdminMode=on) this should always be populated because the
// hostctx middleware resolves it from the request Host.
func currentTenantID(r *http.Request) string {
	entry := hostctx.EntryFromContext(r.Context())
	if entry == nil {
		return ""
	}
	return entry.ID
}

// GetMyBilling returns an http.HandlerFunc for GET /me/billing. It fetches the
// current context's subscription from the billing service and forwards the response.
//
// On success returns the JSON shape documented in billing service's
// subscriptionCurrentResponse (tier, quantity, next_invoice_cents, etc.).
// On either gate failure (AdminMode off OR permission denied) returns 404.
// On billing-service 404 (no subscription linked to this context) the frontend
// should render a "billing not available" fallback.
func (h *BillingHandler) GetMyBilling(permissions *store.PermissionsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gateOrNotFound(w, r, permissions) {
			return
		}
		tenantID := currentTenantID(r)
		if tenantID == "" {
			// Managed deployment but no context entry - typical for single-context
			// admin access. Return 404 so the frontend hides the page.
			Error(w, http.StatusNotFound, "Billing not available for this context")
			return
		}

		base := strings.TrimSuffix(h.cfg.BillingServiceURL, "/")
		target := base + "/billing/subscription/current?tenant_id=" + url.QueryEscape(tenantID)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			h.log.Error("billing proxy build request", "err", err)
			Error(w, http.StatusInternalServerError, "Failed to build billing request")
			return
		}
		if h.cfg.BillingInternalSecret != "" {
			req.Header.Set("X-Billing-Secret", h.cfg.BillingInternalSecret)
		}

		resp, err := h.http.Do(req)
		if err != nil {
			h.log.Error("billing proxy request", "err", err)
			Error(w, http.StatusBadGateway, "Billing service unreachable")
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		if err != nil {
			h.log.Error("billing proxy read body", "err", err)
			Error(w, http.StatusBadGateway, "Failed to read billing response")
			return
		}

		// Inject live active_host_count from the tenant's own DB. Billing
		// service only knows the Stripe billed quantity; the UX spec (Phase 5e)
		// wants the card to distinguish "what you actually run" from "what
		// you're billed for". On any non-2xx we pass the upstream body through
		// unchanged so error shapes stay predictable for the frontend.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && h.hosts != nil {
			if active, cerr := h.hosts.Count(r.Context()); cerr == nil {
				var payload map[string]interface{}
				if jerr := json.Unmarshal(body, &payload); jerr == nil && payload != nil {
					payload["active_host_count"] = active
					if merged, mErr := json.Marshal(payload); mErr == nil {
						body = merged
					}
				}
			} else {
				h.log.Warn("billing page active-host count failed", "err", cerr)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}
}

// GetMyBillingPortal returns an http.HandlerFunc for POST /me/billing/portal.
// Exchanges a {return_url} for a Stripe billing portal URL scoped to the current context.
func (h *BillingHandler) GetMyBillingPortal(permissions *store.PermissionsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gateOrNotFound(w, r, permissions) {
			return
		}
		tenantID := currentTenantID(r)
		if tenantID == "" {
			Error(w, http.StatusNotFound, "Billing not available for this context")
			return
		}

		var in struct {
			ReturnURL string `json:"return_url"`
		}
		// Missing/invalid body is acceptable; billing service fills in a default return URL.
		_ = decodeJSON(r, &in)

		payload, err := json.Marshal(map[string]string{
			"tenant_id":  tenantID,
			"return_url": strings.TrimSpace(in.ReturnURL),
		})
		if err != nil {
			Error(w, http.StatusInternalServerError, "Failed to encode billing request")
			return
		}

		base := strings.TrimSuffix(h.cfg.BillingServiceURL, "/")
		target := base + "/billing/portal-session"

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(payload))
		if err != nil {
			h.log.Error("billing portal proxy build request", "err", err)
			Error(w, http.StatusInternalServerError, "Failed to build billing request")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if h.cfg.BillingInternalSecret != "" {
			req.Header.Set("X-Billing-Secret", h.cfg.BillingInternalSecret)
		}

		resp, err := h.http.Do(req)
		if err != nil {
			h.log.Error("billing portal proxy request", "err", err)
			Error(w, http.StatusBadGateway, "Billing service unreachable")
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		if err != nil {
			h.log.Error("billing portal proxy read body", "err", err)
			Error(w, http.StatusBadGateway, "Failed to read billing response")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}
}

// tierChangeInput is the body accepted by both /me/billing/tier-change and
// /me/billing/tier-change/preview. The context id is NEVER accepted from the
// caller - we inject it server-side so a user with can_manage_billing on
// context A cannot target context B's subscription by spoofing the payload.
type tierChangeInput struct {
	NewTier  string `json:"new_tier"`
	Interval string `json:"interval"`
}

// proxyTierChange forwards a tier-change request to the billing service. The
// preview flag selects the /tier-change/preview vs /tier-change path; both
// endpoints share the same request body shape (with tenant_id injected here).
func (h *BillingHandler) proxyTierChange(permissions *store.PermissionsStore, preview bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gateOrNotFound(w, r, permissions) {
			return
		}
		tenantID := currentTenantID(r)
		if tenantID == "" {
			Error(w, http.StatusNotFound, "Billing not available for this context")
			return
		}

		var in tierChangeInput
		if err := decodeJSON(r, &in); err != nil {
			Error(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}
		newTier := strings.ToLower(strings.TrimSpace(in.NewTier))
		interval := strings.ToLower(strings.TrimSpace(in.Interval))
		// Normalise legacy labels so the frontend can send either form.
		switch interval {
		case "monthly":
			interval = "month"
		case "annual", "yearly":
			interval = "year"
		}
		if newTier != "starter" && newTier != "plus" && newTier != "max" {
			Error(w, http.StatusBadRequest, "new_tier must be one of: starter, plus, max")
			return
		}
		if interval != "month" && interval != "year" {
			Error(w, http.StatusBadRequest, "interval must be 'month' or 'year'")
			return
		}

		payload, err := json.Marshal(map[string]string{
			"tenant_id": tenantID,
			"new_tier":  newTier,
			"interval":  interval,
		})
		if err != nil {
			Error(w, http.StatusInternalServerError, "Failed to encode billing request")
			return
		}

		base := strings.TrimSuffix(h.cfg.BillingServiceURL, "/")
		target := base + "/billing/tier-change"
		if preview {
			target += "/preview"
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(payload))
		if err != nil {
			h.log.Error("tier-change proxy build request", "err", err, "preview", preview)
			Error(w, http.StatusInternalServerError, "Failed to build billing request")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if h.cfg.BillingInternalSecret != "" {
			req.Header.Set("X-Billing-Secret", h.cfg.BillingInternalSecret)
		}

		resp, err := h.http.Do(req)
		if err != nil {
			h.log.Error("tier-change proxy request", "err", err, "preview", preview)
			Error(w, http.StatusBadGateway, "Billing service unreachable")
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if err != nil {
			h.log.Error("tier-change proxy read body", "err", err, "preview", preview)
			Error(w, http.StatusBadGateway, "Failed to read billing response")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}
}

// PostMyBillingTierChange returns the handler for POST /me/billing/tier-change.
// Applies a tier/interval change to the current context's subscription.
func (h *BillingHandler) PostMyBillingTierChange(permissions *store.PermissionsStore) http.HandlerFunc {
	return h.proxyTierChange(permissions, false)
}

// PostMyBillingTierChangePreview returns the handler for POST /me/billing/tier-change/preview.
// Returns what the charge / effective date would be without mutating Stripe.
func (h *BillingHandler) PostMyBillingTierChangePreview(permissions *store.PermissionsStore) http.HandlerFunc {
	return h.proxyTierChange(permissions, true)
}

// PostMyBillingSync returns an http.HandlerFunc for POST /me/billing/sync.
//
// This is the tenant-facing "Sync host count" action. Flow:
//  1. Double-gate (AdminMode + can_manage_billing) same as the other
//     billing endpoints; failures collapse to 404.
//  2. POST to the regional provisioner's internal sync-usage endpoint.
//     The provisioner performs a live count on the tenant DB, forwards
//     the result to the manager, and the manager updates Stripe.
//  3. The provisioner's JSON response (including the freshly-projected
//     billing_state block) is forwarded verbatim to the caller so the
//     frontend can optimistically update without waiting for the next
//     GetMyBilling poll.
//
// Timeout: 30 seconds. The happy path is usually 2-3 seconds but live
// count + manager push + Stripe mutation can peak at 5-10s.
//
// Auth to the provisioner: X-Registry-Reload-Secret (shared secret that
// already exists on the server via REGISTRY_RELOAD_SECRET). If the
// PROVISIONER_URL env is not configured we surface 503 rather than
// silently 404-ing because that's an operator misconfiguration.
func (h *BillingHandler) PostMyBillingSync(permissions *store.PermissionsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gateOrNotFound(w, r, permissions) {
			return
		}
		tenantID := currentTenantID(r)
		if tenantID == "" {
			Error(w, http.StatusNotFound, "Billing not available for this context")
			return
		}
		if strings.TrimSpace(h.cfg.ProvisionerURL) == "" {
			h.log.Warn("billing sync called but PROVISIONER_URL is not configured")
			Error(w, http.StatusServiceUnavailable, "Provisioner is not configured")
			return
		}

		base := strings.TrimSuffix(h.cfg.ProvisionerURL, "/")
		target := base + "/provisioner/api/internal/tenants/" + url.PathEscape(tenantID) + "/sync-usage"

		// Use a per-request client with a longer timeout; the live count +
		// manager push + Stripe mutation can take 5-10s in the worst case,
		// and the shared h.http client is tuned for the faster subscription
		// read/portal calls.
		client := &http.Client{Timeout: 30 * time.Second}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, nil)
		if err != nil {
			h.log.Error("billing sync proxy build request", "err", err)
			Error(w, http.StatusInternalServerError, "Failed to build sync request")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if h.cfg.RegistryReloadSecret != "" {
			req.Header.Set("X-Registry-Reload-Secret", h.cfg.RegistryReloadSecret)
		}

		resp, err := client.Do(req)
		if err != nil {
			h.log.Error("billing sync proxy request", "err", err)
			Error(w, http.StatusBadGateway, "Provisioner unreachable")
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if err != nil {
			h.log.Error("billing sync proxy read body", "err", err)
			Error(w, http.StatusBadGateway, "Failed to read sync response")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}
}
