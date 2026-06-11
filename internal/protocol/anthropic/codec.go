package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lansonsam/eliis/internal/core/types"
)

const ProtocolName = "anthropic"

// Codec translates Anthropic wire objects to and from Eliis IR.
type Codec struct{}

func NewCodec() *Codec {
	return &Codec{}
}

func (c *Codec) Name() string {
	return ProtocolName
}

func (c *Codec) DecodeRequest(r *http.Request) (*types.UnifiedRequest, error) {
	defer r.Body.Close()

	var req MessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("decode anthropic request: %w", err)
	}
	return DecodeMessagesRequest(req)
}

func (c *Codec) EncodeResponse(resp *types.UnifiedResponse, w http.ResponseWriter) error {
	out, err := EncodeMessagesResponse(resp)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(out)
}

func (c *Codec) DecodeStreamChunk(_ []byte) (*types.UnifiedChunk, error) {
	return nil, errors.New("anthropic stream chunk decoding is not implemented yet")
}

func (c *Codec) EncodeStreamChunk(_ *types.UnifiedChunk) ([]byte, error) {
	return nil, errors.New("anthropic stream chunk encoding is not implemented yet")
}

// DecodeMessagesRequest maps an Anthropic Messages request into IR.
func DecodeMessagesRequest(req MessagesRequest) (*types.UnifiedRequest, error) {
	messages, err := decodeMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	stream := false
	if req.Stream != nil {
		stream = *req.Stream
	}

	return &types.UnifiedRequest{
		Model:         req.Model,
		Messages:      messages,
		System:        decodeSystem(req.System),
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		TopK:          req.TopK,
		Stream:        stream,
		StopSequences: req.StopSequences,
		Tools:         decodeTools(req.Tools),
		ToolChoice:    req.ToolChoice,
		Thinking:      decodeThinking(req.Thinking),
		Extra:         decodeExtra(req),
	}, nil
}

// EncodeMessagesRequest maps IR into an Anthropic Messages request.
func EncodeMessagesRequest(req *types.UnifiedRequest) (*MessagesRequest, error) {
	messages, err := encodeMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	stream := req.Stream
	return &MessagesRequest{
		Model:         req.Model,
		Messages:      messages,
		MaxTokens:     req.MaxTokens,
		System:        req.System,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		TopK:          req.TopK,
		StopSequences: req.StopSequences,
		Stream:        &stream,
		Tools:         encodeTools(req.Tools),
		ToolChoice:    encodeToolChoice(req.ToolChoice),
		Thinking:      encodeThinking(req.Thinking),
	}, nil
}

// EncodeMessagesResponse maps IR into an Anthropic Messages response.
func EncodeMessagesResponse(resp *types.UnifiedResponse) (*MessagesResponse, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}
	content, err := encodeResponseContent(resp.Message.Parts)
	if err != nil {
		return nil, err
	}
	return &MessagesResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      resp.Model,
		StopReason: encodeStopReason(resp.FinishReason),
		Usage:      encodeUsage(resp.Usage),
	}, nil
}

// WriteError writes an Anthropic-compatible error envelope.
func WriteError(w http.ResponseWriter, status int, errorType string, message string) {
	if errorType == "" {
		errorType = "api_error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Type: "error",
		Error: ErrorDetail{
			Type:    errorType,
			Message: message,
		},
	})
}

func decodeMessages(in []InputMessage) ([]types.Message, error) {
	out := make([]types.Message, 0, len(in))
	for _, msg := range in {
		parts, err := decodeContent(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("decode anthropic %s content: %w", msg.Role, err)
		}
		out = append(out, types.Message{
			Role:      types.Role(msg.Role),
			Parts:     parts,
			ToolCalls: collectToolCalls(parts),
		})
	}
	return out, nil
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
	case []ContentBlockParam:
		return decodeContentBlocks(v)
	case []any:
		blocks := make([]ContentBlockParam, 0, len(v))
		for _, item := range v {
			raw, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			var block ContentBlockParam
			if err := json.Unmarshal(raw, &block); err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		}
		return decodeContentBlocks(blocks)
	default:
		return nil, fmt.Errorf("unsupported anthropic content shape %T", content)
	}
}

func decodeContentBlocks(in []ContentBlockParam) ([]types.ContentPart, error) {
	out := make([]types.ContentPart, 0, len(in))
	for _, block := range in {
		switch block.Type {
		case "text":
			out = append(out, types.ContentPart{Type: types.ContentTypeText, Text: block.Text})
		case "tool_use":
			out = append(out, types.ContentPart{
				Type: types.ContentTypeToolUse,
				ToolUse: &types.ToolUseBlock{
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				},
			})
		case "tool_result":
			resultParts, err := decodeContent(block.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, types.ContentPart{
				Type: types.ContentTypeToolResult,
				ToolResult: &types.ToolResultBlock{
					ToolUseID: block.ToolUseID,
					Content:   resultParts,
					IsError:   block.IsError != nil && *block.IsError,
				},
			})
		case "thinking":
			out = append(out, types.ContentPart{
				Type:      types.ContentTypeReasoning,
				Reasoning: block.Thinking,
			})
		default:
			return nil, fmt.Errorf("unsupported anthropic content block type %q", block.Type)
		}
	}
	return out, nil
}

func encodeMessages(in []types.Message) ([]InputMessage, error) {
	out := make([]InputMessage, 0, len(in))
	for _, msg := range in {
		switch msg.Role {
		case types.RoleUser, types.RoleAssistant:
			content, err := encodeContent(msg.Parts)
			if err != nil {
				return nil, fmt.Errorf("encode %s content: %w", msg.Role, err)
			}
			out = append(out, InputMessage{
				Role:    string(msg.Role),
				Content: content,
			})
		case types.RoleTool:
			content, err := encodeContent(msg.Parts)
			if err != nil {
				return nil, fmt.Errorf("encode tool content: %w", err)
			}
			out = append(out, InputMessage{
				Role:    "user",
				Content: content,
			})
		default:
			return nil, fmt.Errorf("unsupported IR role %q for anthropic", msg.Role)
		}
	}
	return out, nil
}

func encodeContent(parts []types.ContentPart) (any, error) {
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 && parts[0].Type == types.ContentTypeText {
		return parts[0].Text, nil
	}

	out := make([]ContentBlockParam, 0, len(parts))
	for _, part := range parts {
		block, err := encodeContentPart(part)
		if err != nil {
			return nil, err
		}
		out = append(out, block)
	}
	return out, nil
}

func encodeContentPart(part types.ContentPart) (ContentBlockParam, error) {
	switch part.Type {
	case types.ContentTypeText:
		return ContentBlockParam{Type: "text", Text: part.Text}, nil
	case types.ContentTypeImage:
		if part.Media == nil {
			return ContentBlockParam{}, errors.New("image content missing media")
		}
		return ContentBlockParam{
			Type: "image",
			Source: &BlockSource{
				Type: "url",
				URL:  part.Media.URL,
			},
		}, nil
	case types.ContentTypeToolUse:
		if part.ToolUse == nil {
			return ContentBlockParam{}, errors.New("tool_use content missing payload")
		}
		return ContentBlockParam{
			Type:  "tool_use",
			ID:    part.ToolUse.ID,
			Name:  part.ToolUse.Name,
			Input: part.ToolUse.Input,
		}, nil
	case types.ContentTypeToolResult:
		if part.ToolResult == nil {
			return ContentBlockParam{}, errors.New("tool_result content missing payload")
		}
		content, err := encodeContent(part.ToolResult.Content)
		if err != nil {
			return ContentBlockParam{}, err
		}
		return ContentBlockParam{
			Type:      "tool_result",
			ToolUseID: part.ToolResult.ToolUseID,
			IsError:   boolPtr(part.ToolResult.IsError),
			Content:   content,
		}, nil
	case types.ContentTypeReasoning:
		return ContentBlockParam{Type: "thinking", Thinking: part.Reasoning}, nil
	default:
		return ContentBlockParam{}, fmt.Errorf("unsupported IR content type %q for anthropic", part.Type)
	}
}

func encodeResponseContent(parts []types.ContentPart) ([]ContentBlock, error) {
	out := make([]ContentBlock, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case types.ContentTypeText:
			if part.Text != "" {
				out = append(out, ContentBlock{Type: "text", Text: part.Text})
			}
		case types.ContentTypeReasoning:
			out = append(out, ContentBlock{Type: "thinking", Thinking: part.Reasoning})
		case types.ContentTypeToolUse:
			if part.ToolUse == nil {
				return nil, errors.New("tool_use content missing payload")
			}
			out = append(out, ContentBlock{
				Type:  "tool_use",
				ID:    part.ToolUse.ID,
				Name:  part.ToolUse.Name,
				Input: part.ToolUse.Input,
			})
		case types.ContentTypeToolResult:
			return nil, errors.New("tool_result is not valid in anthropic assistant response")
		default:
			return nil, fmt.Errorf("unsupported IR content type %q for anthropic response", part.Type)
		}
	}
	if len(out) == 0 {
		out = append(out, ContentBlock{Type: "text", Text: ""})
	}
	return out, nil
}

func decodeSystem(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []ContentBlockParam:
		return textFromSystemBlocks(v)
	case []any:
		blocks := make([]ContentBlockParam, 0, len(v))
		for _, item := range v {
			raw, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var block ContentBlockParam
			if err := json.Unmarshal(raw, &block); err != nil {
				continue
			}
			blocks = append(blocks, block)
		}
		return textFromSystemBlocks(blocks)
	default:
		return ""
	}
}

func textFromSystemBlocks(blocks []ContentBlockParam) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func decodeTools(in []ToolDef) []types.ToolDef {
	out := make([]types.ToolDef, 0, len(in))
	for _, tool := range in {
		out = append(out, types.ToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}
	return out
}

func encodeTools(in []types.ToolDef) []ToolDef {
	out := make([]ToolDef, 0, len(in))
	for _, tool := range in {
		out = append(out, ToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}
	return out
}

func collectToolCalls(parts []types.ContentPart) []types.ToolUseBlock {
	var out []types.ToolUseBlock
	for _, part := range parts {
		if part.Type == types.ContentTypeToolUse && part.ToolUse != nil {
			out = append(out, *part.ToolUse)
		}
	}
	return out
}

func decodeThinking(in *ThinkingConfig) *types.ThinkingConfig {
	if in == nil {
		return nil
	}
	cfg := &types.ThinkingConfig{
		Enabled: in.Type == "enabled" || in.Type == "adaptive",
	}
	if in.BudgetTokens != nil {
		cfg.BudgetTokens = *in.BudgetTokens
	}
	return cfg
}

func encodeThinking(in *types.ThinkingConfig) *ThinkingConfig {
	if in == nil || !in.Enabled {
		return nil
	}
	cfg := &ThinkingConfig{Type: "enabled"}
	if in.BudgetTokens > 0 {
		cfg.BudgetTokens = &in.BudgetTokens
	}
	return cfg
}

func encodeToolChoice(choice any) any {
	switch v := choice.(type) {
	case string:
		switch v {
		case "auto":
			return ToolChoiceAuto{Type: "auto"}
		case "required":
			return ToolChoiceAny{Type: "any"}
		case "none":
			return ToolChoiceNone{Type: "none"}
		default:
			return nil
		}
	default:
		return choice
	}
}

func decodeExtra(req MessagesRequest) map[string]any {
	extra := make(map[string]any)
	if req.Metadata != nil {
		extra["anthropic:metadata"] = req.Metadata
	}
	if req.ServiceTier != "" {
		extra["anthropic:service_tier"] = req.ServiceTier
	}
	if len(req.McpServers) > 0 {
		extra["anthropic:mcp_servers"] = req.McpServers
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func encodeStopReason(reason string) string {
	switch reason {
	case "stop", "":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_use":
		return "tool_use"
	case "content_filter":
		return "refusal"
	default:
		return reason
	}
}

func encodeUsage(in *types.TokenUsage) *Usage {
	if in == nil {
		return nil
	}
	return &Usage{
		InputTokens:              in.InputTokens,
		OutputTokens:             in.OutputTokens,
		CacheCreationInputTokens: in.CacheCreationInputTokens,
		CacheReadInputTokens:     in.CacheReadInputTokens,
	}
}

func boolPtr(v bool) *bool {
	return &v
}
