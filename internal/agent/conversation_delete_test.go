package agent

import (
	"context"
	"testing"
	"time"
)

func (s *memoryConversationStore) Delete(ctx context.Context, documentID string, id ConversationID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.items[id]; ok && r.DocumentID != documentID {
		return ErrConversationScopeMismatch
	}
	delete(s.items, id)
	return ctx.Err()
}
func TestDeleteConversationRejectsBusyAndForgetsCachedContext(t *testing.T) {
	s := newMemoryConversationStore()
	s.items["chat"] = ConversationRecord{ID: "chat", DocumentID: "doc"}
	h := &AgentHost{store: s, conversations: map[ConversationID]*conversationState{"chat": {documentID: "doc", busy: true}}}
	if err := h.DeleteConversation(context.Background(), "doc", "chat"); err != ErrConversationBusy {
		t.Fatalf("busy delete=%v", err)
	}
	h.conversations["chat"].busy = false
	if err := h.DeleteConversation(context.Background(), "other", "chat"); err != ErrConversationScopeMismatch {
		t.Fatalf("scope delete=%v", err)
	}
	if err := h.DeleteConversation(context.Background(), "doc", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Snapshot("chat"); ok {
		t.Fatal("deleted cached context retained")
	}
	if _, found, _ := s.Load(context.Background(), "doc", "chat"); found {
		t.Fatal("stored context retained")
	}
}

type gatedDeleteStore struct {
	*memoryConversationStore
	loaded  chan struct{}
	release chan struct{}
}

func (s *gatedDeleteStore) Load(ctx context.Context, doc string, id ConversationID) (ConversationRecord, bool, error) {
	r, found, err := s.memoryConversationStore.Load(ctx, doc, id)
	close(s.loaded)
	<-s.release
	return r, found, err
}
func TestDeleteSerializesWithConversationLoading(t *testing.T) {
	s := &gatedDeleteStore{memoryConversationStore: newMemoryConversationStore(), loaded: make(chan struct{}), release: make(chan struct{})}
	s.items["chat"] = ConversationRecord{ID: "chat", DocumentID: "doc"}
	h := &AgentHost{store: s, conversations: make(map[ConversationID]*conversationState)}
	loaded := make(chan struct{})
	go func() { h.LoadConversation(context.Background(), "doc", "chat"); close(loaded) }()
	<-s.loaded
	deleted := make(chan error, 1)
	go func() { deleted <- h.DeleteConversation(context.Background(), "doc", "chat") }()
	select {
	case err := <-deleted:
		close(s.release)
		<-loaded
		t.Fatalf("delete overtook storage read/cache install: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(s.release)
	<-loaded
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Snapshot("chat"); ok {
		t.Fatal("load resurrected deleted context")
	}
}
