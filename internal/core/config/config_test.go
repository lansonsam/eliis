package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndExpandsEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	path := filepath.Join(t.TempDir(), "eliis.yaml")
	if err := os.WriteFile(path, []byte(`
upstreams:
  openai:
    api_key: "${OPENAI_API_KEY}"
routes:
  anthropic_messages:
    model: "gpt-test"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Addr != defaultServerAddr {
		t.Fatalf("Server.Addr = %q, want %q", cfg.Server.Addr, defaultServerAddr)
	}
	if cfg.Log.Level != defaultLogLevel {
		t.Fatalf("Log.Level = %q, want %q", cfg.Log.Level, defaultLogLevel)
	}
	if cfg.Log.Format != defaultLogFormat {
		t.Fatalf("Log.Format = %q, want %q", cfg.Log.Format, defaultLogFormat)
	}
	if cfg.Upstreams.OpenAI.BaseURL != defaultOpenAIBaseURL {
		t.Fatalf("OpenAI.BaseURL = %q, want %q", cfg.Upstreams.OpenAI.BaseURL, defaultOpenAIBaseURL)
	}
	if cfg.Upstreams.OpenAI.APIKey != "test-key" {
		t.Fatalf("OpenAI.APIKey = %q, want env-expanded key", cfg.Upstreams.OpenAI.APIKey)
	}
	if cfg.Upstreams.OpenAI.Timeout != defaultOpenAITimeout {
		t.Fatalf("OpenAI.Timeout = %q, want %q", cfg.Upstreams.OpenAI.Timeout, defaultOpenAITimeout)
	}
	if cfg.Routes.AnthropicMessages.Backend != defaultRouteBackend {
		t.Fatalf("Route.Backend = %q, want %q", cfg.Routes.AnthropicMessages.Backend, defaultRouteBackend)
	}
	if cfg.Routes.AnthropicMessages.Model != "gpt-test" {
		t.Fatalf("Route.Model = %q, want gpt-test", cfg.Routes.AnthropicMessages.Model)
	}
	if cfg.Routes.AnthropicMessages.DefaultMaxTokens != defaultRouteMaxTokens {
		t.Fatalf("Route.DefaultMaxTokens = %d, want %d", cfg.Routes.AnthropicMessages.DefaultMaxTokens, defaultRouteMaxTokens)
	}
}

func TestOpenAIUpstreamTimeoutDuration(t *testing.T) {
	cfg := OpenAIUpstream{Timeout: "3s"}
	got, err := cfg.TimeoutDuration()
	if err != nil {
		t.Fatalf("TimeoutDuration() error = %v", err)
	}
	if got != 3*time.Second {
		t.Fatalf("TimeoutDuration() = %v, want 3s", got)
	}
}
