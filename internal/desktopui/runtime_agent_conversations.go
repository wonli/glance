package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/workspace"
)

type workspaceAgentConversationStore struct {
	runtime *desktopRuntime
}

func (r *desktopRuntime) createAgentConversation(ctx context.Context, documentID string) (agent.ConversationRecord, error) {
	if strings.TrimSpace(documentID) == "" {
		return agent.ConversationRecord{}, agent.ErrConversationDocumentRequired
	}
	id, err := agent.NewConversationID()
	if err != nil {
		return agent.ConversationRecord{}, err
	}
	now := time.Now().UTC()
	record := agent.ConversationRecord{ID: id, DocumentID: documentID, Messages: []provider.ModelMessage{}, Reasoning: []agent.ReasoningBlock{}, CreatedAt: now, UpdatedAt: now}
	if err := (workspaceAgentConversationStore{runtime: r}).Save(ctx, record); err != nil {
		return agent.ConversationRecord{}, err
	}
	return record, nil
}

func (s workspaceAgentConversationStore) Save(ctx context.Context, record agent.ConversationRecord) error {
	if err := conversationStoreContextError(ctx); err != nil {
		return err
	}
	messages, err := json.Marshal(record.Messages)
	if err != nil {
		return fmt.Errorf("encode Agent conversation messages: %w", err)
	}
	reasoning, err := json.Marshal(record.Reasoning)
	if err != nil {
		return fmt.Errorf("encode Agent conversation reasoning: %w", err)
	}
	store, unlock := s.lockWorkspaceStore()
	defer unlock()
	if store == nil {
		return errors.New("workspace conversation storage is unavailable")
	}
	return store.SaveAgentConversation(workspace.AgentConversation{
		ID: string(record.ID), DocumentID: record.DocumentID, Title: record.Title,
		Messages: messages, Reasoning: reasoning, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	})
}

func (s workspaceAgentConversationStore) Load(ctx context.Context, documentID string, id agent.ConversationID) (agent.ConversationRecord, bool, error) {
	if err := conversationStoreContextError(ctx); err != nil {
		return agent.ConversationRecord{}, false, err
	}
	store, unlock := s.lockWorkspaceStore()
	defer unlock()
	if store == nil {
		return agent.ConversationRecord{}, false, errors.New("workspace conversation storage is unavailable")
	}
	conversation, found, err := store.LoadAgentConversation(documentID, string(id))
	if err != nil || !found {
		return agent.ConversationRecord{}, found, err
	}
	var messages = make([]provider.ModelMessage, 0)
	if len(conversation.Messages) != 0 {
		if err := json.Unmarshal(conversation.Messages, &messages); err != nil {
			return agent.ConversationRecord{}, false, fmt.Errorf("decode Agent conversation messages: %w", err)
		}
	}
	var reasoning = make([]agent.ReasoningBlock, 0)
	if len(conversation.Reasoning) != 0 {
		if err := json.Unmarshal(conversation.Reasoning, &reasoning); err != nil {
			return agent.ConversationRecord{}, false, fmt.Errorf("decode Agent conversation reasoning: %w", err)
		}
	}
	return agent.ConversationRecord{
		ID: id, DocumentID: conversation.DocumentID, Title: conversation.Title,
		Messages: messages, Reasoning: reasoning, CreatedAt: conversation.CreatedAt, UpdatedAt: conversation.UpdatedAt,
	}, true, nil
}

func (s workspaceAgentConversationStore) List(ctx context.Context, documentID string) ([]agent.ConversationSummary, error) {
	if err := conversationStoreContextError(ctx); err != nil {
		return nil, err
	}
	store, unlock := s.lockWorkspaceStore()
	defer unlock()
	if store == nil {
		return nil, errors.New("workspace conversation storage is unavailable")
	}
	items, err := store.ListAgentConversations(documentID)
	if err != nil {
		return nil, err
	}
	result := make([]agent.ConversationSummary, len(items))
	for index, item := range items {
		result[index] = agent.ConversationSummary{ID: agent.ConversationID(item.ID), Title: item.Title, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
	}
	return result, nil
}

func (s workspaceAgentConversationStore) lockWorkspaceStore() (*workspace.Store, func()) {
	if s.runtime == nil {
		return nil, func() {}
	}
	s.runtime.workspaceDBMu.Lock()
	s.runtime.mu.Lock()
	store := s.runtime.workspaceStore
	s.runtime.mu.Unlock()
	return store, s.runtime.workspaceDBMu.Unlock
}

func conversationStoreContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
