package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/lansonsam/eliis/internal/core/types"
)

func TestDecodeMessagesRequestSystemBlocks(t *testing.T) {
	maxTokens := 128
	req, err := DecodeMessagesRequest(MessagesRequest{
		Model:     "claude-test",
		MaxTokens: &maxTokens,
		System: []any{
			map[string]any{"type": "text", "text": "System one."},
			map[string]any{"type": "text", "text": "System two."},
		},
		Messages: []InputMessage{{
			Role:    "user",
			Content: "Hello",
		}},
	})
	if err != nil {
		t.Fatalf("DecodeMessagesRequest() error = %v", err)
	}
	if req.System != "System one.\n\nSystem two." {
		t.Fatalf("System = %q", req.System)
	}
	if req.MaxTokens == nil || *req.MaxTokens != maxTokens {
		t.Fatalf("MaxTokens = %v, want %d", req.MaxTokens, maxTokens)
	}
	if len(req.Messages) != 1 || req.Messages[0].Parts[0].Text != "Hello" {
		t.Fatalf("Messages = %#v", req.Messages)
	}
}

func TestEncodeMessagesResponseText(t *testing.T) {
	resp, err := EncodeMessagesResponse(&types.UnifiedResponse{
		ID:           "msg_test",
		Model:        "claude-test",
		FinishReason: "stop",
		Message: types.Message{
			Role: types.RoleAssistant,
			Parts: []types.ContentPart{{
				Type: types.ContentTypeText,
				Text: "Hello!",
			}},
		},
		Usage: &types.TokenUsage{
			InputTokens:              10,
			OutputTokens:             5,
			CacheCreationInputTokens: 2,
			CacheReadInputTokens:     3,
		},
	})
	if err != nil {
		t.Fatalf("EncodeMessagesResponse() error = %v", err)
	}

	if resp.Type != "message" || resp.Role != "assistant" {
		t.Fatalf("type/role = %q/%q", resp.Type, resp.Role)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "Hello!" {
		t.Fatalf("Content = %#v", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 || resp.Usage.CacheCreationInputTokens != 2 || resp.Usage.CacheReadInputTokens != 3 {
		t.Fatalf("Usage = %#v", resp.Usage)
	}
}

func TestEncodeMessagesResponseToolUse(t *testing.T) {
	input := json.RawMessage(`{"city":"Paris"}`)
	resp, err := EncodeMessagesResponse(&types.UnifiedResponse{
		ID:           "msg_tool",
		Model:        "claude-test",
		FinishReason: "tool_use",
		Message: types.Message{
			Role: types.RoleAssistant,
			Parts: []types.ContentPart{{
				Type: types.ContentTypeToolUse,
				ToolUse: &types.ToolUseBlock{
					ID:    "call_1",
					Name:  "get_weather",
					Input: input,
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("EncodeMessagesResponse() error = %v", err)
	}

	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content = %#v", resp.Content)
	}
	block := resp.Content[0]
	if block.Type != "tool_use" || block.ID != "call_1" || block.Name != "get_weather" || string(block.Input) != string(input) {
		t.Fatalf("tool block = %#v", block)
	}
}
