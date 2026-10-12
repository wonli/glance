// Package kimi adapts the Kimi Chat Completions and model catalog APIs.
package kimi

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

const (
	DefaultBaseURL    = "https://api.moonshot.cn/v1"
	listRequestName   = "provider.models"
	streamRequestName = "provider.chat"
)

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
	endpoint, err := modelsEndpoint(c.config.BaseURL)
	if err != nil {
		return nil, err
	}
	request := &modelsRequest{url: endpoint, apiKey: c.config.APIKey}
	id := &apic.ApiId{Name: listRequestName, Client: request}
	var models []provider.ModelInfo
	client := apic.NewApiClient().WithContext(ctx).WithHTTPClient(c.config.HTTPClient)
	err = client.CallFunc(id, &apic.Options{Debug: false}, func(_ *apic.Api, data []byte) error {
		if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
			return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
		}
		var response struct {
			Data []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("%w: decode model catalog: %v", provider.ErrInvalidResponse, err)
		}
		models = make([]provider.ModelInfo, 0, len(response.Data))
		for _, item := range response.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.DisplayName)
			}
			if name == "" {
				name = id
			}
			models = append(models, provider.ModelInfo{ID: id, Name: name})
		}
		return nil
	})
	if err != nil {
		return nil, sanitizeError(err, c.config.APIKey)
	}
	return models, nil
}

func (c *Client) Stream(ctx context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	if emit == nil {
		return fmt.Errorf("%w: model event handler is required", provider.ErrConfiguration)
	}
	endpoint, err := chatEndpoint(c.config.BaseURL)
	if err != nil {
		return err
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
		request.RequestID = fmt.Sprintf("chat-%d", c.nextID.Add(1))
	}
	streamCtx, cancel := context.WithCancel(ctx)
	if err := c.register(request.RequestID, cancel); err != nil {
		cancel()
		return err
	}
	defer func() { c.remove(request.RequestID); cancel() }()

	body := struct {
		Model         string               `json:"model"`
		Messages      []chatMessage        `json:"messages"`
		Tools         []provider.ModelTool `json:"tools,omitempty"`
		Temperature   *float64             `json:"temperature,omitempty"`
		Stream        bool                 `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}{Model: model, Messages: chatMessages(request.Messages), Tools: request.Tools, Temperature: request.Temperature, Stream: true}
	body.StreamOptions.IncludeUsage = true
	api := &streamRequest{url: endpoint, apiKey: c.config.APIKey, body: body, emit: emit, cancel: cancel}
	api.OnEvent(api.receive)
	id := &apic.ApiId{Name: streamRequestName, Client: api, Stream: true}
	err = apic.NewApiClient().WithContext(streamCtx).WithHTTPClient(c.config.HTTPClient).Call(id, &apic.Options{Debug: false})
	if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
		return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
	}
	if api.callbackErr != nil {
		return api.callbackErr
	}
	if err != nil {
		return sanitizeError(err, c.config.APIKey)
	}
	if !api.done {
		return fmt.Errorf("%w: model stream ended without a completion marker", provider.ErrInvalidResponse)
	}
	return nil
}

func (c *Client) Stop(_ context.Context, requestID string) error {
	c.mu.Lock()
	cancel := c.active[requestID]
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
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

type chatMessage struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content,omitempty"`
	Name       string                   `json:"name,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	ToolCalls  []provider.ModelToolCall `json:"tool_calls,omitempty"`
}

func chatMessages(messages []provider.ModelMessage) []chatMessage {
	result := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, chatMessage{
			Role: message.Role, Content: message.Content, Name: message.Name,
			ToolCallID: message.ToolCallID, ToolCalls: message.ToolCalls,
		})
	}
	return result
}

func (a *modelsRequest) Url() string                 { return a.url }
func (a *modelsRequest) HttpMethod() apic.HttpMethod { return http.MethodGet }
func (a *modelsRequest) Headers() apic.Params        { return authHeaders(a.apiKey) }
func (a *modelsRequest) Debug() bool                 { return false }

type streamRequest struct {
	apic.Apic
	url, apiKey string
	body        any
	emit        func(provider.ModelEvent) error
	cancel      context.CancelFunc
	callbackErr error
	done        bool
}

func (a *streamRequest) Url() string                 { return a.url }
func (a *streamRequest) HttpMethod() apic.HttpMethod { return http.MethodPost }
func (a *streamRequest) Headers() apic.Params {
	headers := authHeaders(a.apiKey)
	headers["Accept"] = "text/event-stream"
	return headers
}
func (a *streamRequest) PostBody() any { return a.body }
func (a *streamRequest) Debug() bool   { return false }

func (a *streamRequest) receive(line string) {
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
				Content   *string `json:"content"`
				Reasoning *string `json:"reasoning_content"`
				ToolCalls []struct {
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
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		a.callbackErr = fmt.Errorf("%w: decode model stream chunk: %v", provider.ErrInvalidResponse, err)
		a.cancel()
		return
	}
	if chunk.Usage != nil {
		a.callbackErr = a.emit(provider.ModelEvent{Usage: &provider.TokenUsage{PromptTokens: chunk.Usage.Prompt, CompletionTokens: chunk.Usage.Completion, TotalTokens: chunk.Usage.Total}})
	}
	for _, choice := range chunk.Choices {
		if a.callbackErr != nil {
			break
		}
		// Keep reasoning and answer separate, and deliver reasoning first when a
		// server includes both in the same SSE frame.
		if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
			a.callbackErr = a.emit(provider.ModelEvent{ReasoningDelta: *choice.Delta.Reasoning})
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

func authHeaders(key string) apic.Params {
	headers := apic.Params{"Content-Type": "application/json"}
	if strings.TrimSpace(key) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(key)
	}
	return headers
}

func modelsEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("%w: model provider base URL is required", provider.ErrConfiguration)
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	}
	return baseURL + "/models", nil
}

func chatEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("%w: model provider base URL is required", provider.ErrConfiguration)
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL, nil
	}
	return baseURL + "/chat/completions", nil
}

func sanitizeError(err error, apiKey string) error {
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
