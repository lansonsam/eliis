// Package anthropic mirrors the Anthropic Messages API wire format
// (POST /v1/messages). These DTOs are protocol-faithful: they describe
// what flies on the wire and nothing more. Translation between these
// DTOs and the project IR (internal/core/types) lives in codec.go.
//
// API reference (snapshot 2026-05):
//
//	https://docs.anthropic.com/en/api/messages
//	https://docs.anthropic.com/en/api/messages-streaming
//
// Conventions:
//   - Optional sampling parameters use pointer types so we can tell
//     "unset" from a meaningful zero (e.g. Temperature=0).
//   - Union types (content can be string OR array; system likewise) are
//     typed as `any` and unpacked in the codec via type switches.
//   - Server-tool blocks (web_search, code_execution, mcp_*) and other
//     low-traffic shapes are kept as json.RawMessage to avoid lock-in.
package anthropic

import "encoding/json"

// ============================================================================
// Request
// ============================================================================

// MessagesRequest is the body for POST /v1/messages.
//
// Note: `max_tokens` is REQUIRED by the Anthropic API (unlike OpenAI), but is
// declared as a pointer here so the codec can detect "client forgot to set it"
// vs "client explicitly set 0" (the latter pre-warms the prompt cache).
type MessagesRequest struct {
	Model     string         `json:"model"`
	Messages  []InputMessage `json:"messages"`
	MaxTokens *int           `json:"max_tokens,omitempty"`

	// System prompt — string OR []TextBlockParam (the latter supports
	// per-block cache_control and citations).
	System any `json:"system,omitempty"`

	// Sampling
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Streaming
	Stream *bool `json:"stream,omitempty"`

	// Tools
	Tools      []ToolDef `json:"tools,omitempty"`
	ToolChoice any       `json:"tool_choice,omitempty"` // ToolChoice* variants

	// Extended thinking
	Thinking *ThinkingConfig `json:"thinking,omitempty"`

	// Request metadata
	Metadata    *Metadata `json:"metadata,omitempty"`
	ServiceTier string    `json:"service_tier,omitempty"` // "auto" | "standard_only"
	Container   string    `json:"container,omitempty"`    // container reuse id

	// Pass-through extension fields (kept opaque to avoid coupling to
	// preview features that change shape frequently).
	ContextManagement json.RawMessage `json:"context_management,omitempty"`
	McpServers        json.RawMessage `json:"mcp_servers,omitempty"`
	Betas             []string        `json:"betas,omitempty"`
}

// Metadata describes per-request metadata (currently only user_id).
type Metadata struct {
	// UserID is an opaque identifier (uuid/hash) used for abuse detection.
	// Do NOT include personally identifying information.
	UserID string `json:"user_id,omitempty"`
}

// InputMessage is one item in `messages`. Roles are limited to "user" and
// "assistant"; system prompts go in the top-level System field.
type InputMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content any    `json:"content"` // string | []ContentBlockParam
}

// ============================================================================
// Content blocks (request side)
// ============================================================================

// ContentBlockParam is one element of an InputMessage.Content array.
//
// Type values seen on the request side:
//
//	"text"            — TextBlockParam (text + cache_control + citations)
//	"image"           — ImageBlockParam (Source)
//	"document"        — DocumentBlockParam (Source + title + context + citations)
//	"thinking"        — ThinkingBlockParam (thinking + signature)
//	"redacted_thinking" — RedactedThinkingBlockParam (data)
//	"tool_use"        — ToolUseBlockParam (id + name + input)
//	"tool_result"     — ToolResultBlockParam (tool_use_id + content + is_error)
//	"server_tool_use" / "web_search_tool_result" / "code_execution_tool_result"
//	                  — server-tool blocks; treated as opaque
//	"container_upload" — ContainerUploadBlockParam
//
// One field corresponds to each Type; consumers switch on Type and read the
// matching field.
type ContentBlockParam struct {
	Type string `json:"type"`

	// Common across most block types
	CacheControl *CacheControl `json:"cache_control,omitempty"`

	// type=text
	Text      string         `json:"text,omitempty"`
	Citations []TextCitation `json:"citations,omitempty"`

	// type=image / type=document
	Source *BlockSource `json:"source,omitempty"`

	// type=document
	Title   string `json:"title,omitempty"`
	Context string `json:"context,omitempty"`

	// type=thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// type=redacted_thinking
	Data string `json:"data,omitempty"`

	// type=tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"` // raw JSON arguments

	// type=tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   *bool  `json:"is_error,omitempty"`
	// Content for tool_result is string OR []ContentBlockParam (subset:
	// text/image). Kept as `any` so codecs can normalise both shapes.
	Content any `json:"content,omitempty"`

	// Server-tool blocks (web_search, code_execution, mcp_*) — opaque
	// pass-through; promoted to typed fields if/when needed.
	Extra json.RawMessage `json:"-"`
}

// CacheControl marks a content block as a prompt-cache breakpoint.
type CacheControl struct {
	Type string `json:"type"`          // always "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "5m" | "1h" (default "5m")
}

// BlockSource is the source descriptor for image / document blocks.
//
// Type values:
//
//	"base64"   — inline data + media_type (e.g. "image/png")
//	"url"      — remote url
//	"file"     — uploaded file_id
//	"text"     — plain text document (data is the text)
//	"content"  — nested ContentBlockParam list (for documents)
type BlockSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      any    `json:"data,omitempty"` // string | []ContentBlockParam (when Type=content)
	URL       string `json:"url,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

// ============================================================================
// Citations
// ============================================================================

// TextCitation is one element of a TextBlockParam.citations array.
//
// Type values:
//
//	"char_location"          — CitationCharLocation
//	"page_location"          — CitationPageLocation
//	"content_block_location" — CitationContentBlockLocation
//	"search_result_location" — CitationSearchResultLocation
//	"web_search_result_location" — CitationWebSearchResultLocation
type TextCitation struct {
	Type           string `json:"type"`
	CitedText      string `json:"cited_text,omitempty"`
	DocumentIndex  int    `json:"document_index,omitempty"`
	DocumentTitle  string `json:"document_title,omitempty"`
	StartCharIndex int    `json:"start_char_index,omitempty"`
	EndCharIndex   int    `json:"end_char_index,omitempty"`
	StartPageNum   int    `json:"start_page_number,omitempty"`
	EndPageNum     int    `json:"end_page_number,omitempty"`
	StartBlockIdx  int    `json:"start_block_index,omitempty"`
	EndBlockIdx    int    `json:"end_block_index,omitempty"`
	URL            string `json:"url,omitempty"`
	Title          string `json:"title,omitempty"`
	EncryptedIndex string `json:"encrypted_index,omitempty"`
}

// ============================================================================
// Tools
// ============================================================================

// ToolDef declares a client tool the model is allowed to call.
//
// For server tools (web_search_*, code_execution_*, computer_*, bash_*,
// text_editor_*), use ToolDefRaw via the `extra` channel — they have wildly
// different shapes per tool version and are best transported opaquely.
type ToolDef struct {
	Type         string          `json:"type,omitempty"`        // "" or "custom" for client tools
	Name         string          `json:"name"`                  //
	Description  string          `json:"description,omitempty"` //
	InputSchema  json.RawMessage `json:"input_schema"`          // JSON Schema (required)
	CacheControl *CacheControl   `json:"cache_control,omitempty"`
}

// Tool-choice variants. ToolChoice in MessagesRequest is `any`; codecs
// emit one of these structs.

// ToolChoiceAuto lets the model decide whether to use any tool.
type ToolChoiceAuto struct {
	Type                   string `json:"type"` // "auto"
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"`
}

// ToolChoiceAny forces the model to use some tool.
type ToolChoiceAny struct {
	Type                   string `json:"type"` // "any"
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"`
}

// ToolChoiceTool forces the model to use a specific named tool.
type ToolChoiceTool struct {
	Type                   string `json:"type"` // "tool"
	Name                   string `json:"name"`
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"`
}

// ToolChoiceNone disables all tool use.
type ToolChoiceNone struct {
	Type string `json:"type"` // "none"
}

// ============================================================================
// Thinking
// ============================================================================

// ThinkingConfig configures Claude's extended thinking output.
//
// Type values:
//
//	"enabled"  — BudgetTokens (>=1024) and optional Display
//	"disabled" — explicitly turn thinking off
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"` // "summarized" | "omitted"
}

// ============================================================================
// Non-streaming response
// ============================================================================

// MessagesResponse is the body returned by POST /v1/messages when stream=false.
//
// On streaming responses the server sends a sequence of MessageStreamEvent
// items; see the streaming section below.
type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // always "message"
	Role         string         `json:"role"` // always "assistant"
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"` // "end_turn"|"max_tokens"|"stop_sequence"|"tool_use"|"pause_turn"|"refusal"
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
	Container    string         `json:"container,omitempty"`
}

// ContentBlock is one element of MessagesResponse.Content.
//
// Type values:
//
//	"text"          — Text + Citations
//	"thinking"      — Thinking + Signature
//	"redacted_thinking" — Data
//	"tool_use"      — ID + Name + Input
//	"server_tool_use" / "web_search_tool_result" / "code_execution_tool_result"
//	                — server-side tool blocks; carried opaquely
type ContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Citations []TextCitation `json:"citations,omitempty"`

	// type=thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// type=redacted_thinking
	Data string `json:"data,omitempty"`

	// type=tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ============================================================================
// Usage
// ============================================================================

// Usage records token consumption.
type Usage struct {
	InputTokens              int                `json:"input_tokens"`
	OutputTokens             int                `json:"output_tokens"`
	CacheCreationInputTokens int                `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int                `json:"cache_read_input_tokens,omitempty"`
	CacheCreation            *CacheCreationInfo `json:"cache_creation,omitempty"`
	ServerToolUse            *ServerToolUse     `json:"server_tool_use,omitempty"`
	ServiceTier              string             `json:"service_tier,omitempty"`
}

// CacheCreationInfo breaks cache-creation tokens by TTL bucket.
type CacheCreationInfo struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens,omitempty"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens,omitempty"`
}

// ServerToolUse counts server-side tool invocations.
type ServerToolUse struct {
	WebSearchRequests int `json:"web_search_requests,omitempty"`
}

// ============================================================================
// Streaming events (SSE)
// ============================================================================
//
// Anthropic streams a *sequence* of typed events (unlike OpenAI's single
// chunk shape). The wire format is:
//
//	event: <event_type>
//	data: <json>
//
// Event types: message_start, message_delta, message_stop,
//              content_block_start, content_block_delta, content_block_stop,
//              ping, error.
//
// MessageStreamEvent is a union envelope: codecs read .Type and access the
// matching field. Only fields relevant to a given event type are populated.

// MessageStreamEvent is one SSE event in the messages-streaming response.
type MessageStreamEvent struct {
	Type string `json:"type"`

	// type=message_start
	Message *MessagesResponse `json:"message,omitempty"`

	// type=content_block_start / content_block_delta / content_block_stop
	Index        *int              `json:"index,omitempty"`
	ContentBlock *ContentBlock     `json:"content_block,omitempty"`
	Delta        *StreamEventDelta `json:"delta,omitempty"`

	// type=message_delta — Delta carries stop_reason / stop_sequence,
	// Usage carries the cumulative output_tokens.
	Usage *Usage `json:"usage,omitempty"`

	// type=error
	Error *ErrorDetail `json:"error,omitempty"`
}

// StreamEventDelta is the polymorphic delta payload.
//
// On content_block_delta, sub-Type values:
//
//	"text_delta"        — Text
//	"input_json_delta"  — PartialJSON (concatenate to build tool_use.input)
//	"thinking_delta"    — Thinking
//	"signature_delta"   — Signature
//	"citations_delta"   — Citation
//
// On message_delta:
//
//	StopReason / StopSequence are populated; Type is unset.
type StreamEventDelta struct {
	Type         string        `json:"type,omitempty"`
	Text         string        `json:"text,omitempty"`
	PartialJSON  string        `json:"partial_json,omitempty"`
	Thinking     string        `json:"thinking,omitempty"`
	Signature    string        `json:"signature,omitempty"`
	Citation     *TextCitation `json:"citation,omitempty"`
	StopReason   string        `json:"stop_reason,omitempty"`
	StopSequence string        `json:"stop_sequence,omitempty"`
}

// ============================================================================
// Errors
// ============================================================================

// ErrorResponse is the standard Anthropic error envelope.
type ErrorResponse struct {
	Type  string      `json:"type"` // always "error"
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the inner error object.
//
// Common Type values: "invalid_request_error", "authentication_error",
// "permission_error", "not_found_error", "request_too_large",
// "rate_limit_error", "api_error", "overloaded_error".
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
