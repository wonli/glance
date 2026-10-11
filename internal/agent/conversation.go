package agent

import "encoding/json"

type ConversationID string

type EventType string

const (
	EventAssistantDelta EventType = "assistant.delta"
	EventToolStarted    EventType = "tool.started"
	EventToolCompleted  EventType = "tool.completed"
	EventTurnCompleted  EventType = "turn.completed"
	EventTurnFailed     EventType = "turn.failed"
)

// AgentEvent is safe to forward to a UI event queue. It contains no provider
// credentials and never carries Gio widgets or layout state.
type AgentEvent struct {
	ConversationID ConversationID  `json:"conversation_id"`
	Type           EventType       `json:"type"`
	Text           string          `json:"text,omitempty"`
	ToolName       string          `json:"tool_name,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	IsError        bool            `json:"is_error,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Error          string          `json:"error,omitempty"`
}
