package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"glance/internal/agent/provider"
	"glance/internal/agent/provider/deepseek"
	"glance/internal/agent/provider/kimi"
	"glance/internal/agent/provider/minimax"
	"glance/internal/agent/provider/openai"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
func response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

type providerConfig func(*http.Client) provider.ModelProvider

type providerRequestBody struct {
	RequestID json.RawMessage       `json:"request_id"`
	Thinking  providerThinkingBody  `json:"thinking"`
	Messages  []providerMessageBody `json:"messages"`
}

type providerThinkingBody struct {
	Type string `json:"type"`
}

type providerMessageBody struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent json.RawMessage `json:"reasoning_content"`
	ToolCallID       string          `json:"tool_call_id"`
	ToolName         json.RawMessage `json:"tool_name"`
}

func chatProviderConfigs(baseURL string, apiKey string) []struct {
	name, chatPath string
	make           providerConfig
	reasoningField string
	thinkingMode   string
} {
	return []struct {
		name, chatPath string
		make           providerConfig
		reasoningField string
		thinkingMode   string
	}{
		{"minimax", "/v1/text/chatcompletion_v2", func(c *http.Client) provider.ModelProvider {
			return minimax.New(minimax.Config{BaseURL: baseURL, APIKey: apiKey, Model: "test-model", HTTPClient: c})
		}, "reasoning_content", ""},
		{"kimi", "/v1/chat/completions", func(c *http.Client) provider.ModelProvider {
			return kimi.New(kimi.Config{BaseURL: baseURL, APIKey: apiKey, Model: "test-model", HTTPClient: c})
		}, "reasoning_content", ""},
		{"deepseek", "/v1/chat/completions", func(c *http.Client) provider.ModelProvider {
			return deepseek.New(deepseek.Config{BaseURL: baseURL, APIKey: apiKey, Model: "test-model", HTTPClient: c})
		}, "reasoning_content", "enabled"},
		{"openai", "/v1/chat/completions", func(c *http.Client) provider.ModelProvider {
			return openai.New(openai.Config{BaseURL: baseURL, APIKey: apiKey, Model: "test-model", HTTPClient: c})
		}, "", ""},
	}
}

func TestChatProviderModelListAndStreamContracts(t *testing.T) {
	for _, test := range chatProviderConfigs("https://provider.test/v1", "test-key") {
		t.Run(test.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1/models" {
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-key" {
						t.Errorf("model request=%s headers=%v", r.Method, r.Header)
					}
					return response(r, http.StatusOK, `{"data":[{"id":"model-a","name":"Model A"},{"id":"model-b"}]}`), nil
				}
				if r.URL.Path != test.chatPath || r.Method != http.MethodPost {
					t.Errorf("chat request=%s %s", r.Method, r.URL.Path)
				}
				var body providerRequestBody
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode body: %v", err)
				}
				if len(body.RequestID) > 0 {
					t.Error("local RequestID leaked into provider request")
				}
				if test.thinkingMode != "" {
					if body.Thinking.Type != test.thinkingMode {
						t.Errorf("thinking request mode=%v, want %q", body.Thinking.Type, test.thinkingMode)
					}
				}
				messages := body.Messages
				if len(messages) != 3 {
					t.Errorf("serialized messages=%v, want user, assistant, and tool result", messages)
				} else if test.name == "deepseek" {
					var reasoning string
					if err := json.Unmarshal(messages[1].ReasoningContent, &reasoning); err != nil || reasoning != "provider-only thought" {
						t.Errorf("DeepSeek assistant reasoning=%v (error %v)", reasoning, err)
					}
				} else if len(messages[1].ReasoningContent) > 0 {
					t.Errorf("%s request leaked another provider's reasoning field", test.name)
				}
				if len(messages) == 3 {
					toolResult := messages[2]
					if toolResult.Role != "tool" || toolResult.ToolCallID != "tool-1" || toolResult.Content != "tool result" {
						t.Errorf("serialized tool result=%+v", toolResult)
					} else if len(toolResult.ToolName) > 0 {
						t.Errorf("%s request leaked internal tool_name into Chat Completions payload: %+v", test.name, toolResult)
					}
				}
				textChunk := `"content":"hello"`
				reasoningChunk := ""
				if test.reasoningField != "" {
					reasoningChunk = `"reasoning_content":"think",`
				}
				toolChunk := `,"tool_calls":[{"index":0,"id":"tool-1","type":"function","function":{"name":"echo","arguments":"{\"x\":1}"}}]`
				stream := "data: {\"choices\":[{\"delta\":{" + reasoningChunk + textChunk + toolChunk + "}}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\n" + "data: [DONE]\n\n"
				return response(r, http.StatusOK, stream), nil
			})}
			modelProvider := test.make(httpClient)
			models, err := modelProvider.ListModels(context.Background())
			if err != nil {
				t.Fatalf("ListModels: %v", err)
			}
			if len(models) != 2 || models[0] != (provider.ModelInfo{ID: "model-a", Name: "Model A"}) || models[1] != (provider.ModelInfo{ID: "model-b", Name: "model-b"}) {
				t.Fatalf("models=%+v", models)
			}
			var events []provider.ModelEvent
			err = modelProvider.Stream(context.Background(), provider.ModelRequest{
				RequestID: "request-1", Model: "test-model",
				Messages: []provider.ModelMessage{
					{Role: "user", Content: "hi"},
					{Role: "assistant", Content: "partial"},
					{Role: "tool", ToolCallID: "tool-1", ToolName: "echo", Content: "tool result"},
				},
				ReasoningByMessage: map[int]string{1: "provider-only thought"},
			}, func(event provider.ModelEvent) error { events = append(events, event); return nil })
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			if test.reasoningField != "" {
				reasoningIndex, textIndex := -1, -1
				for index, event := range events {
					if event.ReasoningDelta != "" {
						reasoningIndex = index
					}
					if event.TextDelta != "" {
						textIndex = index
					}
				}
				if reasoningIndex < 0 || textIndex < 0 || reasoningIndex >= textIndex {
					t.Fatalf("reasoning/text events=%+v", events)
				}
			}
			if test.reasoningField == "" {
				textSeen := false
				for _, event := range events {
					if event.TextDelta == "hello" && event.ReasoningDelta == "" {
						textSeen = true
					}
				}
				if !textSeen {
					t.Fatalf("text-only events=%+v", events)
				}
			}
			if !events[len(events)-1].Done {
				t.Fatalf("missing completion marker: %+v", events)
			}
			var toolSeen, usageSeen bool
			for _, event := range events {
				if event.ToolCallDelta != nil {
					toolSeen = event.ToolCallDelta.ID == "tool-1" && event.ToolCallDelta.Name == "echo" && event.ToolCallDelta.ArgumentsDelta == `{"x":1}`
				}
				if event.Usage != nil {
					usageSeen = event.Usage.PromptTokens == 3 && event.Usage.CompletionTokens == 4 && event.Usage.TotalTokens == 7
				}
			}
			if !toolSeen || !usageSeen {
				t.Fatalf("normalized tool/usage events missing: %+v", events)
			}
		})
	}
}

func TestChatProviderListModelsReturnsStatusOnly(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(r, http.StatusUnauthorized, `{"error":"credential detail must not escape"}`), nil
	})}
	for _, test := range chatProviderConfigs("https://provider.test/v1", "test-key") {
		t.Run(test.name, func(t *testing.T) {
			_, modelsErr := test.make(client).ListModels(context.Background())
			var status *provider.HTTPStatusError
			if !errors.As(modelsErr, &status) || status.StatusCode != http.StatusUnauthorized {
				t.Fatalf("ListModels error=%v", modelsErr)
			}
			if strings.Contains(modelsErr.Error(), "credential detail") {
				t.Fatal("API error body was exposed")
			}
		})
	}
}

func TestChatProviderStopCancelsOnlyNamedStream(t *testing.T) {
	for _, test := range chatProviderConfigs("https://provider.test/v1", "") {
		t.Run(test.name, func(t *testing.T) {
			started, canceled := make(chan string, 2), make(chan string, 2)
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					return nil, err
				}
				started <- body.Model
				<-r.Context().Done()
				canceled <- body.Model
				return nil, r.Context().Err()
			})}
			modelProvider := test.make(httpClient)
			results := make(chan error, 2)
			for _, model := range []string{"one", "two"} {
				model := model
				go func() {
					results <- modelProvider.Stream(context.Background(), provider.ModelRequest{RequestID: model, Model: model, Messages: []provider.ModelMessage{{Role: "user", Content: "x"}}}, func(provider.ModelEvent) error { return nil })
				}()
			}
			for range 2 {
				<-started
			}
			if err := modelProvider.Stop(context.Background(), "one"); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			select {
			case got := <-canceled:
				if got != "one" {
					t.Fatalf("canceled stream=%q", got)
				}
			case <-time.After(time.Second):
				t.Fatal("Stop did not cancel requested stream")
			}
			select {
			case got := <-canceled:
				t.Fatalf("Stop canceled unrelated stream %q", got)
			case <-time.After(30 * time.Millisecond):
			}
			if err := modelProvider.Stop(context.Background(), "two"); err != nil {
				t.Fatalf("Stop second: %v", err)
			}
			for range 2 {
				select {
				case <-results:
				case <-time.After(time.Second):
					t.Fatal("canceled streams did not return")
				}
			}
		})
	}
}
