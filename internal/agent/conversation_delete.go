package agent

import (
	"context"
	"errors"
)

// DeleteConversation serializes deletion with turn startup. Busy conversations
// must finish cancellation before deletion so their final save cannot restore them.
func (h *AgentHost) DeleteConversation(ctx context.Context, documentID string, id ConversationID) error {
	if h == nil {
		return ErrHostClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrHostClosed
	}
	if state := h.conversations[id]; state != nil {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.documentID != documentID {
			return ErrConversationScopeMismatch
		}
		if state.busy {
			return ErrConversationBusy
		}
	}
	store, ok := h.store.(interface {
		Delete(context.Context, string, ConversationID) error
	})
	if !ok {
		return errors.New("conversation deletion is unavailable")
	}
	if err := store.Delete(ctx, documentID, id); err != nil {
		return err
	}
	if h.deletedConversations == nil {
		h.deletedConversations = make(map[ConversationID]struct{})
	}
	h.deletedConversations[id] = struct{}{}
	delete(h.conversations, id)
	return nil
}
