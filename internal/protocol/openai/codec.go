package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lansonsam/eliis/internal/core/types"
)

const ProtocolName = "openai"

// Codec translates OpenAI wire objects to and from Eliis IR.
type Codec struct{}

func NewCodec() *Codec {
	return &Codec{}
}

func (c *Codec) Name() string {
	return ProtocolName
}

func (c *Codec) DecodeRequest(r *http.Request) (*types.UnifiedRequest, error) {
	defer r.Body.Close()

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("decode openai request: %w", err)
	}
	return DecodeChatCompletionRequest(req)
}

func (c *Codec) EncodeResponse(_ *types.UnifiedResponse, _ http.ResponseWriter) error {
	return errors.New("openai response encoding is not implemented yet")
}

func (c *Codec) DecodeStreamChunk(_ []byte) (*types.UnifiedChunk, error) {
	return nil, errors.New("openai stream chunk decoding is not implemented yet")
}

func (c *Codec) EncodeStreamChunk(_ *types.UnifiedChunk) ([]byte, error) {
	return nil, errors.New("openai stream chunk encoding is not implemented yet")
}

// DecodeChatCompletionRequest maps an OpenAI Chat Completions request into IR.
func DecodeChatCompletionRequest(req ChatCompletionRequest) (*types.UnifiedRequest, error) {
	system, messages, err := decodeMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	maxTokens := req.MaxCompletionTokens
	if maxTokens == nil {
		maxTokens = req.MaxTokens
	}

	stream := false
	if req.Stream != nil {
		stream = *req.Stream
	}

	return &types.UnifiedRequest{
		Model:          req.Model,
		Messages:       messages,
		System:         system,
		MaxTokens:      maxTokens,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		Stream:         stream,
		StopSequences:  decodeStop(req.Stop),
		N:              req.N,
		Tools:          decodeTools(req.Tools),
		ToolChoice:     req.ToolChoice,
		Thinking:       decodeThinking(req.ReasoningEffort),
		ResponseFormat: decodeResponseFormat(req.ResponseFormat),
		Extra:          decodeExtra(req),
	}, nil
}

// EncodeChatCompletionRequest maps IR into an OpenAI Chat Completions request.
func EncodeChatCompletionRequest(req *types.UnifiedRequest) (*ChatCompletionRequest, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}

	messages, err := encodeChatMessages(req.System, req.Messages)
	if err != nil {
		return nil, err
	}

	stream := req.Stream
	return &ChatCompletionRequest{
		Model:               req.Model,
		Messages:            messages,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		Stop:                encodeStop(req.StopSequences),
		N:                   req.N,
		MaxCompletionTokens: req.MaxTokens,
		Stream:              &stream,
		ReasoningEffort:     encodeReasoningEffort(req.Thinking),
		Tools:               encodeTools(req.Tools),
		ToolChoice:          encodeToolChoice(req.ToolChoice),
		ResponseFormat:      encodeResponseFormat(req.ResponseFormat),
	}, nil
}

// DecodeChatCompletionResponse maps an OpenAI Chat Completions response into IR.
func DecodeChatCompletionResponse(resp ChatCompletionResponse) (*types.UnifiedResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, errors.New("openai response has no choices")
	}
	choice := resp.Choices[0]

	parts, err := decodeContent(choice.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("decode openai response content: %w", err)
	}
	toolCalls := decodeToolCalls(choice.Message.ToolCalls)
	parts = append(parts, toolUseParts(toolCalls)...)

	message := types.Message{
		Role:      types.RoleAssistant,
		Parts:     parts,
		ToolCalls: toolCalls,
		Name:      choice.Message.Name,
	}

	return &types.UnifiedResponse{
		ID:           resp.ID,
		Model:        resp.Model,
		Message:      message,
		FinishReason: decodeFinishReason(choice.FinishReason),
		Usage:        decodeUsage(resp.Usage),
		Extra:        decodeResponseExtra(resp),
	}, nil
}

func decodeMessages(in []ChatMessage) (string, []types.Message, error) {
	var systemParts []string
	out := make([]types.Message, 0, len(in))

	for _, msg := range in {
		switch msg.Role {
		case "system", "developer":
			text, ok := msg.Content.(string)
			if !ok {
				return "", nil, fmt.Errorf("%s message content must be string", msg.Role)
			}
			if text != "" {
				systemParts = append(systemParts, text)
			}
		case "user", "assistant":
			parts, err := decodeContent(msg.Content)
			if err != nil {
				return "", nil, fmt.Errorf("decode %s content: %w", msg.Role, err)
			}
			toolCalls := decodeToolCalls(msg.ToolCalls)
			out = append(out, types.Message{
				Role:      types.Role(msg.Role),
				Parts:     append(parts, toolUseParts(toolCalls)...),
				ToolCalls: toolCalls,
				Name:      msg.Name,
			})
		case "tool":
			parts, err := decodeContent(msg.Content)
			if err != nil {
				return "", nil, fmt.Errorf("decode tool content: %w", err)
			}
			out = append(out, types.Message{
				Role: types.RoleTool,
				Parts: []types.ContentPart{{
					Type: types.ContentTypeToolResult,
					ToolResult: &types.ToolResultBlock{
						ToolUseID: msg.ToolCallID,
						Name:      msg.Name,
						Content:   parts,
					},
				}},
				Name: msg.Name,
			})
		default:
			return "", nil, fmt.Errorf("unsupported openai role %q", msg.Role)
		}
	}

	return strings.Join(systemParts, "\n\n"), out, nil
}

func decodeContent(content any) ([]types.ContentPart, error) {
	switch v := content.(type) {
	case nil:
		return nil, nil
	case string:
		if v == "" {
			return nil, nil
		}
		return []types.ContentPart{{Type: types.ContentTypeText, Text: v}}, nil
	case []ContentPart:
		return decodeTypedContentParts(v)
	case []any:
		parts := make([]ContentPart, 0, len(v))
		for _, item := range v {
			raw, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			var part ContentPart
			if err := json.Unmarshal(raw, &part); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		return decodeTypedContentParts(parts)
	default:
		return nil, fmt.Errorf("unsupported openai content shape %T", content)
	}
}

func decodeTypedContentParts(in []ContentPart) ([]types.ContentPart, error) {
	out := make([]types.ContentPart, 0, len(in))
	for _, part := range in {
		switch part.Type {
		case "text":
			out = append(out, types.ContentPart{Type: types.ContentTypeText, Text: part.Text})
		case "image_url":
			if part.ImageURL == nil {
				return nil, errors.New("image_url part missing payload")
			}
			out = append(out, types.ContentPart{
				Type: types.ContentTypeImage,
				Media: &types.MediaData{
					URL: part.ImageURL.URL,
				},
				Extra: map[string]any{
					"openai:image_detail": part.ImageURL.Detail,
				},
			})
		default:
			return nil, fmt.Errorf("unsupported openai content part type %q", part.Type)
		}
	}
	return out, nil
}

func decodeToolCalls(in []ToolCall) []types.ToolUseBlock {
	out := make([]types.ToolUseBlock, 0, len(in))
	for _, call := range in {
		if call.Type != "function" || call.Function == nil {
			continue
		}
		out = append(out, types.ToolUseBlock{
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: normalizeToolInput(call.Function.Arguments),
		})
	}
	return out
}

func toolUseParts(in []types.ToolUseBlock) []types.ContentPart {
	out := make([]types.ContentPart, 0, len(in))
	for i := range in {
		call := in[i]
		out = append(out, types.ContentPart{
			Type:    types.ContentTypeToolUse,
			ToolUse: &call,
		})
	}
	return out
}

func decodeTools(in []Tool) []types.ToolDef {
	out := make([]types.ToolDef, 0, len(in))
	for _, tool := range in {
		if tool.Type != "function" || tool.Function == nil {
			continue
		}
		out = append(out, types.ToolDef{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	return out
}

func decodeStop(stop any) []string {
	switch v := stop.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func decodeThinking(effort string) *types.ThinkingConfig {
	if effort == "" {
		return nil
	}
	return &types.ThinkingConfig{
		Enabled: true,
		Effort:  effort,
	}
}

func decodeResponseFormat(in *ResponseFormat) *types.ResponseFormat {
	if in == nil {
		return nil
	}

	format := &types.ResponseFormat{Type: in.Type}
	if in.JSONSchema != nil {
		format.Schema = in.JSONSchema.Schema
	}
	return format
}

func decodeExtra(req ChatCompletionRequest) map[string]any {
	extra := make(map[string]any)
	if req.FrequencyPenalty != nil {
		extra["openai:frequency_penalty"] = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		extra["openai:presence_penalty"] = req.PresencePenalty
	}
	if req.Seed != nil {
		extra["openai:seed"] = req.Seed
	}
	if req.Logprobs != nil {
		extra["openai:logprobs"] = req.Logprobs
	}
	if req.TopLogprobs != nil {
		extra["openai:top_logprobs"] = req.TopLogprobs
	}
	if req.User != "" {
		extra["openai:user"] = req.User
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func encodeChatMessages(system string, in []types.Message) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(in)+1)
	if system != "" {
		out = append(out, ChatMessage{Role: "system", Content: system})
	}

	for _, msg := range in {
		switch msg.Role {
		case types.RoleSystem:
			content, err := encodeTextLikeContent(msg.Parts)
			if err != nil {
				return nil, fmt.Errorf("encode system content: %w", err)
			}
			if text, ok := content.(string); ok && text != "" {
				out = append(out, ChatMessage{Role: "system", Content: text, Name: msg.Name})
			}
		case types.RoleUser:
			messages, err := encodeUserMessage(msg)
			if err != nil {
				return nil, err
			}
			out = append(out, messages...)
		case types.RoleAssistant:
			encoded, err := encodeAssistantMessage(msg)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded)
		case types.RoleTool:
			messages, err := encodeToolMessages(msg.Parts, msg.Name)
			if err != nil {
				return nil, err
			}
			out = append(out, messages...)
		default:
			return nil, fmt.Errorf("unsupported IR role %q for openai", msg.Role)
		}
	}
	return out, nil
}

func encodeUserMessage(msg types.Message) ([]ChatMessage, error) {
	var out []ChatMessage
	var regular []types.ContentPart

	flushRegular := func() error {
		if len(regular) == 0 {
			return nil
		}
		content, err := encodeOpenAIContent(regular)
		if err != nil {
			return err
		}
		out = append(out, ChatMessage{Role: "user", Content: content, Name: msg.Name})
		regular = nil
		return nil
	}

	for _, part := range msg.Parts {
		if part.Type != types.ContentTypeToolResult {
			regular = append(regular, part)
			continue
		}
		if err := flushRegular(); err != nil {
			return nil, fmt.Errorf("encode user content: %w", err)
		}
		if part.ToolResult == nil {
			return nil, errors.New("tool_result content missing payload")
		}
		content, err := encodeToolResultContent(part.ToolResult.Content)
		if err != nil {
			return nil, err
		}
		out = append(out, ChatMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: part.ToolResult.ToolUseID,
			Name:       part.ToolResult.Name,
		})
	}
	if err := flushRegular(); err != nil {
		return nil, fmt.Errorf("encode user content: %w", err)
	}
	return out, nil
}

func encodeAssistantMessage(msg types.Message) (ChatMessage, error) {
	contentParts := make([]types.ContentPart, 0, len(msg.Parts))
	toolCalls := make([]ToolCall, 0, len(msg.ToolCalls))

	for _, part := range msg.Parts {
		switch part.Type {
		case types.ContentTypeToolUse:
			if part.ToolUse == nil {
				return ChatMessage{}, errors.New("tool_use content missing payload")
			}
			toolCalls = append(toolCalls, encodeToolCall(*part.ToolUse))
		case types.ContentTypeToolResult:
			return ChatMessage{}, errors.New("assistant message cannot contain tool_result for openai")
		default:
			contentParts = append(contentParts, part)
		}
	}
	if len(toolCalls) == 0 && len(msg.ToolCalls) > 0 {
		for _, call := range msg.ToolCalls {
			toolCalls = append(toolCalls, encodeToolCall(call))
		}
	}

	content, err := encodeOpenAIContent(contentParts)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("encode assistant content: %w", err)
	}
	return ChatMessage{
		Role:      "assistant",
		Content:   content,
		Name:      msg.Name,
		ToolCalls: toolCalls,
	}, nil
}

func encodeToolMessages(parts []types.ContentPart, name string) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(parts))
	for _, part := range parts {
		if part.Type != types.ContentTypeToolResult || part.ToolResult == nil {
			continue
		}
		content, err := encodeToolResultContent(part.ToolResult.Content)
		if err != nil {
			return nil, err
		}
		toolName := part.ToolResult.Name
		if toolName == "" {
			toolName = name
		}
		out = append(out, ChatMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: part.ToolResult.ToolUseID,
			Name:       toolName,
		})
	}
	return out, nil
}

func encodeOpenAIContent(parts []types.ContentPart) (any, error) {
	if len(parts) == 0 {
		return "", nil
	}
	return encodeTextLikeContent(parts)
}

func encodeTextLikeContent(parts []types.ContentPart) (any, error) {
	if len(parts) == 1 && parts[0].Type == types.ContentTypeText {
		return parts[0].Text, nil
	}

	allText := true
	var texts []string
	encoded := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case types.ContentTypeText:
			texts = append(texts, part.Text)
			encoded = append(encoded, ContentPart{Type: "text", Text: part.Text})
		case types.ContentTypeImage:
			allText = false
			if part.Media == nil || part.Media.URL == "" {
				return nil, errors.New("image content missing URL for openai")
			}
			encoded = append(encoded, ContentPart{
				Type: "image_url",
				ImageURL: &ContentImageURL{
					URL: part.Media.URL,
				},
			})
		case types.ContentTypeReasoning:
			// Do not forward prior hidden reasoning to OpenAI-compatible upstreams.
		case types.ContentTypeToolUse, types.ContentTypeToolResult:
			return nil, fmt.Errorf("%s content is handled outside message content", part.Type)
		default:
			return nil, fmt.Errorf("unsupported IR content type %q for openai", part.Type)
		}
	}
	if allText {
		return strings.Join(texts, "\n"), nil
	}
	return encoded, nil
}

func encodeToolResultContent(parts []types.ContentPart) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	var texts []string
	for _, part := range parts {
		switch part.Type {
		case types.ContentTypeText:
			texts = append(texts, part.Text)
		default:
			return "", fmt.Errorf("unsupported tool_result content type %q for openai", part.Type)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func encodeTools(in []types.ToolDef) []Tool {
	out := make([]Tool, 0, len(in))
	for _, tool := range in {
		out = append(out, Tool{
			Type: "function",
			Function: &FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return out
}

func encodeToolCall(call types.ToolUseBlock) ToolCall {
	return ToolCall{
		ID:   call.ID,
		Type: "function",
		Function: &FunctionCall{
			Name:      call.Name,
			Arguments: string(normalizeRawJSON(call.Input)),
		},
	}
}

func encodeStop(stop []string) any {
	switch len(stop) {
	case 0:
		return nil
	case 1:
		return stop[0]
	default:
		return stop
	}
}

func encodeReasoningEffort(in *types.ThinkingConfig) string {
	if in == nil || !in.Enabled {
		return ""
	}
	return in.Effort
}

func encodeResponseFormat(in *types.ResponseFormat) *ResponseFormat {
	if in == nil {
		return nil
	}
	out := &ResponseFormat{Type: in.Type}
	if len(in.Schema) > 0 {
		out.JSONSchema = &ResponseFormatSchema{Schema: in.Schema}
	}
	return out
}

func encodeToolChoice(choice any) any {
	switch v := choice.(type) {
	case nil:
		return nil
	case string:
		switch v {
		case "auto", "none", "required":
			return v
		case "any":
			return "required"
		default:
			return nil
		}
	case map[string]any:
		return encodeToolChoiceMap(v)
	default:
		return choice
	}
}

func encodeToolChoiceMap(choice map[string]any) any {
	typ, _ := choice["type"].(string)
	switch typ {
	case "auto", "none":
		return typ
	case "any", "required":
		return "required"
	case "tool":
		name, _ := choice["name"].(string)
		if name == "" {
			return nil
		}
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": name,
			},
		}
	default:
		return nil
	}
}

func decodeFinishReason(reason string) string {
	switch reason {
	case "stop", "":
		return "stop"
	case "length":
		return "length"
	case "tool_calls", "function_call":
		return "tool_use"
	case "content_filter":
		return "content_filter"
	default:
		return reason
	}
}

func decodeUsage(in *Usage) *types.TokenUsage {
	if in == nil {
		return nil
	}
	usage := &types.TokenUsage{
		InputTokens:  in.PromptTokens,
		OutputTokens: in.CompletionTokens,
		TotalTokens:  in.TotalTokens,
	}
	if in.CompletionTokensDetails != nil {
		usage.ReasoningTokens = in.CompletionTokensDetails.ReasoningTokens
	}
	return usage
}

func decodeResponseExtra(resp ChatCompletionResponse) map[string]any {
	extra := make(map[string]any)
	if resp.SystemFingerprint != "" {
		extra["openai:system_fingerprint"] = resp.SystemFingerprint
	}
	if resp.ServiceTier != "" {
		extra["openai:service_tier"] = resp.ServiceTier
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func normalizeToolInput(arguments string) json.RawMessage {
	return normalizeRawJSON(json.RawMessage(arguments))
}

func normalizeRawJSON(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(trimmed)) {
		return json.RawMessage(trimmed)
	}
	return json.RawMessage(`{}`)
}
