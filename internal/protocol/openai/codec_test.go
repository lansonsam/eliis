package openai

import (
	"encoding/json"
	"testing"

	"github.com/lansonsam/eliis/internal/core/types"
)

func TestEncodeChatCompletionRequest(t *testing.T) {
	maxTokens := 256
	temp := 0.2
	params := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	toolInput := json.RawMessage(`{"city":"Paris"}`)

	req, err := EncodeChatCompletionRequest(&types.UnifiedRequest{
		Model:       "gpt-test",
		System:      "You are concise.",
		MaxTokens:   &maxTokens,
		Temperature: &temp,
		Tools: []types.ToolDef{{
			Name:        "get_weather",
			Description: "Get weather.",
			Parameters:  params,
		}},
		Messages: []types.Message{
			{
				Role:  types.RoleUser,
				Parts: []types.ContentPart{{Type: types.ContentTypeText, Text: "Hi"}},
			},
			{
				Role: types.RoleAssistant,
				Parts: []types.ContentPart{{
					Type: types.ContentTypeToolUse,
					ToolUse: &types.ToolUseBlock{
						ID:    "call_1",
						Name:  "get_weather",
						Input: toolInput,
					},
				}},
			},
			{
				Role: types.RoleUser,
				Parts: []types.ContentPart{{
					Type: types.ContentTypeToolResult,
					ToolResult: &types.ToolResultBlock{
						ToolUseID: "call_1",
						Name:      "get_weather",
						Content: []types.ContentPart{{
							Type: types.ContentTypeText,
							Text: "Sunny",
						}},
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("EncodeChatCompletionRequest() error = %v", err)
	}

	if req.Model != "gpt-test" {
		t.Fatalf("Model = %q, want gpt-test", req.Model)
	}
	if req.MaxCompletionTokens == nil || *req.MaxCompletionTokens != maxTokens {
		t.Fatalf("MaxCompletionTokens = %v, want %d", req.MaxCompletionTokens, maxTokens)
	}
	if req.Temperature == nil || *req.Temperature != temp {
		t.Fatalf("Temperature = %v, want %v", req.Temperature, temp)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function == nil || req.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("Tools = %#v, want function get_weather", req.Tools)
	}
	if len(req.Messages) != 4 {
		t.Fatalf("len(Messages) = %d, want 4", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are concise." {
		t.Fatalf("system message = %#v", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "Hi" {
		t.Fatalf("user message = %#v", req.Messages[1])
	}
	if req.Messages[2].Role != "assistant" || len(req.Messages[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool message = %#v", req.Messages[2])
	}
	if req.Messages[2].ToolCalls[0].Function == nil || req.Messages[2].ToolCalls[0].Function.Arguments != string(toolInput) {
		t.Fatalf("tool call = %#v", req.Messages[2].ToolCalls[0])
	}
	if req.Messages[3].Role != "tool" || req.Messages[3].ToolCallID != "call_1" || req.Messages[3].Content != "Sunny" {
		t.Fatalf("tool result message = %#v", req.Messages[3])
	}
}

func TestDecodeChatCompletionResponseText(t *testing.T) {
	resp, err := DecodeChatCompletionResponse(ChatCompletionResponse{
		ID:    "chatcmpl_1",
		Model: "gpt-test",
		Choices: []Choice{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: "Hello!",
			},
			FinishReason: "stop",
		}},
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			CompletionTokensDetails: &CompletionTokensDetails{
				ReasoningTokens: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("DecodeChatCompletionResponse() error = %v", err)
	}

	if resp.ID != "chatcmpl_1" || resp.Model != "gpt-test" {
		t.Fatalf("response id/model = %q/%q", resp.ID, resp.Model)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if len(resp.Message.Parts) != 1 || resp.Message.Parts[0].Text != "Hello!" {
		t.Fatalf("Message.Parts = %#v", resp.Message.Parts)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 || resp.Usage.ReasoningTokens != 2 {
		t.Fatalf("Usage = %#v", resp.Usage)
	}
}

func TestDecodeChatCompletionResponseToolCalls(t *testing.T) {
	resp, err := DecodeChatCompletionResponse(ChatCompletionResponse{
		ID:    "chatcmpl_2",
		Model: "gpt-test",
		Choices: []Choice{{
			Index: 0,
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: &FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Paris"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	})
	if err != nil {
		t.Fatalf("DecodeChatCompletionResponse() error = %v", err)
	}

	if resp.FinishReason != "tool_use" {
		t.Fatalf("FinishReason = %q, want tool_use", resp.FinishReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %#v", resp.Message.ToolCalls)
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "get_weather" || string(call.Input) != `{"city":"Paris"}` {
		t.Fatalf("ToolCall = %#v", call)
	}
	if len(resp.Message.Parts) != 1 || resp.Message.Parts[0].ToolUse == nil {
		t.Fatalf("Parts = %#v", resp.Message.Parts)
	}
}
