// Package provider defines provider-neutral Agent model contracts and DTOs.
package provider

import (
	"context"
	"encoding/json"
)

// ModelProvider implements model discovery, streaming, and request-scoped stop.
type ModelProvider interface {
	ListModels(context.Context) ([]ModelInfo, error)
	Stream(context.Context, ModelRequest, func(ModelEvent) error) error
	Stop(context.Context, string) error
}

// ModelInfo is the provider-neutral model catalog item used by settings UI.
type ModelInfo struct {
	ID   string
	Name string
}

type ModelRequest struct {
	// RequestID identifies one local stream so Stop can cancel only that request.
	// Providers must never serialize it to an inference endpoint.
	RequestID string `json:"-"`
	// ReasoningByMessage carries separately persisted thinking text keyed by
	// Messages index. Only providers whose protocol requires it should map it
	// into their wire format; it is never part of a normal ModelMessage.
	ReasoningByMessage map[int]string `json:"-"`
	Model              string         `json:"model,omitempty"`
	Messages           []ModelMessage `json:"messages"`
	Tools              []ModelTool    `json:"tools,omitempty"`
	Temperature        *float64       `json:"temperature,omitempty"`
}

type ModelMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	Name       string `json:"name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	// ToolName is retained in persisted context and used by protocols such as
	// Ollama. Chat Completions adapters map messages to their own wire DTOs and
	// omit it when the protocol does not define this field.
	ToolName  string          `json:"tool_name,omitempty"`
	ToolCalls []ModelToolCall `json:"tool_calls,omitempty"`
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
	ReasoningDelta string
	TextDelta      string
	ToolCallDelta  *ToolCallDelta
	Usage          *TokenUsage
	FinishReason   string
	Done           bool
}
