package ollama

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

const DefaultBaseURL = "http://127.0.0.1:11434"

type Config struct {
	BaseURL, Model string
	HTTPClient     *http.Client
}
type Provider struct {
	config Config
	mu     sync.Mutex
	active map[string]context.CancelFunc
	nextID atomic.Uint64
}

func New(config Config) *Provider {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = DefaultBaseURL
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	return &Provider{config: config, active: make(map[string]context.CancelFunc)}
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := &apiRequest{url: p.config.BaseURL + "/api/tags"}
	id := &apic.ApiId{Name: "ollama.models", Client: request}
	var result []provider.ModelInfo
	err := apic.NewApiClient().WithContext(ctx).WithHTTPClient(p.config.HTTPClient).CallFunc(id, &apic.Options{Debug: false}, func(_ *apic.Api, data []byte) error {
		if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
			return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
		}
		var response struct {
			Models []struct {
				Name  string `json:"name"`
				Model string `json:"model"`
			} `json:"models"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("%w: decode Ollama model catalog: %v", provider.ErrInvalidResponse, err)
		}
		result = make([]provider.ModelInfo, 0, len(response.Models))
		for _, model := range response.Models {
			id := strings.TrimSpace(model.Name)
			if id == "" {
				id = strings.TrimSpace(model.Model)
			}
			if id == "" {
				continue
			}
			result = append(result, provider.ModelInfo{ID: id, Name: id})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) Stream(ctx context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	if emit == nil {
		return fmt.Errorf("%w: model event handler is required", provider.ErrConfiguration)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = strings.TrimSpace(p.config.Model)
	}
	if model == "" {
		return fmt.Errorf("%w: model name is required", provider.ErrConfiguration)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("%w: at least one model message is required", provider.ErrInvalidRequest)
	}
	messages, err := toMessages(request.Messages)
	if err != nil {
		return err
	}
	if request.RequestID == "" {
		request.RequestID = fmt.Sprintf("ollama-%d", p.nextID.Add(1))
	}
	streamCtx, cancel := context.WithCancel(ctx)
	if err := p.register(request.RequestID, cancel); err != nil {
		cancel()
		return err
	}
	defer func() { p.remove(request.RequestID); cancel() }()
	body := chatRequest{Model: model, Messages: messages, Tools: request.Tools, Stream: true, Think: true}
	body.Options.Temperature = request.Temperature
	handler := &streamRequest{url: p.config.BaseURL + "/api/chat", body: body, emit: emit, cancel: cancel}
	handler.OnEvent(handler.receive)
	id := &apic.ApiId{Name: "ollama.chat", Client: handler, Stream: true}
	err = apic.NewApiClient().WithContext(streamCtx).WithHTTPClient(p.config.HTTPClient).Call(id, &apic.Options{Debug: false, Timeout: 0})
	if id.Response != nil && id.Response.HttpStatus != http.StatusOK {
		return &provider.HTTPStatusError{StatusCode: id.Response.HttpStatus}
	}
	if handler.callbackErr != nil {
		return handler.callbackErr
	}
	if err != nil {
		return err
	}
	if !handler.done {
		return fmt.Errorf("%w: Ollama stream ended before completion", provider.ErrInvalidResponse)
	}
	return nil
}

func (p *Provider) Stop(_ context.Context, requestID string) error {
	p.mu.Lock()
	cancel := p.active[requestID]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (p *Provider) register(id string, cancel context.CancelFunc) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.active[id]; exists {
		return fmt.Errorf("%w: duplicate model request ID", provider.ErrInvalidRequest)
	}
	p.active[id] = cancel
	return nil
}
func (p *Provider) remove(id string) { p.mu.Lock(); delete(p.active, id); p.mu.Unlock() }

type apiRequest struct {
	apic.Apic
	url string
}

func (a *apiRequest) Url() string                 { return a.url }
func (a *apiRequest) HttpMethod() apic.HttpMethod { return http.MethodGet }
func (a *apiRequest) Debug() bool                 { return false }

type chatRequest struct {
	Model    string               `json:"model"`
	Messages []ollamaMessage      `json:"messages"`
	Tools    []provider.ModelTool `json:"tools,omitempty"`
	Stream   bool                 `json:"stream"`
	Think    bool                 `json:"think"`
	Options  struct {
		Temperature *float64 `json:"temperature,omitempty"`
	} `json:"options,omitempty"`
}
type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}
type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func toMessages(messages []provider.ModelMessage) ([]ollamaMessage, error) {
	converted := make([]ollamaMessage, 0, len(messages))
	for mi, message := range messages {
		out := ollamaMessage{Role: message.Role, Content: message.Content}
		if message.Role == "tool" {
			out.ToolName = message.ToolName
		}
		for ci, call := range message.ToolCalls {
			args := json.RawMessage(call.Function.Arguments)
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			if !json.Valid(args) {
				return nil, fmt.Errorf("%w: Ollama tool arguments are invalid JSON at message %d call %d", provider.ErrInvalidRequest, mi, ci)
			}
			item := ollamaToolCall{}
			item.Function.Name = call.Function.Name
			item.Function.Arguments = args
			out.ToolCalls = append(out.ToolCalls, item)
		}
		converted = append(converted, out)
	}
	return converted, nil
}

type streamRequest struct {
	apic.Apic
	url         string
	body        any
	emit        func(provider.ModelEvent) error
	cancel      context.CancelFunc
	callbackErr error
	done        bool
}

func (a *streamRequest) Url() string                 { return a.url }
func (a *streamRequest) HttpMethod() apic.HttpMethod { return http.MethodPost }
func (a *streamRequest) Headers() apic.Params {
	return apic.Params{"Content-Type": "application/json", "Accept": "application/x-ndjson"}
}
func (a *streamRequest) PostBody() any { return a.body }
func (a *streamRequest) Debug() bool   { return false }
func (a *streamRequest) receive(line string) {
	if a.callbackErr != nil {
		return
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	var chunk struct {
		Message struct {
			Content   string `json:"content"`
			Thinking  string `json:"thinking"`
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		Error  string `json:"error"`
		Done   bool   `json:"done"`
		Prompt int    `json:"prompt_eval_count"`
		Eval   int    `json:"eval_count"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &chunk); err != nil {
		a.fail(fmt.Errorf("%w: decode Ollama stream chunk: %v", provider.ErrInvalidResponse, err))
		return
	}
	if strings.TrimSpace(chunk.Error) != "" {
		a.fail(fmt.Errorf("Ollama returned an API error"))
		return
	}
	if chunk.Message.Thinking != "" {
		a.callbackErr = a.emit(provider.ModelEvent{ReasoningDelta: chunk.Message.Thinking})
	}
	if a.callbackErr == nil && chunk.Message.Content != "" {
		a.callbackErr = a.emit(provider.ModelEvent{TextDelta: chunk.Message.Content})
	}
	if chunk.Done && a.callbackErr == nil {
		for index, call := range chunk.Message.ToolCalls {
			args := call.Function.Arguments
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			delta := &provider.ToolCallDelta{Index: index, ID: fmt.Sprintf("ollama-%d", index), Type: "function", Name: call.Function.Name, ArgumentsDelta: string(args)}
			a.callbackErr = a.emit(provider.ModelEvent{ToolCallDelta: delta})
			if a.callbackErr != nil {
				break
			}
		}
		if a.callbackErr == nil && (chunk.Prompt != 0 || chunk.Eval != 0) {
			a.callbackErr = a.emit(provider.ModelEvent{Usage: &provider.TokenUsage{PromptTokens: chunk.Prompt, CompletionTokens: chunk.Eval, TotalTokens: chunk.Prompt + chunk.Eval}})
		}
		if a.callbackErr == nil {
			a.done = true
			a.callbackErr = a.emit(provider.ModelEvent{Done: true})
		}
	}
	if a.callbackErr != nil {
		a.cancel()
	}
}
func (a *streamRequest) fail(err error) { a.callbackErr = err; a.cancel() }
