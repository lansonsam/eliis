package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lansonsam/eliis/internal/core/types"
)

func TestInvokePostsChatCompletionsAndDecodesResponse(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_test",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`))
	}))
	defer server.Close()

	backend := New(Config{BaseURL: server.URL + "/", APIKey: "upstream-key", Timeout: time.Second})
	maxTokens := 128
	resp, err := backend.Invoke(context.Background(), &types.UnifiedRequest{
		Model:     "gpt-test",
		System:    "You are concise.",
		MaxTokens: &maxTokens,
		Messages: []types.Message{{
			Role:  types.RoleUser,
			Parts: []types.ContentPart{{Type: types.ContentTypeText, Text: "Hi"}},
		}},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer upstream-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "gpt-test" {
		t.Fatalf("body model = %#v", gotBody["model"])
	}
	if gotBody["max_completion_tokens"].(float64) != float64(maxTokens) {
		t.Fatalf("body max_completion_tokens = %#v", gotBody["max_completion_tokens"])
	}
	if resp.ID != "chatcmpl_test" || resp.Message.Parts[0].Text != "Hello" {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
}

func TestInvokeReturnsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"authentication_error"}}`))
	}))
	defer server.Close()

	backend := New(Config{BaseURL: server.URL})
	_, err := backend.Invoke(context.Background(), &types.UnifiedRequest{Model: "gpt-test"})
	if err == nil {
		t.Fatal("Invoke() error = nil, want upstream error")
	}
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("error = %T, want *UpstreamError", err)
	}
	if upstream.StatusCode != http.StatusUnauthorized || upstream.Type != "authentication_error" || upstream.Message != "bad key" {
		t.Fatalf("upstream error = %#v", upstream)
	}
}
