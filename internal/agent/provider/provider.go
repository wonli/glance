// Package provider contains model protocol adapters used by the Agent Host.
package provider

import (
	"context"
	"encoding/json"
	"net/http"
)

// ModelProvider streams one model response. The Agent Host owns tool execution
// and starts another stream after it has appended tool results to the messages.
type ModelProvider interface {
	Stream(context.Context, ModelRequest, func(ModelEvent) error) error
}

type ModelRequest struct {
	Model       string         `json:"model,omitempty"`
	Messages    []ModelMessage `json:"messages"`
	Tools       []ModelTool    `json:"tools,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
}

type ModelMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolCalls  []ModelToolCall `json:"tool_calls,omitempty"`
}

type ModelTool struct {
	Type     string            `json:"type"`
	Function ModelToolFunction `json:"function"`
}

type ModelToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ModelToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type,omitempty"`
	Function ModelToolCallFunction `json:"function"`
}

type ModelToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCallDelta struct {
	Index          int
	ID             string
	Type           string
	Name           string
	ArgumentsDelta string
}

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ModelEvent struct {
	TextDelta     string
	ToolCallDelta *ToolCallDelta
	Usage         *TokenUsage
	FinishReason  string
	Done          bool
}

// ChatCompletionsConfig keeps credentials and network configuration outside
// ordinary settings and conversation state.
type ChatCompletionsConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// DefaultChatCompletionsBaseURL returns the endpoint prefix used by the
// migrated provider adapters. Unknown IDs can still use a custom BaseURL.
func DefaultChatCompletionsBaseURL(providerID string) string {
	switch providerID {
	case "minimax":
		return "https://api.minimaxi.com/v1"
	case "kimi":
		return "https://api.moonshot.cn/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	default:
		return ""
	}
}

// OllamaConfig describes a local Ollama server. APIKey is intentionally not
// part of this config because the local adapter does not send cloud credentials.
type OllamaConfig struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}
