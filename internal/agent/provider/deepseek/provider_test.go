package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
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

func TestStreamMapsMCPNamesAcrossToolDefinitionsHistoryAndFragmentedReplies(t *testing.T) {
	var wireName string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Tools    []provider.ModelTool `json:"tools"`
			Messages []chatMessage        `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		wireName = body.Tools[0].Function.Name
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(wireName) {
			t.Errorf("DeepSeek rejects dotted MCP tool name %q", wireName)
		}
		if wireName == body.Tools[1].Function.Name {
			t.Error("distinct MCP names share a wire alias")
		}
		if body.Messages[1].ToolCalls[0].Function.Name != wireName {
			t.Error("history does not use the same wire tool name")
		}
		stream := ""
		for i, fragment := range []string{wireName[:6], wireName[6:]} {
			id := ""
			if i == 0 {
				id = "call-2"
			}
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]string{"name": fragment, "arguments": ""}}}}}}})
			stream += "data: " + string(b) + "\n\n"
		}
		stream += "data: [DONE]\n\n"
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream)), Request: r}, nil
	})}
	request := provider.ModelRequest{Model: "deepseek-flash", Tools: []provider.ModelTool{
		{Type: "function", Function: provider.ModelToolFunction{Name: "document.inspect", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: provider.ModelToolFunction{Name: "document_inspect", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: provider.ModelToolFunction{Name: "glance", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}, Messages: []provider.ModelMessage{{Role: "user", Content: "检查"}, {Role: "assistant", ToolCalls: []provider.ModelToolCall{{ID: "call-1", Type: "function", Function: provider.ModelToolCallFunction{Name: "document.inspect", Arguments: "{}"}}}}, {Role: "tool", ToolCallID: "call-1", Content: "{}"}}}
	var decoded string
	err := New(Config{HTTPClient: client}).Stream(context.Background(), request, func(e provider.ModelEvent) error {
		if e.ToolCallDelta != nil {
			decoded += e.ToolCallDelta.Name
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded != "document.inspect" {
		t.Fatalf("MCP execution name=%q, want document.inspect", decoded)
	}
	if request.Tools[0].Function.Name != "document.inspect" || request.Messages[1].ToolCalls[0].Function.Name != "document.inspect" {
		t.Fatal("adapter changed neutral request or saved history")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
