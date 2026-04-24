package handler

import (
	"io"
	"net/http"
	"net/url"
	"strings"
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

	form := url.Values{}
	form.Set("NAME", name)
	form.Set("EMAIL", email)
	form.Set(honeypotField, "")

	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, marketingSubscribeURL, strings.NewReader(form.Encode()))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create request")
		return
	}
	proxyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		Error(w, http.StatusBadGateway, "Failed to reach marketing service")
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		JSON(w, http.StatusOK, map[string]string{"message": "Subscribed successfully"})
		return
	}
	Error(w, http.StatusBadGateway, "Marketing service returned an error")
}
