package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"glance/internal/agent/provider"
)

func TestStreamRestoresPriorReasoningForDeepSeekToolTurn(t *testing.T) {
	var posted struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&posted); err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"继续检查\"}}]}\n" +
				"data: [DONE]\n")),
			Request: request,
		}, nil
	})}
	model := New(Config{BaseURL: "https://deepseek.example/v1", APIKey: "test-key", HTTPClient: client})
	request := provider.ModelRequest{
		Model: "deepseek-chat",
		Messages: []provider.ModelMessage{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "检查文档"},
			{Role: "assistant", Content: "正在读取", ToolCalls: []provider.ModelToolCall{{
				ID: "call-1", Type: "function", Function: provider.ModelToolCallFunction{Name: "inspect", Arguments: "{}"},
			}}},
			{Role: "tool", ToolCallID: "call-1", ToolName: "inspect", Content: "读取完成"},
		},
		ReasoningByMessage: map[int]string{1: "不能附加到用户消息", 2: "我需要先确认文档范围。"},
		Tools:              []provider.ModelTool{{Type: "function", Function: provider.ModelToolFunction{Name: "inspect", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	}
	if err := model.Stream(context.Background(), request, func(provider.ModelEvent) error { return nil }); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var reasoning string
	if err := json.Unmarshal(posted.Messages[2]["reasoning_content"], &reasoning); err != nil {
		t.Fatalf("decode assistant reasoning_content: %v", err)
	}
	if reasoning != "我需要先确认文档范围。" {
		t.Fatalf("assistant reasoning_content=%q", reasoning)
	}
	if _, exists := posted.Messages[1]["reasoning_content"]; exists {
		t.Fatal("provider-specific reasoning was attached to a user message")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
