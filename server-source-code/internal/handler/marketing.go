package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

const marketingSubscribeURL = "https://marketing.9.technology/index.php/lists/lq880fhpl44b6/subscribe"
const honeypotField = "62f7db5ee090a53e362464fbe0341d7154a992bb"

// MarketingHandler handles marketing-related routes (e.g. newsletter subscribe proxy).
type MarketingHandler struct{}

// NewMarketingHandler creates a new marketing handler.
func NewMarketingHandler() *MarketingHandler {
	return &MarketingHandler{}
}

// SubscribeRequest is the request body for POST /marketing/subscribe.
type SubscribeRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SubscribeToList POSTs name+email to the configured Listmonk public list URL.
// Returns nil on 2xx, a wrapped error otherwise.
func SubscribeToList(ctx context.Context, name, email string) error {
	form := url.Values{}
	form.Set("NAME", name)
	form.Set("EMAIL", email)
	form.Set(honeypotField, "")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, marketingSubscribeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post to marketing list: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("marketing list returned status %d", resp.StatusCode)
	}
	return nil
}

// AutoSubscribeIfHosted enrolls a freshly-created user in the newsletter when
// the instance is running in admin_mode (managed/SaaS deployments). On
// self-hosted instances (admin_mode off) this is a no-op so users keep their
// opt-in choice. Runs in a detached goroutine: the request handler returns
// immediately and a Listmonk hiccup never blocks user creation. Errors are
// logged at warn-level only.
//
// Call after a successful user insert with a populated u.ID and u.Email.
func AutoSubscribeIfHosted(adminMode bool, users *store.UsersStore, log *slog.Logger, u *models.User) {
	if !adminMode || users == nil || u == nil || u.Email == "" {
		return
	}
	// Discord auto-creation synthesises `discord_<id>@discord.local` when
	// the user hasn't shared a real email. Subscribing those produces hard
	// bounces and hurts the marketing sender's domain reputation, so skip.
	if strings.HasSuffix(strings.ToLower(u.Email), "@discord.local") {
		return
	}
	// Snapshot the user fields the goroutine needs so it never races with
	// later mutations of the caller's *models.User.
	id := u.ID
	email := u.Email
	name := newsletterDisplayName(u)

	go func() {
		// Detach from the request context (which dies when the handler
		// returns) but keep a hard ceiling so a slow Listmonk can't pin a
		// goroutine forever.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := SubscribeToList(ctx, name, email); err != nil {
			if log != nil {
				log.Warn("auto newsletter subscribe failed", "user_id", id, "error", err)
			}
			return
		}
		if err := users.SetNewsletterSubscribed(ctx, id); err != nil {
			if log != nil {
				log.Warn("auto newsletter flag update failed", "user_id", id, "error", err)
			}
		}
	}()
}

// Subscribe proxies the newsletter subscription to the external marketing system.
// Public endpoint - no auth required (used during first-time setup).
func (h *MarketingHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req SubscribeRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(req.Email)
	if name == "" || email == "" {
		Error(w, http.StatusBadRequest, "Name and email are required")
		return
	}

	if err := SubscribeToList(r.Context(), name, email); err != nil {
		Error(w, http.StatusBadGateway, "Failed to reach marketing service")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"message": "Subscribed successfully"})
}
