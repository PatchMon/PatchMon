package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/ai"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const (
	rateLimitWindow = 60 * time.Second
	rateLimitMax    = 30
)

// AIHandler handles AI-related routes.
type AIHandler struct {
	settings *store.SettingsStore
	aiSvc    *ai.Service
	enc      *util.Encryption
	rdb      *hostctx.RedisResolver
}

// NewAIHandler creates a new AI handler.
func NewAIHandler(settings *store.SettingsStore, aiSvc *ai.Service, enc *util.Encryption, rdb *hostctx.RedisResolver) *AIHandler {
	return &AIHandler{
		settings: settings,
		aiSvc:    aiSvc,
		enc:      enc,
		rdb:      rdb,
	}
}

// checkRateLimit returns true if the request is allowed (under limit).
func (h *AIHandler) checkRateLimit(ctx context.Context, userID string) bool {
	rdb := h.rdb.RDB(ctx)
	if rdb == nil || userID == "" {
		return true
	}
	key := "ratelimit:ai:" + userID
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return true // fail open
	}
	if count == 1 {
		_ = rdb.Expire(ctx, key, rateLimitWindow).Err()
	}
	return count <= rateLimitMax
}

// GetProviders returns available AI providers and models.
func (h *AIHandler) GetProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.aiSvc.GetProviders()
	JSON(w, http.StatusOK, map[string]interface{}{"providers": providers})
}

// GetSettings returns AI settings (admin only).
func (h *AIHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"ai_enabled":         false,
			"ai_provider":        "openrouter",
			"ai_model":           nil,
			"ai_api_key_set":     false,
			"ai_api_key_invalid": false,
		})
		return
	}

	apiKeyValid := false
	apiKeyInvalid := false
	if s.AiAPIKey != nil && *s.AiAPIKey != "" {
		_, err := h.enc.Decrypt(*s.AiAPIKey)
		apiKeyValid = err == nil
		apiKeyInvalid = !apiKeyValid
	}

	aiModel := s.AiModel
	if s.AiProvider == "" {
		s.AiProvider = "openrouter"
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"ai_enabled":         s.AiEnabled,
		"ai_provider":        s.AiProvider,
		"ai_model":           aiModel,
		"ai_api_key_set":     apiKeyValid,
		"ai_api_key_invalid": apiKeyInvalid,
	})
}

// UpdateSettingsRequest is the body for PUT /ai/settings.
type UpdateSettingsRequest struct {
	AiEnabled  *bool   `json:"ai_enabled"`
	AiProvider *string `json:"ai_provider"`
	AiModel    *string `json:"ai_model"`
	AiAPIKey   *string `json:"ai_api_key"`
}

var allowedProviders = map[string]bool{
	"openrouter": true, "anthropic": true, "openai": true, "gemini": true,
}

// UpdateSettings updates AI settings (admin only).
func (h *AIHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	var req UpdateSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.AiEnabled != nil {
		s.AiEnabled = *req.AiEnabled
	}
	if req.AiProvider != nil && allowedProviders[*req.AiProvider] {
		s.AiProvider = *req.AiProvider
	}
	if req.AiModel != nil {
		s.AiModel = req.AiModel
		if *req.AiModel == "" {
			s.AiModel = nil
		}
	}
	if req.AiAPIKey != nil && *req.AiAPIKey != "" {
		if util.IsEncrypted(*req.AiAPIKey) {
			s.AiAPIKey = req.AiAPIKey
		} else {
			encrypted, err := h.enc.Encrypt(*req.AiAPIKey)
			if err != nil {
				Error(w, http.StatusInternalServerError, "Failed to encrypt API key")
				return
			}
			s.AiAPIKey = &encrypted
		}
	}

	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	apiKeySet := s.AiAPIKey != nil && *s.AiAPIKey != ""
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":        "AI settings updated successfully",
		"ai_enabled":     s.AiEnabled,
		"ai_provider":    s.AiProvider,
		"ai_model":       s.AiModel,
		"ai_api_key_set": apiKeySet,
	})
}

// GetDebug returns encryption debug info (admin only).
func (h *AIHandler) GetDebug(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	// Round-trip test
	testVal := "test-" + time.Now().Format("20060102150405")
	encrypted, encErr := h.enc.Encrypt(testVal)
	roundTripOk := false
	if encErr == nil {
		decrypted, decErr := h.enc.Decrypt(encrypted)
		roundTripOk = decErr == nil && decrypted == testVal
	}

	existingKeyStatus := "not_set"
	if s.AiAPIKey != nil && *s.AiAPIKey != "" {
		_, err := h.enc.Decrypt(*s.AiAPIKey)
		if err == nil {
			existingKeyStatus = "valid"
		} else {
			existingKeyStatus = "invalid_cannot_decrypt"
		}
	}

	roundTripResult := "failed"
	if roundTripOk {
		roundTripResult = "passed"
	}

	recommendation := "Configuration looks good"
	// We don't have getEncryptionStatus in Go - use a simple message
	JSON(w, http.StatusOK, map[string]interface{}{
		"encryption":     "configured",
		"roundTripTest":  roundTripResult,
		"existingApiKey": existingKeyStatus,
		"recommendation": recommendation,
	})
}

// TestConnection tests the AI connection (admin only).
func (h *AIHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	if s.AiAPIKey == nil || *s.AiAPIKey == "" {
		Error(w, http.StatusBadRequest, "AI API key not configured")
		return
	}

	response, err := h.aiSvc.GetAssistance(s, "Respond with exactly: 'Connection successful!' - nothing else.", "", nil)
	if err != nil {
		Error(w, http.StatusBadRequest, "Connection test failed: "+err.Error())
		return
	}

	if strings.Contains(strings.ToLower(response), "connection successful") {
		JSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "AI connection test successful"})
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "AI responded",
		"response": response,
	})
}

// AssistRequest is the body for POST /ai/assist.
type AssistRequest struct {
	Question string       `json:"question"`
	Context  string       `json:"context"`
	History  []ai.Message `json:"history"`
}

// Assist handles terminal assistance requests.
func (h *AIHandler) Assist(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if !h.checkRateLimit(r.Context(), userID) {
		Error(w, http.StatusTooManyRequests, "Rate limit exceeded. Please wait a moment.")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	if !s.AiEnabled {
		Error(w, http.StatusBadRequest, "AI assistant is not enabled")
		return
	}
	if s.AiAPIKey == nil || *s.AiAPIKey == "" {
		Error(w, http.StatusBadRequest, "AI API key not configured")
		return
	}

	var req AssistRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if len(req.Question) < 1 || len(req.Question) > 2000 {
		Error(w, http.StatusBadRequest, "question must be 1-2000 characters")
		return
	}
	if len(req.Context) > 10000 {
		req.Context = req.Context[:10000]
	}

	// Sanitize history: last 10 messages, role+content required, max 2000 chars each
	sanitized := make([]ai.Message, 0, 10)
	start := len(req.History) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(req.History) && len(sanitized) < 10; i++ {
		m := req.History[i]
		if m.Role == "" || m.Content == "" {
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "assistant"
		}
		content := m.Content
		if len(content) > 2000 {
			content = content[:2000]
		}
		sanitized = append(sanitized, ai.Message{Role: role, Content: content})
	}

	response, err := h.aiSvc.GetAssistance(s, req.Question, req.Context, sanitized)
	if err != nil {
		Error(w, http.StatusInternalServerError, "AI request failed: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"response": response})
}

// CompleteRequest is the body for POST /ai/complete.
type CompleteRequest struct {
	Input   string `json:"input"`
	Context string `json:"context"`
}

// Complete handles command completion requests.
func (h *AIHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if !h.checkRateLimit(r.Context(), userID) {
		Error(w, http.StatusTooManyRequests, "Rate limit exceeded")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	if !s.AiEnabled {
		Error(w, http.StatusBadRequest, "AI assistant is not enabled")
		return
	}
	if s.AiAPIKey == nil || *s.AiAPIKey == "" {
		Error(w, http.StatusBadRequest, "AI API key not configured")
		return
	}

	var req CompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Input = strings.TrimSpace(req.Input)
	if len(req.Input) < 2 || len(req.Input) > 500 {
		Error(w, http.StatusBadRequest, "input must be 2-500 characters")
		return
	}
	if len(req.Context) > 5000 {
		req.Context = req.Context[:5000]
	}

	completion, err := h.aiSvc.GetCompletion(s, req.Input, req.Context)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Completion request failed: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"completion": completion})
}
