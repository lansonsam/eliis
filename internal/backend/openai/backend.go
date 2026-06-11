// Package openai implements an OpenAI-compatible upstream backend.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lansonsam/eliis/internal/core/contract"
	"github.com/lansonsam/eliis/internal/core/types"
	protocol "github.com/lansonsam/eliis/internal/protocol/openai"
)

const defaultName = "openai"

// Config controls the OpenAI-compatible backend.
type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// Backend delivers IR requests to an OpenAI-compatible Chat Completions endpoint.
type Backend struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

var _ contract.Backend = (*Backend)(nil)

// New constructs an OpenAI-compatible backend.
func New(cfg Config) *Backend {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Backend{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		client:  client,
	}
}

func (b *Backend) Name() string {
	return defaultName
}

// Invoke forwards an IR request to /chat/completions and decodes the response into IR.
func (b *Backend) Invoke(ctx context.Context, req *types.UnifiedRequest) (*types.UnifiedResponse, error) {
	if b.baseURL == "" {
		return nil, errors.New("openai base URL is empty")
	}

	openAIReq, err := protocol.EncodeChatCompletionRequest(req)
	if err != nil {
		return nil, fmt.Errorf("encode openai request: %w", err)
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if b.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+b.apiKey)
	}

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call openai upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeUpstreamError(resp)
	}

	var openAIResp protocol.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	ir, err := protocol.DecodeChatCompletionResponse(openAIResp)
	if err != nil {
		return nil, fmt.Errorf("decode openai response to IR: %w", err)
	}
	return ir, nil
}

func (b *Backend) InvokeStream(context.Context, *types.UnifiedRequest) (contract.StreamReader, error) {
	return nil, errors.New("openai streaming is not implemented yet")
}

// UpstreamError captures an HTTP error returned by the upstream provider.
type UpstreamError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *UpstreamError) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("upstream returned HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("upstream returned HTTP %d (%s): %s", e.StatusCode, e.Type, e.Message)
}

func decodeUpstreamError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	upstream := &UpstreamError{
		StatusCode: resp.StatusCode,
		Type:       "api_error",
		Message:    strings.TrimSpace(string(body)),
	}

	var openAIError protocol.ErrorResponse
	if err := json.Unmarshal(body, &openAIError); err == nil && openAIError.Error.Message != "" {
		upstream.Type = openAIError.Error.Type
		upstream.Message = openAIError.Error.Message
	}
	if upstream.Message == "" {
		upstream.Message = resp.Status
	}
	return upstream
}
