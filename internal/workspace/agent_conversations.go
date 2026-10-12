package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AgentConversation stores provider-neutral chat context for one document.
// Messages contains only messages sent to or returned by the model; reasoning
// is stored separately for transcript display and is never prompt context.
type AgentConversation struct {
	ID         string
	DocumentID string
	Title      string
	Messages   []byte
	Reasoning  []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AgentConversationSummary struct {
	ID        string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) SaveAgentConversation(conversation AgentConversation) error {
	if s == nil || s.db == nil {
		return errors.New("workspace store is not open")
	}
	if conversation.ID == "" || conversation.DocumentID == "" {
		return errors.New("conversation ID and document ID are required")
	}
	now := time.Now().UTC()
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = now
	}
	if conversation.UpdatedAt.IsZero() {
		conversation.UpdatedAt = now
	}
	if conversation.Messages == nil {
		conversation.Messages = []byte("[]")
	}
	if conversation.Reasoning == nil {
		conversation.Reasoning = []byte("[]")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingDocumentID string
	var existingCreatedAt int64
	err = tx.QueryRow(`SELECT document_id, created_at FROM agent_conversations WHERE id=?`, conversation.ID).Scan(&existingDocumentID, &existingCreatedAt)
	switch {
	case err == nil:
		if existingDocumentID != conversation.DocumentID {
			return errors.New("conversation belongs to a different document")
		}
		conversation.CreatedAt = time.Unix(0, existingCreatedAt).UTC()
	case errors.Is(err, sql.ErrNoRows):
	default:
		return err
	}
	_, err = tx.Exec(`INSERT INTO agent_conversations(id, document_id, title, messages, reasoning, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title, messages=excluded.messages, reasoning=excluded.reasoning, updated_at=excluded.updated_at`,
		conversation.ID, conversation.DocumentID, conversation.Title, conversation.Messages, conversation.Reasoning,
		conversation.CreatedAt.UTC().UnixNano(), conversation.UpdatedAt.UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("save agent conversation: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ListAgentConversations(documentID string) ([]AgentConversationSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workspace store is not open")
	}
	if documentID == "" {
		return nil, errors.New("document ID is required")
	}
	rows, err := s.db.Query(`SELECT id, title, created_at, updated_at FROM agent_conversations WHERE document_id=? ORDER BY updated_at DESC, id DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AgentConversationSummary, 0)
	for rows.Next() {
		var item AgentConversationSummary
		var createdAt, updatedAt int64
		if err := rows.Scan(&item.ID, &item.Title, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		item.UpdatedAt = time.Unix(0, updatedAt).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) LoadAgentConversation(documentID, id string) (AgentConversation, bool, error) {
	if s == nil || s.db == nil {
		return AgentConversation{}, false, errors.New("workspace store is not open")
	}
	if documentID == "" || id == "" {
		return AgentConversation{}, false, errors.New("document ID and conversation ID are required")
	}
	var conversation AgentConversation
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT id, document_id, title, messages, reasoning, created_at, updated_at FROM agent_conversations WHERE id=?`, id).Scan(
		&conversation.ID, &conversation.DocumentID, &conversation.Title, &conversation.Messages, &conversation.Reasoning, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentConversation{}, false, nil
	}
	if err != nil {
		return AgentConversation{}, false, err
	}
	if conversation.DocumentID != documentID {
		return AgentConversation{}, false, errors.New("conversation belongs to a different document")
	}
	conversation.CreatedAt = time.Unix(0, createdAt).UTC()
	conversation.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return conversation, true, nil
}

// DeleteAgentConversation removes only the conversation owned by documentID.
func (s *Store) DeleteAgentConversation(documentID, id string) error {
	if s == nil || s.db == nil {
		return errors.New("workspace store is not open")
	}
	if documentID == "" || id == "" {
		return errors.New("document ID and conversation ID are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	err = tx.QueryRow(`SELECT document_id FROM agent_conversations WHERE id=?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != documentID {
		return errors.New("conversation belongs to a different document")
	}
	if _, err = tx.Exec(`DELETE FROM agent_conversations WHERE document_id=? AND id=?`, documentID, id); err != nil {
		return err
	}
	return tx.Commit()
}
