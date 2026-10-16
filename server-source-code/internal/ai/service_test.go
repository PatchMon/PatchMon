package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var anthropicRetired = []string{
	"claude-sonnet-4-20250514",
	"claude-3-5-sonnet-20241022",
	"claude-3-5-haiku-20241022",
}

func TestAnthropicModelsAreCurrent(t *testing.T) {
	cfg := providers["anthropic"]
	for _, id := range anthropicRetired {
		if cfg.defaultModel == id {
			t.Errorf("default model %q is retired", id)
		}
		for _, m := range cfg.models {
			if m.ID == id {
				t.Errorf("model list offers retired model %q", id)
			}
		}
	}
}

func withAnthropicServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := providers["anthropic"]
	patched := orig
	patched.baseURL = srv.URL
	providers["anthropic"] = patched
	t.Cleanup(func() { providers["anthropic"] = orig })
}

func TestCallAnthropicSkipsThinkingBlocks(t *testing.T) {
	withAnthropicServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"content":[{"type":"thinking","thinking":"","signature":"x"},{"type":"text","text":"ls -la"}]}`)
	})
	got, err := callAnthropic("key", "claude-sonnet-5-5", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}

func TestResolveModel(t *testing.T) {
	cases := []struct {
		provider, model, want string
	}{
		{"anthropic", "claude-sonnet-4-20250514", "claude-sonnet-5-5"},
		{"anthropic", "claude-3-5-haiku-20241022", "claude-sonnet-5-5"},
		{"anthropic", "", "claude-sonnet-5-5"},
		{"anthropic", "claude-haiku-5-5", "claude-haiku-5-5"},
		{"anthropic", "claude-custom-model", "claude-custom-model"},
		{"openrouter", "anthropic/claude-3.5-sonnet", "anthropic/claude-sonnet-5.5"},
		{"openrouter", "openai/gpt-4o", "openai/gpt-4o"},
		{"openrouter", "google/gemini-2.5-flash", "anthropic/claude-sonnet-5.5"},
		{"gemini", "gemini-1.5-flash", "gemini-3.5-flash-lite"},
		{"gemini", "gemini-2.5-flash", "gemini-2.5-flash"},
		{"openai", "gpt-4o", "gpt-4o"},
		{"openai", "gpt-4-turbo", "gpt-4o-mini"},
		{"unknown", "x", "x"},
	}
	for _, c := range cases {
		if got := ResolveModel(c.provider, c.model); got != c.want {
			t.Errorf("ResolveModel(%q, %q) = %q, want %q", c.provider, c.model, got, c.want)
		}
	}
}

func TestProviderDefaultsAreListedAndNotRetired(t *testing.T) {
	for id, cfg := range providers {
		if retiredModels[id][cfg.defaultModel] {
			t.Errorf("%s default %q is retired", id, cfg.defaultModel)
		}
		listed := false
		for _, m := range cfg.models {
			if m.ID == cfg.defaultModel {
				listed = true
			}
			if retiredModels[id][m.ID] {
				t.Errorf("%s offers retired model %q", id, m.ID)
			}
		}
		if !listed {
			t.Errorf("%s default %q is not in its model list", id, cfg.defaultModel)
		}
	}
}

func TestCallAnthropicThinkingConfig(t *testing.T) {
	cases := []struct {
		model         string
		wantThinking  string
		wantMaxTokens float64
	}{
		{"claude-sonnet-5-5", "between_tools", 100},
		{"claude-haiku-5-5", "disabled", 100},
		{"claude-opus-5-5", "", 16000},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			var body map[string]any
			withAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}]}`)
			})
			if _, err := callAnthropic("key", c.model, []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err != nil {
				t.Fatal(err)
			}
			got := ""
			if th, ok := body["thinking"].(map[string]any); ok {
				got, _ = th["type"].(string)
			}
			if got != c.wantThinking {
				t.Errorf("thinking type = %q, want %q", got, c.wantThinking)
			}
			if body["max_tokens"] != c.wantMaxTokens {
				t.Errorf("max_tokens = %v, want %v", body["max_tokens"], c.wantMaxTokens)
			}
			if _, ok := body["temperature"]; ok {
				t.Error("temperature must not be sent to Anthropic")
			}
		})
	}
}

func TestCallAnthropicErrorsWhenThinkingExhaustsTokens(t *testing.T) {
	withAnthropicServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"content":[{"type":"thinking","thinking":"","signature":"x"}],"stop_reason":"max_tokens"}`)
	})
	if _, err := callAnthropic("key", "claude-opus-5-5", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err == nil {
		t.Fatal("expected an error when no text is returned")
	}
}

func TestCallAnthropicErrorsOnRefusal(t *testing.T) {
	withAnthropicServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"content":[],"stop_reason":"refusal"}`)
	})
	if _, err := callAnthropic("key", "claude-sonnet-5-5", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err == nil {
		t.Fatal("expected an error for a refusal")
	}
}

func TestCallOpenRouterOmitsTemperatureForClaude(t *testing.T) {
	for model, want := range map[string]bool{"anthropic/claude-sonnet-5.5": false, "openai/gpt-4o": true} {
		var body map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		}))
		orig := providers["openrouter"]
		patched := orig
		patched.baseURL = srv.URL
		providers["openrouter"] = patched
		_, err := callOpenRouter("key", model, []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100, temperature: 0.3})
		providers["openrouter"] = orig
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, got := body["temperature"]; got != want {
			t.Errorf("%s: temperature sent = %v, want %v", model, got, want)
		}
	}
}

func TestNormaliseCompletion(t *testing.T) {
	cases := []struct{ input, response, want string }{
		{"systemctl rest", "systemctl restart", "art"},
		{"systemctl rest", "systemctl restart\n\nThis restarts a service.", "art"},
		{"journalctl -u nginx", "journalctl -u nginx -f", " -f"},
		{"ssh", "ssh-keygen", "-keygen"},
		{"docker ps ", "docker ps -a", "-a"},
		{"docker ps ", "docker ps  -a", "-a"},
		{"systemctl rest", "`systemctl restart`", "art"},
		{"systemctl rest", "\"systemctl restart\"", "art"},
		{"git commit -", `git commit -m ""`, `m ""`},
		{"ls -l", "ls -l", ""},
		{"systemctl rest", "art", ""},
		{"systemctl rest", "sudo systemctl restart", ""},
		{"ls", "```bash\nls -la\n```", ""},
		{"ls", "ls " + strings.Repeat("x", 300), ""},
		{"ls", "ls -la `pwd`", " -la `pwd`"},
		{"echo `da", "echo `date`", "te`"},
		{"echo \"a", "echo \"ab\"", "b\""},
		{"ls  ", "ls -la", "-la"},
		{"ls\t", "ls -la", "-la"},
		{"ls é", "ls é.txt", ".txt"},
		{"ls", "ls -la\x1b[2J", ""},
		{"Systemctl rest", "systemctl restart", ""},
		{"ls", "`ls -la `", " -la"},
		{"ls", "ls -la\u202e", ""},
		{"systemctl rest", "`\"systemctl restart\"`", ""},
	}
	for _, c := range cases {
		t.Run(c.input+"|"+c.response, func(t *testing.T) {
			if got := normaliseCompletion(c.input, c.response); got != c.want {
				t.Errorf("normaliseCompletion(%q, %q) = %q, want %q", c.input, c.response, got, c.want)
			}
		})
	}
}

func withProviderServer(t *testing.T, id string, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := providers[id]
	patched := orig
	patched.baseURL = srv.URL
	providers[id] = patched
	t.Cleanup(func() { providers[id] = orig })
}

func TestCallGeminiRequestByModel(t *testing.T) {
	cases := []struct {
		model         string
		wantLevel     string
		wantMaxTokens float64
		wantTemp      bool
	}{
		{"gemini-3.5-flash-lite", "MINIMAL", reasoningMinMaxTokens, false},
		{"gemini-3.6-flash", "MINIMAL", reasoningMinMaxTokens, false},
		{"gemini-3.8-flash", "LOW", reasoningMinMaxTokens, false},
		{"gemini-2.5-flash", "", 100, true},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			var body map[string]any
			withProviderServer(t, "gemini", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
			})
			if _, err := callGemini("key", c.model, []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100, temperature: 0.2}); err != nil {
				t.Fatal(err)
			}
			gen, _ := body["generationConfig"].(map[string]any)
			level := ""
			if tc, ok := gen["thinkingConfig"].(map[string]any); ok {
				level, _ = tc["thinkingLevel"].(string)
			}
			if level != c.wantLevel {
				t.Errorf("thinkingLevel = %q, want %q", level, c.wantLevel)
			}
			if gen["maxOutputTokens"] != c.wantMaxTokens {
				t.Errorf("maxOutputTokens = %v, want %v", gen["maxOutputTokens"], c.wantMaxTokens)
			}
			if _, ok := gen["temperature"]; ok != c.wantTemp {
				t.Errorf("temperature sent = %v, want %v", ok, c.wantTemp)
			}
		})
	}
}

func TestCallGeminiSkipsThoughtParts(t *testing.T) {
	withProviderServer(t, "gemini", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true},{"text":"ls ","thoughtSignature":"x"},{"text":"-la"}]}}]}`)
	})
	got, err := callGemini("key", "gemini-3.8-flash", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}

func TestCallGeminiErrorsWhenThinkingExhaustsTokens(t *testing.T) {
	withProviderServer(t, "gemini", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[]},"finishReason":"MAX_TOKENS"}]}`)
	})
	if _, err := callGemini("key", "gemini-3.8-flash", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err == nil {
		t.Fatal("expected an error when no text is returned")
	}
}

func TestCallOpenAIRequestByModel(t *testing.T) {
	cases := []struct {
		model      string
		wantLimit  string
		wantTokens float64
		wantEffort string
		wantTemp   bool
	}{
		{"gpt-5.6-sol", "max_completion_tokens", reasoningMinMaxTokens, "low", false},
		{"gpt-4o-mini", "max_tokens", 100, "", true},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			var body map[string]any
			withProviderServer(t, "openai", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
			})
			if _, err := callOpenAI("key", c.model, []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100, temperature: 0.2}); err != nil {
				t.Fatal(err)
			}
			if body[c.wantLimit] != c.wantTokens {
				t.Errorf("%s = %v, want %v", c.wantLimit, body[c.wantLimit], c.wantTokens)
			}
			effort, _ := body["reasoning_effort"].(string)
			if effort != c.wantEffort {
				t.Errorf("reasoning_effort = %q, want %q", effort, c.wantEffort)
			}
			if _, ok := body["temperature"]; ok != c.wantTemp {
				t.Errorf("temperature sent = %v, want %v", ok, c.wantTemp)
			}
		})
	}
}

func TestCallOpenAIErrorsWhenReasoningExhaustsTokens(t *testing.T) {
	withProviderServer(t, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`)
	})
	if _, err := callOpenAI("key", "gpt-5.6-sol", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err == nil {
		t.Fatal("expected an error when no text is returned")
	}
}

func TestCallOpenRouterReasoningForGemini3(t *testing.T) {
	var body map[string]any
	withProviderServer(t, "openrouter", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	if _, err := callOpenRouter("key", "google/gemini-3.8-flash", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100, temperature: 0.2}); err != nil {
		t.Fatal(err)
	}
	if r, _ := body["reasoning"].(map[string]any); r["effort"] != "low" {
		t.Errorf("reasoning = %v, want effort low", body["reasoning"])
	}
	if body["max_tokens"] != float64(reasoningMinMaxTokens) {
		t.Errorf("max_tokens = %v, want %v", body["max_tokens"], reasoningMinMaxTokens)
	}
	if _, ok := body["temperature"]; ok {
		t.Error("temperature must not be sent to a reasoning model")
	}
}

func TestCallOpenRouterErrorsWhenReasoningExhaustsTokens(t *testing.T) {
	withProviderServer(t, "openrouter", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`)
	})
	if _, err := callOpenRouter("key", "google/gemini-3.8-flash", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100}); err == nil {
		t.Fatal("expected an error when no text is returned")
	}
}

func TestCallOpenRouterNoReasoningForGPT4o(t *testing.T) {
	var body map[string]any
	withProviderServer(t, "openrouter", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	if _, err := callOpenRouter("key", "openai/gpt-4o", []Message{{Role: "user", Content: "hi"}}, &callOptions{maxTokens: 100, temperature: 0.2}); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["reasoning"]; ok {
		t.Error("reasoning must not be sent to a non-reasoning model")
	}
	if body["max_tokens"] != float64(100) {
		t.Errorf("max_tokens = %v, want 100", body["max_tokens"])
	}
}
