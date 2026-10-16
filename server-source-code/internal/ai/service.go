package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

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
			{ID: "anthropic/claude-sonnet-5.5", Name: "Claude Sonnet 5.5"},
			{ID: "anthropic/claude-haiku-5.5", Name: "Claude Haiku 5.5"},
			{ID: "openai/gpt-4o", Name: "GPT-4o"},
			{ID: "openai/gpt-4o-mini", Name: "GPT-4o Mini"},
			{ID: "google/gemini-3.8-flash", Name: "Gemini 3.8 Flash"},
			{ID: "meta-llama/llama-3.1-70b-instruct", Name: "Llama 3.1 70B"},
		},
		defaultModel: "anthropic/claude-sonnet-5.5",
	},
	"anthropic": {
		name:    "Anthropic Claude",
		baseURL: "https://api.anthropic.com/v1",
		models: []Model{
			{ID: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5"},
			{ID: "claude-opus-5-5", Name: "Claude Opus 5.5"},
			{ID: "claude-haiku-5-5", Name: "Claude Haiku 5.5"},
		},
		defaultModel: "claude-sonnet-5-5",
	},
	"openai": {
		name:    "OpenAI",
		baseURL: "https://api.openai.com/v1",
		models: []Model{
			{ID: "gpt-4o", Name: "GPT-4o"},
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini"},
			{ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol"},
		},
		defaultModel: "gpt-4o-mini",
	},
	"gemini": {
		name:    "Google Gemini",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		models: []Model{
			{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite"},
			{ID: "gemini-3.6-flash", Name: "Gemini 3.6 Flash"},
			{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"},
		},
		defaultModel: "gemini-3.5-flash-lite",
	},
}

var retiredModels = map[string]map[string]bool{
	"openrouter": {
		"anthropic/claude-3.5-sonnet": true,
		"anthropic/claude-3-haiku":    true,
		"google/gemini-pro-1.5":       true,
		"google/gemini-2.5-flash":     true,
	},
	"anthropic": {
		"claude-sonnet-4-20250514":   true,
		"claude-3-5-sonnet-20241022": true,
		"claude-3-5-haiku-20241022":  true,
	},
	"openai": {
		"gpt-4-turbo": true,
	},
	"gemini": {
		"gemini-1.5-pro":       true,
		"gemini-1.5-flash":     true,
		"gemini-2.0-flash-exp": true,
	},
}

var anthropicThinking = map[string]map[string]string{
	"claude-sonnet-5-5": {"type": "between_tools"},
	"claude-haiku-5-5":  {"type": "disabled"},
}

var anthropicMinMaxTokens = map[string]int{
	"claude-opus-5-5": 16000,
}

var reasoningEffort = map[string]map[string]string{
	"openrouter": {
		"google/gemini-3.8-flash": "low",
	},
	"openai": {
		"gpt-5.6-sol": "low",
	},
	"gemini": {
		"gemini-3.5-flash-lite": "MINIMAL",
		"gemini-3.6-flash":      "MINIMAL",
		"gemini-3.8-flash":      "LOW",
	},
}

// Reasoning tokens count against the output limit, so a small one can return no text.
const reasoningMinMaxTokens = 4096

// ResolveModel returns the saved model, or the provider default when the saved
// model is unset or has been retired by the provider.
func ResolveModel(provider, model string) string {
	cfg, ok := providers[provider]
	if !ok {
		return model
	}
	if model == "" || retiredModels[provider][model] {
		return cfg.defaultModel
	}
	return model
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

	systemPromptCompletion = `You complete shell commands as the user types them, like a terminal autocomplete.

You receive the text typed so far inside <typed> tags, and sometimes recent terminal output for context. Reply with the single most likely complete command line: it must start with the typed text exactly as typed, character for character, followed by the completion.

Rules:
- Reply with the command line only, on one line. No explanation, no quotes, no backticks, no code blocks.
- Never change, fix or reorder the typed text; only add to the end of it.
- If the typed text ends partway through a word, finish that word.
- Prefer the most common, safe usage on Linux servers. Never suggest destructive commands (such as rm -rf /, mkfs or dd to a disk) unless the typed text already leads there.
- If there is no sensible completion, reply with the typed text unchanged.

Examples:
<typed>systemctl rest</typed>
systemctl restart
<typed>docker ps </typed>
docker ps -a
<typed>journalctl -u nginx</typed>
journalctl -u nginx -f
<typed>ssh</typed>
ssh-keygen
<typed>apt list --upgr</typed>
apt list --upgradable`
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
	if len(strings.TrimSpace(input)) < 2 {
		return "", nil
	}
	response, err := s.callAI(settings, "<typed>"+input+"</typed>", &callOptions{
		maxTokens:   100,
		temperature: 0.2,
	}, systemPromptCompletion, context, nil)
	if err != nil {
		return "", err
	}
	return normaliseCompletion(input, response), nil
}

const maxCompletionLen = 200

func unwrapCompletion(s string) string {
	if len(s) >= 2 && (s[0] == '`' || s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func normaliseCompletion(input, response string) string {
	out := strings.TrimLeft(response, "\r\n")
	if i := strings.IndexAny(out, "\r\n"); i >= 0 {
		out = out[:i]
	}
	out = strings.TrimLeft(strings.TrimRight(out, " \t"), " \t")
	if strings.HasPrefix(out, "```") {
		return ""
	}
	suffix, ok := completionSuffix(input, out)
	if !ok {
		suffix, ok = completionSuffix(input, strings.TrimRight(unwrapCompletion(out), " \t"))
	}
	if !ok || len(suffix) > maxCompletionLen || strings.IndexFunc(suffix, unsafeCompletionRune) >= 0 {
		return ""
	}
	return suffix
}

func unsafeCompletionRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029'
}

func completionSuffix(input, out string) (string, bool) {
	trimmedInput := strings.TrimRight(input, " \t")
	var suffix string
	switch {
	case strings.HasPrefix(out, input):
		suffix = out[len(input):]
	case trimmedInput != input && strings.HasPrefix(out, trimmedInput+" "):
		suffix = out[len(trimmedInput):]
	default:
		return "", false
	}
	if trimmedInput != input {
		suffix = strings.TrimLeft(suffix, " \t")
	}
	return suffix, true
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
	model = ResolveModel(provider, model)

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
		"model":      model,
		"messages":   messages,
		"max_tokens": opts.maxTokens,
		"stream":     false,
	}
	if effort, ok := reasoningEffort["openrouter"][model]; ok {
		body["max_tokens"] = max(opts.maxTokens, reasoningMinMaxTokens)
		body["reasoning"] = map[string]string{"effort": effort}
	} else if !strings.HasPrefix(model, "anthropic/") {
		body["temperature"] = opts.temperature
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
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	if out.Choices[0].Message.Content == "" && out.Choices[0].FinishReason == "length" {
		return "", fmt.Errorf("OpenRouter API returned no text before reaching its token limit")
	}
	return out.Choices[0].Message.Content, nil
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
	maxTokens := max(opts.maxTokens, anthropicMinMaxTokens[model])
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"system":     systemMsg,
		"messages":   conv,
	}
	if thinking, ok := anthropicThinking[model]; ok {
		body["thinking"] = thinking
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
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	if text.Len() == 0 {
		switch out.StopReason {
		case "max_tokens":
			return "", fmt.Errorf("anthropic API returned no text before reaching max_tokens")
		case "refusal":
			return "", fmt.Errorf("anthropic API declined to answer this request")
		}
	}
	return text.String(), nil
}

// OpenAI
func callOpenAI(apiKey, model string, messages []Message, opts *callOptions) (string, error) {
	cfg := providers["openai"]
	if model == "" {
		model = cfg.defaultModel
	}
	body := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	if effort, ok := reasoningEffort["openai"][model]; ok {
		body["max_completion_tokens"] = max(opts.maxTokens, reasoningMinMaxTokens)
		body["reasoning_effort"] = effort
	} else {
		body["max_tokens"] = opts.maxTokens
		body["temperature"] = opts.temperature
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
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	if out.Choices[0].Message.Content == "" && out.Choices[0].FinishReason == "length" {
		return "", fmt.Errorf("OpenAI API returned no text before reaching its token limit")
	}
	return out.Choices[0].Message.Content, nil
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
	genCfg := map[string]interface{}{
		"maxOutputTokens": opts.maxTokens,
	}
	if level, ok := reasoningEffort["gemini"][model]; ok {
		genCfg["maxOutputTokens"] = max(opts.maxTokens, reasoningMinMaxTokens)
		genCfg["thinkingConfig"] = map[string]string{"thinkingLevel": level}
	} else {
		genCfg["temperature"] = opts.temperature
	}
	body := map[string]interface{}{
		"contents":         contents,
		"generationConfig": genCfg,
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
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 {
		return "", nil
	}
	var text strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		if !p.Thought {
			text.WriteString(p.Text)
		}
	}
	if text.Len() == 0 && out.Candidates[0].FinishReason == "MAX_TOKENS" {
		return "", fmt.Errorf("gemini API returned no text before reaching maxOutputTokens")
	}
	return text.String(), nil
}
