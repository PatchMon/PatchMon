package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

// Model represents an AI model option.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProviderConfig holds provider metadata and models.
type ProviderConfig struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Models       []Model `json:"models"`
	DefaultModel string  `json:"defaultModel"`
}

// Provider definitions matching Node aiService.js
var providers = map[string]struct {
	name         string
	baseURL      string
	models       []Model
	defaultModel string
}{
	"openrouter": {
		name:    "OpenRouter",
		baseURL: "https://openrouter.ai/api/v1",
		models: []Model{
			{ID: "anthropic/claude-3.5-sonnet", Name: "Claude 3.5 Sonnet"},
			{ID: "anthropic/claude-3-haiku", Name: "Claude 3 Haiku"},
			{ID: "openai/gpt-4o", Name: "GPT-4o"},
			{ID: "openai/gpt-4o-mini", Name: "GPT-4o Mini"},
			{ID: "google/gemini-pro-1.5", Name: "Gemini Pro 1.5"},
			{ID: "meta-llama/llama-3.1-70b-instruct", Name: "Llama 3.1 70B"},
		},
		defaultModel: "anthropic/claude-3.5-sonnet",
	},
	"anthropic": {
		name:    "Anthropic Claude",
		baseURL: "https://api.anthropic.com/v1",
		models: []Model{
			{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4"},
			{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"},
			{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku"},
		},
		defaultModel: "claude-sonnet-4-20250514",
	},
	"openai": {
		name:    "OpenAI",
		baseURL: "https://api.openai.com/v1",
		models: []Model{
			{ID: "gpt-4o", Name: "GPT-4o"},
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini"},
			{ID: "gpt-4-turbo", Name: "GPT-4 Turbo"},
		},
		defaultModel: "gpt-4o-mini",
	},
	"gemini": {
		name:    "Google Gemini",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		models: []Model{
			{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro"},
			{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash"},
			{ID: "gemini-2.0-flash-exp", Name: "Gemini 2.0 Flash"},
		},
		defaultModel: "gemini-1.5-flash",
	},
}

const (
	systemPromptAssistant = `You are a helpful terminal assistant integrated into PatchMon, a server management tool.
Your role is to help system administrators understand terminal output, diagnose issues, and suggest solutions.

Guidelines:
- Be concise and direct in your responses
- When explaining errors, provide the likely cause and solution
- Suggest specific commands when appropriate
- Format command suggestions in code blocks
- If you're unsure, say so rather than guessing
- Focus on Linux/Unix system administration topics`

	systemPromptCompletion = `You are a terminal command completion assistant. Given the current command being typed and recent terminal context, suggest the most likely command completion.

Guidelines:
- Only respond with the completion text (what comes after the cursor)
- If multiple completions are possible, choose the most common/useful one
- Keep completions practical and safe
- Do not include explanations, just the completion
- If no good completion exists, respond with an empty string
- Consider the terminal history for context`
)

// Service provides AI assistance and completion.
type Service struct {
	enc *util.Encryption
}

// NewService creates a new AI service.
func NewService(enc *util.Encryption) *Service {
	return &Service{enc: enc}
}

// GetProviders returns available providers and models (no secrets).
func (s *Service) GetProviders() []ProviderConfig {
	out := make([]ProviderConfig, 0, len(providers))
	for id, cfg := range providers {
		out = append(out, ProviderConfig{
			ID:           id,
			Name:         cfg.name,
			Models:       cfg.models,
			DefaultModel: cfg.defaultModel,
		})
	}
	return out
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// callOptions holds provider call parameters.
type callOptions struct {
	maxTokens   int
	temperature float64
}

// GetAssistance returns AI assistance for terminal questions.
func (s *Service) GetAssistance(settings *models.Settings, question, context string, history []Message) (string, error) {
	return s.callAI(settings, question, &callOptions{
		maxTokens:   1024,
		temperature: 0.7,
	}, systemPromptAssistant, context, history)
}

// GetCompletion returns a command completion suggestion.
func (s *Service) GetCompletion(settings *models.Settings, input, context string) (string, error) {
	if len(input) < 2 {
		return "", nil
	}
	prompt := `Current command being typed: "` + input + `"
Complete this command. Only respond with the remaining text to add, nothing else.`

	response, err := s.callAI(settings, prompt, &callOptions{
		maxTokens:   100,
		temperature: 0.3,
	}, systemPromptCompletion, context, nil)
	if err != nil {
		return "", err
	}
	// Clean up: remove quotes, newlines
	trimmed := strings.TrimSpace(response)
	trimmed = regexp.MustCompile(`^["']|["']$`).ReplaceAllString(trimmed, "")
	return trimmed, nil
}

func (s *Service) callAI(settings *models.Settings, prompt string, opts *callOptions, systemPrompt, context string, history []Message) (string, error) {
	if s.enc == nil {
		return "", fmt.Errorf("encryption not configured")
	}
	if settings.AiAPIKey == nil || *settings.AiAPIKey == "" {
		return "", fmt.Errorf("AI API key not configured")
	}
	apiKey, err := s.enc.Decrypt(*settings.AiAPIKey)
	if err != nil || apiKey == "" {
		return "", fmt.Errorf("failed to decrypt AI API key")
	}

	messages := []Message{{Role: "system", Content: systemPrompt}}
	if context != "" {
		messages = append(messages,
			Message{Role: "user", Content: "Recent terminal output:\n```\n" + context + "\n```"},
			Message{Role: "assistant", Content: "I've noted the terminal context. How can I help?"},
		)
	}
	messages = append(messages, history...)
	messages = append(messages, Message{Role: "user", Content: prompt})

	provider := settings.AiProvider
	if provider == "" {
		provider = "openrouter"
	}
	model := ""
	if settings.AiModel != nil {
		model = *settings.AiModel
	}

	switch provider {
	case "openrouter":
		return callOpenRouter(apiKey, model, messages, opts)
	case "anthropic":
		return callAnthropic(apiKey, model, messages, opts)
	case "openai":
		return callOpenAI(apiKey, model, messages, opts)
	case "gemini":
		return callGemini(apiKey, model, messages, opts)
	default:
		return "", fmt.Errorf("unknown AI provider: %s", provider)
	}
}

// OpenRouter
func callOpenRouter(apiKey, model string, messages []Message, opts *callOptions) (string, error) {
	cfg := providers["openrouter"]
	if model == "" {
		model = cfg.defaultModel
	}
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  opts.maxTokens,
		"temperature": opts.temperature,
		"stream":      false,
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", cfg.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://patchmon.app")
	req.Header.Set("X-Title", "PatchMon Terminal Assistant")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter API error: %d - %s", resp.StatusCode, string(data))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) > 0 {
		return out.Choices[0].Message.Content, nil
	}
	return "", nil
}

// Anthropic
func callAnthropic(apiKey, model string, messages []Message, opts *callOptions) (string, error) {
	cfg := providers["anthropic"]
	if model == "" {
		model = cfg.defaultModel
	}
	var systemMsg string
	var conv []Message
	for _, m := range messages {
		if m.Role == "system" {
			systemMsg = m.Content
		} else {
			conv = append(conv, m)
		}
	}
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": opts.maxTokens,
		"system":     systemMsg,
		"messages":   conv,
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", cfg.baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error: %d - %s", resp.StatusCode, string(data))
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Content) > 0 {
		return out.Content[0].Text, nil
	}
	return "", nil
}

// OpenAI
func callOpenAI(apiKey, model string, messages []Message, opts *callOptions) (string, error) {
	cfg := providers["openai"]
	if model == "" {
		model = cfg.defaultModel
	}
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  opts.maxTokens,
		"temperature": opts.temperature,
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", cfg.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(data))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) > 0 {
		return out.Choices[0].Message.Content, nil
	}
	return "", nil
}

// Gemini
func callGemini(apiKey, model string, messages []Message, opts *callOptions) (string, error) {
	cfg := providers["gemini"]
	if model == "" {
		model = cfg.defaultModel
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}
	var contents []content
	var systemInstruction *struct {
		Parts []part `json:"parts"`
	}
	for _, m := range messages {
		if m.Role == "system" {
			systemInstruction = &struct {
				Parts []part `json:"parts"`
			}{Parts: []part{{Text: m.Content}}}
		} else {
			role := "user"
			if m.Role == "assistant" {
				role = "model"
			}
			contents = append(contents, content{Role: role, Parts: []part{{Text: m.Content}}})
		}
	}
	body := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": opts.maxTokens,
			"temperature":     opts.temperature,
		},
	}
	if systemInstruction != nil {
		body["systemInstruction"] = systemInstruction
	}
	reqBody, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", cfg.baseURL, model, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error: %d - %s", resp.StatusCode, string(data))
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Candidates) > 0 && len(out.Candidates[0].Content.Parts) > 0 {
		return out.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", nil
}
