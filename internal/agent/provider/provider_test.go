package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatCompletionsProviderStreamsTextToolCallsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request method/path=%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q; want configured bearer key", got)
		}
		var request struct {
			Model         string         `json:"model"`
			Messages      []ModelMessage `json:"messages"`
			Tools         []ModelTool    `json:"tools"`
			Stream        bool           `json:"stream"`
			StreamOptions struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "model-test" || !request.Stream || !request.StreamOptions.IncludeUsage || len(request.Tools) != 1 || len(request.Messages) != 1 {
			t.Errorf("unexpected Chat Completions request: %+v", request)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"document.list\",\"arguments\":\"{\\\"limit\\\":\"}}]}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"10}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n",
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":3,\"total_tokens\":7}}\n\n",
			"data: [DONE]\n\n",
		)
	}))
	defer server.Close()

	provider := NewChatCompletions(ChatCompletionsConfig{BaseURL: server.URL + "/v1", APIKey: "test-key"})
	tool := ModelTool{Type: "function", Function: ModelToolFunction{Name: "document.list", Parameters: json.RawMessage(`{"type":"object"}`)}}
	var events []ModelEvent
	err := provider.Stream(context.Background(), ModelRequest{
		Model: "model-test", Messages: []ModelMessage{{Role: "user", Content: "hi"}}, Tools: []ModelTool{tool},
	}, func(event ModelEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var text strings.Builder
	var toolCall ToolCallDelta
	var usage *TokenUsage
	for _, event := range events {
		text.WriteString(event.TextDelta)
		if event.ToolCallDelta != nil {
			toolCall.ID = firstNonEmpty(toolCall.ID, event.ToolCallDelta.ID)
			toolCall.Name = firstNonEmpty(toolCall.Name, event.ToolCallDelta.Name)
			toolCall.ArgumentsDelta += event.ToolCallDelta.ArgumentsDelta
		}
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if text.String() != "Hello " || toolCall.ID != "call-1" || toolCall.Name != "document.list" || toolCall.ArgumentsDelta != `{"limit":10}` {
		t.Fatalf("streamed text/tool call = %q / %+v", text.String(), toolCall)
	}
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("streamed usage = %+v; want total 7", usage)
	}
}

func TestDefaultChatCompletionsBaseURLs(t *testing.T) {
	for providerID, want := range map[string]string{
		"minimax":  "https://api.minimaxi.com/v1",
		"kimi":     "https://api.moonshot.cn/v1",
		"deepseek": "https://api.deepseek.com",
	} {
		if got := DefaultChatCompletionsBaseURL(providerID); got != want {
			t.Errorf("DefaultChatCompletionsBaseURL(%q)=%q; want %q", providerID, got, want)
		}
	}
	if got := DefaultChatCompletionsBaseURL("custom"); got != "" {
		t.Fatalf("custom provider base URL = %q; want empty for caller configuration", got)
	}
}

func TestOllamaProviderStreamsTextToolCallsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("request method/path=%s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Model    string          `json:"model"`
			Messages []ollamaMessage `json:"messages"`
			Tools    []ModelTool     `json:"tools"`
			Stream   bool            `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "local-model" || !request.Stream || len(request.Tools) != 1 || len(request.Messages) != 1 {
			t.Errorf("unexpected Ollama request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"message":{"content":"本地回复"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"tool_calls":[{"function":{"name":"document.inspect","arguments":{"document_id":"doc-1"}}}]},"done":true,"prompt_eval_count":5,"eval_count":2}`)
	}))
	defer server.Close()

	provider := NewOllama(OllamaConfig{BaseURL: server.URL, Model: "local-model"})
	var events []ModelEvent
	err := provider.Stream(context.Background(), ModelRequest{
		Messages: []ModelMessage{{Role: "user", Content: "检查"}},
		Tools:    []ModelTool{{Type: "function", Function: ModelToolFunction{Name: "document.inspect", Parameters: json.RawMessage(`{}`)}}},
	}, func(event ModelEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(events) < 4 || events[0].TextDelta != "本地回复" {
		t.Fatalf("Ollama events = %+v", events)
	}
	var call *ToolCallDelta
	var usage *TokenUsage
	for _, event := range events {
		if event.ToolCallDelta != nil {
			call = event.ToolCallDelta
		}
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if call == nil || call.Name != "document.inspect" || !strings.Contains(call.ArgumentsDelta, "doc-1") {
		t.Fatalf("Ollama tool call = %+v", call)
	}
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("Ollama usage = %+v; want total 7", usage)
	}
}

func firstNonEmpty(left, right string) string {
	if left != "" {
		return left
	}
	return right
}
