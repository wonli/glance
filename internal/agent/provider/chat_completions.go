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

const maxSSELineBytes = 4 << 20

// ChatCompletionsProvider implements the common streaming chat-completions
// protocol used by MiniMax, Kimi, DeepSeek, and compatible services.
type ChatCompletionsProvider struct {
	config ChatCompletionsConfig
}

func NewChatCompletions(config ChatCompletionsConfig) *ChatCompletionsProvider {
	return &ChatCompletionsProvider{config: config}
}

func (p *ChatCompletionsProvider) Stream(ctx context.Context, request ModelRequest, emit func(ModelEvent) error) error {
	if emit == nil {
		return fmt.Errorf("%w: model event handler is required", ErrConfiguration)
	}
	endpoint, err := chatCompletionsEndpoint(p.config.BaseURL)
	if err != nil {
		return err
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

	body := struct {
		Model         string         `json:"model"`
		Messages      []ModelMessage `json:"messages"`
		Tools         []ModelTool    `json:"tools,omitempty"`
		Temperature   *float64       `json:"temperature,omitempty"`
		Stream        bool           `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}{
		Model: model, Messages: request.Messages, Tools: request.Tools,
		Temperature: request.Temperature, Stream: true,
	}
	body.StreamOptions.IncludeUsage = true
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode model request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("%w: create model request: %v", ErrConfiguration, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	client := p.config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send model request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	return readChatCompletionSSE(resp.Body, emit)
}

type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("model provider returned HTTP %d", e.StatusCode)
}

func chatCompletionsEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("%w: model provider base URL is required", ErrConfiguration)
	}
	if strings.HasSuffix(strings.TrimRight(baseURL, "/"), "/chat/completions") {
		return strings.TrimRight(baseURL, "/"), nil
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions", nil
}

func readChatCompletionSSE(reader io.Reader, emit func(ModelEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxSSELineBytes)
	dataLines := make([]string, 0, 1)
	done := false
	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if data == "[DONE]" {
			done = true
			return emit(ModelEvent{Done: true})
		}
		return emitChatCompletionChunk(data, emit)
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: read model stream: %v", ErrInvalidResponse, err)
	}
	if err := dispatch(); err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("%w: model stream ended without a completion marker", ErrInvalidResponse)
	}
	return nil
}

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content   *string `json:"content"`
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
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func emitChatCompletionChunk(data string, emit func(ModelEvent) error) error {
	var chunk chatCompletionChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return fmt.Errorf("%w: decode model stream chunk: %v", ErrInvalidResponse, err)
	}
	if chunk.Usage != nil {
		if err := emit(ModelEvent{Usage: &TokenUsage{
			PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens: chunk.Usage.TotalTokens,
		}}); err != nil {
			return err
		}
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			if err := emit(ModelEvent{TextDelta: *choice.Delta.Content}); err != nil {
				return err
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			delta := &ToolCallDelta{
				Index: toolCall.Index, ID: toolCall.ID, Type: toolCall.Type,
				Name: toolCall.Function.Name, ArgumentsDelta: toolCall.Function.Arguments,
			}
			if delta.ID != "" || delta.Name != "" || delta.ArgumentsDelta != "" {
				if err := emit(ModelEvent{ToolCallDelta: delta}); err != nil {
					return err
				}
			}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			if err := emit(ModelEvent{FinishReason: *choice.FinishReason}); err != nil {
				return err
			}
		}
	}
	return nil
}
