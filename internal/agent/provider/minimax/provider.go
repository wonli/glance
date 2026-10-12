// Package minimax adapts MiniMax's native text Chat Completion v2 API and its
// OpenAI-compatible model catalog endpoint.
package minimax

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"glance/internal/agent/provider"

	"github.com/wonli/apic/v2"
)

const DefaultBaseURL = "https://api.minimaxi.com"

type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type Client struct {
	config Config
	mu     sync.Mutex
	active map[string]context.CancelFunc
	nextID atomic.Uint64
}

func New(config Config) *Client {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = DefaultBaseURL
	}
	return &Client{config: config, active: make(map[string]context.CancelFunc)}
}

func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := &modelsRequest{url: minimaxModelsURL(c.config.BaseURL), apiKey: c.config.APIKey}
	id := &apic.ApiId{Name: "minimax.models", Client: request}
	var models []provider.ModelInfo
	err := apic.NewApiClient().WithContext(ctx).WithHTTPClient(c.config.HTTPClient).CallFunc(id, &apic.Options{Debug: false}, func(_ *apic.Api, data []byte) error {
		if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
			return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
		}
		var catalog struct {
			Data []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &catalog); err != nil {
			return fmt.Errorf("%w: decode MiniMax model catalog: %v", provider.ErrInvalidResponse, err)
		}
		models = make([]provider.ModelInfo, 0, len(catalog.Data))
		for _, item := range catalog.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			name := strings.TrimSpace(item.DisplayName)
			if name == "" {
				name = strings.TrimSpace(item.Name)
			}
			if name == "" {
				name = id
			}
			models = append(models, provider.ModelInfo{ID: id, Name: name})
		}
		return nil
	})
	if err != nil {
		return nil, redact(err, c.config.APIKey)
	}
	return models, nil
}

func (c *Client) Stream(ctx context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	if emit == nil {
		return fmt.Errorf("%w: model event handler is required", provider.ErrConfiguration)
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = strings.TrimSpace(c.config.Model)
	}
	if model == "" {
		return fmt.Errorf("%w: model name is required", provider.ErrConfiguration)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("%w: at least one model message is required", provider.ErrInvalidRequest)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if request.RequestID == "" {
		request.RequestID = fmt.Sprintf("minimax-%d", c.nextID.Add(1))
	}
	streamCtx, cancel := context.WithCancel(ctx)
	if err := c.register(request.RequestID, cancel); err != nil {
		cancel()
		return err
	}
	defer func() { c.remove(request.RequestID); cancel() }()

	body := minimaxChatRequest{Model: model, Messages: minimaxMessages(request.Messages), Tools: request.Tools, Stream: true, Temperature: request.Temperature}
	body.StreamOptions.IncludeUsage = true
	handler := &chatRequest{url: minimaxChatURL(c.config.BaseURL), apiKey: c.config.APIKey, body: body, emit: emit, cancel: cancel}
	handler.OnEvent(handler.receive)
	id := &apic.ApiId{Name: "minimax.chat", Client: handler, Stream: true}
	err := apic.NewApiClient().WithContext(streamCtx).WithHTTPClient(c.config.HTTPClient).Call(id, &apic.Options{Debug: false})
	if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
		return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
	}
	if handler.callbackErr != nil {
		return handler.callbackErr
	}
	if err != nil {
		return redact(err, c.config.APIKey)
	}
	if !handler.done {
		return fmt.Errorf("%w: MiniMax stream ended without a completion marker", provider.ErrInvalidResponse)
	}
	return nil
}

func (c *Client) Stop(_ context.Context, requestID string) error {
	c.mu.Lock()
	cancel := c.active[requestID]
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (c *Client) register(id string, cancel context.CancelFunc) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.active[id]; exists {
		return fmt.Errorf("%w: duplicate model request ID", provider.ErrInvalidRequest)
	}
	c.active[id] = cancel
	return nil
}

func (c *Client) remove(id string) { c.mu.Lock(); delete(c.active, id); c.mu.Unlock() }

type modelsRequest struct {
	apic.Apic
	url, apiKey string
}

func (a *modelsRequest) Url() string                 { return a.url }
func (a *modelsRequest) HttpMethod() apic.HttpMethod { return http.MethodGet }
func (a *modelsRequest) Headers() apic.Params        { return minimaxHeaders(a.apiKey) }
func (a *modelsRequest) Debug() bool                 { return false }

type minimaxChatRequest struct {
	Model         string               `json:"model"`
	Messages      []minimaxMessage     `json:"messages"`
	Tools         []provider.ModelTool `json:"tools,omitempty"`
	Stream        bool                 `json:"stream"`
	Temperature   *float64             `json:"temperature,omitempty"`
	StreamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type minimaxMessage struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content,omitempty"`
	Name       string                   `json:"name,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	ToolCalls  []provider.ModelToolCall `json:"tool_calls,omitempty"`
}

func minimaxMessages(messages []provider.ModelMessage) []minimaxMessage {
	result := make([]minimaxMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, minimaxMessage{
			Role: message.Role, Content: message.Content, Name: message.Name,
			ToolCallID: message.ToolCallID, ToolCalls: message.ToolCalls,
		})
	}
	return result
}

type chatRequest struct {
	apic.Apic
	url, apiKey string
	body        any
	emit        func(provider.ModelEvent) error
	cancel      context.CancelFunc
	callbackErr error
	done        bool
}

func (a *chatRequest) Url() string                 { return a.url }
func (a *chatRequest) HttpMethod() apic.HttpMethod { return http.MethodPost }
func (a *chatRequest) Headers() apic.Params {
	headers := minimaxHeaders(a.apiKey)
	headers["Accept"] = "text/event-stream"
	return headers
}
func (a *chatRequest) PostBody() any { return a.body }
func (a *chatRequest) Debug() bool   { return false }

func (a *chatRequest) receive(line string) {
	if a.callbackErr != nil {
		return
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" {
		return
	}
	if data == "[DONE]" {
		a.done = true
		a.callbackErr = a.emit(provider.ModelEvent{Done: true})
		if a.callbackErr != nil {
			a.cancel()
		}
		return
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          *string `json:"content"`
				ReasoningContent *string `json:"reasoning_content"`
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
		BaseResp *struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		a.fail(fmt.Errorf("%w: decode MiniMax stream event: %v", provider.ErrInvalidResponse, err))
		return
	}
	if chunk.BaseResp != nil && chunk.BaseResp.StatusCode != 0 {
		a.fail(fmt.Errorf("MiniMax rejected the request (status %d)", chunk.BaseResp.StatusCode))
		return
	}
	if chunk.Usage != nil {
		prompt, completion := chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
		if prompt == 0 {
			prompt = chunk.Usage.InputTokens
		}
		if completion == 0 {
			completion = chunk.Usage.OutputTokens
		}
		total := chunk.Usage.TotalTokens
		if total == 0 {
			total = prompt + completion
		}
		a.callbackErr = a.emit(provider.ModelEvent{Usage: &provider.TokenUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}})
	}
	for _, choice := range chunk.Choices {
		if a.callbackErr != nil {
			break
		}
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			a.callbackErr = a.emit(provider.ModelEvent{ReasoningDelta: *choice.Delta.ReasoningContent})
		}
		if a.callbackErr == nil && choice.Delta.Content != nil && *choice.Delta.Content != "" {
			a.callbackErr = a.emit(provider.ModelEvent{TextDelta: *choice.Delta.Content})
		}
		for _, call := range choice.Delta.ToolCalls {
			if a.callbackErr != nil {
				break
			}
			delta := &provider.ToolCallDelta{Index: call.Index, ID: call.ID, Type: call.Type, Name: call.Function.Name, ArgumentsDelta: call.Function.Arguments}
			if delta.ID != "" || delta.Name != "" || delta.ArgumentsDelta != "" {
				a.callbackErr = a.emit(provider.ModelEvent{ToolCallDelta: delta})
			}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" && a.callbackErr == nil {
			a.callbackErr = a.emit(provider.ModelEvent{FinishReason: *choice.FinishReason})
		}
	}
	if a.callbackErr != nil {
		a.cancel()
	}
}

func (a *chatRequest) fail(err error) { a.callbackErr = err; a.cancel() }

func minimaxHeaders(apiKey string) apic.Params {
	headers := apic.Params{"Content-Type": "application/json"}
	if key := strings.TrimSpace(apiKey); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	return headers
}

func minimaxModelsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/text/chatcompletion_v2") {
		base = strings.TrimSuffix(base, "/text/chatcompletion_v2")
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/models"
	}
	return base + "/v1/models"
}

func minimaxChatURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/text/chatcompletion_v2") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/text/chatcompletion_v2"
	}
	return base + "/v1/text/chatcompletion_v2"
}

func redact(err error, apiKey string) error {
	if err == nil || apiKey == "" {
		return err
	}
	message := strings.ReplaceAll(err.Error(), apiKey, "[redacted]")
	if message == err.Error() {
		return err
	}
	return redactedError{message: message, cause: err}
}

type redactedError struct {
	message string
	cause   error
}

func (e redactedError) Error() string { return e.message }
func (e redactedError) Unwrap() error { return e.cause }
