// Package openai mirrors the OpenAI Chat Completions wire format
// (POST /v1/chat/completions). These DTOs are protocol-faithful: they
// describe what flies on the wire and nothing more. Translation between
// these DTOs and the project IR (internal/core/types) lives in codec.go.
//
// API reference (snapshot 2026-05):
//
//	https://platform.openai.com/docs/api-reference/chat/create
//
// Field-naming and pointer conventions:
//   - All optional sampling parameters use pointer types so we can tell
//     "unset" from a meaningful zero (e.g. Temperature=0).
//   - Union types (e.g. message.content can be string OR array) are typed
//     as `any` and unpacked in the codec via type switches.
//   - Fields we want to forward verbatim without parsing (logit_bias,
//     metadata, audio request blobs, ...) use json.RawMessage.
package openai

import "encoding/json"

// ============================================================================
// Request
// ============================================================================

// ChatCompletionRequest is the body for POST /v1/chat/completions.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`

	// Sampling
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`
	Stop             any      `json:"stop,omitempty"` // string | []string
	N                *int     `json:"n,omitempty"`

	// Length control
	MaxTokens           *int `json:"max_tokens,omitempty"`            // deprecated, use MaxCompletionTokens
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"` // includes reasoning tokens

	// Streaming
	Stream        *bool          `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Reasoning models (o1, o3, gpt-5, ...)
	ReasoningEffort string `json:"reasoning_effort,omitempty"` // none|minimal|low|medium|high|xhigh
	Verbosity       string `json:"verbosity,omitempty"`        // low|medium|high

	// Tools
	Tools             []Tool `json:"tools,omitempty"`
	ToolChoice        any    `json:"tool_choice,omitempty"` // string | object
	ParallelToolCalls *bool  `json:"parallel_tool_calls,omitempty"`

	// Output structure / modalities
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Modalities     []string        `json:"modalities,omitempty"` // "text" | "audio"
	Audio          json.RawMessage `json:"audio,omitempty"`      // ChatCompletionAudioParam
	Prediction     json.RawMessage `json:"prediction,omitempty"` // ChatCompletionPredictionContent

	// Logprobs
	Logprobs    *bool `json:"logprobs,omitempty"`
	TopLogprobs *int  `json:"top_logprobs,omitempty"`

	// Caching / identity / billing
	User                 string            `json:"user,omitempty"` // deprecated, replaced by SafetyIdentifier + PromptCacheKey
	SafetyIdentifier     string            `json:"safety_identifier,omitempty"`
	PromptCacheKey       string            `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string            `json:"prompt_cache_retention,omitempty"` // "in_memory" | "24h"
	ServiceTier          string            `json:"service_tier,omitempty"`           // "auto"|"default"|"flex"|"scale"|"priority"
	Store                *bool             `json:"store,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	LogitBias            map[string]int    `json:"logit_bias,omitempty"`

	// Web search (built-in tool)
	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"`

	// Deprecated function-calling pre-tools API (kept for compatibility)
	FunctionCall json.RawMessage `json:"function_call,omitempty"`
	Functions    json.RawMessage `json:"functions,omitempty"`
}

// StreamOptions tunes streamed responses; only valid when Stream=true.
type StreamOptions struct {
	IncludeUsage       *bool `json:"include_usage,omitempty"`
	IncludeObfuscation *bool `json:"include_obfuscation,omitempty"`
}

// ============================================================================
// Messages
// ============================================================================

// ChatMessage is one item in `messages`. The same struct is reused both for
// inbound request items and for assistant messages in the response — the role
// determines which fields are populated.
//
// Role values:
//
//	"developer" — replaces "system" on o1+/gpt-5 models
//	"system"    — pre-o1 system instructions
//	"user"      — end-user input
//	"assistant" — model output
//	"tool"      — tool result returning to the model
//	"function"  — deprecated function-calling result
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content,omitempty"` // string | []ContentPart | []RefusalContentPart
	Name    string `json:"name,omitempty"`

	// Assistant-only
	ToolCalls    []ToolCall      `json:"tool_calls,omitempty"`
	Refusal      string          `json:"refusal,omitempty"`
	Annotations  json.RawMessage `json:"annotations,omitempty"` // url_citation list
	Audio        json.RawMessage `json:"audio,omitempty"`       // assistant audio response
	FunctionCall *FunctionCall   `json:"function_call,omitempty"`

	// Tool-message-only
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ContentPart is one element of a message-content array.
//
// Type values:
//
//	"text"        — Text
//	"image_url"   — ImageURL
//	"input_audio" — InputAudio
//	"file"        — File
//	"refusal"     — Refusal (assistant only)
type ContentPart struct {
	Type       string             `json:"type"`
	Text       string             `json:"text,omitempty"`
	ImageURL   *ContentImageURL   `json:"image_url,omitempty"`
	InputAudio *ContentInputAudio `json:"input_audio,omitempty"`
	File       *ContentFile       `json:"file,omitempty"`
	Refusal    string             `json:"refusal,omitempty"`
}

// ContentImageURL carries an inline-or-remote image input.
type ContentImageURL struct {
	URL    string `json:"url"`              // either an https URL or "data:image/...;base64,..."
	Detail string `json:"detail,omitempty"` // "auto" | "low" | "high"
}

// ContentInputAudio carries inline base64-encoded audio input.
type ContentInputAudio struct {
	Data   string `json:"data"`   // base64
	Format string `json:"format"` // "wav" | "mp3"
}

// ContentFile carries an uploaded-file reference or inline base64 file data.
type ContentFile struct {
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"` // base64
}

// ============================================================================
// Tools
// ============================================================================

// Tool is one element in `tools`.
type Tool struct {
	Type     string              `json:"type"` // "function" | "custom"
	Function *FunctionDefinition `json:"function,omitempty"`
	Custom   *CustomToolDef      `json:"custom,omitempty"`
}

// FunctionDefinition declares a JSON-schema-described function tool.
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
	Strict      *bool           `json:"strict,omitempty"`
}

// CustomToolDef declares a free-form-text or grammar-constrained tool.
type CustomToolDef struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Format      *CustomToolFormat `json:"format,omitempty"`
}

// CustomToolFormat restricts the input format for a CustomToolDef.
type CustomToolFormat struct {
	Type    string             `json:"type"` // "text" | "grammar"
	Grammar *CustomToolGrammar `json:"grammar,omitempty"`
}

// CustomToolGrammar carries a Lark or regex grammar definition.
type CustomToolGrammar struct {
	Definition string `json:"definition"`
	Syntax     string `json:"syntax"` // "lark" | "regex"
}

// ToolCall is one element in assistant.tool_calls (or chunk.delta.tool_calls).
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "function" | "custom"
	Function *FunctionCall   `json:"function,omitempty"`
	Custom   *CustomToolCall `json:"custom,omitempty"`
}

// FunctionCall is the model-emitted name + JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string; may be invalid JSON
}

// CustomToolCall is the model-emitted invocation of a custom tool.
type CustomToolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"` // raw text per CustomToolFormat
}

// ============================================================================
// Response format / web search
// ============================================================================

// ResponseFormat constrains the assistant output shape.
type ResponseFormat struct {
	Type       string                `json:"type"` // "text" | "json_object" | "json_schema"
	JSONSchema *ResponseFormatSchema `json:"json_schema,omitempty"`
}

// ResponseFormatSchema is the json_schema sub-object for Structured Outputs.
type ResponseFormatSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// WebSearchOptions configures the built-in web search tool.
type WebSearchOptions struct {
	SearchContextSize string                 `json:"search_context_size,omitempty"` // "low"|"medium"|"high"
	UserLocation      *WebSearchUserLocation `json:"user_location,omitempty"`
}

// WebSearchUserLocation provides approximate user location for ranking.
type WebSearchUserLocation struct {
	Type        string                   `json:"type"` // "approximate"
	Approximate *WebSearchApproxLocation `json:"approximate,omitempty"`
}

// WebSearchApproxLocation is the approximate location detail.
type WebSearchApproxLocation struct {
	City     string `json:"city,omitempty"`
	Country  string `json:"country,omitempty"` // ISO-3166-1 alpha-2
	Region   string `json:"region,omitempty"`
	Timezone string `json:"timezone,omitempty"` // IANA tz, e.g. "America/Los_Angeles"
}

// ============================================================================
// Non-streaming response
// ============================================================================

// ChatCompletionResponse is returned for POST /v1/chat/completions when stream=false.
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // always "chat.completion"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	ServiceTier       string   `json:"service_tier,omitempty"`
}

// Choice is one alternative completion (n>1 produces multiple).
type Choice struct {
	Index        int             `json:"index"`
	Message      ChatMessage     `json:"message"`
	FinishReason string          `json:"finish_reason"` // "stop"|"length"|"tool_calls"|"content_filter"|"function_call"
	Logprobs     *ChoiceLogprobs `json:"logprobs,omitempty"`
}

// ChoiceLogprobs holds per-token log-probability info for a choice.
type ChoiceLogprobs struct {
	Content []TokenLogprob `json:"content,omitempty"`
	Refusal []TokenLogprob `json:"refusal,omitempty"`
}

// TokenLogprob is one token + its log probability.
type TokenLogprob struct {
	Token       string       `json:"token"`
	Bytes       []int        `json:"bytes,omitempty"`
	Logprob     float64      `json:"logprob"`
	TopLogprobs []TopLogprob `json:"top_logprobs,omitempty"`
}

// TopLogprob is one of the top-k candidates at a token position.
type TopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int   `json:"bytes,omitempty"`
	Logprob float64 `json:"logprob"`
}

// ============================================================================
// Usage
// ============================================================================

// Usage records token consumption for the request.
type Usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails breaks down prompt tokens by category.
type PromptTokensDetails struct {
	AudioTokens  int `json:"audio_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// CompletionTokensDetails breaks down completion tokens by category.
type CompletionTokensDetails struct {
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

// ============================================================================
// Streaming chunks (SSE)
// ============================================================================

// ChatCompletionChunk is one SSE event payload (the JSON after `data: `)
// when stream=true. The terminating `data: [DONE]` line has no payload and
// is handled outside this struct.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"` // always "chat.completion.chunk"
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	// Usage appears only on the trailing chunk when StreamOptions.IncludeUsage=true.
	Usage             *Usage `json:"usage,omitempty"`
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
	ServiceTier       string `json:"service_tier,omitempty"`
}

// ChunkChoice is the per-choice payload in a streaming chunk.
type ChunkChoice struct {
	Index int        `json:"index"`
	Delta ChunkDelta `json:"delta"`
	// FinishReason is a pointer to distinguish "not yet" (null) from "stop".
	FinishReason *string         `json:"finish_reason"`
	Logprobs     *ChoiceLogprobs `json:"logprobs,omitempty"`
}

// ChunkDelta is the incremental content in one streaming chunk.
type ChunkDelta struct {
	Role         string          `json:"role,omitempty"`
	Content      string          `json:"content,omitempty"`
	ToolCalls    []ChunkToolCall `json:"tool_calls,omitempty"`
	Refusal      string          `json:"refusal,omitempty"`
	FunctionCall *FunctionCall   `json:"function_call,omitempty"` // deprecated
	Annotations  json.RawMessage `json:"annotations,omitempty"`
}

// ChunkToolCall is the streaming form of a tool call: function.arguments
// arrives in increments and must be concatenated by index.
type ChunkToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"` // "function" | "custom"
	Function *FunctionCall   `json:"function,omitempty"`
	Custom   *CustomToolCall `json:"custom,omitempty"`
}

// ============================================================================
// Errors
// ============================================================================

// ErrorResponse is the standard OpenAI error envelope.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the inner error object.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}
