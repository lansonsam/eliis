package types

import "encoding/json"

// Role identifies who produced a message in a conversation.
//
// Mapped from:
//   - OpenAI:    messages[].role  ("system" | "user" | "assistant" | "tool")
//   - Anthropic: messages[].role  ("user" | "assistant"; "system" is lifted to UnifiedRequest.System)
//   - Gemini:    contents[].role  ("user" | "model" — codecs translate "model" ↔ "assistant")
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType discriminates the payload kind inside a ContentPart.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeAudio      ContentType = "audio"
	ContentTypeVideo      ContentType = "video"
	ContentTypeFile       ContentType = "file"
	ContentTypeReasoning  ContentType = "reasoning"   // assistant thinking / reasoning chain
	ContentTypeToolUse    ContentType = "tool_use"    // assistant invokes a tool
	ContentTypeToolResult ContentType = "tool_result" // tool returns a result back to the model
)

// MediaData carries an inline-or-remote media payload (image / audio / video / file).
//
// Exactly one of (URL, Data) should be set. MIMEType is required for inline Data,
// optional for URL (codecs may rely on Content-Type sniffing for remote resources).
type MediaData struct {
	URL      string // remote reference (OpenAI image_url.url / Gemini fileData.fileUri)
	Data     []byte // inline bytes; codecs encode/decode base64 on the wire
	MIMEType string // e.g. "image/png", "audio/mp3"
	Name     string // optional file name (only meaningful for ContentTypeFile)
}

// ToolUseBlock is a unified tool invocation produced by an assistant.
//
// Mapped from:
//   - OpenAI:    message.tool_calls[].{id, function.name, function.arguments}
//   - Anthropic: content_block(type=tool_use).{id, name, input}
//   - Gemini:    parts[].functionCall.{name, args}  — no per-call ID; codecs synth one
//
// Input is kept as raw JSON to avoid lossy any↔struct round-trips between
// protocols that disagree on argument encoding (string vs object, args vs arguments).
type ToolUseBlock struct {
	ID    string          // tool-call correlation id (synthesised for Gemini)
	Name  string          // tool / function name
	Input json.RawMessage // raw JSON arguments object
}

// ToolResultBlock is the unified payload returned from a tool back to the model.
//
// Mapped from:
//   - OpenAI:    role="tool" message.{tool_call_id, content}
//   - Anthropic: content_block(type=tool_result).{tool_use_id, content, is_error}
//   - Gemini:    parts[].functionResponse.{name, response}
type ToolResultBlock struct {
	ToolUseID string        // correlates to ToolUseBlock.ID
	Name      string        // function name (Gemini requires it; OpenAI/Anthropic optional)
	Content   []ContentPart // result payload (typically a single text part)
	IsError   bool          // true if the tool reported an error (Anthropic-native; others encode in text)
}

// ContentPart is a single block inside a Message. A Message holds an ordered
// slice of ContentPart so that text / media / tool calls / reasoning can
// interleave the way Anthropic and Gemini natively express them.
//
// Exactly one of the typed fields below should be populated based on Type:
//
//	text                    -> Text
//	image/audio/video/file  -> Media
//	reasoning               -> Reasoning
//	tool_use                -> ToolUse
//	tool_result             -> ToolResult
type ContentPart struct {
	Type       ContentType
	Text       string           // text payload (Type == ContentTypeText)
	Media      *MediaData       // image / audio / video / file payload
	Reasoning  string           // thinking / reasoning text (Type == ContentTypeReasoning)
	ToolUse    *ToolUseBlock    // assistant → tool invocation
	ToolResult *ToolResultBlock // tool → assistant result
	Extra      map[string]any   // protocol-specific carry-over (e.g. anthropic cache_control)
}

// Message is the IR-level conversation turn. All content lives in Parts; even
// pure-text messages should be expressed as a single ContentTypeText part so
// that codecs have a single, uniform iteration path.
//
// ToolCalls is a convenience denormalisation of all ContentTypeToolUse parts
// on assistant messages — useful for codecs (e.g. OpenAI) that emit tool_calls
// as a separate top-level field. Producers must keep ToolCalls in sync with
// Parts; consumers may read whichever is cheaper.
type Message struct {
	Role      Role
	Parts     []ContentPart
	ToolCalls []ToolUseBlock // mirror of all Parts[i].ToolUse on assistant messages
	Name      string         // optional speaker / tool name (OpenAI message.name)
	Extra     map[string]any // protocol-specific fields
}

// ToolDef declares a tool the model is allowed to call.
//
// Mapped from:
//   - OpenAI:    tools[].{type:"function", function:{name, description, parameters}}
//   - Anthropic: tools[].{name, description, input_schema}
//   - Gemini:    tools[].functionDeclarations[].{name, description, parameters}
type ToolDef struct {
	Name        string
	Description string
	// Parameters is a JSON Schema document describing the tool input.
	// Kept as raw JSON to preserve schema fidelity across protocols.
	Parameters json.RawMessage
}

// ThinkingConfig requests / configures model "thinking" output where supported.
//
// Mapped from:
//   - OpenAI:    reasoning_effort  ("low" | "medium" | "high")
//   - Anthropic: thinking.{type:"enabled", budget_tokens}
//   - Gemini:    generationConfig.thinkingConfig
//
// Codecs translate best-effort; absent upstream capability is silently dropped.
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens int    // anthropic budget_tokens; 0 = unset
	Effort       string // "low" | "medium" | "high" (openai-style)
}

// ResponseFormat constrains the assistant output shape.
//
// Mapped from:
//   - OpenAI:    response_format.{type, json_schema}
//   - Anthropic: no native field — encoded as a system instruction by the codec
//   - Gemini:    generationConfig.{responseMimeType, responseSchema}
type ResponseFormat struct {
	Type   string          // "text" | "json_object" | "json_schema"
	Schema json.RawMessage // JSON Schema document, when Type == "json_schema"
}

// TokenUsage normalises the three protocols' usage accounting.
//
// Mapped from:
//   - OpenAI:    usage.{prompt_tokens, completion_tokens, total_tokens,
//     completion_tokens_details.reasoning_tokens}
//   - Anthropic: usage.{input_tokens, output_tokens,
//     cache_read_input_tokens, cache_creation_input_tokens}
//   - Gemini:    usageMetadata.{promptTokenCount, candidatesTokenCount,
//     totalTokenCount, thoughtsTokenCount}
//
// TotalTokens may be zero on partial streams; consumers should compute it as
// InputTokens + OutputTokens when needed.
type TokenUsage struct {
	InputTokens              int
	OutputTokens             int
	TotalTokens              int
	CacheReadInputTokens     int // anthropic cache_read_input_tokens
	CacheCreationInputTokens int // anthropic cache_creation_input_tokens
	ReasoningTokens          int // openai reasoning_tokens / gemini thoughtsTokenCount
}

// UnifiedRequest is the IR for an inbound chat-style request, normalised
// across OpenAI / Anthropic / Gemini wire formats.
//
// Pointer-typed sampling fields preserve the "unset vs zero" distinction
// that the wire protocols rely on (e.g. Temperature=0 is a meaningful value
// distinct from "not provided").
//
// Extra captures protocol-specific knobs (logprobs, safety_settings, ...)
// that have no IR-level counterpart. Keys are namespaced by the source codec
// (see docs/IR.md "Extra conventions").
type UnifiedRequest struct {
	Model          string
	Messages       []Message
	System         string // promoted from Anthropic top-level / Gemini systemInstruction / OpenAI system message
	MaxTokens      *int
	Temperature    *float64
	TopP           *float64
	TopK           *int
	Stream         bool
	StopSequences  []string
	N              *int
	Tools          []ToolDef
	ToolChoice     any // "auto" | "none" | "required" | {type:"tool", name:"..."} (codec-specific shapes)
	Thinking       *ThinkingConfig
	ResponseFormat *ResponseFormat
	Extra          map[string]any
}

// UnifiedResponse is the IR for a completed (non-streaming) response.
type UnifiedResponse struct {
	ID           string
	Model        string
	Message      Message
	FinishReason string // normalised: "stop" | "length" | "tool_use" | "content_filter" | "error"
	Usage        *TokenUsage
	Extra        map[string]any
}

// UnifiedChunk is the IR for a single streaming delta.
//
// Delta carries incremental content for the in-progress assistant message
// (text, tool_use arguments, reasoning). The terminating chunk should set
// FinishReason and — where the upstream provides it — Usage.
type UnifiedChunk struct {
	ID           string
	Index        int
	Delta        *Message    // incremental content; nil on usage-only / terminator chunks
	FinishReason *string     // pointer to distinguish "not yet" (nil) from "" (explicit empty, rare)
	Usage        *TokenUsage // typically only on the final chunk
	Extra        map[string]any
}
