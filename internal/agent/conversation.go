package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"glance/internal/agent/provider"
)

type ConversationID string

func NewConversationID() (ConversationID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate conversation ID: %w", err)
	}
	return ConversationID("conversation-" + hex.EncodeToString(value[:])), nil
}

func ConversationInterrupted(messages []provider.ModelMessage) bool {
	if len(messages) == 0 {
		return false
	}
	hasUserMessage := false
	for _, message := range messages {
		if message.Role == "user" {
			hasUserMessage = true
		}
	}
	if !hasUserMessage {
		return false
	}
	last := messages[len(messages)-1]
	return last.Role != "assistant" || len(last.ToolCalls) > 0 || strings.TrimSpace(last.Content) == ""
}

type EventType string

const (
	EventAssistantDelta    EventType = "assistant.delta"
	EventReasoningDelta    EventType = "assistant.reasoning_delta"
	EventConversationTitle EventType = "conversation.title"
	EventToolStarted       EventType = "tool.started"
	EventToolCompleted     EventType = "tool.completed"
	EventTurnCompleted     EventType = "turn.completed"
	EventTurnFailed        EventType = "turn.failed"
)

type ReasoningBlock struct {
	// BeforeMessage is the assistant message index this transcript belongs to.
	// A negative value marks reasoning from an incomplete attempt with no
	// assistant message; it remains visible but is never sent as model context.
	BeforeMessage int
	Content       string
}

type ConversationRecord struct {
	ID         ConversationID
	DocumentID string
	Title      string
	Messages   []provider.ModelMessage
	Reasoning  []ReasoningBlock
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ConversationSummary struct {
	ID        ConversationID
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ConversationStore interface {
	Save(context.Context, ConversationRecord) error
	Load(context.Context, string, ConversationID) (ConversationRecord, bool, error)
	List(context.Context, string) ([]ConversationSummary, error)
}

// AgentEvent is safe to forward to a UI event queue. It contains no provider
// credentials and never carries Gio widgets or layout state.
type AgentEvent struct {
	ConversationID ConversationID  `json:"conversation_id"`
	Type           EventType       `json:"type"`
	Text           string          `json:"text,omitempty"`
	Title          string          `json:"title,omitempty"`
	Reasoning      string          `json:"reasoning,omitempty"`
	ToolName       string          `json:"tool_name,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	IsError        bool            `json:"is_error,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Error          string          `json:"error,omitempty"`
}
