// Package embed exposes Eliis as an embeddable protocol translation library.
package embed

import (
	"encoding/json"
	"fmt"

	"github.com/lansonsam/eliis/internal/core/contract"
	"github.com/lansonsam/eliis/internal/protocol/anthropic"
	"github.com/lansonsam/eliis/internal/protocol/lens"
	"github.com/lansonsam/eliis/internal/protocol/openai"
)

type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolGemini    Protocol = "gemini"
)

type Route struct {
	Input            Protocol
	Output           Protocol
	OutputModel      string
	DefaultMaxTokens int
}

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func TranslateJSON(route Route, input []byte) ([]byte, error) {
	return New().TranslateJSON(route, input)
}

// TranslateJSON translates one non-streaming request body from route.Input
// protocol JSON into route.Output protocol JSON.
func (e *Engine) TranslateJSON(route Route, input []byte) ([]byte, error) {
	inputProtocol := NormalizeProtocol(route.Input)
	outputProtocol := NormalizeProtocol(route.Output)

	if inputProtocol == "" || outputProtocol == "" {
		return nil, fmt.Errorf("unknown protocol route: %q -> %q", route.Input, route.Output)
	}
	if inputProtocol == outputProtocol {
		return append([]byte(nil), input...), nil
	}

	switch {
	case inputProtocol == ProtocolOpenAI && outputProtocol == ProtocolAnthropic:
		return translateOpenAIToAnthropic(route, input)
	default:
		return nil, fmt.Errorf("unsupported protocol route: %s -> %s", inputProtocol, outputProtocol)
	}
}

func NormalizeProtocol(p Protocol) Protocol {
	switch p {
	case "openai", "oai":
		return ProtocolOpenAI
	case "anthropic", "claude", "a", "a社":
		return ProtocolAnthropic
	case "gemini", "google":
		return ProtocolGemini
	default:
		return ""
	}
}

func translateOpenAIToAnthropic(route Route, input []byte) ([]byte, error) {
	var openAIReq openai.ChatCompletionRequest
	if err := json.Unmarshal(input, &openAIReq); err != nil {
		return nil, fmt.Errorf("decode openai json: %w", err)
	}

	ir, err := openai.DecodeChatCompletionRequest(openAIReq)
	if err != nil {
		return nil, err
	}

	chain := contract.LensChain{
		lens.OverrideModel{Model: route.OutputModel},
		lens.EnsureMaxTokens{Default: route.DefaultMaxTokens},
	}
	if err := chain.Apply(ir); err != nil {
		return nil, err
	}

	anthropicReq, err := anthropic.EncodeMessagesRequest(ir)
	if err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(anthropicReq, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode anthropic json: %w", err)
	}
	return out, nil
}
