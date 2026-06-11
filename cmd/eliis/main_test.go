package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lansonsam/eliis/internal/core/config"
)

func TestAnthropicMessagesRoute(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-upstream-key" {
			t.Fatalf("upstream Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_test",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello from upstream"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}
		}`))
	}))
	defer upstream.Close()

	router, err := buildRouter(&config.Root{
		Upstreams: config.Upstreams{OpenAI: config.OpenAIUpstream{
			BaseURL: upstream.URL,
			APIKey:  "test-upstream-key",
			Timeout: "5s",
		}},
		Routes: config.Routes{AnthropicMessages: config.AnthropicMessagesRoute{
			Backend:          "openai",
			Model:            "gpt-test",
			DefaultMaxTokens: 321,
		}},
	})
	if err != nil {
		t.Fatalf("buildRouter() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-opus-4-8",
		"system":"You are concise.",
		"messages":[{"role":"user","content":"Hi"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if upstreamBody["model"] != "gpt-test" {
		t.Fatalf("upstream model = %#v", upstreamBody["model"])
	}
	if upstreamBody["max_completion_tokens"].(float64) != 321 {
		t.Fatalf("upstream max_completion_tokens = %#v", upstreamBody["max_completion_tokens"])
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["type"] != "message" || got["role"] != "assistant" || got["model"] != "gpt-test" || got["stop_reason"] != "end_turn" {
		t.Fatalf("response = %#v", got)
	}
	content := got["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "Hello from upstream" {
		t.Fatalf("content block = %#v", block)
	}
}

func TestAnthropicMessagesRouteRejectsStreaming(t *testing.T) {
	router, err := buildRouter(&config.Root{
		Upstreams: config.Upstreams{OpenAI: config.OpenAIUpstream{
			BaseURL: "http://127.0.0.1:1/v1",
			Timeout: "5s",
		}},
		Routes: config.Routes{AnthropicMessages: config.AnthropicMessagesRoute{
			Backend:          "openai",
			DefaultMaxTokens: 1024,
		}},
	})
	if err != nil {
		t.Fatalf("buildRouter() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-opus-4-8",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"Hi"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "streaming is not implemented yet") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
