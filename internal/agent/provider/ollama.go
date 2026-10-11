package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaProvider adapts Ollama's native newline-delimited chat stream.
type OllamaProvider struct {
	config OllamaConfig
}

func NewOllama(config OllamaConfig) *OllamaProvider {
	return &OllamaProvider{config: config}
}

func (p *OllamaProvider) Stream(ctx context.Context, request ModelRequest, emit func(ModelEvent) error) error {
	if emit == nil {
		return fmt.Errorf("%w: model event handler is required", ErrConfiguration)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(p.config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = strings.TrimSpace(p.config.Model)
	}
	if model == "" {
		return fmt.Errorf("%w: model name is required", ErrConfiguration)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("%w: at least one model message is required", ErrInvalidRequest)
	}

	messages, err := toOllamaMessages(request.Messages)
	if err != nil {
		return err
	}
	body := struct {
		Model    string          `json:"model"`
		Messages []ollamaMessage `json:"messages"`
		Tools    []ModelTool     `json:"tools,omitempty"`
		Stream   bool            `json:"stream"`
		Think    bool            `json:"think"`
		Options  struct {
			Temperature *float64 `json:"temperature,omitempty"`
		} `json:"options,omitempty"`
	}{Model: model, Messages: messages, Tools: request.Tools, Stream: true}
	body.Options.Temperature = request.Temperature
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode Ollama request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	client := p.config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send Ollama request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return &HTTPStatusError{StatusCode: resp.StatusCode}
	}
	return readOllamaStream(resp.Body, emit)
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

func toOllamaMessages(messages []ModelMessage) ([]ollamaMessage, error) {
	converted := make([]ollamaMessage, 0, len(messages))
	for messageIndex, message := range messages {
		ollama := ollamaMessage{Role: message.Role, Content: message.Content}
		if message.Role == "tool" {
			ollama.ToolName = message.ToolName
		}
		for callIndex, call := range message.ToolCalls {
			arguments := json.RawMessage(call.Function.Arguments)
			if len(arguments) == 0 {
				arguments = json.RawMessage("{}")
			}
			if !json.Valid(arguments) {
				return nil, fmt.Errorf("%w: Ollama tool call arguments are invalid JSON at message %d, call %d", ErrInvalidRequest, messageIndex, callIndex)
			}
			ollamaCall := ollamaToolCall{}
			ollamaCall.Function.Name = call.Function.Name
			ollamaCall.Function.Arguments = arguments
			ollama.ToolCalls = append(ollama.ToolCalls, ollamaCall)
		}
		converted = append(converted, ollama)
	}
	return converted, nil
}

type ollamaChunk struct {
	Message struct {
		Content   string `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done            bool `json:"done"`
	PromptEvalCount int  `json:"prompt_eval_count"`
	EvalCount       int  `json:"eval_count"`
}

func readOllamaStream(reader io.Reader, emit func(ModelEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxSSELineBytes)
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk ollamaChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return fmt.Errorf("%w: decode Ollama stream chunk: %v", ErrInvalidResponse, err)
		}
		if chunk.Message.Content != "" {
			if err := emit(ModelEvent{TextDelta: chunk.Message.Content}); err != nil {
				return err
			}
		}
		if chunk.Done {
			for index, call := range chunk.Message.ToolCalls {
				arguments := call.Function.Arguments
				if len(arguments) == 0 {
					arguments = json.RawMessage("{}")
				}
				delta := &ToolCallDelta{
					Index: index, ID: fmt.Sprintf("ollama-%d", index), Type: "function",
					Name: call.Function.Name, ArgumentsDelta: string(arguments),
				}
				if err := emit(ModelEvent{ToolCallDelta: delta}); err != nil {
					return err
				}
			}
			if chunk.PromptEvalCount != 0 || chunk.EvalCount != 0 {
				if err := emit(ModelEvent{Usage: &TokenUsage{
					PromptTokens: chunk.PromptEvalCount, CompletionTokens: chunk.EvalCount,
					TotalTokens: chunk.PromptEvalCount + chunk.EvalCount,
				}}); err != nil {
					return err
				}
			}
			if err := emit(ModelEvent{Done: true}); err != nil {
				return err
			}
			done = true
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: read Ollama stream: %v", ErrInvalidResponse, err)
	}
	if !done {
		return fmt.Errorf("%w: Ollama stream ended before completion", ErrInvalidResponse)
	}
	return nil
}
