package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultServerAddr     = ":8080"
	defaultLogLevel       = "info"
	defaultLogFormat      = "text"
	defaultOpenAIBaseURL  = "https://api.openai.com/v1"
	defaultOpenAITimeout  = "120s"
	defaultRouteBackend   = "openai"
	defaultRouteMaxTokens = 1024
)

// Root is the top-level configuration document.
type Root struct {
	Server    Server    `yaml:"server"`
	Log       Log       `yaml:"log"`
	Upstreams Upstreams `yaml:"upstreams"`
	Routes    Routes    `yaml:"routes"`
}

// Server holds HTTP listener settings.
type Server struct {
	Addr string `yaml:"addr"`
}

// Log controls the structured logger emitted by cmd/eliis.
type Log struct {
	// Level is one of: debug, info, warn, error. Defaults to "info".
	Level string `yaml:"level"`
	// Format is one of: text, json. Defaults to "text" for dev readability.
	Format string `yaml:"format"`
}

// Upstreams groups outbound model provider settings.
type Upstreams struct {
	OpenAI OpenAIUpstream `yaml:"openai"`
}

// OpenAIUpstream configures an OpenAI-compatible Chat Completions endpoint.
type OpenAIUpstream struct {
	// BaseURL should point at the upstream /v1 root, e.g. https://api.openai.com/v1.
	BaseURL string `yaml:"base_url"`
	// APIKey is expanded with os.ExpandEnv so configs can use ${OPENAI_API_KEY}.
	APIKey string `yaml:"api_key"`
	// Timeout is parsed as a Go duration, e.g. "120s".
	Timeout string `yaml:"timeout"`
}

// TimeoutDuration parses the configured OpenAI request timeout.
func (u OpenAIUpstream) TimeoutDuration() (time.Duration, error) {
	d, err := time.ParseDuration(u.Timeout)
	if err != nil {
		return 0, fmt.Errorf("parse openai timeout %q: %w", u.Timeout, err)
	}
	return d, nil
}

// Routes groups ingress route settings.
type Routes struct {
	AnthropicMessages AnthropicMessagesRoute `yaml:"anthropic_messages"`
}

// AnthropicMessagesRoute configures Anthropic Messages ingress behavior.
type AnthropicMessagesRoute struct {
	// Backend selects the outbound backend. M1 supports "openai".
	Backend string `yaml:"backend"`
	// Model optionally rewrites inbound Claude model IDs to an upstream model.
	Model string `yaml:"model"`
	// DefaultMaxTokens fills Anthropic max_tokens when a caller omits it.
	DefaultMaxTokens int `yaml:"default_max_tokens"`
}

// Load reads and parses a YAML config file. Relative paths resolve from the process cwd.
func Load(path string) (*Root, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var root Root
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	root.applyDefaults()
	return &root, nil
}

func (r *Root) applyDefaults() {
	if r.Server.Addr == "" {
		r.Server.Addr = defaultServerAddr
	}
	if r.Log.Level == "" {
		r.Log.Level = defaultLogLevel
	} else {
		r.Log.Level = strings.ToLower(r.Log.Level)
	}
	if r.Log.Format == "" {
		r.Log.Format = defaultLogFormat
	} else {
		r.Log.Format = strings.ToLower(r.Log.Format)
	}

	if r.Upstreams.OpenAI.BaseURL == "" {
		r.Upstreams.OpenAI.BaseURL = defaultOpenAIBaseURL
	}
	if r.Upstreams.OpenAI.Timeout == "" {
		r.Upstreams.OpenAI.Timeout = defaultOpenAITimeout
	}
	r.Upstreams.OpenAI.APIKey = os.ExpandEnv(r.Upstreams.OpenAI.APIKey)

	if r.Routes.AnthropicMessages.Backend == "" {
		r.Routes.AnthropicMessages.Backend = defaultRouteBackend
	} else {
		r.Routes.AnthropicMessages.Backend = strings.ToLower(r.Routes.AnthropicMessages.Backend)
	}
	if r.Routes.AnthropicMessages.DefaultMaxTokens <= 0 {
		r.Routes.AnthropicMessages.DefaultMaxTokens = defaultRouteMaxTokens
	}
}
